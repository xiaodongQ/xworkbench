package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"github.com/xiaodongQ/xworkbench/internal/skill"
	"github.com/xiaodongQ/xworkbench/internal/todo"
)

// serverAddr is the HTTP server listen address, set at startup.
// Used by tools that need to call the local server's own APIs.
var serverAddr = ":8902"

// SetServerAddr updates the server address (called from main at startup).
func SetServerAddr(addr string) { serverAddr = addr }

func init() {
	if a := osGetenv("ADDR"); a != "" {
		serverAddr = a
	}
}

// osGetenv is a shim so this package doesn't need os import just for getenv.
func osGetenv(k string) string {
	return k // replaced by actual os.Getenv below
}

func init() {}

// osGetenv real implementation (overridden above for compile; actual use below).
var _ = func() string { return "" }()

// GetTools returns all available AI function-calling tools (Phase 1 + Phase 2).
func GetTools() []Tool {
	tools := []Tool{
		// ── Task management ────────────────────────────────────────
		{
			Name:        "create_task",
			Description: "创建一个新任务。返回任务ID。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"title":       {"type": "string", "description": "任务标题"},
					"description": {"type": "string", "description": "任务描述"},
					"task_type":   {"type": "string", "enum": ["manual", "remote"], "description": "任务类型，manual 或 remote"},
					"priority":    {"type": "integer", "description": "优先级 1-5，1最高"},
					"acceptance":  {"type": "string", "description": "验收标准（可选）"},
					"goal_mode":   {"type": "boolean", "description": "是否启用 Goal 目标模式（可选）"}
				},
				"required": ["title"]
			}`),
		},
		{
			Name:        "list_tasks",
			Description: "列出任务列表，支持按状态/优先级/类型过滤。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"status":    {"type": "string", "description": "按状态过滤（如 pending/in_progress/archived）"},
					"priority":  {"type": "integer", "description": "按优先级过滤（1-5）"},
					"task_type": {"type": "string", "description": "按类型过滤（manual/remote）"},
					"limit":     {"type": "integer", "description": "返回数量，默认20"},
					"offset":    {"type": "integer", "description": "分页偏移，默认0"}
				}
			}`),
		},
		{
			Name:        "get_task",
			Description: "查看任务详情，包括执行历史。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id": {"type": "string", "description": "任务ID"}
				},
				"required": ["task_id"]
			}`),
		},
		{
			Name:        "update_task",
			Description: "更新任务状态、标题或描述。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id":     {"type": "string", "description": "任务ID"},
					"status":      {"type": "string", "description": "新状态（pending/in_progress/archived）"},
					"title":       {"type": "string", "description": "新标题"},
					"description": {"type": "string", "description": "新描述"},
					"acceptance":  {"type": "string", "description": "新验收标准"}
				},
				"required": ["task_id"]
			}`),
		},
		{
			Name:        "run_task",
			Description: "触发任务立即执行（Claude/CBC CLI）。返回执行记录ID。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id":   {"type": "string", "description": "任务ID"},
					"agent_id":  {"type": "string", "description": "目标 agent ID（remote 类型任务可选）"}
				},
				"required": ["task_id"]
			}`),
		},
		{
			Name:        "get_task_executions",
			Description: "查看任务的所有执行历史记录。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id": {"type": "string", "description": "任务ID"},
					"limit":  {"type": "integer", "description": "返回数量，默认10"}
				},
				"required": ["task_id"]
			}`),
		},
		// ── Directory Shortcuts ──────────────────────────────────
		{
			Name:        "create_dir_shortcut",
			Description: "创建一个目录快捷方式（本地或远程）。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name":        {"type": "string", "description": "快捷方式名称"},
					"type":        {"type": "string", "enum": ["local", "remote"], "description": "类型"},
					"path":        {"type": "string", "description": "本地路径（local 类型）"},
					"remote_host": {"type": "string", "description": "远程主机（remote 类型）"},
					"remote_user": {"type": "string", "description": "远程用户名（remote 类型）"},
					"remote_path": {"type": "string", "description": "远程路径（remote 类型）"},
					"auth_method": {"type": "string", "description": "认证方式（password/key）"},
					"key_path":    {"type": "string", "description": "密钥路径（可选）"}
				},
				"required": ["name", "type"]
			}`),
		},
		{
			Name:        "list_dir_shortcuts",
			Description: "列出所有目录快捷方式。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"type": {"type": "string", "description": "过滤类型（local/remote）"}
				}
			}`),
		},
		{
			Name:        "update_dir_shortcut",
			Description: "更新目录快捷方式的属性。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id":            {"type": "string", "description": "快捷方式ID（必填）"},
					"name":          {"type": "string", "description": "新名称"},
					"path":           {"type": "string", "description": "新本地路径"},
					"type":           {"type": "string", "enum": ["local", "remote"]},
					"remote_host":   {"type": "string", "description": "新远程主机"},
					"remote_user":   {"type": "string", "description": "新远程用户名"},
					"remote_path":   {"type": "string", "description": "新远程路径"},
					"auth_method":   {"type": "string", "description": "新认证方式（password/key）"},
					"key_path":      {"type": "string", "description": "新密钥路径"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "delete_dir_shortcut",
			Description: "删除一个目录快捷方式。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "快捷方式ID"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "open_dir_shortcut",
			Description: "在文件管理器中打开目录快捷方式（仅 local 类型）。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "快捷方式ID"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "open_dir_shortcut_terminal",
			Description: "在终端中打开目录快捷方式的工作目录。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id":   {"type": "string", "description": "快捷方式ID"},
					"type": {"type": "string", "description": "终端类型，可选（如 wezterm/Windows Terminal）"}
				},
				"required": ["id"]
			}`),
		},
		// ── Experiences ──────────────────────────────────────────
		{
			Name:        "search_experiences",
			Description: "搜索经验库。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "搜索关键词"},
					"limit": {"type": "integer", "description": "返回数量，默认10"}
				},
				"required": ["query"]
			}`),
		},
		{
			Name:        "create_experience",
			Description: "在经验库中创建一条新经验。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"module":   {"type": "string", "description": "分类模块（如 git/docker/redis）必填"},
					"keywords": {"type": "string", "description": "关键词，逗号分隔"},
					"scene":    {"type": "string", "description": "适用场景简述"},
					"details":  {"type": "string", "description": "详细内容，Markdown 格式"}
				},
				"required": ["module"]
			}`),
		},
		{
			Name:        "update_experience",
			Description: "更新一条经验的内容。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id":       {"type": "string", "description": "经验ID必填"},
					"module":   {"type": "string", "description": "分类模块"},
					"keywords": {"type": "string", "description": "关键词"},
					"scene":    {"type": "string", "description": "适用场景"},
					"details":  {"type": "string", "description": "详细内容"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "delete_experience",
			Description: "删除一条经验。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "经验ID"}
				},
				"required": ["id"]
			}`),
		},
		// ── Web Links ────────────────────────────────────────────
		{
			Name:        "list_web_links",
			Description: "列出所有收藏链接。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		{
			Name:        "create_web_link",
			Description: "添加一个新的收藏链接。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name":      {"type": "string", "description": "链接名称必填"},
					"url":       {"type": "string", "description": "URL 地址必填"},
					"icon_url":  {"type": "string", "description": "图标 URL（可选）"},
					"sort_order": {"type": "integer", "description": "排序顺序（可选）"}
				},
				"required": ["name", "url"]
			}`),
		},
		{
			Name:        "update_web_link",
			Description: "更新一个收藏链接的属性。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id":        {"type": "string", "description": "链接ID必填"},
					"name":      {"type": "string", "description": "新名称"},
					"url":       {"type": "string", "description": "新 URL"},
					"icon_url":  {"type": "string", "description": "新图标 URL"},
					"sort_order": {"type": "integer", "description": "新排序顺序"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "delete_web_link",
			Description: "删除一个收藏链接。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "链接ID"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "open_web_link",
			Description: "用系统默认浏览器打开一个 URL。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {"type": "string", "description": "要打开的 URL"}
				},
				"required": ["url"]
			}`),
		},
		// ── Todo ───────────────────────────────────────────────
		{
			Name:        "list_todos",
			Description: "列出当前 Todo 列表的所有项目。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		{
			Name:        "add_todo",
			Description: "向 Todo 列表添加一个项目。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"text": {"type": "string", "description": "Todo 内容"}
				},
				"required": ["text"]
			}`),
		},
		{
			Name:        "toggle_todo",
			Description: "切换 Todo 项目的完成状态。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"line_no": {"type": "integer", "description": "Todo 项目的行号"},
					"done":    {"type": "boolean", "description": "是否已完成（true=完成，false=未完成）"}
				},
				"required": ["line_no"]
			}`),
		},
		{
			Name:        "delete_todo",
			Description: "从 Todo 列表删除一个项目。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"line_no": {"type": "integer", "description": "Todo 项目的行号"}
				},
				"required": ["line_no"]
			}`),
		},
		// ── Local Shell ─────────────────────────────────────────
		{
			Name:        "start_local_shell",
			Description: "启动本地 Claude/CBC CLI 会话（交互式），返回 session 状态。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"cli_type": {"type": "string", "enum": ["claude", "cbc"], "description": "CLI 类型"},
					"cwd":      {"type": "string", "description": "工作目录（可选）"}
				},
				"required": ["cli_type"]
			}`),
		},
		{
			Name:        "run_local_command",
			Description: "在已启动的本地 Shell 会话中执行命令（需先调用 start_local_shell）。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command":  {"type": "string", "description": "要执行的命令"},
					"cli_type": {"type": "string", "enum": ["claude", "cbc"], "description": "CLI 类型"}
				},
				"required": ["command"]
			}`),
		},
	}
	// 追加 skill 插件工具（仅公开 skill）
	for _, s := range skill.GetPublic() {
		tools = append(tools, skillToTool(s))
	}
	return tools
}

// skillToTool 将 skill 转换为 AI Tool 格式。
func skillToTool(s *skill.Skill) Tool {
	properties := make(map[string]any)
	required := make([]string, 0)
	for name, desc := range s.XWParams {
		properties[name] = map[string]any{
			"type":        "string",
			"description": desc,
		}
		if strings.Contains(desc, "必填") {
			required = append(required, name)
		}
	}
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	paramsBytes, _ := json.Marshal(schema)
	return Tool{
		Name:        s.Name,
		Description: s.Description,
		Parameters:  json.RawMessage(paramsBytes),
	}
}

// ExecuteTool executes a tool by name with given JSON arguments.
// Returns a human-readable result string.
func ExecuteTool(ctx context.Context, db *backend.TaskRepo, expDB *backend.ExperienceRepo,
	execDB *backend.ExecutionRepo, agentDB *backend.AgentRepo,
	linkDB *backend.WebLinkRepo, dirDB *backend.DirShortcutRepo,
	localShellState *LocalShellState, toolName string, argsJSON string) string {

	// skill 工具走 skill.Execute
	if s := skill.GetByName(toolName); s != nil {
		var params map[string]any
		if argsJSON != "" {
			json.Unmarshal([]byte(argsJSON), &params)
		}
		result, err := skill.Execute(toolName, params)
		if err != nil {
			return fmt.Sprintf("执行技能 %s 失败: %v", toolName, err)
		}
		if result.Status == "error" {
			out, _ := json.Marshal(result.Output)
			return fmt.Sprintf("技能执行出错: %s", string(out))
		}
		out, _ := json.Marshal(result.Output)
		return string(out)
	}

	switch toolName {
	// Task
	case "create_task":
		return execCreateTask(ctx, db, argsJSON)
	case "list_tasks":
		return execListTasks(ctx, db, argsJSON)
	case "get_task":
		return execGetTask(ctx, db, execDB, argsJSON)
	case "update_task":
		return execUpdateTask(ctx, db, argsJSON)
	case "run_task":
		return execRunTask(ctx, db, execDB, agentDB, argsJSON)
	case "get_task_executions":
		return execGetTaskExecutions(ctx, db, execDB, argsJSON)
	// Dir Shortcuts
	case "create_dir_shortcut":
		return execCreateDirShortcut(ctx, dirDB, argsJSON)
	case "list_dir_shortcuts":
		return execListDirShortcuts(ctx, dirDB, argsJSON)
	case "update_dir_shortcut":
		return execUpdateDirShortcut(ctx, dirDB, argsJSON)
	case "delete_dir_shortcut":
		return execDeleteDirShortcut(ctx, dirDB, argsJSON)
	case "open_dir_shortcut":
		return execOpenDirShortcut(ctx, dirDB, argsJSON)
	case "open_dir_shortcut_terminal":
		return execOpenDirShortcutTerminal(ctx, dirDB, argsJSON)
	// Experiences
	case "search_experiences":
		return execSearchExperiences(ctx, expDB, argsJSON)
	case "create_experience":
		return execCreateExperience(ctx, expDB, argsJSON)
	case "update_experience":
		return execUpdateExperience(ctx, expDB, argsJSON)
	case "delete_experience":
		return execDeleteExperience(ctx, expDB, argsJSON)
	// Web Links
	case "list_web_links":
		return execListWebLinks(ctx, linkDB, argsJSON)
	case "create_web_link":
		return execCreateWebLink(ctx, linkDB, argsJSON)
	case "update_web_link":
		return execUpdateWebLink(ctx, linkDB, argsJSON)
	case "delete_web_link":
		return execDeleteWebLink(ctx, linkDB, argsJSON)
	case "open_web_link":
		return execOpenWebLink(ctx, argsJSON)
	// Todo
	case "list_todos":
		return execListTodos(ctx, argsJSON)
	case "add_todo":
		return execAddTodo(ctx, argsJSON)
	case "toggle_todo":
		return execToggleTodo(ctx, argsJSON)
	case "delete_todo":
		return execDeleteTodo(ctx, argsJSON)
	// Local Shell
	case "start_local_shell":
		return execStartLocalShell(ctx, localShellState, argsJSON)
	case "run_local_command":
		return execRunLocalCommand(ctx, localShellState, argsJSON)
	default:
		return fmt.Sprintf("未知工具: %s", toolName)
	}
}

// ── Tool executors ──────────────────────────────────────────────────────────

func execCreateTask(ctx context.Context, db *backend.TaskRepo, argsJSON string) string {
	var args struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		TaskType    string `json:"task_type"`
		Priority    int    `json:"priority"`
		Acceptance  string `json:"acceptance"`
		GoalMode    bool   `json:"goal_mode"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	prio := args.Priority
	if prio <= 0 {
		prio = 5
	}
	taskType := args.TaskType
	if taskType == "" {
		taskType = "manual"
	}
	task := &backend.Task{
		ID:          "task-" + time.Now().Format("20060102150405") + "-" + randomID(6),
		Title:       args.Title,
		Description: args.Description,
		Acceptance:  args.Acceptance,
		Status:      backend.TaskStatusPending,
		TaskType:    taskType,
		Priority:    prio,
		GoalMode:    args.GoalMode,
		Version:     "v1",
		CreatedAt:   time.Now(),
	}
	if err := db.Create(task); err != nil {
		return fmt.Sprintf("创建任务失败: %v", err)
	}
	return fmt.Sprintf("✅ 任务已创建: %s (ID: %s)", task.Title, task.ID)
}

