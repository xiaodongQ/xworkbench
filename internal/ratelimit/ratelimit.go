// Package ratelimit 提供 token-bucket 速率限制。
// 用于保护 agent API 防止被刷或陷入循环。
package ratelimit

import (
	"net/http"
	"sync"
	"time"
)

// Limiter token-bucket per key。
// 容量 = rate，refill 速率 = rate / 60s。
// 内存维护，重启即清空（best-effort 防护）。
type Limiter struct {
	ratePerMin int
	mu         sync.Mutex
	buckets    map[string]*bucket
}

type bucket struct {
	tokens  int
	updated time.Time
}

func New(ratePerMin int) *Limiter {
	if ratePerMin <= 0 {
		return nil
	}
	return &Limiter{
		ratePerMin: ratePerMin,
		buckets:    map[string]*bucket{},
	}
}

// Allow 消费一个 token。返回 (true, retryAfter=0) 表示允许；返回 (false, retryAfter) 表示被限。
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: l.ratePerMin - 1, updated: now}
		return true, 0
	}
	// refill：按 (now - updated) 累加
	elapsed := now.Sub(b.updated)
	refill := int(elapsed.Minutes() * float64(l.ratePerMin))
	if refill > 0 {
		b.tokens += refill
		if b.tokens > l.ratePerMin {
			b.tokens = l.ratePerMin
		}
		b.updated = now
	}
	if b.tokens <= 0 {
		// 还需要等待 1/refill_per_sec 秒
		need := 1.0 / float64(l.ratePerMin) * 60.0
		return false, time.Duration(need*float64(time.Second))
	}
	b.tokens--
	return true, 0
}

// Middleware 返回一个标准中间件，handler 在 X-Agent-Id 或 Authorization Bearer 头
// 提取 key。key 缺失的请求不限制。
func (l *Limiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if l == nil {
				next.ServeHTTP(w, r)
				return
			}
			key := extractKey(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			ok, retry := l.Allow(key)
			if !ok {
				w.Header().Set("Retry-After", retry.Round(time.Second).String())
				w.Header().Set("X-RateLimit-Limit", "60")
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractKey 从 request 提取速率限制 key。
// 优先用 Authorization 头（agent token），其次 IP。
func extractKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 {
		return auth[7:] // 跳过 "Bearer "
	}
	return r.RemoteAddr
}

// 为了避免 import 循环：保留此文件无依赖