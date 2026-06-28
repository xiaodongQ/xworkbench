package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"go.uber.org/zap"
)

func newEvalTestServer(t *testing.T) *APIServer {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	// writeErr 依赖 main.go 的全局 logger（nil 会 panic），这里给个 noop。
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	// AI 自治能力默认开启（config.json 单一来源），以便 run-loop/reevaluate/learn
	// 现有测试能走完整逻辑。如某个测试需要验证"未启用返 403"路径，单独重设 config.AppConfig。
	if config.Get() == nil {
		config.Set(config.DefaultConfig())
	}
	config.Update(func(c *config.Config) { c.AILoopEnabled = true })
	s := &APIServer{
		db:      backend.NewTaskRepo(db),
		dirDB:   backend.NewDirShortcutRepo(db),
		schedDB: backend.NewScheduledTaskRepo(db),
		execDB:  backend.NewExecutionRepo(db),
		evalDB:  backend.NewEvaluationRepo(db),
	}
	// running map 默认初始化（continue/evaluate 类 handler 会用）
	s.running = map[string]context.CancelFunc{}
	// runLoops map 默认初始化（run-loop 任务级去重用）
	s.runLoops = map[string]bool{}
	return s
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

// TestHandleTaskReevaluate_Success 验证 reevaluate 在 task + execution 都存在时返回 200
// 并启动后台 evaluator goroutine。handler 不等评估完成(异步模式),只验证入口校验 +
// 返回体格式(execution_id / status=reevaluating / cli_type / model)。
func TestHandleTaskReevaluate_Success(t *testing.T) {
	s := newEvalTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/reevaluate", s.handleTaskReevaluate)

	task := &backend.Task{ID: "reeval-task-1", Title: "reeval test"}
	if err := s.db.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	exec := &backend.Execution{
		ID:        "reeval-exec-1",
		TaskID:    "reeval-task-1",
		Source:    "manual",
		Command:   "echo hi",
		Output:    "hi\n",
		ExitCode:  0,
		StartedAt: time.Now(),
	}
	if err := s.execDB.Create(exec); err != nil {
		t.Fatalf("create execution: %v", err)
	}

	body := `{"cli_type":"claude","model":"haiku"}`
	req := httptest.NewRequest("POST", "/api/tasks/reeval-task-1/reevaluate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("reevaluate status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["status"] != "reevaluating" {
		t.Errorf("status = %v, want \"reevaluating\"", resp["status"])
	}
	if resp["execution_id"] != "reeval-exec-1" {
		t.Errorf("execution_id = %v, want \"reeval-exec-1\"", resp["execution_id"])
	}
	if resp["cli_type"] != "claude" {
		t.Errorf("cli_type = %v, want \"claude\"", resp["cli_type"])
	}
	if resp["model"] != "haiku" {
		t.Errorf("model = %v, want \"haiku\"", resp["model"])
	}
}

// TestHandleTaskReevaluate_NoExecution 验证 task 存在但没有任何 execution 时返 400。
// 这是 reevaluate 独有的失败路径(learn 没有这条,因为它至少要有个 exec 才有意义)。
func TestHandleTaskReevaluate_NoExecution(t *testing.T) {
	s := newEvalTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/reevaluate", s.handleTaskReevaluate)

	task := &backend.Task{ID: "reeval-empty-task", Title: "no exec"}
	s.db.Create(task)

	req := httptest.NewRequest("POST", "/api/tasks/reeval-empty-task/reevaluate", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("reevaluate no-exec code = %d, want 400; body=%s", w.Code, w.Body.String())
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
