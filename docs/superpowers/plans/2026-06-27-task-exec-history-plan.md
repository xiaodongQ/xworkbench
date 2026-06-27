# 任务详情执行历史区块实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在任务详情弹窗底部增加执行历史区块，展示完整执行列表并支持分页，点击后跳转到自动化 Tab 查看详情

**Architecture:** 前端改动，在 `task-modal` HTML 中增加区块容器，`tasks.js` 中新增加载和渲染逻辑，分页由前端实现（前端分页，不调额外 API）

**Tech Stack:** Vanilla JS + Go HTTP API (`GET /api/tasks/{id}/executions`)

---

## 文件改动

| 文件 | 改动 |
|---|---|
| `cmd/server/index.html` | task-modal 内增加执行历史区块 HTML |
| `cmd/server/static/js/views/tasks.js` | 新增 `loadTaskExecHistory`、`renderTaskExecHistory`、`prevTaskExecs`、`nextTaskExecs` 函数 |

---

## Task 1: HTML 区块容器

**文件:** Modify: `cmd/server/index.html:720`（modal-actions 上方）

- [ ] **Step 1: 在 task-modal 的 `modal-actions` 上方插入执行历史区块容器**

在 `index.html` 找到 task-modal 里的 `modal-actions div`，在其上方插入：

```html
    <!-- 执行历史区块（由 tasks.js 的 loadTaskExecHistory 渲染） -->
    <div id="task-exec-history" class="hidden" style="margin-top:16px;padding-top:16px;border-top:1px solid var(--border)">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:10px">
        <span style="font-weight:600;font-size:13px">📋 执行历史</span>
        <span id="task-exec-history-info" style="font-size:11px;color:var(--text-secondary)"></span>
      </div>
      <div id="task-exec-history-list" style="max-height:200px;overflow-y:auto"></div>
      <div id="task-exec-history-pager" style="display:flex;justify-content:space-between;align-items:center;margin-top:8px">
        <button class="btn btn-small" id="task-exec-prev" onclick="prevTaskExecs()">← 上一页</button>
        <span id="task-exec-history-page" style="font-size:11px;color:var(--text-secondary)"></span>
        <button class="btn btn-small" id="task-exec-next" onclick="nextTaskExecs()">下一页 →</button>
      </div>
    </div>
```

- [ ] **Step 2: 提交**

```bash
git add cmd/server/index.html
git commit -m "feat(tasks): add exec history container in task-modal"
```

---

## Task 2: JS 加载和渲染逻辑

**文件:** Modify: `cmd/server/static/js/views/tasks.js`

- [ ] **Step 1: 在文件末尾（`viewTask` 函数下方）增加分页状态变量和核心函数**

```javascript
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
    return '<div onclick="jumpToExecDetail(' + e.id + ')" style="cursor:pointer;padding:6px 8px;border-radius:4px;margin-bottom:4px;background:var(--hover);font-size:12px;display:flex;gap:8px;align-items:center">' +
      '<span style="color:' + color + ';flex-shrink:0">' + icon + '</span>' +
      '<span style="color:var(--text-secondary);flex-shrink:0">' + cli + (model ? ' / ' + model : '') + '</span>' +
      '<span style="color:var(--text-secondary);flex-shrink:0">' + dur + '</span>' +
      '<span style="color:var(--text-secondary);flex-shrink:0">' + date + '</span>' +
      '<span style="margin-left:auto;color:var(--text-secondary)">' + score + '</span>' +
    '</div>';
  }).join('');

  prevBtn.classList.toggle('hidden', _taskExecPage <= 1);
  nextBtn.classList.toggle('hidden', _taskExecPage >= totalPages);
  pageEl.textContent = '';
}

function prevTaskExecs() {
  if (_taskExecPage > 1) { _taskExecPage--; renderTaskExecHistory(); }
}

function nextTaskExecs() {
  const totalPages = Math.ceil(_taskExecList.length / _taskExecPageSize);
  if (_taskExecPage < totalPages) { _taskExecPage++; renderTaskExecHistory(); }
}

function jumpToExecDetail(execId) {
  switchTab('automation');
  // automation Tab 加载完会调 loadRecentExecutions，等待一下再打开详情
  setTimeout(function() {
    if (typeof viewExecutionDetail === 'function') {
      viewExecutionDetail(execId);
    }
  }, 100);
}
```

- [ ] **Step 2: 在 `viewTask` 末尾调用 `loadTaskExecHistory`**

在 `tasks.js` 的 `viewTask` 函数末尾（`loadTaskEvents()` 调用之后）增加：

```javascript
  // 加载执行历史
  loadTaskExecHistory(t.id);
```

- [ ] **Step 3: 提交**

```bash
git add cmd/server/static/js/views/tasks.js
git commit -m "feat(tasks): add exec history in task detail modal with pagination"
```

---

## Task 3: 验证

- [ ] **Step 1: 启动服务**

```bash
./scripts/run.sh
```

- [ ] **Step 2: 打开浏览器，进入任务 Tab，点击一个有执行记录的任务详情**

验证：
1. 弹窗底部出现"执行历史"区块
2. 列表正确显示执行记录（状态图标/CLI/耗时/时间/评估分）
3. N > 10 时分页控件显示，翻页正常
4. 点击记录跳转自动化 Tab 并打开详情弹窗
5. 无执行记录时显示"暂无执行记录"
