// Tasks Tab：列表 + 状态过滤 + 认领/归档 + task modal（新建/查看/编辑）
// 依赖 api.js

let tasks = [];

// ===== 多选经验库（exp picker） =====
// _selectedExps: [{id, module, scene, keywords}]
let _selectedExps = [];
let _allExpsCache = null; // 缓存全量经验库

async function _loadAllExps() {
  if (_allExpsCache) return _allExpsCache;
  _allExpsCache = await fetchJSON('/api/experiences');
  return _allExpsCache;
}

async function openExpPicker() {
  // 初始填充：已选 + 全部
  const all = await _loadAllExps();
  _renderExpPickerList(all, '');
  document.getElementById('exp-picker-search').value = '';
  document.getElementById('exp-picker-modal').classList.remove('hidden');
  setTimeout(() => document.getElementById('exp-picker-search').focus(), 50);
}

function _renderExpPickerList(all, keyword) {
  const kw = (keyword || '').toLowerCase();
  const filtered = kw
    ? all.filter(e => (e.module || '').toLowerCase().includes(kw)
                   || (e.scene || '').toLowerCase().includes(kw)
                   || (e.keywords || '').toLowerCase().includes(kw))
    : all;
  const selectedIds = new Set(_selectedExps.map(s => s.id));
  document.getElementById('exp-picker-list').innerHTML = filtered.length === 0
    ? '<div class="chip-empty" style="padding:20px">无匹配</div>'
    : filtered.map(e => {
        const checked = selectedIds.has(e.id) ? 'checked' : '';
        const selected = selectedIds.has(e.id) ? ' selected' : '';
        return `<label class="exp-picker-item${selected}">
          <input type="checkbox" data-exp-id="${e.id}" ${checked}>
          <div style="flex:1">
            <div><span class="ep-module">${esc(e.module)}</span><span class="ep-id">${e.id.slice(0, 8)}</span></div>
            ${e.scene ? `<div class="ep-scene">${esc(e.scene)}</div>` : ''}
            ${e.keywords ? `<div class="ep-kw">🏷 ${esc(e.keywords)}</div>` : ''}
          </div>
        </label>`;
      }).join('');
  _updateExpPickerCount();
  // 绑定 change 事件
  document.querySelectorAll('#exp-picker-list input[type=checkbox]').forEach(cb => {
    cb.addEventListener('change', e => {
      const id = e.target.dataset.expId;
      const exp = all.find(x => x.id === id);
      if (!exp) return;
      if (e.target.checked) {
        if (!_selectedExps.find(s => s.id === id)) _selectedExps.push({id, module: exp.module, scene: exp.scene, keywords: exp.keywords});
      } else {
        _selectedExps = _selectedExps.filter(s => s.id !== id);
      }
      e.target.closest('.exp-picker-item').classList.toggle('selected', e.target.checked);
      _updateExpPickerCount();
    });
  });
}

function _updateExpPickerCount() {
  document.getElementById('exp-picker-count').textContent = `已选 ${_selectedExps.length} 条`;
}

document.getElementById('exp-picker-search').addEventListener('input', debounce(async e => {
  const all = await _loadAllExps();
  _renderExpPickerList(all, e.target.value);
}, 200));

function closeExpPicker() { document.getElementById('exp-picker-modal').classList.add('hidden'); }
function confirmExpPicker() {
  renderTaskExpChips();
  closeExpPicker();
}

// 渲染任务 modal 内的已选 chip
function renderTaskExpChips() {
  const el = document.getElementById('task-exps-list');
  if (_selectedExps.length === 0) {
    el.innerHTML = '<span class="chip-empty">未选</span>';
    return;
  }
  el.innerHTML = _selectedExps.map(s => {
    const text = s.scene || s.module || '';
    return `<span class="chip" data-exp-id="${s.id}">
      <span class="chip-id">${s.id.slice(0, 8)}</span>
      <span class="chip-text">${esc(text)}</span>
      <span class="chip-del" onclick="removeExpFromTask('${s.id}')" title="移除">×</span>
    </span>`;
  }).join('');
}

function removeExpFromTask(id) {
  _selectedExps = _selectedExps.filter(s => s.id !== id);
  renderTaskExpChips();
}

let _taskSortField = 'created_at';
let _taskSortDir = 'desc';
function setTaskSort(field) {
  if (_taskSortField === field) {
    _taskSortDir = _taskSortDir === 'asc' ? 'desc' : 'asc';
  } else {
    _taskSortField = field;
    _taskSortDir = 'desc';
  }
  loadTasks();
}
function sortIcon(field) {
  if (_taskSortField !== field) return ' ↕';
  return _taskSortDir === 'asc' ? ' ↑' : ' ↓';
}

async function loadTasks() {
  const status = document.getElementById('filter-status').value;
  const taskType = document.getElementById('filter-task-type').value;
  const params = [];
  if (status) params.push('status=' + status);
  if (taskType) params.push('task_type=' + taskType);
  const url = API + '/api/tasks' + (params.length ? '?' + params.join('&') : '');
  try {
    tasks = await fetchJSON(url);
    renderTaskTable(tasks);
  } catch(e) { console.error('[loadTasks] err:', e); }
}

function renderTaskTable(list) {
  const el = document.getElementById('task-list');
  if (!list || list.length === 0) {
    el.innerHTML = '<div class="empty">暂无任务</div>';
    document.getElementById('task-count').textContent = '0 条任务';
    return;
  }
  // 排序
  const sorted = [...list].sort((a, b) => {
    let va = a[_taskSortField], vb = b[_taskSortField];
    if (_taskSortField === 'created_at') { va = new Date(va); vb = new Date(vb); }
    if (va < vb) return _taskSortDir === 'asc' ? -1 : 1;
    if (va > vb) return _taskSortDir === 'asc' ? 1 : -1;
    return 0;
  });
  document.getElementById('task-count').textContent = list.length + ' 条任务';
  el.innerHTML = `<table class="task-table">
    <thead><tr>
      <th class="col-title" style="text-align:left">标题</th>
      <th class="col-status" style="cursor:pointer" onclick="setTaskSort('status')">状态${sortIcon('status')}</th>
      <th class="col-type">类型</th>
      <th class="col-loop">优化</th>
      <th class="col-time" style="cursor:pointer" onclick="setTaskSort('created_at')">创建时间${sortIcon('created_at')}</th>
      <th class="col-ops" style="text-align:left">操作</th>
    </tr></thead>
    <tbody>${sorted.map(t => {
      const ops = taskOpsByStatus(t);
      return `
      <tr>
        <td class="col-title">
          <div style="display:flex;align-items:center;gap:6px">
            <span class="title" onclick="editTask('${t.id}')" title="编辑：${esc(t.title)}" style="cursor:pointer;color:var(--text-secondary);font-size:13px;white-space:nowrap">✏️</span>
            <div class="task-title-cell">
              <div class="title">${esc(t.title)}</div>
              ${t.description ? `<div class="desc" title="${esc(t.description)}">${esc(t.description)}</div>` : ''}
            </div>
          </div>
        </td>
        <td class="col-status">${statusTag(t.status)}</td>
        <td class="col-type">${taskTypeTag(t.task_type)}</td>
        <td class="col-loop">${loopStatusTag(t)}</td>
        <td class="col-time" style="color:var(--text-secondary);font-size:12px">${fmt(t.created_at)}</td>
        <td class="col-ops">
          <div style="display:flex;align-items:center;gap:4px;flex-wrap:wrap">
            <button class="btn btn-secondary btn-small" onclick="viewTask('${t.id}')">查看</button>
            ${ops}
          </div>
        </td>
      </tr>`;
    }).join('')}</tbody>
  </table>`;
}

