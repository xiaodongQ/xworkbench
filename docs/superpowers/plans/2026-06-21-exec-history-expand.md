# 执行会话展开面板实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在执行列表行内通过📎图标触发展开面板，原地展示该执行关联的完整会话历史并提供继续对话输入框。

**Architecture:** 复用 `automation.js` 中已有的 `loadRecentExecutions` 的 `renderRow` 渲染逻辑，在其基础上对有 `resume_uuid` 的行增加📎按钮和展开面板。展开面板 HTML 由新增函数 `renderExecSessionPanel(exec)` 生成，面板底部继续对话调用已有 `submitContinue()` 接口。

**Tech Stack:** Vanilla JS (ES6+), HTML, CSS（内联样式复用现有 design tokens）

---

## 文件结构

| 文件 | 改动类型 | 职责 |
|------|----------|------|
| `cmd/server/static/js/views/automation.js` | 修改 | `renderRow` 增加📎按钮；新增 `renderExecSessionPanel(exec)`、`toggleExecSessionPanel(id)`、`submitContinueFromPanel(execId, prompt)` |
| `cmd/server/index.html` | 无需改动 | 📎按钮通过 JS 动态插入，不改 HTML |

---

## 任务分解

### Task 1: 在 renderRow 中增加📎图标按钮

**文件:** `cmd/server/static/js/views/automation.js:403-453` (renderRow 函数)

**目标:** 对有 `resume_uuid` 的执行行，在按钮区增加📎图标，点击触发展开面板。

- [ ] **Step 1: 找到 renderRow 中的按钮区**

在 `renderRow` 函数里，定位到按钮区域。当前按钮有两三个：
```javascript
<button class="btn btn-small" onclick="viewExecutionDetail('${e.id}')" title="查看详情">📋</button>
<button class="btn btn-small" onclick="runEvaluation('${e.id}')" ...>📊</button>
```

在 `📊` 按钮之后增加📎按钮，仅当 `e.resume_uuid` 存在时显示。

- [ ] **Step 2: 增加展开容器 div**

在 `</div>` (行结尾) 后面、`<div id="exec-group-${e.id}"...>` 之前，插入一个隐藏的展开容器：
```javascript
<div id="exec-session-panel-${e.id}" class="hidden" style="display:none;padding:8px 12px 8px 36px;background:var(--hover);border-bottom:1px solid var(--border);font-size:12px"></div>
```

- [ ] **Step 3: 给📎按钮绑定点击事件**

按钮 HTML：
```javascript
e.resume_uuid
  ? `<button class="btn btn-small" onclick="toggleExecSessionPanel('${e.id}')" title="查看会话历史并继续对话">📎</button>`
  : ''
```

- [ ] **Step 4: 提交**

```bash
git add cmd/server/static/js/views/automation.js
git commit -m "feat(exec-list): add 📎 button to exec rows with resume_uuid"
```

---

### Task 2: 实现展开面板渲染函数 renderExecSessionPanel

**文件:** `cmd/server/static/js/views/automation.js` (新增函数)

**目标:** 生成展开面板的 HTML 内容，包括：会话历史列表 + 继续对话输入框。

- [ ] **Step 1: 实现 renderExecSessionPanel(exec) 函数**

新增在 `automation.js` 末尾（约 `loadRecentExecutions` 函数之后），核心逻辑：

