package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"github.com/xiaodongQ/xworkbench/internal/logger"
	"golang.org/x/crypto/ssh"
)

// defaultKeyFileName 默认密钥文件名（Ed25519）
const defaultKeyFileName = "xworkbench_id_ed25519"

// ResolveKeyPath 解析实际使用的私钥路径。
// 优先级：LocalKeyPath > KeyPath > config.ssh.default_key_path > ~/.ssh/xworkbench_id_ed25519
func ResolveKeyPath(dir *backend.DirShortcut) string {
	// 1. 单记录覆盖
	if dir.LocalKeyPath != "" {
		return expandPath(dir.LocalKeyPath)
	}
	// 2. 已有字段 KeyPath（兼容老数据）
	if dir.KeyPath != "" {
		return expandPath(dir.KeyPath)
	}
	// 3. 全局默认
	cfg := config.Get()
	if cfg != nil && cfg.SSH.DefaultKeyPath != "" {
		return expandPath(cfg.SSH.DefaultKeyPath)
	}
	// 4. 硬编码兜底
	return filepath.Join(os.Getenv("HOME"), ".ssh", defaultKeyFileName)
}

// expandPath 展开 ~ 和环境变量
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	return os.ExpandEnv(path)
}

// GenerateKeyPair 生成本地 Ed25519 密钥对（如果不存在）。
// 返回私钥路径和公钥内容。
func GenerateKeyPair() (privateKeyPath, publicKeyContent string, err error) {
	sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", "", fmt.Errorf("create .ssh dir: %w", err)
	}

	privateKeyPath = filepath.Join(sshDir, defaultKeyFileName)
	publicKeyPath := privateKeyPath + ".pub"

	// 已存在则复用
	if _, err := os.Stat(privateKeyPath); err == nil {
		pubBytes, err := os.ReadFile(publicKeyPath)
		if err != nil {
			return "", "", fmt.Errorf("read existing public key: %w", err)
		}
		return privateKeyPath, strings.TrimSpace(string(pubBytes)), nil
	}

	// 用 ssh-keygen 生成 Ed25519 密钥对（空密码，-f 指定路径，-N '' 表示无密码）
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", privateKeyPath, "-N", "", "-C", "xworkbench-generated")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("ssh-keygen generate: %w", err)
	}

	// 读取生成的公钥内容
	pubBytes, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", "", fmt.Errorf("read public key: %w", err)
	}

	logger.Logger.Infow("[ssh_key_manager] generated new Ed25519 key pair", "private", privateKeyPath)
	return privateKeyPath, strings.TrimSpace(string(pubBytes)), nil
}

// CheckRemoteAuthorizedKeys 检测公钥是否已在远程 authorized_keys 中。
func CheckRemoteAuthorizedKeys(ctx context.Context, cfg SSHConfig, pubKey string) (bool, error) {
	client, err := dialSSH(cfg)
	if err != nil {
		return false, fmt.Errorf("dial ssh: %w", err)
	}
	defer client.Close()

	// 检查 ~/.ssh/authorized_keys 是否包含公钥
	// 用 grep -F（固定字符串，不正则）避免特殊字符问题
	cmd := fmt.Sprintf("grep -qF '%s' ~/.ssh/authorized_keys 2>/dev/null && echo 'found' || echo 'not_found'", pubKey)
	out, err := runSingleCommand(ctx, client, cmd)
	if err != nil {
		return false, fmt.Errorf("check authorized_keys: %w", err)
	}
	return strings.TrimSpace(out) == "found", nil
}

