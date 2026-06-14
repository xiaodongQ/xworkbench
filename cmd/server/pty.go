//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// PTYSession holds a single PTY session state, registered globally by tab_id.
type PTYSession struct {
	ptmx  *os.File
	cmd   *exec.Cmd
	tabID string
	mu    sync.Mutex
}

// ptySessions 全局 PTY session 注册表，按 tab_id 索引。
// 用于 REST API submit-input 绕过 WebSocket 直接写 PTY stdin。
var (
	ptySessions = make(map[string]*PTYSession)
	ptyMu       sync.RWMutex
)

// registerPTY 将 session 注册到全局表，goroutine 结束时自动注销。
func registerPTY(tabID string, sess *PTYSession) {
	ptyMu.Lock()
	ptySessions[tabID] = sess
	ptyMu.Unlock()
	go func() {
		// 等待 cmd 结束
		sess.cmd.Wait()
		ptyMu.Lock()
		delete(ptySessions, tabID)
		ptyMu.Unlock()
	}()
}

// FindPTY 查找活跃 PTY session。
func FindPTY(tabID string) *PTYSession {
	ptyMu.RLock()
	defer ptyMu.RUnlock()
	return ptySessions[tabID]
}

// WriteInput 向 PTY stdin 写入字符串（自动加换行）。
func (s *PTYSession) WriteInput(input string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.ptmx.WriteString(input + "\n")
	return err
}

// authRequiredPatterns 检测需要授权的输出模式（中英文）。
var authRequiredPatterns = []string{
	"Approve",
	"y/N",
	"[Y/n]",
	"[y/N]",
	"Yes/No",
	"yes/no",
	"CONFIRM",
	"confirm",
	"permission",
	"continue anyway",
	"是否确认",
	"请确认",
	"是否需要",
	"按 Y 确认",
	"请按",
}

// authRequiredAntiPatterns 排除项（permission denied 等误报）。
var authRequiredAntiPatterns = []string{
	"Permission denied",
	"permission denied",
	"read permission",
	"write permission",
	"no confirmation",
	"No confirmation",
	"不需要确认",
	"无需确认",
}

// detectAuthRequired 检查一行是否包含授权提示
func detectAuthRequired(line string) bool {
	for _, p := range authRequiredAntiPatterns {
		if strings.Contains(line, p) {
			return false
		}
	}
	for _, p := range authRequiredPatterns {
		if strings.Contains(line, p) {
			return true
		}
	}
	return false
}

// wsNotifyMsg 构造一个 JSON 通知消息发给前端
type wsNotifyMsg struct {
	Type  string `json:"type"`
	TabID string `json:"tab_id"`
	Extra string `json:"extra,omitempty"`
}

func sendNotify(conn *websocket.Conn, tabID, notifyType, extra string) {
	if conn == nil {
		return
	}
	data, _ := json.Marshal(wsNotifyMsg{Type: notifyType, TabID: tabID, Extra: extra})
	conn.WriteMessage(websocket.TextMessage, data)
}

