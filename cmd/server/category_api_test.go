package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"go.uber.org/zap"
)

// setupCategoryTestServer 创建带分类仓库的测试 server
func setupCategoryTestServer(t *testing.T) (*APIServer, func()) {
	t.Helper()
	if logger == nil {
		zl, _ := zap.NewProduction()
		logger = zl.Sugar()
	}
	db, cleanup, err := backend.TestDB()
	if err != nil {
		t.Fatal(err)
	}
	srv := &APIServer{
		linkDB:    backend.NewWebLinkRepo(db),
		dirDB:     backend.NewDirShortcutRepo(db),
		linkCatDB: backend.NewLinkCategoryRepo(db),
		dirCatDB:  backend.NewDirCategoryRepo(db),
	}
	return srv, cleanup
}

// TestLinkCategoryCRUD 链接分类 CRUD
func TestLinkCategoryCRUD(t *testing.T) {
	srv, cleanup := setupCategoryTestServer(t)
	defer cleanup()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/link-categories", srv.handleLinkCategories)
	mux.HandleFunc("POST /api/link-categories", srv.handleLinkCategoryCreate)
	mux.HandleFunc("PUT /api/link-categories/{id}", srv.handleLinkCategoryUpdate)
	mux.HandleFunc("DELETE /api/link-categories/{id}", srv.handleLinkCategoryDelete)

	// List - 应该有默认分类
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/link-categories", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET list code=%d body=%s", rec.Code, rec.Body.String())
	}
	var list []backend.LinkCategory
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) < 1 {
		t.Fatal("default category missing")
	}

	// Create
	body, _ := json.Marshal(map[string]string{"name": "工作", "icon": "💼"})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/link-categories", bytes.NewReader(body))
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("POST create code=%d body=%s", rec.Code, rec.Body.String())
	}
	var created backend.LinkCategory
	json.NewDecoder(rec.Body).Decode(&created)
	if created.ID == "" {
		t.Error("created category missing ID")
	}
	if created.Name != "工作" {
		t.Errorf("Name=%q, want 工作", created.Name)
	}

	// List - 现在应该有 2 个
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/link-categories", nil)
	mux.ServeHTTP(rec, req)
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 2 {
		t.Errorf("list len=%d, want 2", len(list))
	}

	// Delete
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/api/link-categories/"+created.ID, nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("DELETE code=%d body=%s", rec.Code, rec.Body.String())
	}

	// Delete default should fail
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/api/link-categories/default-link", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("delete default code=%d, want 400", rec.Code)
	}
}

// TestDirCategoryCRUD 目录分类 CRUD
func TestDirCategoryCRUD(t *testing.T) {
	srv, cleanup := setupCategoryTestServer(t)
	defer cleanup()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/dir-categories", srv.handleDirCategories)
	mux.HandleFunc("POST /api/dir-categories", srv.handleDirCategoryCreate)
	mux.HandleFunc("DELETE /api/dir-categories/{id}", srv.handleDirCategoryDelete)

	// Create
	body, _ := json.Marshal(map[string]string{"name": "开发", "icon": "💻"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/dir-categories", bytes.NewReader(body))
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("POST code=%d body=%s", rec.Code, rec.Body.String())
	}
	var created backend.DirCategory
	json.NewDecoder(rec.Body).Decode(&created)

	// Delete default should fail
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/api/dir-categories/default-dir", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("delete default code=%d, want 400", rec.Code)
	}

	// Delete our created cat
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/api/dir-categories/"+created.ID, nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("delete code=%d, want 200", rec.Code)
	}
}

// TestWebLinkWithCategory 测试链接 API 支持分类
func TestWebLinkWithCategory(t *testing.T) {
	srv, cleanup := setupCategoryTestServer(t)
	defer cleanup()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/web-links", srv.handleWebLinks)
	mux.HandleFunc("POST /api/web-links", srv.handleWebLinkCreate)
	mux.HandleFunc("PUT /api/web-links/{id}", srv.handleWebLinkUpdate)

	// 创建分类
	srv.linkCatDB.Create(&backend.LinkCategory{
		ID:        "work",
		Name:      "工作",
		SortOrder: 1,
		CreatedAt: time.Now(),
	})

	// 创建链接（指定分类）
	body, _ := json.Marshal(map[string]string{
		"name":        "GitHub",
		"url":         "https://github.com",
		"category_id": "work",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/web-links", bytes.NewReader(body))
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("POST code=%d body=%s", rec.Code, rec.Body.String())
	}
	var link backend.WebLink
	json.NewDecoder(rec.Body).Decode(&link)
	if link.CategoryID != "work" {
		t.Errorf("CategoryID=%q, want work", link.CategoryID)
	}

	// 创建链接（不指定分类，应默认）
	body2, _ := json.Marshal(map[string]string{
		"name": "Default",
		"url":  "https://default.com",
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/web-links", bytes.NewReader(body2))
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("POST 2 code=%d body=%s", rec.Code, rec.Body.String())
	}
	var link2 backend.WebLink
	json.NewDecoder(rec.Body).Decode(&link2)
	if link2.CategoryID != "default-link" {
		t.Errorf("default CategoryID=%q, want default-link", link2.CategoryID)
	}

	// 列出应该看到 2 个
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/web-links", nil)
	mux.ServeHTTP(rec, req)
	var list []backend.WebLink
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 2 {
		t.Errorf("list len=%d, want 2", len(list))
	}

	// 切换第一个链接到 default 分类
	body3, _ := json.Marshal(map[string]string{"category_id": "default-link"})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("PUT", "/api/web-links/"+link.ID, bytes.NewReader(body3))
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("PUT code=%d", rec.Code)
	}
}

// TestDirShortcutWithCategory 测试目录 API 支持分类
func TestDirShortcutWithCategory(t *testing.T) {
	srv, cleanup := setupCategoryTestServer(t)
	defer cleanup()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/dir-shortcuts", srv.handleDirShortcutCreate)

	body, _ := json.Marshal(map[string]string{
		"name":        "Test",
		"path":        "/tmp",
		"category_id": "default-dir",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/dir-shortcuts", bytes.NewReader(body))
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("POST code=%d body=%s", rec.Code, rec.Body.String())
	}
	body2 := rec.Body.String()
	if !strings.Contains(body2, `"category_id":"default-dir"`) {
		t.Errorf("body should contain category_id, got %s", body2)
	}
}

// TestCategoryIsDefaultFlag verifies that is_default=true is returned for the default category
// even when it has no items (front-end relies on this to show default even when empty).
func TestCategoryIsDefaultFlag(t *testing.T) {
	srv, cleanup := setupCategoryTestServer(t)
	defer cleanup()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/link-categories", srv.handleLinkCategories)
	mux.HandleFunc("GET /api/dir-categories", srv.handleDirCategories)

	// Link categories: default should have is_default=true even with no links
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/link-categories", nil)
	mux.ServeHTTP(rec, req)
	var linkCats []backend.LinkCategory
	json.NewDecoder(rec.Body).Decode(&linkCats)
	found := false
	for _, c := range linkCats {
		if c.ID == "default-link" && c.IsDefault {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("default-link category should have is_default=true, got %+v", linkCats)
	}

	// Dir categories: default should have is_default=true even with no dirs
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/dir-categories", nil)
	mux.ServeHTTP(rec, req)
	var dirCats []backend.DirCategory
	json.NewDecoder(rec.Body).Decode(&dirCats)
	found = false
	for _, c := range dirCats {
		if c.ID == "default-dir" && c.IsDefault {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("default-dir category should have is_default=true, got %+v", dirCats)
	}
}
