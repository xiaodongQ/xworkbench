package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/hub"
	"go.uber.org/zap"
)

// newTestAPIServer creates a minimal APIServer for remote agent endpoint tests.
func newRemoteAgentTestServer(t *testing.T) *APIServer {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	wsHub := hub.New()
	return &APIServer{
		db:       backend.NewTaskRepo(db),
		expDB:    backend.NewExperienceRepo(db),
		execDB:   backend.NewExecutionRepo(db),
		evalDB:   backend.NewEvaluationRepo(db),
		agentDB:  backend.NewAgentRepo(db),
		eventDB:  backend.NewTaskEventRepo(db),
		hub:      wsHub,
		running:  map[string]context.CancelFunc{},
	}
}

func registerTestAgentForRemote(t *testing.T, s *APIServer, name string) (agentID, token string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/agents/register", s.handleAgentRegister)
	body := map[string]string{"name": name, "capabilities": "task-execute,streaming-output", "version": "0.1.0"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/agents/register", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("register status %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		AgentID string `json:"agent_id"`
		Token   string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.AgentID, resp.Token
}

func seedRemoteTaskSimple(t *testing.T, s *APIServer, title string, agentID string) string {
	t.Helper()
	task := &backend.Task{
		ID:           "remote-task-" + title,
		Title:        title,
		Description:  "test",
		Acceptance:   "test",
		Status:       backend.TaskStatusPending,
		TaskType:     backend.TaskTypeRemote,
		Priority:     5,
		Version:      "v1",
		CreatedAt:    time.Now(),
		AssignedAgentID: agentID,
	}
	if err := s.db.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	return task.ID
}

// TestClaimNext_GET_LongPoll tests that GET /api/tasks/claim-next supports timeout param.
func TestClaimNext_GET_LongPoll(t *testing.T) {
	s := newRemoteAgentTestServer(t)
	agentID, token := registerTestAgentForRemote(t, s, "agent-poll")
	if err := s.agentDB.SetAutoClaimEnabled(agentID, true); err != nil {
		t.Fatalf("enable auto_claim: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tasks/claim-next", s.handleTaskClaimNext)
	mux.HandleFunc("POST /api/tasks/claim-next", s.handleTaskClaimNext)

	// No tasks → immediate 204 (no timeout param)
	req := httptest.NewRequest("GET", "/api/tasks/claim-next?agent_id="+agentID+"&timeout=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("no task without timeout: expected 204, got %d", w.Code)
	}
}

// TestClaimNext_GET_WithTask tests GET claim-next returns task when available.
func TestClaimNext_GET_WithTask(t *testing.T) {
	s := newRemoteAgentTestServer(t)
	agentID, token := registerTestAgentForRemote(t, s, "agent-with-task")
	if err := s.agentDB.SetAutoClaimEnabled(agentID, true); err != nil {
		t.Fatalf("enable auto_claim: %v", err)
	}

	taskID := seedRemoteTaskSimple(t, s, "test-poll", agentID)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tasks/claim-next", s.handleTaskClaimNext)

	req := httptest.NewRequest("GET", "/api/tasks/claim-next?agent_id="+agentID+"&timeout=5", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("claim-next failed: %d %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "claimed" {
		t.Errorf("expected status=claimed, got %v", resp["status"])
	}
	tid, _ := resp["task_id"].(string)
	if tid != taskID {
		t.Errorf("expected task_id=%s, got %s", taskID, tid)
	}
	if resp["prompt"] == "" {
		t.Error("expected non-empty prompt in response")
	}
}

// TestClaimNext_BothMethodsWork tests POST still works alongside GET.
func TestClaimNext_BothMethodsWork(t *testing.T) {
	s := newRemoteAgentTestServer(t)
	agentID, token := registerTestAgentForRemote(t, s, "agent-both")
	if err := s.agentDB.SetAutoClaimEnabled(agentID, true); err != nil {
		t.Fatalf("enable auto_claim: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/claim-next", s.handleTaskClaimNext)
	mux.HandleFunc("GET /api/tasks/claim-next", s.handleTaskClaimNext)

	// POST still works
	body, _ := json.Marshal(map[string]string{"agent_id": agentID})
	req := httptest.NewRequest("POST", "/api/tasks/claim-next", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == 0 {
		t.Error("POST claim-next: expected non-zero status, route not registered?")
	}
}

// TestXwcliEndpoints test that install script and xwcli.py are served.
func TestXwcliEndpoints(t *testing.T) {
	s := &APIServer{}

	// Install script endpoint
	req := httptest.NewRequest("GET", "/api/xwcli/install.sh?server=http://localhost:8902", nil)
	w := httptest.NewRecorder()
	s.handleXwcliInstall(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("install.sh status: %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/plain; charset=utf-8" {
		t.Errorf("install.sh content-type: %s", ct)
	}
	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("xwcli")) || !bytes.Contains([]byte(body), []byte("localhost:8902")) {
		t.Errorf("install.sh missing expected content, got: %s", body[:intMin(200, len(body))])
	}

	// xwcli.py endpoint
	req2 := httptest.NewRequest("GET", "/api/xwcli/xwcli.py", nil)
	w2 := httptest.NewRecorder()
	s.handleXwcliDownload(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("xwcli.py status: %d", w2.Code)
	}
	body2 := w2.Body.String()
	if !bytes.Contains([]byte(body2), []byte("xwcli")) || !bytes.Contains([]byte(body2), []byte("claim_next")) {
		t.Errorf("xwcli.py missing expected content")
	}
}

func intMin(a, b int) int { if a < b { return a }; return b }