// SetupAuthorizedKeys 用已有认证把公钥写入远程 authorized_keys。
// 自动处理目录不存在、权限问题。
func SetupAuthorizedKeys(ctx context.Context, cfg SSHConfig, pubKey string) error {
	client, err := dialSSH(cfg)
	if err != nil {
		return fmt.Errorf("dial ssh: %w", err)
	}
	defer client.Close()

	// 构建写入命令：创建目录 + 设置权限 + 追加公钥（去重）
	script := fmt.Sprintf(`
mkdir -p ~/.ssh && chmod 700 ~/.ssh
grep -qF '%s' ~/.ssh/authorized_keys 2>/dev/null || echo '%s' >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
`, pubKey, pubKey)

	_, err = runSingleCommand(ctx, client, script)
	if err != nil {
		return fmt.Errorf("setup authorized_keys: %w", err)
	}

	logger.Logger.Infow("[ssh_key_manager] authorized_keys updated", "host", cfg.Host, "user", cfg.User)
	return nil
}

// BuildSSHConfigFromDirShortcut 从 DirShortcut 构建 SSHConfig。
func BuildSSHConfigFromDirShortcut(dir *backend.DirShortcut) SSHConfig {
	// 解析端口，默认 22
	port := 22
	if dir.RemotePort != "" {
		if p, err := strconv.Atoi(dir.RemotePort); err == nil && p > 0 {
			port = p
		}
	}

	// AuthMethod 默认 "password"
	authMethod := dir.AuthMethod
	if authMethod == "" {
		authMethod = "password"
	}

	return SSHConfig{
		Host:         dir.RemoteHost,
		Port:         port,
		User:         dir.RemoteUser,
		AuthMethod:   authMethod,
		Password:     dir.RemotePassword,
		KeyPath:      ResolveKeyPath(dir),
		KeyPassword:  dir.KeyPassword,
		TimeoutSec:   10,
	}
}

// IsKeyAuthAvailable 检测当前配置是否已可密钥免密登录。
// 检测逻辑：私钥文件存在 + 公钥在远程 authorized_keys 中。
func IsKeyAuthAvailable(ctx context.Context, dir *backend.DirShortcut) (bool, error) {
	keyPath := ResolveKeyPath(dir)
	if _, err := os.Stat(keyPath); err != nil {
		return false, nil // 私钥不存在
	}

	// 读取本地公钥内容
	_, pubKey, err := GenerateKeyPair()
	if err != nil {
		return false, err
	}

	// 用 password 或已有 key 认证去检测远程 authorized_keys
	cfg := BuildSSHConfigFromDirShortcut(dir)
	return CheckRemoteAuthorizedKeys(ctx, cfg, pubKey)
}

// EnsureKeyAuthAvailable 确保密钥免密可用：生成密钥对 + 上传公钥（如需要）。
// 使用 dir 中已有的认证信息（password 或 key）来完成初始配置。
func EnsureKeyAuthAvailable(ctx context.Context, dir *backend.DirShortcut) (keyPath string, err error) {
	// 1. 生成或读取本地密钥对
	keyPath, pubKey, err := GenerateKeyPair()
	if err != nil {
		return "", fmt.Errorf("generate key pair: %w", err)
	}

	// 2. 构建 SSH 配置（用 dir 的 password 或已有 key 做初始认证上传公钥）
	cfg := BuildSSHConfigFromDirShortcut(dir)

	// 3. 检查远程是否已有公钥
	hasKey, err := CheckRemoteAuthorizedKeys(ctx, cfg, pubKey)
	if err != nil {
		// 网络或连接问题，不阻塞，继续尝试上传
		logger.Logger.Warnw("[ssh_key_manager] check authorized_keys failed, trying setup anyway",
			"host", cfg.Host, "error", err.Error())
	}

	if hasKey {
		logger.Logger.Infow("[ssh_key_manager] remote already has public key", "host", cfg.Host)
		return keyPath, nil
	}

	// 4. 上传公钥到远程
	if err := SetupAuthorizedKeys(ctx, cfg, pubKey); err != nil {
		return "", fmt.Errorf("upload public key: %w", err)
	}

	return keyPath, nil
}

// runSingleCommand 在已建连的 SSH client 上执行单条命令，返回 stdout。
func runSingleCommand(ctx context.Context, client *ssh.Client, cmd string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()

	out, err := sess.Output(cmd)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

