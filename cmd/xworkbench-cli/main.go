// xworkbench-cli - xworkbench Agent CLI 工具
package main

import (
	"fmt"
	"os"
)

var flagServer string

func main() {
	// 环境变量
	if env := os.Getenv("XWORKBENCH_SERVER"); env != "" && flagServer == "" {
		flagServer = env
	}

	// 解析 --server flag 后提取子命令
	cmd := ""
	var cmdArgs []string
	for i := 1; i < len(os.Args); i++ {
		a := os.Args[i]
		if a == "--server" || a == "-server" {
			if i+1 < len(os.Args) {
				flagServer = os.Args[i+1]
				i++
			}
			continue
		}
		if cmd == "" {
			cmd = a
		} else {
			cmdArgs = append(cmdArgs, a)
		}
	}

	if flagServer == "" {
		flagServer = "http://localhost:8902"
	}

	if cmd == "" || cmd == "help" || cmd == "-h" || cmd == "--help" {
		if len(cmdArgs) > 0 {
			printCommandHelp(cmdArgs[0])
		} else {
			printUsage()
		}
		os.Exit(0)
	}

	switch cmd {
	case "manual-task":
		handleTask(cmdArgs)
	case "sched-task":
		handleScheduled(cmdArgs)
	case "tasks":
		handleTasksAll(cmdArgs)
	case "exec":
		handleExec(cmdArgs)
	case "experience":
		handleExperience(cmdArgs)
	case "todo":
		handleTodo(cmdArgs)
	case "config":
		handleConfig(cmdArgs)
	case "models":
		handleModels(cmdArgs)
	case "stats":
		handleStats(cmdArgs)
	case "dir-shortcut":
		handleDirShortcut(cmdArgs)
	case "web-link":
		handleWebLink(cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": \"unknown command: %q\"}\n", cmd)
		os.Exit(1)
	}
}

func handleTask(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printTaskHelp()
		return
	}
	if err := runTask(args); err != nil {
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
		os.Exit(1)
	}
}

func handleExec(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printExecHelp()
		return
	}
	if err := runExec(args); err != nil {
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
		os.Exit(1)
	}
}

func handleExperience(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printExperienceHelp()
		return
	}
	if err := runExperience(args); err != nil {
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
		os.Exit(1)
	}
}

func handleScheduled(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printScheduledHelp()
		return
	}
	if err := runScheduled(args); err != nil {
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
		os.Exit(1)
	}
}

func handleTasksAll(args []string) {
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help") {
		fmt.Print("tasks - 查询所有任务（手动+定时）\n\n")
		fmt.Print("用法: xworkbench-cli tasks [options]\n\n")
		fmt.Print("同时查询手动任务和定时任务，合并返回。\n")
		fmt.Print("常用选项: --task-type local|remote  --status pending\n")
		return
	}
	// 并发调用两个 list
	fmt.Println(`{"manual_tasks":`)
	taskList(args)
	fmt.Println(`, "sched_tasks":`)
	scheduledList(args)
	fmt.Println(`}`)
}

func handleTodo(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printTodoHelp()
		return
	}
	if err := runTodo(args); err != nil {
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
		os.Exit(1)
	}
}

func handleConfig(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printConfigHelp()
		return
	}
	if err := runConfig(args); err != nil {
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
		os.Exit(1)
	}
}

func handleModels(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printModelsHelp()
		return
	}
	if err := runModels(args); err != nil {
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
		os.Exit(1)
	}
}

func handleStats(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printStatsHelp()
		return
	}
	if err := runStats(args); err != nil {
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
		os.Exit(1)
	}
}

func handleDirShortcut(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printDirShortcutHelp()
		return
	}
	if err := runDirShortcut(args); err != nil {
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
		os.Exit(1)
	}
}

func handleWebLink(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printWebLinkHelp()
		return
	}
	if err := runWebLink(args); err != nil {
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`xworkbench-cli - xworkbench 工作台 Agent CLI 工具

用法: xworkbench-cli [OPTIONS] <command> [subcommand] [args...]

xworkbench-cli 提供对 xworkbench 工作台的完整操作能力，包括任务管理、
执行控制、经验库、定时调度、配置管理等。所有命令返回 JSON 格式输出。

全局选项:
  --server string
        xworkbench 服务器地址 (默认: http://localhost:8902)
        可通过环境变量 XWORKBENCH_SERVER 覆盖

命令:
  tasks         查询所有任务（手动+定时，自动合并）
  manual-task   手动任务 (一次执行，支持远程 --target-dir-id)
  sched-task    定时任务 (cron 调度，支持远程，需先启调度器)
  exec          执行记录 (list/get/evaluate/cancel/continue)
  experience    经验库 (list/get/create/update/delete)
  todo          Todo 管理 (list/add/toggle/edit)
  config        配置管理 (get/set)
  models        可用模型列表
  stats         仪表盘统计
  dir-shortcut  目录快捷方式 (list/create/update/delete/open)
  web-link      链接快捷方式 (list/create/update/delete/open)

调度器说明:
  定时任务的 cron 调度需要服务端调度器运行才能自动触发。
  - 调度器状态：xworkbench-cli stats (查看 running 字段)
  - 调度器启停需在 Web 工作台操作，CLI 不支持远程启停

使用 "xworkbench-cli help <command>" 查看特定命令的详细帮助。

示例:
  xworkbench-cli manual-task list --status pending
  xworkbench-cli manual-task list --task-type remote
  xworkbench-cli sched-task list --task-type remote
  xworkbench-cli sched-task run <id>
  xworkbench-cli exec list --limit 10
  xworkbench-cli stats
  xworkbench-cli task run 123
  xworkbench-cli exec list --task-id 123
  xworkbench-cli stats
  xworkbench-cli --server http://x:8902 task list
`)
}

