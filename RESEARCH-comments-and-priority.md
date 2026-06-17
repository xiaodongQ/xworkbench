# 调研：任务评论 + 任务优先级队列

> 状态：v2（2026-06-17 更新）
> 落地情况：两个能力均已实装到 `da66abf`，本文档保留作为设计记录。

## 1. 价值

**任务评论（Comments）**
- 借鉴：GitHub PR/Issue 评论、Jira 评论
- 价值：任务执行过程中需要协作/讨论/记录，AI agent 多轮思考、人类反馈、错误诊断都要有个落点

**任务优先级队列（Priority Queue）**
- 借鉴：Celery priority queue、AWS SQS priority、Linear priority
- 价值：远程 agent 当前是 FIFO 抢任务，重要任务可能卡在队列尾

## 2. 设计

### 2.1 任务评论（task_comments）

**表结构**（`internal/backend/repo.go` schema 内联）：

```sql
CREATE TABLE IF NOT EXISTS task_comments (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    author TEXT NOT NULL,           -- user / agent:xxx / system
    content TEXT NOT NULL,
    mentions TEXT,                  -- JSON: [@user1, @user2]
    parent_id TEXT,                 -- 嵌套回复
    created_at DATETIME,
    updated_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_task_comments_task ON task_comments(task_id, created_at DESC);
```

**API 路由**（`cmd/server/main.go:259-262`）：

```
GET    /api/tasks/{id}/comments   # 列表（按 created_at ASC, id ASC）
POST   /api/tasks/{id}/comments   # 新建（author 缺省 "user"）
PUT    /api/comments/{id}         # 编辑
DELETE /api/comments/{id}         # **硬删**（见 §4 偏差）
```

**配套动作**：创建/更新会写 `task_events` 审计（`event_type=commented`），并 dispatch `task.commented` webhook。

**Go 模型**（`internal/backend/repo.go:2149`，**升级兼容**约定）：

```go
type TaskComment struct {
    ID        string    `json:"id"`
    TaskID    string    `json:"task_id"`
    Author    string    `json:"author"`
    Content   string    `json:"content"`
    Mentions  string    `json:"mentions,omitempty"`  // JSON 数组
    ParentID  string    `json:"parent_id,omitempty"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

- 可选字段全部 `omitempty`，老客户端解析新结构不会因新增字段爆
- 表用 `CREATE TABLE IF NOT EXISTS`，旧库自动跳过

### 2.2 评论是否喂给 AI

**结论（2026-06-17 与用户确认）：评论触发 AI 暂不做。**

- agent 执行任务时，**不会**自动把 `task_comments` 拼进 prompt
- 调用方（main agent / 人）若需把评论当上下文，由调用方主动 `GET /api/tasks/{id}/comments` 拉，再塞给执行 agent
- 不做 @mention 自动响应、不推 system event、不开 `comment.mentioned` webhook
- `mentions` 字段先保留（schema 占位），但当前无任何逻辑读它
- 后续若需要做，单独开一份调研（@mention 路由、agent 身份识别、防循环触发等）

### 2.3 任务优先级

**现状**：tasks 表已有 `priority` 字段（INTEGER DEFAULT 5），但 List 没按它排，`ClaimTask` 也不管优先级。

**改造**（`cmd/server/main.go:2793` `handleTaskClaimNext` + `internal/backend/repo.go:2232` `NextClaimable`）：

- `List` 默认按 `priority DESC, created_at ASC` 排序（任务列表页）
- 新增 `POST /api/tasks/claim-next`：agent 调一下自动领到最高优先级的 pending remote 任务
  - 排除条件：status=pending、task_type=remote、未被 claim、有未完成 hard 依赖的任务
  - 找不到 → 204 No Content
  - agent 需开启 `auto_claim_enabled` 才允许使用
- `ClaimTask` 行为不变（agent 仍可指定 task_id）
- 排序规则：`priority DESC, created_at ASC`（同优先级时 FIFO，老任务优先）

**API**：

```
POST /api/tasks/claim-next
Header: Authorization: Bearer <token>
Body: { "agent_id": "xxx" }
Response: { "status": "claimed", "task": {...} } 或 204 No Content
```

**审计/Webhook**：
- `task_events`：event_type=`claimed_via_priority`，actor=`agent:<id>`
- webhook：`task.claimed`，payload 包含 `via: "claim-next"`

## 3. 优先级（落地排序）

1. **优先级队列**（已落地，`NextClaimable` + `handleTaskClaimNext`）
2. **任务评论**（已落地，4 个 API + 审计 + webhook）

## 4. 风险 / 文档 vs 实现偏差

- ❌ ~~评论编辑历史需要 schema 变更~~：本版本不做
- ❌ ~~评论触发 AI~~：本版本不做
- ❌ ~~软删（保留 + 标记）~~：**实际是硬删**（`DELETE FROM task_comments`），与调研方案不符
  - 现状：DELETE 直接物理删除
  - 影响：误删无法恢复，但实现简单、节省存储
  - 后续若需要软删：`ALTER TABLE task_comments ADD COLUMN deleted_at DATETIME` + DELETE 改为 UPDATE，加索引，handler 返回前过滤
- ✅ ~~priority 修改要触发 `task.priority_changed` webhook~~ **已实现**（`handleTaskUpdate` 检测 `req.Priority != nil && *req.Priority != oldPriority` 后 dispatch，payload=`{task_id, old, new}`）
  - 顺手修了：原 `TaskRepo.Update` SQL 漏写 priority 字段，导致 priority 只能创建不能修改（隐含 bug）
  - 指针语义：`req.Priority *int`，未传 = nil = 不更新不触发，传 0 = 显式设为 0
  - 交付：命令 `go test ./cmd/server/ -run TestHandleTaskUpdate_Priority -v`
- claim-next 和 claim 互斥：claim-next 拿的是**任意一个**最高优先级任务，claim 是指定任务 ID，两条路径都走 `ClaimTask` 走 DB 事务，并发安全

## 5. 升级兼容性（已落实）

| 维度 | 措施 |
|------|------|
| Go struct | 可选字段全 `omitempty`，新增字段不影响老客户端解析 |
| SQLite schema | `CREATE TABLE IF NOT EXISTS` + `CREATE INDEX IF NOT EXISTS`，旧库自动跳过 |
| 增量迁移 | `migrateTasksColumns` / `migrateScheduledTasksColumns` 等 ALTER TABLE 模式，单独加列不会重写整表 |
| Webhook 事件 | 新事件类型（如 `task.commented`）不破坏老订阅方，老 webhook 配置仍可继续工作 |
| API 路径 | 全部新增路径（`/api/tasks/{id}/comments`、`/api/comments/{id}`、`/api/tasks/claim-next`），未改动老路径 |

## 6. 借鉴来源

- **GitHub Discussions API** - 评论/回复/编辑
- **Celery Task Priority** - 优先级字段
- **Linear Issue Priority** - 4 档（Urgent/High/Medium/Low）
