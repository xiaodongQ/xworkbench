package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	heartbeatInterval = 15 * time.Second
	claimInterval     = 10 * time.Second
	execTimeout       = 10 * time.Minute
)

// AgentConfig is persisted to ~/.config/xwcli/agent.json.
type AgentConfig struct {
	AgentID     string `json:"agent_id"`
	Token       string `json:"token"`
	ServerURL   string `json:"server_url"`
	MachineName string `json:"machine_name"`
	Version     string `json:"version"`
}

type apiResponse struct {
	StatusCode int
	Body       map[string]any
	RawBody    string
}

func configPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".config", "xwcli", "agent.json")
}

func configDirPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".config", "xwcli")
}

func loadConfig() (*AgentConfig, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil, fmt.Errorf("未注册，请先运行: xwcli register --server <url> --name <name>")
	}
	var cfg AgentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("配置文件格式错误: %w", err)
	}
	return &cfg, nil
}

func saveConfig(cfg *AgentConfig) error {
	if err := os.MkdirAll(configDirPath(), 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	if err := os.WriteFile(configPath(), data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}

func apiRequest(method, url string, data any, token string) (*apiResponse, error) {
	var body []byte
	if data != nil {
		var err error
		body, err = json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		parsed = map[string]any{"raw": string(respBody)}
	}
	return &apiResponse{StatusCode: resp.StatusCode, Body: parsed, RawBody: string(respBody)}, nil
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getTaskID(resp *apiResponse) string {
	if tid := getString(resp.Body, "task_id"); tid != "" {
		return tid
	}
	if task, ok := resp.Body["task"].(map[string]any); ok {
		return getString(task, "id")
	}
	return ""
}

// cmdRegister registers this machine as an agent.
func cmdRegister() error {
	if flagServer == "" {
		return fmt.Errorf("--server is required")
	}
	server := strings.TrimSuffix(flagServer, "/")

	cfg := &AgentConfig{
		ServerURL:   server,
		MachineName: flagName,
		Version:     "1.0.0",
	}
	url := server + "/api/agents/register"
	resp, err := apiRequest("POST", url, map[string]any{
		"name":         flagName,
		"capabilities": "task-execute",
		"version":      "1.0.0",
	}, "")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("注册失败 (%d): %s", resp.StatusCode, resp.RawBody)
	}
	cfg.AgentID = getString(resp.Body, "agent_id")
	cfg.Token = getString(resp.Body, "token")
	if cfg.AgentID == "" || cfg.Token == "" {
		return fmt.Errorf("服务器响应缺少 agent_id 或 token: %s", resp.RawBody)
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("OK: 注册成功! agent_id=%s\n", cfg.AgentID)
	fmt.Printf("    配置文件: %s\n", configPath())
	return nil
}

func cmdStatus() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(data))
	return nil
}

func cmdStop() error {
	pidFile := filepath.Join(configDirPath(), "xwcli.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Println("未找到 pid 文件，可能没有在运行")
		return nil
	}
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	if pid > 0 {
		if proc, err := os.FindProcess(pid); err == nil {
			proc.Kill()
			fmt.Printf("已发送 SIGTERM 到 PID %d\n", pid)
		}
	}
	os.Remove(pidFile)
	return nil
}

func cmdRun() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	pidFile := filepath.Join(configDirPath(), "xwcli.pid")
	os.MkdirAll(configDirPath(), 0755)
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)

	fmt.Printf("[xwcli] 启动 agent, server=%s, machine=%s\n", cfg.ServerURL, cfg.MachineName)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doHeartbeat(cfg.Token, cfg.ServerURL, "idle", "")

	eg, gctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		currentTaskID := ""
		for {
			select {
			case <-gctx.Done():
				return gctx.Err()
			case <-ticker.C:
				status := "idle"
				if currentTaskID != "" {
					status = "busy"
				}
				doHeartbeat(cfg.Token, cfg.ServerURL, status, currentTaskID)
			}
		}
	})

	eg.Go(func() error {
		for {
			select {
			case <-gctx.Done():
				return gctx.Err()
			default:
			}

			result := doClaimNext(cfg.Token, cfg.ServerURL, cfg.AgentID)
			if result == nil {
				time.Sleep(claimInterval)
				continue
			}

			taskID := getTaskID(result)
			prompt := getString(result.Body, "prompt")
			if taskID == "" || prompt == "" {
				time.Sleep(claimInterval)
				continue
			}

			fmt.Printf("[xwcli] 领到任务 task_id=%s\n", taskID)
			start := time.Now()
			stdout, stderr, exitCode, duration := runClaude(prompt)
			output := stdout
			if stderr != "" {
				output += "\n[STDERR] " + stderr
			}
			lastError := ""
			if exitCode != 0 {
				lastError = fmt.Sprintf("exit_code=%d", exitCode)
			}
			_ = start
			doReport(cfg.Token, cfg.ServerURL, taskID, output, "archived", nil, lastError, duration)
			fmt.Printf("[xwcli] 任务完成 task_id=%s, duration=%ds, exit=%d\n", taskID, duration, exitCode)
		}
	})

	return eg.Wait()
}

