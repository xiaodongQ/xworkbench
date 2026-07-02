// Package httplog 提供一个记录 HTTP handler 进出日志的中间件。
//
// 字段：method、path、status、dur_ms。
// 等级：GET → Debug；5xx → Error；4xx → Warn；其余 → Info。
//
// logger 参数采用依赖注入：调用方传 zap logger，测试可传自己的 observer。
// 这样跟项目其他模块一样统一用 zap，不会被 stdlib slog 拦截干抗。
package httplog

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Middleware 包装 next，记录每次请求的处理结果。
// logger 传 nil 时中间件为 no-op（不记录日志，避免 nil 崩溃）。
// 对于需要 WebSocket 升级的路径（/api/pty、/ws、/api/rpty），直接放行以避免
// statusRecorder 包装导致 http.Hijacker 接口无法通过检查。
func Middleware(next http.Handler, logger *zap.SugaredLogger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WebSocket 升级路径直接放行，不包装 statusRecorder
		if r.URL.Path == "/api/pty" || r.URL.Path == "/ws" || r.URL.Path == "/api/rpty" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		if logger == nil {
			return
		}
		switch {
		case rw.status >= 500:
			logger.Errorw("http", "method", r.Method, "path", r.URL.Path, "status", rw.status, "dur_ms", time.Since(start).Milliseconds())
		case rw.status >= 400:
			logger.Warnw("http", "method", r.Method, "path", r.URL.Path, "status", rw.status, "dur_ms", time.Since(start).Milliseconds())
		case r.Method == http.MethodGet:
			logger.Debugw("http", "method", r.Method, "path", r.URL.Path, "status", rw.status, "dur_ms", time.Since(start).Milliseconds())
		default:
			logger.Infow("http", "method", r.Method, "path", r.URL.Path, "status", rw.status, "dur_ms", time.Since(start).Milliseconds())
		}
	})
}

// statusRecorder 包装 ResponseWriter 以便捕获下游写出的 status code。
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		// Write 不显式调 WriteHeader 时，net/http 会自动以 200 写入
		r.wroteHeader = true
	}
	return r.ResponseWriter.Write(b)
}
