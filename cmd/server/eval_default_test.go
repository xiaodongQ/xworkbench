package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"go.uber.org/zap"
)

// 复用 config_export_import_test.go 的 test server 模式（独立构造，不依赖全局状态）
func newEvalDefaultTestServer(t *testing.T) (*APIServer, *http.ServeMux) {
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
		db:      backend.NewTaskRepo(db),
		expDB:   backend.NewExperienceRepo(db),
		dirDB:   backend.NewDirShortcutRepo(db),
		linkDB:  backend.NewWebLinkRepo(db),
		schedDB: backend.NewScheduledTaskRepo(db),
		running: map[string]context.CancelFunc{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handleSetConfig)
	mux.HandleFunc("GET /api/models", s.handleModelList)
	return s, mux
}

// TestEvalDefault_PutConfig 验证 PUT /api/config 接受 eval_model_defaults 字段，
// 写入 AppConfig.Models[cli].EvalDefault，且 GET /api/config + /api/models 都返回正确。
func TestEvalDefault_PutConfig(t *testing.T) {
	// 隔离 AppConfig：保存原值快照，测后恢复（深拷贝，不受字段修改影响）
	t.Cleanup(config.TestSnapshotAndRestore())

	// 重置 AppConfig 到默认值，避免其他测试污染
	config.Set(config.DefaultConfig())

	_, mux := newEvalDefaultTestServer(t)

	// 1. PUT eval_model_defaults: { claude: "opus" }
	w := doRequest(t, mux, "PUT", "/api/config", map[string]any{
		"eval_model_defaults": map[string]string{"claude": "opus"},
	})
	if w.Code != 200 {
		t.Fatalf("PUT status=%d body=%s", w.Code, w.Body.String())
	}
	if got := config.Get().Models["claude"].EvalDefault; got != "opus" {
		t.Errorf("after PUT, claude.EvalDefault=%q, want %q", got, "opus")
	}

	// 2. GET /api/config 应当返回 eval_default 字段
	w = doRequest(t, mux, "GET", "/api/config", nil)
	if w.Code != 200 {
		t.Fatalf("GET /api/config status=%d body=%s", w.Code, w.Body.String())
	}
	var cfgResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &cfgResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	models, _ := cfgResp["models"].(map[string]any)
	claude, _ := models["claude"].(map[string]any)
	if got, _ := claude["eval_default"].(string); got != "opus" {
		t.Errorf("GET /api/config: models.claude.eval_default=%q, want %q", got, "opus")
	}

	// 3. GET /api/models 也应当返回 cli_type_models[claude].eval_default
	w = doRequest(t, mux, "GET", "/api/models", nil)
	if w.Code != 200 {
		t.Fatalf("GET /api/models status=%d body=%s", w.Code, w.Body.String())
	}
	var modelsResp struct {
		CLITypes map[string]struct {
			Default     string `json:"default"`
			EvalDefault string `json:"eval_default"`
		} `json:"cli_type_models"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &modelsResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := modelsResp.CLITypes["claude"].EvalDefault; got != "opus" {
		t.Errorf("GET /api/models: cli_type_models.claude.eval_default=%q, want %q", got, "opus")
	}
}

// TestEvalDefault_FallbackChain 验证未传 eval_default 时，PUT 仅改 model_defaults
// 不会影响 eval_default（独立保存）。
func TestEvalDefault_FallbackChain(t *testing.T) {
	t.Cleanup(config.TestSnapshotAndRestore())
	config.Set(config.DefaultConfig())
	// 初始：claude.eval_default = haiku (默认)
	if got := config.Get().Models["claude"].EvalDefault; got == "" {
		t.Fatal("precondition: claude.EvalDefault should be set in default config")
	}

	_, mux := newEvalDefaultTestServer(t)

	// 只改 model_defaults，不传 eval_model_defaults
	w := doRequest(t, mux, "PUT", "/api/config", map[string]any{
		"model_defaults": map[string]string{"claude": "opus"},
	})
	if w.Code != 200 {
		t.Fatalf("PUT status=%d", w.Code)
	}
	// model_defaults 改了
	if got := config.Get().Models["claude"].Default; got != "opus" {
		t.Errorf("claude.Default=%q, want %q", got, "opus")
	}
	// eval_default 保持不变（独立字段）
	if got := config.Get().Models["claude"].EvalDefault; got == "" {
		t.Errorf("claude.EvalDefault should be preserved (独立于 model_defaults)")
	}
}

// TestEvalDefault_FallbackHelper 验证 helper 函数：未设 → Default → "sonnet" 三级 fallback。
func TestEvalDefault_FallbackHelper(t *testing.T) {
	t.Cleanup(config.TestSnapshotAndRestore())

	t.Run("eval_default_set", func(t *testing.T) {
		config.Set(config.DefaultConfig())
		config.Update(func(c *config.Config) {
			cl := c.Models["claude"]
			cl.EvalDefault = "opus"
			c.Models["claude"] = cl
		})
		if got := evalDefaultModel("claude"); got != "opus" {
			t.Errorf("evalDefaultModel(claude)=%q, want %q", got, "opus")
		}
	})

	t.Run("eval_default_empty_falls_back_to_default", func(t *testing.T) {
		config.Set(config.DefaultConfig())
		config.Update(func(c *config.Config) {
			cl := c.Models["claude"]
			cl.EvalDefault = ""
			cl.Default = "sonnet"
			c.Models["claude"] = cl
		})
		if got := evalDefaultModel("claude"); got != "sonnet" {
			t.Errorf("evalDefaultModel(claude)=%q, want %q (fallback to default)", got, "sonnet")
		}
	})

	t.Run("both_empty_falls_back_to_sonnet", func(t *testing.T) {
		config.Set(config.DefaultConfig())
		config.Update(func(c *config.Config) {
			cl := c.Models["claude"]
			cl.EvalDefault = ""
			cl.Default = ""
			c.Models["claude"] = cl
		})
		if got := evalDefaultModel("claude"); got != "sonnet" {
			t.Errorf("evalDefaultModel(claude)=%q, want %q (last resort fallback)", got, "sonnet")
		}
	})

	t.Run("unknown_cli_falls_back_to_sonnet", func(t *testing.T) {
		config.Set(config.DefaultConfig())
		if got := evalDefaultModel("nonexistent"); got != "sonnet" {
			t.Errorf("evalDefaultModel(nonexistent)=%q, want %q", got, "sonnet")
		}
	})
}

// 避免 unused import 警告（httptest 在 doRequest 中实际使用）
var _ = httptest.NewRecorder