func doHeartbeat(token, serverURL, status, currentTaskID string) {
	resp, err := apiRequest("POST", serverURL+"/api/agents/heartbeat", map[string]string{
		"status":          status,
		"current_task_id": currentTaskID,
	}, token)
	if err != nil {
		fmt.Printf("[xwcli] heartbeat error: %v\n", err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[xwcli] heartbeat failed: %d %s\n", resp.StatusCode, resp.RawBody)
	}
}

func doClaimNext(token, serverURL, agentID string) *apiResponse {
	url := fmt.Sprintf("%s/api/tasks/claim-next?agent_id=%s&timeout=30", serverURL, agentID)
	resp, err := apiRequest("GET", url, nil, token)
	if err != nil {
		fmt.Printf("[xwcli] claim-next error: %v\n", err)
		return nil
	}
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == 0 {
		return nil
	}
	if resp.StatusCode == http.StatusOK && getString(resp.Body, "status") == "claimed" {
		return resp
	}
	return nil
}

func doReport(token, serverURL, taskID, resultOutput, status string, score *float64, lastError string, durationSec int) {
	url := fmt.Sprintf("%s/api/tasks/%s/report", serverURL, taskID)
	payload := map[string]any{
		"status":        status,
		"result_output": resultOutput,
	}
	if lastError != "" {
		payload["last_error"] = lastError
	}
	if score != nil {
		payload["evaluation_score"] = *score
	}
	resp, err := apiRequest("POST", url, payload, token)
	if err != nil {
		fmt.Printf("[xwcli] report error: %v\n", err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[xwcli] report failed: %d %s\n", resp.StatusCode, resp.RawBody)
	}
}

func runClaude(prompt string) (stdout, stderr string, exitCode int, durationSec int) {
	clis := []string{"claude", "hermes"}
	for _, cli := range clis {
		path, err := exec.LookPath(cli)
		if err != nil {
			continue
		}

		// Try --print mode first (non-interactive), then fallback to stdin
		argsList := [][]string{
			{path, "--print", prompt},
			{path},
		}
		for _, args := range argsList {
			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
			cmd := exec.CommandContext(ctx, args[0], args[1:]...)
			cmd.Stdin = nil

			var outBuf, errBuf bytes.Buffer
			cmd.Stdout = &outBuf
			cmd.Stderr = &errBuf

			err := cmd.Run()
			cancel()
			durationSec = int(time.Since(start).Seconds())

			if err == nil {
				return outBuf.String(), errBuf.String(), 0, durationSec
			}

			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				return outBuf.String(), errBuf.String(), exitErr.ExitCode(), durationSec
			}

			if errors.Is(err, context.DeadlineExceeded) {
				return "", fmt.Sprintf("%s 执行超时（10分钟）", cli), 124, durationSec
			}
		}
	}
	return "", "ERROR: claude/hermes 均未找到", 127, 0
}
