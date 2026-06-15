package ratelimit

import (
	"testing"
	"time"
)

// TestLimiterAllow 验证 token-bucket 限制：rate 以内的请求通过，超限被拒。
func TestLimiterAllow(t *testing.T) {
	l := New(5) // 5 req/min
	key := "test-key"

	// 前 5 个通过
	for i := 0; i < 5; i++ {
		ok, _ := l.Allow(key)
		if !ok {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	// 第 6 个应被拒
	ok, retry := l.Allow(key)
	if ok {
		t.Errorf("6th request should be rate limited")
	}
	if retry == 0 {
		t.Errorf("retry should be > 0, got %v", retry)
	}
}

// TestLimiterDifferentKeys 验证不同 key 互不影响。
func TestLimiterDifferentKeys(t *testing.T) {
	l := New(2)
	// key1 消耗光
	l.Allow("key1")
	l.Allow("key1")
	ok, _ := l.Allow("key1")
	if ok { t.Errorf("key1 should be limited") }
	// key2 仍能使用
	ok, _ = l.Allow("key2")
	if !ok { t.Errorf("key2 should be allowed") }
}

// TestNilLimiter 验证 nil limiter 永远允许（用于禁用场景）。
func TestNilLimiter(t *testing.T) {
	var l *Limiter
	ok, _ := l.Allow("any")
	if !ok { t.Errorf("nil limiter should always allow") }
}

// TestLimiterRefill 验证 token 随时间 refill。
func TestLimiterRefill(t *testing.T) {
	l := New(60) // 1 token per second
	key := "refill-key"
	// 消耗
	for i := 0; i < 60; i++ { l.Allow(key) }
	ok, _ := l.Allow(key)
	if ok { t.Errorf("should be limited after 60 calls") }
	// 等待 1.5s 应该 refill 1 个
	time.Sleep(1500 * time.Millisecond)
	ok, _ = l.Allow(key)
	if !ok { t.Errorf("should be allowed after refill (got: %v)", ok) }
}