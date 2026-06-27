# 任务详情执行历史区块

## 背景

手动任务完成后，点击详情只看到基本信息，无法直接查看执行结果和输出。需要在任务详情弹窗里展示该任务的完整执行历史。

## 设计

### 功能

在 `task-modal`（任务详情弹窗）底部增加"执行历史"区块，展示该任务所有执行记录，支持分页。

### UI 布局

```
[任务详情弹窗 task-modal]
├── 基本信息（标题/描述/类型/验收标准）
├── 关联经验库 chips
├── ⚠️ 待交互内容（waiting_input 时显示）
├── 🤖 AI 自治区块
├── ─────────────────  ← 新增
│  📋 执行历史（共 N 条，第 X/Y 页）
│  ┌─ ● success · glm-5.1 · 12.3s · 2026-06-27 10:30 · score=8.2
│  ├─ ✗ failed  · glm-5.1 · 3.2s  · 2026-06-27 09:15
│  ├─ ● success · sonnet  · 8.1s  · 2026-06-26 22:00
│  └─ ...（更多记录）
│  [上一页] [下一页]  ← 分页控制
└─ [取消] [创建/保存]
```

### 交互

- 点击某条执行 → `switchTab('automation')` → 触发 `viewExecutionDetail(execId)` 打开执行详情弹窗
- 分页：每页 10 条，总数 N > 10 才显示分页控件
- 空状态：显示"暂无执行记录"

### 状态徽章

- success = 绿●
- failed = 红✗
- running = 黄⏳
- timeout = 灰⏱
- cancelled = 灰—

评估分：无时显示"未评"，有分时显示 score 值。

### 数据

- API：`GET /api/tasks/{id}/executions`
- 后端已支持，返回该 task 的所有 executions（按 created_at 倒序）
- 前端负责分页切页（前端分页，不调额外 API）

### 文件改动

| 文件 | 改动 |
|---|---|
| `cmd/server/index.html` | task-modal 内增加执行历史区块 HTML |
| `cmd/server/static/js/views/tasks.js` | `viewTask()` 加载 executions，`renderTaskExecHistory()` 渲染分页列表 |

### 实现步骤

1. `index.html`：在 `task-modal` 的 `modal-actions` 上方插入执行历史区块容器 `<div id="task-exec-history">`
2. `tasks.js`：
   - `viewTask()` 末尾调用 `loadTaskExecHistory(taskId)` 加载并渲染
   - `loadTaskExecHistory(taskId)`：GET `/api/tasks/{id}/executions`，调用 `renderTaskExecHistory(execs)`
   - `renderTaskExecHistory(execs)`：分页渲染列表，绑定点击跳转逻辑
   - 分页状态用闭包变量保存（`currentPage`、`totalPages`）
