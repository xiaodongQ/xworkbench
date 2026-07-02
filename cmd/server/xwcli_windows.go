//go:build windows

package main

import (
	"net/http"
)

func (s *APIServer) handleXwcliInstall(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusServiceUnavailable, "xwcli install script not available on Windows")
}

func (s *APIServer) handleXwcliDownload(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusServiceUnavailable, "xwcli download not available on Windows")
}

