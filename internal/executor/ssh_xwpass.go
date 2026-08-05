package executor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/logger"
)

// ResolveXwSshpassBin 返回当前平台对应的 xw-sshpass 路径。
// 优先从 tools/xw-sshpass/ 目录查找，找不到则尝试 PATH。
func ResolveXwSshpassBin() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	osMap := map[string]string{
		"darwin":  "darwin",
		"linux":   "linux",
		"windows": "windows",
	}
	osStr := osMap[goos]
	if osStr == "" {
		return ""
	}
	archStr := "amd64"
	if goarch == "arm64" {
		archStr = "arm64"
	}
	binName := fmt.Sprintf("xw-sshpass-%s-%s", osStr, archStr)
	if goos == "windows" {
		binName += ".exe"
	}

	// 1. 从 tools/xw-sshpass/ 目录查找
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	toolsDir := filepath.Join(wd, "tools", "xw-sshpass")
	binPath := filepath.Join(toolsDir, binName)
	if _, err := os.Stat(binPath); err == nil {
		return binPath
	}

	// 2. PATH 中查找
	if bin, err := exec.LookPath(binName); err == nil {
		return bin
	}
	// 3. 尝试不带平台后缀的 xw-sshpass
	if bin, err := exec.LookPath("xw-sshpass"); err == nil {
		return bin
	}
	return ""
}

// RunViaXwSshpass 通过 xw-sshpass 子进程在远程机器上执行命令。
// 和 web 终端的连接方式一致（交互式 shell），确保 .zshrc/.bashrc 等环境正确加载。
func RunViaXwSshpass(ctx context.Context, ds *backend.DirShortcut, cmd []string, stdin string, chunkCB func(string)) (*Result, error) {
	xwBin := ResolveXwSshpassBin()
	if xwBin == "" {
		return nil, fmt.Errorf("xw-sshpass binary not found")
	}

	userHost := ds.RemoteUser
	if userHost == "" {
		userHost = "root"
	}
	userHost = userHost + "@" + ds.RemoteHost
	// 注意：xw-sshpass 不会从 user@host:port 剥离 port，
	// 必须用其顶层 -P <port> flag 显式传端口。

	var args []string
	if ds.AuthMethod == "key" {
		keyPath := ResolveKeyPath(ds)
		if keyPath == "" {
			return nil, fmt.Errorf("ssh key not found")
		}
		args = append(args, "-i", keyPath)
	} else {
		if ds.RemotePassword == "" {
			return nil, fmt.Errorf("no password or key configured for %s", ds.Name)
		}
		args = append(args, "-p", ds.RemotePassword)
	}
	// 端口（非默认值才显式传，win-sshpass 内部默认 22）
	if ds.RemotePort != "" && ds.RemotePort != "22" {
		args = append(args, "-P", ds.RemotePort)
	}
	args = append(args, "ssh", userHost)
	// Shell wrapper: mkdir xworkbench-task, cd, source rc files, zsh priority bash fallback
	// sh -c 需要用单引号包裹命令字符串
	cmdStr := strings.Join(cmd, " ")
	if len(cmd) >= 3 && (cmd[0] == "sh" || cmd[0] == "bash") && cmd[1] == "-c" {
		// sh -c <script> → sh -c '<script>' 避免参数拆分
		cmdStr = cmd[0] + " -c '" + strings.ReplaceAll(cmd[2], "'", "'\\''") + "'"
	}
	escaped := strings.ReplaceAll(cmdStr, "'", "'\\''")
	shellCmd := fmt.Sprintf(
		`mkdir -p ~/xworkbench-task && cd ~/xworkbench-task && command -v zsh >/dev/null 2>&1 && zsh -c 'source ~/.zshrc 2>/dev/null; %s' || bash -c 'source ~/.bashrc 2>/dev/null; %s'`,
		escaped, escaped)
	args = append(args, shellCmd)

	logger.Logger.Infow("task run: xw-sshpass", "bin", xwBin, "args", args)

	execCmd := exec.CommandContext(ctx, xwBin, args...)
	var outBuilder, errBuilder strings.Builder

	stdoutPipe, err := execCmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := execCmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if stdin != "" {
		stdinPipe, err := execCmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		go func() {
			defer stdinPipe.Close()
			stdinPipe.Write([]byte(stdin))
		}()
	}
	if err := execCmd.Start(); err != nil {
		return nil, fmt.Errorf("xw-sshpass start: %w", err)
	}

	outDone := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text() + "\n"
			outBuilder.WriteString(line)
			if chunkCB != nil {
				chunkCB(line)
			}
		}
		close(outDone)
	}()

	errDone := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := "[err] " + scanner.Text() + "\n"
			errBuilder.WriteString(line)
			if chunkCB != nil {
				chunkCB(line)
			}
		}
		close(errDone)
	}()

	<-outDone
	<-errDone

	waitErr := execCmd.Wait()
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return &Result{
		Output:   outBuilder.String(),
		ErrorOut: errBuilder.String(),
		ExitCode: exitCode,
	}, nil
}
