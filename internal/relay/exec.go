// Package relay 提供跨平台代理和消息转发功能。
package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/executor"
)

// ExecRequest 是 /api/exec 的请求结构。
type ExecRequest struct {
	Command    string `json:"command"`
	Cwd        string `json:"cwd"`
	TimeoutMs  int    `json:"timeout_ms"`
}

// ExecResponse 是 /api/exec 的响应结构。
type ExecResponse struct {
	Output     string `json:"output"`
	ErrorOut  string `json:"error_out"`
	ExitCode  int    `json:"exit_code"`
	DurationMs int64 `json:"duration_ms"`
	Error     string `json:"error,omitempty"`
}

// HandleExec 接收命令，在 Windows 上执行并返回结果。
func HandleExec(w http.ResponseWriter, r *http.Request) {
	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Command == "" {
		http.Error(w, "command is required", http.StatusBadRequest)
		return
	}

	timeout := 30 * time.Second
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	start := time.Now()
	result, err := executor.Run(ctx, parseShell(req.Command), req.Cwd, nil)

	resp := ExecResponse{
		DurationMs: time.Since(start).Milliseconds(),
	}
	if err != nil {
		resp.Error = err.Error()
	}
	if result != nil {
		resp.Output = result.Output
		resp.ErrorOut = result.ErrorOut
		resp.ExitCode = result.ExitCode
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// parseShell 将 shell 命令字符串解析为 exec.Command 所需的 []string。
// 支持简单的命令解析，处理命令和参数。
func parseShell(cmd string) []string {
	// 简单的 shell 解析，处理基本空格分隔
	// 注意：这是简化实现，不处理引号转义等复杂情况
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}
	return parts
}