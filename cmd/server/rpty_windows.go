//go:build windows

package main

import (
	"encoding/json"
	"net/http"
)

// RPTYSession is not available on Windows.
type RPTYSession struct{}

// RegisterRPTY is a no-op on Windows.
func RegisterRPTY(tabID string, sess *RPTYSession) {}

// FindRPTY always returns nil on Windows.
func FindRPTY(tabID string) *RPTYSession { return nil }

// UnregisterRPTY is a no-op on Windows.
func UnregisterRPTY(tabID string) {}

func (s *RPTYSession) markWsClosed()        {}
func (s *RPTYSession) cancelGracePeriod()   {}
func (s *RPTYSession) WriteInput(string) error { return errWindowsNotSupportedRPTY }

func (s *APIServer) handleRemotePty(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusServiceUnavailable, "Remote PTY not supported on Windows")
}

func (s *APIServer) handleRptyInput(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusServiceUnavailable, "Remote PTY not supported on Windows")
}

func writeErrRPTY(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

var errWindowsNotSupportedRPTY = &windowsRPTYError{}

type windowsRPTYError struct{}

func (e *windowsRPTYError) Error() string { return "Remote PTY not supported on Windows" }
