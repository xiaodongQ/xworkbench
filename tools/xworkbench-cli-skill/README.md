# xworkbench-cli Skill

xworkbench 工作台 Agent CLI 工具，供 Claude Code 等 Agent 操作工作台。

## 安装

1. 从 xworkbench-cli 仓库获取 `xworkbench-cli-skill` 目录
2. 确保 `xworkbench-cli` 二进制有执行权限：`chmod +x xworkbench-cli`
3. 拷贝到 Claude Code 的 skills 目录：`cp -r xworkbench-cli-skill ~/.claude/skills/`

## 使用方法

```
xworkbench-cli [OPTIONS] <command> [subcommand] [args...]

Options:
  --server      xworkbench server URL (default: http://localhost:8902)
```

## 命令列表

### task - 任务管理

```bash
xworkbench-cli task list [--status <status>] [--limit <n>]
xworkbench-cli task create --title <title> [--description <desc>]
xworkbench-cli task get <task_id>
xworkbench-cli task update <task_id> [--title <title>] [--description <desc>] [--status <status>] [--priority <n>]
xworkbench-cli task run <task_id>
xworkbench-cli task cancel <task_id>
xworkbench-cli task delete <task_id>
```

### exec - 执行记录

```bash
xworkbench-cli exec list [--task-id <id>] [--limit <n>]
xworkbench-cli exec get <execution_id>
xworkbench-cli exec evaluate <execution_id>
xworkbench-cli exec cancel <execution_id>
xworkbench-cli exec continue <execution_id>
```

### experience - 经验库

```bash
xworkbench-cli experience list [--module <module>]
xworkbench-cli experience get <experience_id>
xworkbench-cli experience create --module <module> --scene <scene> --keywords <keywords> [--tool-usage <usage>] [--log-samples <samples>] [--code-snippets <snippets>]
xworkbench-cli experience update <experience_id> [--module <module>] [--scene <scene>] [--keywords <keywords>]
xworkbench-cli experience delete <experience_id>
```

### scheduled - 定时任务

```bash
xworkbench-cli scheduled list
xworkbench-cli scheduled get <scheduled_id>
xworkbench-cli scheduled create --name <name> --cron <cron> [--command-type <type>] [--model <model>] [--prompt <prompt>]
xworkbench-cli scheduled update <scheduled_id> [--name <name>] [--cron <cron>] [--command-type <type>] [--model <model>] [--prompt <prompt>]
xworkbench-cli scheduled delete <scheduled_id>
xworkbench-cli scheduled run <scheduled_id>
xworkbench-cli scheduled toggle <scheduled_id>
```

### todo - Todo 管理

```bash
xworkbench-cli todo list
xworkbench-cli todo add --content <content>
xworkbench-cli todo toggle <line_no>
xworkbench-cli todo edit <line_no> <content>
```

### config - 配置管理

```bash
xworkbench-cli config get [<key>]
xworkbench-cli config set <key> <value>
```

### models - 模型列表

```bash
xworkbench-cli models list
```

### stats - 统计信息

```bash
xworkbench-cli stats
```

### dir-shortcut - 目录快捷

```bash
xworkbench-cli dir-shortcut list
xworkbench-cli dir-shortcut create --name <name> --path <path>
xworkbench-cli dir-shortcut update <id> [--name <name>] [--path <path>]
xworkbench-cli dir-shortcut delete <id>
xworkbench-cli dir-shortcut open <id>
```

### web-link - 链接快捷

```bash
xworkbench-cli web-link list
xworkbench-cli web-link create --title <title> --url <url>
xworkbench-cli web-link update <id> [--title <title>] [--url <url>]
xworkbench-cli web-link delete <id>
xworkbench-cli web-link open <id>
```

## 输出格式

所有命令返回 JSON：

```json
{"ok": true, "data": {...}}
{"ok": false, "error": "error message"}
```

## 构建

```bash
cd cmd/xworkbench-cli
go build -o xworkbench-cli .
```
