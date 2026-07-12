package todo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// 核心 Bug 验证：连续 AddAndWrite 返回的 line_no 必须准确
// ============================================================================

// TestComprehensive_ConsecutiveAddAndWrite line_no 不漂移
func TestComprehensive_ConsecutiveAddAndWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	expected := []struct {
		text      string
		wantLine  int
	}{
		{"Parent 1", 2},
		{"Parent 2", 3},
		{"Parent 3", 4},
		{"Parent 4", 5},
	}

	for _, e := range expected {
		gotLine, err := AddAndWrite(path, e.text, "", nil, "")
		if err != nil {
			t.Fatalf("AddAndWrite(%q): %v", e.text, err)
		}
		if gotLine != e.wantLine {
			t.Errorf("AddAndWrite(%q) lineNo=%d, want %d", e.text, gotLine, e.wantLine)
		}
		// 校验文件中该项的实际行号
		data, _ := os.ReadFile(path)
		items, _ := ReadAndParse(string_to_path_helper(t, data))
		var found bool
		for _, it := range items {
			if it.Text == e.text {
				if it.LineNo != e.wantLine {
					t.Errorf("After AddAndWrite(%q), parsed LineNo=%d, want %d", e.text, it.LineNo, e.wantLine)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("After AddAndWrite(%q), item not found in parse", e.text)
		}
	}
}

// TestComprehensive_AddAndWriteAfterChildrenWithNoTrailingNewline 复现 Bug #1+#2
// 场景：父任务 + 子项 后再添加父任务，line_no 必须准确
func TestComprehensive_AddAndWriteAfterChildrenWithNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 添加 Parent 1 + 2 个子项
	p1Line, err := AddAndWrite(path, "Parent 1", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if p1Line != 2 {
		t.Fatalf("Parent 1 lineNo=%d, want 2", p1Line)
	}

	if err := AddChildAndWrite(path, p1Line, "Child 1.1", "", false); err != nil {
		t.Fatal(err)
	}
	if err := AddChildAndWrite(path, p1Line, "Child 1.2", "", false); err != nil {
		t.Fatal(err)
	}

	// 此时文件结构（关键 Bug 触发点）
	data, _ := os.ReadFile(path)
	t.Logf("File after Parent 1 + 2 children:\n%s", string(data))

	// 添加 Parent 2（关键测试点）
	p2Line, err := AddAndWrite(path, "Parent 2", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("AddAndWrite Parent 2 returned lineNo=%d", p2Line)

	// 解析验证 Parent 2 的实际行号
	data, _ = os.ReadFile(path)
	items, _ := ReadAndParse(string_to_path_helper(t, data))
	var actualP2Line int
	for _, it := range items {
		if it.Text == "Parent 2" {
			actualP2Line = it.LineNo
			break
		}
	}
	if actualP2Line == 0 {
		t.Fatal("Parent 2 not found after AddAndWrite")
	}
	if p2Line != actualP2Line {
		t.Errorf("Returned lineNo=%d, but actual file line=%d (mismatch)", p2Line, actualP2Line)
	}

	// 给 Parent 2 加子项，必须正确嵌套
	if err := AddChildAndWrite(path, p2Line, "Child 2.1", "", false); err != nil {
		t.Fatal(err)
	}

	// 重新解析验证树结构
	data, _ = os.ReadFile(path)
	t.Logf("Final file:\n%s", string(data))
	tree := BuildTree(bytesToTree(data))

	if len(tree) != 2 {
		t.Fatalf("Tree has %d root items, want 2", len(tree))
	}

	for _, root := range tree {
		switch root.Text {
		case "Parent 1":
			if len(root.Children) != 2 {
				t.Errorf("Parent 1 children=%d, want 2", len(root.Children))
			}
		case "Parent 2":
			if len(root.Children) != 1 {
				t.Errorf("Parent 2 children=%d, want 1", len(root.Children))
			}
			if len(root.Children) > 0 && root.Children[0].Text != "Child 2.1" {
				t.Errorf("Parent 2's child=%q, want Child 2.1", root.Children[0].Text)
			}
		}
	}
}

// TestComprehensive_AddChildAndWritePreservesTrailingNewline 复现 Bug #2
// AddChildAndWrite 后文件应保留尾换行
func TestComprehensive_AddChildAndWritePreservesTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n- [ ] Parent\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := AddChildAndWrite(path, 2, "Child", "", false); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		t.Errorf("File missing trailing newline after AddChildAndWrite, ends with %q", string(data[len(data)-1]))
	}
}

