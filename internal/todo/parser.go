// Package todo 解析和写回 markdown 形式的 todo 列表。
package todo

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// Item 单个待办项。
type Item struct {
	LineNo   int      `json:"line_no"` // 1-based
	Indent   string   `json:"indent"`
	Done     bool     `json:"done"`
	Text     string   `json:"text"`
	DueDate  string   `json:"due_date,omitempty"` // "YYYY-MM-DD"，""=未设
	Tags     []string `json:"tags,omitempty"`
	Note     string   `json:"note,omitempty"`
	Children []*Item  `json:"children,omitempty"` // 前端用，不写入文件
}

var (
	itemRe = regexp.MustCompile(`^(\s*)-\s+\[( |x|X)\]\s+(.+)$`)
	dueRe  = regexp.MustCompile(`\s+due:(\d{4}-\d{2}-\d{2}|\d{2}-\d{2})\b`)
	tagsRe = regexp.MustCompile(`\s+tags:([\w,]+)`)
)

// Parse 从 markdown 文本中解析 todo 项。
func Parse(content string) []Item {
	var items []Item
	for i, line := range strings.Split(content, "\n") {
		m := itemRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text, dueDate, tags := parseMetadata(m[3])
		items = append(items, Item{
			LineNo:  i + 1,
			Indent:  m[1],
			Done:    m[2] != " ",
			Text:    text,
			DueDate: dueDate,
			Tags:    tags,
		})
	}
	return items
}

// parseMetadata 从文本末尾提取 due: 和 tags: 元数据，返回 (cleanText, dueDate, tags)。
func parseMetadata(text string) (string, string, []string) {
	var dueDate string
	var tags []string

	// 提取 due:YYYY-MM-DD 或 due:MM-DD
	if dueRe.MatchString(text) {
		match := dueRe.FindStringSubmatch(text)
		dueDate = match[1]
		// 如果是 MM-DD 格式，补上当前年份
		if len(dueDate) == 5 {
			composed := fmt.Sprintf("%d-%s", time.Now().Year(), dueDate)
			if _, err := time.Parse("2006-01-02", composed); err == nil {
				dueDate = composed
			} else {
				// 非法日期(如非闰年的 02-29)留空，不存储无效日期
				dueDate = ""
			}
		}
		// 从原文中移除 due:... 部分
		text = dueRe.ReplaceAllString(text, "")
	}

	// 提取 tags:tag1,tag2
	if tagsRe.MatchString(text) {
		match := tagsRe.FindStringSubmatch(text)
		for _, t := range strings.Split(match[1], ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
		text = tagsRe.ReplaceAllString(text, "")
	}

	return strings.TrimSpace(text), dueDate, tags
}

// ReadAndParse 读文件 + 解析。文件不存在返回 (nil, nil)。
func ReadAndParse(path string) ([]Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return Parse(string(data)), nil
}

// ToggleAndWrite 把 items 的 Done 状态写回文件（先 .bak 再 atomic rename）。
func ToggleAndWrite(path string, items []Item) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for _, it := range items {
		if it.LineNo < 1 || it.LineNo > len(lines) {
			return fmt.Errorf("line_no %d out of range", it.LineNo)
		}
		marker := " "
		if it.Done {
			marker = "x"
		}
		lines[it.LineNo-1] = it.Indent + "- [" + marker + "] " + it.Text
	}
	return atomicWrite(path, strings.Join(lines, "\n"))
}

// AddAndWrite 在文件末尾追加一行 `- [ ] text`（先 .bak 再 atomic rename）。
// 文件不存在时直接创建。
func AddAndWrite(path, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("text is empty")
	}
	var content string
	if data, err := os.ReadFile(path); err == nil {
		content = string(data)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	content += "- [ ] " + text + "\n"
	return atomicWrite(path, content)
}

// DeleteAndWrite 删除指定行号的项（按 1-based line_no）。
func DeleteAndWrite(path string, lineNo int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	if lineNo < 1 || lineNo > len(lines) {
		return fmt.Errorf("line_no %d out of range", lineNo)
	}
	// 跳过 lineNo-1 那行，其他保持
	kept := make([]string, 0, len(lines)-1)
	kept = append(kept, lines[:lineNo-1]...)
	kept = append(kept, lines[lineNo:]...)
	return atomicWrite(path, strings.Join(kept, "\n"))
}

func atomicWrite(path, content string) error {
	bak := path + ".bak"
	if err := os.WriteFile(bak, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(bak, path)
}
