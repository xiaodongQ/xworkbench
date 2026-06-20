package httplog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// runWithHandler 跑一次 middleware，用 zap observer 拦截日志（保持与项目其他模块一致的日志系统）。
func runWithHandler(t *testing.T, h http.HandlerFunc, req *http.Request) ([]observer.LoggedEntry, int) {
	t.Helper()
	core, recorded := observer.New(zapcore.DebugLevel)
	testLogger := zap.New(core).Sugar()

	rec := httptest.NewRecorder()
	Middleware(h, testLogger).ServeHTTP(rec, req)
	return recorded.All(), rec.Code
}

// recordedToString 把 observer 记录的所有 entry 转成可断言的字符串（用 "msg key=val key=val" 形式）。
func recordedToString(entries []observer.LoggedEntry) string {
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(e.Message)
		for _, f := range e.Context {
			sb.WriteString(" " + f.Key + "=" + f.String)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func TestMiddleware_200_Info(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})
	entries, code := runWithHandler(t, h, httptest.NewRequest("GET", "/api/tasks", nil))
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one log entry")
	}
	text := recordedToString(entries)
	// GET → Debug (not Info)
	if !strings.Contains(text, "http method=GET") || !strings.Contains(text, "path=/api/tasks") {
		t.Errorf("missing expected log attrs: %s", text)
	}
	// 验证 status 字段是 200（int 字段在 observer.String 里是空，需走 Context 原生字段）
	if len(entries) > 0 {
		var gotStatus int64
		for _, f := range entries[0].Context {
			if f.Key == "status" {
				gotStatus = f.Integer
			}
		}
		if gotStatus != 200 {
			t.Errorf("status = %d, want 200", gotStatus)
		}
	}
	for _, e := range entries {
		if e.Level != zapcore.DebugLevel {
			t.Errorf("GET should be Debug level, got %v", e.Level)
		}
	}
}

func TestMiddleware_NilLogger(t *testing.T) {
	// 传 nil logger 不应 panic（no-op 模式）。
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	rec := httptest.NewRecorder()
	Middleware(h, nil).ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != 200 {
		t.Errorf("code = %d, want 200", rec.Code)
	}
}
