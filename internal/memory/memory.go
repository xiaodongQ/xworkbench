package memory

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// 硬上限 20KB，超过此大小拒绝写入
	HardLimit = 20 * 1024
	// 软上限 18KB，接近时给出警告
	SoftLimit = 18 * 1024
)

var (
	ErrFileTooLarge = errors.New("memory.md exceeds 20KB limit")
)

// Memory 条目
type Entry struct {
	Date     string // "YYYY-MM-DD"
	Text     string // 条目正文（不含 [date] 前缀）
	Category string // 分类名（如 "用户 & 环境"）
	Line     int    // 在文件中的行号
}

// Store 管理 data/memory.md
type Store struct {
	Path    string
	entries []Entry // 内存缓存（非持久化）
}

// New 创建 Store，路径不存在时返回错误。
func New(path string) *Store {
	return &Store{Path: path}
}

// Load 加载已有 memory.md，返回 Store + 内容字符串。
func Load(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Size 返回当前文件大小（字节）。
func (s *Store) Size() int {
	info, err := os.Stat(s.Path)
	if err != nil {
		return 0
	}
	return int(info.Size())
}

// parseEntries 从文件内容解析出所有条目。
func parseEntries(content string) []Entry {
	var entries []Entry
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 匹配 [YYYY-MM-DD] 内容
		m := regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2})\]\s*(.+)$`).FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// 找所属分类：从下往上找 ## 标题
		category := "默认"
		for j := i - 1; j >= 0; j-- {
			hdr := strings.TrimSpace(lines[j])
			if strings.HasPrefix(hdr, "## ") {
				category = strings.TrimPrefix(hdr, "## ")
				break
			}
		}
		entries = append(entries, Entry{
			Date:     m[1],
			Text:     m[2],
			Category: category,
			Line:     i + 1,
		})
	}
	return entries
}

// Add 追加一条记忆条目。
// category 如果不存在会自动创建分类 section。
// 返回操作结果描述。
func (s *Store) Add(text, category string) (string, error) {
	if s.Size() > HardLimit {
		return "", ErrFileTooLarge
	}

	content, err := os.ReadFile(s.Path)
	if err != nil {
		// 文件不存在就创建
		if os.IsNotExist(err) {
			content = []byte{}
		} else {
			return "", err
		}
	}
	str := string(content)

	entries := parseEntries(str)
	date := time.Now().Format("2006-01-02")
	entryLine := "[" + date + "] " + text

	// 精确去重
	for _, e := range entries {
		if e.Text == text && e.Category == category {
			return "⚠️ 重复条目，已跳过（同一分类下相同内容）", nil
		}
	}

	// 软上限警告
	warn := ""
	if s.Size() > SoftLimit {
		warn = "⚠️ 文件接近 20KB 上限，建议运行 memory_prune 整合。"
	}

	// 写文件
	str, err = s.appendEntry(str, entryLine, category)
	if err != nil {
		return "", err
	}

	// 再次检查大小
	if len(str) > HardLimit {
		// 回滚：重新读文件（撤销写入）
		return "", ErrFileTooLarge
	}

	if err := os.WriteFile(s.Path, []byte(str), 0644); err != nil {
		return "", err
	}

	return "✅ 已添加 [" + date + "] " + text + warn, nil
}

// appendEntry 将条目追加到对应分类 section，没有则创建。
func (s *Store) appendEntry(content, entryLine, category string) (string, error) {
	sectionName := "## " + category
	lines := strings.Split(content, "\n")

	// 查找是否有该分类 section
	sectionIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == sectionName {
			sectionIdx = i
			break
		}
	}

	var buf bytes.Buffer
	if sectionIdx >= 0 {
		// 追加到该 section 下方的第一个空行或下一个 ## 之前
		insertIdx := sectionIdx + 1
		for insertIdx < len(lines) {
			l := strings.TrimSpace(lines[insertIdx])
			if l == "" {
				insertIdx++
				continue
			}
			if strings.HasPrefix(l, "## ") {
				break
			}
			insertIdx++
		}
		// 如果 section 后没有内容，追加到 section 后面
		if sectionIdx+1 >= len(lines) || strings.TrimSpace(lines[sectionIdx+1]) != "" {
			// 需要在 section 后加空行再插条目
		}
		before := lines[:insertIdx]
		after := lines[insertIdx:]
		buf.WriteString(strings.Join(before, "\n"))
		if len(before) > 0 && strings.TrimSpace(before[len(before)-1]) != "" {
			buf.WriteString("\n")
		}
		buf.WriteString("\n" + entryLine + "\n")
		buf.WriteString(strings.Join(after, "\n"))
	} else {
		// 无该分类，检查是否有 # Memory 标题
		hasMemoryHeader := strings.Contains(content, "# Memory")
		if !hasMemoryHeader {
			buf.WriteString("# Memory\n\n")
		}
		// 追加新 section
		if !hasMemoryHeader || !strings.HasSuffix(strings.TrimRight(content, "\n"), "\n") {
			buf.WriteString(content)
			if !strings.HasSuffix(content, "\n") {
				buf.WriteString("\n")
			}
		} else {
			buf.WriteString(strings.TrimRight(content, "\n"))
		}
		buf.WriteString("\n" + sectionName + "\n" + entryLine + "\n")
	}

	return buf.String(), nil
}

// List 返回当前所有条目。
func (s *Store) List() []Entry {
	content, err := os.ReadFile(s.Path)
	if err != nil {
		return nil
	}
	return parseEntries(string(content))
}

// Prune 整合去重：移除精确重复条目。
func (s *Store) Prune() (string, error) {
	content, err := os.ReadFile(s.Path)
	if err != nil {
		return "", err
	}
	str := string(content)
	entries := parseEntries(str)

	// 精确去重：同分类同日期同内容只保留最新（最后出现的）
	seen := make(map[string]bool)
	var toRemove []int
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		key := e.Category + "||" + e.Date + "||" + e.Text
		if seen[key] {
			toRemove = append(toRemove, e.Line)
		} else {
			seen[key] = true
		}
	}

	if len(toRemove) == 0 {
		return "✅ 无需整合（无重复条目）", nil
	}

	// 从文件移除重复行
	lines := strings.Split(str, "\n")
	removed := 0
	var kept []string
	for i, line := range lines {
		lineNo := i + 1
		isDuplicate := false
		for _, r := range toRemove {
			if r == lineNo {
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			kept = append(kept, line)
		} else {
			removed++
		}
	}

	newContent := strings.Join(kept, "\n")
	if err := os.WriteFile(s.Path, []byte(newContent), 0644); err != nil {
		return "", err
	}

	return "✅ 整合完成，移除 " + itoa(removed) + " 条重复条目", nil
}

// ContentForSystemPrompt 返回适合注入 system prompt 的记忆内容。
func (s *Store) ContentForSystemPrompt() string {
	content, err := os.ReadFile(s.Path)
	if err != nil || len(content) == 0 {
		return ""
	}
	str := strings.TrimSpace(string(content))
	if str == "" || str == "# Memory" {
		return ""
	}
	return "\n\n## 记忆\n" + str + "\n"
}

// EnsureDir 确保 data 目录存在。
func EnsureDir(dataDir string) error {
	return os.MkdirAll(dataDir, 0755)
}

// MemoryPath 返回默认 memory.md 路径（dataDir/memory.md）。
func MemoryPath(dataDir string) string {
	return filepath.Join(dataDir, "memory.md")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
