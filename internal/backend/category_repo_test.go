package backend

import (
	"strings"
	"testing"
	"time"
)

// TestLinkCategoryRepo_Create 测试创建链接分类
func TestLinkCategoryRepo_Create(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	repo := NewLinkCategoryRepo(db)

	cat := &LinkCategory{
		ID:        "test-1",
		Name:      "工作",
		Icon:      "💼",
		SortOrder: 1,
		CreatedAt: time.Now(),
	}
	if err := repo.Create(cat); err != nil {
		t.Fatal(err)
	}

	list, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	// 应该至少有 1 个默认分类 + 我们刚加的
	if len(list) < 2 {
		t.Fatalf("List returned %d categories, want at least 2", len(list))
	}

	// 验证默认分类存在
	var foundDefault bool
	for _, c := range list {
		if c.IsDefault && c.ID == "default-link" {
			foundDefault = true
			break
		}
	}
	if !foundDefault {
		t.Error("default category not found")
	}
}

// TestLinkCategoryRepo_DeleteCannotDeleteDefault 不能删除默认分类
func TestLinkCategoryRepo_DeleteCannotDeleteDefault(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	repo := NewLinkCategoryRepo(db)

	err = repo.Delete("default-link")
	if err == nil {
		t.Fatal("should not be able to delete default category")
	}
	if !strings.Contains(err.Error(), "default") {
		t.Errorf("error = %q, want containing 'default'", err.Error())
	}
}

// TestLinkCategoryRepo_DeleteMovesItemsToDefault 删除分类时条目归入默认
func TestLinkCategoryRepo_DeleteMovesItemsToDefault(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	catRepo := NewLinkCategoryRepo(db)
	linkRepo := NewWebLinkRepo(db)

	// 创建分类 + 链接
	if err := catRepo.Create(&LinkCategory{ID: "temp", Name: "临时", SortOrder: 2, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := linkRepo.Create(&WebLink{ID: "l1", Name: "test", URL: "https://x.com", SortOrder: 1, CategoryID: "temp", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	// 删除分类
	if err := catRepo.Delete("temp"); err != nil {
		t.Fatal(err)
	}

	// 验证链接归入默认分类
	links, _ := linkRepo.List()
	var found bool
	for _, l := range links {
		if l.ID == "l1" {
			found = true
			if l.CategoryID != "default-link" {
				t.Errorf("link.CategoryID=%q, want default-link", l.CategoryID)
			}
		}
	}
	if !found {
		t.Error("link not found")
	}
}

// TestWebLinkRepo_DefaultCategory 链接未指定分类时归入默认
func TestWebLinkRepo_DefaultCategory(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	repo := NewWebLinkRepo(db)

	if err := repo.Create(&WebLink{ID: "l1", Name: "test", URL: "https://x.com", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	links, _ := repo.List()
	if len(links) != 1 {
		t.Fatalf("List returned %d, want 1", len(links))
	}
	if links[0].CategoryID != "default-link" {
		t.Errorf("default CategoryID=%q, want default-link", links[0].CategoryID)
	}
}

// TestWebLinkRepo_CategoryExplicit 指定分类时正常
func TestWebLinkRepo_CategoryExplicit(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	catRepo := NewLinkCategoryRepo(db)
	repo := NewWebLinkRepo(db)

	if err := catRepo.Create(&LinkCategory{ID: "work", Name: "工作", SortOrder: 1, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if err := repo.Create(&WebLink{ID: "l1", Name: "test", URL: "https://x.com", CategoryID: "work", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	links, _ := repo.List()
	if links[0].CategoryID != "work" {
		t.Errorf("CategoryID=%q, want work", links[0].CategoryID)
	}
}

// TestDirCategoryRepo_Create 测试创建目录分类
func TestDirCategoryRepo_Create(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	repo := NewDirCategoryRepo(db)

	cat := &DirCategory{
		ID:        "dev",
		Name:      "开发",
		Icon:      "💻",
		SortOrder: 1,
		CreatedAt: time.Now(),
	}
	if err := repo.Create(cat); err != nil {
		t.Fatal(err)
	}

	list, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 {
		t.Fatalf("List returned %d categories, want at least 2", len(list))
	}
}

// TestDirShortcutRepo_DefaultCategory 目录未指定分类时归入默认
func TestDirShortcutRepo_DefaultCategory(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	repo := NewDirShortcutRepo(db)

	if err := repo.Create(&DirShortcut{ID: "d1", Name: "test", Path: "/tmp", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	list, _ := repo.List()
	if len(list) != 1 {
		t.Fatalf("List returned %d, want 1", len(list))
	}
	if list[0].CategoryID != "default-dir" {
		t.Errorf("default CategoryID=%q, want default-dir", list[0].CategoryID)
	}
}

// TestSeedDefaultCategories_Idempotent 多次调用 seedDefaultCategories 幂等
func TestSeedDefaultCategories_Idempotent(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// 第二次调用不应创建重复
	if err := seedDefaultCategories(db); err != nil {
		t.Fatal(err)
	}
	linkCats, _ := NewLinkCategoryRepo(db).List()
	dirCats, _ := NewDirCategoryRepo(db).List()

	linkDefaultCount := 0
	for _, c := range linkCats {
		if c.IsDefault {
			linkDefaultCount++
		}
	}
	if linkDefaultCount != 1 {
		t.Errorf("link default count=%d, want 1", linkDefaultCount)
	}

	dirDefaultCount := 0
	for _, c := range dirCats {
		if c.IsDefault {
			dirDefaultCount++
		}
	}
	if dirDefaultCount != 1 {
		t.Errorf("dir default count=%d, want 1", dirDefaultCount)
	}
}
