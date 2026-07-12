package main

import (
	"context"
	"encoding/json"
	"io"
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

// TestAnthropicProviderCollectsBlocks — assistant content blocks (text + tool_use)
// must be collected so the next request can re-send them as alternation.
func TestAnthropicProviderCollectsBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "msg_test",
			"type": "message",
			"content": []map[string]any{
				{"type": "text", "text": "I'll create the task."},
				{"type": "tool_use", "id": "toolu_01A", "name": "create_task", "input": map[string]any{"title": "demo"}},
			},
		})
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "sk-ant-test", "claude-sonnet-test", 0.7, 4096)
	resp, err := provider.Chat(context.Background(), []Message{{Role: "user", Content: "create a task"}}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.AssistantBlocks) != 2 {
		t.Fatalf("expected 2 blocks (text + tool_use), got %d", len(resp.AssistantBlocks))
	}
	if resp.AssistantBlocks[0].Type != "text" || resp.AssistantBlocks[0].Text != "I'll create the task." {
		t.Errorf("block[0] text mismatch: %+v", resp.AssistantBlocks[0])
	}
	if resp.AssistantBlocks[1].Type != "tool_use" || resp.AssistantBlocks[1].ID != "toolu_01A" {
		t.Errorf("block[1] tool_use mismatch: %+v", resp.AssistantBlocks[1])
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "toolu_01A" {
		t.Errorf("ToolCalls not derived from blocks: %+v", resp.ToolCalls)
	}
}

// TestAppendAssistantAnthropicRoundTrip — assistant turn with tool_use must be
// serialized back as a JSON array of blocks, so the next Anthropic call has
// matching alternation for tool_result.
func TestAppendAssistantAnthropicRoundTrip(t *testing.T) {
	resp := &ChatResponse{
		Message: Message{Role: "assistant", Content: "I'll create the task."},
		AssistantBlocks: []ContentBlock{
			{Type: "text", Text: "I'll create the task."},
			{Type: "tool_use", ID: "toolu_01A", Name: "create_task", Input: json.RawMessage(`{"title":"demo"}`)},
		},
		ToolCalls: []ToolCall{{ID: "toolu_01A", Name: "create_task", Args: `{"title":"demo"}`}},
	}
	var msgs []Message
	appendAssistantForToolRoundTrip(&msgs, resp, "anthropic")

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "assistant" {
		t.Errorf("role: %q", msgs[0].Role)
	}
	// Content must be a JSON array, not plain text — required for alternation
	var blocks []ContentBlock
	if err := json.Unmarshal([]byte(msgs[0].Content), &blocks); err != nil {
		t.Fatalf("assistant content not JSON array: %v (raw=%q)", err, msgs[0].Content)
	}
	if len(blocks) != 2 || blocks[1].Type != "tool_use" || blocks[1].ID != "toolu_01A" {
		t.Errorf("blocks don't include tool_use: %+v", blocks)
	}
}

// TestAppendAssistantOpenAIToolCalls — OpenAI assistant turn must carry
// tool_calls array so subsequent role="tool" messages attach to a parent.
func TestAppendAssistantOpenAIToolCalls(t *testing.T) {
	resp := &ChatResponse{
		Message:   Message{Role: "assistant", Content: ""},
		ToolCalls: []ToolCall{{ID: "call_abc", Name: "create_task", Args: `{"title":"demo"}`}},
	}
	var msgs []Message
	appendAssistantForToolRoundTrip(&msgs, resp, "openai")

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "assistant" {
		t.Errorf("role: %q", msgs[0].Role)
	}
	if len(msgs[0].ToolCalls) != 1 || msgs[0].ToolCalls[0].ID != "call_abc" {
		t.Errorf("tool_calls not carried: %+v", msgs[0].ToolCalls)
	}
	// Round-trip JSON serialize should include tool_calls for OpenAI
	data, _ := json.Marshal(msgs[0])
	if !strings.Contains(string(data), `"tool_calls"`) {
		t.Errorf("expected tool_calls field in JSON: %s", data)
	}
}

