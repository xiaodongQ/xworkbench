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
      <span class="todo-menu-wrap">
        <button class="todo-menu-trigger" type="button" onclick="event.stopPropagation(); toggleLinkMenu(event, '${l.id}')" title="更多操作" aria-label="更多操作">⋮</button>
        <div class="todo-menu closed">
          <button class="todo-menu-item" onclick="event.stopPropagation(); editLink('${l.id}'); closeAllLinkMenus();">
            <span class="todo-menu-icon">✎</span><span>编辑</span>
          </button>
          <button class="todo-menu-item danger" onclick="event.stopPropagation(); deleteLink('${l.id}'); closeAllLinkMenus();">
            <span class="todo-menu-icon">×</span><span>删除</span>
          </button>
        </div>
      </span>
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
  // icon_url 可能是 URL 或 emoji，回显到 select（select 只支持预定义 emoji）
  const iconVal = l.icon_url || '';
  const iconSelect = document.getElementById('link-icon');
  if (iconSelect.querySelector(`option[value="${iconVal}"]`)) {
    iconSelect.value = iconVal;
  } else {
    iconSelect.value = ''; // URL 或未知格式，显示为首字母
  }
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
  loadDirSettingsCategories();
}

async function loadDirSettingsCategories() {
  const cats = await fetchJSON('/api/dir-categories') || [];
  const list = document.getElementById('dir-settings-category-list');
  if (!list) return;

  if (cats.length === 0) {
    list.innerHTML = '<div style="color:var(--text-secondary);font-size:12px;padding:8px">暂无分类</div>';
    return;
  }

  list.innerHTML = sortByOrder(cats).map(c => `
    <div style="display:flex;align-items:center;gap:8px;padding:4px 0;border-bottom:1px solid var(--border);font-size:12px">
      <span>${esc(c.icon || '')} ${esc(c.name)}</span>
      <span style="flex:1"></span>
      ${c.is_default ? '<span style="color:var(--text-secondary)">默认</span>' :
        `<button class="btn btn-small" onclick="void deleteDirCategory('${esc(c.id)}').then(() => loadDirSettingsCategories())">删除</button>`}
    </div>
  `).join('');
}

async function addDirCategoryFromSettings() {
  const name = document.getElementById('new-dir-settings-category-name').value.trim();
  const icon = document.getElementById('new-dir-settings-category-icon').value.trim();
  if (!name) { alert('分类名必填'); return; }
  await fetch('/api/dir-categories', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({name, icon})
  });
  document.getElementById('new-dir-settings-category-name').value = '';
  document.getElementById('new-dir-settings-category-icon').value = '';
  // 同时刷新：① settings panel 列表 ② 首页 chips
  await loadDirSettingsCategories();
  await refreshDirCategoryList();
  loadDirs();
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

// 渲染单个活跃 todo item（操作合并到一个 ⋮ 按钮下的 dropdown）
function renderTodoItem(i) {
  const today = new Date().toISOString().split('T')[0];
  const isOverdue = i.due_date && !i.done && i.due_date < today;
  const dueLabel = i.due_date ? i.due_date.slice(5) : '';
  const childCount = i._child_count || 0;
  const hasChildren = childCount > 0;
  const isExpanded = _todoExpandedSet.has(i.line_no);
  // 顶级已完成项显示归档菜单项
  const isTopLevelDone = i.done && (i._depth === 0);

  let extraHtml = '';
  const childClickAttr = hasChildren ? `onclick="event.stopPropagation(); toggleTodoItem(${i.line_no})"` : '';
  if (hasChildren) {
    extraHtml += `<span class="todo-child-count" ${childClickAttr} title="${childCount} 个子项">${childCount}</span>`;
  }
  if (i.due_date) {
    extraHtml += `<span class="todo-due ${isOverdue ? 'overdue' : ''}" ${childClickAttr} title="${esc(i.due_date)}">📅 ${dueLabel}</span>`;
  }

  // 操作菜单：编辑 / 归档（条件）/ 删除
  const menuItems = `
    <button class="todo-menu-item" onclick="event.stopPropagation(); openTodoEditModal(${i.line_no}); closeAllTodoMenus();">
      <span class="todo-menu-icon">✎</span><span>编辑</span>
    </button>
    ${isTopLevelDone ? `
    <button class="todo-menu-item" onclick="event.stopPropagation(); archiveTodoItem(${i.line_no}); closeAllTodoMenus();">
      <span class="todo-menu-icon">📦</span><span>归档</span>
    </button>` : ''}
    <button class="todo-menu-item danger" onclick="event.stopPropagation(); deleteTodoItem(${i.line_no}); closeAllTodoMenus();">
      <span class="todo-menu-icon">×</span><span>删除</span>
    </button>`;
  const menuHtml = `<span class="todo-menu-wrap">
    <button class="todo-menu-trigger" type="button" onclick="event.stopPropagation(); toggleTodoMenu(event, ${i.line_no})" title="更多操作" aria-label="更多操作">⋮</button>
    <div class="todo-menu closed" data-line-no="${i.line_no}">${menuItems}</div>
  </span>`;

  const indent = i._depth * 20;
  const onclickAttr = hasChildren ? `onclick="event.stopPropagation(); toggleTodoItem(${i.line_no})"` : '';
  let html = `<div class="todo-item ${i.done?'done':''} ${isOverdue?'overdue-row':''} ${hasChildren?'has-children':''}" ${onclickAttr} ${hasChildren ? `data-line-no="${i.line_no}" data-parent-line-no="${i._parent_line_no != null ? i._parent_line_no : ''}" style="padding-left:${indent}px"` : `style="padding-left:${indent}px" data-parent-line-no="${i._parent_line_no != null ? i._parent_line_no : ''}"}`}>
  <span class="todo-indent"></span>
  <input type="checkbox" ${i.done?'checked':''} onchange="event.stopPropagation(); toggleTodo(${i.line_no}, this.checked)">
  <span class="todo-text" ${hasChildren ? `onclick="event.stopPropagation(); toggleTodoItem(${i.line_no})"` : ''}>${esc(i.text)}</span>
  ${extraHtml}
  ${menuHtml}
</div>`;
  return html;
}

