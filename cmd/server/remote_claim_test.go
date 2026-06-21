package main

import (
	"bytes"
	"context"
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

// newRemoteClaimTestServer 构造最小化 APIServer 用于 claim/report 流程测试。
func newRemoteClaimTestServer(t *testing.T) *APIServer {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	wsHub := hub.New()
	s := &APIServer{
		db:       backend.NewTaskRepo(db),
		expDB:    backend.NewExperienceRepo(db),
		execDB:   backend.NewExecutionRepo(db),
		evalDB:   backend.NewEvaluationRepo(db),
		agentDB:  backend.NewAgentRepo(db),
		eventDB:  backend.NewTaskEventRepo(db),
		hub:      wsHub,
		running:  map[string]context.CancelFunc{},
	}
	return s
}

// 创建一个已注册的 agent 并返回明文 token（注册时返回的）
func registerTestAgent(t *testing.T, s *APIServer, name string) (agentID, token string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/agents/register", s.handleAgentRegister)
	body := map[string]string{"name": name, "capabilities": "remote-task", "version": "test-v0.1"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/agents/register", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("register: status %d, body %s", w.Code, w.Body.String())
	}
	var resp struct {
		AgentID string `json:"agent_id"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp.AgentID, resp.Token
}

// seedRemoteTask 准备一个 pending + remote + 关联经验的任务
func seedRemoteTask(t *testing.T, s *APIServer, title, desc, acceptance string, expIDs []string) string {
	t.Helper()
	task := &backend.Task{
		ID: "remote-task-" + title, Title: title, Description: desc, Acceptance: acceptance,
		Status: backend.TaskStatusPending, TaskType: backend.TaskTypeRemote,
		Priority: 5, Version: "v1", CreatedAt: time.Now(),
	}
	if err := s.db.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if len(expIDs) > 0 {
		if err := s.db.AttachExperiences(task.ID, expIDs); err != nil {
			t.Fatalf("attach exp: %v", err)
		}
	}
	return task.ID
}

// seedExperience 创建一个 experience
func seedExperience(t *testing.T, s *APIServer, module, scene, keywords, details string) string {
	t.Helper()
	exp := &backend.Experience{
		ID: "exp-" + module + "-" + scene, Module: module, Scene: scene,
		Keywords: keywords, Details: details, Version: "v1", CreatedAt: time.Now(),
	}
	if err := s.expDB.Create(exp); err != nil {
		t.Fatalf("create exp: %v", err)
	}
	return exp.ID
}

// ============== 测试 ==============

// TestRemoteClaim_ReturnsBuildTaskPrompt 验证：claim 成功后响应应包含 agent 可直接用的
// 最终 prompt 字符串（由 BuildTaskPrompt 生成，含 task 信息 + 经验库内容）。
// 这是远程 agent 执行任务的契约：拿到 claim 响应就能直接喂给 claude CLI。
func TestRemoteClaim_ReturnsBuildTaskPrompt(t *testing.T) {
	s := newRemoteClaimTestServer(t)
	agentID, token := registerTestAgent(t, s, "agent-prompt-1")

	expID := seedExperience(t, s, "git", "merge-conflict", "rebase",
		"用 git rebase + 编辑器手动解决冲突，commit --no-edit 后继续 rebase --continue")
	taskID := seedRemoteTask(t, s, "合并冲突", "main 分支有冲突", "冲突解决 + 测试通过", []string{expID})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/claim", s.handleTaskClaim)
	body, _ := json.Marshal(map[string]string{"agent_id": agentID})
	req := httptest.NewRequest("POST", "/api/tasks/"+taskID+"/claim", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("claim failed: status %d, body %s", w.Code, w.Body.String())
	}

	var resp struct {
		Status     string          `json:"status"`
		Task       *backend.Task   `json:"task"`
		Experiences []*backend.Experience `json:"experiences"`
		Prompt     string          `json:"prompt"` // 期望后端生成完整 prompt
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "claimed" {
		t.Errorf("status should be claimed, got %q", resp.Status)
	}
	if resp.Prompt == "" {
		t.Fatal("claim response should include prebuilt 'prompt' field for agent convenience")
	}
	// 验证 prompt 含 task 三要素
	for _, must := range []string{"合并冲突", "main 分支有冲突", "冲突解决 + 测试通过"} {
		if !strings.Contains(resp.Prompt, must) {
			t.Errorf("prompt missing task content %q\n---prompt---\n%s", must, resp.Prompt)
		}
	}
	// 验证 prompt 含经验库内容
	for _, must := range []string{"git", "merge-conflict", "rebase", "git rebase"} {
		if !strings.Contains(resp.Prompt, must) {
			t.Errorf("prompt missing experience content %q\n---prompt---\n%s", must, resp.Prompt)
		}
	}
}

// TestClaimNext_ReturnsBuildTaskPrompt claim-next 端点同样需要返回预生成的 prompt。
func TestClaimNext_ReturnsBuildTaskPrompt(t *testing.T) {
	s := newRemoteClaimTestServer(t)
	agentID, token := registerTestAgent(t, s, "agent-prompt-2")
	// claim-next 需要 auto_claim_enabled=true
	if err := s.agentDB.SetAutoClaimEnabled(agentID, true); err != nil {
		t.Fatalf("enable auto_claim: %v", err)
	}

	expID := seedExperience(t, s, "test", "unit-test", "mock", "用 testify mock 隔离外部依赖")
	taskID := seedRemoteTask(t, s, "补单测", "auth 模块覆盖率太低", "覆盖率 > 80%", []string{expID})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/claim-next", s.handleTaskClaimNext)
	body, _ := json.Marshal(map[string]string{"agent_id": agentID})
	req := httptest.NewRequest("POST", "/api/tasks/claim-next", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("claim-next failed: status %d, body %s", w.Code, w.Body.String())
	}

	var resp struct {
		Status     string           `json:"status"`
		Task       *backend.Task    `json:"task"`
		Experiences []*backend.Experience `json:"experiences"`
		Prompt     string           `json:"prompt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Prompt == "" {
		t.Fatal("claim-next response should include prebuilt 'prompt' field")
	}
	if !strings.Contains(resp.Prompt, "补单测") || !strings.Contains(resp.Prompt, "testify mock") {
		t.Errorf("prompt missing expected content:\n%s", resp.Prompt)
	}
	if resp.Task == nil || resp.Task.ID != taskID {
		t.Errorf("expected task %s, got %+v", taskID, resp.Task)
	}
}

// TestClaim_UnauthorizedAgentIDMismatch 验证 token 与 body 中 agent_id 不匹配时 401。
func TestClaim_UnauthorizedAgentIDMismatch(t *testing.T) {
	s := newRemoteClaimTestServer(t)
	_, token := registerTestAgent(t, s, "agent-mismatch")
	taskID := seedRemoteTask(t, s, "x", "d", "a", nil)
	_ = token // 下面用到

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/claim", s.handleTaskClaim)
	body, _ := json.Marshal(map[string]string{"agent_id": "fake-agent-id"})
	req := httptest.NewRequest("POST", "/api/tasks/"+taskID+"/claim", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestClaim_NoBearerToken 缺 token 应 401。
func TestClaim_NoBearerToken(t *testing.T) {
	s := newRemoteClaimTestServer(t)
	taskID := seedRemoteTask(t, s, "x", "d", "a", nil)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/claim", s.handleTaskClaim)
	body, _ := json.Marshal(map[string]string{"agent_id": "x"})
	req := httptest.NewRequest("POST", "/api/tasks/"+taskID+"/claim", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestClaim_AlreadyClaimed 已被 claim 的 task 再次 claim 应 409。
func TestClaim_AlreadyClaimed(t *testing.T) {
	s := newRemoteClaimTestServer(t)
	agentID, _ := registerTestAgent(t, s, "agent-dup")

	// 第二个 agent 试图抢已被第一个 claim 的 task
	agentID2, token2 := registerTestAgent(t, s, "agent-dup-2")

	taskID := seedRemoteTask(t, s, "x", "d", "a", nil)
	if err := s.db.ClaimTask(taskID, agentID); err != nil {
		t.Fatalf("first claim: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/claim", s.handleTaskClaim)
	body, _ := json.Marshal(map[string]string{"agent_id": agentID2})
	req := httptest.NewRequest("POST", "/api/tasks/"+taskID+"/claim", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token2)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestClaim_ManualTaskRejected 非 remote 类型的 task 不应被 claim。
func TestClaim_ManualTaskRejected(t *testing.T) {
	s := newRemoteClaimTestServer(t)
	agentID, token := registerTestAgent(t, s, "agent-manual")

	// 显式建一个 manual task（task_type=manual）
	task := &backend.Task{
		ID: "manual-task-1", Title: "manual", Description: "d", Acceptance: "a",
		Status: backend.TaskStatusPending, TaskType: backend.TaskTypeManual,
		Priority: 5, Version: "v1", CreatedAt: time.Now(),
	}
	if err := s.db.Create(task); err != nil {
		t.Fatalf("create: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/claim", s.handleTaskClaim)
	body, _ := json.Marshal(map[string]string{"agent_id": agentID})
	req := httptest.NewRequest("POST", "/api/tasks/"+task.ID+"/claim", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("manual task claim should be rejected, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestReport_NotClaimerRejected 非 claimer 上报应被拒。
func TestReport_NotClaimerRejected(t *testing.T) {
	s := newRemoteClaimTestServer(t)
	agentA, _ := registerTestAgent(t, s, "agent-rpt-a")
	agentBID, tokenB := registerTestAgent(t, s, "agent-rpt-b")

	taskID := seedRemoteTask(t, s, "x", "d", "a", nil)
	if err := s.db.ClaimTask(taskID, agentA); err != nil {
		t.Fatalf("claim: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/report", s.handleTaskReport)
	body, _ := json.Marshal(map[string]any{
		"agent_id": agentBID, "status": backend.TaskStatusArchived,
		"result_output": "fake result",
	})
	req := httptest.NewRequest("POST", "/api/tasks/"+taskID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenB)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("non-claimer report should fail, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestReport_SuccessSetsCompletedAt report 成功时 completed_at 应被设置。
func TestReport_SuccessSetsCompletedAt(t *testing.T) {
	s := newRemoteClaimTestServer(t)
	agentID, token := registerTestAgent(t, s, "agent-rpt-ok")
	taskID := seedRemoteTask(t, s, "x", "d", "a", nil)
	if err := s.db.ClaimTask(taskID, agentID); err != nil {
		t.Fatalf("claim: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks/{id}/report", s.handleTaskReport)
	body, _ := json.Marshal(map[string]any{
		"agent_id": agentID, "status": backend.TaskStatusArchived,
		"result_output": "ok",
	})
	req := httptest.NewRequest("POST", "/api/tasks/"+taskID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("report failed: %d, %s", w.Code, w.Body.String())
	}

	got, err := s.db.Get(taskID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != backend.TaskStatusArchived {
		t.Errorf("status should be archived, got %q", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("completed_at should be set on report")
	}
	if got.ResultOutput != "ok" {
		t.Errorf("result_output should be 'ok', got %q", got.ResultOutput)
	}
}
