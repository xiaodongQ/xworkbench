package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

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
