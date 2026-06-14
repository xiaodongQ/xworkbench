// ===== 共享 API 客户端 + DOM 工具 + Tab 切换 =====
// 5 个 view JS 都依赖这里的工具函数

const API = '';
let currentTab = 'dashboard';

// ===== 全局 CLI + 模型列表 =====
const CLI_MODELS = {
  claude: [
    {value: 'haiku', label: 'haiku（快+便宜）'},
    {value: 'sonnet', label: 'sonnet（推荐 · 准确）'},
    {value: 'opus', label: 'opus（最强 · 贵）'},
  ],
  cbc: [
    {value: 'glm-5.1', label: 'GLM-5.1（x1.06）'},
    {value: 'glm-5.0', label: 'GLM-5.0（x0.80）'},
    {value: 'glm-5.0-turbo', label: 'GLM-5.0-Turbo（x0.95）'},
    {value: 'glm-5v-turbo', label: 'GLM-5v-Turbo（x0.95）'},
    {value: 'glm-4.7', label: 'GLM-4.7（x0.23）'},
    {value: 'minimax-m3', label: 'MiniMax-M3（x0.25）'},
    {value: 'minimax-m2.7', label: 'MiniMax-M2.7（x0.26）'},
    {value: 'kimi-k2.6', label: 'Kimi-K2.6（x0.59）'},
    {value: 'kimi-k2.5', label: 'Kimi-K2.5（x0.45）'},
    {value: 'hy3-preview', label: 'Hy3 preview（x0.37）'},
    {value: 'deepseek-v4-pro', label: 'Deepseek-V4-Pro（x0.25）'},
    {value: 'deepseek-v4-flash', label: 'Deepseek-V4-Flash（x0.13）'},
    {value: 'deepseek-v3-2-volc', label: 'DeepSeek-V3.2（x0.29）'},
  ]
};

function buildModelOptions(cliType) {
  const models = CLI_MODELS[cliType] || CLI_MODELS.claude;
  return models.map(m => '<option value="' + m.value + '">' + m.label + '</option>').join('');
}

async function fetchJSON(url, opts) {
  const r = await fetch(url, opts);
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

function statusTag(status) {
  const labels = {pending: '待认领', in_progress: '进行中', archived: '已完成', exception: '异常'};
  return `<span class="status-pill status-${status}">${labels[status] || status}</span>`;
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
  console.log('[switchTab]', tab, 'loadTasks typeof:', typeof loadTasks, 'has task-list:', !!document.getElementById('task-list'));
  if (tab === 'dashboard' && typeof loadDashboard === 'function') loadDashboard();
  if (tab === 'tasks' && typeof loadTasks === 'function') {
    console.log('[switchTab] calling loadTasks');
    try { loadTasks(); } catch(e) { console.error('[loadTasks error]', e); }
  }
  if (tab === 'experiences' && typeof loadExps === 'function') loadExps();
  if (tab === 'automation' && typeof loadAutomation === 'function') { loadAutomation(); if (typeof loadTerminalSetting === 'function') loadTerminalSetting(); }
  if (tab === 'aichat' && typeof initTerminal === 'function') initTerminal();
  if (tab === 'relay' && typeof loadRelayStats === 'function') loadRelayStats();
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
  // 按真换行拆行
  const lines = text.split('\n');
  tip.innerHTML = lines.map((l, i) =>
    i === 0 ? `<span class="tt-line tt-head">${esc(l)}</span>` : `<span class="tt-line">${esc(l)}</span>`
  ).join('');
  const rect = target.getBoundingClientRect();
  tip.style.left = (rect.right + 8) + 'px';
  tip.style.top = (rect.top + rect.height / 2) + 'px';
  tip.classList.add('show');
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

// ===== Sidebar 收起/展开（localStorage 持久化，默认收起） =====
function loadSidebar() {
  const collapsed = localStorage.getItem('sf-sidebar-collapsed') !== 'false'; // 默认收起
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
  const rect = target.getBoundingClientRect();
  tip.style.left = (rect.right + 8) + 'px';
  tip.style.top = (rect.top + rect.height / 2) + 'px';
  tip.classList.add('show');
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
