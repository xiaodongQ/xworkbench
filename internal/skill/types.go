package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-wordwrap"
	"gopkg.in/yaml.v3"
)

// Skill 表示一个可执行的 skill 插件。
type Skill struct {
	Name        string            `json:"name"`                  // 唯一标识符
	Description string            `json:"description"`           // AI 决策用描述
	Version     string            `json:"version,omitempty"`     // 版本号
	Author      string            `json:"author,omitempty"`      // 作者

	// xworkbench 扩展字段（xw_ 前缀）
	XWCommand  string            `json:"xw_command,omitempty"`  // 执行命令，如 python3 scripts/check.py
	XWParams   map[string]string `json:"xw_params,omitempty"`   // 参数名 → 描述
	XWOutput   map[string]string `json:"xw_output,omitempty"`   // 输出字段 → 描述
	XWXamples  []Example         `json:"xw_examples,omitempty"` // 示例（拼写保护字段名）
	XWExamples []Example         `json:"-"`                     // 别名，避免 YAML 解析问题

	Dir string `json:"-"` // skill 目录绝对路径
}

// Example 是 xw_examples 中的单个示例。
type Example struct {
	Description string         `json:"description"`
	Params      map[string]any `json:"params"`
}

// SkillMeta 是 YAML frontmatter 解析后的中间结构。
type skillMeta struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Version     string            `yaml:"version"`
	Author      string            `yaml:"author"`
	AllowedTools []string         `yaml:"allowed-tools"`

	// xworkbench 扩展（xw_ 前缀）
	XWCommand  string            `yaml:"xw_command"`
	XWParams   map[string]string `yaml:"xw_params"`
	XWOutput   map[string]string `yaml:"xw_output"`
	XWExamples []Example         `yaml:"xw_examples"`
}

// ParseSkillMeta 解析 skill 目录中的 SKILL.md，返回 Skill 元数据。
// 不执行脚本，只读取 YAML frontmatter。
func ParseSkillMeta(skillFile string) (*Skill, error) {
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, fmt.Errorf("ReadFile: %w", err)
	}

	// 提取 YAML frontmatter（--- ... --- 包裹的内容）
	content := string(data)
	frontmatter := extractFrontmatter(content)
	if frontmatter == "" {
		return nil, fmt.Errorf("no YAML frontmatter found in %s", skillFile)
	}

	var meta skillMeta
	if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
		return nil, fmt.Errorf("YAML parse error: %w", err)
	}

	if meta.Name == "" {
		return nil, fmt.Errorf("name field is required")
	}

	// 处理 xw_examples 字段
	examples := meta.XWExamples

	return &Skill{
		Name:        meta.Name,
		Description: meta.Description,
		Version:     meta.Version,
		Author:      meta.Author,
		XWCommand:   meta.XWCommand,
		XWParams:    meta.XWParams,
		XWOutput:    meta.XWOutput,
		XWExamples:  examples,
		Dir:         filepath.Dir(skillFile),
	}, nil
}

// extractFrontmatter 从 markdown 内容中提取 YAML frontmatter。
// 匹配开头的 --- ... --- 块（支持 Unix 和 Windows 换行）。
func extractFrontmatter(content string) string {
	// 支持 --- 开头（可能前面有 BOM 或空白）
	trimmed := strings.TrimLeft(content, " \t\r\n")
	if !strings.HasPrefix(trimmed, "---") {
		return ""
	}

	// 找第一个 --- 的位置
	idx1 := strings.Index(trimmed, "---")
	if idx1 < 0 {
		return ""
	}

	// 找第二个 ---（从第一个之后开始）
	rest := trimmed[idx1+3:]
	// 跳过可能的换行
	rest = strings.TrimLeft(rest, "\n\r")
	idx2 := strings.Index(rest, "---")
	if idx2 < 0 {
		return ""
	}

	fm := rest[:idx2]
	// 去掉末尾的换行（但保留内容中的空行）
	fm = strings.TrimRight(fm, "\n\r")
	return fm
}

// ToTool 将 Skill 转换为 AI Tools 函数调用格式。
func (s *Skill) ToTool() Tool {
	// 将 xw_params 转换为 JSON Schema properties
	properties := make(map[string]any)
	required := make([]string, 0)

	// 收集必填参数（描述中含"必填"的）
	for name, desc := range s.XWParams {
		properties[name] = map[string]any{
			"type":        "string",
			"description": desc,
		}
		if strings.Contains(desc, "必填") {
			required = append(required, name)
		}
	}

	paramsSchema, _ := json.Marshal(map[string]any{
		"type":       "object",
		"properties": properties,
	})

	var params map[string]any
	json.Unmarshal(paramsSchema, &params)

	return Tool{
		Name:        s.Name,
		Description: s.Description,
		InputSchema: params,
	}
}

// Tool 对应 AI function calling 的函数定义格式。
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ExecuteSkillResult 是 ExecuteSkill 的返回结果。
type ExecuteSkillResult struct {
	Status  string         `json:"status"`  // ok | error
	Output  map[string]any `json:"output"`  // 解析后的 JSON 输出
	RawOut  string         `json:"raw_out"` // 原始 stdout
	RawErr  string         `json:"raw_err"` // 原始 stderr
}

// WrapDescription 将超长 description 截断到指定宽度。
func WrapDescription(desc string, width uint) string {
	if desc == "" {
		return desc
	}
	wrapped := wordwrap.WrapString(desc, width)
	lines := strings.Split(wrapped, "\n")
	if len(lines) > 2 {
		return strings.Join(lines[:2], " ") + "..."
	}
	return wrapped
}
