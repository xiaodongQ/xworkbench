package httplog

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func runWithHandler(t *testing.T, h http.HandlerFunc, req *http.Request) (string, int) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
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
	// GET → Debug (not Info)
	if !strings.Contains(out, `level=DEBUG`) || !strings.Contains(out, `status=200`) || !strings.Contains(out, `path=/api/tasks`) {
		t.Errorf("missing expected log attrs: %s", out)
	}
}