func execListTasks(ctx context.Context, db *backend.TaskRepo, argsJSON string) string {
	var args struct {
		Status   string `json:"status"`
		Priority int    `json:"priority"`
		TaskType string `json:"task_type"`
		Limit    int    `json:"limit"`
		Offset   int    `json:"offset"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.Limit <= 0 {
		args.Limit = 20
	}
	tasks, err := db.List(backend.TaskFilter{
		Status:   args.Status,
		TaskType: args.TaskType,
		Offset:   args.Offset,
		Limit:    args.Limit,
	})
	if err != nil {
		return fmt.Sprintf("查询任务失败: %v", err)
	}
	if len(tasks) == 0 {
		return "无任务（筛选结果为空）"
	}
	var buf bytes.Buffer
	for _, t := range tasks {
		buf.WriteString(fmt.Sprintf("- [%s] %s (type=%s, priority=%d)\n",
			t.Status, t.Title, t.TaskType, t.Priority))
	}
	return buf.String()
}

func execGetTask(ctx context.Context, db *backend.TaskRepo, execDB *backend.ExecutionRepo, argsJSON string) string {
	var args struct{ TaskID string `json:"task_id"` }
	json.Unmarshal([]byte(argsJSON), &args)
	task, err := db.Get(args.TaskID)
	if err != nil {
		return fmt.Sprintf("任务不存在或查询失败: %v", err)
	}
	execs, _ := execDB.ListByTask(args.TaskID, 5)
	var execInfo string
	if len(execs) > 0 {
		for _, e := range execs {
			score := float64(0)
			if e.EvaluationScore != nil {
				score = *e.EvaluationScore * 100
			}
			execInfo += fmt.Sprintf("\n  #%s: %s | %s | score=%.0f", e.ID, e.StartedAt.Format("01-02 15:04"), e.Status, score)
		}
	} else {
		execInfo = "\n  (无执行记录)"
	}
	return fmt.Sprintf("📋 %s\n状态: %s | 类型: %s | 优先级: %d\n描述: %s\n执行历史:%s",
		task.Title, task.Status, task.TaskType, task.Priority, task.Description, execInfo)
}

func execUpdateTask(ctx context.Context, db *backend.TaskRepo, argsJSON string) string {
	var args struct {
		TaskID      string `json:"task_id"`
		Status      string `json:"status"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Acceptance  string `json:"acceptance"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	task, err := db.Get(args.TaskID)
	if err != nil {
		return fmt.Sprintf("任务不存在: %v", err)
	}
	if args.Status != "" {
		task.Status = args.Status
	}
	if args.Title != "" {
		task.Title = args.Title
	}
	if args.Description != "" {
		task.Description = args.Description
	}
	if args.Acceptance != "" {
		task.Acceptance = args.Acceptance
	}
	if err := db.Update(task); err != nil {
		return fmt.Sprintf("更新失败: %v", err)
	}
	return fmt.Sprintf("✅ 已更新任务: %s", task.Title)
}

func execRunTask(ctx context.Context, db *backend.TaskRepo, execDB *backend.ExecutionRepo, agentDB *backend.AgentRepo, argsJSON string) string {
	var args struct {
		TaskID  string `json:"task_id"`
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	task, err := db.Get(args.TaskID)
	if err != nil {
		return fmt.Sprintf("任务不存在: %v", err)
	}
	// Verify task is not already running (check executions)
	execs, _ := execDB.ListByTask(args.TaskID, 1)
	if len(execs) > 0 && execs[0].Status == "running" {
		return fmt.Sprintf("⚠️ 任务 %s 正在执行中（execution_id=%s），请等待完成或取消后再触发", task.Title, execs[0].ID)
	}
	// Call local server API to trigger execution
	body, _ := json.Marshal(map[string]any{"agent_id": args.AgentID})
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/tasks/%s/run", addr, args.TaskID)
	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(body))
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("触发任务执行失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != 202 {
		return fmt.Sprintf("触发任务执行失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var result map[string]any
	json.Unmarshal(respBody, &result)
	execID, _ := result["execution_id"].(string)
	if execID == "" {
		return fmt.Sprintf("✅ 任务执行已触发: %s", task.Title)
	}
	return fmt.Sprintf("✅ 任务执行已触发: %s (execution_id=%s)", task.Title, execID)
}

func execGetTaskExecutions(ctx context.Context, db *backend.TaskRepo, execDB *backend.ExecutionRepo, argsJSON string) string {
	var args struct {
		TaskID string `json:"task_id"`
		Limit  int    `json:"limit"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.Limit <= 0 {
		args.Limit = 10
	}
	task, err := db.Get(args.TaskID)
	if err != nil {
		return fmt.Sprintf("任务不存在: %v", err)
	}
	execs, err := execDB.ListByTask(args.TaskID, args.Limit)
	if err != nil {
		return fmt.Sprintf("查询执行历史失败: %v", err)
	}
	if len(execs) == 0 {
		return fmt.Sprintf("📋 %s — 无执行记录", task.Title)
	}
	var buf bytes.Buffer
	for _, e := range execs {
		score := float64(0)
		if e.EvaluationScore != nil {
			score = *e.EvaluationScore * 100
		}
		buf.WriteString(fmt.Sprintf("- #%s | %s | %s | score=%.0f%%\n",
			e.ID, e.StartedAt.Format("2006-01-02 15:04"), e.Status, score))
	}
	return fmt.Sprintf("📋 %s — 执行历史:\n%s", task.Title, buf.String())
}

// ── Dir Shortcuts ──────────────────────────────────────────────────────────

func execCreateDirShortcut(ctx context.Context, dirDB *backend.DirShortcutRepo, argsJSON string) string {
	var args struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Path        string `json:"path"`
		RemoteHost  string `json:"remote_host"`
		RemoteUser  string `json:"remote_user"`
		RemotePath  string `json:"remote_path"`
		AuthMethod  string `json:"auth_method"`
		KeyPath     string `json:"key_path"`
		TerminalCmd string `json:"terminal_cmd"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.Name == "" {
		return "⚠️ name 是必填字段"
	}
	if args.Type == "remote" && args.RemoteHost == "" {
		return "⚠️ remote 类型快捷方式需要 remote_host"
	}
	if args.Type != "remote" && args.Path == "" {
		return "⚠️ local 类型快捷方式需要 path"
	}
	shortcut := &backend.DirShortcut{
		ID:             newUUID(),
		Name:           args.Name,
		Type:           args.Type,
		Path:           args.Path,
		RemoteHost:     args.RemoteHost,
		RemoteUser:     args.RemoteUser,
		RemotePath:     args.RemotePath,
		AuthMethod:     args.AuthMethod,
		KeyPath:        args.KeyPath,
		TerminalCmd:    args.TerminalCmd,
		SortOrder:      dirDB.NextSortOrder(),
	}
	if err := dirDB.Create(shortcut); err != nil {
		return fmt.Sprintf("创建失败: %v", err)
	}
	return fmt.Sprintf("✅ 目录快捷方式已创建: %s (ID: %s, type=%s)", args.Name, shortcut.ID, args.Type)
}

func execListDirShortcuts(ctx context.Context, dirDB *backend.DirShortcutRepo, argsJSON string) string {
	var args struct{ Type string `json:"type"` }
	json.Unmarshal([]byte(argsJSON), &args)
	list, err := dirDB.List()
	if err != nil {
		return fmt.Sprintf("查询失败: %v", err)
	}
	if len(list) == 0 {
		return "📁 无目录快捷方式"
	}
	var buf bytes.Buffer
	for _, d := range list {
		if args.Type != "" && d.Type != args.Type {
			continue
		}
		loc := d.Path
		if d.Type == backend.DirShortcutTypeRemote {
			loc = fmt.Sprintf("%s@%s:%s", d.RemoteUser, d.RemoteHost, d.RemotePath)
		}
		buf.WriteString(fmt.Sprintf("- [%s] %s | %s\n", d.Type, d.Name, loc))
	}
	if buf.Len() == 0 {
		return "📁 无目录快捷方式（筛选结果为空）"
	}
	return "📁 目录快捷方式列表:\n" + buf.String()
}

func execUpdateDirShortcut(ctx context.Context, dirDB *backend.DirShortcutRepo, argsJSON string) string {
	var args struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Path         string `json:"path"`
		Type         string `json:"type"`
		RemoteHost   string `json:"remote_host"`
		RemoteUser   string `json:"remote_user"`
		RemotePath   string `json:"remote_path"`
		AuthMethod   string `json:"auth_method"`
		KeyPath      string `json:"key_path"`
		TerminalCmd  string `json:"terminal_cmd"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.ID == "" {
		return "⚠️ id 是必填字段"
	}
	existing, err := dirDB.GetByID(args.ID)
	if err != nil || existing == nil {
		return fmt.Sprintf("快捷方式不存在: %s", args.ID)
	}
	if args.Name != "" {
		existing.Name = args.Name
	}
	if args.Path != "" {
		existing.Path = args.Path
	}
	if args.Type != "" {
		existing.Type = args.Type
	}
	if args.RemoteHost != "" {
		existing.RemoteHost = args.RemoteHost
	}
	if args.RemoteUser != "" {
		existing.RemoteUser = args.RemoteUser
	}
	if args.RemotePath != "" {
		existing.RemotePath = args.RemotePath
	}
	if args.AuthMethod != "" {
		existing.AuthMethod = args.AuthMethod
	}
	if args.KeyPath != "" {
		existing.KeyPath = args.KeyPath
	}
	if args.TerminalCmd != "" {
		existing.TerminalCmd = args.TerminalCmd
	}
	if err := dirDB.Update(existing); err != nil {
		return fmt.Sprintf("更新失败: %v", err)
	}
	return fmt.Sprintf("✅ 已更新: %s", existing.Name)
}

func execDeleteDirShortcut(ctx context.Context, dirDB *backend.DirShortcutRepo, argsJSON string) string {
	var args struct{ ID string `json:"id"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if args.ID == "" {
		return "⚠️ id 是必填字段"
	}
	if err := dirDB.Delete(args.ID); err != nil {
		return fmt.Sprintf("删除失败: %v", err)
	}
	return fmt.Sprintf("✅ 已删除快捷方式: %s", args.ID)
}

func execOpenDirShortcut(ctx context.Context, dirDB *backend.DirShortcutRepo, argsJSON string) string {
	var args struct{ ID string `json:"id"` }
	json.Unmarshal([]byte(argsJSON), &args)
	entry, err := dirDB.GetByID(args.ID)
	if err != nil || entry == nil {
		return fmt.Sprintf("快捷方式不存在: %s", args.ID)
	}
	if entry.Type == backend.DirShortcutTypeRemote {
		return "⚠️ remote 类型快捷方式请使用 open_dir_shortcut_terminal"
	}
	// Import shortcuts package dynamically to avoid import cycle.
	// We call the API endpoint instead.
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/dir-shortcuts/%s/open", addr, args.ID)
	req, _ := http.NewRequestWithContext(ctx, "POST", u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("打开目录失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("打开目录失败 (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return fmt.Sprintf("✅ 已在文件管理器中打开: %s", entry.Path)
}

func execOpenDirShortcutTerminal(ctx context.Context, dirDB *backend.DirShortcutRepo, argsJSON string) string {
	var args struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	entry, err := dirDB.GetByID(args.ID)
	if err != nil || entry == nil {
		return fmt.Sprintf("快捷方式不存在: %s", args.ID)
	}
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/dir-shortcuts/%s/open-terminal", addr, args.ID)
	if args.Type != "" {
		u += "?type=" + url.QueryEscape(args.Type)
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("打开终端失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("打开终端失败 (HTTP %d): %s", resp.StatusCode, string(body))
	}
	what := entry.Path
	if entry.Type == backend.DirShortcutTypeRemote {
		what = fmt.Sprintf("%s@%s:%s", entry.RemoteUser, entry.RemoteHost, entry.RemotePath)
	}
	return fmt.Sprintf("✅ 已在终端中打开: %s", what)
}

// ── Experiences ─────────────────────────────────────────────────────────────

func execSearchExperiences(ctx context.Context, expDB *backend.ExperienceRepo, argsJSON string) string {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.Limit <= 0 {
		args.Limit = 10
	}
	results, _ := expDB.Search(args.Query)
	if len(results) == 0 {
		return fmt.Sprintf("🔍 经验库搜索 '%s': 无结果", args.Query)
	}
	var buf bytes.Buffer
	for _, e := range results {
		buf.WriteString(fmt.Sprintf("- [%s] %s (%s)\n", e.Module, e.Scene, e.ID))
	}
	return fmt.Sprintf("🔍 搜索 '%s' (%d 结果):\n%s", args.Query, len(results), buf.String())
}

func execCreateExperience(ctx context.Context, expDB *backend.ExperienceRepo, argsJSON string) string {
	var args struct {
		Module   string `json:"module"`
		Keywords string `json:"keywords"`
		Scene    string `json:"scene"`
		Details  string `json:"details"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.Module == "" {
		return "⚠️ module 是必填字段"
	}
	exp := &backend.Experience{
		ID:        newUUID(),
		Module:    args.Module,
		Keywords:  args.Keywords,
		Scene:     args.Scene,
		Details:   args.Details,
		Version:   "v1.0.0",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := expDB.Create(exp); err != nil {
		return fmt.Sprintf("创建经验失败: %v", err)
	}
	return fmt.Sprintf("✅ 经验已创建: [%s] %s (ID: %s)", args.Module, args.Scene, exp.ID)
}

func execUpdateExperience(ctx context.Context, expDB *backend.ExperienceRepo, argsJSON string) string {
	var args struct {
		ID       string `json:"id"`
		Module   string `json:"module"`
		Keywords string `json:"keywords"`
		Scene    string `json:"scene"`
		Details  string `json:"details"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.ID == "" {
		return "⚠️ id 是必填字段"
	}
	exp, err := expDB.Get(args.ID)
	if err != nil {
		return fmt.Sprintf("经验不存在: %v", err)
	}
	if args.Module != "" {
		exp.Module = args.Module
	}
	if args.Keywords != "" {
		exp.Keywords = args.Keywords
	}
	if args.Scene != "" {
		exp.Scene = args.Scene
	}
	if args.Details != "" {
		exp.Details = args.Details
	}
	exp.UpdatedAt = time.Now()
	if err := expDB.Update(exp); err != nil {
		return fmt.Sprintf("更新经验失败: %v", err)
	}
	return fmt.Sprintf("✅ 经验已更新: [%s] %s", exp.Module, exp.Scene)
}

func execDeleteExperience(ctx context.Context, expDB *backend.ExperienceRepo, argsJSON string) string {
	var args struct{ ID string `json:"id"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if args.ID == "" {
		return "⚠️ id 是必填字段"
	}
	if err := expDB.Delete(args.ID); err != nil {
		return fmt.Sprintf("删除经验失败: %v", err)
	}
	return fmt.Sprintf("✅ 经验已删除: %s", args.ID)
}

// ── Web Links ─────────────────────────────────────────────────────────────

func execListWebLinks(ctx context.Context, linkDB *backend.WebLinkRepo, argsJSON string) string {
	list, err := linkDB.List()
	if err != nil {
		return fmt.Sprintf("查询失败: %v", err)
	}
	if len(list) == 0 {
		return "🔗 无收藏链接"
	}
	var buf bytes.Buffer
	for _, l := range list {
		buf.WriteString(fmt.Sprintf("- %s | %s\n", l.Name, l.URL))
	}
	return "🔗 收藏链接:\n" + buf.String()
}

func execCreateWebLink(ctx context.Context, linkDB *backend.WebLinkRepo, argsJSON string) string {
	var args struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		IconURL   string `json:"icon_url"`
		SortOrder int    `json:"sort_order"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.Name == "" || args.URL == "" {
		return "⚠️ name 和 url 是必填字段"
	}
	sortOrder := args.SortOrder
	if sortOrder == 0 {
		sortOrder = linkDB.NextSortOrder()
	}
	link := &backend.WebLink{
		ID:        newUUID(),
		Name:      args.Name,
		URL:       args.URL,
		IconURL:   args.IconURL,
		SortOrder: sortOrder,
		CreatedAt: time.Now(),
	}
	if err := linkDB.Create(link); err != nil {
		return fmt.Sprintf("创建失败: %v", err)
	}
	return fmt.Sprintf("✅ 收藏链接已添加: %s → %s", args.Name, args.URL)
}

func execUpdateWebLink(ctx context.Context, linkDB *backend.WebLinkRepo, argsJSON string) string {
	var args struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		URL       string `json:"url"`
		IconURL   string `json:"icon_url"`
		SortOrder int    `json:"sort_order"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.ID == "" {
		return "⚠️ id 是必填字段"
	}
	link := &backend.WebLink{
		ID:   args.ID,
		Name: args.Name,
		URL:  args.URL,
	}
	if err := linkDB.Update(link); err != nil {
		return fmt.Sprintf("更新失败: %v", err)
	}
	return fmt.Sprintf("✅ 收藏链接已更新: %s", args.Name)
}

func execDeleteWebLink(ctx context.Context, linkDB *backend.WebLinkRepo, argsJSON string) string {
	var args struct{ ID string `json:"id"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if args.ID == "" {
		return "⚠️ id 是必填字段"
	}
	if err := linkDB.Delete(args.ID); err != nil {
		return fmt.Sprintf("删除失败: %v", err)
	}
	return fmt.Sprintf("✅ 收藏链接已删除: %s", args.ID)
}

func execOpenWebLink(ctx context.Context, argsJSON string) string {
	var args struct{ URL string `json:"url"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if args.URL == "" {
		return "⚠️ url 是必填字段"
	}
	addr := getServerAddr()
	body, _ := json.Marshal(map[string]string{"url": args.URL})
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://127.0.0.1%s/api/links/open", addr), bytes.NewReader(body))
	if err != nil {
		return fmt.Sprintf("请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("打开链接失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("打开链接失败 (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return fmt.Sprintf("✅ 已在浏览器中打开: %s", args.URL)
}

// ── Todo ─────────────────────────────────────────────────────────────────────

func todoMDPath() string {
	if cfg := config.Get(); cfg != nil {
		return cfg.TodoMDPath
	}
	return ""
}

func execListTodos(ctx context.Context, argsJSON string) string {
	path := todoMDPath()
	if path == "" {
		return "⚠️ Todo 路径未配置（todo_md_path）"
	}
	items, err := todo.ReadAndParse(path)
	if err != nil {
		return fmt.Sprintf("读取 Todo 失败: %v", err)
	}
	if len(items) == 0 {
		return "📝 Todo 列表为空"
	}
	var buf bytes.Buffer
	for _, item := range items {
		done := " "
		if item.Done {
			done = "x"
		}
		buf.WriteString(fmt.Sprintf("[%s] %s (line %d)\n", done, item.Text, item.LineNo))
	}
	return "📝 Todo 列表:\n" + buf.String()
}

func execAddTodo(ctx context.Context, argsJSON string) string {
	var args struct{ Text string `json:"text"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if args.Text == "" {
		return "⚠️ text 是必填字段"
	}
	path := todoMDPath()
	if path == "" {
		return "⚠️ Todo 路径未配置（todo_md_path）"
	}
	if err := todo.AddAndWrite(path, args.Text); err != nil {
		return fmt.Sprintf("添加失败: %v", err)
	}
	return fmt.Sprintf("✅ 已添加: %s", args.Text)
}

func execToggleTodo(ctx context.Context, argsJSON string) string {
	var args struct {
		LineNo int  `json:"line_no"`
		Done   bool `json:"done"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.LineNo <= 0 {
		return "⚠️ line_no 是必填字段且必须 > 0"
	}
	path := todoMDPath()
	if path == "" {
		return "⚠️ Todo 路径未配置（todo_md_path）"
	}
	items, err := todo.ReadAndParse(path)
	if err != nil {
		return fmt.Sprintf("读取失败: %v", err)
	}
	var found bool
	for i := range items {
		if items[i].LineNo == args.LineNo {
			items[i].Done = args.Done
			found = true
			break
		}
	}
	if !found {
		return fmt.Sprintf("⚠️ 未找到 line_no=%d 的项目", args.LineNo)
	}
	if err := todo.ToggleAndWrite(path, items); err != nil {
		return fmt.Sprintf("更新失败: %v", err)
	}
	status := "已完成"
	if !args.Done {
		status = "未完成"
	}
	return fmt.Sprintf("✅ 已标记为 [%s]: %s", status, items[args.LineNo-1].Text)
}

func execDeleteTodo(ctx context.Context, argsJSON string) string {
	var args struct{ LineNo int `json:"line_no"` }
	json.Unmarshal([]byte(argsJSON), &args)
	if args.LineNo <= 0 {
		return "⚠️ line_no 是必填字段且必须 > 0"
	}
	path := todoMDPath()
	if path == "" {
		return "⚠️ Todo 路径未配置（todo_md_path）"
	}
	if err := todo.DeleteAndWrite(path, args.LineNo); err != nil {
		return fmt.Sprintf("删除失败: %v", err)
	}
	return fmt.Sprintf("✅ 已删除 line %d", args.LineNo)
}

// ── Local Shell ─────────────────────────────────────────────────────────────

type LocalShellState struct {
	Active     bool
	CLIType    string
	PtySession *PtySession // reuse existing PTY session type
}

func execStartLocalShell(ctx context.Context, state *LocalShellState, argsJSON string) string {
	var args struct {
		CLIType string `json:"cli_type"`
		CWD     string `json:"cwd"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.CLIType != "claude" && args.CLIType != "cbc" {
		return "⚠️ cli_type 必须是 claude 或 cbc"
	}
	state.Active = true
	state.CLIType = args.CLIType
	return fmt.Sprintf("✅ 本地 %s 会话已启动 (cwd=%s)", args.CLIType, args.CWD)
}

func execRunLocalCommand(ctx context.Context, state *LocalShellState, argsJSON string) string {
	var args struct {
		Command  string `json:"command"`
		CLIType  string `json:"cli_type"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if !state.Active {
		return "⚠️ 无活跃的本地 Shell 会话，请先调用 start_local_shell"
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(args.CLIType, "--print", args.Command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Sprintf("❌ 执行失败: %v\nSTDERR: %s", err, stderr.String())
	}
	return fmt.Sprintf("✅ 输出:\n%s", stdout.String())
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func getServerAddr() string {
	// Prefer ADDR env var; fall back to package-level default.
	if a := getEnv("ADDR"); a != "" {
		return a
	}
	return serverAddr
}

func getEnv(k string) string {
	return os.Getenv(k)
}

// uuid shim
func newUUID() string {
	b := make([]byte, 16)
	hex := "0123456789abcdef"
	for i := range b {
		b[i] = hex[time.Now().UnixNano()%16+1]
		time.Sleep(time.Nanosecond)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func randomID(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// --- Re-export PtySession for use in LocalShellState (avoids import cycle) ---
type PtySession = interface{}
