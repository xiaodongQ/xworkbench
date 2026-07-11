//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"github.com/xiaodongQ/xworkbench/internal/executor"
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

// authRequiredPatterns 检测需要授权的输出模式（中英文 + SSH 特有）。
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
	// SSH 特有
	"Password:",
	"Enter passphrase for key",
	"Passphrase for key",
	"Are you sure you want to continue connecting",
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
// query param:
//   - tab_id: 多 Tab 区分标识
//   - session_id: Claude Code 会话 ID（--session-id），稳定身份
//   - resume_uuid: Claude Code 会话续接 ID（--resume），对话历史续接
func (s *APIServer) handlePty(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tab_id")
	sessionID := r.URL.Query().Get("session_id")
	resumeUUID := r.URL.Query().Get("resume_uuid")
	logger.Infof("pty: ws open request tab_id=%q session_id=%q resume_uuid=%q remote=%s", tabID, sessionID, resumeUUID, r.RemoteAddr)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("pty: websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()
	logger.Infof("pty: ws upgraded tab_id=%q", tabID)

	// dir_id 非空 → 远程 SSH via xw-sshpass + creack/pty
	dirID := r.URL.Query().Get("dir_id")
	if dirID != "" {
		s.handlePtyRemote(w, r, conn, tabID, dirID)
		return
	}

	// cli_type query 参数优先，否则读配置
	cliType := r.URL.Query().Get("cli_type")
	if cliType == "" {
		if cfg := config.Get(); cfg != nil {
			cliType = cfg.AichatDefaultCLI
		}
		if cliType == "" {
			cliType = "claude"
		}
	}

	ctxDir := getContextDir()
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	cmdStr := determineAICmd(cliType, ctxDir, sessionID, resumeUUID)
	var cmd *exec.Cmd
	if cmdStr == "" {
		cmd = exec.Command(shell, "-i")
	} else {
		cmd = exec.Command(shell, "-c", cmdStr)
	}
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "TERM=xterm-256color", "CLAUDE_CODE_PROMPT_PATH="+ctxDir)

	logger.Infof("pty: cmd ready tab_id=%q cli=%s shell=%s cmdStr=%q ctxDir=%q argv=%v",
		tabID, cliType, shell, cmdStr, ctxDir, cmd.Args)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "TERM=xterm-256color", "CLAUDE_CODE_PROMPT_PATH="+ctxDir)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		logger.Errorf("pty: pty.Start error tab_id=%q err=%v", tabID, err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] PTY 启动失败: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	logger.Infof("pty: pty started tab_id=%q pid=%d", tabID, pid)

	sess := &PTYSession{ptmx: ptmx, cmd: cmd, tabID: tabID}
	registerPTY(tabID, sess)

	defer func() {
		ptmx.Close()
		_ = cmd.Process.Kill()
		logger.Infof("pty: cleanup done tab_id=%q pid=%d", tabID, pid)
	}()

	banner := fmt.Sprintf("\x1b[36m[xworkbench] PTY 启动 (shell=%s, cli=%s)\x1b[0m\r\n", shell, cliType)
	conn.WriteMessage(websocket.TextMessage, []byte(banner))

	// 监听子进程退出,记录退出码(便于排查"启动了但没 prompt")
	go func() {
		werr := cmd.Wait()
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

	var wg sync.WaitGroup

	// PTY 输出 → WebSocket + auth 检测
	wg.Add(1)
	go func() {
		defer wg.Done()
		var lineBuf strings.Builder
		authNotified := false
		buf := make([]byte, 1024)
		totalBytes := 0
		reads := 0
		for {
			n, rerr := ptmx.Read(buf)
			reads++
			if n > 0 {
				totalBytes += n
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					logger.Errorf("pty: ws write error tab_id=%q err=%v", tabID, werr)
					return
				}
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
			if rerr != nil {
				if rerr == io.EOF {
					logger.Infof("pty: read EOF tab_id=%q bytes=%d reads=%d", tabID, totalBytes, reads)
				} else {
					logger.Errorf("pty: read error tab_id=%q err=%v bytes=%d reads=%d",
						tabID, rerr, totalBytes, reads)
				}
				break
			}
		}
	}()

	// WebSocket 输入 → PTY（含 resize 检测）
	inBytes, err := io.Copy(ptmx, &wsReader{conn: conn, ptmx: ptmx})
	logger.Infof("pty: ws input closed tab_id=%q err=%v inBytes=%d", tabID, err, inBytes)

	wg.Wait()
	logger.Infof("pty: ws fully closed tab_id=%q", tabID)
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
	for {
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
			continue // 丢弃，继续读下一条消息（不能返回 0,nil 否则 io.Copy 死循环）
		}
		if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
			return copy(p, data), nil
		}
	}
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