func printCommandHelp(cmd string) {
	switch cmd {
	case "manual-task":
		printTaskHelp()
	case "sched-task":
		printScheduledHelp()
	case "exec":
	case "todo":
		printTodoHelp()
	case "config":
		printConfigHelp()
	case "models":
		printModelsHelp()
	case "stats":
		printStatsHelp()
	case "dir-shortcut":
		printDirShortcutHelp()
	case "web-link":
		printWebLinkHelp()
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n", cmd)
		os.Exit(1)
	}
}

func printTaskHelp() {
	fmt.Print(`manual-task - 手动任务

用法: xworkbench-cli manual-task <subcommand> [options]

子命令:
  list          列出手动任务，支持按状态/类型过滤
  create        创建手动任务
  get           获取任务详情
  update        更新任务信息
  run           运行任务
  cancel        取消正在运行的任务
  delete        删除任务

示例:
  xworkbench-cli task list                           # 列出所有任务
  xworkbench-cli task list --status pending          # 列出 pending 任务
  xworkbench-cli task list --status running --limit 20
  xworkbench-cli task create --title "新任务" --description "任务描述"
  xworkbench-cli task get 123                        # 获取 ID=123 的任务
  xworkbench-cli task update 123 --status completed --priority 3
  xworkbench-cli task run 123                        # 运行任务
  xworkbench-cli task cancel 123                    # 取消运行
  xworkbench-cli task delete 123                    # 删除任务

任务状态: pending / running / completed / failed / cancelled
`)
}

func printExecHelp() {
	fmt.Print(`exec - 执行记录

用法: xworkbench-cli exec <subcommand> [options]

子命令:
  list          列出执行记录
  get           获取执行详情
  evaluate      对执行结果进行 AI 评估
  cancel        取消正在执行的任务
  continue      继续之前的对话/执行

示例:
  xworkbench-cli exec list                          # 列出最近执行
  xworkbench-cli exec list --task-id 123            # 列出属于任务 123 的执行
  xworkbench-cli exec list --limit 10               # 只显示 10 条
  xworkbench-cli exec get abc123                    # 获取执行详情
  xworkbench-cli exec evaluate abc123               # AI 评估此执行
  xworkbench-cli exec cancel abc123                 # 取消执行
  xworkbench-cli exec continue abc123               # 继续对话
`)
}

func printExperienceHelp() {
	fmt.Print(`experience - 经验库

用法: xworkbench-cli experience <subcommand> [options]

经验库存储可复用的知识片段，包括模块、场景、关键词、工具用法等。

子命令:
  list          列出经验库条目
  get           获取经验详情
  create        创建新经验
  update        更新经验信息
  delete        删除经验

示例:
  xworkbench-cli experience list                    # 列出所有经验
  xworkbench-cli experience list --module redis    # 只列出 redis 模块的经验
  xworkbench-cli experience get 456                # 获取经验详情
  xworkbench-cli experience create \
    --module redis \
    --scene "慢查询优化" \
    --keywords "slowlog,index,explain" \
    --tool-usage "redis-cli slowlog get 10" \
    --code-snippets "# Redis 慢查询分析\nSLOWLOG GET 10"
  xworkbench-cli experience update 456 --keywords "slowlog,index,explain,performance"
  xworkbench-cli experience delete 456
`)
}

