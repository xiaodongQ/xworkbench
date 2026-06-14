package shortcuts

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// TerminalType 定义支持的终端类型
type TerminalType string

const (
	TerminalWezterm    TerminalType = "wezterm"
	TerminalWindows    TerminalType = "wt"         // Windows Terminal
	TerminalPowerShell TerminalType = "powershell" // Windows PowerShell / PowerShell 7+
	TerminalSystemPS   TerminalType = "pwsh"       // PowerShell Core
	TerminalMacOS      TerminalType = "terminal"   // macOS Terminal.app
	TerminalGnome      TerminalType = "gnome"     // GNOME Terminal
	TerminalXTerm      TerminalType = "xterm"      // xterm
	TerminalCmd        TerminalType = "cmd"        // Windows CMD
)

// DefaultTerminal 返回当前平台推荐的默认终端类型
func DefaultTerminal() TerminalType {
	switch runtime.GOOS {
	case "darwin":
		return TerminalWezterm
	case "windows":
		return TerminalWindows
	default:
		return TerminalWezterm
	}
}

// IsSupportedTerminal 检查给定名称是否支持
func IsSupportedTerminal(name string) bool {
	_, ok := ParseTerminalType(name)
	return ok
}

// ParseTerminalType 将字符串解析为 TerminalType，不区分大小写
func ParseTerminalType(name string) (TerminalType, bool) {
	switch strings.ToLower(name) {
	case "wezterm":
		return TerminalWezterm, true
	case "wt", "windows", "windowsterminal":
		return TerminalWindows, true
	case "powershell", "ps":
		return TerminalPowerShell, true
	case "pwsh", "powershell7", "ps7":
		return TerminalSystemPS, true
	case "terminal", "terminal.app", "macos":
		return TerminalMacOS, true
	case "gnome", "gnome-terminal":
		return TerminalGnome, true
	case "xterm":
		return TerminalXTerm, true
	case "cmd", "commandprompt":
		return TerminalCmd, true
	default:
		return "", false
	}
}

// OpenTerminal 在指定目录打开配置的终端。
// termType 为空时使用平台默认终端。
// dir 为空时使用用户主目录。
func OpenTerminal(termType, dir string) error {
	if dir == "" {
		dir = os.Getenv("HOME")
	}
	t, ok := ParseTerminalType(termType)
	if !ok {
		t = DefaultTerminal()
	}
	return openTerminal(t, dir)
}

// openTerminal 是实际执行入口，按类型分发
func openTerminal(t TerminalType, dir string) error {
	switch t {
	case TerminalWezterm:
		return openWezterm(dir)
	case TerminalWindows:
		return openWindowsTerminal(dir)
	case TerminalPowerShell, TerminalSystemPS:
		return openPowerShell(t, dir)
	case TerminalMacOS:
		return openMacOSTerminal(dir)
	case TerminalGnome:
		return openGnomeTerminal(dir)
	case TerminalXTerm:
		return openXTerm(dir)
	case TerminalCmd:
		return openCmd(dir)
	default:
		return fmt.Errorf("unsupported terminal type: %s", t)
	}
}

// openWezterm 打开 wezterm（跨平台）
// wezterm start --cwd <dir>
func openWezterm(dir string) error {
	cmd := exec.Command("wezterm", "start", "--cwd", dir)
	return cmd.Start()
}

// openWindowsTerminal 打开 Windows Terminal (wt.exe)
// wt.exe --starting-directory <dir>
func openWindowsTerminal(dir string) error {
	// wt.exe 默认在用户主目录，直接 Start 即可打开新窗口
	// 指定目录需要 --starting-directory，但新版本才支持
	cmd := exec.Command("wt.exe")
	if dir != "" {
		// Windows 上用 PowerShell /cd 方式中转
		cmd = exec.Command("wt.exe", "-d", dir)
	}
	return cmd.Start()
}

// openPowerShell 打开 PowerShell 并 cd 到目录
func openPowerShell(t TerminalType, dir string) error {
	exe := "powershell.exe"
	if t == TerminalSystemPS {
		exe = "pwsh.exe"
	}
	// -NoExit: 打开后不自动退出，方便看输出
	// -c "cd <dir>": 执行 cd 命令
	cmd := exec.Command(exe, "-NoExit", "-c", fmt.Sprintf("cd '%s'", dir))
	return cmd.Start()
}

// openMacOSTerminal 打开 macOS Terminal.app 并执行 cd
// Terminal.app 不支持 --cwd 参数，用 osascript 脚本注入
func openMacOSTerminal(dir string) error {
	script := fmt.Sprintf(`
tell application "Terminal"
	activate
	do script "cd %s" in front window
end tell
`, shellEscape(dir))
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Start()
}

// openGnomeTerminal 打开 GNOME Terminal 并 cd 到目录
func openGnomeTerminal(dir string) error {
	cmd := exec.Command("gnome-terminal", "--", "--working-directory="+dir)
	return cmd.Start()
}

