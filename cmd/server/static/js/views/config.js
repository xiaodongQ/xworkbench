// ===== 系统配置 Tab =====
// 2 个导入/导出子 tab：📁 快捷目录 / 🔗 快捷链接
// 1 个设置子 tab：🤖 默认 CLI
//
// 每个导入/导出子 tab 分"导出"和"导入"两个面板。
// 导入只支持：粘贴 JSON 文本 + 后端解析（不再有 AI 对话 / 后台任务入口）。
//
// 依赖 api.js (fetchJSON/esc/fmt)

// ---- 状态 ----
let currentCfgTab = 'dir_shortcuts'; // dir_shortcuts | web_links | default_cli
let _cfgPreviewCache = null;     // 最近一次 preview 结果，供"确认导入"使用

const CFG_TAB_LABEL = {
  dir_shortcuts: '快捷目录',
  web_links: '快捷链接',
  default_cli: '默认 CLI',
};

// 子 tab  →  后端 type（export/import 用）
const CFG_TYPE_FOR_TAB = {
  dir_shortcuts: 'dir_shortcuts',
  web_links: 'web_links',
};

// 必填字段在预览时高亮
const CFG_REQUIRED = {
  dir_shortcuts: ['name', 'path'],
  web_links: ['name', 'url'],
};

async function loadConfig() {
  // 首次进入读 localStorage 恢复子 tab（仅用于屏幕截图 / 从快捷入口跳转）
  const saved = localStorage.getItem('cfg-active-tab');
  // 旧版 "shortcuts" 已拆为 dir_shortcuts / web_links，落到 dir_shortcuts 保留"上次的视图"
  const resolved = (saved === 'shortcuts') ? 'dir_shortcuts' : saved;
  if (resolved && CFG_TAB_LABEL[resolved]) {
    switchCfgTab(resolved);
    return;
  }
  // 初次进入只刷新"导出区摘要"
  await refreshExportSummary();
}

// ---- 子 tab 切换 ----
function switchCfgTab(tab) {
  if (!CFG_TAB_LABEL[tab]) return;
  currentCfgTab = tab;
  localStorage.setItem('cfg-active-tab', tab);
  document.querySelectorAll('.cfg-tab-btn').forEach(b => b.classList.toggle('active', b.dataset.tab === tab));
  document.querySelectorAll('.cfg-tab-panel').forEach(p => p.classList.toggle('hidden', p.dataset.tab !== tab));
  _cfgPreviewCache = null;
  document.querySelectorAll('.cfg-preview-area').forEach(el => el.innerHTML = '');
  if (tab === 'default_cli') {
    loadPreferredCLI();
  } else {
    refreshExportSummary();
  }
}

function _activePanel() {
  return document.querySelector(`.cfg-tab-panel[data-tab="${currentCfgTab}"]`);
}

// ---- 导出 ----
async function refreshExportSummary() {
  const type = CFG_TYPE_FOR_TAB[currentCfgTab];
  if (!type) return;
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

// ---- 导入：粘贴 JSON 文本 ----
function _readImportText(tab) {
  // tab 形如 'shortcuts'/'experiences'/'tasks'
  const panel = document.querySelector(`.cfg-tab-panel[data-tab="${tab}"]`);
  if (!panel) return '';
  const ta = panel.querySelector('.cfg-import-text');
  return ta ? ta.value.trim() : '';
}

function _readDedupe(tab) {
  const panel = document.querySelector(`.cfg-tab-panel[data-tab="${tab}"]`);
  if (!panel) return 'skip';
  const sel = panel.querySelector('.cfg-import-dedupe');
  return sel ? sel.value : 'skip';
}

function _setImportResult(tab, html) {
  const panel = document.querySelector(`.cfg-tab-panel[data-tab="${tab}"]`);
  if (!panel) return;
  const el = panel.querySelector('.cfg-preview-area');
  if (el) el.innerHTML = html;
}

async function previewImport(tab) {
  const type = CFG_TYPE_FOR_TAB[tab];
  if (!type) return;
  const raw = _readImportText(tab);
  if (!raw) { alert('请先粘贴 JSON 到上面的输入框'); return; }

  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (e) {
    alert('JSON 解析失败：' + e.message);
    return;
  }

  // 兼容 3 种形态：纯数组 / 完整导出包 / {items: [...]}
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

  _setImportResult(tab, '<div style="padding:20px;color:var(--text-secondary)">⏳ 解析中…</div>');
  try {
    const result = await fetchJSON(API + '/api/config/import/preview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type, items }),
    });
    _cfgPreviewCache = { type, items, tab };
    renderPreview(tab, result);
  } catch (e) {
    _setImportResult(tab, '');
    alert('预览失败：' + e.message);
  }
}

