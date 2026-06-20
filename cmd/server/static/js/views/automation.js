// Automation Tab：scheduler 启停 + 定时任务列表 + scheduled modal + 最近 executions
// 依赖 api.js

// auto-refresh 配置：默认 10s,最低 1s
const REFRESH_KEY = 'automation.refreshSeconds';
let autoRefreshTimer = null;
let _autoRefreshEnabled = true;

// ===== 高级设置弹窗 =====
function showAdvancedSettings() {
  // 加载 AI 自治状态
  if (typeof loadAILoopStatus === 'function') loadAILoopStatus();
  document.getElementById('advanced-settings-modal').classList.remove('hidden');
}
function closeAdvancedSettings() {
  document.getElementById('advanced-settings-modal').classList.add('hidden');
}
// AI 自治区块折叠/展开
function toggleAILoopSection() {
  const content = document.getElementById('ailoop-section-content');
  const arrow = document.getElementById('ailoop-section-arrow');
  if (!content) return;
  const isHidden = content.classList.contains('hidden');
  content.classList.toggle('hidden');
  arrow.style.transform = isHidden ? 'rotate(90deg)' : '';
}

// 暴露全局状态供其他页面使用（如总览页）
window._autoRefreshEnabled = true;

function onEvalCliChange() {
  const cli = document.getElementById('eval-cli-select').value;
  const modelSel = document.getElementById('eval-model-select');
  modelSel.innerHTML = buildModelOptions(cli);
}

window.getRefreshSeconds = function() {
  const v = parseInt(localStorage.getItem(REFRESH_KEY) || '3', 10);
  return isNaN(v) || v < 1 ? 3 : v;
};
window.startAutoRefresh = startAutoRefresh;
window.stopAutoRefresh = stopAutoRefresh;
window.updateRefreshIndicator = updateRefreshIndicator;

function setRefreshSeconds(s) {
  localStorage.setItem(REFRESH_KEY, String(s));
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer);
    startAutoRefresh();
  }
  // 确保下拉框可用
  const select = document.getElementById('auto-refresh-secs');
  if (select) select.disabled = false;
  updateAutoRefreshStatusIndicator(true);
  if (typeof updateDashboardRefreshIndicator === 'function') updateDashboardRefreshIndicator();
}
function startAutoRefresh() {
  if (autoRefreshTimer) clearInterval(autoRefreshTimer);
  const ms = window.getRefreshSeconds() * 1000;
  autoRefreshTimer = setInterval(() => {
    if (!window._autoRefreshEnabled) return;
    if (document.hidden) return; // 后台 tab 不刷
    // modal 打开时只刷后台数据，不刷 modal 视图（避免覆盖用户正在看的内容）
    const anyModalOpen = document.querySelector('.modal-overlay:not(.hidden)');
    if (anyModalOpen && anyModalOpen.id !== 'exec-detail-modal') return;
    loadAutomation({silent: true});
  }, ms);
  updateAutoRefreshStatusIndicator(true);
}
function stopAutoRefresh() {
  if (autoRefreshTimer) { clearInterval(autoRefreshTimer); autoRefreshTimer = null; }
  updateAutoRefreshStatusIndicator(false);
}
function updateRefreshIndicator() {
  const el = document.getElementById('auto-refresh-indicator');
  if (el) el.textContent = autoRefreshTimer ? `🔄 ${window.getRefreshSeconds()}s` : '⏸ 暂停';
}

async function loadAutomation(opts) {
  await Promise.all([loadScheduler(), loadScheduled(), loadRecentExecutions()]);
  if (!opts || !opts.silent) updateRefreshIndicator();
}

// 暴露给 HTML 控件
function manualRefresh() { loadAutomation({silent: false}); }
function changeRefreshSeconds(v) { setRefreshSeconds(parseInt(v, 10)); }
const AUTO_REFRESH_ENABLED_KEY = 'automation.autoRefreshEnabled';

function toggleAutoRefresh() {
  const btn = document.getElementById('auto-refresh-toggle-btn');
  const freqWrap = document.getElementById('auto-refresh-freq-wrap');
  if (autoRefreshTimer) {
    stopAutoRefresh(); window._autoRefreshEnabled = false;
    localStorage.setItem(AUTO_REFRESH_ENABLED_KEY, 'false');
    if (btn) btn.textContent = '开启';
    if (freqWrap) freqWrap.style.display = 'none';
    updateAutoRefreshStatusIndicator(false);
    // 同步总览页状态
    if (typeof updateDashboardRefreshStatus === 'function') updateDashboardRefreshStatus();
  } else {
    window._autoRefreshEnabled = true; startAutoRefresh();
    localStorage.setItem(AUTO_REFRESH_ENABLED_KEY, 'true');
    if (btn) btn.textContent = '暂停';
    if (freqWrap) freqWrap.style.display = 'inline';
    updateAutoRefreshStatusIndicator(true);
    // 同步总览页状态
    if (typeof updateDashboardRefreshStatus === 'function') updateDashboardRefreshStatus();
  }
}

function updateAutoRefreshStatusIndicator(running) {
  const el = document.getElementById('auto-refresh-status');
  const freqWrap = document.getElementById('auto-refresh-freq-wrap');
  if (!el) return;
  if (running) {
    el.innerHTML = '<span style="color:var(--archived)">● 自动刷新</span>';
    if (freqWrap) freqWrap.style.display = 'inline';
  } else {
    el.innerHTML = '<span style="color:var(--text-secondary)">● 自动刷新（已暂停）</span>';
    if (freqWrap) freqWrap.style.display = 'none';
  }
}

