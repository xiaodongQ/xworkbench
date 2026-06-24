package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
)

// TestHandleScheduledList_NextRunAt_Enabled 验证 enabled 任务的 next_run_at 被注入且为未来时间。
func TestHandleScheduledList_NextRunAt_Enabled(t *testing.T) {
	s := newTestServer(t)
	now := time.Now()
	enabled := &backend.ScheduledTask{
		ID:          "sched-enabled-1",
		Name:        "每 5 分一次",
		CronExpr:    "*/5 * * * *",
		CommandType: "shell",
		Enabled:     true,
		TimeoutSec:  60,
		CreatedAt:   now,
	}
	if err := s.schedDB.Create(enabled); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/scheduled", s.handleScheduledList)
	req := httptest.NewRequest("GET", "/api/scheduled", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var list []*backend.ScheduledTask
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	got := list[0]
	if got.NextRunAt == nil {
		t.Fatalf("NextRunAt is nil; want non-nil for enabled task")
	}
	// 期望在 6 分钟以内（5 分 + 误差）
	if d := time.Until(*got.NextRunAt); d < 0 || d > 6*time.Minute {
		t.Errorf("NextRunAt in %v; want within 0..6min from now", d)
	}
}
