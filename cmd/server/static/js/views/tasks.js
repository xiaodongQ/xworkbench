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
  console.log('[loadTasks] url=', url, 'task-list el:', !!document.getElementById('task-list'));
  try {
    tasks = await fetchJSON(url);
    console.log('[loadTasks] got', tasks.length, 'tasks');
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
      <th>类型</th>
      <th class="col-time" style="cursor:pointer" onclick="setTaskSort('created_at')">创建时间${sortIcon('created_at')}</th>
      <th class="col-ops" style="text-align:left">操作</th>
    </tr></thead>
    <tbody>${sorted.map(t => {
      const ops = taskOpsByStatus(t);
      return `
      <tr>
        <td class="col-title" style="display:flex;align-items:center;gap:4px">
          <span class="title" onclick="editTask('${t.id}')" title="编辑：${esc(t.title)}" style="cursor:pointer;color:var(--text-secondary);font-size:13px;white-space:nowrap">✏️</span>
          <span class="task-title-cell">
            <span class="title">${esc(t.title)}</span>
            ${t.description ? `<span class="desc" title="${esc(t.description)}">${esc(t.description)}</span>` : ''}
          </span>
        </td>
        <td class="col-status">${statusTag(t.status)}</td>
        <td>${taskTypeTag(t.task_type)}</td>
        <td class="col-time" style="color:var(--text-secondary);font-size:12px">${fmt(t.created_at)}</td>
        <td class="col-ops" style="display:flex;align-items:center;gap:4px">
          <button class="btn btn-secondary btn-small" onclick="viewTask('${t.id}')">查看</button>
          ${ops}
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
      return `<button class="btn btn-small btn-warning" style="flex-shrink:0" onclick="claimTask('${id}')" title="认领后：状态→in_progress，maintainer 标记为你，可以▶运行">🟡 认领</button>` +
             `<button class="btn btn-small" style="flex-shrink:0;background:#f59e0b;color:#fff" onclick="archiveTask('${id}')" title="直接归档（不需执行）">归档</button>` +
             `<button class="btn btn-small btn-danger" style="flex-shrink:0" onclick="deleteTask('${id}','${esc(t.title)}')" title="硬删任务">🗑 删除</button>`;
    case 'in_progress':
      return `<button class="btn btn-small btn-primary" style="flex-shrink:0" onclick="runTask('${id}')" title="立即用 AI CLI 跑这个任务（流式输出在 /api/tasks/{id}/run）">▶ 运行</button>` +
             `<button class="btn btn-small" style="flex-shrink:0;background:#94a3b8;color:#fff" onclick="unclaimTask('${id}')" title="退回 pending（清空 maintainer/started_at）">↩ 取消认领</button>` +
             `<button class="btn btn-small" style="flex-shrink:0;background:#f59e0b;color:#fff" onclick="archiveTask('${id}')" title="归档">归档</button>` +
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
  document.getElementById('run-task-type').value = 'claude';
  toggleRunTaskModelGroup();
  // 使用配置的默认模型
  const defaultModel = getDefaultModel('claude') || 'sonnet';
  document.getElementById('run-task-model').value = defaultModel;
  document.getElementById('run-task-modal').classList.remove('hidden');
  // 存当前 taskId 供 submit 用
  document.getElementById('run-task-modal').dataset.taskId = task.id;
  // type 切换时联动 model 是否可选
  const typeSel = document.getElementById('run-task-type');
  typeSel.onchange = toggleRunTaskModelGroup;
  // model 切换时保存为默认
  const modelSel = document.getElementById('run-task-model');
  modelSel.onchange = () => {
    const type = typeSel.value;
    if (type !== 'shell') saveDefaultModel(type, modelSel.value);
  };
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
  closeRunTaskModal();
  try {
    const body = JSON.stringify({command_type: type, model: model});
    const r = await fetchJSON(API + '/api/tasks/' + taskId + '/run', {method:'POST', headers:{'Content-Type':'application/json'}, body});
    const summary = type + (type !== 'shell' ? ' / ' + model : '');
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
  document.getElementById('task-module').value = t.module || '';
  document.getElementById('task-module').readOnly = true;
  document.getElementById('task-resources').value = t.resources || '';
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
  loadTaskComments(t.id);
}

async function editTask(id) {
  let t = tasks.find(x => x.id === id);
  if (!t) {
    await loadTasks();
    t = tasks.find(x => x.id === id);
  }
  if (t) showTaskModal(t);
}

function showTaskModal(task) {
  document.getElementById('task-modal-title').textContent = task ? '编辑任务' : '新建任务';
  document.getElementById('task-id').value = task ? task.id : '';
  document.getElementById('task-title').value = task ? task.title : '';
  document.getElementById('task-title').readOnly = false;
  document.getElementById('task-desc').value = task ? (task.description || '') : '';
  document.getElementById('task-desc').readOnly = false;
  document.getElementById('task-module').value = task ? (task.module || '') : '';
  document.getElementById('task-module').readOnly = false;
  document.getElementById('task-resources').value = task ? (task.resources || '') : '';
  document.getElementById('task-acceptance').value = task ? (task.acceptance || '') : '';
  document.getElementById('task-acceptance').readOnly = false;
  document.getElementById('task-type').value = task ? (task.task_type || 'manual') : 'manual';
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
  loadTaskComments(t.id);
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
    experience_id: _selectedExps.map(s => s.id).join(','),
    module: document.getElementById('task-module').value,
    resources: document.getElementById('task-resources').value,
    acceptance: document.getElementById('task-acceptance').value,
    task_type: document.getElementById('task-type').value
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

async function loadTaskComments(taskId) { console.log("loadTaskComments called, taskId:", taskId);
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
