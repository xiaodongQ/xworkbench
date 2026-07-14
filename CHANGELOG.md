# Changelog

> 完整功能介绍见 [xworkbench-intro-standalone.html](https://xiaodongq.github.io/assets/xworkbench/xworkbench-intro-standalone.html)

---

## v2.5 · 7.12-7.15 新增/调整的能力

> 跨 Terminal / AI Chat / Markdown 渲染基础设施 / CI 等多条线，按功能组织。

### 10.1 终端体验优化

- `be97579` 终端侧栏可收起 + 列表状态以前端 termPool 为准（多会话管理）
- `9244e18` 终端会话列表按创建时间排序，避免刷新时跳位置
- `ce6f06f` 连接后延迟刷新会话列表 + 多处增加 `renderSessionList`
- `8845ab0` 终端会话列表状态同步（revert 后再修回）
- `024787f` 终端侧栏切换图标 `◀/▶` → `☰`
- `0f5905c` 终端左侧标题字号 `9px → 10px`

### 10.2 总览页改进

- `cdf04b6` 总览页紧凑化 + UI 优化
- `c13cc25` 总览统计改用**执行次数**替代运行时长 + 每日统计按日期排序
- `e13edc0` `substr` 替代 `DATE()` 修复 SQLite monotonic clock 格式解析失败问题

### 10.3 AI Chat 对话增强（73d9926）

`ai_tools.go` + `aichat.js` 大幅重构，AI 可直接操作 Todo / 任务 / 远程 Agent：

- **Todo 操作**：新增 `edit_todo`（编辑内容）、`archive_todo`（归档）、`add_todo_child`（添加子任务）三个工具
- **任务管理**：新增 `delete_task`（需二次确认）、`reevaluate_task`（重评上一次执行）、`get_task_eval_history`（评历史）
- **远程 Agent**：新增 `list_agents`（列出所有工作节点）
- `e756afd` AI 对话多轮协议 alternation 修复（Anthropic + OpenAI）
- `3ec9ab7` AI 对话 `tool_result` 协议错误 + 无超时/无限轮 bug

### 10.4 Markdown 渲染基础设施

- `c26e0bc` 引入 markdown 渲染基础设施（vendor + 整合层 + 单测 + 复用文档），打通 AI Chat 和 Automation 两处渲染

### 10.5 AI 助手 & 执行详情 Markdown 支持

- `622a9fc` 侧边 AI 助手支持 markdown 渲染 + 复制原文按钮
- `079efd6` 执行详情卡片 markdown 渲染 + 原文切换
- `3f3c170` execution detail markdown section cards + tooltip 样式

### 10.6 Windows PTY 全面补齐

- `b874e29` 移除 `pty_session_manager.go` 的 `!windows` build tag，Windows 构建不再跳过
- `8ffa938` 补齐 Windows PTY 会话管理、auth 检测和资源清理
- `93a09b6` Windows `stop()` 先 graceful 再 force kill，避免 `SIGKILL` 绕过 Go 清理

### 10.7 调度器 & 样式修复

- `e2b21f4` 解析不到 `session_id` 时清空 `LastSessionID`，下次调度重建会话
- `519985f` 字体/样式优化 + 修复 xwcli 安装命令不显示

### 10.8 CI/CD 多平台构建

- `38556c8` 新增 `cross-platform build` workflow，支持 linux/macos/windows 三平台手动触发
- `df2e858` Windows PowerShell build step 兼容（加 `shell: bash`）
- `beaf97f` macOS release target 明确指定 `amd64`（darwin x86_64）
- `452cbaa` 配置文件模板路径从 `.conf` 改为 `.json`
- `d1f0362` v3.0-stable release notes 准备
- `92a9057` release workflow 加 `contents:write` permission + glob pattern 上传产物
- `ede8fa5` build/release job 拆分，消除多平台并发写 release 的 race condition
- `9cf4137` SSH command builder 测试补全缺失的 `AuthMethod` 字段

---

## v2.4 · 6.25 之后新增特性

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

---

## v2.3 · 6.19-6.24 新增/调整的能力

> [博客第 9 节](https://xiaodongq.github.io/2026/06/19/assistant-all-in-one-advance-feature/#9) 6.19 到 6.24 之间接入的特性，按功能大类组织（不按 commit 时间顺序）。

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
