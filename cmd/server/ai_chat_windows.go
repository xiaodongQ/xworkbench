//go:build windows

package main

import (
	"io"
	"net/http"
	"strings"
)

func (s *APIServer) handleAIChat(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusServiceUnavailable, "AI Chat not supported on Windows")
}

func (s *APIServer) handleAIConfigGet(w http.ResponseWriter, r *http.Request) {
	// Return empty/minimal config on Windows
	writeJSON(w, map[string]any{
		"ai_chat": map[string]any{
			"enabled":  false,
			"provider": "openai",
			"model":    "",
		},
	})
}

func (s *APIServer) handleAIConfigUpdate(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusServiceUnavailable, "AI Chat not supported on Windows")
}

func (s *APIServer) handleAIConfigSetKey(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusServiceUnavailable, "AI Chat not supported on Windows")
}

func (s *APIServer) handleAIConfigTest(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusServiceUnavailable, "AI Chat not supported on Windows")
}

func (s *APIServer) isAuthenticated(r *http.Request) bool {
	return r.Header.Get("X-User-ID") != ""
}

func readString(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	return strings.TrimSpace(string(b))
}

// writeErr and writeJSON helpers — also needed by Windows stubs