// 调度器控制（供 HTML 按钮调用）
function schedulerStart() { fetch('/api/scheduler/start', {method:'POST'}).then(() => { loadScheduler(); loadScheduledSummary(); }); }
function schedulerStop() { fetch('/api/scheduler/stop', {method:'POST'}).then(() => { loadScheduler(); }); }

// ===== AI 自治能力开关（UI 入口：高级设置弹窗）=====
// 后端：AppSettings (ai_loop_enabled) > config.json (ai_loop.enabled) > 默认 false
// 页面只能改 AppSettings（运行时热调）；config.json 需要手动编辑重启。
// 从设置页 toggle 后会刷 task-modal 上的 AI 自治区块可见性。
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

// 自动化页面加载时同步 AI 自治状态
if (typeof window !== 'undefined') {
  document.addEventListener('DOMContentLoaded', () => {
    setTimeout(() => {
      if (typeof loadAILoopStatus === 'function') loadAILoopStatus();
    }, 500);
  });
}

// 页面首次进入：根据本地存储恢复自动刷新状态
if (typeof window !== 'undefined') {
  document.addEventListener('DOMContentLoaded', () => {
    setTimeout(() => {
      const savedEnabled = localStorage.getItem(AUTO_REFRESH_ENABLED_KEY);
      const shouldEnable = savedEnabled === null ? true : savedEnabled === 'true';
      const btn = document.getElementById('auto-refresh-toggle-btn');
      const freqWrap = document.getElementById('auto-refresh-freq-wrap');
      const select = document.getElementById('auto-refresh-secs');
      // 恢复下拉框值为本地存储的值
      if (select) {
        const savedSecs = localStorage.getItem(REFRESH_KEY);
        if (savedSecs) select.value = savedSecs;
      }
      if (shouldEnable) {
        if (typeof loadAutomation === 'function') { startAutoRefresh(); }
        if (btn) btn.textContent = '暂停';
        if (freqWrap) freqWrap.style.display = 'inline';
        updateAutoRefreshStatusIndicator(true);
      } else {
        window._autoRefreshEnabled = false;
        if (btn) btn.textContent = '开启';
        if (freqWrap) freqWrap.style.display = 'none';
        updateAutoRefreshStatusIndicator(false);
      }
      // 同步总览页状态
      if (typeof updateDashboardRefreshStatus === 'function') updateDashboardRefreshStatus();
    }, 500);
  });
}

async function loadScheduledSummary() {
  const list = await fetchJSON('/api/scheduled');
  const el = document.getElementById('scheduled-summary');
  if (!list || list.length === 0) {
    el.innerHTML = '<div style="color:var(--text-secondary);font-size:12px">暂无定时任务</div>';
    return;
  }
  el.innerHTML = list.slice(0, 5).map(s => {
    const status = s.last_status || 'pending';
    const enabledBadge = s.enabled
      ? ''
      : ' <span style="color:#f59e0b;font-size:10px;font-weight:600">(已禁用)</span>';
    return `<div class="scheduled-item">
      <span class="s-name">${esc(s.name)}${enabledBadge}</span>
      <span class="s-cron">${esc(s.cron_expr)}</span>
      <span class="s-status ${status}">${status}</span>
    </div>`;
  }).join('');
}

async function loadScheduled() {
  const list = await fetchJSON('/api/scheduled');
  // 顺手拉最近 exec，找 last_execution_id 对应的 completed_at 判断 running
  const execs = await fetchJSON('/api/executions?limit=50').catch(() => []);
  const execMap = {};
  for (const e of (execs || [])) execMap[e.id] = e;
  const el = document.getElementById('scheduled-list');
  if (!list || list.length === 0) {
    el.innerHTML = '<div style="color:var(--text-secondary);font-size:13px;text-align:center;padding:40px 0">暂无定时任务<br><br><span style="font-size:12px">点击右上"+ 新建定时任务"创建<br>支持标准 cron 5 字段 或 @every 30s</span></div>';
    return;
  }
  el.innerHTML = `<table><thead><tr><th>名称</th><th>Cron</th><th>类型</th><th>状态</th><th>最近执行</th><th>操作</th></tr></thead><tbody>` + list.map(s => {
    const lastRun = s.last_run_at ? new Date(s.last_run_at).toLocaleString() : '-';
    const baseStatus = s.last_status || 'pending';
    // running 检测：last_execution_id 对应的 execution 没有 completed_at
    let statusBadge = `<span class="s-status ${baseStatus}">${baseStatus}</span>`;
    if (s.last_execution_id && execMap[s.last_execution_id] && !execMap[s.last_execution_id].completed_at) {
      statusBadge = '<span class="s-status" style="background:var(--info,#3b82f6);color:#fff">运行中</span>';
    }
    const enabledBadge = s.enabled ? '' : ' <span style="color:#f59e0b;font-size:11px;font-weight:600">(已禁用)</span>';
    const toggleLabel = s.enabled ? '⏸ 停用' : '▶ 启用';
    const toggleBtnClass = s.enabled ? 'btn btn-small' : 'btn btn-small btn-primary';
    return `<tr>
      <td>
        <span class="edit-icon" onclick="editScheduled('${s.id}')" title="编辑" style="cursor:pointer;margin-right:6px;color:var(--text-secondary);font-size:14px">✏️</span>
        <strong>${esc(s.name)}</strong>${enabledBadge}
      </td>
      <td><code>${esc(s.cron_expr)}</code></td>
      <td>${esc(s.command_type)}${s.model?' / '+esc(s.model):''}</td>
      <td>${statusBadge}</td>
      <td style="font-size:11px;color:var(--text-secondary)">${lastRun}</td>
      <td>
        <button class="${toggleBtnClass}" onclick="toggleScheduled('${s.id}', ${s.enabled})" title="${s.enabled ? '停止调度' : '启用调度'}">${toggleLabel}</button>
        <button class="btn btn-small" onclick="runScheduled('${s.id}')">▶ 执行</button>
        <button class="btn btn-small" onclick="deleteScheduled('${s.id}')">删除</button>
      </td>
    </tr>`;
  }).join('') + '</tbody></table>';
}

