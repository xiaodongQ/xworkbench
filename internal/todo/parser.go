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
	DueDate  string   `json:"due_date,omitempty"`   // "YYYY-MM-DD"，""=未设
	Tags     []string `json:"tags,omitempty"`
	Note     string   `json:"note,omitempty"`
	Created  string   `json:"created,omitempty"`  // "YYYY-MM-DD"，创建日期
	Archived string   `json:"archived,omitempty"` // "YYYY-MM-DD"，归档日期
	Children []*Item  `json:"children,omitempty"` // 前端用，不写入文件
}

var (
	itemRe           = regexp.MustCompile(`^(\s*)-\s+\[( |x|X)\]\s+(.+)$`)
	dueRe            = regexp.MustCompile(`\s+due:(\d{4}-\d{2}-\d{2}|\d{2}-\d{2})\b`)
	tagsRe           = regexp.MustCompile(`\s+tags:([\w,]+)`)
	createdRe        = regexp.MustCompile(`\s+created:(\d{4}-\d{2}-\d{2})\b`)
	archivedRe       = regexp.MustCompile(`\s+archived:(\d{4}-\d{2}-\d{2})\b`)
)

const (
	archiveSeparator = "--- archived (must exist for archived, do not delete) ---"
	archiveHeader    = "## 📦 已归档"
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
		text, dueDate, tags, created, archived := parseFullMetadata(m[3])
		item := Item{
			LineNo:   i + 1,
			Indent:   m[1],
			Done:     m[2] != " ",
			Text:     text,
			DueDate:  dueDate,
			Tags:     tags,
			Created:  created,
			Archived: archived,
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

// parseFullMetadata 从文本末尾提取所有元数据，返回 (cleanText, dueDate, tags, created, archived)。
func parseFullMetadata(text string) (string, string, []string, string, string) {
	var dueDate, created, archived string
	var tags []string

	// 提取 due:YYYY-MM-DD 或 due:MM-DD
	if dueRe.MatchString(text) {
		match := dueRe.FindStringSubmatch(text)
		dueDate = match[1]
		if len(dueDate) == 5 {
			composed := fmt.Sprintf("%d-%s", time.Now().Year(), dueDate)
			if _, err := time.Parse("2006-01-02", composed); err == nil {
				dueDate = composed
			} else {
				dueDate = ""
			}
		}
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

	// 提取 created:YYYY-MM-DD
	if createdRe.MatchString(text) {
		match := createdRe.FindStringSubmatch(text)
		created = match[1]
		text = createdRe.ReplaceAllString(text, "")
	}

	// 提取 archived:YYYY-MM-DD
	if archivedRe.MatchString(text) {
		match := archivedRe.FindStringSubmatch(text)
		archived = match[1]
		text = archivedRe.ReplaceAllString(text, "")
	}

	return strings.TrimSpace(text), dueDate, tags, created, archived
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

// itemToLine 将 Item 转换回 Markdown 行，保留所有元数据。
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
	if item.Created != "" {
		text += " created:" + item.Created
	}
	if item.Archived != "" {
		text += " archived:" + item.Archived
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

// AddAndWrite 在活跃区末尾追加一项（含 due_date / tags / note 元数据）。
// 如果文件存在分隔线，新项插入到分隔线之前（活跃区末尾）。
// 文件不存在时直接创建。返回新行的 line_no（1-based）。
// 不再自动创建月份标题，直接追加到活跃区末尾。
func AddAndWrite(path, text, dueDate string, tags []string, note string) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("text is empty")
	}

	created := time.Now().Format("2006-01-02")
	item := &Item{
		Text:    text,
		DueDate: dueDate,
		Tags:    tags,
		Created: created,
	}
	newItemLine := itemToLine(item)

	// 构建新项内容（含 note 行）
	var newContent strings.Builder
	newContent.WriteString(newItemLine)
	if note != "" {
		for _, line := range strings.Split(strings.TrimRight(note, "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			newContent.WriteString("\n  > " + line)
		}
	}
	newContent.WriteString("\n")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，直接创建
			return 1, atomicWrite(path, newContent.String())
		}
		return 0, err
	}

	lines := strings.Split(string(data), "\n")
	separatorIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == archiveSeparator {
			separatorIdx = i
			break
		}
	}

	var result string
	if separatorIdx == -1 {
		// 无分隔线，直接追加到末尾
		result = strings.TrimRight(string(data), "\n")
		if result != "" && !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
		result += newContent.String()
		return scannedLineNo(result, newItemLine), atomicWrite(path, result)
	}

	// 有分隔线，插入到分隔线之前
	activeLines := lines[:separatorIdx]
	for i := 0; i < len(activeLines); i++ {
		result += activeLines[i] + "\n"
	}
	// 清理末尾空行再追加新项
	result = strings.TrimRight(result, "\n")
	if result != "" && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	result += newContent.String()
	// 追加分隔线和归档区
	result += archiveSeparator + "\n"
	for i := separatorIdx + 1; i < len(lines); i++ {
		result += lines[i] + "\n"
	}
	// 写完文件前扫描 result，按新 item 行精确返回 1-based 行号
	// （活跃区末尾非固定行：TrimRight 会"吃掉"分隔线前的空行让位给新 item，
	//  实际行号依赖 activeLines 末尾有几个空行，无法用算式覆盖）
	return scannedLineNo(result, newItemLine), atomicWrite(path, result)
}

// scannedLineNo 在 result 中找到 newItemLine 第一次出现的位置，返回 1-based 行号。
// 用字符串扫描替代"用算式硬算行号"——避免 AddAndWrite 内部 TrimRight/补换行带来的边界误差。
// 找不到返回 0（调用方应自己处理这种情况，理论上不可能）。
func scannedLineNo(result, newItemLine string) int {
	for i, l := range strings.Split(strings.TrimRight(result, "\n"), "\n") {
		if l == newItemLine {
			return i + 1
		}
	}
	return 0
}

// AddChildAndWrite 在指定父行后追加一个子项。
// parentLineNo 是 Parse/Flatten 返回的 line_no（1-based）。
// indent 继承父行缩进 + 2 空格（支持嵌套）；done 控制勾选状态。
// parentLineNo 是 Parse/Flatten 返回的 line_no（按项计，跳过空行），
// 需要扫描文件找到对应的实际行号后再插入。
func AddChildAndWrite(path string, parentLineNo int, text, dueDate string, done bool) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("text is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")

	// parentLineNo 是 1-based，转 0-based
	parentIdx := parentLineNo - 1
	if parentIdx < 0 || parentIdx >= len(lines) {
		return fmt.Errorf("parent line_no %d not found in file", parentLineNo)
	}

	// 提取父行缩进
	parentMatch := itemRe.FindStringSubmatch(lines[parentIdx])
	parentIndent := ""
	if len(parentMatch) >= 2 {
		parentIndent = parentMatch[1]
	}
	childIndent := parentIndent + "  "

	// 扫描后续行，找到插入位置：更深缩进 item/孙项、note 行跳过，
	// 遇到分隔线/归档区标题/同级或更浅的 item 停止；空行本身不停止（跳过）
	insertIdx := parentIdx + 1
	for insertIdx < len(lines) {
		line := lines[insertIdx]
		trimmed := strings.TrimSpace(line)

		// 遇到分隔线或归档区标题则停止（活跃区边界）
		if trimmed == archiveSeparator || strings.HasPrefix(trimmed, "## 📦") {
			break
		}

		// 空行不停止，跳过继续看下一行
		if trimmed == "" {
			insertIdx++
			continue
		}

		if !itemRe.MatchString(line) {
			insertIdx++
			continue
		}
		subMatch := itemRe.FindStringSubmatch(line)
		if len(subMatch) < 2 {
			insertIdx++
			continue
		}
		subIndent := subMatch[1]
		if len(subIndent) > len(parentIndent) {
			insertIdx++
			continue
		}
		break
	}

	item := &Item{
		Text:    text,
		DueDate: dueDate,
		Done:    done,
		Indent:  childIndent,
	}
	newLine := itemToLine(item)
	before := lines[:insertIdx:insertIdx]
	after := lines[insertIdx:]

	// 移除 before 末尾的连续空行，避免子项和分隔线之间出现多余空行
	for len(before) > 0 && strings.TrimSpace(before[len(before)-1]) == "" {
		before = before[:len(before)-1]
	}
	// 如果 after 以 archiveSeparator 开头，只保留其前的一个空行
	if len(after) > 0 && strings.TrimSpace(after[0]) == archiveSeparator {
		// 移除 --- 前的所有空行，然后留一个空行
		for len(after) > 0 && strings.TrimSpace(after[0]) == "" {
			after = after[1:]
		}
		if len(after) > 0 {
			after = append([]string{""}, after...)
		}
	}

	newLines := append(before, newLine)
	newLines = append(newLines, after...)
	joined := strings.Join(newLines, "\n")
	// 保证文件以换行结尾（pre-existing bug：before 末尾 trim 空行会让 after 的尾空行丢失）
	if !strings.HasSuffix(joined, "\n") {
		joined += "\n"
	}
	return atomicWrite(path, joined)
}

