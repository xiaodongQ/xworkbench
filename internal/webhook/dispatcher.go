// Package webhook 实现异步事件分发到外部 URL。
// 借鉴 GitHub webhooks：HMAC-SHA256 签名 + 失败重试 + 事件过滤。
package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
)

type Dispatcher struct {
	repo *backend.WebhookRepo
	cli  *http.Client
}

func NewDispatcher(repo *backend.WebhookRepo) *Dispatcher {
	return &Dispatcher{
		repo: repo,
		cli:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Dispatch 异步派发事件。事件会被发送到所有 enabled 且匹配 events 列表的 webhook。
// 重试 3 次（指数退避），不阻塞调用方。
func (d *Dispatcher) Dispatch(eventType string, payload any) {
	go d.deliver(eventType, payload)
}

func (d *Dispatcher) deliver(eventType string, payload any) {
	hooks, err := d.repo.ListEnabled()
	if err != nil {
		slog.Error("webhook list failed", "err", err)
		return
	}
	for _, h := range hooks {
		if !matchesEvent(h.Events, eventType) {
			continue
		}
		d.deliverOne(h, eventType, payload)
	}
}

func (d *Dispatcher) deliverOne(h *backend.Webhook, eventType string, payload any) {
	body, err := json.Marshal(map[string]any{
		"event":     eventType,
		"timestamp": time.Now().Unix(),
		"payload":   payload,
	})
	if err != nil {
		slog.Error("webhook marshal failed", "err", err)
		return
	}
	// 3 次重试，指数退避
	for attempt := 1; attempt <= 3; attempt++ {
		err := d.sendOnce(h, eventType, body)
		if err == nil {
			d.repo.MarkTriggered(h.ID)
			slog.Info("webhook delivered", "id", h.ID, "event", eventType, "attempt", attempt)
			return
		}
		slog.Warn("webhook attempt failed", "id", h.ID, "event", eventType, "attempt", attempt, "err", err)
		time.Sleep(time.Duration(1<<attempt) * time.Second)
	}
	d.repo.IncrementFail(h.ID)
	slog.Error("webhook permanently failed", "id", h.ID, "event", eventType)
}

func (d *Dispatcher) sendOnce(h *backend.Webhook, eventType string, body []byte) error {
	req, err := http.NewRequest("POST", h.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Id", h.ID)
	req.Header.Set("X-Event-Type", eventType)
	if h.Secret != "" {
		mac := hmac.New(sha256.New, []byte(h.Secret))
		mac.Write(body)
		req.Header.Set("X-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := d.cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return &httpError{status: resp.StatusCode, msg: "non-2xx response"}
}

type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string {
	return e.msg + " (status=" + strconv.Itoa(e.status) + ")"
}

// matchesEvent 检查 events 列表（逗号分隔）是否包含 eventType。
// 空 events = 全部匹配。
func matchesEvent(events, eventType string) bool {
	if events == "" {
		return true
	}
	for _, e := range strings.Split(events, ",") {
		if strings.TrimSpace(e) == eventType {
			return true
		}
	}
	return false
}