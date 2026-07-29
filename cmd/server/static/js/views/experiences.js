// Experiences Tab：经验库列表 + 模块搜索 + exp modal（查看/添加/删除）+ 分类管理
// 依赖 api.js

let exps = [];
let expCategories = [];
const _expCatExpanded = JSON.parse(localStorage.getItem('sf-exp-cat-expanded') || '{}');

function isExpCategoryExpanded(catId) {
  return _expCatExpanded[catId] !== false;
}

function toggleExpCategory(catId) {
  _expCatExpanded[catId] = _expCatExpanded[catId] === false ? true : false;
  localStorage.setItem('sf-exp-cat-expanded', JSON.stringify(_expCatExpanded));
  renderExpTable(exps);
}

async function loadExps() {
  const module = document.getElementById('exp-search').value;
  const url = API + '/api/experiences' + (module ? '?module=' + encodeURIComponent(module) : '');
  try {
    const [expsData, catsData] = await Promise.all([
      fetchJSON(url),
      fetchJSON('/api/exp-categories')
    ]);
    exps = expsData;
    expCategories = catsData || [];
    updateExpCategoryFilter();
    renderExpTable(exps);
  } catch(e) { console.error(e); }
}

function updateExpCategoryFilter() {
  const sel = document.getElementById('filter-exp-category');
  if (!sel) return;
  const current = sel.value;
  sel.innerHTML = '<option value="">全部分类</option>' +
    expCategories.map(c => `<option value="${c.id}">${esc((c.icon || '') + ' ' + c.name)}</option>`).join('');
  if (current && sel.querySelector('option[value="' + current + '"]')) {
    sel.value = current;
  }
}

function renderExpTable(list) {
  const el = document.getElementById('exp-list');
  if (!list || list.length === 0) {
    el.innerHTML = '<div class="empty">暂无经验</div>';
    document.getElementById('exp-count').textContent = '0 条经验';
    return;
  }

  // 按分类过滤
  const catFilter = document.getElementById('filter-exp-category').value;
  const filtered = catFilter ? list.filter(e => e.category_id === catFilter) : list;

  if (filtered.length === 0) {
    el.innerHTML = '<div class="empty">暂无经验</div>';
    document.getElementById('exp-count').textContent = '0 条经验';
    return;
  }

  document.getElementById('exp-count').textContent = filtered.length + ' 条经验';

  // 按 category_id 分组
  const byCat = {};
  for (const e of filtered) {
    const catId = e.category_id || 'default-exp-cat';
    if (!byCat[catId]) {
      byCat[catId] = { id: catId, name: '', icon: '', items: [] };
    }
    byCat[catId].items.push(e);
  }

  // 填充分类名称
  for (const catId in byCat) {
    if (catId !== 'default-exp-cat') {
      const cat = expCategories.find(c => c.id === catId);
      if (cat) {
        byCat[catId].name = cat.name;
        byCat[catId].icon = cat.icon || '';
      }
    } else {
      const defaultCat = expCategories.find(c => c.id === 'default-exp-cat');
      byCat[catId].name = defaultCat ? defaultCat.name : '默认';
      byCat[catId].icon = defaultCat ? (defaultCat.icon || '📚') : '📚';
    }
  }

  // 排序：其他分类在前，默认分类在后
  const catOrder = [...expCategories].sort((a, b) => {
    if (a.id === 'default-exp-cat') return 1;
    if (b.id === 'default-exp-cat') return -1;
    return a.sort_order - b.sort_order;
  });

  const sortedCats = Object.values(byCat).sort((a, b) => {
    if (a.id === 'default-exp-cat') return 1;
    if (b.id === 'default-exp-cat') return -1;
    const ai = catOrder.findIndex(c => c.id === a.id);
    const bi = catOrder.findIndex(c => c.id === b.id);
    return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi);
  });

  el.innerHTML = sortedCats.map((cat, catIndex) => {
    const items = cat.items;
    const isExpanded = isExpCategoryExpanded(cat.id);
    const thead = catIndex === 0
      ? `<thead><tr>
          <th class="col-module" style="text-align:left">模块</th>
          <th class="col-kw">关键词</th>
          <th class="col-scene">适用场景</th>
          <th class="col-ops">操作</th>
        </tr></thead>`
      : '';

    return `<div class="task-category-group">
      <table class="exp-table">
        <colgroup>
          <col style="width:160px">
          <col style="width:220px">
          <col style="width:auto">
          <col style="width:100px">
        </colgroup>
        ${thead}
        <tbody>
          <tr class="task-category-header-row"
              onclick="toggleExpCategory('${cat.id}')"
              style="cursor:pointer">
            <td colspan="4" style="padding:6px 12px;font-size:12px">
              <div style="display:flex;align-items:center;gap:6px">
                <span style="font-size:12px;color:var(--text-secondary)">${isExpanded ? '▼' : '▶'}</span>
                <span>${esc((cat.icon || '') + ' ' + cat.name)}</span>
                <span style="margin-left:auto;color:var(--text-secondary)">${items.length}</span>
              </div>
            </td>
          </tr>
        </tbody>
        <tbody class="${isExpanded ? '' : 'hidden'}">
          ${items.map(e => `
            <tr>
              <td class="col-module">
                <span class="edit-icon" onclick="editExp('${e.id}')" title="编辑" style="cursor:pointer;color:var(--text-secondary);font-size:14px;margin-right:4px">✏️</span>
                <span style="font-weight:500">${esc(e.module)}</span>
              </td>
              <td class="col-kw" style="font-size:12px;color:var(--text-secondary)">${esc(e.keywords || '-')}</td>
              <td class="col-scene" style="font-size:12px;color:var(--text-secondary)">${esc(e.scene || '-')}</td>
              <td class="col-ops" style="display:flex;align-items:center;gap:4px;justify-content:flex-start">
                <button class="btn btn-secondary btn-small" style="flex-shrink:0" onclick="viewExp('${e.id}')">查看</button>
                <button class="btn btn-danger btn-small" style="flex-shrink:0" onclick="deleteExp('${e.id}')">删除</button>
              </td>
            </tr>`).join('')}
        </tbody>
      </table>
    </div>`;
  }).join('');
}

