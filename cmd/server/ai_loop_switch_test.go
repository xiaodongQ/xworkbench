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

// TestAILoopStatus_DefaultOff 验证默认状态：未启用，源是 "default"
// 这个测试是回归保护：AI 自治能力默认关，需要用户主动开（防止误用 + 默认安全）。
func TestAILoopStatus_DefaultOff(t *testing.T) {
	srv := newTestAPIServer(t)
	defer srv.cleanup()
	// 显式清除 AppSettings 里可能留下的 ai_loop_enabled（其他测试可能已开）
	_ = srv.srv.setDB.Set("ai_loop_enabled", "")

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
	if resp["source"] != "default" {
		t.Errorf("default source = %v, want \"default\"", resp["source"])
	}
}

// TestAILoopStatus_ConfigEnabled 验证 config.json ai_loop.enabled=true 时 enabled
// 这是位置文件控制的路径。
func TestAILoopStatus_ConfigEnabled(t *testing.T) {
	// 备份 + 恢复 config.AppConfig
	orig := config.AppConfig
	defer func() { config.AppConfig = orig }()
	config.AppConfig = &config.Config{AILoop: config.AILoopConfig{Enabled: true}}

	srv := newTestAPIServer(t)
	defer srv.cleanup()
	_ = srv.srv.setDB.Set("ai_loop_enabled", "")

	rec := httptest.NewRecorder()
	srv.srv.handleAILoopStatus(rec, httptest.NewRequest("GET", "/api/ai-loop/status", nil))
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["enabled"] != true {
		t.Errorf("config-enabled = %v, want true; body=%s", resp["enabled"], rec.Body.String())
	}
	if resp["source"] != "config.json" {
		t.Errorf("source = %v, want config.json", resp["source"])
	}
}

// TestAILoopStatus_AppSettingsOverridesConfig 验证 AppSettings 优先级高于 config.json
// 这是"运行时热调"的路径：用户在设置页 PUT 之后立即生效。
func TestAILoopStatus_AppSettingsOverridesConfig(t *testing.T) {
	orig := config.AppConfig
	defer func() { config.AppConfig = orig }()
	config.AppConfig = &config.Config{AILoop: config.AILoopConfig{Enabled: false}} // config 关

	srv := newTestAPIServer(t)
	defer srv.cleanup()
	defer func() { _ = srv.srv.setDB.Set("ai_loop_enabled", "") }()

	// 在 AppSettings 里开
	if err := srv.srv.setDB.Set("ai_loop_enabled", "1"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.srv.handleAILoopStatus(rec, httptest.NewRequest("GET", "/api/ai-loop/status", nil))
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["enabled"] != true {
		t.Errorf("app_settings should override config; enabled = %v, want true", resp["enabled"])
	}
	if resp["source"] != "app_settings" {
		t.Errorf("source = %v, want app_settings", resp["source"])
	}
}

// TestAILoopStatus_AppSettingsOverridesConfig_Off 验证 AppSettings 也能"覆盖关闭"
// 即用户从设置页关掉 AI 自治，config.json 即使开了也会被 AppSettings 覆盖。
func TestAILoopStatus_AppSettingsOverridesConfig_Off(t *testing.T) {
	orig := config.AppConfig
	defer func() { config.AppConfig = orig }()
	config.AppConfig = &config.Config{AILoop: config.AILoopConfig{Enabled: true}} // config 开

	srv := newTestAPIServer(t)
	defer srv.cleanup()
	defer func() { _ = srv.srv.setDB.Set("ai_loop_enabled", "") }()

	// 在 AppSettings 里关
	if err := srv.srv.setDB.Set("ai_loop_enabled", "0"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.srv.handleAILoopStatus(rec, httptest.NewRequest("GET", "/api/ai-loop/status", nil))
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["enabled"] != false {
		t.Errorf("app_settings off should override config on; enabled = %v, want false", resp["enabled"])
	}
	if resp["source"] != "app_settings" {
		t.Errorf("source = %v, want app_settings", resp["source"])
	}
}

// TestRunLoop_Disabled_Returns403 验证 run-loop handler 在开关未启用时返回 403
func TestRunLoop_Disabled_Returns403(t *testing.T) {
	orig := config.AppConfig
	defer func() { config.AppConfig = orig }()
	config.AppConfig = &config.Config{} // 默认关

	srv := newTestAPIServer(t)
	defer srv.cleanup()
	_ = srv.srv.setDB.Set("ai_loop_enabled", "")

	// 调 run-loop 端点
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
	config.AppConfig = &config.Config{} // 默认关

	srv := newTestAPIServer(t)
	defer srv.cleanup()
	_ = srv.srv.setDB.Set("ai_loop_enabled", "")

	body := `{"model":"sonnet"}`
	req := httptest.NewRequest("POST", "/api/tasks/test-id/reevaluate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "test-id")
	rec := httptest.NewRecorder()
	srv.srv.handleTaskReevaluate(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("disabled reevaluate code = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

// TestLearn_Disabled_Returns403 验证 learn 在开关未启用时返回 403
func TestLearn_Disabled_Returns403(t *testing.T) {
	orig := config.AppConfig
	defer func() { config.AppConfig = orig }()
	config.AppConfig = &config.Config{} // 默认关

	srv := newTestAPIServer(t)
	defer srv.cleanup()
	_ = srv.srv.setDB.Set("ai_loop_enabled", "")

	req := httptest.NewRequest("POST", "/api/tasks/test-id/learn", nil)
	req.SetPathValue("id", "test-id")
	rec := httptest.NewRecorder()
	srv.srv.handleTaskLearn(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("disabled learn code = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

// testServer 包装：提供 srv + cleanup。
type testServer struct {
	srv     *APIServer
	cleanup func()
}

func newTestAPIServer(t *testing.T) *testServer {
	t.Helper()
	// 借用 eval_loop_test.go 里的 newEvalTestServer，扩展补 setDB 已经在
	// （newEvalTestServer 已经设了 setDB）
	s := newEvalTestServer(t)
	if s.setDB == nil {
		t.Fatal("setDB is nil in test server")
	}
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	return &testServer{srv: s, cleanup: func() {}}
}
