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
  const content = document.getElementById('advanced-ailoop-content');
  const arrow = document.getElementById('advanced-ailoop-arrow');
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
  // 切 CLI 后用系统配置的评估默认初始化（eval_default → default 兜底）
  const defaultModel = getEvalDefaultModel(cli);
  if (defaultModel && modelSel.querySelector('option[value="' + defaultModel + '"]')) {
    modelSel.value = defaultModel;
  }
  // 注意：评估页 model 下拉的 onchange 不再调 saveEvalDefaultModel，
  // 避免评估页选其他模型时污染系统配置的 eval_default。
  // 评估请求仍然读下拉当前值（getEvalModel() 已在调用），本次评估用新值；
  // 重新进入评估页/刷新后下拉重置为系统配置的 eval_default。
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
// 单一来源：config.json（ai_loop_enabled 顶层字段）
// 页面改完 PUT /api/config 即落盘；下次进入会保持。
async function loadAILoopStatus(taskId) {
  try {
    const resp = await fetchJSON('/api/ai-loop/status');
    const enabled = !!resp.enabled;
    // 1. 同步「高级设置」弹窗里 AI 自治 widget 状态
    const checkbox = document.getElementById('ailoop-toggle');
    if (checkbox) checkbox.checked = enabled;
    const widgetBadge = document.getElementById('ailoop-badge');
    if (widgetBadge) {
      widgetBadge.textContent = enabled ? '已启用' : '未启用';
      widgetBadge.style.background = enabled ? '#10b981' : '#6b7280';
    }
    const srcEl = document.getElementById('ailoop-source');
    if (srcEl) srcEl.textContent = '· 来源：config.json';
    // 2. 同步任务详情弹窗里的 AI 自治按钮区（如果 modal 打开着）
    const taskBlock = document.getElementById('task-ailoop-block');
    if (taskBlock) {
      taskBlock.classList.toggle('hidden', !enabled);
      const taskSrc = document.getElementById('task-ailoop-source');
      if (taskSrc) taskSrc.textContent = enabled ? '(config.json)' : '';
    }
    // 3. Learn 按钮依赖 ai_loop_enabled，没开时隐藏
    const learnBtn = document.getElementById('btn-learn');
    if (learnBtn) {
      learnBtn.style.display = enabled ? '' : 'none';
    }
    // 4. 如果当前 task 正在 loop 中，禁用启动按钮
    if (taskId && Array.isArray(resp.running) && resp.running.includes(taskId)) {
      const runBtn = document.getElementById('btn-run-loop');
      if (runBtn) {
        runBtn.disabled = true;
        runBtn.textContent = '⏳ 运行中';
      }
    }
    return enabled;
  } catch (e) {
    console.error('[ai-loop] status load failed:', e);
    return false;
  }
}

// _ailoopInflight 防止用户在 fetch 期间反复点导致多个并发 PUT；
// 最终落盘状态以最后一次点操作为准（服务端按请求顺序处理）。
let _ailoopInflight = false;