// DeleteAndWrite 删除指定行号 item 及其所有子项（缩进更深）和关联 note 行。
func DeleteAndWrite(path string, lineNo int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	idx := lineNo - 1 // 转 0-based
	if idx < 0 || idx >= len(lines) {
		return fmt.Errorf("line_no %d out of range", lineNo)
	}

	// 取父行缩进宽度
	parentMatch := itemRe.FindStringSubmatch(lines[idx])
	parentIndentLen := 0
	if len(parentMatch) >= 2 {
		parentIndentLen = len(parentMatch[1])
	}

	// 从 parentIdx+1 向下扫描：更深缩进的 item 行 + 关联 note 行 删除，遇到同级/更浅/空行停止
	endIdx := idx + 1
	for endIdx < len(lines) {
		line := lines[endIdx]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		if !itemRe.MatchString(line) {
			if strings.HasPrefix(line, "  > ") && len(line)-len(strings.TrimLeft(line, " ")) > parentIndentLen {
				endIdx++
				continue
			}
			break
		}
		subMatch := itemRe.FindStringSubmatch(line)
		if len(subMatch) < 2 {
			break
		}
		subIndentLen := len(subMatch[1])
		if subIndentLen > parentIndentLen {
			endIdx++
			continue
		}
		break
	}

	kept := make([]string, 0, len(lines)-endIdx+idx)
	kept = append(kept, lines[:idx]...)
	kept = append(kept, lines[endIdx:]...)
	return atomicWrite(path, strings.Join(kept, "\n"))
}

