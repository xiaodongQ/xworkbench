---
name: xworkbench-cli
description: 操作 xworkbench 工作台（任务/执行/经验库/调度/todo/配置管理）
version: 1.0.0
xw_params:
  tasks: "查询所有任务（合并手动+定时）"
  manual-task: "手动任务 CRUD (list/create/get/update/run/delete)"
  sched-task: "定时任务 CRUD (list/create/get/update/delete/run/toggle)"
  exec: "执行记录 (list/get/evaluate/cancel/continue)"
  experience: "经验库 (list/get/create/update/delete)"
  todo: "Todo 管理 (list/add/toggle/edit)"
  config: "配置管理 (get/set)"
  models: "列出可用模型"
  stats: "仪表盘统计"
  dir-shortcut: "目录快捷 (list/create/update/delete/open)"
  web-link: "链接快捷 (list/create/update/delete/open)"
xw_examples:
  - description: "列出所有任务（手动+定时）"
    params:
      subcommand: tasks
      limit: 20
  - description: "列出手动任务"
    params:
      subcommand: manual-task
      limit: 0
      status: pending
  - description: "列出远程定时任务"
    params:
      subcommand: sched-task
      task_type: remote
  - description: "获取统计信息"
    params:
      subcommand: stats
---

# xworkbench-cli Skill

xworkbench 工作台 Agent CLI 工具，管理手动任务、定时任务、执行记录、经验库等。流程与交互

**核心规则：**
- 用户说"查任务"/"有哪些任务"→ 用 `tasks <options>` 命令，自动合并手动+定时
- CLI 部署在远端时，提醒 `export XWORKBENCH_SERVER=http://<ip>:8902`，之后无需 `--server`

**常用场景与命令：**
- 查所有任务（≤20条） → `tasks --limit 20`
- 只看远程任务 → `tasks --task-type remote`
- 创建手动任务 → `manual-task create --title "标题" --description "描述"`
- 创建远程手动任务 → 加 `--target-dir-id <dir_shortcut_id>`
- 创建定时任务 → `sched-task create --name "名" --cron "@every 30s" --command-type shell --prompt "echo hi"`
- 立即执行定时任务 → `sched-task run <id>`
- 查看执行记录 → `exec list --limit 10`
- 查看执行详情/输出 → `exec get <execution_id>`
- 继续对话 → `exec continue <execution_id>` (只支持 claude/cbc)
- 查统计 → `stats`

**远程任务定义：**
- 手动任务 `task_type=remote` + `assigned_dir_shortcut_id` 指向 DirShortcut
- 定时任务 `assigned_dir_shortcut_id` 非空为远程
- 创建远程任务需指定 `--target-dir-id <id>`，id 可查界面「系统配置→目录快捷」

**执行记录：**
- `exec list` 默认列出所有（手动+定时），用 `--task-id` 或 `--scheduled-task-id` 过滤
- `exec get <id>` 查看完整输出，`exec evaluate <id>` AI 评估执行质量

**定时任务调度器：**
- 需在 Web 工作台启动调度器后 cron 才自动触发
- `sched-task run` 手动触发，不依赖调度器
- `sched-task toggle` 切换启用/禁用
- `stats` 查看调度器状态
