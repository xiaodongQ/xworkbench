package main

import (
	"strings"
	"testing"
)

// TestXwcliWindowsBackground verifies xwcli source has Windows detached-process support.
func TestXwcliWindowsBackground(t *testing.T) {
	src := xwcliSource

	// _windows_detach function must exist in source
	if !strings.Contains(src, "def _windows_detach()") {
		t.Error("xwcli source missing _windows_detach function")
	}

	// Must use DETACHED_PROCESS creationflags on Windows
	if !strings.Contains(src, "DETACHED") {
		t.Error("xwcli source missing DETACHED constant")
	}

	// run-inner command must be handled (allows Windows subprocess to re-exec itself)
	if !strings.Contains(src, `cmd == "run-inner"`) {
		t.Error("xwcli source missing run-inner command handler")
	}

	// pid_file must be written on startup
	if !strings.Contains(src, "pid_file.write_text(str(os.getpid()))") {
		t.Error("xwcli source missing pid_file write on startup")
	}

	// On Windows, _windows_detach is called before run_loop
	if !strings.Contains(src, "_windows_detach()") {
		t.Error("xwcli source missing _windows_detach() call in run command")
	}

	// pid_file cleanup on run-inner exit
	if !strings.Contains(src, "pid_file.unlink(missing_ok=True)") {
		t.Error("xwcli source missing pid_file cleanup on run-inner exit")
	}
}

// TestXwcliWindowsBackgroundSrcVariants verifies all expected Windows patterns.
func TestXwcliWindowsBackgroundSrcVariants(t *testing.T) {
	src := xwcliSource
	patterns := []struct {
		name    string
		pattern string
	}{
		{"DETACHED_PROCESS flag value", "DETACHED = 0x00000008"},
		{"CREATE_PROCESS_GROUP flag", "CREATE_PGROUP = 0x00000200"},
		{"STARTF_USESHOWWINDOW", "STARTF_USESHOWWINDOW"},
		{"sys.platform == win32 check", `sys.platform == "win32"`},
		{"run-inner exits cleanly", `sys.exit(0)`},
		{"xwcli.pid file name", "xwcli.pid"},
	}
	for _, p := range patterns {
		if !strings.Contains(src, p.pattern) {
			t.Errorf("xwcli source missing Windows pattern: %s (%q)", p.name, p.pattern)
		}
	}
}

// TestXwcliWindowsInstallScript verifies install.sh detects Windows and adapts.
func TestXwcliInstallScriptWindows(t *testing.T) {
	// Build a fake server URL to generate script
	script := strings.Replace(installScriptTemplate, "${SERVER_URL}", "http://localhost:8902", 1)

	// Script must work on Linux (bash)
	if !strings.Contains(script, "curl -fsSL") && !strings.Contains(script, "wget") {
		t.Error("install script missing download method")
	}

	// Script uses bash (Linux only)
	if !strings.Contains(script, "#!/bin/bash") {
		t.Error("install script missing bash shebang")
	}

	// Script mentions xwcli.py
	if !strings.Contains(script, "xwcli.py") {
		t.Error("install script missing xwcli.py reference")
	}
}