async function toggleAILoop(checked) {
  const checkbox = document.getElementById('ailoop-toggle');
  // 1. 已有请求在飞：禁用 checkbox 等当前完成（避免 UI 与服务端不一致）
  if (_ailoopInflight) {
    if (checkbox) checkbox.checked = !checked;
    return;
  }
  _ailoopInflight = true;
  if (checkbox) checkbox.disabled = true;
  try {
    await fetchJSON('/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ai_loop_enabled: checked }),
    });
    await loadAILoopStatus();
  } catch (e) {
    alert('切换 AI 自治开关失败：' + e.message);
    // 回滚 checkbox 状态
    if (checkbox) checkbox.checked = !checked;
  } finally {
    _ailoopInflight = false;
    if (checkbox) checkbox.disabled = false;
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

// scheduled_tasks.last_status 值（pending/success/failed/timeout/cancelled/build_error）→ 中文显示。
// CSS 仍按英文 class 挂颜色（.s-status.success / .s-status.failed），只翻译文本。
const SCHED_STATUS_TEXT = {
  pending:     '待运行',
  success:     '成功',
  failed:      '失败',
  timeout:     '超时',
  cancelled:   '已取消',
  build_error: '命令错误',
};
const schedStatusText = (raw) => SCHED_STATUS_TEXT[raw] || SCHED_STATUS_TEXT.pending;

// 定时任务表格排序：三档循环 asc → desc → ''（默认/恢复原序）
const SCHED_SORT_KEY = 'automation.schedSortDir'; // 'asc' | 'desc' | ''


// 更新定时任务表格排序图标状态
function updateSchedSortIcon() {
  const dir = localStorage.getItem(SCHED_SORT_KEY); // null=默认（显示⇅）
  const icon = document.getElementById('sched-sort-icon');
  if (icon) {
    icon.textContent = dir === 'asc' ? '↑' : dir === 'desc' ? '↓' : '⇅';
    icon.title = '点击排序（下一档：' + nextSortLabel(SCHED_SORT_KEY) + '）';
  }
}

// 返回 sortKey 对应的"下一档"提示文字（用于 tooltip）
const nextSortLabel = (key) => {
  const prev = localStorage.getItem(key) || 'asc';
  const next = prev === 'asc' ? 'desc' : prev === 'desc' ? '' : 'asc';
  if (next === 'asc') return '↑ 升序';
  if (next === 'desc') return '↓ 降序';
  return '按时间排序';
}

// 切换定时任务表格排序方向（asc → desc → '' → asc）
function toggleSchedSort() {
  const prev = localStorage.getItem(SCHED_SORT_KEY) || 'asc';
  const next = prev === 'asc' ? 'desc' : prev === 'desc' ? '' : 'asc';
  if (next) localStorage.setItem(SCHED_SORT_KEY, next);
  else localStorage.removeItem(SCHED_SORT_KEY);
  updateSchedSortIcon();
  loadScheduled();
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
      <span class="s-status ${status}">${schedStatusText(status)}</span>
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
  const initSortIcon = () => {
    const dir = localStorage.getItem(SCHED_SORT_KEY); // null=默认（显示⇅）
    return dir === 'asc' ? '↑' : dir === 'desc' ? '↓' : '⇅';
  };
  // 按下次运行时间排序（三档：asc / desc / ''=恢复原序）
  const sortDir = localStorage.getItem(SCHED_SORT_KEY); // null 表示默认（恢复原序）
  // sortDir 为 null/'' 时不做排序，直接用原始 list
  const sortedList = sortDir ? [...list].sort((a, b) => {
    // disabled 排最后
    if (!a.enabled && !b.enabled) return 0;
    if (!a.enabled) return 1;
    if (!b.enabled) return -1;
    // 无 next_run_at（cron 解析失败）排最后
    if (!a.next_run_at && !b.next_run_at) return 0;
    if (!a.next_run_at) return 1;
    if (!b.next_run_at) return -1;
    const diff = new Date(a.next_run_at) - new Date(b.next_run_at);
    return sortDir === 'asc' ? diff : -diff;
  }) : list;
el.innerHTML = `<table><thead><tr><th>名称</th><th>Cron</th><th>类型</th><th>状态</th><th style="cursor:pointer;user-select:none" onclick="toggleSchedSort()">最近执行 <span id="sched-sort-icon">${initSortIcon()}</span></th><th>操作</th></tr></thead><tbody>` + sortedList.map(s => {
    const lastRun = s.last_run_at ? new Date(s.last_run_at).toLocaleString() : '-';
    const nextRun = (s.enabled && s.next_run_at)
      ? `<div style="font-size:10px;color:var(--info);margin-top:2px;white-space:nowrap">⏰ 下次 ${esc(new Date(s.next_run_at).toLocaleString())}</div>`
      : '';
    const baseStatus = s.last_status || 'pending';
    // running 检测：last_execution_id 对应的 execution 没有 completed_at
    let statusBadge = `<span class="s-status ${baseStatus}">${schedStatusText(baseStatus)}</span>`;
    if (s.last_execution_id && execMap[s.last_execution_id] && !execMap[s.last_execution_id].completed_at) {
      statusBadge = '<span class="s-status" style="background:var(--info,#3b82f6);color:#fff">运行中</span>';
    }
    const enabledBadge = s.enabled ? '' : ' <span style="color:#f59e0b;font-size:11px;font-weight:600">(已禁用)</span>';
    const toggleLabel = s.enabled ? '⏸ 停用' : '▶ 启用';
    const toggleBtnClass = s.enabled ? 'btn btn-small' : 'btn btn-small btn-primary';
    return `<tr>
      <td style="padding:4px 6px">
        <span class="edit-icon" onclick="editScheduled('${s.id}')" title="编辑" style="cursor:pointer;margin-right:6px;color:var(--text-secondary);font-size:14px">✏️</span>
        <strong>${esc(s.name)}</strong>${enabledBadge}
      </td>
      <td style="max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;padding:4px 6px"><code style="font-size:10.5px">${esc(s.cron_expr)}</code></td>
      <td title="${esc(s.command_type)}" style="padding:4px 6px"><span style="font-size:10.5px">${esc(s.command_type)}</span><br><span style="font-size:10.5px;color:var(--text-secondary)">${s.model ? esc(s.model) : ''}</span></td>
      <td style="padding:4px 6px">${statusBadge}</td>
      <td style="font-size:11px;color:var(--text-secondary);vertical-align:top;padding:4px 6px">${lastRun}${nextRun}</td>
      <td style="padding:4px 6px">
        <button class="${toggleBtnClass}" onclick="toggleScheduled('${s.id}', ${s.enabled})" title="${s.enabled ? '停止调度' : '启用调度'}">${toggleLabel}</button>
        <button class="btn btn-small" onclick="runScheduled('${s.id}')">▶ 执行</button>
        <button class="btn btn-small" onclick="deleteScheduled('${s.id}')">删除</button>
      </td>
    </tr>`;
  }).join('') + '</tbody></table>';
  // 页面加载时恢复排序图标状态
  updateSchedSortIcon();
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
  // 注意：调度任务 modal 的 model onchange 不再调 saveDefaultModel，
  // 避免调度页选其他模型时污染系统配置的 default。
  // 调度任务的 model 存在 scheduled_tasks.model 列（stored 字段），
  // 与全局 default 无关；新建议 task 时下拉用 getDefaultModel(cli) 初始化。
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
    // 按 resume_uuid 分组：同 session 的所有 execution 归为一条展示
    // 规则：同 session 链中 started_at 最早的 execution 是根节点（显示为一行，含子项折叠）；其余是子节点
    // 注意：resume_uuid 是 claude session_id，不是 execution id，不能用 `resume_uuid === id` 判断
    const rootBySession = {}; // resume_uuid -> earliest exec (root)
    for (const e of list) {
      if (!e.resume_uuid) continue;
      const cur = rootBySession[e.resume_uuid];
      if (!cur || new Date(e.started_at) < new Date(cur.started_at)) {
        rootBySession[e.resume_uuid] = e;
      }
    }
    const isRoot = (e) => !!e.resume_uuid && rootBySession[e.resume_uuid]?.id === e.id;
    const groupMap = {}; // root_exec_id -> { root: exec, children: [] }

    const topLevel = []; // 没有 resume_uuid 的独立行 + 各 session 根节点
    for (const e of list) {
      if (!e.resume_uuid) {
        // 独立 execution，无分组
        topLevel.push(e);
      } else if (isRoot(e)) {
        // 自己是 session 根节点
        groupMap[e.id] = { root: e, children: [] };
        topLevel.push(e);
      } else {
        // 是某根节点的子节点
        const root = rootBySession[e.resume_uuid];
        if (root && groupMap[root.id]) {
          groupMap[root.id].children.push(e);
        } else {
          // 根节点不在当前列表中（被截断了），当作独立行
          topLevel.push(e);
        }
      }
    }
    const renderRow = (e, depth) => {
      const dt = new Date(e.started_at).toLocaleString();
      const src = e.source === 'scheduled' ? '⏰' : e.source === 'continue' ? '💬' : '▶';
      // 优先用显式 status 字段（2026-06 加），fallback 老逻辑
      const isRunning = e.status ? e.status === 'running' : !e.completed_at;
      const isEvaluating = _evaluatingIds.has(e.id);
      let statusIcon, statusColor, statusTitle;
      let evalBadge = '';
      if (isRunning) {
        statusIcon = '⏳'; statusColor = 'var(--info,#3b82f6)';
        statusTitle = '执行中…（尚未 Finish）';
      } else if (e.status === 'success' || e.exit_code === 0) {
        statusIcon = '✓'; statusColor = 'var(--archived)';
        statusTitle = e.status ? `status=${e.status}, exit_code=0` : 'exit_code=0';
      } else if (e.status === 'timeout') {
        statusIcon = '⏱ 超时'; statusColor = 'var(--warning)';
        statusTitle = '执行超时（10/30 min）';
      } else if (e.status === 'cancelled') {
        // cancelled 状态涵盖两种来源：① 用户点 ⚠ 取消（手动）；② 服务重启时 ForceFinish（orphan on startup）。
        // 通过 error 文案区分,前端展示不同 tooltip——orphan 不是用户主动的,不能误导。
        const isOrphan = e.error && e.error.indexOf('orphaned on startup') >= 0;
        statusIcon = '⊘'; statusColor = 'var(--text-secondary)';
        statusTitle = isOrphan ? '服务重启时被强制结束（orphan on startup）' : '用户主动取消';
      } else {
        statusIcon = '✗ ' + e.exit_code; statusColor = 'var(--exception)';
        statusTitle = `status=${e.status || 'failed'}, exit_code=${e.exit_code}`;
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
        ? 'display:flex;gap:8px;padding:5px 6px;border-bottom:1px solid var(--border);font-size:11px;align-items:center;flex-wrap:nowrap;background:rgba(59,130,246,0.08);border-left:3px solid #3b82f6;' + indent
        : 'display:flex;gap:8px;padding:5px 6px;border-bottom:1px solid var(--border);font-size:11px;align-items:center;flex-wrap:nowrap;' + indent;
      const title = esc(e.task_title || e.scheduled_task_title || e.command || "(无标题)");
      const cmdTip = e.command ? esc(e.command.slice(0, 200)) : '';
      const hasKids = groupMap[e.id] && groupMap[e.id].children.length > 0;
      const toggle = hasKids
        ? `<span id="exec-toggle-${e.id}" onclick="toggleExecGroup('${e.id}')" style="cursor:pointer;color:var(--text-secondary);font-size:14px;flex-shrink:0" title="展开/折叠会话链">▶</span>`
        : '<span style="width:14px;flex-shrink:0;display:inline-block"></span>';
      const groupIcon = hasKids ? '💬' : src;
      const cliLabel = e.cli_type ? `[${e.cli_type}]` : '';
      const cliColor = e.cli_type === 'cbc' ? '#8b5cf6' : e.cli_type === 'shell' ? '#10b981' : '#3b82f6';
      const groupTitle = hasKids ? `<b>[会话链 ${groupMap[e.id].children.length + 1} 轮]</b> ` : '';
      // 来源 tooltip 改成中文,跟图标语义对齐(scheduled→定时任务 / continue→继续对话 / 其他→手动任务)
      const srcText = e.source === 'scheduled' ? '定时任务' : e.source === 'continue' ? '继续对话' : '手动任务';
      // 固定列宽占位:groupIcon(20px) + cliLabel(54px) 始终占位,无论有没有 cli_type,
      // title 起始 x 才能纵向对齐(不然 cli_label 时有时无会偏 50px)。
      return `<div style="${rowStyle}" data-exec-id="${e.id}">
        ${toggle}
        <span style="flex-shrink:0;width:20px;text-align:center" title="${srcText}">${groupIcon}</span>
        <span style="flex-shrink:0;width:54px;font-size:10px;font-weight:600;color:${cliColor};text-align:left">${cliLabel}</span>
        <span style="color:var(--text-secondary);font-family:monospace;flex-shrink:0;width:150px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${dt}</span>
        <span style="flex:1;min-width:0;font-size:11px;padding-left:72px;margin-left:-72px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;cursor:pointer" onclick="viewExecutionDetail('${e.id}')">${groupTitle}${title}</span>${cmdTip ? `<span title="命令: ${cmdTip}" style="margin-left:4px;font-size:11px;color:#60a5fa;flex-shrink:0">ⓘ</span>` : ''}
        <span style="font-size:11px;color:${statusColor};flex-shrink:0" title="${statusTitle}">${statusIcon}</span>
        ${evalBadge}
        <button class="btn btn-small" onclick="viewExecutionDetail('${e.id}')" title="查看详情">📋</button>
        <button class="btn btn-small" onclick="runEvaluation('${e.id}')" title="AI 评估" style="${isEvaluating?'opacity:0.5;cursor:wait':''}">${isEvaluating?'⏳':'📊'}</button>
        ${(() => {
          // 「取消」按钮只在 execution 已卡住 ≥1 分钟时显示：
          // 任务刚启动时（短任务通常几秒~几分钟内完成）不需要这个手动恢复入口，
          // 避免误操作。1 分钟后还没完成才算疑似卡住（正常超时至少 5~10 分钟）。
          // 列表 30s 自动刷新或 WS 推送时会重渲,ageMs 自然重新计算。
          if (!isRunning) return '';
          const startedMs = e.started_at ? new Date(e.started_at).getTime() : 0;
          const ageMs = Date.now() - startedMs;
          if (ageMs < 60000) return '';
          return `<button class="btn btn-small" style="background:var(--warning);color:#fff;border-color:var(--warning)" onclick="cancelExecution('${e.id}')" title="已运行 ${Math.floor(ageMs/60000)} 分钟仍未完成，点击取消（强制结束）">⚠ 取消</button>`;
        })()}
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

// loadMoreExecutions: 增加 recentExecLimit + 重渲染最近执行列表。
async function loadMoreExecutions() {
  recentExecLimit += 50;
  // 给所有"加载更多"按钮加 loading 反馈（innerHTML 重渲染前）
  document.querySelectorAll('[data-exec-loadmore]').forEach(b => {
    b.disabled = true; b.textContent = '⏳ 加载中...';
  });
  await loadRecentExecutions();
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
  // 立刻重置继续对话按钮状态,避免上一次 viewExecutionDetail 留下的
  // disabled + 误导性 tooltip 在 fetch 完成前被用户 hover 看到。
  const cbInit = document.getElementById('exec-continue-btn');
  if (cbInit) { cbInit.disabled = true; cbInit.title = '加载中...'; }
  try {
    const exec = await fetchJSON('/api/executions/' + id);
    document.getElementById('exec-detail-cmd').value = exec.command || '';
    // 解析 claude -p --output-format json：取 result 字段，附带 num_turns 元数据
    const renderedOutput = renderExecOutput(exec.output);
    // 默认只显示 AI 答复部分，辅助信息可切换展开
    const aiReplyOnly = extractAIReplyOnly(renderedOutput);
    const outputEl = document.getElementById('exec-detail-output');
    outputEl.value = aiReplyOnly;
    outputEl._fullOutput = renderedOutput;       // 存完整版
    outputEl._showingFull = false;               // 当前状态
    const toggle = document.getElementById('exec-output-toggle');
    if (toggle) {
      const hasExtra = renderedOutput !== aiReplyOnly;
      toggle.style.display = hasExtra ? 'inline' : 'none';
      toggle.textContent = hasExtra ? '▼ 显示全部输出（含 CLI 元数据 + 动作清单的完整输出）' : '';
    }
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
    // 面包屑：如果该 exec 属于某会话链（resume_uuid 存在），计算并显示「第 K 轮 / 共 N 轮」
    // 用 id="exec-chain-crumb" 标记,submitContinue 后可单独更新这一格,不用重渲整个 meta。
    let crumb = '';
    if (exec.resume_uuid && exec.task_id) {
      try {
        const siblings = await fetchJSON('/api/tasks/' + exec.task_id + '/executions');
        crumb = buildExecChainCrumb(exec, siblings);
      } catch (_) { /* 面包屑拉取失败不影响主流程 */ }
    }
    document.getElementById('exec-detail-meta').innerHTML =
      '<span style="font-size:11px;font-weight:600;' + srcColor + '">' + srcText + '</span>' +
      ' <span style="margin-left:8px;color:var(--text-secondary)">|</span>' +
      ' <span style="cursor:help" title="' + detailTip + '"><b>' + esc(nameDisplay) + '</b> ⓘ</span>' +
      ' <span style="margin-left:8px;color:var(--text-secondary)">|</span>' +
      ' exit_code=' + exitDisplay + ' · ' + new Date(exec.started_at).toLocaleString() + ' · 耗时 ' + dur +
      crumb;

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
    // 继续对话按钮：有 resume_uuid 时可点，否则禁用 + tooltip 提示
    //（不隐藏：让用户知道这个功能存在，理解为什么这个 execution 不可续）
    // 启用态用 btn-primary（蓝色）跟「📊 评估」主操作对齐，禁用态用 btn-secondary（灰色）
    //——之前一直用灰色，看着跟「关闭」按钮一样没主次，启用后也不够显眼。
    const continueBtn = document.getElementById('exec-continue-btn');
    if (continueBtn) {
      // 禁用态判定：① 没有 resume_uuid；② 同会话链（resume_uuid 相同）上存在尚未完成的执行
      //（包括 exec 自己还在 running,或者之前那轮 continue 还没跑完）。
      // 避免用户在前一轮对话还没产出结果时就续一轮,session 状态会打架。
      let blockedReason = null;
      if (!exec.resume_uuid) {
        blockedReason = '该执行未生成 session_id，无法继续对话（执行失败或尚未完成）';
      } else if (exec.task_id) {
        try {
          const siblings = await fetchJSON('/api/tasks/' + exec.task_id + '/executions');
          const chainRunning = (siblings || []).some(s =>
            s.resume_uuid === exec.resume_uuid &&
            (s.status === 'running' || !s.completed_at)
          );
          if (chainRunning) blockedReason = '该会话链上存在尚未完成的对话，无法继续对话';
        } catch (_) { /* 拉取失败时按"放行"处理,不影响主流程 */ }
      }
      if (blockedReason) {
        continueBtn.classList.remove('hidden', 'btn-primary');
        continueBtn.classList.add('btn-secondary');
        continueBtn.disabled = true;
        continueBtn.title = blockedReason;
      } else {
        continueBtn.classList.remove('hidden', 'btn-secondary');
        continueBtn.classList.add('btn-primary');
        continueBtn.disabled = false;
        continueBtn.removeAttribute('title');
      }
    }
    // 拉已有评估（展示所有历史）
    try {
      const evals = await fetchJSON('/api/executions/' + id + '/evaluations');
      renderEvalHistory(evals);
    } catch (e) { /* 忽略 */ }
    // 加载对话历史
    loadExecConversation(id);
    document.getElementById('exec-detail-modal').classList.remove('hidden');
  } catch (e) {
    alert('加载执行详情失败：' + e.message);
    // 失败时也要恢复继续对话按钮的 loading 状态,避免按钮永久 disabled + title
    // 永久停留在"加载中..."(入口 cbInit 设的状态)。
    // 错误信息塞进 tooltip,用户 hover 知道为什么 + 关闭重开 modal 重试。
    if (cbInit) { cbInit.disabled = true; cbInit.title = '加载失败：' + e.message + '（关闭弹窗重试）'; }
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
  // 回车触发提交（一次性绑定,避免反复 show 时累积监听器）
  if (!input.dataset.enterBound) {
    input.addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter') {
        ev.preventDefault();
        submitContinue();
      }
    });
    input.dataset.enterBound = '1';
  }
}

// buildExecChainCrumb 纯函数：给定 exec 和其 task 的全部 executions,返回「会话链 第 K/N 轮」HTML。
// 找不到 idx（exec 不在链里）或参数不全,返回空字符串。
// span 带 id="exec-chain-crumb",便于 updateExecChainCrumb 原地替换。
function buildExecChainCrumb(exec, siblings) {
  if (!exec || !exec.resume_uuid || !exec.task_id) return '';
  const sameChain = (siblings || []).filter(s => s.resume_uuid === exec.resume_uuid)
    .sort((a, b) => new Date(a.started_at) - new Date(b.started_at));
  const idx = sameChain.findIndex(s => s.id === exec.id);
  if (idx < 0) return '';
  return `<span id="exec-chain-crumb" style="margin-left:8px;color:var(--text-secondary);background:var(--hover);padding:1px 6px;border-radius:3px;font-size:11px">会话链 第 ${idx + 1}/${sameChain.length} 轮</span>`;
}

// updateExecChainCrumb 重新拉 exec + 同 task executions,更新 meta 里的「会话链 第 K/N 轮」徽章。
// 调用时机：submitContinue 提交成功后会话链新增一轮,K/N 都要重算;若以后引入 WS 推送也可复用。
// 已存在的 #exec-chain-crumb span 会被原地替换,避免重渲整个 meta(里面包含 exit_code/耗时 等其他信息)。
async function updateExecChainCrumb(execId) {
  const meta = document.getElementById('exec-detail-meta');
  if (!meta) return;
  let exec, siblings;
  try {
    exec = await fetchJSON('/api/executions/' + execId);
    siblings = await fetchJSON('/api/tasks/' + exec.task_id + '/executions');
  } catch (_) { return; /* 拉取失败不动 UI,保持上次状态 */ }
  const html = buildExecChainCrumb(exec, siblings);
  const existing = document.getElementById('exec-chain-crumb');
  if (existing) {
    if (html) existing.outerHTML = html;
    else existing.remove();
  } else if (html) {
    meta.insertAdjacentHTML('beforeend', html);
  }
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
    const newExecId = res && res.execution_id;
    document.getElementById('exec-continue-section').classList.add('hidden');
    document.getElementById('exec-continue-prompt').value = '';
    // 刷新执行列表
    loadRecentExecutions();
    // 不立即关 modal：用户刚发的 prompt 重要，要让用户看到反馈。
    // 在 modal 顶部插入一条反馈条带（包含新 exec ID + 点击跳转详情）。
    _showContinueFeedback(newExecId, prompt);
    // 刷新 exec-detail-modal 里的对话历史 timeline（修 bug：之前只刷 task-modal，
    // exec-detail-modal 的 timeline 一直停留，跑到结束 modal 重开才"出现"，体验割裂）。
    if (typeof loadExecConversation === 'function') {
      loadExecConversation(currentExecId);
    }
    // 同步刷新顶部「会话链 第 K/N 轮」面包屑：新 exec 已落库,链长 +1,K 也要重算
    //（之前要关闭重开 modal 才会刷新,体验割裂）。
    if (typeof updateExecChainCrumb === 'function') {
      updateExecChainCrumb(currentExecId);
    }
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

// _showContinueFeedback 在 exec-detail-modal 顶部插入一条反馈条带，
// 告诉用户“你刚发的 prompt 是什么 / 产生的新执行 ID / 点此跳详情”。
// 这解决了之前 closeExecDetailModal 后用户丢失上下文的 UX 问题。
function _showContinueFeedback(newExecId, promptText) {
  // 只在当前查看的 exec 与新提交的一致时才显示反馈条带
  // 切换到其他 exec 时旧条带自动失效，不应带过去
  if (currentExecId !== newExecId) return;
  // 移除旧条带（如果有）
  const old = document.getElementById('continue-feedback-strip');
  if (old) old.remove();
  const meta = document.getElementById('exec-detail-meta');
  if (!meta) return;
  const strip = document.createElement('div');
  strip.id = 'continue-feedback-strip';
  strip.style.cssText = 'background:rgba(16,185,129,0.12);border:1px solid #10b981;border-radius:6px;padding:10px 12px;margin-bottom:8px;font-size:12px;line-height:1.5';
  const promptShort = promptText.length > 80 ? promptText.slice(0, 80) + '...' : promptText;
  strip.innerHTML = `
    <div style="color:#10b981;font-weight:600;margin-bottom:4px">✓ 继续对话已提交</div>
    <div style="color:var(--text-secondary);margin-bottom:4px">你的 prompt：<span style="color:var(--text)">${esc(promptShort)}</span></div>
    <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
      <span style="color:var(--text-secondary)">新执行 ID：</span>
      <code style="background:var(--hover);padding:1px 6px;border-radius:3px;font-size:11px">${esc(newExecId || '?')}</code>
      <button class="btn btn-small" onclick="if('${esc(newExecId || '')}'){viewExecutionDetail('${esc(newExecId || '')}')}">📄 查看新执行详情</button>
      <button class="btn btn-small btn-secondary" onclick="document.getElementById('continue-feedback-strip')?.remove()">✕ 关闭反馈</button>
    </div>
  `;
  // 插在 meta 元素后，但要在滚动容器前
  const scrollContainer = meta.closest('div[style*="overflow-y:auto"]');
  if (scrollContainer && scrollContainer.parentNode) {
    scrollContainer.parentNode.insertBefore(strip, scrollContainer);
  } else {
    meta.parentNode.insertBefore(strip, meta.nextSibling);
  }
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
  const chainCheck = document.getElementById('eval-chain-checkbox');
  const evalChain = chainCheck && chainCheck.checked;
  // 评估时禁用 select 避免反复改
  const sel = document.getElementById('eval-model-select');
  if (sel) sel.disabled = true;
  const timeoutInput = document.getElementById('eval-timeout');
  const timeoutSec = timeoutInput ? parseInt(timeoutInput.value) || 120 : 120;
  const btn = event && event.target;
  const oldText = btn && btn.textContent;
  if (btn) { btn.disabled = true; btn.textContent = '⏳'; }
  _markEvaluating(execId, true);

  // 如果是评估整个会话链，直接调用 chain API
  const evalStartedAt = new Date();
  try {
    if (evalChain) {
      // 合并评估
      await fetchJSON('/api/executions/' + execId + '/evaluate-chain', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({cli_type: getEvalCliType(), model: getEvalModel(), timeout_sec: timeoutSec})});
    } else {
      // 单个评估
      await fetchJSON('/api/executions/' + execId + '/evaluate', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({cli_type: getEvalCliType(), model: getEvalModel(), timeout_sec: timeoutSec})});
    }
    // 轮询拿结果（评估异步执行，根据超时时间动态计算轮次）
    const maxIterations = Math.max(60, Math.ceil(timeoutSec / 2) + 10); // 至少60次，或 timeoutSec/2 + 10
    for (let i = 0; i < maxIterations; i++) {
      await new Promise(r => setTimeout(r, 2000));
      const evals = await fetchJSON('/api/executions/' + execId + '/evaluations');
      if (evals && evals.length > 0) {
        // 只认定本次评估开始后创建的新评估
        const newEvals = evals.filter(e => e.created_at && new Date(e.created_at) > evalStartedAt);
        if (newEvals.length > 0) {
          if (currentExecId === execId) renderEvalHistory(evals);
          _markEvaluating(execId, false);
          loadRecentExecutions();
          return;
        }
      }
      // 还在评估中，持续更新状态
      if (currentExecId === execId && i % 2 === 0) {
        const card = document.getElementById('exec-detail-eval');
        const chainTip = evalChain ? '（会话链）' : '';
        card.innerHTML = `<div style="font-size:13px"><span style="color:var(--info,#3b82f6)">⏳ 评估中${chainTip}…</span> <span style="color:var(--text-secondary);font-size:11px">已 ${(i+1)*2}s</span></div>`;
      }
    }
    alert('评估超时（>' + timeoutSec + ' 秒），请检查 claude CLI 是否可用');
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

// cancelExecution 强制结束卡住的 execution。
// 解决 WS 断连后执行列表里永远显示「运行中」的问题——用户能一键自救。
// 后端 /api/executions/{id}/cancel 会智能选择：in-flight 调 cancel func；否则直接写 completed_at。
async function cancelExecution(id) {
  if (!id) return;
  if (!confirm('确认取消这条执行？\n\n适用场景：子进程已死但 DB 没收到 done 事件（WS 断连 / 服务重启后），导致一直显示「运行中」。\n\n已运行超过 1 分钟还没完成才显示这个按钮，确认执行是真正卡住了再点。')) return;
  try {
    const resp = await fetchJSON('/api/executions/' + id + '/cancel', {method: 'POST'});
    const mode = resp.mode || (resp.already_done ? 'already_done' : 'unknown');
    console.log('[cancelExecution]', id, mode, resp);
    // 立即刷新一次（不等 setInterval）
    loadRecentExecutions();
    if (currentExecId === id) {
      // 如果当前打开的是这个 exec 的 detail modal，也重新拉一次详情
      viewExecutionDetail(id);
    }
  } catch (e) {
    alert('取消失败：' + (e.message || e));
  }
}

// execAutoRefreshTimer 30s 兜底轮询：即使 _autoRefreshEnabled = false（用户暂停了自动刷新），
// 也能保证 running execution 状态最终更新。避免 WS 断连 + 用户暂停刷新双重盲区。
// 注意：主刷新由 startAutoRefresh（默认 3s）负责，这里只是兜底。
let _execAutoRefreshTimer = null;
function _startExecAutoRefresh() {
  if (_execAutoRefreshTimer) clearInterval(_execAutoRefreshTimer);
  _execAutoRefreshTimer = setInterval(() => {
    // 只在 automation tab 可见时跑
    if (document.hidden) return;
    const automationTab = document.getElementById('page-automation');
    if (!automationTab || automationTab.classList.contains('hidden')) return;
    // 找列表容器，强制刷一次（不读 _autoRefreshEnabled 标志，覆盖用户暂停的场景）
    const el = document.getElementById('recent-execs');
    const el2 = document.getElementById('exec-list');
    if (el || el2) loadRecentExecutions();
  }, 30000);
}
_startExecAutoRefresh(); // 启动一次，30s 兜底

// renderExecOutput 解析 `claude -p --output-format json` 的输出，提取 result 字段。
// 非 JSON 格式 fallback 原样返回。
// 输出结构重排：AI 答复 → 执行信息 → 辅助评估（动作清单）
function renderExecOutput(raw) {
  if (!raw) return '(无输出)';
  const trimmed = raw.trim();
  const sessionId = extractSessionId(trimmed);
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) return raw;
  let obj;
  try { obj = JSON.parse(trimmed); } catch { return raw; }
  // claude json 输出结构（单对象）
  if (typeof obj.result === 'string') {
    const resultText = obj.result || '';
    // 动作清单从 "## 动作清单" 标记开始，后面是 AI 自评内容
    const actionReportIdx = resultText.indexOf('## 动作清单');
    let aiReply = resultText;
    let actionReport = '';
    if (actionReportIdx !== -1) {
      aiReply = resultText.slice(0, actionReportIdx).trim();
      actionReport = resultText.slice(actionReportIdx);
    }

    const sections = [];

    // 1. AI 答复（最上面，用户最关心）
    if (aiReply) {
      sections.push('=== AI 答复 ===\n' + aiReply);
    }

    // 2. AI CLI 执行信息
    const meta = [];
    if (sessionId) meta.push(`sessionId=${sessionId}`);
    if (typeof obj.num_turns === 'number') meta.push(`num_turns=${obj.num_turns}`);
    if (obj.is_error) meta.push('is_error=true');
    if (obj.stop_reason) meta.push(`stop_reason=${obj.stop_reason}`);
    if (obj.permission_denials && obj.permission_denials.length) {
      meta.push(`permission_denials=[${obj.permission_denials.join(',')}]`);
    }
    if (meta.length) {
      sections.push('--- AI CLI 执行信息 ---\n' + meta.join(' | '));
    }

    // 3. AI 自评动作清单（辅助评估）
    if (actionReport) {
      sections.push('--- AI 自评动作清单（辅助评估） ---\n' + actionReport);
    }

    return sections.join('\n\n');
  }
  // cbc 最终输出也是 {type:"result", result:"..."}，与 claude 结构一致
  if (Array.isArray(obj)) {
    for (const item of obj) {
      if (item.type === 'result' && typeof item.result === 'string') {
        const resultText = item.result || '';
        const actionReportIdx = resultText.indexOf('## 动作清单');
        let aiReply = resultText;
        let actionReport = '';
        if (actionReportIdx !== -1) {
          aiReply = resultText.slice(0, actionReportIdx).trim();
          actionReport = resultText.slice(actionReportIdx);
        }
        const sections = [];
        if (aiReply) sections.push('=== AI 答复 ===\n' + aiReply);
        const meta = [];
        if (item.num_turns != null) meta.push(`num_turns=${item.num_turns}`);
        if (item.is_error) meta.push('is_error=true');
        if (item.session_id) meta.push(`sessionId=${item.session_id}`);
        if (meta.length) sections.push('--- AI CLI 执行信息 ---\n' + meta.join(' | '));
        if (actionReport) sections.push('--- AI 自评动作清单（辅助评估） ---\n' + actionReport);
        return sections.join('\n\n');
      }
    }
    // 兜底：流式中间数组，找 assistant 消息
    let resultText = '';
    for (const item of obj) {
      if (item.type === 'message' && item.role === 'assistant' && Array.isArray(item.content)) {
        for (const c of item.content) {
          if (c.type === 'text' || c.type === 'output_text') {
            resultText += c.text + '\n';
          }
        }
      }
    }
    const actionReportIdx = resultText.indexOf('## 动作清单');
    let aiReply = resultText.trim();
    let actionReport = '';
    if (actionReportIdx !== -1) {
      aiReply = resultText.slice(0, actionReportIdx).trim();
      actionReport = resultText.slice(actionReportIdx);
    }
    const sections = [];
    if (aiReply) sections.push('=== AI 答复 ===\n' + aiReply);
    const meta = [];
    if (sessionId) meta.push(`sessionId=${sessionId}`);
    if (meta.length) sections.push('--- AI CLI 执行信息 ---\n' + meta.join(' | '));
    if (actionReport) sections.push('--- AI 自评动作清单（辅助评估） ---\n' + actionReport);
    return sections.join('\n\n');
  }
  return raw;
}

// extractAIReplyOnly 从 renderExecOutput 的输出中只提取 AI 答复部分，
// 用于默认折叠显示。
function extractAIReplyOnly(rendered) {
  if (!rendered) return rendered;
  // 第一个 "=== AI 答复 ===\n" 到第一个 "\n\n---" 之前是 AI 答复
  const match = rendered.match(/^=== AI 答复 ===\n([\s\S]*?)(?=\n\n---)/);
  return match ? match[0] : rendered;
}

// toggleExecOutput 切换执行输出的完整/简洁视图。
function toggleExecOutput(link) {
  const el = document.getElementById('exec-detail-output');
  if (!el || !el._fullOutput) return;
  if (el._showingFull) {
    el.value = extractAIReplyOnly(el._fullOutput);
    el._showingFull = false;
    link.textContent = '▶ 显示全部输出（含 CLI 元数据 + 动作清单的完整输出）';
  } else {
    el.value = el._fullOutput;
    el._showingFull = true;
    link.textContent = '▼ 收起辅助信息';
  }
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
    // fix: 字段从 terminal_type 改为顶层 default_terminal
    await fetchJSON('/api/config', {
      method: 'PUT',
      body: JSON.stringify({ default_terminal: value }),
    });
  } catch (e) {
    console.warn('保存默认终端失败:', e);
  }
}

// 对话历史
function toggleExecConversation() {
  const body = document.getElementById('exec-conversation-body');
  const arrow = document.getElementById('exec-conversation-toggle');
  if (body.classList.contains('hidden')) {
    body.classList.remove('hidden');
    arrow.style.transform = 'rotate(90deg)';
  } else {
    body.classList.add('hidden');
    arrow.style.transform = 'rotate(0deg)';
  }
}

// 一键展开/收起所有命令和输出详情
let _execDetailsExpanded = false;
function toggleAllExecDetails() {
  _execDetailsExpanded = !_execDetailsExpanded;
  const body = document.getElementById('exec-conversation-body');
  if (!body) return;
  body.querySelectorAll('pre[id^="tl-cmd-"], pre[id^="tl-out-"], pre[id^="tl-fullout-"]').forEach(el => {
    if (_execDetailsExpanded) {
      el.classList.remove('hidden');
    } else {
      el.classList.add('hidden');
    }
  });
  // 更新按钮文字
  const btn = document.querySelector('button[onclick="toggleAllExecDetails()"]');
  if (btn) btn.textContent = _execDetailsExpanded ? '收起详情' : '展开详情';
}

async function loadExecConversation(execId) {
  const countEl = document.getElementById('exec-conversation-count');
  const body = document.getElementById('exec-conversation-body');
  try {
    const exec = await fetchJSON('/api/executions/' + execId);
    const resumeUuid = exec?.resume_uuid;
    let execs;
    if (resumeUuid) {
      execs = await fetchJSON('/api/executions?resume_uuid=' + encodeURIComponent(resumeUuid));
    } else {
      execs = [exec];
    }
    body.innerHTML = renderExecConversationTimeline(execs);
    countEl.textContent = '(' + execs.length + ' 轮)';
  } catch (e) {
    body.innerHTML = '<div style="padding:8px;color:var(--exception);font-size:12px">加载失败：' + esc(e.message) + '</div>';
  }
}

function renderExecConversationTimeline(execs) {
  if (!execs || execs.length === 0) {
    return '<div style="padding:8px;color:var(--text-secondary);font-size:12px">暂无对话历史</div>';
  }
  const totalRounds = execs.length;
  // 倒序排列：最近的在前，最早的在后
  execs.sort((a, b) => new Date(b.started_at) - new Date(a.started_at));
  // 根节点是最早的一条，倒序后在末尾
  const rootIdx = execs.length - 1;
  return execs.map((e, idx) => {
    // idx=0 是最新的，显示最大轮次；idx=rootIdx 显示"原始"
    const isRoot = idx === rootIdx;
    const roundNum = totalRounds - idx;
    const tag = isRoot
      ? '<span style="background:var(--accent);color:#fff;padding:1px 6px;border-radius:3px;font-size:10px">原始</span>'
      : '<span style="background:#0ea5e9;color:#fff;padding:1px 6px;border-radius:3px;font-size:10px">第' + roundNum + '轮</span>';
    const ts = e.started_at ? new Date(e.started_at).toLocaleString('zh-CN', {hour12: false}) : '';
    // 状态指示：running 的 exec exit_code 默认 0，会被错画成 ✓，先看 completed_at（2026-06 status 字段也可用）
    const isRunning = !e.completed_at || e.status === 'running';
    const status = isRunning
      ? '<span style="color:var(--info,#3b82f6);font-size:10px" title="运行中…">⏳</span>'
      : (e.exit_code === 0
        ? '<span style="color:#10b981;font-size:10px">✓</span>'
        : '<span style="color:var(--exception);font-size:10px">✗</span>');
    const prompt = e.prompt ? esc(e.prompt) : '<i style="color:var(--text-secondary)">(无prompt)</i>';
    // command 折叠展示
    const cmdId = 'tl-cmd-' + e.id.replace(/[^a-zA-Z0-9]/g, '_');
    const cmd = e.command ? esc(e.command) : '';
    const hasCmd = e.command && e.command.length > 0;
    // output 默认展开，尝试解析 JSON 提取 result 字段
    const outId = 'tl-out-' + e.id.replace(/[^a-zA-Z0-9]/g, '_');
    const fullOutId = 'tl-fullout-' + e.id.replace(/[^a-zA-Z0-9]/g, '_');
    let out = e.output ? e.output : '';
    let hasOut = e.output && e.output.length > 0;
    // 尝试解析 JSON，提取 result 字段作为自然语言展示
    if (hasOut) {
      try {
        const obj = JSON.parse(e.output);
        // claude: {result: "..."}
        if (obj && obj.result && typeof obj.result === 'string') {
          out = obj.result;
        } else if (obj && obj.error) {
          out = '错误: ' + obj.error;
        } else if (Array.isArray(obj)) {
          // cbc: 在数组中找 {type:"result", result:"..."}
          for (const item of obj) {
            if (item.type === 'result' && typeof item.result === 'string') {
              out = item.result;
              break;
            }
          }
        }
      } catch (_) {
        // 解析失败，使用原始内容
      }
    }
    // 从 out 中提取 AI 答复（去掉动作清单），用于默认展示
    const actionReportIdx = out.indexOf('## 动作清单');
    const aiReplyForList = actionReportIdx !== -1 ? out.slice(0, actionReportIdx).trim() : out.trim();
    const hasActionReport = actionReportIdx !== -1;
    // error 折叠展示
    const errId = 'tl-err-' + e.id.replace(/[^a-zA-Z0-9]/g, '_');
    const hasErr = e.error && e.error.length > 0;
    const err = e.error ? esc(e.error.slice(0, 200) + (e.error.length > 200 ? '...' : '')) : '';

    return '<div style="padding:8px 0;border-bottom:1px solid var(--border)">' +
      '<div style="display:flex;gap:6px;align-items:flex-start">' +
        '<div style="flex-shrink:0;margin-top:2px">' + status + '</div>' +
        '<div style="flex:1;min-width:0">' +
          '<div style="display:flex;gap:4px;align-items:center;margin-bottom:4px">' + tag + '<span style="color:var(--text-secondary);font-size:10px">' + ts + '</span></div>' +
          '<div style="color:var(--text);font-size:11px;margin-bottom:4px"><b style="color:#0ea5e9">问:</b> ' + prompt + '</div>' +
          (hasCmd ? '<div style="margin-bottom:4px"><span style="font-size:10px;color:var(--text-secondary);cursor:pointer" onclick="(function(self){var p=document.getElementById(\'' + cmdId + '\');if(p.classList.contains(\'hidden\')){p.classList.remove(\'hidden\');self.textContent=\'▼ 命令\';}else{p.classList.add(\'hidden\');self.textContent=\'▶ 命令\';}})(this)">▶ 命令</span><pre id="' + cmdId + '" class="hidden" style="margin:4px 0 0;padding:6px;background:var(--hover);border:1px solid var(--border);border-radius:3px;font-size:10px;white-space:pre-wrap;word-break:break-all;max-height:150px;overflow-y:auto">' + cmd + '</pre></div>' : '') +
          (hasOut ? '<div style="margin-bottom:4px"><span style="font-size:10px;color:var(--text-secondary);cursor:pointer" onclick="(function(self){var p=document.getElementById(\'' + outId + '\');if(p.classList.contains(\'hidden\')){p.classList.remove(\'hidden\');self.textContent=\'▼ 输出\';}else{p.classList.add(\'hidden\');self.textContent=\'▶ 输出\';}})(this)">▶ 输出</span><pre id="' + outId + '" style="margin:4px 0 0;padding:6px;background:var(--hover);border:1px solid var(--border);border-radius:3px;font-size:10px;white-space:pre-wrap;word-break:break-all;max-height:200px;overflow-y:auto;color:var(--archived)">' + esc(aiReplyForList) + '</pre>' + (hasActionReport ? '<a href="#" class="action-report-toggle" style="font-size:10px;color:var(--primary);text-decoration:none" onclick="var f=document.getElementById(\'' + fullOutId + '\');var t=this;if(f.classList.contains(\'hidden\')){f.classList.remove(\'hidden\');t.textContent=\'▼ 含动作清单的完整输出（辅助评估）\';}else{f.classList.add(\'hidden\');t.textContent=\'▶ 含动作清单的完整输出（辅助评估）\';}return false">▶ 含动作清单（辅助评估）</a><pre id="' + fullOutId + '" class="hidden" style="margin:4px 0 0;padding:6px;background:var(--hover);border:1px solid var(--border);border-radius:3px;font-size:10px;white-space:pre-wrap;word-break:break-all;max-height:200px;overflow-y:auto;color:var(--archived)">' + esc(out) + '</pre>' : '') + '</div>' : '') +
          (hasErr ? '<div style="margin-bottom:4px;color:var(--exception);font-size:10px">✗ 错误: ' + err + '</div>' : '') +
        '</div>' +
      '</div>' +
    '</div>';
  }).join('');
}


// 页面加载时从 /api/config 读取终端类型设置
async function loadTerminalSetting() {
  try {
    const data = await fetchJSON('/api/config');
    // fix: 默认终端字段从 terminal.default_type 改为顶层 default_terminal
    const val = data.default_terminal || data.terminal?.default_type || 'wezterm';
    const sel = document.getElementById('default-terminal-select');
    if (sel) sel.value = val;
  } catch (e) {
    const sel = document.getElementById('default-terminal-select');
    if (sel) sel.value = 'wezterm';
  }
}
