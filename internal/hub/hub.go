// Package hub 是 WebSocket 广播中心，6 个频道共享一个连接池。
package hub

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/xiaodongQ/xworkbench/internal/wsmsg"
	"github.com/xiaodongQ/xworkbench/internal/logger"
)



// Hub 维护所有客户端连接，提供频道广播。
type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

func New() *Hub {
	return &Hub{clients: map[*websocket.Conn]struct{}{}}
}

// Register 把新连接加入 hub。
func (h *Hub) Register(c *websocket.Conn) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

// Unregister 移除连接（无需锁外 close）。
func (h *Hub) Unregister(c *websocket.Conn) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		c.Close()
	}
	h.mu.Unlock()
}

// Broadcast 向所有客户端推消息（不做背压；网络/慢客户端会被踢）。
func (h *Hub) Broadcast(channel string, payload any) {
	msg := wsmsg.Message{Channel: channel, Payload: payload}
	data, err := json.Marshal(msg)
	if err != nil {
		logger.Logger.Errorf("hub marshal: %v", err)
		return
	}
	h.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		conns = append(conns, c)
	}
	h.mu.RUnlock()
	for _, c := range conns {
		if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
			h.Unregister(c)
		}
	}
}

// Count 返回当前连接数（便于 stats/debug）。
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
