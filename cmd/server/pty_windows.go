//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/x/xpty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// handlePty 启动一个 ConPTY + WebSocket 终端会话。
func (s *APIServer) handlePty(w http.ResponseWriter, r *http.Request) {
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
	cmdStr := determineAICmd(cliType, ctxDir)
	var cmd *exec.Cmd
	if cmdStr == "" {
		cmd = exec.Command("cmd.exe")
	} else {
		cmd = exec.Command("cmd.exe", "/c", cmdStr)
	}
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")

	// 创建 ConPTY，80 列 24 行
	pty, err := xpty.NewPty(80, 24)
	if err != nil {
		log.Printf("xpty new error: %v", err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] ConPTY 启动失败: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	defer pty.Close()

	if err := pty.Start(cmd); err != nil {
		log.Printf("xpty start error: %v", err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] ConPTY 启动失败: "+err.Error()+"\x1b[0m\r\n"))
		return
	}

	banner := fmt.Sprintf("\x1b[36m[xworkbench] ConPTY 启动 (cli=%s)\x1b[0m\r\n", cliType)
	conn.WriteMessage(websocket.TextMessage, []byte(banner))

	// PTY 输出 → WebSocket
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := pty.Read(buf)
			if err != nil {
				break
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				break
			}
		}
	}()

	// WebSocket 输入 → PTY（含 resize 检测）
	_, err = io.Copy(pty, &wsReader{conn: conn, pty: pty})

	// Windows ConPTY 需要用 xpty.WaitProcess 等待进程退出
	xpty.WaitProcess(context.Background(), cmd)

	wg.Wait()
}

// wsReader 将 WebSocket 消息转发到 PTY，同时拦截 resize 消息。
type wsReader struct {
	conn *websocket.Conn
	pty  xpty.Pty
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
			cols := parseInt(parts[1], 80)
			rows := parseInt(parts[2], 24)
			r.pty.Resize(cols, rows)
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
		return ""
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
		tmpFile := os.Getenv("TEMP") + "\\claude-code-xworkbench-prompt.txt"
		os.WriteFile(tmpFile, []byte(promptFiles), 0644)
		return "claude --prompt-file " + tmpFile
	}
	return "claude"
}

