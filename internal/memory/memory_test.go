package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_MemoryNotExist(t *testing.T) {
	tmp := t.TempDir()
	_, err := Load(filepath.Join(tmp, "nonexistent.md"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("Load nonexistent should return os.IsNotExist, got: %v", err)
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	content, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if content != "" {
		t.Errorf("empty file → empty content, got %q", content)
	}
}

func TestLoad_WithContent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	content := `# Memory — 最后更新: 2026-07-06

## 用户 & 环境
[2026-07-06] 用户时区 Asia/Shanghai`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("Load returned empty content")
	}
	if !strings.Contains(result, "用户时区") {
		t.Errorf("Load content mismatch, got: %s", result)
	}
}

func TestAdd_FirstEntry(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	store := New(path)

	result, err := store.Add("[2026-07-06] 用户时区 Asia/Shanghai", "用户 & 环境")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "✅") {
		t.Errorf("Add result should contain ✅, got: %s", result)
	}
	if store.Size() == 0 {
		t.Error("Size should be > 0 after Add")
	}
}

func TestAdd_ExactDuplicate(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	store := New(path)

	store.Add("[2026-07-06] 用户时区 Asia/Shanghai", "用户 & 环境")
	result, err := store.Add("[2026-07-06] 用户时区 Asia/Shanghai", "用户 & 环境")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "重复") {
		t.Errorf("exact duplicate should report duplicate, got: %s", result)
	}
}

func TestAdd_SameCategorySameDay(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	store := New(path)

	store.Add("[2026-07-06] 常用模型 minimax/MiniMax-M3", "用户 & 环境")
	result, err := store.Add("[2026-07-06] 常用模型 minimax/MiniMax-M3", "用户 & 环境")
	if err != nil {
		t.Fatal(err)
	}
	// Should be deduped
	if !strings.Contains(result, "重复") {
		t.Errorf("same entry same day same category should be deduped, got: %s", result)
	}
}

func TestAdd_DifferentCategory(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	store := New(path)

	store.Add("[2026-07-06] 飞书推送带日期", "约定")
	result, err := store.Add("[2026-07-06] 飞书推送带日期", "用户 & 环境")
	if err != nil {
		t.Fatal(err)
	}
	// Different category → allowed (content same but category different)
	if !strings.Contains(result, "✅") {
		t.Errorf("different category should allow, got: %s", result)
	}
}

func TestAdd_FileTooLarge_Rejects(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	// Pre-write a file that already exceeds hard limit (20KB)
	large := strings.Repeat("# 章节\n[2026-07-01] x\n", 2100)
	if err := os.WriteFile(path, []byte(large), 0644); err != nil {
		t.Fatal(err)
	}

	store := New(path)
	_, err := store.Add("[2026-07-06] 新条目", "测试")
	if err != ErrFileTooLarge {
		t.Errorf("Add to oversized file should return ErrFileTooLarge, got: %v", err)
	}
}

func TestAdd_WarningNearLimit(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	// Write content near 18KB warning threshold (≈18.4KB, soft=18KB, hard=20KB)
	// 230 lines × 82 chars ≈ 18.8KB
	line := "# 章节\n[2026-07-01] " + strings.Repeat("x", 65) + "\n"
	warning := strings.Repeat(line, 230)
	if err := os.WriteFile(path, []byte(warning), 0644); err != nil {
		t.Fatal(err)
	}

	store := New(path)
	if store.Size() <= 18*1024 {
		t.Fatalf("pre-write should exceed soft limit (18KB), got %d bytes", store.Size())
	}
	result, err := store.Add("新条目", "测试")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "⚠️") {
		t.Errorf("near-limit add should warn ⚠️, got: %s", result)
	}
}

func TestList(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	store := New(path)
	store.Add("[2026-07-06] 用户时区 Asia/Shanghai", "用户 & 环境")
	store.Add("[2026-07-06] 飞书推送带日期", "约定")

	list := store.List()
	if len(list) != 2 {
		t.Errorf("List length = %d, want 2", len(list))
	}
}

