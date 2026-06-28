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
    // 「高级设置」默认收起；只有当 skip-perm 启用时才默认展开。
    // 展开/收起状态每次进 Tab 都按后端 dangerously_skip_permissions 决定，
    // 不记 localStorage（避免「展开过但已关闭」这种过期状态）。
    loadPreferredCLI();
    loadModelDefaults();
    loadSkipPerm();
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

function _scrollToPreview(tab) {
  const panel = document.querySelector(`.cfg-tab-panel[data-tab="${tab}"]`);
  if (!panel) return;
  const el = panel.querySelector('.cfg-preview-area');
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
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
    _scrollToPreview(tab);
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
  _scrollToPreview(tab);
  if (typeof loadDirs === 'function') loadDirs();
  if (typeof loadLinks === 'function') loadLinks();
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

// loadModelDefaults 加载 4 个模型下拉（claude/cbc 的执行/评估默认）。
// 用 buildModelOptions / getDefaultModel / getEvalDefaultModel 复用 CLI_MODELS 缓存。
async function loadModelDefaults() {
  // 确保 CLI_MODELS 已加载（api.js 全局缓存）
  if (!window.CLI_MODELS || !window.CLI_MODELS.claude) {
    await loadCLIModels();
  }
  for (const cli of ['claude', 'cbc']) {
    const sel = document.getElementById('cfg-default-' + cli);
    if (sel) {
      sel.innerHTML = buildModelOptions(cli);
      const def = getDefaultModel(cli);
      sel.value = (def && sel.querySelector('option[value="' + def + '"]'))
        ? def
        : (sel.options[0] ? sel.options[0].value : '');
    }
    const evalSel = document.getElementById('cfg-eval-default-' + cli);
    if (evalSel) {
      evalSel.innerHTML = buildModelOptions(cli);
      const evalDef = getEvalDefaultModel(cli);
      evalSel.value = (evalDef && evalSel.querySelector('option[value="' + evalDef + '"]'))
        ? evalDef
        : (evalSel.options[0] ? evalSel.options[0].value : '');
    }
  }
}

// saveDefaultCLI 一次提交：preferred_cli + 4 个模型默认值。
// 替代旧的 savePreferredCLI（只写 preferred_cli）。
async function saveDefaultCLI() {
  const checked = document.querySelector('input[name="cfg-pref-cli"]:checked');
  if (!checked) { alert('请先选一个 CLI'); return; }
  const preferredCli = checked.value;
  const claudeDefault = document.getElementById('cfg-default-claude').value;
  const cbcDefault = document.getElementById('cfg-default-cbc').value;
  const claudeEval = document.getElementById('cfg-eval-default-claude').value;
  const cbcEval = document.getElementById('cfg-eval-default-cbc').value;
  // 防御：后端 handleSetConfig 收到空字符串会覆盖 Models[cli].Default 为空，
  // 4 个 select 都有 fallback（loadModelDefaults），理论上不会空，但兜底校验一下
  if (!claudeDefault || !cbcDefault || !claudeEval || !cbcEval) {
    alert('模型默认值不能为空');
    return;
  }
  const status = document.getElementById('cfg-pref-cli-status');
  status.textContent = '保存中…';
  try {
    await fetchJSON(API + '/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        preferred_cli: preferredCli,
        model_defaults: { claude: claudeDefault, cbc: cbcDefault },
        eval_model_defaults: { claude: claudeEval, cbc: cbcEval },
      }),
    });
    // 同步本地缓存：preferred_cli 同步到 window._preferredCLI
    window._preferredCLI = preferredCli;
    // 4 个模型默认值同步到 CLI_MODELS（其他页面 getDefaultModel 直接用缓存）
    if (window.CLI_MODELS) {
      if (CLI_MODELS.claude) {
        CLI_MODELS.claude.default = claudeDefault;
        CLI_MODELS.claude.eval_default = claudeEval;
      }
      if (CLI_MODELS.cbc) {
        CLI_MODELS.cbc.default = cbcDefault;
        CLI_MODELS.cbc.eval_default = cbcEval;
      }
    }
    // 同步"评估"下拉（让 eval-cli-select + eval-model-select 跟随 preferred_cli）
    const evalCliSel = document.getElementById('eval-cli-select');
    if (evalCliSel) {
      evalCliSel.value = preferredCli;
      if (typeof onEvalCliChange === 'function') onEvalCliChange();
    }
    status.innerHTML = `已保存 · <span style="color:${preferredCli==='cbc'?'#f59e0b':'#3b82f6'}">${preferredCli}</span> · 执行 <span style="color:#3b82f6">claude</span>=${claudeDefault} <span style="color:#f59e0b">cbc</span>=${cbcDefault} · 评估 <span style="color:#3b82f6">claude</span>=${claudeEval} <span style="color:#f59e0b">cbc</span>=${cbcEval}`;
  } catch (e) {
    status.textContent = '保存失败：' + e.message;
  }
}

