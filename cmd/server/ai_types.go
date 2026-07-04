
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/xiaodongQ/xworkbench/internal/config"
)

// --- Core types ---

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`              // system | user | assistant | tool
	Content string `json:"content,omitempty"` // text content
}

// ToolCall represents a tool call returned by the model.
type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Args  string `json:"arguments"` // JSON string
	Result string `json:"result,omitempty"` // filled after execution
}

// Tool represents a function-calling tool definition.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// ChatResponse is the response from a non-streaming chat call.
type ChatResponse struct {
	Message   Message    `json:"message"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// AIEvent is a streaming event from ChatStream.
type AIEvent struct {
	Type      string // "content" | "tool_call" | "done" | "error"
	Content   string
	ToolCall  *ToolCall
	Error     error
}

// --- Provider interface ---

// AIProvider is the interface for AI chat providers (OpenAI / Anthropic).
type AIProvider interface {
	// Chat sends a non-streaming chat request.
	Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error)
	// ChatStream sends a streaming chat request, calling cb for each event.
	ChatStream(ctx context.Context, messages []Message, tools []Tool, cb func(AIEvent)) error
}

// NewAIProviderFromConfig creates a provider from config.AIChat.
// Returns nil if AI Chat is not enabled (provider or api_key empty).
func NewAIProviderFromConfig(cfg *config.Config) AIProvider {
	if !cfg.AIChat.IsEnabled() {
		return nil
	}
	switch cfg.AIChat.Provider {
	case "openai":
		return NewOpenAIProvider(
			cfg.AIChat.BaseURL,
			cfg.AIChat.APIKey,
			cfg.AIChat.Model,
			cfg.AIChat.Temperature,
			cfg.AIChat.MaxTokens,
		)
	case "anthropic":
		return NewAnthropicProvider(
			cfg.AIChat.BaseURL,
			cfg.AIChat.APIKey,
			cfg.AIChat.Model,
			cfg.AIChat.Temperature,
			cfg.AIChat.MaxTokens,
		)
	default:
		return nil
	}
}

// --- OpenAI Provider ---

// OpenAIProvider implements AIProvider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	baseURL   string
	apiKey    string
	model     string
	temperature float64
	maxTokens int
}

