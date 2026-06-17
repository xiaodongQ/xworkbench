package httplog

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	loglib "github.com/xiaodongQ/xworkbench/internal/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// runWithHandler 跑 middleware 并捕获 zap 输出。
// 中间件用的是 zap（loglib.Logger），不是 slog.Default()，所以必须注入 zap logger 才能读到。
func runWithHandler(t *testing.T, h http.HandlerFunc, req *http.Request) (string, int) {
	t.Helper()
	var buf bytes.Buffer
	prev := loglib.Logger
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "" // 测试不关心时间戳
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encCfg),
		zapcore.AddSync(&buf),
		zapcore.DebugLevel,
	)
	loglib.Set(zap.New(core).Sugar())
	t.Cleanup(func() { loglib.Set(prev) })

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
	// ConsoleEncoder 格式：`<level>	<msg>	<json-fields>`，所以字段都在 json block 里。
	if !strings.HasPrefix(out, "debug\t") {
		t.Errorf("expected level=debug prefix, got: %s", out)
	}
	if !strings.Contains(out, `"path": "/api/tasks"`) || !strings.Contains(out, `"status": 200`) {
		t.Errorf("missing expected log attrs: %s", out)
	}
}
