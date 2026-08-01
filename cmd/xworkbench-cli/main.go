// xworkbench-cli - xworkbench Agent CLI 工具
package main

import (
	"flag"
	"fmt"
	"os"
)

var flagServer string

func main() {
	if len(os.Args) < 2 {
		printUsage(nil)
		os.Exit(1)
	}

	// 全局 --server 参数
	flag.StringVar(&flagServer, "server", "http://localhost:8902", "xworkbench server URL")
	flag.Parse()

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "task":
		if err := runTask(args); err != nil {
			fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
			os.Exit(1)
		}
	case "exec":
		if err := runExec(args); err != nil {
			fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
			os.Exit(1)
		}
	case "experience":
		if err := runExperience(args); err != nil {
			fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
			os.Exit(1)
		}
	case "scheduled":
		if err := runScheduled(args); err != nil {
			fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
			os.Exit(1)
		}
	case "todo":
		if err := runTodo(args); err != nil {
			fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
			os.Exit(1)
		}
	case "config":
		if err := runConfig(args); err != nil {
			fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
			os.Exit(1)
		}
	case "models":
		if err := runModels(args); err != nil {
			fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
			os.Exit(1)
		}
	case "stats":
		if err := runStats(args); err != nil {
			fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
			os.Exit(1)
		}
	case "dir-shortcut":
		if err := runDirShortcut(args); err != nil {
			fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
			os.Exit(1)
		}
	case "web-link":
		if err := runWebLink(args); err != nil {
			fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": %q}\n", err.Error())
			os.Exit(1)
		}
	case "help", "-h", "--help":
		printUsage(nil)
	default:
		fmt.Fprintf(os.Stderr, "{\"ok\": false, \"error\": \"unknown command: %q\"}\n", cmd)
		os.Exit(1)
	}
}

func printUsage(_ *flag.FlagSet) {
	fmt.Println(`xworkbench-cli - xworkbench Agent CLI 工具

Usage: xworkbench-cli [OPTIONS] <command> [subcommand] [args...]

Commands:
  task          Task operations (list/create/get/update/run/cancel/delete)
  exec          Execution operations (list/get/evaluate/cancel/continue)
  experience    Experience operations (list/get/create/update/delete)
  scheduled     Scheduled task operations (list/get/create/update/delete/run/toggle)
  todo          Todo operations (list/add/toggle/edit)
  config        Config operations (get/set)
  models        List available models
  stats         Get dashboard statistics
  dir-shortcut  Directory shortcut operations (list/create/update/delete/open)
  web-link      Web link operations (list/create/update/delete/open)

Options:
  --server      xworkbench server URL (default: http://localhost:8902)

Examples:
  xworkbench-cli task list --status pending
  xworkbench-cli task create --title "My task" --description "Details"
  xworkbench-cli task run 123
  xworkbench-cli exec list --task-id 123
  xworkbench-cli stats
`)
}