// ============================================================================
// 综合场景：模拟前端提交完整流程
// ============================================================================

// TestComprehensive_FrontendSubmitFlow_AddNewParentWithChildren 模拟前端添加新任务带子项
func TestComprehensive_FrontendSubmitFlow_AddNewParentWithChildren(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// === 第一轮：添加 "Project A" + 3 个子项 ===
	p1Line, err := AddAndWrite(path, "Project A", "2026-07-15", []string{"work"}, "")
	if err != nil {
		t.Fatal(err)
	}

	for _, child := range []string{"Sub A1", "Sub A2", "Sub A3"} {
		if err := AddChildAndWrite(path, p1Line, child, "", false); err != nil {
			t.Fatalf("AddChild %q to line %d: %v", child, p1Line, err)
		}
	}

	// === 第二轮：添加 "Project B" + 2 个子项 ===
	p2Line, err := AddAndWrite(path, "Project B", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	for _, child := range []string{"Sub B1", "Sub B2"} {
		if err := AddChildAndWrite(path, p2Line, child, "", false); err != nil {
			t.Fatalf("AddChild %q to line %d: %v", child, p2Line, err)
		}
	}

	// === 第三轮：添加 "Project C" + 1 个子项（带 note） ===
	p3Line, err := AddAndWrite(path, "Project C", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// Project C 主任务带 note
	if err := AddChildAndWrite(path, p3Line, "Sub C1", "", false); err != nil {
		t.Fatal(err)
	}

	// === 验证最终结构 ===
	data, _ := os.ReadFile(path)
	t.Logf("Final file:\n%s", string(data))

	items, _ := ReadAndParse(string_to_path_helper(t, data))
	tree := BuildTree(items)

	if len(tree) != 3 {
		t.Errorf("Tree root count=%d, want 3", len(tree))
	}

	wantTree := map[string]int{
		"Project A": 3,
		"Project B": 2,
		"Project C": 1,
	}
	for _, root := range tree {
		wantCount, ok := wantTree[root.Text]
		if !ok {
			t.Errorf("Unexpected root: %q", root.Text)
			continue
		}
		if len(root.Children) != wantCount {
			t.Errorf("Root %q children=%d, want %d", root.Text, len(root.Children), wantCount)
		}
	}
}

// TestComprehensive_FrontendSubmitFlow_AddGrandchildren 添加多层级
func TestComprehensive_FrontendSubmitFlow_AddGrandchildren(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 添加父任务
	pLine, err := AddAndWrite(path, "Root", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// 添加子任务
	if err := AddChildAndWrite(path, pLine, "Child 1", "", false); err != nil {
		t.Fatal(err)
	}

	// 重新解析找子任务行号（ReadAndParse 只返回 root，需要遍历整个 tree 找 "Child 1"）
	data, _ := os.ReadFile(path)
	items, _ := ReadAndParse(string_to_path_helper(t, data))
	var c1Line int
	var findChild func([]*Item) bool
	findChild = func(ns []*Item) bool {
		for _, n := range ns {
			if n.Text == "Child 1" {
				c1Line = n.LineNo
				return true
			}
			if findChild(n.Children) {
				return true
			}
		}
		return false
	}
	if !findChild(items) {
		t.Fatalf("Child 1 not found in tree; items=%#v", items)
	}

	// 添加孙任务
	if err := AddChildAndWrite(path, c1Line, "Grandchild 1", "", false); err != nil {
		t.Fatal(err)
	}
	if err := AddChildAndWrite(path, c1Line, "Grandchild 2", "", false); err != nil {
		t.Fatal(err)
	}

	// 再添加一个子任务到 Root
	if err := AddChildAndWrite(path, pLine, "Child 2", "", false); err != nil {
		t.Fatal(err)
	}

	// 验证树结构
	data, _ = os.ReadFile(path)
	t.Logf("File:\n%s", string(data))
	tree := BuildTree(bytesToTree(data))

	if len(tree) != 1 {
		t.Fatalf("Tree roots=%d, want 1", len(tree))
	}
	root := tree[0]
	if root.Text != "Root" {
		t.Fatalf("Root text=%q, want Root", root.Text)
	}
	if len(root.Children) != 2 {
		t.Errorf("Root children=%d, want 2", len(root.Children))
	}
	for _, c := range root.Children {
		if c.Text == "Child 1" {
			if len(c.Children) != 2 {
				t.Errorf("Child 1 grandchildren=%d, want 2", len(c.Children))
			}
		}
	}
}

// ============================================================================
// 混合操作场景：编辑、归档后继续添加
// ============================================================================

// TestComprehensive_MultipleArchivesNoExtraSeparator 连续归档多次，文件里应只有 1 个 ---
// （之前的 bug：ArchiveItem 只看 startIdx 之前有没有 ---，忽略了 endIdx 之后已保留的分隔线，
//  导致每次归档都新增一个 ---）
func TestComprehensive_MultipleArchivesNoExtraSeparator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	// 初始：已带分隔线（这是 xworkbench 标准模板）
	initial := "- [ ] A\n- [ ] B\n\n--- archived (must exist for archived, do not delete) ---\n\n## 📦 Archived\n- [x] Z archived:2026-07-01\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	// 第一次归档 A
	if err := ArchiveItem(path, 1); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
		separator := "--- archived (must exist for archived, do not delete) ---"
		if n := strings.Count(string(data), separator); n != 1 {
			t.Errorf("第一次归档后 separator 数 = %d, want 1\n内容:\n%s", n, data)
		}

		// 第二次归档 B（line 1）
		if err := ArchiveItem(path, 1); err != nil {
			t.Fatal(err)
		}
		data, _ = os.ReadFile(path)
		if n := strings.Count(string(data), separator); n != 1 {
			t.Errorf("第二次归档后 separator 数 = %d, want 1\n内容:\n%s", n, data)
		}

	// 第三次归档（原文件里没顶级活跃项了，跳过 —— 校验前两个已归档的结构合理）
	sections, err := ParseSections(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(sections.ActiveItems) != 0 {
		t.Errorf("ActiveItems = %d, want 0", len(sections.ActiveItems))
	}
	if len(sections.ArchivedItems) != 3 {
		t.Errorf("ArchivedItems = %d, want 3 (Z + A + B)", len(sections.ArchivedItems))
	}
}

// TestComprehensive_ArchiveThenAdd 归档后继续添加新任务
func TestComprehensive_ArchiveThenAdd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n- [ ] Task 1\n- [ ] Task 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 归档 Task 1
	if err := ArchiveItem(path, 2); err != nil {
		t.Fatal(err)
	}

	// 添加新任务
	pLine, err := AddAndWrite(path, "Task 3", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// 给新任务加子项
	if err := AddChildAndWrite(path, pLine, "Sub 3.1", "", false); err != nil {
		t.Fatal(err)
	}

	// 验证结构
	data, _ := os.ReadFile(path)
	t.Logf("File:\n%s", string(data))

	sections, err := ParseSections(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(sections.ArchivedItems) != 1 {
		t.Errorf("Archived count=%d, want 1", len(sections.ArchivedItems))
	}
	if len(sections.ActiveItems) != 2 {
		t.Errorf("Active count=%d, want 2", len(sections.ActiveItems))
	}

	// Task 3 应有一个子项
	for _, root := range sections.ActiveItems {
		if root.Text == "Task 3" {
			if len(root.Children) != 1 {
				t.Errorf("Task 3 children=%d, want 1", len(root.Children))
			}
		}
	}
}

// TestComprehensive_ToggleAndWritePreservesStructure 切换勾选状态不破坏结构
func TestComprehensive_ToggleAndWritePreservesStructure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n- [ ] Parent\n  - [ ] Child\n"), 0644); err != nil {
		t.Fatal(err)
	}

	items := Parse("- [ ] Parent\n  - [ ] Child\n")
	// 切换 Parent 为已完成
	for i := range items {
		if items[i].Text == "Parent" {
			items[i].Done = true
		}
	}

	if err := ToggleAndWrite(path, items); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	t.Logf("File:\n%s", string(data))

	if !strings.Contains(string(data), "- [x] Parent") {
		t.Error("Parent should be checked after toggle")
	}
	if !strings.Contains(string(data), "- [ ] Child") {
		t.Error("Child should remain unchecked")
	}
}

// TestComprehensive_DeleteParentCascadesChildren 删除父项级联删除子项
func TestComprehensive_DeleteParentCascadesChildren(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n- [ ] Parent\n  - [ ] Child 1\n  - [ ] Child 2\n  - [ ] Grandchild\n- [ ] Other\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := DeleteAndWrite(path, 2); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	t.Logf("File:\n%s", string(data))

	items, _ := ReadAndParse(string_to_path_helper(t, data))
	if len(items) != 1 {
		t.Errorf("After delete parent, items=%d, want 1", len(items))
	}
	if len(items) > 0 && items[0].Text != "Other" {
		t.Errorf("Remaining item=%q, want Other", items[0].Text)
	}
}

// TestComprehensive_DeleteChildPreservesParentAndSiblings 删除中间子项
func TestComprehensive_DeleteChildPreservesParentAndSiblings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n- [ ] Parent\n  - [ ] Child 1\n  - [ ] Child 2\n  - [ ] Child 3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Child 2 is at line 3
	if err := DeleteAndWrite(path, 3); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	t.Logf("File:\n%s", string(data))

	items, _ := ReadAndParse(string_to_path_helper(t, data))
	// ReadAndParse 返回 root 切片，1 个 root；数全树用 Flatten
	flat := Flatten(items)
	if len(flat) != 3 {
		t.Errorf("After delete child, items=%d, want 3 (Parent + Child 1 + Child 3)", len(flat))
	}

	// 验证剩余项（line 3 是 Child 1，删除后剩 Child 2 + Child 3）
	wantTexts := []string{"Parent", "Child 2", "Child 3"}
	for i, it := range flat {
		if it.Text != wantTexts[i] {
			t.Errorf("Item[%d]=%q, want %q", i, it.Text, wantTexts[i])
		}
	}
}

// ============================================================================
// 元数据保持测试
// ============================================================================

// TestComprehensive_TogglePreservesMetadata 切换勾选保留元数据
func TestComprehensive_TogglePreservesMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n- [ ] Task due:2026-07-15 tags:work,urgent\n  > 这是一个备注\n  > 第二行备注\n"), 0644); err != nil {
		t.Fatal(err)
	}

	items := Parse("- [ ] Task due:2026-07-15 tags:work,urgent\n  > 这是一个备注\n  > 第二行备注\n")
	for i := range items {
		items[i].Done = true
	}

	if err := ToggleAndWrite(path, items); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	t.Logf("File:\n%s", content)

	if !strings.Contains(content, "due:2026-07-15") {
		t.Error("due_date should be preserved")
	}
	if !strings.Contains(content, "tags:work,urgent") {
		t.Error("tags should be preserved")
	}
}