function toggleScheduled(id, currentlyEnabled) {
  fetch('/api/scheduled/' + id + '/toggle', {method:'POST'})
    .then(r => r.json())
    .then(() => { loadScheduled(); loadScheduledSummary(); })
    .catch(e => alert('切换失败：' + e.message));
}

function showScheduledModal() {
  document.getElementById('sched-id').value = '';
  document.getElementById('sched-modal-title').textContent = '新建定时任务';
  document.getElementById('sched-name').value = '';
  document.getElementById('sched-cron').value = '@every 30s';
  document.getElementById('sched-type').value = 'shell';
  document.getElementById('sched-model').value = '';
  document.getElementById('sched-prompt').value = '';
  document.getElementById('sched-timeout').value = '';
  document.getElementById('sched-enabled').checked = true;
  document.getElementById('sched-submit-btn').textContent = '创建';
  document.getElementById('scheduled-modal').classList.remove('hidden');
  onSchedTypeChange();
  setTimeout(() => document.getElementById('sched-name').focus(), 50);
}
function closeScheduledModal() { document.getElementById('scheduled-modal').classList.add('hidden'); }
function onSchedTypeChange() {
  const type = document.getElementById('sched-type').value;
  const modelSel = document.getElementById('sched-model');
  if (type === 'shell') {
    modelSel.disabled = true;
    modelSel.style.opacity = '0.4';
  } else {
    modelSel.disabled = false;
    modelSel.style.opacity = '1';
    modelSel.innerHTML = buildModelOptions(type);
    const defaultModel = getDefaultModel(type);
    if (defaultModel && modelSel.querySelector('option[value="' + defaultModel + '"]')) {
      modelSel.value = defaultModel;
    }
  }
  // model 切换时保存为默认
  modelSel.onchange = () => {
    if (type !== 'shell') saveDefaultModel(type, modelSel.value);
  };
}
async function editScheduled(id) {
  const s = await fetchJSON('/api/scheduled/' + id);
  document.getElementById('sched-id').value = s.id;
  document.getElementById('sched-modal-title').textContent = '编辑定时任务';
  document.getElementById('sched-name').value = s.name;
  document.getElementById('sched-cron').value = s.cron_expr;
  document.getElementById('sched-type').value = s.command_type;
  document.getElementById('sched-prompt').value = s.prompt;
  document.getElementById('sched-timeout').value = s.timeout_sec || '';
  document.getElementById('sched-enabled').checked = s.enabled;
  document.getElementById('sched-submit-btn').textContent = '保存';
  document.getElementById('scheduled-modal').classList.remove('hidden');
  // 编辑时：先设置已有模型值，再更新下拉框选项，最后恢复已有值（避免被全局默认值覆盖）
  const savedModel = s.model || '';
  onSchedTypeChange();
  document.getElementById('sched-model').value = savedModel;
}
function submitScheduled() {
  const id = document.getElementById('sched-id').value;
  const name = document.getElementById('sched-name').value.trim();
  const cron = document.getElementById('sched-cron').value.trim();
  const type = document.getElementById('sched-type').value;
  const model = document.getElementById('sched-model').value.trim();
  const promptText = document.getElementById('sched-prompt').value.trim();
  const timeoutSec = parseInt(document.getElementById('sched-timeout').value) || 0;
  const enabled = document.getElementById('sched-enabled').checked;
  if (!name || !cron || !promptText) { alert('名称、Cron、Prompt 必填'); return; }
  const body = {name, cron_expr:cron, command_type:type, prompt:promptText, model, timeout_sec:timeoutSec, enabled};
  const method = id ? 'PUT' : 'POST';
  const url = id ? '/api/scheduled/' + id : '/api/scheduled';
  fetch(url, {method, headers:{'Content-Type':'application/json'}, body:JSON.stringify(body)})
    .then(r => { if (!r.ok) throw new Error('创建/更新失败'); closeScheduledModal(); loadScheduled(); loadScheduledSummary(); loadScheduler(); })
    .catch(e => { alert('操作失败：' + e.message); console.error(e); });
}
function runScheduled(id) {
  fetch('/api/scheduled/' + id + '/run-now', {method:'POST'})
    .then(r => { if (!r.ok) throw new Error('立即执行失败'); setTimeout(() => { loadScheduled(); loadRecentExecutions(); }, 500); })
    .catch(e => { alert('执行失败：' + e.message); console.error(e); });
}
function deleteScheduled(id) {
  if (!confirm('删除该定时任务？')) return;
  fetch('/api/scheduled/' + id, {method:'DELETE'})
    .then(r => { if (!r.ok) throw new Error('删除失败'); loadScheduled(); loadScheduledSummary(); })
    .catch(e => { alert('删除失败：' + e.message); console.error(e); });
}

