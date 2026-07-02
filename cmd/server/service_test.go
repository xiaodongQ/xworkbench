package main

import (
	"testing"
)

// TestServiceFlagDefined verifies the --service flag is defined (not nil).
func TestServiceFlagDefined(t *testing.T) {
	if serviceFlag == nil {
		t.Fatal("serviceFlag is nil (service.go not compiled?)")
	}
}

// TestRunServiceFlagNoFlag returns handled=false when no --service flag set.
func TestRunServiceFlagNoFlag(t *testing.T) {
	handled := runServiceFlag(nil, nil)
	if handled {
		t.Error("runServiceFlag: expected handled=false with no --service flag on non-Windows")
	}
}

// TestServiceStopChIsNilOnNonWindows verifies serviceStopCh is nil on non-Windows.
func TestServiceStopChIsNilOnNonWindows(t *testing.T) {
	// On non-Windows, serviceStopCh is never assigned (stays nil)
	if serviceStopCh != nil {
		t.Errorf("serviceStopCh: expected nil on non-Windows, got %v", serviceStopCh)
	}
}

// TestIsWindowsService returns false on non-Windows.
func TestIsWindowsService(t *testing.T) {
	if isWindowsService() {
		t.Error("isWindowsService: expected false on non-Windows")
	}
}