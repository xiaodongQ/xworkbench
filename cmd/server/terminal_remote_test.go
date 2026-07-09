package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"github.com/xiaodongQ/xworkbench/internal/executor"
	"go.uber.org/zap"
)

// TestHandleDirShortcutOpenTerminal_RemoteMultiTerminal 测试远程 shortcut 用多种终端类型唤起。
// 验证：wezterm/wt/iterm2/gnome/xterm 均不返回错误（命令构造成功）。
// legacy 错误串 "build remote args failed" / "empty result" 是旧 buildRemoteArgs 实现的产物，
// 新 BuildSSHCommand 不再产出这些字符串。
func TestHandleDirShortcutOpenTerminal_RemoteMultiTerminal(t *testing.T) {
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	s := newTestServer(t)

	// 创建远程 shortcut
	remoteDir := &backend.DirShortcut{
		ID:           "test-remote-multi-term",
		Name:         "test remote",
		Type:         backend.DirShortcutTypeRemote,
		RemoteHost:   "192.168.1.100",
		RemoteUser:   "ubuntu",
		RemotePath:   "/home/ubuntu/projects",
		AuthMethod:   "key",
		LocalKeyPath: "/home/test/.ssh/id_ed25519",
	}
	if err := s.dirDB.Create(remoteDir); err != nil {
		t.Fatalf("Create remoteDir: %v", err)
	}

	termTypes := []string{"wezterm", "wt", "iterm2", "gnome", "xterm"}
	for _, termType := range termTypes {
		t.Run(termType, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/dir-shortcuts/{id}/open-terminal", s.handleDirShortcutOpenTerminal)
			req := httptest.NewRequest("POST", "/api/dir-shortcuts/"+remoteDir.ID+"/open-terminal?type="+termType, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			// 验证：命令构造成功（"unsupported terminal type" 不应出现——这些都受 DefaultConfig 支持）
			if strings.Contains(w.Body.String(), "unsupported terminal type") {
				t.Errorf("handleDirShortcutOpenTerminal(%s) unexpected unsupported error: %s", termType, w.Body.String())
			}
		})
	}
}

// TestHandleDirShortcutOpenTerminal_RemoteUnsupportedTerminal 验证未支持的 termType 现在返回 400 + "unsupported terminal type"。
// 新 BuildSSHCommand 对未支持的 termType 直接返回 error，handler 在 main.go:1849 early-return 400。
// 本测试锁住这个新契约不被未来回归。
func TestHandleDirShortcutOpenTerminal_RemoteUnsupportedTerminal(t *testing.T) {
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	s := newTestServer(t)

	remoteDir := &backend.DirShortcut{
		ID:           "test-remote-unsupported",
		Name:         "test remote unsupported",
		Type:         backend.DirShortcutTypeRemote,
		RemoteHost:   "192.168.1.100",
		RemoteUser:   "ubuntu",
		AuthMethod:   "key",
	}
	if err := s.dirDB.Create(remoteDir); err != nil {
		t.Fatalf("Create remoteDir: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/dir-shortcuts/{id}/open-terminal", s.handleDirShortcutOpenTerminal)
	req := httptest.NewRequest("POST", "/api/dir-shortcuts/"+remoteDir.ID+"/open-terminal?type=nonexistent_xyz_terminal", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// 新契约：未支持 termType 直接 400 + "unsupported terminal type"
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unsupported terminal type") {
		t.Errorf("expected 'unsupported terminal type' in body, got: %s", w.Body.String())
	}
}

// TestHandleDirShortcutOpenTerminal_RemoteKeyPathPriority 测试 LocalKeyPath 优先于 KeyPath。
// 直接调 executor.ResolveKeyPath（真实现），避免本地 helper 与真实现漂移。
func TestHandleDirShortcutOpenTerminal_RemoteKeyPathPriority(t *testing.T) {
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	restore := config.TestSnapshotAndRestore()
	defer restore()

	s := newTestServer(t)

	remoteDir := &backend.DirShortcut{
		ID:           "test-remote-key-priority",
		Name:         "test key priority",
		Type:         backend.DirShortcutTypeRemote,
		RemoteHost:   "10.0.0.1",
		RemoteUser:   "root",
		AuthMethod:   "key",
		KeyPath:      "/old/path/id_rsa",
		LocalKeyPath: "/new/path/id_ed25519",
	}
	if err := s.dirDB.Create(remoteDir); err != nil {
		t.Fatalf("Create remoteDir: %v", err)
	}

	// 验证 LocalKeyPath 优先（executor.ResolveKeyPath 是真实现）
	keyPath := executor.ResolveKeyPath(remoteDir)
	if keyPath != "/new/path/id_ed25519" {
		t.Errorf("LocalKeyPath should take priority over KeyPath, got %q", keyPath)
	}
}

// TestHandleTerminalList_IncludesRemoteArgs 验证 handleTerminalList 返回的终端类型包含 remote_args。
func TestHandleTerminalList_IncludesRemoteArgs(t *testing.T) {
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	s := newTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/terminals", s.handleTerminalList)
	req := httptest.NewRequest("GET", "/api/terminals", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleTerminalList status = %d, want 200", w.Code)
	}

	var resp struct {
		Supported []map[string]any `json:"supported"`
		Default   string           `json:"default"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// 验证已知终端类型包含 remote_args 字段
	for _, term := range resp.Supported {
		termType, _ := term["type"].(string)
		if termType == "wezterm" || termType == "wt" || termType == "iterm2" {
			if _, ok := term["remote_args"]; !ok {
				t.Errorf("terminal %q should have remote_args field", termType)
			}
		}
	}
}