// 按状态返回操作按钮 HTML（返回按钮列表，td 本身已是 flex 容器）
function taskOpsByStatus(t) {
  const id = t.id;
  switch (t.status) {
    case 'pending':
      return `<button class="btn btn-small btn-warning" style="flex-shrink:0" onclick="claimTask('${id}')" title="认领后：状态→待执行，maintainer 标记为你，可以▶运行">🟡 认领</button>` +
             `<button class="btn btn-small" style="flex-shrink:0;background:#f59e0b;color:#fff" onclick="archiveTask('${id}')" title="直接归档（不需执行）">归档</button>` +
             `<button class="btn btn-small btn-danger" style="flex-shrink:0" onclick="deleteTask('${id}','${esc(t.title)}')" title="硬删任务">🗑 删除</button>`;
    case 'in_progress':
      return `<button class="btn btn-small btn-primary" style="flex-shrink:0" onclick="runTask('${id}')" title="立即用 AI CLI 跑这个任务（流式输出在 /api/tasks/{id}/run）">▶ 运行</button>` +
             `<button class="btn btn-small" style="flex-shrink:0;background:#94a3b8;color:#fff" onclick="unclaimTask('${id}')" title="退回 pending（清空 maintainer/started_at）">↩ 取消认领</button>` +
             `<button class="btn btn-small" style="flex-shrink:0;background:#f59e0b;color:#fff" onclick="archiveTask('${id}')" title="归档">归档</button>` +
             `<button class="btn btn-small btn-danger" style="flex-shrink:0" onclick="deleteTask('${id}','${esc(t.title)}')" title="硬删任务">🗑 删除</button>`;
    case 'running':
      return `<button class="btn btn-small" style="flex-shrink:0;background:#f59e0b;color:#fff" onclick="cancelTask('${id}')" title="强制取消卡住的任务执行（将任务状态置为异常）">⚠ 取消</button>` +
             `<button class="btn btn-small btn-danger" style="flex-shrink:0" onclick="deleteTask('${id}','${esc(t.title)}')" title="硬删任务">🗑 删除</button>`;
    case 'archived':
      return `<button class="btn btn-small" style="flex-shrink:0" onclick="reopenTask('${id}')" title="归档→重新打开回到 pending">↻ 重新打开</button>` +
             `<button class="btn btn-small btn-danger" style="flex-shrink:0" onclick="deleteTask('${id}','${esc(t.title)}')" title="硬删任务">🗑 删除</button>`;
    case 'exception':
      return `<button class="btn btn-small btn-warning" style="flex-shrink:0" onclick="reopenTask('${id}')" title="异常→重新打开回到 pending">↻ 重新打开</button>` +
             `<button class="btn btn-small" style="flex-shrink:0;background:#f59e0b;color:#fff" onclick="archiveTask('${id}')" title="归档">归档</button>` +
             `<button class="btn btn-small btn-danger" style="flex-shrink:0" onclick="deleteTask('${id}','${esc(t.title)}')" title="硬删任务">🗑 删除</button>`;
    default:
      return `<button class="btn btn-small btn-secondary" style="flex-shrink:0" onclick="viewTask('${id}')">详情</button>`;
  }
}

async function claimTask(id) {
  await fetch(API + '/api/tasks/' + id + '/status', {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({status: 'in_progress', maintainer: 'factory-agent'})
  });
  reloadCurrentTab();
}

async function unclaimTask(id) {
  if (!confirm('确认取消认领？状态会回到 pending，清空 maintainer/started_at/heartbeat。')) return;
  const r = await fetch(API + '/api/tasks/' + id + '/unclaim', {method:'POST'});
  if (!r.ok) { const b = await r.json().catch(() => ({})); alert('取消认领失败：' + (b.error || r.statusText)); return; }
  reloadCurrentTab();
}

async function cancelTask(id) {
  if (!confirm('确认取消该任务的执行？\n\n适用于：WS 断连或服务重启后，执行已结束但页面仍显示"运行中"。点击后任务将标记为异常。')) return;
  const r = await fetch(API + '/api/tasks/' + id + '/cancel', {method:'POST'});
  if (!r.ok) { const b = await r.json().catch(() => ({})); alert('取消失败：' + (b.error || r.statusText)); return; }
  reloadCurrentTab();
}

async function reopenTask(id) {
  if (!confirm('重新打开任务？状态会回到 pending。')) return;
  // 调 update status（pending）
  const r = await fetch(API + '/api/tasks/' + id + '/status', {
    method: 'PUT', headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({status: 'pending', maintainer: ''})
  });
  if (!r.ok) { const b = await r.json().catch(() => ({})); alert('重新打开失败：' + (b.error || r.statusText)); return; }
  reloadCurrentTab();
}

async function runTask(id) {
  const t = tasks.find(t => t.id === id) || (await loadTasks()).find(t => t.id === id);
  if (!t) { alert('找不到任务：' + id); return; }
  showRunTaskModal(t);
}

function showRunTaskModal(task) {
  document.getElementById('run-task-title').textContent = task.title + (task.description ? ' — ' + task.description.slice(0, 60) : '');
  // command_type/model：优先用 task 创建时确定的默认值，可临时调整
  const typeSel = document.getElementById('run-task-type');
  const taskCmdType = task.command_type || 'claude';
  const opt = Array.from(typeSel.options).find(o => o.value === taskCmdType);
  typeSel.value = opt ? taskCmdType : 'claude';
  toggleRunTaskModelGroup();
  const modelSel = document.getElementById('run-task-model');
  modelSel.innerHTML = buildModelOptions(typeSel.value);
  modelSel.value = task.model || getDefaultModel(typeSel.value) || '';
  // 默认填上次同任务的 session_id（实现“续传”便捷入口）：异步拉最近一次 execution
  const resumeInput = document.getElementById('run-task-resume');
  if (resumeInput) {
    resumeInput.value = '';
    resumeInput.placeholder = '加载中...';
    fetchJSON(API + '/api/tasks/' + task.id + '/executions').then(execs => {
      const last = (execs || []).find(e => e.resume_uuid);
      if (last && last.resume_uuid) {
        resumeInput.value = last.resume_uuid;
        resumeInput.placeholder = '上次 session: ' + last.resume_uuid.slice(0,8) + '...（留空开新会话）';
      } else {
        resumeInput.placeholder = '留空则开新会话';
      }
    }).catch(() => { resumeInput.placeholder = '留空则开新会话'; });
  }
  // 填充 agent 列表（异步：拉 + 绑定了远程 dir_shortcut 的 agent）
  loadRunTaskAgentOptions();
  document.getElementById('run-task-modal').classList.remove('hidden');
  // 存当前 taskId 供 submit 用
  document.getElementById('run-task-modal').dataset.taskId = task.id;
  // type 切换时联动 model 是否可选
  typeSel.onchange = toggleRunTaskModelGroup;
  // 注意：任务运行 modal 的 model onchange 不再调 saveDefaultModel，
  // 避免任务页选其他模型时污染系统配置的 default。
  // 任务运行仍然用下拉当前值；重新打开 modal 时下拉重置为系统配置的 default。
}

