package todo

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseSections(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantActive  int
		wantArchived int
	}{
		{
			name: "无分隔线，全部为活跃区",
			content: `# Todo
- [ ] 任务A
- [x] 任务B
`,
			wantActive:   2,
			wantArchived: 0,
		},
		{
			name: "有分隔线，分开活跃区和归档区",
			content: `# Todo
- [ ] 活跃任务

---

## 📦 已归档
- [x] 归档任务
`,
			wantActive:   1,
			wantArchived: 1,
		},
		{
			name: "分隔线在中间",
			content: `- [ ] 任务A
- [x] 任务B

---

## 📦 已归档
- [x] 归档A
`,
			wantActive:   2,
			wantArchived: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sections, err := ParseSections(tt.content)
			if err != nil {
				t.Fatalf("ParseSections error: %v", err)
			}
			if len(sections.ActiveItems) != tt.wantActive {
				t.Errorf("ActiveItems len = %d, want %d", len(sections.ActiveItems), tt.wantActive)
			}
			if len(sections.ArchivedItems) != tt.wantArchived {
				t.Errorf("ArchivedItems len = %d, want %d", len(sections.ArchivedItems), tt.wantArchived)
			}
		})
	}
}

func TestParseFullMetadata(t *testing.T) {
	tests := []struct {
		input          string
		expText       string
		expDue        string
		expTags       []string
		expCreated    string
		expArchived   string
	}{
		{"任务 created:2026-07-01", "任务", "", nil, "2026-07-01", ""},
		{"任务 archived:2026-07-11", "任务", "", nil, "", "2026-07-11"},
		{"任务 created:2026-07-01 archived:2026-07-11", "任务", "", nil, "2026-07-01", "2026-07-11"},
		{"任务 due:2026-07-15 created:2026-07-01 archived:2026-07-11 tags:work", "任务", "2026-07-15", []string{"work"}, "2026-07-01", "2026-07-11"},
		{"普通任务", "普通任务", "", nil, "", ""},
	}
	for _, tt := range tests {
		text, due, tags, created, archived := parseFullMetadata(tt.input)
		if text != tt.expText {
			t.Errorf("parseFullMetadata(%q) text = %q, want %q", tt.input, text, tt.expText)
		}
		if due != tt.expDue {
			t.Errorf("parseFullMetadata(%q) due = %q, want %q", tt.input, due, tt.expDue)
		}
		if !reflect.DeepEqual(tags, tt.expTags) {
			t.Errorf("parseFullMetadata(%q) tags = %v, want %v", tt.input, tags, tt.expTags)
		}
		if created != tt.expCreated {
			t.Errorf("parseFullMetadata(%q) created = %q, want %q", tt.input, created, tt.expCreated)
		}
		if archived != tt.expArchived {
			t.Errorf("parseFullMetadata(%q) archived = %q, want %q", tt.input, archived, tt.expArchived)
		}
	}
}

func strSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestArchiveItem(t *testing.T) {
	t.Run("归档顶级项", func(t *testing.T) {
		content := `# Todo
- [ ] 任务A due:2026-07-15
- [x] 任务B due:2026-07-10
- [ ] 任务C
`
		path := filepath.Join(t.TempDir(), "todo.md")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// 任务B 的 lineNo = 3
		if err := ArchiveItem(path, 3); err != nil {
			t.Fatalf("ArchiveItem error: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		sections, err := ParseSections(string(data))
		if err != nil {
			t.Fatal(err)
		}
		if len(sections.ActiveItems) != 2 {
			t.Errorf("ActiveItems len = %d, want 2", len(sections.ActiveItems))
		}
		if len(sections.ArchivedItems) != 1 {
			t.Errorf("ArchivedItems len = %d, want 1", len(sections.ArchivedItems))
		}

		archived := sections.ArchivedItems[0]
		if archived.Archived == "" {
			t.Error("archived item should have Archived field set")
		}
		if archived.Text != "任务B" {
			t.Errorf("archived text = %q, want %q", archived.Text, "任务B")
		}
	})

	t.Run("归档含子项的顶级项", func(t *testing.T) {
		content := `# Todo
- [ ] 任务A
- [x] 任务B
  - [x] 子任务B1
  - [ ] 子任务B2
- [ ] 任务C
`
		path := filepath.Join(t.TempDir(), "todo.md")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// 任务B 的 lineNo = 3
		if err := ArchiveItem(path, 3); err != nil {
			t.Fatalf("ArchiveItem error: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		sections, err := ParseSections(string(data))
		if err != nil {
			t.Fatal(err)
		}
		if len(sections.ActiveItems) != 2 {
			t.Errorf("ActiveItems len = %d, want 2", len(sections.ActiveItems))
		}
		// 归档项及其子项都在 ArchivedItems 中（树结构，根节点=1，子节点=2）
		if len(sections.ArchivedItems) != 1 {
			t.Errorf("ArchivedItems roots = %d, want 1", len(sections.ArchivedItems))
		}
		// 用 Flatten 验证总项数（含子项）
		archivedFlat := Flatten(sections.ArchivedItems)
		if len(archivedFlat) != 3 {
			t.Errorf("ArchivedItems (flattened) = %d, want 3", len(archivedFlat))
		}
	})

	t.Run("子项不能单独归档", func(t *testing.T) {
		content := `# Todo
- [x] 任务B
  - [x] 子任务B1
`
		path := filepath.Join(t.TempDir(), "todo.md")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// 子任务B1 的 lineNo = 3
		err := ArchiveItem(path, 3)
		if err == nil {
			t.Error("ArchiveItem should error for child item")
		}
		if !strings.Contains(err.Error(), "子项不能单独归档") {
			t.Errorf("error = %q, want containing %q", err.Error(), "子项不能单独归档")
		}
	})

	t.Run("归档到已有分隔线的文件", func(t *testing.T) {
		// 已有归档项有 archived 日期，归档新项后，两个归档项各在一个月份组（2个根）
		content := `# Todo
- [ ] 任务A

---

## 📦 已归档

### 2026年06月
- [x] 已有归档 archived:2026-06-20
`
		path := filepath.Join(t.TempDir(), "todo.md")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// 任务A 的 lineNo = 2
		if err := ArchiveItem(path, 2); err != nil {
			t.Fatalf("ArchiveItem error: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		sections, err := ParseSections(string(data))
		if err != nil {
			t.Fatal(err)
		}
		if len(sections.ActiveItems) != 0 {
			t.Errorf("ActiveItems len = %d, want 0", len(sections.ActiveItems))
		}
		// 两个归档项在不同月份组，所以2个根
		if len(sections.ArchivedItems) != 2 {
			t.Errorf("ArchivedItems roots = %d, want 2", len(sections.ArchivedItems))
		}
		archivedFlat := Flatten(sections.ArchivedItems)
		if len(archivedFlat) != 2 {
			t.Errorf("ArchivedItems (flattened) = %d, want 2", len(archivedFlat))
		}
	})
}

func TestWriteSections(t *testing.T) {
	t.Run("写入活跃区和归档区", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "todo.md")

		active := []*Item{
			{Text: "任务A", Done: false, Created: "2026-07-01"},
			{Text: "任务B", Done: true, Created: "2026-06-15"},
		}
		archived := []*Item{
			{Text: "归档A", Done: true, Created: "2026-06-01", Archived: "2026-07-01"},
			{Text: "归档B", Done: true, Created: "2026-05-01", Archived: "2026-06-20"},
		}

		if err := WriteSections(path, active, archived); err != nil {
			t.Fatalf("WriteSections error: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		// 验证分隔线存在
		if !strings.Contains(string(data), "---") {
			t.Error("file should contain separator ---")
		}
		// 验证活跃区标题
		if !strings.Contains(string(data), "## 📋 活跃中") {
			t.Error("file should contain active section title")
		}
		// 验证归档区标题
		if !strings.Contains(string(data), "## 📦 已归档") {
			t.Error("file should contain archived section title")
		}
		// 验证月份分组
		if !strings.Contains(string(data), "### 2026年07月") {
			t.Error("file should contain month group for July 2026")
		}
	})

	t.Run("只写活跃区（无归档）", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "todo.md")

		active := []*Item{
			{Text: "任务A", Done: false, Created: "2026-07-01"},
		}

		if err := WriteSections(path, active, []*Item{}); err != nil {
			t.Fatalf("WriteSections error: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		// 有活跃项时也应该有活跃区标题和分隔线
		if !strings.Contains(string(data), "## 📋 活跃中") {
			t.Error("file should contain active section title")
		}
		if !strings.Contains(string(data), "---") {
			t.Error("file should contain separator")
		}
	})
}

func TestItemToLine(t *testing.T) {
	t.Run("包含所有元数据", func(t *testing.T) {
		item := &Item{
			Indent:   "",
			Done:     true,
			Text:     "任务A",
			DueDate:  "2026-07-15",
			Tags:     []string{"work", "urgent"},
			Created:  "2026-07-01",
			Archived: "2026-07-11",
		}
		line := itemToLine(item)
		if !strings.Contains(line, "due:2026-07-15") {
			t.Errorf("line = %q, want containing due:2026-07-15", line)
		}
		if !strings.Contains(line, "tags:work,urgent") {
			t.Errorf("line = %q, want containing tags:work,urgent", line)
		}
		if !strings.Contains(line, "created:2026-07-01") {
			t.Errorf("line = %q, want containing created:2026-07-01", line)
		}
		if !strings.Contains(line, "archived:2026-07-11") {
			t.Errorf("line = %q, want containing archived:2026-07-11", line)
		}
		if !strings.Contains(line, "[x]") {
			t.Errorf("line = %q, want containing [x]", line)
		}
	})
}
