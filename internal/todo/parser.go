// Package todo 解析和写回 markdown 形式的 todo 列表。
package todo

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Item 单个待办项。
type Item struct {
	LineNo int    `json:"line_no"` // 1-based
	Indent string `json:"indent"`
	Done   bool   `json:"done"`
	Text   string `json:"text"`
}

var itemRe = regexp.MustCompile(`^(\s*)-\s+\[( |x|X)\]\s+(.+)$`)

// Parse 从 markdown 文本中解析 todo 项。
func Parse(content string) []Item {
	var items []Item
	for i, line := range strings.Split(content, "\n") {
		m := itemRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		items = append(items, Item{
			LineNo: i + 1,
			Indent: m[1],
			Done:   m[2] != " ",
			Text:   m[3],
		})
	}
	return items
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
