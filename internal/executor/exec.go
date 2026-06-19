// Package executor 提供子进程流式执行 + 4 种人工确认信号检测。
package executor

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/logger"
)


// SetLogger 供 server 注 入已配置好的 logger，避免各自初始化写到 stderr。

// Result 一次执行的完整结果。
type Result struct {
	Output   string
	ErrorOut string
	CmdStr   string
	ExitCode int
	Err      error // 启动/等待过程中的错误（如 ctx 超时）
}

// Run 启动子进程并流式回调 stdout/stderr。ctx 取消会 kill 子进程。
// onChunk 收到的每段以 "\n" 结尾的文本片段（来自 stdout 或 stderr，前者无前缀，后者带 "[err] "）。
//
// dir 非空时设置子进程的工作目录（用于落地 ScheduledTask.WorkingDir）。
// dir 为空时继承父进程 cwd（evaluator 等场景不需要指定）。
//
// stdin 非空时通过管道写入子进程标准输入。Windows cmd.exe 命令行长度
// 限制 8191 字符，长 prompt 必须走 stdin 否则会被截断。shell 类型的
// prompt 走临时脚本文件，不通过 stdin。
func Run(ctx context.Context, cmd []string, dir string, stdin string, onChunk func(string)) (*Result, error) {
	if len(cmd) == 0 {
		return nil, errors.New("empty command")
	}
	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	if dir != "" {
		c.Dir = dir
	}
	if stdin != "" {
		stdinPipe, err := c.StdinPipe()
		if err != nil {
			return nil, err
		}
		go func() {
			defer stdinPipe.Close()
			io.WriteString(stdinPipe, stdin)
		}()
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		return nil, err
	}
	started := time.Now()
	if err := c.Start(); err != nil {
		return nil, err
	}
	logger.Logger.Debugw("executor: process started", "cmd", truncateCmd(cmd), "pid", c.Process.Pid)

	var out, errBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, e := stdout.Read(buf)
			if n > 0 {
				s := string(buf[:n])
				out.WriteString(s)
				if onChunk != nil {
					onChunk(s)
				}
			}
			if e != nil {
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, e := stderr.Read(buf)
			if n > 0 {
				s := string(buf[:n])
				errBuf.WriteString(s)
				if onChunk != nil {
					onChunk("[err] " + s)
				}
			}
			if e != nil {
				return
			}
		}
	}()

	waitErr := c.Wait()
	wg.Wait()

	exit := -1
	if c.ProcessState != nil {
		exit = c.ProcessState.ExitCode()
	}
	res := &Result{
		Output:   out.String(),
		ErrorOut: errBuf.String(),
		CmdStr:   strings.Join(cmd, " "),
		ExitCode: exit,
	}
	if waitErr != nil {
		// ctx 超时返回的 error 带 "signal: killed"
		res.Err = waitErr
	}
	if exit != 0 || waitErr != nil {
		logger.Logger.Errorw("executor: process exited",
			"cmd", truncateCmd(cmd),
			"exit_code", exit,
			"dur_ms", time.Since(started).Milliseconds(),
			"err", errStr(waitErr),
			"stdout_bytes", len(res.Output),
			"stderr_bytes", len(res.ErrorOut),
			"stderr", res.ErrorOut,
		)
	} else {
		logger.Logger.Infow("executor: process exited",
			"cmd", truncateCmd(cmd),
			"exit_code", exit,
			"dur_ms", time.Since(started).Milliseconds(),
			"err", errStr(waitErr),
			"stdout_bytes", len(res.Output),
			"stderr_bytes", len(res.ErrorOut),
		)
	}
	return res, nil
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// truncateCmd 把命令行拼成可读字符串,超过 200 字符截断(避免 AI CLI 的长 prompt
// 把日志撑爆)。truncateCmd 同时会标记原长度,方便定位"是不是 prompt 太长"。
func truncateCmd(cmd []string) string {
	full := strings.Join(cmd, " ")
	const max = 200
	if len(full) <= max {
		return full
	}
	return full[:max] + "...[truncated, total " + strconv.Itoa(len(full)) + " chars]"
}
