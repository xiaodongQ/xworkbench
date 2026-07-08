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

// openLink 打开链接：HTTP 用浏览器，本地路径/file:// 用系统原生工具
async function openLink(url) {
  // 判断是否本地路径：Unix绝对路径、~、file://、Windows盘符、UNC路径
  const isLocal = /^(file:\/\/|\/[^/]|~|\\|[a-zA-Z]:[\\/]|[\\\/]{2})/.test(url);
  if (!isLocal) {
    window.open(url, '_blank');
    return;
  }
  try {
    await fetchJSON('/api/links/open', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({url}),
    });
  } catch(e) {
    alert('打不开：' + e.message);
  }
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
      <div class="link-icon" onclick="openLink('${esc(l.url)}')">${icon}</div>
      <div class="link-text" onclick="openLink('${esc(l.url)}')" title="${esc(l.url)}">
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
      <span class="dir-term" onclick="event.stopPropagation();openDirTerminal('${d.id}')" title="打开外部终端">⬢</span>
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
    sel.value = data.default_terminal || term.default_type || 'wezterm';
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
let _dirDetectGen = 0;
function onDirTermTypeChange() {
  const gen = ++_dirDetectGen;
  const type = document.getElementById('dir-settings-terminal-select').value.toLowerCase();
  const pathInput = document.getElementById('dir-settings-terminal-path');
  const pathDiv = document.getElementById('dir-term-detected-path');
  fetchJSON('/api/config').then(data => {
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
  if (gen === undefined) gen = ++_dirDetectGen;
  const type = document.getElementById('dir-settings-terminal-select').value;
  const pathDiv = document.getElementById('dir-term-detected-path');
  const pathInput = document.getElementById('dir-settings-terminal-path');
  pathDiv.textContent = '检测中...';
  pathDiv.style.color = 'var(--text-secondary)';
  try {
    const r = await fetch('/api/terminals/detect?type=' + encodeURIComponent(type));
    const data = await r.json();
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
  document.getElementById('dir-remote-port').value = d.remote_port || '';
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
  const remotePort = document.getElementById('dir-remote-port').value.trim();
  const remoteUser = document.getElementById('dir-remote-user').value.trim();
  const remotePath = document.getElementById('dir-remote-path').value.trim();
  const authMethod = document.getElementById('dir-auth-method').value;
  const remotePassword = document.getElementById('dir-remote-password').value;
  const keyPath = document.getElementById('dir-key-path').value.trim();
  const terminalCmd = document.getElementById('dir-terminal-cmd').value.trim();
  if (!name) { alert('名称必填'); return; }
  if (type === 'local' && !path) { alert('本地目录路径必填'); return; }
  if (type === 'remote' && (!remoteHost || !remoteUser)) { alert('主机和用户名必填'); return; }
  const payload = { name, type, path, remote_host: remoteHost, remote_port: remotePort, remote_user: remoteUser, remote_path: remotePath, auth_method: authMethod, remote_password: remotePassword, key_path: keyPath, terminal_cmd: terminalCmd };
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

async function openDirTerminal(id) {
  try {
    const data = await fetchJSON('/api/config');
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
// 展开状态：Set of parent lineNo，localStorage 持久化
let _todoExpandedSet = new Set(JSON.parse(localStorage.getItem('todoExpandedSet') || '[]'));
function saveTodoExpandedSet() {
    localStorage.setItem('todoExpandedSet', JSON.stringify([..._todoExpandedSet]));
}

// 递归过滤树：父项被过滤则子项也移除
function filterTree(items) {
    if (!items) return [];
    return items.filter(item => {
        if (!_todoFilter.showDone && item.done) return false;
        return true;
    }).map(item => {
        const filtered = filterTree(item.children);
        return Object.assign({}, item, { children: filtered });
    });
}

// 只排当前层级节点，子项保持原顺序
function sortTree(items) {
    const today = new Date().toISOString().split('T')[0];
    return [...items].sort((a, b) => {
        const aOverdue = a.due_date && !a.done && a.due_date < today;
        const bOverdue = b.due_date && !b.done && b.due_date < today;
        if (aOverdue && !bOverdue) return -1;
        if (!aOverdue && bOverdue) return 1;
        if (a.due_date && b.due_date) return a.due_date.localeCompare(b.due_date);
        if (a.due_date && !b.due_date) return -1;
        if (!a.due_date && b.due_date) return 1;
        return (a.line_no || 0) - (b.line_no || 0);
    }).map(item => {
        if (item.children && item.children.length > 0) {
            return Object.assign({}, item, { children: sortTree(item.children) });
        }
        return item;
    });
}

// 扁平化树：打平为单层列表，附加 _depth、_child_count、_parent_line_no，删除 children
function flattenItems(items, depth, parentLineNo) {
    if (depth === undefined) depth = 0;
    let result = [];
    for (const item of items) {
        const childCount = item.children ? item.children.length : 0;
        const flat = Object.assign({}, item, {
            _depth: depth,
            _child_count: childCount,
            _parent_line_no: parentLineNo !== undefined ? parentLineNo : null,
        });
        delete flat.children;
        result.push(flat);
        if (item.children) {
            result = result.concat(flattenItems(item.children, depth + 1, item.line_no));
        }
    }
    return result;
}

// 渲染单个 todo item（扁平列表，每行独立；用 _depth 算缩进，_child_count 显示徽章）
function renderTodoItem(i) {
  const today = new Date().toISOString().split('T')[0];
  const isOverdue = i.due_date && !i.done && i.due_date < today;
  const dueLabel = i.due_date ? i.due_date.slice(5) : '';
  const childCount = i._child_count || 0;
  const hasChildren = childCount > 0;
  const isExpanded = _todoExpandedSet.has(i.line_no);
  // 初始隐藏：父项未展开时隐藏子项
  const hiddenByParent = i._parent_line_no != null && !_todoExpandedSet.has(i._parent_line_no);

  let extraHtml = '';
  const childClickAttr = hasChildren ? `onclick="event.stopPropagation(); toggleTodoItem(${i.line_no})"` : '';
  if (i.due_date) {
    extraHtml += `<span class="todo-due ${isOverdue ? 'overdue' : ''}" ${childClickAttr} title="${esc(i.due_date)}">📅 ${dueLabel}</span>`;
  }
  if (hasChildren) {
    extraHtml += `<span class="todo-child-count" ${childClickAttr} title="${childCount} 个子项">${childCount}</span>`;
  }

  const indent = i._depth * 20;
  const onclickAttr = hasChildren ? `onclick="event.stopPropagation(); toggleTodoItem(${i.line_no})"` : '';
  const textOnclickAttr = hasChildren ? `onclick="toggleTodoItem(${i.line_no})"` : '';
  let html = `<div class="todo-item ${i.done?'done':''} ${isOverdue?'overdue-row':''} ${hasChildren?'has-children':''}" ${onclickAttr} ${hasChildren ? `data-line-no="${i.line_no}" data-parent-line-no="${i._parent_line_no}" style="padding-left:${indent}px"` : `style="padding-left:${indent}px" data-parent-line-no="${i._parent_line_no != null ? i._parent_line_no : ''}"}`}>
  <span class="todo-indent"></span>
  <input type="checkbox" ${i.done?'checked':''} onchange="event.stopPropagation(); toggleTodo(${i.line_no}, this.checked)">
  <span class="todo-text" ${textOnclickAttr}>${esc(i.text)}</span>
  ${extraHtml}
  <span class="todo-edit" onclick="event.stopPropagation(); openTodoEditModal(${i.line_no})" title="编辑">✎</span>
  <span class="todo-del" onclick="event.stopPropagation(); deleteTodoItem(${i.line_no})" title="删除">×</span>
</div>`;
  return html;
}

// 点击父任务文字，展开/收起子项（递归）
function toggleTodoItem(lineNo) {
    const itemEl = document.querySelector('.todo-item[data-line-no="' + lineNo + '"]');
    if (!itemEl) return;
    // 找直接子项（_parent_line_no === lineNo）
    const childEls = document.querySelectorAll('.todo-item[data-parent-line-no="' + lineNo + '"]');
    if (childEls.length === 0) return; // 无子项不动作
    const isExpanded = _todoExpandedSet.has(lineNo);
    if (isExpanded) {
        // 递归隐藏子项的子项
        const hideRecursive = (parentNo) => {
            const els = document.querySelectorAll('.todo-item[data-parent-line-no="' + parentNo + '"]');
            els.forEach(el => {
                el.style.display = 'none';
                const ln = el.dataset.lineNo;
                if (ln) hideRecursive(ln);
            });
        };
        childEls.forEach(el => { el.style.display = 'none'; hideRecursive(el.dataset.lineNo); });
        _todoExpandedSet.delete(lineNo);
    } else {
        childEls.forEach(el => { el.style.display = ''; });
        _todoExpandedSet.add(lineNo);
    }
    saveTodoExpandedSet();
}

// 标题栏：展开/收起所有
function toggleTodoExpandAll() {
    const anyExpanded = _todoExpandedSet.size > 0;
    if (anyExpanded) {
        // 全部收起
        _todoExpandedSet.clear();
        const btn = document.getElementById('todo-expand-btn');
        if (btn) { btn.textContent = '▶'; btn.title = '展开所有子项'; }
    } else {
        // 全部展开：递归收集所有有子项的 line_no
        const collectParents = (items) => {
            for (const item of items) {
                if (item.children && item.children.length > 0) {
                    _todoExpandedSet.add(item.line_no);
                    collectParents(item.children);
                }
            }
        };
        collectParents(window._todoTreeData || []);
        const btn = document.getElementById('todo-expand-btn');
        if (btn) { btn.textContent = '▼'; btn.title = '收起所有子项'; }
    }
    saveTodoExpandedSet();
    loadTodo(); // 重新渲染
}

async function loadTodo() {
  const data = await fetchJSON('/api/todo');
  const el = document.getElementById('todo-list');
  if (!data.path) { el.innerHTML = '<div style="color:var(--text-secondary);font-size:12px">未配置 todo.md 路径，点击"设置"</div>'; return; }
  // 保存原始树数据供 findTodoItem 编辑弹框使用
  window._todoTreeData = data.items || [];
  const items = flattenItems(sortTree(filterTree(window._todoTreeData)));
  window._todoItems = items;
  const filterBtn = document.getElementById('todo-filter-btn');
  if (filterBtn) {
    filterBtn.textContent = _todoFilter.showDone ? '◉' : '☐';
    filterBtn.title = '点击切换到: ' + (_todoFilter.showDone ? '仅未完成' : '显示全部');
  }
  const expandBtn = document.getElementById('todo-expand-btn');
  if (expandBtn) {
    if (_todoExpandedSet.size > 0) {
        expandBtn.textContent = '▼';
        expandBtn.title = '收起所有子项';
    } else {
        expandBtn.textContent = '▶';
        expandBtn.title = '展开所有子项';
    }
  }
  if (items.length === 0) {
    const hint = _todoFilter.showDone ? '' : '（全部已完成）';
    el.innerHTML = '<div style="color:var(--text-secondary);font-size:12px">' + esc(data.path) + ' 无 todo 项' + hint + '</div>'; return;
  }
  // 初始渲染时：_parent_line_no 非空且父项未展开的子项需隐藏
  let html = '';
  for (const item of items) {
    const hiddenByParent = item._parent_line_no != null && !_todoExpandedSet.has(item._parent_line_no);
    html += renderTodoItem(item);
    // 在 renderTodoItem 里隐藏靠 CSS，简化处理：render 后统一处理
  }
  el.innerHTML = html;
  // 统一处理初始隐藏
  el.querySelectorAll('.todo-item[data-parent-line-no]').forEach(el => {
    const pln = el.dataset.parentLineNo;
    if (pln && pln !== '' && !_todoExpandedSet.has(parseInt(pln))) {
        el.style.display = 'none';
    }
  });
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
  document.getElementById('todo-modal-title').textContent = '添加任务';
  document.getElementById('todo-edit-line-no').value = '';
  document.getElementById('todo-modal-title-input').value = '';
  document.getElementById('todo-modal-due').value = '';
  document.getElementById('todo-modal-submit').textContent = '添加';
  document.getElementById('todo-modal-children').innerHTML = '';
  document.getElementById('todo-add-modal').classList.remove('hidden');
  setTimeout(() => document.getElementById('todo-modal-title-input').focus(), 50);
}
function openTodoEditModal(lineNo) {
  const item = findTodoItem(lineNo);
  if (!item) return;
  document.getElementById('todo-modal-title').textContent = '编辑任务';
  document.getElementById('todo-edit-line-no').value = lineNo;
  document.getElementById('todo-modal-title-input').value = item.text;
  document.getElementById('todo-modal-due').value = item.due_date || '';
  document.getElementById('todo-modal-submit').textContent = '保存';
  // 渲染现有子项
  const container = document.getElementById('todo-modal-children');
  container.innerHTML = '';
  if (item.children) {
    item.children.forEach(c => addTodoChildRow(c.text, c.line_no, c.done));
  }
  document.getElementById('todo-add-modal').classList.remove('hidden');
  setTimeout(() => document.getElementById('todo-modal-title-input').focus(), 50);
}
function closeTodoAddModal() {
  document.getElementById('todo-add-modal').classList.add('hidden');
}
// 添加一行子项输入框，lineNo/done 可选（有则是已有子项，删除时需调用 API）
function addTodoChildRow(text, lineNo, done) {
  const container = document.getElementById('todo-modal-children');
  const row = document.createElement('div');
  row.style.display = 'flex';
  row.style.gap = '4px';
  row.style.alignItems = 'center';
  if (lineNo !== undefined) {
    row.dataset.lineNo = lineNo;
    row.dataset.done = done ? '1' : '0';
    row.dataset.originalDone = done ? '1' : '0';
  }
  row.innerHTML = `
    <input type="checkbox" class="todo-child-done" ${done ? 'checked' : ''}>
    <input type="text" class="todo-child-row" value="${esc(text || '')}" placeholder="子项内容" style="flex:1;box-sizing:border-box">
    <button class="btn btn-small" onclick="removeTodoChildRow(this)" title="删除">×</button>
  `;
  container.appendChild(row);
}
function removeTodoChildRow(btn) {
  const row = btn.parentElement;
  const lineNo = row.dataset.lineNo;
  if (lineNo) {
    deleteTodoItem(parseInt(lineNo));
  }
  row.remove();
}
async function submitTodoModal() {
  const text = document.getElementById('todo-modal-title-input').value.trim();
  if (!text) { alert('请输入任务内容'); return; }
  const dueDate = document.getElementById('todo-modal-due').value;
  const editLineNo = document.getElementById('todo-edit-line-no').value;

  if (editLineNo) {
    // 编辑
    const childRows = Array.from(document.querySelectorAll('#todo-modal-children > div'));
    const existingLineNos = childRows.filter(r => r.dataset.lineNo).map(r => parseInt(r.dataset.lineNo));
    // 新增子项（含勾选状态）
    const newChildRows = childRows.filter(r => !r.dataset.lineNo);
    // 已有子项文本
    const keptTexts = childRows.filter(r => r.dataset.lineNo).map(r => r.querySelector('.todo-child-row').value.trim()).filter(t => t);

    const item = findTodoItem(parseInt(editLineNo));
    const originalChildLineNos = (item && item.children) ? item.children.map(c => c.line_no) : [];

    // 删除被移除的子项
    for (const ln of originalChildLineNos) {
      if (!existingLineNos.includes(ln)) {
        await fetch('/api/todo/' + ln, { method: 'DELETE' });
      }
    }

    // 更新主任务
    await fetch('/api/todo/' + editLineNo + '/edit', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text, due_date: dueDate })
    });

    // 同步已有子项勾选状态
    for (const ln of existingLineNos) {
      const row = childRows.find(r => r.dataset.lineNo == ln);
      if (row) {
        const currentDone = row.querySelector('.todo-child-done').checked;
        const originalDone = row.dataset.originalDone === '1';
        if (currentDone !== originalDone) {
          await fetch('/api/todo/' + ln, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ done: currentDone })
          });
        }
        const newText = row.querySelector('.todo-child-row').value.trim();
        if (newText) {
          await fetch('/api/todo/' + ln + '/edit', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ text: newText })
          });
        }
      }
    }

    // 添加新增子项（含勾选状态）
    for (const row of newChildRows) {
      const childText = row.querySelector('.todo-child-row').value.trim();
      const childDone = row.querySelector('.todo-child-done').checked;
      if (childText) {
        await fetch('/api/todo/' + editLineNo + '/children', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ text: childText, done: childDone })
        });
      }
    }
  } else {
    // 添加新任务
    const body = { text };
    if (dueDate) body.due_date = dueDate;
    const r = await fetch('/api/todo', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
    if (!r.ok) { const b = await r.json().catch(() => ({})); alert('添加失败：' + (b.error || r.statusText)); return; }
    const result = await r.json();
    const parentLineNo = result.line_no;

    // 新增子项（含勾选状态）
    const childRows = Array.from(document.querySelectorAll('#todo-modal-children > div'));
    for (const row of childRows) {
      const childText = row.querySelector('.todo-child-row').value.trim();
      const childDone = row.querySelector('.todo-child-done').checked;
      if (childText) {
        await fetch('/api/todo/' + parentLineNo + '/children', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ text: childText, done: childDone })
        });
      }
    }
  }
  closeTodoAddModal();
  loadTodo();
}

