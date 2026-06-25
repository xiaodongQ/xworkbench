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
	"go.uber.org/zap"
)

// TestAILoopStatus_DefaultOff 验证默认状态：未启用（config.json 默认关）。
// 这个测试是回归保护：AI 自治能力默认关，需要用户主动开（防止误用 + 默认安全）。
func TestAILoopStatus_DefaultOff(t *testing.T) {
	orig := config.AppConfig
	defer func() { config.AppConfig = orig }()

	srv := newTestAPIServer(t)
	defer srv.cleanup()
	// helper 默认开 AILoopEnabled，关掉以验证"未启用"路径
	config.AppConfig.AILoopEnabled = false

	rec := httptest.NewRecorder()
	srv.srv.handleAILoopStatus(rec, httptest.NewRequest("GET", "/api/ai-loop/status", nil))
	if rec.Code != 200 {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v, raw=%s", err, rec.Body.String())
	}
	if resp["enabled"] != false {
		t.Errorf("default enabled = %v, want false", resp["enabled"])
	}
}

// TestAILoopStatus_ConfigEnabled 验证 config.json ai_loop_enabled=true 时 enabled
// 这是单一来源（config.json）的路径。
func TestAILoopStatus_ConfigEnabled(t *testing.T) {
	orig := config.AppConfig
	defer func() { config.AppConfig = orig }()
	config.AppConfig = &config.Config{AILoopEnabled: true}

	srv := newTestAPIServer(t)
	defer srv.cleanup()

	rec := httptest.NewRecorder()
	srv.srv.handleAILoopStatus(rec, httptest.NewRequest("GET", "/api/ai-loop/status", nil))
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["enabled"] != true {
		t.Errorf("config-enabled = %v, want true; body=%s", resp["enabled"], rec.Body.String())
	}
}

// TestRunLoop_Disabled_Returns403 验证 run-loop handler 在开关未启用时返回 403
func TestRunLoop_Disabled_Returns403(t *testing.T) {
	orig := config.AppConfig
	defer func() { config.AppConfig = orig }()

	srv := newTestAPIServer(t)
	defer srv.cleanup()
	config.AppConfig.AILoopEnabled = false

	body := `{"prompt":"test","model":"sonnet"}`
	req := httptest.NewRequest("POST", "/api/tasks/test-id/run-loop", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "test-id")
	rec := httptest.NewRecorder()
	srv.srv.handleTaskRunLoop(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("disabled run-loop code = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "未启用") {
		t.Errorf("body should contain \"未启用\", got: %s", rec.Body.String())
	}
}

// TestReevaluate_Disabled_Returns403 验证 reevaluate 在开关未启用时返回 403
func TestReevaluate_Disabled_Returns403(t *testing.T) {
	orig := config.AppConfig
	defer func() { config.AppConfig = orig }()

	srv := newTestAPIServer(t)
	defer srv.cleanup()
	config.AppConfig.AILoopEnabled = false

	body := `{"model":"sonnet"}`
	req := httptest.NewRequest("POST", "/api/tasks/test-id/reevaluate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "test-id")
	rec := httptest.NewRecorder()
	srv.srv.handleTaskReevaluate(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("disabled reevaluate code = %d, want 403", rec.Code)
	}
}

// TestLearn_Disabled_Returns403 验证 learn 在开关未启用时返回 403
func TestLearn_Disabled_Returns403(t *testing.T) {
	orig := config.AppConfig
	defer func() { config.AppConfig = orig }()

	srv := newTestAPIServer(t)
	defer srv.cleanup()
	config.AppConfig.AILoopEnabled = false

	req := httptest.NewRequest("POST", "/api/tasks/test-id/learn", nil)
	req.SetPathValue("id", "test-id")
	rec := httptest.NewRecorder()
	srv.srv.handleTaskLearn(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("disabled learn code = %d, want 403", rec.Code)
	}
}

// testServer 包装：提供 srv + cleanup。
type testServer struct {
	srv     *APIServer
	cleanup func()
}

func newTestAPIServer(t *testing.T) *testServer {
	t.Helper()
	s := newEvalTestServer(t)
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	return &testServer{srv: s, cleanup: func() {}}
}

// TestRunLoop_ReturnsImmediatelyWithStatusStarted 验证 handleTaskRunLoop 立即返回
// （不阻塞 HTTP handler），body 含 {status:"started"}，这是把 handler 从同步改成
// 异步 + WS 推送的核心改动回归保护。
//
// 旧实现：handler 在 for 循环里跑 max_iterations 次 claude -p + evaluator，默认
// 阻塞 1.5-3 分钟，配合 WriteTimeout=60s 必然 client timeout/server 关闭连接。
// 新实现：handler 立即返 202 + task_id,后台 goroutine 跑循环,通过 WS 推进度。
//
// 用 shell prompt("echo hi") 让 background 跑得快(虽然我们不验证 background 完成),
// 主要断言：HTTP 返回时间 < 500ms(背景再慢也不能卡 handler)+ body 字段正确。
// 需要真实 task 存在(handler 先 s.db.Get 校验) + hub 存在(background WS 推送用)。
func TestRunLoop_ReturnsImmediatelyWithStatusStarted(t *testing.T) {
	srv := newTestAPIServer(t)
	defer srv.cleanup()
	// background goroutine 跑 WS 推送,需要 hub 否则 nil panic
	srv.srv.hub = hub.New()
	// handler 校验 task 存在,先建一个
	if err := srv.srv.db.Create(&backend.Task{
		ID:        "test-id",
		Title:     "test",
		Status:    "pending",
		Version:   "v0.0.1",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	body := `{"prompt":"echo hi","cli_type":"shell","max_iterations":1}`
	req := httptest.NewRequest("POST", "/api/tasks/test-id/run-loop", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "test-id")
	rec := httptest.NewRecorder()

	start := time.Now()
	srv.srv.handleTaskRunLoop(rec, req)
	elapsed := time.Since(start)

	// 1. handler 不应被 background 阻塞:即使 max_iterations 较大,HTTP 响应必须 < 500ms
	//    (旧实现:同步跑至少一次 claude 30s+,必然超时)
	if elapsed > 500*time.Millisecond {
		t.Errorf("run-loop handler blocked %v, want < 500ms (异步化失效)", elapsed)
	}

	// 2. body 含 status=started(证明 handler 走的是异步分支,不是同步写 result)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v, raw=%s", err, rec.Body.String())
	}
	if resp["status"] != "started" {
		t.Errorf("resp.status = %v, want \"started\" (同步实现会返 {loop_done, history}); body=%s", resp["status"], rec.Body.String())
	}
	if resp["task_id"] != "test-id" {
		t.Errorf("resp.task_id = %v, want \"test-id\"", resp["task_id"])
	}

	// 给 background goroutine 一点时间完成(shell echo 通常 < 1s),避免进程退出时还在跑
	// 测试只验证 handler 立即返回,不验证 background 完成;后者要更重的 mock
	time.Sleep(500 * time.Millisecond)
}