```javascript
// renderExecSessionPanel: 根据 exec 的 resume_uuid 加载完整会话链，渲染展开面板
// 复用 tasks.js 里的 _convStatusBadge, _convEscape, _convTruncate, _sanitizeId（需确认这些是否已暴露到 window）
// 若未暴露，直接复制粘贴需要的辅助函数到本文件
async function renderExecSessionPanel(exec) {
  if (!exec.resume_uuid) return '<div style="color:var(--text-secondary)">无会话ID</div>';

  // 加载该 resume_uuid 关联的所有 execution（GET /api/executions?resume_uuid=xxx）
  let execs;
  try {
    execs = await fetchJSON('/api/executions?resume_uuid=' + encodeURIComponent(exec.resume_uuid));
  } catch (e) {
    return '<div style="color:var(--exception)">加载会话历史失败：' + esc(e.message) + '</div>';
  }

  if (!execs || execs.length === 0) {
    return '<div style="color:var(--text-secondary)">暂无会话历史</div>';
  }

  // 按 started_at 升序排列
  execs.sort((a, b) => new Date(a.started_at) - new Date(b.started_at));

  // 找到根节点（resume_uuid == id 的那个，或第一条）
  const root = execs.find(e => e.resume_uuid === e.id) || execs[0];

  // 生成历史列表 HTML（精简版时间线）
  const historyItems = execs.map((e, idx) => {
    const isRoot = e.id === root.id;
    const tag = isRoot
      ? '<span style="background:var(--accent);color:#fff;padding:1px 6px;border-radius:3px;font-size:10px">原始</span>'
      : '<span style="background:#0ea5e9;color:#fff;padding:1px 6px;border-radius:3px;font-size:10px">继续 ' + idx + '</span>';
    const ts = e.started_at ? new Date(e.started_at).toLocaleString('zh-CN', {hour12: false}) : '';
    const status = e.exit_code === 0
      ? '<span style="color:#10b981;font-size:10px">✓</span>'
      : '<span style="color:var(--exception);font-size:10px">✗</span>';
    const prompt = e.prompt ? esc(e.prompt) : '<i style="color:var(--text-secondary)">(无prompt)</i>';
    return `<div style="display:flex;gap:6px;align-items:flex-start;padding:4px 0;border-bottom:1px solid var(--border)">
      <div style="flex-shrink:0;margin-top:2px">${status}</div>
      <div style="flex:1;min-width:0">
        <div style="display:flex;gap:4px;align-items:center;margin-bottom:2px">${tag}<span style="color:var(--text-secondary);font-size:10px">${ts}</span></div>
        <div style="color:var(--text);font-size:11px;word-break:break-word">${prompt}</div>
      </div>
    </div>`;
  }).join('');

  // 继续对话输入框
  const panelId = 'exec-session-panel-' + exec.id;
  const inputId = 'panel-continue-input-' + exec.id;
  const submitId = 'panel-continue-submit-' + exec.id;

  return `
    <div style="margin:6px 0">
      <div style="font-size:11px;color:var(--text-secondary);margin-bottom:6px;font-weight:600">💬 会话历史 (${execs.length}条)</div>
      <div style="border:1px solid var(--border);border-radius:4px;overflow:hidden;margin-bottom:8px;max-height:200px;overflow-y:auto">
        ${historyItems}
      </div>
      <div style="display:flex;gap:6px;align-items:center">
        <input id="${inputId}" placeholder="输入想继续问的内容..." style="flex:1;padding:6px 8px;border:1px solid var(--border);border-radius:4px;font-size:12px" onkeydown="if(event.key==='Enter' && !event.shiftKey){event.preventDefault();submitContinueFromPanel('${exec.id}')}">
        <button id="${submitId}" class="btn btn-small btn-primary" onclick="submitContinueFromPanel('${exec.id}')">▶</button>
      </div>
    </div>
  `;
}
```

**注意：** `esc` 函数在 automation.js 中是否已定义？需要确认。如果未定义，用 `String(s).replace(/[&<>"']/g, ...)` 内联实现。

- [ ] **Step 2: 提交**

```bash
git add cmd/server/static/js/views/automation.js
git commit -m "feat(exec-list): add renderExecSessionPanel for inline session history"
```

---

### Task 3: 实现 toggleExecSessionPanel 和 submitContinueFromPanel

**文件:** `cmd/server/static/js/views/automation.js` (新增函数)

**目标:** 实现展开/收起逻辑，以及从面板内提交继续对话。

- [ ] **Step 1: 实现 toggleExecSessionPanel(execId)**

