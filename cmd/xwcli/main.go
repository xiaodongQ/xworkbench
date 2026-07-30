// xwcli - xworkbench 远程 agent 客户端（Go 原生二进制）
package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	flagServer string
	flagName   string
)

func main() {
	if len(os.Args) < 2 {
		printUsage(nil)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "register":
		fs := flag.NewFlagSet("register", flag.ExitOnError)
		fs.StringVar(&flagServer, "server", "", "xworkbench server URL (e.g. http://localhost:8902)")
		fs.StringVar(&flagName, "name", hostname(), "machine name for this agent")
		if err := fs.Parse(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := cmdRegister(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "run":
		if err := cmdRun(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "status":
		if err := cmdStatus(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "stop":
		if err := cmdStop(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "version", "--version", "-version":
		fmt.Println("xwcli 1.0.0")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

func printUsage(_ *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, `xwcli - xworkbench 远程 agent 客户端

用法:
    xwcli register --server <url> --name <machine-name>
    xwcli run
    xwcli status
    xwcli stop

选项:
  --server string    xworkbench server URL (register 子命令)
  --name string      机器名称 (register 子命令，默认本机 hostname)
`)
}

// hostname returns the machine hostname, falling back to "unknown".
func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
