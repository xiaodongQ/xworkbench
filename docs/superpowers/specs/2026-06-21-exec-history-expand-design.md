# 执行会话展开面板设计（修订版）

## 背景

当前 xworkbench 中"继续对话"和"对话历史"功能分散在多个地方，结构混乱：

- **task-modal**（任务详情弹窗）：有 task-conversation-section（对话历史）、task-events-section（活动历史）— 位置不合适
- **exec-detail-modal**（执行详情弹窗）：有📋详情、💬继续对话按钮
- **自动化页面**：执行列表有📎/🔗按钮，可以展开会话历史面板

用户场景：继续对话需求都在**自动化页面**的最近执行列表中，点击执行详情。

## 目标

1. **task-modal** — 移除 task-conversation-section 和 task-events-section，保持简洁
2. **exec-detail-modal** — 增加对话历史、活动历史区块，作为执行详情的标配内容
3. **自动化页面** — 保留📎/🔗展开面板，作为快速继续对话入口

## 交互设计

### task-modal（简化后）

移除以下区块：
- ❌ task-conversation-section（对话历史）
- ❌ task-events-section（活动历史）
- ❌ ai-loop-section（AI 自治）
- ❌ task-comment-section（评论）

保留：
- ✅ 任务基本信息
- ✅ 执行历史（仅列表，无展开面板）
- ✅ 创建/编辑表单

### exec-detail-modal（增强后）

在执行详情弹窗底部增加：

```
💬 对话历史 (N条) [展开/收起]
📜 活动历史 (N条) [展开/收起]
```

点击展开后显示完整历史，逻辑复用现有的 `renderTaskConversation()` 和 `loadTaskEvents()`。

### 自动化页面（保持不变）

📎/🔗展开面板保持现有设计，作为快速入口。

## 技术实现

### 改动文件

| 文件 | 改动 |
|------|------|
| `index.html` | task-modal 移除对话历史/活动历史区块 |
| `tasks.js` | 移除 `renderTaskConversation`、`toggleTaskConversation`、`loadTaskConversation`、`loadTaskEvents`、`toggleTaskEvents` |
| `automation.js` | 保持📎/🔗展开面板；exec-detail-modal 增加对话历史/活动历史区块 |

### exec-detail-modal 增加内容

在 `automation.js` 的 `viewExecutionDetail()` 中，弹窗底部增加：

```html
<div id="exec-conversation-section" class="form-group" style="border-top:1px solid var(--border);padding-top:10px;margin-top:10px">
  <label onclick="toggleExecConversation()">
    <span id="exec-conversation-toggle">▶</span> 💬 对话历史
  </label>
  <div id="exec-conversation-body" class="hidden"></div>
</div>

<div id="exec-events-section" class="form-group" style="border-top:1px solid var(--border);padding-top:10px;margin-top:10px">
  <label onclick="toggleExecEvents()">
    <span id="exec-events-toggle">▶</span> 📜 活动历史
  </label>
  <div id="exec-events-body" class="hidden"></div>
</div>
```

复用 `renderTaskConversation()` 渲染对话历史（传入该 execution 的 resume_uuid 关联的所有执行）。

## 设计优势

1. **职责清晰**：task-modal 管任务基本信息，exec-detail-modal 管执行详情和历史
2. **信息分层**：自动化页面看列表找入口，exec-detail-modal 看完整历史
3. **减少冗余**：同一信息不在多处重复
