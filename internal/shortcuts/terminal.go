package shortcuts

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// buildRemoteArgs 已迁移到 internal/executor/ssh_command_builder.go 的 BuildSSHCommand。

// resolveXwSshpassBin 返回当前平台对应的 xw-sshpass 路径。
// 优先从 tools/xw-sshpass/ 目录查找，找不到则尝试 PATH。
func resolveXwSshpassBin() string {
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

	// 1. 从 tools/xw-sshpass/ 目录查找（cwd 为项目根）
	toolsDir := filepath.Join(getProjectRoot(), "tools", "xw-sshpass")
	binPath := filepath.Join(toolsDir, binName)
	if _, err := os.Stat(binPath); err == nil {
		return binPath
	}

	// 2. fallback 到 PATH
	if bin, err := exec.LookPath(binName); err == nil {
		return bin
	}
	// 3. 尝试不带平台后缀的 xw-sshpass
	if bin, err := exec.LookPath("xw-sshpass"); err == nil {
		return bin
	}

	return ""
}

// getProjectRoot 返回项目根目录（xworkbench 二进制所在目录，即 cwd）。
// xworkbench 通过 scripts/run.sh 启动时，cwd 为项目根目录。
func getProjectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// OpenRemoteDirShortcut 用配置的终端软件打开远程 SSH 连接。
// 支持 wezterm 等终端，连接后 cd 到 remote_path（默认主目录）。
// 统一使用 xw-sshpass 处理认证（密码或密钥）。
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
		"remotePath", dir.RemotePath, "authMethod", dir.AuthMethod)

	// 统一使用 xw-sshpass 处理远程连接（支持密码和密钥认证）
	xwBin := resolveXwSshpassBin()
	if xwBin == "" {
		return fmt.Errorf("xw-sshpass not found, please build it first")
	}

	// 构建 ssh 命令基本参数
	sshTarget := dir.RemoteUser
	if sshTarget == "" {
		sshTarget = "root"
	}
	if dir.RemoteHost != "" {
		sshTarget = sshTarget + "@" + dir.RemoteHost
	}

	// 构建 xw-sshpass 参数
	newArgs := []string{xwBin}

	// 密码认证
	if dir.AuthMethod == "password" && dir.RemotePassword != "" {
		newArgs = append(newArgs, "-p", dir.RemotePassword)
	}

	// 密钥认证
	if dir.AuthMethod == "key" {
		keyPath := executor.ResolveKeyPath(dir)
		if keyPath != "" {
			newArgs = append(newArgs, "-i", keyPath)
		}
	}

	// 远程工作目录
	if dir.RemotePath != "" {
		newArgs = append(newArgs, "-w", dir.RemotePath)
	}

	// 添加 ssh 命令和目标
	newArgs = append(newArgs, "ssh", sshTarget)

	logger.Logger.Infow("[OpenRemoteDirShortcut] using xw-sshpass",
		"xwBin", xwBin, "args", newArgs)

	return execRemoteTerminal(termType, binPath, newArgs)
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