func NewOpenAIProvider(baseURL, apiKey, model string, temperature float64, maxTokens int) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &OpenAIProvider{
		baseURL:    baseURL,
		apiKey:    apiKey,
		model:     model,
		temperature: temperature,
		maxTokens: maxTokens,
	}
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	reqBody := map[string]any{
		"model":       p.model,
		"messages":    messages,
		"temperature": p.temperature,
		"max_tokens":  p.maxTokens,
	}
	if len(tools) > 0 {
		toolsSchema := make([]map[string]any, len(tools))
		for i, t := range tools {
			toolsSchema[i] = map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  json.RawMessage(t.Parameters),
				},
			}
		}
		reqBody["tools"] = toolsSchema
	}

	var reqBodyBytes []byte
	reqBodyBytes, _ = json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Body = io.NopCloser(bytes.NewReader(reqBodyBytes))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Role           string                   `json:"role"`
				Content        string                   `json:"content"`
				ToolCalls      []map[string]any         `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in OpenAI response")
	}

	msg := apiResp.Choices[0].Message
	result := &ChatResponse{
		Message: Message{Role: msg.Role, Content: msg.Content},
	}
	for _, tc := range msg.ToolCalls {
		fn, _ := tc["function"].(map[string]any)
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:   tc["id"].(string),
			Name: fn["name"].(string),
			Args: fn["arguments"].(string),
		})
	}
	return result, nil
}

func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []Message, tools []Tool, cb func(AIEvent)) error {
	reqBody := map[string]any{
		"model":       p.model,
		"messages":    messages,
		"temperature": p.temperature,
		"max_tokens":  p.maxTokens,
		"stream":      true,
	}
	if len(tools) > 0 {
		toolsSchema := make([]map[string]any, len(tools))
		for i, t := range tools {
			toolsSchema[i] = map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  json.RawMessage(t.Parameters),
				},
			}
		}
		reqBody["tools"] = toolsSchema
	}

	var reqBodyBytes []byte
	reqBodyBytes, _ = json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", bytes.NewReader(reqBodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(body))
	}

	dec := json.NewDecoder(resp.Body)
	for dec.More() {
		var sse struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := dec.Decode(&sse); err != nil {
			break
		}
		for _, c := range sse.Choices {
			for _, tc := range c.Delta.ToolCalls {
				cb(AIEvent{
					Type:     "tool_call",
					ToolCall: &ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: tc.Function.Arguments},
				})
			}
			if c.Delta.Content != "" {
				cb(AIEvent{Type: "content", Content: c.Delta.Content})
			}
		}
	}
	cb(AIEvent{Type: "done"})
	return nil
}

// --- Anthropic Provider ---

// AnthropicProvider implements AIProvider for Anthropic APIs.
type AnthropicProvider struct {
	baseURL   string
	apiKey    string
	model     string
	temperature float64
	maxTokens int
}

func NewAnthropicProvider(baseURL, apiKey, model string, temperature float64, maxTokens int) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &AnthropicProvider{
		baseURL:    baseURL,
		apiKey:    apiKey,
		model:     model,
		temperature: temperature,
		maxTokens: maxTokens,
	}
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	anthropicMsgs := make([]map[string]any, len(messages))
	for i, m := range messages {
		// Tool result messages have Content as JSON array format for Anthropic
		if m.Role == "user" && strings.HasPrefix(m.Content, "[{") {
			var contentArr []any
			if err := json.Unmarshal([]byte(m.Content), &contentArr); err == nil {
				anthropicMsgs[i] = map[string]any{"role": m.Role, "content": contentArr}
				continue
			}
		}
		anthropicMsgs[i] = map[string]any{"role": m.Role, "content": m.Content}
	}

	reqBody := map[string]any{
		"model":       p.model,
		"messages":    anthropicMsgs,
		"temperature": p.temperature,
		"max_tokens":  p.maxTokens,
	}
	if len(tools) > 0 {
		toolsSchema := make([]map[string]any, len(tools))
		for i, t := range tools {
			toolsSchema[i] = map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"input_schema": json.RawMessage(t.Parameters),
			}
		}
		reqBody["tools"] = toolsSchema
	}

	var reqBodyBytes []byte
	reqBodyBytes, _ = json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(reqBodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Anthropic API error %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Content []struct {
			Type   string `json:"type"`
			Text   string `json:"text"`
			Name   string `json:"name"`
			Input_ any    `json:"input"` // can be string or object (tool call args)
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	result := &ChatResponse{}
	for _, c := range apiResp.Content {
		if c.Type == "text" {
			result.Message = Message{Role: "assistant", Content: c.Text}
		} else if c.Type == "tool_use" {
			var inputJSON []byte
			var argsStr string
			switch v := c.Input_.(type) {
			case string:
				argsStr = v
			case map[string]any:
				inputJSON, _ = json.Marshal(v)
				argsStr = string(inputJSON)
			}
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:   "call_anthropic",
				Name: c.Name,
				Args: argsStr,
			})
		}
	}
	return result, nil
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, messages []Message, tools []Tool, cb func(AIEvent)) error {
	// Anthropic streaming: use text/event-stream
	anthropicMsgs := make([]map[string]string, len(messages))
	for i, m := range messages {
		anthropicMsgs[i] = map[string]string{"role": m.Role, "content": m.Content}
	}

	reqBody := map[string]any{
		"model":       p.model,
		"messages":    anthropicMsgs,
		"temperature": p.temperature,
		"max_tokens":  p.maxTokens,
		"stream":      true,
	}
	if len(tools) > 0 {
		toolsSchema := make([]map[string]any, len(tools))
		for i, t := range tools {
			toolsSchema[i] = map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"input_schema": json.RawMessage(t.Parameters),
			}
		}
		reqBody["tools"] = toolsSchema
	}

	var reqBodyBytes []byte
	reqBodyBytes, _ = json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(reqBodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Anthropic API error %d: %s", resp.StatusCode, string(body))
	}

	dec := json.NewDecoder(resp.Body)
	for dec.More() {
		var event struct {
			Type   string `json:"type"`
			Index  int    `json:"index"`
			Delta  struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				Name       string `json:"name"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := dec.Decode(&event); err != nil {
			break
		}
		switch event.Type {
		case "content_block_delta":
			if event.Delta.Type == "text_delta" {
				cb(AIEvent{Type: "content", Content: event.Delta.Text})
			} else if event.Delta.Type == "input_json_delta" {
				cb(AIEvent{
					Type: "tool_call",
					ToolCall: &ToolCall{ID: fmt.Sprintf("call_%d", event.Index), Name: event.Delta.Name, Args: event.Delta.PartialJSON},
				})
			}
		}
	}
	cb(AIEvent{Type: "done"})
	return nil
}

// --- io helpers ---

