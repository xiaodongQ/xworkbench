package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/hub"
	"go.uber.org/zap"
)

// newExecCancelTestServer 构造最小 APIServer，只装 cancel 测试需要的字段。
// 返回共享的 db（用于直接插测试数据）+ mux（用于发 HTTP 请求）+ server。
// 三个指针用同一个 db，避免 :memory: 多 db 实例。
func newExecCancelTestServer(t *testing.T) (*APIServer, *http.ServeMux, *sql.DB) {
	t.Helper()
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	s := &APIServer{
		execDB:  backend.NewExecutionRepo(db),
		hub:     hub.New(),
		running: map[string]context.CancelFunc{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/executions/{id}/cancel", s.handleExecutionCancel)
	return s, mux, db
}

// TestExecutionCancel_AlreadyDone 验证已完成的 execution 调 cancel 返回 already_done。
func TestExecutionCancel_AlreadyDone(t *testing.T) {
	_, mux, db := newExecCancelTestServer(t)
	execRepo := backend.NewExecutionRepo(db)

	exec := &backend.Execution{
		ID:        "exec-cancel-done",
		Source:    "manual",
		Command:   "echo done",
		StartedAt: time.Now().Add(-time.Minute),
	}
	if err := execRepo.Create(exec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := execRepo.Finish(exec.ID, "done", "", 0, ""); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	w := doRequest(t, mux, "POST", "/api/executions/exec-cancel-done/cancel", nil)
	if w.Code != 200 {
		t.Fatalf("POST status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, _ := resp["already_done"].(bool); !got {
		t.Errorf("expected already_done=true, got resp=%v", resp)
	}
}

// TestExecutionCancel_RunningMapHit 验证 running map 里有 task_id 的 cancel func 时，
// cancel 调用 cancel() 并返回 mode=running。
func TestExecutionCancel_RunningMapHit(t *testing.T) {
	s, mux, db := newExecCancelTestServer(t)
	execRepo := backend.NewExecutionRepo(db)

	exec := &backend.Execution{
		ID:        "exec-cancel-running",
		TaskID:    "task-running-1",
		Source:    "manual",
		Command:   "sleep 999",
		StartedAt: time.Now(),
	}
	if err := execRepo.Create(exec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	cancelCalled := false
	s.mu.Lock()
	s.running["task-running-1"] = func() { cancelCalled = true }
	s.mu.Unlock()

	w := doRequest(t, mux, "POST", "/api/executions/exec-cancel-running/cancel", nil)
	if w.Code != 200 {
		t.Fatalf("POST status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, _ := resp["mode"].(string); got != "running" {
		t.Errorf("expected mode=running, got resp=%v", resp)
	}
	if !cancelCalled {
		t.Errorf("expected cancel func to be invoked")
	}
}

// TestExecutionCancel_ForceFinish 验证 running map 里没有（goroutine 消失 / 服务重启后），
// cancel 直接写 completed_at=now, status=cancelled, error=reason。
func TestExecutionCancel_ForceFinish(t *testing.T) {
	s, mux, db := newExecCancelTestServer(t)
	execRepo := backend.NewExecutionRepo(db)

	exec := &backend.Execution{
		ID:        "exec-cancel-force",
		TaskID:    "task-no-inflight",
		Source:    "manual",
		Command:   "ghost",
		StartedAt: time.Now().Add(-2 * time.Hour),
	}
	if err := execRepo.Create(exec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// running map 故意不注册
	_ = s

	w := doRequest(t, mux, "POST", "/api/executions/exec-cancel-force/cancel", nil)
	if w.Code != 200 {
		t.Fatalf("POST status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, _ := resp["mode"].(string); got != "force_finished" {
		t.Errorf("expected mode=force_finished, got resp=%v", resp)
	}

	got, err := execRepo.Get(exec.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CompletedAt == nil {
		t.Errorf("after force-finish, CompletedAt should be set")
	}
	if got.Status != "cancelled" {
		t.Errorf("after force-finish, status=%q, want %q", got.Status, "cancelled")
	}
	if !strings.Contains(got.Error, "manually cancelled") {
		t.Errorf("after force-finish, error should mention 'manually cancelled', got %q", got.Error)
	}
}
