package executor

import (
	"encoding/json"
	"regexp"
	"strings"
)

// confirmSignals 4 种人工确认信号（中英文都覆盖）。
// 移植自 ai-task-system v2.4 的 cli_executor.py:62-115。
var confirmSignals = []string{
	"?", "[Y/n]", "[是/否]", "[y/n]", "[Yes/No]",
	"是否要", "要不要", "是否需要", "请确认",
	"不确定", "需要更多信息", "请告诉我", "请选择",
	"Press Enter", "按 Enter", "输入选择",
	"Continue?", "Proceed?", "Confirm",
}

// confirmJSONRe 匹配 {"confirm_type": ...} 形式。
var confirmJSONRe = regexp.MustCompile(`\{[^{}]*"confirm_type"[^{}]*\}`)

// NeedsUserInput 检测输出是否需要人工确认。
func NeedsUserInput(output string) bool {
	if output == "" {
		return false
	}
	for _, s := range confirmSignals {
		if strings.Contains(output, s) {
			return true
		}
	}
	return false
}

// ParseConfirmRequest 尝试从输出解析结构化确认请求（JSON 形式）。
func ParseConfirmRequest(output string) map[string]any {
	if output == "" {
		return nil
	}
	match := confirmJSONRe.FindString(output)
	if match == "" {
		return nil
	}
	var m map[string]any
	if json.Unmarshal([]byte(match), &m) == nil {
		if _, ok := m["confirm_type"]; ok {
			return m
		}
	}
	return nil
}
