package scheduler

import (
	"runtime"
	"testing"
	"time"
)

// TestLoadLocalLocation 跨平台验证 time.LoadLocation("Local") 能工作。
// Go 1.15+ 内嵌 tzdata，Windows 上 Local 也应能成功解析（通过 tzdata 兜底）。
// 这个测试在交叉编译时只验证编译通过；运行时断言由用户在 Windows 上跑一次。
func TestLoadLocalLocation(t *testing.T) {
	loc, err := time.LoadLocation("Local")
	if err != nil {
		t.Fatalf("time.LoadLocation(\"Local\") on %s: %v", runtime.GOOS, err)
	}
	if loc == nil {
		t.Fatalf("nil location on %s", runtime.GOOS)
	}
	now := time.Now().In(loc)
	if now.Location().String() == "" {
		t.Errorf("location string is empty")
	}
}
