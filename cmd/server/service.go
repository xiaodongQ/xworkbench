//go:build !windows

package main

import (
	"flag"
)

// serviceFlag handles --service on non-Windows platforms (no-op stub).
var serviceFlag = flag.String("service", "", `run as a Windows service (no-op on this platform)`)

// runServiceFlag returns false on non-Windows (service mode not supported).
// startFn and stopFn are ignored on non-Windows.
func runServiceFlag(startFn, stopFn func()) bool {
	return false
}

// RegisterServiceFuncs is a stub on non-Windows.
func RegisterServiceFuncs(start, stop func()) {}

// isWindowsService returns false on non-Windows.
func isWindowsService() bool { return false }

// StopServiceByChannel is a stub on non-Windows.
func StopServiceByChannel() {}

// serviceStopCh is nil on non-Windows (set by runServiceFlag on Windows).
var serviceStopCh chan struct{}