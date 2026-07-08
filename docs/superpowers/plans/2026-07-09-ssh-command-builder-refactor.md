# SSH Command Builder 重构实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修掉快捷目录"打开远程终端"时 `ssh ssh ...` 双 binary bug，把 SSH 命令拼装职责从 `shortcuts/terminal.go` 拆到独立的 `executor/ssh_command_builder.go`，并把 SSH 兼容算法（Kex/HostKey/Cipher）挪到 `config.json` 可配置。

**Architecture:** 新建 `BuildSSHCommand(dir, termType) ([]string, error)` 作为纯函数负责 ssh 参数拼装（含 binary 选择、去重、`-i` 条件、兼容算法）；`shortcuts/terminal.go` 仅负责把 `[]string` 套到终端壳（wezterm/iterm2/wt...）并 `exec.Command().Start()`。

**Tech Stack:** Go 1.25 / vanilla config.json（无新依赖）/ table-driven 测试。

**关联 spec:** `docs/superpowers/specs/2026-07-09-ssh-command-builder-refactor-design.md`

---

## 文件结构

| 文件 | 职责 |
|---|---|
| `internal/executor/ssh_command_builder.go` | 新建：`BuildSSHCommand` + `buildShellCmd` + `shellQuote` + `fileExists` 闭包 + `resolveSSHBinary` |
| `internal/executor/ssh_command_builder_test.go` | 新建：10 个表驱动测试 |
| `internal/shortcuts/terminal.go` | 改造：删除 `buildRemoteArgs` / `buildShellCmd` / `shellQuote` / `fileExists`；`OpenRemoteDirShortcut` 改调 `executor.BuildSSHCommand` |
| `internal/shortcuts/terminal_remote_test.go` | 改造：测试改为调 `executor.BuildSSHCommand` 或保留为本地集成测试 |
| `internal/config/config.go` | 新增 `SSHCompatAlgorithms` 类型；`SSHKeyConfig` 升级为 `SSHConfig`；`DefaultConfig()` 加 `CompatAlgorithms` 默认值 |
| `config.template.conf` | 新增 `ssh.compat_algorithms` 节段 |

---

## Task 1: 加 SSHCompatAlgorithms 配置字段

**Files:**
- Modify: `internal/config/config.go:140,180-184,608-694`

- [ ] **Step 1: 新增 SSHCompatAlgorithms 类型**

在 `internal/config/config.go` 第 180 行之前（`SSHKeyConfig` 定义前）插入：

```go
// SSHCompatAlgorithms SSH 兼容算法（按 ssh -o 选项拆成三组）。
// 默认全开老算法（兼容老服务器），用户可在 config.json 设为空对象关闭。
type SSHCompatAlgorithms struct {
	Kex     []string `json:"kex,omitempty"`      // 对应 -o KexAlgorithms=+...
	HostKey []string `json:"host_key,omitempty"` // 对应 -o HostKeyAlgorithms=+...
	Cipher  []string `json:"cipher,omitempty"`   // 对应 -o Ciphers=+...
}
```

- [ ] **Step 2: 把 SSHKeyConfig 升级为 SSHConfig 并加 CompatAlgorithms 字段**

修改第 180-184 行（`SSHKeyConfig` 定义）为：

```go
// SSHConfig SSH 密钥相关全局配置
type SSHConfig struct {
	DefaultKeyPath   string              `json:"default_key_path,omitempty"`
	CompatAlgorithms SSHCompatAlgorithms `json:"compat_algorithms"`
}
```

修改第 140 行 `Config` struct 中的 SSH 字段类型：
- `SSH      SSHKeyConfig   `json:"ssh"`` 改为 `SSH      SSHConfig       `json:"ssh"``

- [ ] **Step 3: 在 DefaultConfig() 加 SSH 默认值**

修改 `DefaultConfig()`（第 598-694 行之间），找到 `Relay: RelayConfig{...}` 之前，插入：

```go
		SSH: SSHConfig{
			CompatAlgorithms: SSHCompatAlgorithms{
				Kex: []string{
					"+diffie-hellman-group1-sha1",
					"+diffie-hellman-group-exchange-sha1",
				},
				HostKey: []string{"+ssh-rsa", "+ssh-dss"},
				Cipher: []string{
					"+3des-cbc", "+aes128-cbc", "+aes192-cbc", "+aes256-cbc",
				},
			},
		},
```

> 注意：保持 `DefaultKeyPath` 留空（让 `ResolveKeyPath` 兜底到 `~/.ssh/xworkbench_id_ed25519`），与现状一致。

