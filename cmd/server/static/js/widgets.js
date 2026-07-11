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
  const cats = await loadLinkCategories();
  let list = sortByOrder(await fetchJSON('/api/web-links'));
  // 按选中分类过滤
  if (_linkActiveCategoryId) {
    list = list.filter(l => l.category_id === _linkActiveCategoryId);
  }
  const grid = document.getElementById('links-grid');
  if (!list || list.length === 0) {
    const msg = _linkActiveCategoryId ? '该分类下暂无链接' : '点击 + 添加你的第一个链接';
    grid.innerHTML = `<div style="color:var(--text-secondary);font-size:12px;text-align:center;padding:20px 0">${esc(msg)}</div>`;
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


async function showLinkModal() {
  document.getElementById('link-name').value = '';
  document.getElementById('link-url').value = '';
  document.getElementById('link-icon').value = '';
  document.getElementById('link-modal').dataset.editId = '';
  await populateLinkCategorySelect();
  // 默认选中当前激活分类
  if (_linkActiveCategoryId) {
    document.getElementById('link-category').value = _linkActiveCategoryId;
  }
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
  const category_id = document.getElementById('link-category').value;
  if (!name || !url) { alert('名称和 URL 必填'); return; }
  if (id) {
    await fetch('/api/web-links/' + id, {method:'PUT', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({name, url, icon_url: icon, category_id})});
  } else {
    await fetch('/api/web-links', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({name, url, icon_url: icon, category_id})});
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
  await populateLinkCategorySelect();
  if (l.category_id) {
    document.getElementById('link-category').value = l.category_id;
  }
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
  const cats = await loadDirCategories();
  let list = sortByOrder(await fetchJSON('/api/dir-shortcuts'));
  if (_dirActiveCategoryId) {
    list = list.filter(d => d.category_id === _dirActiveCategoryId);
  }
  const el = document.getElementById('dir-list');
  if (!list || list.length === 0) {
    const msg = _dirActiveCategoryId ? '该分类下暂无目录' : '';
    el.innerHTML = `<div class="dir-item" onclick="showDirModal()" style="font-style:italic;color:var(--text-secondary)">
      <span class="dir-icon">📂</span>
      <span class="dir-text">
        <span class="dir-name">${msg || '+ 添加目录'}</span>
        <span class="dir-path">${msg ? '' : '点击添加'}</span>
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

async function showDirModal() {
  document.getElementById('dir-name').value = '';
  document.getElementById('dir-type').value = 'local';
  document.getElementById('dir-path').value = '';
  document.getElementById('dir-remote-host').value = '';
  document.getElementById('dir-remote-user').value = '';
  document.getElementById('dir-remote-path').value = '';
  document.getElementById('dir-auth-method').value = 'password';
  document.getElementById('dir-remote-password').value = '';
  document.getElementById('dir-key-path').value = '';
  await populateDirCategorySelect();
  if (_dirActiveCategoryId) {
    document.getElementById('dir-category').value = _dirActiveCategoryId;
  }
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
function toggleDirPassword(btn) {
  const input = document.getElementById('dir-remote-password');
  if (input.type === 'password') {
    input.type = 'text';
    btn.textContent = '🔒';
  } else {
    input.type = 'password';
    btn.textContent = '👁';
  }
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
  document.getElementById('dir-remote-user').value = d.remote_user || '';
  document.getElementById('dir-remote-path').value = d.remote_path || '';
  document.getElementById('dir-auth-method').value = d.auth_method || 'password';
  document.getElementById('dir-remote-password').value = d.remote_password || '';
  document.getElementById('dir-key-path').value = d.key_path || '';
  await populateDirCategorySelect();
  if (d.category_id) {
    document.getElementById('dir-category').value = d.category_id;
  }
  document.getElementById('dir-modal').dataset.editId = id;
  document.getElementById('dir-modal-title').textContent = '编辑目录';
  document.getElementById('dir-submit-btn').textContent = '保存';
  document.getElementById('dir-modal').classList.remove('hidden');
  onDirTypeChange();
  onAuthMethodChange();
}

async function submitDir() {
  const id = document.getElementById('dir-modal').dataset.editId;
  const name = document.getElementById('dir-name').value.trim();
  const type = document.getElementById('dir-type').value;
  const path = document.getElementById('dir-path').value.trim();
  const remote_host = document.getElementById('dir-remote-host').value.trim();
  const remote_user = document.getElementById('dir-remote-user').value.trim();
  const remote_path = document.getElementById('dir-remote-path').value.trim();
  const auth_method = document.getElementById('dir-auth-method').value;
  const remote_password = document.getElementById('dir-remote-password').value;
  const key_path = document.getElementById('dir-key-path').value.trim();
  const category_id = document.getElementById('dir-category').value;
  if (!name) { alert('名称必填'); return; }
  const payload = {name, type, path, remote_host, remote_user, remote_path, auth_method, remote_password, key_path, category_id};
  if (id) {
    await fetch('/api/dir-shortcuts/' + id, {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify(payload)});
  } else {
    await fetch('/api/dir-shortcuts', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(payload)});
  }
  closeDirModal();
  loadDirs();
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
// ===== 待办（支持增删 + 勾选 + 归档） =====
// 展开状态：Set of parent lineNo，localStorage 持久化
let _todoExpandedSet = new Set(JSON.parse(localStorage.getItem('todoExpandedSet') || '[]'));
function saveTodoExpandedSet() {
    localStorage.setItem('todoExpandedSet', JSON.stringify([..._todoExpandedSet]));
}
// 归档区显示状态
let _todoShowArchived = false;

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

// 按 due_date 排序（跨月份统一排序）：过期优先，其次按日期顺序
function sortByDueDate(items) {
    const today = new Date().toISOString().split('T')[0];
    return [...items].sort((a, b) => {
        // 跳过子项，子项跟随父项排序
        if (a._depth > 0 && b._depth > 0) return 0;
        if (a._depth > 0) return -1;
        if (b._depth > 0) return 1;
        const aOverdue = a.due_date && !a.done && a.due_date < today;
        const bOverdue = b.due_date && !b.done && b.due_date < today;
        if (aOverdue && !bOverdue) return -1;
        if (!aOverdue && bOverdue) return 1;
        if (a.due_date && b.due_date) return a.due_date.localeCompare(b.due_date);
        if (a.due_date && !b.due_date) return -1;
        if (!a.due_date && b.due_date) return 1;
        return 0;
    });
}

// 对树结构按 due_date 排序（顶级项按 due_date 排序，子项保持顺序跟随父项）
function sortTreeWithDueDate(items) {
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
            return Object.assign({}, item, { children: sortTreeWithDueDate(item.children) });
        }
        return item;
    });
}

// 扁平化树：打平为单层列表，附加 _depth、_child_count、_parent_line_no，删除 children
// 子项继承父项的 created 和 archived，确保按月份分组时子项跟父项在一起
function flattenItems(items, depth, parentInfo) {
    if (depth === undefined) depth = 0;
    parentInfo = parentInfo || { parentLineNo: undefined, created: '', archived: '' };
    let result = [];
    for (const item of items) {
        const childCount = item.children ? item.children.length : 0;
        // 子项没有日期时继承父项的
        const created = item.created || parentInfo.created || '';
        const archived = item.archived || parentInfo.archived || '';
        const flat = Object.assign({}, item, {
            _depth: depth,
            _child_count: childCount,
            _parent_line_no: parentInfo.parentLineNo !== undefined ? parentInfo.parentLineNo : null,
            created: created,
            archived: archived,
        });
        delete flat.children;
        result.push(flat);
        if (item.children) {
            result = result.concat(flattenItems(item.children, depth + 1, {
                parentLineNo: item.line_no,
                created: created,
                archived: archived,
            }));
        }
    }
    return result;
}

// 获取月份标签
function getMonthLabel(dateStr) {
    if (!dateStr) return '未标注日期';
    const d = new Date(dateStr);
    if (isNaN(d.getTime())) return '未标注日期';
    return d.getFullYear() + '年' + String(d.getMonth() + 1).padStart(2, '0') + '月';
}

// 获取月份排序键（用于分组排序）
function getMonthKey(dateStr) {
    if (!dateStr) return '9999-99';
    return dateStr.substring(0, 7); // YYYY-MM
}

// 按月份分组
function groupByMonth(items, dateField) {
    const groups = {};
    for (const item of items) {
        const dateStr = item[dateField] || '';
        const key = getMonthKey(dateStr);
        if (!groups[key]) groups[key] = [];
        groups[key].push(item);
    }
    // 按月份排序
    const sortedKeys = Object.keys(groups).sort();
    const result = [];
    for (const key of sortedKeys) {
        result.push({ key, items: groups[key] });
    }
    return result;
}

// 渲染单个活跃 todo item
function renderTodoItem(i) {
  const today = new Date().toISOString().split('T')[0];
  const isOverdue = i.due_date && !i.done && i.due_date < today;
  const dueLabel = i.due_date ? i.due_date.slice(5) : '';
  const childCount = i._child_count || 0;
  const hasChildren = childCount > 0;
  const isExpanded = _todoExpandedSet.has(i.line_no);
  // 顶级已完成项显示归档按钮
  const isTopLevelDone = i.done && (i._depth === 0);

  let extraHtml = '';
  const childClickAttr = hasChildren ? `onclick="event.stopPropagation(); toggleTodoItem(${i.line_no})"` : '';
  if (hasChildren) {
    extraHtml += `<span class="todo-child-count" ${childClickAttr} title="${childCount} 个子项">${childCount}</span>`;
  }
  if (i.due_date) {
    extraHtml += `<span class="todo-due ${isOverdue ? 'overdue' : ''}" ${childClickAttr} title="${esc(i.due_date)}">📅 ${dueLabel}</span>`;
  }
  if (isTopLevelDone) {
    extraHtml += `<span class="todo-archive" onclick="event.stopPropagation(); archiveTodoItem(${i.line_no})" title="归档">📦</span>`;
  }

  const indent = i._depth * 20;
  const onclickAttr = hasChildren ? `onclick="event.stopPropagation(); toggleTodoItem(${i.line_no})"` : '';
  let html = `<div class="todo-item ${i.done?'done':''} ${isOverdue?'overdue-row':''} ${hasChildren?'has-children':''}" ${onclickAttr} ${hasChildren ? `data-line-no="${i.line_no}" data-parent-line-no="${i._parent_line_no != null ? i._parent_line_no : ''}" style="padding-left:${indent}px"` : `style="padding-left:${indent}px" data-parent-line-no="${i._parent_line_no != null ? i._parent_line_no : ''}"}`}>
  <span class="todo-indent"></span>
  <input type="checkbox" ${i.done?'checked':''} onchange="event.stopPropagation(); toggleTodo(${i.line_no}, this.checked)">
  <span class="todo-text" ${hasChildren ? `onclick="event.stopPropagation(); toggleTodoItem(${i.line_no})"` : ''}>${esc(i.text)}</span>
  ${extraHtml}
  <span class="todo-edit" onclick="event.stopPropagation(); openTodoEditModal(${i.line_no})" title="编辑">✎</span>
  <span class="todo-del" onclick="event.stopPropagation(); deleteTodoItem(${i.line_no})" title="删除">×</span>
</div>`;
  return html;
}

// 渲染单个归档 item（简化版，无展开/收起，无编辑）
// 仅顶级项显示删除按钮
function renderArchivedItem(i) {
  const dueLabel = i.due_date ? i.due_date.slice(5) : '';
  const indent = (i._depth || 0) * 20;
  let extraHtml = '';
  if (i.due_date) {
    extraHtml += `<span class="todo-due" title="${esc(i.due_date)}">📅 ${dueLabel}</span>`;
  }
  const isTopLevel = !i._parent_line_no;
  const actionsHtml = isTopLevel
    ? `<span class="todo-del" onclick="event.stopPropagation(); deleteTodoItem(${i.line_no})" title="删除">×</span>`
    : '';
  let html = `<div class="todo-item done" style="padding-left:${indent}px" data-line-no="${i.line_no}" data-parent-line-no="${i._parent_line_no != null ? i._parent_line_no : ''}">
  <span class="todo-indent"></span>
  <input type="checkbox" checked disabled>
  <span class="todo-text">${esc(i.text)}</span>
  ${extraHtml}
  ${actionsHtml}
</div>`;
  return html;
}

// 渲染月份分组（前端不显示月份标签，md 文件里已有 ### YYYY年MM月 分组）
function renderMonthGroup(label, items, isArchived) {
  let html = '<div class="todo-month-group">';
  for (const item of items) {
    if (isArchived) {
      html += renderArchivedItem(item);
    } else {
      html += renderTodoItem(item);
    }
  }
  html += '</div>';
  return html;
}

// 点击父任务文字，展开/收起子项（递归）
function toggleTodoItem(lineNo) {
    const itemEl = document.querySelector('.todo-item[data-line-no="' + lineNo + '"]');
    if (!itemEl) return;
    const childEls = document.querySelectorAll('.todo-item[data-parent-line-no="' + lineNo + '"]');
    if (childEls.length === 0) return;
    const isExpanded = _todoExpandedSet.has(lineNo);
    if (isExpanded) {
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
        _todoExpandedSet.clear();
        const btn = document.getElementById('todo-expand-btn');
        if (btn) { btn.textContent = '▶'; btn.title = '展开所有子项'; }
    } else {
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
    loadTodo();
}

// 切换归档区显示
function toggleTodoArchived() {
    _todoShowArchived = !_todoShowArchived;
    const btn = document.getElementById('todo-archive-btn');
    if (btn) {
        btn.style.color = _todoShowArchived ? 'var(--primary)' : 'var(--text-secondary)';
    }
    loadTodo();
}

async function loadTodo() {
  const data = await fetchJSON('/api/todo');
  const el = document.getElementById('todo-list');
  if (!data.path) { el.innerHTML = '<div style="color:var(--text-secondary);font-size:12px">未配置 todo.md 路径，点击"设置"</div>'; return; }

  // 保存原始树数据
  window._todoTreeData = data.items || [];
  window._todoArchivedData = data.archived_items || [];

  // 归档显示状态：优先使用后端配置，否则用本地状态
  if (data.show_archived !== undefined) {
    _todoShowArchived = data.show_archived;
  }

  // 过滤和排序活跃项：先过滤，再排序（树结构内排序），再扁平
  const filtered = filterTree(window._todoTreeData);
  const sorted = sortTreeWithDueDate(filtered);
  window._todoItems = flattenItems(sorted);

  // 更新按钮状态
  const filterBtn = document.getElementById('todo-filter-btn');
  if (filterBtn) {
    filterBtn.textContent = _todoFilter.showDone ? '◉' : '☐';
    filterBtn.title = '点击切换到: ' + (_todoFilter.showDone ? '仅未完成' : '显示全部');
  }
  const expandBtn = document.getElementById('todo-expand-btn');
  if (expandBtn) {
    expandBtn.textContent = _todoExpandedSet.size > 0 ? '▼' : '▶';
    expandBtn.title = _todoExpandedSet.size > 0 ? '收起所有子项' : '展开所有子项';
  }
  const archiveBtn = document.getElementById('todo-archive-btn');
  if (archiveBtn) {
    archiveBtn.style.color = _todoShowArchived ? 'var(--primary)' : 'var(--text-secondary)';
  }

  if (sorted.length === 0) {
    const hint = _todoFilter.showDone ? '' : '（全部已完成）';
    el.innerHTML = '<div style="color:var(--text-secondary);font-size:12px">' + esc(data.path) + ' 无 todo 项' + hint + '</div>';
  } else {
    // 按月份分组渲染（使用已扁平的 window._todoItems）
    const groups = groupByMonth(window._todoItems, 'created');
    let html = '';
    for (const group of groups) {
      const label = getMonthLabel(group.items[0].created);
      html += renderMonthGroup(label, group.items, false);
    }
    el.innerHTML = html;
  }

  // 处理初始隐藏（子项未展开）
  el.querySelectorAll('.todo-item[data-parent-line-no]').forEach(el => {
    const pln = el.dataset.parentLineNo;
    if (pln && pln !== '' && !_todoExpandedSet.has(parseInt(pln))) {
        el.style.display = 'none';
    }
  });

  // 渲染归档区
  const archivedEl = document.getElementById('todo-archived-section');
  if (archivedEl) {
    if (_todoShowArchived && window._todoArchivedData.length > 0) {
      archivedEl.style.display = 'block';
      const archivedFlat = flattenItems(window._todoArchivedData);
      const groups = groupByMonth(archivedFlat, 'archived');
      let html = '';
      for (const group of groups) {
        const label = getMonthLabel(group.items[0].archived);
        html += renderMonthGroup(label, group.items, true);
      }
      archivedEl.innerHTML = html;
    } else {
      archivedEl.style.display = 'none';
      archivedEl.innerHTML = '';
    }
  }
}

function toggleTodo(lineNo, done) {
  fetch('/api/todo/' + lineNo, {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({done})})
    .then(() => loadTodo());
}

async function archiveTodoItem(lineNo) {
  const r = await fetch('/api/todo/' + lineNo + '/archive', {method:'PUT'});
  if (!r.ok) { const b = await r.json().catch(() => ({})); alert('归档失败：' + (b.error || r.statusText)); return; }
  loadTodo();
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
function clearTodoDueDate() {
  document.getElementById('todo-modal-due').value = '';
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
    const editBody = { text };
    if (dueDate) {
      editBody.due_date = dueDate;
    } else {
      editBody.clear_due_date = true;
    }
    await fetch('/api/todo/' + editLineNo + '/edit', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(editBody)
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
  const [pathData, configData] = await Promise.all([
    fetchJSON('/api/todo/path'),
    fetchJSON('/api/config')
  ]);
  document.getElementById('todo-path-input').value = pathData.path || '';
  document.getElementById('todo-show-archived-cb').checked = !!configData.todo_show_archived;
  document.getElementById('todo-path-modal').classList.remove('hidden');
  setTimeout(() => document.getElementById('todo-path-input').focus(), 50);
}
function closeTodoPathModal() { document.getElementById('todo-path-modal').classList.add('hidden'); }
function submitTodoPath() {
  const path = document.getElementById('todo-path-input').value.trim();
  if (!path) { alert('路径必填'); return; }
  const showArchived = document.getElementById('todo-show-archived-cb').checked;
  Promise.all([
    fetch('/api/todo/path', {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({path})}),
    fetch('/api/config', {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({todo_show_archived: showArchived})})
  ]).then(() => {
    closeTodoPathModal();
    loadTodo();
  });
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
    initCategoryState();
    setTimeout(() => { loadLinks(); loadDirs(); loadTodo(); }, 100);
  });
}



// ===== 分类管理（链接 + 目录） =====

// 全局状态：当前选中的分类 ID（空字符串表示"全部"）
let _linkActiveCategoryId = '';
let _dirActiveCategoryId = '';

// ===== 链接分类 =====

async function loadLinkCategories() {
  try {
    const [cats, links] = await Promise.all([
      fetchJSON('/api/link-categories'),
      fetchJSON('/api/web-links'),
    ]);
    const allCats = cats || [];
    const allLinks = links || [];

    // 统计每个分类的链接数量
    const countByCat = {};
    for (const l of allLinks) {
      countByCat[l.category_id || ''] = (countByCat[l.category_id || ''] || 0) + 1;
    }

    // 过滤：count=0 的非默认分类不显示（默认分类始终保留）
    const visibleCats = allCats.filter(c => c.is_default || (countByCat[c.id] || 0) > 0);

    const bar = document.getElementById('link-categories-bar');
    if (!bar) return visibleCats;
    // 保存当前选择
    const activeId = _linkActiveCategoryId;
    // 第一个 chip = "全部"
    let html = `<span class="category-chip ${activeId === '' ? 'active' : ''}" data-cat-id="" onclick="selectLinkCategory('')">全部</span>`;
    for (const c of visibleCats) {
      const icon = c.icon || '';
      html += `<span class="category-chip ${activeId === c.id ? 'active' : ''}" data-cat-id="${esc(c.id)}" onclick="selectLinkCategory('${esc(c.id)}')">${esc(icon + (icon ? ' ' : '') + c.name)}</span>`;
    }
    // 管理按钮（如果有多个可见分类才显示）
    if (visibleCats.length > 1) {
      html += `<span class="category-chip cat-manage" onclick="showLinkCategoryModal()" title="管理分类">⚙</span>`;
    }
    bar.innerHTML = html;
    return visibleCats;
  } catch (e) {
    console.error('loadLinkCategories:', e);
    return [];
  }
}

function selectLinkCategory(catId) {
  _linkActiveCategoryId = catId;
  localStorage.setItem('sf-link-active-cat', catId);
  loadLinks();
}

async function loadLinks() {
  const cats = await loadLinkCategories();
  let list = sortByOrder(await fetchJSON('/api/web-links'));
  // 按选中分类过滤
  if (_linkActiveCategoryId) {
    list = list.filter(l => l.category_id === _linkActiveCategoryId);
  }
  const grid = document.getElementById('links-grid');
  if (!list || list.length === 0) {
    const msg = _linkActiveCategoryId ? '该分类下暂无链接' : '点击 + 添加你的第一个链接';
    grid.innerHTML = `<div style="color:var(--text-secondary);font-size:12px;text-align:center;padding:20px 0">${esc(msg)}</div>`;
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

// 修改现有函数：填充分类下拉框
async function populateLinkCategorySelect() {
  const sel = document.getElementById('link-category');
  if (!sel) return;
  const cats = await fetchJSON('/api/link-categories') || [];
  sel.innerHTML = cats.map(c => `<option value="${esc(c.id)}">${esc((c.icon || '') + ' ' + c.name)}</option>`).join('');
}

async function showLinkModal() {
  document.getElementById('link-name').value = '';
  document.getElementById('link-url').value = '';
  document.getElementById('link-icon').value = '';
  document.getElementById('link-modal').dataset.editId = '';
  await populateLinkCategorySelect();
  // 默认选中当前激活分类
  if (_linkActiveCategoryId) {
    document.getElementById('link-category').value = _linkActiveCategoryId;
  }
  const titleEl = document.querySelector('#link-modal h2');
  if (titleEl) titleEl.textContent = '添加链接';
  document.getElementById('link-modal').classList.remove('hidden');
  setTimeout(() => document.getElementById('link-name').focus(), 50);
}

async function submitLink() {
  const id = document.getElementById('link-modal').dataset.editId;
  const name = document.getElementById('link-name').value.trim();
  const url = document.getElementById('link-url').value.trim();
  const icon = document.getElementById('link-icon').value.trim();
  const category_id = document.getElementById('link-category').value;
  if (!name || !url) { alert('名称和 URL 必填'); return; }
  if (id) {
    await fetch('/api/web-links/' + id, {method:'PUT', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({name, url, icon_url: icon, category_id})});
  } else {
    await fetch('/api/web-links', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({name, url, icon_url: icon, category_id})});
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
  await populateLinkCategorySelect();
  if (l.category_id) {
    document.getElementById('link-category').value = l.category_id;
  }
  const titleEl = document.querySelector('#link-modal h2');
  if (titleEl) titleEl.textContent = '编辑链接';
  document.getElementById('link-modal').dataset.editId = id;
  document.getElementById('link-modal').classList.remove('hidden');
}

// ===== 链接分类管理 Modal =====
function showLinkCategoryModal() {
  document.getElementById('link-category-modal').classList.remove('hidden');
  refreshLinkCategoryList();
}
function closeLinkCategoryModal() {
  document.getElementById('link-category-modal').classList.add('hidden');
}
async function refreshLinkCategoryList() {
  const list = await fetchJSON('/api/link-categories') || [];
  const container = document.getElementById('link-category-list');
  if (list.length === 0) {
    container.innerHTML = '<div style="color:var(--text-secondary);font-size:12px;padding:8px">暂无分类</div>';
    return;
  }
  // 拖动重排：整行 draggable；data-id 携带分类 id；onXxx 回调走通用 catRowDrag* 助手
  container.innerHTML = list.map(c => `
    <div class="cat-row" draggable="true" data-id="${esc(c.id)}"
         ondragstart="catRowDragStart(event, 'link')"
         ondragover="catRowDragOver(event)"
         ondragleave="catRowDragLeave(event)"
         ondrop="catRowDrop(event, 'link', refreshLinkCategoryList)"
         style="display:flex;align-items:center;gap:8px;padding:6px;border-bottom:1px solid var(--border);cursor:move">
      <span class="drag-handle" title="拖动重排"></span>
      <span>${esc(c.icon || '')}</span>
      <span style="flex:1">${esc(c.name)}</span>
      ${c.is_default ? '<span style="color:var(--text-secondary);font-size:11px">默认</span>' :
        `<button class="btn btn-small" onclick="event.stopPropagation();deleteLinkCategory('${esc(c.id)}')">删除</button>`}
    </div>
  `).join('');
}
async function addLinkCategory() {
  const name = document.getElementById('new-link-category-name').value.trim();
  const icon = document.getElementById('new-link-category-icon').value.trim();
  if (!name) { alert('分类名必填'); return; }
  await fetch('/api/link-categories', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({name, icon})});
  document.getElementById('new-link-category-name').value = '';
  document.getElementById('new-link-category-icon').value = '';
  await refreshLinkCategoryList();
  loadLinks();
}
async function deleteLinkCategory(id) {
  if (!confirm('删除该分类？其下链接将归入默认分类')) return;
  await fetch('/api/link-categories/' + id, {method:'DELETE'});
  await refreshLinkCategoryList();
  if (_linkActiveCategoryId === id) {
    _linkActiveCategoryId = '';
    localStorage.setItem('sf-link-active-cat', '');
  }
  loadLinks();
}

// ===== 目录分类 =====

async function loadDirCategories() {
  try {
    const [cats, dirs] = await Promise.all([
      fetchJSON('/api/dir-categories'),
      fetchJSON('/api/dir-shortcuts'),
    ]);
    const allCats = cats || [];
    const allDirs = dirs || [];

    // 统计每个分类的目录数量
    const countByCat = {};
    for (const d of allDirs) {
      countByCat[d.category_id || ''] = (countByCat[d.category_id || ''] || 0) + 1;
    }

    // 过滤：count=0 的非默认分类不显示（默认分类始终保留）
    const visibleCats = allCats.filter(c => c.is_default || (countByCat[c.id] || 0) > 0);

    const bar = document.getElementById('dir-categories-bar');
    if (!bar) return visibleCats;
    const activeId = _dirActiveCategoryId;
    let html = `<span class="category-chip ${activeId === '' ? 'active' : ''}" data-cat-id="" onclick="selectDirCategory('')">全部</span>`;
    for (const c of visibleCats) {
      const icon = c.icon || '';
      html += `<span class="category-chip ${activeId === c.id ? 'active' : ''}" data-cat-id="${esc(c.id)}" onclick="selectDirCategory('${esc(c.id)}')">${esc(icon + (icon ? ' ' : '') + c.name)}</span>`;
    }
    if (visibleCats.length > 1) {
      html += `<span class="category-chip cat-manage" onclick="showDirCategoryModal()" title="管理分类">⚙</span>`;
    }
    bar.innerHTML = html;
    return visibleCats;
  } catch (e) {
    console.error('loadDirCategories:', e);
    return [];
  }
}

function selectDirCategory(catId) {
  _dirActiveCategoryId = catId;
  localStorage.setItem('sf-dir-active-cat', catId);
  loadDirs();
}

async function loadDirs() {
  const cats = await loadDirCategories();
  let list = sortByOrder(await fetchJSON('/api/dir-shortcuts'));
  if (_dirActiveCategoryId) {
    list = list.filter(d => d.category_id === _dirActiveCategoryId);
  }
  const el = document.getElementById('dir-list');
  if (!list || list.length === 0) {
    const msg = _dirActiveCategoryId ? '该分类下暂无目录' : '';
    el.innerHTML = `<div class="dir-item" onclick="showDirModal()" style="font-style:italic;color:var(--text-secondary)">
      <span class="dir-icon">📂</span>
      <span class="dir-text">
        <span class="dir-name">${msg || '+ 添加目录'}</span>
        <span class="dir-path">${msg ? '' : '点击添加'}</span>
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

async function populateDirCategorySelect() {
  const sel = document.getElementById('dir-category');
  if (!sel) return;
  const cats = await fetchJSON('/api/dir-categories') || [];
  sel.innerHTML = cats.map(c => `<option value="${esc(c.id)}">${esc((c.icon || '') + ' ' + c.name)}</option>`).join('');
}

async function showDirModal() {
  document.getElementById('dir-name').value = '';
  document.getElementById('dir-type').value = 'local';
  document.getElementById('dir-path').value = '';
  document.getElementById('dir-remote-host').value = '';
  document.getElementById('dir-remote-user').value = '';
  document.getElementById('dir-remote-path').value = '';
  document.getElementById('dir-auth-method').value = 'password';
  document.getElementById('dir-remote-password').value = '';
  document.getElementById('dir-key-path').value = '';
  await populateDirCategorySelect();
  if (_dirActiveCategoryId) {
    document.getElementById('dir-category').value = _dirActiveCategoryId;
  }
  document.getElementById('dir-modal').dataset.editId = '';
  document.getElementById('dir-modal').classList.remove('hidden');
  document.getElementById('dir-modal-title').textContent = '添加目录';
  document.getElementById('dir-submit-btn').textContent = '添加';
  onDirTypeChange();
  onAuthMethodChange();
  setTimeout(() => document.getElementById('dir-name').focus(), 50);
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
  await populateDirCategorySelect();
  if (d.category_id) {
    document.getElementById('dir-category').value = d.category_id;
  }
  document.getElementById('dir-modal').dataset.editId = id;
  document.getElementById('dir-modal-title').textContent = '编辑目录';
  document.getElementById('dir-submit-btn').textContent = '保存';
  document.getElementById('dir-modal').classList.remove('hidden');
  onDirTypeChange();
  onAuthMethodChange();
}

async function submitDir() {
  const id = document.getElementById('dir-modal').dataset.editId;
  const name = document.getElementById('dir-name').value.trim();
  const type = document.getElementById('dir-type').value;
  const path = document.getElementById('dir-path').value.trim();
  const remote_host = document.getElementById('dir-remote-host').value.trim();
  const remote_user = document.getElementById('dir-remote-user').value.trim();
  const remote_path = document.getElementById('dir-remote-path').value.trim();
  const auth_method = document.getElementById('dir-auth-method').value;
  const remote_password = document.getElementById('dir-remote-password').value;
  const key_path = document.getElementById('dir-key-path').value.trim();
  const category_id = document.getElementById('dir-category').value;
  if (!name) { alert('名称必填'); return; }
  const payload = {name, type, path, remote_host, remote_user, remote_path, auth_method, remote_password, key_path, category_id};
  if (id) {
    await fetch('/api/dir-shortcuts/' + id, {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify(payload)});
  } else {
    await fetch('/api/dir-shortcuts', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(payload)});
  }
  closeDirModal();
  loadDirs();
}

// ===== 目录分类管理 Modal =====
function showDirCategoryModal() {
  document.getElementById('dir-category-modal').classList.remove('hidden');
  refreshDirCategoryList();
}
function closeDirCategoryModal() {
  document.getElementById('dir-category-modal').classList.add('hidden');
}
async function refreshDirCategoryList() {
  const list = await fetchJSON('/api/dir-categories') || [];
  const container = document.getElementById('dir-category-list');
  if (list.length === 0) {
    container.innerHTML = '<div style="color:var(--text-secondary);font-size:12px;padding:8px">暂无分类</div>';
    return;
  }
  container.innerHTML = list.map(c => `
    <div class="cat-row" draggable="true" data-id="${esc(c.id)}"
         ondragstart="catRowDragStart(event, 'dir')"
         ondragover="catRowDragOver(event)"
         ondragleave="catRowDragLeave(event)"
         ondrop="catRowDrop(event, 'dir', refreshDirCategoryList)"
         style="display:flex;align-items:center;gap:8px;padding:6px;border-bottom:1px solid var(--border);cursor:move">
      <span class="drag-handle" title="拖动重排"></span>
      <span>${esc(c.icon || '')}</span>
      <span style="flex:1">${esc(c.name)}</span>
      ${c.is_default ? '<span style="color:var(--text-secondary);font-size:11px">默认</span>' :
        `<button class="btn btn-small" onclick="event.stopPropagation();deleteDirCategory('${esc(c.id)}')">删除</button>`}
    </div>
  `).join('');
}
async function addDirCategory() {
  const name = document.getElementById('new-dir-category-name').value.trim();
  const icon = document.getElementById('new-dir-category-icon').value.trim();
  if (!name) { alert('分类名必填'); return; }
  await fetch('/api/dir-categories', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({name, icon})});
  document.getElementById('new-dir-category-name').value = '';
  document.getElementById('new-dir-category-icon').value = '';
  await refreshDirCategoryList();
  loadDirs();
}
async function deleteDirCategory(id) {
  if (!confirm('删除该分类？其下目录将归入默认分类')) return;
  await fetch('/api/dir-categories/' + id, {method:'DELETE'});
  await refreshDirCategoryList();
  if (_dirActiveCategoryId === id) {
    _dirActiveCategoryId = '';
    localStorage.setItem('sf-dir-active-cat', '');
  }
  loadDirs();
}

// ===== 初始化：从 localStorage 恢复选中分类 =====
function initCategoryState() {
  _linkActiveCategoryId = localStorage.getItem('sf-link-active-cat') || '';
  _dirActiveCategoryId = localStorage.getItem('sf-dir-active-cat') || '';
}


// ===== 分类排序（拖动重排） =====
async function saveCategoryOrder(type, ids) {
  // type: 'link' | 'dir'
  const endpoint = type === 'link' ? '/api/link-categories' : '/api/dir-categories';
  for (let i = 0; i < ids.length; i++) {
    await fetch(endpoint + '/' + ids[i], {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({sort_order: i + 1})
    });
  }
  if (type === 'link') loadLinks(); else loadDirs();
}

// ===== 分类 Modal 拖动重排（cat-row） =====
let _catDragSrcId = null;
let _catDragType = null;

function catRowDragStart(e, type) {
  _catDragSrcId = e.currentTarget.dataset.id;
  _catDragType = type;
  e.dataTransfer.effectAllowed = 'move';
  e.dataTransfer.setData('text/plain', _catDragSrcId || '');
  e.currentTarget.style.opacity = '0.4';
}

function catRowDragOver(e) {
  if (!_catDragSrcId) return;
  e.preventDefault();
  e.dataTransfer.dropEffect = 'move';
  e.currentTarget.style.borderTop = '2px solid var(--primary)';
}

function catRowDragLeave(e) {
  e.currentTarget.style.borderTop = '';
}

async function catRowDrop(e, type, reloadFn) {
  e.preventDefault();
  e.currentTarget.style.borderTop = '';
  // 清理本 modal 内所有 cat-row 的拖拽样式
  const modal = e.currentTarget.closest('.modal');
  if (modal) {
    modal.querySelectorAll('.cat-row').forEach(el => { el.style.opacity = ''; });
  }
  const tgtId = e.currentTarget.dataset.id;
  if (!_catDragSrcId || _catDragSrcId === tgtId || type !== _catDragType) {
    _catDragSrcId = null;
    _catDragType = null;
    return;
  }
  // 收集当前 modal 内所有 cat-row 的 id 顺序
  const idsInNewOrder = modal
    ? Array.from(modal.querySelectorAll('.cat-row[data-id]')).map(el => el.dataset.id)
    : [];
  const srcIdx = idsInNewOrder.indexOf(_catDragSrcId);
  const tgtIdx = idsInNewOrder.indexOf(tgtId);
  if (srcIdx < 0 || tgtIdx < 0) {
    _catDragSrcId = null;
    _catDragType = null;
    return;
  }
  idsInNewOrder.splice(srcIdx, 1);
  idsInNewOrder.splice(tgtIdx, 0, _catDragSrcId);
  _catDragSrcId = null;
  _catDragType = null;
  await saveCategoryOrder(type, idsInNewOrder);
  if (reloadFn) reloadFn();
}
