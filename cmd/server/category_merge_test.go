package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"go.uber.org/zap"
)

// helper: build a full link-category server with merge endpoint
func newLinkMergeTestMux(t *testing.T, srv *APIServer) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/link-categories", srv.handleLinkCategories)
	mux.HandleFunc("POST /api/link-categories", srv.handleLinkCategoryCreate)
	mux.HandleFunc("PUT /api/link-categories/{id}", srv.handleLinkCategoryUpdate)
	mux.HandleFunc("DELETE /api/link-categories/{id}", srv.handleLinkCategoryDelete)
	mux.HandleFunc("POST /api/link-categories/{id}/merge", srv.handleLinkCategoryMerge)
	mux.HandleFunc("GET /api/web-links", srv.handleWebLinks)
	mux.HandleFunc("POST /api/web-links", srv.handleWebLinkCreate)
	mux.HandleFunc("PUT /api/web-links/{id}", srv.handleWebLinkUpdate)
	return mux
}

func newDirMergeTestMux(t *testing.T, srv *APIServer) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/dir-categories", srv.handleDirCategories)
	mux.HandleFunc("POST /api/dir-categories", srv.handleDirCategoryCreate)
	mux.HandleFunc("PUT /api/dir-categories/{id}", srv.handleDirCategoryUpdate)
	mux.HandleFunc("DELETE /api/dir-categories/{id}", srv.handleDirCategoryDelete)
	mux.HandleFunc("POST /api/dir-categories/{id}/merge", srv.handleDirCategoryMerge)
	mux.HandleFunc("GET /api/dir-shortcuts", srv.handleDirShortcuts)
	mux.HandleFunc("POST /api/dir-shortcuts", srv.handleDirShortcutCreate)
	mux.HandleFunc("PUT /api/dir-shortcuts/{id}", srv.handleDirShortcutUpdate)
	return mux
}

// TestLinkCategoryMerge 合并链接分类：源类下链接迁移到目标类，源类被删除
func TestLinkCategoryMerge(t *testing.T) {
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
		linkCatDB: backend.NewLinkCategoryRepo(db),
	}
	mux := newLinkMergeTestMux(t, srv)

	// 1. 创建 catA、catB
	for _, name := range []string{"CatA", "CatB"} {
		rec := doRequest(t, mux, "POST", "/api/link-categories", map[string]string{"name": name})
		if rec.Code != http.StatusOK {
			t.Fatalf("create cat %s: code=%d body=%s", name, rec.Code, rec.Body.String())
		}
	}
	rec := doRequest(t, mux, "GET", "/api/link-categories", nil)
	var cats []backend.LinkCategory
	json.NewDecoder(rec.Body).Decode(&cats)
	var idA, idB, idDefault string
	for _, c := range cats {
		switch c.Name {
		case "CatA":
			idA = c.ID
		case "CatB":
			idB = c.ID
		default:
			if c.IsDefault {
				idDefault = c.ID
			}
		}
	}
	if idA == "" || idB == "" {
		t.Fatalf("missing test cats: A=%q B=%q", idA, idB)
	}

	// 2. 在 catA 下创建 2 个链接
	for i := 0; i < 2; i++ {
		rec := doRequest(t, mux, "POST", "/api/web-links", map[string]any{
			"name": "linkA", "url": "https://a.example/" + string(rune('a'+i)),
			"category_id": idA,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("create link: %d", rec.Code)
		}
	}

	// 3. 合并 A -> B
	rec = doRequest(t, mux, "POST", "/api/link-categories/"+idA+"/merge", map[string]string{"target_id": idB})
	if rec.Code != http.StatusOK {
		t.Fatalf("merge: code=%d body=%s", rec.Code, rec.Body.String())
	}

	// 4. 验证：catA 已删；剩下的链接 cat_id == idB
	rec = doRequest(t, mux, "GET", "/api/link-categories", nil)
	json.NewDecoder(rec.Body).Decode(&cats)
	for _, c := range cats {
		if c.ID == idA {
			t.Errorf("catA still present after merge")
		}
	}
	rec = doRequest(t, mux, "GET", "/api/web-links", nil)
	var links []backend.WebLink
	json.NewDecoder(rec.Body).Decode(&links)
	mergedCount := 0
	for _, l := range links {
		if l.CategoryID == idB {
			mergedCount++
		}
	}
	if mergedCount != 2 {
		t.Errorf("after merge: links under catB=%d, want 2", mergedCount)
	}

	// 5. 重复合并到自己应报错
	rec = doRequest(t, mux, "POST", "/api/link-categories/"+idB+"/merge", map[string]string{"target_id": idB})
	if rec.Code == http.StatusOK {
		t.Errorf("self-merge should fail; got 200")
	}
	_ = idDefault // unused

	// 6. 合并到不存在的目标应报错
	rec = doRequest(t, mux, "POST", "/api/link-categories", map[string]string{"name": "CatC"})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup CatC: %d", rec.Code)
	}
	rec = doRequest(t, mux, "GET", "/api/link-categories", nil)
	json.NewDecoder(rec.Body).Decode(&cats)
	var idC string
	for _, c := range cats {
		if c.Name == "CatC" {
			idC = c.ID
			break
		}
	}
	rec = doRequest(t, mux, "POST", "/api/link-categories/"+idC+"/merge", map[string]string{"target_id": "nonexistent-id-xyz"})
	if rec.Code == http.StatusOK {
		t.Errorf("merge to nonexistent should fail; got 200 body=%s", rec.Body.String())
	}
}

