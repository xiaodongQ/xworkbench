//go:build windows

package main

import (
	"log"
	"net/http"
)

// handlePty 在 Windows 上不可用（creack/pty 不支持 Windows ConPTY）。
// 返回 503 + 友好提示，UI 探测 navigator.userAgentData 隐藏 "AI Chat" Tab。
//
// 必须是 (s *APIServer) 方法，与 Unix 上的 pty.go 签名一致，
// 这样 main.go 的路由注册 s.handlePty 在两个平台都能编译。
func (s *APIServer) handlePty(w http.ResponseWriter, r *http.Request) {
	log.Printf("PTY requested on Windows, returning 503")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte(`{"error":"PTY not supported on Windows. Use the web UI for task execution instead."}`))
}