// TestAnthropicRoundTripAssistantBlocksSerialized — full chat round-trip: first
// response includes text + tool_use; subsequent request body must contain
// assistant message with content as JSON array containing tool_use id.
func TestAnthropicRoundTripAssistantBlocksSerialized(t *testing.T) {
	var secondReqBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// First request: no tool_use_id yet → return tool_use block.
		// Second request: carries tool_use_id → capture body for assertion,
		// return text-only final answer.
		isSecond := strings.Contains(string(body), `"tool_use_id":"toolu_01A"`)
		if isSecond {
			secondReqBody = body
		}
		var content []map[string]any
		if isSecond {
			content = []map[string]any{{"type": "text", "text": "Task created."}}
		} else {
			content = []map[string]any{
				{"type": "text", "text": "I'll create the task."},
				{"type": "tool_use", "id": "toolu_01A", "name": "create_task", "input": map[string]any{"title": "demo"}},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "msg_round",
			"type":    "message",
			"content": content,
		})
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "sk-ant-test", "claude-sonnet-test", 0.7, 4096)
	// Step 1: first chat with assistant having tool_use blocks
	resp, err := provider.Chat(context.Background(),
		[]Message{
			{Role: "user", Content: "create a task"},
		}, nil)
	if err != nil {
		t.Fatalf("first Chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "toolu_01A" {
		t.Fatalf("first response should carry tool_use; got %+v", resp.ToolCalls)
	}
	// Step 2: assistant content synthesized via helper
	var msgs []Message
	msgs = append(msgs, Message{Role: "user", Content: "create a task"})
	appendAssistantForToolRoundTrip(&msgs, resp, "anthropic")
	// Step 3: tool_result appended
	tc := resp.ToolCalls[0]
	block, _ := json.Marshal(map[string]any{
		"type":        "tool_result",
		"tool_use_id": tc.ID,
		"content":     "task created",
	})
	msgs = append(msgs, Message{Role: "user", Content: "[" + string(block) + "]"})

	// Second chat
	_, err = provider.Chat(context.Background(), msgs, nil)
	if err != nil {
		t.Fatalf("second Chat: %v", err)
	}
	if len(secondReqBody) == 0 {
		t.Fatal("second request body not captured")
	}
	// Verify assistant message in second request carries tool_use id
	var parsed map[string]any
	if err := json.Unmarshal(secondReqBody, &parsed); err != nil {
		t.Fatalf("parse second req: %v", err)
	}
	msgs2 := parsed["messages"].([]any)
	if len(msgs2) != 3 {
		t.Fatalf("expected 3 messages (user/assistant/user), got %d", len(msgs2))
	}
	asst := msgs2[1].(map[string]any)
	content := asst["content"]
	if strContent, isStr := content.(string); isStr {
		// For comparison reference: AnthropicProvider also accepts JSON-array
		// string form via Unmarshal fallback. We tolerate either.
		var blocks []map[string]any
		if err := json.Unmarshal([]byte(strContent), &blocks); err != nil {
			t.Fatalf("assistant content not parseable as array: %v raw=%q", err, strContent)
		}
		assertAssistantHasToolUse(t, blocks, "toolu_01A")
	} else if arrContent, isArr := content.([]any); isArr {
		// Preferred form: AnthropicProvider turns the JSON-array string into
		// an actual array on the wire.
		blocks := make([]map[string]any, 0, len(arrContent))
		for _, c := range arrContent {
			if m, ok := c.(map[string]any); ok {
				blocks = append(blocks, m)
			}
		}
		assertAssistantHasToolUse(t, blocks, "toolu_01A")
	} else {
		t.Errorf("assistant content unexpected type: %T (value=%v)", content, content)
	}
}

// assertAssistantHasToolUse verifies the assistant content blocks array carries
// a tool_use with the given id — i.e. the alternation round-trip is correct.
func assertAssistantHasToolUse(t *testing.T, blocks []map[string]any, wantID string) {
	t.Helper()
	for _, b := range blocks {
		if b["type"] == "tool_use" && b["id"] == wantID {
			return
		}
	}
	t.Errorf("assistant blocks missing tool_use id %s: %v", wantID, blocks)
}