package runner

import (
	"runtime"
	"strings"
	"testing"
)

func TestBuildCommandClaude(t *testing.T) {
	got, cleanup, err := BuildCommand("claude", "sonnet", "sess-1", "解析 slowlog")
	if err != nil {
		t.Fatal(err)
	}
	if cleanup != nil {
		t.Errorf("cleanup should be nil for claude type")
	}
	want := []string{"claude", "-p", "--allowedTools", "Bash,Write,Edit,Read,Grep", "--output-format", "json", "--model", "sonnet", "--session-id", "sess-1", "解析 slowlog"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildCommandCbc(t *testing.T) {
	got, cleanup, err := BuildCommand("cbc", "opus", "", "写一个 hello world")
	if err != nil {
		// PATH 中可能没有 cbc/codebuddy，跳过
		t.Skip("cbc/codebuddy not in PATH:", err)
	}
	if cleanup != nil {
		t.Errorf("cleanup should be nil for cbc type")
	}
	// 至少验证第一项是 cbc 或 codebuddy
	if got[0] != "cbc" && got[0] != "codebuddy" {
		t.Errorf("got[0] = %q, want cbc or codebuddy", got[0])
	}
	if got[1] != "-p" {
		t.Errorf("got[1] = %q, want -p", got[1])
	}
}

func TestBuildCommandShell(t *testing.T) {
	got, cleanup, err := BuildCommand("shell", "", "", "echo hi")
	if err != nil {
		t.Fatal(err)
	}
	if cleanup == nil {
		t.Fatalf("cleanup should not be nil for shell type")
	}
	defer cleanup()
	// 不再用 sh -c 形式 — 改用临时文件避免 shell 注入。
	if runtime.GOOS == "windows" {
		if got[0] != "powershell.exe" || got[1] != "-NoProfile" || got[2] != "-NonInteractive" || got[3] != "-File" {
			t.Errorf("windows shell cmd shape: %v", got)
		}
	} else {
		if got[0] != "sh" || got[1] == "-c" {
			t.Errorf("unix shell cmd should not use -c: %v", got)
		}
	}
}

func TestBuildCommandUnknown(t *testing.T) {
	if _, _, err := BuildCommand("nonsense", "", "", "x"); err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestBuildCommandClaudeWithActionReport(t *testing.T) {
	got, _, err := BuildCommand("claude", "haiku", "", "用 osascript 通知我", WithActionReport())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("cmd too short: %v", got)
	}
	// 最后一个元素应该包含原 prompt + 动作清单后缀
	last := got[len(got)-1]
	if !strings.Contains(last, "用 osascript 通知我") {
		t.Errorf("missing original prompt in: %s", last)
	}
	if !strings.Contains(last, "## 动作清单") {
		t.Errorf("missing action report suffix in: %s", last)
	}
}

func TestBuildCommandShellNoActionReport(t *testing.T) {
	got, cleanup, err := BuildCommand("shell", "", "", "echo hi", WithActionReport())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer cleanup()
	// shell 类型不加动作清单；命令 argv 不直接含 echo hi（它在临时文件里）
	if runtime.GOOS == "windows" {
		if strings.Join(got, " ") == "sh -c echo hi" {
			t.Errorf("shell cmd shouldn't be sh -c form: %v", got)
		}
	} else {
		if strings.Join(got, " ") == "sh -c echo hi" {
			t.Errorf("shell cmd shouldn't be sh -c form: %v", got)
		}
	}
}
