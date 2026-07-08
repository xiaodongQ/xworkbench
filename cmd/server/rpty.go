//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gorilla/websocket"
	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/executor"
	"golang.org/x/crypto/ssh"
	"os"
)

var rptyUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// RPTYSession holds a single remote PTY session over SSH.
// Registered globally by tab_id, used by handleRptyInput (REST submit-input).
// Supports reconnect: WS close with ?reconnect=1 starts a grace period (60s) before
// SSH session cleanup. Reconnect within grace period reuses the existing session.
type RPTYSession struct {
	tabID         string
	dirID         string
	client        *ssh.Client
	session       *ssh.Session
	stdin         io.WriteCloser
	stdout        io.Reader
	wsClosed      bool        // WS 已断开，等待重连或清理
	wsCloseTime   time.Time   // WS 断开时间
	cleanupTimer  *time.Timer // 宽限期计时器，超时后关闭 SSH
	mu            sync.Mutex
}

// ReconnectGraceSec 重连宽限期秒数。
const ReconnectGraceSec = 60

// rptySessions 全局 RPTY session 注册表，按 tab_id 索引。
var (
	rptySessions = make(map[string]*RPTYSession)
	rptyMu       sync.RWMutex
)

// RegisterRPTY 注册 RPTY session 到全局表，goroutine 结束时自动注销。
func RegisterRPTY(tabID string, sess *RPTYSession) {
	rptyMu.Lock()
	rptySessions[tabID] = sess
	rptyMu.Unlock()
	go func() {
		if sess.session != nil {
			sess.session.Wait()
		}
		rptyMu.Lock()
		delete(rptySessions, tabID)
		rptyMu.Unlock()
	}()
}

// FindRPTY 查找活跃或处于重连宽限期的 RPTY session。
func FindRPTY(tabID string) *RPTYSession {
	rptyMu.RLock()
	defer rptyMu.RUnlock()
	return rptySessions[tabID]
}

// UnregisterRPTY 主动注销 RPTY session（真正清理，不走宽限期）。
func UnregisterRPTY(tabID string) {
	rptyMu.Lock()
	sess, ok := rptySessions[tabID]
	if ok {
		delete(rptySessions, tabID)
	}
	rptyMu.Unlock()
	if sess != nil {
		if sess.cleanupTimer != nil {
			sess.cleanupTimer.Stop()
		}
		go func() {
			if sess.session != nil { sess.session.Close() }
			if sess.client != nil { sess.client.Close() }
		}()
	}
}

// markWsClosed 标记 session 进入重连宽限期（WS 刷新断开），启动清理计时器。
// 宽限期内重连（FindRPTY）会看到 wsClosed=true，取消计时器并复用 session。
func (s *RPTYSession) markWsClosed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wsClosed { return } // 已在宽限期
	s.wsClosed = true
	s.wsCloseTime = time.Now()
	s.cleanupTimer = time.AfterFunc(ReconnectGraceSec*time.Second, func() {
		logger.Infof("rpty: grace period expired tab_id=%q, closing SSH", s.tabID)
		UnregisterRPTY(s.tabID)
	})
	logger.Infof("rpty: ws closed, grace period started tab_id=%q", s.tabID)
}

// cancelGracePeriod 取消重连宽限期（重连成功时调用）。
func (s *RPTYSession) cancelGracePeriod() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.wsClosed { return }
	if s.cleanupTimer != nil {
		s.cleanupTimer.Stop()
		s.cleanupTimer = nil
	}
	s.wsClosed = false
	logger.Infof("rpty: reconnect succeeded, grace period cancelled tab_id=%q", s.tabID)
}

