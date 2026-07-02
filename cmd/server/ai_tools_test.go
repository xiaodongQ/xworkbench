package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
)

// TestAIToolsRegistered verifies all 10 tools are registered in GetTools().
func TestAIToolsRegistered(t *testing.T) {
	tools := GetTools()
	names := make(map[string]bool)
	for _, t := range tools {
		names[t.Name] = true
	}

	expected := []string{
		"create_task",
		"list_tasks",
		"get_task",
		"update_task",
		"create_dir_shortcut",
		"list_dir_shortcuts",
		"run_task",
		"search_experiences",
		"get_task_executions",
		"run_local_command",
		"start_local_shell",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("tool %q not registered", name)
		}
	}
}

// TestAIToolsHaveRequiredFields verifies each tool has name, description, and valid JSON parameters.
func TestAIToolsHaveRequiredFields(t *testing.T) {
	tools := GetTools()
	if len(tools) == 0 {
		t.Fatal("no tools registered")
	}
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("tool has empty name")
		}
		if tool.Description == "" {
			t.Errorf("tool %s has empty description", tool.Name)
		}
		if len(tool.Parameters) == 0 {
			t.Errorf("tool %s has no parameters schema", tool.Name)
			continue
		}
		// Validate JSON schema is parseable
		var schema map[string]any
		if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
			t.Errorf("tool %s parameters not valid JSON: %v", tool.Name, err)
		}
	}
}

// TestExecuteToolCreateTask verifies create_task tool executes and returns task ID.
func TestExecuteToolCreateTask(t *testing.T) {
	server := newTestServerForTools(t)
	// Use existing test server's repos

	args := `{"title":"test task from tool","description":"created by tool test","task_type":"manual","priority":5}`
	result := ExecuteTool(context.Background(), server.db, server.expDB, server.execDB, server.agentDB, nil, "create_task", args)

	if result == "" {
		t.Error("create_task returned empty result")
	}
	if !containsStr(result, "task-") && !containsStr(result, "创建") {
		t.Errorf("create_task result unexpected: %s", result)
	}
}

// TestExecuteToolListTasks verifies list_tasks tool executes without error.
func TestExecuteToolListTasks(t *testing.T) {
	server := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), server.db, server.expDB, server.execDB, server.agentDB, nil, "list_tasks", `{"limit":10}`)

	if result == "" {
		t.Error("list_tasks returned empty result")
	}
	// Should contain task list or empty array
	if !containsStr(result, "[") && !containsStr(result, "task") && !containsStr(result, "无") {
		t.Errorf("list_tasks result unexpected: %s", result)
	}
}

// TestExecuteToolUnknown verifies unknown tool returns error.
func TestExecuteToolUnknown(t *testing.T) {
	server := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), server.db, server.expDB, server.execDB, server.agentDB, nil, "unknown_tool", `{}`)

	if !containsStr(result, "未知") && !containsStr(result, "unknown") && !containsStr(result, "error") {
		t.Errorf("unknown tool should return error, got: %s", result)
	}
}

// TestExecuteToolListDirShortcuts verifies list_dir_shortcuts works.
func TestExecuteToolListDirShortcuts(t *testing.T) {
	server := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), server.db, server.expDB, server.execDB, server.agentDB, nil, "list_dir_shortcuts", `{}`)

	if result == "" {
		t.Error("list_dir_shortcuts returned empty")
	}
}

// TestExecuteToolSearchExperiences verifies search_experiences works.
func TestExecuteToolSearchExperiences(t *testing.T) {
	server := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), server.db, server.expDB, server.execDB, server.agentDB, nil, "search_experiences", `{"query":"test","limit":5}`)

	if result == "" {
		t.Error("search_experiences returned empty")
	}
}

// TestExecuteToolGetTask verifies get_task works with valid task ID.
func TestExecuteToolGetTask(t *testing.T) {
	server := newTestServerForTools(t)
	// Create a task first
	task := newTestTaskForTools()
	if err := server.db.Create(task); err != nil {
		t.Fatalf("create test task: %v", err)
	}

	result := ExecuteTool(context.Background(), server.db, server.expDB, server.execDB, server.agentDB, nil, "get_task", `{"task_id":"`+task.ID+`"}`)

	if result == "" {
		t.Error("get_task returned empty")
	}
	if !containsStr(result, task.Title) && !containsStr(result, task.ID) {
		t.Errorf("get_task result should contain task info: %s", result)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- Helpers ---

func newTestServerForTools(t *testing.T) *APIServer {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	return &APIServer{
		db:      backend.NewTaskRepo(db),
		expDB:   backend.NewExperienceRepo(db),
		execDB:  backend.NewExecutionRepo(db),
		evalDB:  backend.NewEvaluationRepo(db),
		agentDB: backend.NewAgentRepo(db),
		eventDB: backend.NewTaskEventRepo(db),
	}
}

func newTestTaskForTools() *backend.Task {
	return &backend.Task{
		ID:          "tool-test-" + time.Now().Format("150405"),
		Title:       "Tool Test Task",
		Description: "test",
		Acceptance:  "test",
		Status:      backend.TaskStatusPending,
		Priority:    5,
		Version:     "v1",
		CreatedAt:   time.Now(),
	}
}