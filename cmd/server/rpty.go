//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
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
type RPTYSession struct {
	tabID   string
	dirID   string
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	mu      sync.Mutex
}

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

// FindRPTY 查找活跃 RPTY session。
func FindRPTY(tabID string) *RPTYSession {
	rptyMu.RLock()
	defer rptyMu.RUnlock()
	return rptySessions[tabID]
}

// UnregisterRPTY 主动注销 RPTY session。
func UnregisterRPTY(tabID string) {
	rptyMu.Lock()
	delete(rptySessions, tabID)
	rptyMu.Unlock()
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

	// 密钥认证优先；密码认证暂不支持（需要 ssh-agent 注入）
	if dir.AuthMethod != "key" {
		writeErr(w, http.StatusBadRequest, "only key auth is supported for remote PTY")
		return
	}

	// 解析密钥路径
	_ = executor.ResolveKeyPath(dir)

	// 构建 SSH 地址
	addr := dir.RemoteHost
	user := dir.RemoteUser
	if user == "" {
		user = "root"
	}

	// 升级 WebSocket
	conn, err := rptyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("rpty: websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()
	logger.Infow("rpty: ws upgraded", "tab_id", tabID, "host", dir.RemoteHost, "user", user)

	// 建立 SSH 连接（密钥认证）
	keyPath := executor.ResolveKeyPath(dir)
	keyData, kerr := os.ReadFile(keyPath)
	if kerr != nil {
		logger.Errorf("rpty: read key file error: %v", kerr)
		conn.WriteMessage(websocket.TextMessage,
			[]byte(fmt.Sprintf("\r\n\x1b[31m[xworkbench] 读取 SSH 密钥文件失败: %v\x1b[0m\r\n", kerr)))
		return
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		logger.Errorf("rpty: load signer error: %v", err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte(fmt.Sprintf("\r\n\x1b[31m[xworkbench] 加载 SSH 密钥失败: %v\x1b[0m\r\n", err)))
		return
	}
	sshCfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		Timeout:         0,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 演示用
	}
	if dir.RemotePassword != "" {
		sshCfg.Auth = append(sshCfg.Auth, ssh.Password(dir.RemotePassword))
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

	// 请求 PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", 80, 40, modes); err != nil {
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
	shellCmd := "exec $SHELL -l"
	if dir.RemotePath != "" {
		shellCmd = fmt.Sprintf("cd '%s' && exec $SHELL -l", dir.RemotePath)
	}
	if dir.TerminalCmd != "" {
		shellCmd = fmt.Sprintf("cd '%s' && %s && exec $SHELL -l", dir.RemotePath, dir.TerminalCmd)
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
		return 0, nil
	}
	if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
		return copy(p, data), nil
	}
	return 0, nil
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