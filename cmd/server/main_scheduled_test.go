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
	if err := s.sch.Reload(); err != nil {
		t.Fatalf("sch.Reload: %v", err)
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

// TestHandleScheduledList_NextRunAt_Disabled 验证 disabled 任务的 next_run_at 为 nil（字段不出现）。
func TestHandleScheduledList_NextRunAt_Disabled(t *testing.T) {
	s := newTestServer(t)
	disabled := &backend.ScheduledTask{
		ID:          "sched-disabled-1",
		Name:        "已禁用",
		CronExpr:    "*/5 * * * *",
		CommandType: "shell",
		Enabled:     false,
		TimeoutSec:  60,
		CreatedAt:   time.Now(),
	}
	if err := s.schedDB.Create(disabled); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.sch.Reload(); err != nil {
		t.Fatalf("sch.Reload: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/scheduled", s.handleScheduledList)
	req := httptest.NewRequest("GET", "/api/scheduled", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// 用 map 解码以验证字段是否真的"不出现"（omitempty 行为）
	var list []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}
	if _, ok := list[0]["next_run_at"]; ok {
		t.Errorf("next_run_at should be omitted for disabled task; got %v", list[0]["next_run_at"])
	}
}

// TestHandleScheduledList_NextRunAt_InvalidCron 验证非法 cron 不阻断整列表。
func TestHandleScheduledList_NextRunAt_InvalidCron(t *testing.T) {
	s := newTestServer(t)
	now := time.Now()
	bad := &backend.ScheduledTask{
		ID:          "sched-bad-1",
		Name:        "非法 cron",
		CronExpr:    "not a cron",
		CommandType: "shell",
		Enabled:     true,
		TimeoutSec:  60,
		CreatedAt:   now,
	}
	good := &backend.ScheduledTask{
		ID:          "sched-good-1",
		Name:        "合法 cron",
		CronExpr:    "0 9 * * 1-5",
		CommandType: "shell",
		Enabled:     true,
		TimeoutSec:  60,
		CreatedAt:   now,
	}
	if err := s.schedDB.Create(bad); err != nil {
		t.Fatalf("Create bad: %v", err)
	}
	if err := s.schedDB.Create(good); err != nil {
		t.Fatalf("Create good: %v", err)
	}
	if err := s.sch.Reload(); err != nil {
		t.Fatalf("sch.Reload: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/scheduled", s.handleScheduledList)
	req := httptest.NewRequest("GET", "/api/scheduled", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var list []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}

	// bad 应该没有 next_run_at；good 应该有
	for _, item := range list {
		id, _ := item["id"].(string)
		_, has := item["next_run_at"]
		switch id {
		case "sched-bad-1":
			if has {
				t.Errorf("bad task should not have next_run_at; got %v", item["next_run_at"])
			}
		case "sched-good-1":
			if !has {
				t.Errorf("good task should have next_run_at")
			}
		}
	}
}

// TestHandleScheduledList_NextRunAt_EveryDescriptor 验证 @every 描述符被识别。
func TestHandleScheduledList_NextRunAt_EveryDescriptor(t *testing.T) {
	s := newTestServer(t)
	task := &backend.ScheduledTask{
		ID:          "sched-every-1",
		Name:        "每 30 秒",
		CronExpr:    "@every 30s",
		CommandType: "shell",
		Enabled:     true,
		TimeoutSec:  10,
		CreatedAt:   time.Now(),
	}
	if err := s.schedDB.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.sch.Reload(); err != nil {
		t.Fatalf("sch.Reload: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/scheduled", s.handleScheduledList)
	req := httptest.NewRequest("GET", "/api/scheduled", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var list []*backend.ScheduledTask
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}
	if list[0].NextRunAt == nil {
		t.Fatalf("NextRunAt is nil; @every should be parsed")
	}
	// 期望 30 秒 + 误差
	if d := time.Until(*list[0].NextRunAt); d < 25*time.Second || d > 35*time.Second {
		t.Errorf("NextRunAt in %v; want ~30s", d)
	}
}
