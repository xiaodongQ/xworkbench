package task

import (
	"fmt"
	"strings"

	"github.com/xiaodongQ/xworkbench/internal/backend"
)

// BuildTaskPrompt 用 task + experience 信息构建 Rich AI prompt。
// 注入: title / description / resources / acceptance / priority / experience 内容。
// 多经验支持：传 []*Experience 切片，每条经验单独一段（带 index）。
// 单经验时退化为旧行为（无 index 前缀），保持 prompt diff 最小。
// 0 个 experience = 跳过经验库段落（与旧版一致）。
func BuildTaskPrompt(t *backend.Task, exps ...*backend.Experience) string {
	var b strings.Builder
	b.WriteString("# 任务背景\n")
	b.WriteString(fmt.Sprintf("## 任务标题\n%s\n", t.Title))
	if t.Description != "" {
		b.WriteString(fmt.Sprintf("## 任务描述\n%s\n", t.Description))
	}
	if t.Priority > 0 {
		b.WriteString(fmt.Sprintf("## 优先级\n%d（1 低 5 高）\n", t.Priority))
	}
	if t.Resources != "" {
		b.WriteString(fmt.Sprintf("## 资源/文档\n%s\n", t.Resources))
	}
	if t.Acceptance != "" {
		b.WriteString(fmt.Sprintf("## 验收标准\n%s\n", t.Acceptance))
	}
	if len(exps) > 0 {
		if len(exps) == 1 {
			// 单经验：保持旧行为（无 index 前缀）
			appendExpBlock(&b, "", exps[0])
		} else {
			// 多经验：每段加 ## 经验 N（模块: ...）。nil 元素不计入 index。
			b.WriteString("# 相关经验库\n")
			total := 0
			for _, exp := range exps {
				if exp != nil { total++ }
			}
			idx := 0
			for _, exp := range exps {
				if exp == nil { continue }
				idx++
				appendExpBlock(&b, fmt.Sprintf("（%d/%d，模块: %s）", idx, total, exp.Module), exp)
			}
		}
	}
	return b.String()
}

func appendExpBlock(b *strings.Builder, suffix string, exp *backend.Experience) {
	if exp.Module != "" {
		b.WriteString(fmt.Sprintf("## 分类%s\n%s\n", suffix, exp.Module))
	}
	if exp.Scene != "" {
		b.WriteString(fmt.Sprintf("## 场景%s\n%s\n", suffix, exp.Scene))
	}
	if exp.Keywords != "" {
		b.WriteString(fmt.Sprintf("## 关键词%s\n%s\n", suffix, exp.Keywords))
	}
	if exp.Details != "" {
		b.WriteString(fmt.Sprintf("## 详细内容%s\n%s\n", suffix, exp.Details))
	}
}
// BuildTaskPromptShort 简化版 prompt，只包含：描述 + 验收标准 + 动作清单格式要求。
// 用于手动任务的 AI 执行，保持命令行简洁。
func BuildTaskPromptShort(t *backend.Task) string {
	var b strings.Builder
	if t.Description != "" {
		b.WriteString(t.Description)
		b.WriteString("\n\n")
	}
	if t.Acceptance != "" {
		b.WriteString("## 验收标准\n")
		b.WriteString(t.Acceptance)
		b.WriteString("\n\n")
	}
	// 动作清单格式要求
	b.WriteString(ActionReportFormat)
	return b.String()
}

// ActionReportFormat 动作清单输出格式要求（供手动任务和评估用）。
const ActionReportFormat = `## 任务完成后必须输出"动作清单"（便于自动评估）
请严格按以下 Markdown 格式输出，**必须用真实可执行命令，不允许用 ... 占位符**：

## 动作清单
- 命令: <实际执行的命令，完整可复制>
- 退出码: <命令退出码，无命令填 N/A>
- 工具调用: <Bash / Read / Write / Edit / 其他 / 无>
- 验证步骤: <如何确认结果正确，无验证填 N/A>
`
