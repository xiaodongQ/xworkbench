//go:build !windows

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
	// Auth: require valid session token or API key
	if !s.isAuthenticated(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

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
		writeErr(w, http.StatusServiceUnavailable, "AI chat not configured")
		return
	}

	provider := NewAIProviderFromConfig(cfg)
	if provider == nil {
		writeErr(w, http.StatusServiceUnavailable, "AI provider not available")
		return
	}

	tools := GetTools()

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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Execute tool calls if present
	if len(resp.ToolCalls) > 0 {
		for i := range resp.ToolCalls {
			tc := &resp.ToolCalls[i]
			tc.Result = ExecuteTool(
				context.Background(),
				s.db, s.expDB, s.execDB, s.agentDB,
				nil, // localShellState - pass nil for now
				tc.Name, tc.Args,
			)
			// Append tool result as a special message and continue
			req.Messages = append(req.Messages, Message{
				Role:    "tool",
				Content: tc.Result,
			})
		}
		// Continue conversation with tool results
		resp, err = provider.Chat(context.Background(), req.Messages, nil)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, map[string]any{
		"id":      "chat_" + time.Now().Format("20060102150405"),
		"message": resp.Message,
	})
}

// handleAIConfigGet returns current AI config (api_key masked).
func (s *APIServer) handleAIConfigGet(w http.ResponseWriter, r *http.Request) {
	cfg := config.Snapshot()
	if cfg == nil {
		cfg = &config.Config{}
	}

	// Return a copy with masked API key
	resp := map[string]any{
		"ai_chat": map[string]any{
			"provider":     cfg.AIChat.Provider,
			"api_key":     cfg.AIChat.MaskedAPIKey(),
			"model":       cfg.AIChat.Model,
			"base_url":    cfg.AIChat.BaseURL,
			"temperature": cfg.AIChat.Temperature,
			"max_tokens": cfg.AIChat.MaxTokens,
			"enabled":     cfg.AIChat.IsEnabled(),
		},
	}
	writeJSON(w, resp)
}

// handleAIConfigUpdate updates AI config fields (except api_key).
func (s *APIServer) handleAIConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider     string  `json:"provider"`
		Model       string  `json:"model"`
		BaseURL     string  `json:"base_url"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
		SystemPrompt string  `json:"system_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	cfg := config.Snapshot()
	if cfg == nil {
		cfg = &config.Config{}
	}
	if req.Provider != "" {
		cfg.AIChat.Provider = req.Provider
	}
	if req.Model != "" {
		cfg.AIChat.Model = req.Model
	}
	if req.BaseURL != "" {
		cfg.AIChat.BaseURL = req.BaseURL
	}
	if req.Temperature > 0 {
		cfg.AIChat.Temperature = req.Temperature
	}
	if req.MaxTokens > 0 {
		cfg.AIChat.MaxTokens = req.MaxTokens
	}
	if req.SystemPrompt != "" {
		cfg.AIChat.SystemPrompt = req.SystemPrompt
	}

	config.Update(func(c *config.Config) {
		*c = *cfg
	})
	if err := config.Save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleAIConfigSetKey updates only the api_key field.
func (s *APIServer) handleAIConfigSetKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.APIKey == "" {
		writeErr(w, http.StatusBadRequest, "api_key required")
		return
	}
	config.Update(func(c *config.Config) {
		c.AIChat.APIKey = req.APIKey
	})
	if err := config.Save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleAIConfigTest tests the current API key by sending a minimal request.
func (s *APIServer) handleAIConfigTest(w http.ResponseWriter, r *http.Request) {
	cfg := config.Snapshot()
	if cfg == nil || !cfg.AIChat.IsEnabled() {
		writeErr(w, http.StatusBadRequest, "AI chat not configured")
		return
	}
	provider := NewAIProviderFromConfig(cfg)
	if provider == nil {
		writeErr(w, http.StatusBadRequest, "unknown provider: " + cfg.AIChat.Provider)
		return
	}
	// Send a trivial request
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := provider.Chat(ctx, []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// isAuthenticated checks if the request has a valid session token.
func (s *APIServer) isAuthenticated(r *http.Request) bool {
	// For now: check Bearer token in Authorization header
	// TODO: implement proper session token validation
	token := extractBearerToken(r)
	return token != "" || r.Header.Get("X-User-ID") != ""
}

// readString reads and returns the entire request body as a string.
func readString(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	return strings.TrimSpace(string(b))
}