let recentExecLimit = 10;
async function loadRecentExecutions() {
  const render = (target, list, errMsg) => {
    if (errMsg) {
      target.innerHTML = `<div style="padding:8px;color:var(--exception);font-size:12px">⚠ ${errMsg} <button class="btn btn-small" style="margin-left:8px" onclick="loadRecentExecutions()">重试</button></div>`;
      return;
    }
    if (!list || list.length === 0) {
      target.innerHTML = '<div style="color:var(--text-secondary);font-size:12px;padding:8px">暂无执行</div>'
        + `<div style="padding:8px;text-align:center"><button class="btn btn-small" onclick="loadMoreExecutions()">📥 加载更多 (尝试加载 ${recentExecLimit} 条)</button></div>`;
      return;
    }
    // 按 session_group_id 分组：同组的所有 execution 归为一条展示
    // 规则：session_group_id == id 的是根节点（显示为一行，含子项折叠）；session_group_id != "" && != id 的是子节点（隐藏顶层，归入父节点）
    const groupMap = {}; // session_group_id -> { root: exec, children: [] }
    const topLevel = []; // 没有 session_group_id 或 session_group_id == id 的
    for (const e of list) {
      if (!e.session_group_id) {
        // 独立 execution，无分组
        topLevel.push(e);
      } else if (e.session_group_id === e.id) {
        // 自己是根节点
        groupMap[e.id] = { root: e, children: [] };
        topLevel.push(e);
      } else {
        // 是某根节点的子节点
        if (groupMap[e.session_group_id]) {
          groupMap[e.session_group_id].children.push(e);
        } else {
          // 根节点不在当前列表中（被截断了），当作独立行
          topLevel.push(e);
        }
      }
    }
    const renderRow = (e, depth) => {
      const dt = new Date(e.started_at).toLocaleString();
      const src = e.source === 'scheduled' ? '⏰' : e.source === 'continue' ? '💬' : '▶';
      const isRunning = !e.completed_at;
      const isEvaluating = _evaluatingIds.has(e.id);
      let statusIcon, statusColor, statusTitle;
      let evalBadge = '';
      if (isRunning) {
        statusIcon = '⏳'; statusColor = 'var(--info,#3b82f6)';
        statusTitle = '执行中…（尚未 Finish）';
      } else if (e.exit_code === 0) {
        statusIcon = '✓'; statusColor = 'var(--archived)';
        statusTitle = 'exit_code=0';
      } else {
        statusIcon = '✗ ' + e.exit_code; statusColor = 'var(--exception)';
        statusTitle = 'exit_code=' + e.exit_code;
      }
      if (isEvaluating) {
        evalBadge = ' <span class="s-status" style="background:var(--info,#3b82f6);color:#fff;font-size:11px;font-weight:600;padding:2px 10px;border-radius:10px;animation:pulse 1.5s ease-in-out infinite">⏳ 评估中</span>';
      } else if (e.evaluation_score !== undefined && e.evaluation_score !== null) {
        const sc = e.evaluation_score;
        const scoreColor = sc >= 8 ? 'var(--archived)' : sc >= 5 ? 'var(--warning)' : 'var(--exception)';
        const scoreBg = sc >= 8 ? 'rgba(16,185,129,0.15)' : sc >= 5 ? 'rgba(245,158,11,0.15)' : 'rgba(239,68,68,0.15)';
        const evalCount = e.evaluation_count || 0;
        const evalCountStr = evalCount > 1 ? `×${evalCount}` : '';
        evalBadge = ` <span class="s-status" style="background:${scoreBg};color:${scoreColor};font-size:11px;font-weight:600;padding:2px 10px;border-radius:10px" title="AI 评估分数（点击查看详情）" onclick="viewExecutionDetail('${e.id}')">📊${evalCountStr} ${sc}/10</span>`;
      }
      const indent = depth > 0 ? 'margin-left:' + (depth * 20) + 'px;border-left:2px solid var(--border);padding-left:8px;' : '';
      const rowStyle = isEvaluating
        ? 'display:flex;gap:8px;padding:6px 8px;border-bottom:1px solid var(--border);font-size:12px;align-items:center;background:rgba(59,130,246,0.08);border-left:3px solid #3b82f6;' + indent
        : 'display:flex;gap:8px;padding:6px 8px;border-bottom:1px solid var(--border);font-size:12px;align-items:center;' + indent;
      const title = esc(e.task_title || e.scheduled_task_title || e.command || "(无标题)");
      const cmdTip = e.command ? esc(e.command.slice(0, 200)) : '';
      const hasKids = groupMap[e.id] && groupMap[e.id].children.length > 0;
      const toggle = hasKids
        ? `<span id="exec-toggle-${e.id}" onclick="toggleExecGroup('${e.id}')" style="cursor:pointer;color:var(--text-secondary);font-size:14px" title="展开/折叠会话链">▶</span>`
        : '<span style="width:14px;display:inline-block"></span>';
      const groupIcon = hasKids ? '💬' : src;
      const groupTitle = hasKids ? `<b>[会话链 ${groupMap[e.id].children.length + 1} 轮]</b> ` : '';
      return `<div style="${rowStyle}">
        ${toggle}
        <span title="${e.source}">${groupIcon}</span>
        <span style="color:var(--text-secondary);font-family:monospace">${dt}</span>
        <span style="flex:1;max-width:500px;font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;cursor:pointer" onclick="viewExecutionDetail('${e.id}')">${groupTitle}${title}</span>${cmdTip ? `<span title="命令: ${cmdTip}" style="margin-left:4px;font-size:11px;color:#60a5fa">ⓘ</span>` : ''}
        <span style="font-size:11px;color:${statusColor}" title="${statusTitle}">${statusIcon}</span>
        ${evalBadge}
        <button class="btn btn-small" onclick="viewExecutionDetail('${e.id}')" title="查看详情">📋</button>
        <button class="btn btn-small" onclick="runEvaluation('${e.id}')" title="AI 评估" style="${isEvaluating?'opacity:0.5;cursor:wait':''}">${isEvaluating?'⏳':'📊'}</button>
      </div>
      <div id="exec-group-${e.id}" style="display:none">${(groupMap[e.id]?.children || []).map(c => renderRow(c, depth + 1)).join('')}</div>`;
    };
    const atEnd = list.length < recentExecLimit;
    target.innerHTML = topLevel.map(e => renderRow(e, 0)).join('') + `<div style="padding:8px;text-align:center;color:var(--text-secondary);font-size:11px">
        当前显示 ${list.length} 条（请求 ${recentExecLimit} 条）
        ${atEnd ? ' · 已到末尾' : `<button class="btn btn-small" data-exec-loadmore style="margin-left:8px" onclick="loadMoreExecutions()">📥 加载更多 (+50)</button>`}
      </div>`;
  };
  const el = document.getElementById('recent-execs');
  const el2 = document.getElementById('exec-list');
  let list, errMsg;
  try {
    list = await fetchJSON('/api/executions?limit=' + recentExecLimit);
  } catch (e) {
    console.error('[loadRecentExecutions]', e);
    errMsg = '加载失败：' + (e.message || e);
  }
  if (el) render(el, list, errMsg);
  if (el2) render(el2, list, errMsg);
}

