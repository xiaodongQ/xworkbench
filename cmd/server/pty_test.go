package main

import (
	"testing"
)

func TestDetectAuthRequired(t *testing.T) {
	tests := []struct {
		line  string
		want  bool
	}{
		{"Authorize this action: y/N", true},
		{"Do you want to proceed? Yes/No", true},
		{"Run command? [y/N]", true},
		{"CONFIRM: continue", true},
		{"continue anyway", true},
		{"Task completed successfully", false},
		{"Error: file not found", false},
		{"Select an option:", false},
		{"Permission denied", false},   // anti-pattern
		{"read permission on /tmp", false}, // anti-pattern
		{"", false},
	}
	for _, tt := range tests {
		got := detectAuthRequired(tt.line)
		if got != tt.want {
			t.Errorf("detectAuthRequired(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}