- [ ] **Step 4: 验证编译通过**

```bash
go build ./...
```

Expected: 编译通过，无错误。注意 `SSHKeyConfig` 改名后，可能有其他文件引用。`grep -rn "SSHKeyConfig" internal/ cmd/` 检查无遗漏。

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go
git commit -m "refactor(config): SSHKeyConfig 升级为 SSHConfig，新增 CompatAlgorithms 字段"
```

---

## Task 2: 新建 ssh_command_builder.go 骨架

**Files:**
- Create: `internal/executor/ssh_command_builder.go`

- [ ] **Step 1: 创建文件，写入辅助函数**

新建 `internal/executor/ssh_command_builder.go`：

```go
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
```

- [ ] **Step 2: 验证编译通过**

```bash
go build ./...
```

Expected: 编译通过（函数可能未被调用，有 unused 警告也无所谓；下一 task 会接入调用方）。

- [ ] **Step 3: Commit**

```bash
git add internal/executor/ssh_command_builder.go
git commit -m "feat(executor): 新建 ssh_command_builder，BuildSSHCommand 拼装 ssh args"
```

---

## Task 3: 写 BuildSSHCommand 表驱动测试

**Files:**
- Create: `internal/executor/ssh_command_builder_test.go`

- [ ] **Step 1: 新建测试文件**

```go
package executor

import (
	"strings"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
)

// withMockKeyExists 临时替换 sshKeyFileExists，返回 restore 函数。
func withMockKeyExists(exists bool) func() {
	orig := sshKeyFileExists
	sshKeyFileExists = func(path string) bool { return exists }
	return func() { sshKeyFileExists = orig }
}

// setupTestConfig 注入测试用 config.AppConfig，返回 restore。
func setupTestConfig(t *testing.T) func() {
	t.Helper()
	restoreGlobal := config.TestSnapshotAndRestore()
	cfg := config.DefaultConfig()
	cfg.Terminal.Types["wezterm"] = config.TerminalTypeDef{
		Bin:        "wezterm",
		RemoteArgs: []string{"ssh", "-i", "{key_path}", "{user}@{host}", "-t", "--", "sh", "-c", "{shell_cmd}"},
	}
	cfg.Terminal.Types["custombin"] = config.TerminalTypeDef{
		Bin:        "wezterm",
		RemoteArgs: []string{"/opt/custom/ssh", "-i", "{key_path}", "{user}@{host}"},
	}
	cfg.Terminal.Types["noterm"] = config.TerminalTypeDef{
		Bin:        "wezterm",
		RemoteArgs: nil,
	}
	config.Set(cfg)
	return restoreGlobal
}

