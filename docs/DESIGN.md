# xworkbench v2 — All-in-One 个人工作台

> v2 = xworkbench（v1 Go 单体）+ ai-task-system v2.4（Python 调度/执行）+ 5 个新功能（链接/目录/todo/定时/AI 任务）
> 单 Go 二进制 / 9 张表 / 30+ API / 跨 macOS·Linux·Windows

## 1. 演进背景

| 阶段 | 形态 | 短板 |
|---|---|---|
| **v1** | xworkbench：Go 单体 + 经验库 + PTY + 5 Tab | 无调度器、无任务执行引擎、无超时重试 |
| **v2.4 旧** | ai-task-system：Python FastAPI + 调度器 + AI CLI | 无经验库、无 TDD 验收、PTY 缺失、无定时触发 |

**v2 目标**：单 Go 二进制，融合两边强项 + 加 5 个新功能，做日常 homepage/launcher。

## 2. 实施历程

| Phase | 工作量 | 关键产出 |
|---|---|---|
| Phase 1 | 1-3 天 | 9 张表 + 字段迁移（PRAGMA user_version=2） |
| Phase 2 | 4-6 天 | executor 包（BuildCommand / Run 流式 / 4 种确认信号）+ hub + ExecutionRepo + 任务执行 API |
| Phase 3 | 7-10 天 | 5 个新 repo + shortcuts + todo + scheduler + 25 handler + 5 widget + 4 form modal |
| Phase 4 | 11-12 天 | pty build tag 隔离（pty_unix/pty_windows）+ 跨三平台编译 |
| Phase 5 | 13 天 | 文档 |

## 3. 数据模型（9 张表）

`./data/xworkbench.db`（SQLite，无 CGO 跨平台）

| 表 | 来源 | 用途 |
|---|---|---|
| **tasks** | v1 + v2.4 合并 | 任务（24 字段：title/description/status/priority/started_at/executor_model/last_heartbeat...） |
| **experiences** | v1 | 经验库（模块/关键词/日志路径/工具/场景/样例/代码片段） |
| **skill_versions** | v1 | 任务迭代产生的 Skill 版本（accuracy/iter_count） |
| **executions** | v2.4 | 一次执行的 stdout/stderr/exit_code/command/source |
| **evaluations** | v2.4 | LLM 打分（0-10 + comments） |
| **web_links** | 新功能 1 | 主页快捷链接 |
| **dir_shortcuts** | 新功能 2 | 目录快捷（点击调系统资源管理器） |
| **scheduled_tasks** | 新功能 3+5 | 定时任务（cron / @every） |
| **app_settings** | 新功能 4 | KV（todo_md_path / default_model / timezone） |
| **app_meta** | 通用 | KV（user_version 等） |

字段迁移：`InitSchema` 内 `migrateTasksColumns` 用 `PRAGMA table_info` 探测后 `ALTER TABLE ADD COLUMN`，旧 db 不丢数据。

## 4. 后端模块

```
internal/
  backend/      models + InitSchema + 8 个 repo（Task/Experience/Execution/Evaluation/WebLink/DirShortcut/Scheduled/AppSettings）
  executor/     Run(ctx, cmd, onChunk) 流式 + 超时
  executor/runner/  BuildCommand(typ, model, session, prompt)  // claude/cbc/shell
  executor/confirm.go  NeedsUserInput + ParseConfirmRequest  // 4 种信号
  hub/          WebSocket 广播中心
  wsmsg/        6 频道常量 + Message struct
  scheduler/    robfig/cron 包装 + Reload/Start/Stop/RunNow
  shortcuts/    OpenDir 跨平台（darwin/linux/windows）
  todo/         Parse + ReadAndParse + ToggleAndWrite
```

### 4.1 CLI 命令构造（移植 cli_executor.py:26-56）

```go
BuildCommand(typ, model, sessionID, prompt) ([]string, error)
// claude: claude --print --verbose [--model ...] [--session-id ...] "<prompt>"
// cbc:    cbc -p [--model ...] "<prompt>"  // PATH 中无 cbc 时回落 codebuddy
// shell:  sh -c "<prompt>"
```

### 4.2 4 种人工确认信号（移植 cli_executor.py:62-115）

`NeedsUserInput` 检测 18 个中英文信号（`?`/`[Y/n]`/`请确认`/`是否要`/`Continue?`...），`ParseConfirmRequest` 正则提取 `{confirm_type:...}`。

### 4.3 调度器

`robfig/cron/v3` 跨平台（不依赖 OS scheduler），`Reload` 重建 cron 引擎从 DB 加载 enabled=1 任务，每次触发写 `executions`（source='scheduled'）并通过 `wsmsg.ChannelScheduled` 推 WS。

