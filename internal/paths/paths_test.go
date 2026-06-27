package paths

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestAITaskRoot_Override(t *testing.T) {
	// $AI_TASK_ROOT 优先于 $AI_SANDBOX_DIR 和默认值
	want := filepath.Join(t.TempDir(), "task-root")
	t.Setenv("AI_TASK_ROOT", want)
	t.Setenv("AI_SANDBOX_DIR", "/should/be/ignored")
	if got := AITaskRoot(); got != want {
		t.Errorf("AITaskRoot() = %q, want %q", got, want)
	}
}

func TestAITaskRoot_FallbackToSandbox(t *testing.T) {
	// $AI_TASK_ROOT 未设时，回退到 $AI_SANDBOX_DIR（兼容老部署）
	want := filepath.Join(t.TempDir(), "legacy-sandbox")
	t.Setenv("AI_TASK_ROOT", "")
	t.Setenv("AI_SANDBOX_DIR", want)
	if got := AITaskRoot(); got != want {
		t.Errorf("AITaskRoot() = %q, want %q (fallback to AI_SANDBOX_DIR)", got, want)
	}
}

func TestAITaskRoot_Default(t *testing.T) {
	// 都没设时用 data/ai-task-dir
	t.Setenv("AI_TASK_ROOT", "")
	t.Setenv("AI_SANDBOX_DIR", "")
	got := AITaskRoot()
	want := "data/ai-task-dir"
	if got != want {
		t.Errorf("AITaskRoot() = %q, want %q", got, want)
	}
}

func TestAITaskDir_AppendsTaskID(t *testing.T) {
	// 验证 AITaskDir(taskID) 在 root/<today>/ 下追加 taskID 子目录
	tmp := t.TempDir()
	t.Setenv("AI_TASK_ROOT", tmp)
	got := AITaskDir("task-123")
	dateDir := time.Now().Format("2006-01-02")
	want := filepath.Join(tmp, dateDir, "task-123")
	if got != want {
		t.Errorf("AITaskDir() = %q, want %q", got, want)
	}
	// 验证目录已创建
	if _, err := os.Stat(want); err != nil {
		t.Errorf("AITaskDir() should mkdir %q: %v", want, err)
	}
}
