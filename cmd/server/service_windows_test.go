//go:build windows

package main

import (
	"testing"
)

func TestWindowsConstants(t *testing.T) {
	if windowsServiceName != "xworkbench" {
		t.Errorf("windowsServiceName: want 'xworkbench', got %q", windowsServiceName)
	}
	if windowsServiceDisplayName == "" {
		t.Error("windowsServiceDisplayName must be non-empty")
	}
}

func TestRunServiceFlagRunValue(t *testing.T) {
	// When --service=run is set but service name doesn't exist,
	// svc.Run returns an error (service not registered) - which is expected.
	// We just verify the flag parsing path works.
	if serviceFlag == nil {
		t.Fatal("serviceFlag is nil")
	}
}