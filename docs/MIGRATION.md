# v1 → v2 迁移指南

## 数据层变更

### tasks 表（24 字段，10 个新增）

| 新增字段 | 类型 | 默认 | 来源 |
|---|---|---|---|
| `priority` | INTEGER | 5 | v2.4 抢占权重 |
| `started_at` | DATETIME | NULL | v2.4 |
| `completed_at` | DATETIME | NULL | v2.4 |
| `executor_model` | TEXT | NULL | v2.4 claude model |
| `cbc_model` | TEXT | NULL | v2.4 cbc model |
| `iteration_count` | INTEGER | 0 | v2.4 |
| `max_iterations` | INTEGER | 20 | v2.4 |
| `improvement_threshold` | REAL | NULL | v2.4 |
| `last_heartbeat` | DATETIME | NULL | v2.4 |
| `last_error` | TEXT | NULL | v0.1 自加 |

### 新增 6 张表

```sql
executions       v2.4 一次执行的 stdout/stderr/exit_code/command/source
evaluations      v2.4 LLM 打分（0-10 + comments）
web_links        v0.1 新功能 1
dir_shortcuts    v0.1 新功能 2
scheduled_tasks  v0.1 新功能 3+5（cron 表达式 + claude/cbc/shell）
app_settings     v0.1 KV（todo_md_path / timezone / default_model）
app_meta         v0.1 KV（user_version=2）
```

## 自动迁移机制

`InitSchema` 调用 `migrateTasksColumns`：

```go
rows, _ := db.Query(`PRAGMA table_info(tasks)`)
// 探测已有列名
// 对每个 v2 新字段，如果不存在就 ALTER TABLE ADD COLUMN
```

旧 v1 db 不丢数据，新字段默认 NULL / 默认值。`PRAGMA user_version = 2` 标记已升级。

## 手动迁移步骤（如果自动失败）

```bash
# 1. 备份
cp data/xworkbench.db data/xworkbench.db.v1.bak

# 2. 启动新版本（自动建表 + 迁移）
go build -o xworkbench ./cmd/server
DB_PATH=./data/xworkbench.db ADDR=:8902 ./xworkbench
# 9 张表自动建好，旧数据保留

# 3. 验证
sqlite3 data/xworkbench.db "PRAGMA user_version;"   # 应该是 2
sqlite3 data/xworkbench.db ".tables"                  # 应该看到 9 张表
```

## 完全重建（不保留数据）

```bash
rm data/xworkbench.db
./xworkbench  # 启动时自动建空 db
```

或从零开始（plan § 6.4 MIGRATION 推荐路径）：

```bash
rm data/xworkbench.db
go build -o /tmp/xworkbench ./cmd/server
DB_PATH=/tmp/fresh.db /tmp/xworkbench
```

## API 兼容性

### 兼容（无破坏）

- `GET /api/tasks` / `POST /api/tasks` / `GET /api/tasks/{id}` / `PUT /api/tasks/{id}/status`
- `GET /api/experiences` / `POST` / `GET /{id}`
- `GET /api/stats` / `GET /api/pty` / `GET /`

### 新增

- `POST /api/tasks/{id}/run` / `/cancel` / `/executions`
- 5 个新功能的所有端点（见 [DESIGN.md §5](DESIGN.md#5-api30-端点)）
- WebSocket `/ws`

### 不变

- 旧 `tasks` 字段保持兼容
- 旧 `experiences` 字段完全保留

## 旧 ai-task-system v2.4 Python 数据

**默认不导入**（plan 决策：从零开始）。如需导入，方法：

```sql
-- 1. 把 v2.4 的 db 拷到 v2 同目录
cp ../ai-task-system/v2.4/v2.4.db data/v24-import.db

-- 2. 用 sqlite3 导出
sqlite3 data/v24-import.db ".dump tasks executions" > v24-dump.sql

-- 3. 编辑 v24-dump.sql：去掉 CREATE TABLE、PRAGMA 等；保留 INSERT
-- 4. 注入到 v2
sqlite3 data/xworkbench.db < v24-dump.sql
```

⚠️ 注意：v2.4 的 task.id 格式可能不同（UUID vs 自增 INT），导入后建议用 v2 的 `migrateTasksColumns` 重新对齐。
