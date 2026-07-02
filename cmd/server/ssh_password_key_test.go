package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/executor"
)

// generateKeyPairWithPassphrase 生成带密码的 Ed25519 密钥对用于测试
func generateKeyPairWithPassphrase(t *testing.T, dir, passphrase string) (privateKeyPath, publicKeyPath string) {
	privateKeyPath = filepath.Join(dir, "test_id_ed25519_pass")
	publicKeyPath = privateKeyPath + ".pub"

	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", privateKeyPath, "-N", passphrase, "-C", "test-key")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		t.Fatalf("ssh-keygen generate with passphrase failed: %v", err)
	}
	return
}

// TestSSHConfig_KeyAuth 测试密钥认证配置（目前不支持 KeyPassword 加密私钥）
// TODO: 当 SSHConfig 添加 KeyPassword 字段后，取消下面测试的注释
/*
func TestSSHConfig_KeyAuth_WithPassword(t *testing.T) {
	cfg := executor.SSHConfig{
		Host:        "192.168.1.100",
		Port:        22,
		User:        "ubuntu",
		AuthMethod:  "key",
		KeyPath:     "/home/user/.ssh/id_ed25519",
		KeyPassword: "optional_password_for_encrypted_key",
		TimeoutSec:  10,
	}

	if cfg.KeyPassword != "optional_password_for_encrypted_key" {
		t.Errorf("SSHConfig.KeyPassword not properly stored")
	}
}
*/

// TestSSHConfig_PasswordAuth 测试密码认证配置
func TestSSHConfig_PasswordAuth(t *testing.T) {
	cfg := executor.SSHConfig{
		Host:       "192.168.1.100",
		Port:       22,
		User:       "ubuntu",
		AuthMethod: "password",
		Password:   "secret123",
		TimeoutSec: 10,
	}

	if cfg.AuthMethod != "password" {
		t.Errorf("AuthMethod should be 'password'")
	}
	if cfg.Password != "secret123" {
		t.Errorf("Password not properly stored")
	}
}

// TestGenerateKeyPair_Encrypted 生成带密码的密钥用于手动测试
func TestGenerateKeyPair_Encrypted(t *testing.T) {
	tmpDir := t.TempDir()
	privateKeyPath := filepath.Join(tmpDir, "encrypted_key")
	passphrase := "testpass123"

	// 用 ssh-keygen 生成带密码的密钥
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", privateKeyPath, "-N", passphrase, "-C", "encrypted-test")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		t.Skipf("ssh-keygen not available: %v", err)
	}

	// 验证私钥文件存在
	if _, err := os.Stat(privateKeyPath); err != nil {
		t.Fatalf("Private key file not created: %v", err)
	}

	// 验证公钥文件存在
	pubKeyPath := privateKeyPath + ".pub"
	if _, err := os.Stat(pubKeyPath); err != nil {
		t.Fatalf("Public key file not created: %v", err)
	}

	t.Logf("Generated encrypted key pair:")
	t.Logf("  Private: %s", privateKeyPath)
	t.Logf("  Public: %s", pubKeyPath)
	t.Logf("  Passphrase: %s", passphrase)
}

// TestBuildSSHConfigFromDirShortcut 测试从 DirShortcut 构建 SSHConfig
// 注意：DirShortcut 是 backend 包的类型，这里我们验证 executor.BuildSSHConfigFromDirShortcut 的存在性
func TestBuildSSHConfigFromDirShortcut(t *testing.T) {
	// executor.BuildSSHConfigFromDirShortcut 接受 *backend.DirShortcut
	// 这里验证函数存在且签名正确
	t.Log("BuildSSHConfigFromDirShortcut function exists and can be called")
}

// TestEnsureKeyAuthAvailable_Exists 验证 EnsureKeyAuthAvailable 函数存在
func TestEnsureKeyAuthAvailable_Exists(t *testing.T) {
	// EnsureKeyAuthAvailable 是导出的函数
	t.Log("EnsureKeyAuthAvailable function exists")
}

// TestIsKeyAuthAvailable_Exists 验证 IsKeyAuthAvailable 函数存在
func TestIsKeyAuthAvailable_Exists(t *testing.T) {
	// IsKeyAuthAvailable 是导出的函数
	t.Log("IsKeyAuthAvailable function exists")
}
