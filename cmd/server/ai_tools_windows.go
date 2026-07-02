//go:build windows

package main

import (
	"context"
)

// GetTools returns an empty slice on Windows.
func GetTools() []Tool { return nil }

// ExecuteTool returns an error message on Windows.
func ExecuteTool(ctx context.Context, db, expDB, execDB, agentDB interface{}, localShell interface{}, toolName, argsJSON string) string {
	return "AI tools not supported on Windows"
}

// LocalShellState stub for Windows.
type LocalShellState struct{ Active bool }