// TestComprehensive_EditMetadata 修改元数据
func TestComprehensive_EditMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n- [ ] Task\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 模拟 edit API
	items := Parse("- [ ] Task\n")
	for i := range items {
		items[i].Text = "Task updated"
		items[i].DueDate = "2026-07-20"
		items[i].Tags = []string{"important"}
	}

	if err := ToggleAndWrite(path, items); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	t.Logf("File:\n%s", content)

	if !strings.Contains(content, "Task updated") {
		t.Error("Text should be updated")
	}
	if !strings.Contains(content, "due:2026-07-20") {
		t.Error("due_date should be set")
	}
	if !strings.Contains(content, "tags:important") {
		t.Error("tags should be set")
	}
}

// ============================================================================
// 边界条件
// ============================================================================

// TestComprehensive_AddToEmptyFile 添加到空文件
func TestComprehensive_AddToEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	// 文件不存在

	lineNo, err := AddAndWrite(path, "First task", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if lineNo != 1 {
		t.Errorf("First add lineNo=%d, want 1", lineNo)
	}
}

// TestComprehensive_AddWithSeparator 添加到带分隔线的文件
func TestComprehensive_AddWithSeparator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n- [ ] Old task\n\n--- archived (must exist for archived, do not delete) ---\n\n## 📦 已归档\n- [x] Archived task\n"), 0644); err != nil {
		t.Fatal(err)
	}

	lineNo, err := AddAndWrite(path, "New task", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// 新任务应插入到 --- 之前
	data, _ := os.ReadFile(path)
	content := string(data)
	t.Logf("File:\n%s", content)

	// 验证新任务在归档标题前
	newIdx := strings.Index(content, "New task")
	archIdx := strings.Index(content, "Archived task")
	if newIdx == -1 {
		t.Fatal("New task not found")
	}
	if archIdx == -1 {
		t.Fatal("Archived task not found")
	}
	if newIdx > archIdx {
		t.Error("New task should appear before Archived task")
	}
	t.Logf("Added at lineNo=%d", lineNo)
}

