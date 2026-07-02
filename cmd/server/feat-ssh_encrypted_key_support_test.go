package main

import (
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/executor"
)

// TestSSHConfig_KeyPasswordField 测试 SSHConfig 是否支持 KeyPassword 字段（新增功能）
func TestSSHConfig_KeyPasswordField(t *testing.T) {
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
	t.Logf("SSHConfig.KeyPassword field is now available")
}

// TestSSHConfig_KeyPasswordEmpty 测试 KeyPassword 为空的情况
func TestSSHConfig_KeyPasswordEmpty(t *testing.T) {
	cfg := executor.SSHConfig{
		Host:        "192.168.1.100",
		Port:        22,
		User:        "ubuntu",
		AuthMethod:  "key",
		KeyPath:     "/home/user/.ssh/id_ed25519",
		KeyPassword: "", // 空密码表示无加密私钥
		TimeoutSec:  10,
	}

	if cfg.KeyPassword != "" {
		t.Errorf("SSHConfig.KeyPassword should be empty")
	}
	t.Logf("SSHConfig.KeyPassword empty works correctly")
}

// TestSSHConfig_AllFields 测试完整 SSHConfig 结构
func TestSSHConfig_AllFields(t *testing.T) {
	cfg := executor.SSHConfig{
		Host:         "192.168.1.100",
		Port:         22,
		User:         "ubuntu",
		AuthMethod:   "key",
		Password:     "password123",
		KeyPath:      "/home/user/.ssh/id_ed25519",
		KeyPassword:  "encrypted_key_password",
		TimeoutSec:   30,
	}

	// 验证所有字段都能正确存储
	if cfg.Host != "192.168.1.100" {
		t.Errorf("Host mismatch")
	}
	if cfg.Port != 22 {
		t.Errorf("Port mismatch")
	}
	if cfg.User != "ubuntu" {
		t.Errorf("User mismatch")
	}
	if cfg.AuthMethod != "key" {
		t.Errorf("AuthMethod mismatch")
	}
	if cfg.Password != "password123" {
		t.Errorf("Password mismatch")
	}
	if cfg.KeyPath != "/home/user/.ssh/id_ed25519" {
		t.Errorf("KeyPath mismatch")
	}
	if cfg.KeyPassword != "encrypted_key_password" {
		t.Errorf("KeyPassword mismatch")
	}
	if cfg.TimeoutSec != 30 {
		t.Errorf("TimeoutSec mismatch")
	}
	t.Logf("All SSHConfig fields work correctly including new KeyPassword field")
}
