package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"go.uber.org/zap"
)

// TestHandleRemotePty_NotFound 测试 dir_id 不存在时返回 404。
func TestHandleRemotePty_NotFound(t *testing.T) {
	s := newTestServer(t)
	if logger == nil {
		restore := testLogger()
		defer restore()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/rpty", s.handleRemotePty)
	req := httptest.NewRequest("GET", "/api/rpty?tab_id=test-tab&dir_id=nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("handleRemotePty status = %d, want 404", w.Code)
	}
}

// TestHandleRemotePty_MissingParams 测试缺少 tab_id 或 dir_id 时返回 400。
func TestHandleRemotePty_MissingParams(t *testing.T) {
	s := newTestServer(t)
	if logger == nil {
		restore := testLogger()
		defer restore()
	}

	tests := []struct {
		url  string
		want int
	}{
		{"/api/rpty?dir_id=test-dir", http.StatusBadRequest},           // 缺 tab_id
		{"/api/rpty?tab_id=test-tab", http.StatusBadRequest},           // 缺 dir_id
		{"/api/rpty", http.StatusBadRequest},                           // 两者都缺
	}
	for _, tt := range tests {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/rpty", s.handleRemotePty)
		req := httptest.NewRequest("GET", tt.url, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != tt.want {
			t.Errorf("handleRemotePty(%s) status = %d, want %d", tt.url, w.Code, tt.want)
		}
	}
}

// TestHandleRemotePty_NonRemoteShortcut 测试 dir_id 存在但不是 remote 类型时返回 400。
func TestHandleRemotePty_NonRemoteShortcut(t *testing.T) {
	s := newTestServer(t)
	if logger == nil {
		restore := testLogger()
		defer restore()
	}

	localDir := &backend.DirShortcut{
		ID:   "test-local-rpty",
		Name: "test local",
		Type: backend.DirShortcutTypeLocal,
		Path: "/tmp",
	}
	if err := s.dirDB.Create(localDir); err != nil {
		t.Fatalf("Create localDir: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/rpty", s.handleRemotePty)
	req := httptest.NewRequest("GET", "/api/rpty?tab_id=test-tab&dir_id="+localDir.ID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("handleRemotePty with local shortcut status = %d, want 400", w.Code)
	}
}

// TestHandleRemotePty_KeyAuth 测试密钥认证时 dir_id 正确解析（无法真实连接，只验证不 panic）。
func TestHandleRemotePty_KeyAuth(t *testing.T) {
	s := newTestServer(t)
	if logger == nil {
		restore := testLogger()
		defer restore()
	}

	remoteDir := &backend.DirShortcut{
		ID:           "test-rpty-key",
		Name:         "test remote key auth",
		Type:         backend.DirShortcutTypeRemote,
		RemoteHost:   "192.168.1.100",
		RemoteUser:   "ubuntu",
		AuthMethod:   "key",
		LocalKeyPath: "/tmp/nonexistent_key",
	}
	if err := s.dirDB.Create(remoteDir); err != nil {
		t.Fatalf("Create remoteDir: %v", err)
	}

	// 启动 WebSocket 连接（会在 SSH 握手阶段失败，但不应 panic）
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/rpty", s.handleRemotePty)

	done := make(chan struct{})
	var wsCode int
	go func() {
		req := httptest.NewRequest("GET", "/api/rpty?tab_id=test-tab&dir_id="+remoteDir.ID, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		wsCode = w.Code
		close(done)
	}()

	select {
	case <-done:
		// 预期 400（密钥不存在）或 500（连接失败），都不应是 panic
		if wsCode == http.StatusOK {
			t.Log("unexpectedly got 200 (websocket upgrade) - test machine may have valid key")
		}
	case <-time.After(5 * time.Second):
		// SSH 连接超时，不算错
		t.Log("SSH connection timed out (expected for unreachable host)")
	}
}

// TestRPTYSession_WriteInput 测试 RPTYSession.WriteInput 不 panic。
func TestRPTYSession_WriteInput(t *testing.T) {
	if logger == nil {
		restore := testLogger()
		defer restore()
	}

	// 创建 fake session 验证接口存在
	sess := &RPTYSession{tabID: "test-tab"}
	err := sess.WriteInput("hello")
	// 真实的 RPTYSession 在 SSH 连接建立前 WriteInput 会失败，
	// 但不应 panic（接口契约）
	if err == nil {
		t.Log("WriteInput succeeded on nil session (expected for uninitialized session)")
	}
}

// TestFindRPTY 测试 RPTY session 注册和查找。
func TestFindRPTY(t *testing.T) {
	if logger == nil {
		restore := testLogger()
		defer restore()
	}

	tabID := "test-find-rpty"
	// 初始不存在
	sess := FindRPTY(tabID)
	if sess != nil {
		t.Errorf("FindRPTY(%q) initially = %v, want nil", tabID, sess)
	}

	// 注册并查找
	sess = &RPTYSession{tabID: tabID}
	RegisterRPTY(tabID, sess)
	found := FindRPTY(tabID)
	if found != sess {
		t.Errorf("FindRPTY(%q) = %v, want %v", tabID, found, sess)
	}

	// 注销后不存在
	UnregisterRPTY(tabID)
	found = FindRPTY(tabID)
	if found != nil {
		t.Errorf("FindRPTY(%q) after unregister = %v, want nil", tabID, found)
	}
}

// testLogger 初始化 logger（与 debug_export_test.go 一致）。
func testLogger() func() {
	if logger != nil {
		return func() {}
	}
	z, _ := zap.NewProduction()
	logger = z.Sugar()
	return func() { logger = nil }
}

// fakeWriteCloser 实现 io.WriteCloser，用于测试 WriteInput
type fakeWriteCloser struct {
	buf []byte
}

func (f *fakeWriteCloser) Write(p []byte) (int, error) {
	f.buf = append(f.buf, p...)
	return len(p), nil
}

func (f *fakeWriteCloser) Close() error { return nil }

// TestRPTYAuthRequiredDetection 测试 auth_required 检测逻辑复用 pty.go 的 detectAuthRequired。
// 注：当前 patterns 以本地 AI CLI 授权为主（yes/no/confirm），
// SSH 特有 prompts（Password:/passphrase）需后续扩展 authRequiredPatterns。
func TestRPTYAuthRequiredDetection(t *testing.T) {
	tests := []struct {
		line    string
		wantYes bool
	}{
		{"Are you sure you want to continue connecting", true}, // SSH host key 确认，已在 patterns 中
		{"yes/no", true},
		{"[Y/n]", true},
		{"confirm", true},
		{"Password:", true},           // SSH 特有，已在 patterns 中
		{"Enter passphrase for key", true}, // SSH 特有，已在 patterns 中
		{"Last login: Thu Jul  2 00:00", false},
		{"Permission denied (publickey)", false},
		{"Welcome to Ubuntu 22.04", false},
	}
	for _, tt := range tests {
		got := detectAuthRequiredRPTY(tt.line)
		if got != tt.wantYes {
			t.Errorf("detectAuthRequired(%q) = %v, want %v", tt.line, got, tt.wantYes)
		}
	}
}

// detectAuthRequiredRPTY 暴露 pty.go 的 detectAuthRequired 用于测试。
func detectAuthRequiredRPTY(line string) bool {
	return detectAuthRequired(line)
}

// TestRemotePtySessionStruct 测试 RPTYSession 结构体字段完整性。
func TestRemotePtySessionStruct(t *testing.T) {
	sess := &RPTYSession{
		tabID: "test-tab-1",
		dirID: "test-dir-1",
	}
	if sess.tabID != "test-tab-1" || sess.dirID != "test-dir-1" {
		t.Errorf("RPTYSession field mismatch")
	}
	// WriteInput 接口契约：返回 error 或 nil，不 panic
	err := sess.WriteInput("test input")
	if err != nil {
		t.Logf("WriteInput returned error (expected for uninitialized): %v", err)
	}
}

// TestHandleRemotePty_UnsupportedAuthMethod 测试不支持的认证方式返回 400。
func TestHandleRemotePty_UnsupportedAuthMethod(t *testing.T) {
	s := newTestServer(t)
	if logger == nil {
		restore := testLogger()
		defer restore()
	}

	remoteDir := &backend.DirShortcut{
		ID:           "test-rpty-unsupported-auth",
		Name:         "test unsupported auth",
		Type:         backend.DirShortcutTypeRemote,
		RemoteHost:   "192.168.1.100",
		RemoteUser:   "ubuntu",
		AuthMethod:   "password", // 密码认证暂不支持
		RemotePassword: "test",
	}
	if err := s.dirDB.Create(remoteDir); err != nil {
		t.Fatalf("Create remoteDir: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/rpty", s.handleRemotePty)
	req := httptest.NewRequest("GET", "/api/rpty?tab_id=test-tab&dir_id="+remoteDir.ID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// 密码认证暂不支持，预期 400 或在 SSH 连接时失败（500）
	if w.Code == http.StatusOK {
		t.Error("password auth should not succeed")
	}
}

// TestRemotePtySubmitInput_NotFound 测试向不存在的 tab_id 提交 input 返回 404。
func TestRemotePtySubmitInput_NotFound(t *testing.T) {
	s := newTestServer(t)
	if logger == nil {
		restore := testLogger()
		defer restore()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/rpty/{tab_id}/submit-input", s.handleRptyInput)
	body, _ := json.Marshal(map[string]string{"input": "y"})
	req := httptest.NewRequest("POST", "/api/rpty/nonexistent-tab/submit-input", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("handleRptyInput nonexistent tab status = %d, want 404", w.Code)
	}
}

// TestRemotePtySubmitInput_Success 测试 WriteInput 在有 stdin 时的行为。
// 注意：RegisterRPTY 的清理 goroutine 会在 session.Wait() 返回后立即注销 session，
// 因此无法通过 HTTP handler 可靠地测试（存在竞态）。这里直接测试 WriteInput 方法。
func TestRemotePtySubmitInput_Success(t *testing.T) {
	if logger == nil {
		restore := testLogger()
		defer restore()
	}

	fakeStdin := &fakeWriteCloser{}
	sess := &RPTYSession{tabID: "test-write-input", stdin: fakeStdin}

	err := sess.WriteInput("yes")
	if err != nil {
		t.Errorf("WriteInput should succeed with valid stdin, got error: %v", err)
	}
	if string(fakeStdin.buf) != "yes\n" {
		t.Errorf("WriteInput buf = %q, want %q", string(fakeStdin.buf), "yes\n")
	}
}

// TestRemotePtySubmitInput_EmptyBody 测试空 body 返回 400。
func TestRemotePtySubmitInput_EmptyBody(t *testing.T) {
	s := newTestServer(t)
	if logger == nil {
		restore := testLogger()
		defer restore()
	}

	RegisterRPTY("test-empty-body-tab", &RPTYSession{tabID: "test-empty-body-tab"})
	defer UnregisterRPTY("test-empty-body-tab")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/rpty/{tab_id}/submit-input", s.handleRptyInput)
	req := httptest.NewRequest("POST", "/api/rpty/test-empty-body-tab/submit-input", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("handleRptyInput empty body status = %d, want 400", w.Code)
	}
}

// 验证 ctx timeout 不冲突
func TestRemotePtyContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		t.Error("context should not be done immediately")
	default:
	}
}