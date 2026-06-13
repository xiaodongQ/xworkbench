package httplog

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 捕获 slog 输出到 buffer，验证 200/404/500 三种 status 的等级与字段。
func runWithHandler(t *testing.T, h http.HandlerFunc, req *http.Request) (string, int) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	rec := httptest.NewRecorder()
	Middleware(h).ServeHTTP(rec, req)
	return buf.String(), rec.Code
}

func TestMiddleware_200_Info(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})
	out, code := runWithHandler(t, h, httptest.NewRequest("GET", "/api/tasks", nil))
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(out, `level=INFO`) || !strings.Contains(out, `status=200`) || !strings.Contains(out, `path=/api/tasks`) {
		t.Errorf("missing expected log attrs: %s", out)
	}
}

func TestMiddleware_404_Warn(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	_, code := runWithHandler(t, h, httptest.NewRequest("GET", "/missing", nil))
	if code != 404 {
		t.Fatalf("code = %d", code)
	}
}

func TestMiddleware_500_Error(t *testing.T) {
	out, _ := runWithHandler(t,
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		httptest.NewRequest("POST", "/x", nil),
	)
	if !strings.Contains(out, `level=ERROR`) || !strings.Contains(out, `status=500`) {
		t.Errorf("missing ERROR/500: %s", out)
	}
}

func TestMiddleware_DefaultStatusWhenNotWritten(t *testing.T) {
	// 不显式 WriteHeader 时，net/http 隐式写 200
	out, code := runWithHandler(t,
		func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hi")) },
		httptest.NewRequest("GET", "/implicit", nil),
	)
	if code != 200 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(out, `status=200`) {
		t.Errorf("expected status=200 from implicit: %s", out)
	}
}
