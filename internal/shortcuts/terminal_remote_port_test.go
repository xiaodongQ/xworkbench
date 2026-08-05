package shortcuts

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
)

// TestOpenRemoteDirShortcut_PortFlag 验证外部终端唤起路径在 RemotePort 非默认时，
// xw-sshpass 收到 -P <port> flag（而非把 :port 塞进 user@host）。
//
// 用 fake 替换 xw-sshpass 和终端二进制，捕获实际 exec 参数。
func TestOpenRemoteDirShortcut_PortFlag(t *testing.T) {
	// 注意：t.TempDir() 每次调用返回新目录——用 os.MkdirTemp 手动管理
	rootTmp, err := os.MkdirTemp("", "xw-sshpass-test-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(rootTmp) })

	// 1. fake xw-sshpass：什么都不做（终端会记录到 xw-sshpass 的全部参数）
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	toolsDir := filepath.Join(cwd, "tools", "xw-sshpass")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	fakeSshpass := filepath.Join(toolsDir, platformSshpassName())
	sshpassScript := "#!/bin/bash\nexit 0\n"
	if err := os.WriteFile(fakeSshpass, []byte(sshpassScript), 0755); err != nil {
		t.Fatalf("write fake sshpass: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(fakeSshpass) })

	// 2. fake 终端：把参数原样写入 capture 文件
	tmpCapture := filepath.Join(rootTmp, "captured_args.txt")
	fakeTerm := filepath.Join(rootTmp, "fake-terminal")
	terminalScript := "#!/bin/bash\nprintf '%s\\n' \"$@\" > " + tmpCapture + "\nexit 0\n"
	if err := os.WriteFile(fakeTerm, []byte(terminalScript), 0755); err != nil {
		t.Fatalf("write fake term: %v", err)
	}

	// 3. 注入 config
	restoreGlobal := config.TestSnapshotAndRestore()
	t.Cleanup(restoreGlobal)
	cfg := config.DefaultConfig()
	cfg.DefaultTerminal = "fake"
	cfg.Terminal.Types["fake"] = config.TerminalTypeDef{
		Bin:  fakeTerm,
		Args: []string{"{dir}"},
		Path: fakeTerm,
	}
	config.Set(cfg)

	tests := []struct {
		name        string
		port        string
		wantPortArg string
	}{
		{name: "default_22", port: "22", wantPortArg: ""},
		{name: "empty", port: "", wantPortArg: ""},
		{name: "custom_2222", port: "2222", wantPortArg: "-P 2222"},
		{name: "custom_8888", port: "8888", wantPortArg: "-P 8888"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Remove(tmpCapture)

			dir := &backend.DirShortcut{
				Name:           "test-dir",
				Type:           backend.DirShortcutTypeRemote,
				RemoteHost:     "10.0.0.1",
				RemoteUser:     "root",
				RemotePort:     tt.port,
				AuthMethod:     "password",
				RemotePassword: "secret",
			}
			if err := OpenRemoteDirShortcut(dir, "fake", fakeTerm); err != nil {
				t.Fatalf("OpenRemoteDirShortcut err: %v", err)
			}

			// execRemoteTerminal 用 Start() 不等子进程，需要轮询等 fake-terminal 写完 capture
			var captured []byte
			var err error
			for i := 0; i < 50; i++ {
				captured, err = os.ReadFile(tmpCapture)
				if err == nil && len(captured) > 0 {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if err != nil {
				t.Fatalf("read capture: %v", err)
			}
			capturedStr := strings.TrimSpace(string(captured))
			t.Logf("terminal 收到参数:\n%s", capturedStr)

			if tt.wantPortArg != "" {
				// args 是每行一个，需要检查两行："-P" 和 "<port>"
				wantParts := strings.Fields(tt.wantPortArg)
				if wantParts[0] == "-P" {
					if !strings.Contains(capturedStr, "\n"+wantParts[0]+"\n") {
						t.Errorf("期望包含 -P 行，实际: %s", capturedStr)
					}
					if !strings.Contains(capturedStr, "\n"+wantParts[1]+"\n") {
						t.Errorf("期望包含 %q 行，实际: %s", wantParts[1], capturedStr)
					}
				} else if !strings.Contains(capturedStr, tt.wantPortArg) {
					t.Errorf("期望包含 %q，实际: %s", tt.wantPortArg, capturedStr)
				}
			} else {
				if strings.Contains(capturedStr, "\n-P\n") {
					t.Errorf("默认端口不应出现 -P 行，实际: %s", capturedStr)
				}
			}
			hostMarker := "@10.0.0.1:"
			if strings.Contains(capturedStr, hostMarker) {
				t.Errorf("user@host 不应再含 :port 后缀，实际: %s", capturedStr)
			}
		})
	}
}

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