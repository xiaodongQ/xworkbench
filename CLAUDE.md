# xworkbench · Claude 项目背景卡

> 此文件在 Claude 进入此项目时自动加载，避免每次重新分析。修改项目结构时同步更新本文件。

## 1. 项目定位

**All-in-One 个人工作台**: 单 Go 二进制 + SQLite,跨 macOS/Linux/Windows,融合 task / experience / scheduled / AI evaluation,5 个新功能(Web Links / Dir Shortcuts / Todo.md / Scheduler / 自动化)。

- **入口**: [README.md](README.md)
- **设计文档**: [DESIGN.md](DESIGN.md)
- **计划文档**: [docs/superpowers/plans/](docs/superpowers/plans/)

## 2. 技术栈

| 维度 | 选择 |
|---|---|
| 语言 | Go 1.25 |
| 存储 | SQLite,纯 Go `modernc.org/sqlite`(无 CGO) |
| HTTP | Go 1.22+ stdlib `mux`(path pattern 匹配) |
| WebSocket | `gorilla/websocket` |
| PTY | `creack/pty` |
| 前端 | vanilla JS + CSS(无框架,5 个 view 文件) |
| 嵌入资源 | `//go:embed` 静态文件 + HTML |

## 3. 规模

- 25 个 .go 文件,~4021 行
- 7 个 JS 文件(views) + 1 个 CSS
- 10 张 SQLite 表
- 48 个 HTTP 路由
- 1 个 WebSocket 频道

## 4. 目录结构

```
cmd/server/
├── main.go              # 入口 + 所有 HTTP handler(APIServer struct 包含所有 repo)
├── index.html           # 5 Tab 单页
├── static/css/base.css
└── static/js/
    ├── api.js           # fetchJSON 封装
    ├── widgets.js       # 5 widget(链接/目录/todo/dashboard/调度)
    └── views/{automation,experiences,tasks,aichat,dashboard}.js

internal/
├── backend/             # 数据层(10 张表 repo + models)
│   ├── models.go        # 所有 struct
│   ├── repo.go          # 所有 DDL + Repo 方法
│   └── ...
├── executor/            # 子进程执行
│   ├── exec.go          # 同步 Run + 流式回调
│   ├── confirm.go       # 4 种确认信号检测
│   └── runner/build.go  # BuildCommand(claude / cbc / shell)
├── evaluator/           # LLM-as-a-Judge(AI 任务打分)
├── scheduler/           # cron 调度(cron v3)
├── experience/          # 知识库
├── hub/                 # WebSocket Hub
├── wsmsg/               # WS 消息类型
├── httplog/             # HTTP 日志中间件
├── shortcuts/           # OS 资源管理器(goose/Explorer)
├── todo/                # 解析 todo.md
└── task/                # 任务领域
```

## 5. 数据模型(10 张表)

| 表 | 关键字段 | 说明 |
|---|---|---|
| `tasks` | id, title, description, status, priority, experience_id, resources, acceptance | 任务 |
| `executions` | id, task_id, scheduled_task_id, source, command, model, started_at, completed_at, output, error, exit_code, **evaluation_score** | 一次执行 |
| `evaluations` | id, task_id, execution_id, evaluator_model, score, comments, created_at | AI 评估结果 |
| `experiences` | id, module, scene, keywords, tool_usage, log_samples, code_snippets | 知识库 |
| `scheduled_tasks` | id, name, cron_expr, command_type, model, prompt, enabled, last_run_at, last_status | 定时任务 |
| `web_links` / `dir_shortcuts` / `app_settings` / `app_meta` / `skill_versions` | | |

## 6. 5 大功能模块

| Tab | 后端路由 | 前端 |
|---|---|---|
| **任务**(tasks) | `/api/tasks/*` | `views/tasks.js` |
| **经验库**(experiences) | `/api/experiences/*` | `views/experiences.js` |
| **自动化**(automation) | `/api/scheduled/*` + `/api/scheduler/*` + `/api/executions/*` + `/api/evaluations` | `views/automation.js` |
| **AI Chat** | `/api/pty`, `/ws` | `views/aichat.js` |
| **Dashboard** | `/api/stats` | `views/dashboard.js` |

5 个 widget(首页卡片):web-links / dir-shortcuts / todo.md / scheduled-summary / recent-executions。

## 7. AI 任务执行链路(核心)

### 7.1 执行流
```
用户点"▶ 运行" → POST /api/tasks/{id}/run
  → main.go handleTaskRun 构造 prompt
  → runner.BuildCommand("claude", model, "", prompt, WithActionReport())
  → executor.Run 同步跑子进程(claude -p --output-format json)
  → executions 表写记录(开始/完成时间 + output + exit_code)
  → WS 广播 'executions' 频道
```

### 7.2 评估流
```
用户点"📊 AI 评估" → POST /api/executions/{id}/evaluate
  → handleExecutionEvaluate 构造评估 prompt(用 BuildTaskPrompt 注入完整 task + experience 信息)
  → evaluator.Evaluate 异步调 claude 打分
  → ParseJSONExecution 解析 num_turns / result / is_error
  → parseEval 提取 "评分: X" + "评语: ..."
  → evaluations 表写记录
  → 前端每 2s 轮询,渲染评分卡
```

