package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"github.com/xiaodongQ/xworkbench/internal/hub"
	"go.uber.org/zap"
)

// TestAIChatEndpoint requiresAuth returns 401 without token.
func TestAIChatEndpointRequiresAuth(t *testing.T) {
	server := newAPIServerForTest(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/ai/chat", server.handleAIChat)

	body := map[string]any{"messages": []map[string]string{{"role": "user", "content": "hi"}}}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/ai/chat", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Errorf("expected 401/403 without auth, got %d", w.Code)
	}
}

// TestAIConfigEndpointGet returns config without exposing api_key.
func TestAIConfigEndpointGet(t *testing.T) {
	server := newAPIServerForTest(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ai/config", server.handleAIConfigGet)

	req := httptest.NewRequest("GET", "/api/ai/config", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("config get failed: %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	aiChat := resp["ai_chat"].(map[string]any)
	if aiChat["api_key"] != nil && aiChat["api_key"] != "" {
		// api_key should be masked or empty
		key := aiChat["api_key"].(string)
		if len(key) > 4 && key[:4] == "sk-" && !strContains(key, "••••") {
			t.Errorf("api_key should be masked, got: %s", key)
		}
	}
}

// TestAIConfigEndpointUpdate accepts non-api_key fields.
func TestAIConfigEndpointUpdate(t *testing.T) {
	server := newAPIServerForTest(t)
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/ai/config", server.handleAIConfigUpdate)

	body := map[string]any{
		"provider": "openai",
		"model":    "gpt-4o-mini",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/ai/config", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
		t.Fatalf("config update failed: %d - %s", w.Code, w.Body.String())
	}
}

// TestAIProviderFromDisabledConfig returns nil.
func TestAIProviderFromDisabledConfig(t *testing.T) {
	cfg := snapshotEmptyConfig()
	p := NewAIProviderFromConfig(cfg)
	if p != nil {
		t.Errorf("expected nil provider for disabled config, got %T", p)
	}
}

// TestAIProviderFromOpenAIConfig returns OpenAI provider.
func TestAIProviderFromOpenAIConfig(t *testing.T) {
	cfg := snapshotEmptyConfig()
	cfg.AIChat.ActiveProvider = "openai"
	cfg.AIChat.OpenAI.APIKey = "sk-test-key"
	cfg.AIChat.OpenAI.Model = "gpt-4o"
	cfg.AIChat.OpenAI.Temperature = 0.9

	p := NewAIProviderFromConfig(cfg)
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	_, ok := p.(*OpenAIProvider)
	if !ok {
		t.Errorf("expected *OpenAIProvider, got %T", p)
	}
}

// TestAIProviderFromAnthropicConfig returns Anthropic provider.
func TestAIProviderFromAnthropicConfig(t *testing.T) {
	cfg := snapshotEmptyConfig()
	cfg.AIChat.ActiveProvider = "anthropic"
	cfg.AIChat.Anthropic.APIKey = "sk-ant-test"
	cfg.AIChat.Anthropic.Model = "claude-sonnet-4"

	p := NewAIProviderFromConfig(cfg)
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	_, ok := p.(*AnthropicProvider)
	if !ok {
		t.Errorf("expected *AnthropicProvider, got %T", p)
	}
}

// --- Helpers ---

func strContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func newAPIServerForTest(t *testing.T) *APIServer {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	if logger == nil {
		zl, _ := zap.NewProduction()
		logger = zl.Sugar()
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

func snapshotEmptyConfig() *config.Config {
	return &config.Config{}
}