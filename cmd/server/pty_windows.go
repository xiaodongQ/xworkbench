//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/x/xpty"
	"github.com/gorilla/websocket"
	"github.com/xiaodongQ/xworkbench/internal/config"
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
	tabID := r.URL.Query().Get("tab_id")
	logger.Infof("pty: ws open request tab_id=%q remote=%s", tabID, r.RemoteAddr)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("pty: websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()
	logger.Infof("pty: ws upgraded tab_id=%q", tabID)

	cliType := ""
	if cfg := config.Get(); cfg != nil {
		cliType = cfg.AichatDefaultCLI
	}
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

	logger.Infof("pty: cmd ready tab_id=%q cli=%s cmdStr=%q ctxDir=%q argv=%v",
		tabID, cliType, cmdStr, ctxDir, cmd.Args)

	// 创建 ConPTY，80 列 24 行
	pty, err := xpty.NewPty(80, 24)
	if err != nil {
		logger.Errorf("pty: xpty.NewPty error tab_id=%q err=%v", tabID, err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] ConPTY 启动失败: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	defer pty.Close()

	if err := pty.Start(cmd); err != nil {
		logger.Errorf("pty: xpty.Start error tab_id=%q err=%v", tabID, err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] ConPTY 启动失败: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	logger.Infof("pty: xpty started tab_id=%q pid=%d", tabID, pid)

	banner := fmt.Sprintf("\x1b[36m[xworkbench] ConPTY 启动 (cli=%s)\x1b[0m\r\n", cliType)
	conn.WriteMessage(websocket.TextMessage, []byte(banner))

	// 监听子进程退出
	go func() {
		werr := xpty.WaitProcess(context.Background(), cmd)
		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		if werr != nil {
			logger.Infof("pty: cli exited tab_id=%q pid=%d err=%v exitCode=%d",
				tabID, pid, werr, exitCode)
		} else {
			logger.Infof("pty: cli exited tab_id=%q pid=%d exitCode=%d",
				tabID, pid, exitCode)
		}
	}()

	// PTY 输出 → WebSocket
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		totalBytes := 0
		reads := 0
		for {
			n, rerr := pty.Read(buf)
			reads++
			if n > 0 {
				totalBytes += n
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					logger.Infof("pty: ws write error tab_id=%q err=%v", tabID, werr)
					return
				}
			}
			if rerr != nil {
				if rerr == io.EOF {
					logger.Infof("pty: read EOF tab_id=%q bytes=%d reads=%d", tabID, totalBytes, reads)
				} else {
					logger.Infof("pty: read error tab_id=%q err=%v bytes=%d reads=%d",
						tabID, rerr, totalBytes, reads)
				}
				break
			}
		}
	}()

	// WebSocket 输入 → PTY（含 resize 检测）
	inBytes, err := io.Copy(pty, &wsReader{conn: conn, pty: pty})
	logger.Infof("pty: ws input closed tab_id=%q err=%v inBytes=%d", tabID, err, inBytes)

	wg.Wait()
	logger.Infof("pty: ws fully closed tab_id=%q", tabID)
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
	ctxDir := filepath.Join(filepath.Dir(exe), "context")
	if _, err := os.Stat(ctxDir); err == nil {
		return ctxDir
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	ctxDir = filepath.Join(cwd, "context")
	if _, err := os.Stat(ctxDir); err == nil {
		return ctxDir
	}
	return ""
}

// determineAICmd 根据 CLI 类型构造 AI 命令。
//
// 这里是 PTY(交互式终端)路径,只决定启动哪个 CLI 让用户输入。
// 之前版本会从 context/ 目录读 prompt 拼成文件再传 `claude --prompt-file`,
// 但 claude CLI 没有这个 flag,会启动失败。任务执行链路才需要 prompt 注入。
func determineAICmd(cliType, ctxDir string) string {
	_ = ctxDir // 保留参数避免改调用方
	if cmd := os.Getenv("CLAUDE_CMD"); cmd != "" {
		return cmd
	}
	switch cliType {
	case "cbc":
		return "cbc"
	case "shell":
		return ""
	default:
		return "claude"
	}
}

// FindPTY stub for Windows — PTY sessions not yet implemented.

// PTYSession stub for Windows build.
type PTYSession struct{}

// WriteInput stub for Windows.
func (s *PTYSession) WriteInput(input string) error { return nil }

func FindPTY(tabID string) *PTYSession { return nil }

// handlePtyInput stub for Windows — returns 404.
func (s *APIServer) handlePtyInput(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotFound, "PTY submit-input not supported on Windows")
}
