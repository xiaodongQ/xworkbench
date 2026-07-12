package todo

// 这个测试是 USER 报告场景的精确复现（连同 - [ ] 1 / - [x] 2 / 3 / 4 + 测试新增2），用来最终验证修复效果。
// 验证通过后可删。保留以防未来回归。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2E_UserReportedScenario(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	// 仅归档区，活跃区为空（避免 "Old" 这种预设项干扰根计数断言）
	initial := "\n--- archived (must exist for archived, do not delete) ---\n\n## 📦 Archived\n- [x] X archived:2026-07-01\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	t.Logf("初始文件:\n%s", initial)

	// 1. 添加"测试新增"
	parentLn, err := AddAndWrite(path, "测试新增", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("1. AddAndWrite('测试新增') → lineNo=%d", parentLn)
	if parentLn != 1 {
		t.Errorf("first add: line_no = %d, want 1 (空活跃区新项到第一行)", parentLn)
	}

	// 2. 添加子项 1, 2
	c1Ln := mustAddChild(t, path, parentLn, "1")
	_ = c1Ln
	c2Ln := mustAddChild(t, path, parentLn, "2", true /* done */)

	// 3. 给"2"加孙项 3, 4
	gcLn1 := mustAddChild(t, path, c2Ln, "3")
	_ = gcLn1
	gcLn2 := mustAddChild(t, path, c2Ln, "4")

	// 4. 添加"测试新增2"
	parentLn2, err := AddAndWrite(path, "测试新增2", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("4. AddAndWrite('测试新增2') → lineNo=%d", parentLn2)
	// 孙项"4"在 line 5（gcLn2=5）。下一次 AddAndWrite 紧接活跃区尾，空行保留在分隔线前。
	if wantLn := gcLn2 + 1; parentLn2 != wantLn {
		t.Errorf("lineNo 应该是 %d（孙项 4 之后 1 行），但返回 %d", wantLn, parentLn2)
	}

	// 5. 给"测试新增2"加子项
	mustAddChild(t, path, parentLn2, "sub-1")
	mustAddChild(t, path, parentLn2, "sub-2")

	data, _ := os.ReadFile(path)
	t.Logf("最终文件:\n%s", string(data))

	items, _ := ReadAndParseSections(path)
	t.Logf("活跃区有 %d 个根节点", len(items.ActiveItems))
	for _, root := range items.ActiveItems {
		t.Logf("  Root[%q] (line=%d) → %d 子项:", root.Text, root.LineNo, len(root.Children))
		for _, c := range root.Children {
			marker := ""
			if c.Text == "2" {
				marker = " ← c2"
			}
			t.Logf("    - [%q] (line=%d)%s", c.Text, c.LineNo, marker)
			for _, gc := range c.Children {
				t.Logf("      · [%q] (line=%d)", gc.Text, gc.LineNo)
			}
		}
	}

	// 断言：结构正确（仅数活跃区，排除归档项干扰）
	if len(items.ActiveItems) != 2 {
		t.Fatalf("活跃区期望 2 个根（测试新增 + 测试新增2），实际 %d", len(items.ActiveItems))
	}
	for _, root := range items.ActiveItems {
		switch root.Text {
		case "测试新增":
			if len(root.Children) != 2 {
				t.Errorf("测试新增 应有 2 子项，实际 %d", len(root.Children))
			}
			if len(root.Children) >= 2 {
				c2 := root.Children[1]
				if c2.Text != "2" {
					t.Errorf("第 2 子项期望 '2'，实际 %q", c2.Text)
				}
				if len(c2.Children) != 2 {
					t.Errorf("'2' 应有 2 孙项，实际 %d", len(c2.Children))
				}
				wantGCs := []string{"3", "4"}
				for i, gc := range c2.Children {
					if gc.Text != wantGCs[i] {
						t.Errorf("孙项 [%d] 期望 %q, 实际 %q", i, wantGCs[i], gc.Text)
					}
				}
			}
		case "测试新增2":
			if len(root.Children) != 2 {
				t.Errorf("测试新增2 应有 2 子项，实际 %d", len(root.Children))
			}
			wantSubs := []string{"sub-1", "sub-2"}
			for i, c := range root.Children {
				if c.Text != wantSubs[i] {
					t.Errorf("测试新增2 第 %d 子项期望 %q, 实际 %q", i, wantSubs[i], c.Text)
				}
			}
		default:
			t.Errorf("未知根: %q", root.Text)
		}
	}
}

func mustAddChild(t *testing.T, path string, parentLn int, text string, done ...bool) int {
	t.Helper()
	isDone := false
	if len(done) > 0 {
		isDone = done[0]
	}
	if err := AddChildAndWrite(path, parentLn, text, "", isDone); err != nil {
		t.Fatalf("AddChild %q at line %d: %v", text, parentLn, err)
	}
	// 用 Parse 按 text 精确匹配，避免 strings.Contains 把 "2026" 也命中 "2"。
	data, _ := os.ReadFile(path)
	flat := Parse(string(data))
	for _, it := range flat {
		if it.Text == text {
			return it.LineNo
		}
	}
	t.Fatalf("can't find %q in updated file", text)
	return 0
}

// 防 "_ = strings" / "_ = fmt" 被 linter 标
var _ = fmt.Sprintf
var _ = strings.TrimSpace
