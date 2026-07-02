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

// OutputDirHintTpl 输出目录约定段模板，导出供外部 caller 拼到 prompt 末尾。
// 一个 `%s` 占位符表示输出目录绝对路径。
// 通过 BuildTaskPromptWithOutput 拼到 BuildTaskPrompt 末尾；
// handleExecutionContinue / runLoopBackground 等用自己的 prompt 时也用本模板。
const OutputDirHintTpl = `

## 输出目录约定
若存在文件输出，全部写到 ` + "`%s`" + ` 目录（CWD 已设为该目录）：
- 不要修改源码树（internal/、cmd/、go.mod 等）；只在该目录内读写
- 命名前缀建议：本次 task 简述 + 文件用途，如 ` + "`feat-foo_test.go`" + `
- 任务完成后可列出该目录内容便于评估
`

// BuildTaskPromptWithOutput 在 BuildTaskPrompt 基础上追加「输出目录约定」段。
// 用于所有由 claude/cbc 调起、需要 AI 写文件的 task 入口（手动、继续对话、run-loop）。
//
// outputDir 为空时退化为 BuildTaskPrompt（向后兼容：evaluator、learn 等元任务
// 不需要输出约定）。
//
// outputDir 通常传 paths.AITaskDir(taskID)（每个 task 独立子目录，多任务并发写互不干扰）。
func BuildTaskPromptWithOutput(t *backend.Task, outputDir string, exps ...*backend.Experience) string {
	base := BuildTaskPrompt(t, exps...)
	if outputDir == "" {
		return base
	}
	return base + fmt.Sprintf(OutputDirHintTpl, outputDir)
}