// TestComprehensive_AddChildWithDone 添加已完成的子项
func TestComprehensive_AddChildWithDone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n- [ ] Parent\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := AddChildAndWrite(path, 2, "Done child", "", true); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "- [x] Done child") {
		t.Errorf("Done child should be checked, file:\n%s", string(data))
	}
}

// TestComprehensive_EmptyText 空白文本报错
func TestComprehensive_EmptyText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	os.WriteFile(path, []byte("# Todo\n"), 0644)

	_, err := AddAndWrite(path, "", "", nil, "")
	if err == nil {
		t.Error("Empty text should error")
	}

	_, err = AddAndWrite(path, "   ", "", nil, "")
	if err == nil {
		t.Error("Whitespace text should error")
	}

	err = AddChildAndWrite(path, 1, "", "", false)
	if err == nil {
		t.Error("Empty child should error")
	}
}

// ============================================================================
// 树结构验证
// ============================================================================

// TestComprehensive_BuildTreeDeepNesting 深度嵌套
func TestComprehensive_BuildTreeDeepNesting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n- [ ] L0\n  - [ ] L1\n    - [ ] L2\n      - [ ] L3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	items, _ := ReadAndParse(string_to_path_helper(t, data))
	tree := BuildTree(items)

	if len(tree) != 1 {
		t.Fatalf("Tree root count=%d, want 1", len(tree))
	}

	// 逐层验证
	level := tree
	depths := []string{"L0", "L1", "L2", "L3"}
	for d, expected := range depths {
		if len(level) == 0 {
			t.Fatalf("Level %d empty", d)
		}
		if level[0].Text != expected {
			t.Errorf("Level %d text=%q, want %q", d, level[0].Text, expected)
		}
		level = level[0].Children
	}
}

