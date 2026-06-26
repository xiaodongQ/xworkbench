// Widget 侧栏（左侧常驻）：链接（列表）/ 目录 / 待办
// 链接：每行一条；目录：打开失败弹错误；待办：支持增删
// 依赖 api.js (fetchJSON/esc)

// 通用：按 sort_order 排序
function sortByOrder(arr) { return [...arr].sort((a,b) => (a.sort_order||0) - (b.sort_order||0)); }

// 通用：拖动重排 helper（PUT sort_order 批量更新）
async function reorderAndSave(type, idsInNewOrder) {
  // 并行 PUT 全部 sort_order
  const promises = idsInNewOrder.map((id, idx) =>
    fetch(`/api/${type}/${id}`, {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({sort_order: idx + 1}),
    })
  );
  await Promise.all(promises);
}

// ===== 链接（列表样式：每行一条） =====
async function loadLinks() {
  const list = sortByOrder(await fetchJSON('/api/web-links'));
  const grid = document.getElementById('links-grid');
  if (!list || list.length === 0) {
    grid.innerHTML = '<div style="color:var(--text-secondary);font-size:12px;text-align:center;padding:20px 0">点击 + 添加你的第一个链接</div>';
    return;
  }
  grid.innerHTML = list.map((l, idx) => {
    const initial = (l.name || '?')[0].toUpperCase();
    const icon = l.icon_url
      ? `<img src="${esc(l.icon_url)}" onerror="this.outerHTML='${initial}'">`
      : initial;
    return `<div class="link-row" draggable="true" data-id="${l.id}" data-idx="${idx}"
        ondragstart="widgetDragStart(event, 'web-links')" ondragover="widgetDragOver(event)" ondrop="widgetDrop(event, 'web-links', loadLinks)" ondragleave="widgetDragLeave(event)">
      <span class="drag-handle" title="拖动排序">⋮⋮</span>
      <div class="link-icon" onclick="window.open('${esc(l.url)}','_blank')">${icon}</div>
      <div class="link-text" onclick="window.open('${esc(l.url)}','_blank')" title="${esc(l.url)}">
        <div class="link-name">${esc(l.name)}</div>
        <div class="link-url">${esc(l.url)}</div>
      </div>
      <div class="link-edit" onclick="event.stopPropagation();editLink('${l.id}')" title="编辑">✎</div>
      <div class="link-del" onclick="event.stopPropagation();deleteLink('${l.id}')" title="删除">×</div>
    </div>`;
  }).join('');
}

function showLinkModal() {
  document.getElementById('link-name').value = '';
  document.getElementById('link-url').value = '';
  document.getElementById('link-icon').value = '';
  document.getElementById('link-modal').dataset.editId = '';  // 确保新建时清空
  const titleEl = document.querySelector('#link-modal h2');
  if (titleEl) titleEl.textContent = '添加链接';
  document.getElementById('link-modal').classList.remove('hidden');
  setTimeout(() => document.getElementById('link-name').focus(), 50);
}
function closeLinkModal() {
  document.getElementById('link-modal').classList.add('hidden');
  document.getElementById('link-modal').dataset.editId = '';
  const titleEl = document.querySelector('#link-modal h2');
  if (titleEl) titleEl.textContent = '添加链接';
}
async function submitLink() {
  const id = document.getElementById('link-modal').dataset.editId;
  const name = document.getElementById('link-name').value.trim();
  const url = document.getElementById('link-url').value.trim();
  const icon = document.getElementById('link-icon').value.trim();
  if (!name || !url) { alert('名称和 URL 必填'); return; }
  if (id) {
    await fetch('/api/web-links/' + id, {method:'PUT', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({name, url, icon_url: icon})});
  } else {
    await fetch('/api/web-links', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({name,url,icon_url:icon})});
  }
  closeLinkModal();
  loadLinks();
}
async function editLink(id) {
  const list = sortByOrder(await fetchJSON('/api/web-links'));
  const l = list.find(x => x.id === id);
  if (!l) return;
  document.getElementById('link-name').value = l.name || '';
  document.getElementById('link-url').value = l.url || '';
  document.getElementById('link-icon').value = l.icon_url || '';
  const titleEl = document.querySelector('#link-modal h2');
  if (titleEl) titleEl.textContent = '编辑链接';
  document.getElementById('link-modal').dataset.editId = id;
  document.getElementById('link-modal').classList.remove('hidden');
}
function deleteLink(id) {
  if (!confirm('删除该链接？')) return;
  fetch('/api/web-links/' + id, {method:'DELETE'}).then(() => loadLinks());
}

