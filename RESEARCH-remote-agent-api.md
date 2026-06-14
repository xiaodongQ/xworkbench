# 调研：Remote Agent API + 远程任务闭环

## 📋 需求回顾

骨架已铺好（列已加，待补 handler）：
- `task_type / claimer_agent_id / result_output / evaluation_score` 列已加到 tasks 表 ✓
- `TaskTypeManual/Scheduled/Remote` 常量已定义 ✓（代码层尚未）
- 待完成：
  1. Agent 注册/心跳 API（`POST /api/agents/register`、`POST /api/agents/{id}/heartbeat`）
  2. Remote task claim/report（`POST /api/tasks/{id}/claim`、`POST /api/tasks/{id}/report`）
  3. 前端远程任务创建界面

---

## 1. 现状盘点

### 1.1 数据库层（repo.go + sqlite.go）

**migrateTasksColumns** 已加列：
```sql
task_type         TEXT DEFAULT 'manual'
claimer_agent_id  TEXT
result_output     TEXT
evaluation_score  REAL DEFAULT 0
```

**缺失**：models.go 中 Task struct 没有对应字段，常量也未定义。

### 1.2 代码结构

```
cmd/server/main.go        # HTTP handler 入口（所有路由 + handler）
internal/backend/models.go   # 数据模型（缺新字段）
internal/backend/repo.go    # DB 操作（claim/update 可基于现有 UpdateStatus 扩展）
internal/backend/sqlite.go  # 表结构初始化（未含 tasks 新列）
```

### 1.3 现有相关逻辑

- `handleTaskRun` → 立即执行，状态变 `in_progress`，写 executions 行
- `handleTaskRunLoop` → 评估闭环，支持换模型重试（但仍是本地执行）
- `handleTaskUnclaim` → 把 task 状态恢复到 `pending`（相当于放弃 claim）
- **无 agents 表，无远程心跳机制**

---

## 2. API 设计草案

### 2.1 Agent 注册

```
POST /api/agents/register
Body: { "name": "agent-001", "capabilities": ["remote-task"], "version": "0.1.0" }
Response: { "agent_id": "uuid", "token": "hmac-secret", "registered_at": "..." }
```

**需要新增**：
- `agents` 表：id / name / token / capabilities / version / last_heartbeat / created_at
- `agents` repo 层（类似 TaskRepo）

### 2.2 Agent 心跳

```
POST /api/agents/{id}/heartbeat
Header: Authorization: Bearer <token>
Body: { "status": "idle", "current_task_id": "..." }
Response: { "ok": true, "server_time": "..." }
```

- 心跳超时（如 >30s 无心跳）→ 将该 agent 对应 claimer 的 tasks 状态恢复 `pending`
- 需要一个后台 goroutine 定期检查 agents 心跳（类似 scheduler）

### 2.3 Remote Task Claim

```
POST /api/tasks/{id}/claim
Header: Authorization: Bearer <token>
Body: { "agent_id": "uuid" }
Response: { "task": {...}, "status": "claimed" }
```

逻辑：
1. 验证 token + agent_id 匹配
2. 检查 task.status == `pending` && task_type == `remote`
3. UPDATE tasks SET status=`in_progress`, claimer_agent_id=?, claimed_at=NOW()
4. 返回 task 全量数据（含 prompt / resources / acceptance）

**注意**：当前 `handleTaskUnclaim` 是清空 maintainer 但保持 pending，这里 claim 需要原子操作防止重复抢。

### 2.4 Remote Task Report

```
POST /api/tasks/{id}/report
Header: Authorization: Bearer <token>
Body: {
  "agent_id": "uuid",
  "status": "archived",          // archived | exception
  "result_output": "...",        // 任务执行输出摘要
  "evaluation_score": 8.5,       // 可选，agent 自评
  "error": ""                    // 可选，异常信息
}
Response: { "ok": true }
```

逻辑：
1. 验证 token + agent_id 匹配（必须是该 task 的 claimer）
2. 更新 tasks.status / result_output / evaluation_score / last_error
3. 如果是正常完成，触发 `handleTaskLearn` 类似流程（可选）

---

## 3. Task Type 常量

```go
const (
    TaskTypeManual  = "manual"
    TaskTypeScheduled = "scheduled"
    TaskTypeRemote = "remote"
)
```

需要加到 models.go 的 Task struct：
```go
type Task struct {
    ...
    TaskType        string  `json:"task_type,omitempty"`
    ClaimerAgentID  string  `json:"claimer_agent_id,omitempty"`
    ResultOutput    string  `json:"result_output,omitempty"`
    EvaluationScore *float64 `json:"evaluation_score,omitempty"`
}
```

---

## 4. 前端远程任务创建界面

### 4.1 任务类型选择

在创建任务弹窗/页面加一个 `task_type` 选择器（radio 或 dropdown）：
- `manual`（默认）：本地执行
- `scheduled`：定时执行（已有 scheduled tasks UI）
- `remote`：远程 Agent 领取

### 4.2 远程任务列表

新增 Tab 或 Filter：
```
GET /api/tasks?task_type=remote&status=pending
```
只展示 `task_type=remote && status=pending` 的任务，供远程 agent 轮询。

### 4.3 Agent 视角

Agent 侧 UI（如果是独立页面）：
- Dashboard：显示已 claim 的任务
- 领取新任务按钮（对应 claim API）
- 执行报告表单（对应 report API）

---

## 5. 实现优先级建议

| # | 内容 | 优先级 | 备注 |
|---|------|--------|------|
| 1 | models.go 加新字段 + 常量 | P0 | 其他一切的前置 |
| 2 | agents 表 + repo 层 | P1 | 注册/心跳依赖此 |
| 3 | Agent 注册 API | P1 |  |
| 4 | Agent 心跳 API + 超时检测 goroutine | P1 | 后台保活 |
| 5 | Task claim API | P2 | 原子操作防抢 |
| 6 | Task report API | P2 |  |
| 7 | 前端任务类型选择 | P2 |  |
| 8 | 前端远程任务列表 + Agent 界面 | P3 | 可最后做 |

---

## 6. 潜在风险

1. **并发抢任务**：claim 时需要 `UPDATE ... WHERE status='pending' AND task_type='remote'`，用 SQL 原子操作而非先查后改（避免 TOCTOU）
2. **Agent 伪造身份**：token 用 HMAC-SHA256 或 JWT，不要明文存密码
3. **心跳超时**：后台 goroutine 需要 stoppable（用 context），防止泄露
4. **远程 agent 失控**（不 report）：心跳超时后要将任务重新放回 pending 池

---

*调研完成时间：2026-06-15*