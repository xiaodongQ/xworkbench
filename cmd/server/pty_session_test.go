package main

import (
	"strings"
	"testing"
)

// TestDetermineAICmd_SessionParams verifies determineAICmd passes session/resume flags.
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
		{"claude with session_id (valid UUID)", "claude", validUUID, "", "--resume " + validUUID},
		{"claude with resume_uuid", "claude", "", validUUID, "--resume " + validUUID},
		// both: sessionID and resumeUUID both use --resume
		{"claude with both (both valid UUIDs)", "claude", validUUID, validUUID2, "--resume " + validUUID},
		{"cbc with session_id (valid UUID)", "cbc", validUUID, "", "--resume " + validUUID},
		// codex falls through to default claude
		{"codex no session", "codex", "", "", "claude"},
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

// TestEnrichCmd verifies enrichCmd adds --resume flags correctly.
// sessionID (scheduler last_session_id) and resumeUUID both use --resume flag.
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
		{"session only", "claude", validUUID, "", "claude --resume " + validUUID},
		{"resume only", "claude", "", validUUID, "claude --resume " + validUUID},
		// both: both get --resume (sessionID = last_session_id, resumeUUID = explicit resume)
		{"both", "claude", validUUID, validUUID2, "claude --resume " + validUUID + " --resume " + validUUID2},
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