// 填充“运行位置”下拉：列出所有绑定了 dir_shortcut 的 agent。
async function loadRunTaskAgentOptions() {
  const sel = document.getElementById('run-task-agent');
  if (!sel) return;
  sel.innerHTML = '<option value="">本机执行（默认）</option>';
  try {
    const agents = await fetchJSON(API + '/api/agents');
    // 顺带查 dir_shortcuts 以获取 host name
    const dirs = await fetchJSON(API + '/api/dir-shortcuts');
    const dirMap = {};
    (dirs || []).forEach(d => { dirMap[d.id] = d; });
    (agents || []).forEach(a => {
      if (!a.bound_dir_shortcut_id) return; // 跳过未绑定的
      const ds = dirMap[a.bound_dir_shortcut_id];
      const label = a.name + (ds ? ' → ' + ds.remote_user + '@' + ds.remote_host : ' → [绑定的远端]');
      const opt = document.createElement('option');
      opt.value = a.id;
      opt.textContent = label;
      sel.appendChild(opt);
    });
  } catch (e) {
    console.warn('loadRunTaskAgentOptions failed', e);
  }
}

function closeRunTaskModal() {
  document.getElementById('run-task-modal').classList.add('hidden');
}

// type=shell 时 model 无效，灰显 model 下拉；claude/cbc 时切换模型列表
function toggleRunTaskModelGroup() {
  const type = document.getElementById('run-task-type').value;
  const grp = document.getElementById('run-task-model-group');
  const modelSel = document.getElementById('run-task-model');
  if (type === 'shell') {
    grp.style.opacity = '0.4';
    grp.style.pointerEvents = 'none';
  } else {
    grp.style.opacity = '1';
    grp.style.pointerEvents = 'auto';
    modelSel.innerHTML = buildModelOptions(type);
    // 切换后默认选中配置的默认模型
    const defaultModel = getDefaultModel(type);
    if (defaultModel && modelSel.querySelector('option[value="' + defaultModel + '"]')) {
      modelSel.value = defaultModel;
    }
  }
}

async function submitRunTask() {
  const taskId = document.getElementById('run-task-modal').dataset.taskId;
  const type = document.getElementById('run-task-type').value;
  const model = document.getElementById('run-task-model').value;
  const agentId = document.getElementById('run-task-agent') ? document.getElementById('run-task-agent').value : '';
  const resumeSessionId = document.getElementById('run-task-resume') ? document.getElementById('run-task-resume').value.trim() : '';
  closeRunTaskModal();
  try {
    const body = JSON.stringify({command_type: type, model: model, agent_id: agentId, resume_session_id: resumeSessionId});
    const r = await fetchJSON(API + '/api/tasks/' + taskId + '/run', {method:'POST', headers:{'Content-Type':'application/json'}, body});
    const summary = type + (type !== 'shell' ? ' / ' + model : '') + (agentId ? ' / 远端 agent' : ' / 本机') + (resumeSessionId ? ' / 续传' : '');
    // 询问是否跳转到自动化 Tab 看流式输出
    if (confirm(`✅ 已启动 execution_id=${r.execution_id}\n（${summary}）\n\n跳转到"⚡ 自动化 Tab"查看流式输出吗？`)) {
      switchTab('automation');
      // 自动化 Tab 的自动刷新会拉最新 executions,无需手动 reload
    } else {
      reloadCurrentTab();
    }
  } catch (e) { alert('启动失败：' + e.message); }
}

async function archiveTask(id) {
  await fetch(API + '/api/tasks/' + id + '/status', {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({status: 'archived'})
  });
  reloadCurrentTab();
}

// deleteTask 硬删 task + 关联 executions / evaluations（不可恢复）
async function deleteTask(id, title) {
  if (!confirm(`⚠️ 确认硬删任务？\n\n任务: ${title}\n\n将同时删除所有 execution 和 evaluation 记录。\n此操作不可恢复！`)) return;
  try {
    const r = await fetchJSON(API + '/api/tasks/' + id, {method: 'DELETE'});
    if (r.status === 'deleted') {
      reloadCurrentTab();
    }
  } catch (e) {
    alert('删除失败：' + e.message);
  }
}

function viewTask(id) {
  const t = tasks.find(t => t.id === id);
  if (!t) { loadTasks().then(() => viewTask(id)); return; }
  document.getElementById('task-modal-title').textContent = '任务详情';
  document.getElementById('task-id').value = t.id;
  document.getElementById('task-title').value = t.title;
  document.getElementById('task-title').readOnly = true;
  document.getElementById('task-desc').value = t.description || '';
  document.getElementById('task-desc').readOnly = true;
  document.getElementById('task-acceptance').value = t.acceptance || '';
  document.getElementById('task-acceptance').readOnly = true;
  document.getElementById('task-submit-btn').classList.add('hidden');
  // 待交互内容展示
  const waitingSection = document.getElementById('task-waiting-input-section');
  const waitingContent = document.getElementById('task-waiting-input-content');
  if (t.status === 'waiting_input' && t.waiting_input) {
    waitingSection.classList.remove('hidden');
    waitingContent.textContent = t.waiting_input;
  } else {
    waitingSection.classList.add('hidden');
  }
  // 经验库：展示 chip 列表（不可编辑）
  _selectedExps = [];
  if (t.experience_id) {
    const ids = t.experience_id.split(',').map(s => s.trim()).filter(Boolean);
    _loadAllExps().then(all => {
      _selectedExps = ids.map(eid => {
        const e = all.find(x => x.id === eid) || {id: eid, module: '?', scene: ''};
        return {id: eid, module: e.module, scene: e.scene, keywords: e.keywords};
      });
      renderTaskExpChips();
    });
  } else {
    renderTaskExpChips();
  }
  document.getElementById('task-modal').classList.remove('hidden');
  // 对话历史 / 活动历史 / 评论区块（HTML 未完成，guard 防止崩溃）
  try { loadTaskComments(t.id); } catch(e) { console.warn('loadTaskComments:', e.message); }
  _taskConvLoaded = false;
  try { loadTaskConversation(); } catch(e) { console.warn('loadTaskConversation:', e.message); }
  try { loadTaskEvents(); } catch(e) { console.warn('loadTaskEvents:', e.message); }
  // AI 自治：加载开关状态、决定是否显示 AI 自治区块；也用于判断启动按钮是否该禁用
  if (typeof loadAILoopStatus === 'function') loadAILoopStatus(t.id);
  // 重置 Run Loop 表单状态
  const lp = document.getElementById('loop-prompt');
  if (lp) lp.value = t.description || t.title || '';
  const lt = document.getElementById('loop-threshold');
  if (lt) lt.value = '7';
  const lm = document.getElementById('loop-maxiter');
  if (lm) lm.value = '3';
  const runBtn = document.getElementById('btn-run-loop');
  if (runBtn) {
    runBtn.disabled = false;
    runBtn.textContent = '▶ 启动';
  }
  const progWrap = document.getElementById('ai-loop-progress');
  if (progWrap) progWrap.classList.add('hidden');
  const histEl = document.getElementById('ai-loop-progress-history');
  if (histEl) histEl.dataset.history = '[]';
  // 尝试恢复上次 loop 结果（避免关闭弹窗后进度丢失）
  restoreLoopProgress(t.id);
  // 加载执行历史
  loadTaskExecHistory(t.id);
}

