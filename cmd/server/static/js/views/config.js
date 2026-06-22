// ===== 数据管理 Tab =====
// 4 个子 tab：快捷 / 经验 / 手动任务 / 自动化任务
// 每个子 tab 分"导出"和"导入"两个面板。
// 导入支持 4 种入口：
//   1) 上传 JSON 文件
//   2) 粘贴 JSON 文本
//   3) 粘贴"自然语言文本"→ 跳 AI 对话 Tab 让 AI 调用 API 完成导入
//   4) 粘贴"自然语言文本"→ 起后台任务用 -p 异步完成导入
//
// 依赖 api.js (fetchJSON/esc/fmt)

// ---- 状态 ----
let currentCfgTab = 'shortcuts'; // shortcuts | experiences | tasks_manual | tasks_scheduled
let _cfgPreviewCache = null;     // 最近一次 preview 结果，供"确认导入"使用

const CFG_TAB_LABEL = {
  shortcuts: '快捷',
  experiences: '经验',
  tasks_manual: '手动任务',
  tasks_scheduled: '自动化任务',
};

const CFG_TYPE_FOR_TAB = {
  shortcuts: 'dir_shortcuts',
  experiences: 'experiences',
  tasks_manual: 'tasks_manual',
  tasks_scheduled: 'tasks_scheduled',
};

// 必填字段在预览时高亮
const CFG_REQUIRED = {
  dir_shortcuts: ['name', 'path'],
  web_links: ['name', 'url'],
  experiences: ['module'],
  tasks_manual: ['title'],
  tasks_scheduled: ['name', 'cron_expr', 'command_type'],
};

async function loadConfig() {
  // 首次进入读 localStorage 恢复子 tab（仅用于屏幕截图 / 从快捷入口跳转）
  const saved = localStorage.getItem('cfg-active-tab');
  if (saved && CFG_TAB_LABEL[saved]) {
    switchCfgTab(saved);
    return;
  }
  // 初次进入只刷新"导出区摘要"
  await refreshExportSummary();
}

// ---- 子 tab 切换 ----
function switchCfgTab(tab) {
  currentCfgTab = tab;
  document.querySelectorAll('.cfg-tab-btn').forEach(b => b.classList.toggle('active', b.dataset.tab === tab));
  document.querySelectorAll('.cfg-tab-panel').forEach(p => p.classList.toggle('hidden', p.dataset.tab !== tab));
  _cfgPreviewCache = null;
  document.querySelectorAll('.cfg-preview-area').forEach(el => el.innerHTML = '');
  refreshExportSummary();
}

function _activePanel() {
  return document.querySelector(`.cfg-tab-panel[data-tab="${currentCfgTab}"]`);
}

// ---- 导出 ----
async function refreshExportSummary() {
  const type = CFG_TYPE_FOR_TAB[currentCfgTab];
  try {
    const all = await fetchJSON(API + '/api/config/export');
    const arr = all[type] || [];
    const panel = _activePanel();
    if (!panel) return;
    panel.querySelectorAll('.cfg-export-summary').forEach(el => {
      el.innerHTML = `当前 <b>${CFG_TAB_LABEL[currentCfgTab]}</b> 共 <b style="color:var(--primary)">${arr.length}</b> 条`;
    });
  } catch (e) {
    console.error('refreshExportSummary', e);
  }
}

function exportJson(type) {
  // 浏览器直接走下载流程（后端设置了 Content-Disposition）
  window.open(API + '/api/config/export?types=' + encodeURIComponent(type), '_blank');
}

function exportAll() {
  window.open(API + '/api/config/export', '_blank');
}

async function copyAllToClipboard() {
  try {
    const data = await fetchJSON(API + '/api/config/export');
    const text = JSON.stringify(data, null, 2);
    await navigator.clipboard.writeText(text);
    toast('已复制完整备份到剪贴板（' + text.length + ' 字节）');
  } catch (e) {
    alert('复制失败：' + e.message);
  }
}

// ---- 导入：上传文件 / 粘贴文本 ----
function onImportFile(input) {
  const f = input.files && input.files[0];
  if (!f) return;
  const reader = new FileReader();
  reader.onload = e => {
    // 写到当前 panel 的 textarea
    const panel = input.closest('.cfg-tab-panel') || _activePanel();
    const ta = panel && panel.querySelector('.cfg-import-text');
    if (ta) ta.value = e.target.result;
    toast('文件已加载（' + f.name + '）');
  };
  reader.readAsText(f);
  input.value = '';
}

