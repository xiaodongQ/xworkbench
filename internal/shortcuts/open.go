// Package shortcuts 提供跨平台"打开目录"能力。
package shortcuts

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// OpenDir 用系统资源管理器打开本地目录。
//   macOS:   open <path>
//   Linux:   xdg-open <path>
//   Windows: explorer <path>
func OpenDir(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("path not accessible: %w", err)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("explorer", path)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start() // 异步，不等退出
}
