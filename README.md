# xworkbench v2 — All-in-One 个人工作台

> 单 Go 二进制 · 9 张表 · 5 大功能（链接 / 目录 / todo / 定时 / AI 任务）· 跨 macOS / Linux / Windows

融合 `xworkbench v1`（Go + 经验库 + PTY + 漂亮 UI）和 `ai-task-system v2.4`（Python 调度 + AI CLI 执行器），加上 5 个新功能，作为日常 homepage / launcher / 任务调度统一入口。

详见 [DESIGN.md](DESIGN.md)。

## 30 秒跑起来

```bash
git clone <repo>
cd xworkbench
go build -o xworkbench ./cmd/server
DB_PATH=./data/xworkbench.db ADDR=:8901 ./xworkbench
```

浏览器打开 [http://localhost:8901](http://localhost:8901)，看到 5 Tab + 5 widget 即可。

## 5 分钟 demo（5 个新功能）

### 1. 网页链接（左上 widget）
- 点 "**+ 添加**" → 填名称 + URL → 网格图标卡片
- 悬停卡片显示删除 ×
- 点击卡片外链新窗口

### 2. 目录快捷
- 点 "**+ 添加**" → 填名称 + 本地绝对路径（如 `~/code`）
- 列表显示，点击调系统资源管理器打开
  - macOS → Finder
  - Linux → xdg-open
  - Windows → Explorer

### 3. 定时任务（Automation Tab）
- 点 "**+ 新建定时任务**" → 填名称 + Cron + 类型 + Prompt
- 类型支持：
  - `shell`：`sh -c "<prompt>"`
  - `claude`：`claude --print --verbose --model <m> "<prompt>"`
  - `cbc`：`cbc -p --model <m> "<prompt>"`（无 cbc 时回落 `codebuddy`）
- Cron 支持标准 5 字段 + `@every 30s` / `@hourly` 等预设
- 顶部 "**▶ 启动**" 启动调度器，**"⏸ 停止**" / "**🔄 重载**" 控制
- 表格右侧 "**▶ 跑**" 立即触发一次（不依赖 cron）

### 4. todo.md（左下 widget）
- 点 "**设置**" → 配 todo.md 路径（如 `/Users/me/notes/todo.md`）
- 解析 `- [ ]` / `- [x]` 项
- 点击 checkbox 自动写回文件（先 `.bak` 再原子 rename）
- 修改单行不影响其他内容

### 5. 调度器（widget + Automation）
- 启动/停止/重载按钮 + 彩色徽章（绿点=运行中 / 灰点=停止）
- 创建/编辑/删除定时任务即时生效（自动重载 cron）
- 每次执行写 `executions` 表 + 推 WS `scheduled` 频道

## 三平台编译

```bash
# macOS（默认）
go build -o xworkbench ./cmd/server

# Linux
GOOS=linux GOARCH=amd64 go build -o xworkbench-linux ./cmd/server

# Windows
GOOS=windows GOARCH=amd64 go build -o xworkbench.exe ./cmd/server
```

| 平台 | PTY 终端 | AI Chat Tab | 调度器 | OpenDir |
|---|---|---|---|---|
| macOS | ✅ 真 PTY | ✅ 可用 | ✅ | ✅ open |
| Linux | ✅ 真 PTY | ✅ 可用 | ✅ | ✅ xdg-open |
| Windows | ❌ 503 | ⚠️ stub | ✅ | ✅ explorer |

## API 速查

```bash
# 任务
curl localhost:8901/api/tasks
curl -X POST localhost:8901/api/tasks -d '{"title":"x","description":"y"}' -H "Content-Type: application/json"
curl -X POST localhost:8901/api/tasks/{id}/run -d '{"command_type":"shell","prompt":"echo hi"}' -H "Content-Type: application/json"

# 5 个新功能
curl localhost:8901/api/web-links
curl -X POST localhost:8901/api/web-links -d '{"name":"GitHub","url":"https://github.com/xiaodongQ/"}' -H "Content-Type: application/json"
curl -X POST localhost:8901/api/dir-shortcuts -d '{"name":"code","path":"~/code"}' -H "Content-Type: application/json"
curl -X POST localhost:8901/api/dir-shortcuts/{id}/open    # 调系统资源管理器
curl -X POST localhost:8901/api/scheduled -d '{"name":"heartbeat","cron_expr":"@every 30s","command_type":"shell","prompt":"echo tick","enabled":true}' -H "Content-Type: application/json"
curl -X POST localhost:8901/api/scheduler/start
curl -X PUT localhost:8901/api/todo/path -d '{"path":"/path/to/todo.md"}' -H "Content-Type: application/json"
curl -X PUT localhost:8901/api/todo/1 -d '{"done":true}' -H "Content-Type: application/json"  # line 1 勾选

# WebSocket
wscat -c ws://localhost:8901/ws
```

完整端点列表见 [DESIGN.md §5](DESIGN.md#5-api30-端点)。

## 文档导航

- [DESIGN.md](DESIGN.md) — 架构设计 + 实施历程
- [docs/CLI.md](docs/CLI.md) — claude / cbc / shell 命令格式
- [docs/MIGRATION.md](docs/MIGRATION.md) — v1 → v2 迁移指南
- [docs/SCHEDULER.md](docs/SCHEDULER.md) — Cron 语法 + 跨平台调度

## 测试

```bash
go test ./...  # 6 包全通过
```

## 依赖

- Go 1.22+（stdlib mux 模式匹配 ≥ 1.22）
- 运行时：可选 `claude` / `codebuddy` / `cbc` CLI（在 PATH 中）
- SQLite 无 CGO，纯 Go `modernc.org/sqlite`

## 数据备份

`data/` 目录保留 6 份历史快照：
- `xworkbench.db.v1.bak` — v1 旧库
- `xworkbench.db.v2.fresh.db` — v2 全新 schema 空库
- `xworkbench.db.v2.phase2.db` / `phase3.db` / `phase3-ui.db` / `phase4-modal.db` — 各阶段验证库
