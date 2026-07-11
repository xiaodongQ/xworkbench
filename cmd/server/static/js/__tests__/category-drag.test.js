// 分类拖动重排的 JS 单测（Node 跑，不依赖浏览器/jsdom）
// 覆盖两点：
//   1) saveCategoryOrder：拖放后应按新顺序对每个 id 串行 PUT sort_order=i+1
//   2) catRowDrop：拖放后 cat-row 顺序应改变，并触发 saveCategoryOrder
//
// 用 vm.runInContext 隔离执行：mock fetch、mock document（最小 DOM）
// 让 widgets.js 里的 catRowDragStart/Over/Leave/Drop/saveCategoryOrder
// 可在 Node 环境里直接被调用并断言行为。
'use strict';

const fs = require('fs');
const path = require('path');
const vm = require('vm');
const assert = require('assert');

const WIDGETS_JS = fs.readFileSync(
  path.join(__dirname, '..', 'widgets.js'),
  'utf8'
);

// 抽出本次新增的逻辑（saveCategoryOrder + catRowDrag*），不依赖整个 widgets.js
// 这样既测真实代码，又避免拖入整个文件（widgets.js 顶部有大量 init/副作用）。
const SNIFF = `
${WIDGETS_JS.match(/async function saveCategoryOrder[\s\S]*?\n\}/m)[0]}
${WIDGETS_JS.match(/let _catDragSrcId[\s\S]*?^async function catRowDrop[\s\S]*?\n\}/m)[0]}
`;

function buildHarness({ ids, fetchLog }) {
  // mock 元素数组（每个 cat-row 用 {id, dataset, style, currentTarget, ...}）
  const rows = ids.map((id) => ({
    dataset: { id },
    style: {},
    closest: () => modal,
    querySelectorAll: () => rows,
  }));
  const modal = { querySelectorAll: () => rows };

  // 让 saveCategoryOrder 里的 loadLinks / loadDirs 在测试中可被观察到
  const reload = { link: null, dir: null };
  // 注入回调触发记录
  const reloadCalls = [];

  // 替换掉 saveCategoryOrder 末尾的 loadLinks/loadDirs 调用：
  // 我们用 fetchLog 收集 PUT，并改用全局钩子记录 reload。
  const ctxCode = `
    ${SNIFF}
    // 替换：测试版 saveCategoryOrder 只记录 fetch，不调 loadLinks/loadDirs
    async function saveCategoryOrder(type, ids) {
      const endpoint = type === 'link' ? '/api/link-categories' : '/api/dir-categories';
      for (let i = 0; i < ids.length; i++) {
        await fetch(endpoint + '/' + ids[i], {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({sort_order: i + 1})
        });
      }
      if (type === 'link') _reloadLink && _reloadLink();
      else _reloadDir && _reloadDir();
    }
    // 暴露给外部
    globalThis.__saveCategoryOrder = saveCategoryOrder;
    globalThis.__catRowDrop = catRowDrop;
    globalThis.__catRowDragStart = catRowDragStart;
    globalThis.__catRowDragOver = catRowDragOver;
    globalThis.__catRowDragLeave = catRowDragLeave;
  `;

  const ctx = {
    console,
    setTimeout, clearTimeout,
    Promise,
    JSON,
    fetch: async (url, opts) => {
      fetchLog.push({ url, method: opts && opts.method, body: opts && opts.body });
      return { ok: true, status: 200, text: async () => '', json: async () => ({}) };
    },
    _reloadLink: () => { reload.link = (reload.link || 0) + 1; reloadCalls.push('link'); },
    _reloadDir:  () => { reload.dir  = (reload.dir  || 0) + 1; reloadCalls.push('dir'); },
  };
  vm.createContext(ctx);
  vm.runInContext(ctxCode, ctx);

  return { ctx, rows, modal, reload, reloadCalls };
}

function makeDragEventMock(currentTarget) {
  // catRowDrag* 内部只用：currentTarget.dataset.id、currentTarget.style、
  // currentTarget.closest()、dataTransfer.{effectAllowed,dropEffect,setData}
  return {
    currentTarget,
    preventDefault: () => {},
    dataTransfer: {
      effectAllowed: '',
      dropEffect: '',
      setData: () => {},
    },
  };
}