func TestList_Empty(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	store := New(path)

	list := store.List()
	if len(list) != 0 {
		t.Errorf("List on empty memory should be empty, got %d", len(list))
	}
}

func TestPrune(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	store := New(path)

	store.Add("[2026-07-06] 条目一", "测试")
	store.Add("[2026-07-06] 条目一", "测试") // duplicate
	store.Add("[2026-07-05] 旧条目", "测试")

	result, err := store.Prune()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "整合") && !strings.Contains(result, "dedup") {
		t.Errorf("Prune should mention dedup，整合, got: %s", result)
	}
	// After prune + dedup of exact duplicate, should have 2 entries
	list := store.List()
	if len(list) < 2 {
		t.Errorf("After prune should have at least 2 entries, got %d: %v", len(list), list)
	}
}

func TestSize(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	store := New(path)

	if store.Size() != 0 {
		t.Errorf("empty store size = 0, got %d", store.Size())
	}

	store.Add("[2026-07-06] 用户时区 Asia/Shanghai", "用户 & 环境")
	if store.Size() == 0 {
		t.Error("Size should be > 0 after Add")
	}
}

func TestContentForSystemPrompt(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	store := New(path)

	store.Add("[2026-07-06] 用户时区 Asia/Shanghai", "用户 & 环境")
	store.Add("[2026-07-06] 常用模型 minimax/MiniMax-M3", "用户 & 环境")

	prompt := store.ContentForSystemPrompt()
	if !strings.Contains(prompt, "用户时区") {
		t.Errorf("system prompt should contain memory content, got: %s", prompt)
	}
}

func TestAutoCreateSection(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	// Write a file without sections
	if err := os.WriteFile(path, []byte("旧内容\n"), 0644); err != nil {
		t.Fatal(err)
	}

	store := New(path)
	result, err := store.Add("[2026-07-06] 新条目", "测试分类")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "✅") {
		t.Errorf("Add should succeed on file without sections, got: %s", result)
	}

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "## 测试分类") {
		t.Errorf("Add should create section ## 测试分类, got: %s", string(content))
	}
}

func TestAdd_CategoryAlreadyExists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	content := `# Memory

## 用户 & 环境
[2026-07-05] 旧条目
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := New(path)
	result, err := store.Add("[2026-07-06] 新条目", "用户 & 环境")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "✅") {
		t.Errorf("Add to existing category should succeed, got: %s", result)
	}

	// Verify entry appears under existing section
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "[2026-07-06] 新条目") {
		t.Errorf("new entry should appear in file, got: %s", string(data))
	}
}

func TestContentForSystemPrompt_Empty(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	store := New(path)

	prompt := store.ContentForSystemPrompt()
	if prompt != "" {
		t.Errorf("empty memory → empty system prompt, got %q", prompt)
	}
}

func TestAdd_NewCategory(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	if err := os.WriteFile(path, []byte("# Memory\n\n## 用户 & 环境\n[2026-07-05] 旧\n"), 0644); err != nil {
		t.Fatal(err)
	}

	store := New(path)
	result, err := store.Add("[2026-07-06] 新分类条目", "约定")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "✅") {
		t.Errorf("Add new category should succeed, got: %s", result)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "## 约定") {
		t.Errorf("new category section should be created, got: %s", string(data))
	}
}

func TestNearLimit_20KB(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "memory.md")
	// Write at ~19KB: 10 lines of ~1900 bytes each
	line := "# 章节\n[2026-07-01] " + strings.Repeat("a", 1850) + "\n"
	near := strings.Repeat(line, 10)
	if err := os.WriteFile(path, []byte(near), 0644); err != nil {
		t.Fatal(err)
	}

	store := New(path)
	size := store.Size()
	if size < 18*1024 {
		t.Fatalf("pre-write should be near 19KB, got %d", size)
	}

	// Adding should warn (soft limit = 18KB)
	result, _ := store.Add("新条目", "测试")
	if !strings.Contains(result, "⚠️") {
		t.Errorf("Near 20KB add should warn ⚠️, got: %s", result)
	}
}

