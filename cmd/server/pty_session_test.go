package main

import (
	"strings"
	"testing"
)

// TestDetermineAICmd_SessionParams verifies determineAICmd passes resume flags correctly.
// sessionID is PTY tab ID (not AI session), only resumeUUID triggers --resume.
func TestDetermineAICmd_SessionParams(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	validUUID2 := "660e8400-e29b-41d4-a716-446655440001"
	tests := []struct {
		name        string
		cliType     string
		sessionID   string
		resumeUUID  string
		wantContain string
	}{
		{"claude no session", "claude", "", "", "claude"},
		// sessionID is tab ID, not AI session - should NOT add --resume
		{"claude with session_id (tab ID, not AI session)", "claude", validUUID, "", "claude"},
		{"claude with resume_uuid", "claude", "", validUUID, "--resume " + validUUID},
		// both: only resumeUUID is used (sessionID is ignored for --resume)
		{"claude with both (only resumeUUID used)", "claude", validUUID, validUUID2, "--resume " + validUUID2},
		// sessionID is tab ID - should NOT add --resume for cbc either
		{"cbc with session_id (tab ID, not AI session)", "cbc", validUUID, "", "cbc"},
		// codex falls through to default claude
		{"codex no session", "codex", "", "", "claude"},
		// shell returns "" → caller uses exec.Command(shell, "-i") for interactive shell
		{"shell no session", "shell", "", "", ""},
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

// TestEnrichCmd verifies enrichCmd adds --resume flags correctly.
// sessionID is PTY tab ID (not AI session), only resumeUUID triggers --resume.
func TestEnrichCmd(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	validUUID2 := "660e8400-e29b-41d4-a716-446655440001"
	tests := []struct {
		name       string
		cmd        string
		sessionID  string
		resumeUUID string
		want       string
	}{
		{"empty both", "claude", "", "", "claude"},
		{"session only (tab ID, not AI session)", "claude", validUUID, "", "claude"},
		{"resume only", "claude", "", validUUID, "claude --resume " + validUUID},
		{"both (only resumeUUID used)", "claude", validUUID, validUUID2, "claude --resume " + validUUID2},
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