// 渲染单个归档 item（简化版，无展开/收起，无编辑）
// 仅顶级项显示 ⋮ 操作按钮（仅删除）
function renderArchivedItem(i) {
  const dueLabel = i.due_date ? i.due_date.slice(5) : '';
  const indent = (i._depth || 0) * 20;
  let extraHtml = '';
  if (i.due_date) {
    extraHtml += `<span class="todo-due" title="${esc(i.due_date)}">📅 ${dueLabel}</span>`;
  }
  const isTopLevel = !i._parent_line_no;
  const menuHtml = isTopLevel
    ? `<span class="todo-menu-wrap">
         <button class="todo-menu-trigger" type="button" onclick="event.stopPropagation(); toggleTodoMenu(event, ${i.line_no})" title="更多操作" aria-label="更多操作">⋮</button>
         <div class="todo-menu closed" data-line-no="${i.line_no}">
           <button class="todo-menu-item danger" onclick="event.stopPropagation(); deleteTodoItem(${i.line_no}); closeAllTodoMenus();">
             <span class="todo-menu-icon">×</span><span>删除</span>
           </button>
         </div>
       </span>`
    : '';
  let html = `<div class="todo-item done" style="padding-left:${indent}px" data-line-no="${i.line_no}" data-parent-line-no="${i._parent_line_no != null ? i._parent_line_no : ''}">
  <span class="todo-indent"></span>
  <input type="checkbox" checked disabled>
  <span class="todo-text">${esc(i.text)}</span>
  ${extraHtml}
  ${menuHtml}
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

  // 归档显示状态：首次加载时用后端配置，后续 toggle 不再覆盖
  if (!window._todoShowArchivedInited && data.show_archived !== undefined) {
    _todoShowArchived = data.show_archived;
    window._todoShowArchivedInited = true;
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

  // 渲染归档区（仅展示顶级项，不展示子项）
  const archivedEl = document.getElementById('todo-archived-section');
  if (archivedEl) {
    if (_todoShowArchived && window._todoArchivedData.length > 0) {
      archivedEl.style.display = 'block';
      // 只取顶级归档项（无 _parent_line_no）
      const topLevelArchived = window._todoArchivedData.filter(i => !i._parent_line_no);
      const groups = groupByMonth(topLevelArchived, 'archived');
      let html = '';
      for (const group of groups) {
        const label = getMonthLabel(group.items[0].archived);
        html += renderMonthGroup(label, group.items, true);
      }
      archivedEl.innerHTML = '<div style="font-size:11px;color:var(--text-secondary);margin-bottom:4px;font-weight:500">📦 归档区</div>' + html;
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
  if (!confirm('确认归档该任务及其子项？')) return;
  const r = await fetch('/api/todo/' + lineNo + '/archive', {method:'PUT'});
  if (!r.ok) { const b = await r.json().catch(() => ({})); alert('归档失败：' + (b.error || r.statusText)); return; }
  loadTodo();
}

// 单 ⋮ 按钮下的 dropdown 菜单：每次只展开一个，外部点击 / Esc / 滚动都关。
let _openTodoMenu = null;        // 当前打开的 .todo-menu 节点
let _todoMenuDocListenersOn = false;

function toggleTodoMenu(event, lineNo) {
  event.stopPropagation();
  const trigger = event.currentTarget;
  const menu = trigger.parentElement.querySelector('.todo-menu');
  if (!menu) return;

  // 点击同一个 trigger → 关闭
  if (_openTodoMenu === menu && !menu.classList.contains('closed')) {
    closeAllTodoMenus();
    return;
  }
  closeAllTodoMenus();
  positionTodoMenu(menu, trigger);
  menu.classList.remove('closed');
  trigger.classList.add('open');
  _openTodoMenu = menu;
  installTodoMenuDocListeners();
}

function closeAllTodoMenus() {
  if (_openTodoMenu) {
    _openTodoMenu.classList.add('closed');
    const trigger = _openTodoMenu.parentElement.querySelector('.todo-menu-trigger');
    if (trigger) trigger.classList.remove('open');
    _openTodoMenu = null;
  }
}

// 用 fixed 定位以避开 todo-container (overflow-y:auto) 的裁剪。
function positionTodoMenu(menu, trigger) {
  // 先临时可见以测量尺寸（.closed = display:none 时 getBoundingClientRect 不可信）
  const hadClosed = menu.classList.contains('closed');
  if (hadClosed) menu.classList.remove('closed');
  menu.style.visibility = 'hidden';
  const mRect = menu.getBoundingClientRect();
  const tRect = trigger.getBoundingClientRect();
  let top = tRect.bottom + 4;
  let left = tRect.right - mRect.width;
  // 溢出底部则向上展开
  if (top + mRect.height > window.innerHeight - 8) {
    top = tRect.top - mRect.height - 4;
  }
  if (left < 8) left = 8;
  if (left + mRect.width > window.innerWidth - 8) {
    left = window.innerWidth - mRect.width - 8;
  }
  menu.style.position = 'fixed';
  menu.style.top = top + 'px';
  menu.style.left = left + 'px';
  menu.style.visibility = '';
  if (hadClosed) menu.classList.add('closed');
}

function installTodoMenuDocListeners() {
  if (_todoMenuDocListenersOn) return;
  _todoMenuDocListenersOn = true;
  document.addEventListener('click', (e) => {
    if (!_openTodoMenu) return;
    if (e.target.closest('.todo-menu')) return;
    if (e.target.closest('.todo-menu-trigger')) return;
    closeAllTodoMenus();
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && _openTodoMenu) closeAllTodoMenus();
  });
  const container = document.getElementById('todo-container');
  if (container) container.addEventListener('scroll', () => {
    if (_openTodoMenu) closeAllTodoMenus();
  });
}

// loadTodo 重新渲染时关闭任何 open menu（DOM 被替换，引用会失效）
const _origLoadTodo = loadTodo;
loadTodo = async function () {
  closeAllTodoMenus();
  return _origLoadTodo.apply(this, arguments);
};

// 链接的 ⋮ dropdown 菜单（共享 .todo-menu-trigger / .todo-menu 样式）
let _openLinkMenu = null;
let _linkMenuDocListenersOn = false;

function toggleLinkMenu(event, linkId) {
  event.stopPropagation();
  const trigger = event.currentTarget;
  const menu = trigger.parentElement.querySelector('.todo-menu');
  if (!menu) return;
  if (_openLinkMenu === menu && !menu.classList.contains('closed')) {
    closeAllLinkMenus();
    return;
  }
  closeAllLinkMenus();
  positionLinkMenu(menu, trigger);
  menu.classList.remove('closed');
  trigger.classList.add('open');
  _openLinkMenu = menu;
  installLinkMenuDocListeners();
}

function closeAllLinkMenus() {
  if (_openLinkMenu) {
    _openLinkMenu.classList.add('closed');
    const trigger = _openLinkMenu.parentElement.querySelector('.todo-menu-trigger');
    if (trigger) trigger.classList.remove('open');
    _openLinkMenu = null;
  }
}

function positionLinkMenu(menu, trigger) {
  const hadClosed = menu.classList.contains('closed');
  if (hadClosed) menu.classList.remove('closed');
  menu.style.visibility = 'hidden';
  const mRect = menu.getBoundingClientRect();
  const tRect = trigger.getBoundingClientRect();
  let top = tRect.bottom + 4;
  let left = tRect.right - mRect.width;
  if (top + mRect.height > window.innerHeight - 8) {
    top = tRect.top - mRect.height - 4;
  }
  if (left < 8) left = 8;
  if (left + mRect.width > window.innerWidth - 8) {
    left = window.innerWidth - mRect.width - 8;
  }
  menu.style.position = 'fixed';
  menu.style.top = top + 'px';
  menu.style.left = left + 'px';
  menu.style.visibility = '';
  if (hadClosed) menu.classList.add('closed');
}

function installLinkMenuDocListeners() {
  if (_linkMenuDocListenersOn) return;
  _linkMenuDocListenersOn = true;
  document.addEventListener('click', (e) => {
    if (!_openLinkMenu) return;
    if (e.target.closest('.todo-menu')) return;
    if (e.target.closest('.todo-menu-trigger')) return;
    closeAllLinkMenus();
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && _openLinkMenu) closeAllLinkMenus();
  });
  const container = document.getElementById('links-container') || document.getElementById('web-links');
  if (container) container.addEventListener('scroll', () => {
    if (_openLinkMenu) closeAllLinkMenus();
  });
}

// loadLinks 重新渲染时关闭 open menu
const _origLoadLinks = loadLinks;
loadLinks = async function () {
  closeAllLinkMenus();
  return _origLoadLinks.apply(this, arguments);
};


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
  document.getElementById('todo-modal-children-group').style.display = '';
  document.getElementById('todo-add-modal').classList.remove('hidden');
  setTimeout(() => document.getElementById('todo-modal-title-input').focus(), 50);
}
function openTodoEditModal(lineNo) {
  const item = findTodoItem(lineNo);
  if (!item) return;
  // 计算深度（递归遍历树）
  const depth = calcItemDepth(window._todoTreeData, lineNo, 0);
  const isMaxDepth = depth >= 3;

  document.getElementById('todo-modal-title').textContent = '编辑任务';
  document.getElementById('todo-edit-line-no').value = lineNo;
  document.getElementById('todo-modal-title-input').value = item.text;
  document.getElementById('todo-modal-due').value = item.due_date || '';
  document.getElementById('todo-modal-submit').textContent = '保存';
  // 渲染现有子项
  const childrenGroup = document.getElementById('todo-modal-children-group');
  const container = document.getElementById('todo-modal-children');
  container.innerHTML = '';
  if (isMaxDepth) {
    childrenGroup.style.display = 'none';
    container.innerHTML = '<div style="font-size:11px;color:var(--text-secondary)">已达到最大嵌套层级（3层），无法添加子项</div>';
  } else {
    childrenGroup.style.display = '';
    if (item.children) {
      item.children.forEach(c => addTodoChildRow(c.text, c.line_no, c.done));
    }
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

// 递归计算项的深度（0 = 顶级）
function calcItemDepth(items, targetLineNo, depth) {
    for (const item of items) {
        if (item.line_no === targetLineNo) return depth;
        if (item.children) {
            const d = calcItemDepth(item.children, targetLineNo, depth + 1);
            if (d >= 0) return d;
        }
    }
    return -1;
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
  fetch('/api/config', {method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({todo_md_path: path, todo_show_archived: showArchived})})
    .then(() => {
      closeTodoPathModal();
      _todoShowArchived = showArchived;
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

// 链接分类展开状态
const _linkExpandedCats = JSON.parse(localStorage.getItem('sf-link-expanded') || '{}');

function toggleLinkCategory(catId) {
  _linkExpandedCats[catId] = _linkExpandedCats[catId] === false ? true : false;
  localStorage.setItem('sf-link-expanded', JSON.stringify(_linkExpandedCats));
  loadLinks();
}

function isLinkCategoryExpanded(catId) {
  return _linkExpandedCats[catId] !== false;
}

// ===== 链接分组展示 =====

async function loadLinks() {
  const [cats, links] = await Promise.all([
    fetchJSON('/api/link-categories'),
    fetchJSON('/api/web-links'),
  ]);

  const allCats = sortByOrder(cats || []);
  const allLinks = links || [];

  // 按 category_id 分组
  const byCat = {};
  for (const l of allLinks) {
    const catId = l.category_id || '';
    if (!byCat[catId]) byCat[catId] = [];
    byCat[catId].push(l);
  }

  const grid = document.getElementById('links-grid');
  if (!grid) return;

  let html = '';
  for (const cat of allCats) {
    const items = byCat[cat.id] || [];
    const isExpanded = isLinkCategoryExpanded(cat.id);
    const arrow = isExpanded ? '▼' : '▶';

    html += `<div class="link-category-group">`;
    html += `<div class="link-category-header" onclick="toggleLinkCategory('${esc(cat.id)}')">`;
    html += `<span style="color:var(--text-secondary)">${arrow}</span>`;
    html += `<span>${esc(cat.icon || '')} ${esc(cat.name)}</span>`;
    html += `<span style="color:var(--text-secondary);margin-left:auto">${items.length}</span>`;
    html += `</div>`;
    html += `<div class="link-category-items${isExpanded ? '' : ' hidden'}" data-cat-id="${esc(cat.id)}">`;
    if (items.length === 0) {
      html += `<div style="padding:8px 16px;color:var(--text-secondary);font-size:12px">暂无链接</div>`;
    } else {
	      for (const l of sortByOrder(items)) {
	        const initial = (l.name || '?')[0].toUpperCase();
	        const iconUrl = l.icon_url || '';
	        let icon;
	        if (iconUrl && !iconUrl.startsWith('http') && !iconUrl.startsWith('file://') && !iconUrl.startsWith('/')) {
	          icon = `<span style="font-size:14px">${esc(iconUrl)}</span>`;
	        } else if (iconUrl) {
	          icon = `<img src="${esc(iconUrl)}" onerror="this.outerHTML='${initial}'">`;
	        } else {
	          icon = initial;
	        }
        html += `<div class="link-row" draggable="true" data-id="${l.id}" data-cat-id="${esc(cat.id)}"
            ondragstart="linkCatDragStart(event, '${esc(cat.id)}')" ondragover="linkCatDragOver(event)" ondrop="linkCatDrop(event, '${esc(cat.id)}')" ondragleave="linkCatDragLeave(event)">
          <span class="drag-handle" title="拖动排序">⋮⋮</span>
          <div class="link-icon" onclick="openLink('${esc(l.url)}')">${icon}</div>
          <div class="link-text" onclick="openLink('${esc(l.url)}')" title="${esc(l.url)}">
            <div class="link-name">${esc(l.name)}</div>
            <div class="link-url">${esc(l.url)}</div>
          </div>
          <span class="todo-menu-wrap">
            <button class="todo-menu-trigger" type="button" onclick="event.stopPropagation(); toggleLinkMenu(event, '${l.id}')" title="更多操作" aria-label="更多操作">⋮</button>
            <div class="todo-menu closed">
              <button class="todo-menu-item" onclick="event.stopPropagation(); editLink('${l.id}'); closeAllLinkMenus();">
                <span class="todo-menu-icon">✎</span><span>编辑</span>
              </button>
              <button class="todo-menu-item danger" onclick="event.stopPropagation(); deleteLink('${l.id}'); closeAllLinkMenus();">
                <span class="todo-menu-icon">×</span><span>删除</span>
              </button>
            </div>
          </span>
        </div>`;
      }
    }
    html += `</div></div>`;
  }

  if (html === '') {
    html = `<div style="color:var(--text-secondary);font-size:12px;text-align:center;padding:20px 0">点击 + 添加你的第一个链接</div>`;
  }

  grid.innerHTML = html;
}

// ===== 链接同分类拖动 =====

let _linkDragSrcId = null;
let _linkDragCatId = null;

function linkCatDragStart(e, catId) {
  _linkDragSrcId = e.currentTarget.dataset.id;
  _linkDragCatId = catId;
  e.dataTransfer.effectAllowed = 'move';
  e.currentTarget.style.opacity = '0.4';
}

function linkCatDragOver(e) {
  if (!_linkDragSrcId) return;
  const tgtCatId = e.currentTarget.closest('.link-category-items')?.dataset.catId;
  if (tgtCatId !== _linkDragCatId) return;
  e.preventDefault();
  e.dataTransfer.dropEffect = 'move';
  e.currentTarget.style.borderTop = '2px solid var(--primary)';
}

function linkCatDragLeave(e) {
  e.currentTarget.style.borderTop = '';
}

async function linkCatDrop(e, catId) {
  e.preventDefault();
  e.currentTarget.style.borderTop = '';

  // 只允许同分类内拖动
  if (catId !== _linkDragCatId) {
    _linkDragSrcId = null;
    _linkDragCatId = null;
    return;
  }

  const tgtId = e.currentTarget.dataset.id;
  document.querySelectorAll('.link-row[draggable="true"]').forEach(el => el.style.opacity = '');

  if (!_linkDragSrcId || _linkDragSrcId === tgtId) {
    _linkDragSrcId = null;
    _linkDragCatId = null;
    return;
  }

  const container = e.currentTarget.closest('.link-category-items');
  const rows = Array.from(container.querySelectorAll('.link-row[data-id]'));
  const idsInNewOrder = rows.map(el => el.dataset.id);

  const srcIdx = idsInNewOrder.indexOf(_linkDragSrcId);
  const tgtIdx = idsInNewOrder.indexOf(tgtId);

  if (srcIdx < 0 || tgtIdx < 0) {
    _linkDragSrcId = null;
    _linkDragCatId = null;
    return;
  }

  idsInNewOrder.splice(srcIdx, 1);
  idsInNewOrder.splice(tgtIdx, 0, _linkDragSrcId);

  // 获取所有链接的 category_id，避免排序时丢失
  const allLinks = await fetchJSON('/api/web-links');
  const catById = {};
  for (const l of allLinks) { catById[l.id] = l.category_id || ''; }

  // 并行 PUT 全部 sort_order 和 category_id
  const promises = idsInNewOrder.map((id, idx) =>
    fetch(`/api/web-links/${id}`, {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({sort_order: idx + 1, category_id: catById[id] || ''}),
    })
  );
  await Promise.all(promises);

  _linkDragSrcId = null;
  _linkDragCatId = null;
  loadLinks();
}

// ===== 链接设置 Modal =====

function showLinkSettingsModal() {
  document.getElementById('link-settings-modal').classList.remove('hidden');
  loadLinkSettingsCategories();
}

function closeLinkSettingsModal() {
  document.getElementById('link-settings-modal').classList.add('hidden');
}

async function loadLinkSettingsCategories() {
  const cats = await fetchJSON('/api/link-categories') || [];
  const list = document.getElementById('link-settings-category-list');
  if (!list) return;

  if (cats.length === 0) {
    list.innerHTML = '<div style="color:var(--text-secondary);font-size:12px;padding:8px">暂无分类</div>';
    return;
  }

  list.innerHTML = sortByOrder(cats).map(c => `
    <div style="display:flex;align-items:center;gap:8px;padding:4px 0;border-bottom:1px solid var(--border);font-size:12px">
      <span>${esc(c.icon || '')} ${esc(c.name)}</span>
      <span style="flex:1"></span>
      ${c.is_default ? '<span style="color:var(--text-secondary)">默认</span>' :
        `<button class="btn btn-small" onclick="void deleteLinkCategory('${esc(c.id)}').then(() => loadLinkSettingsCategories())">删除</button>`}
    </div>
  `).join('');
}

async function addLinkCategoryFromSettings() {
  const name = document.getElementById('new-link-settings-category-name').value.trim();
  const icon = document.getElementById('new-link-settings-category-icon').value.trim();
  if (!name) { alert('分类名必填'); return; }
  await fetch('/api/link-categories', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({name, icon})
  });
  document.getElementById('new-link-settings-category-name').value = '';
  document.getElementById('new-link-settings-category-icon').value = '';
  // 同时刷新：① settings panel 列表 ② 首页 chips（之前漏了 → 添加/删除后首页不立即显示新分类）
  await loadLinkSettingsCategories();
  await refreshLinkCategoryList();
  loadLinks();
}

// 保留旧函数兼容：loadLinkCategories 返回分类列表（不再渲染 chips）
async function loadLinkCategories() {
  const cats = await fetchJSON('/api/link-categories') || [];
  return sortByOrder(cats);
}

function selectLinkCategory(catId) {
  _linkActiveCategoryId = catId;
  localStorage.setItem('sf-link-active-cat', catId);
  loadLinks();
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
  // icon_url 可能是 URL 或 emoji，回显到 select（select 只支持预定义 emoji）
  const iconVal = l.icon_url || '';
  const iconSelect = document.getElementById('link-icon');
  if (iconSelect.querySelector(`option[value="${iconVal}"]`)) {
    iconSelect.value = iconVal;
  } else {
    iconSelect.value = ''; // URL 或未知格式，显示为首字母
  }
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

// ===== 目录分类展开状态 =====

const _dirExpandedCats = JSON.parse(localStorage.getItem('sf-dir-expanded') || '{}');

function toggleDirCategory(catId) {
  _dirExpandedCats[catId] = _dirExpandedCats[catId] === false ? true : false;
  localStorage.setItem('sf-dir-expanded', JSON.stringify(_dirExpandedCats));
  loadDirs();
}

function isDirCategoryExpanded(catId) {
  return _dirExpandedCats[catId] !== false;
}

// ===== 目录分组展示 =====

async function loadDirs() {
  const [cats, dirs] = await Promise.all([
    fetchJSON('/api/dir-categories'),
    fetchJSON('/api/dir-shortcuts'),
  ]);

  const allCats = sortByOrder(cats || []);
  const allDirs = dirs || [];

  // 按 category_id 分组
  const byCat = {};
  for (const d of allDirs) {
    const catId = d.category_id || '';
    if (!byCat[catId]) byCat[catId] = [];
    byCat[catId].push(d);
  }

  const el = document.getElementById('dir-list');
  if (!el) return;

  let html = '';
  for (const cat of allCats) {
    const items = byCat[cat.id] || [];
    const isExpanded = isDirCategoryExpanded(cat.id);
    const arrow = isExpanded ? '▼' : '▶';

    html += `<div class="dir-category-group">`;
    html += `<div class="dir-category-header" onclick="toggleDirCategory('${esc(cat.id)}')">`;
    html += `<span style="color:var(--text-secondary)">${arrow}</span>`;
    html += `<span>${esc(cat.icon || '')} ${esc(cat.name)}</span>`;
    html += `<span style="color:var(--text-secondary);margin-left:auto">${items.length}</span>`;
    html += `</div>`;
    html += `<div class="dir-category-items${isExpanded ? '' : ' hidden'}" data-cat-id="${esc(cat.id)}">`;
    if (items.length === 0) {
      html += `<div style="padding:8px 16px;color:var(--text-secondary);font-size:12px">暂无目录</div>`;
    } else {
      for (const d of sortByOrder(items)) {
        html += `<div class="dir-item${d.type === 'remote' ? ' dir-remote' : ''}" draggable="true" data-id="${d.id}" data-cat-id="${esc(cat.id)}"
            ondragstart="dirCatDragStart(event, '${esc(cat.id)}')" ondragover="dirCatDragOver(event)" ondrop="dirCatDrop(event, '${esc(cat.id)}')" ondragleave="dirCatDragLeave(event)">
          <span class="drag-handle" title="拖动排序"></span>
          <span class="dir-icon" onclick="openDir('${d.id}')">${d.type === 'remote' ? '🌐' : '📁'}</span>
          <span class="dir-term" onclick="event.stopPropagation();openDirTerminal('${d.id}')" title="打开外部终端">⬢</span>
          <span class="dir-text" onclick="openDir('${d.id}')">
            <span class="dir-name">${esc(d.name)}</span>
            <span class="dir-path" title="${esc(d.type === 'remote' ? d.remote_user + '@' + d.remote_host : d.path)}">${esc(d.type === 'remote' ? d.remote_user + '@' + d.remote_host : d.path)}</span>
          </span>
          <span class="dir-edit" onclick="event.stopPropagation();editDir('${d.id}')" title="编辑">✎</span>
          <span class="dir-del" onclick="event.stopPropagation();deleteDir('${d.id}')" title="删除">×</span>
        </div>`;
      }
    }
    html += `</div></div>`;
  }

  if (html === '') {
    html = `<div class="dir-item" onclick="showDirModal()" style="font-style:italic;color:var(--text-secondary)">
      <span class="dir-icon">📂</span>
      <span class="dir-text">
        <span class="dir-name">+ 添加目录</span>
        <span class="dir-path">点击添加</span>
      </span>
    </div>`;
  }

  el.innerHTML = html;
}

// ===== 目录同分类拖动 =====

let _dirDragSrcId = null;
let _dirDragCatId = null;

function dirCatDragStart(e, catId) {
  _dirDragSrcId = e.currentTarget.dataset.id;
  _dirDragCatId = catId;
  e.dataTransfer.effectAllowed = 'move';
  e.currentTarget.style.opacity = '0.4';
}

function dirCatDragOver(e) {
  if (!_dirDragSrcId) return;
  const tgtCatId = e.currentTarget.closest('.dir-category-items')?.dataset.catId;
  if (tgtCatId !== _dirDragCatId) return;
  e.preventDefault();
  e.dataTransfer.dropEffect = 'move';
  e.currentTarget.style.borderTop = '2px solid var(--primary)';
}

function dirCatDragLeave(e) {
  e.currentTarget.style.borderTop = '';
}

async function dirCatDrop(e, catId) {
  e.preventDefault();
  e.currentTarget.style.borderTop = '';

  if (catId !== _dirDragCatId) {
    _dirDragSrcId = null;
    _dirDragCatId = null;
    return;
  }

  const tgtId = e.currentTarget.dataset.id;
  document.querySelectorAll('.dir-item[draggable="true"]').forEach(el => el.style.opacity = '');

  if (!_dirDragSrcId || _dirDragSrcId === tgtId) {
    _dirDragSrcId = null;
    _dirDragCatId = null;
    return;
  }

  const container = e.currentTarget.closest('.dir-category-items');
  const rows = Array.from(container.querySelectorAll('.dir-item[data-id]'));
  const idsInNewOrder = rows.map(el => el.dataset.id);

  const srcIdx = idsInNewOrder.indexOf(_dirDragSrcId);
  const tgtIdx = idsInNewOrder.indexOf(tgtId);

  if (srcIdx < 0 || tgtIdx < 0) {
    _dirDragSrcId = null;
    _dirDragCatId = null;
    return;
  }

  idsInNewOrder.splice(srcIdx, 1);
  idsInNewOrder.splice(tgtIdx, 0, _dirDragSrcId);

  // 获取所有目录的完整数据，避免排序时丢失字段
  const allDirs = await fetchJSON('/api/dir-shortcuts');
  const dataById = {};
  for (const d of allDirs) { dataById[d.id] = d; }

  // 并行 PUT 全部 sort_order 和所有字段
  const promises = idsInNewOrder.map((id, idx) => {
    const d = dataById[id] || {};
    return fetch(`/api/dir-shortcuts/${id}`, {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        sort_order: idx + 1,
        category_id: d.category_id || '',
        name: d.name || '',
        type: d.type || 'local',
        path: d.path || '',
        remote_host: d.remote_host || '',
        remote_user: d.remote_user || '',
        remote_path: d.remote_path || '',
        remote_password: d.remote_password || '',
        auth_method: d.auth_method || 'password',
        key_path: d.key_path || '',
      }),
    });
  });
  await Promise.all(promises);

  _dirDragSrcId = null;
  _dirDragCatId = null;
  loadDirs();
}

// 保留旧函数兼容：loadDirCategories 返回分类列表
async function loadDirCategories() {
  const cats = await fetchJSON('/api/dir-categories') || [];
  return sortByOrder(cats);
}

function selectDirCategory(catId) {
  _dirActiveCategoryId = catId;
  localStorage.setItem('sf-dir-active-cat', catId);
  loadDirs();
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

// ===== 分类 chip 右键菜单 =====
// type: 'link' | 'dir'; catId: 目标分类 id（'' = 全部，是默认分类的特殊处理）
async function onCategoryContextMenu(e, type, catId) {
  e.preventDefault();
  e.stopPropagation();
  hideCategoryContextMenu();

  // "全部" chip(id 为空)是默认分类的代理：不允许重命名/删除/合并
  if (catId === '') return;

  // 取该分类的元数据（图义、名称、是否默认）
  const apiPath = type === 'link' ? '/api/link-categories' : '/api/dir-categories';
  const cats = await fetchJSON(apiPath) || [];
  const cat = cats.find(c => c.id === catId);
  if (!cat) return;
  const isDefault = !!cat.is_default;

  // 取该分类下的条目数（用于合并提示）
  const itemsApi = type === 'link' ? '/api/web-links' : '/api/dir-shortcuts';
  const items = await fetchJSON(itemsApi) || [];
  const count = items.filter(it => it.category_id === catId).length;

  // 可合并的候选分类列表（排除自身与默认）
  const candidates = cats.filter(c => c.id !== catId);

  const menu = document.createElement('div');
  menu.className = 'cat-context-menu';
  menu.dataset.type = type;
  menu.dataset.catId = catId;

  // 名称
  const itemName = document.createElement('div');
  itemName.className = 'menu-item';
  itemName.textContent = isDefault ? '默认分类（不可重命名）' : '✎ 重命名';
  if (!isDefault) {
    itemName.onclick = async () => { hideCategoryContextMenu(); await renameCategory(type, cat); };
  } else {
    itemName.style.color = 'var(--text-secondary)';
    itemName.style.cursor = 'default';
  }
  menu.appendChild(itemName);

  // 合并到...（需要至少有一个非自身候选）
  const mergeWrap = document.createElement('div');
  mergeWrap.className = 'menu-item menu-submenu';
  mergeWrap.innerHTML = `<span>↪ 合并到… (${count})</span><span class="menu-arrow">▸</span>`;
  if (candidates.length === 0) {
    const empty = document.createElement('div');
    empty.className = 'menu-empty';
    empty.textContent = '无可选目标';
    const sub = document.createElement('div');
    sub.className = 'menu-submenu-list';
    sub.appendChild(empty);
    mergeWrap.appendChild(sub);
    mergeWrap.style.color = 'var(--text-secondary)';
  } else {
    const sub = document.createElement('div');
    sub.className = 'menu-submenu-list';
    for (const cand of candidates) {
      const it = document.createElement('div');
      it.className = 'menu-item';
      it.title = cand.name;
      it.textContent = (cand.icon ? cand.icon + ' ' : '') + cand.name;
      it.onclick = async (ev) => { ev.stopPropagation(); hideCategoryContextMenu(); await mergeCategoryInto(type, cat, cand, count); };
      sub.appendChild(it);
    }
    mergeWrap.appendChild(sub);
  }
  menu.appendChild(mergeWrap);

  // 分隔符
  const div = document.createElement('div');
  div.className = 'menu-divider';
  menu.appendChild(div);

  // 删除
  const del = document.createElement('div');
  del.className = 'menu-item danger';
  if (isDefault) {
    del.textContent = '默认分类（不可删除）';
    del.style.cursor = 'default';
  } else {
    del.textContent = `🗑 删除（${count} 条将归入默认）`;
    del.onclick = async () => { hideCategoryContextMenu(); await deleteCategoryWithConfirm(type, cat, count); };
  }
  menu.appendChild(del);

  // 定位 + 边界反转
  positionContextMenu(menu, e.clientX, e.clientY);
  document.body.appendChild(menu);
  // 点击外部 / Esc 关闭
  setTimeout(() => {
    document.addEventListener('mousedown', hideCategoryContextMenu, { once: true });
    document.addEventListener('keydown', escHideCategoryMenu, { once: true });
    document.addEventListener('scroll', hideCategoryContextMenu, { once: true, capture: true });
  }, 0);
}

function positionContextMenu(menu, x, y) {
  // 先 append 后立即 measure 才能拿到尺寸
  menu.style.left = '0px';
  menu.style.top = '0px';
  const rect = menu.getBoundingClientRect();
  const vw = window.innerWidth, vh = window.innerHeight;
  let left = x, top = y;
  if (left + rect.width > vw - 8) left = Math.max(8, vw - rect.width - 8);
  if (top + rect.height > vh - 8) top = Math.max(8, vh - rect.height - 8);
  menu.style.left = left + 'px';
  menu.style.top = top + 'px';
}

function escHideCategoryMenu(e) {
  if (e.key === 'Escape') hideCategoryContextMenu();
}

function hideCategoryContextMenu() {
  document.querySelectorAll('.cat-context-menu').forEach(el => el.remove());
}

// ===== 重命名分类 =====
async function renameCategory(type, cat) {
  const apiPath = type === 'link' ? '/api/link-categories' : '/api/dir-categories';
  const newName = prompt(`重命名分类 “${cat.name}” 为：`, cat.name || '');
  if (newName === null) return;
  const trimmed = newName.trim();
  if (!trimmed) { alert('分类名不能为空'); return; }
  if (trimmed === cat.name) return;
  const payload = { name: trimmed, icon: cat.icon || '', sort_order: cat.sort_order || 0 };
  const resp = await fetch(`${apiPath}/${encodeURIComponent(cat.id)}`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!resp.ok) {
    const t = await resp.text().catch(() => '');
    alert('重命名失败：' + (t || resp.statusText));
    return;
  }
  if (type === 'link') loadLinks(); else loadDirs();
}

// ===== 删除分类 =====
async function deleteCategoryWithConfirm(type, cat, count) {
  const msg = count > 0
    ? `确定删除分类 “${cat.name}”？该分类下的 ${count} 个条目将归入默认分类。`
    : `确定删除空分类 “${cat.name}”？`;
  if (!confirm(msg)) return;
  const apiPath = type === 'link' ? '/api/link-categories' : '/api/dir-categories';
  const resp = await fetch(`${apiPath}/${encodeURIComponent(cat.id)}`, { method: 'DELETE' });
  if (!resp.ok) {
    const t = await resp.text().catch(() => '');
    alert('删除失败：' + (t || resp.statusText));
    return;
  }
  // 如果当前激活分类被刪，重置为“全部”
  if (type === 'link' && _linkActiveCategoryId === cat.id) {
    _linkActiveCategoryId = '';
    localStorage.setItem('sf-link-active-cat', '');
  } else if (type === 'dir' && _dirActiveCategoryId === cat.id) {
    _dirActiveCategoryId = '';
    localStorage.setItem('sf-dir-active-cat', '');
  }
  if (type === 'link') loadLinks(); else loadDirs();
}

// ===== 合并分类到另一个 =====
async function mergeCategoryInto(type, srcCat, tgtCat, count) {
  if (srcCat.id === tgtCat.id) return;
  const msg = count > 0
    ? `将 “${srcCat.name}” 下的 ${count} 个条目全部迁移到 “${tgtCat.name}”，然后删除 “${srcCat.name}”？该操作不可逆。`
    : `将空分类 “${srcCat.name}” 合并到 “${tgtCat.name}” 并删除 “${srcCat.name}”？`;
  if (!confirm(msg)) return;
  const apiPath = type === 'link' ? '/api/link-categories' : '/api/dir-categories';
  const resp = await fetch(`${apiPath}/${encodeURIComponent(srcCat.id)}/merge`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ target_id: tgtCat.id }),
  });
  if (!resp.ok) {
    const t = await resp.text().catch(() => '');
    alert('合并失败：' + (t || resp.statusText));
    return;
  }
  // 如果是当前激活分类，重置为“全部”
  if (type === 'link' && _linkActiveCategoryId === srcCat.id) {
    _linkActiveCategoryId = '';
    localStorage.setItem('sf-link-active-cat', '');
  } else if (type === 'dir' && _dirActiveCategoryId === srcCat.id) {
    _dirActiveCategoryId = '';
    localStorage.setItem('sf-dir-active-cat', '');
  }
  if (type === 'link') loadLinks(); else loadDirs();
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
