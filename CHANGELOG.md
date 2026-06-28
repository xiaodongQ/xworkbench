# Changelog

> 完整功能介绍见 [xworkbench-intro-standalone.html](https://xiaodongq.github.io/assets/xworkbench/xworkbench-intro-standalone.html)

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
