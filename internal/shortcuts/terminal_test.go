package shortcuts

import (
	"testing"
)

func TestIsSupportedTerminal(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"wezterm", true},
		{"WezTerm", true},
		{"WEZTERM", true},
		{"wt", true},
		{"powershell", true},
		{"pwsh", true},
		{"pwsh7", true},
		{"terminal", true},
		{"gnome", true},
		{"xterm", true},
		{"cmd", true},
		{"", false},
		{"foobar", false},
		{"windows terminal", false},
	}
	for _, tt := range tests {
		got := IsSupportedTerminal(tt.input)
		if got != tt.want {
			t.Errorf("IsSupportedTerminal(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestDefaultTerminal(t *testing.T) {
	got := DefaultTerminal()
	if got == "" {
		t.Error("DefaultTerminal() returned empty string")
	}
	if !IsSupportedTerminal(got) {
		t.Errorf("DefaultTerminal() returned unsupported type %q", got)
	}
}

func TestParseSSHURL(t *testing.T) {
	tests := []struct {
		input      string
		wantUser   string
		wantHost   string
		wantPort   string
		wantPath   string
		wantErr    bool
	}{
		// ssh:// URL 形式
		{"ssh://user@host/path", "user", "host", "", "/path", false},
		{"ssh://host/path", "", "host", "", "/path", false},
		{"ssh://host", "", "host", "", "", false},
		// user@host 形式
		{"user@host:/path", "user", "host", "", "/path", false},
		{"user@host", "user", "host", "", "", false},
		// host/path 形式（无 user）
		{"host/path", "", "host", "", "/path", false},
		// host:/path — 只有一个冒号，后面是路径分隔符，Host 包含冒号（ParseSSHURL 行为）
		{"host:/path", "", "host:", "", "/path", false},
	}
	for _, tt := range tests {
		info, err := ParseSSHURL(tt.input)
		if tt.wantErr && err == nil {
			t.Errorf("ParseSSHURL(%q) succeeded, want error", tt.input)
			continue
		}
		if !tt.wantErr && err != nil {
			t.Errorf("ParseSSHURL(%q) returned error %v, want success", tt.input, err)
			continue
		}
		if !tt.wantErr {
			if info.User != tt.wantUser {
				t.Errorf("ParseSSHURL(%q).User = %q, want %q", tt.input, info.User, tt.wantUser)
			}
			if info.Host != tt.wantHost {
				t.Errorf("ParseSSHURL(%q).Host = %q, want %q", tt.input, info.Host, tt.wantHost)
			}
			if info.Port != tt.wantPort {
				t.Errorf("ParseSSHURL(%q).Port = %q, want %q", tt.input, info.Port, tt.wantPort)
			}
			if info.Path != tt.wantPath {
				t.Errorf("ParseSSHURL(%q).Path = %q, want %q", tt.input, info.Path, tt.wantPath)
			}
		}
	}
}

func TestOpenTerminal_NotFound(t *testing.T) {
	err := OpenTerminal("nonexistent_terminal_xyz", "/tmp", "")
	if err == nil {
		t.Error("OpenTerminal with nonexistent terminal type succeeded, want error")
	}
}