// WriteInput 向远程 PTY stdin 写入字符串（自动加换行）。
func (s *RPTYSession) WriteInput(input string) error {
	if s == nil || s.stdin == nil {
		return fmt.Errorf("session not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.stdin.Write([]byte(input + "\n"))
	return err
}

// handleRemotePty 启动一个远程 SSH PTY + WebSocket 终端会话。
// query param:
//   - tab_id: PTY session 唯一标识（前端 tab 维度）
//   - dir_id: DirShortcut ID，解析 host/user/auth 配置
func (s *APIServer) handleRemotePty(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tab_id")
	dirID := r.URL.Query().Get("dir_id")

	if tabID == "" || dirID == "" {
		writeErr(w, http.StatusBadRequest, "tab_id and dir_id are required")
		return
	}

	// 查找 DirShortcut
	list, err := s.dirDB.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
		writeErr(w, http.StatusNotFound, "shortcut not found")
		return
	}
	if dir.Type != backend.DirShortcutTypeRemote {
		writeErr(w, http.StatusBadRequest, "not a remote shortcut")
		return
	}

	// 构建 SSH 地址
	port := dir.RemotePort
	if port == "" {
		port = "22"
	}
	addr := net.JoinHostPort(dir.RemoteHost, port)
	user := dir.RemoteUser
	if user == "" {
		user = "root"
	}

	// 升级 WebSocket（提前到此，认证失败时也能通过 WS 回写错误）
	conn, err := rptyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("rpty: websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()
	logger.Infow("rpty: ws upgraded", "tab_id", tabID, "host", dir.RemoteHost, "user", user)

	// 建立 SSH 连接（支持 key 或 password 认证）
	var authMethods []ssh.AuthMethod
	// 密钥认证（有密钥文件时优先）
	if dir.AuthMethod == "key" {
		keyPath := executor.ResolveKeyPath(dir)
		keyData, kerr := os.ReadFile(keyPath)
		if kerr == nil {
			var signer ssh.Signer
			var serr error
			if dir.KeyPassword != "" {
				signer, serr = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(dir.KeyPassword))
			} else {
				signer, serr = ssh.ParsePrivateKey(keyData)
			}
			if serr == nil {
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
		}
	}
	// 密码认证
	if dir.RemotePassword != "" {
		authMethods = append(authMethods, ssh.Password(dir.RemotePassword))
	}
	if len(authMethods) == 0 {
		conn.WriteMessage(websocket.TextMessage,
			[]byte("\r\n\x1b[31m[xworkbench] 无可用认证方式（未配置密钥也无密码）\x1b[0m\r\n"))
		return
	}
	sshCfg := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		Timeout:         0,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 演示用
	}

	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		logger.Errorf("rpty: ssh dial error: %v", err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte(fmt.Sprintf("\r\n\x1b[31m[xworkbench] SSH 连接失败: %v\x1b[0m\r\n", err)))
		return
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		logger.Errorf("rpty: new session error: %v", err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte(fmt.Sprintf("\r\n\x1b[31m[xworkbench] SSH 会话创建失败: %v\x1b[0m\r\n", err)))
		return
	}

	// 等待前端的第一条 resize 消息，获取正确的 PTY 尺寸
	// 这确保 bash 启动时就获得正确的 COLUMNS/LINES
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	conn.SetReadDeadline(time.Time{}) // 清除超时
	if err != nil || !strings.HasPrefix(string(data), "resize,") {
		// 没有收到 resize 或超时，使用默认值
		logger.Warnf("rpty: no resize received, using defaults tab_id=%q", tabID)
	}
	var cols, rows int = 120, 40
	if strings.HasPrefix(string(data), "resize,") {
		parts := strings.Split(string(data), ",")
		if len(parts) == 3 {
			cols = parseInt(parts[1], 80)
			rows = parseInt(parts[2], 24)
		}
	}
	logger.Infof("rpty: using PTY size %dx%d tab_id=%q", cols, rows, tabID)

	// 请求 PTY（使用前端指定的尺寸）
	// ECHO=0：服务器不回显输入字符，前端 xterm.js 通过 setLocalEchoHandler
	// 显示本地 ghost echo（打字时立即显示，输入行处理完毕后自动消失）。
	// 这样既避免双字符（服务器+xterm.js 各回显一次），又保证打字立即可见。
	modes := ssh.TerminalModes{
		ssh.ECHO:          0, // 关闭服务器回显，前端负责本地 echo
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		session.Close()
		client.Close()
		logger.Errorf("rpty: request pty error: %v", err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte(fmt.Sprintf("\r\n\x1b[31m[xworkbench] PTY 请求失败: %v\x1b[0m\r\n", err)))
		return
	}

	// 打开 stdin/stdout pipe
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		logger.Errorf("rpty: stdin pipe error: %v", err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte(fmt.Sprintf("\r\n\x1b[31m[xworkbench] STDIN 管道失败: %v\x1b[0m\r\n", err)))
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		logger.Errorf("rpty: stdout pipe error: %v", err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte(fmt.Sprintf("\r\n\x1b[31m[xworkbench] STDOUT 管道失败: %v\x1b[0m\r\n", err)))
		return
	}

	// 注册 session
	sess := &RPTYSession{
		tabID:   tabID,
		dirID:   dirID,
		client:  client,
		session: session,
		stdin:   stdin,
		stdout:  stdout,
	}
	RegisterRPTY(tabID, sess)

	// 构建 cd 到 remote_path 的启动命令
	// 显式设置 COLUMNS/LINES 环境变量，确保 bash 使用正确的终端宽度
	// 使用 $SHELL 或 fallback 到 bash，避免 exec 失败导致连接断开
	shellCmd := fmt.Sprintf(`COLUMNS=%d LINES=%d SHELL=${SHELL:-/bin/bash} && exec ${SHELL} -l`, cols, rows)
	if dir.RemotePath != "" {
		// 先 cd 到目录，失败时仍保持 shell 存活（用 || true 防止 cd 失败导致 shell 退出）
		shellCmd = fmt.Sprintf(`COLUMNS=%d LINES=%d SHELL=${SHELL:-/bin/bash} && cd "%s" 2>/dev/null || true && exec ${SHELL} -l`, cols, rows, dir.RemotePath)
	}
	if dir.TerminalCmd != "" {
		shellCmd = fmt.Sprintf(`COLUMNS=%d LINES=%d SHELL=${SHELL:-/bin/bash} && cd "%s" 2>/dev/null || true && %s && exec ${SHELL} -l`, cols, rows, dir.RemotePath, dir.TerminalCmd)
	}

	if err := session.Start(shellCmd); err != nil {
		UnregisterRPTY(tabID)
		session.Close()
		client.Close()
		logger.Errorf("rpty: start shell error: %v", err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte(fmt.Sprintf("\r\n\x1b[31m[xworkbench] 启动 Shell 失败: %v\x1b[0m\r\n", err)))
		return
	}

	banner := fmt.Sprintf("\x1b[36m[xworkbench] SSH 已连接: %s@%s\x1b[0m\r\n", user, dir.RemoteHost)
	conn.WriteMessage(websocket.TextMessage, []byte(banner))

	// 监听 session 退出
	go func() {
		err := session.Wait()
		exitCode := -1
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		}
		if err != nil {
			logger.Infof("rpty: session exited tab_id=%q exitCode=%d err=%v", tabID, exitCode, err)
		} else {
			logger.Infof("rpty: session exited tab_id=%q exitCode=%d", tabID, exitCode)
		}
		conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[33m[连接已关闭]\x1b[0m\r\n"))
		conn.Close()
		UnregisterRPTY(tabID)
	}()

	var wg sync.WaitGroup

	// PTY stdout → WebSocket + auth 检测
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		var lineBuf strings.Builder
		authNotified := false
		for {
			n, rerr := stdout.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					logger.Errorf("rpty: ws write error: %v", werr)
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
					logger.Infof("rpty: read EOF tab_id=%q", tabID)
				} else {
					logger.Errorf("rpty: read error tab_id=%q err=%v", tabID, rerr)
				}
				break
			}
		}
	}()

	// WebSocket 输入 → PTY stdin
	_, err = io.Copy(stdin, &rptyWsReader{conn: conn, session: session})
	logger.Infof("rpty: ws input closed tab_id=%q err=%v", tabID, err)

	wg.Wait()
	logger.Infof("rpty: fully closed tab_id=%q", tabID)

	// 清理
	session.Close()
	client.Close()
}

// rptyWsReader 将 WebSocket 消息转发到远程 SSH stdin，同时拦截 resize 消息。
type rptyWsReader struct {
	conn    *websocket.Conn
	session *ssh.Session
}

func (r *rptyWsReader) Read(p []byte) (int, error) {
	for {
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
				_ = r.session.WindowChange(cols, rows)
			}
			continue // 丢弃，继续读下一条消息（不能返回 0,nil 否则 io.Copy 死循环）
		}
		if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
			return copy(p, data), nil
		}
	}
}

// handleRptyInput 处理 POST /api/rpty/{tab_id}/submit-input
// 向指定 tab_id 的远程 PTY stdin 写入用户输入（用于授权确认）。
func (s *APIServer) handleRptyInput(w http.ResponseWriter, r *http.Request) {
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
	sess := FindRPTY(tabID)
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