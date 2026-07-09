package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/executor"
)

// TestSSH_PasswordKeyAuth_BuildSSHConfigFromDirShortcut 测试从 DirShortcut 构建 SSHConfig
func TestSSH_PasswordKeyAuth_BuildSSHConfigFromDirShortcut(t *testing.T) {
	tests := []struct {
		name string
		dir  *backend.DirShortcut
		opts []string // 要检查的字段，格式 "field=value"
	}{
		{
			name: "password_auth_explicit",
			dir: &backend.DirShortcut{
				RemoteHost:     "192.168.1.100",
				RemotePort:     "22",
				RemoteUser:     "admin",
				AuthMethod:     "password",
				RemotePassword: "secret123",
			},
			opts: []string{"AuthMethod=password", "Password=secret123"},
		},
		{
			name: "key_auth_with_explicit_path_and_key_password",
			dir: &backend.DirShortcut{
				RemoteHost:    "192.168.1.100",
				RemotePort:    "2222",
				RemoteUser:    "root",
				AuthMethod:    "key",
				LocalKeyPath:  "/home/user/.ssh/id_ed25519",
				KeyPassword:   "encrypted_pass",
			},
			opts: []string{"AuthMethod=key", "KeyPath=/home/user/.ssh/id_ed25519", "KeyPassword=encrypted_pass"},
		},
		{
			name: "key_auth_without_password_(unencrypted_key)",
			dir: &backend.DirShortcut{
				RemoteHost:   "192.168.1.100",
				RemoteUser:   "root",
				AuthMethod:   "key",
				LocalKeyPath: "/home/user/.ssh/id_ed25519",
			},
			opts: []string{"AuthMethod=key", "KeyPath=/home/user/.ssh/id_ed25519"},
		},
		{
			name: "default_auth_(no_method_specified_=_password)",
			dir: &backend.DirShortcut{
				RemoteHost:     "192.168.1.100",
				RemoteUser:     "root",
				RemotePassword: "fallback_password",
			},
			opts: []string{"AuthMethod=password"},
		},
		{
			name: "custom_port",
			dir: &backend.DirShortcut{
				RemoteHost:      "192.168.1.100",
				RemotePort:      "2222",
				RemoteUser:      "root",
				AuthMethod:      "key",
				LocalKeyPath:    "/home/user/.ssh/key",
				RemotePassword:  "for_initial_setup",
			},
			opts: []string{"Port=2222", "AuthMethod=key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := executor.BuildSSHConfigFromDirShortcut(tt.dir)
			for _, opt := range tt.opts {
				parts := strings.SplitN(opt, "=", 2)
				if len(parts) != 2 {
					t.Fatalf("invalid opt format: %s", opt)
				}
				field, want := parts[0], parts[1]
				var got string
				switch field {
				case "AuthMethod":
					got = cfg.AuthMethod
				case "Password":
					got = cfg.Password
				case "KeyPath":
					got = cfg.KeyPath
				case "KeyPassword":
					got = cfg.KeyPassword
				case "Port":
					// numeric check below
					if (cfg.Port != 2222 && want == "2222") || (cfg.Port != 22 && want == "22") {
						t.Errorf("Port = %d, want %s", cfg.Port, want)
					}
					continue
				case "Host":
					got = cfg.Host
				case "User":
					got = cfg.User
				default:
					t.Fatalf("unknown field: %s", field)
				}
				if got != want {
					t.Errorf("%s = %q, want %q", field, got, want)
				}
			}
		})
	}
}

// TestSSH_PasswordKeyAuth_ResolveKeyPathPriority 测试密钥路径解析优先级
func TestSSH_PasswordKeyAuth_ResolveKeyPathPriority(t *testing.T) {
	dir := &backend.DirShortcut{
		LocalKeyPath: "/custom/path/key",
		KeyPath:      "/old/path/key",
	}

	path := executor.ResolveKeyPath(dir)
	if path != "/custom/path/key" {
		t.Errorf("ResolveKeyPath() = %q, want /custom/path/key", path)
	}
}

// TestSSH_PasswordKeyAuth_ResolveKeyPathTilde 测试 ~ 展开
func TestSSH_PasswordKeyAuth_ResolveKeyPathTilde(t *testing.T) {
	dir := &backend.DirShortcut{
		LocalKeyPath: "~/ssh_keys/mykey",
	}

	path := executor.ResolveKeyPath(dir)
	home := os.Getenv("HOME")
	want := filepath.Join(home, "ssh_keys/mykey")

	if path != want {
		t.Errorf("ResolveKeyPath() = %q, want %q", path, want)
	}
}

