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
	lines := strings.Split(content, "\n")
	noteMap := parseNotes(lines)

	var items []Item
	for i, line := range lines {
		m := itemRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text, dueDate, tags := parseMetadata(m[3])
		item := Item{
			LineNo:  i + 1,
			Indent:  m[1],
			Done:    m[2] != " ",
			Text:    text,
			DueDate: dueDate,
			Tags:    tags,
		}
		if note, ok := noteMap[i+1]; ok {
			item.Note = note
		}
		items = append(items, item)
	}
	return items
}

// parseNotes 扫描所有行，收集 `>` 开头的缩进行作为备注，关联到上一个任务项。
// key 为 1-based 行号(即对应 Item 的 LineNo),value 为合并后的备注文本(多行用 \n 拼接)。
func parseNotes(lines []string) map[int]string {
	notes := map[int]string{}
	lastItemLine := 0

	for lineNo, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if itemRe.MatchString(line) {
			lastItemLine = lineNo + 1 // 转为 1-based
			continue
		}
		if lastItemLine > 0 && strings.HasPrefix(trimmed, ">") {
			noteText := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
			if noteText == "" {
				continue
			}
			if existing, ok := notes[lastItemLine]; ok {
				notes[lastItemLine] = existing + "\n" + noteText
			} else {
				notes[lastItemLine] = noteText
			}
		}
	}
	return notes
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

// BuildTree 将扁平 Item 列表按缩进构建为父子树结构。
// level 由 len(Indent)/2 推导（每级缩进 2 空格）。返回的 slice 是原 items 的子集（根节点）。
func BuildTree(items []*Item) []*Item {
	if len(items) == 0 {
		return items
	}

	var roots []*Item
	var stack []*Item

	for _, item := range items {
		level := len(item.Indent) / 2

		for len(stack) > level {
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			roots = append(roots, item)
		} else {
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, item)
		}

		stack = append(stack, item)
	}

	return roots
}

// Flatten 将树结构（深度优先）展平为扁平列表。
// 供 ToggleAndWrite 等需要扁平 LineNo 的函数使用。
func Flatten(items []*Item) []Item {
	var out []Item
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, *it)
		out = append(out, Flatten(it.Children)...)
	}
	return out
}

// ReadAndParse 读文件 + 解析 + 构建树。文件不存在返回 (nil, nil)。
func ReadAndParse(path string) ([]*Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	flat := Parse(string(data))
	// 转指针列表以便 BuildTree 设置 Children
	ptrs := make([]*Item, len(flat))
	for i := range flat {
		ptrs[i] = &flat[i]
	}
	return BuildTree(ptrs), nil
}

// itemToLine 将 Item 转换回 Markdown 行，保留 due_date、tags 元数据。
// note 行（`> ...`）不从此函数输出，由调用方在追加时单独处理。
func itemToLine(item *Item) string {
	done := " "
	if item.Done {
		done = "x"
	}

	text := item.Text
	if item.DueDate != "" {
		text += " due:" + item.DueDate
	}
	if len(item.Tags) > 0 {
		text += " tags:" + strings.Join(item.Tags, ",")
	}

	return fmt.Sprintf("%s- [%s] %s", item.Indent, done, text)
}

// ToggleAndWrite 把 items 写回文件（先 .bak 再 atomic rename）。
// 用 itemToLine 重新生成目标行，保留 due_date / tags。
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
		lines[it.LineNo-1] = itemToLine(&it)
	}
	return atomicWrite(path, strings.Join(lines, "\n"))
}

// AddAndWrite 在文件末尾追加一项（含 due_date / tags / note 元数据）。
// 文件不存在时直接创建。
func AddAndWrite(path, text, dueDate string, tags []string, note string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("text is empty")
	}

	// 读取已有内容，确保末尾换行
	var content string
	if data, err := os.ReadFile(path); err == nil {
		content = string(data)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// 用 Item + itemToLine 生成主行，保留 metadata
	item := &Item{
		Text:    text,
		DueDate: dueDate,
		Tags:    tags,
	}
	content += itemToLine(item) + "\n"

	// 追加 note（缩进 2 空格 + `> ...`）
	if note != "" {
		for _, line := range strings.Split(strings.TrimRight(note, "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			content += "  > " + line + "\n"
		}
	}

	return atomicWrite(path, content)
}

// AddChildAndWrite 在指定父行后追加一个子项（缩进 +2 空格）。
// parentLineNo 是 Parse/Flatten 返回的 line_no（按项计，跳过空行），
// 需要扫描文件找到对应的实际行号后再插入。
func AddChildAndWrite(path string, parentLineNo int, text, dueDate string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("text is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")

	// 找到 parentLineNo 对应的实际文件行（跳过空行和非项行）
	itemCount := 0
	var actualLineIdx int = -1
	for i, line := range lines {
		if itemRe.MatchString(line) {
			itemCount++
			if itemCount == parentLineNo {
				actualLineIdx = i
				break
			}
		}
	}
	if actualLineIdx < 0 {
		return fmt.Errorf("parent line_no %d not found in file", parentLineNo)
	}

	item := &Item{
		Text:    text,
		DueDate: dueDate,
		Indent:  "  ", // 子项固定 +2 空格缩进
	}
	newLine := itemToLine(item)
	// 插入到父行之后（actualLineIdx 是 0-based，+1 变成 1-based 的 parentLineNo）
	// 注意：必须用 full slice 语法 [:n:n] 强制分配新数组，避免 append(before,...)
	// 复用了 lines 的底层数组导致 after 被覆盖。
	before := lines[:actualLineIdx+1:actualLineIdx+1]
	after := lines[actualLineIdx+1:]
	newLines := append(before, newLine)
	// 跳过 after 开头的空行（避免末尾空行导致项重复）
	i := 0
	for i < len(after) && strings.TrimSpace(after[i]) == "" {
		i++
	}
	newLines = append(newLines, after[i:]...)
	return atomicWrite(path, strings.Join(newLines, "\n"))
}

// DeleteAndWrite 删除指定行号的项（按 1-based line_no）。
// 直接按行号切除并写回，note 行不会被误删（note 是单独的 `>` 行，不在 item 行号范围内）。
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
