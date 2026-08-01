# xworkbench-cli Skill

xworkbench 工作台 Agent CLI 工具，供 Claude Code 等 Agent 操作工作台。

## 安装

### 方式一：一键安装（推荐）

如果 xworkbench 服务器已运行，执行安装脚本即可：

```bash
curl http://localhost:8902/api/xworkbench-cli/install | bash
```

安装脚本会自动：
1. 检测 Claude Code 或 CodeBuddy 环境
2. 下载对应平台的二进制
3. 下载 SKILL.md 和 README.md
4. 安装到 `~/.claude/skills/xworkbench-cli/` 或 `~/.codebuddy/skills/xworkbench-cli/`

### 方式二：手动安装

1. 下载二进制：
   ```bash
   curl http://localhost:8902/api/xworkbench-cli/download -o xworkbench-cli
   chmod +x xworkbench-cli
   ```
2. 下载 SKILL.md：
   ```bash
   curl http://localhost:8902/api/xworkbench-cli/skill.md -o SKILL.md
   ```
3. 拷贝到 skills 目录：
   ```bash
   mkdir -p ~/.claude/skills/xworkbench-cli/
   cp xworkbench-cli SKILL.md ~/.claude/skills/xworkbench-cli/
   ```

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

## 跨平台构建

```bash
cd cmd/xworkbench-cli
GOOS=darwin GOARCH=amd64 go build -o ../tools/xworkbench-cli-skill/xworkbench-cli-darwin-amd64 .
GOOS=linux GOARCH=amd64 go build -o ../tools/xworkbench-cli-skill/xworkbench-cli-linux-amd64 .
GOOS=windows GOARCH=amd64 go build -o ../tools/xworkbench-cli-skill/xworkbench-cli-windows-amd64.exe .
```