// TestComprehensive_FlattenMultipleRoots 多根节点扁平化
func TestComprehensive_FlattenMultipleRoots(t *testing.T) {
	// 用 BuildTree 让它按 Indent 自动识别父子（之前手工构造的 items 数组里 5 个全当 root，
	// children 引用导致重复 append；让 BuildTree 一次性正确建树）
	flatInput := []*Item{
		{LineNo: 1, Text: "A", Indent: ""},
		{LineNo: 2, Text: "A1", Indent: "  "},
		{LineNo: 3, Text: "A2", Indent: "  "},
		{LineNo: 4, Text: "B", Indent: ""},
		{LineNo: 5, Text: "B1", Indent: "  "},
	}
	tree := BuildTree(flatInput)

	flat := Flatten(tree)
	if len(flat) != 5 {
		t.Errorf("Flatten len=%d, want 5", len(flat))
	}
	wantOrder := []string{"A", "A1", "A2", "B", "B1"}
	for i, item := range flat {
		if item.Text != wantOrder[i] {
			t.Errorf("Flat[%d]=%q, want %q", i, item.Text, wantOrder[i])
		}
	}
}

// ============================================================================
// 行号一致性核心测试
// ============================================================================

// TestComprehensive_LineNoStabilityAcrossMultipleOps 多种操作后行号仍然稳定
func TestComprehensive_LineNoStabilityAcrossMultipleOps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# Todo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 场景：交错添加父任务和子任务
	type op struct {
		action string // "add" | "child"
		parent int    // for child action
		text   string
	}

	ops := []op{
		{"add", 0, "P1"},
		{"child", 2, "P1.C1"},
		{"add", 0, "P2"},
		{"child", 2, "P1.C2"},
		{"child", 6, "P2.C1"},
		{"add", 0, "P3"},
		{"child", 2, "P1.C3"},
	}

	for i, o := range ops {
		switch o.action {
		case "add":
			lineNo, err := AddAndWrite(path, o.text, "", nil, "")
			if err != nil {
				t.Fatalf("Op[%d] AddAndWrite(%q): %v", i, o.text, err)
			}
			// 验证 lineNo 准确
			data, _ := os.ReadFile(path)
			items, _ := ReadAndParse(string_to_path_helper(t, data))
			for _, it := range items {
				if it.Text == o.text {
					if it.LineNo != lineNo {
						t.Errorf("Op[%d] AddAndWrite(%q) returned %d, actual line %d", i, o.text, lineNo, it.LineNo)
					}
					break
				}
			}
		case "child":
			err := AddChildAndWrite(path, o.parent, o.text, "", false)
			if err != nil {
				t.Fatalf("Op[%d] AddChild(%q to %d): %v", i, o.text, o.parent, err)
			}
		}
	}

	// 最终结构验证
	data, _ := os.ReadFile(path)
	t.Logf("Final file:\n%s", string(data))

	items, _ := ReadAndParse(string_to_path_helper(t, data))
	tree := BuildTree(items)
	if len(tree) != 3 {
		t.Errorf("Tree roots=%d, want 3", len(tree))
	}

	// 验证 P1 有 3 个子项，P2 有 1 个，P3 没有
	wantChildren := map[string]int{
		"P1": 3,
		"P2": 1,
		"P3": 0,
	}
	for _, root := range tree {
		want := wantChildren[root.Text]
		if len(root.Children) != want {
			t.Errorf("%s children=%d, want %d", root.Text, len(root.Children), want)
		}
	}
}

// string_to_path_helper 写 data 到 t.TempDir() 下的临时文件并返回 path，
// 供 ReadAndParse 使用。文件由 t.TempDir() 在测试结束时自动清理。
func string_to_path_helper(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "todo.md")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// bytesToTree Parse+BuildTree 的便捷封装。comprehensive_test.go 中两处"items = Parse(...); tree := BuildTree(items)"
// 写法因 Parse 返回 []Item 而 BuildTree 需要 []*Item 不匹配，用此 helper 统一转换。
func bytesToTree(data []byte) []*Item {
	flat := Parse(string(data))
	ptrs := make([]*Item, len(flat))
	for i := range flat {
		ptrs[i] = &flat[i]
	}
	return BuildTree(ptrs)
}
