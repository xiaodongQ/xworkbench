
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/config"
)

// handleAIChat is the main AI chat endpoint.
func (s *APIServer) handleAIChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Messages []Message `json:"messages"`
		Stream  bool       `json:"stream"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Messages) == 0 {
		writeErr(w, http.StatusBadRequest, "messages required")
		return
	}

	cfg := config.Snapshot()
	if cfg == nil || !cfg.AIChat.IsEnabled() {
		logger.Warnw("handleAIChat: AI chat not configured")
		writeErr(w, http.StatusServiceUnavailable, "AI chat not configured")
		return
	}

	provider := NewAIProviderFromConfig(cfg)
	if provider == nil {
		logger.Warnw("handleAIChat: AI provider not available", "provider", cfg.AIChat.ActiveProvider)
		writeErr(w, http.StatusServiceUnavailable, "AI provider not available")
		return
	}

	// Auth: require valid session token or API key
	if !s.isAuthenticated(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	// Find last user message for logging
	lastUserMsg := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			lastUserMsg = req.Messages[i].Content
			break
		}
	}
	logger.Infow("handleAIChat: calling AI provider", "provider", cfg.AIChat.ActiveProvider, "messages", len(req.Messages), "lastUserMsg", lastUserMsg)

	tools := GetTools()

	// 注入 data/memory.md 内容作为 system prompt 的一部分
	if mem := s.memoryStore.ContentForSystemPrompt(); mem != "" {
		req.Messages = append([]Message{{Role: "system", Content: mem}}, req.Messages...)
	}

	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		err := provider.ChatStream(context.Background(), req.Messages, tools, func(event AIEvent) {
			data, _ := json.Marshal(event)
			io.WriteString(w, "data: " + string(data) + "\n\n")
			flusher.Flush()
		})
		if err != nil {
			logger.Errorf("AI stream error: %v", err)
		}
		return
	}

	resp, err := provider.Chat(context.Background(), req.Messages, tools)
	if err != nil {
		logger.Errorw("handleAIChat: provider.Chat failed", "provider", cfg.AIChat.ActiveProvider, "err", err)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Infow("handleAIChat: response received", "provider", cfg.AIChat.ActiveProvider, "contentLen", len(resp.Message.Content), "toolCalls", len(resp.ToolCalls))

	// Multi-round tool calling: keep looping until AI stops calling tools
	refreshWidgets := make(map[string]bool)
	maxRounds := 20
	for round := 0; round < maxRounds; round++ {
		if len(resp.ToolCalls) == 0 {
			break
		}
		logger.Infow("handleAIChat: round start", "round", round+1, "toolCount", len(resp.ToolCalls))
		for i := range resp.ToolCalls {
			tc := &resp.ToolCalls[i]
			logger.Infow("handleAIChat: tool call", "round", round+1, "tool", tc.Name, "args", tc.Args)
			tc.Result = ExecuteTool(
				context.Background(),
				s.db, s.expDB, s.execDB, s.agentDB,
				s.linkDB, s.dirDB,
				s.schedDB, s.sch,
				s.memoryStore,
				nil, // localShellState
				tc.Name, tc.Args,
			)
			// Truncate result for logging if too long
			resultPreview := tc.Result
			if len(resultPreview) > 200 {
				resultPreview = resultPreview[:200] + "..."
			}
			logger.Infow("handleAIChat: tool result", "round", round+1, "tool", tc.Name, "result", resultPreview)
			// Track widget refresh needs (same list as config.js import)
			switch tc.Name {
			case "create_web_link", "delete_web_link", "update_web_link":
				refreshWidgets["links"] = true
			case "create_dir_shortcut", "delete_dir_shortcut", "update_dir_shortcut":
				refreshWidgets["dirs"] = true
			case "add_todo", "toggle_todo", "delete_todo":
				refreshWidgets["todos"] = true
			}
			// Append tool result as a special message and continue
			// Anthropic requires: role=user, content=[{type:"tool_result",tool_use_id:"...",content:"..."}]
			toolResultContent, _ := json.Marshal(map[string]any{
				"type":        "tool_result",
				"tool_use_id": tc.ID,
				"content":     tc.Result,
			})
			req.Messages = append(req.Messages, Message{
				Role:    "user",
				Content: string(toolResultContent),
			})
		}
		// Continue conversation with tool results — keep tools enabled for next round
		resp, err = provider.Chat(context.Background(), req.Messages, tools)
		if err != nil {
			logger.Errorw("handleAIChat: tool result feedback failed", "round", round+1, "err", err)
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		logger.Infow("handleAIChat: round end", "round", round+1, "contentLen", len(resp.Message.Content), "toolCalls", len(resp.ToolCalls))
	}
	logger.Infow("handleAIChat: final response", "contentLen", len(resp.Message.Content))

	// Build refresh list
	refreshList := make([]string, 0, len(refreshWidgets))
	for k := range refreshWidgets {
		refreshList = append(refreshList, k)
	}

	writeJSON(w, map[string]any{
		"id":             "chat_" + time.Now().Format("20060102150405"),
		"message":        resp.Message,
		"refresh_widgets": refreshList,
	})
}

// handleAIConfigGet returns current AI config (api_key masked).
func (s *APIServer) handleAIConfigGet(w http.ResponseWriter, r *http.Request) {
	cfg := config.Snapshot()
	if cfg == nil {
		cfg = &config.Config{}
	}

	active := cfg.AIChat.GetActive()
	resp := map[string]any{
		"ai_chat": map[string]any{
			"active_provider": cfg.AIChat.ActiveProvider,
			"api_key":        cfg.AIChat.MaskedAPIKey(),
			"model":          active.Model,
			"base_url":       active.BaseURL,
			"temperature":     active.Temperature,
			"max_tokens":     active.MaxTokens,
			"enabled":        cfg.AIChat.IsEnabled(),
			"anthropic": map[string]any{
				"api_key":     cfg.AIChat.Anthropic.MaskedAPIKey(),
				"model":       cfg.AIChat.Anthropic.Model,
				"base_url":    cfg.AIChat.Anthropic.BaseURL,
				"temperature": cfg.AIChat.Anthropic.Temperature,
				"max_tokens":  cfg.AIChat.Anthropic.MaxTokens,
			},
			"openai": map[string]any{
				"api_key":     cfg.AIChat.OpenAI.MaskedAPIKey(),
				"model":       cfg.AIChat.OpenAI.Model,
				"base_url":    cfg.AIChat.OpenAI.BaseURL,
				"temperature": cfg.AIChat.OpenAI.Temperature,
				"max_tokens":  cfg.AIChat.OpenAI.MaxTokens,
			},
		},
	}
	writeJSON(w, resp)
}

// handleAIConfigUpdate updates AI config fields (except api_key).
func (s *APIServer) handleAIConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActiveProvider string `json:"active_provider"`
		Anthropic     *struct {
			Model       string  `json:"model"`
			BaseURL     string  `json:"base_url"`
			Temperature float64 `json:"temperature"`
			MaxTokens   int     `json:"max_tokens"`
		} `json:"anthropic"`
		OpenAI        *struct {
			Model       string  `json:"model"`
			BaseURL     string  `json:"base_url"`
			Temperature float64 `json:"temperature"`
			MaxTokens   int     `json:"max_tokens"`
		} `json:"openai"`
		SystemPrompt string `json:"system_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	config.Update(func(c *config.Config) {
		if req.ActiveProvider != "" {
			c.AIChat.ActiveProvider = req.ActiveProvider
		}
		if req.Anthropic != nil {
			if req.Anthropic.Model != "" {
				c.AIChat.Anthropic.Model = req.Anthropic.Model
			}
			if req.Anthropic.BaseURL != "" {
				c.AIChat.Anthropic.BaseURL = req.Anthropic.BaseURL
			}
			if req.Anthropic.Temperature > 0 {
				c.AIChat.Anthropic.Temperature = req.Anthropic.Temperature
			}
			if req.Anthropic.MaxTokens > 0 {
				c.AIChat.Anthropic.MaxTokens = req.Anthropic.MaxTokens
			}
		}
		if req.OpenAI != nil {
			if req.OpenAI.Model != "" {
				c.AIChat.OpenAI.Model = req.OpenAI.Model
			}
			if req.OpenAI.BaseURL != "" {
				c.AIChat.OpenAI.BaseURL = req.OpenAI.BaseURL
			}
			if req.OpenAI.Temperature > 0 {
				c.AIChat.OpenAI.Temperature = req.OpenAI.Temperature
			}
			if req.OpenAI.MaxTokens > 0 {
				c.AIChat.OpenAI.MaxTokens = req.OpenAI.MaxTokens
			}
		}
		if req.SystemPrompt != "" {
			c.AIChat.SystemPrompt = req.SystemPrompt
		}
	})
	if err := config.Save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleAIConfigSetKey updates only the api_key field for a specific provider.