// determineAICmd 根据 CLI 类型和会话参数构造 AI 命令。
//
// 注释:这里是 PTY(交互式终端)路径,只决定启动哪个 CLI 让用户输入。
// 之前版本会从 context/ 目录读 system-prompt / schemas 拼成文件再传
// `claude --prompt-file <file>`,但 claude CLI 实际没有这个 flag(只有
// `claude --print "..."` 位置参数形式),导致 cli=claude 启动立即报
// `error: unknown option '--prompt-file'` 后退出,PTY 看不到 prompt。
// 任务执行链路(`internal/executor/runner`)才需要 prompt 注入,不在此处。
func determineAICmd(cliType, ctxDir, sessionID, resumeUUID string) string {
	if cmd := os.Getenv("CLAUDE_CMD"); cmd != "" {
		return enrichCmd(cmd, sessionID, resumeUUID)
	}
	switch cliType {
	case "cbc":
		return enrichCmd("cbc", sessionID, resumeUUID)
	case "shell":
		return "sh"
	default:
		return enrichCmd("claude", sessionID, resumeUUID)
	}
}

// enrichCmd 给命令加 --resume 参数。
// 只有 resumeUUID 非空时才加 --resume（sessionID 在 PTY 场景是 tab ID，不是 AI session）。
func enrichCmd(cmd string, sessionID, resumeUUID string) string {
	if resumeUUID == "" {
		return cmd
	}
	return cmd + " --resume " + resumeUUID
}

