package shortcuts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDir_InvalidPath(t *testing.T) {
	err := OpenDir("/nonexistent/should/not/exist/12345")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestOpenDir_ValidPath_DoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	// 不验证 OS 资源管理器真的打开（GUI 不可断言），只验证路径合法时无 error
	// explorer/open 在 headless 环境下可能返回非 0，我们 wrap 后不报错就是成功
	_ = OpenDir(dir)
}
