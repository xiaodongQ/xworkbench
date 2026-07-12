package todo

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	in := `# header
- [ ] 写周报
  - [x] 子任务 A
- [X] 改 PR
not a todo
- [] 不合法
`
	got := Parse(in)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(got), got)
	}
	if got[0].Text != "写周报" || got[0].Done {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Indent != "  " || got[1].Text != "子任务 A" || !got[1].Done {
		t.Errorf("got[1] = %+v", got[1])
	}
	if got[2].Text != "改 PR" || !got[2].Done {
		t.Errorf("got[2] = %+v", got[2])
	}
}

func TestParseMetadata(t *testing.T) {
	tests := []struct {
		input   string
		expText string
		expDue  string
		expTags []string
	}{
		{"买牛奶 due:2026-07-08", "买牛奶", "2026-07-08", nil},
		{"购物 tags:personal,shopping", "购物", "", []string{"personal", "shopping"}},
		{"任务 due:2026-07-10 tags:work,urgent", "任务", "2026-07-10", []string{"work", "urgent"}},
		{"普通任务", "普通任务", "", nil},
	}
	for _, tt := range tests {
		text, due, tags, _, _ := parseFullMetadata(tt.input)
		if text != tt.expText || due != tt.expDue || !reflect.DeepEqual(tags, tt.expTags) {
			t.Errorf("parseFullMetadata(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.input, text, due, tags, tt.expText, tt.expDue, tt.expTags)
		}
	}
}

func TestParseNotes(t *testing.T) {
	in := `- [ ] 任务一
  > 这是备注第一行
  > 这是备注第二行
- [x] 任务二`

	items := Parse(in)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	wantNote := "这是备注第一行\n这是备注第二行"
	if items[0].Note != wantNote {
		t.Errorf("items[0].Note = %q, want %q", items[0].Note, wantNote)
	}
	if items[1].Note != "" {
		t.Errorf("items[1].Note = %q, want %q", items[1].Note, "")
	}
}

func TestParseNotes_NoNote(t *testing.T) {
	in := `- [ ] 任务一
- [x] 任务二`
	items := Parse(in)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Note != "" {
		t.Errorf("items[0].Note = %q, want empty", items[0].Note)
	}
	if items[1].Note != "" {
		t.Errorf("items[1].Note = %q, want empty", items[1].Note)
	}
}

func TestParseNotes_SingleLine(t *testing.T) {
	in := `- [ ] 任务一
  > 单行备注
- [x] 任务二`
	items := Parse(in)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Note != "单行备注" {
		t.Errorf("items[0].Note = %q, want %q", items[0].Note, "单行备注")
	}
}

func TestReadAndParse_NonExist(t *testing.T) {
	items, err := ReadAndParse("/nonexistent/file.md")
	if err != nil {
		t.Fatal(err)
	}
	if items != nil {
		t.Errorf("expected nil for non-existent, got %v", items)
	}
}

func TestBuildTree(t *testing.T) {
	items := []*Item{
		{LineNo: 1, Indent: "", Text: "任务1"},
		{LineNo: 2, Indent: "  ", Text: "子任务1.1"},
		{LineNo: 3, Indent: "    ", Text: "子任务1.1.1"},
		{LineNo: 4, Indent: "  ", Text: "子任务1.2"},
		{LineNo: 5, Indent: "", Text: "任务2"},
	}

	tree := BuildTree(items)

	if len(tree) != 2 {
		t.Fatalf("expected 2 root items, got %d", len(tree))
	}
	if len(tree[0].Children) != 2 {
		t.Fatalf("expected 2 children for task1, got %d", len(tree[0].Children))
	}
	if len(tree[0].Children[0].Children) != 1 {
		t.Fatalf("expected 1 child for subtask 1.1, got %d", len(tree[0].Children[0].Children))
	}
	if tree[0].Text != "任务1" {
		t.Errorf("expected first root '任务1', got %q", tree[0].Text)
	}
	if tree[0].Children[0].Text != "子任务1.1" {
		t.Errorf("expected first child '子任务1.1', got %q", tree[0].Children[0].Text)
	}
	if tree[0].Children[0].Children[0].Text != "子任务1.1.1" {
		t.Errorf("expected grandchild '子任务1.1.1', got %q", tree[0].Children[0].Children[0].Text)
	}
	if tree[0].Children[1].Text != "子任务1.2" {
		t.Errorf("expected second child '子任务1.2', got %q", tree[0].Children[1].Text)
	}
	if tree[1].Text != "任务2" {
		t.Errorf("expected second root '任务2', got %q", tree[1].Text)
	}
	if len(tree[1].Children) != 0 {
		t.Errorf("expected no children for task2, got %d", len(tree[1].Children))
	}
}

func TestBuildTree_Empty(t *testing.T) {
	if got := BuildTree(nil); got != nil {
		t.Errorf("BuildTree(nil) = %v, want nil", got)
	}
	if got := BuildTree([]*Item{}); len(got) != 0 {
		t.Errorf("BuildTree([]) len = %d, want 0", len(got))
	}
}

func TestBuildTree_SingleLevel(t *testing.T) {
	// 全部同级，应全部为根，Children 为空
	items := []*Item{
		{LineNo: 1, Indent: "", Text: "a"},
		{LineNo: 2, Indent: "", Text: "b"},
		{LineNo: 3, Indent: "", Text: "c"},
	}
	tree := BuildTree(items)
	if len(tree) != 3 {
		t.Fatalf("expected 3 roots, got %d", len(tree))
	}
	for i, it := range tree {
		if len(it.Children) != 0 {
			t.Errorf("tree[%d].Children len = %d, want 0", i, len(it.Children))
		}
	}
}

func TestReadAndParse_BuildsTree(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	content := "- [ ] 父任务1\n  - [ ] 子任务1.1\n    - [ ] 孙任务\n  - [x] 子任务1.2\n- [ ] 父任务2\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tree, err := ReadAndParse(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(tree))
	}
	if tree[0].Text != "父任务1" || len(tree[0].Children) != 2 {
		t.Errorf("tree[0] = %+v, want 父任务1 with 2 children", tree[0])
	}
	if tree[0].Children[0].Text != "子任务1.1" || len(tree[0].Children[0].Children) != 1 {
		t.Errorf("tree[0].Children[0] = %+v", tree[0].Children[0])
	}
	if tree[0].Children[0].Children[0].Text != "孙任务" {
		t.Errorf("grandchild text = %q, want 孙任务", tree[0].Children[0].Children[0].Text)
	}
	if !tree[0].Children[1].Done {
		t.Errorf("子任务1.2 should be done")
	}
}