function viewExp(id) {
  const e = exps.find(e => e.id === id);
  if (!e) { loadExps().then(() => viewExp(id)); return; }
  document.getElementById('exp-modal-title').textContent = '经验详情: ' + e.module;
  document.getElementById('exp-id').value = e.id;
  document.getElementById('exp-module').value = e.module;
  document.getElementById('exp-module').readOnly = true;
  document.getElementById('exp-keywords').value = e.keywords || '';
  document.getElementById('exp-scene').value = e.scene || '';
  document.getElementById('exp-details').value = e.details || '';
  document.getElementById('exp-submit-btn').classList.add('hidden');
  document.getElementById('exp-modal').classList.remove('hidden');
}

function showExpModal(exp) {
  document.getElementById('exp-modal-title').textContent = exp ? '编辑经验' : '添加经验';
  document.getElementById('exp-id').value = exp ? exp.id : '';
  document.getElementById('exp-module').value = exp ? exp.module : '';
  document.getElementById('exp-module').readOnly = false;
  document.getElementById('exp-keywords').value = exp ? (exp.keywords || '') : '';
  document.getElementById('exp-scene').value = exp ? (exp.scene || '') : '';
  document.getElementById('exp-details').value = exp ? (exp.details || '') : '';
  document.getElementById('exp-submit-btn').classList.remove('hidden');

  // 填充分类下拉
  const catSel = document.getElementById('exp-category');
  if (catSel) {
    catSel.innerHTML = '<option value="">默认分类</option>' +
      expCategories.map(c => `<option value="${c.id}">${esc((c.icon || '') + ' ' + c.name)}</option>`).join('');
    catSel.value = exp ? (exp.category_id || '') : '';
  }

  document.getElementById('exp-modal').classList.remove('hidden');
  setTimeout(() => document.getElementById('exp-module').focus(), 50);
}

async function editExp(id) {
  const e = exps.find(x => x.id === id);
  if (!e) { await loadExps(); e = exps.find(x => x.id === id); }
  if (e) showExpModal(e);
}

function closeExpModal() {
  document.getElementById('exp-modal').classList.add('hidden');
}

