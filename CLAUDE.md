# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# xworkbench · Claude 项目背景卡

> 此文件在 Claude 进入此项目时自动加载,避免每次重新分析。修改项目结构时同步更新本文件。

## 1. 项目定位

**All-in-One 个人工作台**: 单 Go 二进制 + SQLite,跨 macOS/Linux/Windows,融合 task / experience / scheduled / AI evaluation,6 Tab(总览/任务/经验库/自动化/AI 对话/代理)+ 5 widget(链接/目录/todo/调度/最近执行)。

- 入口: [README.md](README.md)
- 设计文档: [docs/DESIGN.md](docs/DESIGN.md)
- 子文档: [docs/CLI.md](docs/CLI.md) / [docs/SCHEDULER.md](docs/SCHEDULER.md) / [docs/MIGRATION.md](docs/MIGRATION.md) / [docs/relay-proxy-design.md](docs/relay-proxy-design.md)
- 计划/回顾: [docs/superpowers/plans/](docs/superpowers/plans/)

## 2. 技术栈

| 维度 | 选择 |
|---|---|
| 语言 | Go 1.25 |
| 存储 | SQLite,纯 Go `modernc.org/sqlite`(无 CGO) |
| HTTP | Go 1.22+ stdlib `mux`(path pattern 匹配,如 `GET /api/tasks/{id}`) |
| WebSocket | `gorilla/websocket` |
| PTY | `creack/pty`(Unix 真 PTY;Windows ConPTY stub 返回 503) |
| 调度 | `robfig/cron/v3`(进程内,跨平台,不依赖 OS scheduler) |
| 前端 | vanilla JS + CSS(无框架,1 HTML + 6 view JS + widgets + api) |
| 嵌入资源 | `//go:embed index.html static`(F5 刷新即生效,无 hash 缓存) |

## 3. 规模(随时会变,以代码为准)

- 41 个 .go 文件,~6882 行
- 8 个 JS 文件(6 view + api + widgets) + 1 个 CSS + 1 个 index.html
- 11 张 SQLite 表
- 65 个 HTTP 路由(`grep -c "HandleFunc\|Handle(" cmd/server/main.go`)
- 1 个 WebSocket 频道(`/ws`,6 业务频道:scheduler/task/exec/scheduled/shortcut/todo)

## 4. 配置体系(2026-06 重构后,记清)

两套并存,**互不混用**:

| 维度 | 存哪 | 改的入口 | 说明 |
|---|---|---|---|
| 终端类型 + 模型列表 | `./config.json` | `internal/config` (`Load/Save/LoadFromPath`) | 配置驱动,启动读;运行时改 → `PUT /api/config` 回写 |
| 用户级 KV(`aichat_default_cli` / `default_terminal` / `todo_md_path` / `scheduler.enabled` / ...) | SQLite `app_settings` 表 | `internal/backend.AppSettingsRepo` | `GET /api/settings` / `PUT /api/settings/{key}` |
| 路径/常量 | `internal/paths` | 编译期 | context 目录、data 目录、bin 目录的统一入口 |

**重要**:`aichat_default_cli` 是 app_settings 里,**不在** config.json。前端 aichat.js 的 `loadCliSetting()` 走 `/api/settings` 拿。改 CLI 默认值要走 `PUT /api/settings/aichat_default_cli`,不是改 config.json。

`config.json` 结构(`internal/config/config.go`):
```json
{
  "terminal": {
    "default_type": "wezterm",
    "detect_paths":  { "wezterm": ["/Applications/WezTerm.app/Contents/MacOS/WezTerm"] },
    "types": { "wezterm": {"bin": "wezterm", "args": [...], "name": "WezTerm", "plate": "all", "path": "..."} }
  },
  "models": {
    "claude": { "default": "sonnet", "options": [{"value":"sonnet","label":"..."}] },
    "cbc":    { "default": "glm-5.0", "options": [...] }
  }
}
```

## 5. 目录结构

