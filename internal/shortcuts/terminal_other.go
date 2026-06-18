//go:build !windows

package shortcuts

import (
	"fmt"
	"os/exec"
	"strings"
)

// openTerminalCmd 在非 Windows 平台用 cmd /C start（仅 Windows 解释器）
// 在 Linux/macOS 上不需要这层，直接 exec.Command(bin, args...) 即可。
// 这里保留 dir 参数的语义：仅 Windows 下通过 rawCmd 把 dir 传进去。
func openTerminalCmd(bin string, args []string, dir string) *exec.Cmd {
	_ = dir
	_ = fmt.Sprintf
	_ = strings.Join
	return exec.Command(bin, args...)
}
