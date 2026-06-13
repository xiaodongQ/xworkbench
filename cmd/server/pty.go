//go:build !windows

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local development
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type PTYSession struct {
	ptmx *os.File
}

// handlePty 启动一个 PTY + WebSocket 终端会话。
// AI CLI 类型从 aichat_default_cli 设置读取，默认为 claude。
// 支持：claude / cbc / codex / shell。
func (s *APIServer) handlePty(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// 读取用户偏好的 CLI 类型
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
		// shell 类型：进入系统默认 shell 的交互模式
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
	defer ptmx.Close()
	defer cmd.Process.Kill()

	banner := fmt.Sprintf("\x1b[36m[xworkbench] PTY 启动 (shell=%s, cli=%s)\x1b[0m\r\n", shell, cliType)
	conn.WriteMessage(websocket.TextMessage, []byte(banner))

	// PTY 输出 → WebSocket（单独 goroutine）
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				break
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				break
			}
		}
	}()

	// WebSocket 输入 → PTY（含 resize 检测，不开独立 goroutine 避免并发读 WS）
	_, err = io.Copy(ptmx, &wsReader{conn: conn, ptmx: ptmx})

	wg.Wait()
}

// wsReader 将 WebSocket 消息转发到 PTY，同时拦截 resize 消息。
// 不开独立 goroutine 读 WS，避免与 io.Copy 并发读导致死锁。
type wsReader struct {
	conn *websocket.Conn
	ptmx *os.File
}

func (r *wsReader) Read(p []byte) (int, error) {
	msgType, data, err := r.conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	// 拦截 resize 消息："resize,<cols>,<rows>"
	if msgType == websocket.TextMessage && strings.HasPrefix(string(data), "resize,") {
		parts := strings.Split(string(data), ",")
		if len(parts) == 3 {
			var ws pty.Winsize
			ws.Cols = uint16(parseInt(parts[1], 80))
			ws.Rows = uint16(parseInt(parts[2], 24))
			pty.Setsize(r.ptmx, &ws)
		}
		return 0, nil // 不透传 resize 消息给 PTY
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
// ctxDir 存在时读取 .xworkbench/context/*.md 作为 prompt-file。
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