async function editTask(id) {
  let t = tasks.find(x => x.id === id);
  if (!t) {
    await loadTasks();
    t = tasks.find(x => x.id === id);
  }
  if (t) showTaskModal(t);
}

async function showTaskModal(task) {
  document.getElementById('task-modal-title').textContent = task ? '编辑任务' : '新建任务';
  document.getElementById('task-id').value = task ? task.id : '';
  document.getElementById('task-title').value = task ? task.title : '';
  document.getElementById('task-title').readOnly = false;
  document.getElementById('task-desc').value = task ? (task.description || '') : '';
  document.getElementById('task-desc').readOnly = false;
  document.getElementById('task-acceptance').value = task ? (task.acceptance || '') : '';
  document.getElementById('task-acceptance').readOnly = false;
  document.getElementById('task-type').value = task ? (task.task_type || 'manual') : 'manual';

  // command_type / model：新建默认 claude+haiku；编辑时回填已有值
  const cmdType = task ? (task.command_type || 'claude') : 'claude';
  const mdl = task ? (task.model || '') : '';
  document.getElementById('task-command-type').value = cmdType;
  const modelSel = document.getElementById('task-model');
  if (typeof loadCLIModels === 'function') await loadCLIModels();
  modelSel.innerHTML = buildModelOptions(cmdType);
  modelSel.value = mdl || getDefaultModel(cmdType) || '';
  // command_type 切换时联动刷新 model 列表
  document.getElementById('task-command-type').onchange = function() {
    const type = this.value;
    modelSel.innerHTML = buildModelOptions(type);
    modelSel.value = getDefaultModel(type) || '';
  };

  document.getElementById('task-submit-btn').classList.remove('hidden');
  // 经验库：编辑模式从 task.experience_id 解析
  _selectedExps = [];
  if (task && task.experience_id) {
    const ids = task.experience_id.split(',').map(s => s.trim()).filter(Boolean);
    _loadAllExps().then(all => {
      _selectedExps = ids.map(id => {
        const e = all.find(x => x.id === id) || {id, module: '?', scene: ''};
        return {id, module: e.module, scene: e.scene, keywords: e.keywords};
      }).filter(Boolean);
      renderTaskExpChips();
    });
  } else {
    renderTaskExpChips();
  }
  document.getElementById('task-modal').classList.remove('hidden');
  if (task) loadTaskComments(task.id);
}

function closeTaskModal() {
  document.getElementById('task-modal').classList.add('hidden');
}

async function submitTask() {
  const id = document.getElementById('task-id').value;
  const title = document.getElementById('task-title').value.trim();
  if (!title) { alert('请输入标题'); return; }
  const body = {
    title,
    description: document.getElementById('task-desc').value,
    experience_ids: _selectedExps.map(s => s.id),
    acceptance: document.getElementById('task-acceptance').value,
    task_type: document.getElementById('task-type').value,
    command_type: document.getElementById('task-command-type').value,
    model: document.getElementById('task-model').value,
  };
  if (id) {
    await fetch(API + '/api/tasks/' + id, {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body)
    });
  } else {
    await fetch(API + '/api/tasks', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body)
    });
  }
  closeTaskModal();
  loadDashboard();
  if (currentTab === 'tasks') loadTasks();
}

async function loadTaskComments(taskId) {
  const container = document.getElementById('task-comment-list');
  const countEl = document.getElementById('task-comment-count');
  if (!container) { console.warn('comment container not found'); return; }
  let list;
  try {
    list = await fetchJSON('/api/tasks/' + taskId + '/comments');
  } catch(e) {
    console.error('loadTaskComments failed:', e);
    container.innerHTML = '<span style="color:var(--exception);font-size:12px">加载评论失败</span>';
    return;
  }
  countEl.textContent = list.length > 0 ? '(' + list.length + ')' : '';
  if (!list.length) {
    container.innerHTML = '<span style="color:var(--text-secondary);font-size:12px">暂无评论</span>';
    return;
  }
  container.innerHTML = list.map(c => {
    const dt = c.created_at ? new Date(c.created_at).toLocaleString() : '';
    return '<div style="padding:6px 0;border-bottom:1px solid var(--border);font-size:12px">' +
      '<span style="color:var(--text-secondary)">' + (c.author || 'user') + ' · ' + dt + '</span>' +
      '<div style="margin-top:2px">' + esc(c.content) + '</div></div>';
  }).join('');
}

async function submitTaskComment() {
  const taskId = document.getElementById('task-id').value;
  if (!taskId) return;
  const input = document.getElementById('task-comment-input');
  const content = input.value.trim();
  if (!content) return;
  input.value = '';
  await fetchJSON('/api/tasks/' + taskId + '/comments', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({author: 'user', content})
  });
  await loadTaskComments(taskId);
}

// ===== 对话历史（task-modal 内嵌区块） =====
// 按 resume_uuid 分组展示该 task 下所有 execution 的对话链。
// 仅在 task-modal 打开时占用，关闭时不占资源（无全局轮询）。
let _taskConvLoaded = false; // 防止 viewTask 多次触发

function toggleTaskConversation() {
  const body = document.getElementById('task-conversation-body');
  const arrow = document.getElementById('task-conversation-toggle');
  if (body.classList.contains('hidden')) {
    body.classList.remove('hidden');
    arrow.style.transform = 'rotate(90deg)';
  } else {
    body.classList.add('hidden');
    arrow.style.transform = 'rotate(0deg)';
  }
}

async function loadTaskConversation() {
  const taskId = document.getElementById('task-id').value;
  if (!taskId) return;
  const body = document.getElementById('task-conversation-body');
  if (!body) return;
  try {
    const execs = await fetchJSON('/api/tasks/' + taskId + '/executions');
    _taskConvLoaded = true;
    renderTaskConversation(execs || []);
    // 自动展开：如果该 task 有 exec 历史，默认展开对话历史（避免用户不知要点 ▶）
    // 空对话则保持折叠，不占视觉空间。
    if ((execs || []).length > 0) {
      const b = document.getElementById('task-conversation-body');
      const arrow = document.getElementById('task-conversation-toggle');
      if (b && b.classList.contains('hidden')) {
        b.classList.remove('hidden');
        if (arrow) arrow.style.transform = 'rotate(90deg)';
      }
    }
  } catch (e) {
    body.innerHTML = '<div style="padding:8px;color:var(--exception);font-size:12px">⚠ 加载失败：' + e.message + '</div>';
  }
}