async function previewImport() {
  const type = CFG_TYPE_FOR_TAB[currentCfgTab];
  const panel = _activePanel();
  if (!panel) return;
  const ta = panel.querySelector('.cfg-import-text');
  const raw = ta ? ta.value.trim() : '';
  if (!raw) { alert('请先粘贴 JSON 或选择文件'); return; }

  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (e) {
    alert('JSON 解析失败：' + e.message);
    return;
  }

  // 兼容"完整导出包"和"单类数组"两种形态
  let items;
  if (Array.isArray(parsed)) {
    items = parsed;
  } else if (parsed && Array.isArray(parsed[type])) {
    items = parsed[type];
  } else if (parsed && parsed.items && Array.isArray(parsed.items)) {
    items = parsed.items;
  } else {
    alert('无法识别：期望一个数组，或包含 "' + type + '" 字段的对象');
    return;
  }

  try {
    const result = await fetchJSON(API + '/api/config/import/preview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type, items }),
    });
    _cfgPreviewCache = { type, items, result };
    renderPreview(result);
  } catch (e) {
    alert('预览失败：' + e.message);
  }
}

function renderPreview(result) {
  const panel = _activePanel();
  const el = panel && panel.querySelector('.cfg-preview-area');
  if (!el) return;
  if (!result.items || result.items.length === 0) {
    el.innerHTML = '<div class="empty">无数据</div>';
    return;
  }
  const validCount = result.items.filter(i => i.valid).length;
  const dupCount = result.items.filter(i => i.valid && i.reason && i.reason.startsWith('已存在')).length;
  el.innerHTML = `
    <div style="display:flex;gap:16px;align-items:center;margin-bottom:12px;flex-wrap:wrap">
      <span>共 <b>${result.total}</b> 条</span>
      <span style="color:#16a34a">✓ ${validCount} 条可导入</span>
      ${dupCount > 0 ? `<span style="color:#f59e0b">⚠ ${dupCount} 条已存在</span>` : ''}
      <span style="flex:1"></span>
      <label>去重：
        <select id="cfg-dedupe" style="padding:4px 8px;border-radius:4px;border:1px solid var(--border);background:var(--card);color:var(--text)">
          <option value="skip">跳过（推荐）</option>
          <option value="overwrite">覆盖</option>
          <option value="append">全部新增</option>
        </select>
      </label>
      <button class="btn btn-primary" onclick="confirmImport()">确认导入</button>
      <button class="btn btn-small" onclick="cancelPreview()">取消</button>
    </div>
    <table class="exp-table" style="width:100%">
      <thead><tr>
        <th style="width:50px;text-align:left">#</th>
        <th style="width:60px;text-align:left">状态</th>
        <th style="text-align:left">摘要</th>
        <th style="text-align:left">说明</th>
      </tr></thead>
      <tbody>${result.items.map(it => `
        <tr style="background:${it.valid ? (it.reason ? '#fef3c7' : 'transparent') : '#fee2e2'}">
          <td>${it.index}</td>
          <td>${it.valid ? (it.reason && it.reason.startsWith('已存在') ? '⚠️ 重复' : '✓') : '✗ 无效'}</td>
          <td>${esc(it.summary || '')}</td>
          <td style="font-size:12px;color:var(--text-secondary)">${esc(it.reason || '')}</td>
        </tr>`).join('')}
      </tbody>
    </table>
  `;
}

function cancelPreview() {
  _cfgPreviewCache = null;
  document.querySelectorAll('.cfg-preview-area').forEach(el => el.innerHTML = '');
}

async function confirmImport() {
  if (!_cfgPreviewCache) { alert('请先点"预览"'); return; }
  const dedupe = document.getElementById('cfg-dedupe').value;
  const panel = _activePanel();
  const pv = panel && panel.querySelector('.cfg-preview-area');
  if (pv) pv.innerHTML = '<div style="padding:20px;color:var(--text-secondary)">⏳ 导入中…</div>';
  try {
    const result = await fetchJSON(API + '/api/config/import', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        type: _cfgPreviewCache.type,
        items: _cfgPreviewCache.items,
        dedupe,
      }),
    });
    showImportResult(result);
    refreshExportSummary();
  } catch (e) {
    alert('导入失败：' + e.message);
  }
}

