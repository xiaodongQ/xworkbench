// Dashboard Tab：stats + 7 天柱图 + 最近任务 + 调度器 + 最近 executions
// 链接/目录/todo widget 已搬到左侧 widget-sidebar (widgets.js)
// 依赖 api.js (fetchJSON/esc/statusTag/fmt)

// ===== AI 自治能力开关（UI 入口：dashboard 上的 checkbox）=====
// 后端：AppSettings (ai_loop_enabled) > config.json (ai_loop.enabled) > 默认 false
// 页面只能改 AppSettings（运行时热调）；config.json 需要手动编辑重启。
// 从设置页 toggle 后会刷 dashboard widget + task-modal 上的 AI 自治区块可见性。
async function loadAILoopStatus() {
  try {
    const resp = await fetchJSON('/api/ai-loop/status');
    const enabled = !!resp.enabled;
    const source = resp.source || 'default';
    // 1. 同步 widget 状态
    const checkbox = document.getElementById('ailoop-toggle');
    if (checkbox) checkbox.checked = enabled;
    const badge = document.getElementById('ailoop-badge');
    if (badge) {
      badge.textContent = enabled ? '已启用' : '未启用';
      badge.style.background = enabled ? '#10b981' : '#6b7280';
    }
    const srcEl = document.getElementById('ailoop-source');
    if (srcEl) {
      const label = {default: '默认', 'config.json': '位置文件', app_settings: '设置页'}[source] || source;
      srcEl.textContent = '· 来源：' + label;
    }
    // 2. 同步 task-modal 的 AI 自治区块（如果 modal 打开着）
    const section = document.getElementById('ai-loop-section');
    if (section) {
      section.classList.toggle('hidden', !enabled);
      const srcBadge = document.getElementById('ai-loop-source-badge');
      if (srcBadge) srcBadge.textContent = enabled ? '(' + source + ')' : '';
    }
    return enabled;
  } catch (e) {
    console.error('[ai-loop] status load failed:', e);
    return false;
  }
}

async function toggleAILoop(checked) {
  try {
    await fetchJSON('/api/settings/ai_loop_enabled', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: checked ? '1' : '0' }),
    });
    await loadAILoopStatus();
  } catch (e) {
    alert('切换 AI 自治开关失败：' + e.message);
    // 回滚 checkbox 状态
    const checkbox = document.getElementById('ailoop-toggle');
    if (checkbox) checkbox.checked = !checked;
  }
}

async function loadDashboard() {
  try {
    const [stats, recent] = await Promise.all([
      fetchJSON(API + '/api/stats'),
      fetchJSON(API + '/api/tasks?limit=5')
    ]);
    document.getElementById('stat-pending').textContent = stats.pending_tasks;
    document.getElementById('stat-in_progress').textContent = stats.in_progress_tasks;
    document.getElementById('stat-waiting_input').textContent = stats.waiting_input_tasks;
    document.getElementById('stat-archived').textContent = stats.archived_tasks;
    document.getElementById('stat-exception').textContent = stats.exception_tasks;
    renderChart(stats.daily_stats || []);
    renderRecentTasks(recent);
  } catch(e) { console.error(e); }
  // 调度器 + 最近执行（链接/目录/todo 由 widgets.js 独立加载）
  initDashboardAutoRefresh();
  loadScheduler();
  loadScheduledSummary();
  loadRecentExecutions();
  loadAILoopStatus();
}

function renderChart(daily) {
  const el = document.getElementById('chart-bars');
  if (!daily || daily.length === 0) {
    el.innerHTML = '<div class="empty" style="padding:20px">暂无数据</div>';
    return;
  }
  const max = Math.max(...daily.map(d => d.count), 1);
  el.innerHTML = daily.map(d => `
    <div class="bar-wrap">
      <div class="bar" style="height:${(d.count / max) * 80}px"></div>
      <div class="bar-label">${d.date ? d.date.slice(5) : ''}</div>
    </div>
  `).join('');
}

function renderRecentTasks(list) {
  const el = document.getElementById('recent-list');
  if (!list || list.length === 0) {
    el.innerHTML = '<div class="empty">暂无任务</div>';
    return;
  }
  el.innerHTML = `<table>
    <thead><tr><th>标题</th><th>模块</th><th>状态</th><th>时间</th></tr></thead>
    <tbody>${list.map(t => `
      <tr onclick="viewTask('${t.id}')" style="cursor:pointer">
        <td><div class="task-title-cell"><div class="title">${esc(t.title)}</div></div></td>
        <td style="color:var(--text-secondary);font-size:12px">${esc(t.module || '-')}</td>
        <td>${statusTag(t.status)}</td>
        <td style="color:var(--text-secondary);font-size:12px">${fmt(t.created_at)}</td>
      </tr>`).join('')}</tbody>
  </table>`;
}

// ===== 调度器状态（dashboard 顶部，只读展示） =====
async function loadScheduler() {
  const data = await fetchJSON('/api/scheduler/status');
  const running = !!data.running;
  document.querySelectorAll('#scheduler-badge, #scheduler-badge-2').forEach(el => {
    el.outerHTML = running
      ? '<span id="' + el.id + '" class="scheduler-badge running"><span class="dot green"></span>运行中</span>'
      : '<span id="' + el.id + '" class="scheduler-badge stopped"><span class="dot gray"></span>已停止</span>';
  });
  // 同步自动化页面的调度器按钮状态（只禁用，不改文案）
  document.querySelectorAll('[data-sched-action]').forEach(btn => {
    const act = btn.dataset.schedAction;
    if (act === 'start') btn.disabled = running;
    else if (act === 'stop') btn.disabled = !running;
  });
}

// ===== 总览页自动刷新状态指示器（只读展示） =====
let dashboardAutoRefreshTimer = null;

function updateDashboardRefreshStatus() {
  const el = document.getElementById('dashboard-refresh-status');
  if (!el) return;
  const secs = typeof window.getRefreshSeconds === 'function' ? window.getRefreshSeconds() : 3;
  const isEnabled = window._autoRefreshEnabled;
  if (isEnabled) {
    el.innerHTML = '<span style="color:var(--archived)">● 自动刷新</span> · <span style="color:var(--archived);font-weight:500">' + secs + 's</span>';
  } else {
    el.innerHTML = '<span style="color:var(--text-secondary)">● 自动刷新（已暂停）</span>';
  }
}

function startDashboardAutoRefresh() {
  if (dashboardAutoRefreshTimer) clearInterval(dashboardAutoRefreshTimer);
  const ms = (window.getRefreshSeconds || function() { return 3; })() * 1000;
  dashboardAutoRefreshTimer = setInterval(() => {
    if (!window._autoRefreshEnabled) return;
    if (document.hidden) return;
    const anyModalOpen = document.querySelector('.modal-overlay:not(.hidden)');
    if (anyModalOpen) return;
    loadRecentExecutions();
    updateDashboardRefreshStatus();
  }, ms);
}
function stopDashboardAutoRefresh() {
  if (dashboardAutoRefreshTimer) { clearInterval(dashboardAutoRefreshTimer); dashboardAutoRefreshTimer = null; }
}

function initDashboardAutoRefresh() {
  const isEnabled = window._autoRefreshEnabled;
  if (isEnabled) {
    startDashboardAutoRefresh();
  }
  updateDashboardRefreshStatus();
}