## 5. API（30+ 端点）

```
GET    /api/tasks              支持 status/offset/limit 过滤
POST   /api/tasks
GET    /api/tasks/{id}
PUT    /api/tasks/{id}/status
POST   /api/tasks/{id}/run                 // 立即执行（command_type+prompt from body or task）
POST   /api/tasks/{id}/cancel              // kill 子进程
GET    /api/tasks/{id}/executions
GET    /api/executions                      // 最近 50 条
GET    /api/experiences                     // 支持 module 模糊
POST   /api/experiences
GET    /api/experiences/{id}
GET    /api/stats
GET    /api/pty                             // macOS/Linux 真 PTY，Windows 503
GET    /ws                                  // WebSocket 升级

# 5 个新功能
GET/POST/PUT/DELETE  /api/web-links[/{id}]
GET/POST/PUT/DELETE  /api/dir-shortcuts[/{id}]
POST                 /api/dir-shortcuts/{id}/open    // 跨平台 OpenDir
GET/POST/GET/PUT/DELETE/POST  /api/scheduled[/{id}]   // 含 /run-now
POST/POST/GET/POST  /api/scheduler/{start,stop,status,reload}
GET/PUT/PUT/GET      /api/todo[/{line_no}/path]      // toggle 走 URL line_no
GET/PUT              /api/settings[/{key}]
```

WS 6 频道：`scheduler / task / exec / scheduled / shortcut / todo`

## 6. 前端（embed.FS 嵌入单 HTML）

- 5 Tab：Dashboard / Tasks / Experiences / Automation / AI Chat
- **Dashboard Tab** 追加 12 列 widget grid（5 widget：链接 / 目录 / todo / scheduler 徽章+启停 / 最近 executions）
- **Automation Tab** 完整重写：scheduler 启停按钮 + 定时任务表（CRUD + ▶ 立即跑）+ 最近 executions 列表
- 4 个 form modal（link/dir/todo-path/scheduled）替代原 prompt()
- `gorilla/websocket` 客户端 6 频道分发
- xterm.js 终端（macOS/Linux）

## 7. 跨平台

| 平台 | 编译命令 | 二进制 | 备注 |
|---|---|---|---|
| macOS | `go build` | 15.0 MB | 真 PTY |
| Linux | `GOOS=linux go build` | 15.5 MB | 真 PTY |
| Windows | `GOOS=windows go build` | 15.9 MB | PTY stub（返回 503） |

`creack/pty` 通过 `//go:build !windows` 隔离；`internal/shortcuts/open.go` 用 `runtime.GOOS` 切 `open`/`xdg-open`/`explorer`；调度器和 executor 走 `os/exec` 天然跨平台（Windows 自动用 `TerminateProcess`）。

## 8. 验收

- `go test ./...` 6 包全通过（executor / runner / shortcuts / todo / experience / task）
- 三平台 `go build` 全 exit=0
- 端到端：创建链接 + 调度器启动 + `@every 5s` 定时任务 → 7s 内触发 N 次 → executions 表 source=scheduled + last_status=success
- 6 份 db 备份保留在 `data/`（v1 → v2 fresh → phase2/3/3-ui/4-modal）

## 9. 仍欠

- evaluator LLM 打分（plan § 10 标注 v0.1 不做）
- `/api/tasks/{id}/submit-input`（人工确认输入，detect 已就位，路由未加）
- Windows Service 注册（`kardianos/service`，可后续补）

## 10. 目录结构

```
xworkbench/
├── cmd/server/
│   ├── main.go            HTTP 入口（30+ 路由 + 装配 9 个 repo）
│   ├── pty.go             PTY Unix 实现（//go:build !windows）
│   ├── pty_windows.go     PTY Windows stub（503）
│   └── index.html         45 KB 单页 SPA（5 Tab + 5 widget + 4 modal）
├── internal/
│   ├── backend/           models.go + repo.go（9 张表 + 8 repo + InitSchema + 迁移）
│   ├── executor/          exec.go + confirm.go + executor_test.go
│   ├── executor/runner/   build.go + runner_test.go
│   ├── hub/               hub.go
│   ├── wsmsg/             types.go
│   ├── scheduler/         scheduler.go
│   ├── shortcuts/         open.go + open_test.go
│   ├── todo/              parser.go + parser_test.go
│   ├── task/              task_test.go
│   └── experience/        experience_test.go
├── docs/
│   ├── CLI.md             claude / cbc / shell 命令格式
│   ├── MIGRATION.md       v1 → v2 迁移指南
│   └── SCHEDULER.md       cron 语法 + 跨平台
├── data/                  SQLite + 6 份备份
├── go.mod
├── DESIGN.md              本文档
└── README.md              启动 + 5 功能 demo
```
