package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
)

// TestHandleExecutionContinue_CommandContainsPrompt 验证继续对话新 exec 的
// Command 字段必须包含用户新发的 prompt 摘要（不再是裸的 cmd slice）。
// 根因：原实现 runner.CmdString(cmd) 在 stdin 传 prompt 时命令里看不到 prompt；
//      改用 runner.CmdStringWithPrompt(cmd, prompt) 后命令形如：
//      claude -p ... --resume <uuid> "用户 prompt 摘要..."
// 这个测试是回归保护：用户进 exec 详情看到命令 textarea 期望看到自己发的内容。
func TestHandleExecutionContinue_CommandContainsPrompt(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = config.DefaultConfig()
	}
	s := newEvalTestServer(t)
	defer func() { config.AppConfig.AILoopEnabled = false }()
	config.AppConfig.AILoopEnabled = true
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/executions/{id}/continue", s.handleExecutionContinue)

	// 原 exec：必须有 resume_uuid 才能继续
	orig := &backend.Execution{
		ID:              "orig-exec-1",
		TaskID:          "task-1",
		Source:          "manual",
		Command:         "claude -p --resume xxx",
		Prompt:          "原始 prompt",
		Model:           "haiku",
		StartedAt:       time.Now().Add(-time.Hour),
		ResumeSessionID: "session-uuid-abc",
	}
	if err := s.execDB.Create(orig); err != nil {
		t.Fatalf("create orig exec: %v", err)
	}
	if err := s.execDB.Finish(orig.ID, "原 output", "", 0, "session-uuid-abc"); err != nil {
		t.Fatalf("finish orig exec: %v", err)
	}

	newPrompt := "请继续解释刚才的代码" // 用户在继续对话表单里输的新 prompt
	body := `{"prompt":"` + newPrompt + `","model":"sonnet"}`
	req := httptest.NewRequest("POST", "/api/executions/orig-exec-1/continue", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("continue status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	// 拉新的 continue exec（最新一条）检查 Command 字段
	list, err := s.execDB.ListByTask("task-1", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected >=2 execs, got %d", len(list))
	}
	// ListByTask 顺序：按 started_at desc（最新在前），新 continue exec 应该是第一条
	newExec := list[0]
	if newExec.Source != "continue" {
		t.Errorf("new exec source = %q, want continue", newExec.Source)
	}
	if !strings.Contains(newExec.Command, newPrompt) {
		t.Errorf("新 exec.Command 应该包含用户 prompt 摘要\nCommand: %s\nPrompt:  %s", newExec.Command, newPrompt)
	}
	if !strings.Contains(newExec.Command, "--resume") {
		t.Errorf("新 exec.Command 应包含 --resume 参数（继续对话关键）\nCommand: %s", newExec.Command)
	}
	if newExec.Prompt != newPrompt {
		t.Errorf("新 exec.Prompt = %q, want %q", newExec.Prompt, newPrompt)
	}
}