function loadMoreExecutions() {
  recentExecLimit += 50;
  // 给所有"加载更多"按钮加 loading 反馈（innerHTML 重渲染前）
  document.querySelectorAll('[data-exec-loadmore]').forEach(b => {
    b.disabled = true; b.textContent = '⏳ 加载中...';
  });
  loadRecentExecutions();
}

// 会话链折叠/展开
function toggleExecGroup(id) {
  const el = document.getElementById('exec-group-' + id);
  const tog = document.getElementById('exec-toggle-' + id);
  if (!el) return;
  if (el.style.display === 'none') {
    el.style.display = 'block';
    if (tog) tog.textContent = '▼';
  } else {
    el.style.display = 'none';
    if (tog) tog.textContent = '▶';
  }
}

// ===== Execution 详情 + 评估 =====
let currentExecId = null;

async function viewExecutionDetail(id) {
  currentExecId = id;
  try {
    const exec = await fetchJSON('/api/executions/' + id);
    document.getElementById('exec-detail-cmd').value = exec.command || '';
    // 解析 claude -p --output-format json：取 result 字段，附带 num_turns 元数据
    const renderedOutput = renderExecOutput(exec.output);
    document.getElementById('exec-detail-output').value = renderedOutput;
    document.getElementById('exec-detail-error').value = exec.error || '';
    const isRunning = !exec.completed_at;
    const ok = !isRunning && exec.exit_code === 0;
    const dur = exec.completed_at && exec.started_at
      ? Math.round((new Date(exec.completed_at) - new Date(exec.started_at)) / 100) / 10 + 's'
      : '⏳ 进行中…';
    const exitDisplay = isRunning
      ? '<b style="color:var(--info,#3b82f6)">运行中</b>'
      : `<b style="color:${ok?'var(--archived)':'var(--exception)'}">${exec.exit_code}</b>`;
    const execTitle = exec.task_title || exec.scheduled_task_title || '(无标题)';
    const isScheduled = exec.source === 'scheduled';
    const srcColor = isScheduled ? 'color:#3b82f6' : 'color:#16a34a';
    const srcText = isScheduled ? '计划任务' : '手动任务';
    const assocId = exec.task_id || exec.scheduled_task_id || '-';
    const detailLines = [];
    detailLines.push('来源: ' + (isScheduled ? '计划任务' : '手动任务'));
    detailLines.push('ID: ' + assocId);
    if (exec.task_title) detailLines.push('任务: ' + exec.task_title);
    if (exec.scheduled_task_title) detailLines.push('定时: ' + exec.scheduled_task_title);
    if (exec.model) detailLines.push('模型: ' + exec.model);
    if (exec.command) detailLines.push('命令: ' + exec.command.slice(0, 300));
    const detailTip = detailLines.join('\n').replace(/"/g, '&quot;');
    const nameDisplay = execTitle.length > 40 ? execTitle.slice(0, 40) + '...' : execTitle;
    document.getElementById('exec-detail-title').innerHTML = '';
    document.getElementById('exec-detail-meta').innerHTML =
      '<span style="font-size:11px;font-weight:600;' + srcColor + '">' + srcText + '</span>' +
      ' <span style="margin-left:8px;color:var(--text-secondary)">|</span>' +
      ' <span style="cursor:help" title="' + detailTip + '"><b>' + esc(nameDisplay) + '</b> ⓘ</span>' +
      ' <span style="margin-left:8px;color:var(--text-secondary)">|</span>' +
      ' exit_code=' + exitDisplay + ' · ' + new Date(exec.started_at).toLocaleString() + ' · 耗时 ' + dur;

    const isEvalNow = _evaluatingIds.has(id);
    const evalBtn = document.getElementById('exec-detail-eval-btn');
    const evalSel = document.getElementById('eval-model-select');
    if (isEvalNow) {
      document.getElementById('exec-detail-eval').innerHTML = '<span style="color:var(--info,#3b82f6);font-size:12px">⏳ 评估中...</span>';
      if (evalBtn) { evalBtn.disabled = true; evalBtn.textContent = '⏳ 评估中'; }
      if (evalSel) evalSel.disabled = true;
    } else {
      document.getElementById('exec-detail-eval').innerHTML = '<span style="color:var(--text-secondary);font-size:12px">点下方"📊 AI 评估"按钮调 LLM 给这次执行打分</span>';
      if (evalBtn) { evalBtn.disabled = false; evalBtn.textContent = '📊 AI 评估'; }
      if (evalSel) evalSel.disabled = false;
    }
    // 继续对话按钮：有 session_id 时可点，否则显示但置灰 + tooltip 提示
    //（不隐藏：让用户知道这个功能存在，理解为什么这个 execution 不可续）
    const continueBtn = document.getElementById('exec-continue-btn');
    if (continueBtn) {
      if (exec.resume_uuid) {
        continueBtn.classList.remove('hidden');
        continueBtn.disabled = false;
        continueBtn.removeAttribute('title');
      } else {
        continueBtn.classList.remove('hidden');
        continueBtn.disabled = true;
        continueBtn.title = '该执行未生成 session id（首次失败 / 未跑的 execution 无法继续对话）';
      }
    }
    // 拉已有评估（展示所有历史）
    try {
      const evals = await fetchJSON('/api/executions/' + id + '/evaluations');
      renderEvalHistory(evals);
    } catch (e) { /* 忽略 */ }
    // 拉评论
    loadExecComments(id);
    document.getElementById('exec-detail-modal').classList.remove('hidden');
  } catch (e) {
    alert('加载执行详情失败：' + e.message);
  }
}

function closeExecDetailModal() {
  document.getElementById('exec-detail-modal').classList.add('hidden');
  currentExecId = null;
}

function showContinuePrompt() {
  const section = document.getElementById('exec-continue-section');
  const input = document.getElementById('exec-continue-prompt');
  if (!section || !input) return;
  section.classList.remove('hidden');
  input.focus();
}

async function submitContinue() {
  const execId = currentExecId;
  const prompt = document.getElementById('exec-continue-prompt')?.value?.trim();
  if (!execId || !prompt) return;
  const btn = document.getElementById('exec-continue-submit');
  if (btn) { btn.disabled = true; btn.textContent = '⏳'; }
  try {
    const res = await fetchJSON('/api/executions/' + execId + '/continue', {
      method: 'POST',
      body: JSON.stringify({ prompt }),
    });
    document.getElementById('exec-continue-section').classList.add('hidden');
    document.getElementById('exec-continue-prompt').value = '';
    closeExecDetailModal();
    // 刷新执行列表
    loadRecentExecutions();
    // 若 task-modal 打开着，同步刷新它的对话历史（C 方案：原任务上下文看会话链）
    const taskModal = document.getElementById('task-modal');
    if (taskModal && !taskModal.classList.contains('hidden') && typeof loadTaskConversation === 'function') {
      loadTaskConversation();
    }
  } catch (e) {
    alert('继续对话失败：' + e.message);
  } finally {
    if (btn) { btn.disabled = false; btn.textContent = '▶ 发送'; }
  }
}

// ===== 执行评论 =====

async function loadExecComments(execId) {
  const container = document.getElementById('exec-comment-list');
  const countEl = document.getElementById('exec-comment-count');
  if (!container) return;
  let list;
  try {
    list = await fetchJSON('/api/executions/' + execId + '/comments');
  } catch (e) {
    container.innerHTML = '<span style="color:var(--exception);font-size:12px">加载评论失败</span>';
    return;
  }
  if (countEl) countEl.textContent = list && list.length > 0 ? '(' + list.length + ')' : '';
  if (!list || !list.length) {
    container.innerHTML = '<span style="color:var(--text-secondary);font-size:12px">暂无评论</span>';
    return;
  }
  container.innerHTML = list.map(c => {
    const dt = c.created_at ? new Date(c.created_at).toLocaleString() : '';
    return '<div style="padding:6px 0;border-bottom:1px solid var(--border);font-size:12px">' +
      '<span style="color:var(--text-secondary)">' + esc(c.author || 'user') + ' · ' + dt + '</span>' +
      '<div style="margin-top:2px">' + esc(c.content) + '</div></div>';
  }).join('');
}

async function submitExecComment() {
  const execId = currentExecId;
  if (!execId) return;
  const input = document.getElementById('exec-comment-input');
  const content = input.value.trim();
  if (!content) return;
  input.value = '';
  await fetchJSON('/api/executions/' + execId + '/comments', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({author: 'user', content})
  });
  await loadExecComments(execId);
}

