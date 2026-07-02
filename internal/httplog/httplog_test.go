package httplog

import (
	"bufio"
	"net"
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

// hijackableResponseWriter 真正实现 http.Hijacker 接口。
// 用于测试中间件是否包装了 ResponseWriter 而丢失 Hijacker 能力。
type hijackableResponseWriter struct {
	header http.Header
}

func (h *hijackableResponseWriter) Header() http.Header { return h.header }
func (h *hijackableResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}
func (h *hijackableResponseWriter) WriteHeader(code int) {}
func (h *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

// TestMiddleware_WebSocketPaths_PreserveHijacker 验证 WebSocket 升级路径
//（/api/pty、/ws、/api/rpty）的 ResponseWriter 不被 statusRecorder 包装，
// 否则下游 gorilla/websocket Upgrade 会因 "response does not implement http.Hijacker" 失败。
// 回归保护：2026-07 commit 18d8e2c 新增 /api/rpty 路由时漏改 httplog 白名单，导致远端 PTY 连不上。
func TestMiddleware_WebSocketPaths_PreserveHijacker(t *testing.T) {
	wsPaths := []string{"/api/pty", "/ws", "/api/rpty"}
	for _, path := range wsPaths {
		t.Run(path, func(t *testing.T) {
			hijackable := &hijackableResponseWriter{header: http.Header{}}

			var wrapped bool
			var hijackOK bool
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, wrapped = w.(*statusRecorder)
				_, hijackOK = w.(http.Hijacker)
			})

			req := httptest.NewRequest("GET", path, nil)
			Middleware(h, nil).ServeHTTP(hijackable, req)

			if wrapped {
				t.Errorf("path %q: ResponseWriter was wrapped by statusRecorder, must pass through", path)
			}
			if !hijackOK {
				t.Errorf("path %q: ResponseWriter does not implement http.Hijacker, websocket upgrade will fail", path)
			}
		})
	}
}

// TestMiddleware_NonWebSocketPath_Wraps 验证非 WebSocket 路径仍被 statusRecorder 包装
// （保证常规 HTTP 日志中间件行为不变）。
func TestMiddleware_NonWebSocketPath_Wraps(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(*statusRecorder); !ok {
			t.Error("non-WS path should be wrapped by statusRecorder")
		}
		w.WriteHeader(200)
	})
	Middleware(h, nil).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/tasks", nil))
}