func atomicWrite(path, content string) error {
	bak := path + ".bak"
	if err := os.WriteFile(bak, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(bak, path)
}

// ParsedSections 解析后的两个分区：活跃区 + 归档区。
type ParsedSections struct {
	ActiveItems   []*Item // 归档区之外的所有项（含已勾选未归档）
	ArchivedItems  []*Item // archived 非空的项
}

// ParseSections 解析 todo.md 内容，返回活跃区和归档区两个独立的项列表。
// 文件无分隔线时，全部视为活跃区项。
func ParseSections(content string) (*ParsedSections, error) {
	lines := strings.Split(content, "\n")

	// 定位分隔线位置
	separatorIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == archiveSeparator {
			separatorIdx = i
			break
		}
	}

	var activeContent, archivedContent string
	if separatorIdx == -1 {
		activeContent = content
	} else {
		activeContent = strings.Join(lines[:separatorIdx], "\n")
		archivedContent = strings.Join(lines[separatorIdx+1:], "\n")
	}

	// 解析活跃区（跳过月份标题行）
	// activeContent 的行号从 1 开始
	activeFlat := parseItemsWithSkip(activeContent, 0)
	activePtrs := make([]*Item, len(activeFlat))
	for i := range activeFlat {
		activePtrs[i] = &activeFlat[i]
	}

	// 解析归档区
	// archivedContent 的起始行号是 separatorIdx+1（1-indexed）
	// 由于 join(lines[separatorIdx+1:], "\n") 会产生额外空行，用 separatorIdx+1 作为偏移量
	var archivedFlat []Item
	if archivedContent != "" {
		archivedFlat = parseItemsWithSkip(archivedContent, separatorIdx+1)
	}
	archivedPtrs := make([]*Item, len(archivedFlat))
	for i := range archivedFlat {
		archivedPtrs[i] = &archivedFlat[i]
	}

	return &ParsedSections{
		ActiveItems:   BuildTree(activePtrs),
		ArchivedItems: BuildTree(archivedPtrs),
	}, nil
}

// parseItemsWithSkip 解析 content 中的 todo 项，跳过月份标题行（### xxx）和空白行。
func parseItemsWithSkip(content string, lineOffset int) []Item {
	lines := strings.Split(content, "\n")
	noteMap := parseNotes(lines)

	var items []Item
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// 跳过月份标题行和空行
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "###") {
			continue
		}
		if strings.HasPrefix(trimmed, "##") {
			continue
		}

		m := itemRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text, dueDate, tags, created, archived := parseFullMetadata(m[3])
		item := Item{
			LineNo:   i + 1 + lineOffset,
			Indent:   m[1],
			Done:     m[2] != " ",
			Text:     text,
			DueDate:  dueDate,
			Tags:     tags,
			Created:  created,
			Archived: archived,
		}
		if note, ok := noteMap[i+1]; ok {
			item.Note = note
		}
		items = append(items, item)
	}
	return items
}

