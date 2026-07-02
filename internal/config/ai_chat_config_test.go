package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAIChatConfigDefault verifies AIChat defaults to empty (disabled).
func TestAIChatConfigDefault(t *testing.T) {
	c := DefaultConfig()
	if c.AIChat.Provider != "" {
		t.Errorf("AIChat.Provider: want '', got %q", c.AIChat.Provider)
	}
	if c.AIChat.APIKey != "" {
		t.Errorf("AIChat.APIKey: want '', got %q", c.AIChat.APIKey)
	}
	if c.AIChat.Model != "" {
		t.Errorf("AIChat.Model: want '', got %q", c.AIChat.Model)
	}
}

// TestAIChatConfigSetGet verifies AIChat fields can be set and retrieved.
func TestAIChatConfigSetGet(t *testing.T) {
	cfg := &Config{
		AIChat: AIChatConfig{
			Provider:    "openai",
			APIKey:      "sk-test123456",
			Model:       "gpt-4o",
			BaseURL:     "https://api.openai.com",
			Temperature: 0.8,
			MaxTokens:   8192,
			SystemPrompt: "你是一个助手",
		},
	}

	if cfg.AIChat.Provider != "openai" {
		t.Errorf("Provider: want openai, got %q", cfg.AIChat.Provider)
	}
	if cfg.AIChat.APIKey != "sk-test123456" {
		t.Errorf("APIKey: want sk-test123456, got %q", cfg.AIChat.APIKey)
	}
	if cfg.AIChat.Model != "gpt-4o" {
		t.Errorf("Model: want gpt-4o, got %q", cfg.AIChat.Model)
	}
	if cfg.AIChat.BaseURL != "https://api.openai.com" {
		t.Errorf("BaseURL: want https://api.openai.com, got %q", cfg.AIChat.BaseURL)
	}
	if cfg.AIChat.Temperature != 0.8 {
		t.Errorf("Temperature: want 0.8, got %f", cfg.AIChat.Temperature)
	}
	if cfg.AIChat.MaxTokens != 8192 {
		t.Errorf("MaxTokens: want 8192, got %d", cfg.AIChat.MaxTokens)
	}
	if cfg.AIChat.SystemPrompt != "你是一个助手" {
		t.Errorf("SystemPrompt: want '你是一个助手', got %q", cfg.AIChat.SystemPrompt)
	}
}

// TestAIChatConfigJSONRoundTrip verifies AIChat serializes and deserializes correctly.
func TestAIChatConfigJSONRoundTrip(t *testing.T) {
	cfg := &Config{
		AIChat: AIChatConfig{
			Provider:    "anthropic",
			APIKey:      "sk-ant-test",
			Model:       "claude-sonnet-4",
			BaseURL:     "https://api.anthropic.com",
			Temperature: 0.5,
			MaxTokens:   4096,
		},
	}

	// Serialize
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Deserialize
	var cp Config
	if err := json.Unmarshal(data, &cp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if cp.AIChat.Provider != "anthropic" {
		t.Errorf("after round-trip: Provider = %q, want anthropic", cp.AIChat.Provider)
	}
	if cp.AIChat.APIKey != "sk-ant-test" {
		t.Errorf("after round-trip: APIKey = %q, want sk-ant-test", cp.AIChat.APIKey)
	}
	if cp.AIChat.Model != "claude-sonnet-4" {
		t.Errorf("after round-trip: Model = %q, want claude-sonnet-4", cp.AIChat.Model)
	}
}

// TestAIChatConfigMaskedAPIKey verifies API key is masked in config responses.
func TestAIChatConfigMaskedAPIKey(t *testing.T) {
	cfg := &Config{
		AIChat: AIChatConfig{
			Provider: "openai",
			APIKey:   "sk-verylongapikey123456",
		},
	}

	masked := cfg.AIChat.MaskedAPIKey()
	want := cfg.AIChat.APIKey[:4] + strings.Repeat("•", len(cfg.AIChat.APIKey)-8) + cfg.AIChat.APIKey[len(cfg.AIChat.APIKey)-4:]
	if masked != want {
		t.Errorf("MaskedAPIKey: got %q, want %q", masked, want)
	}
}

// TestAIChatConfigMaskedAPIKeyShort verifies short keys are fully masked.
func TestAIChatConfigMaskedAPIKeyShort(t *testing.T) {
	cfg := &Config{AIChat: AIChatConfig{APIKey: "sk-ab"}} // len=5, <=8, all masked after sk-
	masked := cfg.AIChat.MaskedAPIKey()
	// len<=8: return "sk-" + dots for len-3 = 2 dots
	if masked != "sk-"+strings.Repeat("•", 2) {
		t.Errorf("MaskedAPIKey short: got %q", masked)
	}
}

// TestAIChatConfigEnabled verifies IsEnabled returns true only when provider + api_key are set.
func TestAIChatConfigEnabled(t *testing.T) {
	tests := []struct {
		provider, apiKey string
		want             bool
	}{
		{"", "", false},
		{"openai", "", false},
		{"", "sk-test", false},
		{"openai", "sk-test", true},
		{"anthropic", "sk-ant", true},
	}

	for _, tt := range tests {
		cfg := &Config{AIChat: AIChatConfig{Provider: tt.provider, APIKey: tt.apiKey}}
		if got := cfg.AIChat.IsEnabled(); got != tt.want {
			t.Errorf("IsEnabled(provider=%q, apiKey=%q): want %v, got %v",
				tt.provider, tt.apiKey, tt.want, got)
		}
	}
}