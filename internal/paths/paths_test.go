package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDBPath_DBPathOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "explicit.db")
	t.Setenv("DB_PATH", want)
	if got := ResolveDBPath(); got != want {
		t.Errorf("ResolveDBPath() = %q, want %q", got, want)
	}
}

func TestResolveDBPath_SkillFactoryHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DB_PATH", "")
	t.Setenv("SKILL_FACTORY_HOME", home)
	got := ResolveDBPath()
	want := filepath.Join(home, "data", "skill-factory.db")
	if got != want {
		t.Errorf("ResolveDBPath() = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Dir(got)); err != nil {
		t.Errorf("parent dir not created: %v", err)
	}
}

func TestResolveDBPath_UserConfigDir(t *testing.T) {
	// 走 UserConfigDir 分支的前提：DB_PATH 没设、SKILL_FACTORY_HOME 没设。
	t.Setenv("DB_PATH", "")
	t.Setenv("SKILL_FACTORY_HOME", "")

	// 直接覆盖 HOME（UserConfigDir 的基础）— macOS / Linux / Windows 都尊重 $HOME。
	cfg := t.TempDir()
	t.Setenv("HOME", cfg)
	t.Setenv("USERPROFILE", cfg) // Windows 上 UserConfigDir 读 USERPROFILE

	got := ResolveDBPath()
	// 不强制等于（因为不同 OS 的 UserConfigDir 实现细节不同），
	// 但路径应该被解析到 $HOME 之下或 cwd fallback。
	if !strings.HasPrefix(got, cfg) && got != "data/skill-factory.db" {
		t.Errorf("ResolveDBPath() = %q, expected under $HOME=%s or legacy fallback", got, cfg)
	}
}
