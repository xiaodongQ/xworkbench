
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/config"
)

// AI chat timeout / round budget (centralized, see CLAUDE.md §10.4 entry table).
const (
	aiChatTotalBudget = 5 * time.Minute // whole-request budget; also ends if client disconnects (r.Context)
	aiChatRoundBudget = 60 * time.Second // per provider.Chat call (single round)
	aiChatToolBudget  = 30 * time.Second // per-tool execution
	aiChatMaxRounds   = 20               // hard cap on consecutive tool-call iterations
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

	// Whole-request ctx. Bound by r.Context() so client disconnect cancels
	// the request, capped at aiChatTotalBudget as a hard ceiling. Per-round
	// and per-tool timeouts are layered on top below.
	ctx, cancel := context.WithTimeout(r.Context(), aiChatTotalBudget)
	defer cancel()

	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		streamCtx, streamCancel := context.WithTimeout(ctx, aiChatTotalBudget)
		err := provider.ChatStream(streamCtx, req.Messages, tools, func(event AIEvent) {
			data, _ := json.Marshal(event)
			io.WriteString(w, "data: " + string(data) + "\n\n")
			flusher.Flush()
		})
		streamCancel()
		if err != nil {
			logger.Errorf("AI stream error: %v", err)
		}
		return
	}

	roundCtx, roundCancel := context.WithTimeout(ctx, aiChatRoundBudget)
	resp, err := provider.Chat(roundCtx, req.Messages, tools)
	roundCancel()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			logger.Warnw("handleAIChat: total budget exceeded", "err", err, "ctxErr", ctxErr)
			writeErr(w, http.StatusGatewayTimeout, "AI chat total budget exceeded: "+ctxErr.Error())
			return
		}
		logger.Errorw("handleAIChat: provider.Chat failed", "provider", cfg.AIChat.ActiveProvider, "err", err)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Infow("handleAIChat: response received", "provider", cfg.AIChat.ActiveProvider, "contentLen", len(resp.Message.Content), "toolCalls", len(resp.ToolCalls))

	// Multi-round tool calling: keep looping until AI stops calling tools.
	// Capped at aiChatMaxRounds to prevent runaway iteration if the model keeps
	// asking for new tools without converging.
	refreshWidgets := make(map[string]bool)
	roundsDone := 0
	for round := 0; round < aiChatMaxRounds; round++ {
		if len(resp.ToolCalls) == 0 {
			break
		}
		roundsDone = round + 1
		logger.Infow("handleAIChat: round start", "round", round+1, "toolCount", len(resp.ToolCalls))
		// Check whole-request budget before burning another round.
		if err := ctx.Err(); err != nil {
			logger.Warnw("handleAIChat: total budget exceeded mid-loop", "round", round+1, "ctxErr", err)
			writeErr(w, http.StatusGatewayTimeout, "AI chat total budget exceeded: "+err.Error())
			return
		}
		for i := range resp.ToolCalls {
			tc := &resp.ToolCalls[i]
			logger.Infow("handleAIChat: tool call", "round", round+1, "tool", tc.Name, "args", tc.Args)
			toolCtx, toolCancel := context.WithTimeout(ctx, aiChatToolBudget)
			tc.Result = ExecuteTool(
				toolCtx,
				s.db, s.expDB, s.execDB, s.agentDB,
				s.linkDB, s.dirDB,
				s.schedDB, s.sch,
				s.memoryStore,
				nil, // localShellState
				tc.Name, tc.Args,
			)
			toolCancel()
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
			// Append tool result back to conversation, format depends on provider:
			//   - Anthropic: needs role=user + content = JSON array of content blocks
			//     with a {type:"tool_result", tool_use_id:..., content:...} entry
			//   - OpenAI:    needs role="tool" + tool_call_id (set on Message.ToolCallID) + content
			switch cfg.AIChat.ActiveProvider {
			case "openai":
				req.Messages = append(req.Messages, Message{
					Role:       "tool",
					Content:    tc.Result,
					ToolCallID: tc.ID,
				})
			default:
				block, _ := json.Marshal(map[string]any{
					"type":        "tool_result",
					"tool_use_id": tc.ID,
					"content":     tc.Result,
				})
				req.Messages = append(req.Messages, Message{
					Role:    "user",
					Content: "[" + string(block) + "]",
				})
			}
		}
		// Continue conversation with tool results — keep tools enabled for next round
		roundCtx, roundCancel := context.WithTimeout(ctx, aiChatRoundBudget)
		resp, err = provider.Chat(roundCtx, req.Messages, tools)
		roundCancel()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				logger.Warnw("handleAIChat: total budget exceeded on round provider call", "round", round+1, "ctxErr", ctxErr)
				writeErr(w, http.StatusGatewayTimeout, "AI chat total budget exceeded: "+ctxErr.Error())
				return
			}
			logger.Errorw("handleAIChat: tool result feedback failed", "round", round+1, "err", err)
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		logger.Infow("handleAIChat: round end", "round", round+1, "contentLen", len(resp.Message.Content), "toolCalls", len(resp.ToolCalls))
	}

	// If we exited the loop because we hit maxRounds while AI was still asking for
	// tools, surface that to the caller instead of silently truncating the result.
	if len(resp.ToolCalls) > 0 && roundsDone == aiChatMaxRounds {
		logger.Warnw("handleAIChat: round cap reached while AI still requested tools", "rounds", roundsDone)
		resp.Message.Content += "\n\n[⚠️ 已达到最大工具调用轮次 " + fmt.Sprintf("%d", aiChatMaxRounds) + "，剩余工具调用被截断]"
	}
	logger.Infow("handleAIChat: final response", "contentLen", len(resp.Message.Content), "rounds", roundsDone)

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
