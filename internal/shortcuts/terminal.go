package shortcuts

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"github.com/xiaodongQ/xworkbench/internal/executor"
	"github.com/xiaodongQ/xworkbench/internal/logger"
)



// IsSupportedTerminal 检查终端类型是否支持
func IsSupportedTerminal(termType string) bool {
	cfg := config.Get()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	_, ok := cfg.Terminal.Types[strings.ToLower(termType)]
	return ok
}

// DetectTerminalPath 检测终端类型的可执行文件路径，优先从 PATH 找，找不到时探测配置中的路径。
func DetectTerminalPath(termType string) string {
	cfg := config.Get()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	key := strings.ToLower(termType)
	typeDef, ok := cfg.Terminal.Types[key]
	if !ok {
		return ""
	}
	// 1. PATH 中找 bin 名称
	if path, err := exec.LookPath(typeDef.Bin); err == nil {
		return path
	}
	// 2. 探测配置中的路径
	if paths, ok := cfg.Terminal.DetectPaths[key]; ok {
		for _, p := range paths {
			if strings.HasPrefix(p, "~/") {
				if home := os.Getenv("HOME"); home != "" {
					p = home + p[1:]
				}
			}
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	// 3. PATH 二次尝试
	if path, err := exec.LookPath(typeDef.Bin); err == nil {
		return path
	}
	return ""
}

// DefaultTerminal 返回配置的默认终端类型（顶层字段，不再位于 terminal.default_type）
func DefaultTerminal() string {
	cfg := config.Get()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return cfg.DefaultTerminal
}

// buildRemoteArgs 根据终端类型和 DirShortcut 构建远程唤起的完整 args。
// 优先使用 config 中预置的 RemoteArgs 模板；未知终端类型用泛用 ssh 兜底。
// 变量替换：{user}、{host}、{key_path}、{shell_cmd}。
// shell_cmd 规则：cd 到 remote_path（如有）+ TerminalCmd（如有） + exec $SHELL -l。
func buildRemoteArgs(termType string, dir *backend.DirShortcut, keyPath string) []string {
	// 构建 ssh target
	sshTarget := dir.RemoteHost
	if dir.RemoteUser != "" {
		sshTarget = dir.RemoteUser + "@" + dir.RemoteHost
	}

	// 构建 shell_cmd
	shellCmd := buildShellCmd(dir)

	// 查配置中的 RemoteArgs 模板
	cfg := config.Get()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	termDef, ok := cfg.Terminal.Types[strings.ToLower(termType)]

	var template []string
	if ok && len(termDef.RemoteArgs) > 0 {
		template = termDef.RemoteArgs
	} else {
		// 兜底：泛用 ssh 命令
		// 只有密钥文件存在时才传 -i 参数，避免 "Identity file not accessible" 警告
		if keyPath != "" && fileExists(keyPath) {
			template = []string{"ssh", "-i", "{key_path}", "{user}@{host}", "-t", "--", "sh", "-c", "{shell_cmd}"}
		} else {
			template = []string{"ssh", "{user}@{host}", "-t", "--", "sh", "-c", "{shell_cmd}"}
		}
	}

	// 变量替换
	result := make([]string, 0, len(template))
	for _, arg := range template {
		arg = strings.ReplaceAll(arg, "{key_path}", shellQuote(keyPath))
		arg = strings.ReplaceAll(arg, "{user}@{host}", sshTarget)
		arg = strings.ReplaceAll(arg, "{host}", dir.RemoteHost)
		arg = strings.ReplaceAll(arg, "{user}", dir.RemoteUser)
		arg = strings.ReplaceAll(arg, "{shell_cmd}", shellQuote(shellCmd))
		result = append(result, arg)
	}
	return result
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// buildShellCmd 构建远端执行的 shell 命令。
// 规则：cd remote_path（如有） → TerminalCmd（如有） → exec $SHELL -l。
func buildShellCmd(dir *backend.DirShortcut) string {
	parts := []string{}
	if dir.RemotePath != "" {
		parts = append(parts, "cd '"+dir.RemotePath+"'")
	}
	if dir.TerminalCmd != "" {
		parts = append(parts, dir.TerminalCmd)
	}
	parts = append(parts, "exec $SHELL -l")
	return strings.Join(parts, " && ")
}

// shellQuote 给字符串加单引号并转义内部单引号（简单实现，用于命令行参数）。
func shellQuote(s string) string {
	if s == "" {
		return ""
	}
	// 将 ' 替换为 '\''（单引号-反斜单引号-单引号-单引号）
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// OpenRemoteDirShortcut 用配置的终端软件打开远程 SSH 连接。
// 支持 wezterm 等终端，连接后 cd 到 remote_path（默认主目录）。
// 优先使用密钥认证：自动解析密钥路径（LocalKeyPath > KeyPath > 全局默认 > ~/.ssh/xworkbench_id_ed25519）。
func OpenRemoteDirShortcut(dir *backend.DirShortcut, termType, binPath string) error {
	return openRemoteDirShortcutImpl(context.Background(), dir, termType, binPath, false)
}

// OpenRemoteDirShortcutWithKeyAuth 打开远程终端，并确保密钥免密配置已就绪（生成密钥对 + 上传公钥）。
// 适用于用户首次点击"打开终端"时的引导流程。
func OpenRemoteDirShortcutWithKeyAuth(ctx context.Context, dir *backend.DirShortcut, termType, binPath string) error {
	return openRemoteDirShortcutImpl(ctx, dir, termType, binPath, true)
}

func openRemoteDirShortcutImpl(ctx context.Context, dir *backend.DirShortcut, termType, binPath string, ensureKeyAuth bool) error {
	if dir.Type != backend.DirShortcutTypeRemote {
		return fmt.Errorf("not a remote shortcut: type=%s", dir.Type)
	}

	// 解析密钥路径（优先级：LocalKeyPath > KeyPath > 全局默认 > 兜底）
	keyPath := executor.ResolveKeyPath(dir)

	// 可选：确保密钥免密已配置（首次使用时）
	if ensureKeyAuth {
		_, err := executor.EnsureKeyAuthAvailable(ctx, dir)
		if err != nil {
			logger.Logger.Warnw("[OpenRemoteDirShortcut] ensure key auth failed, continuing anyway",
				"error", err.Error(), "host", dir.RemoteHost)
		}
	}

	logger.Logger.Infow("[OpenRemoteDirShortcut] opening",
		"termType", termType, "bin", binPath,
		"remotePath", dir.RemotePath, "keyPath", keyPath)

	// 用 buildRemoteArgs 获取完整 args 列表
	args := buildRemoteArgs(termType, dir, keyPath)
	if len(args) == 0 {
		return fmt.Errorf("build remote args failed: empty result")
	}

	return execRemoteTerminal(termType, binPath, args)
}

// execRemoteTerminal 用终端类型的 binPath 执行远程唤起命令。
// remoteArgs[0] 是 ssh（或其他远程可执行文件），通过终端的 RemoteBin 或 "ssh" 调用。
func execRemoteTerminal(termType, binPath string, remoteArgs []string) error {
	cfg := config.Get()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	termDef, ok := cfg.Terminal.Types[strings.ToLower(termType)]

	var bin string
	if ok {
		bin = termDef.Bin
	}
	if bin == "" {
		bin = "ssh"
	}
	if binPath != "" {
		bin = binPath
	}

	// 用终端类型的本地唤起方式包装远程命令
	localArgs := buildLocalArgsForRemote(termType, remoteArgs)

	logger.Logger.Infow("[execRemoteTerminal]",
		"bin", bin, "localArgs", localArgs)

	return exec.Command(bin, localArgs...).Start()
}

// buildLocalArgsForRemote 根据终端类型构建本地唤起参数，把 remoteArgs 作为子命令传入。
// 例如 wezterm: ["start", "--", "ssh", "-i", "...", "user@host"]
// 例如 iterm2: ["-e", "tell app \"iTerm2\" to create window command \"...\""]
func buildLocalArgsForRemote(termType string, remoteArgs []string) []string {
	// 将 remoteArgs 拼成空白分隔的字符串（用于嵌入 osascript/powershell）
	remoteCmdStr := strings.Join(remoteArgs, " ")

	switch strings.ToLower(termType) {
	case "wezterm":
		// wezterm start -- ssh -i ... user@host
		args := []string{"start"}
		args = append(args, remoteArgs...)
		return args
	case "wt":
		// wt.exe new-tab -p 80 -H "user@host" ssh -i ... user@host
		// WT uses -H/--title for window title
		sshTarget := ""
		for _, a := range remoteArgs {
			if strings.Contains(a, "@") {
				sshTarget = a
				break
			}
		}
		args := []string{"new-tab"}
		if sshTarget != "" {
			args = append(args, "-H", sshTarget)
		}
		args = append(args, remoteArgs...)
		return args
	case "iterm2":
		// osascript -e 'tell application "iTerm2" to create window with default profile command "ssh -i ..."'
		return []string{"-e", fmt.Sprintf(`tell application "iTerm2" to create window with default profile command "%s"`, remoteCmdStr)}
	case "terminal":
		// osascript -e 'tell application "Terminal" to do script "ssh -i ..."'
		return []string{"-e", fmt.Sprintf(`tell application "Terminal" to do script "%s"`, remoteCmdStr)}
	case "gnome":
		// gnome-terminal -- ssh -i ... user@host
		args := []string{"--"}
		args = append(args, remoteArgs...)
		return args
	case "xterm":
		// xterm -e ssh -i ... user@host
		args := []string{"-e"}
		args = append(args, remoteArgs...)
		return args
	case "powershell", "pwsh", "pwsh7":
		// powershell.exe -NoExit -Command "ssh -i ... user@host"
		return []string{"-NoExit", "-Command", remoteCmdStr}
	case "cmd":
		// cmd.exe /K ssh -i ... user@host
		args := []string{"/K"}
		args = append(args, remoteArgs...)
		return args
	default:
		// 泛用终端：直接执行 remoteArgs
		return remoteArgs
	}
}

// OpenRemoteTerminal 打开远程终端（简化版：用 ssh 命令，URL 风格）
func OpenRemoteTerminal(termType, target string) error {
	info, err := ParseSSHURL(target)
	if err != nil {
		return err
	}
	args := []string{}
	if info.User != "" {
		args = append(args, info.User+"@"+info.Host)
	} else {
		args = append(args, info.Host)
	}
	if info.Port != "" {
		args = append(args, "-p", info.Port)
	}
	if info.Path != "" {
		args = append(args, "-c", "cd '"+info.Path+"'; exec $SHELL")
	}
	cmd := exec.Command("ssh", args...)
	return cmd.Start()
}

// OpenTerminal 打开终端到指定目录。
// termType: 终端类型 key（wezterm/wt/powershell 等）
// dir: 工作目录
// binPath: 自定义二进制路径（优先使用）
func OpenTerminal(termType, dir, binPath string) error {
	if dir == "" {
		dir = os.Getenv("HOME")
	}
	cfg := config.Get()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	key := strings.ToLower(termType)
	typeDef, ok := cfg.Terminal.Types[key]
	if !ok {
		return fmt.Errorf("unsupported terminal type: %s", termType)
	}
	bin := typeDef.Bin
	if binPath != "" {
		bin = binPath
	}
	logger.Logger.Infow("[OpenTerminal]", "termType", termType, "dir", dir, "bin", bin, "binPath", binPath, "at", "terminal.go")
	// 构建 args，替换 {dir} 占位符
	args := make([]string, len(typeDef.Args))
	for i, a := range typeDef.Args {
		args[i] = strings.ReplaceAll(a, "{dir}", dir)
	}
	logger.Logger.Infow("[OpenTerminal] exec", "bin", bin, "args", args, "at", "terminal.go")
	cmd := buildOpenTerminalCmd(bin, args, dir)
	return cmd.Start()
}

// buildOpenTerminalCmd 构造打开终端的 *exec.Cmd。
// Windows 走 syscall.SysProcAttr.CmdLine 精确控制命令行（terminal_windows.go）；
// 其他平台用 cmd /C start "" 创建独立窗口（terminal_other.go）。
func buildOpenTerminalCmd(bin string, args []string, dir string) *exec.Cmd {
	return openTerminalCmd(bin, args, dir)
}

// ParseSSHURL 解析 SSH URL 或目标字符串
type SSHInfo struct {
	User    string
	Host    string
	Port    string
	Path    string
}

func ParseSSHURL(raw string) (*SSHInfo, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "ssh://") {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid ssh url: %w", err)
		}
		info := &SSHInfo{Host: u.Hostname(), Path: u.Path}
		if u.User != nil {
			info.User = u.User.Username()
		}
		if u.Port() != "" && u.Port() != "22" {
			info.Port = u.Port()
		}
		return info, nil
	}
	at := strings.IndexByte(raw, '@')
	colon := strings.LastIndexByte(raw, ':')
	slash := strings.IndexByte(raw, '/')
	if at == -1 {
		if slash > 0 {
			return &SSHInfo{Host: raw[:slash], Path: raw[slash:]}, nil
		}
		return &SSHInfo{Host: raw}, nil
	}
	info := &SSHInfo{User: raw[:at], Host: raw[at+1:]}
	if colon > at {
		afterColon := raw[colon+1:]
		if strings.HasPrefix(afterColon, "/") {
			info.Host = raw[at+1 : colon]
			info.Path = afterColon
		} else if slash > colon {
			info.Host = raw[at+1 : colon]
			info.Port = strings.TrimSpace(afterColon[:slash-colon-1])
			info.Path = raw[slash:]
		} else if isAllDigits(strings.TrimSpace(afterColon)) {
			info.Host = raw[at+1 : colon]
			info.Port = strings.TrimSpace(afterColon)
		}
	} else if slash > at {
		info.Path = raw[slash:]
	}
	return info, nil
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}