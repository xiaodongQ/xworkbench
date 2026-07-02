//go:build windows

package main

import (
	"context"
	"encoding/json"
)

// AIProvider is the interface for AI chat providers.
type AIProvider interface {
	Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error)
	Name() string
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolCall represents a function call from the AI.
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"arguments"`
}

// Tool represents a function-calling tool definition.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ChatResponse is the response from an AI chat call.
type ChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// AIEvent is used for streaming responses.
type AIEvent struct {
	Type          string `json:"type"`
	Content       string `json:"content,omitempty"`
	ToolCallID   string `json:"tool_call_id,omitempty"`
	ToolCallName string `json:"tool_call_name,omitempty"`
	ToolCallArgs string `json:"tool_call_arguments,omitempty"`
}

// NewAIProviderFromConfig returns nil on Windows (AI Chat not supported).
func NewAIProviderFromConfig(cfg interface{}) AIProvider { return nil }

// OpenAIProvider stub — not available on Windows
type OpenAIProvider struct{}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	return nil, errWindowsAIChat
}
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []Message, tools []Tool, cb func(AIEvent)) error {
	return errWindowsAIChat
}
func (p *OpenAIProvider) Name() string { return "" }

// AnthropicProvider stub — not available on Windows
type AnthropicProvider struct{}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	return nil, errWindowsAIChat
}
func (p *AnthropicProvider) ChatStream(ctx context.Context, messages []Message, tools []Tool, cb func(AIEvent)) error {
	return errWindowsAIChat
}
func (p *AnthropicProvider) Name() string { return "" }

var errWindowsAIChat = &windowsAIChatError{}

type windowsAIChatError struct{}

func (e *windowsAIChatError) Error() string { return "AI Chat not supported on Windows" }