// handlePtyRemote 处理远程 SSH PTY：通过 xw-sshpass 子进程 + creack/pty 包装。
// dirID 对应一个 DirShortcut，解析后构建 xw-sshpass ssh ... 命令并启动。
func (s *APIServer) handlePtyRemote(w http.ResponseWriter, r *http.Request, conn *websocket.Conn, tabID, dirID string) {
	// 查找 DirShortcut
	list, err := s.dirDB.List()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] 无法读取目录配置: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	var dir *backend.DirShortcut
	for _, d := range list {
		if d.ID == dirID {
			dir = d
			break
		}
	}
	if dir == nil {
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] 未找到该目录配置\x1b[0m\r\n"))
		return
	}
	if dir.Type != backend.DirShortcutTypeRemote {
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] 该目录不是远程类型\x1b[0m\r\n"))
		return
	}

	// 构建 SSH 目标地址
	port := dir.RemotePort
	if port == "" {
		port = "22"
	}
	userHost := dir.RemoteUser
	if userHost == "" {
		userHost = "root"
	}
	userHost = userHost + "@" + dir.RemoteHost
	if port != "22" {
		userHost = userHost + ":" + port
	}

	// 解析 xw-sshpass 路径（直接复用 terminal.go 的逻辑）
	xwBin := resolveXwSshpassBin()
	if xwBin == "" {
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] 未找到 xw-sshpass 二进制，请确认已安装\x1b[0m\r\n"))
		return
	}

	// 构建 xw-sshpass 命令参数（直接 exec，不用 shell -c，确保 PTY 正确）
	var xwArgs []string
	if dir.AuthMethod == "key" {
		// 密钥认证
		keyPath := executor.ResolveKeyPath(dir)
		if keyPath == "" {
			conn.WriteMessage(websocket.TextMessage,
				[]byte("\r\n\x1b[31m[xworkbench] 未找到 SSH 密钥文件\x1b[0m\r\n"))
			return
		}
		xwArgs = []string{xwBin, "-i", keyPath, "ssh", userHost}
	} else if dir.RemotePassword != "" {
		// 密码认证
		xwArgs = []string{xwBin, "-p", dir.RemotePassword, "ssh", userHost}
	} else {
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] 无可用认证方式（未配置密钥也无密码）\x1b[0m\r\n"))
		return
	}

	// 如果有 remote_path，用 -w 选项切换目录
	if dir.RemotePath != "" {
		// 在 ssh 之前插入 -w <path>
		insertAt := len(xwArgs) - 1 // 在 "ssh" 之前插入
		newArgs := make([]string, 0, len(xwArgs)+2)
		newArgs = append(newArgs, xwArgs[:insertAt]...)
		newArgs = append(newArgs, "-w", dir.RemotePath)
		newArgs = append(newArgs, xwArgs[insertAt:]...)
		xwArgs = newArgs
	}

	logger.Infof("pty: remote cmd ready tab_id=%q args=%v", tabID, xwArgs)

	cmd := exec.Command(xwBin, xwArgs[1:]...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		logger.Errorf("pty: remote pty.Start error tab_id=%q err=%v", tabID, err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] PTY 启动失败: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	logger.Infof("pty: remote pty started tab_id=%q pid=%d", tabID, pid)

	sess := &PTYSession{ptmx: ptmx, cmd: cmd, tabID: tabID}
	registerPTY(tabID, sess)

	defer func() {
		ptmx.Close()
		_ = cmd.Process.Kill()
		logger.Infof("pty: remote cleanup done tab_id=%q pid=%d", tabID, pid)
	}()

	banner := fmt.Sprintf("\x1b[36m[xworkbench] SSH 已连接: %s\x1b[0m\r\n", userHost)
	conn.WriteMessage(websocket.TextMessage, []byte(banner))

	logger.Infof("pty: remote banner sent, ptmx fd=%v tab_id=%q", ptmx.Fd(), tabID)

	// 监听子进程退出
	go func() {
		werr := cmd.Wait()
		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		if werr != nil {
			logger.Infof("pty: remote cli exited tab_id=%q pid=%d err=%v exitCode=%d",
				tabID, pid, werr, exitCode)
		} else {
			logger.Infof("pty: remote cli exited tab_id=%q pid=%d exitCode=%d",
				tabID, pid, exitCode)
		}
	}()

	var wg sync.WaitGroup

	// PTY 输出 → WebSocket + auth 检测
	wg.Add(1)
	go func() {
		defer wg.Done()
		var lineBuf strings.Builder
		authNotified := false
		buf := make([]byte, 4096)
		totalBytes := 0
		reads := 0
		for {
			n, rerr := ptmx.Read(buf)
			reads++
			if reads <= 3 || n > 0 {
				logger.Infof("pty: [REMOTE] ptmx.Read tab_id=%q n=%d rerr=%v reads=%d totalBytes=%d", tabID, n, rerr, reads, totalBytes)
			}
			if n > 0 {
				totalBytes += n
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					logger.Errorf("pty: remote ws write error tab_id=%q err=%v", tabID, werr)
					return
				}
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
			if rerr != nil {
				if rerr == io.EOF {
					logger.Infof("pty: remote read EOF tab_id=%q bytes=%d reads=%d", tabID, totalBytes, reads)
				} else {
					logger.Errorf("pty: remote read error tab_id=%q err=%v bytes=%d reads=%d",
						tabID, rerr, totalBytes, reads)
				}
				break
			}
		}
	}()

	// WebSocket 输入 → PTY
	inBytes, err := io.Copy(ptmx, &wsReader{conn: conn, ptmx: ptmx})
	logger.Infof("pty: remote ws input closed tab_id=%q err=%v inBytes=%d", tabID, err, inBytes)

	wg.Wait()
	logger.Infof("pty: remote fully closed tab_id=%q", tabID)
}

// resolveXwSshpassBin 返回当前平台对应的 xw-sshpass 路径。
// 优先从 tools/xw-sshpass/ 目录查找，找不到则尝试 PATH。
func resolveXwSshpassBin() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	osMap := map[string]string{
		"darwin":  "darwin",
		"linux":   "linux",
		"windows": "windows",
	}
	osStr := osMap[goos]
	if osStr == "" {
		return ""
	}
	archStr := "amd64"
	if goarch == "arm64" {
		archStr = "arm64"
	}
	binName := fmt.Sprintf("xw-sshpass-%s-%s", osStr, archStr)
	if goos == "windows" {
		binName += ".exe"
	}

	// 1. 从 tools/xw-sshpass/ 目录查找
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	toolsDir := filepath.Join(wd, "tools", "xw-sshpass")
	binPath := filepath.Join(toolsDir, binName)
	if _, err := os.Stat(binPath); err == nil {
		return binPath
	}

	// 2. PATH 中查找
	if bin, err := exec.LookPath(binName); err == nil {
		return bin
	}
	// 3. 尝试不带平台后缀的 xw-sshpass
	if bin, err := exec.LookPath("xw-sshpass"); err == nil {
		return bin
	}
	return ""
}
