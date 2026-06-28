# xworkbench v2 — All-in-One 个人工作台

All-in-One 个人工作台：集成本地快捷（网页书签/目录/置顶 todo）、AI 任务执行（claude/cbc/shell）+ 评估、经验库沉淀、定时调度、远程 Agent 代理。本地二进制开箱即用，配置文件 `config.json` 搞定所有偏好。

> 单 Go 二进制 · 16 表 · 7 Tab（总览 / 任务 / 经验库 / 自动化 / 系统配置 / AI 对话 / 代理）+ 4 widget · 97 HTTP 路由 · ~16600 行 Go · 跨 macOS / Linux / Windows（2026-06-28）

**完整功能介绍**：[xworkbench-intro-standalone.html](https://xiaodongq.github.io/assets/xworkbench/xworkbench-intro-standalone.html)（独立 HTML，包含所有 Tab/widget 截图与交互说明）

## 30 秒跑起来

```bash
git clone <repo>
cd xworkbench
./scripts/build.sh          # 编译
./scripts/run.sh            # 启动（默认 :8902）
```

浏览器打开 [http://localhost:8902](http://localhost:8902)，看到 7 Tab 即可。

> 配置文件 `./config.json`（可选，默认配置已包含终端类型和模型列表）；首次使用建议 `cp config.json.template config.json`

## 6 大功能

| 功能 | 说明 |
|---|---|
| **网页链接** | 网格图标卡片，悬停删除，点击外链新窗口 |
| **目录快捷** | 调系统资源管理器打开（Finder / xdg-open / explorer）|
| **定时任务** | Cron 调度 + 立即触发，支持 shell / claude / cbc |
| **todo.md** | 解析 `- [ ]` / `- [x]`，checkbox 点击自动写回 |
| **AI 任务 + 经验库** | 任务执行 / AI 评估 / 最佳实践沉淀 |
| **代理（Relay）** | 命令执行 + HTTP 代理转发，跨平台操作桥梁 |

## 功能截图

| 总览 | 快捷目录/链接/todo | 手动任务 | 自动化 |
|:---:|:---:|:---:|:---:|
| ![总览](https://raw.githubusercontent.com/xiaodongQ/xworkbench/main/docs/screenshots/01-dashboard.png) | ![快捷](https://raw.githubusercontent.com/xiaodongQ/xworkbench/main/docs/screenshots/09-shortcuts-term.png) | ![任务](https://raw.githubusercontent.com/xiaodongQ/xworkbench/main/docs/screenshots/02-tasks.png) | ![自动化](https://raw.githubusercontent.com/xiaodongQ/xworkbench/main/docs/screenshots/04-automation.png) |

## 快速命令

```bash
./scripts/run.sh --stop     # 停止
./scripts/run.sh --restart  # 重启
./scripts/run.sh --log      # 查看日志
./scripts/run.sh --status   # 运行状态
```

## 文档导航

- [docs/DESIGN.md](docs/DESIGN.md) — 架构设计 + 实施历程
- [docs/CLI.md](docs/CLI.md) — claude / cbc / shell 命令格式
- [docs/SCHEDULER.md](docs/SCHEDULER.md) — Cron 语法 + 跨平台调度
- [docs/MIGRATION.md](docs/MIGRATION.md) — v1 → v2 迁移指南
- [docs/relay-proxy-design.md](docs/relay-proxy-design.md) — 代理层设计
- [CHANGELOG.md](CHANGELOG.md) — 版本变更记录

## 依赖

- Go 1.22+
- 运行时：可选 `claude` / `cbc`（即 codebuddy）CLI（在 PATH 中）
- SQLite 无 CGO，纯 Go `modernc.org/sqlite`

## 安全声明

- **AI 任务执行**：AI 操作具有实际影响力（执行命令、写文件等），操作结果由调用者自行承担。建议非可信环境下关闭 `ai_loop_enabled`，或配置 `dangerously_skip_permissions` 时充分评估风险。
- **代理 API**：`relay.api_key` 默认为弱口令，生产部署务必改为强随机串；不建议将端口暴露至公网。
- **配置文件**：`config.json` 包含敏感信息（API key、token 等），请勿提交至公共仓库。
