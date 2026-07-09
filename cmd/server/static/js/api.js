// ===== 共享 API 客户端 + DOM 工具 + Tab 切换 =====
// 5 个 view JS 都依赖这里的工具函数

const API = '';
let currentTab = 'dashboard';

// ===== HTML 转义工具（防 XSS）=====
function escapeHtml(s) {
  if (s === null || s === undefined) return '';
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// ===== 全局 CLI + 模型列表（从后端加载） =====
let CLI_MODELS = null;

async function loadCLIModels() {
  if (CLI_MODELS) return CLI_MODELS;
  try {
    const data = await fetchJSON('/api/models');
    CLI_MODELS = data.cli_type_models || {claude: [], cbc: []};
  } catch (e) {
    console.error('loadCLIModels failed:', e);
    CLI_MODELS = {claude: [], cbc: []};
  }
  return CLI_MODELS;
}

function buildModelOptions(cliType) {
  if (!CLI_MODELS) return '';
  const group = CLI_MODELS[cliType] || CLI_MODELS.claude;
  const models = (group && group.options) ? group.options : [];
  return models.map(m => '<option value="' + m.value + '">' + m.label + '</option>').join('');
}

function getDefaultModel(cliType) {
  if (!CLI_MODELS) return '';
  const group = CLI_MODELS[cliType] || CLI_MODELS.claude;
  return (group && group.default) ? group.default : '';
}

// getEvalDefaultModel 评估默认模型：eval_default → default → ''
function getEvalDefaultModel(cliType) {
  if (!CLI_MODELS) return '';
  const group = CLI_MODELS[cliType] || CLI_MODELS.claude;
  if (!group) return '';
  return group.eval_default || group.default || '';
}

async function saveDefaultModel(cliType, model) {
  try {
    await fetchJSON('/api/config', {
      method: 'PUT',
      body: JSON.stringify({ model_defaults: { [cliType]: model } }),
    });
    // 更新本地缓存
    if (CLI_MODELS && CLI_MODELS[cliType]) {
      CLI_MODELS[cliType].default = model;
    }
  } catch (e) {
    console.warn('保存默认模型失败:', e);
  }
}

async function saveEvalDefaultModel(cliType, model) {
  try {
    await fetchJSON('/api/config', {
      method: 'PUT',
      body: JSON.stringify({ eval_model_defaults: { [cliType]: model } }),
    });
    if (CLI_MODELS && CLI_MODELS[cliType]) {
      CLI_MODELS[cliType].eval_default = model;
    }
  } catch (e) {
    console.warn('保存评估默认模型失败:', e);
  }
}

async function fetchJSON(url, opts) {
  const r = await fetch(url, opts);
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

// 全局缓存系统设置：preferred_cli、ai_loop_enabled 等。
// 页面加载时拉一次，后续使用 window._preferredCLI 等变量访问。
// 同时同步"评估"下拉（eval-cli-select + eval-model-select），改 preferred_cli 后评估默认跟随。
async function loadSystemSettings() {
  try {
    const cfg = await fetchJSON(API + '/api/config');
    window._preferredCLI = cfg.preferred_cli || 'claude';
    // 同步评估下拉（如元素已存在）
    if (typeof loadCLIModels === 'function') await loadCLIModels();
    const cliSel = document.getElementById('eval-cli-select');
    if (cliSel) {
      cliSel.value = window._preferredCLI;
      if (typeof onEvalCliChange === 'function') onEvalCliChange();
    }
  } catch (e) {
    window._preferredCLI = 'claude';
  }
}
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', loadSystemSettings);
} else {
  loadSystemSettings();
}

function statusTag(status) {
  const labels = {pending: '待认领', in_progress: '待执行', running: '执行中', archived: '已完成', exception: '异常', waiting_input: '待交互'};
  return `<span class="status-pill status-${status}">${labels[status] || status}</span>`;
}

function taskTypeTag(type) {
  const labels = {manual: '本地', scheduled: '定时', remote: '远程'};
  const colors = {manual: '#64748b', scheduled: '#0ea5e9', remote: '#8b5cf6'};
  const t = type || 'manual';
  return `<span style="font-size:11px;padding:2px 6px;border-radius:3px;background:${colors[t] || '#64748b'};color:#fff">${labels[t] || t}</span>`;
}

// loopStatusTag 根据任务的 iteration_count / evaluation_score / max_iterations 显示 loop 状态标签
function loopStatusTag(task) {
  const iter = task.iteration_count || 0;
  const max = task.max_iterations || 0;
  const score = task.evaluation_score;
  if (iter === 0) return '<span style="color:var(--text-secondary);font-size:11px">—</span>';
  const passed = score != null && task.improvement_threshold != null && score >= task.improvement_threshold;
  const icon = passed ? '✓' : '✗';
  const color = passed ? '#22c55e' : (score != null ? '#ef4444' : '#9ca3af');
  return `<span style="font-size:11px;color:${color}" title="优化 ${iter} 轮${score != null ? '· score=' + score.toFixed(1) : ''}">${icon} ${iter}/${max}</span>`;
}

function fmt(ts) {
  if (!ts) return '-';
  const d = new Date(ts);
  return d.toLocaleDateString('zh-CN', {year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'});
}

function esc(s) {
  if (!s) return '';
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function debounce(fn, ms) {
  let t;
  return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms); };
}

function switchTab(tab) {
  // 离开当前 tab 前的清理
  if (currentTab === 'dashboard' && typeof stopDashboardAutoRefresh === 'function') stopDashboardAutoRefresh();
  currentTab = tab;
  localStorage.setItem('sf-current-tab', tab);
  document.querySelectorAll('.nav-item').forEach(n => n.classList.toggle('active', n.dataset.tab === tab));
  document.querySelectorAll('.main > div').forEach(p => p.classList.toggle('hidden', p.id !== 'page-' + tab));
  if (tab === 'dashboard' && typeof loadDashboard === 'function') loadDashboard();
  if (tab === 'tasks' && typeof loadTasks === 'function') {
    try { loadTasks(); } catch(e) { console.error('[loadTasks error]', e); }
  }
  if (tab === 'experiences' && typeof loadExps === 'function') loadExps();
  if (tab === 'automation' && typeof loadAutomation === 'function') { loadAutomation(); if (typeof loadTerminalSetting === 'function') loadTerminalSetting(); }
  if (tab === 'aichat' && typeof renderAICheat === 'function') renderAICheat(document.getElementById('aichat-root'));
  if (tab === 'rterm' && typeof initRptyTabOnFirstVisit === 'function') initRptyTabOnFirstVisit();
  if (tab === 'relay' && typeof loadRelayStats === 'function') { loadRelayStats(); if (typeof loadAgents === 'function') loadAgents(); }
  if (tab === 'config' && typeof loadConfig === 'function') loadConfig();
}

// 初始化：恢复上次停留的 tab（移到 index.html init 脚本末尾执行，依赖所有 view JS 加载完）

function reloadCurrentTab() {
  if (currentTab === 'dashboard') loadDashboard();
  else if (currentTab === 'tasks') loadTasks();
  else if (currentTab === 'experiences') loadExps();
  else if (currentTab === 'automation') { if (typeof loadAutomation === 'function') loadAutomation(); }
}

// 全局 ESC 关 modal
document.addEventListener('keydown', e => {
  if (e.key === 'Escape') {
    const execDetail = document.getElementById('exec-detail-modal');
    // 如果执行详情弹窗开着，只关它；避免连同外层的 task-modal 一起关掉
    if (execDetail && !execDetail.classList.contains('hidden')) {
      execDetail.classList.add('hidden');
      return;
    }
    document.querySelectorAll('.modal-overlay').forEach(m => m.classList.add('hidden'));
  }
});

// ===== 主题切换（localStorage 持久化 + 跟随系统偏好） =====
function loadTheme() {
  let theme = localStorage.getItem('sf-theme');
  if (!theme) {
    theme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  document.documentElement.setAttribute('data-theme', theme);
  updateThemeBtn(theme);
}
function toggleTheme() {
  const cur = document.documentElement.getAttribute('data-theme') || 'light';
  const next = cur === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-theme', next);
  localStorage.setItem('sf-theme', next);
  updateThemeBtn(next);
}
function updateThemeBtn(theme) {
  const el = document.getElementById('theme-btn');
  if (el) el.querySelector('.nav-icon').textContent = theme === 'dark' ? '☀️' : '🌙';
  const txt = el && el.querySelector('.theme-btn-text');
  if (txt) txt.textContent = theme === 'dark' ? '浅色' : '深色';
}
loadTheme();

// ===== 自定义 tooltip（真实 DOM，\n 真换行） =====
// positionTooltip 把 tip 放到 anchorRect 旁边：水平优先右侧（放不下翻左侧），
// 垂直对齐元素中点（受 transform translateY(-50%) 影响，最终视觉中心对齐元素中心）。
// 视口上下边缘自动内缩 8px，避免被屏幕边界裁掉。
function positionTooltip(tip, anchorRect) {
  const margin = 8;
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  // 先重置到 (0,0) + show 触发布局,offsetWidth/Height 才有真实尺寸
  // （否则上一次定位的值会让浏览器读取 stale geometry）
  tip.style.left = '0px';
  tip.style.top = '0px';
  tip.classList.add('show');
  const tipW = tip.offsetWidth;
  const tipH = tip.offsetHeight;
  // 水平:右侧放不下翻到左侧（贴在 anchor 左边外 8px）
  let left = anchorRect.right + margin;
  if (left + tipW + margin > vw) {
    left = Math.max(margin, anchorRect.left - tipW - margin);
  }
  // 垂直:对齐元素中点（注意 transform 是 translateY(-50%)，top 是 tip 中心位置）
  let top = anchorRect.top + anchorRect.height / 2;
  if (top - tipH / 2 < margin) top = margin + tipH / 2;
  if (top + tipH / 2 + margin > vh) top = vh - margin - tipH / 2;
  tip.style.left = left + 'px';
  tip.style.top = top + 'px';
}

let _tooltipEl = null;
function ensureTooltip() {
  if (_tooltipEl) return _tooltipEl;
  _tooltipEl = document.createElement('div');
  _tooltipEl.className = 'custom-tooltip';
  document.body.appendChild(_tooltipEl);
  return _tooltipEl;
}
function showTooltip(target) {
  let text = target.dataset.tooltip;
  if (!text) return;
  // 兜底：HTML 字面 "\n"（反斜杠+n）→ 真换行
  text = text.replace(/\\n/g, '\n');
  const tip = ensureTooltip();
  // 继承触发元素的类（如 tooltip-narrow），便于 CSS 控制宽度等样式
  tip.className = 'custom-tooltip' + (target.className ? ' ' + target.className : '');
  // 按真换行拆行
  const lines = text.split('\n');
  tip.innerHTML = lines.map((l, i) =>
    i === 0 ? `<span class="tt-line tt-head">${esc(l)}</span>` : `<span class="tt-line">${esc(l)}</span>`
  ).join('');
  positionTooltip(tip, target.getBoundingClientRect());
}
function hideTooltip() {
  if (_tooltipEl) _tooltipEl.classList.remove('show');
}
function bindTooltips() {
  document.querySelectorAll('.sidebar.collapsed [data-tooltip]').forEach(el => {
    el.addEventListener('mouseenter', () => showTooltip(el));
    el.addEventListener('mouseleave', hideTooltip);
    el.addEventListener('focus', () => showTooltip(el));
    el.addEventListener('blur', hideTooltip);
  });
}
// 收起态变化时重绑（因为 display: none 切换会丢事件）
const _obs = new MutationObserver(bindTooltips);
_obs.observe(document.getElementById('sidebar') || document.body, {attributes: true, attributeFilter: ['class']});
bindTooltips();

// 通用 data-tooltip 绑定（非 sidebar 的 ⓘ 等图标也支持）
function bindGeneralTooltips() {
  document.querySelectorAll('[data-tooltip]').forEach(el => {
    if (el.closest('.sidebar')) return; // 跳过 sidebar（已有 bindTooltips 处理）
    if (el._tooltipBound) return;
    el._tooltipBound = true;
    el.addEventListener('mouseenter', () => showTooltip(el));
    el.addEventListener('mouseleave', hideTooltip);
    el.addEventListener('focus', () => showTooltip(el));
    el.addEventListener('blur', hideTooltip);
  });
}
new MutationObserver(bindGeneralTooltips).observe(document.body, {childList: true, subtree: true});
bindGeneralTooltips();

// ===== Sidebar 收起/展开（localStorage 持久化，默认展开） =====
function loadSidebar() {
  const stored = localStorage.getItem('sf-sidebar-collapsed');
  const collapsed = stored === null ? false : stored === 'true'; // 默认展开（null=首次访问）
  document.getElementById('sidebar').classList.toggle('collapsed', collapsed);
  updateToggleLabel();
}
function toggleSidebar() {
  const sb = document.getElementById('sidebar');
  sb.classList.toggle('collapsed');
  localStorage.setItem('sf-sidebar-collapsed', sb.classList.contains('collapsed'));
  updateToggleLabel();
}
function updateToggleLabel() {
  const sb = document.getElementById('sidebar');
  const lbl = sb && sb.querySelector('.toggle-label');
  if (lbl) lbl.textContent = sb.classList.contains('collapsed') ? '' : '收起';
}
loadSidebar();

// ===== 全局快速 tooltip：替代浏览器原生 title 慢延迟 =====
let _fastTipEl = null;
function ensureFastTip() {
  if (_fastTipEl) return _fastTipEl;
  _fastTipEl = document.createElement('div');
  _fastTipEl.className = 'custom-tooltip';
  document.body.appendChild(_fastTipEl);
  return _fastTipEl;
}
function showFastTip(target) {
  const text = target.getAttribute('title');
  if (!text) return;
  // 阻止浏览器原生 tooltip 弹(把 title 暂存,鼠标走时恢复)
  if (target._origTitle === undefined) target._origTitle = text;
  target.setAttribute('title', '');
  const tip = ensureFastTip();
  tip.innerHTML = `<span class="tt-line">${esc(text)}</span>`;
  positionTooltip(tip, target.getBoundingClientRect());
}
function hideFastTip(target) {
  if (_fastTipEl) _fastTipEl.classList.remove('show');
  if (target && target._origTitle !== undefined) {
    target.setAttribute('title', target._origTitle);
  }
}
function bindFastTooltips() {
  // 排除 sidebar collapsed 的 data-tooltip(那个有专门系统)
  document.querySelectorAll('[title]').forEach(el => {
    if (el.closest('.sidebar.collapsed [data-tooltip]')) return;
    if (el._fastTipBound) return;
    el._fastTipBound = true;
    el.addEventListener('mouseenter', () => showFastTip(el));
    el.addEventListener('mouseleave', () => hideFastTip(el));
    el.addEventListener('focus', () => showFastTip(el));
    el.addEventListener('blur', () => hideFastTip(el));
  });
}
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', bindFastTooltips);
} else {
  bindFastTooltips();
}
// 自动化 widget 重新渲染后也需要重绑
const _origLoadLinks = window.loadLinks;
const _origLoadDirs = window.loadDirs;
const _origLoadTodo = window.loadTodo;

// 重新渲染后自动 rebind 快速 tooltip
new MutationObserver(() => bindFastTooltips()).observe(document.body, {childList: true, subtree: true});
