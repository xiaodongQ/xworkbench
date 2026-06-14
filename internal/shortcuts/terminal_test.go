package shortcuts

import (
	"testing"
)

func TestParseTerminalType(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   TerminalType
		wantOk bool
	}{
		{"wezterm lower", "wezterm", TerminalWezterm, true},
		{"WezTerm mixed", "WezTerm", TerminalWezterm, true},
		{"WEZTERM upper", "WEZTERM", TerminalWezterm, true},
		{"wt", "wt", TerminalWindows, true},
		{"Windows", "Windows", TerminalWindows, true},
		{"windows terminal", "windows terminal", "", false},
		{"powershell", "powershell", TerminalPowerShell, true},
		{"ps", "ps", TerminalPowerShell, true},
		{"pwsh", "pwsh", TerminalSystemPS, true},
		{"pwsh7", "pwsh7", TerminalSystemPS, true},
		{"terminal", "terminal", TerminalMacOS, true},
		{"Terminal.app", "Terminal.app", TerminalMacOS, true},
		{"gnome", "gnome", TerminalGnome, true},
		{"xterm", "xterm", TerminalXTerm, true},
		{"cmd", "cmd", TerminalCmd, true},
		{"empty", "", "", false},
		{"unknown", "foobar", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseTerminalType(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ParseTerminalType(%q) ok=%v, want %v", tt.input, ok, tt.wantOk)
				return
			}
			if ok && got != tt.want {
				t.Errorf("ParseTerminalType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsSupportedTerminal(t *testing.T) {
	supported := []string{"wezterm", "wt", "powershell", "pwsh", "terminal", "gnome", "xterm", "cmd"}
	for _, s := range supported {
		if !IsSupportedTerminal(s) {
			t.Errorf("IsSupportedTerminal(%q) = false, want true", s)
		}
	}
	unsupported := []string{"", "foobar", "noterm", "konsole", "alacritty"}
	for _, s := range unsupported {
		if IsSupportedTerminal(s) {
			t.Errorf("IsSupportedTerminal(%q) = true, want false", s)
		}
	}
}

func TestParseSSHURL(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		wantU  string
		wantH  string
		wantP  string
		wantPt string
	}{
		{"ssh://user@host", "ssh://user@host", "user", "host", "", ""},
		{"ssh://user@host:22/path", "ssh://user@host:22/path", "user", "host", "", "/path"},
		{"ssh://user@host:2222/path", "ssh://user@host:2222/path", "user", "host", "2222", "/path"},
		{"ssh://host/path", "ssh://host/path", "", "host", "", "/path"},
		{"ssh://host", "ssh://host", "", "host", "", ""},
		{"user@host", "user@host", "user", "host", "", ""},
		{"user@host:/path", "user@host:/path", "user", "host", "", "/path"},
		{"user@host:2222/path", "user@host:2222/path", "user", "host", "2222", "/path"},
		{"user@host:2222", "user@host:2222", "user", "host", "2222", ""},
		{"host", "host", "", "host", "", ""},
		{"host/path", "host/path", "", "host", "", "/path"},
		{"user@host:/workspace", "user@host:/workspace", "user", "host", "", "/workspace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseSSHURL(tt.raw)
			if err != nil {
				t.Fatalf("ParseSSHURL(%q) error: %v", tt.raw, err)
			}
			if info.User != tt.wantU {
				t.Errorf("ParseSSHURL(%q).User = %q, want %q", tt.raw, info.User, tt.wantU)
			}
			if info.Host != tt.wantH {
				t.Errorf("ParseSSHURL(%q).Host = %q, want %q", tt.raw, info.Host, tt.wantH)
			}
			if info.Port != tt.wantP {
				t.Errorf("ParseSSHURL(%q).Port = %q, want %q", tt.raw, info.Port, tt.wantP)
			}
			if info.Path != tt.wantPt {
				t.Errorf("ParseSSHURL(%q).Path = %q, want %q", tt.raw, info.Path, tt.wantPt)
			}
		})
	}
}

func TestOpenTerminal_NotFound(t *testing.T) {
	err := OpenTerminal("nonexistent-terminal-xyz", "/tmp")
	if err == nil {
		t.Fatal("OpenTerminal with nonexistent type: expected error, got nil")
	}
}

func TestDefaultTerminal(t *testing.T) {
	dt := DefaultTerminal()
	if dt == "" {
		t.Fatal("DefaultTerminal() returned empty string")
	}
	if !IsSupportedTerminal(string(dt)) {
		t.Errorf("DefaultTerminal() = %q, which is not a supported terminal", dt)
	}
}

func TestIsAllDigits(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"2222", true},
		{"22", true},
		{"", false},
		{"22a", false},
		{"a22", false},
		{" 22", false},
	}
	for _, tt := range tests {
		got := isAllDigits(tt.s)
		if got != tt.want {
			t.Errorf("isAllDigits(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}