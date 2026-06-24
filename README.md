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
