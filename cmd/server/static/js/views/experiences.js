// Experiences Tab：经验库列表 + 模块搜索 + exp modal（查看/添加/删除）
// 依赖 api.js

let exps = [];

async function loadExps() {
  const module = document.getElementById('exp-search').value;
  const url = API + '/api/experiences' + (module ? '?module=' + encodeURIComponent(module) : '');
  try {
    exps = await fetchJSON(url);
    renderExpTable(exps);
  } catch(e) { console.error(e); }
}

function renderExpTable(list) {
  const el = document.getElementById('exp-list');
  if (!list || list.length === 0) {
    el.innerHTML = '<div class="empty">暂无经验</div>';
    document.getElementById('exp-count').textContent = '0 条经验';
    return;
  }
  document.getElementById('exp-count').textContent = list.length + ' 条经验';
  el.innerHTML = `<table class="exp-table">
    <thead><tr><th class="col-module" style="text-align:left">模块</th><th class="col-kw">关键词</th><th class="col-scene">适用场景</th><th class="col-ops">操作</th></tr></thead>
    <tbody>${list.map(e => `
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
      </tr>`).join('')}</tbody>
  </table>`;
}

function viewExp(id) {
  const e = exps.find(e => e.id === id);
  if (!e) { loadExps().then(() => viewExp(id)); return; }
  document.getElementById('exp-modal-title').textContent = '经验详情: ' + e.module;
  document.getElementById('exp-id').value = e.id;
  document.getElementById('exp-module').value = e.module;
  document.getElementById('exp-module').readOnly = true;
  document.getElementById('exp-keywords').value = e.keywords || '';
  document.getElementById('exp-log-paths').value = e.log_paths || '';
  document.getElementById('exp-tool-usage').value = e.tool_usage || '';
  document.getElementById('exp-scene').value = e.scene || '';
  document.getElementById('exp-log-samples').value = e.log_samples || '';
  document.getElementById('exp-code-snippets').value = e.code_snippets || '';
  document.getElementById('exp-submit-btn').classList.add('hidden');
  document.getElementById('exp-modal').classList.remove('hidden');
}

function showExpModal(exp) {
  document.getElementById('exp-modal-title').textContent = exp ? '编辑经验' : '添加经验';
  document.getElementById('exp-id').value = exp ? exp.id : '';
  document.getElementById('exp-module').value = exp ? exp.module : '';
  document.getElementById('exp-module').readOnly = !!exp; // 编辑时不可改模块
  document.getElementById('exp-keywords').value = exp ? (exp.keywords || '') : '';
  document.getElementById('exp-log-paths').value = exp ? (exp.log_paths || '') : '';
  document.getElementById('exp-tool-usage').value = exp ? (exp.tool_usage || '') : '';
  document.getElementById('exp-scene').value = exp ? (exp.scene || '') : '';
  document.getElementById('exp-log-samples').value = exp ? (exp.log_samples || '') : '';
  document.getElementById('exp-code-snippets').value = exp ? (exp.code_snippets || '') : '';
  document.getElementById('exp-submit-btn').classList.remove('hidden');
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
  if (!module) { alert('请输入模块名'); return; }
  const body = {
    module,
    keywords: document.getElementById('exp-keywords').value,
    log_paths: document.getElementById('exp-log-paths').value,
    tool_usage: document.getElementById('exp-tool-usage').value,
    scene: document.getElementById('exp-scene').value,
    log_samples: document.getElementById('exp-log-samples').value,
    code_snippets: document.getElementById('exp-code-snippets').value
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
