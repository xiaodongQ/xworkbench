// xwcli - xworkbench 远程 agent 客户端（Go 原生二进制）
package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	flagServer  string
	flagName    string
	flagVersion bool
)

func main() {
	fs := flag.NewFlagSet("xwcli", flag.ContinueOnError)
	fs.StringVar(&flagServer, "server", "", "xworkbench server URL (e.g. http://localhost:8902)")
	fs.StringVar(&flagName, "name", hostname(), "machine name for this agent")
	fs.BoolVar(&flagVersion, "version", false, "print version")

	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			printUsage(fs)
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if flagVersion {
		fmt.Println("xwcli 1.0.0")
		return
	}

	if fs.NArg() < 1 {
		printUsage(fs)
		os.Exit(1)
	}

	cmd := fs.Arg(0)

	var err error
	switch cmd {
	case "register":
		err = cmdRegister()
	case "run":
		err = cmdRun()
	case "status":
		err = cmdStatus()
	case "stop":
		err = cmdStop()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage(fs)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage(fs *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, `xwcli - xworkbench 远程 agent 客户端

用法:
    xwcli register --server <url> --name <machine-name>
    xwcli run
    xwcli status
    xwcli stop

选项:
`)
	fs.PrintDefaults()
}

// hostname returns the machine hostname, falling back to "unknown".
func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
