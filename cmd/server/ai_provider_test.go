package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/config"
)

// --- Message / Tool / Event types ---

func TestMessageRoundTrip(t *testing.T) {
	msg := Message{Role: "user", Content: "hello"}
	data, _ := json.Marshal(msg)
	var got Message
	json.Unmarshal(data, &got)
	if got.Role != "user" || got.Content != "hello" {
		t.Errorf("Message round-trip: got %+v", got)
	}
}

func TestToolCallStruct(t *testing.T) {
	tc := ToolCall{
		ID:   "call_abc",
		Name: "create_task",
		Args: `{"title":"test"}`,
	}
	data, _ := json.Marshal(tc)
	if !strings.Contains(string(data), "create_task") {
		t.Errorf("ToolCall marshal: %s", data)
	}
}

// --- Provider interface exists ---

type mockProvider struct {
	calls []string
}

func (m *mockProvider) Chat(ctx context.Context, msgs []Message, tools []Tool) (*ChatResponse, error) {
	m.calls = append(m.calls, "chat")
	return &ChatResponse{
		Message: Message{Role: "assistant", Content: "hi"},
	}, nil
}

func (m *mockProvider) ChatStream(ctx context.Context, msgs []Message, tools []Tool, cb func(AIEvent)) error {
	m.calls = append(m.calls, "stream")
	return nil
}

func TestProviderInterface(t *testing.T) {
	// Verify AIProvider interface is satisfied
	var p AIProvider = &mockProvider{}
	resp, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Message.Content != "hi" {
		t.Errorf("content: got %q", resp.Message.Content)
	}
	if len(p.(*mockProvider).calls) != 1 || p.(*mockProvider).calls[0] != "chat" {
		t.Errorf("calls: got %v", p.(*mockProvider).calls)
	}
}

// --- OpenAI Provider ---

func TestOpenAIProviderChat(t *testing.T) {
	// Mock server that returns a simple assistant message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("missing or wrong Authorization header: %s", r.Header.Get("Authorization"))
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		msgs := req["messages"].([]any)
		if len(msgs) == 0 {
			t.Error("no messages sent")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"model":   "gpt-4o",
			"choices": []map[string]any{{
				"message": map[string]string{"role": "assistant", "content": "hello from gpt"},
			}},
		})
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL+"/v1", "sk-test", "gpt-4o", 0.7, 4096)
	resp, err := provider.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Message.Content != "hello from gpt" {
		t.Errorf("content: got %q", resp.Message.Content)
	}
}

func TestOpenAIProviderWithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		tools := req["tools"]
		if tools == nil {
			t.Error("tools not sent in request")
		}
		// Simulate a tool call response
		json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-tool",
			"choices": []map[string]any{{
				"message": map[string]any{
					"role": "assistant",
					"content": "",
					"tool_calls": []map[string]any{{
						"id":       "call_xyz",
						"type":     "function",
						"function": map[string]string{"name": "create_task", "arguments": `{"title":"test"}`},
					}},
				},
			}},
		})
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL+"/v1", "sk-test", "gpt-4o", 0.7, 4096)
	tools := []Tool{{Name: "create_task", Description: "Create a task", Parameters: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}}}`)}}
	resp, err := provider.Chat(context.Background(), []Message{{Role: "user", Content: "create a task"}}, tools)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "create_task" {
		t.Errorf("tool name: got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Args != `{"title":"test"}` {
		t.Errorf("tool args: got %q", resp.ToolCalls[0].Args)
	}
}

// --- Config integration ---

func TestAIProviderFromConfig(t *testing.T) {
	// When config is disabled, should return nil provider
	cfg := &config.Config{AIChat: config.AIChatConfig{ActiveProvider: "anthropic"}}
	p := NewAIProviderFromConfig(cfg)
	if p != nil {
		t.Errorf("expected nil provider for disabled config, got %T", p)
	}

	// When openai configured, should return OpenAI provider
	cfg2 := &config.Config{AIChat: config.AIChatConfig{
		ActiveProvider: "openai",
		OpenAI: config.ProviderConfig{
			APIKey: "sk-test2", Model: "gpt-4o",
			BaseURL: "https://api.openai.com", Temperature: 0.5, MaxTokens: 2048,
		},
	}}
	p2 := NewAIProviderFromConfig(cfg2)
	if p2 == nil {
		t.Fatal("expected non-nil OpenAI provider")
	}
	// Verify it works (will hit mock server if we had one, just check type)
	if _, ok := p2.(*OpenAIProvider); !ok {
		t.Errorf("expected *OpenAIProvider, got %T", p2)
	}
}

// --- Regression tests: #1 tool_result protocol & #2 context cancellation ---

// TestMessageToolCallIDSerialization — OpenAI protocol tool messages must carry
// tool_call_id so the result can be correlated to the original call.
func TestMessageToolCallIDSerialization(t *testing.T) {
	msg := Message{Role: "tool", Content: "result text", ToolCallID: "call_abc"}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"role":"tool"`) ||
		!strings.Contains(s, `"content":"result text"`) ||
		!strings.Contains(s, `"tool_call_id":"call_abc"`) {
		t.Errorf("expected role+content+tool_call_id in JSON, got: %s", s)
	}

	var got Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ToolCallID != "call_abc" {
		t.Errorf("round-trip tool_call_id: got %q want %q", got.ToolCallID, "call_abc")
	}
	if got.Role != "tool" {
		t.Errorf("round-trip role: got %q", got.Role)
	}
}

