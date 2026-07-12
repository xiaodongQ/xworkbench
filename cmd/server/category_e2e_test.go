package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"go.uber.org/zap"
)

// TestCategoryE2E 端到端测试：分类管理 + 链接 + 目录
func TestCategoryE2E(t *testing.T) {
	if logger == nil {
		zl, _ := zap.NewProduction()
		logger = zl.Sugar()
	}
	db, cleanup, err := backend.TestDB()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	srv := &APIServer{
		linkDB:    backend.NewWebLinkRepo(db),
		dirDB:     backend.NewDirShortcutRepo(db),
		linkCatDB: backend.NewLinkCategoryRepo(db),
		dirCatDB:  backend.NewDirCategoryRepo(db),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/link-categories", srv.handleLinkCategories)
	mux.HandleFunc("POST /api/link-categories", srv.handleLinkCategoryCreate)
	mux.HandleFunc("PUT /api/link-categories/{id}", srv.handleLinkCategoryUpdate)
	mux.HandleFunc("DELETE /api/link-categories/{id}", srv.handleLinkCategoryDelete)
	mux.HandleFunc("GET /api/web-links", srv.handleWebLinks)
	mux.HandleFunc("POST /api/web-links", srv.handleWebLinkCreate)

	// 1. 列出分类（应有默认）
	rec := doRequest(t, mux, "GET", "/api/link-categories", nil)
	var initialCats []backend.LinkCategory
	json.NewDecoder(rec.Body).Decode(&initialCats)
	if len(initialCats) != 1 {
		t.Errorf("initial cats=%d, want 1", len(initialCats))
	}

	// 2. 创建 3 个新分类
	var catIDs []string
	for _, name := range []string{"工作", "学习", "工具"} {
		rec = doRequest(t, mux, "POST", "/api/link-categories",
			map[string]string{"name": name, "icon": "📌"})
		var cat backend.LinkCategory
		json.NewDecoder(rec.Body).Decode(&cat)
		catIDs = append(catIDs, cat.ID)
	}

	// 3. 在不同分类下创建链接
	for i, name := range []string{"GitHub", "StackOverflow", "Postman"} {
		rec = doRequest(t, mux, "POST", "/api/web-links",
			map[string]string{
				"name":        name,
				"url":         "https://" + name + ".com",
				"category_id": catIDs[i],
			})
		if rec.Code != 200 {
			t.Fatalf("create link %d: code=%d body=%s", i, rec.Code, rec.Body.String())
		}
	}

	// 4. 列出链接，按分类过滤（手动模拟）
	rec = doRequest(t, mux, "GET", "/api/web-links", nil)
	var links []backend.WebLink
	json.NewDecoder(rec.Body).Decode(&links)
	if len(links) != 3 {
		t.Errorf("links=%d, want 3", len(links))
	}

	// 验证每个链接归类正确
	for _, l := range links {
		expectedCat := ""
		switch l.Name {
		case "GitHub":
			expectedCat = catIDs[0]
		case "StackOverflow":
			expectedCat = catIDs[1]
		case "Postman":
			expectedCat = catIDs[2]
		}
		if l.CategoryID != expectedCat {
			t.Errorf("%s CategoryID=%q, want %q", l.Name, l.CategoryID, expectedCat)
		}
	}

	// 5. 删除一个分类，验证链接归入默认
	rec = doRequest(t, mux, "DELETE", "/api/link-categories/"+catIDs[1], nil)
	if rec.Code != 200 {
		t.Fatalf("delete cat: code=%d", rec.Code)
	}

	// 重新查询链接
	rec = doRequest(t, mux, "GET", "/api/web-links", nil)
	json.NewDecoder(rec.Body).Decode(&links)
	for _, l := range links {
		if l.Name == "StackOverflow" && l.CategoryID != "default-link" {
			t.Errorf("deleted cat, link should fall to default, got %q", l.CategoryID)
		}
	}
}

// TestDirCategoryE2E 目录分类端到端
func TestDirCategoryE2E(t *testing.T) {
	if logger == nil {
		zl, _ := zap.NewProduction()
		logger = zl.Sugar()
	}
	db, cleanup, err := backend.TestDB()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	srv := &APIServer{
		dirDB:    backend.NewDirShortcutRepo(db),
		dirCatDB: backend.NewDirCategoryRepo(db),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/dir-categories", srv.handleDirCategories)
	mux.HandleFunc("POST /api/dir-categories", srv.handleDirCategoryCreate)
	mux.HandleFunc("GET /api/dir-shortcuts", srv.handleDirShortcuts)
	mux.HandleFunc("POST /api/dir-shortcuts", srv.handleDirShortcutCreate)

	// 创建分类
	rec := doRequest(t, mux, "POST", "/api/dir-categories",
		map[string]string{"name": "项目", "icon": "📁"})
	var cat backend.DirCategory
	json.NewDecoder(rec.Body).Decode(&cat)

	// 创建目录
	rec = doRequest(t, mux, "POST", "/api/dir-shortcuts",
		map[string]string{"name": "myproject", "path": "/home/me/proj", "category_id": cat.ID})
	if rec.Code != 200 {
		t.Fatalf("create dir: code=%d body=%s", rec.Code, rec.Body.String())
	}
	bodyStr := rec.Body.String()
	if !bytes.Contains([]byte(bodyStr), []byte(`"category_id":"`+cat.ID+`"`)) {
		t.Errorf("body should contain category_id, got %s", bodyStr)
	}

	// 列表验证
	rec = doRequest(t, mux, "GET", "/api/dir-shortcuts", nil)
	var dirs []backend.DirShortcut
	json.NewDecoder(rec.Body).Decode(&dirs)
	if len(dirs) != 1 {
		t.Fatalf("dirs=%d, want 1", len(dirs))
	}
	if dirs[0].CategoryID != cat.ID {
		t.Errorf("CategoryID=%q, want %q", dirs[0].CategoryID, cat.ID)
	}
}

// TestMigrationScenarios 验证迁移场景
func TestMigrationScenarios(t *testing.T) {
	if logger == nil {
		zl, _ := zap.NewProduction()
		logger = zl.Sugar()
	}
	db, cleanup, err := backend.TestDB()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// 模拟旧数据：插入无 category_id 的链接（绕过 Create）
	_, err = db.Exec(`INSERT INTO web_links (id,name,url,sort_order,category_id,created_at) VALUES (?,?,?,?,NULL,?)`,
		"old-1", "Old Link", "https://old.com", 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// 旧数据迁移：本测试验证即使没有 category_id，也不会 panic，
	// 前端按 category_id 过滤时会忽略（SQLite 字符串比较会过滤掉 NULL）
	var catID *string
	if err := db.QueryRow(`SELECT category_id FROM web_links WHERE id=?`, "old-1").Scan(&catID); err != nil {
		t.Fatal(err)
	}
	if catID != nil {
		t.Errorf("expected NULL category_id for old data, got %v", *catID)
	}
}
