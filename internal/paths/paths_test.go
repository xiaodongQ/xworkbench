package paths

import (
	"path/filepath"
	"testing"
)

func TestResolveDBPath_DBPathOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "explicit.db")
	t.Setenv("DB_PATH", want)
	if got := ResolveDBPath(); got != want {
		t.Errorf("ResolveDBPath() = %q, want %q", got, want)
	}
}

func TestResolveDBPath_Default(t *testing.T) {
	// DB_PATH 未设置时，使用 cwd 下的 data/xworkbench.db
	t.Setenv("DB_PATH", "")
	got := ResolveDBPath()
	want := "data/xworkbench.db"
	if got != want {
		t.Errorf("ResolveDBPath() = %q, want %q", got, want)
	}
}

func TestAISandboxDir_Override(t *testing.T) {
	// AI_SANDBOX_DIR 显式覆盖（运维/测试常用）。
	want := filepath.Join(t.TempDir(), "sandbox")
	t.Setenv("AI_SANDBOX_DIR", want)
	if got := AISandboxDir(); got != want {
		t.Errorf("AISandboxDir() = %q, want %q", got, want)
	}
}

func TestAISandboxDir_Default(t *testing.T) {
	// AI_SANDBOX_DIR 未设置时，使用 cwd 下的 data/ai-sandbox。
	// 跟 ResolveDBPath 行为一致，假设 cwd 是项目根。
	t.Setenv("AI_SANDBOX_DIR", "")
	got := AISandboxDir()
	want := "data/ai-sandbox"
	if got != want {
		t.Errorf("AISandboxDir() = %q, want %q", got, want)
	}
}
