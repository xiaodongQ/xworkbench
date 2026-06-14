package shortcuts

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/xiaodongQ/xworkbench/internal/config"
)

// IsSupportedTerminal 检查终端类型是否支持
func IsSupportedTerminal(termType string) bool {
	cfg := config.AppConfig
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	_, ok := cfg.Terminal.Types[strings.ToLower(termType)]
	return ok
}

// DetectTerminalPath 检测终端类型的可执行文件路径，优先从 PATH 找，找不到时探测配置中的路径。
func DetectTerminalPath(termType string) string {
	cfg := config.AppConfig
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

// DefaultTerminal 返回配置的默认终端类型
func DefaultTerminal() string {
	cfg := config.AppConfig
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return cfg.Terminal.DefaultType
}

// OpenRemoteTerminal 打开远程终端（简化版：用 ssh 命令）
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
	cfg := config.AppConfig
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
	slog.Info("[OpenTerminal]",
		slog.String("termType", termType),
		slog.String("dir", dir),
		slog.String("bin", bin),
		slog.String("binPath", binPath),
		slog.String("at", "terminal.go:93"))
	// 构建 args，替换 {dir} 占位符
	args := make([]string, len(typeDef.Args))
	for i, a := range typeDef.Args {
		args[i] = strings.ReplaceAll(a, "{dir}", dir)
	}
	slog.Info("[OpenTerminal] exec",
		slog.String("bin", bin),
		slog.Any("args", args),
		slog.String("at", "terminal.go:113"))
	cmd := exec.Command(bin, args...)
	return cmd.Start()
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