// TestSSH_PasswordKeyAuth_GenerateKeyPair 测试密钥对生成
func TestSSH_PasswordKeyAuth_GenerateKeyPair(t *testing.T) {
	privKeyPath, pubKey, err := executor.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	if privKeyPath == "" {
		t.Error("Private key path should not be empty")
	}
	if pubKey == "" {
		t.Error("Public key should not be empty")
	}
	if !strings.HasPrefix(pubKey, "ssh-") {
		t.Errorf("Public key should start with 'ssh-', got %q", pubKey)
	}

	// 清理测试密钥
	os.Remove(privKeyPath)
	os.Remove(privKeyPath + ".pub")
}

// TestSSH_PasswordKeyAuth_BuildSSHCommand 测试 SSH 命令构建
func TestSSH_PasswordKeyAuth_BuildSSHCommand(t *testing.T) {
	// 创建一个临时密钥文件用于测试
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")
	pubPath := keyPath + ".pub"

	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-C", "test")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		t.Fatalf("ssh-keygen failed: %v", err)
	}
	defer os.Remove(pubPath)

	tests := []struct {
		name          string
		dir           *backend.DirShortcut
		terminalType  string
		authMethod    string
		expectKeyFlag bool
	}{
		{
			name: "password_auth_-_no_key_flag_(default_key_not_exists)",
			dir: &backend.DirShortcut{
				RemoteUser:     "root",
				RemoteHost:     "192.168.1.150",
				AuthMethod:     "password",
				RemotePassword: "secret",
			},
			terminalType:  "wezterm",
			authMethod:    "password",
			expectKeyFlag: false,
		},
		{
			name: "key_auth_-_has_key_flag",
			dir: &backend.DirShortcut{
				RemoteUser:    "root",
				RemoteHost:    "192.168.1.150",
				AuthMethod:    "key",
				LocalKeyPath:  keyPath, // 使用实际存在的临时密钥
			},
			terminalType:  "wezterm",
			authMethod:    "key",
			expectKeyFlag: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := executor.BuildSSHCommand(tt.dir, tt.terminalType)
			if err != nil {
				t.Fatalf("BuildSSHCommand failed: %v", err)
			}

			// 检查是否包含 ssh
			if args[0] != "ssh" {
				t.Errorf("args[0] = %q, want ssh", args[0])
			}

			// 检查是否有 -i flag
			hasKeyFlag := false
			for i, arg := range args {
				if arg == "-i" && i+1 < len(args) {
					hasKeyFlag = true
					break
				}
			}
			if hasKeyFlag != tt.expectKeyFlag {
				t.Errorf("hasKeyFlag = %v, want %v", hasKeyFlag, tt.expectKeyFlag)
			}
		})
	}
}

// TestSSH_PasswordKeyAuth_Integration_AuthMethods 集成测试：验证两种认证方式的完整配置
func TestSSH_PasswordKeyAuth_Integration_AuthMethods(t *testing.T) {
	tests := []struct {
		name string
		dir  *backend.DirShortcut
	}{
		{
			name: "password_authentication",
			dir: &backend.DirShortcut{
				RemoteHost:     "192.168.1.100",
				RemotePort:     "22",
				RemoteUser:     "admin",
				AuthMethod:     "password",
				RemotePassword: "secret123",
			},
		},
		{
			name: "key_authentication_with_encrypted_private_key",
			dir: &backend.DirShortcut{
				RemoteHost:    "192.168.1.100",
				RemotePort:    "22",
				RemoteUser:    "root",
				AuthMethod:    "key",
				LocalKeyPath:  "/home/user/.ssh/id_ed25519",
				KeyPassword:   "my_key_password",
			},
		},
		{
			name: "key_authentication_with_unencrypted_private_key",
			dir: &backend.DirShortcut{
				RemoteHost:   "192.168.1.100",
				RemotePort:   "22",
				RemoteUser:   "root",
				AuthMethod:   "key",
				LocalKeyPath: "/home/user/.ssh/id_ed25519",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := executor.BuildSSHConfigFromDirShortcut(tt.dir)

			// 验证基本配置
			if cfg.Host == "" {
				t.Error("Host should not be empty")
			}
			if cfg.User == "" {
				t.Error("User should not be empty")
			}
			if cfg.Port == 0 {
				t.Error("Port should have default value")
			}

			// 验证认证方式配置
			switch cfg.AuthMethod {
			case "password":
				if cfg.Password == "" {
					t.Error("Password auth requires password")
				}
			case "key":
				if cfg.KeyPath == "" {
					t.Error("Key auth requires key path")
				}
			default:
				t.Errorf("Unknown auth method: %s", cfg.AuthMethod)
			}
		})
	}
}

// TestSSH_PasswordKeyAuth_IsKeyAuthAvailable 测试密钥可用性检测
func TestSSH_PasswordKeyAuth_IsKeyAuthAvailable(t *testing.T) {
	// 测试密钥不存在时的检测
	dir := &backend.DirShortcut{
		RemoteHost:     "192.168.1.100",
		RemoteUser:     "admin",
		RemotePassword: "secret",
		LocalKeyPath:   "/nonexistent/path/key",
	}

	available, err := executor.IsKeyAuthAvailable(context.Background(), dir)
	if err != nil {
		t.Errorf("IsKeyAuthAvailable error = %v", err)
	}
	if available {
		t.Error("KeyAuth should not be available when key file doesn't exist")
	}
}

