# exec-detail-modal 增加对话历史和活动历史 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development

**Goal:** 在 exec-detail-modal（执行详情弹窗）底部增加对话历史和活动历史区块，从 task-modal 移除这些区块。

**Architecture:**
- exec-detail-modal 在 `viewExecutionDetail()` 末尾增加对话历史和活动历史展开区
- 对话历史复用 `renderTaskConversation()` 的渲染逻辑（需调整参数）
- 活动历史复用 `loadTaskEvents()` 的逻辑

**Tech Stack:** Vanilla JS, HTML

---

## 文件结构

| 文件 | 改动类型 | 职责 |
|------|----------|------|
| `cmd/server/index.html` | 修改 | task-modal 移除 task-conversation-section、task-events-section、ai-loop-section、task-comment-section |
| `cmd/server/static/js/views/automation.js` | 修改 | exec-detail-modal 增加对话历史/活动历史区块的渲染和加载逻辑 |
| `cmd/server/static/js/views/tasks.js` | 可选移除 | 如果确认 task-conversation/task-events 不再被其他方调用，可移除 |

---

## 任务分解

### Task 1: 在 exec-detail-modal 增加对话历史和活动历史

**文件:** `cmd/server/index.html` 和 `cmd/server/static/js/views/automation.js`

**目标:** 在 exec-detail-modal 底部（`</div>` 关闭前）增加两个可展开区块。

#### Step 1: 在 index.html 的 exec-detail-modal 末尾增加 HTML

定位 `id="exec-detail-modal"` 弹窗，找到 `</div>` 结尾（在 `modal-actions` 之后）之前。

在约 `</div>` 前插入：

```html
<!-- 对话历史（按需展开） -->
<div id="exec-conversation-section" class="form-group" style="border-top:1px solid var(--border);padding-top:10px;margin-top:10px">
  <label style="cursor:pointer;user-select:none;display:flex;align-items:center;gap:6px" onclick="toggleExecConversation()">
    <span id="exec-conversation-toggle" style="display:inline-block;transition:transform 0.15s;font-size:10px;width:12px;text-align:center">▶</span>
    💬 对话历史 <span id="exec-conversation-count" style="font-size:11px;color:var(--text-secondary);font-weight:normal"></span>
    <button type="button" class="btn btn-small" style="margin-left:auto" onclick="event.stopPropagation();loadExecConversation()">🔄 刷新</button>
  </label>
  <div id="exec-conversation-body" class="hidden" style="max-height:300px;overflow-y:auto;margin-top:8px;border:1px solid var(--border);border-radius:4px;background:var(--card-bg)"></div>
</div>

<!-- 活动历史 -->
<div id="exec-events-section" class="form-group" style="border-top:1px solid var(--border);padding-top:10px;margin-top:10px">
  <label style="cursor:pointer;user-select:none;display:flex;align-items:center;gap:6px" onclick="toggleExecEvents()">
    <span id="exec-events-toggle" style="display:inline-block;transition:transform 0.15s;font-size:10px;width:12px;text-align:center">▶</span>
    📜 活动历史 <span id="exec-events-count" style="font-size:11px;color:var(--text-secondary);font-weight:normal"></span>
    <button type="button" class="btn btn-small" style="margin-left:auto" onclick="event.stopPropagation();loadExecEvents()">🔄 刷新</button>
  </label>
  <div id="exec-events-body" class="hidden" style="max-height:200px;overflow-y:auto;margin-top:8px;border:1px solid var(--border);border-radius:4px;background:var(--card-bg)"></div>
</div>
```

#### Step 2: 在 automation.js 实现相关函数

在 `viewExecutionDetail()` 末尾（modal 显示后），调用加载函数：

```javascript
// 加载对话历史和活动历史
loadExecConversation(id);
loadExecEvents(id);
```

新增以下函数（在 automation.js 末尾）：