function renderEvalCard(ev) {
  const score = ev.score;
  const isParseFailed = score < 0; // -1 表示评估员输出无法解析
  const color = isParseFailed
    ? 'var(--text-secondary)'
    : score >= 8 ? 'var(--archived)' : score >= 5 ? 'var(--warning)' : 'var(--exception)';
  const scoreDisplay = isParseFailed ? '解析失败' : `${score}/10`;
  const cardStyle = isParseFailed
    ? 'font-size:13px;color:var(--text-secondary);font-style:italic'
    : 'font-size:13px';
  // 从评语里解析 num_turns / permission_denials 客观证据（sonnet 评语里常含）
  const evidence = parseEvalEvidence(ev.comments || '');
  // 耗时显示（duration_s 单位为秒）
  const durS = ev.duration_s || 0;
  const durDisplay = durS >= 1 ? durS.toFixed(1) + 's' : durS.toFixed(3) + 's';
  return `
    <div style="${cardStyle}">
      📊 AI 评估: <b style="color:${color};font-size:18px">${scoreDisplay}</b>
      <span style="color:var(--text-secondary);font-size:11px;margin-left:8px">${esc(ev.evaluator_model || '')} · ${durDisplay} · ${esc(new Date(ev.created_at).toLocaleString())}</span>
      ${evidence.numTurnsBadge}
    </div>
    ${ev.comments ? `<div style="margin-top:6px;color:var(--text-secondary);font-size:12px;white-space:pre-wrap">${esc(ev.comments)}</div>` : ''}
  `;
}

