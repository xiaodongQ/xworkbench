package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
)

// newTestServer creates an APIServer with in-memory SQLite for testing.
func newTestServer(t *testing.T) *APIServer {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	setDB := backend.NewAppSettingsRepo(db)
	setDB.Set("default_terminal", "wezterm") // avoid nil panic in handlers
	return &APIServer{
		db:      backend.NewTaskRepo(db),
		setDB:   setDB,
		dirDB:   backend.NewDirShortcutRepo(db),
		schedDB: backend.NewScheduledTaskRepo(db),
	}
}

func TestHandleTerminalList(t *testing.T) {
	s := newTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/terminals", s.handleTerminalList)
	req := httptest.NewRequest("GET", "/api/terminals", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleTerminalList status = %d, want 200", w.Code)
	}
	var resp struct {
		Supported []map[string]string `json:"supported"`
		Default  string             `json:"default"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Supported) == 0 {
		t.Fatal("supported terminal list is empty")
	}
	found := false
	for _, s := range resp.Supported {
		if s["type"] == "wezterm" {
			found = true
			break
		}
	}
	if !found {
		t.Error("wezterm not found in supported list")
	}
}

func TestHandleGetSetDefaultTerminal(t *testing.T) {
	s := newTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/settings/default_terminal", s.handleGetDefaultTerminal)
	mux.HandleFunc("PUT /api/settings/default_terminal", s.handleSetDefaultTerminal)

	// GET initial value
	req := httptest.NewRequest("GET", "/api/settings/default_terminal", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleGetDefaultTerminal status = %d, want 200", w.Code)
	}

	// PUT valid value
	body := strings.NewReader(`{"value":"wt"}`)
	req = httptest.NewRequest("PUT", "/api/settings/default_terminal", body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleSetDefaultTerminal valid status = %d, want 200", w.Code)
	}

	// GET verify
	req = httptest.NewRequest("GET", "/api/settings/default_terminal", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var got map[string]string
	json.NewDecoder(w.Body).Decode(&got)
	if got["value"] != "wt" {
		t.Errorf("handleGetDefaultTerminal after set = %q, want wt", got["value"])
	}

	// PUT invalid value
	body = strings.NewReader(`{"value":"notexist"}`)
	req = httptest.NewRequest("PUT", "/api/settings/default_terminal", body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("handleSetDefaultTerminal invalid status = %d, want 400", w.Code)
	}
}

func TestHandleDirShortcutOpenTerminal_NotFound(t *testing.T) {
	s := newTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/dir-shortcuts/{id}/open-terminal", s.handleDirShortcutOpenTerminal)
	req := httptest.NewRequest("POST", "/api/dir-shortcuts/nonexistent-id/open-terminal?type=wezterm", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("handleDirShortcutOpenTerminal notfound status = %d, want 404", w.Code)
	}
}

func TestHandleSetDefaultTerminal_InvalidJSON(t *testing.T) {
	s := newTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/settings/default_terminal", s.handleSetDefaultTerminal)
	body := strings.NewReader(`not json`)
	req := httptest.NewRequest("PUT", "/api/settings/default_terminal", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("handleSetDefaultTerminal bad json status = %d, want 400", w.Code)
	}
}

// TestHandleDirShortcutOpenTerminal_InvalidType 测试无效终端类型 → 400
// 使用 mux 路由正确提取 {id} path 参数
func TestHandleDirShortcutOpenTerminal_InvalidType(t *testing.T) {
	s := newTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/dir-shortcuts/{id}/open-terminal", s.handleDirShortcutOpenTerminal)

	// 创建目录
	dir := &backend.DirShortcut{
		ID: "test-dir-1", Name: "test", Path: "/tmp", SortOrder: 1, CreatedAt: time.Now(),
	}
	if err := s.dirDB.Create(dir); err != nil {
		t.Fatalf("create test dir shortcut: %v", err)
	}

	// 无效终端类型 → BadRequest
	req := httptest.NewRequest("POST", "/api/dir-shortcuts/test-dir-1/open-terminal?type=notexist", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("handleDirShortcutOpenTerminal invalid type status = %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

// TestOpenTerminal_ExecutableNotFound 测试终端不存在 → 400
func TestOpenTerminal_ExecutableNotFound(t *testing.T) {
	s := newTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/dir-shortcuts/{id}/open-terminal", s.handleDirShortcutOpenTerminal)

	dir := &backend.DirShortcut{
		ID: "test-dir-2", Name: "test2", Path: "/tmp", SortOrder: 2, CreatedAt: time.Now(),
	}
	s.dirDB.Create(dir)

	req := httptest.NewRequest("POST", "/api/dir-shortcuts/test-dir-2/open-terminal?type=nonexistent_xyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("open-terminal noexec status = %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

// TestHandleDirShortcutCRUD 测试完整的 dir-shortcut CRUD 流程
func TestHandleDirShortcutCRUD(t *testing.T) {
	s := newTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/dir-shortcuts", s.handleDirShortcuts)
	mux.HandleFunc("POST /api/dir-shortcuts", s.handleDirShortcutCreate)
	mux.HandleFunc("DELETE /api/dir-shortcuts/{id}", s.handleDirShortcutDelete)

	// CREATE
	body := strings.NewReader(`{"name":"test-crud","path":"/workspace"}`)
	req := httptest.NewRequest("POST", "/api/dir-shortcuts", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleDirShortcutCreate status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var created map[string]interface{}
	json.NewDecoder(w.Body).Decode(&created)
	id := created["id"].(string)
	if id == "" {
		t.Fatal("created shortcut has no id")
	}

	// LIST
	req = httptest.NewRequest("GET", "/api/dir-shortcuts", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleDirShortcuts status = %d, want 200", w.Code)
	}
	var list []*backend.DirShortcut
	json.NewDecoder(w.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("expected 1 shortcut, got %d", len(list))
	}

	// DELETE
	req = httptest.NewRequest("DELETE", "/api/dir-shortcuts/"+id, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleDirShortcutDelete status = %d, want 200", w.Code)
	}
}
