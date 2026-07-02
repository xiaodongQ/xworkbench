package shortcuts

import (
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
)

// TestBuildRemoteArgs 测试模板变量替换逻辑。
// 覆盖场景：user+key / host-only+key / with remote_path / shell_cmd cd
func TestBuildRemoteArgs(t *testing.T) {
	restore := config.TestSnapshotAndRestore()
	defer restore()

	tests := []struct {
		name        string
		termType    string
		dir         *backend.DirShortcut
		keyPath     string
		wantContain []string // 结果 args 中必须包含的片段（顺序无关）
		wantExclude []string // 结果 args 中必须不包含的片段
	}{
		{
			name:     "wezterm with user and key",
			termType: "wezterm",
			dir: &backend.DirShortcut{
				RemoteUser:  "ubuntu",
				RemoteHost:  "192.168.1.100",
				RemotePath:  "/home/ubuntu/projects",
				AuthMethod:  "key",
			},
			keyPath:     "/home/user/.ssh/id_ed25519",
			wantContain: []string{"ssh", "-i", "/home/user/.ssh/id_ed25519", "ubuntu@192.168.1.100"},
			wantExclude: []string{"-p"},
		},
		{
			name:     "wezterm host-only no user",
			termType: "wezterm",
			dir: &backend.DirShortcut{
				RemoteUser:  "",
				RemoteHost:  "192.168.1.100",
				RemotePath:  "",
				AuthMethod:  "key",
			},
			keyPath:     "/home/user/.ssh/id_ed25519",
			wantContain: []string{"ssh", "-i", "/home/user/.ssh/id_ed25519", "192.168.1.100"},
			wantExclude: []string{"-p", "@"},
		},
		{
			name:     "wt Windows Terminal with user and key",
			termType: "wt",
			dir: &backend.DirShortcut{
				RemoteUser:  "admin",
				RemoteHost:  "192.168.1.50",
				AuthMethod:  "key",
			},
			keyPath:     "/home/user/.ssh/id_ed25519",
			wantContain: []string{"ssh", "-i", "/home/user/.ssh/id_ed25519", "admin@192.168.1.50"},
		},
		{
			name:     "iterm2 with user and key",
			termType: "iterm2",
			dir: &backend.DirShortcut{
				RemoteUser:  "developer",
				RemoteHost:  "dev.example.com",
				AuthMethod:  "key",
			},
			keyPath:     "/home/user/.ssh/id_ed25519",
			wantContain: []string{"ssh", "-i", "/home/user/.ssh/id_ed25519", "developer@dev.example.com"},
		},
		{
			name:     "gnome-terminal with remote_path",
			termType: "gnome",
			dir: &backend.DirShortcut{
				RemoteUser:  "ubuntu",
				RemoteHost:  "192.168.1.100",
				RemotePath:  "/var/log",
				AuthMethod:  "key",
			},
			keyPath:     "/home/user/.ssh/id_ed25519",
			wantContain: []string{"ssh", "-i", "/home/user/.ssh/id_ed25519", "ubuntu@192.168.1.100"},
			wantExclude: []string{},
		},
		{
			name:     "unsupported terminal type falls back to generic ssh",
			termType: "nonexistent_terminal_xyz",
			dir: &backend.DirShortcut{
				RemoteUser:  "root",
				RemoteHost:  "10.0.0.1",
				AuthMethod:  "key",
			},
			keyPath:     "/home/user/.ssh/id_ed25519",
			wantContain: []string{"ssh", "-i", "/home/user/.ssh/id_ed25519", "root@10.0.0.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildRemoteArgsForTest(tt.termType, tt.dir, tt.keyPath)
			argsStr := joinArgs(args)
			for _, want := range tt.wantContain {
				if !contains(argsStr, want) {
					t.Errorf("buildRemoteArgs(%q) = %v, want contain %q", tt.name, args, want)
				}
			}
			for _, dont := range tt.wantExclude {
				if contains(argsStr, dont) {
					t.Errorf("buildRemoteArgs(%q) = %v, want exclude %q", tt.name, args, dont)
				}
			}
		})
	}
}

// TestBuildRemoteArgs_ShellCmd 测试 shell_cmd 中包含 cd 到 remote_path 和 TerminalCmd。
// shell_cmd 整体被 shellQuote 包裹，引号会转义。
func TestBuildRemoteArgs_ShellCmd(t *testing.T) {
	restore := config.TestSnapshotAndRestore()
	defer restore()

	dir := &backend.DirShortcut{
		RemoteUser:  "ubuntu",
		RemoteHost:  "192.168.1.100",
		RemotePath:  "/home/ubuntu/projects",
		TerminalCmd: "tmux new-session -d -s work",
		AuthMethod:  "key",
	}
	args := buildRemoteArgsForTest("wezterm", dir, "/home/user/.ssh/id_ed25519")
	argsStr := joinArgs(args)

	// shell_cmd 整体被 shellQuote 包裹，单引号会转义为 '\'''
	// 检查 cd 和路径关键字存在（引号转义后形式多样，不做精确匹配）
	if !contains(argsStr, "cd") || !contains(argsStr, "/home/ubuntu/projects") {
		t.Errorf("shell_cmd should contain cd to remote_path, got: %v", argsStr)
	}
	// TerminalCmd 出现在最终命令中
	if !contains(argsStr, "tmux new-session -d -s work") {
		t.Errorf("shell_cmd should contain TerminalCmd, got: %v", argsStr)
	}
}

// TestBuildRemoteArgs_EmptyKeyPath 空 keyPath 时不应 panic。
func TestBuildRemoteArgs_EmptyKeyPath(t *testing.T) {
	restore := config.TestSnapshotAndRestore()
	defer restore()

	dir := &backend.DirShortcut{
		RemoteUser:  "root",
		RemoteHost:  "10.0.0.1",
		AuthMethod:  "key",
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("buildRemoteArgs with empty keyPath panicked: %v", r)
		}
	}()
	args := buildRemoteArgsForTest("wezterm", dir, "")
	_ = args // 空 keyPath 时行为未定义，但不应 panic
}

// joinArgs 拼接 args 便于 contains 检查。
func joinArgs(args []string) string {
	result := ""
	for _, a := range args {
		result += " " + a
	}
	return result
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// buildRemoteArgsForTest 是 buildRemoteArgs 的测试包装，暴露为 package-internal 函数。
func buildRemoteArgsForTest(termType string, dir *backend.DirShortcut, keyPath string) []string {
	return buildRemoteArgs(termType, dir, keyPath)
}