```
cmd/server/
├── main.go              # 入口 + 所有 HTTP handler(APIServer struct 包含所有 repo)
├── pty.go / pty_windows.go  # PTY build tag 隔离
├── *_test.go            # httptest 集成测试(terminal_api_test.go 等)
├── index.html           # 6 Tab 单页
└── static/{css,js}/
    ├── api.js           # fetchJSON + switchTab + 主题 + tooltip + CLI_MODELS 缓存
    ├── widgets.js       # 5 widget(链接/目录/todo/dashboard/调度)
    └── views/{dashboard,tasks,experiences,automation,aichat,relay}.js

internal/
├── backend/             # 数据层(11 张表 repo + models)
│   ├── models.go        # 所有 struct
│   ├── repo.go          # 所有 DDL + Repo 方法(InitSchema 唯一来源)
│   └── httplog/         # 独立包?实际在 internal/httplog
├── config/              # 2026-06 新增:config.json 加载/保存 + 类型定义
├── paths/               # 路径常量(context/data/bin 目录)
├── executor/            # 子进程执行
│   ├── exec.go          # 同步 Run + 流式 onChunk 回调
│   ├── confirm.go       # 18 种确认信号检测
│   └── runner/build.go  # BuildCommand(claude / cbc / shell)
├── evaluator/           # LLM-as-a-Judge(AI 任务打分,evalPromptTpl + parseEval)
├── scheduler/           # cron 包装(Reload/Start/Stop/RunNow)
├── experience/          # 知识库纯领域(测试)
├── hub/                 # WebSocket Hub
├── wsmsg/               # 6 频道常量 + Message struct
├── httplog/             # HTTP 日志中间件
├── shortcuts/           # OS 资源管理器 + 终端打开(跨平台)
├── todo/                # 解析 todo.md
├── task/                # 任务纯领域(测试)
└── relay/               # 2026-06 新增:Exec 命令执行 + HTTP 代理转发
```

## 6. 数据模型(11 张表)

| 表 | 关键字段 | 说明 |
|---|---|---|
| `tasks` | id, title, description, status, priority, experience_id, resources, acceptance | 任务(24 字段,带自动迁移) |
| `task_experiences` | task_id, experience_id | 多对多关联(2026 新增,取代单 experience_id) |
| `executions` | id, task_id, scheduled_task_id, source, command, model, started_at, completed_at, output, error, exit_code, **evaluation_score** | 一次执行 |
| `evaluations` | id, task_id, execution_id, evaluator_model, score, comments, created_at | AI 评估结果 |
| `experiences` | id, module, scene, keywords, tool_usage, log_samples, code_snippets | 知识库 |
| `scheduled_tasks` | id, name, cron_expr, command_type, model, prompt, enabled, last_run_at, last_status | 定时任务 |
| `web_links` / `dir_shortcuts` / `app_settings` / `app_meta` / `skill_versions` | | |

字段迁移:`InitSchema` 内 `migrateTasksColumns` 用 `PRAGMA table_info` 探测后 `ALTER TABLE ADD COLUMN`,旧 db 不丢数据。

## 7. 6 Tab + 5 widget

| Tab | 后端路由 | 前端 |
|---|---|---|
| **总览**(dashboard) | `/api/stats` | `views/dashboard.js` |
| **任务**(tasks) | `/api/tasks/*` | `views/tasks.js` |
| **经验库**(experiences) | `/api/experiences/*` | `views/experiences.js` |
| **自动化**(automation) | `/api/scheduled/*` + `/api/scheduler/*` + `/api/executions/*` + `/api/evaluations` | `views/automation.js` |
| **AI 对话**(aichat) | `/api/pty`, `/ws` | `views/aichat.js`(多 Tab PTY) |
| **代理**(relay,2026 新增) | `/api/exec` + `/api/proxy/*` + `/api/relay/*` | `views/relay.js` |

5 widget(首页卡片):web-links / dir-shortcuts / todo.md / scheduled-summary / recent-executions。

## 8. AI 任务执行链路(核心)

### 8.1 执行流
```
用户点"▶ 运行" → POST /api/tasks/{id}/run
  → handleTaskRun 构造 prompt(BuildTaskPrompt 注入 task + experience)
  → runner.BuildCommand("claude", model, "", prompt, WithActionReport())
  → executor.Run 同步跑子进程(claude -p --output-format json)
  → executions 表写记录(开始/完成时间 + output + exit_code)
  → WS 广播 'executions' 频道
```

### 8.2 评估流
```
用户点"📊 AI 评估" → POST /api/executions/{id}/evaluate
  → handleExecutionEvaluate 构造评估 prompt(evalPromptTpl:指令 vs AI 自报动作清单 vs Claude 元数据 vs 实际 stdout 四方对照)
  → evaluator.Evaluate 异步调 claude 打分
  → parseEval 提取 "评分: X" + "评语: ..." (scoreRe / cmtRe)
  → evaluations 表写记录(Score=-1 = 解析失败,前端显示灰卡;0 = 真低分)
  → 前端 1-60s 轮询,渲染评分卡
```

### 8.3 Prompt 模板
- **执行 prompt** `BuildTaskPrompt(task, experience)`:注入 5 个 task 字段(title/description/priority/resources/acceptance)+ 7 个 experience 字段(module/keywords/log_paths/tool_usage/scene/log_samples/code_snippets)
- **评估 prompt** `evalPromptTpl`:要求基于"指令 vs AI 自报动作清单 vs Claude 执行元数据(num_turns) vs 实际 stdout"四方对照打分