function renderEvalHistory(evals) {
  const container = document.getElementById('exec-detail-eval');
  if (!evals || evals.length === 0) {
    container.innerHTML = '<span style="color:var(--text-secondary);font-size:12px">暂无评估记录，点下方"📊 AI 评估"按钮开始评估</span>';
    return;
  }
  // 最新在前的历史列表
  const cards = evals.map((ev, i) => {
    const badge = i === 0 ? ' <span style="background:var(--info,#3b82f6);color:#fff;font-size:10px;padding:1px 6px;border-radius:8px">最新</span>' : '';
    return renderEvalCard(ev).replace('</div>', badge + '</div>');
  }).join('<div style="border-top:1px dashed var(--border);margin:8px 0"></div>');
  container.innerHTML = cards;
}

// parseEvalEvidence 从评语里 grep num_turns=N / permission_denials=[...] 等客观证据。
// sonnet 评语常包含 "num_turns=3, 退出码 0" 这种描述，匹配到直接做小徽章显示。
function parseEvalEvidence(comments) {
  const out = { numTurns: null, badge: '', numTurnsBadge: '' };
  const m = comments.match(/num_turns\s*=\s*(\d+)/i);
  if (m) {
    out.numTurns = parseInt(m[1], 10);
    const tool = out.numTurns >= 2;
    out.numTurnsBadge = ` <span title="Claude 任务执行 turn 数：>= 2 表示调过工具（客观证据）" style="margin-left:6px;padding:1px 6px;border-radius:8px;font-size:10px;background:${tool?'#d4edda':'#f8d7da'};color:${tool?'#155724':'#721c24'}">turns=${out.numTurns}${tool?' 🔧':' 💬'}</span>`;
  }
  return out;
}

async function runEvaluation(id) {
  const execId = id || currentExecId;
  if (!execId) { alert('请先打开一条执行'); return; }
  // 评估时禁用 select 避免反复改
  const sel = document.getElementById('eval-model-select');
  if (sel) sel.disabled = true;
  const timeoutInput = document.getElementById('eval-timeout');
  const timeoutSec = timeoutInput ? parseInt(timeoutInput.value) || 120 : 120;
  const btn = event && event.target;
  const oldText = btn && btn.textContent;
  if (btn) { btn.disabled = true; btn.textContent = '⏳'; }
  _markEvaluating(execId, true);
  // 记录评估开始时间（用 Date 对象避免时区字符串比较问题）
  const evalStartedAt = new Date();
  try {
    await fetchJSON('/api/executions/' + execId + '/evaluate', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({cli_type: getEvalCliType(), model: getEvalModel(), timeout_sec: timeoutSec})});
    // 评估中状态：弹窗 + 列表徽章（_markEvaluating 已刷列表）
    if (currentExecId === execId) {
      const card = document.getElementById('exec-detail-eval');
      card.innerHTML = `<div style="font-size:13px"><span style="color:var(--info,#3b82f6)">⏳ 评估中…</span> <span style="color:var(--text-secondary);font-size:11px">${esc(getEvalModel())} · 预计 5-30s</span></div>`;
    }
    // 轮询拿结果（评估异步执行，最长 3 分钟）
    for (let i = 0; i < 60; i++) {
      await new Promise(r => setTimeout(r, 2000));
      const evals = await fetchJSON('/api/executions/' + execId + '/evaluations');
      if (evals && evals.length > 0) {
        // 只认定本次评估开始后创建的新评估
        const newEvals = evals.filter(e => e.created_at && new Date(e.created_at) > evalStartedAt);
        if (newEvals.length > 0) {
          if (currentExecId === execId) renderEvalHistory(evals);
          _markEvaluating(execId, false);
          // 刷新列表（评分可能影响渲染）
          loadRecentExecutions();
          return;
        }
      }
      // 还在评估中，持续更新状态
      if (currentExecId === execId && i % 2 === 0) {
        const card = document.getElementById('exec-detail-eval');
        card.innerHTML = `<div style="font-size:13px"><span style="color:var(--info,#3b82f6)">⏳ 评估中…</span> <span style="color:var(--text-secondary);font-size:11px">${esc(getEvalModel())} · 已 ${(i+1)*2}s</span></div>`;
      }
    }
    alert('评估超时（>2 分钟），请检查 claude CLI 是否可用');
    _markEvaluating(execId, false);
    if (currentExecId === execId) {
      const card = document.getElementById('exec-detail-eval');
      card.innerHTML = `<div style="font-size:13px;color:var(--exception)">⚠ 评估超时</div>`;
    }
  } catch (e) {
    _markEvaluating(execId, false);
    alert('评估失败：' + e.message);
  } finally {
    if (btn) { btn.disabled = false; btn.textContent = oldText; }
    if (sel) sel.disabled = false;
  }
}

