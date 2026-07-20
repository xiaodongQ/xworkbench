//go:build windows

package shortcuts

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// openTerminalCmd 在 Windows 下用 syscall.SysProcAttr.CmdLine 构造
// *exec.Cmd，精确控制命令行（Go 不会自动转义引号），避免 start 命令
// 把 title 误识别为命令。
func openTerminalCmd(bin string, args []string, dir string) *exec.Cmd {
	parts := []string{bin}
	parts = append(parts, args...)
	rawCmd := fmt.Sprintf(`cmd /C start "" /D "%s" /F %s`, dir, strings.Join(parts, " "))
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: rawCmd,
	}
	return cmd
}
