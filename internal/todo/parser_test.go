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

func TestToggleAndWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "todo.md")
	original := "# h\n- [ ] a\n- [ ] b\n"
	if err := os.WriteFile(p, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	items, _ := ReadAndParse(p)
	items[0].Done = true
	if err := ToggleAndWrite(p, items); err != nil {
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
	if err := AddAndWrite(p, "new item"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	want := "# h\n- [ ] a\n- [ ] new item\n"
	if string(data) != want {
		t.Errorf("got:  %q\nwant: %q", string(data), want)
	}
	// 空文本应报错
	if err := AddAndWrite(p, "  "); err == nil {
		t.Error("expected error for empty text")
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
