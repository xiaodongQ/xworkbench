package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xiaodongQ/xworkbench/internal/wsmsg"
)

// TestBroadcast_ChannelExecChunk 验证 ChannelExec 推送格式：前端 ws-client.js
// 期望收到 {"channel":"exec","payload":{"execution_id":"...","chunk":"..."}}
// 这条推送会让 exec-detail-modal 实时显示 stdout 流。
func TestBroadcast_ChannelExecChunk(t *testing.T) {
	h := New()
	// 模拟一个 ws 客户端
	connCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		connCh <- c
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	// 等待 server 端 register 完成（upgrader 同步触发）
	c := <-connCh
	h.Register(c)
	defer h.Unregister(c)

	// 触发 Broadcast
	h.Broadcast(wsmsg.ChannelExec, map[string]any{
		"execution_id": "exec-123",
		"task_id":      "task-456",
		"chunk":        "hello world chunk",
	})

	// 客户端读取（带超时避免 hang）
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// 解析 + 断言格式
	var msg wsmsg.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v, raw=%s", err, string(data))
	}
	if msg.Channel != wsmsg.ChannelExec {
		t.Errorf("channel = %q, want %q", msg.Channel, wsmsg.ChannelExec)
	}
	// payload 是 map[string]any，转回 map 验证
	payloadBytes, _ := json.Marshal(msg.Payload)
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if payload["execution_id"] != "exec-123" {
		t.Errorf("execution_id = %v, want exec-123", payload["execution_id"])
	}
	if payload["chunk"] != "hello world chunk" {
		t.Errorf("chunk = %v, want 'hello world chunk'", payload["chunk"])
	}
	if payload["task_id"] != "task-456" {
		t.Errorf("task_id = %v, want task-456", payload["task_id"])
	}
}

// TestBroadcast_ChannelExecDone 验证 done 推送格式：{done:true, exit_code:N}
func TestBroadcast_ChannelExecDone(t *testing.T) {
	h := New()
	connCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		connCh <- c
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	c := <-connCh
	h.Register(c)
	defer h.Unregister(c)

	h.Broadcast(wsmsg.ChannelExec, map[string]any{
		"execution_id": "exec-789",
		"task_id":      "task-012",
		"done":         true,
		"exit_code":    0,
	})

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var msg wsmsg.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Channel != wsmsg.ChannelExec {
		t.Errorf("channel = %q, want %q", msg.Channel, wsmsg.ChannelExec)
	}
	payloadBytes, _ := json.Marshal(msg.Payload)
	var payload map[string]any
	_ = json.Unmarshal(payloadBytes, &payload)
	if payload["done"] != true {
		t.Errorf("done = %v, want true", payload["done"])
	}
	// exit_code 是 int 0，json 序列化为 0
	if payload["exit_code"] != float64(0) {
		t.Errorf("exit_code = %v, want 0", payload["exit_code"])
	}
}

// TestBroadcast_MultipleClients 验证多客户端都收到（这是 hub 的核心能力）
func TestBroadcast_MultipleClients(t *testing.T) {
	h := New()
	const nClients = 3
	conns := make([]*websocket.Conn, nClients)
	connCh := make(chan *websocket.Conn, nClients)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connCh <- c
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	for i := 0; i < nClients; i++ {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		conns[i] = c
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// 等所有 server-side conn 都到
	var wg sync.WaitGroup
	wg.Add(nClients)
	go func() {
		for i := 0; i < nClients; i++ {
			c := <-connCh
			h.Register(c)
			wg.Done()
		}
	}()
	wg.Wait()

	h.Broadcast(wsmsg.ChannelExec, map[string]any{
		"execution_id": "x", "chunk": "fanout",
	})

	// 每个客户端都应能读到
	for i, c := range conns {
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Errorf("client %d read: %v", i, err)
			continue
		}
		if !strings.Contains(string(data), "fanout") {
			t.Errorf("client %d got %s, missing 'fanout'", i, string(data))
		}
	}
}
