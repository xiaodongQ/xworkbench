package todo

import (
	"os"
	"path/filepath"
	"reflect"
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
		text, due, tags := parseMetadata(tt.input)
		if text != tt.expText || due != tt.expDue || !reflect.DeepEqual(tags, tt.expTags) {
			t.Errorf("parseMetadata(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.input, text, due, tags, tt.expText, tt.expDue, tt.expTags)
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
	if err := AddAndWrite(p, "new item", "", nil, ""); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	want := "# h\n- [ ] a\n- [ ] new item\n"
	if string(data) != want {
		t.Errorf("got:  %q\nwant: %q", string(data), want)
	}
	// 空文本应报错
	if err := AddAndWrite(p, "  ", "", nil, ""); err == nil {
		t.Error("expected error for empty text")
	}
}

func TestAddAndWrite_WithMetadata(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(p, []byte("# h\n- [ ] a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := AddAndWrite(p, "购物", "2026-07-08", []string{"personal", "shopping"}, "记得带环保袋\n别忘卡"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	want := "# h\n- [ ] a\n- [ ] 购物 due:2026-07-08 tags:personal,shopping\n  > 记得带环保袋\n  > 别忘卡\n"
	if string(data) != want {
		t.Errorf("got:  %q\nwant: %q", string(data), want)
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
