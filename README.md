# xworkbench v2 — All-in-One 个人工作台

> 单 Go 二进制 · 14 张表 · 7 Tab（总览 / 任务 / 经验库 / 自动化 / 系统配置 / AI 对话 / 代理）+ 5 widget · 91 HTTP 路由 · ~14600 行 Go · 跨 macOS / Linux / Windows

融合 `xworkbench v1`（Go + 经验库 + PTY + 漂亮 UI）和 `ai-task-system v2.4`（Python 调度 + AI CLI 执行器），加上多个新功能，作为日常 homepage / launcher / 任务调度统一入口。

详见 [docs/DESIGN.md](docs/DESIGN.md)。

## 30 秒跑起来

```bash
git clone <repo>
cd xworkbench
./scripts/build.sh          # 编译
./scripts/run.sh             # 启动（默认 :8902）
```

浏览器打开 [http://localhost:8902](http://localhost:8902)，看到 7 Tab（总览 / 任务 / 经验库 / 自动化 / 系统配置 / AI 对话 / 代理）即可。

> 2026-06-23 拆分后：「数据管理」页改名为「系统配置」并提升为独立 Tab（`config`），与其它业务 Tab 平级。Agent 管理面板嵌在「代理」Tab 内（不开新 Tab），系统配置 Tab 负责偏好/导入导出/快捷目录/快捷链接。

> 配置文件：`./config.json`（可选，默认配置已包含终端类型和模型列表）

## 配置

`config.json` 用来覆盖 [internal/config/config.go](internal/config/config.go) 里 `DefaultConfig()` 给出的默认值。**它本身在 `.gitignore`**，不入库；本仓库分发 `config.json.template` 模板。

**首次使用**：

```bash
# 1. 复制模板为本地配置
cp config.json.template config.json

# 2. 按需修改（常见项见下表）
vim config.json
```

或者直接走 UI：系统配置 Tab → 改完点保存（自动写本地 `config.json`，不需要手动编辑）。

**字段速览**（完整 schema 见模板文件，每行 `_comment` 字段标注了用途）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `default_terminal` | string | 默认终端类型（wezterm / wt / gnome 等），空=系统检测 |
| `preferred_cli` | string | 全局优先 CLI（`claude` / `cbc`），影响新建任务的默认 `command_type` |
| `ai_loop_enabled` | bool | AI 自治能力开关（run-loop / reevaluate / learn 后端能力）|
| `aichat_default_cli` | string | AI 对话 Tab 新 Tab 默认起哪个 CLI（codex/cbc/shell/claude）|
| `dangerously_skip_permissions` | bool | AI 任务完全放开 CLI 权限（用 `--dangerously-skip-permissions`），**慎用** |
| `todo_md_path` | string | todo widget 读取的 todo.md 路径，空=不读 |
| `scheduler_enabled` | bool | 上次调度器运行状态（启动时自动恢复）|
| `relay.api_key` | string | 代理接口认证 token；空=关闭认证；生产环境务必改成强随机串 |
| `terminal.types.<key>` | object | 自定义终端类型定义（`bin` / `args` / `name` / `plate`）|
| `models.<cli>.default` | string | 任务执行的默认模型（用户创建任务 / 调度器用）|
| `models.<cli>.eval_default` | string | AI 评估员默认模型（独立于 default），未设时 fallback 到 default |

## 6 大功能

### 1. 网页链接
- 点 "**+ 添加**" → 填名称 + URL → 网格图标卡片
- 悬停显示删除 ×，点击外链新窗口

### 2. 目录快捷
- 点 "**+ 添加**" → 填名称 + 本地绝对路径
- 列表显示，点击调系统资源管理器打开（macOS Finder / Linux xdg-open / Windows Explorer）
- 支持设置默认终端类型和路径

### 3. 定时任务（Automation Tab）
- 点 "**+ 新建定时任务**" → 填名称 + Cron + 类型 + Prompt
- 类型支持 `shell` / `claude` / `cbc`，模型可配置
- Cron 支持标准 5 字段 + `@every 30s` / `@hourly` 等预设
- 顶部 "**▶ 启动**" / "**⏸ 停止**" / "**🔄 重载**" 控制调度器
- 表格右侧 "**▶ 跑**" 立即触发一次（不依赖 cron）

### 4. todo.md
- 点 "**设置**" → 配 todo.md 路径
- 解析 `- [ ]` / `- [x]` 项，点击 checkbox 自动写回文件

### 5. AI 任务 + 经验库
- 任务页面：查看 / 认领 / 归档，AI 评估辅助
- 经验库：最佳实践沉淀，支持查看（只读）

### 6. 代理（Relay）
- 命令执行 / HTTP 代理转发 / 跨平台操作桥梁

## 三平台编译

```bash
./scripts/build.sh          # macOS（默认）→ ./bin/xworkbench
./scripts/build.sh -a       # 三平台全量编译 → ./bin/xworkbench-{darwin,linux,windows}
./scripts/build.sh -c       # 清理 ./bin
```

| 平台 | PTY 终端 | AI 对话 | 调度器 | OpenDir |
|---|---|---|---|---|
| macOS | ✅ 真 PTY | ✅ | ✅ | ✅ Finder |
| Linux | ✅ 真 PTY | ✅ | ✅ | ✅ xdg-open |
| Windows | ✅ ConPTY | ⚠️ stub | ✅ | ✅ explorer |

## 快速命令

```bash
./scripts/run.sh --stop     # 停止
./scripts/run.sh --restart # 重启
./scripts/run.sh --log     # 查看日志
./scripts/run.sh --status  # 运行状态
```

## 6.25 之后新增特性（继 6.19-6.24 之后）

> 与博客 [9 节](https://xiaodongq.github.io/2026/06/19/assistant-all-in-one-advance-feature/#9) 不重复的部分，6.25 之后接入的新能力。

### 9.13 执行/评估模型独立配置（55a826c + 672b891）
- `models.<cli>` 拆为 `default` (执行) / `eval_default` (评估) 两个字段
- 默认值：claude `default=sonnet` / `eval_default=sonnet`，cbc `default=glm-5.1` / `eval_default=glm-5.0`
- 未设 `eval_default` → fallback 到 `default` → 硬编码 `"sonnet"`
- 前端两个下拉独立：创建任务选 default；exec-detail-modal 选 eval
- 变更动机：评估需要更稳的判断(sonnet)，执行可走性价比(haiku/glm-4.7)

### 9.14 execution.status 显式状态 + 手动取消（be08ccf）
- 背景：执行列表里某条记录"一直显示运行中"（ctx 超时后 `error` 字段空白 + 服务器重启后 in-flight goroutine 消失）
- 3 个独立问题一次性修：
  1. `executions` 加 `status` 列，6 个取值：`running / success / failed / timeout / cancelled / build_error`（取代"completed_at+exit_code+error 拼凑"判定）
  2. 新增 `POST /api/executions/{id}/cancel` 接口
  3. UI 加「⚠ 标记完成」按钮 + 30s 兜底轮询（主刷新还是 3s auto-refresh）
  4. 错误信息保留：`res.Err.Error()` 兜底写入 errOut（ctx 超时后用户能看到具体错误）

### 9.15 继续对话延续原 CLI/model（9ddad16）
- 背景：`handleExecutionContinue` 以前硬编码 CLI="claude"，原 exec 如果是 cbc/shell 会切断运行环境
- 改动：`executions` 表加 `cli_type TEXT` 字段（老库自动 ALTER 迁移），继续对话时沿用原 exec 的 CLI/model
- 评估/前端用 `execution.cli_type` 判断运行环境，避免 cbc session 误以 claude 评估

### 9.16 任务详情 AI 自治区块加回 + Run Loop 异步化（2e547ad）
- AI 自治按钮缺失：task-modal 里 `#ai-loop-section` / 3 个按钮 / 进度容器 DOM 从来没渲染，加回 DOM + 3 个 handler
- Run Loop 同步阻塞问题：之前在 HTTP handler 里跑 `for { exec → eval → 换模型重试 }`，超 60s 触发前端超时断连；改异步（轮询 status）+ HTTP handler 立即返回
- 优雅关闭：scheduler 添加 shutdown 钩子，Stop 时等 in-flight goroutine 完成

### 9.17 代理页"生成 Linux 调用脚本"快捷功能（5345f3f）
- 在「代理」Tab 加按钮「📜 生成 Linux 调用脚本」
- 一键生成可拷贝的 shell 脚本（命令执行 + HTTP 转发），拷到 Linux 机器后改 3 处即可调用 xworkbench 代理
- 用法：Linux 机器无需装 xworkbench，通过 relay.api_key 鉴权

### 9.18 调度器 AI 默认超时 1 小时 → 10 分钟（5b449a6）
- `scheduled_tasks.timeout_sec=0` 时：AI 1h → 10min（与 handleTaskRun/handleExecutionContinue 保持一致），shell 保持 5min
- 调度器超时表记在 §10.4

### 9.19 其它 6.25 后微改进
- `c12db3b` scheduler trigger 后刷新 nextRun map，让 next_run_at UI 实时更新
- `f0459a8` 执行列表 UI 精简（删冗余入口 + 标记完成改取消）
- `76e1ea1` btn-secondary 视觉禁用样式 + 继续对话按钮 loading/error 反馈
- `403f040` tooltip 自适应视口边界 + jumpToRoot 自动加载更多重试
- `a722e31` 快捷示例加 `@every m/h` 粒度 + 说明支持单位
- `ed2067f` cron 表达式说明文案简化
- `14817dc` 关闭 ai_loop + 换 cbc 默认模型（个人偏好调整，非 bug）

## 文档导航

- [docs/DESIGN.md](docs/DESIGN.md) — 架构设计 + 实施历程（**注：DESIGN.md 写于 v2 初版定型时，9 张表与早期形态描述仅作历史参考，现状以本 README + CLAUDE.md 为准**）
- [docs/CLI.md](docs/CLI.md) — claude / cbc / shell 命令格式
- [docs/SCHEDULER.md](docs/SCHEDULER.md) — Cron 语法 + 跨平台调度
- [docs/MIGRATION.md](docs/MIGRATION.md) — v1 → v2 迁移指南
- [docs/relay-proxy-design.md](docs/relay-proxy-design.md) — 代理层设计（Exec + HTTP 代理）

## 6.19-6.24 之间新增/调整的能力（汇总）

> 这些是 [博客](https://xiaodongq.github.io/2026/06/19/assistant-all-in-one-advance-feature/) 第 9 节 6.19 之后到 6.24 之间接入的特性，按"功能大类"组织（不按 commit 时间顺序）。

### 9.1 调度器下次执行时间（next_run_at 注入）
- **背景**：用户反馈定时任务列表只能看"上次执行"，没法预知下次什么时候跑
- **实现链路**：
  - `backend.ScheduledTask` 加 `NextRunAt *time.Time` 字段（`cf019d6` 暂未注入）
  - `scheduler` 暴露 `NextRunAt(taskID) (time.Time, bool)` 走内部 entry（`ea6ec48`），**不现场 Parse+Next(now)**——避免 handler 调用时 now 漂移导致 UI 跳动
  - `handleScheduledList` 在 list 循环里 `s.sch.NextRunAt(t.ID)`，有就注入（`69dfe7a`）
  - 前端 `automation.js` 表格「下次执行时间」列：仅 `enabled` 任务显示，加 ⏰ icon + info 色（`f8c93e3`）区别于「上次」
  - 4 个测试覆盖：`@every` 描述符解析（`e3e2b03`）、非法 cron 不阻断整列表（`943e6ce`）、disabled 任务字段不出现（`3310a3c`）、newTestServer 启 scheduler 适配（`13034fd`）
  - e2e 加 `case_concurrent_scheduled` 验证并发不报 SQLITE_BUSY（`84ee1f5`），同时 backend OpenDB PRAGMA 修复 + scheduler singleflight（`2da84db`）
- **博客未提此特性族**（当时还在写），属于漏项，已补回

### 9.2 任务执行链路小修复（c922745 等）
- **任务表 td 恢复表格语义**（`c922745`）：之前为 flex 行为把 td 排版改了，导致部分场景下 `<td>` 不是表格 cell 行为，flex 下沉到内层 div 修复
- **远程 agent claim 后缺 prompt 修复**（`b12fb7e`）：claim/claim-next 接口之前只返 task + experiences 原始数据，agent 端必须自己拼 prompt；改为直接返回预生成 prompt，`Get` 读 `completed_at`
- **手动任务执行注入经验库**（`1d6db92`）：Bug 1 修复——手动任务执行时经验库未注入（与自动任务行为不一致），前端同步从旧 `experience_id` 切到新 `experience_ids` 数组提交
- **自动化页 5 处 resume_uuid 误判修复**（`ebd3bc9`）：原代码用 `if (x.resume_uuid)` 判断是否有会话链，5 处未覆盖非空字符串边界
- **scheduled 频道 chunk 推送改走 exec 频道**（`6292757`）：原 ChannelScheduled 重复推，改为前端的 automation.js 按 event 字段过滤；`docs/wsmsg` ChannelScheduled 注释同步更新（`7688f52`）

### 9.3 任务管理（手动/远程）双修复
- `b12fb7e` 远程 agent claim 响应只返 raw 数据 → 改为返预生成 prompt
- `1d6db92` 手动任务缺经验库 + 前端用新字段 → 一并修复
- 这些都是 bug 修复而非新能力，但与 9.2 中 6.19 之后的「远程 Agent」功能密切相关，**6.19 博客只写了能力上线没写后续 bug fix**

## 依赖

- Go 1.22+
- 运行时：可选 `claude` / `cbc`（即 codebuddy）CLI（在 PATH 中）
- SQLite 无 CGO，纯 Go `modernc.org/sqlite`