// renderTaskConversation: 按 resume_uuid 分组，每组内按 started_at 升序。
// 根节点（resume_uuid == id）标记为「原始执行」；continue 节点标「继续 N」。
// 视觉走时间线：左侧时间轴（竖线 + 圆点）+ 右侧卡片。
// 命令 / 命令中含的 prompt 摘要默认折叠，需要时点 ▶ 展开。
function renderTaskConversation(execs) {
  const countEl = document.getElementById('task-conversation-count');
  const body = document.getElementById('task-conversation-body');
  if (!execs.length) {
    countEl.textContent = '(0)';
    body.innerHTML = '<div style="padding:8px;color:var(--text-secondary);font-size:12px">该任务暂无执行记录</div>';
    return;
  }
  // 分组
  const groups = new Map(); // groupKey -> [execs]
  for (const e of execs) {
    const key = e.resume_uuid || e.id; // 没有 resume_uuid 的单次执行也单独成组（key = 自身 id）
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(e);
  }
  // 组内按 started_at 排序
  for (const arr of groups.values()) {
    arr.sort((a, b) => new Date(a.started_at) - new Date(b.started_at));
  }
  // 按最近活跃（最后一条 started_at）倒序
  const sortedGroups = [...groups.values()].sort(
    (a, b) => new Date(b[b.length-1].started_at) - new Date(a[a.length-1].started_at)
  );
  let totalRounds = 0;
  for (const arr of sortedGroups) totalRounds += arr.length;
  countEl.textContent = '(' + totalRounds + ' 轮 / ' + sortedGroups.length + ' 会话)';
  // 渲染时间线。每个会话一条时间轴；会话内多个轮次，竖线贯穿。
  // 结构：
  //   <div class="conv-tl">  // 外层容器
  //     <div class="conv-session">  // 每个会话一个
  //       <div class="conv-session-header">💬 会话 N</div>
  //       <div class="conv-tl-rail">  // 时间轴竖线
  //         <div class="conv-node">  // 每个 exec 一个节点
  //           <div class="conv-node-dot"></div>  // 圆点
  //           <div class="conv-node-card">  // 卡片
  //             <div class="conv-node-header">原始/继续 K · 模型 · 时间</div>
  //             <div class="conv-node-prompt">问: ...</div>
  //             <div class="conv-node-output">答: ...</div>
  //             <div class="conv-node-cmd">命令 ... (折叠)</div>
  //             <div class="conv-node-actions">[详情]</div>
  //           </div>
  //         </div>
  //       </div>
  //     </div>
  //   </div>
  const html = sortedGroups.map((arr, gi) => {
    const isChain = arr.length > 1;
    const header = isChain
      ? `<div style="background:var(--accent-soft);padding:4px 10px;font-size:11px;color:var(--accent);font-weight:600;border-radius:3px;margin:6px 0 4px 22px">💬 会话 ${gi+1} · ${arr.length} 轮</div>`
      : '';
    const items = arr.map((e, idx) => {
      const isRoot = idx === 0;
      const isLast = idx === arr.length - 1;
      const tag = isRoot
        ? '<span style="background:var(--accent);color:#fff;padding:1px 6px;border-radius:3px;font-size:10px">原始</span>'
        : '<span style="background:#0ea5e9;color:#fff;padding:1px 6px;border-radius:3px;font-size:10px">继续 ' + idx + '</span>';
      const status = _convStatusBadge(e);
      const ts = e.started_at ? new Date(e.started_at).toLocaleString('zh-CN', {hour12: false}) : '';
      const prompt = e.prompt ? _convEscape(e.prompt) : '<i style="color:var(--text-secondary)">(无 prompt)</i>';
      const output = e.output ? _convEscape(e.output) : '';
      const modelBadge = e.model ? `<span style="background:#6366f1;color:#fff;padding:1px 5px;border-radius:3px;font-size:9px;margin-left:4px">${_convEscape(e.model)}</span>` : '';
      const cmdShort = e.command ? _convTruncate(e.command.replace(/\n/g, ' '), 60) : '';
      // 每个节点的 dot 颜色：成功=绿、失败=红、进行中=蓝
      const dotColor = (e.exit_code === 0) ? '#10b981' : (e.exit_code != null && e.exit_code !== 0) ? '#ef4444' : (e.error ? '#ef4444' : '#0ea5e9');
      // 最后一个节点不画竖线底部线帽
      const railStyle = isLast ? 'border-bottom:1px solid transparent' : '';
      // 时间线节点
      const nodeId = 'tl-node-' + _sanitizeId(e.id);
      const cmdPanelId = 'tl-cmd-' + _sanitizeId(e.id);
      return `<div style="display:flex;gap:0;align-items:stretch;position:relative;padding:4px 0">
        <div style="flex-shrink:0;width:18px;display:flex;flex-direction:column;align-items:center">
          <div style="width:10px;height:10px;border-radius:50%;background:${dotColor};border:2px solid var(--card-bg);margin-top:8px;box-shadow:0 0 0 1px ${dotColor};flex-shrink:0"></div>
          <div style="flex:1;width:2px;background:var(--border);${railStyle}"></div>
        </div>
        <div style="flex:1;min-width:0;margin-left:8px">
          <div style="display:flex;gap:6px;align-items:center;flex-wrap:wrap;margin-bottom:3px">
            ${tag}${modelBadge}
            <span style="color:var(--text-secondary);font-size:10px">${ts}</span>
            ${status}
          </div>
          <div style="background:var(--hover);border:1px solid var(--border);border-radius:5px;padding:6px 8px;font-size:12px;margin-bottom:3px">
            <div style="color:var(--text);word-break:break-word;margin-bottom:${output ? '4px' : '0'}"><b style="color:#0ea5e9">问:</b> ${prompt}</div>
            ${output ? `<div style="color:var(--text-secondary);word-break:break-word"><b style="color:#10b981">答:</b> ${output}</div>` : ''}
          </div>
          ${cmdShort ? `<div style="margin-bottom:4px">
            <span style="font-size:10px;color:var(--text-secondary);cursor:pointer;user-select:none" onclick="(function(){
              var p=document.getElementById('${cmdPanelId}');
              if(!p) return;
              if(p.classList.contains('hidden')){p.classList.remove('hidden');}
              else{p.classList.add('hidden');}
            })()">▶ 命令</span>
            <div id="${cmdPanelId}" class="hidden" style="margin-top:3px;padding:4px 6px;background:var(--card-bg);border:1px solid var(--border);border-radius:3px;font-family:monospace;font-size:11px;color:var(--text-secondary);word-break:break-all;max-height:80px;overflow-y:auto">${_convEscape(e.command || '')}</div>
          </div>` : ''}
          <div style="text-align:right;margin-bottom:2px">
            <button class="btn btn-small" style="flex-shrink:0" onclick="viewExecutionDetail('${e.id}')">详情</button>
          </div>
        </div>
      </div>`;
    }).join('');
    return header + items;
  }).join('');
  body.innerHTML = html;
}

