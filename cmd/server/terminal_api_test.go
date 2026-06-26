package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"github.com/xiaodongQ/xworkbench/internal/hub"
	"github.com/xiaodongQ/xworkbench/internal/scheduler"
)

// newTestServer creates an APIServer with in-memory SQLite for testing.
func newTestServer(t *testing.T) *APIServer {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	// config.json（单一来源）设置 default_terminal，避免 handler 走 shortcuts.DefaultTerminal() fallback
	if config.Get() == nil {
		config.Set(config.DefaultConfig())
	}
	config.Update(func(c *config.Config) { c.DefaultTerminal = "wezterm" })
	schedDB := backend.NewScheduledTaskRepo(db)
	execDB := backend.NewExecutionRepo(db)
	h := hub.New()
	sch := scheduler.New(schedDB, execDB, h)
	// Reload 加载 enabled 任务到 entries map(用于 NextRunAt),
	// 不调 Start() 因为 Start 会启 cron engine 实际跑 task——测试期间 task 实际执行会污染 DB。
	if err := sch.Reload(); err != nil {
		t.Fatalf("scheduler.Reload: %v", err)
	}
	return &APIServer{
		db:      backend.NewTaskRepo(db),
		dirDB:   backend.NewDirShortcutRepo(db),
		schedDB: schedDB,
		execDB:  execDB,
		sch:     sch,
		hub:     h,
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
		Default   string              `json:"default"`
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
	if resp.Default != "wezterm" {
		t.Errorf("default = %q, want wezterm", resp.Default)
	}
}

// TestHandleSetConfig_DefaultTerminal 验证 PUT /api/config 写入 default_terminal 后立即生效
func TestHandleSetConfig_DefaultTerminal(t *testing.T) {
	s := newTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/config", s.handleSetConfig)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)

	// PUT default_terminal
	body := strings.NewReader(`{"default_terminal":"wt"}`)
	req := httptest.NewRequest("PUT", "/api/config", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleSetConfig default_terminal status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	// GET 验证已生效
	req = httptest.NewRequest("GET", "/api/config", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var got map[string]interface{}
	json.NewDecoder(w.Body).Decode(&got)
	if got["default_terminal"] != "wt" {
		t.Errorf("default_terminal after set = %q, want wt", got["default_terminal"])
	}

	// PUT 无效值 → 400
	body = strings.NewReader(`{"default_terminal":"notexist"}`)
	req = httptest.NewRequest("PUT", "/api/config", body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("handleSetConfig invalid default_terminal status = %d, want 400", w.Code)
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
