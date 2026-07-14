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
		RemoteUser:  "root",
		RemoteHost:  "192.168.1.150",
		RemotePath:  "/home/workspace",
		AuthMethod:  "key",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if args[0] != "ssh" {
		t.Errorf("args[0] = %q, want %q", args[0], "ssh")
	}
	if countOccurrences(args, "ssh") != 1 {
		t.Errorf("expected exactly one ssh, got args=%v", args)
	}

	if !containsSeq(args, []string{"-i"}) || !containsSeq(args, []string{"root@192.168.1.150"}) {
		t.Errorf("missing -i or ssh target, args=%v", args)
	}

	if !containsAny(args, "/home/workspace") {
		t.Errorf("missing remote_path in shell_cmd, args=%v", args)
	}
}

func TestBuildSSHCommand_DedupSSH(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(true)()

	dir := &backend.DirShortcut{
		RemoteUser:  "u",
		RemoteHost:  "h",
		AuthMethod:  "key",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if countOccurrences(args, "ssh") != 1 {
		t.Errorf("expected exactly one 'ssh', got %d in args=%v", countOccurrences(args, "ssh"), args)
	}
}

func TestBuildSSHCommand_CustomBin(t *testing.T) {
	defer setupTestConfig(t)()
	defer withMockKeyExists(true)()

	dir := &backend.DirShortcut{
		RemoteUser:  "u",
		RemoteHost:  "h",
		AuthMethod:  "key",
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
		RemoteUser:    "u",
		RemoteHost:    "h",
		LocalKeyPath:  "/tmp/non_existent_key",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	}
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

	cfg := config.Get()
	cfg.SSH.CompatAlgorithms = config.SSHCompatAlgorithms{}

	dir := &backend.DirShortcut{
		RemoteUser:  "u",
		RemoteHost:  "h",
		AuthMethod:  "key",
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
		RemoteUser:           "u",
		RemoteHost:           "h",
		UseLegacyAlgorithms:  true,
		AuthMethod:           "key",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsSeq(args, []string{"-o", "KexAlgorithms=+diffie-hellman-group1-sha1"}) {
		t.Errorf("expected KexAlgorithms -o, args=%v", args)
	}
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
		RemoteUser:  "u",
		RemoteHost:  "h",
		RemotePath:  "/home/my path",
		AuthMethod:  "key",
	}
	args, err := BuildSSHCommand(dir, "wezterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
		RemoteUser:  "u",
		RemoteHost:  "h",
		RemotePath:  "/var/log",
		AuthMethod:  "key",
	}
	args, err := BuildSSHCommand(dir, "noterm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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