// TestLinkCategoryMergeBlockDefault 合并默认分类应被拒绝
func TestLinkCategoryMergeBlockDefault(t *testing.T) {
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
		linkCatDB: backend.NewLinkCategoryRepo(db),
	}
	mux := newLinkMergeTestMux(t, srv)

	// 找出默认分类 id
	rec := doRequest(t, mux, "GET", "/api/link-categories", nil)
	var cats []backend.LinkCategory
	json.NewDecoder(rec.Body).Decode(&cats)
	var defaultID string
	for _, c := range cats {
		if c.IsDefault {
			defaultID = c.ID
		}
	}
	if defaultID == "" {
		t.Fatal("no default category found")
	}

	// 准备一个目标分类
	rec = doRequest(t, mux, "POST", "/api/link-categories", map[string]string{"name": "Target"})
	if rec.Code != http.StatusOK {
		t.Fatalf("create target: %d", rec.Code)
	}
	var targetCat backend.LinkCategory
	json.NewDecoder(rec.Body).Decode(&targetCat)

	// 尝试合并默认 → target，应失败
	rec = doRequest(t, mux, "POST", "/api/link-categories/"+defaultID+"/merge", map[string]string{"target_id": targetCat.ID})
	if rec.Code == http.StatusOK {
		t.Errorf("merging default should fail; got 200")
	}
	if !strings.Contains(rec.Body.String(), "default") {
		t.Errorf("expected default-related error, got %s", rec.Body.String())
	}
}

// TestDirCategoryMerge 合并目录分类：源类下快捷方式迁移到目标类，源类被删除
func TestDirCategoryMerge(t *testing.T) {
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
		dirDB:     backend.NewDirShortcutRepo(db),
		dirCatDB:  backend.NewDirCategoryRepo(db),
	}
	mux := newDirMergeTestMux(t, srv)

	// 创建 DirA、DirB
	for _, name := range []string{"DirA", "DirB"} {
		rec := doRequest(t, mux, "POST", "/api/dir-categories", map[string]string{"name": name})
		if rec.Code != http.StatusOK {
			t.Fatalf("create cat %s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	rec := doRequest(t, mux, "GET", "/api/dir-categories", nil)
	var cats []backend.DirCategory
	json.NewDecoder(rec.Body).Decode(&cats)
	var idA, idB string
	for _, c := range cats {
		if c.Name == "DirA" {
			idA = c.ID
		}
		if c.Name == "DirB" {
			idB = c.ID
		}
	}
	if idA == "" || idB == "" {
		t.Fatal("missing test categories")
	}

	// 在 A 下添加 2 个目录快捷方式
	for _, p := range []string{"/tmp/a", "/tmp/b"} {
		rec := doRequest(t, mux, "POST", "/api/dir-shortcuts", map[string]any{
			"name": "ds_" + p, "type": "local", "path": p, "category_id": idA,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("create shortcut: %d", rec.Code)
		}
	}

	// 合并 A -> B
	rec = doRequest(t, mux, "POST", "/api/dir-categories/"+idA+"/merge", map[string]string{"target_id": idB})
	if rec.Code != http.StatusOK {
		t.Fatalf("merge: %d %s", rec.Code, rec.Body.String())
	}

	// 验证：DirA 已删；所有 shortcut 都在 B 下
	rec = doRequest(t, mux, "GET", "/api/dir-categories", nil)
	json.NewDecoder(rec.Body).Decode(&cats)
	for _, c := range cats {
		if c.ID == idA {
			t.Errorf("DirA still present after merge")
		}
	}
	rec = doRequest(t, mux, "GET", "/api/dir-shortcuts", nil)
	var ds []backend.DirShortcut
	json.NewDecoder(rec.Body).Decode(&ds)
	countB := 0
	for _, d := range ds {
		if d.CategoryID == idB {
			countB++
		}
	}
	if countB != 2 {
		t.Errorf("after merge: shortcuts under DirB=%d, want 2", countB)
	}
}
