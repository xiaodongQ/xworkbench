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
