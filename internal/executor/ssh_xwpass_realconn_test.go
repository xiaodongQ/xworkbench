package executor

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
)

// TestRunViaXwSshpass_RealConnectionToPort 端到端验证：port="2223" 时实际连到 2223 端口，
// 而非默认 22。
func TestRunViaXwSshpass_RealConnectionToPort(t *testing.T) {
	ln22, err := net.Listen("tcp", "127.0.0.1:2222")
	if err != nil {
		t.Skipf("port 2222 不可用: %v", err)
	}
	defer ln22.Close()
	lnCustom, err := net.Listen("tcp", "127.0.0.1:2223")
	if err != nil {
		t.Skipf("port 2223 不可用: %v", err)
	}
	defer lnCustom.Close()

	var hit22, hitCustom int32
	go func() {
		for {
			conn, err := ln22.Accept()
			if err != nil {
				return
			}
			hit22++
			conn.Close()
		}
	}()
	go func() {
		for {
			conn, err := lnCustom.Accept()
			if err != nil {
				return
			}
			hitCustom++
			conn.Close()
		}
	}()

	tmpBinDir := t.TempDir()
	fakeBin := filepath.Join(tmpBinDir, "real-xw-sshpass")
	script := `#!/bin/bash
PORT="22"
while [[ $# -gt 0 ]]; do
  case "$1" in
    -P) PORT="$2"; shift 2 ;;
    -p|-i|-w) shift 2 ;;
    ssh) shift; break ;;
    *) shift ;;
  esac
done
USER_HOST="$1"
timeout 5 ssh -p "$PORT" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
  -o ConnectTimeout=2 -o PreferredAuthentications=none -o BatchMode=yes \
  "${USER_HOST%@*}@${USER_HOST#*@}" </dev/null 2>/dev/null
exit 0
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatalf("write fake: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	toolsDir := filepath.Join(cwd, "tools", "xw-sshpass")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	realBin := filepath.Join(toolsDir, platformSshpassName())
	t.Cleanup(func() { _ = os.Remove(realBin) })
	data, _ := os.ReadFile(fakeBin)
	if err := os.WriteFile(realBin, data, 0755); err != nil {
		t.Fatalf("write real bin: %v", err)
	}

	ds := &backend.DirShortcut{
		Name:           "real-test",
		Type:           backend.DirShortcutTypeRemote,
		RemoteHost:     "127.0.0.1",
		RemoteUser:     "anyuser",
		RemotePort:     "2223",
		AuthMethod:     "password",
		RemotePassword: "ignored",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, _ = RunViaXwSshpass(ctx, ds, []string{"echo", "hi"}, "", nil)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if hitCustom > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Logf("port=2223 -> hit 2222=%d hit 2223=%d", hit22, hitCustom)
	if hitCustom == 0 {
		t.Errorf("期望 2223 端口收到连接，实际 0 次 — 端口没生效")
	}
	if hit22 > 0 {
		t.Errorf("不希望 2222 端口收到连接，但收到 %d 次", hit22)
	}
}

// TestRunViaXwSshpass_DefaultPort_NoExplicitFlag 反向验证：RemotePort="22" 时
// 不传 -P flag（避免冗余），且仍连到 22 端口。
func TestRunViaXwSshpass_DefaultPort_NoExplicitFlag(t *testing.T) {
	// 监听 22 端口几乎不可能（特权端口+可能没 sshd），
	// 我们改用：抓 fake xw-sshpass 收到的命令行，验证没有 "-P" flag 出现。
	tmpBinDir := t.TempDir()
	captureFile := filepath.Join(tmpBinDir, "captured.txt")
	fakeBin := filepath.Join(tmpBinDir, "xw-sshpass")
	script := `#!/bin/bash
echo "$@" > ` + captureFile + `
exit 0
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatalf("write fake: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	toolsDir := filepath.Join(cwd, "tools", "xw-sshpass")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	realBin := filepath.Join(toolsDir, platformSshpassName())
	t.Cleanup(func() { _ = os.Remove(realBin) })
	data, _ := os.ReadFile(fakeBin)
	if err := os.WriteFile(realBin, data, 0755); err != nil {
		t.Fatalf("write real bin: %v", err)
	}

	tests := []struct {
		name      string
		port      string
		wantNoP   bool // 期望参数中不出现 -P flag
	}{
		{name: "port_22", port: "22", wantNoP: true},
		{name: "port_empty", port: "", wantNoP: true},
		{name: "port_2222", port: "2222", wantNoP: false}, // 应该出现 -P
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Remove(captureFile)
			ds := &backend.DirShortcut{
				Name:           "default-test",
				Type:           backend.DirShortcutTypeRemote,
				RemoteHost:     "10.0.0.1",
				RemoteUser:     "root",
				RemotePort:     tt.port,
				AuthMethod:     "password",
				RemotePassword: "secret",
			}
			ctx := context.Background()
			_, _ = RunViaXwSshpass(ctx, ds, []string{"echo", "hi"}, "", nil)

			captured, err := os.ReadFile(captureFile)
			if err != nil {
				t.Fatalf("read capture: %v", err)
			}
			capturedStr := string(captured)
			t.Logf("xw-sshpass 收到: %s", capturedStr)

			hasPFlag := false
			for _, arg := range splitArgs(capturedStr) {
				if arg == "-P" {
					hasPFlag = true
					break
				}
			}
			if tt.wantNoP && hasPFlag {
				t.Errorf("默认端口不应出现 -P flag，实际: %s", capturedStr)
			}
			if !tt.wantNoP && !hasPFlag {
				t.Errorf("非默认端口应出现 -P flag，实际: %s", capturedStr)
			}
		})
	}
}

// splitArgs 简单按空白切分
func splitArgs(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}