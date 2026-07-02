//go:build windows

package main

import (
	"flag"
)

// serviceFlag is a no-op stub on Windows (Service mode not used).
var serviceFlag = flag.String("service", "", `run as a Windows Service (disabled, use run_background.ps1)`)

// runServiceFlag returns false on Windows (Service mode not used).
// Use scripts/run_background.ps1 for background operation on Windows.
func runServiceFlag(startFn, stopFn func()) bool {
	return false
}

// serviceStopCh is nil on Windows (not used without Service mode).
var serviceStopCh chan struct{}
