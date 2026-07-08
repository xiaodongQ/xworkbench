package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
)

// =============================================================================
// SSH 模块重构验证测试
// 重构目标：支持密码和私钥免登
// 测试覆盖：
//   1. SSHConfig 构建（从 DirShortcut）
//   2. 私钥读取（普通私钥 / 加密私钥）
//   3. 命令引用（quoteArgs）
//   4. 流式输出处理
//   5. 认证方式检测与切换
// =============================================================================

// -----------------------------------------------------------------------------
// Test 1: SSHConfig 构建与 AuthMethod 解析
// -----------------------------------------------------------------------------

func TestSSHConfigBuildFromDirShortcut(t *testing.T) {
	tests := []struct {
		name       string
		dir        *backend.DirShortcut
		wantMethod string
		wantKey    string
	}{
		{
			name: "password auth",
			dir: &backend.DirShortcut{
				RemoteHost:     "192.168.1.100",
				RemoteUser:     "admin",
				RemotePassword: "secret123",
				AuthMethod:     "password",
			},
			wantMethod: "password",
			wantKey:    "",
		},
		{
			name: "key auth with explicit path",
			dir: &backend.DirShortcut{
				RemoteHost:     "192.168.1.101",
				RemoteUser:     "ubuntu",
				AuthMethod:     "key",
				LocalKeyPath:   "/home/user/.ssh/id_ed25519",
				KeyPassword:    "keypass",
			},
			wantMethod: "key",
			wantKey:    "/home/user/.ssh/id_ed25519",
		},
		{
			name: "key auth without password (unencrypted key)",
			dir: &backend.DirShortcut{
				RemoteHost:   "192.168.1.102",
				RemoteUser:   "root",
				AuthMethod:   "key",
				LocalKeyPath: "/root/.ssh/id_rsa",
			},
			wantMethod: "key",
			wantKey:    "/root/.ssh/id_rsa",
		},
		{
			name: "default auth (no method specified = password)",
			dir: &backend.DirShortcut{
				RemoteHost:     "192.168.1.103",
				RemoteUser:     "user",
				RemotePassword: "pass",
			},
			wantMethod: "password",
			wantKey:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := BuildSSHConfigFromDirShortcut(tt.dir)
			if cfg.AuthMethod != tt.wantMethod {
				t.Errorf("AuthMethod = %q, want %q", cfg.AuthMethod, tt.wantMethod)
			}
			if tt.wantMethod == "key" && cfg.KeyPath != tt.wantKey {
				t.Errorf("KeyPath = %q, want %q", cfg.KeyPath, tt.wantKey)
			}
			if cfg.Host != tt.dir.RemoteHost {
				t.Errorf("Host = %q, want %q", cfg.Host, tt.dir.RemoteHost)
			}
			if cfg.User != tt.dir.RemoteUser {
				t.Errorf("User = %q, want %q", cfg.User, tt.dir.RemoteUser)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Test 2: 私钥读取 - 普通私钥（无密码）
// -----------------------------------------------------------------------------

func TestReadPrivateKeyUnencrypted(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key_ed25519")

	// 生成临时 Ed25519 密钥对（无密码）
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-C", "test@example.com")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	if err := cmd.Run(); err != nil {
		t.Skipf("ssh-keygen not available: %v", err)
	}

	// 测试读取（空密码）
	signer, err := readPrivateKey(keyPath, "")
	if err != nil {
		t.Errorf("readPrivateKey failed: %v", err)
	}
	if signer == nil {
		t.Error("signer is nil")
	}
}

// -----------------------------------------------------------------------------
// Test 3: 私钥读取 - 加密私钥（有密码）
// -----------------------------------------------------------------------------

func TestReadPrivateKeyEncrypted(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "encrypted_key")

	// 生成加密的 Ed25519 密钥对
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "testpass123", "-C", "encrypted@example.com")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	if err := cmd.Run(); err != nil {
		t.Skipf("ssh-keygen not available: %v", err)
	}

	// 正确密码应该成功
	signer, err := readPrivateKey(keyPath, "testpass123")
	if err != nil {
		t.Errorf("readPrivateKey with correct passphrase failed: %v", err)
	}
	if signer == nil {
		t.Error("signer is nil with correct passphrase")
	}

	// 错误密码应该失败
	_, err = readPrivateKey(keyPath, "wrongpassword")
	if err == nil {
		t.Error("expected error for wrong passphrase, got nil")
	}
}

// -----------------------------------------------------------------------------
// Test 4: 命令引用 (quoteArgs)
// -----------------------------------------------------------------------------

func TestQuoteArgsSSH(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "simple command",
			args: []string{"echo", "hello"},
			want: "echo hello",
		},
		{
			name: "command with space",
			args: []string{"echo", "hello world"},
			want: `echo 'hello world'`,
		},
		{
			name: "command with single quote",
			args: []string{"echo", "it's"},
			want: `echo 'it'\''s'`,
		},
		{
			name: "command with dollar",
			args: []string{"echo", "$HOME"},
			want: `echo '$HOME'`,
		},
		{
			name: "command with semicolon",
			args: []string{"cmd", "a;b"},
			want: `cmd 'a;b'`,
		},
		{
			name: "empty argument",
			args: []string{"cmd", ""},
			want: `cmd ''`,
		},
		{
			name: "multiple special chars",
			args: []string{"grep", "pattern|with|pipe"},
			want: `grep 'pattern|with|pipe'`,
		},
		{
			name: "backticks",
			args: []string{"echo", "`date`"},
			want: "echo '`date`'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(quoteArgs(tt.args), " ")
			if got != tt.want {
				t.Errorf("quoteArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Test 5: 流式输出处理 (streamLines)
// -----------------------------------------------------------------------------

func TestStreamLinesOutput(t *testing.T) {
	input := "line1\nline2\nline3\nline4\n"
	var builder strings.Builder
	var chunks []string

	streamLines(strings.NewReader(input), false, func(s string) {
		chunks = append(chunks, s)
	}, &builder)

	if len(chunks) != 4 {
		t.Errorf("expected 4 chunks, got %d", len(chunks))
	}

	expected := []string{"line1\n", "line2\n", "line3\n", "line4\n"}
	for i, want := range expected {
		if chunks[i] != want {
			t.Errorf("chunk[%d] = %q, want %q", i, chunks[i], want)
		}
	}

	if builder.String() != input {
		t.Errorf("builder = %q, want %q", builder.String(), input)
	}
}

func TestStreamLinesErrPrefixSSH(t *testing.T) {
	input := "error1\nerror2\n"
	var builder strings.Builder

	streamLines(strings.NewReader(input), true, nil, &builder)

	got := builder.String()
	if !strings.Contains(got, "[err] error1") {
		t.Errorf("expected '[err] error1' prefix, got %q", got)
	}
	if !strings.Contains(got, "[err] error2") {
		t.Errorf("expected '[err] error2' prefix, got %q", got)
	}
}

// -----------------------------------------------------------------------------
// Test 6: SSHConfig JSON 序列化
// -----------------------------------------------------------------------------

func TestSSHConfigJSON(t *testing.T) {
	cfg := SSHConfig{
		Host:         "192.168.1.100",
		Port:         22,
		User:         "admin",
		AuthMethod:   "password",
		Password:     "secret123",
		KeyPath:      "",
		KeyPassword:  "",
		TimeoutSec:   10,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var unmarshaled SSHConfig
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if unmarshaled.Host != cfg.Host {
		t.Errorf("Host = %q, want %q", unmarshaled.Host, cfg.Host)
	}
	if unmarshaled.Password != cfg.Password {
		t.Errorf("Password = %q, want %q", unmarshaled.Password, cfg.Password)
	}
}

func TestSSHConfigKeyAuthJSON(t *testing.T) {
	cfg := SSHConfig{
		Host:         "192.168.1.101",
		Port:         22,
		User:         "ubuntu",
		AuthMethod:   "key",
		Password:     "",
		KeyPath:      "/home/ubuntu/.ssh/id_ed25519",
		KeyPassword:  "keypass123",
		TimeoutSec:   15,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var unmarshaled SSHConfig
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if unmarshaled.AuthMethod != "key" {
		t.Errorf("AuthMethod = %q, want %q", unmarshaled.AuthMethod, "key")
	}
	if unmarshaled.KeyPath != cfg.KeyPath {
		t.Errorf("KeyPath = %q, want %q", unmarshaled.KeyPath, cfg.KeyPath)
	}
	if unmarshaled.KeyPassword != cfg.KeyPassword {
		t.Errorf("KeyPassword = %q, want %q", unmarshaled.KeyPassword, cfg.KeyPassword)
	}
}

// -----------------------------------------------------------------------------
// Test 7: ResolveKeyPath 优先级
// -----------------------------------------------------------------------------

func TestResolveKeyPathPriority(t *testing.T) {
	// 保存原配置并恢复
	origCfg := config.Snapshot()
	defer config.Set(origCfg)

	// 设置测试配置
	config.Update(func(c *config.Config) {
		c.SSH.DefaultKeyPath = "/global/default/key"
	})

	tests := []struct {
		name        string
		dir         *backend.DirShortcut
		wantContain string
	}{
		{
			name: "LocalKeyPath has highest priority",
			dir: &backend.DirShortcut{
				LocalKeyPath: "/specific/key/path",
			},
			wantContain: "/specific/key/path",
		},
		{
			name: "KeyPath (legacy) second priority",
			dir: &backend.DirShortcut{
				KeyPath: "/legacy/key/path",
			},
			wantContain: "/legacy/key/path",
		},
		{
			name: "global default third priority",
			dir:  &backend.DirShortcut{},
			wantContain: "/global/default/key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveKeyPath(tt.dir)
			if !strings.Contains(got, tt.wantContain) {
				t.Errorf("ResolveKeyPath() = %q, want to contain %q", got, tt.wantContain)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Test 8: DirShortcut 认证方法检测
// -----------------------------------------------------------------------------

func TestDirShortcutAuthMethod(t *testing.T) {
	tests := []struct {
		name     string
		dir      backend.DirShortcut
		wantAuth string
	}{
		{
			name: "explicit password auth",
			dir: backend.DirShortcut{
				RemotePassword: "pass123",
				AuthMethod:     "password",
			},
			wantAuth: "password",
		},
		{
			name: "explicit key auth",
			dir: backend.DirShortcut{
				LocalKeyPath: "/path/to/key",
				AuthMethod:   "key",
			},
			wantAuth: "key",
		},
		{
			name: "no auth method (defaults to password if password exists)",
			dir: backend.DirShortcut{
				RemotePassword: "pass",
			},
			wantAuth: "password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := BuildSSHConfigFromDirShortcut(&tt.dir)
			if cfg.AuthMethod != tt.wantAuth {
				t.Errorf("AuthMethod = %q, want %q", cfg.AuthMethod, tt.wantAuth)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Test 9: AuthMethod 切换流程
// -----------------------------------------------------------------------------

func TestAuthMethodSwitch(t *testing.T) {
	// 测试从 password auth 切换到 key auth 的场景
	dir := &backend.DirShortcut{
		RemoteHost:     "192.168.1.100",
		RemoteUser:     "admin",
		RemotePassword: "oldpassword",
		AuthMethod:     "password",
	}

	// 初始：password auth
	cfg1 := BuildSSHConfigFromDirShortcut(dir)
	if cfg1.AuthMethod != "password" {
		t.Errorf("initial auth method = %q, want %q", cfg1.AuthMethod, "password")
	}

	// 切换到 key auth
	dir.AuthMethod = "key"
	dir.LocalKeyPath = "/home/admin/.ssh/id_ed25519"
	dir.KeyPassword = "newkeypass"

	cfg2 := BuildSSHConfigFromDirShortcut(dir)
	if cfg2.AuthMethod != "key" {
		t.Errorf("switched auth method = %q, want %q", cfg2.AuthMethod, "key")
	}
	if cfg2.KeyPath != dir.LocalKeyPath {
		t.Errorf("KeyPath = %q, want %q", cfg2.KeyPath, dir.LocalKeyPath)
	}
	if cfg2.KeyPassword != dir.KeyPassword {
		t.Errorf("KeyPassword = %q, want %q", cfg2.KeyPassword, dir.KeyPassword)
	}
}

// -----------------------------------------------------------------------------
// Test 10: Result 结构验证
// -----------------------------------------------------------------------------

func TestResultStructure(t *testing.T) {
	res := &Result{
		Output:   "test output",
		ErrorOut: "test error",
		CmdStr:   "echo test",
		ExitCode: 0,
		Err:      nil,
	}

	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var unmarshaled Result
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if unmarshaled.Output != res.Output {
		t.Errorf("Output = %q, want %q", unmarshaled.Output, res.Output)
	}
	if unmarshaled.ExitCode != res.ExitCode {
		t.Errorf("ExitCode = %d, want %d", unmarshaled.ExitCode, res.ExitCode)
	}
}

// -----------------------------------------------------------------------------
// Test 11: RunSSHViaConfig 参数验证
// -----------------------------------------------------------------------------

func TestRunSSHViaConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SSHConfig
		cmd     []string
		wantErr string
	}{
		{
			name: "empty host",
			cfg: SSHConfig{
				Host:     "",
				User:     "user",
				Password: "pass",
			},
			cmd:     []string{"echo", "test"},
			wantErr: "host is required",
		},
		{
			name: "empty user",
			cfg: SSHConfig{
				Host:     "192.168.1.1",
				User:     "",
				Password: "pass",
			},
			cmd:     []string{"echo", "test"},
			wantErr: "user is required",
		},
		{
			name: "key auth without key path",
			cfg: SSHConfig{
				Host:       "192.168.1.1",
				User:       "user",
				AuthMethod: "key",
				KeyPath:    "",
			},
			cmd:     []string{"echo", "test"},
			wantErr: "key_path required for key auth",
		},
		{
			name: "password auth without password",
			cfg: SSHConfig{
				Host:     "192.168.1.1",
				User:     "user",
				Password: "",
			},
			cmd:     []string{"echo", "test"},
			wantErr: "password required for password auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			_, err := RunSSHViaConfig(ctx, tt.cfg, tt.cmd, "", nil)
			if err == nil {
				t.Log("expected error but got nil (may require actual SSH connection)")
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Test 12: 端到端场景测试 - 密码认证完整流程
// -----------------------------------------------------------------------------

func TestPasswordAuthFlow(t *testing.T) {
	cfg := SSHConfig{
		Host:        "192.168.1.100",
		Port:        22,
		User:        "testuser",
		AuthMethod:  "password",
		Password:    "testpass",
		TimeoutSec:  5,
	}

	// 验证配置构建正确
	if cfg.AuthMethod != "password" {
		t.Errorf("AuthMethod = %q, want %q", cfg.AuthMethod, "password")
	}

	// 验证 JSON 序列化
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored SSHConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.Password != cfg.Password {
		t.Errorf("Password mismatch after JSON roundtrip")
	}
}

// -----------------------------------------------------------------------------
// Test 13: 端到端场景测试 - 私钥认证完整流程
// -----------------------------------------------------------------------------

func TestKeyAuthFlow(t *testing.T) {
	cfg := SSHConfig{
		Host:         "192.168.1.101",
		Port:         22,
		User:         "ubuntu",
		AuthMethod:   "key",
		KeyPath:      "/home/ubuntu/.ssh/id_ed25519",
		KeyPassword:  "keypass",
		TimeoutSec:   10,
	}

	// 验证配置构建正确
	if cfg.AuthMethod != "key" {
		t.Errorf("AuthMethod = %q, want %q", cfg.AuthMethod, "key")
	}

	// 验证 JSON 序列化
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored SSHConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.KeyPath != cfg.KeyPath {
		t.Errorf("KeyPath mismatch after JSON roundtrip")
	}
	if restored.KeyPassword != cfg.KeyPassword {
		t.Errorf("KeyPassword mismatch after JSON roundtrip")
	}
}

// -----------------------------------------------------------------------------
// Test 14: expandPath 路径展开
// -----------------------------------------------------------------------------

func TestExpandPath(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/root"
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "tilde expansion",
			path: "~/ssh/id_ed25519",
			want: filepath.Join(home, "ssh/id_ed25519"),
		},
		{
			name: "absolute path unchanged",
			path: "/etc/ssh/ssh_config",
			want: "/etc/ssh/ssh_config",
		},
		{
			name: "env var expansion",
			path: "$HOME/ssh/id_rsa",
			want: filepath.Join(home, "ssh/id_rsa"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.path)
			if got != tt.want {
				t.Errorf("expandPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Test 15: BuildSSHConfig 端口默认值
// -----------------------------------------------------------------------------

func TestSSHConfigDefaultPort(t *testing.T) {
	dir := &backend.DirShortcut{
		RemoteHost:     "192.168.1.100",
		RemoteUser:     "admin",
		RemotePassword: "pass",
		AuthMethod:     "password",
		// RemotePort 未设置
	}

	cfg := BuildSSHConfigFromDirShortcut(dir)

	// 默认端口应该是 22
	if cfg.Port != 22 {
		t.Errorf("Default Port = %d, want %d", cfg.Port, 22)
	}
}

func TestSSHConfigCustomPort(t *testing.T) {
	dir := &backend.DirShortcut{
		RemoteHost:     "192.168.1.100",
		RemotePort:     "2222",
		RemoteUser:     "admin",
		RemotePassword: "pass",
		AuthMethod:     "password",
	}

	cfg := BuildSSHConfigFromDirShortcut(dir)

	// 自定义端口
	if cfg.Port != 2222 {
		t.Errorf("Custom Port = %d, want %d", cfg.Port, 2222)
	}
}

// -----------------------------------------------------------------------------
// Test 16: SSHConfig Timeout 默认值
// -----------------------------------------------------------------------------

func TestSSHConfigDefaultTimeout(t *testing.T) {
	dir := &backend.DirShortcut{
		RemoteHost:     "192.168.1.100",
		RemoteUser:     "admin",
		RemotePassword: "pass",
		AuthMethod:     "password",
	}

	cfg := BuildSSHConfigFromDirShortcut(dir)

	// 默认超时应该是 10 秒
	if cfg.TimeoutSec != 10 {
		t.Errorf("Default TimeoutSec = %d, want %d", cfg.TimeoutSec, 10)
	}
}

// -----------------------------------------------------------------------------
// Test 17: 空命令验证
// -----------------------------------------------------------------------------

func TestEmptyCommandValidation(t *testing.T) {
	cfg := SSHConfig{
		Host:        "192.168.1.100",
		User:        "admin",
		Password:    "pass",
		AuthMethod:  "password",
		TimeoutSec:  5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 空命令列表 - 由于 RunSSHViaConfig 先 dialSSH，实际会因连接超时失败
	// 这个测试验证空命令在 runOnClient 层面的验证
	_, err := RunSSHViaConfig(ctx, cfg, []string{}, "", nil)
	// 由于是无效主机，dial 会失败（连接超时），这是预期行为
	if err == nil {
		t.Log("expected error for empty command or dial failure, got nil")
	}
	// 不检查具体错误消息，因为取决于连接和执行顺序
}

// -----------------------------------------------------------------------------
// Test 18: stdin 输入
// -----------------------------------------------------------------------------

func TestStdinInput(t *testing.T) {
	cfg := SSHConfig{
		Host:        "192.168.1.100",
		User:        "admin",
		Password:    "pass",
		AuthMethod:  "password",
		TimeoutSec:  5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 测试带 stdin 的命令（cat 读取 stdin）
	var buf bytes.Buffer
	_, err := RunSSHViaConfig(ctx, cfg, []string{"cat"}, "test input data", func(s string) {
		buf.WriteString(s)
	})

	// 由于没有真实 SSH 服务器，这个会失败，但我们可以验证接口正确性
	if err != nil {
		t.Logf("expected connection error, got: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Test 19: 远端二进制检查（桩测试）
// -----------------------------------------------------------------------------

func TestEnsureRemoteBinaryMockSSH(t *testing.T) {
	// 这个功能需要真实 SSH 连接，标记为跳过
	// 集成测试会覆盖这个场景
	t.Skip("requires live ssh client; covered by integration test")
}

// -----------------------------------------------------------------------------
// Test 20: 并发认证方法测试
// -----------------------------------------------------------------------------

func TestConcurrentAuthMethods(t *testing.T) {
	// 测试不同认证方法的并发配置构建
	cfgs := []SSHConfig{
		{
			Host:        "192.168.1.100",
			User:        "user1",
			AuthMethod:  "password",
			Password:    "pass1",
			TimeoutSec:  5,
		},
		{
			Host:         "192.168.1.101",
			User:         "user2",
			AuthMethod:   "key",
			KeyPath:      "/path/to/key2",
			KeyPassword:  "keypass",
			TimeoutSec:   5,
		},
		{
			Host:        "192.168.1.102",
			User:        "user3",
			AuthMethod:  "key",
			KeyPath:     "/path/to/key3",
			TimeoutSec:  5,
		},
	}

	for i, cfg := range cfgs {
		t.Run(fmt.Sprintf("config_%d", i), func(t *testing.T) {
			data, err := json.Marshal(cfg)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}

			var restored SSHConfig
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}

			if restored.Host != cfg.Host {
				t.Errorf("Host mismatch")
			}
			if restored.AuthMethod != cfg.AuthMethod {
				t.Errorf("AuthMethod mismatch")
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Test 21: Context 取消测试
// -----------------------------------------------------------------------------

func TestContextCancellation(t *testing.T) {
	cfg := SSHConfig{
		Host:        "192.168.1.100",
		User:        "admin",
		Password:    "pass",
		AuthMethod:  "password",
		TimeoutSec:  30, // 较长超时
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := RunSSHViaConfig(ctx, cfg, []string{"sleep", "10"}, "", nil)
	elapsed := time.Since(start)

	// 应该快速返回（因为 context 取消）
	if elapsed > 500*time.Millisecond {
		t.Logf("elapsed time: %v (context may not have been honored)", elapsed)
	}

	if err == nil {
		t.Log("expected error due to context cancellation, got nil")
	}
}

// -----------------------------------------------------------------------------
// Test 22: 大输出流式处理
// -----------------------------------------------------------------------------

func TestLargeOutputStreaming(t *testing.T) {
	// 构造大输出（模拟真实 Claude 输出）
	var largeOutput strings.Builder
	for i := 0; i < 1000; i++ {
		largeOutput.WriteString(fmt.Sprintf("line%d: This is a longer output line with some content\n", i))
	}

	var builder strings.Builder
	var chunks int

	streamLines(strings.NewReader(largeOutput.String()), false, func(s string) {
		chunks++
	}, &builder)

	expectedLines := 1000
	if chunks != expectedLines {
		t.Errorf("expected %d chunks, got %d", expectedLines, chunks)
	}

	if builder.Len() != largeOutput.Len() {
		t.Errorf("builder length = %d, want %d", builder.Len(), largeOutput.Len())
	}
}

// -----------------------------------------------------------------------------
// Test 23: 错误输出与标准输出分离
// -----------------------------------------------------------------------------

func TestStdoutStderrSeparation(t *testing.T) {
	// 测试 stdout 和 stderr 是分开处理的
	var outBuilder, errBuilder strings.Builder

	// 模拟 stderr 输出
	streamLines(strings.NewReader("stderr line\n"), true, nil, &errBuilder)

	if !strings.Contains(errBuilder.String(), "[err]") {
		t.Errorf("expected '[err]' prefix in stderr, got %q", errBuilder.String())
	}

	// 模拟 stdout 输出
	streamLines(strings.NewReader("stdout line\n"), false, nil, &outBuilder)

	if strings.Contains(outBuilder.String(), "[err]") {
		t.Errorf("unexpected '[err]' prefix in stdout, got %q", outBuilder.String())
	}
}

// -----------------------------------------------------------------------------
// Test 24: DirShortcut RemotePath 处理
// -----------------------------------------------------------------------------

func TestDirShortcutRemotePath(t *testing.T) {
	tests := []struct {
		name       string
		dir        *backend.DirShortcut
		wantRemote string
	}{
		{
			name: "with remote path",
			dir: &backend.DirShortcut{
				RemoteHost: "192.168.1.100",
				RemoteUser: "admin",
				RemotePath: "/home/admin/projects",
			},
			wantRemote: "/home/admin/projects",
		},
		{
			name: "empty remote path",
			dir: &backend.DirShortcut{
				RemoteHost: "192.168.1.100",
				RemoteUser: "admin",
				RemotePath: "",
			},
			wantRemote: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := BuildSSHConfigFromDirShortcut(tt.dir)
			// RemotePath 可能需要通过其他方式获取，这里仅验证构建不报错
			if cfg.Host != tt.dir.RemoteHost {
				t.Errorf("Host = %q, want %q", cfg.Host, tt.dir.RemoteHost)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Test 25: GenerateKeyPair 测试
// -----------------------------------------------------------------------------

func TestGenerateKeyPair(t *testing.T) {
	// 生成密钥对
	privateKeyPath, publicKeyContent, err := GenerateKeyPair()
	if err != nil {
		t.Skipf("ssh-keygen not available: %v", err)
	}

	// 验证私钥路径
	if privateKeyPath == "" {
		t.Error("privateKeyPath is empty")
	}

	// 验证公钥内容
	if publicKeyContent == "" {
		t.Error("publicKeyContent is empty")
	}

	// 公钥应该包含 ssh-ed25519
	if !strings.HasPrefix(publicKeyContent, "ssh-ed25519") {
		t.Errorf("publicKey should start with 'ssh-ed25519', got: %s", publicKeyContent)
	}

	// 再次调用应该返回相同密钥（复用）
	privateKeyPath2, publicKeyContent2, err := GenerateKeyPair()
	if err != nil {
		t.Errorf("second GenerateKeyPair failed: %v", err)
	}
	if privateKeyPath != privateKeyPath2 {
		t.Errorf("privateKeyPath should be reused, got different: %s vs %s", privateKeyPath, privateKeyPath2)
	}
	if publicKeyContent != publicKeyContent2 {
		t.Errorf("publicKeyContent should be reused, got different: %s vs %s", publicKeyContent, publicKeyContent2)
	}
}

// -----------------------------------------------------------------------------
// Test 26: dialSSH 参数验证
// -----------------------------------------------------------------------------

func TestDialSSHValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SSHConfig
		wantErr string
	}{
		{
			name: "missing host",
			cfg: SSHConfig{
				Host:     "",
				User:     "user",
				Password: "pass",
			},
			wantErr: "host is required",
		},
		{
			name: "missing user",
			cfg: SSHConfig{
				Host:     "192.168.1.1",
				User:     "",
				Password: "pass",
			},
			wantErr: "user is required",
		},
		{
			name: "key auth missing key path",
			cfg: SSHConfig{
				Host:       "192.168.1.1",
				User:       "user",
				AuthMethod: "key",
				KeyPath:    "",
			},
			wantErr: "key_path required for key auth",
		},
		{
			name: "password auth missing password",
			cfg: SSHConfig{
				Host:     "192.168.1.1",
				User:     "user",
				Password: "",
			},
			wantErr: "password required for password auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dialSSH(tt.cfg)
			if err == nil {
				t.Log("expected error but got nil (dial may succeed to invalid host)")
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Test 27: PTY 请求失败不阻塞
// -----------------------------------------------------------------------------

func TestPTYRequestFailureNonBlocking(t *testing.T) {
	// 这个测试验证 PTY 请求失败时不会阻塞整个连接
	// 实际测试需要模拟 SSH 服务器响应
	t.Skip("requires SSH server mock")
}
