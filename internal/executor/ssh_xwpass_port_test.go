package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
)

// platformSshpassName 复刻 ResolveXwSshpassBin 的命名规则
func platformSshpassName() string {
	osMap := map[string]string{"darwin": "darwin", "linux": "linux", "windows": "windows"}
	osStr := osMap[runtime.GOOS]
	if osStr == "" {
		return "xw-sshpass"
	}
	archStr := "amd64"
	if runtime.GOARCH == "arm64" {
		archStr = "arm64"
	}
	name := fmt.Sprintf("xw-sshpass-%s-%s", osStr, archStr)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// TestRunViaXwSshpass_PortFlag 验证当 RemotePort 非默认时，xw-sshpass 收到 -P <port> flag
// （而不是把 :port 拼到 user@host 字符串里）。
//
// 用 fake xw-sshpass 替换真实二进制，捕获实际参数。
func TestRunViaXwSshpass_PortFlag(t *testing.T) {
	// 1. 准备 fake xw-sshpass，捕获参数到文件
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	toolsDir := filepath.Join(cwd, "tools", "xw-sshpass")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	// ResolveXwSshpassBin 在 tools/xw-sshpass/ 下找 "xw-sshpass-<os>-<arch>" 文件
	binName := "xw-sshpass"
	if v := os.Getenv("FAKE_XW_SSHPASS_NAME"); v != "" {
		binName = v
	} else {
		binName = platformSshpassName()
	}
	fakeBin := filepath.Join(toolsDir, binName)
	tmpCapture := filepath.Join(t.TempDir(), "captured_args.txt")
	// 注意：测试运行后恢复原文件
	origBinBytes, origErr := os.ReadFile(fakeBin)
	origExisted := origErr == nil
	t.Cleanup(func() {
		if origExisted {
			_ = os.WriteFile(fakeBin, origBinBytes, 0755)
		} else {
			_ = os.Remove(fakeBin)
		}
	})
	fakeScript := `#!/bin/bash
echo "$@" > ` + tmpCapture + `
exit 0
`
	if err := os.WriteFile(fakeBin, []byte(fakeScript), 0755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}

	tests := []struct {
		name           string
		port           string
		wantPortArg    string // 期望在参数列表中出现的字面 "-P <port>"
		wantHostArgHasColon bool // user@host 是否含 ":port"（修复后应为 false）
	}{
		{
			name:           "default_port_22_no_-P_flag",
			port:           "22",
			wantPortArg:    "",
			wantHostArgHasColon: false,
		},
		{
			name:           "empty_port_no_-P_flag",
			port:           "",
			wantPortArg:    "",
			wantHostArgHasColon: false,
		},
		{
			name:           "custom_port_2222_explicit_-P",
			port:           "2222",
			wantPortArg:    "-P 2222",
			wantHostArgHasColon: false,
		},
		{
			name:           "custom_port_5022_explicit_-P",
			port:           "5022",
			wantPortArg:    "-P 5022",
			wantHostArgHasColon: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Remove(tmpCapture) // 清空上次捕获

			ds := &backend.DirShortcut{
				Name:           "test-dir",
				Type:           backend.DirShortcutTypeRemote,
				RemoteHost:     "10.0.0.1",
				RemoteUser:     "root",
				RemotePort:     tt.port,
				AuthMethod:     "password",
				RemotePassword: "secret",
			}
			ctx := context.Background()
			// 执行 echo 命令，期望从 fake 拿到参数捕获
			_, err := RunViaXwSshpass(ctx, ds, []string{"sh", "-c", "echo hi"}, "", nil)
			if err != nil {
				t.Fatalf("RunViaXwSshpass err: %v", err)
			}

			captured, err := os.ReadFile(tmpCapture)
			if err != nil {
				t.Fatalf("read capture: %v", err)
			}
			capturedStr := strings.TrimSpace(string(captured))
			t.Logf("captured args: %s", capturedStr)

			if tt.wantPortArg != "" {
				if !strings.Contains(capturedStr, tt.wantPortArg) {
					t.Errorf("expected args to contain %q, got %q", tt.wantPortArg, capturedStr)
				}
			}

			// 校验 user@host 不再含 ":port"（避免 ParseSSHArgs 把 host:port 当 host）
			// capturedStr 形如 "-p secret -P 2222 ssh root@10.0.0.1 mkdir -p ..."
			// 我们关心 "ssh root@10.0.0.1" 后面是否含 ":port"
			// 简单做法：查找 "@10.0.0.1:" 是否存在
			hostMarker := "@" + ds.RemoteHost + ":"
			if strings.Contains(capturedStr, hostMarker) != tt.wantHostArgHasColon {
				t.Errorf("user@host 含 :port 标志符 %q 出现情况不匹配, captured=%q",
					hostMarker, capturedStr)
			}
		})
	}
}