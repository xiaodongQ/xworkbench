// Dashboard Tab：stats + 7 天柱图 + 最近任务 + 调度器 + 最近 executions
// 链接/目录/todo widget 已搬到左侧 widget-sidebar (widgets.js)
// 依赖 api.js (fetchJSON/esc/statusTag/fmt)

let currentChartRange = '7d';

// 折线图 tooltip（hover 圆点显示数量）
window.showChartTooltip = function(e, el) {
  const tt = document.getElementById('chart-tooltip');
  if (!tt) return;
  tt.innerHTML = '<b>' + el.dataset.date + '</b> 执行 <b>' + el.dataset.count + '</b> 次任务';
  tt.classList.remove('hidden');
  const wrap = document.querySelector('.chart-wrap');
  if (!wrap) return;
  const rect = wrap.getBoundingClientRect();
  tt.style.left = (e.clientX - rect.left + 12) + 'px';
  tt.style.top = (e.clientY - rect.top - 36) + 'px';
};
window.hideChartTooltip = function() {
  const tt = document.getElementById('chart-tooltip');
  if (tt) tt.classList.add('hidden');
};

function switchChartRange(range) {
  currentChartRange = range;
  document.querySelectorAll('[id^="btn-range-"]').forEach(b => { b.style.opacity = '0.5'; b.classList.remove('btn-primary'); });
  const btn = document.getElementById('btn-range-' + range);
  if (btn) { btn.style.opacity = '1'; btn.classList.add('btn-primary'); }
  loadChartStats();
}

async function loadChartStats() {
  try {
    const stats = await fetchJSON(API + '/api/stats?range=' + currentChartRange);
    renderChart(stats.daily_stats || []);
  } catch (e) { console.error(e); }
}

async function loadDashboard() {
  // 确保当前 range 按钮激活状态正确
  document.querySelectorAll('[id^="btn-range-"]').forEach(b => { b.style.opacity = '0.5'; b.classList.remove('btn-primary'); });
  const activeBtn = document.getElementById('btn-range-' + currentChartRange);
  if (activeBtn) { activeBtn.style.opacity = '1'; activeBtn.classList.add('btn-primary'); }
  try {
    const [stats, recent] = await Promise.all([
      fetchJSON(API + '/api/stats?range=' + currentChartRange),
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
}

function renderChart(daily) {
  const el = document.getElementById('chart-bars');
  if (!daily || daily.length === 0) {
    el.innerHTML = '<div class="empty" style="padding:20px">暂无数据</div>';
    return;
  }
  const W = el.clientWidth || 600;
  const H = 120;
  const padL = 28, padR = 12, padT = 12, padB = 28;
  const max = Math.max(...daily.map(d => d.count), 1);
  const n = daily.length;
  const innerW = W - padL - padR;
  const innerH = H - padT - padB;

  // Y 轴刻度（取整）
  const yTicks = 4;
  const yLabels = [];
  for (let i = 0; i <= yTicks; i++) {
    const v = Math.round((max / yTicks) * i);
    yLabels.push(v);
  }

  // X 轴标签（按密度取舍）
  const showEvery = n > 60 ? Math.ceil(n / 12) : n > 20 ? Math.ceil(n / 10) : 1;
  const xLabelAt = (i) => padL + (i / (n - 1 || 1)) * innerW;
  const yPos = (v) => padT + innerH - (v / max) * innerH;

  // 构建 SVG 元素
  const gridLines = yLabels.map(v =>
    `<line class="chart-grid-line" x1="${padL}" y1="${yPos(v)}" x2="${W - padR}" y2="${yPos(v)}"/>
     <text class="chart-y-label" x="${padL - 4}" y="${yPos(v) + 4}" text-anchor="end">${v}</text>`
  ).join('');

  const xLabels = daily.map((d, i) => {
    if (i % showEvery !== 0 && i !== n - 1) return '';
    return `<text class="chart-y-label" x="${xLabelAt(i)}" y="${H - 6}" text-anchor="middle">${d.date ? d.date.slice(5) : ''}</text>`;
  }).join('');

  // 折线点
  const pts = daily.map((d, i) => `${xLabelAt(i)},${yPos(d.count)}`).join(' ');
  const areaPath = `M ${xLabelAt(0)},${padT + innerH} L ${daily.map((d, i) => `${xLabelAt(i)},${yPos(d.count)}`).join(' L ')} L ${xLabelAt(n - 1)},${padT + innerH} Z`;

  // tooltip 跟随鼠标
  const dots = daily.map((d, i) => {
    const x = xLabelAt(i);
    const y = yPos(d.count);
    return `<circle class="chart-dot" cx="${x}" cy="${y}" r="4"
      data-date="${d.date}" data-count="${d.count}"
      onmousemove="showChartTooltip(event,this)"
      onmouseleave="hideChartTooltip()"/>`;
  }).join('');

  el.innerHTML = `
    <div class="chart-wrap" style="position:relative">
      <svg class="chart-svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}">
        <defs>
          <linearGradient id="areaGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="#c2410c" stop-opacity="0.3"/>
            <stop offset="100%" stop-color="#c2410c" stop-opacity="0.02"/>
          </linearGradient>
        </defs>
        ${gridLines}
        <line class="chart-axis-line" x1="${padL}" y1="${padT}" x2="${padL}" y2="${padT + innerH}"/>
        <line class="chart-axis-line" x1="${padL}" y1="${padT + innerH}" x2="${W - padR}" y2="${padT + innerH}"/>
        ${xLabels}
        <path class="chart-area" d="${areaPath}"/>
        <polyline class="chart-polyline" points="${pts}"/>
        ${dots}
      </svg>
      <div id="chart-tooltip" class="chart-tooltip hidden"></div>
    </div>`;
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