// TestSSH_PasswordKeyAuth_EnsureKeyAuthAvailable_Signature 测试 EnsureKeyAuthAvailable 函数签名
func TestSSH_PasswordKeyAuth_EnsureKeyAuthAvailable_Signature(t *testing.T) {
	dir := &backend.DirShortcut{
		RemoteHost:     "192.168.1.100",
		RemoteUser:     "admin",
		RemotePassword: "secret",
	}

	// 这个测试只验证函数签名，不实际连接或上传
	// 因为没有真实的 SSH 服务器
	_, err := executor.EnsureKeyAuthAvailable(context.Background(), dir)

	// 预期会失败（因为没有真实服务器），但函数应该被调用
	// 不应该 panic 或编译错误
	_ = err
}

// TestSSH_PasswordKeyAuth_SSHConfig_Structure 测试 SSHConfig 结构完整性
func TestSSH_PasswordKeyAuth_SSHConfig_Structure(t *testing.T) {
	cfg := executor.SSHConfig{
		Host:         "192.168.1.100",
		Port:         22,
		User:         "ubuntu",
		AuthMethod:   "key",
		Password:     "secret123",
		KeyPath:      "/home/user/.ssh/id_ed25519",
		KeyPassword:  "encrypted_key_password",
		TimeoutSec:   30,
	}

	// 验证所有字段
	if cfg.Host != "192.168.1.100" {
		t.Errorf("Host mismatch: got %s", cfg.Host)
	}
	if cfg.Port != 22 {
		t.Errorf("Port mismatch: got %d", cfg.Port)
	}
	if cfg.User != "ubuntu" {
		t.Errorf("User mismatch: got %s", cfg.User)
	}
	if cfg.AuthMethod != "key" {
		t.Errorf("AuthMethod mismatch: got %s", cfg.AuthMethod)
	}
	if cfg.Password != "secret123" {
		t.Errorf("Password mismatch: got %s", cfg.Password)
	}
	if cfg.KeyPath != "/home/user/.ssh/id_ed25519" {
		t.Errorf("KeyPath mismatch: got %s", cfg.KeyPath)
	}
	if cfg.KeyPassword != "encrypted_key_password" {
		t.Errorf("KeyPassword mismatch: got %s", cfg.KeyPassword)
	}
	if cfg.TimeoutSec != 30 {
		t.Errorf("TimeoutSec mismatch: got %d", cfg.TimeoutSec)
	}
}

// TestSSH_PasswordKeyAuth_KeyPairGenerationIntegration 测试密钥对生成的完整流程
func TestSSH_PasswordKeyAuth_KeyPairGenerationIntegration(t *testing.T) {
	// 生成密钥对
	privKeyPath, pubKey, err := executor.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// 验证私钥文件存在
	if _, err := os.Stat(privKeyPath); os.IsNotExist(err) {
		t.Fatalf("Private key file not created: %s", privKeyPath)
	}

	// 验证公钥文件存在
	pubKeyPath := privKeyPath + ".pub"
	if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
		t.Fatalf("Public key file not created: %s", pubKeyPath)
	}

	// 验证公钥内容
	if pubKey == "" {
		t.Error("Public key content should not be empty")
	}
	if !strings.HasPrefix(pubKey, "ssh-") {
		t.Errorf("Public key should start with 'ssh-', got %q", pubKey)
	}

	// 验证公钥文件内容与返回内容一致
	pubContent, err := os.ReadFile(pubKeyPath)
	if err != nil {
		t.Fatalf("Failed to read public key file: %v", err)
	}
	if strings.TrimSpace(string(pubContent)) != pubKey {
		t.Errorf("Public key mismatch: file=%q, returned=%q", string(pubContent), pubKey)
	}

	// 清理测试密钥
	os.Remove(privKeyPath)
	os.Remove(pubKeyPath)
}

// TestSSH_PasswordKeyAuth_ResolveKeyPath_Fallback 测试密钥路径回退逻辑
func TestSSH_PasswordKeyAuth_ResolveKeyPath_Fallback(t *testing.T) {
	// 没有任何路径设置，应该回退到 ~/.ssh/xworkbench_id_ed25519
	dir := &backend.DirShortcut{}

	path := executor.ResolveKeyPath(dir)
	home := os.Getenv("HOME")
	expectedDefault := filepath.Join(home, ".ssh", "xworkbench_id_ed25519")

	if path != expectedDefault {
		t.Errorf("ResolveKeyPath() = %q, want default %q", path, expectedDefault)
	}
}