function showImportResult(result) {
  const panel = _activePanel();
  const el = panel && panel.querySelector('.cfg-preview-area');
  const errs = (result.errors || []).slice(0, 20);
  el.innerHTML = `
    <div style="padding:16px;background:var(--card);border-radius:6px;border:1px solid var(--border)">
      <h3 style="margin:0 0 12px">导入完成</h3>
      <div style="display:flex;gap:20px;font-size:14px">
        <span style="color:#16a34a">✓ 新增 <b>${result.created}</b></span>
        ${result.updated > 0 ? `<span style="color:#0ea5e9">↻ 覆盖 <b>${result.updated}</b></span>` : ''}
        ${result.skipped > 0 ? `<span style="color:#94a3b8">⤴ 跳过 <b>${result.skipped}</b></span>` : ''}
        ${errs.length > 0 ? `<span style="color:#dc2626">✗ 失败 <b>${errs.length}</b></span>` : ''}
      </div>
      ${errs.length > 0 ? `
        <details style="margin-top:12px">
          <summary style="cursor:pointer;color:var(--text-secondary)">失败明细（最多 20 条）</summary>
          <ul style="margin-top:8px;font-size:12px;color:var(--text-secondary)">
            ${errs.map(e => `<li>#${e.index}: ${esc(e.reason)}</li>`).join('')}
          </ul>
        </details>
      ` : ''}
      <div style="margin-top:12px">
        <button class="btn btn-small" onclick="cancelPreview()">关闭</button>
      </div>
    </div>
  `;
  _cfgPreviewCache = null;
}

// ---- AI 自然语言触发：两路 ----

// 通用提示词模板：告诉 AI 当前 type、要调用的接口、期望产出
function buildAIPrompt(type, userText) {
  return [
    '你需要把下面这段自然语言文本解析为 ' + type + ' 类型的 JSON 数组，并通过 HTTP 接口导入到 xworkbench。',
    '',
    '接口：POST /api/config/import',
    '请求体：',
    '{',
    '  "type": "' + type + '",',
    '  "items": [...],   // 数组，每个元素是 ' + type + ' 的完整 JSON 对象',
    '  "dedupe": "append"  // 默认 append，全部新增',
    '}',
    '',
    '必填字段：',
    ...((CFG_REQUIRED[type] || []).map(f => '  - ' + f)),
    '',
    '执行步骤：',
    '1) 把下面的文本解析为符合要求的 items 数组；',
    '2) 用 curl 调用 POST /api/config/import 真正导入；',
    '3) 把导入结果（created/updated/skipped/errors）总结给我。',
    '',
    '---- 待解析的文本 ----',
    userText,
  ].join('\n');
}

// 路径 A：跳 AI 对话 Tab，把 prompt 写到 PTY stdin
async function aiChatImport() {
  const panel = _activePanel();
  const ta = panel && panel.querySelector('.cfg-import-text');
  const userText = ta ? ta.value.trim() : '';
  if (!userText) { alert('请先把要解析的文本粘到上面的框里'); return; }
  const type = CFG_TYPE_FOR_TAB[currentCfgTab];
  const prompt = buildAIPrompt(type, userText);

  // 切到 AI 对话 Tab
  if (typeof switchTab === 'function') switchTab('aichat');
  // 等 PTY 连上
  await new Promise(r => setTimeout(r, 600));
  // 找 PTY ws 注入
  // aichat.js 把 ws 暴露在 window._ptyWs 或局部变量 ptyWs
  const ws = (typeof ptyWs !== 'undefined' && ptyWs) || window._ptyWs;
  if (ws && ws.readyState === 1) {
    ws.send(prompt + '\n');
    toast('已把导入指令发到 AI 对话 Tab');
  } else {
    alert('PTY 未连接，请在 AI 对话 Tab 等 xterm 起来后重试');
  }
}

// 路径 B：起后台 -p 任务
async function aiTaskImport() {
  const panel = _activePanel();
  const ta = panel && panel.querySelector('.cfg-import-text');
  const userText = ta ? ta.value.trim() : '';
  if (!userText) { alert('请先把要解析的文本粘到上面的框里'); return; }
  const type = CFG_TYPE_FOR_TAB[currentCfgTab];
  const prompt = buildAIPrompt(type, userText);

  // 截断标题避免太长
  const title = '[导入]' + CFG_TAB_LABEL[currentCfgTab] + ': ' + userText.slice(0, 40).replace(/\s+/g, ' ');
  try {
    // 1) 建任务
    const task = await fetchJSON(API + '/api/tasks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        title,
        description: prompt,
        task_type: 'manual',
        priority: 5,
      }),
    });
    // 2) 跑（默认 claude）
    const run = await fetchJSON(API + '/api/tasks/' + task.id + '/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ command_type: 'claude', model: 'sonnet' }),
    });
    toast('已起后台导入任务: ' + task.id + '（execution_id=' + run.execution_id + '）');
    if (confirm('跳到"⚡ 自动化 Tab"查看导入进度吗？')) {
      switchTab('automation');
      if (typeof manualRefresh === 'function') manualRefresh();
    }
  } catch (e) {
    alert('后台任务创建失败：' + e.message);
  }
}

// ---- 小工具 ----
function toast(msg) {
  // 简单 alert 替代；项目里如果有 toast 函数会被覆盖
  if (typeof window.toast === 'function') { window.toast(msg); return; }
  console.log('[toast]', msg);
}
