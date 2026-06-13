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
	b.WriteString("# 用户指令\n")
	b.WriteString(t.Description)
	return b.String()
}

func appendExpBlock(b *strings.Builder, suffix string, exp *backend.Experience) {
	if exp.Module != "" {
		b.WriteString(fmt.Sprintf("## 模块%s\n%s\n", suffix, exp.Module))
	}
	if exp.Scene != "" {
		b.WriteString(fmt.Sprintf("## 场景%s\n%s\n", suffix, exp.Scene))
	}
	if exp.Keywords != "" {
		b.WriteString(fmt.Sprintf("## 关键词%s\n%s\n", suffix, exp.Keywords))
	}
	if exp.ToolUsage != "" {
		b.WriteString(fmt.Sprintf("## 工具用法%s\n%s\n", suffix, exp.ToolUsage))
	}
	if exp.LogSamples != "" {
		b.WriteString(fmt.Sprintf("## 日志样例%s\n%s\n", suffix, exp.LogSamples))
	}
	if exp.CodeSnippets != "" {
		b.WriteString(fmt.Sprintf("## 代码片段%s\n%s\n", suffix, exp.CodeSnippets))
	}
	if exp.LogPaths != "" {
		b.WriteString(fmt.Sprintf("## 日志路径%s\n%s\n", suffix, exp.LogPaths))
	}
}