```javascript
// 对话历史
function toggleExecConversation() {
  const body = document.getElementById('exec-conversation-body');
  const arrow = document.getElementById('exec-conversation-toggle');
  if (body.classList.contains('hidden')) {
    body.classList.remove('hidden');
    arrow.style.transform = 'rotate(90deg)';
  } else {
    body.classList.add('hidden');
    arrow.style.transform = 'rotate(0deg)';
  }
}

async function loadExecConversation(execId) {
  const countEl = document.getElementById('exec-conversation-count');
  const body = document.getElementById('exec-conversation-body');
  try {
    // 获取该 execution 关联的所有执行（按 resume_uuid 分组）
    // 如果是根节点，用 id；如果是子节点，用 resume_uuid
    const exec = await fetchJSON('/api/executions/' + execId);
    const resumeUuid = exec?.resume_uuid;
    let execs;
    if (resumeUuid) {
      execs = await fetchJSON('/api/executions?resume_uuid=' + encodeURIComponent(resumeUuid));
    } else {
      execs = [exec];
    }
    // 复用 tasks.js 的渲染逻辑，但需要复制过来因为不是 window 函数
    body.innerHTML = renderExecConversationTimeline(execs);
    countEl.textContent = '(' + execs.length + ' 轮)';
  } catch (e) {
    body.innerHTML = '<div style="padding:8px;color:var(--exception);font-size:12px">加载失败：' + esc(e.message) + '</div>';
  }
}

// 渲染精简版时间线（复用于 exec-detail）
function renderExecConversationTimeline(execs) {
  if (!execs || execs.length === 0) {
    return '<div style="padding:8px;color:var(--text-secondary);font-size:12px">暂无对话历史</div>';
  }
  // 按 started_at 升序
  execs.sort((a, b) => new Date(a.started_at) - new Date(b.started_at));
  const root = execs.find(e => e.resume_uuid === e.id) || execs[0];
  return execs.map((e, idx) => {
    const isRoot = e.id === root.id;
    const tag = isRoot
      ? '<span style="background:var(--accent);color:#fff;padding:1px 6px;border-radius:3px;font-size:10px">原始</span>'
      : '<span style="background:#0ea5e9;color:#fff;padding:1px 6px;border-radius:3px;font-size:10px">继续 ' + idx + '</span>';
    const ts = e.started_at ? new Date(e.started_at).toLocaleString('zh-CN', {hour12: false}) : '';
    const status = e.exit_code === 0
      ? '<span style="color:#10b981;font-size:10px">✓</span>'
      : '<span style="color:var(--exception);font-size:10px">✗</span>';
    const prompt = e.prompt ? esc(e.prompt) : '<i style="color:var(--text-secondary)">(无prompt)</i>';
    return `<div style="display:flex;gap:6px;padding:6px 0;border-bottom:1px solid var(--border)">
      <div style="flex-shrink:0;margin-top:2px">${status}</div>
      <div style="flex:1;min-width:0">
        <div style="display:flex;gap:4px;align-items:center;margin-bottom:2px">${tag}<span style="color:var(--text-secondary);font-size:10px">${ts}</span></div>
        <div style="color:var(--text);font-size:11px;word-break:break-word">${prompt}</div>
      </div>
    </div>`;
  }).join('');
}

// 活动历史
function toggleExecEvents() {
  const body = document.getElementById('exec-events-body');
  const arrow = document.getElementById('exec-events-toggle');
  if (body.classList.contains('hidden')) {
    body.classList.remove('hidden');
    arrow.style.transform = 'rotate(90deg)';
  } else {
    body.classList.add('hidden');
    arrow.style.transform = 'rotate(0deg)';
  }
}

async function loadExecEvents(execId) {
  const countEl = document.getElementById('exec-events-count');
  const body = document.getElementById('exec-events-body');
  try {
    // 获取该 execution 的 task_id，然后加载 task events
    const exec = await fetchJSON('/api/executions/' + execId);
    if (!exec?.task_id) {
      body.innerHTML = '<div style="padding:8px;color:var(--text-secondary);font-size:12px">无关联任务</div>';
      return;
    }
    const events = await fetchJSON('/api/tasks/' + exec.task_id + '/events');
    if (!events || events.length === 0) {
      body.innerHTML = '<div style="padding:8px;color:var(--text-secondary);font-size:12px">暂无活动记录</div>';
      countEl.textContent = '(0)';
      return;
    }
    countEl.textContent = '(' + events.length + ' 条)';
    body.innerHTML = events.map(ev => {
      const ts = ev.created_at ? new Date(ev.created_at).toLocaleString('zh-CN', {hour12: false}) : '';
      return `<div style="padding:4px 0;border-bottom:1px solid var(--border);font-size:11px">
        <span style="color:var(--text-secondary)">${ts}</span>
        <span style="margin-left:6px">${esc(ev.event || '')}</span>
      </div>`;
    }).join('');
  } catch (e) {
    body.innerHTML = '<div style="padding:8px;color:var(--exception);font-size:12px">加载失败：' + esc(e.message) + '</div>';
  }
}
```

- [ ] **提交**

---

### Task 2: 从 task-modal 移除对话历史、活动历史区块

**文件:** `cmd/server/index.html`

**目标:** 从 task-modal 中移除：
- task-conversation-section
- task-events-section
- ai-loop-section
- task-comment-section

找到这些区块并删除对应的 HTML。

- [ ] **提交**

---

### Task 3: 自验证

启动 xworkbench，打开自动化页面，点击任意执行的📋详情按钮，验证：
1. exec-detail-modal 底部有对话历史和活动历史
2. 点击可以展开/收起
3. task-modal 中已无这些区块

- [ ] **验证**

---

## 自检清单

- [ ] exec-detail-modal 有对话历史和活动历史展开区块
- [ ] task-modal 已移除对话历史、活动历史、AI自治、评论区块
- [ ] 展开/收起逻辑正常
- [ ] 数据加载正常