// formatNote 格式化 note 行。
func formatNote(note, indent string) string {
	if note == "" {
		return ""
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(note, "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, indent+"> "+line)
	}
	return strings.Join(lines, "\n")
}

// itemLines 生成单个 item（含子孙项、备注）的行列表。
func itemLines(item *Item) []string {
	var lines []string
	lines = append(lines, itemToLine(item))
	if item.Note != "" {
		lines = append(lines, formatNote(item.Note, item.Indent+"  "))
	}
	for _, child := range item.Children {
		lines = append(lines, itemLines(child)...)
	}
	return lines
}

// WriteSections 将活跃区和归档区按格式写入文件。
// 活跃区：直接输出 items（不过月分组）
// 归档区：有归档项时 --- 分隔线 + items
func WriteSections(path string, active, archived []*Item) error {
	var sb strings.Builder

	// 写活跃区
	for _, item := range active {
		for _, line := range itemLines(item) {
			sb.WriteString(line + "\n")
		}
	}

	// 写分隔线和归档区（仅当有归档项时）
	if len(archived) > 0 {
		sb.WriteString(archiveSeparator + "\n\n" + archiveHeader + "\n\n")
		for _, item := range archived {
			for _, line := range itemLines(item) {
				sb.WriteString(line + "\n")
			}
		}
	}

	return atomicWrite(path, sb.String())
}

// ReadAndParseSections 读文件 + ParseSections + 构建树。文件不存在返回 (nil, nil)。
func ReadAndParseSections(path string) (*ParsedSections, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return ParseSections(string(data))
}

// ArchiveItem 将指定 lineNo 的顶级项（含子孙项）从活跃区移到归档区，写入 archived:日期。
// lineNo 是 ParseSections 解析后 Flatten 得到的 lineNo（基于原始文件）。

// ArchiveItem 将指定 lineNo 的顶级项（含子孙项）从活跃区移到归档区，写入 archived:日期。
// 直接操作文件行，不调用 WriteSections，避免行号错乱。
func ArchiveItem(path string, lineNo int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	idx := lineNo - 1
	if idx < 0 || idx >= len(lines) {
		return fmt.Errorf("line_no %d out of range", lineNo)
	}

	// 校验顶级项
	m := itemRe.FindStringSubmatch(lines[idx])
	if m == nil {
		return fmt.Errorf("line_no %d is not a todo item", lineNo)
	}
	if m[1] != "" {
		return fmt.Errorf("子项不能单独归档")
	}

	// 确定该项（含子孙项+备注）的范围 [startIdx, endIdx)
	startIdx := idx
	endIdx := idx + 1
	parentIndentLen := 0
	for endIdx < len(lines) {
		line := lines[endIdx]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		// 遇到分隔线或归档区标题则停止
		if trimmed == archiveSeparator || strings.HasPrefix(trimmed, "## 📦") {
			break
		}
		if !itemRe.MatchString(line) {
			if strings.HasPrefix(line, "  > ") {
				indentLen := len(line) - len(strings.TrimLeft(line, " "))
				if indentLen > parentIndentLen {
					endIdx++
					continue
				}
			}
			break
		}
		subMatch := itemRe.FindStringSubmatch(line)
		if len(subMatch) < 2 {
			break
		}
		subIndentLen := len(subMatch[1])
		if subIndentLen > parentIndentLen {
			endIdx++
			continue
		}
		break
	}

	// 提取要归档的行，给顶级项（缩进为空）加 archived: 标签，子项保持原样
	today := time.Now().Format("2006-01-02")
	var movedLinesUpdated []string
	for _, line := range lines[startIdx:endIdx] {
		m2 := itemRe.FindStringSubmatch(line)
		if m2 != nil && !archivedRe.MatchString(line) {
			if m2[1] == "" {
				// 顶级项加 archived 标签
				updatedLine := m2[1] + "- [" + m2[2] + "] " + m2[3] + " archived:" + today
				movedLinesUpdated = append(movedLinesUpdated, updatedLine)
			} else {
				movedLinesUpdated = append(movedLinesUpdated, line)
			}
		} else {
			movedLinesUpdated = append(movedLinesUpdated, line)
		}
	}

	// 构建新文件内容
	// 1. 保留活跃区：startIdx 之前 + endIdx 之后，但要跳过末尾空行
	keptLines := append(lines[:startIdx], lines[endIdx:]...)
	// 清理末尾空行
	for len(keptLines) > 0 && strings.TrimSpace(keptLines[len(keptLines)-1]) == "" {
		keptLines = keptLines[:len(keptLines)-1]
	}

	var sb strings.Builder
	for i, line := range keptLines {
		sb.WriteString(line)
		if i < len(keptLines)-1 {
			sb.WriteString("\n")
		}
	}

	// 2. 在 keptLines 末尾补分隔线（如果原文件已有就只换行，避免重复加）
	// 关键：必须看整个原文件，而不是只看 startIdx 之前——因为 keptLines 已经把
	// endIdx 之后的归档区（含原分隔线）保留下来了，只看 startIdx 之前会把那个
	// 已存在的分隔符漏算，导致每次归档都多出一个分隔线。
	hasSeparator := false
	for _, l := range lines {
		if strings.TrimSpace(l) == archiveSeparator {
			hasSeparator = true
			break
		}
	}
	if !hasSeparator {
		sb.WriteString("\n\n" + archiveSeparator + "\n\n" + archiveHeader + "\n\n")
	} else {
		sb.WriteString("\n")
	}

	// 3. 追加归档项
	for _, line := range movedLinesUpdated {
		sb.WriteString(line + "\n")
	}

	return atomicWrite(path, sb.String())
}

// UnarchiveItem 将指定 lineNo 的项（含子孙项）从归档区恢复到活跃区，移除 archived 标签。
// 直接操作文件行，不调用 WriteSections，避免行号错乱。
func UnarchiveItem(path string, lineNo int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	idx := lineNo - 1
	if idx < 0 || idx >= len(lines) {
		return fmt.Errorf("line_no %d out of range", lineNo)
	}

	// 找到分隔线位置
	separatorIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == archiveSeparator {
			separatorIdx = i
			break
		}
	}
	if separatorIdx == -1 || idx < separatorIdx {
		return fmt.Errorf("该项不在归档区")
	}

	// 校验归档区顶级项
	m := itemRe.FindStringSubmatch(lines[idx])
	if m == nil {
		return fmt.Errorf("line_no %d is not a todo item", lineNo)
	}
	if m[1] != "" {
		return fmt.Errorf("子项不能单独恢复")
	}

	// 确定范围 [startIdx, endIdx)
	startIdx := idx
	endIdx := idx + 1
	parentIndentLen := 0
	for endIdx < len(lines) {
		line := lines[endIdx]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		if !itemRe.MatchString(line) {
			if strings.HasPrefix(line, "  > ") {
				indentLen := len(line) - len(strings.TrimLeft(line, " "))
				if indentLen > parentIndentLen {
					endIdx++
					continue
				}
			}
			break
		}
		subMatch := itemRe.FindStringSubmatch(line)
		if len(subMatch) < 2 {
			break
		}
		subIndentLen := len(subMatch[1])
		if subIndentLen > parentIndentLen {
			endIdx++
			continue
		}
		break
	}

	// 提取要恢复的行，移除 archived:标签，保留缩进
	var movedLinesUpdated []string
	for _, line := range lines[startIdx:endIdx] {
		m2 := itemRe.FindStringSubmatch(line)
		if m2 != nil {
			text := archivedRe.ReplaceAllString(m2[3], "")
			// 保留原始缩进
			updatedLine := m2[1] + "- [" + m2[2] + "] " + strings.TrimSpace(text)
			movedLinesUpdated = append(movedLinesUpdated, updatedLine)
		} else {
			// 非 todo 行（note）保留原始缩进
			movedLinesUpdated = append(movedLinesUpdated, line)
		}
	}

	// 构建新文件内容
	// 1. 保留活跃区（分隔线之前）
	var sb strings.Builder
	for i := 0; i < separatorIdx; i++ {
		sb.WriteString(lines[i] + "\n")
	}

	// 2. 清理活跃区末尾的空行
	content := sb.String()
	// 移除末尾连续的空行（最多2个）
	emptyCount := 0
	for len(content) > 0 && content[len(content)-1] == '\n' {
		if emptyCount >= 2 {
			break
		}
		content = content[:len(content)-1]
		emptyCount++
	}
	sb.Reset()
	sb.WriteString(content)
	if !strings.HasSuffix(sb.String(), "\n") && sb.String() != "" {
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// 3. 追加恢复的项
	for _, line := range movedLinesUpdated {
		sb.WriteString(line + "\n")
	}

	// 4. 如果归档区还有其他内容，保留
	for i := endIdx; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		// 跳过 ## 标题行，保留其他内容
		if strings.HasPrefix(trimmed, "##") {
			continue
		}
		sb.WriteString(lines[i] + "\n")
	}

	return atomicWrite(path, sb.String())
}