func TestFlatten(t *testing.T) {
	items := []*Item{
		{LineNo: 1, Indent: "", Text: "a"},
		{LineNo: 2, Indent: "  ", Text: "a1"},
		{LineNo: 3, Indent: "    ", Text: "a1a"},
		{LineNo: 4, Indent: "  ", Text: "a2"},
		{LineNo: 5, Indent: "", Text: "b"},
	}
	tree := BuildTree(items)
	flat := Flatten(tree)
	if len(flat) != 5 {
		t.Fatalf("Flatten len = %d, want 5", len(flat))
	}
	want := []string{"a", "a1", "a1a", "a2", "b"}
	for i, w := range want {
		if flat[i].Text != w {
			t.Errorf("flat[%d].Text = %q, want %q", i, flat[i].Text, w)
		}
	}
}

func TestToggleAndWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	original := "# h\n- [ ] a\n- [ ] b\n"
	if err := os.WriteFile(p, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	items, _ := ReadAndParse(p)
	flat := Flatten(items)
	flat[0].Done = true
	if err := ToggleAndWrite(p, flat); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	got := string(data)
	want := "# h\n- [x] a\n- [ ] b\n"
	if got != want {
		t.Errorf("\ngot:  %q\nwant: %q", got, want)
	}
}

func TestAddAndWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(p, []byte("# h\n- [ ] a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := AddAndWrite(p, "new item", "", nil, ""); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	// 新项会带有 created: 日期
	if !strings.Contains(string(data), "- [ ] new item created:") {
		t.Errorf("got:  %q\nwant to contain: %q", string(data), "- [ ] new item created:")
	}
	// 空文本应报错
	if _, err := AddAndWrite(p, "  ", "", nil, ""); err == nil {
		t.Error("expected error for empty text")
	}
}

func TestAddAndWrite_WithMetadata(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(p, []byte("# h\n- [ ] a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := AddAndWrite(p, "购物", "2026-07-08", []string{"personal", "shopping"}, "记得带环保袋\n别忘卡"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	// 检查关键内容，新项会带有 created: 日期
	if !strings.Contains(string(data), "due:2026-07-08") {
		t.Errorf("got:  %q\nwant to contain: %q", string(data), "due:2026-07-08")
	}
	if !strings.Contains(string(data), "tags:personal,shopping") {
		t.Errorf("got:  %q\nwant to contain: %q", string(data), "tags:personal,shopping")
	}
	if !strings.Contains(string(data), "created:") {
		t.Errorf("got:  %q\nwant to contain: %q", string(data), "created:")
	}
}

func TestAddAndWrite_WithArchive(t *testing.T) {
	// 验证新项插入到活跃区（分隔线之前），而不是归档区
	content := `# Todo
- [ ] 活跃任务

--- archived (must exist for archived, do not delete) ---

## 📦 已归档
- [x] 归档任务 archived:2026-07-01
`
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	lineNo, err := AddAndWrite(p, "新任务", "", nil, "")
	if err != nil {
		t.Fatalf("AddAndWrite error: %v", err)
	}

	data, _ := os.ReadFile(p)
	sections, err := ParseSections(string(data))
	if err != nil {
		t.Fatalf("ParseSections error: %v", err)
	}

	// 新任务应该在活跃区，不在归档区
	if len(sections.ActiveItems) != 2 {
		t.Errorf("ActiveItems = %d, want 2", len(sections.ActiveItems))
	}
	if len(sections.ArchivedItems) != 1 {
		t.Errorf("ArchivedItems = %d, want 1", len(sections.ArchivedItems))
	}
	// 新任务的 line_no 应该在活跃区（分隔线之前，空行保留，故第4行）
	if lineNo != 4 {
		t.Errorf("new item lineNo = %d, want 4 (before separator)", lineNo)
	}
	// 验证新任务文本
	found := false
	for _, item := range sections.ActiveItems {
		if item.Text == "新任务" {
			found = true
			break
		}
	}
	if !found {
		t.Error("new item should be in active section")
	}
}

func TestItemToLine_PreservesMetadata(t *testing.T) {
	tests := []struct {
		name string
		item *Item
		want string
	}{
		{
			name: "plain unchecked",
			item: &Item{Text: "购物"},
			want: "- [ ] 购物",
		},
		{
			name: "checked",
			item: &Item{Text: "购物", Done: true},
			want: "- [x] 购物",
		},
		{
			name: "with due",
			item: &Item{Text: "购物", DueDate: "2026-07-08"},
			want: "- [ ] 购物 due:2026-07-08",
		},
		{
			name: "with tags",
			item: &Item{Text: "购物", Tags: []string{"personal", "shopping"}},
			want: "- [ ] 购物 tags:personal,shopping",
		},
		{
			name: "with due and tags",
			item: &Item{Text: "任务", Done: true, DueDate: "2026-07-10", Tags: []string{"work", "urgent"}},
			want: "- [x] 任务 due:2026-07-10 tags:work,urgent",
		},
		{
			name: "indented",
			item: &Item{Indent: "  ", Text: "子任务", Done: true, Tags: []string{"sub"}},
			want: "  - [x] 子任务 tags:sub",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := itemToLine(tt.item)
			if got != tt.want {
				t.Errorf("itemToLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToggleAndWrite_PreservesMetadata(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	original := "# h\n- [ ] 购物 due:2026-07-08 tags:personal\n- [ ] 任务 tags:work\n"
	if err := os.WriteFile(p, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	items, _ := ReadAndParse(p)
	flat := Flatten(items)
	// 标记第一个为 done
	for i := range flat {
		if flat[i].LineNo == 2 {
			flat[i].Done = true
		}
	}
	if err := ToggleAndWrite(p, flat); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	want := "# h\n- [x] 购物 due:2026-07-08 tags:personal\n- [ ] 任务 tags:work\n"
	if string(data) != want {
		t.Errorf("got:  %q\nwant: %q", string(data), want)
	}
}

func TestDeleteAndWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(p, []byte("# h\n- [ ] a\n- [ ] b\n- [ ] c\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := DeleteAndWrite(p, 2); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	want := "# h\n- [ ] b\n- [ ] c\n"
	if string(data) != want {
		t.Errorf("got:  %q\nwant: %q", string(data), want)
	}
	// 越界
	if err := DeleteAndWrite(p, 99); err == nil {
		t.Error("expected error for out-of-range")
	}
}

func TestAddChildAndWrite(t *testing.T) {
	// Case 1: root item followed by blank lines — child must appear exactly once
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(p, []byte("- [ ] P\n\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := AddChildAndWrite(p, 1, "child1", "", false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	// Must NOT have child1 twice; exactly one child entry
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "child1") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("child1 appears %d times, want exactly 1: %q", count, data)
	}

	// Case 2: add second child to same parent (lineNo=1 still works after file changed)
	// parent line_no=1, child1 is now line_no=2
	if err := AddChildAndWrite(p, 1, "child2", "", false); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(p)
	count1, count2 := 0, 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "child1") {
			count1++
		}
		if strings.Contains(line, "child2") {
			count2++
		}
	}
	if count1 != 1 || count2 != 1 {
		t.Errorf("child1=%d child2=%d, want each 1: %q", count1, count2, data)
	}

	// Case 3: add child to existing child (nested), file has no blank lines
	p2 := filepath.Join(dir, "todo2.md")
	if err := os.WriteFile(p2, []byte("- [ ] P\n  - [ ] child1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tree2, _ := ReadAndParse(p2)
	flat2 := Flatten(tree2)
	// flat2[1] is child1 with line_no=2
	if err := AddChildAndWrite(p2, flat2[1].LineNo, "child1a", "", false); err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(p2)
	countChild1a := 0
	for _, line := range strings.Split(string(data2), "\n") {
		if strings.Contains(line, "child1a") {
			countChild1a++
		}
	}
	if countChild1a != 1 {
		t.Errorf("child1a appears %d times, want 1: %q", countChild1a, data2)
	}

	// Case 4: invalid parent line_no
	if err := AddChildAndWrite(p, 99, "orphan", "", false); err == nil {
		t.Error("expected error for invalid parent line_no")
	}
}

func TestDeleteAndWriteWithChildren(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	// 父项 + 2个子项（含孙子项） + 另一个父项
	content := `- [ ] 父任务
  - [ ] 子任务1
    - [ ] 孙子任务
  - [x] 子任务2
- [ ] 另一个任务
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// 删除"父任务"（line_no=1），应级联删除所有子项
	if err := DeleteAndWrite(p, 1); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	// 只剩"另一个任务"
	if !strings.Contains(string(data), "另一个任务") {
		t.Errorf("expected '另一个任务' to remain, got: %q", data)
	}
	// 子任务/孙子任务不应存在
	if strings.Contains(string(data), "子任务1") {
		t.Errorf("child '子任务1' should be deleted")
	}
	if strings.Contains(string(data), "孙子任务") {
		t.Errorf("grandchild '孙子任务' should be deleted")
	}
}

func TestDeleteAndWriteChildOnly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	content := `- [ ] 父任务
  - [ ] 子任务1
  - [ ] 子任务2
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// 只删除"子任务1"（line_no=2），不碰父任务和其他
	if err := DeleteAndWrite(p, 2); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	// 父任务和子任务2应保留
	if !strings.Contains(string(data), "父任务") {
		t.Errorf("'父任务' should remain")
	}
	if !strings.Contains(string(data), "子任务2") {
		t.Errorf("'子任务2' should remain")
	}
	// 子任务1已删除
	if strings.Contains(string(data), "子任务1") {
		t.Errorf("'子任务1' should be deleted")
	}
}

// TestAddAndWrite_ConsecutiveReturnsCorrectLineNo 验证连续两次 AddAndWrite
// 返回的 line_no 始终等于新 item 在文件里的真实 1-based 行号（不依赖算式 / 边界巧合）。
// 这个用例之前无法通过，因为 AddAndWrite 有分隔线分支返回 separatorIdx（0-based），
// 当 separator 前的空行被前一次写入"吃掉"后，从第二次起就会持续 off-by-one。
func TestAddAndWrite_ConsecutiveReturnsCorrectLineNo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	// xworkbench 标准模板：分隔线前恰好一个空行
	initial := "- [ ] Old\n\n--- archived (must exist for archived, do not delete) ---\n\n## 📦 Archived\n- [x] X archived:2026-07-01\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	// 第一次：碰巧正确（off-by-one 在边界刚好被吞空行"对齐"）
	ln1, err := AddAndWrite(path, "First", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if ln1 != 3 {
		t.Errorf("first add: line_no = %d, want 3", ln1)
	}

	// 第二次：新项插入活跃区末尾，空行保留
	ln2, err := AddAndWrite(path, "Second", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if ln2 != 4 {
		t.Errorf("second add: line_no = %d, want 4", ln2)
	}

	// 第三次再压一次，确认多次连续都正确
	ln3, err := AddAndWrite(path, "Third", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if ln3 != 5 {
		t.Errorf("third add: line_no = %d, want 5", ln3)
	}

	// 给"Second"加一个子项 —— 这是用户报告的精确路径：
	//   前端拿 ln2 当 parentLineNo 调 AddChildAndWrite，子项必须挂在"Second"下。
	if err := AddChildAndWrite(path, ln2, "sub-of-second", "", false); err != nil {
		t.Fatal(err)
	}

	items, err := ReadAndParse(path)
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildTree(items)

	// 期望：3 个顶级项，Second 只有 1 个子项 sub-of-second
	var secondNode *Item
	for _, root := range tree {
		if root.Text == "Second" {
			secondNode = root
			break
		}
	}
	if secondNode == nil {
		t.Fatalf("Second item not found in tree: %+v", tree)
	}
	if len(secondNode.Children) != 1 || secondNode.Children[0].Text != "sub-of-second" {
		t.Errorf("Second should have exactly 1 child 'sub-of-second', got: %+v", secondNode.Children)
	}
}

// TestAddAndWrite_NoSeparator 返回真实行号（避免末尾无换行时的 off-by-one）。
func TestAddAndWrite_NoSeparator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(path, []byte("# h\n- [ ] a\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 单条追加
	ln1, err := AddAndWrite(path, "new1", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if ln1 != 3 {
		t.Errorf("first add (no separator): line_no = %d, want 3", ln1)
	}

	// 连续追加，第二条应在第 4 行
	ln2, err := AddAndWrite(path, "new2", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if ln2 != 4 {
		t.Errorf("second add (no separator): line_no = %d, want 4", ln2)
	}
}
