---
name: xworkbench-cli
description: 操作 xworkbench 工作台（任务/执行/经验库/调度/todo/配置管理）
version: 1.0.0
xw_command: xworkbench-cli
xw_params:
  manual_task_list: "列出手动任务 (--status 状态 --task-type local|remote --limit 数量)"
  manual_task_create: "创建手动任务 (--title 标题 --description 描述 [--target-dir-id])"
  manual_task_get: "获取手动任务详情 (task_id)"
  manual_task_update: "更新手动任务 (task_id --title --description --status --priority [--target-dir-id])"
  manual_task_run: "运行手动任务 (task_id)"
  manual_task_cancel: "取消手动任务 (task_id)"
  manual_task_delete: "删除手动任务 (task_id)"
  sched_task_list: "列出定时任务 (--task-type local|remote)"
  sched_task_get: "获取定时任务详情 (scheduled_id)"
  sched_task_create: "创建定时任务 (--name --cron [--command-type] [--model] [--prompt] [--target-dir-id])"
  sched_task_update: "更新定时任务 (scheduled_id --name --cron [...] [--target-dir-id])"
  sched_task_delete: "删除定时任务 (scheduled_id)"
  sched_task_run: "立即运行定时任务 (scheduled_id)"
  sched_task_toggle: "切换定时任务启用/禁用 (scheduled_id)"
  exec_list: "列出执行记录 (--task-id --scheduled-task-id --limit)"
  exec_get: "获取执行详情 (execution_id)"
  exec_evaluate: "AI 评估执行 (execution_id)"
  exec_cancel: "取消执行 (execution_id)"
  exec_continue: "继续对话 (execution_id)"
  experience_list: "列出经验库 (--module)"
  experience_get: "获取经验详情 (experience_id)"
  experience_create: "创建经验 (--module --scene --keywords [--tool-usage] [--log-samples] [--code-snippets])"
  experience_update: "更新经验 (experience_id --module --scene --keywords ...)"
  experience_delete: "删除经验 (experience_id)"
  todo_list: "列出 todo"
  todo_add: "添加 todo (--content)"
  todo_toggle: "切换完成状态 (line_no)"
  todo_edit: "编辑 todo (line_no content)"
  config_get: "获取配置 (key 可选)"
  config_set: "设置配置 (key value)"
  models_list: "列出可用模型"
  stats: "获取统计信息"
  dir_shortcut_list: "列出目录快捷"
  dir_shortcut_create: "创建目录快捷 (--name --path)"
  dir_shortcut_update: "更新目录快捷 (id --name --path)"
  dir_shortcut_delete: "删除目录快捷 (id)"
  dir_shortcut_open: "打开目录 (id)"
  web_link_list: "列出链接快捷"
  web_link_create: "创建链接快捷 (--title --url)"
  web_link_update: "更新链接快捷 (id --title --url)"
  web_link_delete: "删除链接快捷 (id)"
  web_link_open: "打开链接 (id)"
xw_output:
  ok: "成功标志 (true/false)"
  data: "API 返回的数据"
  error: "错误信息"
xw_examples:
  - description: "列出所有 pending 任务"
    params:
      subcommand: list
      status: pending
  - description: "创建新任务"
    params:
      subcommand: create
      title: "优化性能"
      description: "分析并优化慢查询"
  - description: "运行指定任务"
    params:
      subcommand: run
      task_id: "123"
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
