package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAIChatConfigDefault verifies AIChat defaults are as per DefaultConfig().
// DefaultConfig() pre-fills anthropic as active provider with a default model.
func TestAIChatConfigDefault(t *testing.T) {
	c := DefaultConfig()
	if c.AIChat.ActiveProvider != "anthropic" {
		t.Errorf("AIChat.ActiveProvider: want 'anthropic', got %q", c.AIChat.ActiveProvider)
	}
	if c.AIChat.Anthropic.APIKey != "" {
		t.Errorf("AIChat.Anthropic.APIKey: want '', got %q", c.AIChat.Anthropic.APIKey)
	}
	if c.AIChat.Anthropic.Model == "" {
		t.Errorf("AIChat.Anthropic.Model: want non-empty default, got ''")
	}
}

// TestAIChatConfigSetGet verifies AIChat fields can be set and retrieved.
func TestAIChatConfigSetGet(t *testing.T) {
	cfg := &Config{
		AIChat: AIChatConfig{
			ActiveProvider: "openai",
			OpenAI: ProviderConfig{
				APIKey:      "sk-test123456",
				Model:       "gpt-4o",
				BaseURL:     "https://api.openai.com",
				Temperature: 0.8,
				MaxTokens:   8192,
			},
			Anthropic: ProviderConfig{
				APIKey:      "sk-ant-test",
				Model:       "claude-sonnet-4-20250514",
				BaseURL:     "https://api.anthropic.com",
				Temperature: 0.8,
				MaxTokens:   8192,
			},
			SystemPrompt: "你是一个助手",
		},
	}

	if cfg.AIChat.ActiveProvider != "openai" {
		t.Errorf("ActiveProvider: want openai, got %q", cfg.AIChat.ActiveProvider)
	}
	if cfg.AIChat.OpenAI.APIKey != "sk-test123456" {
		t.Errorf("OpenAI.APIKey: want sk-test123456, got %q", cfg.AIChat.OpenAI.APIKey)
	}
	if cfg.AIChat.OpenAI.Model != "gpt-4o" {
		t.Errorf("OpenAI.Model: want gpt-4o, got %q", cfg.AIChat.OpenAI.Model)
	}
	if cfg.AIChat.OpenAI.BaseURL != "https://api.openai.com" {
		t.Errorf("OpenAI.BaseURL: want https://api.openai.com, got %q", cfg.AIChat.OpenAI.BaseURL)
	}
	if cfg.AIChat.OpenAI.Temperature != 0.8 {
		t.Errorf("OpenAI.Temperature: want 0.8, got %f", cfg.AIChat.OpenAI.Temperature)
	}
	if cfg.AIChat.OpenAI.MaxTokens != 8192 {
		t.Errorf("OpenAI.MaxTokens: want 8192, got %d", cfg.AIChat.OpenAI.MaxTokens)
	}
	if cfg.AIChat.SystemPrompt != "你是一个助手" {
		t.Errorf("SystemPrompt: want '你是一个助手', got %q", cfg.AIChat.SystemPrompt)
	}
}

// TestAIChatConfigJSONRoundTrip verifies AIChat serializes and deserializes correctly.
func TestAIChatConfigJSONRoundTrip(t *testing.T) {
	cfg := &Config{
		AIChat: AIChatConfig{
			ActiveProvider: "anthropic",
			Anthropic: ProviderConfig{
				APIKey:      "sk-ant-test",
				Model:       "claude-sonnet-4-20250514",
				BaseURL:     "https://api.anthropic.com",
				Temperature: 0.5,
				MaxTokens:   4096,
			},
			OpenAI: ProviderConfig{
				APIKey:  "sk-openai-test",
				Model:   "gpt-4o",
				BaseURL: "https://api.openai.com",
			},
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

	if cp.AIChat.ActiveProvider != "anthropic" {
		t.Errorf("after round-trip: ActiveProvider = %q, want anthropic", cp.AIChat.ActiveProvider)
	}
	if cp.AIChat.Anthropic.APIKey != "sk-ant-test" {
		t.Errorf("after round-trip: Anthropic.APIKey = %q, want sk-ant-test", cp.AIChat.Anthropic.APIKey)
	}
	if cp.AIChat.Anthropic.Model != "claude-sonnet-4-20250514" {
		t.Errorf("after round-trip: Anthropic.Model = %q, want claude-sonnet-4-20250514", cp.AIChat.Anthropic.Model)
	}
}

// TestAIChatConfigMaskedAPIKey verifies API key is masked in config responses.
func TestAIChatConfigMaskedAPIKey(t *testing.T) {
	cfg := &Config{
		AIChat: AIChatConfig{
			ActiveProvider: "openai",
			OpenAI: ProviderConfig{
				APIKey: "sk-verylongapikey123456",
			},
		},
	}

	// MaskedAPIKey delegates to GetActive (OpenAI), which has BaseURL/APIKey/Model
	masked := cfg.AIChat.MaskedAPIKey()
	key := cfg.AIChat.OpenAI.APIKey
	want := key[:4] + strings.Repeat("•", len(key)-8) + key[len(key)-4:]
	if masked != want {
		t.Errorf("MaskedAPIKey: got %q, want %q", masked, want)
	}
}

// TestAIChatConfigMaskedAPIKeyShort verifies short keys are fully masked.
// Short key in the active (anthropic by default) provider.
func TestAIChatConfigMaskedAPIKeyShort(t *testing.T) {
	cfg := &Config{
		AIChat: AIChatConfig{
			ActiveProvider: "openai",
			OpenAI:         ProviderConfig{APIKey: "sk-ab"},
		},
	}
	masked := cfg.AIChat.MaskedAPIKey()
	// len<=8: return "sk-" + dots for len-3 = 2 dots
	if masked != "sk-"+strings.Repeat("•", 2) {
		t.Errorf("MaskedAPIKey short: got %q", masked)
	}
}

// TestAIChatConfigEnabled verifies IsEnabled returns true only when at least one
// provider has both APIKey and Model set.
func TestAIChatConfigEnabled(t *testing.T) {
	tests := []struct {
		name       string
		anthropic  ProviderConfig
		openai     ProviderConfig
		want       bool
	}{
		{
			name:      "both empty",
			anthropic: ProviderConfig{},
			openai:    ProviderConfig{},
			want:      false,
		},
		{
			name:      "anthropic api key only, no model",
			anthropic: ProviderConfig{APIKey: "sk-ant", Model: ""},
			openai:    ProviderConfig{},
			want:      false,
		},
		{
			name:      "openai api key only, no model",
			anthropic: ProviderConfig{},
			openai:    ProviderConfig{APIKey: "sk-test", Model: ""},
			want:      false,
		},
		{
			name:      "anthropic complete",
			anthropic: ProviderConfig{APIKey: "sk-ant", Model: "claude-sonnet-4-20250514"},
			openai:    ProviderConfig{},
			want:      true,
		},
		{
			name:      "openai complete",
			anthropic: ProviderConfig{},
			openai:    ProviderConfig{APIKey: "sk-test", Model: "gpt-4o"},
			want:      true,
		},
		{
			name:      "both complete",
			anthropic: ProviderConfig{APIKey: "sk-ant", Model: "claude-sonnet-4-20250514"},
			openai:    ProviderConfig{APIKey: "sk-test", Model: "gpt-4o"},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				AIChat: AIChatConfig{
					Anthropic: tt.anthropic,
					OpenAI:    tt.openai,
				},
			}
			if got := cfg.AIChat.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled: want %v, got %v", tt.want, got)
			}
		})
	}
}
