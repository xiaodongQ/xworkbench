package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandlePtyInput_NotFound 测试不存在的 tab_id → 404
func TestHandlePtyInput_NotFound(t *testing.T) {
	s := &APIServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/pty/{tab_id}/submit-input", s.handlePtyInput)

	body := `{"input":"y"}`
	req := httptest.NewRequest("POST", "/api/pty/tab-nonexistent/submit-input", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("handlePtyInput notfound = %d, want 404, body=%s", w.Code, w.Body.String())
	}
}

// TestHandlePtyInput_InvalidJSON 测试无效 JSON → 400
func TestHandlePtyInput_InvalidJSON(t *testing.T) {
	s := &APIServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/pty/{tab_id}/submit-input", s.handlePtyInput)

	body := `not json`
	req := httptest.NewRequest("POST", "/api/pty/tab-1/submit-input", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("handlePtyInput bad json = %d, want 400", w.Code)
	}
}

// TestHandlePtyInput_NoBody 测试空 body → 400
func TestHandlePtyInput_NoBody(t *testing.T) {
	s := &APIServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/pty/{tab_id}/submit-input", s.handlePtyInput)

	req := httptest.NewRequest("POST", "/api/pty/tab-1/submit-input", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("handlePtyInput no body = %d, want 400", w.Code)
	}
}

// TestFindPTY_EmptyRegistry 测试空注册表 → nil
func TestFindPTY_EmptyRegistry(t *testing.T) {
	ptyMu.Lock()
	ptySessions = make(map[string]*PTYSession)
	ptyMu.Unlock()

	if got := FindPTY("any-tab"); got != nil {
		t.Errorf("FindPTY on empty registry = %v, want nil", got)
	}
}

// TestDetectAuthRequired_SubtlePatterns 边界模式检测
func TestDetectAuthRequired_SubtlePatterns(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"Do you approve this action? [Y/n]", true},
		{"是否确认继续？（按 Y 确认）", true},
		{"Continue anyway? (y/N)", true},
		{"Permission denied", false}, // anti-pattern
		{"Error: permission denied reading file", false},
		{"  Approve  ", true},
		{"No confirmation needed", false},
		{"", false},
	}
	for _, c := range cases {
		got := detectAuthRequired(c.line)
		if got != c.want {
			t.Errorf("detectAuthRequired(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

// TestDetectAuthRequired_Confirm 检测 CONFIRM 大写触发（大小写敏感）
func TestDetectAuthRequired_Confirm(t *testing.T) {
	if detectAuthRequired("please CONFIRM") != true {
		t.Error("CONFIRM uppercase not detected")
	}
}