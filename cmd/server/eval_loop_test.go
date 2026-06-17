package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"go.uber.org/zap"
)

// init 初始化 main 包级 logger，writeErr 会用到它。
// 不做这一行调用 writeErr 的 handler 会 nil pointer panic。
func init() {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
}

func newEvalTestServer(t *testing.T) *APIServer {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	return &APIServer{
		db:      backend.NewTaskRepo(db),
		setDB:   backend.NewAppSettingsRepo(db),
		dirDB:   backend.NewDirShortcutRepo(db),
		schedDB: backend.NewScheduledTaskRepo(db),
		execDB:  backend.NewExecutionRepo(db),
		evalDB:  backend.NewEvaluationRepo(db),
	}
}

func TestHandleTaskEvalHistory_NotFound(t *testing.T) {
	s := newEvalTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tasks/{id}/eval-history", s.handleTaskEvalHistory)
	req := httptest.NewRequest("GET", "/api/tasks/nonexistent/eval-history", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("eval-history notfound = %d, want 404", w.Code)
	}
}

func TestHandleTaskReevaluate_NotFound(t *testing.T) {
	s := newEvalTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/reevaluate", s.handleTaskReevaluate)
	body := `{"cli_type":"claude","model":"haiku"}`
	req := httptest.NewRequest("POST", "/api/tasks/nonexistent/reevaluate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("reevaluate notfound = %d, want 404", w.Code)
	}
}

func TestHandleTaskLearn_NotFound(t *testing.T) {
	s := newEvalTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/learn", s.handleTaskLearn)
	req := httptest.NewRequest("POST", "/api/tasks/nonexistent/learn", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("learn notfound = %d, want 404", w.Code)
	}
}

func TestHandleTaskRunLoop_NotFound(t *testing.T) {
	s := newEvalTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/run-loop", s.handleTaskRunLoop)
	body := `{"prompt":"hello","model":"haiku"}`
	req := httptest.NewRequest("POST", "/api/tasks/nonexistent/run-loop", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("run-loop notfound = %d, want 404", w.Code)
	}
}

func TestHandleTaskRunLoop_InvalidJSON(t *testing.T) {
	s := newEvalTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/run-loop", s.handleTaskRunLoop)
	task := &backend.Task{ID: "loop-task-1", Title: "test"}
	s.db.Create(task)
	body := `not json`
	req := httptest.NewRequest("POST", "/api/tasks/loop-task-1/run-loop", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("run-loop bad json = %d, want 400", w.Code)
	}
}

func TestHandleTaskLearn_Success(t *testing.T) {
	s := newEvalTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/learn", s.handleTaskLearn)
	mux.HandleFunc("GET /api/experiences", s.handleExperiences)

	task := &backend.Task{
		ID:          "learn-task-1",
		Title:       "学习测试任务",
		Description: "测试 self-learning",
		Status:      backend.TaskStatusArchived,
	}
	if err := s.db.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	exec := &backend.Execution{
		ID:        "learn-exec-1",
		TaskID:    "learn-task-1",
		Source:    "manual",
		Command:   "echo hello",
		Output:    "hello\nworld",
		ExitCode:  0,
		StartedAt: time.Now(),
	}
	if err := s.execDB.Create(exec); err != nil {
		t.Fatalf("create execution: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/tasks/learn-task-1/learn", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("learn status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestHandleTaskEvalHistory_WithExecutions(t *testing.T) {
	s := newEvalTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tasks/{id}/eval-history", s.handleTaskEvalHistory)

	task := &backend.Task{ID: "hist-task-1", Title: "test", Status: backend.TaskStatusArchived}
	s.db.Create(task)
	exec := &backend.Execution{
		ID:        "hist-exec-1",
		TaskID:    "hist-task-1",
		Source:    "manual",
		Command:   "echo test",
		Output:    "test output",
		ExitCode:  0,
		StartedAt: time.Now(),
	}
	s.execDB.Create(exec)

	req := httptest.NewRequest("GET", "/api/tasks/hist-task-1/eval-history", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("eval-history status = %d, want 200", w.Code)
	}
	var result []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) != 1 {
		t.Errorf("expected 1 execution in history, got %d", len(result))
	}
}
