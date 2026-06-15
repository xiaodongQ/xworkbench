// Package runner 提供 AI CLI 命令构造（claude / cbc / shell）。
package runner

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// BuildCommand 根据类型构造命令列表。
//
// 返回的 cleanup 闭包用于释放命令创建过程中产生的资源（当前只有 shell
// 类型会产生临时脚本文件）。非 shell 类型 cleanup 为 nil，调用方可
// 无脑 defer 调一次。返回的 error 仅指构造错误，命令执行错误由 executor
// 在 wait 时报。
//
// 命令形式：
//
//	claude:   claude -p --output-format json [--model <m>] [--session-id <sid>] "<prompt>"
//	          输出为单次 JSON（含 num_turns / result / is_error 等元数据，便于 evaluator 判定真伪）
//	cbc:      cbc -p [--model <m>] "<prompt>"   （PATH 中无 cbc 时回落到 codebuddy）
//	shell:    sh <tmpfile.sh> / powershell -File <tmpfile.ps1>
//
// shell 类型不再用 `sh -c "<prompt>"` 形式 — 那样会让 prompt 中含的
// `;` / `&` / `|` / `$()` 等被 shell 二次解析，等于 shell 注入。
// 改用临时脚本文件（一次写入，文件名安全），执行解释器直接喂文件。
func BuildCommand(typ, model, sessionID, prompt string, opts ...func(*buildOpts)) (cmd []string, cleanup func(), err error) {
	slog.Debug("runner: BuildCommand",
		slog.String("type", typ),
		slog.String("model", model),
		slog.Int("prompt_chars", len(prompt)),
	)
	o := &buildOpts{
		allowedTools: []string{"Bash", "Write", "Edit", "Read", "Grep"},
	}
	for _, opt := range opts {
		opt(o)
	}
	switch typ {
	case "claude":
		cmd := []string{"claude", "-p", "--output-format", "json"}
		if model != "" {
			cmd = append(cmd, "--model", model)
		}
		if sessionID != "" {
			cmd = append(cmd, "--session-id", sessionID)
		}
		if len(o.allowedTools) > 0 {
			cmd = append(cmd, "--allowedTools", strings.Join(o.allowedTools, ","))
		}
		finalPrompt := prompt
		if o.actionReport {
			finalPrompt = prompt + ActionReportSuffix
		}
		cmd = append(cmd, finalPrompt)
		return cmd, nil, nil
	case "cbc", "codebuddy":
		bin := "cbc"
		if _, err := exec.LookPath("cbc"); err != nil {
			if _, err2 := exec.LookPath("codebuddy"); err2 == nil {
				bin = "codebuddy"
			} else {
				return nil, nil, errors.New("neither cbc nor codebuddy found in PATH")
			}
		}
		cmd := []string{bin, "-p"}
		if model != "" {
			cmd = append(cmd, "--model", model)
		}
		finalPrompt := prompt
		if o.actionReport {
			finalPrompt = prompt + ActionReportSuffix
		}
		cmd = append(cmd, finalPrompt)
		return cmd, nil, nil
	case "shell":
		return shellRunCommand(prompt)
	default:
		return nil, nil, fmt.Errorf("unknown command_type: %q", typ)
	}
}

// shellRunCommand 把 prompt 写到临时文件并返回 sh/powershell 直接执行该文件的命令。
// 返回的 cleanup 删除该临时文件，调用方应 defer cleanup()。
func shellRunCommand(prompt string) ([]string, func(), error) {
	var name, interp string
	if runtime.GOOS == "windows" {
		// .ps1 后缀让 PowerShell 走文件解析而不是 -Command 字符串解析。
		name = "sf-shell-*.ps1"
		interp = "powershell.exe"
	} else {
		name = "sf-shell-*.sh"
		interp = "sh"
	}
	f, err := os.CreateTemp("", name)
	if err != nil {
		return nil, nil, fmt.Errorf("create temp script: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(prompt); err != nil {
		f.Close()
		_ = os.Remove(path)
		return nil, nil, fmt.Errorf("write temp script: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, nil, fmt.Errorf("close temp script: %w", err)
	}
	var cmd []string
	if runtime.GOOS == "windows" {
		// -File 走文件路径，不会对 prompt 文本做命令行再解析。
		cmd = []string{interp, "-NoProfile", "-NonInteractive", "-File", path}
	} else {
		cmd = []string{interp, path}
	}
	cleanup := func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			slog.Warn("runner: remove temp script", slog.String("path", path), slog.String("err", err.Error()))
		}
	}
	return cmd, cleanup, nil
}

func CmdString(cmd []string) string { return strings.Join(cmd, " ") }

// ActionReportSuffix 追加到 AI 任务执行 prompt 末尾，要求 AI 自报动作清单，
// 便于后续 evaluator 交叉验证"嘴上说做了 vs 实际执行了"。
// shell 类型不适用。
const ActionReportSuffix = `

## 任务完成后必须输出"动作清单"（便于自动评估）
请严格按以下 Markdown 格式输出，**必须用真实可执行命令，不允许用 ` + "`...`" + ` 占位符**：

## 动作清单
- 命令: <实际执行的命令，完整可复制>
- 退出码: <命令退出码，无命令填 N/A>
- 工具调用: <Bash / Read / Write / Edit / 其他 / 无>
- 验证步骤: <如何确认结果正确，无验证填 N/A>
`

// WithActionReport 返回一个选项，启用动作清单自报后缀（仅对 claude/cbc 生效）。
func WithActionReport() func(*buildOpts) { return func(o *buildOpts) { o.actionReport = true } }

// WithAllowedTools 返回一个选项，设置允许的工具列表（仅对 claude 生效）。
func WithAllowedTools(tools ...string) func(*buildOpts) {
	return func(o *buildOpts) {
		o.allowedTools = tools
	}
}

type buildOpts struct {
	actionReport bool
	allowedTools []string
}
