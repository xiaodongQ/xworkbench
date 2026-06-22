package executor

import (
	"strings"
	"testing"
)

func TestQuoteArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"no special", []string{"echo", "hello"}, "echo hello"},
		{"single space", []string{"echo", "hello world"}, "echo 'hello world'"},
		{"single quote", []string{"echo", "it's"}, `echo 'it'\''s'`},
		{"dollar", []string{"echo", "$HOME"}, "echo '$HOME'"},
		{"semicolon", []string{"cmd", "a;b"}, "cmd 'a;b'"},
		{"empty arg", []string{"cmd", ""}, "cmd ''"},
		{"already quoted", []string{`"x"`, "y"}, `'"x"' y`},
		{"backslash", []string{`a\b`}, `'a\b'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(quoteArgs(tt.in), " ")
			if got != tt.want {
				t.Errorf("quoteArgs(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStreamLines(t *testing.T) {
	var collected []string
	var builder strings.Builder
	streamLines(strings.NewReader("line1\nline2\nline3\n"), false, func(s string) {
		collected = append(collected, s)
	}, &builder)
	if len(collected) != 3 {
		t.Errorf("expected 3 chunks, got %d", len(collected))
	}
	if collected[0] != "line1\n" {
		t.Errorf("first chunk = %q, want %q", collected[0], "line1\n")
	}
	if builder.String() != "line1\nline2\nline3\n" {
		t.Errorf("builder = %q", builder.String())
	}
}

func TestStreamLinesWithErrPrefix(t *testing.T) {
	var builder strings.Builder
	streamLines(strings.NewReader("err1\nerr2\n"), true, nil, &builder)
	got := builder.String()
	if !strings.Contains(got, "[err] err1") {
		t.Errorf("expected '[err] err1' prefix, got %q", got)
	}
}

func TestEnsureRemoteBinaryMock(t *testing.T) {
	// 这函数要真 ssh client 才能跑，单元测试跳过。
	// 端到端测试在 ⑤ 阶段用本地 sshd + socat 验证。
	t.Skip("requires live ssh client; covered by integration test in step 5")
}
