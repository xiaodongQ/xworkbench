# 调研：Task 审计日志 + 任务依赖

## 1. 现状

xworkbench 当前没有审计日志，任务状态变更只在内存中流转、数据库 UPDATE 后没记录。
任务之间也没有依赖关系，所有 pending 任务平铺，agent 不知道应该先做哪个后做哪个。

**这两个特性都值得借鉴：**
- 审计日志：GitHub PR 事件流、Datadog 审计、Kubernetes audit log
- 任务依赖：Airflow DAG、Temporal workflow、Jira 子任务

## 2. 设计

### 2.1 审计日志（task_events）

**表结构：**
```sql
CREATE TABLE task_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL,
    event_type TEXT NOT NULL,  -- created | claimed | released | reported | timeout | heartbeat_lost
    actor TEXT,                -- agent_id | user_id | "system"
    payload TEXT,              -- JSON 附加信息
    created_at DATETIME
);
CREATE INDEX idx_task_events_task ON task_events(task_id, created_at DESC);
```

**API：**
```
GET /api/tasks/{id}/events      # 任务时间线（按时间倒序）
GET /api/agents/{id}/events     # Agent 视角的审计
```

**埋点位置：**
- `handleTaskCreate` → event_type=created
- `TaskRepo.ClaimTask` → event_type=claimed
- `TaskRepo.ReportTask` → event_type=reported (status 变更)
- 心跳 checker → event_type=heartbeat_lost
- 任务超时 → event_type=task_timeout

### 2.2 任务依赖（task_dependencies）

**表结构：**
```sql
CREATE TABLE task_dependencies (
    task_id TEXT NOT NULL,         -- 依赖方（这个任务）
    depends_on TEXT NOT NULL,      -- 被依赖方（前置任务）
    type TEXT DEFAULT 'hard',      -- hard（必须完成才可 claim） | soft（建议但不强求）
    created_at DATETIME,
    PRIMARY KEY (task_id, depends_on)
);
```

**业务规则：**
- 任务可被 claim 的前置条件：所有 `hard` 依赖任务 status=archived
- 任务 status=pending 时，如果有未完成的 hard 依赖 → 不出现在 remote agent 视角的 pending 列表
- 任务可被删除的前提：没有其他任务依赖它（避免悬空依赖）

**API：**
```
POST /api/tasks/{id}/dependencies
Body: { "depends_on": "task-xxx", "type": "hard" }
Response: 201 Created

GET /api/tasks/{id}/dependencies    # 列出当前任务依赖
GET /api/tasks/{id}/dependents      # 列出依赖当前任务的下游

DELETE /api/tasks/{id}/dependencies/{dep_id}
```

**ClaimTask 增强：**
```sql
UPDATE tasks SET status='in_progress', ...
WHERE id=? AND status='pending' AND task_type='remote'
  AND NOT EXISTS (
    SELECT 1 FROM task_dependencies d
    WHERE d.task_id = tasks.id AND d.type='hard'
      AND d.depends_on IN (SELECT id FROM tasks WHERE status != 'archived')
  )
```

## 3. 实现优先级

1. **审计日志**（先做，影响小，价值大）
2. **任务依赖**（后做，业务影响大，需要 e2e 充分验证）

## 4. 风险点

- 审计日志表会随时间膨胀 → 需要考虑定期归档（先不做）
- 任务依赖形成环 → 插入时检测（DFS）
- 任务依赖级联删除 → 删 task 时清理 dependencies 行

## 5. 借鉴来源

- **Celery** - event/audit 模式
- **Airflow** - task dependencies 表达
- **GitHub Issues** - timeline 视图
- **Linear** - 简洁的关系型 API