async function submitExp() {
  const id = document.getElementById('exp-id').value;
  const module = document.getElementById('exp-module').value.trim();
  if (!module) { alert('请输入模块'); return; }
  const body = {
    module,
    category_id: document.getElementById('exp-category').value,
    keywords: document.getElementById('exp-keywords').value,
    scene: document.getElementById('exp-scene').value,
    details: document.getElementById('exp-details').value
  };
  if (id) {
    await fetch(API + '/api/experiences/' + id, {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
  } else {
    await fetch(API + '/api/experiences', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
  }
  closeExpModal();
  loadExps();
}

async function deleteExp(id) {
  if (!confirm('确认删除这条经验？')) return;
  await fetch(API + '/api/experiences/' + id, {method: 'DELETE'});
  loadExps();
}

// ===== 经验分类管理 =====

function showExpCategoryModal() {
  loadExpCategoryList();
  document.getElementById('exp-category-modal').classList.remove('hidden');
}

function closeExpCategoryModal() {
  document.getElementById('exp-category-modal').classList.add('hidden');
}

function loadExpCategoryList() {
  fetchJSON('/api/exp-categories').then(cats => {
    expCategories = cats || [];
    const el = document.getElementById('exp-category-list');
    el.innerHTML = expCategories.map((c, i) => {
      const isDefault = c.id === 'default-exp-cat';
      const dragAttrs = isDefault ? '' : `draggable="true" ondragstart="onExpCatDragStart(event,${i})" ondragover="onExpCatDragOver(event)" ondrop="onExpCatDrop(event,${i})" ondragend="onExpCatDragEnd(event)"`;
      const dragHandle = isDefault ? '' : '<span class="drag-handle" style="cursor:grab;margin-right:6px;color:var(--text-secondary)">⋮⋮</span>';
      return `<div class="cat-row" ${dragAttrs} style="display:flex;align-items:center;justify-content:space-between;padding:6px 8px;border-bottom:1px solid var(--border)">
        <div style="display:flex;align-items:center;gap:6px">
          ${dragHandle}
          <span>${esc((c.icon || '') + ' ' + c.name)}</span>
        </div>
        <button class="btn btn-danger btn-small" onclick="deleteExpCategory('${c.id}')" ${isDefault ? 'disabled' : ''}>删除</button>
      </div>`;
    }).join('');
  });
}

let _expCatDragSrcIndex = -1;

function onExpCatDragStart(e, i) {
  _expCatDragSrcIndex = i;
  e.target.style.opacity = '0.5';
  e.dataTransfer.effectAllowed = 'move';
}

function onExpCatDragOver(e) {
  e.preventDefault();
  e.dataTransfer.dropEffect = 'move';
}

function onExpCatDrop(e, destIndex) {
  e.preventDefault();
  e.target.style.opacity = '';
  if (_expCatDragSrcIndex < 0 || _expCatDragSrcIndex === destIndex) return;

  const [moved] = expCategories.splice(_expCatDragSrcIndex, 1);
  expCategories.splice(destIndex, 0, moved);

  const reorderData = expCategories.map((c, i) => ({ id: c.id, sort_order: i }));
  fetchJSON('/api/exp-categories/reorder', { method: 'PUT', body: JSON.stringify(reorderData) })
    .then(() => loadExpCategoryList())
    .then(() => updateExpCategoryFilter());
}

function onExpCatDragEnd(e) {
  e.target.style.opacity = '';
  _expCatDragSrcIndex = -1;
}

function addExpCategory() {
  const name = document.getElementById('new-exp-category-name').value.trim();
  const icon = document.getElementById('new-exp-category-icon').value.trim();
  if (!name) { alert('请输入分类名'); return; }
  fetchJSON('/api/exp-categories', {
    method: 'POST',
    body: JSON.stringify({name, icon})
  }).then(() => {
    document.getElementById('new-exp-category-name').value = '';
    document.getElementById('new-exp-category-icon').value = '📚';
    loadExpCategoryList();
    updateExpCategoryFilter();
  });
}

function deleteExpCategory(id) {
  if (!confirm('删除分类后，该分类下的经验将移至默认分类。确认删除？')) return;
  fetchJSON('/api/exp-categories/' + id, { method: 'DELETE' })
    .then(() => {
      loadExpCategoryList();
      updateExpCategoryFilter();
      loadExps();
    });
}
