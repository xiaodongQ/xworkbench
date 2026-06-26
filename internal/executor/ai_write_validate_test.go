package executor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateAIWrites_RemovesUntrackedInInternal 模拟 AI 写到 internal/
// 路径下（沙盒被绕过的场景），验证 ValidateAIWrites 自动清理。
func TestValidateAIWrites_RemovesUntrackedInInternal(t *testing.T) {
	// 用 t.TempDir() 起一个真 git repo（ValidateAIWrites 依赖 git status）
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@test.com")
	runGit(t, repo, "config", "user.name", "Test")
	// 提交一个空 initial commit，让 git status 工作正常
	runGit(t, repo, "commit", "--allow-empty", "-m", "init", "-q")

	// 模拟 AI 写到 internal/sum/twosum.go
	sumDir := filepath.Join(repo, "internal", "sum")
	if err := os.MkdirAll(sumDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	bad := filepath.Join(sumDir, "twosum.go")
	if err := os.WriteFile(bad, []byte("package sum\n// AI 漏网之鱼\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// 在 repo 内调 ValidateAIWrites
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	ValidateAIWrites()

	// 文件应已被删
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed by ValidateAIWrites, but it still exists", bad)
	}
}

// TestValidateAIWrites_RevertsModifiedFile 模拟 AI 改动了 tracked 文件
// （add a line to internal/foo.go），验证 git checkout 把它恢复。
func TestValidateAIWrites_RevertsModifiedFile(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@test.com")
	runGit(t, repo, "config", "user.name", "Test")

	// 提交一个 tracked 文件
	target := filepath.Join(repo, "internal", "foo.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("package foo\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "init", "-q")

	// AI 改了这个 tracked 文件
	if err := os.WriteFile(target, []byte("package foo\n// AI 漏改\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// 跑 ValidateAIWrites
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	ValidateAIWrites()

	// 文件应被还原
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "package foo\n" {
		t.Errorf("expected file restored, got %q", string(got))
	}
}

// TestValidateAIWrites_NoOpOutsideProtected 写到 data/ 路径（合理）不
// 应被清理——这是 post-write validation 的边界条件。
func TestValidateAIWrites_NoOpOutsideProtected(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@test.com")
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "commit", "--allow-empty", "-m", "init", "-q")

	// 写到 data/ 下（合理：AI 沙盒的合法 destination）
	dataDir := filepath.Join(repo, "data", "ai-sandbox")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legit := filepath.Join(dataDir, "ok.go")
	if err := os.WriteFile(legit, []byte("// ok\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldCwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	ValidateAIWrites()

	if _, err := os.Stat(legit); err != nil {
		t.Errorf("legit file in data/ should NOT be removed, got err: %v", err)
	}
}

// TestIsAIProtectedPath 单元测试 isAIProtectedPath 的路径匹配。
func TestIsAIProtectedPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"cmd/server/main.go", true},
		{"internal/foo/bar.go", true},
		{"openspec/specs/x.md", true},
		{"data/ai-sandbox/ok.go", false},
		{"data/xworkbench.db", false},
		{"config.json", false},
		{".claude/settings.json", false},
		{"README.md", false},
		{"internal", false}, // 边界：纯目录名不带斜杠的不算（git status 永远带 file 后缀）
	}
	for _, c := range cases {
		if got := isAIProtectedPath(c.path); got != c.want {
			t.Errorf("isAIProtectedPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