## 9. AI CLI 命令构造(`internal/executor/runner/build.go`)

```go
BuildCommand(typ, model, sessionID, prompt) ([]string, error)
// claude: claude -p --output-format json [--model m] [--session-id sid] "<prompt>"
// cbc:    cbc -p [--model m] "<prompt>"  // PATH 无 cbc 时回落 codebuddy
// shell:  sh -c "<prompt>"
```

`WithActionReport()`:给 claude/cbc prompt 末尾追加"动作清单"格式要求(便于 evaluator 验证是否真调了工具)。**evaluator 不传**,否则自指。

`determineAICmd`(`pty.go`)根据 `aichat_default_cli` 决定 PTY 起哪个 CLI(codex/cbc/shell/claude)。

## 10. 关键约定

### 10.1 评估模型
默认 `sonnet`(前端 `/api/evaluations` 渲染 / 后端 `main.go:476` fallback 都是),UI 可在 exec-detail-modal 下拉选 haiku/sonnet/opus。**模型列表来自 `config.json` 的 `models.claude.options`**,不写死。

### 10.2 端口 / DB / 配置路径
- 默认端口 `:8901`(避免 8080 冲突),环境变量 `ADDR` 可覆盖;`scripts/run.sh --port 9090` 也行
- DB 路径 `DB_PATH` 环境变量,默认 `./data/xworkbench.db`
- 配置路径 `-config ./config.json` flag(`internal/config.LoadFromPath`)

### 10.3 解析失败 vs 真低分
`Evaluation.Score = -1` 表示评估员输出无法解析(前端显示"解析失败"灰卡),`0` 表示真低分。**不要 fallback 到 0**。

### 10.4 调度器超时
`executor.Run` 默认 30 分钟 ctx 超时。`scheduled_tasks.last_status` 取值:`success` / `failed` / `timeout` / `build_error`(如 cbc/codebuddy 都不在 PATH)。

### 10.5 PTY 多 Tab
`/api/pty?tab_id=...` 每个 tab 独立 WebSocket + xterm.js。`tabRegistry: id -> {id, name, term, ws, needsAuth, wsConnected}`(`aichat.js`),最多 5 Tab。`needsAuth` 由 `authRequiredPatterns` 18 种中英文信号检测(Approve/[Y/n]/请确认/...)驱动 UI 红点。

## 11. 常用命令

```bash
# 编译(推荐用脚本,带 -s -w -trimpath)
./scripts/build.sh                    # 当前平台 → ./bin/xworkbench
./scripts/build.sh -a                 # 三平台全量 → ./bin/xworkbench-{darwin,linux,windows}
go build -o xworkbench ./cmd/server   # 直接 go build 也行(不推荐,没去符号表)

# 运行(脚本版,自带 pid + log + restart)
./scripts/run.sh                      # 启动,二进制不存在时自动 build
./scripts/run.sh --stop | --restart | --log | --status
./scripts/run.sh --port 9090          # 自定义端口

# 运行(直跑版,调试用)
DB_PATH=./data/xworkbench.db ADDR=:8901 ./bin/xworkbench -config ./config.json

# 三平台交叉编译
GOOS=linux  GOARCH=amd64 go build -o xworkbench-linux  ./cmd/server
GOOS=windows GOARCH=amd64 go build -o xworkbench.exe    ./cmd/server

# 测试
go test ./...                         # 全量,~4-6s
go test -v ./internal/evaluator/      # 单包
go test -run TestName ./internal/...  # 单测试

# 端到端(临时端口 + 临时 db,不动默认 8901)
./scripts/e2e.sh                      # 全部 demo case
./scripts/e2e.sh basic                # 只跑 basic
./scripts/e2e.sh fast                 # 复用运行中的 server(配合 run.sh --restart)
E2E_BASE_URL=http://x:9001 ./scripts/e2e.sh fast   # 跑远端 server
```

## 12. 开发模式

### 加新端点
1. `cmd/server/main.go` 加 `mux.HandleFunc("METHOD /api/...", s.handleXxx)`
2. 在 `APIServer` struct 字段添新 repo(如需)
3. 写 `handleXxx` 函数,error 走 `writeErr(w, code, msg)`,成功走 `writeJSON(w, data)`
4. 前端在对应 view JS 调 `fetchJSON('/api/...')`
5. 测试:`cmd/server/xxx_api_test.go` 用 `httptest.NewRequest` + `mux` 路由,参考 `terminal_api_test.go`

### 改评估逻辑
- `internal/evaluator/evaluator.go`:评估 prompt + 解析 + 注入数据
- 评估 prompt 模板 `evalPromptTpl`
- 解析正则 `scoreRe` / `cmtRe`
- 解析失败 → Score=-1(不 fallback 到 0,前端用此区分)

