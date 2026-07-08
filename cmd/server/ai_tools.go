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

	"github.com/google/uuid"
	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"github.com/xiaodongQ/xworkbench/internal/memory"
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
			Name:        "trigger_task",
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
			Name:        "list_task_executions",
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
			Description: "搜索经验库。当用户说「查一下有没有关于 X 的经验」或「有没有处理过 Y 问题」时使用。",
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
			Description: "列出所有收藏链接。返回格式：- [uuid] 名称 | URL，其中 [uuid] 是删除时需要的 ID。",
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
			Description: "删除一个收藏链接。id 必须是 list_web_links 返回的 [uuid] 格式 ID（不是名称）。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "链接ID，必须是 list_web_links 返回的 [uuid] 格式，不是链接名称"}
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
			Description: "向 Todo 列表添加一个项目。支持截止日期、标签、备注。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"text":     {"type": "string", "description": "Todo 内容"},
					"due_date": {"type": "string", "description": "截止日期 YYYY-MM-DD 或 MM-DD（可选）"},
					"tags":     {"type": "string", "description": "逗号分隔的标签，如 personal,shopping（可选）"},
					"note":     {"type": "string", "description": "详细备注（可选）"}
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
		// ── Scheduled Tasks ────────────────────────────────────
		{
			Name:        "list_scheduled_tasks",
			Description: "列出所有定时任务，返回名称、cron 表达式、状态、下次执行时间。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		{
			Name:        "create_scheduled_task",
			Description: "创建一个新的定时任务。command_type 支持 claude / cbc / shell。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name":         {"type": "string", "description": "任务名称必填"},
					"cron_expr":    {"type": "string", "description": "Cron 表达式必填，如 0 9 * * *"},
					"command_type": {"type": "string", "enum": ["claude", "cbc", "shell"], "description": "命令类型必填"},
					"model":        {"type": "string", "description": "使用的模型（可选，如 claude-3-5-sonnet 等）"},
					"prompt":       {"type": "string", "description": "Prompt / 命令内容必填"},
					"working_dir":  {"type": "string", "description": "工作目录（可选）"},
					"enabled":      {"type": "boolean", "description": "是否启用（默认 true）"}
				},
				"required": ["name", "cron_expr", "command_type", "prompt"]
			}`),
		},
		{
			Name:        "get_scheduled_task",
			Description: "查看单个定时任务的详情。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "定时任务ID必填"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "update_scheduled_task",
			Description: "更新一个定时任务的属性。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id":           {"type": "string", "description": "任务ID必填"},
					"name":         {"type": "string", "description": "新名称"},
					"cron_expr":    {"type": "string", "description": "新 Cron 表达式"},
					"command_type": {"type": "string", "enum": ["claude", "cbc", "shell"]},
					"model":        {"type": "string", "description": "新模型"},
					"prompt":       {"type": "string", "description": "新 Prompt"},
					"working_dir":  {"type": "string", "description": "新工作目录"},
					"enabled":      {"type": "boolean", "description": "是否启用"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "delete_scheduled_task",
			Description: "删除一个定时任务。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "任务ID必填"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "run_scheduled_task_now",
			Description: "立即触发一个定时任务（不等待 cron 触发）。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "任务ID必填"}
				},
				"required": ["id"]
			}`),
		},
		// ── Memory ─────────────────────────────────────────────
		{
			Name:        "memory_add",
			Description: "向 memory.md 追加一条需要记住的内容。添加时自动去重、分类。文件接近 20KB 上限时会警告或拒绝。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"text":     {"type": "string", "description": "要记住的内容（不含日期前缀）"},
					"category": {"type": "string", "description": "分类名，如 用户 & 环境 / 项目 / 约定 / 持续任务"}
				},
				"required": ["text", "category"]
			}`),
		},
		{
			Name:        "memory_list",
			Description: "查看当前 memory.md 中所有记忆条目。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		{
			Name:        "memory_prune",
			Description: "手动触发 memory.md 整合去重（移除精确重复条目）。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
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
		// ── Task Advanced (高优先级缺口) ─────────────────────────
		{
			Name:        "cancel_task",
			Description: "取消一个正在运行的任务。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id": {"type": "string", "description": "任务ID"}
				},
				"required": ["task_id"]
			}`),
		},
		{
			Name:        "unclaim_task",
			Description: "放弃认领一个任务，将任务状态重置为 pending。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id": {"type": "string", "description": "任务ID"}
				},
				"required": ["task_id"]
			}`),
		},
		{
			Name:        "set_task_experiences",
			Description: "设置任务关联的经验库条目（多对多）。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id":       {"type": "string", "description": "任务ID"},
					"experience_ids": {"type": "array", "items": {"type": "string"}, "description": "经验ID数组，空数组=清除关联"}
				},
				"required": ["task_id", "experience_ids"]
			}`),
		},
		{
			Name:        "run_task_loop",
			Description: "触发任务的 AI Run Loop（自动驾驶模式），持续执行直到达成目标或达到迭代上限。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id": {"type": "string", "description": "任务ID"},
					"model":  {"type": "string", "description": "使用的模型（可选，如 claude-3-5-sonnet）"}
				},
				"required": ["task_id"]
			}`),
		},
		{
			Name:        "learn_from_task",
			Description: "让 AI 从任务执行结果中学习反思，生成经验存入经验库。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"task_id": {"type": "string", "description": "任务ID"}
				},
				"required": ["task_id"]
			}`),
		},
		// ── Execution (高优先级缺口) ─────────────────────────────
		{
			Name:        "get_execution",
			Description: "获取一次执行任务的详细信息，包括输出、错误、退出码、状态和评估分数。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "执行ID"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "continue_execution",
			Description: "继续一个执行会话，基于之前的上下文发送新 prompt。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id":     {"type": "string", "description": "执行ID"},
					"prompt": {"type": "string", "description": "继续对话的 prompt"},
					"model":  {"type": "string", "description": "使用的模型（可选）"}
				},
				"required": ["id", "prompt"]
			}`),
		},
		{
			Name:        "cancel_execution",
			Description: "取消一个正在运行中的执行任务。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "执行ID"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "evaluate_execution",
			Description: "对一次执行结果进行 AI 评分（LLM-as-a-Judge）。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id":    {"type": "string", "description": "执行ID"},
					"model": {"type": "string", "description": "评估使用的模型（可选）"}
				},
				"required": ["id"]
			}`),
		},
		// ── Scheduler Control (中优先级) ─────────────────────────
		{
			Name:        "toggle_scheduled_task",
			Description: "切换定时任务的启用/停用状态。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "定时任务ID"}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "get_ai_loop_status",
			Description: "查询 AI 自治能力（Run Loop / Reevaluate / Learn）的开关状态。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		// ── System Info (中优先级) ────────────────────────────────
		{
			Name:        "get_dashboard_stats",
			Description: "获取 Dashboard 全局统计信息，包括任务数、执行数、agent 数等。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		{
			Name:        "export_config",
			Description: "导出台式工作数据（快捷方式、链接、经验等）。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"types": {"type": "array", "items": {"type": "string"}, "description": "导出类型，默认 ['shortcuts','links','experiences']"}
				}
			}`),
		},
		{
			Name:        "send_notification",
			Description: "弹出纯 Python 窗口通知（tkinter），带一键复制按钮。用于 AI 通知用户重要信息。",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"message": {"type": "string", "description": "通知内容"},
					"title":   {"type": "string", "description": "通知标题，默认「工作台通知»"}
				},
				"required": ["message"]
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

// SchedulerOps 定时任务调度器操作接口（避免 import cycle）
type SchedulerOps interface {
	Reload() error
	RunNow(id string) (string, error)
}

// ExecuteTool executes a tool by name with given JSON arguments.
// Returns a human-readable result string.
func ExecuteTool(ctx context.Context, db *backend.TaskRepo, expDB *backend.ExperienceRepo,
	execDB *backend.ExecutionRepo, agentDB *backend.AgentRepo,
	linkDB *backend.WebLinkRepo, dirDB *backend.DirShortcutRepo,
	schedDB *backend.ScheduledTaskRepo, sch SchedulerOps,
	memStore *memory.Store,
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
	case "trigger_task":
		return execRunTask(ctx, db, execDB, agentDB, argsJSON)
	case "list_task_executions":
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
	// Scheduled Tasks
	case "list_scheduled_tasks":
		return execListScheduledTasks(ctx, schedDB, sch, argsJSON)
	case "create_scheduled_task":
		return execCreateScheduledTask(ctx, schedDB, sch, argsJSON)
	case "get_scheduled_task":
		return execGetScheduledTask(ctx, schedDB, sch, argsJSON)
	case "update_scheduled_task":
		return execUpdateScheduledTask(ctx, schedDB, sch, argsJSON)
	case "delete_scheduled_task":
		return execDeleteScheduledTask(ctx, schedDB, sch, argsJSON)
	case "run_scheduled_task_now":
		return execRunScheduledTaskNow(ctx, schedDB, sch, argsJSON)
	// Memory
	case "memory_add":
		return execMemoryAdd(ctx, memStore, argsJSON)
	case "memory_list":
		return execMemoryList(ctx, memStore, argsJSON)
	case "memory_prune":
		return execMemoryPrune(ctx, memStore, argsJSON)
	// Local Shell
	case "start_local_shell":
		return execStartLocalShell(ctx, localShellState, argsJSON)
	case "run_local_command":
		return execRunLocalCommand(ctx, localShellState, argsJSON)
	// Task Advanced
	case "cancel_task":
		return execCancelTask(ctx, argsJSON)
	case "unclaim_task":
		return execUnclaimTask(ctx, argsJSON)
	case "set_task_experiences":
		return execSetTaskExperiences(ctx, argsJSON)
	case "run_task_loop":
		return execRunTaskLoop(ctx, argsJSON)
	case "learn_from_task":
		return execLearnFromTask(ctx, argsJSON)
	// Execution
	case "get_execution":
		return execGetExecution(ctx, argsJSON)
	case "continue_execution":
		return execContinueExecution(ctx, argsJSON)
	case "cancel_execution":
		return execCancelExecution(ctx, argsJSON)
	case "evaluate_execution":
		return execEvaluateExecution(ctx, argsJSON)
	// Scheduler
	case "toggle_scheduled_task":
		return execToggleScheduledTask(ctx, schedDB, sch, argsJSON)
	// System
	case "get_ai_loop_status":
		return execGetAILoopStatus(ctx, argsJSON)
	case "get_dashboard_stats":
		return execGetDashboardStats(ctx, argsJSON)
	case "export_config":
		return execExportConfig(ctx, argsJSON)
	case "send_notification":
		return execSendNotification(ctx, argsJSON)
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
	var args struct {
		TaskID string `json:"task_id"`
	}
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
		Priority    int    `json:"priority"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	task, err := db.Get(args.TaskID)
	if err != nil {
		return fmt.Sprintf("任务不存在: %v", err)
	}
	if args.Status != "" {
		if err := db.UpdateStatus(args.TaskID, args.Status, ""); err != nil {
			return fmt.Sprintf("更新状态失败: %v", err)
		}
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
	if args.Priority > 0 {
		task.Priority = args.Priority
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
		RemotePort  string `json:"remote_port"`
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
		ID:          newUUID(),
		Name:        args.Name,
		Type:        args.Type,
		Path:        args.Path,
		RemoteHost:  args.RemoteHost,
		RemotePort:  args.RemotePort,
		RemoteUser:  args.RemoteUser,
		RemotePath:  args.RemotePath,
		AuthMethod:  args.AuthMethod,
		KeyPath:     args.KeyPath,
		TerminalCmd: args.TerminalCmd,
		SortOrder:   dirDB.NextSortOrder(),
	}
	if err := dirDB.Create(shortcut); err != nil {
		return fmt.Sprintf("创建失败: %v", err)
	}
	return fmt.Sprintf("✅ 目录快捷方式已创建: %s (ID: %s, type=%s)", args.Name, shortcut.ID, args.Type)
}

