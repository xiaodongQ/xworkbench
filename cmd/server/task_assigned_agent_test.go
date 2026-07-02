package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/hub"
	"go.uber.org/zap"
)

func newAssignedAgentTestServer(t *testing.T) *APIServer {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	return &APIServer{
		db:      backend.NewTaskRepo(db),
		expDB:   backend.NewExperienceRepo(db),
		execDB:  backend.NewExecutionRepo(db),
		evalDB:  backend.NewEvaluationRepo(db),
		agentDB: backend.NewAgentRepo(db),
		eventDB: backend.NewTaskEventRepo(db),
		hub:     hub.New(),
	}
}

// TestAssignedAgentID_CreateVerifyCreate 创建 task 时 assigned_agent_id 正确写入 DB。
func TestAssignedAgentID_CreateVerifyGet(t *testing.T) {
	s := newAssignedAgentTestServer(t)

	// 创建一个带 assigned_agent_id 的 remote task
	task := &backend.Task{
		ID:              "task-assigned-1",
		Title:           "Test Assigned Agent",
		Description:     "desc",
		Acceptance:      "acc",
		Status:          backend.TaskStatusPending,
		TaskType:        backend.TaskTypeRemote,
		Priority:        5,
		Version:         "v1",
		CreatedAt:       time.Now(),
		AssignedAgentID: "agent-xxx-123",
	}
	if err := s.db.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Get 验证字段存在且正确
	got, err := s.db.Get("task-assigned-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.AssignedAgentID != "agent-xxx-123" {
		t.Errorf("AssignedAgentID: want 'agent-xxx-123', got %q", got.AssignedAgentID)
	}
	if got.TaskType != backend.TaskTypeRemote {
		t.Errorf("TaskType: want remote, got %q", got.TaskType)
	}
}

// TestAssignedAgentID_ListFilter 列表查询可按 assigned_agent_id 过滤。
func TestAssignedAgentID_ListFilter(t *testing.T) {
	s := newAssignedAgentTestServer(t)

	// 建 3 个 task，2 个属于 agent-A，1 个属于 agent-B
	for i, agentID := range []string{"agent-A", "agent-A", "agent-B"} {
		task := &backend.Task{
			ID:              "task-list-" + strings.Repeat(string(rune('a'+i)), 4),
			Title:           "Task for " + agentID,
			Description:     "desc",
			Status:          backend.TaskStatusPending,
			TaskType:        backend.TaskTypeRemote,
			Priority:        5,
			Version:         "v1",
			CreatedAt:       time.Now(),
			AssignedAgentID: agentID,
		}
		if err := s.db.Create(task); err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
	}

	// List all remote tasks
	tasks, err := s.db.List(backend.TaskFilter{Status: "pending", Offset: 0, Limit: 100})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	agentATasks := 0
	for _, t := range tasks {
		if t.AssignedAgentID == "agent-A" {
			agentATasks++
		}
	}
	if agentATasks != 2 {
		t.Errorf("agent-A tasks: want 2, got %d (got %d total tasks)", agentATasks, len(tasks))
	}
}

// TestAssignedAgentID_UpdateUpdate 更新 task 时 assigned_agent_id 可被修改。
func TestAssignedAgentID_Update(t *testing.T) {
	s := newAssignedAgentTestServer(t)

	task := &backend.Task{
		ID:              "task-assigned-update",
		Title:           "Original Title",
		Description:     "desc",
		Status:          backend.TaskStatusPending,
		TaskType:        backend.TaskTypeRemote,
		Priority:        5,
		Version:         "v1",
		CreatedAt:       time.Now(),
		AssignedAgentID: "agent-original",
	}
	if err := s.db.Create(task); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Update assigned_agent_id to new value
	updated := &backend.Task{
		ID:              "task-assigned-update",
		Title:           "Original Title",
		Description:     "desc",
		Status:          backend.TaskStatusPending,
		TaskType:        backend.TaskTypeRemote,
		Priority:        5,
		Version:         "v1",
		AssignedAgentID: "agent-new-456",
	}
	if err := s.db.Update(updated); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := s.db.Get("task-assigned-update")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.AssignedAgentID != "agent-new-456" {
		t.Errorf("AssignedAgentID after update: want 'agent-new-456', got %q", got.AssignedAgentID)
	}
}

// TestAssignedAgentID_CreateViaAPI 通过 handleTaskCreate API 创建含 assigned_agent_id 的 task。
func TestAssignedAgentID_CreateViaAPI(t *testing.T) {
	s := newAssignedAgentTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks", s.handleTaskCreate)

	body := map[string]any{
		"title":              "API Task with Agent",
		"description":        "desc",
		"task_type":          "remote",
		"assigned_agent_id":  "agent-api-789",
		"priority":           5,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("create task API: status %d, body %s", w.Code, w.Body.String())
	}

	var resp struct {
		ID              string `json:"id"`
		AssignedAgentID string `json:"assigned_agent_id"`
		TaskType        string `json:"task_type"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AssignedAgentID != "agent-api-789" {
		t.Errorf("API response AssignedAgentID: want 'agent-api-789', got %q", resp.AssignedAgentID)
	}
	if resp.TaskType != "remote" {
		t.Errorf("API response TaskType: want 'remote', got %q", resp.TaskType)
	}

	// Verify persisted
	got, err := s.db.Get(resp.ID)
	if err != nil {
		t.Fatalf("get after API create: %v", err)
	}
	if got.AssignedAgentID != "agent-api-789" {
		t.Errorf("persisted AssignedAgentID: want 'agent-api-789', got %q", got.AssignedAgentID)
	}
}