// ===== 目录 =====
async function loadDirs() {
  const list = sortByOrder(await fetchJSON('/api/dir-shortcuts'));
  const el = document.getElementById('dir-list');
  if (!list || list.length === 0) {
    el.innerHTML = `<div class="dir-item" onclick="showDirModal()" style="font-style:italic;color:var(--text-secondary)">
      <span class="dir-icon">📂</span>
      <span class="dir-text">
        <span class="dir-name">+ 添加目录</span>
        <span class="dir-path">点击添加</span>
      </span>
    </div>`;
    return;
  }
  el.innerHTML = list.map((d, idx) =>
    `<div class="dir-item${d.type === 'remote' ? ' dir-remote' : ''}" draggable="true" data-id="${d.id}" data-idx="${idx}"
        ondragstart="widgetDragStart(event, 'dir-shortcuts')" ondragover="widgetDragOver(event)" ondrop="widgetDrop(event, 'dir-shortcuts', loadDirs)" ondragleave="widgetDragLeave(event)">
      <span class="drag-handle" title="拖动排序"></span>
      <span class="dir-icon" onclick="openDir('${d.id}')">${d.type === 'remote' ? '🌐' : '📁'}</span>
      <span class="dir-term" onclick="event.stopPropagation();openDirTerminal('${d.id}')" title="打开终端">⬢</span>
      <span class="dir-text" onclick="openDir('${d.id}')">
        <span class="dir-name">${esc(d.name)}</span>
        <span class="dir-path" title="${esc(d.type === 'remote' ? d.remote_user + '@' + d.remote_host : d.path)}">${esc(d.type === 'remote' ? d.remote_user + '@' + d.remote_host : d.path)}</span>
      </span>
      <span class="dir-edit" onclick="event.stopPropagation();editDir('${d.id}')" title="编辑">✎</span>
      <span class="dir-del" onclick="event.stopPropagation();deleteDir('${d.id}')" title="删除">×</span>
    </div>`).join('');
}
function showDirModal() {
  document.getElementById('dir-name').value = '';
  document.getElementById('dir-type').value = 'local';
  document.getElementById('dir-path').value = '';
  document.getElementById('dir-remote-host').value = '';
  document.getElementById('dir-remote-user').value = '';
  document.getElementById('dir-remote-path').value = '';
  document.getElementById('dir-auth-method').value = 'password';
  document.getElementById('dir-remote-password').value = '';
  document.getElementById('dir-key-path').value = '';
  document.getElementById('dir-terminal-cmd').value = '';
  document.getElementById('dir-modal').dataset.editId = '';
  document.getElementById('dir-modal').classList.remove('hidden');
  document.getElementById('dir-modal-title').textContent = '添加目录';
  document.getElementById('dir-submit-btn').textContent = '添加';
  onDirTypeChange();
  onAuthMethodChange();
  setTimeout(() => document.getElementById('dir-name').focus(), 50);
}
function closeDirModal() { document.getElementById('dir-modal').classList.add('hidden'); }
function onDirTypeChange() {
  const type = document.getElementById('dir-type').value;
  const remoteFields = document.getElementById('dir-remote-fields');
  const localPathGroup = document.getElementById('dir-local-path-group');
  if (type === 'remote') {
    remoteFields.classList.remove('hidden');
    localPathGroup.classList.add('hidden');
    // 远程目录默认填充 root 用户
    const remoteUserInput = document.getElementById('dir-remote-user');
    if (!remoteUserInput.value) remoteUserInput.value = 'root';
  } else {
    remoteFields.classList.add('hidden');
    localPathGroup.classList.remove('hidden');
  }
}
function onAuthMethodChange() {
  const method = document.getElementById('dir-auth-method').value;
  document.getElementById('dir-password-group').classList.toggle('hidden', method === 'key');
  document.getElementById('dir-key-path-group').classList.toggle('hidden', method !== 'key');
}
function showDirSettingsModal() {
  fetchJSON('/api/config').then(data => {
    const term = data.terminal;
    const sel = document.getElementById('dir-settings-terminal-select');
    // fix: 默认终端已上移到顶层 default_terminal（旧字段 terminal.default_type 已不存在）
    sel.value = data.default_terminal || term.default_type || 'wezterm';
    // 显示当前选中类型的 path
    const typeKey = sel.value.toLowerCase();
    const typeDef = term.types[typeKey];
    document.getElementById('dir-settings-terminal-path').value = typeDef?.path || '';
    if (typeDef?.path) {
      document.getElementById('dir-term-detected-path').textContent = '已保存: ' + typeDef.path;
      document.getElementById('dir-term-detected-path').style.color = 'var(--archived)';
    } else {
      document.getElementById('dir-term-detected-path').textContent = '点击"检测"查找可用路径';
      document.getElementById('dir-term-detected-path').style.color = 'var(--text-secondary)';
    }
  }).catch(() => {
    document.getElementById('dir-settings-terminal-select').value = 'wezterm';
    document.getElementById('dir-settings-terminal-path').value = '';
  });
  document.getElementById('dir-settings-modal').classList.remove('hidden');
}
function closeDirSettingsModal() { document.getElementById('dir-settings-modal').classList.add('hidden'); }
// 终端检测的代际计数器：每次 onDirTermTypeChange 自增，避免 stale 的 detect 结果
// 覆盖当前 select 对应的 path input（race 防护）
let _dirDetectGen = 0;
function onDirTermTypeChange() {
  // 切换类型时显示该类型已保存的路径
  const gen = ++_dirDetectGen;
  const type = document.getElementById('dir-settings-terminal-select').value.toLowerCase();
  const pathInput = document.getElementById('dir-settings-terminal-path');
  const pathDiv = document.getElementById('dir-term-detected-path');
  fetchJSON('/api/config').then(data => {
    // 回调期间用户又切了 select → 丢弃 stale 结果
    if (gen !== _dirDetectGen) return;
    const typeDef = data.terminal.types[type];
    if (typeDef?.path) {
      pathInput.value = typeDef.path;
      pathDiv.textContent = '已保存: ' + typeDef.path;
      pathDiv.style.color = 'var(--archived)';
    } else {
      pathInput.value = '';
      pathDiv.textContent = '点击"检测"查找可用路径';
      pathDiv.style.color = 'var(--text-secondary)';
    }
  });
  detectDirTerminalPath(gen);
}
async function detectDirTerminalPath(gen) {
  // gen 参数可选：显式传入则复用 onDirTermTypeChange 的代际计数；否则自增
  if (gen === undefined) gen = ++_dirDetectGen;
  const type = document.getElementById('dir-settings-terminal-select').value;
  const pathDiv = document.getElementById('dir-term-detected-path');
  const pathInput = document.getElementById('dir-settings-terminal-path');
  pathDiv.textContent = '检测中...';
  pathDiv.style.color = 'var(--text-secondary)';
  try {
    const r = await fetch('/api/terminals/detect?type=' + encodeURIComponent(type));
    const data = await r.json();
    // race 防护：fetch 回来后如果代际已变（用户切了 select）或 select 当前值变了，
    // 则本结果已 stale，丢弃，避免污染当前 select 对应的 path input。
    if (gen !== _dirDetectGen) return;
    const curType = document.getElementById('dir-settings-terminal-select').value;
    if (curType.toLowerCase() !== type.toLowerCase()) return;
    if (data.path) {
      pathDiv.textContent = data.path;
      pathDiv.style.color = 'var(--archived)';
      pathInput.value = data.path;
    } else {
      pathDiv.textContent = '未找到 ' + type + '，请手动填写路径';
      pathDiv.style.color = 'var(--exception)';
    }
  } catch (e) {
    if (gen !== _dirDetectGen) return;
    pathDiv.textContent = '检测失败：' + e.message;
    pathDiv.style.color = 'var(--exception)';
  }
}
function submitDirSettings() {
  const term = document.getElementById('dir-settings-terminal-select').value;
  const path = document.getElementById('dir-settings-terminal-path').value.trim();
  fetch('/api/config', {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    // fix: 同步写默认终端（default_terminal）+ 终端 path
    // terminal_type/terminal_path 只更新 types 表里某条的 path；
    // 真正生效的是顶层 default_terminal
    body: JSON.stringify({default_terminal: term, terminal_type: term, terminal_path: path})
  }).then(() => closeDirSettingsModal()).catch(e => {
    alert('保存失败：' + e.message);
    closeDirSettingsModal();
  });
}
async function editDir(id) {
  const list = sortByOrder(await fetchJSON('/api/dir-shortcuts'));
  const d = list.find(x => x.id === id);
  if (!d) return;
  document.getElementById('dir-name').value = d.name || '';
  document.getElementById('dir-type').value = d.type || 'local';
  document.getElementById('dir-path').value = d.path || '';
  document.getElementById('dir-remote-host').value = d.remote_host || '';
  document.getElementById('dir-remote-user').value = d.remote_user || '';
  document.getElementById('dir-remote-path').value = d.remote_path || '';
  document.getElementById('dir-auth-method').value = d.auth_method || 'password';
  document.getElementById('dir-remote-password').value = d.remote_password || '';
  document.getElementById('dir-key-path').value = d.key_path || '';
  document.getElementById('dir-terminal-cmd').value = d.terminal_cmd || '';
  onDirTypeChange();
  onAuthMethodChange();
  document.getElementById('dir-modal').dataset.editId = id;
  document.getElementById('dir-modal').classList.remove('hidden');
  document.getElementById('dir-modal-title').textContent = '编辑目录';
  document.getElementById('dir-submit-btn').textContent = '保存';
  setTimeout(() => document.getElementById('dir-name').focus(), 50);
}
function submitDir() {
  const id = document.getElementById('dir-modal').dataset.editId;
  const name = document.getElementById('dir-name').value.trim();
  const type = document.getElementById('dir-type').value;
  const path = document.getElementById('dir-path').value.trim();
  const remoteHost = document.getElementById('dir-remote-host').value.trim();
  const remoteUser = document.getElementById('dir-remote-user').value.trim();
  const remotePath = document.getElementById('dir-remote-path').value.trim();
  const authMethod = document.getElementById('dir-auth-method').value;
  const remotePassword = document.getElementById('dir-remote-password').value;
  const keyPath = document.getElementById('dir-key-path').value.trim();
  const terminalCmd = document.getElementById('dir-terminal-cmd').value.trim();
  if (!name) { alert('名称必填'); return; }
  if (type === 'local' && !path) { alert('本地目录路径必填'); return; }
  if (type === 'remote' && (!remoteHost || !remoteUser)) { alert('主机和用户名必填'); return; }
  const payload = { name, type, path, remote_host: remoteHost, remote_user: remoteUser, remote_path: remotePath, auth_method: authMethod, remote_password: remotePassword, key_path: keyPath, terminal_cmd: terminalCmd };
  if (id) {
    fetch('/api/dir-shortcuts/' + id, {method:'PUT', headers:{'Content-Type':'application/json'},
      body: JSON.stringify(payload)})
      .then(() => { closeDirModal(); loadDirs(); });
  } else {
    fetch('/api/dir-shortcuts', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(payload)})
      .then(r => r.json().then(d => ({ok: r.ok, body: d})))
      .then(({ok, body}) => {
        if (!ok) { alert('添加失败：' + (body.error || '未知错误')); return; }
        closeDirModal();
        loadDirs();
      });
  }
}
async function openDir(id) {
  try {
    const r = await fetch('/api/dir-shortcuts/' + id + '/open', {method:'POST'});
    if (!r.ok) {
      const body = await r.json().catch(() => ({}));
      const msg = body.error || '';
      if (msg.includes('remote shortcut')) {
        alert('远程目录无法直接打开，请点击右侧 ⬢ 按钮用终端打开');
        return;
      }
      alert('打开失败：' + (msg || r.statusText || '目录可能不存在或无权限'));
      return;
    }
  } catch (e) {
    alert('打开失败：' + e.message);
  }
}