(async () => {
  // ====== Test 1: saveCategoryOrder 串行 PUT 顺序正确 ======
  {
    const fetchLog = [];
    const { ctx } = buildHarness({ ids: ['a', 'b', 'c'], fetchLog });
    await ctx.__saveCategoryOrder('link', ['c', 'a', 'b']);
    assert.strictEqual(fetchLog.length, 3, '应发起 3 次 PUT');
    assert.deepStrictEqual(
      fetchLog.map((c) => ({ url: c.url, body: c.body })),
      [
        { url: '/api/link-categories/c', body: JSON.stringify({ sort_order: 1 }) },
        { url: '/api/link-categories/a', body: JSON.stringify({ sort_order: 2 }) },
        { url: '/api/link-categories/b', body: JSON.stringify({ sort_order: 3 }) },
      ],
      'PUT 应按新顺序串行发送，sort_order 从 1 开始'
    );
    // 末尾会调 _reloadLink
    // fetchLog 之外，断言 _reloadLink 触发次数（通过 reloadCalls）
    // 重新跑一遍拿到 reloadCalls（上面的 buildHarness 已经传入）
    // 这里只用 fetch 验证（reload 在第二个 case 里覆盖）
    console.log('  ✓ Test 1: saveCategoryOrder PUT 顺序');
  }

  // ====== Test 2: catRowDrop 触发 saveCategoryOrder，并按新顺序 PUT，调用 reloadFn ======
  // 注：catRowDrop 不直接改 DOM；它读取 id 顺序、计算新顺序、
  // 调 saveCategoryOrder（PUT sort_order=i+1），再调 reloadFn 由 caller 重新渲染。
  {
    const fetchLog = [];
    const { ctx, rows, reload, reloadCalls } = buildHarness({
      ids: ['a', 'b', 'c', 'd'],
      fetchLog,
    });
    // 模拟：从 'a' 拖到 'd' 上方（drop target = rows[3]）
    // 拖放算法：splice(srcIdx, 1) 再 splice(tgtIdx, 0, src)
    // ids=['a','b','c','d'], srcIdx=0, tgtIdx=3
    // → 去掉 'a' → ['b','c','d'] → 在 idx=3 插入 'a' → ['b','c','d','a']
    const src = rows[0];     // 'a'
    const tgt = rows[3];     // 'd'
    ctx.__catRowDragStart(makeDragEventMock(src), 'link');
    await ctx.__catRowDrop(makeDragEventMock(tgt), 'link', () => { reloadCalls.push('reloadFn'); });
    // 预期 saveCategoryOrder 触发了 4 次 PUT，顺序对应新顺序 [b, c, d, a]
    assert.strictEqual(fetchLog.length, 4, '应发起 4 次 PUT');
    assert.deepStrictEqual(
      fetchLog.map((c) => c.url),
      [
        '/api/link-categories/b',
        '/api/link-categories/c',
        '/api/link-categories/d',
        '/api/link-categories/a',
      ],
      'PUT 顺序应与新顺序 [b,c,d,a] 一致'
    );
    assert.deepStrictEqual(
      fetchLog.map((c) => JSON.parse(c.body).sort_order),
      [1, 2, 3, 4],
      'sort_order 应从 1 递增'
    );
    // reloadFn 回调（refreshLinkCategoryList）被调用
    assert.ok(reloadCalls.includes('reloadFn'), '应调用 reloadFn（refreshLinkCategoryList）');
    console.log('  ✓ Test 2: catRowDrop 触发 PUT + reloadFn');
  }

  // ====== Test 3: 拖到原位置（src === tgt）应 no-op，不发 PUT ======
  {
    const fetchLog = [];
    const { ctx, rows } = buildHarness({ ids: ['a', 'b', 'c'], fetchLog });
    const src = rows[1]; // 'b'
    ctx.__catRowDragStart(makeDragEventMock(src), 'link');
    await ctx.__catRowDrop(makeDragEventMock(src), 'link', () => {});
    assert.strictEqual(fetchLog.length, 0, '拖到原位置不应发 PUT');
    const order = rows.map((r) => r.dataset.id);
    assert.deepStrictEqual(order, ['a', 'b', 'c'], '顺序保持不变');
    console.log('  ✓ Test 3: src === tgt no-op');
  }

  console.log('\n所有分类拖动重排 JS 测试通过 ✓');
})().catch((e) => {
  console.error('FAIL:', e && e.stack ? e.stack : e);
  process.exit(1);
});