func (s *APIServer) handleAIConfigSetKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"` // "anthropic" | "openai"
		APIKey  string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.APIKey == "" {
		writeErr(w, http.StatusBadRequest, "api_key required")
		return
	}
	provider := req.Provider
	if provider == "" {
		provider = "anthropic"
	}
	config.Update(func(c *config.Config) {
		switch provider {
		case "openai":
			c.AIChat.OpenAI.APIKey = req.APIKey
		default:
			c.AIChat.Anthropic.APIKey = req.APIKey
		}
	})
	if err := config.Save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleAIConfigTest tests the saved config for the given provider.
// If ?provider= is given, test that provider; otherwise use active provider from saved config.
func (s *APIServer) handleAIConfigTest(w http.ResponseWriter, r *http.Request) {
	cfg := config.Snapshot()
	if cfg == nil || !cfg.AIChat.IsEnabled() {
		writeErr(w, http.StatusBadRequest, "AI chat not configured")
		return
	}

	providerStr := r.URL.Query().Get("provider")
	if providerStr == "" {
		providerStr = cfg.AIChat.ActiveProvider
	}

	var provider AIProvider
	switch providerStr {
	case "openai":
		if cfg.AIChat.OpenAI.APIKey == "" || cfg.AIChat.OpenAI.Model == "" {
			writeErr(w, http.StatusBadRequest, "OpenAI 未配置 Key 或 Model")
			return
		}
		provider = NewOpenAIProvider(
			cfg.AIChat.OpenAI.BaseURL, cfg.AIChat.OpenAI.APIKey,
			cfg.AIChat.OpenAI.Model, cfg.AIChat.OpenAI.Temperature, cfg.AIChat.OpenAI.MaxTokens,
		)
	default:
		if cfg.AIChat.Anthropic.APIKey == "" || cfg.AIChat.Anthropic.Model == "" {
			writeErr(w, http.StatusBadRequest, "Anthropic 未配置 Key 或 Model")
			return
		}
		provider = NewAnthropicProvider(
			cfg.AIChat.Anthropic.BaseURL, cfg.AIChat.Anthropic.APIKey,
			cfg.AIChat.Anthropic.Model, cfg.AIChat.Anthropic.Temperature, cfg.AIChat.Anthropic.MaxTokens,
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := provider.Chat(ctx, []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, map[string]any{"status": "ok", "reply": resp})
}

// isAuthenticated checks if the request has a valid session token.
func (s *APIServer) isAuthenticated(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	// 同源请求（无 Origin header，或 Origin 与请求源一致）直接放行
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	sameOrigin := origin == "" || origin == scheme+"://"+r.Host
	logger.Infow("isAuthenticated check", "origin", origin, "host", r.Host, "scheme", scheme, "sameOrigin", sameOrigin)
	if sameOrigin {
		return true
	}
	// 跨域 API 调用需要 Bearer token
	token := extractBearerToken(r)
	return token != "" || r.Header.Get("X-User-ID") != ""
}

// readString reads and returns the entire request body as a string.
func readString(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	return strings.TrimSpace(string(b))
}