// openXTerm 打开 xterm 并 cd 到目录
func openXTerm(dir string) error {
	cmd := exec.Command("xterm", "-e", "bash", "-c", fmt.Sprintf("cd '%s'; exec bash", shellEscape(dir)))
	return cmd.Start()
}

// openCmd 打开 Windows CMD 并 cd 到目录
func openCmd(dir string) error {
	cmd := exec.Command("cmd.exe", "/K", fmt.Sprintf("cd /d %s", dir))
	return cmd.Start()
}

// shellEscape 对字符串做基本转义（用于 osascript 字符串嵌入）
func shellEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

// SSH URL 格式解析
// 支持: ssh://user@host/path, user@host:/path, user@host
type SSHInfo struct {
	User    string
	Host    string
	Port    string
	Path    string
	TermType TerminalType // 远程终端类型
}

// ParseSSHURL 解析 SSH URL 或目标字符串
// 支持格式：
//   ssh://user@host:22/path
//   user@host:/path
//   user@host
//   user@host:2222
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
	// user@host:/path 或 user@host 格式
	at := strings.IndexByte(raw, '@')
	colon := strings.LastIndexByte(raw, ':')
	slash := strings.IndexByte(raw, '/')
	if at == -1 {
		return &SSHInfo{Host: raw}, nil
	}
	info := &SSHInfo{
		User: raw[:at],
		Host: raw[at+1:],
	}
	// user@host:2222 或 user@host:/path
	if colon > at {
		portOrPath := raw[colon+1:]
		if slash > colon {
			info.Port = portOrPath[:slash-colon-1]
			info.Path = raw[slash:]
		} else {
			info.Port = portOrPath
		}
	} else if slash > at {
		info.Path = raw[slash:]
	}
	return info, nil
}

// OpenRemoteTerminal 打开远程终端连接到 SSH 目标
// 支持 wezterm ssh、macOS Terminal ssh、Windows Terminal ssh 等
func OpenRemoteTerminal(termType, sshTarget string) error {
	if sshTarget == "" {
		return fmt.Errorf("ssh target is required")
	}
	info, err := ParseSSHURL(sshTarget)
	if err != nil {
		return err
	}

	t, ok := ParseTerminalType(termType)
	if !ok {
		t = TerminalWezterm // 远程默认用 wezterm（最广泛支持）
	}

	return openRemoteTerminal(t, info)
}

// openRemoteTerminal 用指定终端类型打开远程连接
func openRemoteTerminal(t TerminalType, info *SSHInfo) error {
	switch t {
	case TerminalWezterm:
		return openRemoteWezterm(info)
	case TerminalMacOS:
		return openRemoteMacOSTerminal(info)
	case TerminalWindows:
		return openRemoteWindowsTerminal(info)
	default:
		// 通用: 用系统默认 ssh 命令
		return openRemoteGenericSSH(info)
	}
}

// buildSSHArgs 构建 ssh 命令参数
func buildSSHArgs(info *SSHInfo) []string {
	var args []string
	if info.User != "" {
		args = append(args, info.User+"@"+info.Host)
	} else {
		args = append(args, info.Host)
	}
	if info.Port != "" {
		args = append(args, "-p", info.Port)
	}
	if info.Path != "" {
		args = append(args, "cd '"+info.Path+"'; exec $SHELL")
	}
	return args
}

// openRemoteWezterm 用 wezterm connect 打开远程终端
func openRemoteWezterm(info *SSHInfo) error {
	args := []string{"connect"}
	if info.User != "" {
		args = append(args, info.User+"@"+info.Host)
	} else {
		args = append(args, info.Host)
	}
	if info.Port != "" {
		args = append(args, "--ssh-param", "-p", info.Port)
	}
	cmd := exec.Command("wezterm", args...)
	return cmd.Start()
}

// openRemoteMacOSTerminal 用 Terminal.app 执行 ssh 命令
func openRemoteMacOSTerminal(info *SSHInfo) error {
	sshCmd := "ssh " + strings.Join(buildSSHArgs(info), " ")
	script := fmt.Sprintf(`
tell application "Terminal"
	activate
	do script "%s" in front window
end tell
`, sshCmd)
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Start()
}

// openRemoteWindowsTerminal 用 Windows Terminal 执行 ssh
func openRemoteWindowsTerminal(info *SSHInfo) error {
	sshCmd := "ssh " + strings.Join(buildSSHArgs(info), " ")
	cmd := exec.Command("wt.exe", "new-tab", sshCmd)
	return cmd.Start()
}

// openRemoteGenericSSH 用系统默认 ssh 命令打开远程终端
func openRemoteGenericSSH(info *SSHInfo) error {
	cmd := exec.Command("ssh", buildSSHArgs(info)...)
	return cmd.Start()
}