function _sanitizeId(s) {
  return String(s || '').replace(/[^a-zA-Z0-9_-]/g, '_');
}

function _convStatusBadge(e) {
  if (e.exit_code === 0) return '<span style="color:#10b981;font-size:10px">✓ 完成</span>';
  if (e.exit_code != null && e.exit_code !== 0) return '<span style="color:var(--exception);font-size:10px">✗ 失败(' + e.exit_code + ')</span>';
  if (e.error) return '<span style="color:var(--exception);font-size:10px">✗ 错误</span>';
  return '<span style="color:#0ea5e9;font-size:10px">⏳ 执行中</span>';
}

function _convTruncate(s, n) {
  if (!s) return '';
  return s.length > n ? s.slice(0, n) + '…' : s;
}

function _convEscape(s) {
  return String(s).replace(/[&<>"']/g, c => ({
    '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;'
  }[c]));
}

// ===== 活动历史（task-modal 里的 task_events 区块） =====
// 后端 GET /api/tasks/{id}/events 返回该 task 产生的所有事件（created/claimed/reported/heartbeat_lost/recovered 等）。
// 只读展示，不提供写操作。
function toggleTaskEvents() {
  const body = document.getElementById('task-events-body');
  const arrow = document.getElementById('task-events-toggle');
  if (body.classList.contains('hidden')) {
    body.classList.remove('hidden');
    arrow.style.transform = 'rotate(90deg)';
  } else {
    body.classList.add('hidden');
    arrow.style.transform = 'rotate(0deg)';
  }
}

async function loadTaskEvents() {
  const taskId = document.getElementById('task-id').value;
  if (!taskId) return;
  const body = document.getElementById('task-events-body');
  if (!body) return;
  try {
    const events = await fetchJSON('/api/tasks/' + taskId + '/events?limit=50');
    renderTaskEvents(events || []);
  } catch (e) {
    body.innerHTML = '<div style="padding:8px;color:var(--exception);font-size:12px">⚠ 加载失败：' + e.message + '</div>';
  }
}

// renderTaskEvents 按时间倒序展示（后端已是这个顺序）。event_type 分中英友好名。
// actor 字段可能是 'user' / 'agent:<id>' / 'system'。
function renderTaskEvents(events) {
  const countEl = document.getElementById('task-events-count');
  const body = document.getElementById('task-events-body');
  if (!events.length) {
    countEl.textContent = '(0)';
    body.innerHTML = '<div style="padding:8px;color:var(--text-secondary);font-size:12px">该任务暂无活动事件</div>';
    return;
  }
  countEl.textContent = '(' + events.length + ')';
  const html = events.map(ev => {
    const ts = ev.created_at ? new Date(ev.created_at).toLocaleString('zh-CN', {hour12: false}) : '';
    const typeLabel = _eventTypeLabel(ev.event_type);
    const actorLabel = _eventActorLabel(ev.actor);
    const payload = ev.payload ? _eventPayloadSummary(ev.event_type, ev.payload) : '';
    return `<div style="padding:6px 8px;border-bottom:1px solid var(--border);font-size:12px;display:flex;gap:8px;align-items:flex-start">
      <div style="flex-shrink:0;font-size:11px;color:var(--accent);min-width:90px">${_eventTypeBadge(ev.event_type, typeLabel)}</div>
      <div style="flex:1;min-width:0">
        <div style="display:flex;gap:6px;align-items:center;margin-bottom:2px">
          <span style="color:var(--text-secondary);font-size:10px">${ts}</span>
          <span style="color:var(--text-secondary);font-size:10px">· ${_eventEsc(actorLabel)}</span>
        </div>
        ${payload ? `<div style="color:var(--text);word-break:break-word;font-size:11px">${_eventEsc(payload)}</div>` : ''}
      </div>
    </div>`;
  }).join('');
  body.innerHTML = html;
}

function _eventTypeBadge(type, label) {
  const palette = {
    created: '#10b981',
    claimed: '#0ea5e9',
    reported: '#8b5cf6',
    heartbeat_lost: '#ef4444',
    heartbeat_recovered: '#10b981',
  };
  const color = palette[type] || '#6b7280';
  return `<span style="background:${color};color:#fff;padding:1px 6px;border-radius:3px;font-size:10px">${label}</span>`;
}

function _eventTypeLabel(type) {
  return ({
    created: '创建',
    claimed: '认领',
    reported: '上报',
    heartbeat_lost: '心跳丢失',
    heartbeat_recovered: '心跳恢复',
  })[type] || type;
}

function _eventActorLabel(actor) {
  if (!actor) return '系统';
  if (actor === 'user') return '用户';
  if (actor.startsWith('agent:')) return 'Agent ' + actor.slice(6);
  return actor;
}

// payload 是 JSON 字符串（后端 fmt.Sprintf 拼出来的）。提取几个常见字段拼接成可读句子。
function _eventPayloadSummary(type, payload) {
  try {
    const obj = JSON.parse(payload);
    if (type === 'created' && obj.task_type) return '类型：' + obj.task_type;
    if (type === 'claimed' && obj.agent_id) return 'agent_id=' + obj.agent_id;
    if (type === 'reported') {
      const parts = [];
      if (obj.score != null) parts.push('score=' + obj.score);
      if (obj.is_error) parts.push('is_error=true');
      if (obj.exit_code != null) parts.push('exit_code=' + obj.exit_code);
      return parts.length ? parts.join(' · ') : payload;
    }
    return payload;
  } catch {
    return payload;
  }
}

function _eventEsc(s) {
  return String(s).replace(/[&<>"']/g, c => ({
    '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;'
  }[c]));
}

// ===== AI 自治 三个动作 =====
// 错误处理：后端返 403 “未启用”时提示用户去 dashboard 开。

function _currentTaskId() {
  return document.getElementById('task-id')?.value || '';
}

function _aiLoopProgressShow(text, threshold) {
  const wrap = document.getElementById('ai-loop-progress');
  const textEl = document.getElementById('ai-loop-status-text');
  if (wrap) wrap.classList.remove('hidden');
  if (textEl) textEl.textContent = text || '进行中...';
}

// _loopCurrentIter 跟踪当前进行到的迭代（WS 事件驱动更新）
var _loopCurrentIter = 0;