// handlePty 启动一个 PTY + WebSocket 终端会话。
// query param: tab_id 用于多 Tab 区分
func (s *APIServer) handlePty(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tab_id")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	cliType, _ := s.setDB.Get("aichat_default_cli")
	if cliType == "" {
		cliType = "claude"
	}

	ctxDir := getContextDir()
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	cmdStr := determineAICmd(cliType, ctxDir)
	var cmd *exec.Cmd
	if cmdStr == "" {
		cmd = exec.Command(shell, "-i")
	} else {
		cmd = exec.Command(shell, "-c", cmdStr)
	}
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "TERM=xterm-256color", "CLAUDE_CODE_PROMPT_PATH="+ctxDir)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("pty start error: %v", err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] PTY 启动失败: "+err.Error()+"\x1b[0m\r\n"))
		return
	}

	sess := &PTYSession{ptmx: ptmx, cmd: cmd, tabID: tabID}
	registerPTY(tabID, sess)

	defer func() {
		ptmx.Close()
		cmd.Process.Kill()
	}()

	banner := fmt.Sprintf("\x1b[36m[xworkbench] PTY 启动 (shell=%s, cli=%s)\x1b[0m\r\n", shell, cliType)
	conn.WriteMessage(websocket.TextMessage, []byte(banner))

	var wg sync.WaitGroup

	// PTY 输出 → WebSocket + auth 检测
	wg.Add(1)
	go func() {
		defer wg.Done()
		var lineBuf strings.Builder
		authNotified := false
		buf := make([]byte, 1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				conn.WriteMessage(websocket.BinaryMessage, buf[:n])
				if !authNotified {
					for _, b := range buf[:n] {
						if b == '\n' || b == '\r' {
							line := strings.TrimRight(lineBuf.String(), "\r\n")
							if detectAuthRequired(line) {
								sendNotify(conn, tabID, "auth_required", line)
								authNotified = true
							}
							lineBuf.Reset()
						} else if unicode.IsPrint(rune(b)) || b == '\t' {
							lineBuf.WriteByte(b)
						}
					}
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// WebSocket 输入 → PTY（含 resize 检测）
	_, _ = io.Copy(ptmx, &wsReader{conn: conn, ptmx: ptmx})

	wg.Wait()
}

// handlePtyInput 处理 POST /api/pty/{tab_id}/submit-input
// 向指定 tab_id 的 PTY stdin 写入用户输入（用于授权确认）。
func (s *APIServer) handlePtyInput(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("tab_id")
	if tabID == "" {
		writeErr(w, http.StatusBadRequest, "tab_id is required")
		return
	}
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	sess := FindPTY(tabID)
	if sess == nil {
		writeErr(w, http.StatusNotFound, "no active PTY session for this tab")
		return
	}
	if err := sess.WriteInput(req.Input); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok", "tab_id": tabID})
}

// wsReader 将 WebSocket 消息转发到 PTY，同时拦截 resize 消息。
type wsReader struct {
	conn *websocket.Conn
	ptmx *os.File
}

func (r *wsReader) Read(p []byte) (int, error) {
	msgType, data, err := r.conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	if msgType == websocket.TextMessage && strings.HasPrefix(string(data), "resize,") {
		parts := strings.Split(string(data), ",")
		if len(parts) == 3 {
			var ws pty.Winsize
			ws.Cols = uint16(parseInt(parts[1], 80))
			ws.Rows = uint16(parseInt(parts[2], 24))
			pty.Setsize(r.ptmx, &ws)
		}
		return 0, nil
	}
	if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
		return copy(p, data), nil
	}
	return 0, nil
}

func getContextDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	ctxDir := filepath.Join(filepath.Dir(exe), ".xworkbench", "context")
	if _, err := os.Stat(ctxDir); err == nil {
		return ctxDir
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	ctxDir = filepath.Join(cwd, ".xworkbench", "context")
	if _, err := os.Stat(ctxDir); err == nil {
		return ctxDir
	}
	return ""
}

// determineAICmd 根据 CLI 类型构造 AI 命令。
func determineAICmd(cliType, ctxDir string) string {
	if cmd := os.Getenv("CLAUDE_CMD"); cmd != "" {
		return cmd
	}
	switch cliType {
	case "codex":
		return "codex"
	case "cbc":
		return "cbc"
	case "shell":
		return "sh"
	default:
		cliType = "claude"
	}
	if ctxDir == "" {
		return "claude"
	}
	promptFiles := ""
	for _, f := range []string{"system-prompt.md", "task-schema.md", "experience-schema.md"} {
		if data, err := os.ReadFile(filepath.Join(ctxDir, f)); err == nil {
			promptFiles += string(data) + "\n\n"
		}
	}
	if promptFiles != "" {
		tmpFile := "/tmp/claude-code-xworkbench-prompt.txt"
		os.WriteFile(tmpFile, []byte(promptFiles), 0644)
		return "claude --prompt-file " + tmpFile
	}
	return "claude"
}