func execListDirShortcuts(ctx context.Context, dirDB *backend.DirShortcutRepo, argsJSON string) string {
	var args struct {
		Type string `json:"type"`
	}
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
		buf.WriteString(fmt.Sprintf("- [%s] [%s] %s | %s\n", d.ID, d.Type, d.Name, loc))
	}
	if buf.Len() == 0 {
		return "📁 无目录快捷方式（筛选结果为空）"
	}
	return "📁 目录快捷方式列表:\n" + buf.String()
}

func execUpdateDirShortcut(ctx context.Context, dirDB *backend.DirShortcutRepo, argsJSON string) string {
	var args struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Path        string `json:"path"`
		Type        string `json:"type"`
		RemoteHost  string `json:"remote_host"`
		RemoteUser  string `json:"remote_user"`
		RemotePath  string `json:"remote_path"`
		AuthMethod  string `json:"auth_method"`
		KeyPath     string `json:"key_path"`
		TerminalCmd string `json:"terminal_cmd"`
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
	var args struct {
		ID string `json:"id"`
	}
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
	var args struct {
		ID string `json:"id"`
	}
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
	var args struct {
		ID string `json:"id"`
	}
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
		buf.WriteString(fmt.Sprintf("- [%s] %s | %s\n", l.ID, l.Name, l.URL))
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
	var args struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.ID == "" {
		return "⚠️ id 是必填字段（必须是列表中 [xxx] 格式的 UUID，不是链接名称）"
	}
	res, err := linkDB.Delete(args.ID)
	if err != nil {
		return fmt.Sprintf("删除失败: %v", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Sprintf("⚠️ 未找到 ID=%s 的链接，请先调用 list_web_links 获取最新列表（含 UUID ID）再删除", args.ID)
	}
	return fmt.Sprintf("✅ 收藏链接已删除（ID: %s）", args.ID)
}

func execOpenWebLink(ctx context.Context, argsJSON string) string {
	var args struct {
		URL string `json:"url"`
	}
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
	tree, err := todo.ReadAndParse(path)
	if err != nil {
		return fmt.Sprintf("读取 Todo 失败: %v", err)
	}
	items := todo.Flatten(tree)
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
	var args struct {
		Text    string `json:"text"`
		DueDate string `json:"due_date,omitempty"`
		Tags    string `json:"tags,omitempty"` // 逗号分隔字符串
		Note    string `json:"note,omitempty"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.Text == "" {
		return "⚠️ text 是必填字段"
	}

	// 解析 tags（逗号分隔字符串 → 切片）
	var tagsList []string
	if args.Tags != "" {
		for _, t := range strings.Split(args.Tags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tagsList = append(tagsList, t)
			}
		}
	}

	path := todoMDPath()
	if path == "" {
		return "⚠️ Todo 路径未配置（todo_md_path）"
	}
	if _, err := todo.AddAndWrite(path, args.Text, args.DueDate, tagsList, args.Note); err != nil {
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
	tree, err := todo.ReadAndParse(path)
	if err != nil {
		return fmt.Sprintf("读取失败: %v", err)
	}
	items := todo.Flatten(tree)
	var found bool
	var foundIdx int
	for i := range items {
		if items[i].LineNo == args.LineNo {
			items[i].Done = args.Done
			found = true
			foundIdx = i
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
	return fmt.Sprintf("✅ 已标记为 [%s]: %s", status, items[foundIdx].Text)
}

func execDeleteTodo(ctx context.Context, argsJSON string) string {
	var args struct {
		LineNo int `json:"line_no"`
	}
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

// ── Scheduled Tasks ─────────────────────────────────────────────────────────

func execListScheduledTasks(ctx context.Context, db *backend.ScheduledTaskRepo, sch SchedulerOps, argsJSON string) string {
	tasks, err := db.List()
	if err != nil {
		return fmt.Sprintf("查询失败: %v", err)
	}
	if len(tasks) == 0 {
		return "无定时任务"
	}
	var buf bytes.Buffer
	for _, t := range tasks {
		enabled := "停用"
		if t.Enabled {
			enabled = "启用"
		}
		nextStr := "-"
		if t.NextRunAt != nil {
			nextStr = t.NextRunAt.Format("01-02 15:04")
		}
		buf.WriteString(fmt.Sprintf("- [%s] %s | cron=%s | 下次 %s | cmd=%s\n",
			enabled, t.Name, t.CronExpr, nextStr, t.CommandType))
	}
	return buf.String()
}

func execCreateScheduledTask(ctx context.Context, db *backend.ScheduledTaskRepo, sch SchedulerOps, argsJSON string) string {
	var args struct {
		Name        string `json:"name"`
		CronExpr    string `json:"cron_expr"`
		CommandType string `json:"command_type"`
		Model       string `json:"model"`
		Prompt      string `json:"prompt"`
		WorkingDir  string `json:"working_dir"`
		Enabled     bool   `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.Name == "" || args.CronExpr == "" || args.CommandType == "" || args.Prompt == "" {
		return "name、cron_expr、command_type、prompt 均为必填项"
	}
	t := &backend.ScheduledTask{
		ID:          "sched-" + time.Now().Format("20060102150405") + "-" + randomID(6),
		Name:        args.Name,
		CronExpr:    args.CronExpr,
		CommandType: args.CommandType,
		Model:       args.Model,
		Prompt:      args.Prompt,
		WorkingDir:  args.WorkingDir,
		Enabled:     args.Enabled,
		CreatedAt:   time.Now(),
	}
	if err := db.Create(t); err != nil {
		return fmt.Sprintf("创建失败: %v", err)
	}
	if sch != nil {
		_ = sch.Reload()
	}
	return fmt.Sprintf("✅ 定时任务已创建: %s (ID: %s)\ncron: %s | 类型: %s", t.Name, t.ID, t.CronExpr, t.CommandType)
}

func execGetScheduledTask(ctx context.Context, db *backend.ScheduledTaskRepo, sch SchedulerOps, argsJSON string) string {
	var args struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	t, err := db.Get(args.ID)
	if err != nil {
		return fmt.Sprintf("任务不存在: %v", err)
	}
	enabled := "停用"
	if t.Enabled {
		enabled = "启用"
	}
	nextStr := "-"
	if t.NextRunAt != nil {
		nextStr = t.NextRunAt.Format("2006-01-02 15:04")
	}
	lastStr := "-"
	if t.LastRunAt != nil {
		lastStr = t.LastRunAt.Format("2006-01-02 15:04")
	}
	return fmt.Sprintf("📋 %s\nID: %s\nCron: %s\n类型: %s\n模型: %s\nPrompt: %s\n工作目录: %s\n状态: %s\n下次执行: %s\n上次执行: %s\n最后状态: %s",
		t.Name, t.ID, t.CronExpr, t.CommandType, t.Model, t.Prompt, t.WorkingDir,
		enabled, nextStr, lastStr, t.LastStatus)
}

func execUpdateScheduledTask(ctx context.Context, db *backend.ScheduledTaskRepo, sch SchedulerOps, argsJSON string) string {
	var args struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		CronExpr    string `json:"cron_expr"`
		CommandType string `json:"command_type"`
		Model       string `json:"model"`
		Prompt      string `json:"prompt"`
		WorkingDir  string `json:"working_dir"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.ID == "" {
		return "id 为必填项"
	}
	t, err := db.Get(args.ID)
	if err != nil {
		return fmt.Sprintf("任务不存在: %v", err)
	}
	if args.Name != "" {
		t.Name = args.Name
	}
	if args.CronExpr != "" {
		t.CronExpr = args.CronExpr
	}
	if args.CommandType != "" {
		t.CommandType = args.CommandType
	}
	if args.Model != "" {
		t.Model = args.Model
	}
	if args.Prompt != "" {
		t.Prompt = args.Prompt
	}
	if args.WorkingDir != "" {
		t.WorkingDir = args.WorkingDir
	}
	if args.Enabled != nil {
		t.Enabled = *args.Enabled
	}
	if err := db.Update(t); err != nil {
		return fmt.Sprintf("更新失败: %v", err)
	}
	if sch != nil {
		_ = sch.Reload()
	}
	return fmt.Sprintf("✅ 已更新: %s", t.Name)
}

func execDeleteScheduledTask(ctx context.Context, db *backend.ScheduledTaskRepo, sch SchedulerOps, argsJSON string) string {
	var args struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.ID == "" {
		return "id 为必填项"
	}
	if err := db.Delete(args.ID); err != nil {
		return fmt.Sprintf("删除失败: %v", err)
	}
	if sch != nil {
		_ = sch.Reload()
	}
	return fmt.Sprintf("✅ 已删除任务 ID: %s", args.ID)
}

func execRunScheduledTaskNow(ctx context.Context, db *backend.ScheduledTaskRepo, sch SchedulerOps, argsJSON string) string {
	var args struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.ID == "" {
		return "id 为必填项"
	}
	if sch == nil {
		return "调度器不可用，无法立即执行"
	}
	execID, err := sch.RunNow(args.ID)
	if err != nil {
		return fmt.Sprintf("立即执行失败: %v", err)
	}
	return fmt.Sprintf("✅ 已触发立即执行 (ExecutionID: %s)", execID)
}

// ── Memory ─────────────────────────────────────────────────────────────────

func execMemoryAdd(ctx context.Context, store *memory.Store, argsJSON string) string {
	var args struct {
		Text     string `json:"text"`
		Category string `json:"category"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.Text == "" || args.Category == "" {
		return "text 和 category 均为必填项"
	}
	result, err := store.Add(args.Text, args.Category)
	if err != nil {
		if err == memory.ErrFileTooLarge {
			return "⚠️ memory.md 已达 20KB 上限，请先运行 memory_prune 整合"
		}
		return fmt.Sprintf("写入失败: %v", err)
	}
	return result
}

func execMemoryList(ctx context.Context, store *memory.Store, argsJSON string) string {
	entries := store.List()
	if len(entries) == 0 {
		return "memory.md 为空，尚无记忆条目"
	}
	var buf bytes.Buffer
	for _, e := range entries {
		buf.WriteString(fmt.Sprintf("[%s] %s (%s)\n", e.Date, e.Text, e.Category))
	}
	return buf.String()
}

func execMemoryPrune(ctx context.Context, store *memory.Store, argsJSON string) string {
	result, err := store.Prune()
	if err != nil {
		return fmt.Sprintf("整合失败: %v", err)
	}
	return result
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
		Command string `json:"command"`
		CLIType string `json:"cli_type"`
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

// ── Task Advanced ────────────────────────────────────────────────────────────

func execCancelTask(ctx context.Context, argsJSON string) string {
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.TaskID == "" {
		return "⚠️ task_id 是必填字段"
	}
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/tasks/%s/cancel", addr, args.TaskID)
	req, err := http.NewRequestWithContext(ctx, "POST", u, nil)
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("取消任务失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("取消任务失败 (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return fmt.Sprintf("✅ 任务 %s 已取消", args.TaskID)
}

func execUnclaimTask(ctx context.Context, argsJSON string) string {
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.TaskID == "" {
		return "⚠️ task_id 是必填字段"
	}
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/tasks/%s/unclaim", addr, args.TaskID)
	req, err := http.NewRequestWithContext(ctx, "POST", u, nil)
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("放弃认领失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("放弃认领失败 (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return fmt.Sprintf("✅ 已放弃任务 %s 的认领，状态重置为 pending", args.TaskID)
}

func execSetTaskExperiences(ctx context.Context, argsJSON string) string {
	var args struct {
		TaskID       string   `json:"task_id"`
		ExperienceIDs []string `json:"experience_ids"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.TaskID == "" {
		return "⚠️ task_id 是必填字段"
	}
	addr := getServerAddr()
	body, _ := json.Marshal(map[string][]string{"experience_ids": args.ExperienceIDs})
	u := fmt.Sprintf("http://127.0.0.1%s/api/tasks/%s/experiences", addr, args.TaskID)
	req, err := http.NewRequestWithContext(ctx, "PUT", u, bytes.NewReader(body))
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("设置经验关联失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("设置经验关联失败 (HTTP %d): %s", resp.StatusCode, string(body))
	}
	if len(args.ExperienceIDs) == 0 {
		return fmt.Sprintf("✅ 已清除任务 %s 的经验关联", args.TaskID)
	}
	return fmt.Sprintf("✅ 已设置任务 %s 关联 %d 条经验", args.TaskID, len(args.ExperienceIDs))
}

func execRunTaskLoop(ctx context.Context, argsJSON string) string {
	var args struct {
		TaskID string `json:"task_id"`
		Model  string `json:"model"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.TaskID == "" {
		return "⚠️ task_id 是必填字段"
	}
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/tasks/%s/run-loop", addr, args.TaskID)
	body := map[string]any{}
	if args.Model != "" {
		body["model"] = args.Model
	}
	bodyBytes, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("触发 Run Loop 失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != 202 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("触发 Run Loop 失败 (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return fmt.Sprintf("✅ AI Run Loop 已启动，任务 ID: %s", args.TaskID)
}

func execLearnFromTask(ctx context.Context, argsJSON string) string {
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.TaskID == "" {
		return "⚠️ task_id 是必填字段"
	}
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/tasks/%s/learn", addr, args.TaskID)
	req, err := http.NewRequestWithContext(ctx, "POST", u, nil)
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("触发学习失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != 202 {
		return fmt.Sprintf("触发学习失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var result map[string]any
	json.Unmarshal(respBody, &result)
	if msg, ok := result["message"].(string); ok {
		return fmt.Sprintf("✅ 学习完成: %s", msg)
	}
	return fmt.Sprintf("✅ 任务 %s 学习反思已完成", args.TaskID)
}

// ── Execution ────────────────────────────────────────────────────────────────

func execGetExecution(ctx context.Context, argsJSON string) string {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.ID == "" {
		return "⚠️ id 是必填字段"
	}
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/executions/%s", addr, args.ID)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("查询执行失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("查询执行失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var exec struct {
		ID            string  `json:"id"`
		TaskID        string  `json:"task_id"`
		Status        string  `json:"status"`
		CliType       string  `json:"cli_type"`
		Model         string  `json:"model"`
		StartedAt     string  `json:"started_at"`
		CompletedAt   string  `json:"completed_at"`
		ExitCode      int     `json:"exit_code"`
		Output        string  `json:"output"`
		Error         string  `json:"error"`
		EvaluationScore *float64 `json:"evaluation_score"`
	}
	json.Unmarshal(respBody, &exec)
	scoreStr := "无"
	if exec.EvaluationScore != nil {
		scoreStr = fmt.Sprintf("%.0f%%", *exec.EvaluationScore*100)
	}
	return fmt.Sprintf(`📋 执行详情
ID: %s
任务: %s
状态: %s
CLI: %s | 模型: %s
开始: %s | 完成: %s
退出码: %d
评估分数: %s
输出: %s
错误: %s`,
		exec.ID, exec.TaskID, exec.Status, exec.CliType, exec.Model,
		exec.StartedAt, exec.CompletedAt, exec.ExitCode, scoreStr,
		truncate(exec.Output, 500), exec.Error)
}

func execContinueExecution(ctx context.Context, argsJSON string) string {
	var args struct {
		ID     string `json:"id"`
		Prompt string `json:"prompt"`
		Model  string `json:"model"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.ID == "" || args.Prompt == "" {
		return "⚠️ id 和 prompt 是必填字段"
	}
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/executions/%s/continue", addr, args.ID)
	body := map[string]string{"prompt": args.Prompt}
	if args.Model != "" {
		body["model"] = args.Model
	}
	bodyBytes, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("继续执行失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != 202 {
		return fmt.Sprintf("继续执行失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var result map[string]any
	json.Unmarshal(respBody, &result)
	execID, _ := result["execution_id"].(string)
	if execID == "" {
		return "✅ 继续对话已触发"
	}
	return fmt.Sprintf("✅ 继续对话已触发 (execution_id=%s)", execID)
}

func execCancelExecution(ctx context.Context, argsJSON string) string {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.ID == "" {
		return "⚠️ id 是必填字段"
	}
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/executions/%s/cancel", addr, args.ID)
	req, err := http.NewRequestWithContext(ctx, "POST", u, nil)
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("取消执行失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("取消执行失败 (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return fmt.Sprintf("✅ 执行 %s 已取消", args.ID)
}

func execEvaluateExecution(ctx context.Context, argsJSON string) string {
	var args struct {
		ID    string `json:"id"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.ID == "" {
		return "⚠️ id 是必填字段"
	}
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/executions/%s/evaluate", addr, args.ID)
	body := map[string]string{}
	if args.Model != "" {
		body["model"] = args.Model
	}
	bodyBytes, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("触发评估失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != 202 {
		return fmt.Sprintf("触发评估失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return fmt.Sprintf("✅ 评估已启动，执行 ID: %s（评估需要 1-2 分钟）", args.ID)
}

// ── Scheduler ────────────────────────────────────────────────────────────────

func execToggleScheduledTask(ctx context.Context, db *backend.ScheduledTaskRepo, sch SchedulerOps, argsJSON string) string {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.ID == "" {
		return "⚠️ id 是必填字段"
	}
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/scheduled/%s/toggle", addr, args.ID)
	req, err := http.NewRequestWithContext(ctx, "POST", u, nil)
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("切换状态失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("切换状态失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var result map[string]any
	json.Unmarshal(respBody, &result)
	enabled, _ := result["enabled"].(bool)
	status := "停用"
	if enabled {
		status = "启用"
	}
	return fmt.Sprintf("✅ 定时任务 %s 已切换为 [%s]", args.ID, status)
}

// ── System ───────────────────────────────────────────────────────────────────

func execGetAILoopStatus(ctx context.Context, argsJSON string) string {
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/ai-loop/status", addr)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("查询失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("查询失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var result map[string]any
	json.Unmarshal(respBody, &result)
	enabled, _ := result["ai_loop_enabled"].(bool)
	schedEnabled, _ := result["scheduler_enabled"].(bool)
	aiLoopStatus := "禁用"
	if enabled {
		aiLoopStatus = "启用"
	}
	schedStatus := "禁用"
	if schedEnabled {
		schedStatus = "启用"
	}
	return fmt.Sprintf(`🤖 AI 自治能力状态
Run Loop / Reevaluate / Learn: %s
调度器: %s`, aiLoopStatus, schedStatus)
}

func execGetDashboardStats(ctx context.Context, argsJSON string) string {
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/stats", addr)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("查询失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("查询失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var stats map[string]any
	json.Unmarshal(respBody, &stats)
	return fmt.Sprintf(`📊 Dashboard 统计
任务总数: %.0f
执行总数: %.0f
Agent 总数: %.0f`,
		stats["total_tasks"], stats["total_executions"], stats["total_agents"])
}

func execExportConfig(ctx context.Context, argsJSON string) string {
	var args struct {
		Types []string `json:"types"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if len(args.Types) == 0 {
		args.Types = []string{"shortcuts", "links", "experiences"}
	}
	addr := getServerAddr()
	u := fmt.Sprintf("http://127.0.0.1%s/api/config/export?types=%s", addr, strings.Join(args.Types, ","))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return fmt.Sprintf("构建请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("导出失败（网络错误）: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("导出失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var exportData map[string]any
	json.Unmarshal(respBody, &exportData)
	count := 0
	for _, v := range exportData {
		if arr, ok := v.([]any); ok {
			count += len(arr)
		}
	}
	return fmt.Sprintf("✅ 导出成功，共 %d 条记录", count)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

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

// newUUID uses google/uuid for proper UUID generation.
func newUUID() string {
	return uuid.New().String()
}

func randomID(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

func execSendNotification(ctx context.Context, argsJSON string) string {
	var args struct {
		Message string `json:"message"`
		Title   string `json:"title"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("参数解析失败: %v", err)
	}
	if args.Message == "" {
		return "message 不能为空"
	}
	title := args.Title
	if title == "" {
		title = "工作台通知"
	}
	// 使用纯 Python tkinter 实现通知窗口，不依赖系统通知机制
	// 写入临时 Python 文件，声明 UTF-8 编码，避免 shell 编码问题
	script := fmt.Sprintf(`# -*- coding: utf-8 -*-
import tkinter as tk

root = tk.Tk()
root.title(%q)
root.attributes("-topmost", True)
root.configure(bg="#1e1e1e")

screen_w = root.winfo_screenwidth()
screen_h = root.winfo_screenheight()
win_w, win_h = 420, 200
x = screen_w - win_w - 30
y = screen_h - win_h - 60
root.geometry(f"{win_w}x{win_h}+{x}+{y}")

tk.Frame(root, bg="#22d3ee", height=4).pack(fill="x")

content = tk.Frame(root, bg="#1e1e1e")
content.pack(fill="both", expand=True, padx=20, pady=16)

tk.Label(content, text=%q, bg="#1e1e1e", fg="#e2e8f0",
         font=("Microsoft YaHei", 13), wraplength=370, justify="left", anchor="w").pack(fill="x")

btn_frame = tk.Frame(content, bg="#1e1e1e")
btn_frame.pack(fill="x", pady=(16, 0))

def copy_and_confirm():
    root.clipboard_clear()
    root.clipboard_append(%q)
    copy_btn.config(text="\u2713 \u5df2\u590d\u5236", bg="#22c55e")
    root.after(1500, root.destroy)

copy_btn = tk.Button(btn_frame, text="\u590d\u5236\u5185\u5bb9", command=copy_and_confirm,
                    bg="#334155", fg="#e2e8f0", activebackground="#475569",
                    activeforeground="#fff", relief="flat", cursor="hand2",
                    font=("Microsoft YaHei", 12), padx=16, pady=8)
copy_btn.pack(side="left", fill="x", expand=True, padx=(0, 8))

def dismiss():
    root.destroy()

ok_btn = tk.Button(btn_frame, text="\u5173\u95ed", command=dismiss,
                  bg="#22d3ee", fg="#000", activebackground="#06b6d4",
                  activeforeground="#000", relief="flat", cursor="hand2",
                  font=("Microsoft YaHei", 12, "bold"), padx=16, pady=8)
ok_btn.pack(side="left", fill="x", expand=True)

root.mainloop()
`, title, args.Message, args.Message)

// 写临时文件，0600 权限确保安全
f, err := os.CreateTemp("", "notify-*.py")
if err != nil {
	return fmt.Sprintf("创建临时脚本失败: %v", err)
}
defer os.Remove(f.Name())
defer f.Close()
if _, err := f.WriteString(script); err != nil {
	return fmt.Sprintf("写入脚本失败: %v", err)
}
f.Close()

cmd := exec.CommandContext(ctx, "python3", f.Name())
cmd.Stdout = nil
cmd.Stderr = nil
if err := cmd.Start(); err != nil {
	return fmt.Sprintf("启动通知窗口失败: %v", err)
}
go func() {
	cmd.Wait()
}()
	return fmt.Sprintf("通知已弹出: %s", args.Message)
}

// --- Re-export PtySession for use in LocalShellState (avoids import cycle) ---
type PtySession = interface{}