// 递归查找：从树形数据中找指定 line_no 的 item（保留完整 children）
function findTodoItem(lineNo) {
    if (!window._todoTreeData) return null;
    const search = (items) => {
        for (const item of items) {
            if (item.line_no === lineNo) return item;
            if (item.children) {
                const found = search(item.children);
                if (found) return found;
            }
        }
        return null;
    };
    return search(window._todoTreeData);
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
let _dragSrcId = null;
let _dragType = null;
let _dragReloading = null;

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
  document.querySelectorAll(`[data-id][draggable="true"]`).forEach(el => el.style.opacity = '');
  if (!_dragSrcId || _dragSrcId === tgtId) { _dragSrcId = null; return; }
  const container = e.currentTarget.parentElement;
  const idsInNewOrder = Array.from(container.querySelectorAll('[data-id][draggable="true"]')).map(el => el.dataset.id);
  const srcIdx = idsInNewOrder.indexOf(_dragSrcId);
  const tgtIdx = idsInNewOrder.indexOf(tgtId);
  if (srcIdx < 0 || tgtIdx < 0) { _dragSrcId = null; return; }
  idsInNewOrder.splice(srcIdx, 1);
  idsInNewOrder.splice(tgtIdx, 0, _dragSrcId);
  await reorderAndSave(type, idsInNewOrder);
  _dragSrcId = null;
  if (reloadFn) reloadFn();
}