function _aiLoopProgressRenderHistory(history, threshold) {
  const histEl = document.getElementById('ai-loop-progress-history');
  const pctEl = document.getElementById('ai-loop-progress-pct');
  const barEl = document.getElementById('ai-loop-bar');
  if (!histEl) return;
  const th = threshold || _loopThreshold || 7;
  const max = _loopMaxIter || 3;
  const done = history ? history.length : 0;

  // 更新进度条
  if (barEl) barEl.style.width = max > 0 ? Math.round((done / max) * 100) + '%' : '0%';
  if (pctEl) pctEl.textContent = max > 0 ? done + '/' + max + '轮' : '';

  if (!history || history.length === 0) {
    histEl.innerHTML = '<div style="color:var(--text-secondary);font-size:11px;text-align:center;padding:8px">等待迭代开始...</div>';
    return;
  }
  histEl.innerHTML = history.map((step) => {
    const passed = step.score != null && step.score >= th;
    const scoreStr = step.score != null ? step.score.toFixed(1) : '?';
    const scoreColor = passed ? '#22c55e' : (step.score != null ? '#ef4444' : '#9ca3af');
    const statusIcon = step.error ? '⚠' : (passed ? '✓' : '✗');
    const statusColor = step.error ? '#f97316' : (passed ? '#22c55e' : '#ef4444');
    const errDiv = step.error ? `<div style="color:#ef4444;font-size:10px;margin-left:16px">${esc(step.error)}</div>` : '';
    const commentDiv = step.comments ? `<div style="color:var(--text-secondary);font-size:10px;margin-left:16px;margin-top:2px;max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="${esc(step.comments)}">💬 ${esc(step.comments)}</div>` : '';
    return `<div style="padding:4px 6px;border-bottom:1px solid rgba(255,255,255,0.1);font-size:11px;display:flex;align-items:center;gap:6px">
      <span style="color:${statusColor};flex-shrink:0">${statusIcon}</span>
      <span style="color:var(--text-secondary);flex-shrink:0">${step.model || '?'}</span>
      <span style="color:${scoreColor};font-weight:600">${scoreStr}</span>
      <span style="color:var(--text-secondary)">/ 目标 ${th}</span>
      ${errDiv}
      ${commentDiv}
    </div>`;
  }).join('') || '<div style="color:var(--text-secondary)">（无迭代）</div>';
}

function _aiLoopErrorHandler(e) {
  if (e.message && e.message.includes('403')) {
    alert('AI 自治未启用。请在 Dashboard 页面 “🤖 AI 自治” widget 中开启，或修改 config.json。');
  } else {
    alert('失败：' + e.message);
  }
}

// runTaskLoop fire-and-forget：POST 后立即返回 {status:"started"}，后端在
// 后台 goroutine 跑循环并通过 wsmsg.ChannelExec 推进度。前端用
// handleRunLoopProgress (在 ws-client.js) 增量渲染。3 分钟级别的循环不再阻塞
// HTTP 连接，配合服务端 WriteTimeout=0 和 signal handler 优雅关闭。
async function runTaskLoop() {
  const id = _currentTaskId();
  if (!id) return;
  const loopPrompt = document.getElementById('loop-prompt')?.value?.trim();
  if (!loopPrompt) {
    alert('⚠️ 请填写 Run Loop 的描述内容');
    return;
  }
  const threshold = parseFloat(document.getElementById('loop-threshold')?.value) || 7;
  const maxIter = parseInt(document.getElementById('loop-maxiter')?.value) || 3;
  const runBtn = document.getElementById('btn-run-loop');
  if (runBtn) runBtn.disabled = true;
  _loopStart(id, threshold, maxIter);
  try {
    await fetchJSON('/api/tasks/' + id + '/run-loop', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt: loopPrompt, model: '', cli_type: 'claude', threshold, max_iterations: maxIter }),
    });
  } catch (e) {
    _aiLoopProgressShow('❌ 启动失败：' + e.message, threshold);
    _aiLoopDone();
    alert('❌ Run Loop 启动失败：' + e.message);
  }
}

// _loopStart 初始化进度区（由 WS 事件或直接调用）
var _loopThreshold = 7;
var _loopMaxIter = 3;

function _loopStart(id, threshold, maxIter) {
  _loopThreshold = threshold;
  _loopMaxIter = maxIter;
  _loopCurrentIter = 0;
  const runBtn = document.getElementById('btn-run-loop');
  if (runBtn) {
    runBtn.disabled = true;
    runBtn.textContent = '⏳ 运行中';
  }
  _aiLoopProgressShow('🤖 Run Loop 已启动...', threshold);
  _aiLoopProgressRenderHistory([], threshold);
}

function _aiLoopDone() {
  const runBtn = document.getElementById('btn-run-loop');
  if (runBtn) {
    runBtn.disabled = false;
    runBtn.textContent = '▶ 启动';
  }
  // 持久化最终结果到 localStorage（关闭弹窗再打开可恢复）
  const id = _currentTaskId();
  if (id) {
    const histEl = document.getElementById('ai-loop-progress-history');
    const hist = histEl?.dataset.history ? JSON.parse(histEl.dataset.history) : [];
    const th = _loopThreshold || 7;
    const statusText = document.getElementById('ai-loop-status-text')?.textContent || '';
    saveLoopProgress(id, { history: hist, threshold: th, statusText });
  }
}

function saveLoopProgress(taskId, data) {
  try {
    const all = JSON.parse(localStorage.getItem('loop_progress') || '{}');
    all[taskId] = { ...data, savedAt: Date.now() };
    localStorage.setItem('loop_progress', JSON.stringify(all));
  } catch (_) {}
}

function restoreLoopProgress(taskId) {
  try {
    const all = JSON.parse(localStorage.getItem('loop_progress') || '{}');
    const entry = all[taskId];
    if (!entry || !entry.history || entry.history.length === 0) return;
    // 超过 24 小时的记录忽略
    if (Date.now() - entry.savedAt > 86400000) return;
    const histEl = document.getElementById('ai-loop-progress-history');
    const progWrap = document.getElementById('ai-loop-progress');
    const statusEl = document.getElementById('ai-loop-status-text');
    const barEl = document.getElementById('ai-loop-bar');
    const pctEl = document.getElementById('ai-loop-progress-pct');
    if (histEl) {
      histEl.dataset.history = JSON.stringify(entry.history);
      if (typeof _aiLoopProgressRenderHistory === 'function') {
        _aiLoopProgressRenderHistory(entry.history, entry.threshold);
      }
    }
    if (statusEl) statusEl.textContent = entry.statusText || '上次优化结果';
    if (progWrap) progWrap.classList.remove('hidden');
    const max = _loopMaxIter || 3;
    if (barEl) barEl.style.width = max > 0 ? Math.round((entry.history.length / max) * 100) + '%' : '0%';
    if (pctEl) pctEl.textContent = entry.history.length + '/' + max + '轮';
  } catch (_) {}
}

async function reevaluateTask() {
  const id = _currentTaskId();
  if (!id) return;
  const btn = document.getElementById('btn-reevaluate');
  if (btn?.disabled) {
    alert('⚠️ Reevaluate 需要先运行一次任务，才能评估执行结果。\n\n请先点击「▶ 运行」按钮执行该任务。');
    return;
  }
  const model = document.getElementById('task-run-model')?.value || '';
  if (!confirm('用新模型重新评估该 task 的最新 execution？')) return;
  _aiLoopProgressShow('重评中...');
  try {
    const result = await fetchJSON('/api/tasks/' + id + '/reevaluate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model, cli_type: 'claude' }),
    });
    _aiLoopProgressShow('✓ 重评完成·score=' + (result.score != null ? result.score.toFixed(2) : '?'));
    _aiLoopProgressRenderHistory([{ iteration: 1, model, exit_code: 0, score: result.score }]);
  } catch (e) {
    _aiLoopProgressShow('❌ ' + e.message);
    alert('❌ Reevaluate 失败：' + e.message);
  }
}

