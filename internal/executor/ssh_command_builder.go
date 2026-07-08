package executor

import (
	"fmt"
	"os"
	"strings"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
)

// BuildSSHCommand 根据 DirShortcut + 终端类型构建 ssh 唤起的完整参数列表。
// 返回的 []string 形如 [ssh, -o, Kex=..., ..., root@host, -t, --, sh, -c, '...']。
// 调用方负责将其传递给终端程序（wezterm / iTerm2 / Windows Terminal ...）。
//
// 关键不变量：
//   - 返回 []string 永远以 ssh binary 起头，不重复
//   - -i 永远紧跟一个存在的文件路径（或完全不存在）
//   - compat_algorithms 全空时不传任何 -o
func BuildSSHCommand(dir *backend.DirShortcut, termType string) ([]string, error) {
	cfg := config.Get()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	termDef, ok := cfg.Terminal.Types[strings.ToLower(termType)]
	if !ok {
		return nil, fmt.Errorf("unsupported terminal type: %s", termType)
	}

	binary, template, err := resolveSSHBinary(termDef)
	if err != nil {
		return nil, err
	}

	keyPath := ResolveKeyPath(dir)

	args := []string{binary}
	args = append(args, buildCompatArgs(cfg.SSH.CompatAlgorithms)...)
	args = append(args, template...)

	// 条件移除 -i {key_path} 段
	args = dropKeyFlagIfMissing(args, keyPath)

	// 变量替换
	shellCmd := buildShellCmd(dir)
	sshTarget := dir.RemoteHost
	if dir.RemoteUser != "" {
		sshTarget = dir.RemoteUser + "@" + dir.RemoteHost
	}
	result := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.ReplaceAll(arg, "{key_path}", shellQuote(keyPath))
		arg = strings.ReplaceAll(arg, "{user}@{host}", sshTarget)
		arg = strings.ReplaceAll(arg, "{host}", dir.RemoteHost)
		arg = strings.ReplaceAll(arg, "{user}", dir.RemoteUser)
		arg = strings.ReplaceAll(arg, "{shell_cmd}", shellQuote(shellCmd))
		result = append(result, arg)
	}
	return result, nil
}

// resolveSSHBinary 根据 termDef 决定 ssh binary 和参数模板。
// 规则：
//   - 默认 binary 为 "ssh"
//   - 若 RemoteBin 非空，用 RemoteBin
//   - 若 RemoteArgs 非空且 [0] != "ssh"，则用 [0] 覆盖 binary
//   - 若 RemoteArgs 非空且 [0] == "ssh"，则跳过 [0]（去重）
func resolveSSHBinary(termDef config.TerminalTypeDef) (binary string, template []string, err error) {
	binary = "ssh"
	if termDef.RemoteBin != "" {
		binary = termDef.RemoteBin
	}

	if len(termDef.RemoteArgs) == 0 {
		// 兜底：泛用 ssh 命令
		template = []string{"{user}@{host}", "-t", "--", "sh", "-c", "{shell_cmd}"}
		return
	}

	tpl := termDef.RemoteArgs
	if tpl[0] != "ssh" {
		// 用户自定义 binary 路径
		binary = tpl[0]
		template = append([]string{}, tpl[1:]...)
	} else {
		// 去重：跳过首元素 "ssh"
		template = append([]string{}, tpl[1:]...)
	}
	return
}

// buildCompatArgs 根据 CompatAlgorithms 拼出 -o 选项。
// 任一字段为空数组则不输出对应 -o。
func buildCompatArgs(algos config.SSHCompatAlgorithms) []string {
	var args []string
	if len(algos.Kex) > 0 {
		args = append(args, "-o", "KexAlgorithms="+strings.Join(algos.Kex, ","))
	}
	if len(algos.HostKey) > 0 {
		args = append(args, "-o", "HostKeyAlgorithms="+strings.Join(algos.HostKey, ","))
	}
	if len(algos.Cipher) > 0 {
		args = append(args, "-o", "Ciphers="+strings.Join(algos.Cipher, ","))
	}
	return args
}

// dropKeyFlagIfMissing 若 keyPath 为空或文件不存在，从 args 中移除
// 紧随 "-i" 之后的占位符 "{key_path}"（共两个元素）。
func dropKeyFlagIfMissing(args []string, keyPath string) []string {
	if keyPath != "" && sshKeyFileExists(keyPath) {
		return args
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "-i" && i+1 < len(args) && args[i+1] == "{key_path}" {
			i++ // 跳过 "{key_path}"
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// sshKeyFileExists 检查文件是否存在（测试可通过替换此闭包覆盖）。
var sshKeyFileExists = func(path string) bool {
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

// shellQuote 给字符串加单引号并转义内部单引号。
func shellQuote(s string) string {
	if s == "" {
		return ""
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}