// openDirTerminal 打开终端到指定目录，使用配置的默认终端类型
async function openDirTerminal(id) {
  try {
    const data = await fetchJSON('/api/config');
    // fix: 默认终端字段从 terminal.default_type 改为顶层 default_terminal
    const termType = data.default_terminal || data.terminal?.default_type || 'wezterm';
    const r = await fetch('/api/dir-shortcuts/' + id + '/open-terminal?type=' + encodeURIComponent(termType), {method:'POST'});
    if (!r.ok) {
      const body = await r.json().catch(() => ({}));
      alert('打开终端失败：' + (body.error || r.statusText || '请确认终端已安装'));
      return;
    }
  } catch (e) {
    alert('打开终端失败：' + e.message);
  }
}

function deleteDir(id) {
  if (!confirm('删除该目录快捷？')) return;
  fetch('/api/dir-shortcuts/' + id, {method:'DELETE'}).then(() => loadDirs());
}

// ===== 待办（支持增删 + 勾选） =====
const TODO_SHOW_ALL_KEY = 'todo.showAll';
function toggleTodoShowAll() {
  const el = document.getElementById('todo-show-all-icon');
  const active = el.textContent === '☑';
  const newActive = !active;
  el.textContent = newActive ? '☑' : '☐';
  el.style.opacity = newActive ? '1' : '0.5';
  localStorage.setItem(TODO_SHOW_ALL_KEY, newActive ? 'true' : 'false');
  loadTodo();
}
function initTodoShowAll() {
  const el = document.getElementById('todo-show-all-icon');
  if (!el) return;
  const saved = localStorage.getItem(TODO_SHOW_ALL_KEY);
  const showAll = saved === 'true';
  el.textContent = showAll ? '☑' : '☐';
  el.style.opacity = showAll ? '1' : '0.5';
}
async function loadTodo() {
  const showAll = document.getElementById('todo-show-all-icon')?.textContent === '☑';
  const data = await fetchJSON('/api/todo');
  const el = document.getElementById('todo-list');
  if (!data.path) { el.innerHTML = '<div style="color:var(--text-secondary);font-size:12px">未配置 todo.md 路径，点击"设置"</div>'; return; }
  const items = showAll ? (data.items || []) : (data.items || []).filter(i => !i.done);
  if (items.length === 0) { el.innerHTML = '<div style="color:var(--text-secondary);font-size:12px">' + esc(data.path) + ' 无 todo 项</div>'; return; }
  el.innerHTML = items.map(i =>
    `<div class="todo-item ${i.done?'done':''}">
      <input type="checkbox" ${i.done?'checked':''} onchange="toggleTodo(${i.line_no}, this.checked)">
      <span class="todo-text">${esc(i.text)}</span>
      <span class="todo-del" onclick="deleteTodoItem(${i.line_no})" title="删除">×</span>
    </div>`).join('');
}
function toggleTodo(lineNo, done) {
  fetch('/api/todo/' + lineNo, {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({done})})
    .then(() => loadTodo());
}
async function deleteTodoItem(lineNo) {
  if (!confirm('删除该待办？')) return;
  const r = await fetch('/api/todo/' + lineNo, {method:'DELETE'});
  if (!r.ok) { const b = await r.json().catch(() => ({})); alert('删除失败：' + (b.error || r.statusText)); return; }
  loadTodo();
}
function showTodoAddModal() {
  document.getElementById('todo-add-input').value = '';
  document.getElementById('todo-add-modal').classList.remove('hidden');
  setTimeout(() => document.getElementById('todo-add-input').focus(), 50);
}
function closeTodoAddModal() { document.getElementById('todo-add-modal').classList.add('hidden'); }
async function submitTodoAdd() {
  const text = document.getElementById('todo-add-input').value.trim();
  if (!text) { alert('内容必填'); return; }
  const r = await fetch('/api/todo', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({text})});
  if (!r.ok) { const b = await r.json().catch(() => ({})); alert('添加失败：' + (b.error || r.statusText)); return; }
  closeTodoAddModal();
  loadTodo();
}
async function showTodoPathModal() {
  const d = await fetchJSON('/api/todo/path');
  document.getElementById('todo-path-input').value = d.path || '';
  document.getElementById('todo-path-modal').classList.remove('hidden');
  setTimeout(() => document.getElementById('todo-path-input').focus(), 50);
}
function closeTodoPathModal() { document.getElementById('todo-path-modal').classList.add('hidden'); }
function submitTodoPath() {
  const path = document.getElementById('todo-path-input').value.trim();
  if (!path) { alert('路径必填'); return; }
  fetch('/api/todo/path', {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({path})})
    .then(() => { closeTodoPathModal(); loadTodo(); });
}