// ===== 完全放开 CLI 权限开关（启用人机验证：勾选+点击「启用」才能打开） =====
async function loadSkipPerm() {
  const status = document.getElementById('skip-perm-status');
  const checkbox = document.getElementById('skip-perm-toggle');
  const confirmBtn = document.getElementById('skip-perm-confirm-btn');
  const toggleRow = document.getElementById('skip-perm-toggle-row');
  const summaryBadge = document.getElementById('skip-perm-summary-badge');
  try {
    const r = await fetchJSON(API + '/api/config');
    const enabled = !!r.dangerously_skip_permissions;
    checkbox.checked = enabled;
    if (enabled) {
      // 已启用：checkbox 可点（关闭不需要二次验证）
      toggleRow.style.opacity = '1';
      toggleRow.style.cursor = 'pointer';
      checkbox.disabled = false;
      confirmBtn.style.display = 'none';
      status.textContent = '已启用 · AI 可执行任意操作，请谨慎使用';
      status.style.color = 'var(--exception)';
      if (summaryBadge) { summaryBadge.textContent = '(已启用)'; summaryBadge.style.color = 'var(--exception)'; }
    } else {
      // 未启用：checkbox 禁用，只有"启用"按钮可以触发
      toggleRow.style.opacity = '0.5';
      toggleRow.style.cursor = 'not-allowed';
      checkbox.disabled = true;
      confirmBtn.style.display = '';
      status.textContent = '未启用 · 默认安全状态';
      status.style.color = 'var(--text-secondary)';
      if (summaryBadge) { summaryBadge.textContent = '(未启用)'; summaryBadge.style.color = 'var(--text-secondary)'; }
    }
    // 刷新高级设置容器：决定展开/收起 + 汇总红点
    refreshAdvancedSettingsContainer();
  } catch (e) {
    status.textContent = '加载失败：' + e.message;
  }
}

// refreshAdvancedSettingsContainer 根据子项状态决定：
// 1. 容器是否默认展开（有子项需关注则展开，否则收起）
// 2. 标题上的「有项需要关注」红点 badge
//
// 新增高级设置子项时：把该子项的"需关注判定"加到下面的 childNeedsAttention() 即可。
function refreshAdvancedSettingsContainer() {
  const details = document.getElementById('advanced-settings-details');
  const badge = document.getElementById('advanced-settings-badge');
  const summaryStatus = document.getElementById('advanced-settings-summary-status');
  if (!details) return;

  // 收集所有子项的"需关注"状态
  const children = collectAdvancedSettingsChildren();
  const attentionCount = children.filter(c => c.needsAttention).length;

  // 默认展开：只要有一个子项需要关注就展开（让用户感知到风险）
  details.open = attentionCount > 0;

  // 标题红点 + 总结文案
  if (attentionCount > 0) {
    if (badge) { badge.style.display = ''; badge.textContent = attentionCount === 1 ? '1 项需要关注' : `${attentionCount} 项需要关注`; }
    if (summaryStatus) { summaryStatus.textContent = '点击查看详情'; summaryStatus.style.color = 'var(--exception)'; }
  } else {
    if (badge) badge.style.display = 'none';
    if (summaryStatus) { summaryStatus.textContent = ''; }
  }
}

// collectAdvancedSettingsChildren 返回高级设置里所有子项的当前状态。
// 每项字段：{ id, label, needsAttention }。
// 新增子项时：在这里 push 一项即可，刷新逻辑会自动汇总。
function collectAdvancedSettingsChildren() {
  const children = [];
  // 子项 1：完全放开 CLI 权限
  // 从 checkbox 当前 checked 状态判断（最权威，因为 loadSkipPerm 已同步到 UI）
  const skipPermCheckbox = document.getElementById('skip-perm-toggle');
  const skipPermEnabled = skipPermCheckbox && skipPermCheckbox.checked;
  children.push({
    id: 'skip-perm',
    label: '完全放开 CLI 权限',
    needsAttention: !!skipPermEnabled,
  });
  return children;
}

// 点击 checkbox：已启用时 -> 直接关闭；未启用时 -> 拒绝（必须走按钮）
async function onSkipPermToggleChange(checked) {
  const status = document.getElementById('skip-perm-status');
  const checkbox = document.getElementById('skip-perm-toggle');
  if (!checked) {
    // 用户尝试关闭 -> 允许（不需要二次验证）
    await setSkipPerm(false);
    return;
  }
  // 用户尝试开启 -> 拒绝，要求走 confirmSkipPermEnable
  checkbox.checked = false;
  alert('开启该项需要点击下方的红色“启用”按钮并确认风险。');
}

async function confirmSkipPermEnable() {
  const riskText = [
    '将允许 AI 执行任意 Bash 命令、读写任意路径、跳过所有权限确认。',
    '',
    '后果包括但不限于：',
    '  · 误删 / 误改项目文件（含 .git、配置、数据）',
    '  · 发送任意网络请求（可能消耗额度 / 泄露隐私）',
    '  · 启动后台进程 / 占用资源',
    '  · 调试 / 评价失误导致不可逆操作',
    '',
    '评估员（evaluator）不受该开关影响。',
    '',
    '请确认你已了解上述风险。'
  ].join('\n');
  const confirmed = confirm('⚠️ 启用 --dangerously-skip-permissions\n\n' + riskText + '\n\n是否继续？');
  if (!confirmed) return;
  await setSkipPerm(true);
}

async function setSkipPerm(enabled) {
  const status = document.getElementById('skip-perm-status');
  const checkbox = document.getElementById('skip-perm-toggle');
  const toggleRow = document.getElementById('skip-perm-toggle-row');
  const confirmBtn = document.getElementById('skip-perm-confirm-btn');
  status.textContent = enabled ? '启用中…' : '关闭中…';
  status.style.color = 'var(--text-secondary)';
  try {
    await fetchJSON(API + '/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dangerously_skip_permissions: enabled }),
    });
    // 重新拉取以同步 UI
    await loadSkipPerm();
  } catch (e) {
    status.textContent = (enabled ? '启用' : '关闭') + '失败：' + e.message;
    status.style.color = 'var(--exception)';
    // 回滚 checkbox
    checkbox.checked = !enabled;
  }
}

// ---- 小工具 ----
function toast(msg) {
  // 简单 alert 替代；项目里如果有 toast 函数会被覆盖
  if (typeof window.toast === 'function') { window.toast(msg); return; }
  console.log('[toast]', msg);
}