```javascript
// toggleExecSessionPanel: 展开或收起执行行的会话历史面板
async function toggleExecSessionPanel(execId) {
  const panel = document.getElementById('exec-session-panel-' + execId);
  if (!panel) return;

  if (!panel.classList.contains('hidden')) {
    // 收起
    panel.classList.add('hidden');
    panel.style.display = 'none';
    return;
  }

  // 先展示空白面板（避免点击无响应感）
  panel.classList.remove('hidden');
  panel.style.display = 'block';
  panel.innerHTML = '<div style="color:var(--text-secondary);padding:4px">加载中...</div>';

  // 获取 exec 数据（需要 resume_uuid）
  let exec;
  try {
    const execs = await fetchJSON('/api/executions?limit=1');
    exec = execs && execs.find(e => e.id === execId);
  } catch (e) {
    panel.innerHTML = '<div style="color:var(--exception);padding:4px">加载失败</div>';
    return;
  }

  if (!exec) {
    panel.innerHTML = '<div style="color:var(--text-secondary);padding:4px">未找到执行记录</div>';
    return;
  }

  // 渲染面板内容
  panel.innerHTML = await renderExecSessionPanel(exec);
}
```

- [ ] **Step 2: 实现 submitContinueFromPanel(execId)**

从 `automation.js` 的 `submitContinue()` 复制核心逻辑，改造为面板专用版本（不需要 `currentExecId` 状态）：

```javascript
async function submitContinueFromPanel(execId) {
  const input = document.getElementById('panel-continue-input-' + execId);
  const submitBtn = document.getElementById('panel-continue-submit-' + execId);
  const prompt = input?.value?.trim();
  if (!prompt) return;

  if (submitBtn) { submitBtn.disabled = true; submitBtn.textContent = '⏳'; }

  try {
    const res = await fetchJSON('/api/executions/' + execId + '/continue', {
      method: 'POST',
      body: JSON.stringify({ prompt }),
    });
    const newExecId = res && res.execution_id;
    // 清空输入框
    if (input) input.value = '';
    // 刷新执行列表
    loadRecentExecutions();
    // 提示用户
    const panel = document.getElementById('exec-session-panel-' + execId);
    if (panel) {
      const feedback = document.createElement('div');
      feedback.style.cssText = 'margin-top:8px;padding:6px 8px;background:rgba(16,185,129,0.12);border:1px solid #10b981;border-radius:4px;font-size:11px;color:#10b981';
      feedback.textContent = '✓ 继续对话已提交，新执行ID: ' + (newExecId || '?');
      panel.appendChild(feedback);
      setTimeout(() => feedback.remove(), 5000);
    }
  } catch (e) {
    alert('继续对话失败：' + e.message);
  } finally {
    if (submitBtn) { submitBtn.disabled = false; submitBtn.textContent = '▶'; }
  }
}
```

- [ ] **Step 3: 确认 esc 函数可用**

在 automation.js 中搜索 `function esc` 或 `const esc =`。如果不存在，在文件开头（依赖区）新增：

```javascript
const esc = s => String(s || '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
```

- [ ] **Step 4: 提交**

```bash
git add cmd/server/static/js/views/automation.js
git commit -m "feat(exec-list): add toggleExecSessionPanel and submitContinueFromPanel"
```

---

### Task 4: 自验证

**目标:** 启动 xworkbench server，打开自动化页面，找到有📎图标的执行行，验证展开面板功能正常。

- [ ] **Step 1: 启动 server**

```bash
cd /Users/xd/Documents/workspace/repo/xworkbench && go run ./cmd/server
```

- [ ] **Step 2: 打开浏览器**

访问 http://localhost:18888，进入「自动化」Tab，找到带📎图标的执行行。

- [ ] **Step 3: 验证展开**

点击📎图标，验证：
- 面板正确展开（显示加载中→显示历史列表）
- 历史列表正确显示该 session 的所有执行
- 底部有继续对话输入框
- 输入内容回车可提交
- 提交后执行列表刷新

- [ ] **Step 4: 验证收起**

再次点击📎图标，验证面板收起。

---

## 自检清单

- [ ] spec 中每个 requirement 都能在 plan 中找到对应任务？
  - ✅ 在执行列表行内增加📎图标 — Task 1
  - ✅ 点击📎触发展开面板 — Task 3
  - ✅ 展开面板显示会话历史 — Task 2
  - ✅ 展开面板提供继续对话输入框 — Task 2 & 3
  - ✅ 复用 submitContinue API — Task 3
- [ ] placeholder 扫描：无 TBD/TODO/待实现
- [ ] 类型一致性：函数名在所有 task 中一致（`toggleExecSessionPanel`、`renderExecSessionPanel`、`submitContinueFromPanel`）
