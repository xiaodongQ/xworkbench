---
name: xworkbench-cli
description: 操作 xworkbench 工作台（任务/执行/经验库/调度/todo/配置管理）
version: 1.0.0
xw_command: xworkbench-cli
xw_params:
  task_list: "列出任务 (--status 状态 --task-type manual|remote --limit 数量)"
  task_create: "创建任务 (--title 标题 --description 描述 [--target-dir-id 远程目标DirShortcutID])"
  task_get: "获取任务详情 (task_id)"
  task_update: "更新任务 (task_id --title --description --status --priority [--target-dir-id])"
  task_run: "运行任务 (task_id)"
  task_cancel: "取消任务 (task_id)"
  task_delete: "删除任务 (task_id)"
  exec_list: "列出执行记录 (--task-id --limit)"
  exec_get: "获取执行详情 (execution_id)"
  exec_evaluate: "AI 评估执行 (execution_id)"
  exec_cancel: "取消执行 (execution_id)"
  exec_continue: "继续对话 (execution_id)"
  experience_list: "列出经验库 (--module)"
  experience_get: "获取经验详情 (experience_id)"
  experience_create: "创建经验 (--module --scene --keywords [--tool-usage] [--log-samples] [--code-snippets])"
  experience_update: "更新经验 (experience_id --module --scene --keywords ...)"
  experience_delete: "删除经验 (experience_id)"
  scheduled_list: "列出定时任务 (--task-type manual|remote)"
  scheduled_get: "获取定时任务详情 (scheduled_id)"
  scheduled_create: "创建定时任务 (--name --cron [--command-type] [--model] [--prompt] [--target-dir-id])"
  scheduled_update: "更新定时任务 (scheduled_id --name --cron [...] [--target-dir-id])"
  scheduled_delete: "删除定时任务 (scheduled_id)"
  scheduled_run: "立即运行定时任务 (scheduled_id)"
  scheduled_toggle: "切换启用/禁用 (scheduled_id)"
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

xworkbench 工作台 Agent CLI 工具，支持完整的任务/执行/经验库/调度/todo/配置管理操作。
