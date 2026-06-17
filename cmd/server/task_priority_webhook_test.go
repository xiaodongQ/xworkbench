package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/webhook"
)

// priorityWebhookEvent 简化的事件结构（与 Dispatcher 实际 payload 一致）
type priorityWebhookEvent struct {
	Event     string         `json:"event"`
	Timestamp int64          `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
}

func newPriorityWebhookTestServer(t *testing.T) *APIServer {
	t.Helper()
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	whRepo := backend.NewWebhookRepo(db)
	return &APIServer{
		db:      backend.NewTaskRepo(db),
		setDB:   backend.NewAppSettingsRepo(db),
		dirDB:   backend.NewDirShortcutRepo(db),
		schedDB: backend.NewScheduledTaskRepo(db),
		execDB:  backend.NewExecutionRepo(db),
		evalDB:  backend.NewEvaluationRepo(db),
		eventDB: backend.NewTaskEventRepo(db),
		whDB:    whRepo,
		whDisp:  webhook.NewDispatcher(whRepo),
		cmtDB:   backend.NewTaskCommentRepo(db),
	}
}

// TestHandleTaskUpdate_PriorityChanged_DispatchesWebhook 验证：
// 当 PUT /api/tasks/{id} 修改 priority 时，dispatcher 收到 task.priority_changed 事件
func TestHandleTaskUpdate_PriorityChanged_DispatchesWebhook(t *testing.T) {
	s := newPriorityWebhookTestServer(t)

	// 1. 起一个 httptest server 当 webhook target
	var (
		mu   sync.Mutex
		got  priorityWebhookEvent
		hits int
	)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var ev priorityWebhookEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			t.Errorf("unmarshal webhook body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		got = ev
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	// 2. 注册 webhook 订阅 task.priority_changed
	wh := &backend.Webhook{
		Name:    "test-priority",
		URL:     target.URL,
		Events:  "task.priority_changed",
		Enabled: 1,
	}
	if err := s.whDB.Create(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	// 3. 创建一个 priority=5 的 task
	createBody := `{"title":"priority-test","task_type":"manual","priority":5}`
	createReq := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.handleTaskCreate(createW, createReq)
	if createW.Code != http.StatusOK {
		t.Fatalf("create task: code=%d body=%s", createW.Code, createW.Body.String())
	}
	var created backend.Task
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created task: %v", err)
	}
	if created.Priority != 5 {
		t.Fatalf("created task priority=%d, want 5", created.Priority)
	}

	// 4. PUT 修改 priority=10
	putBody := `{"title":"priority-test","priority":10}`
	putReq := httptest.NewRequest("PUT", "/api/tasks/"+created.ID, strings.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putW := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/tasks/{id}", s.handleTaskUpdate)
	mux.ServeHTTP(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("update task: code=%d body=%s", putW.Code, putW.Body.String())
	}
	var updated backend.Task
	if err := json.Unmarshal(putW.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated task: %v", err)
	}

	// 5. 轮询等待 webhook 触发（异步派发）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		h := hits
		mu.Unlock()
		if h >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits == 0 {
		t.Fatal("expected 1 webhook hit for task.priority_changed, got 0")
	}
	if got.Event != "task.priority_changed" {
		t.Errorf("event=%q, want task.priority_changed", got.Event)
	}
	if got.Payload["task_id"] != created.ID {
		t.Errorf("payload.task_id=%v, want %s", got.Payload["task_id"], created.ID)
	}
	// 旧/新 priority 断言（float64 是 JSON number decode 结果）
	if oldPri, _ := got.Payload["old"].(float64); int(oldPri) != 5 {
		t.Errorf("payload.old=%v, want 5", got.Payload["old"])
	}
	if newPri, _ := got.Payload["new"].(float64); int(newPri) != 10 {
		t.Errorf("payload.new=%v, want 10", got.Payload["new"])
	}
	if updated.Priority != 10 {
		t.Errorf("updated task priority=%d, want 10 (means handleTaskUpdate 不接受 priority 字段)", updated.Priority)
	}
}

// TestHandleTaskUpdate_PriorityUnchanged_NoWebhook 验证：
// priority 没变时，不应该触发 webhook（避免噪音）
func TestHandleTaskUpdate_PriorityUnchanged_NoWebhook(t *testing.T) {
	s := newPriorityWebhookTestServer(t)

	var (
		mu   sync.Mutex
		hits int
	)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	wh := &backend.Webhook{
		Name:    "test-priority-noop",
		URL:     target.URL,
		Events:  "task.priority_changed",
		Enabled: 1,
	}
	if err := s.whDB.Create(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	// 创建 priority=7
	createBody := `{"title":"p-noop","task_type":"manual","priority":7}`
	createReq := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.handleTaskCreate(createW, createReq)
	if createW.Code != http.StatusOK {
		t.Fatalf("create: %d", createW.Code)
	}
	var created backend.Task
	json.Unmarshal(createW.Body.Bytes(), &created)

	// PUT 同样 priority=7（同时改个别的字段比如 title）
	putBody := `{"title":"p-noop-renamed","priority":7}`
	putReq := httptest.NewRequest("PUT", "/api/tasks/"+created.ID, strings.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putW := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/tasks/{id}", s.handleTaskUpdate)
	mux.ServeHTTP(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("update: %d %s", putW.Code, putW.Body.String())
	}

	// 等 300ms 确认没有 webhook 触发
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if hits != 0 {
		t.Errorf("expected 0 webhook hits when priority unchanged, got %d", hits)
	}
}

