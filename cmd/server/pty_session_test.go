package main

import (
	"strings"
	"testing"
)

// TestDetermineAICmd_SessionParams verifies determineAICmd passes session/resume flags.
func TestDetermineAICmd_SessionParams(t *testing.T) {
	tests := []struct {
		name       string
		cliType    string
		sessionID  string
		resumeUUID string
		wantContain string
	}{
		{"claude no session", "claude", "", "", "claude"},
		{"claude with session_id", "claude", "my-session-123", "", "--session-id my-session-123"},
		{"claude with resume_uuid", "claude", "", "resume-abc", "--resume resume-abc"},
		{"claude with both", "claude", "my-session-123", "resume-abc", "--session-id my-session-123"},
		{"cbc with session_id", "cbc", "cbc-session", "", "--session-id cbc-session"},
		{"codex no session", "codex", "", "", "codex"},
		{"shell no session", "shell", "", "", "sh"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineAICmd(tt.cliType, "/tmp", tt.sessionID, tt.resumeUUID)
			if tt.wantContain != "" && !contains(got, tt.wantContain) {
				t.Errorf("determineAICmd(%q,,%q,%q) = %q, want containing %q", tt.cliType, tt.sessionID, tt.resumeUUID, got, tt.wantContain)
			}
		})
	}
}

// TestEnrichCmd verifies enrichCmd adds flags correctly.
func TestEnrichCmd(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		sessionID  string
		resumeUUID string
		want       string
	}{
		{"empty both", "claude", "", "", "claude"},
		{"session only", "claude", "sid", "", "claude --session-id sid"},
		{"resume only", "claude", "", "rid", "claude --resume rid"},
		{"both", "claude", "sid", "rid", "claude --session-id sid --resume rid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enrichCmd(tt.cmd, tt.sessionID, tt.resumeUUID)
			if got != tt.want {
				t.Errorf("enrichCmd(%q,%q,%q) = %q, want %q", tt.cmd, tt.sessionID, tt.resumeUUID, got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool { return strings.Contains(s, substr) }