// ===== 通用拖动重排（HTML5 drag/drop） =====
let _dragSrcId = null;          // 拖动源 id
let _dragType = null;           // 'web-links' | 'dir-shortcuts' | 'todos'
let _dragReloading = null;      // 拖动结束后调用的 reload 回调

function widgetDragStart(e, type) {
  _dragSrcId = e.currentTarget.dataset.id;
  _dragType = type;
  e.dataTransfer.effectAllowed = 'move';
  e.dataTransfer.setData('text/plain', _dragSrcId);
  e.currentTarget.style.opacity = '0.4';
}
function widgetDragOver(e) {
  if (!_dragSrcId) return;
  e.preventDefault();
  e.dataTransfer.dropEffect = 'move';
  e.currentTarget.style.borderTop = '2px solid var(--primary)';
}
function widgetDragLeave(e) {
  e.currentTarget.style.borderTop = '';
}
async function widgetDrop(e, type, reloadFn) {
  e.preventDefault();
  e.currentTarget.style.borderTop = '';
  const tgtId = e.currentTarget.dataset.id;
  // 恢复 opacity
  document.querySelectorAll(`[data-id][draggable="true"]`).forEach(el => el.style.opacity = '');
  if (!_dragSrcId || _dragSrcId === tgtId) { _dragSrcId = null; return; }
  // 重排
  const container = e.currentTarget.parentElement;
  const idsInNewOrder = Array.from(container.querySelectorAll('[data-id][draggable="true"]')).map(el => el.dataset.id);
  // 把 src 移到 tgt 位置
  const srcIdx = idsInNewOrder.indexOf(_dragSrcId);
  const tgtIdx = idsInNewOrder.indexOf(tgtId);
  if (srcIdx < 0 || tgtIdx < 0) { _dragSrcId = null; return; }
  idsInNewOrder.splice(srcIdx, 1);
  idsInNewOrder.splice(tgtIdx, 0, _dragSrcId);
  // 持久化
  await reorderAndSave(type, idsInNewOrder);
  _dragSrcId = null;
  if (reloadFn) reloadFn();
}

// 初始化 todo 显示全部状态
if (typeof window !== "undefined") {
  document.addEventListener("DOMContentLoaded", () => {
    setTimeout(() => { initTodoShowAll(); loadTodo(); }, 100);
  });
}
