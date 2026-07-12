//go:build !windows

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xiaodongQ/xworkbench/internal/config"
)

// SessionInfo 单个终端会话的元数据。
type SessionInfo struct {
	ID           string    `json:"id"`            // "local_shell", "remote_xxx"
	Type         string    `json:"type"`          // "local_shell", "local_claude", "remote"
	DirID        string    `json:"dir_id,omitempty"` // 仅 remote 有值
	Label        string    `json:"label"`         // 列表显示名
	Status       string    `json:"status"`        // "connected", "disconnected", "connecting", "error"
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`

	// 内部字段，不序列化到 JSON
	wsConn *websocket.Conn
	ptmx   *os.File
	cmd    *exec.Cmd
}

// sessionManager 管理所有终端会话。
type sessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*SessionInfo
	stopCh   chan struct{}
}

var (
	terminalSessions = &sessionManager{
		sessions: make(map[string]*SessionInfo),
		stopCh:   make(chan struct{}),
	}
)

// StartIdleChecker 启动空闲超时检查 goroutine。
func (sm *sessionManager) StartIdleChecker() {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sm.checkIdleSessions()
			case <-sm.stopCh:
				return
			}
		}
	}()
}

// Stop 停止空闲检查并关闭所有会话。
func (sm *sessionManager) Stop() {
	close(sm.stopCh)
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for id, s := range sm.sessions {
		sm.disconnectLocked(s)
		delete(sm.sessions, id)
	}
}

func (sm *sessionManager) checkIdleSessions() {
	cfg := config.Get()
	timeoutMinutes := 30 // 默认 30 分钟
	if cfg != nil && cfg.TerminalSessionTimeoutMinutes > 0 {
		timeoutMinutes = cfg.TerminalSessionTimeoutMinutes
	}
	timeout := time.Duration(timeoutMinutes) * time.Minute

	sm.mu.Lock()
	defer sm.mu.Unlock()
	for id, s := range sm.sessions {
		if s.Status == "connected" && time.Since(s.LastActiveAt) > timeout {
			logger.Infof("session: idle timeout %s (%s inactive for %v)", id, s.Label, time.Since(s.LastActiveAt))
			sm.disconnectLocked(s)
			s.Status = "disconnected"
		}
	}
}

// CreateOrReplace 创建或覆盖一个会话记录。
func (sm *sessionManager) CreateOrReplace(si *SessionInfo) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	// 先关闭已有的同 ID 会话
	if old, ok := sm.sessions[si.ID]; ok {
		sm.disconnectLocked(old)
	}
	sm.sessions[si.ID] = si
}

// Get 获取指定会话。
func (sm *sessionManager) Get(id string) *SessionInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[id]
}

// ListResponse 会话列表 API 响应（不含内部字段）。
type ListResponse struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	DirID        string    `json:"dir_id,omitempty"`
	Label        string    `json:"label"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// List 返回所有会话的公开摘要。
func (sm *sessionManager) List() []ListResponse {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make([]ListResponse, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		result = append(result, ListResponse{
			ID:           s.ID,
			Type:         s.Type,
			DirID:        s.DirID,
			Label:        s.Label,
			Status:       s.Status,
			CreatedAt:    s.CreatedAt,
			LastActiveAt: s.LastActiveAt,
		})
	}
	return result
}

// MarkActive 更新最后活跃时间。
func (sm *sessionManager) MarkActive(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[id]; ok {
		s.LastActiveAt = time.Now()
	}
}

// Disconnect 断开指定会话（不删除记录，保留供前端显示）。
func (sm *sessionManager) Disconnect(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[id]; ok && s.Status == "connected" {
		sm.disconnectLocked(s)
		s.Status = "disconnected"
	}
}

func (sm *sessionManager) disconnectLocked(s *SessionInfo) {
	if s.wsConn != nil {
		s.wsConn.Close()
		s.wsConn = nil
	}
	if s.ptmx != nil {
		s.ptmx.Close()
		s.ptmx = nil
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd = nil
	}
}

// MarkDisconnected 标记 WebSocket 异常断开。
func (sm *sessionManager) MarkDisconnected(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[id]; ok && s.Status == "connected" {
		s.Status = "disconnected"
		s.wsConn = nil
	}
}

// === API Handlers ===

// handleTerminalSessions  GET /api/terminal/sessions
func (s *APIServer) handleTerminalSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, terminalSessions.List())
}

// handleTerminalDisconnect  POST /api/terminal/disconnect
func (s *APIServer) handleTerminalDisconnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.SessionID == "" {
		writeErr(w, http.StatusBadRequest, "session_id is required")
		return
	}
	terminalSessions.Disconnect(req.SessionID)
	writeJSON(w, map[string]string{"status": "ok"})
}