func printScheduledHelp() {
	fmt.Print(`sched-task - 定时任务

用法: xworkbench-cli sched-task <subcommand> [options]

定时任务基于 cron 表达式由服务端调度器自动触发，支持本地/远程执行。

调度器:
  需在 Web 工作台手动启动调度器，启动后 cron 才会按周期自动执行。
  立即运行 (run) 无需调度器启动。

远程任务:
  创建时加 --target-dir-id <id> 指定远程 DirShortcut，调度器自动通过 SSH 执行。

子命令:
  list          列出所有定时任务 (--task-type local|remote)
  get           获取定时任务详情
  create        创建定时任务 (--name --cron --prompt --command-type [--target-dir-id])
  update        更新定时任务 (id --name --cron ... [--target-dir-id])
  delete        删除定时任务
  run           立即运行（不依赖调度器）
  toggle        启用/禁用切换

cron 表达式格式:
  ┌───────────── 分钟 (0-59)
  │ ┌─────────── 小时 (0-23)
  │ │ ┌───────── 日 (1-31)
  │ │ │ ┌─────── 月 (1-12)
  │ │ │ │ ┌───── 星期 (0-6, 0=周日)
  │ │ │ │ │
  * * * * *

常用示例:
  @every 5m    每 5 分钟
  @hourly      每小时
  @daily       每天午夜
  0 9 * * *    每天 9:00

示例:
  xworkbench-cli scheduled list
  xworkbench-cli scheduled get 789
  xworkbench-cli scheduled create \
    --name "每日备份" \
    --cron "0 2 * * *" \
    --command-type shell \
    --prompt "backup.sh"
  xworkbench-cli scheduled update 789 --cron "0 3 * * *"  # 修改执行时间
  xworkbench-cli scheduled run 789                       # 立即执行
  xworkbench-cli scheduled toggle 789                    # 启用/禁用
  xworkbench-cli scheduled delete 789
`)
}

func printTodoHelp() {
	fmt.Print(`todo - Todo 管理

用法: xworkbench-cli todo <subcommand> [options]

Todo 使用行号操作，支持增删改查和完成状态切换。

子命令:
  list          列出所有 todo
  add           添加新 todo
  toggle        切换完成状态
  edit          编辑 todo 内容

示例:
  xworkbench-cli todo list                      # 列出所有 todo
  xworkbench-cli todo add --content "完成报告"  # 添加新 todo
  xworkbench-cli todo toggle 3                 # 切换第 3 行的完成状态
  xworkbench-cli todo edit 3 "新内容"          # 修改第 3 行内容

注意: line_no 是 todo list 中显示的行号，不是 ID
`)
}

func printConfigHelp() {
	fmt.Print(`config - 配置管理

用法: xworkbench-cli config <subcommand> [key] [value]

获取或设置 xworkbench 配置。配置存储在 config.json 文件中。

子命令:
  get           获取配置 (可选: 指定 key)
  set           设置配置

常用配置项:
  default_terminal    默认终端类型 (wezterm/iterm2/terminal)
  preferred_cli       优先 CLI (claude/cbc)
  ai_loop_enabled     AI 循环能力开关 (true/false)
  aichat_default_cli  AI 对话默认 CLI
  scheduler_enabled   调度器启用状态

示例:
  xworkbench-cli config get                      # 获取全部配置
  xworkbench-cli config get default_terminal    # 获取特定项
  xworkbench-cli config set default_terminal wezterm
  xworkbench-cli config set ai_loop_enabled true
`)
}

func printModelsHelp() {
	fmt.Print(`models - 可用模型列表

用法: xworkbench-cli models list

列出 xworkbench 中配置的所有可用 AI 模型。

输出包括每个模型的:
  - 模型名称/ID
  - 模型类型 (claude/cbc)
  - 是否为默认模型
  - 可用选项

示例:
  xworkbench-cli models list
`)
}

func printStatsHelp() {
	fmt.Print(`stats - 仪表盘统计

用法: xworkbench-cli stats

获取 xworkbench 仪表盘统计数据，包括:
  - 任务统计 (总数、待处理、进行中、已完成、失败)
  - 执行统计 (总执行数、最近执行)
  - 定时任务状态
  - 经验库数量

示例:
  xworkbench-cli stats
`)
}

func printDirShortcutHelp() {
	fmt.Print(`dir-shortcut - 目录快捷方式

用法: xworkbench-cli dir-shortcut <subcommand> [options]

管理目录快捷方式，点击可在文件管理器中打开对应目录。

子命令:
  list          列出所有目录快捷方式
  create        创建目录快捷方式
  update        更新目录快捷方式
  delete        删除目录快捷方式
  open          在文件管理器中打开目录

示例:
  xworkbench-cli dir-shortcut list
  xworkbench-cli dir-shortcut create --name "项目目录" --path ~/projects
  xworkbench-cli dir-shortcut update 1 --name "新名称"
  xworkbench-cli dir-shortcut open 1
  xworkbench-cli dir-shortcut delete 1
`)
}

func printWebLinkHelp() {
	fmt.Print(`web-link - 链接快捷方式

用法: xworkbench-cli web-link <subcommand> [options]

管理网页链接快捷方式，点击可打开对应 URL。

子命令:
  list          列出所有链接快捷方式
  create        创建链接快捷方式
  update        更新链接快捷方式
  delete        删除链接快捷方式
  open          在浏览器中打开链接

示例:
  xworkbench-cli web-link list
  xworkbench-cli web-link create --title "Google" --url "https://google.com"
  xworkbench-cli web-link update 1 --title "搜索" --url "https://bing.com"
  xworkbench-cli web-link open 1
  xworkbench-cli web-link delete 1
`)
}
