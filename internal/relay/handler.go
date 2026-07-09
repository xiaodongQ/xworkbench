package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/logger"
)



// ProxyRequest describes an HTTP request to be proxied through xworkbench.
type ProxyRequest struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
	TimeoutMs  int               `json:"timeout_ms"`
}

// ProxyResponse describes the response from a proxied request.
type ProxyResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
	Error      string            `json:"error,omitempty"`
}

// RelayHandler handles relay HTTP endpoints.
type RelayHandler struct {
	repo Repo
}

// NewRelayHandler creates a new RelayHandler.
func NewRelayHandler(repo Repo) *RelayHandler {
	return &RelayHandler{repo: repo}
}

// HandleRelayProxy proxies an HTTP request through xworkbench server.
func (h *RelayHandler) HandleRelayProxy(w http.ResponseWriter, r *http.Request) {
	var req ProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	// Validate URL
	parsedURL, err := url.Parse(req.URL)
	if err != nil || !parsedURL.IsAbs() {
		http.Error(w, "invalid absolute URL", http.StatusBadRequest)
		return
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	timeout := 30 * time.Second
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}

	httpReq, err := http.NewRequestWithContext(r.Context(), method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Forward auth headers
	for k, v := range req.Headers {
		if isAuthHeader(k) {
			httpReq.Header.Set(k, v)
		}
	}

	client := &http.Client{Timeout: timeout}
	started := time.Now()
	logger.Logger.Infow("relay: proxy start",
		"method", method,
		"url", req.URL,
		"timeout_ms", req.TimeoutMs,
		"req_bytes", len(req.Body),
	)
	resp, err := client.Do(httpReq)
	durMs := time.Since(started).Milliseconds()
	logEntry := &RelayLog{
		Source:       "proxy",
		Destination:  req.URL,
		Summary:      fmt.Sprintf("%s %s", method, req.URL),
		Direction:    "outbound",
		RequestSize:  len(req.Body),
	}

	var proxyResp ProxyResponse
	if err != nil {
		proxyResp = ProxyResponse{Error: err.Error()}
		logEntry.Status = "failed"
		logEntry.ErrorMsg = err.Error()
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		proxyResp = ProxyResponse{
			StatusCode: resp.StatusCode,
			Headers:    map[string]string{},
			Body:       body,
		}
		for k, v := range resp.Header {
			if len(v) > 0 {
				proxyResp.Headers[k] = v[0]
			}
		}
		logEntry.Status = "success"
		logEntry.ResponseSize = len(body)
	}

	if err != nil || proxyResp.StatusCode >= 400 {
		logger.Logger.Errorw("relay: proxy done",
			"method", method,
			"url", req.URL,
			"status", proxyResp.StatusCode,
			"dur_ms", durMs,
			"err", proxyResp.Error,
			"resp_bytes", len(proxyResp.Body),
		)
	} else {
		logger.Logger.Infow("relay: proxy done",
			"method", method,
			"url", req.URL,
			"status", proxyResp.StatusCode,
			"dur_ms", durMs,
			"resp_bytes", len(proxyResp.Body),
		)
	}

	if h.repo != nil {
		_ = h.repo.Log(logEntry)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(proxyResp)
}

// HandleRelayStats returns relay statistics.
// Query params: from, to (ISO datetime), source (e.g. "exec" or "proxy").
// If source is empty, returns stats for all sources.
func (h *RelayHandler) HandleRelayStats(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	source := r.URL.Query().Get("source")

	if h.repo == nil {
		stats := RelayStats{}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
		return
	}

	stats, err := h.repo.Stats(from, to, source)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

// isAuthHeader returns true if the header key is an authentication header.
func isAuthHeader(key string) bool {
	key = strings.ToLower(key)
	authHeaders := []string{"authorization", "x-api-key", "x-auth-token", "cookie"}
	for _, h := range authHeaders {
		if key == h || strings.HasPrefix(key, h) {
			return true
		}
	}
	return false
}