// ===== Todo 过滤 =====
let _todoFilter = { showDone: localStorage.getItem('todoShowDone') === 'true' };

function showTodoFilterMenu() {
    _todoFilter.showDone = !_todoFilter.showDone;
    localStorage.setItem('todoShowDone', _todoFilter.showDone);
    loadTodo();
}

function getTodoFilterLabel() {
    return _todoFilter.showDone ? '显示全部' : '仅未完成';
}

// 初始化 todo
if (typeof window !== "undefined") {
  document.addEventListener("DOMContentLoaded", () => {
    setTimeout(() => { loadTodo(); }, 100);
  });
}

// ===== 鼠标跟随 Tooltip =====
(function() {
    let tooltip = null;
    document.addEventListener('mouseover', function(e) {
        const target = e.target.closest('[data-tooltip]');
        if (!target) return;
        // 跳过 sidebar（api.js 有专门的 sidebar tooltip 系统）
        if (target.closest('.sidebar')) return;
        if (!tooltip) {
            tooltip = document.createElement('div');
            tooltip.className = 'mouse-tooltip';
            document.body.appendChild(tooltip);
        }
        tooltip.textContent = target.dataset.tooltip;
        tooltip.style.display = 'block';
    });
    document.addEventListener('mousemove', function(e) {
        if (!tooltip || tooltip.style.display === 'none') return;
        tooltip.style.left = (e.clientX + 12) + 'px';
        tooltip.style.top = (e.clientY + 12) + 'px';
    });
    document.addEventListener('mouseout', function(e) {
        const target = e.target.closest('[data-tooltip]');
        if (!target) return;
        // 如果鼠标移动到了 tooltip 自身，不要隐藏（避免移动到 tooltip 时闪烁）
        if (e.relatedTarget === tooltip) return;
        if (tooltip) tooltip.style.display = 'none';
    });
})();