### 改前端(无构建)
- 直接改 `cmd/server/static/js/views/*.js` 或 `index.html`
- 改完浏览器 F5 刷新即可(embed 但无 hash 缓存)
- 共享函数放 `api.js`(`fetchJSON` / `switchTab` / `loadCLIModels` / `buildModelOptions` / `getDefaultModel` / `saveDefaultModel`)

### DB schema 改动
- `internal/backend/repo.go` 的 `InitSchema` 是 DDL 唯一来源
- 加新表:在 InitSchema 加 `CREATE TABLE IF NOT EXISTS ...`(注意 IF NOT EXISTS,老库不重建)
- 加新字段到现有表:用 `migrateTasksColumns` 模式(`PRAGMA table_info` 探测后 `ALTER TABLE ADD COLUMN`)

### 改配置项
- **终端类型 / 模型**:改 `config.json` 即可,启动时 `internal/config.Load` 加载,前端 `loadCLIModels()` 拉 `/api/models` 拿
- **用户级 KV**(`aichat_default_cli` 等):改 `app_settings` 表,前端 `PUT /api/settings/{key}`
- **新增配置项类型**:在 `internal/config/config.go` 加 struct 字段 + `Save()` 反序列化逻辑

### 加新 Tab
1. `index.html` 加 `<div id="page-xxx" class="hidden">...</div>` 和侧边栏 nav-item
2. 新建 `static/js/views/xxx.js`(参考现有 view 的 `loadXxx / renderXxx` 模式)
3. `index.html` 末尾 `<script>` 块加 `<script src="/static/js/views/xxx.js"></script>`
4. `api.js` 的 `switchTab` 加 `if (tab === 'xxx' && typeof loadXxx === 'function') loadXxx();`

## 13. 常见坑

1. **JSON tag omitempty**:`*float64` / `*time.Time` nil 时字段不出现,前端 `obj.score` undefined。`completed_at` 在 running 状态下必为 nil(用于"运行中"徽章检测)。
2. **scheduler.execute 同步**:task 调度时 main goroutine 阻塞等子进程完成,`last_status="running"` 中间态写不进去,改用"last_execution_id 对应 exec.completed_at 是否为空"判断。
3. **claude -p --output-format json**: 单次 JSON ~1.5KB,极轻量。`num_turns=1` 强信号表示 AI 没调任何工具(单轮不可能调)。
4. **evaluator 不传 WithActionReport()**: 评估员不该自报清单,否则自指。
5. **prompt 注入 markdown 限制**:raw string literal 不能用 \` 转义反引号,要么用 `'` 替代,要么 string concat。
6. **static 文件 embed**:`//go:embed index.html static` 编译时打包进二进制。改完前端代码必须 `./scripts/build.sh` 重新编译,光 go run 不行(开发用 `go run` 也是 embed 当前目录的)。
7. **aichat_default_cli 不在 config.json**:见 §4。改 CLI 默认走 `PUT /api/settings/aichat_default_cli`。
8. **PTY 跨平台**:macOS/Linux 真 PTY(`creack/pty`),Windows ConPTY stub 返回 503。`pty.go` 用 `//go:build !windows` 隔离,`pty_windows.go` 是 stub。
9. **gofmt 一定要跑**:Go 1.25 工具链强约束,CI 会卡。

## 14. 关键文件清单(修改时优先看)

| 想改什么 | 看这个文件 |
|---|---|
| AI 评估逻辑 | `internal/evaluator/evaluator.go` |
| 任务执行链路 | `cmd/server/main.go` `handleTaskRun` |
| 评估触发 | `cmd/server/main.go` `handleExecutionEvaluate` |
| DB schema | `internal/backend/repo.go:InitSchema` + `migrateTasksColumns` |
| 任务/评估模型下拉 | `cmd/server/index.html` + `views/automation.js` + `config.json` |
| 评估卡片渲染 | `views/automation.js:renderEvalCard` |
| 自动化 Tab | `views/automation.js` + `index.html` 顶部下拉 |
| AI 对话 Tab(多 Tab PTY) | `cmd/server/pty.go` + `cmd/server/pty_windows.go` + `views/aichat.js` |
| 代理 Tab(Exec + HTTP 代理) | `internal/relay/{exec,handler,repo,types}.go` + `views/relay.js` |
| 终端类型/模型配置 | `internal/config/config.go` + `config.json` + `internal/shortcuts/terminal.go` |
| WS Hub | `internal/hub/` + `internal/wsmsg/` |
| 调度器 | `internal/scheduler/scheduler.go` |
| 端点列表 | `cmd/server/main.go` `routes()` 函数(约 67-160 行) |
| 路径常量 | `internal/paths/paths.go` |
| e2e 测试用例 | `scripts/e2e.sh`(case 函数化) |