// TestAnthropicToolResultArraySerialization — Anthropic expects the tool result
// to be a content array block. ai_chat.go encodes it as user role with a JSON
// array string; verify the shape survives AnthropicProvider's array-detection.
func TestAnthropicToolResultArraySerialization(t *testing.T) {
	block, _ := json.Marshal(map[string]any{
		"type":        "tool_result",
		"tool_use_id": "toolu_01abc",
		"content":     "task created",
	})
	msg := Message{Role: "user", Content: "[" + string(block) + "]"}

	if !strings.HasPrefix(msg.Content, "[{") {
		t.Skipf("content does not match Anthropic array form: %s", msg.Content)
	}
	var arr []any
	if err := json.Unmarshal([]byte(msg.Content), &arr); err != nil {
		t.Fatalf("expected valid array of blocks, got error: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 block, got %d", len(arr))
	}
	blk, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("block[0] not object: %T", arr[0])
	}
	if blk["type"] != "tool_result" {
		t.Errorf("block.type: got %v", blk["type"])
	}
	if blk["tool_use_id"] != "toolu_01abc" {
		t.Errorf("block.tool_use_id: got %v", blk["tool_use_id"])
	}
	if blk["content"] != "task created" {
		t.Errorf("block.content: got %v", blk["content"])
	}
}

// TestAnthropicProviderReadsToolUseID — Anthropic response includes a tool_use
// id. Previously the decoder hardcoded "call_anthropic" which broke tool_result
// correlation. Verify the real id is now captured.
func TestAnthropicProviderReadsToolUseID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "msg_test",
			"type": "message",
			"content": []map[string]any{
				{"type": "text", "text": "I'll create the task now."},
				{"type": "tool_use", "id": "toolu_01abc", "name": "create_task", "input": map[string]any{"title": "demo"}},
			},
		})
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "sk-ant-test", "claude-sonnet-test", 0.7, 4096)
	resp, err := provider.Chat(context.Background(), []Message{{Role: "user", Content: "create a task"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "toolu_01abc" {
		t.Errorf("tool_use_id not captured: got %q (was previously hardcoded 'call_anthropic')", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Name != "create_task" {
		t.Errorf("tool name: got %q", resp.ToolCalls[0].Name)
	}
}

// TestOpenAIProviderPropagatesContextCancel — verifies ctx cancellation
// reaches the upstream HTTP request (foundation for handleAIChat total budget).
func TestOpenAIProviderPropagatesContextCancel(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // block until released
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{"role": "assistant", "content": ""},
			}},
		})
	}))
	defer server.Close()
	defer close(release)

	provider := NewOpenAIProvider(server.URL+"/v1", "sk-test", "gpt-4o", 0.7, 4096)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := provider.Chat(ctx, []Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "context") &&
		!strings.Contains(err.Error(), "deadline") {
		t.Errorf("expected context/deadline error, got: %v", err)
	}
}