async function learnFromTask() {
  const id = _currentTaskId();
  if (!id) return;
  const btn = document.getElementById('btn-learn');
  if (btn) btn.disabled = true;
  if (!confirm('从该 task 的执行记录生成经验写入 experiences 表？')) {
    if (btn) btn.disabled = false;
    return;
  }
  const progWrap = document.getElementById('ai-loop-progress');
  const textEl = document.getElementById('ai-loop-status-text');
  if (progWrap) progWrap.classList.remove('hidden');
  if (textEl) textEl.textContent = '📚 正在分析执行记录生成经验...';
  try {
    const result = await fetchJSON('/api/tasks/' + id + '/learn', { method: 'POST' });
    if (textEl) textEl.textContent = '✓ 经验已生成（exp_id: ' + (result.exp_id || '?') + '）';
  } catch (e) {
    if (textEl) textEl.textContent = '❌ ' + e.message;
    alert('❌ Learn 失败：' + e.message);
  } finally {
    if (btn) btn.disabled = false;
  }
}

// ─── 任务详情执行历史 分页状态 ───
var _taskExecList = [];       // 当前任务所有 executions
var _taskExecPage = 1;        // 当前页（1-based）
var _taskExecPageSize = 10;   // 每页条数
var _taskExecTaskId = '';     // 当前加载的任务 ID

// loadTaskExecHistory 由 viewTask 末尾调用
async function loadTaskExecHistory(taskId) {
  _taskExecTaskId = taskId;
  _taskExecPage = 1;
  try {
    _taskExecList = await fetchJSON(API + '/api/tasks/' + taskId + '/executions');
  } catch (e) {
    _taskExecList = [];
  }
  renderTaskExecHistory();
}

// 根据执行记录数量同步 AI 自治按钮的可用状态（有记录才可点）
function syncAILoopButtons() {
  const hasExec = _taskExecList.length > 0;
  const revalBtn = document.getElementById('btn-reevaluate');
  const learnBtn = document.getElementById('btn-learn');
  if (revalBtn) {
    revalBtn.disabled = !hasExec;
    revalBtn.title = hasExec ? '重新评估最新执行结果' : '需要先运行一次任务';
  }
  if (learnBtn) {
    learnBtn.disabled = !hasExec;
    learnBtn.title = hasExec ? '从执行记录生成经验写入知识库' : '需要先运行一次任务';
  }
}

function renderTaskExecHistory() {
  const container = document.getElementById('task-exec-history');
  const listEl = document.getElementById('task-exec-history-list');
  const infoEl = document.getElementById('task-exec-history-info');
  const pageEl = document.getElementById('task-exec-history-page');
  const prevBtn = document.getElementById('task-exec-prev');
  const nextBtn = document.getElementById('task-exec-next');
  if (!container) return;

  const total = _taskExecList.length;
  const totalPages = Math.max(1, Math.ceil(total / _taskExecPageSize));
  if (_taskExecPage > totalPages) _taskExecPage = totalPages;

  if (total === 0) {
    container.classList.remove('hidden');
    listEl.innerHTML = '<div style="color:var(--text-secondary);font-size:12px;text-align:center;padding:12px">暂无执行记录</div>';
    infoEl.textContent = '';
    pageEl.textContent = '';
    prevBtn.classList.add('hidden');
    nextBtn.classList.add('hidden');
    syncAILoopButtons();
    return;
  }

  container.classList.remove('hidden');
  infoEl.textContent = `共 ${total} 条，第 ${_taskExecPage}/${totalPages} 页`;

  const start = (_taskExecPage - 1) * _taskExecPageSize;
  const pageItems = _taskExecList.slice(start, start + _taskExecPageSize);

  const statusIcon = { success:'●', failed:'✗', running:'⏳', timeout:'⏱', cancelled:'—', build_error:'⚠' };
  const statusColor = { success:'#22c55e', failed:'#ef4444', running:'#eab308', timeout:'#9ca3af', cancelled:'#9ca3af', build_error:'#f97316' };

  listEl.innerHTML = pageItems.map(function(e) {
    const icon = statusIcon[e.status] || '?';
    const color = statusColor[e.status] || '#9ca3af';
    const dur = e.completed_at && e.started_at
      ? (Math.round((new Date(e.completed_at) - new Date(e.started_at)) / 100) / 10) + 's'
      : (e.started_at ? '运行中' : '—');
    const date = e.started_at ? new Date(e.started_at).toLocaleString('zh-CN', {month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}) : '—';
    const score = e.evaluation_score != null ? 'score=' + Number(e.evaluation_score).toFixed(1) : '未评';
    const cli = e.cli_type || 'claude';
    const model = e.model || '';
    return '<div class="task-exec-item" data-tip="点击查看执行详情" onclick="jumpToExecDetail(\'' + e.id + '\')" style="cursor:pointer;padding:6px 8px;border-radius:4px;margin-bottom:4px;background:var(--hover);font-size:12px;display:flex;gap:8px;align-items:center">' +
      '<span style="color:' + color + ';flex-shrink:0">' + icon + '</span>' +
      '<span style="color:var(--text-secondary);flex-shrink:0">' + cli + (model ? ' / ' + model : '') + '</span>' +
      '<span style="color:var(--text-secondary);flex-shrink:0">' + dur + '</span>' +
      '<span style="color:var(--text-secondary);flex-shrink:0">' + date + '</span>' +
      '<span style="color:var(--text-secondary);margin-left:8px">' + score + '</span>' +
    '</div>';
  }).join('');

  // 鼠标跟随 tooltip
  var tooltip = document.getElementById('task-exec-tooltip');
  if (!tooltip) {
    tooltip = document.createElement('div');
    tooltip.id = 'task-exec-tooltip';
    tooltip.style.cssText = 'position:fixed;z-index:9999;background:#333;color:#fff;font-size:11px;padding:4px 8px;border-radius:4px;pointer-events:none;opacity:0;transition:opacity 0.1s';
    document.body.appendChild(tooltip);
  }
  listEl.onmousemove = function(e) {
    var tip = e.target.closest('.task-exec-item');
    if (tip) {
      tooltip.textContent = tip.dataset.tip;
      tooltip.style.left = (e.clientX + 12) + 'px';
      tooltip.style.top = (e.clientY - 28) + 'px';
      tooltip.style.opacity = '1';
    }
  };
  listEl.onmouseleave = function() {
    tooltip.style.opacity = '0';
  };

  prevBtn.classList.toggle('hidden', _taskExecPage <= 1);
  nextBtn.classList.toggle('hidden', _taskExecPage >= totalPages);
  pageEl.textContent = '';
  // 同步 AI 自治按钮可用状态
  syncAILoopButtons();
}

function prevTaskExecs() {
  if (_taskExecPage > 1) { _taskExecPage--; renderTaskExecHistory(); }
}

function nextTaskExecs() {
  const totalPages = Math.ceil(_taskExecList.length / _taskExecPageSize);
  if (_taskExecPage < totalPages) { _taskExecPage++; renderTaskExecHistory(); }
}

function jumpToExecDetail(execId) {
  viewExecutionDetail(execId);
}

