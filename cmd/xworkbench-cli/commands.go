// xworkbench-cli commands
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTP request timeout
const httpTimeout = 30 * time.Second

type apiResponse struct {
	StatusCode int
	Body       map[string]any
	RawBody    string
}

func apiRequest(method, url string, data any) (*apiResponse, error) {
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

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
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

func baseURL() string {
	return strings.TrimSuffix(flagServer, "/")
}

func printJSON(data any) {
	b, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(b))
}

func printOK(data any) {
	printJSON(map[string]any{"ok": true, "data": data})
}

func printError(err error) {
	printJSON(map[string]any{"ok": false, "error": err.Error()})
}

// --- Task commands ---

func runTask(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("task subcommand required (list/create/get/update/run/cancel/delete)")
	}
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "list":
		return taskList(rest)
	case "create":
		return taskCreate(rest)
	case "get":
		return taskGet(rest)
	case "update":
		return taskUpdate(rest)
	case "run":
		return taskRun(rest)
	case "cancel":
		return taskCancel(rest)
	case "delete":
		return taskDelete(rest)
	default:
		return fmt.Errorf("unknown task subcommand: %s", sub)
	}
}

func taskList(args []string) error {
	fs := flag.NewFlagSet("task list", flag.ExitOnError)
	status := fs.String("status", "", "filter by status")
	taskType := fs.String("task-type", "", "filter by type (local/remote)")
	limit := fs.Int("limit", 50, "max results")
	fs.Parse(args)

	url := fmt.Sprintf("%s/api/tasks?limit=%d", baseURL(), *limit)
	if *status != "" {
		url += "&status=" + *status
	}
	if *taskType != "" {
		url += "&task_type=" + *taskType
	}
	resp, err := apiRequest("GET", url, nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func taskCreate(args []string) error {
	fs := flag.NewFlagSet("task create", flag.ExitOnError)
	title := fs.String("title", "", "task title (required)")
	description := fs.String("description", "", "task description")
	targetDirID := fs.String("target-dir-id", "", "remote target dir shortcut ID")
	fs.Parse(args)

	if *title == "" {
		return fmt.Errorf("--title is required")
	}
	payload := map[string]any{"title": *title}
	if *description != "" {
		payload["description"] = *description
	}
	if *targetDirID != "" {
		payload["assigned_dir_shortcut_id"] = *targetDirID
	}

	resp, err := apiRequest("POST", baseURL()+"/api/tasks", payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func taskGet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("task ID required")
	}
	id := args[0]
	resp, err := apiRequest("GET", fmt.Sprintf("%s/api/tasks/%s", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func taskUpdate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("task ID required")
	}
	id := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("task update", flag.ExitOnError)
	title := fs.String("title", "", "task title")
	description := fs.String("description", "", "task description")
	taskStatus := fs.String("status", "", "task status")
	priority := fs.Int("priority", 0, "task priority")
	targetDirID := fs.String("target-dir-id", "", "remote target dir shortcut ID")
	fs.Parse(rest)

	payload := map[string]any{}
	if *title != "" {
		payload["title"] = *title
	}
	if *description != "" {
		payload["description"] = *description
	}
	if *taskStatus != "" {
		payload["status"] = *taskStatus
	}
	if *priority > 0 {
		payload["priority"] = *priority
	}
	if *targetDirID != "" {
		payload["assigned_dir_shortcut_id"] = *targetDirID
	}

	resp, err := apiRequest("PUT", fmt.Sprintf("%s/api/tasks/%s", baseURL(), id), payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func taskRun(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("task ID required")
	}
	id := args[0]
	resp, err := apiRequest("POST", fmt.Sprintf("%s/api/tasks/%s/run", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func taskCancel(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("task ID required")
	}
	id := args[0]
	resp, err := apiRequest("POST", fmt.Sprintf("%s/api/tasks/%s/cancel", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func taskDelete(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("task ID required")
	}
	id := args[0]
	resp, err := apiRequest("DELETE", fmt.Sprintf("%s/api/tasks/%s", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

// --- Exec commands ---

func runExec(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("exec subcommand required (list/get/evaluate/cancel/continue)")
	}
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "list":
		return execList(rest)
	case "get":
		return execGet(rest)
	case "evaluate":
		return execEvaluate(rest)
	case "cancel":
		return execCancel(rest)
	case "continue":
		return execContinue(rest)
	default:
		return fmt.Errorf("unknown exec subcommand: %s", sub)
	}
}

func execList(args []string) error {
	fs := flag.NewFlagSet("exec list", flag.ExitOnError)
	taskID := fs.String("task-id", "", "filter by manual task ID")
	schedTaskID := fs.String("scheduled-task-id", "", "filter by scheduled task ID")
	limit := fs.Int("limit", 50, "max results")
	fs.Parse(args)

	var url string
	if *schedTaskID != "" {
		url = fmt.Sprintf("%s/api/executions?limit=%d", baseURL(), *limit)
	} else if *taskID != "" {
		url = fmt.Sprintf("%s/api/tasks/%s/executions?limit=%d", baseURL(), *taskID, *limit)
	} else {
		url = fmt.Sprintf("%s/api/executions?limit=%d", baseURL(), *limit)
	}
	resp, err := apiRequest("GET", url, nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func execGet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("execution ID required")
	}
	id := args[0]
	resp, err := apiRequest("GET", fmt.Sprintf("%s/api/executions/%s", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func execEvaluate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("execution ID required")
	}
	id := args[0]
	resp, err := apiRequest("POST", fmt.Sprintf("%s/api/executions/%s/evaluate", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func execCancel(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("execution ID required")
	}
	id := args[0]
	resp, err := apiRequest("POST", fmt.Sprintf("%s/api/executions/%s/cancel", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func execContinue(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("execution ID required")
	}
	id := args[0]
	resp, err := apiRequest("POST", fmt.Sprintf("%s/api/executions/%s/continue", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

// --- Experience commands ---

func runExperience(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("experience subcommand required (list/get/create/update/delete)")
	}
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "list":
		return experienceList(rest)
	case "get":
		return experienceGet(rest)
	case "create":
		return experienceCreate(rest)
	case "update":
		return experienceUpdate(rest)
	case "delete":
		return experienceDelete(rest)
	default:
		return fmt.Errorf("unknown experience subcommand: %s", sub)
	}
}

func experienceList(args []string) error {
	fs := flag.NewFlagSet("experience list", flag.ExitOnError)
	module := fs.String("module", "", "filter by module")
	fs.Parse(args)

	url := baseURL() + "/api/experiences"
	if *module != "" {
		url += "?module=" + *module
	}
	resp, err := apiRequest("GET", url, nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func experienceGet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("experience ID required")
	}
	id := args[0]
	resp, err := apiRequest("GET", fmt.Sprintf("%s/api/experiences/%s", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func experienceCreate(args []string) error {
	fs := flag.NewFlagSet("experience create", flag.ExitOnError)
	module := fs.String("module", "", "module name (required)")
	scene := fs.String("scene", "", "scene description (required)")
	keywords := fs.String("keywords", "", "keywords (required)")
	toolUsage := fs.String("tool-usage", "", "tool usage")
	logSamples := fs.String("log-samples", "", "log samples")
	codeSnippets := fs.String("code-snippets", "", "code snippets")
	fs.Parse(args)

	if *module == "" || *scene == "" || *keywords == "" {
		return fmt.Errorf("--module, --scene, --keywords are required")
	}
	payload := map[string]any{
		"module":   *module,
		"scene":    *scene,
		"keywords": *keywords,
	}
	if *toolUsage != "" {
		payload["tool_usage"] = *toolUsage
	}
	if *logSamples != "" {
		payload["log_samples"] = *logSamples
	}
	if *codeSnippets != "" {
		payload["code_snippets"] = *codeSnippets
	}

	resp, err := apiRequest("POST", baseURL()+"/api/experiences", payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func experienceUpdate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("experience ID required")
	}
	id := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("experience update", flag.ExitOnError)
	module := fs.String("module", "", "module name")
	scene := fs.String("scene", "", "scene description")
	keywords := fs.String("keywords", "", "keywords")
	toolUsage := fs.String("tool-usage", "", "tool usage")
	logSamples := fs.String("log-samples", "", "log samples")
	codeSnippets := fs.String("code-snippets", "", "code snippets")
	fs.Parse(rest)

	payload := map[string]any{}
	if *module != "" {
		payload["module"] = *module
	}
	if *scene != "" {
		payload["scene"] = *scene
	}
	if *keywords != "" {
		payload["keywords"] = *keywords
	}
	if *toolUsage != "" {
		payload["tool_usage"] = *toolUsage
	}
	if *logSamples != "" {
		payload["log_samples"] = *logSamples
	}
	if *codeSnippets != "" {
		payload["code_snippets"] = *codeSnippets
	}

	resp, err := apiRequest("PUT", fmt.Sprintf("%s/api/experiences/%s", baseURL(), id), payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func experienceDelete(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("experience ID required")
	}
	id := args[0]
	resp, err := apiRequest("DELETE", fmt.Sprintf("%s/api/experiences/%s", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

// --- Scheduled commands ---

func runScheduled(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("scheduled subcommand required (list/get/create/update/delete/run/toggle)")
	}
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "list":
		return scheduledList(rest)
	case "get":
		return scheduledGet(rest)
	case "create":
		return scheduledCreate(rest)
	case "update":
		return scheduledUpdate(rest)
	case "delete":
		return scheduledDelete(rest)
	case "run":
		return scheduledRun(rest)
	case "toggle":
		return scheduledToggle(rest)
	default:
		return fmt.Errorf("unknown scheduled subcommand: %s", sub)
	}
}

func scheduledList(args []string) error {
	fs := flag.NewFlagSet("scheduled list", flag.ExitOnError)
	taskType := fs.String("task-type", "", "filter by local|remote (client-side)")
	fs.Parse(args)

	resp, err := apiRequest("GET", baseURL()+"/api/scheduled", nil)
	if err != nil {
		return err
	}
	if *taskType == "" || resp.Body == nil {
		printOK(resp.Body)
		return nil
	}
	// 客户端过滤：手动(assigned_dir_shortcut_id 为空) vs 远程(非空)
	tasks, ok := resp.Body["data"].([]any)
	if !ok {
		// 直接打印原始响应
		printOK(resp.Body)
		return nil
	}
	var filtered []any
	for _, t := range tasks {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		hasTarget := false
		if v, ok := m["assigned_dir_shortcut_id"].(string); ok && v != "" {
			hasTarget = true
		}
		if (*taskType == "remote" && hasTarget) || (*taskType == "local" && !hasTarget) {
			filtered = append(filtered, t)
		}
	}
	resp.Body["data"] = filtered
	printOK(resp.Body)
	return nil
}

func scheduledGet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("scheduled task ID required")
	}
	id := args[0]
	resp, err := apiRequest("GET", fmt.Sprintf("%s/api/scheduled/%s", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func scheduledCreate(args []string) error {
	fs := flag.NewFlagSet("scheduled create", flag.ExitOnError)
	name := fs.String("name", "", "task name (required)")
	cron := fs.String("cron", "", "cron expression (required)")
	commandType := fs.String("command-type", "shell", "command type (shell/claude/cbc)")
	model := fs.String("model", "", "model name")
	prompt := fs.String("prompt", "", "prompt/script")
	targetDirID := fs.String("target-dir-id", "", "remote target dir shortcut ID")
	fs.Parse(args)

	if *name == "" || *cron == "" {
		return fmt.Errorf("--name and --cron are required")
	}
	payload := map[string]any{
		"name":         *name,
		"cron_expr":    *cron,
		"command_type": *commandType,
	}
	if *model != "" {
		payload["model"] = *model
	}
	if *prompt != "" {
		payload["prompt"] = *prompt
	}
	if *targetDirID != "" {
		payload["assigned_dir_shortcut_id"] = *targetDirID
	}

	resp, err := apiRequest("POST", baseURL()+"/api/scheduled", payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func scheduledUpdate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("scheduled task ID required")
	}
	id := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("scheduled update", flag.ExitOnError)
	name := fs.String("name", "", "task name")
	cron := fs.String("cron", "", "cron expression")
	commandType := fs.String("command-type", "", "command type")
	model := fs.String("model", "", "model name")
	prompt := fs.String("prompt", "", "prompt/script")
	enabled := fs.Bool("enabled", false, "enabled")
	targetDirID := fs.String("target-dir-id", "", "remote target dir shortcut ID")
	fs.Parse(rest)

	payload := map[string]any{}
	if *name != "" {
		payload["name"] = *name
	}
	if *cron != "" {
		payload["cron_expr"] = *cron
	}
	if *commandType != "" {
		payload["command_type"] = *commandType
	}
	if *model != "" {
		payload["model"] = *model
	}
	if *prompt != "" {
		payload["prompt"] = *prompt
	}
	if *enabled {
		payload["enabled"] = true
	}
	if *targetDirID != "" {
		payload["assigned_dir_shortcut_id"] = *targetDirID
	}

	resp, err := apiRequest("PUT", fmt.Sprintf("%s/api/scheduled/%s", baseURL(), id), payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func scheduledDelete(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("scheduled task ID required")
	}
	id := args[0]
	resp, err := apiRequest("DELETE", fmt.Sprintf("%s/api/scheduled/%s", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func scheduledRun(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("scheduled task ID required")
	}
	id := args[0]
	resp, err := apiRequest("POST", fmt.Sprintf("%s/api/scheduled/%s/run-now", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func scheduledToggle(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("scheduled task ID required")
	}
	id := args[0]
	resp, err := apiRequest("POST", fmt.Sprintf("%s/api/scheduled/%s/toggle", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

// --- Todo commands ---

func runTodo(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("todo subcommand required (list/add/toggle/edit)")
	}
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "list":
		return todoList(rest)
	case "add":
		return todoAdd(rest)
	case "toggle":
		return todoToggle(rest)
	case "edit":
		return todoEdit(rest)
	default:
		return fmt.Errorf("unknown todo subcommand: %s", sub)
	}
}

func todoList(args []string) error {
	resp, err := apiRequest("GET", baseURL()+"/api/todo", nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func todoAdd(args []string) error {
	fs := flag.NewFlagSet("todo add", flag.ExitOnError)
	content := fs.String("content", "", "todo content (required)")
	fs.Parse(args)

	if *content == "" {
		return fmt.Errorf("--content is required")
	}
	payload := map[string]any{"content": *content}

	resp, err := apiRequest("POST", baseURL()+"/api/todo", payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func todoToggle(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("todo line number required")
	}
	lineNo := args[0]
	resp, err := apiRequest("PUT", fmt.Sprintf("%s/api/todo/%s", baseURL(), lineNo), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func todoEdit(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("todo line number and content required")
	}
	lineNo := args[0]
	content := args[1]
	payload := map[string]any{"content": content}

	resp, err := apiRequest("PUT", fmt.Sprintf("%s/api/todo/%s/edit", baseURL(), lineNo), payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

// --- Config commands ---

func runConfig(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("config subcommand required (get/set)")
	}
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "get":
		return configGet(rest)
	case "set":
		return configSet(rest)
	default:
		return fmt.Errorf("unknown config subcommand: %s", sub)
	}
}

func configGet(args []string) error {
	resp, err := apiRequest("GET", baseURL()+"/api/config", nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func configSet(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("key and value required")
	}
	key := args[0]
	value := args[1]

	payload := map[string]any{key: value}
	resp, err := apiRequest("PUT", baseURL()+"/api/config", payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

// --- Models command ---

func runModels(args []string) error {
	resp, err := apiRequest("GET", baseURL()+"/api/models", nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

// --- Stats command ---

func runStats(args []string) error {
	resp, err := apiRequest("GET", baseURL()+"/api/stats", nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

// --- Dir-Shortcut commands ---

func runDirShortcut(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("dir-shortcut subcommand required (list/create/update/delete/open)")
	}
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "list":
		return dirShortcutList(rest)
	case "create":
		return dirShortcutCreate(rest)
	case "update":
		return dirShortcutUpdate(rest)
	case "delete":
		return dirShortcutDelete(rest)
	case "open":
		return dirShortcutOpen(rest)
	default:
		return fmt.Errorf("unknown dir-shortcut subcommand: %s", sub)
	}
}

func dirShortcutList(args []string) error {
	resp, err := apiRequest("GET", baseURL()+"/api/dir-shortcuts", nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func dirShortcutCreate(args []string) error {
	fs := flag.NewFlagSet("dir-shortcut create", flag.ExitOnError)
	name := fs.String("name", "", "shortcut name (required)")
	path := fs.String("path", "", "directory path (required)")
	fs.Parse(args)

	if *name == "" || *path == "" {
		return fmt.Errorf("--name and --path are required")
	}
	payload := map[string]any{"name": *name, "path": *path}

	resp, err := apiRequest("POST", baseURL()+"/api/dir-shortcuts", payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func dirShortcutUpdate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("dir-shortcut ID required")
	}
	id := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("dir-shortcut update", flag.ExitOnError)
	name := fs.String("name", "", "shortcut name")
	path := fs.String("path", "", "directory path")
	fs.Parse(rest)

	payload := map[string]any{}
	if *name != "" {
		payload["name"] = *name
	}
	if *path != "" {
		payload["path"] = *path
	}

	resp, err := apiRequest("PUT", fmt.Sprintf("%s/api/dir-shortcuts/%s", baseURL(), id), payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func dirShortcutDelete(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("dir-shortcut ID required")
	}
	id := args[0]
	resp, err := apiRequest("DELETE", fmt.Sprintf("%s/api/dir-shortcuts/%s", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func dirShortcutOpen(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("dir-shortcut ID required")
	}
	id := args[0]
	resp, err := apiRequest("POST", fmt.Sprintf("%s/api/dir-shortcuts/%s/open", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

// --- Web-Link commands ---

func runWebLink(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("web-link subcommand required (list/create/update/delete/open)")
	}
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "list":
		return webLinkList(rest)
	case "create":
		return webLinkCreate(rest)
	case "update":
		return webLinkUpdate(rest)
	case "delete":
		return webLinkDelete(rest)
	case "open":
		return webLinkOpen(rest)
	default:
		return fmt.Errorf("unknown web-link subcommand: %s", sub)
	}
}

func webLinkList(args []string) error {
	resp, err := apiRequest("GET", baseURL()+"/api/web-links", nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func webLinkCreate(args []string) error {
	fs := flag.NewFlagSet("web-link create", flag.ExitOnError)
	title := fs.String("title", "", "link title (required)")
	url := fs.String("url", "", "link URL (required)")
	fs.Parse(args)

	if *title == "" || *url == "" {
		return fmt.Errorf("--title and --url are required")
	}
	payload := map[string]any{"title": *title, "url": *url}

	resp, err := apiRequest("POST", baseURL()+"/api/web-links", payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func webLinkUpdate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("web-link ID required")
	}
	id := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("web-link update", flag.ExitOnError)
	title := fs.String("title", "", "link title")
	url := fs.String("url", "", "link URL")
	fs.Parse(rest)

	payload := map[string]any{}
	if *title != "" {
		payload["title"] = *title
	}
	if *url != "" {
		payload["url"] = *url
	}

	resp, err := apiRequest("PUT", fmt.Sprintf("%s/api/web-links/%s", baseURL(), id), payload)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func webLinkDelete(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("web-link ID required")
	}
	id := args[0]
	resp, err := apiRequest("DELETE", fmt.Sprintf("%s/api/web-links/%s", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}

func webLinkOpen(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("web-link ID required")
	}
	id := args[0]
	resp, err := apiRequest("POST", fmt.Sprintf("%s/api/links/open?id=%s", baseURL(), id), nil)
	if err != nil {
		return err
	}
	printOK(resp.Body)
	return nil
}
