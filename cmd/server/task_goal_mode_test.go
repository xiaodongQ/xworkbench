package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
)

// TestCreateTask_GoalModeTrue verifies goal_mode bool is stored and returned.
func TestCreateTask_GoalModeTrue(t *testing.T) {
	s := newManualTaskTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks", s.handleTaskCreate)
	mux.HandleFunc("GET /api/tasks/{id}", s.handleTaskGet)

	body, _ := json.Marshal(map[string]any{
		"title":       "Goal 模式任务",
		"description": "重构 SSH 模块",
		"command_type": "claude",
		"model":       "haiku",
		"goal_mode":   true,
	})
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create task: want 200, got %d: %s", w.Code, w.Body.String())
	}

	var task backend.Task
	if err := json.NewDecoder(w.Body).Decode(&task); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !task.GoalMode {
		t.Errorf("task.GoalMode: want true, got false")
	}
}

// TestCreateTask_GoalModeFalse verifies goal_mode defaults to false when omitted.
func TestCreateTask_GoalModeFalse(t *testing.T) {
	s := newManualTaskTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks", s.handleTaskCreate)

	body, _ := json.Marshal(map[string]any{
		"title":       "普通任务",
		"description": "简单任务",
		"command_type": "claude",
	})
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create task: want 200, got %d", w.Code)
	}

	var task backend.Task
	if err := json.NewDecoder(w.Body).Decode(&task); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if task.GoalMode {
		t.Errorf("task.GoalMode: want false (default), got true")
	}
}

// TestUpdateTask_GoalMode verifies updating goal_mode field.
func TestUpdateTask_GoalMode(t *testing.T) {
	s := newManualTaskTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks", s.handleTaskCreate)
	mux.HandleFunc("PUT /api/tasks/{id}", s.handleTaskUpdate)

	// 创建普通任务
	body, _ := json.Marshal(map[string]any{
		"title":       "初始任务",
		"description": "desc",
		"command_type": "claude",
	})
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var created backend.Task
	json.NewDecoder(w.Body).Decode(&created)
	if created.GoalMode {
		t.Fatalf("newly created task should have GoalMode=false, got true")
	}

	// 更新为 goal_mode=true
	goalModeTrue := true
	updateBody, _ := json.Marshal(map[string]any{
		"title":       "初始任务",
		"description": "desc",
		"command_type": "claude",
		"goal_mode":   goalModeTrue,
	})
	req2 := httptest.NewRequest("PUT", "/api/tasks/"+created.ID, bytes.NewReader(updateBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	var updated backend.Task
	json.NewDecoder(w2.Body).Decode(&updated)
	if !updated.GoalMode {
		t.Errorf("updated task.GoalMode: want true, got false")
	}

	// 再更新回 false
	goalModeFalse := false
	updateBody2, _ := json.Marshal(map[string]any{
		"title":       "初始任务",
		"description": "desc",
		"command_type": "claude",
		"goal_mode":   goalModeFalse,
	})
	req3 := httptest.NewRequest("PUT", "/api/tasks/"+created.ID, bytes.NewReader(updateBody2))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, req3)

	var updated2 backend.Task
	json.NewDecoder(w3.Body).Decode(&updated2)
	if updated2.GoalMode {
		t.Errorf("updated task.GoalMode: want false, got true")
	}
}

// TestTaskRun_GoalModePrefix verifies handleTaskRun adds /goal prefix to prompt
// when goal_mode=true and command_type is claude.
// We verify this by checking that the prompt stored in the Execution record
// starts with "/goal ".
func TestTaskRun_GoalModePrefix(t *testing.T) {
	s := newManualTaskTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks", s.handleTaskCreate)
	mux.HandleFunc("POST /api/tasks/{id}/run", s.handleTaskRun)

	// 创建 goal_mode=true 任务
	createBody, _ := json.Marshal(map[string]any{
		"title":       "Goal Run 测试",
		"description": "重构 SSH 模块，使其支持密码和私钥免登",
		"command_type": "claude",
		"model":       "haiku",
		"goal_mode":   true,
	})
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var task backend.Task
	json.NewDecoder(w.Body).Decode(&task)

	// 运行任务（mock executor 实际执行路径，但验证 prompt 被正确拼装）
	// 由于 executor.Run 是真实执行，我们在 runner.BuildCommand 层面验证
	// 通过检查 execution record 的 Prompt 字段是否带 /goal 前缀
	runBody, _ := json.Marshal(map[string]any{
		"command_type": "claude",
		"model":       "haiku",
	})
	req2 := httptest.NewRequest("POST", "/api/tasks/"+task.ID+"/run", bytes.NewReader(runBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	// handleTaskRun 立即返回 202，不等执行完成；检查 execution 记录中的 prompt
	// 找最新的 execution
	execs, _ := s.execDB.ListByTask(task.ID, 10)
	if len(execs) == 0 {
		t.Fatalf("no execution records found for task %s", task.ID)
	}
	latestExec := execs[0]

	if latestExec.Prompt == "" {
		t.Fatalf("execution prompt is empty")
	}
	if len(latestExec.Prompt) < 6 || latestExec.Prompt[:6] != "/goal " {
		t.Errorf("execution prompt should start with '/goal ', got: %q", latestExec.Prompt[:min(20, len(latestExec.Prompt))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}