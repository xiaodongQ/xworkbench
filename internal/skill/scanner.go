package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ToolsDir 是 skill 插件的根目录，由 Init 设置。
var ToolsDir = ""

// registry 存储启动时扫描到的所有 skill，GetAll/GetByName 依赖此变量。
// 必须在 server 启动前通过 Init() 填充。
var registry []*Skill

// Init 扫描 ToolsDir 并将结果存入 registry。应在 server 启动时调用一次。
func Init(toolsDir string) error {
	ToolsDir = toolsDir
	regs, err := ScanToolsDir(toolsDir)
	if err != nil {
		return err
	}
	registry = regs
	return nil
}

// Reload 重新扫描 toolsDir 并刷新 registry。用于 skill 动态创建后热更新。
func Reload() error {
	if ToolsDir == "" {
		return fmt.Errorf("ToolsDir not set")
	}
	regs, err := ScanToolsDir(ToolsDir)
	if err != nil {
		return err
	}
	registry = regs
	return nil
}

// GetAll 返回所有已注册的 skill。
func GetAll() []*Skill {
	return registry
}

// GetByName 根据名字查找 skill。
func GetByName(name string) *Skill {
	for _, s := range registry {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// GetPublic 返回所有对外可见的 skill（排除 xw_internal=true 的内部工具）。
func GetPublic() []*Skill {
	var out []*Skill
	for _, s := range registry {
		if !s.XWInternal {
			out = append(out, s)
		}
	}
	return out
}

// ScanToolsDir 扫描 toolsDir 目录，返回所有有效的 Skill。
// 只收录包含 SKILL.md 且 name 不为空的有效 skill。
func ScanToolsDir(toolsDir string) ([]*Skill, error) {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return nil, fmt.Errorf("ReadDir(%s): %w", toolsDir, err)
	}

	var skills []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name()[0] == '.' {
			continue // 跳过隐藏目录
		}

		skillFile := filepath.Join(toolsDir, entry.Name(), "SKILL.md")
		skill, err := ParseSkillMeta(skillFile)
		if err != nil {
			// 跳过无效 skill（文件不存在或格式错误）
			continue
		}
		if skill.Name == "" || skill.XWCommand == "" {
			// 缺少必要字段，跳过
			continue
		}

		skills = append(skills, skill)
	}

	// 按名字排序，保证顺序确定
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills, nil
}