### 7.3 Prompt 模板
- **执行 prompt**: `BuildTaskPrompt(task, experience)` 注入 5 个 task 字段(title/description/priority/resources/acceptance)+ 7 个 experience 字段
- **评估 prompt**: `evalPromptTpl` 要求基于"指令 vs AI 自报动作清单 vs Claude 执行元数据(num_turns) vs 实际 stdout"四方对照打分

## 8. 关键约定

### 8.1 AI CLI 调用
**claude**: `claude -p --output-format json` (单次 JSON,非 stream-json,默认)
**cbc**: `cbc -p` (PATH 无 cbc 时回落 codebuddy)
**shell**: `sh -c "<prompt>"`
**WithActionReport()**: 给 claude/cbc prompt 末尾追加"动作清单"格式要求(便于 evaluator 验证是否真调了工具)

### 8.2 评估模型
默认 `sonnet`(前端 / 后端 main.go:476 fallback 都是),UI 可在 exec-detail-modal 下拉选 haiku/sonnet/opus。

### 8.3 端口
默认 `:8901`(避免 8080 冲突),环境变量 `ADDR` 可覆盖。DB 路径 `DB_PATH` 环境变量,默认 `./data/xworkbench.db`。

### 8.4 解析失败 vs 真低分
`Evaluation.Score = -1` 表示评估员输出无法解析(前端显示"解析失败"灰卡),`0` 表示真低分。

## 9. 常用命令

```bash
# 编译
go build -o xworkbench ./cmd/server

# 运行(默认端口 8901,默认 db ./data/xworkbench.db)
DB_PATH=./data/xworkbench.db ADDR=:8901 ./xworkbench

# 三平台交叉编译
GOOS=linux  GOARCH=amd64 go build -o xworkbench-linux  ./cmd/server
GOOS=windows GOARCH=amd64 go build -o xworkbench.exe    ./cmd/server

# 测试(6 包,~4s)
go test ./...

# 单包测试
go test -v ./internal/evaluator/
```

## 10. 开发模式

### 加新端点
1. `cmd/server/main.go` 加 `mux.HandleFunc("METHOD /api/...", s.handleXxx)`
2. 在 APIServer struct 字段添新 repo(如需)
3. 写 `handleXxx` 函数,error 走 `writeErr(w, code, msg)`,成功走 `writeJSON(w, data)`
4. 前端在对应 view JS 调 `fetchJSON('/api/...')`

### 改评估逻辑
- `internal/evaluator/evaluator.go`: 评估 prompt + 解析 + 注入数据
- 评估 prompt 模板 `evalPromptTpl`
- 解析正则 `scoreRe` / `cmtRe`
- 解析失败 → Score=-1(不 fallback 到 0,前端用此区分)

### 改前端(无构建)
- 直接改 `cmd/server/static/js/views/*.js` 或 `index.html`
- 改完浏览器 F5 刷新即可(embed 但无 hash 缓存)

### DB schema 改动
- `internal/backend/repo.go` 的 `InitSchema` 是 DDL 唯一来源
- 加新表:在 InitSchema 加 `CREATE TABLE IF NOT EXISTS ...`(注意 IF NOT EXISTS,老库不重建)
- 加新字段到现有表:SQLite ALTER TABLE 限制多,建议新建迁移表 `schema_migrations`

## 11. 常见坑

1. **JSON tag omitempty**:`*float64` / `*time.Time` nil 时字段不出现,前端 `obj.score` undefined。`completed_at` 在 running 状态下必为 nil(用于"运行中"徽章检测)。
2. **scheduler.execute 同步**:task 调度时 main goroutine 阻塞等子进程完成,`last_status="running"` 中间态写不进去,改用"last_execution_id 对应 exec.completed_at 是否为空"判断。
3. **claude -p --output-format json**: 单次 JSON ~1.5KB,极轻量。`num_turns=1` 强信号表示 AI 没调任何工具(单轮不可能调)。
4. **evaluator 不传 WithActionReport()**: 评估员不该自报清单,否则自指。
5. **prompt 注入 markdown 限制**:raw string literal 不能用 \` 转义反引号,要么用 `'` 替代,要么 string concat。

## 12. 关键文件清单(修改时优先看)

| 想改什么 | 看这个文件 |
|---|---|
| AI 评估逻辑 | `internal/evaluator/evaluator.go` |
| 任务执行链路 | `cmd/server/main.go:289` (handleTaskRun) |
| 评估触发 | `cmd/server/main.go:535` (handleExecutionEvaluate) |
| DB schema | `internal/backend/repo.go:InitSchema` |
| 任务/评估模型下拉 | `cmd/server/index.html` + `views/tasks.js` + `views/automation.js` |
| 评估卡片渲染 | `views/automation.js:renderEvalCard` |
| 自动化 Tab | `views/automation.js` + `index.html` 头部下拉 |
| WS Hub | `internal/hub/` |
| 调度器 | `internal/scheduler/scheduler.go` |
| 端点列表 | `cmd/server/main.go:67-125` (routes) |
