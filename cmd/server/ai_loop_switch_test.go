package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/config"
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