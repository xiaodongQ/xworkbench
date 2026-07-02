package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/logger"
	"golang.org/x/crypto/ssh"
)

// SSHConfig 描述一次 SSH 远程执行的连接配置。
// 从 DirShortcut 派生（host/user/password/key_path/auth_method/port）。
type SSHConfig struct {
	Host         string // IP 或域名
	Port         int    // 默认 22
	User         string
	AuthMethod   string // "password" | "key"，默认 "password"
	Password     string // AuthMethod=password 时使用
	KeyPath      string // AuthMethod=key 时使用
	KeyPassword  string // AuthMethod=key 时使用（加密私钥的密码，为空表示无密码）
	TimeoutSec   int    // SSH 连接超时（秒），默认 10
}

// sshClient 抽象 *ssh.Client 的方法，便于 mock 测试。
// 真实实现用 *ssh.Client；测试里用 fakeSSHClient 启动本地 sshd-in-process 或 mock 响应。
type sshClient interface {
	NewSession() (*ssh.Session, error)
	Close() error
}

// dialSSH 建立 SSH 连接（password 或 key 两种鉴权）。
// 出于简化直接内联在 RunSSHViaConfig 入口；这里抽出来方便单测（暂时只覆盖 happy path）。
func dialSSH(cfg SSHConfig) (*ssh.Client, error) {
	if cfg.Host == "" {
		return nil, errors.New("ssh: host is required")
	}
	if cfg.User == "" {
		return nil, errors.New("ssh: user is required")
	}
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, port)
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	var auth []ssh.AuthMethod
	switch cfg.AuthMethod {
	case "key":
		if cfg.KeyPath == "" {
			return nil, errors.New("ssh: key_path required for key auth")
		}
		key, err := readPrivateKey(cfg.KeyPath, cfg.KeyPassword)
		if err != nil {
			return nil, fmt.Errorf("ssh: read key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(key))
	default: // "password" 或空（兼容老 dir_shortcut）
		if cfg.Password == "" {
			return nil, errors.New("ssh: password required for password auth")
		}
		auth = append(auth, ssh.Password(cfg.Password))
	}
	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		Timeout:         timeout,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 演示用，生产建议用 known_hosts
	}
	logger.Logger.Infow("ssh: dialing", "addr", addr, "user", cfg.User, "auth", cfg.AuthMethod)
	return ssh.Dial("tcp", addr, sshCfg)
}

// runOnClient 在已建好的 SSH client 上执行命令并流式回调 onChunk。
// 与本地 executor.Run 接口对齐：返回 *Result，ctx 取消会 kill 远端进程。
// onChunk 收到的文本片段以 "\n" 结尾（与本地行为一致）。
func runOnClient(ctx context.Context, client *ssh.Client, cmd []string, stdin string, onChunk func(string)) (*Result, error) {
	if len(cmd) == 0 {
		return nil, errors.New("empty command")
	}
	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh: new session: %w", err)
	}
	defer sess.Close()
	// request pty? 不需要。claude/cbc/codex 都能非交互跑。
	modes := ssh.TerminalModes{ssh.ECHO: 0}
	if err := sess.RequestPty("xterm", 80, 40, modes); err != nil {
		// pty 失败不阻塞 — 部分 CLI 在非 pty 下也跑得动
		logger.Logger.Warnw("ssh: request pty failed (continue without pty)", "error", err.Error())
	}
	// 拼命令字符串（远端用 sh -c 跑）
	remoteCmd := strings.Join(quoteArgs(cmd), " ")
	stdinPipe, err := sess.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("ssh: stdin pipe: %w", err)
	}
	stdoutPipe, err := sess.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ssh: stdout pipe: %w", err)
	}
	stderrPipe, err := sess.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("ssh: stderr pipe: %w", err)
	}
	if err := sess.Start(remoteCmd); err != nil {
		return nil, fmt.Errorf("ssh: start: %w", err)
	}
	// stdin 写入（如果非空）
	if stdin != "" {
		go func() {
			defer stdinPipe.Close()
			_, _ = stdinPipe.Write([]byte(stdin))
		}()
	} else {
		_ = stdinPipe.Close()
	}
	// 并发读 stdout/stderr
	var outBuilder, errBuilder strings.Builder
	var wgDoneCh = make(chan struct{})
	go func() {
		streamLines(stdoutPipe, false, onChunk, &outBuilder)
		close(wgDoneCh)
	}()
	streamStderr := make(chan struct{})
	go func() {
		streamLines(stderrPipe, true, onChunk, &errBuilder)
		close(streamStderr)
	}()
	// 监听 ctx 取消：远端不能直接 SIGKILL 当前 shell 进程组，session.Signal(SIGKILL) 是最佳近似
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		select {
		case <-ctx.Done():
			_ = sess.Signal(ssh.SIGKILL)
		case <-stopWatch:
		}
	}()
	waitErr := sess.Wait()
	<-wgDoneCh
	<-streamStderr
	res := &Result{
		Output:   outBuilder.String(),
		ErrorOut: errBuilder.String(),
		CmdStr:   remoteCmd,
		ExitCode: 0,
		Err:      waitErr,
	}
	if waitErr != nil {
		// 尝试从 exit status 取退出码
		if exitErr, ok := waitErr.(*ssh.ExitError); ok {
			res.ExitCode = exitErr.ExitStatus()
		} else {
			res.ExitCode = -1
		}
	}
	return res, nil
}

// RunSSHViaConfig 一步：拨号 + 远端执行。给上层 handler 直接用。
func RunSSHViaConfig(ctx context.Context, cfg SSHConfig, cmd []string, stdin string, onChunk func(string)) (*Result, error) {
	client, err := dialSSH(cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return runOnClient(ctx, client, cmd, stdin, onChunk)
}
