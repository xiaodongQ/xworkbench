# xworkbench v2 — All-in-One 个人工作台

> 单 Go 二进制 · 12 张表 · 6 大功能（链接 / 目录 / todo / 定时 / AI 任务 / 经验库）· 跨 macOS / Linux / Windows

融合 `xworkbench v1`（Go + 经验库 + PTY + 漂亮 UI）和 `ai-task-system v2.4`（Python 调度 + AI CLI 执行器），加上多个新功能，作为日常 homepage / launcher / 任务调度统一入口。

详见 [docs/DESIGN.md](docs/DESIGN.md)。

## 30 秒跑起来

```bash
git clone <repo>
cd xworkbench
./scripts/build.sh          # 编译
./scripts/run.sh             # 启动（默认 :8901）
```

浏览器打开 [http://localhost:8901](http://localhost:8901)，看到 6 Tab（总览 / 任务 / 经验库 / 自动化 / AI 对话 / 代理）即可。

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

- [docs/DESIGN.md](docs/DESIGN.md) — 架构设计 + 实施历程
- [docs/CLI.md](docs/CLI.md) — claude / cbc / shell 命令格式
- [docs/MIGRATION.md](docs/MIGRATION.md) — v1 → v2 迁移指南
- [docs/SCHEDULER.md](docs/SCHEDULER.md) — Cron 语法 + 跨平台调度

## 依赖

- Go 1.22+
- 运行时：可选 `claude` / `cbc`（即 codebuddy）CLI（在 PATH 中）
- SQLite 无 CGO，纯 Go `modernc.org/sqlite`