function renderPreview(tab, result) {
  if (!result.items || result.items.length === 0) {
    _setImportResult(tab, '<div style="color:var(--text-secondary);padding:12px">无数据</div>');
    return;
  }
  const validCount = result.items.filter(i => i.valid).length;
  const dupCount = result.items.filter(i => i.valid && i.reason && i.reason.startsWith('已存在')).length;
  _setImportResult(tab, `
    <div style="display:flex;gap:16px;align-items:center;margin-bottom:12px;flex-wrap:wrap">
      <span>共 <b>${result.total}</b> 条</span>
      <span style="color:#16a34a">✓ ${validCount} 条可导入</span>
      ${dupCount > 0 ? `<span style="color:#f59e0b">⚠ ${dupCount} 条已存在</span>` : ''}
      <span style="flex:1"></span>
      <span style="color:var(--text-secondary);font-size:12px">dedupe: ${esc(_readDedupe(tab))}</span>
      <button class="btn btn-small" onclick="cancelPreview('${tab}')">取消</button>
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
  `);
}

function cancelPreview(tab) {
  _cfgPreviewCache = null;
  _setImportResult(tab, '');
}

async function confirmImport(tab) {
  if (!_cfgPreviewCache || _cfgPreviewCache.tab !== tab) {
    alert('请先点"预览"');
    return;
  }
  const dedupe = _readDedupe(tab);
  _setImportResult(tab, '<div style="padding:20px;color:var(--text-secondary)">⏳ 导入中…</div>');
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
    showImportResult(tab, result);
    refreshExportSummary();
  } catch (e) {
    alert('导入失败：' + e.message);
  }
}

function showImportResult(tab, result) {
  const errs = (result.errors || []).slice(0, 20);
  _setImportResult(tab, `
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
        <button class="btn btn-small" onclick="cancelPreview('${tab}')">关闭</button>
      </div>
    </div>
  `);
  _cfgPreviewCache = null;
}

// ---- 🤖 默认 CLI ----
async function loadPreferredCLI() {
  try {
    const r = await fetchJSON(API + '/api/config');
    const v = r.preferred_cli || 'claude';
    document.querySelectorAll('input[name="cfg-pref-cli"]').forEach(r => { r.checked = (r.value === v); });
    document.getElementById('cfg-pref-cli-status').textContent = `当前: ${v} · 来源: config.json`;
  } catch (e) {
    document.getElementById('cfg-pref-cli-status').textContent = '加载失败：' + e.message;
  }
}

async function savePreferredCLI() {
  const checked = document.querySelector('input[name="cfg-pref-cli"]:checked');
  if (!checked) { alert('请先选一个 CLI'); return; }
  const v = checked.value;
  const status = document.getElementById('cfg-pref-cli-status');
  status.textContent = '保存中…';
  try {
    await fetchJSON(API + '/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ preferred_cli: v }),
    });
    // 同步更新全局缓存，让任务页运行 modal 的默认值跟着变
    window._preferredCLI = v;
    // 同步"评估"下拉（让 eval-cli-select + eval-model-select 跟随 preferred_cli）
    const evalCliSel = document.getElementById('eval-cli-select');
    if (evalCliSel) {
      evalCliSel.value = v;
      if (typeof onEvalCliChange === 'function') onEvalCliChange();
    }
    status.textContent = `已保存: ${v} · 来源: config.json`;
  } catch (e) {
    status.textContent = '保存失败：' + e.message;
  }
}

// ---- 小工具 ----
function toast(msg) {
  // 简单 alert 替代；项目里如果有 toast 函数会被覆盖
  if (typeof window.toast === 'function') { window.toast(msg); return; }
  console.log('[toast]', msg);
}