func TestBuildSSHCommand_StandardWezterm(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(true)()

	dir := &backend.DirShortcut{
		RemoteUser: "root",
		RemoteHost: "192.168.1.150",
		RemotePath: "/home/workspace",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 必须以 ssh 起头且只有一个 ssh
	if args[0] != "ssh" {
		t.Errorf("args[0] = %q, want %q", args[0], "ssh")
	}
	if countOccurrences(args, "ssh") != 1 {
		t.Errorf("expected exactly one ssh, got args=%v", args)
	}

	// 必须包含 -i 和目标
	if !containsSeq(args, []string{"-i"}) || !containsSeq(args, []string{"root@192.168.1.150"}) {
		t.Errorf("missing -i or ssh target, args=%v", args)
	}

	// 必须包含 cd remote_path
	if !containsAny(args, "/home/workspace") {
		t.Errorf("missing remote_path in shell_cmd, args=%v", args)
	}
}

func TestBuildSSHCommand_DedupSSH(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(true)()

	dir := &backend.DirShortcut{
		RemoteUser: "u",
		RemoteHost: "h",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ssh 应该仅出现一次（模板首元素被去重）
	if countOccurrences(args, "ssh") != 1 {
		t.Errorf("expected exactly one 'ssh', got %d in args=%v", countOccurrences(args, "ssh"), args)
	}
}

func TestBuildSSHCommand_CustomBin(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(true)()

	dir := &backend.DirShortcut{
		RemoteUser: "u",
		RemoteHost: "h",
	}
	args, err := BuildSSHCommand(dir, "custombin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args[0] != "/opt/custom/ssh" {
		t.Errorf("args[0] = %q, want %q (custom binary)", args[0], "/opt/custom/ssh")
	}
}

func TestBuildSSHCommand_MissingKeyFile(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(false)()

	dir := &backend.DirShortcut{
		RemoteUser: "u",
		RemoteHost: "h",
		LocalKeyPath: "/tmp/non_existent_key",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 应当移除 -i 段
	if containsSeq(args, []string{"-i"}) {
		t.Errorf("expected -i to be dropped, args=%v", args)
	}
	if containsAny(args, "/tmp/non_existent_key") {
		t.Errorf("expected missing key path to be absent, args=%v", args)
	}
}

func TestBuildSSHCommand_EmptyKeyPath(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(false)()

	dir := &backend.DirShortcut{
		RemoteUser: "u",
		RemoteHost: "h",
		// LocalKeyPath/KeyPath/default_key_path 全空
	}
	// ResolveKeyPath 会兜底到 ~/.ssh/xworkbench_id_ed25519；mock 文件不存在
	_ = dir
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containsSeq(args, []string{"-i"}) {
		t.Errorf("expected -i to be dropped when default key file doesn't exist, args=%v", args)
	}
}

func TestBuildSSHCommand_CompatAlgorithmsEmpty(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(true)()

	// 显式置空 compat_algorithms
	cfg := config.Get()
	cfg.SSH.CompatAlgorithms = config.SSHCompatAlgorithms{}

	dir := &backend.DirShortcut{
		RemoteUser: "u",
		RemoteHost: "h",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, a := range args {
		if strings.HasPrefix(a, "KexAlgorithms=") ||
			strings.HasPrefix(a, "HostKeyAlgorithms=") ||
			strings.HasPrefix(a, "Ciphers=") {
			t.Errorf("compat algorithms empty should not emit -o, but got: %v", args)
		}
	}
	if containsSeq(args, []string{"-o"}) {
		t.Errorf("expected no -o flag at all, args=%v", args)
	}
}

func TestBuildSSHCommand_CompatAlgorithmsPartial(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(true)()

	cfg := config.Get()
	cfg.SSH.CompatAlgorithms = config.SSHCompatAlgorithms{
		Kex: []string{"+diffie-hellman-group1-sha1"},
	}

	dir := &backend.DirShortcut{
		RemoteUser: "u",
		RemoteHost: "h",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 应包含 KexAlgorithms 这一项 -o
	if !containsSeq(args, []string{"-o", "KexAlgorithms=+diffie-hellman-group1-sha1"}) {
		t.Errorf("expected KexAlgorithms -o, args=%v", args)
	}
	// 不应包含 HostKeyAlgorithms 或 Ciphers
	for _, a := range args {
		if strings.HasPrefix(a, "HostKeyAlgorithms=") || strings.HasPrefix(a, "Ciphers=") {
			t.Errorf("unexpected -o present: %v", args)
		}
	}
}

func TestBuildSSHCommand_RemotePathWithSpace(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(true)()

	dir := &backend.DirShortcut{
		RemoteUser: "u",
		RemoteHost: "h",
		RemotePath: "/home/my path",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// shell_cmd 元素应包含带空格路径（被 shellQuote 单引号包裹）
	found := false
	for _, a := range args {
		if strings.Contains(a, "/home/my path") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("remote path with space missing from args=%v", args)
	}
}

func TestBuildSSHCommand_TerminalCmdSet(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(true)()

	dir := &backend.DirShortcut{
		RemoteUser:   "u",
		RemoteHost:   "h",
		RemotePath:   "/x",
		TerminalCmd:  "claude",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// shell_cmd 元素应同时含 cd + claude + exec $SHELL
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "claude") || !strings.Contains(joined, "exec $SHELL") {
		t.Errorf("TerminalCmd not in shell_cmd, joined=%q", joined)
	}
}

func TestBuildSSHCommand_UnsupportedTerminal(t *testing.T) {
	defer setupTestConfig(t)()

	dir := &backend.DirShortcut{
		RemoteUser: "u",
		RemoteHost: "h",
	}
	_, err := BuildSSHCommand(dir, "totally_unknown_terminal_xyz")
	if err == nil {
		t.Errorf("expected error for unsupported terminal, got nil")
	}
}

func TestBuildSSHCommand_NoRemoteArgs(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(true)()

	dir := &backend.DirShortcut{
		RemoteUser: "u",
		RemoteHost: "h",
		RemotePath: "/var/log",
	}
	args, err := BuildSSHCommand(dir, "noterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 兜底分支：应当有 -t -- sh -c {shell_cmd}
	if args[0] != "ssh" {
		t.Errorf("args[0] = %q, want ssh", args[0])
	}
	if !containsSeq(args, []string{"-t", "--", "sh", "-c"}) {
		t.Errorf("expected fallback -t -- sh -c, args=%v", args)
	}
}

// ===== 工具函数 =====

func countOccurrences(args []string, target string) int {
	c := 0
	for _, a := range args {
		if a == target {
			c++
		}
	}
	return c
}

func containsSeq(args []string, seq []string) bool {
	for i := 0; i+len(seq) <= len(args); i++ {
		match := true
		for j, s := range seq {
			if args[i+j] != s {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func containsAny(args []string, substr string) bool {
	for _, a := range args {
		if strings.Contains(a, substr) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 运行测试，验证全部通过**

```bash
go test -v -run TestBuildSSHCommand ./internal/executor/
```

Expected: 11 个测试全 PASS。

- [ ] **Step 3: 若有失败，回到 ssh_command_builder.go 修正**

- [ ] **Step 4: Commit**

```bash
git add internal/executor/ssh_command_builder_test.go
git commit -m "test(executor): BuildSSHCommand 表驱动测试（11 个用例）"
```

---

## Task 4: 改造 shortcuts/terminal.go 调用 BuildSSHCommand

**Files:**
- Modify: `internal/shortcuts/terminal.go:77-201`

- [ ] **Step 1: 删除 buildRemoteArgs（line 73-129）整段**

打开 `internal/shortcuts/terminal.go`，删除 line 73-129（`buildRemoteArgs` 函数 + 注释）。同时删除：
- `fileExists` 闭包（line 131-135）
- `buildShellCmd`（line 137-149）
- `shellQuote`（line 151-158）

这些已迁到 `internal/executor/ssh_command_builder.go`。

- [ ] **Step 2: 改写 openRemoteDirShortcutImpl**

把 line 173-201 的 `openRemoteDirShortcutImpl` 改为：

```go
func openRemoteDirShortcutImpl(ctx context.Context, dir *backend.DirShortcut, termType, binPath string, ensureKeyAuth bool) error {
	if dir.Type != backend.DirShortcutTypeRemote {
		return fmt.Errorf("not a remote shortcut: type=%s", dir.Type)
	}

	// 可选：确保密钥免密已配置（首次使用时）
	if ensureKeyAuth {
		_, err := executor.EnsureKeyAuthAvailable(ctx, dir)
		if err != nil {
			logger.Logger.Warnw("[OpenRemoteDirShortcut] ensure key auth failed, continuing anyway",
				"error", err.Error(), "host", dir.RemoteHost)
		}
	}

	logger.Logger.Infow("[OpenRemoteDirShortcut] opening",
		"termType", termType, "remotePath", dir.RemotePath)

	// 用 BuildSSHCommand 获取完整 args 列表
	args, err := executor.BuildSSHCommand(dir, termType)
	if err != nil {
		return fmt.Errorf("build ssh command: %w", err)
	}

	return execRemoteTerminal(termType, binPath, args)
}
```

- [ ] **Step 3: 验证编译通过**

```bash
go build ./...
```

Expected: 编译通过，无错误。

- [ ] **Step 4: 跑既有测试，验证未破坏**

```bash
go test ./internal/shortcuts/ ./internal/executor/
```

Expected: 既有测试可能失败（terminal_remote_test.go 用了 buildRemoteArgsForTest，下一 task 处理），其它全 PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/shortcuts/terminal.go
git commit -m "refactor(shortcuts): openRemoteDirShortcut 改调 executor.BuildSSHCommand"
```

---

## Task 5: 迁移 terminal_remote_test.go 用例到新 API

**Files:**
- Modify: `internal/shortcuts/terminal_remote_test.go` (或新建 `internal/executor/ssh_command_builder_shortcuts_test.go`)

- [ ] **Step 1: 评估现有测试是否还能编译**

```bash
go test ./internal/shortcuts/ 2>&1 | head -30
```

若报 `buildRemoteArgsForTest undefined` 或 `fileExists undefined`：测试需要迁移。

- [ ] **Step 2: 删除/迁移既有测试**

方案：将 `terminal_remote_test.go` 中调用 `buildRemoteArgsForTest` 的用例**改写**为对 `executor.BuildSSHCommand` 的等价测试（已在新 `ssh_command_builder_test.go` 覆盖）。

精简做法：直接**删除** `internal/shortcuts/terminal_remote_test.go`，因为：
- `TestBuildRemoteArgs_*` 系列已被 `internal/executor/ssh_command_builder_test.go` 的 11 个用例完全覆盖
- `terminal_remote_test.go` 里的 `fileExists` mock / `buildRemoteArgsForTest` 函数都已删除

```bash
rm internal/shortcuts/terminal_remote_test.go
```

- [ ] **Step 3: 跑既有测试验证**

```bash
go test ./internal/shortcuts/ ./internal/executor/
```

Expected: PASS，无 `undefined` 错误。

- [ ] **Step 4: Commit**

```bash
git add internal/shortcuts/terminal_remote_test.go
git commit -m "test(shortcuts): 删除已废弃的 terminal_remote_test.go（用例迁到 ssh_command_builder_test.go）"
```

---

## Task 6: 同步 config.template.conf

**Files:**
- Modify: `config.template.conf:14-26` 附近（ssh 节段 + terminal.types 内 remote_args 保持原状）

- [ ] **Step 1: 找到 ssh 节段**

`grep -n '"ssh"' config.template.conf`

- [ ] **Step 2: 在 ssh 节段内加 compat_algorithms**

若 `config.template.conf` 当前 `ssh` 节段形如：

```jsonc
"ssh": {
    "default_key_path": "~/.ssh/xworkbench_id_ed25519"
}
```

则改为：

```jsonc
"ssh": {
    "default_key_path": "~/.ssh/xworkbench_id_ed25519",
    "compat_algorithms": {
        "kex":      ["+diffie-hellman-group1-sha1", "+diffie-hellman-group-exchange-sha1"],
        "host_key": ["+ssh-rsa", "+ssh-dss"],
        "cipher":   ["+3des-cbc", "+aes128-cbc", "+aes192-cbc", "+aes256-cbc"]
    }
}
```

> `terminal.types.*.remote_args` 模板首元素保持 `"ssh"` 不变，由 builder 自动去重。

- [ ] **Step 3: 验证 JSON 语法**

```bash
python3 -m json.tool config.template.conf > /dev/null && echo OK
```

Expected: 输出 `OK`。

- [ ] **Step 4: Commit**

```bash
git add config.template.conf
git commit -m "docs(config): template 加 ssh.compat_algorithms 默认值"
```

---

## Task 7: 全量验证

**Files:** 无

- [ ] **Step 1: 跑全量测试**

```bash
go test ./...
```

Expected: 全 PASS，特别注意 `./internal/executor/` 和 `./internal/shortcuts/` 两个包。

- [ ] **Step 2: 跑既有 SSH 重构验证测试**

```bash
go test -v ./internal/executor/ -run TestResolveKeyPathPriority
go test -v ./internal/executor/ -run TestBuildSSHConfigFromDirShortcut
```

Expected: PASS（确认 SSH builder 重构未破坏密钥解析路径）。

- [ ] **Step 3: 构建二进制**

```bash
./scripts/build.sh
```

Expected: 编译成功，产出 `bin/xworkbench`。

- [ ] **Step 4: 手动端到端（用户配合）**

启动 server：
```bash
./scripts/run.sh --restart
```

浏览器打开 xworkbench，点"快捷目录"里的远程目录 → "打开终端"。验证：
- wezterm 弹出 ssh 会话
- 远程 `pwd` 等于 `RemotePath`
- 若用户在 config.json 把 `compat_algorithms` 置 `{}`，重启后 ssh 命令不再带 `-o Kex=...`

- [ ] **Step 5: 最终 commit（若有遗漏）**

```bash
git status  # 检查是否有未提交
```

若全部已 commit，跳过。

---

## 验收清单

- [ ] `internal/executor/ssh_command_builder.go` 新建
- [ ] `internal/executor/ssh_command_builder_test.go` 11 个用例全 PASS
- [ ] `internal/shortcuts/terminal.go` 主体简化（删除 80 行）
- [ ] `internal/config/config.go` 新增 `SSHCompatAlgorithms` 类型 + `SSHConfig` 升级
- [ ] `config.template.conf` 加 `ssh.compat_algorithms` 节段
- [ ] 全量 `go test ./...` 通过
- [ ] `./scripts/build.sh` 编译通过
- [ ] 端到端点击"打开远程终端"能正常 ssh 登录

## 风险与回退

| 风险 | 回退方式 |
|---|---|
| Task 4 改造导致 `terminal.go` 编译失败 | 检查 `executor` import 是否完整；删除多余函数后未引用 |
| 测试迁移遗漏导致编译失败 | `grep -rn "buildRemoteArgs\|fileExists\|shellQuote" internal/shortcuts/` 应为空 |
| 老 config 缺 `compat_algorithms` 字段 | `DefaultConfig()` 默认值兜底，mergeConfig 通过 fillDefaults 自动补 |
| 模板首元素 "ssh" 在其他自定义终端模板中不复存在 | builder 已自适应去重（条件 `tpl[0] == "ssh"`） |