// getEvalModel 读 exec-detail-modal 里的评估模型下拉
function getEvalModel() {
  const sel = document.getElementById('eval-model-select');
  return sel ? sel.value : 'sonnet';
}
function getEvalCliType() {
  const sel = document.getElementById('eval-cli-select');
  return sel ? sel.value : 'claude';
}

// 正在评估的 execution id 集合（sessionStorage 持久化，刷新后可恢复）
const _evaluatingIds = new Set(JSON.parse(sessionStorage.getItem('_evaluatingIds') || '[]'));
function _markEvaluating(execId, on) {
  if (on) _evaluatingIds.add(execId); else _evaluatingIds.delete(execId);
  sessionStorage.setItem('_evaluatingIds', JSON.stringify([..._evaluatingIds]));
  // 立即刷新最近执行列表显示徽章
  loadRecentExecutions();
}

// renderExecOutput 解析 `claude -p --output-format json` 的输出，提取 result 字段。
// 非 JSON 格式 fallback 原样返回。
function renderExecOutput(raw) {
  if (!raw) return '(无输出)';
  const trimmed = raw.trim();
  // 尝试解析 session_id/sessionId（用于继续对话）
  const sessionId = extractSessionId(trimmed);
  // 必须以 { 或 [ 开头才能解析
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) return raw;
  let obj;
  try { obj = JSON.parse(trimmed); } catch { return raw; }
  // claude json 输出结构（单对象）
  if (typeof obj.result === 'string') {
    const lines = [];
    lines.push(obj.result);
    // 附加元数据头部（方便人工核查）
    const meta = [];
    if (sessionId) meta.push(`sessionId=${sessionId}`);
    if (typeof obj.num_turns === 'number') meta.push(`num_turns=${obj.num_turns}`);
    if (obj.is_error) meta.push('is_error=true');
    if (obj.stop_reason) meta.push(`stop_reason=${obj.stop_reason}`);
    if (obj.permission_denials && obj.permission_denials.length) {
      meta.push(`permission_denials=[${obj.permission_denials.join(',')}]`);
    }
    if (meta.length) {
      lines.unshift('--- Claude JSON 元数据 ---');
      lines.unshift(meta.join(' | '));
    }
    return lines.join('\n');
  }
  // cbc 分段 JSON 输出（数组），找第一个有 sessionId 的块
  if (Array.isArray(obj)) {
    let resultText = '';
    for (const item of obj) {
      if (item.type === 'message' && Array.isArray(item.content)) {
        for (const c of item.content) {
          if (c.type === 'text' || c.type === 'output_text') {
            resultText += c.text + '\n';
          }
        }
      }
    }
    const lines = resultText ? [resultText.trim()] : ['(无result内容)'];
    const meta = [];
    if (sessionId) meta.push(`sessionId=${sessionId}`);
    if (meta.length) {
      lines.unshift('--- Claude JSON 元数据 ---');
      lines.unshift(meta.join(' | '));
    }
    return lines.join('\n');
  }
  return raw;
}

// extractSessionId 从原始输出中提取 session_id（claude）或 sessionId（cbc）。
function extractSessionId(raw) {
  if (!raw) return null;
  // 尝试 JSON 解析
  try {
    const obj = JSON.parse(raw);
    // claude: session_id 在顶层
    if (obj.session_id) return obj.session_id;
    // cbc: sessionId 在 message 类型的块中
    if (Array.isArray(obj)) {
      for (const item of obj) {
        if (item.sessionId) return item.sessionId;
      }
    }
  } catch {}
  // 字符串匹配回退
  // cbc: "sessionId": "xxx"
  let idx = raw.indexOf('"sessionId"');
  if (idx >= 0) {
    const rest = raw.slice(idx + 12);
    const m = rest.match(/^[^"]*"\s*:\s*"([^"]+)"/);
    if (m) return m[1];
  }
  // claude: "session_id": "xxx"
  idx = raw.indexOf('"session_id"');
  if (idx >= 0) {
    const rest = raw.slice(idx + 12);
    const m = rest.match(/^[^"]*"\s*:\s*"([^"]+)"/);
    if (m) return m[1];
  }
  return null;
}

// ===== 终端类型设置 =====
async function onTerminalChange(value) {
  try {
    await fetchJSON('/api/config', {
      method: 'PUT',
      body: JSON.stringify({ terminal_type: value }),
    });
  } catch (e) {
    console.warn('保存默认终端失败:', e);
  }
}

// 页面加载时从 /api/config 读取终端类型设置
async function loadTerminalSetting() {
  try {
    const data = await fetchJSON('/api/config');
    const val = data.terminal?.default_type || 'wezterm';
    const sel = document.getElementById('default-terminal-select');
    if (sel) sel.value = val;
  } catch (e) {
    const sel = document.getElementById('default-terminal-select');
    if (sel) sel.value = 'wezterm';
  }
}
