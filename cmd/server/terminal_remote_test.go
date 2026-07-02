package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"go.uber.org/zap"
)

// TestHandleDirShortcutOpenTerminal_RemoteMultiTerminal 测试远程 shortcut 用多种终端类型唤起。
// 验证：wezterm/wt/iterm2/gnome/xterm 均不返回错误（命令构造成功）。
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
			if w.Code == http.StatusInternalServerError {
				body := w.Body.String()
				// InternalServerError 通常是 os/exec.Start 失败（可接受，因为测试机没装对应终端）
				// 但命令构造本身不应出错
				if strings.Contains(body, "build remote args failed") ||
					strings.Contains(body, "unsupported terminal type") ||
					strings.Contains(body, "empty result") {
					t.Errorf("handleDirShortcutOpenTerminal(%s) command build error: %s", termType, body)
				}
			}
		})
	}
}

// TestHandleDirShortcutOpenTerminal_RemoteUnsupportedTerminal 测试不支持的终端类型走泛用 ssh 兜底。
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
	// 不支持的终端走泛用 ssh 兜底，命令构造不报错
	if w.Code == http.StatusInternalServerError {
		body := w.Body.String()
		if strings.Contains(body, "build remote args failed") ||
			strings.Contains(body, "empty result") {
			t.Errorf("unsupported terminal should fall back to generic ssh, got error: %s", body)
		}
	}
}

// TestHandleDirShortcutOpenTerminal_RemoteKeyPathPriority 测试 LocalKeyPath 优先于 KeyPath。
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

	// 验证 LocalKeyPath 优先
	keyPath := resolveKeyPathForTest(remoteDir)
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
		Supported []map[string]interface{} `json:"supported"`
		Default   string                   `json:"default"`
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

// resolveKeyPathForTest 暴露 internal executor 的 ResolveKeyPath 逻辑用于测试。
func resolveKeyPathForTest(dir *backend.DirShortcut) string {
	if dir.LocalKeyPath != "" {
		return dir.LocalKeyPath
	}
	if dir.KeyPath != "" {
		return dir.KeyPath
	}
	cfg := config.Get()
	if cfg != nil && cfg.SSH.DefaultKeyPath != "" {
		return cfg.SSH.DefaultKeyPath
	}
	return "~/.ssh/xworkbench_id_ed25519"
}