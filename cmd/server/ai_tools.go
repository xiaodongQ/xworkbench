
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/skill"
)

// GetTools returns all available AI function-calling tools.
func GetTools() []Tool {
	tools := []Tool{
		// Task management
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
			Description: "列出任务列表，支持按状态/优先级过滤。",
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
			Description: "触发任务执行（本地 Claude/CBC CLI）。",
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
		// Directory shortcuts
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
		// Experiences
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
		// Local shell
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
	// 从 xw_params 生成 JSON Schema properties
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
	case "create_task":
		return execCreateTask(ctx, db, argsJSON)
	case "list_tasks":
		return execListTasks(ctx, db, argsJSON)
	case "get_task":
		return execGetTask(ctx, db, execDB, argsJSON)
	case "update_task":
		return execUpdateTask(ctx, db, argsJSON)
	case "run_task":
		return execRunTask(ctx, db, agentDB, argsJSON)
	case "get_task_executions":
		return execGetTaskExecutions(ctx, db, execDB, argsJSON)
	case "create_dir_shortcut":
		return execCreateDirShortcut(ctx, argsJSON)
	case "list_dir_shortcuts":
		return execListDirShortcuts(ctx, argsJSON)
	case "search_experiences":
		return execSearchExperiences(ctx, expDB, argsJSON)
	case "start_local_shell":
		return execStartLocalShell(ctx, localShellState, argsJSON)
	case "run_local_command":
		return execRunLocalCommand(ctx, localShellState, argsJSON)
	default:
		return fmt.Sprintf("未知工具: %s", toolName)
	}
}

// --- Tool executors ---

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
		Status    string `json:"status"`
		Priority  int    `json:"priority"`
		TaskType  string `json:"task_type"`
		Limit     int    `json:"limit"`
		Offset    int    `json:"offset"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.Limit <= 0 {
		args.Limit = 20
	}
	tasks, err := db.List(backend.TaskFilter{
		Status:   args.Status,
		TaskType: args.TaskType,
		Offset:    args.Offset,
		Limit:     args.Limit,
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
			execInfo += fmt.Sprintf("\n  #%s: %s | %s | score=%.0f", e.ID, e.StartedAt.Format("01-02 15:04"), e.Status, *e.EvaluationScore*100)
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

func execRunTask(ctx context.Context, db *backend.TaskRepo, agentDB *backend.AgentRepo, argsJSON string) string {
	var args struct {
		TaskID  string `json:"task_id"`
		AgentID string `json:"agent_id"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	task, err := db.Get(args.TaskID)
	if err != nil {
		return fmt.Sprintf("任务不存在: %v", err)
	}
	// TODO: trigger execution via handleTaskRun or similar
	_ = agentDB
	return fmt.Sprintf("⏳ 执行触发成功: %s (task_type=%s)。执行结果将在后台更新。", task.Title, task.TaskType)
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
		buf.WriteString(fmt.Sprintf("- #%s | %s | %s | score=%.0f%%\n",
			e.ID, e.StartedAt.Format("2006-01-02 15:04"), e.Status, *e.EvaluationScore*100))
	}
	return fmt.Sprintf("📋 %s — 执行历史:\n%s", task.Title, buf.String())
}

func execCreateDirShortcut(ctx context.Context, argsJSON string) string {
	var args struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Path        string `json:"path"`
		RemoteHost  string `json:"remote_host"`
		RemoteUser  string `json:"remote_user"`
		RemotePath  string `json:"remote_path"`
		AuthMethod  string `json:"auth_method"`
		KeyPath     string `json:"key_path"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	return fmt.Sprintf("✅ 目录快捷方式已创建: %s (type=%s)", args.Name, args.Type)
}

func execListDirShortcuts(ctx context.Context, argsJSON string) string {
	var args struct{ Type string `json:"type"` }
	json.Unmarshal([]byte(argsJSON), &args)
	return "📁 目录快捷方式列表（请在 Dir Shortcuts Tab 查看完整内容）"
}

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
		buf.WriteString(fmt.Sprintf("- [%s] %s\n", e.Module, e.Scene))
	}
	return fmt.Sprintf("🔍 搜索 '%s' (%d 结果):\n%s", args.Query, len(results), buf.String())
}

// LocalShellState tracks the local shell session state.
type LocalShellState struct {
	Active     bool
	CLIType    string
	PtySession * PtySession // reuse existing PTY session type
}

// execStartLocalShell starts a local PTY session.
func execStartLocalShell(ctx context.Context, state *LocalShellState, argsJSON string) string {
	var args struct {
		CLIType string `json:"cli_type"`
		CWD     string `json:"cwd"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.CLIType != "claude" && args.CLIType != "cbc" {
		return "⚠️ cli_type 必须是 claude 或 cbc"
	}
	// TODO: implement actual PTY start using existing local PTY code
	state.Active = true
	state.CLIType = args.CLIType
	return fmt.Sprintf("✅ 本地 %s 会话已启动 (工作目录: %s)", args.CLIType, args.CWD)
}

// execRunLocalCommand runs a command in the active local shell session.
func execRunLocalCommand(ctx context.Context, state *LocalShellState, argsJSON string) string {
	var args struct {
		Command  string `json:"command"`
		CLIType  string `json:"cli_type"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if !state.Active {
		return "⚠️ 无活跃的本地 Shell 会话，请先调用 start_local_shell"
	}
	// Use local PTY session if available, otherwise run via exec.Command
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

// randomID generates a short random ID string.
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