package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	osexec "os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"github.com/xiaodongQ/xworkbench/internal/evaluator"
	"github.com/xiaodongQ/xworkbench/internal/executor"
	"github.com/xiaodongQ/xworkbench/internal/executor/runner"
	"github.com/xiaodongQ/xworkbench/internal/httplog"
	"github.com/xiaodongQ/xworkbench/internal/hub"
	loglib "github.com/xiaodongQ/xworkbench/internal/logger"
	"github.com/xiaodongQ/xworkbench/internal/memory"
	"github.com/xiaodongQ/xworkbench/internal/paths"
	"github.com/xiaodongQ/xworkbench/internal/ratelimit"
	"github.com/xiaodongQ/xworkbench/internal/relay"
	"github.com/xiaodongQ/xworkbench/internal/scheduler"
	"github.com/xiaodongQ/xworkbench/internal/skill"
	"github.com/xiaodongQ/xworkbench/internal/shortcuts"
	taskpkg "github.com/xiaodongQ/xworkbench/internal/task"
	"github.com/xiaodongQ/xworkbench/internal/todo"
	"github.com/xiaodongQ/xworkbench/internal/wsmsg"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.SugaredLogger

// BuildInfo 在编译时通过 -ldflags 注入，格式: "git-commit build-time"
// 例如: -ldflags "-X main.BuildInfo=abc1234 2026-06-18T10:00:00+0800"
var BuildInfo = "unknown"

//go:embed index.html static
var FS embed.FS

type APIServer struct {
	db           *backend.TaskRepo
	expDB        *backend.ExperienceRepo
	execDB       *backend.ExecutionRepo
	linkDB       *backend.WebLinkRepo
	dirDB        *backend.DirShortcutRepo
	schedDB      *backend.ScheduledTaskRepo
	evalDB       *backend.EvaluationRepo
	agentDB      *backend.AgentRepo
	eventDB      *backend.TaskEventRepo
	cmtDB        *backend.TaskCommentRepo
	execCmtDB    *backend.ExecutionCommentRepo
	sch          *scheduler.Scheduler
	hub          *hub.Hub
	relayHandler *relay.RelayHandler
	mux          *http.ServeMux
	wrapped      http.Handler // mux + httplog.Middleware
	memoryStore  *memory.Store // data/memory.md 管理器

	// 进程内运行中的执行（task_id → cancel func）
	mu      sync.Mutex
	running map[string]context.CancelFunc
	// 进程内跑 run-loop 的任务（task_id → true），防同一任务并发触发多个循环，
	// 避免 WS 推送重复 iteration 事件让前端进度条错乱。
	runLoops map[string]bool
}

func NewAPIServer(
	db *backend.TaskRepo, expDB *backend.ExperienceRepo, execDB *backend.ExecutionRepo,
	linkDB *backend.WebLinkRepo, dirDB *backend.DirShortcutRepo,
	schedDB *backend.ScheduledTaskRepo,
	evalDB *backend.EvaluationRepo, agentDB *backend.AgentRepo,
	eventDB *backend.TaskEventRepo,
	cmtDB *backend.TaskCommentRepo,
	execCmtDB *backend.ExecutionCommentRepo,
	sch *scheduler.Scheduler, h *hub.Hub,
	relayRepo relay.Repo,
) *APIServer {
	s := &APIServer{
		db: db, expDB: expDB, execDB: execDB,
		linkDB: linkDB, dirDB: dirDB, schedDB: schedDB, evalDB: evalDB,
		agentDB: agentDB, eventDB: eventDB,
		cmtDB: cmtDB, execCmtDB: execCmtDB,
		sch: sch, hub: h,
		relayHandler: relay.NewRelayHandler(relayRepo),
		mux:          http.NewServeMux(),
		running:      map[string]context.CancelFunc{},
		runLoops:     map[string]bool{},
		memoryStore:  memory.New(paths.DataDir() + "/memory.md"),
	}
	s.routes()
	s.wrapped = httplog.Middleware(s.mux, loglib.Logger)
	return s
}

// checkRelayAuth returns a middleware that checks API key for relay endpoints.
// Skips auth when Relay.APIKey is empty (disabled) or for same-origin browser requests.
func checkRelayAuth() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 同源请求（浏览器 UI）直接放行
			origin := r.Header.Get("Origin")
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			sameOrigin := origin == "" || origin == scheme+"://"+r.Host
			logger.Infow("checkRelayAuth", "origin", origin, "host", r.Host, "scheme", scheme, "sameOrigin", sameOrigin)
			if sameOrigin {
				next.ServeHTTP(w, r)
				return
			}
			cfg := config.Get()
			if cfg != nil && cfg.Relay.APIKey != "" {
				key := r.Header.Get("Authorization")
				if len(key) > 7 && key[:7] == "Bearer " {
					key = key[7:]
				} else {
					key = r.Header.Get("X-API-Key")
				}
				if key != cfg.Relay.APIKey {
					http.Error(w, `{"error":"unauthorized","hint":"add Authorization: Bearer <api_key> or X-API-Key: <key> (default key: xworkbench)"}`, http.StatusUnauthorized)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *APIServer) routes() {
	mux := s.mux
	mux.HandleFunc("GET /version", s.handleVersion)
	mux.HandleFunc("GET /api/tasks", s.handleTasks)
	mux.HandleFunc("POST /api/tasks", s.handleTaskCreate)
	mux.HandleFunc("GET /api/tasks/{id}", s.handleTaskGet)
	mux.HandleFunc("PUT /api/tasks/{id}", s.handleTaskUpdate)
	mux.HandleFunc("PUT /api/tasks/{id}/status", s.handleTaskStatus)
	mux.HandleFunc("POST /api/tasks/{id}/unclaim", s.handleTaskUnclaim)
	mux.HandleFunc("POST /api/tasks/{id}/run", s.handleTaskRun)
	mux.HandleFunc("POST /api/tasks/{id}/cancel", s.handleTaskCancel)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.handleTaskDelete)
	mux.HandleFunc("PUT /api/tasks/{id}/experiences", s.handleTaskSetExperiences)
	mux.HandleFunc("GET /api/tasks/{id}/executions", s.handleTaskExecutions)
	mux.HandleFunc("GET /api/tasks/{id}/eval-history", s.handleTaskEvalHistory)
	mux.HandleFunc("POST /api/tasks/{id}/reevaluate", s.handleTaskReevaluate)
	mux.HandleFunc("POST /api/tasks/{id}/run-loop", s.handleTaskRunLoop)
	mux.HandleFunc("GET /api/executions", s.handleExecutionsRecent)
	mux.HandleFunc("GET /api/executions/{id}", s.handleExecutionGet)
	mux.HandleFunc("POST /api/executions/{id}/continue", s.handleExecutionContinue)
	mux.HandleFunc("POST /api/executions/{id}/evaluate", s.handleExecutionEvaluate)
	mux.HandleFunc("POST /api/executions/{id}/evaluate-chain", s.handleExecutionEvaluateChain)
	mux.HandleFunc("GET /api/executions/{id}/evaluations", s.handleExecutionEvaluations)
	mux.HandleFunc("POST /api/executions/{id}/cancel", s.handleExecutionCancel)
	mux.HandleFunc("GET /api/experiences", s.handleExperiences)
	mux.HandleFunc("POST /api/experiences", s.handleExpCreate)
	mux.HandleFunc("PUT /api/experiences/{id}", s.handleExpUpdate)
	mux.HandleFunc("DELETE /api/experiences/{id}", s.handleExpDelete)
	mux.HandleFunc("GET /api/experiences/{id}", s.handleExpGet)
	mux.HandleFunc("POST /api/ai/chat", s.handleAIChat)
	mux.HandleFunc("GET /api/ai/config", s.handleAIConfigGet)
	mux.HandleFunc("PUT /api/ai/config", s.handleAIConfigUpdate)
	mux.HandleFunc("POST /api/ai/config/key", s.handleAIConfigSetKey)
	mux.HandleFunc("POST /api/ai/config/test", s.handleAIConfigTest)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/pty", s.handlePty)
	mux.HandleFunc("POST /api/pty/{tab_id}/submit-input", s.handlePtyInput)
	mux.HandleFunc("GET /api/rpty", s.handleRemotePty)
	mux.HandleFunc("POST /api/rpty/{tab_id}/submit-input", s.handleRptyInput)
	mux.HandleFunc("GET /ws", s.handleWS)
	// /static/* 用 embed.FS serve 拆分 CSS/JS 文件
	mux.Handle("GET /static/", http.FileServer(http.FS(FS)))
	mux.HandleFunc("GET /", s.handleIndex)

	// 5 个新功能
	mux.HandleFunc("GET /api/web-links", s.handleWebLinks)
	mux.HandleFunc("POST /api/web-links", s.handleWebLinkCreate)
	mux.HandleFunc("PUT /api/web-links/{id}", s.handleWebLinkUpdate)
	mux.HandleFunc("DELETE /api/web-links/{id}", s.handleWebLinkDelete)
	mux.HandleFunc("POST /api/links/open", s.handleLinkOpen)

	mux.HandleFunc("GET /api/dir-shortcuts", s.handleDirShortcuts)
	mux.HandleFunc("POST /api/dir-shortcuts", s.handleDirShortcutCreate)
	mux.HandleFunc("PUT /api/dir-shortcuts/{id}", s.handleDirShortcutUpdate)
	mux.HandleFunc("DELETE /api/dir-shortcuts/{id}", s.handleDirShortcutDelete)
	mux.HandleFunc("POST /api/dir-shortcuts/{id}/open", s.handleDirShortcutOpen)
	mux.HandleFunc("POST /api/dir-shortcuts/{id}/open-terminal", s.handleDirShortcutOpenTerminal)
	mux.HandleFunc("GET /api/terminals", s.handleTerminalList)
	mux.HandleFunc("GET /api/terminals/detect", s.handleTerminalDetect)
	mux.HandleFunc("GET /api/models", s.handleModelList)
	// 单一配置入口：所有偏好（default_terminal / preferred_cli / ai_loop_enabled /
	// aichat_default_cli / todo_md_path / scheduler_enabled）和部署配置（terminal / models）都走这里
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handleSetConfig)

	mux.HandleFunc("GET /api/scheduled", s.handleScheduledList)
	mux.HandleFunc("POST /api/scheduled", s.handleScheduledCreate)
	mux.HandleFunc("GET /api/scheduled/{id}", s.handleScheduledGet)
	mux.HandleFunc("PUT /api/scheduled/{id}", s.handleScheduledUpdate)
	mux.HandleFunc("POST /api/scheduled/{id}/toggle", s.handleScheduledToggle)
	mux.HandleFunc("DELETE /api/scheduled/{id}", s.handleScheduledDelete)
	mux.HandleFunc("POST /api/scheduled/{id}/run-now", s.handleScheduledRunNow)

	mux.HandleFunc("POST /api/scheduler/start", s.handleSchedulerStart)
	mux.HandleFunc("POST /api/scheduler/stop", s.handleSchedulerStop)
	mux.HandleFunc("GET /api/scheduler/status", s.handleSchedulerStatus)
	mux.HandleFunc("POST /api/scheduler/reload", s.handleSchedulerReload)

	// AI 自治能力开关状态查询（前端 task-modal 根据这个决定是否显示运行区块）
	mux.HandleFunc("GET /api/ai-loop/status", s.handleAILoopStatus)

	mux.HandleFunc("GET /api/todo", s.handleTodo)
	mux.HandleFunc("POST /api/todo", s.handleTodoAdd)
	mux.HandleFunc("PUT /api/todo/{line_no}", s.handleTodoToggle)
	mux.HandleFunc("DELETE /api/todo/{line_no}", s.handleTodoDelete)
	mux.HandleFunc("POST /api/todo/{line_no}/children", s.handleTodoAddChild)
	mux.HandleFunc("PUT /api/todo/{line_no}/edit", s.handleTodoEdit)
	mux.HandleFunc("GET /api/todo/path", s.handleTodoPath)
	mux.HandleFunc("PUT /api/todo/path", s.handleTodoPathSet)

	// relay 代理功能（带 API key 认证）
	relayAuth := checkRelayAuth()
	mux.HandleFunc("POST /api/exec", relayAuth(relay.HandleExec))
	mux.HandleFunc("POST /api/relay/proxy", relayAuth(s.relayHandler.HandleRelayProxy))
	mux.HandleFunc("GET /api/relay/stats", relayAuth(s.relayHandler.HandleRelayStats))

	// 远程 Agent API（带速率限制）
	// 注意：单独一个 sub-mux 给 agent 路由，避免与主 mux 冲突
	rateLimitPerMin := parseInt(os.Getenv("RATE_LIMIT_PER_MIN"), 60)
	agentMux := http.NewServeMux()
	agentMux.HandleFunc("POST /api/agents/register", s.handleAgentRegister)
	agentMux.HandleFunc("POST /api/agents/{id}/heartbeat", s.handleAgentHeartbeat)
	agentMux.HandleFunc("POST /api/tasks/{id}/claim", s.handleTaskClaim)
	agentMux.HandleFunc("POST /api/tasks/{id}/report", s.handleTaskReport)
	agentMux.HandleFunc("GET /api/tasks/claim-next", s.handleTaskClaimNext)
	agentMux.HandleFunc("POST /api/tasks/claim-next", s.handleTaskClaimNext)
	// agent API 套上速率限制 middleware（默认 60/min，可由 RATE_LIMIT_PER_MIN 调整；0 = 禁用）
	{
		var agentHandler http.Handler = agentMux
		if rateLimitPerMin > 0 {
			limiter := ratelimit.New(rateLimitPerMin)
			agentHandler = limiter.Middleware()(agentMux)
		}
		// 挂在主 mux 上（路径完全匹配，使用更具体的 path pattern）
		mux.Handle("POST /api/agents/register", agentHandler)
		mux.Handle("POST /api/agents/{id}/heartbeat", agentHandler)
		mux.Handle("POST /api/tasks/{id}/claim", agentHandler)
		mux.Handle("POST /api/tasks/{id}/report", agentHandler)
		mux.Handle("GET /api/tasks/claim-next", agentHandler)
		mux.Handle("POST /api/tasks/claim-next", agentHandler)
	}

	// 审计 + 依赖
	mux.HandleFunc("GET /api/tasks/{id}/events", s.handleTaskEvents)

	// 远程 Agent 管理 API（主用户调用，不限频、不需要 agent token）
	mux.HandleFunc("GET /api/agents", s.handleAgentsList)
	mux.HandleFunc("POST /api/agents/{id}/release-tasks", s.handleAgentReleaseTasks)
	mux.HandleFunc("POST /api/agents/{id}/reset-token", s.handleAgentResetToken)
	mux.HandleFunc("POST /api/agents/{id}/auto-claim", s.handleAgentSetAutoClaim)
	mux.HandleFunc("POST /api/agents/{id}/bind-dir-shortcut", s.handleAgentSetBoundDirShortcut)
	mux.HandleFunc("DELETE /api/agents/{id}", s.handleAgentDelete)

	// 数据管理：导入 / 导出 / 备份
	mux.HandleFunc("GET /api/config/export", s.handleConfigExport)
	mux.HandleFunc("POST /api/config/import/preview", s.handleConfigImportPreview)
	mux.HandleFunc("POST /api/config/import", s.handleConfigImport)

	// skill 工具技能
	mux.HandleFunc("GET /api/skills", s.handleSkillsList)
	mux.HandleFunc("POST /api/skills/execute", s.handleSkillsExecute)
	mux.HandleFunc("POST /api/skills/create", s.handleSkillsCreate)

	// xwcli 安装脚本（公开，无需认证）
	mux.HandleFunc("GET /api/xwcli/install.sh", s.handleXwcliInstall)
	mux.HandleFunc("GET /api/xwcli/xwcli.py", s.handleXwcliDownload)

	// 评论
	mux.HandleFunc("GET /api/tasks/{id}/comments", s.handleCommentList)
	mux.HandleFunc("POST /api/tasks/{id}/comments", s.handleCommentCreate)
	mux.HandleFunc("PUT /api/comments/{id}", s.handleCommentUpdate)
	mux.HandleFunc("DELETE /api/comments/{id}", s.handleCommentDelete)

	// 执行评论
	mux.HandleFunc("GET /api/executions/{id}/comments", s.handleExecutionCommentList)
	mux.HandleFunc("POST /api/executions/{id}/comments", s.handleExecutionCommentCreate)
	mux.HandleFunc("DELETE /api/execution-comments/{id}", s.handleExecutionCommentDelete)

	// 任务优先级队列：claim-next（自动领下一个最高优先级任务）
}

func (s *APIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.wrapped.ServeHTTP(w, r)
}

// Tasks

func (s *APIServer) handleTasks(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	taskType := r.URL.Query().Get("task_type")
	offset := parseInt(r.URL.Query().Get("offset"), 0)
	limit := parseInt(r.URL.Query().Get("limit"), 50)

	tasks, err := s.db.List(backend.TaskFilter{Status: status, TaskType: taskType, Offset: offset, Limit: limit})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, tasks)
}

func (s *APIServer) handleTaskCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title         string   `json:"title"`
		Description   string   `json:"description"`
		ExperienceID  string   `json:"experience_id"`  // 旧字段（单值），保留向后兼容
		ExperienceIDs []string `json:"experience_ids"` // 新字段：多经验关联
		Acceptance    string   `json:"acceptance"`
		TaskType      string   `json:"task_type"` // 'manual'|'scheduled'|'remote'，默认 'manual'
		Priority      int      `json:"priority"`  // 数字越大越优先，默认 5
		CommandType   string   `json:"command_type"` // claude/shell/cbc，默认 claude
		Model         string   `json:"model"`         // haiku/sonnet/opus
		Prompt           string   `json:"prompt"`             // 执行用 prompt
		GoalMode         bool     `json:"goal_mode"`           // 是否启用 Goal 目标模式
		AssignedAgentID  string   `json:"assigned_agent_id"`   // 指定的远程 agent（task_type=remote）
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	task := &backend.Task{
		ID:           uuid.New().String(),
		Title:        req.Title,
		Description:  req.Description,
		ExperienceID: req.ExperienceID,
		Acceptance:   req.Acceptance,
		Status:       backend.TaskStatusPending,
		Version:      "v0.0.1",
		CreatedAt:    time.Now(),
		TaskType:         req.TaskType,
		Priority:         req.Priority,
		CommandType:      req.CommandType,
		Model:            req.Model,
		Prompt:           req.Prompt,
		GoalMode:         req.GoalMode,
		AssignedAgentID:  req.AssignedAgentID,
	}
	if err := s.db.Create(task); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 多经验关联：experience_ids 优先；空时回退到旧的 experience_id 单值
	expIDs := req.ExperienceIDs
	if len(expIDs) == 0 && req.ExperienceID != "" {
		expIDs = []string{req.ExperienceID}
	}
	if len(expIDs) > 0 {
		if err := s.db.AttachExperiences(task.ID, expIDs); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		task.ExperienceIDs = expIDs
	}
	// 审计：记录创建事件
	s.eventDB.Record(&backend.TaskEvent{
		TaskID: task.ID, EventType: "created",
		Actor: "user", Payload: fmt.Sprintf(`{"task_type":"%s"}`, task.TaskType),
		CreatedAt: time.Now(),
	})
	writeJSON(w, task)
}

func (s *APIServer) handleTaskUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger.Infow("task update", "id", id)
	var req struct {
		Title         string   `json:"title"`
		Description   string   `json:"description"`
		ExperienceID  string   `json:"experience_id"`
		ExperienceIDs []string `json:"experience_ids"`
		Acceptance    string   `json:"acceptance"`
		// Priority 用指针：nil=未传，&0=显式设为 0。
		Priority *int `json:"priority,omitempty"`
		CommandType  string   `json:"command_type"`
		Model        string   `json:"model"`
		Prompt       string   `json:"prompt"`
		GoalMode     *bool    `json:"goal_mode"` // 指针：nil=未传，保持原值
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	task, err := s.db.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	task.Title = req.Title
	task.Description = req.Description
	task.ExperienceID = req.ExperienceID
	task.Acceptance = req.Acceptance
	task.CommandType = req.CommandType
	task.Model = req.Model
	task.Prompt = req.Prompt
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.GoalMode != nil {
		task.GoalMode = *req.GoalMode
	}
	if err := s.db.Update(task); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 多经验关联：experience_ids 优先；空时回退到旧的 experience_id 单值
	expIDs := req.ExperienceIDs
	if len(expIDs) == 0 && req.ExperienceID != "" {
		expIDs = []string{req.ExperienceID}
	}
	if err := s.db.SetTaskExperiences(id, expIDs); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	task, _ = s.db.Get(id)
	writeJSON(w, task)
}

// handleTaskSetExperiences 替换 task 的整个经验列表。传空数组 = 解绑全部。
func (s *APIServer) handleTaskSetExperiences(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		ExperienceIDs []string `json:"experience_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	logger.Infow("task set-experiences", "id", id, "count", len(req.ExperienceIDs))
	if _, err := s.db.Get(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if err := s.db.SetTaskExperiences(id, req.ExperienceIDs); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	task, _ := s.db.Get(id)
	writeJSON(w, task)
}

func (s *APIServer) handleTaskGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := s.db.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, task)
}

func (s *APIServer) handleTaskStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Status     string              `json:"status"`
		Maintainer string              `json:"maintainer"`
		Result     *backend.TaskResult `json:"result,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Status != backend.TaskStatusPending &&
		req.Status != backend.TaskStatusInProgress &&
		req.Status != backend.TaskStatusArchived &&
		req.Status != backend.TaskStatusException {
		writeErr(w, http.StatusBadRequest, "invalid status")
		return
	}
	if req.Maintainer == "" {
		req.Maintainer = "factory-agent"
	}
	if err := s.db.UpdateStatus(id, req.Status, req.Maintainer); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	task, _ := s.db.Get(id)
	writeJSON(w, task)
}

// Experiences

func (s *APIServer) handleExperiences(w http.ResponseWriter, r *http.Request) {
	module := r.URL.Query().Get("module")
	list, err := s.expDB.Search(module)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, list)
}

func (s *APIServer) handleExpCreate(w http.ResponseWriter, r *http.Request) {
	var exp backend.Experience
	if err := json.NewDecoder(r.Body).Decode(&exp); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	exp.ID = uuid.New().String()
	exp.Version = "v1.0.0"
	exp.CreatedAt = time.Now()
	exp.UpdatedAt = time.Now()
	logger.Infow("experience create", "id", exp.ID)
	if err := s.expDB.Create(&exp); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, exp)
}

func (s *APIServer) handleExpGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	exp, err := s.expDB.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, exp)
}

func (s *APIServer) handleExpUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger.Infow("exp update", "id", id)
	var req struct {
		Module   string `json:"module"`
		Keywords string `json:"keywords"`
		Scene    string `json:"scene"`
		Details  string `json:"details"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	exp, err := s.expDB.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	exp.Keywords = req.Keywords
	exp.Scene = req.Scene
	exp.Details = req.Details
	if err := s.expDB.Update(exp); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, exp)
}

func (s *APIServer) handleExpDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger.Infow("exp delete", "id", id)
	if err := s.expDB.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

// handleVersion 返回构建信息，用于确认运行的是哪个版本的二进制
func (s *APIServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"build": BuildInfo,
	})
}

// Stats

func (s *APIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	type Stats struct {
		TotalTasks        int `json:"total_tasks"`
		PendingTasks      int `json:"pending_tasks"`
		InProgressTasks   int `json:"in_progress_tasks"`
		WaitingInputTasks int `json:"waiting_input_tasks"`
		ArchivedTasks     int `json:"archived_tasks"`
		ExceptionTasks    int `json:"exception_tasks"`
		TotalExp          int `json:"total_exp"`
		DailyStats        []struct {
			Date  string `json:"date"`
			Count int    `json:"count"`
		} `json:"daily_stats"`
	}
	range_ := r.URL.Query().Get("range")
	if range_ == "" {
		range_ = "7d"
	}
	all, _ := s.db.List(backend.TaskFilter{Limit: 10000, Offset: 0})
	st := Stats{TotalTasks: len(all)}
	for _, t := range all {
		switch t.Status {
		case backend.TaskStatusPending:
			st.PendingTasks++
		case backend.TaskStatusInProgress:
			st.InProgressTasks++
		case backend.TaskStatusWaitingInput:
			st.WaitingInputTasks++
		case backend.TaskStatusArchived:
			st.ArchivedTasks++
		case backend.TaskStatusException:
			st.ExceptionTasks++
		}
	}
	// 按 range 计算每日/每周统计
	now := time.Now()
	var daily map[string]int
	switch range_ {
	case "1m":
		daily = make(map[string]int)
		for i := 29; i >= 0; i-- {
			d := now.AddDate(0, 0, -i)
			daily[d.Format("2006-01-02")] = 0
		}
		for _, t := range all {
			dateStr := t.CreatedAt.Format("2006-01-02")
			if _, ok := daily[dateStr]; ok {
				daily[dateStr]++
			}
		}
	case "6m":
		daily = make(map[string]int)
		// 按周分组（周一为起始）
		for i := 25; i >= 0; i-- {
			d := now.AddDate(0, 0, -i*7)
			// 调整到本周一
			weekday := int(d.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			monday := d.AddDate(0, 0, -(weekday - 1))
			key := monday.Format("2006-01-02")
			daily[key] = 0
		}
		for _, t := range all {
			tDate := t.CreatedAt
			weekday := int(tDate.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			monday := tDate.AddDate(0, 0, -(weekday - 1))
			key := monday.Format("2006-01-02")
			if _, ok := daily[key]; ok {
				daily[key]++
			}
		}
	default: // "7d"
		daily = make(map[string]int)
		for i := 6; i >= 0; i-- {
			d := now.AddDate(0, 0, -i)
			daily[d.Format("2006-01-02")] = 0
		}
		for _, t := range all {
			dateStr := t.CreatedAt.Format("2006-01-02")
			if _, ok := daily[dateStr]; ok {
				daily[dateStr]++
			}
		}
	}
	for dateStr, count := range daily {
		st.DailyStats = append(st.DailyStats, struct {
			Date  string `json:"date"`
			Count int    `json:"count"`
		}{Date: dateStr, Count: count})
	}
	exps, _ := s.expDB.Search("")
	st.TotalExp = len(exps)
	writeJSON(w, st)
}

// Task execution

// BuildTaskPrompt 的实现已挪到 internal/task/prompt.go（便于跨包测试）。
// 多经验支持：传 []*Experience 切片，每条经验单独一段（带 index）。

// loadExperiencesForTask 按 task.ExperienceIDs 顺序加载所有 experience 内容。
// 单个 exp 查不到时跳过（容错，不阻断运行），保持 prompt 仍然可用。
func (s *APIServer) loadExperiencesForTask(t *backend.Task) []*backend.Experience {
	if len(t.ExperienceIDs) == 0 {
		return nil
	}
	out := make([]*backend.Experience, 0, len(t.ExperienceIDs))
	for _, id := range t.ExperienceIDs {
		if id == "" {
			continue
		}
		exp, err := s.expDB.Get(id)
		if err != nil {
			logger.Warnw("loadExperiencesForTask: missing experience",
				"task_id", t.ID,
				"experience_id", id,
			)
			continue
		}
		out = append(out, exp)
	}
	return out
}

// handleTaskRun 立即执行一次任务。command_type 默认 "claude"（让 AI CLI 解释

// handleTaskRun 立即执行一次任务。command_type 默认 "claude"（让 AI CLI 解释
// 执行 prompt），可显式传 "shell" / "cbc" 走其他 runner。prompt 必传或取自
// task.description；不再 fallback 到 task.title（标题不是命令，避免
// "两数之和: command not found" 之类的隐式错误）。prompt 仍空则报 400。
// 知识库注入: title / description / resources / acceptance / priority / experience 内容全部注入。
func (s *APIServer) handleTaskRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := s.db.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct {
		CommandType     string `json:"command_type"`
		Model           string `json:"model"`
		Prompt          string `json:"prompt"`
		AgentID         string `json:"agent_id"`          // 指定远端 agent 走 SSH 执行（空 = 本机）
		ResumeSessionID string `json:"resume_session_id"` // agent 模式下续传 claude 会话
		GoalMode        bool   `json:"goal_mode"`         // Goal 目标模式（/goal 前缀）
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // body 可选
	// 请求体未传时用 task 创建时确定的默认值
	if req.CommandType == "" {
		req.CommandType = task.CommandType
	}
	if req.CommandType == "" {
		req.CommandType = "claude"
	}
	if req.Model == "" {
		req.Model = task.Model
	}
	// 构造 rich prompt: task 全字段 + 多经验内容注入
	var prompt string
	if req.Prompt != "" {
		// 显式传了 prompt 就用显式的(保留原有行为)
		prompt = req.Prompt
	} else {
		// 没用 body.prompt,自动从 task + 多 experience 组装
		// 修复：用 BuildTaskPromptWithOutput（含经验库 + 输出目录约定）而非 BuildTaskPromptShort（不含经验）
		// 原因：手动任务执行与 agent claim 一致需把经验库喂给 AI CLI；
		// 同时 system prompt 显式告诉 AI「把生成的文件写到 data/ai-task-dir/<task_id>/」
		exps := s.loadExperiencesForTask(task)
		prompt = taskpkg.BuildTaskPromptWithOutput(task, paths.AITaskDir(id), exps...)
		if prompt == "" {
			logger.Warnw("task run rejected: empty prompt after BuildTaskPrompt",
				"task_id", id,
				"command_type", req.CommandType,
			)
			writeErr(w, http.StatusBadRequest, "task has no description and no experience content")
			return
		}
	}
	// Goal 模式：request body 优先，否则 fallback 到 task.GoalMode
	// claude/cbc 执行时 prompt 前加 /goal 前缀
	if (req.GoalMode || task.GoalMode) && (req.CommandType == "claude" || req.CommandType == "cbc") {
		prompt = "/goal " + prompt
	}
	req.Prompt = prompt

	// 决定执行位置：agent_id 非空 + 绑定 dir_shortcut → SSH 远端；否则本机。
	var sshCfg *executor.SSHConfig
	if req.AgentID != "" {
		ag, err := s.agentDB.GetByID(req.AgentID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "agent not found: "+err.Error())
			return
		}
		if ag.BoundDirShortcutID == "" {
			writeErr(w, http.StatusBadRequest,
				fmt.Sprintf("agent %s has no bound_dir_shortcut_id; bind it to a remote dir_shortcut first", ag.Name))
			return
		}
		ds, err := s.dirDB.GetByID(ag.BoundDirShortcutID)
		if err != nil || ds == nil {
			writeErr(w, http.StatusNotFound, "bound dir_shortcut not found")
			return
		}
		if ds.Type != backend.DirShortcutTypeRemote {
			writeErr(w, http.StatusBadRequest, "bound dir_shortcut is not type=remote")
			return
		}
		if ds.RemoteHost == "" || ds.RemoteUser == "" {
			writeErr(w, http.StatusBadRequest, "bound dir_shortcut missing remote_host/remote_user")
			return
		}
		sshCfg = &executor.SSHConfig{
			Host:       ds.RemoteHost,
			User:       ds.RemoteUser,
			AuthMethod: ds.AuthMethod,
			Password:   ds.RemotePassword,
			KeyPath:    ds.KeyPath,
			Port:       22, // 暂不暴露到 UI；未来加
			TimeoutSec: 10,
		}
		logger.Infow("task run: routing to agent via SSH",
			"task_id", id, "agent_id", req.AgentID,
			"agent_name", ag.Name, "dir_shortcut_id", ds.ID, "dir_shortcut_name", ds.Name,
			"remote_host", ds.RemoteHost, "remote_user", ds.RemoteUser)
	}

	// 构造命令（带可选 --resume；可选 --dangerously-skip-permissions）
	skip := config.AppConfig != nil && config.AppConfig.DangerouslySkipPermissions
	var (
		cmd     []string
		stdin   string
		cleanup func()
	)
	if skip {
		cmd, stdin, cleanup, err = runner.BuildCommand(req.CommandType, req.Model, "", req.Prompt,
			runner.WithResume(req.ResumeSessionID),
			runner.WithSkipPermissions(),
			runner.WithActionReport(),
		)
	} else {
		cmd, stdin, cleanup, err = runner.BuildCommand(req.CommandType, req.Model, "", req.Prompt,
			runner.WithResume(req.ResumeSessionID),
			runner.WithActionReport(),
		)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 注:cleanup 不能在这里 defer — handler 写完 response 就返回,defer 立即执行,
	// 会把临时脚本文件在 goroutine 跑命令前删除,导致 exit_code=127。
	// 真正的 cleanup 放在下面 goroutine 的开头。

	// 写 executions 行
	exec := &backend.Execution{
		ID:        uuid.New().String(),
		TaskID:    id,
		Source:    "manual",
		Command:   runner.CmdStringWithPrompt(cmd, req.Prompt),
		Prompt:    req.Prompt, // 保存完整 prompt
		Model:     req.Model,
		CliType:   req.CommandType, // 用于"继续对话"延续原 CLI 类型
		StartedAt: time.Now(),
	}
	if err := s.execDB.Create(exec); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.db.UpdateStatus(id, backend.TaskStatusRunning, "factory-agent")

	logger.Infow("task run started",
		"task_id", id,
		"execution_id", exec.ID,
		"command_type", req.CommandType,
		"model", req.Model,
		"prompt_chars", len(req.Prompt),
		"via", func() string {
			if sshCfg != nil {
				return "ssh:" + sshCfg.Host
			}
			return "local"
		}(),
		"cmd", exec.Command,
	)
	// 打印完整命令（<2K 完整打印，超出截取前 2K）
	fullCmd := runner.CmdString(cmd) + " " + stdin
	if len(fullCmd) <= 2048 {
		logger.Infow("task run full command", "cmd", fullCmd)
	} else {
		logger.Infow("task run command truncated", "cmd", fullCmd[:2048]+"...")
	}

	// 异步跑，10min 超时
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	s.mu.Lock()
	s.running[id] = cancel
	s.mu.Unlock()
	go func() {
		started := time.Now()
		defer func() {
			s.mu.Lock()
			delete(s.running, id)
			s.mu.Unlock()
		}()
		if cleanup != nil {
			defer cleanup()
		}
		chunkCB := func(chunk string) {
			s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
				"execution_id": exec.ID,
				"task_id":      id,
				"chunk":        chunk,
			})
		}
		var res *executor.Result
		var runErr error
		if sshCfg != nil {
			res, runErr = executor.RunSSHViaConfig(ctx, *sshCfg, cmd, stdin, chunkCB)
		} else {
			// AI 任务 CWD 走沙盒（data/ai-sandbox/），避免 AI 写文件污染源码树
			// CWD = 项目根（继承父进程），让 AI 能 ls/Read 项目文件；
			// system prompt 已约定写文件到 data/ai-task-dir/<task_id>/（BuildTaskPromptWithOutput 拼接）。
			res, runErr = executor.Run(ctx, cmd, "", stdin, chunkCB)
		}
		status := backend.TaskStatusArchived
		if res != nil && res.ExitCode != 0 {
			status = backend.TaskStatusException
		}
		out, errOut := "", ""
		exitCode := -1
		if res != nil {
			out = res.Output
			exitCode = res.ExitCode
			// 兜底：ctx 超时时 stderr 通常为空（子进程被 SIGKILL 后管道没收数据），
			// 真正错误信息在 res.Err（"signal: killed"）或 runErr（context.DeadlineExceeded）
			if res.ErrorOut != "" {
				errOut = res.ErrorOut
			} else if res.Err != nil {
				errOut = "executor: " + res.Err.Error()
			}
		}
		if runErr != nil && errOut == "" {
			errOut = "run: " + runErr.Error()
		}
		// 解析 claude -p --output-format json 输出中的 session_id（用于 --resume 继续对话）
		resumeSessionID := extractResumeSessionID(out)
		_ = s.execDB.Finish(exec.ID, out, errOut, exitCode, resumeSessionID)
		// cancel 场景：handleTaskCancel 已标 task 为 failed，goroutine skip 重复更新
		if runErr == nil || !strings.Contains(runErr.Error(), "context canceled") {
			_ = s.db.UpdateStatus(id, status, "factory-agent")
		}
		s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
			"execution_id": exec.ID,
			"task_id":      id,
			"done":         true,
			"exit_code":    exitCode,
		})
		if exitCode != 0 || runErr != nil {
			logger.Errorw("task run finished",
				"task_id", id,
				"execution_id", exec.ID,
				"session_id", resumeSessionID,
				"exit_code", exitCode,
				"status", status,
				"dur_ms", time.Since(started).Milliseconds(),
				"err", errStr(runErr),
			)
		} else {
			logger.Infow("task run finished",
				"task_id", id,
				"execution_id", exec.ID,
				"session_id", resumeSessionID,
				"exit_code", exitCode,
				"status", status,
				"dur_ms", time.Since(started).Milliseconds(),
			)
		}
	}()

	writeJSON(w, map[string]any{
		"execution_id": exec.ID,
		"task_id":      id,
		"command":      exec.Command,
		"status":       "started",
		"via": func() string {
			if sshCfg != nil {
				return "ssh:" + sshCfg.Host
			}
			return "local"
		}(),
	})
}

func (s *APIServer) handleTaskUnclaim(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.db.UpdateStatus(id, backend.TaskStatusPending, ""); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"id": id, "status": "pending"})
}

func (s *APIServer) handleTaskCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	cancel, ok := s.running[id]
	s.mu.Unlock()
	if !ok {
		// 没有 in-flight goroutine（WS 断连 / 服务重启导致 execution 已结束但 task 状态未同步），
		// 仍然强制将 task 标为异常，不返回 404（用户需要的是"把我从运行中解救出来"）。
		_ = s.db.UpdateStatus(id, backend.TaskStatusException, "")
		s.hub.Broadcast(wsmsg.ChannelTask, map[string]any{
			"task_id": id,
			"status":  backend.TaskStatusException,
		})
		writeJSON(w, map[string]any{"task_id": id, "cancelled": true, "forced": true})
		return
	}
	cancel()
	// 立即标 task 为 failed，并广播 ChannelTask 让 tasks Tab 即时刷新
	_ = s.db.UpdateStatus(id, backend.TaskStatusException, "")
	s.hub.Broadcast(wsmsg.ChannelTask, map[string]any{
		"task_id": id,
		"status":  backend.TaskStatusException,
	})
	writeJSON(w, map[string]any{"task_id": id, "cancelled": true})
}

// handleExecutionCancel 强制结束卡住的 execution。两种情况：
//  1. running map 里有 task_id 的 cancel func（in-flight goroutine 还在）→ 调 cancel()，
//     触发 executor.Run ctx 超时，goroutine 内会调 execDB.Finish 写 completed_at
//  2. running map 里没有（服务器重启 / goroutine 已被 GC）→ 直接写 completed_at=now
//     强制把"僵尸"execution 标完成，error="manually cancelled (force)"
//
// 这是 task 详情「⚠ 标记完成」按钮的底层接口，主要解决 WS 断连后前端永远看到「运行中」的问题。
func (s *APIServer) handleExecutionCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	exec, err := s.execDB.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if exec.CompletedAt != nil {
		// 已经完成了，告诉前端无需再取消
		writeJSON(w, map[string]any{
			"execution_id": id,
			"already_done": true,
			"completed_at": exec.CompletedAt,
		})
		return
	}

	// 尝试通过 task_id 找 in-flight goroutine 的 cancel func
	if exec.TaskID != "" {
		s.mu.Lock()
		cancel, ok := s.running[exec.TaskID]
		s.mu.Unlock()
		if ok {
			cancel()
			logger.Infow("execution cancelled via running map", "execution_id", id, "task_id", exec.TaskID)
			writeJSON(w, map[string]any{
				"execution_id": id,
				"task_id":      exec.TaskID,
				"mode":         "running",
				"cancelled":    true,
			})
			return
		}
	}

	// 兜底：in-flight goroutine 已不在（服务器重启 / WS 断连丢 done 事件），
	// 强制写 completed_at。前端下次 listRecent 就能看到状态变化。
	now := time.Now()
	if err := s.execDB.ForceFinish(id, now, "manually cancelled (force, no in-flight goroutine)"); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Warnw("execution force-finished (no in-flight goroutine)", "execution_id", id, "task_id", exec.TaskID)
	// 通过 WS 广播 done 事件，前端若连接着能立即看到更新
	s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
		"execution_id": id,
		"task_id":      exec.TaskID,
		"done":         true,
		"exit_code":    -1,
		"force":        true,
	})
	writeJSON(w, map[string]any{
		"execution_id": id,
		"task_id":      exec.TaskID,
		"mode":         "force_finished",
		"cancelled":    true,
		"completed_at": now,
	})
}

// handleTaskDelete 硬删 task + 关联 executions + evaluations（不可恢复）。
func (s *APIServer) handleTaskDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger.Infow("task delete", "id", id)
	if err := s.db.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"id": id, "status": "deleted"})
}

func (s *APIServer) handleTaskExecutions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	list, err := s.execDB.ListByTask(id, 50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*backend.Execution{}
	}
	writeJSON(w, list)
}

func (s *APIServer) handleExecutionsRecent(w http.ResponseWriter, r *http.Request) {
	limit := parseInt(r.URL.Query().Get("limit"), 50)
	resumeUUID := r.URL.Query().Get("resume_uuid")
	var list []*backend.Execution
	var err error
	if resumeUUID != "" {
		list, err = s.execDB.ListByResumeUUID(resumeUUID, limit)
	} else {
		list, err = s.execDB.ListRecent(limit)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*backend.Execution{}
	}
	writeJSON(w, list)
}

func (s *APIServer) handleExecutionGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	exec, err := s.execDB.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, exec)
}

// handleExecutionContinue 基于某次 execution 的会话继续对话。
// 使用 --resume <uuid> 让 claude 继续之前的会话。
func (s *APIServer) handleExecutionContinue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	orig, err := s.execDB.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if orig.ResumeSessionID == "" {
		writeErr(w, http.StatusBadRequest, "该执行没有 session_id，无法继续对话（可能是该任务执行时未能获取到会话 ID）")
		return
	}
	logger.Infow("execution continue start", "orig_id", id, "session_id", orig.ResumeSessionID)
	var req struct {
		Prompt string `json:"prompt"`
		Model  string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
		writeErr(w, http.StatusBadRequest, "prompt 不能为空")
		return
	}
	// 延续原执行环境:CLI 用原 exec 的 cli_type(老数据可能为空 → fallback claude),
	// model 不传则沿用原 model。--resume session_id 已在命令构造时加进去,
	// 不再硬编码 "claude"。
	cliType := orig.CliType
	if cliType == "" {
		cliType = "claude"
	}
	model := req.Model
	if model == "" {
		model = orig.Model
	}
	// 拼上输出目录约定：让 AI 知道继续对话时写文件往 data/ai-task-dir/<taskID>/ 落
	// （原 exec 已有 taskID；非 task 来源的 exec 用 orig.TaskID 为空 → 不拼）
	// 用 buildPrompt 而非直接改 req.Prompt，保留 req.Prompt 作为「用户原始输入」
	// 存到 exec.Prompt 字段用于前端显示。
	buildPrompt := req.Prompt
	if orig.TaskID != "" {
		buildPrompt = req.Prompt + fmt.Sprintf(taskpkg.OutputDirHintTpl, paths.AITaskDir(orig.TaskID))
	}
	skip := config.AppConfig != nil && config.AppConfig.DangerouslySkipPermissions
	var (
		cmd     []string
		stdin   string
		cleanup func()
	)
	if skip {
		cmd, stdin, cleanup, err = runner.BuildCommand(cliType, model, "", buildPrompt,
			runner.WithResume(orig.ResumeSessionID),
			runner.WithSkipPermissions(),
			runner.WithActionReport(),
		)
	} else {
		cmd, stdin, cleanup, err = runner.BuildCommand(cliType, model, "", buildPrompt,
			runner.WithResume(orig.ResumeSessionID),
			runner.WithActionReport(),
		)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 创建新 execution（continue 触发的）
	exec := &backend.Execution{
		ID:        uuid.New().String(),
		TaskID:    orig.TaskID,
		Source:    "continue",
		Command:   runner.CmdStringWithPrompt(cmd, req.Prompt),
		Prompt:    req.Prompt,
		Model:     model,
		CliType:   cliType,
		StartedAt: time.Now(),
	}
	if err := s.execDB.Create(exec); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 异步执行
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	s.mu.Lock()
	s.running[exec.ID] = cancel
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, exec.ID)
			s.mu.Unlock()
		}()
		if cleanup != nil {
			defer cleanup()
		}
		// CWD = 项目根（继承父进程），让 AI 能 ls/Read 项目文件；
		// system prompt 已约定写文件到 data/ai-task-dir/<task_id>/（BuildTaskPromptWithOutput 拼接）。
		res, _ := executor.Run(ctx, cmd, "", stdin, func(chunk string) {
			s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
				"execution_id": exec.ID,
				"chunk":        chunk,
			})
		})
		out, errOut := "", ""
		exitCode := -1
		if res != nil {
			out, errOut = res.Output, res.ErrorOut
			exitCode = res.ExitCode
		}
		// continue 触发的 execution：沿用原 exec 的 resume_uuid（session_id）。
		// 如果原 exec 没有 resume_uuid，说明原始会话没有成功建立，无法继续。
		// 同时从本次 output 中解析 session_id，覆盖写入（--resume 可能产生新的 session UUID）。
		newSessionID := orig.ResumeSessionID
		if extracted := extractResumeSessionID(out); extracted != "" {
			newSessionID = extracted
		}
		_ = s.execDB.Finish(exec.ID, out, errOut, exitCode, newSessionID)
		s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
			"execution_id": exec.ID,
			"done":         true,
			"exit_code":    exitCode,
		})
		// 如果原 execution 属于某个计划任务，同步 session 信息：
		// 下次计划任务自动运行时，复用同一个 session，保持和手动「继续对话」一致。
		// 仅在新 session 有效时更新（resume 失败时不覆盖已有 session）。
		if orig.ScheduledTaskID != "" && newSessionID != "" {
			if task, err := s.schedDB.Get(orig.ScheduledTaskID); err == nil && task != nil {
				_ = s.schedDB.UpdateSessionInfo(orig.ScheduledTaskID, newSessionID, task.ResumeCount)
			}
		}
		logger.Infow("execution continue finished",
			"orig_id", id, "exec_id", exec.ID, "session_id", orig.ResumeSessionID, "exit_code", exitCode,
			"dur_ms", time.Since(exec.StartedAt).Milliseconds())
	}()
	writeJSON(w, map[string]any{
		"execution_id": exec.ID,
		"status":       "started",
	})
}

// handleExecutionEvaluate 异步调 claude 给 execution 打分。
func (s *APIServer) handleExecutionEvaluate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	exec, err := s.execDB.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct {
		CliType    string `json:"cli_type"`
		Model      string `json:"model"`
		TimeoutSec int    `json:"timeout_sec"` // 评估超时时间，默认120秒
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	// 默认值跟随 config.json 的 preferred_cli（单一来源），改 preferred_cli 后评估 CLI 自动跟随
	// evaluator 必须是真实 AI CLI（claude/cbc），shell 不能做评估
	if req.CliType == "" || !config.IsValidCLI(req.CliType) {
		req.CliType = preferredCLI()
	}
	if req.Model == "" {
		req.Model = evalDefaultModel(req.CliType)
	}
	if req.TimeoutSec <= 0 {
		req.TimeoutSec = 120
	}
	// 找 task prompt：优先用 execution.prompt（scheduled task），其次 BuildTaskPrompt（普通 task）
	prompt := exec.Prompt
	if prompt == "" {
		prompt = exec.Command
	}
	if exec.TaskID != "" {
		if t, err := s.db.Get(exec.TaskID); err == nil {
			prompt = taskpkg.BuildTaskPrompt(t, s.loadExperiencesForTask(t)...)
		}
	}
	// 异步执行（避免 HTTP 阻塞 30s+）
	go func() {
		logger.Infow("evaluator: dispatched",
			"execution_id", id,
			"cli", req.CliType,
			"model", req.Model,
		)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutSec)*time.Second)
		defer cancel()
		_, err := evaluator.RunAndSave(ctx, s.evalDB, s.execDB, exec, prompt, req.CliType, req.Model)
		if err != nil {
			logger.Errorf("evaluator: %v", err)
		}
	}()
	writeJSON(w, map[string]string{"execution_id": id, "status": "evaluating", "cli_type": req.CliType, "model": req.Model})
}

// handleExecutionEvaluateChain 评估整个会话链：获取同 resume_uuid 的所有执行，合并 input/output 后一次评估。
func (s *APIServer) handleExecutionEvaluateChain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	exec, err := s.execDB.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct {
		CliType    string `json:"cli_type"`
		Model      string `json:"model"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	// 默认值跟随 config.json 的 preferred_cli（单一来源）
	// evaluator 必须是真实 AI CLI（claude/cbc），shell 不能做评估
	if req.CliType == "" || !config.IsValidCLI(req.CliType) {
		req.CliType = preferredCLI()
	}
	if req.Model == "" {
		req.Model = evalDefaultModel(req.CliType)
	}
	if req.TimeoutSec <= 0 {
		req.TimeoutSec = 120
	}

	// 获取同 resume_uuid 的所有执行
	var chain []*backend.Execution
	if exec.ResumeSessionID != "" {
		chain, _ = s.execDB.ListByResumeUUID(exec.ResumeSessionID, 50)
	}
	if len(chain) == 0 {
		chain = []*backend.Execution{exec}
	}

	// 找 task prompt
	prompt := exec.Prompt
	if prompt == "" {
		prompt = exec.Command
	}
	if exec.TaskID != "" {
		if t, err := s.db.Get(exec.TaskID); err == nil {
			prompt = taskpkg.BuildTaskPrompt(t, s.loadExperiencesForTask(t)...)
		}
	}

	// 异步执行
	go func() {
		logger.Infow("evaluator: chain dispatched",
			"execution_id", id,
			"chain_size", len(chain),
			"cli", req.CliType,
			"model", req.Model,
		)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutSec)*time.Second)
		defer cancel()
		_, err := evaluator.RunAndSaveChain(ctx, s.evalDB, s.execDB, chain, id, prompt, req.CliType, req.Model)
		if err != nil {
			logger.Errorf("evaluator chain: %v", err)
		}
	}()
	writeJSON(w, map[string]any{"execution_id": id, "status": "evaluating", "cli_type": req.CliType, "model": req.Model, "chain_size": len(chain)})
}

func (s *APIServer) handleExecutionEvaluations(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	list, err := s.evalDB.ListByExecution(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*backend.Evaluation{}
	}
	writeJSON(w, list)
}

// WebSocket

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func (s *APIServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("ws upgrade: %v", err)
		return
	}
	s.hub.Register(conn)
	// 简单读循环：忽略客户端消息，只用来检测断开
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			s.hub.Unregister(conn)
			return
		}
	}
}

// Index

func (s *APIServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := FS.ReadFile("index.html")
	if err != nil {
		http.Error(w, "UI not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// ===== 5 个新功能 handler =====

// --- Web Links ---

func (s *APIServer) handleWebLinks(w http.ResponseWriter, r *http.Request) {
	list, err := s.linkDB.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*backend.WebLink{}
	}
	writeJSON(w, list)
}

func (s *APIServer) handleWebLinkCreate(w http.ResponseWriter, r *http.Request) {
	logger.Infow("weblink create")
	var req struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		IconURL   string `json:"icon_url"`
		SortOrder int    `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.URL == "" {
		writeErr(w, http.StatusBadRequest, "name and url are required")
		return
	}
	// 显式传 sort_order 用显式,否则追加到末尾(max+1)
	if req.SortOrder == 0 {
		req.SortOrder = s.linkDB.NextSortOrder()
	}
	link := &backend.WebLink{
		ID:        uuid.New().String(),
		Name:      req.Name,
		URL:       req.URL,
		IconURL:   req.IconURL,
		SortOrder: req.SortOrder,
		CreatedAt: time.Now(),
	}
	if err := s.linkDB.Create(link); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, link)
}

func (s *APIServer) handleWebLinkUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger.Infow("weblink update", "id", id)
	var req struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		IconURL   string `json:"icon_url"`
		SortOrder int    `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	link := &backend.WebLink{ID: id, Name: req.Name, URL: req.URL, IconURL: req.IconURL, SortOrder: req.SortOrder}
	if err := s.linkDB.Update(link); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, link)
}

func (s *APIServer) handleWebLinkDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger.Infow("weblink delete", "id", id)
	if _, err := s.linkDB.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"id": id, "status": "deleted"})
}

// handleLinkOpen 用系统原生工具打开 URL 或本地路径（支持 file://、Unix 绝对路径、~、Windows 盘符、UNC 路径）。
func (s *APIServer) handleLinkOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeErr(w, http.StatusBadRequest, "url is required")
		return
	}

	url := req.URL
	isLocal := false

	// 展开 ~ 为用户 home 目录
	if strings.HasPrefix(url, "~") {
		if usr, err2 := user.Current(); err2 == nil {
			url = filepath.Join(usr.HomeDir, url[1:])
			isLocal = true
		}
	} else if isFileURL(url) || isLocalPath(url) {
		isLocal = true
	}

	// 将本地路径转换为 file:// URL
	if isLocal && !isFileURL(url) {
		url = pathToFileURL(url)
	}

	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tryOpen := func(c string, args ...string) bool {
		err = osexec.CommandContext(ctx, c, args...).Run()
		return err == nil
	}

	opened := false
	switch runtime.GOOS {
	case "darwin":
		opened = tryOpen("open", url)
	case "windows":
		opened = tryOpen("cmd", "/c", "start", "", url)
	default:
		// linux: xdg-open 为主，fallback 为 gio open
		opened = tryOpen("xdg-open", url)
		if !opened && err != nil {
			opened = tryOpen("gio", "open", url)
		}
	}

	if !opened {
		writeErr(w, http.StatusInternalServerError, "failed to open: "+err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "opened"})
}

// isFileURL 判断是否为 file:// URL
func isFileURL(url string) bool {
	return strings.HasPrefix(url, "file://")
}

// isLocalPath 判断字符串是否为本地路径（Unix 绝对路径、Windows 盘符、UNC 路径）
func isLocalPath(path string) bool {
	// Unix 绝对路径
	if strings.HasPrefix(path, "/") {
		return true
	}
	// Windows 盘符（如 C:\ 或 C:/）
	if matchWindowsDrive(path) {
		return true
	}
	// UNC 路径（\\ 开头）
	if strings.HasPrefix(path, "\\\\") {
		return true
	}
	return false
}

// matchWindowsDrive 检测字符串是否以 Windows 盘符开头（如 C:\, D:/）
func matchWindowsDrive(path string) bool {
	if len(path) < 2 {
		return false
	}
	c := path[0]
	if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
		return false
	}
	return path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}

// pathToFileURL 将本地路径转换为 proper file:// URL
func pathToFileURL(path string) string {
	// Windows: C:\path\to\file → file:///C:/path/to/file
	if matchWindowsDrive(path) {
		drive := strings.ToLower(string(path[0]))
		rest := path[3:] // 去掉 "C:\\"
		rest = strings.ReplaceAll(rest, "\\", "/")
		return "file:///" + drive + ":/" + rest
	}
	// UNC 路径：\\server\share → file:////server/share
	if strings.HasPrefix(path, "\\\\") {
		return "file://" + strings.ReplaceAll(path, "\\", "/")
	}
	// Unix 绝对路径：/home/user/file → file:///home/user/file
	return "file:///" + path
}

// --- Dir Shortcuts ---

func (s *APIServer) handleDirShortcuts(w http.ResponseWriter, r *http.Request) {
	list, err := s.dirDB.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*backend.DirShortcut{}
	}
	writeJSON(w, list)
}

func (s *APIServer) handleDirShortcutCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"name"`
		Path           string `json:"path"`
		SortOrder      int    `json:"sort_order"`
		Type           string `json:"type"`
		RemoteHost     string `json:"remote_host"`
		RemotePort     string `json:"remote_port"`
		RemoteUser     string `json:"remote_user"`
		RemotePath     string `json:"remote_path"`
		RemotePassword string `json:"remote_password"`
		AuthMethod     string `json:"auth_method"`
		KeyPath        string `json:"key_path"`
		TerminalCmd    string `json:"terminal_cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	logger.Infow("dir-shortcut create request",
		"name", req.Name,
		"path", req.Path,
		"type", req.Type,
		"remote_host", req.RemoteHost,
		"remote_port", req.RemotePort,
		"remote_user", req.RemoteUser,
		"remote_path", req.RemotePath,
	)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Type == "remote" && req.RemoteHost == "" {
		writeErr(w, http.StatusBadRequest, "remote_host is required for remote shortcuts")
		return
	}
	if req.Type != "remote" && req.Path == "" {
		writeErr(w, http.StatusBadRequest, "path is required for local shortcuts")
		return
	}
	if req.SortOrder == 0 {
		req.SortOrder = s.dirDB.NextSortOrder()
	}
	d := &backend.DirShortcut{
		ID:             uuid.New().String(),
		Name:           req.Name,
		Path:           req.Path,
		SortOrder:      req.SortOrder,
		Type:           req.Type,
		RemoteHost:     req.RemoteHost,
		RemoteUser:     req.RemoteUser,
		RemotePath:     req.RemotePath,
		RemotePassword: req.RemotePassword,
		AuthMethod:     req.AuthMethod,
		KeyPath:        req.KeyPath,
		TerminalCmd:    req.TerminalCmd,
		CreatedAt:      time.Now(),
	}
	if err := s.dirDB.Create(d); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, d)
}

func (s *APIServer) handleDirShortcutUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name           string `json:"name"`
		Path           string `json:"path"`
		SortOrder      int    `json:"sort_order"`
		Type           string `json:"type"`
		RemoteHost     string `json:"remote_host"`
		RemotePort     string `json:"remote_port"`
		RemoteUser     string `json:"remote_user"`
		RemotePath     string `json:"remote_path"`
		RemotePassword string `json:"remote_password"`
		AuthMethod     string `json:"auth_method"`
		KeyPath        string `json:"key_path"`
		TerminalCmd    string `json:"terminal_cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	d := &backend.DirShortcut{
		ID:             id,
		Name:           req.Name,
		Path:           req.Path,
		SortOrder:      req.SortOrder,
		Type:           req.Type,
		RemoteHost:     req.RemoteHost,
		RemotePort:     req.RemotePort,
		RemoteUser:     req.RemoteUser,
		RemotePath:     req.RemotePath,
		RemotePassword: req.RemotePassword,
		AuthMethod:     req.AuthMethod,
		KeyPath:        req.KeyPath,
		TerminalCmd:    req.TerminalCmd,
	}
	logger.Infow("dir-shortcut update", "id", id, "name", req.Name)
	if err := s.dirDB.Update(d); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, d)
}

func (s *APIServer) handleDirShortcutDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger.Infow("dir-shortcut delete", "id", id)
	if err := s.dirDB.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"id": id, "status": "deleted"})
}

func (s *APIServer) handleDirShortcutOpen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	list, err := s.dirDB.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var entry *backend.DirShortcut
	for _, d := range list {
		if d.ID == id {
			entry = d
			break
		}
	}
	if entry == nil {
		writeErr(w, http.StatusNotFound, "shortcut not found")
		return
	}
	logger.Infow("dir-shortcut open",
		"id", id,
		"name", entry.Name,
		"path", entry.Path,
		"type", entry.Type,
		"remote_host", entry.RemoteHost,
	)
	if entry.Type == backend.DirShortcutTypeRemote {
		writeErr(w, http.StatusBadRequest, "remote shortcut: use /open-terminal to open with SSH")
		return
	}
	if err := shortcuts.OpenDir(entry.Path); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.dirDB.Touch(id)
	writeJSON(w, map[string]any{"id": id, "status": "opened", "path": entry.Path})
}

// handleDirShortcutOpenTerminal 打开指定终端类型的工作目录
func (s *APIServer) handleDirShortcutOpenTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	termType := r.URL.Query().Get("type") // 可选，默认用 config.json 的 default_type
	if termType == "" {
		termType = shortcuts.DefaultTerminal()
	}
	logger.Infow("[handleDirShortcutOpenTerminal]",
		"id", id,
		"termType", termType,
		"at", "main.go:928")

	list, err := s.dirDB.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var entry *backend.DirShortcut
	for _, d := range list {
		if d.ID == id {
			entry = d
			break
		}
	}
	if entry == nil {
		writeErr(w, http.StatusNotFound, "shortcut not found")
		return
	}

	// 判断是远程还是本地路径
	path := entry.Path
	if termType == "" {
		termType = shortcuts.DefaultTerminal()
	}
	cfg := config.Get()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	typeDef, ok := cfg.Terminal.Types[strings.ToLower(termType)]
	if !ok {
		writeErr(w, http.StatusBadRequest, "unsupported terminal type: "+termType)
		return
	}
	logger.Infow("[handleDirShortcutOpenTerminal] opening",
		"dir", path,
		"termType", termType,
		"binPath", typeDef.Path,
		"bin", typeDef.Bin,
		"at", "main.go:964")

	binPath := typeDef.Path
	var openErr error
	if entry.Type == backend.DirShortcutTypeRemote {
		openErr = shortcuts.OpenRemoteDirShortcut(entry, termType, binPath)
	} else {
		if _, err := os.Stat(path); err != nil {
			writeErr(w, http.StatusBadRequest, "目录不存在或不可访问："+path)
			return
		}
		openErr = shortcuts.OpenTerminal(termType, path, binPath)
	}
	if openErr != nil {
		writeErr(w, http.StatusBadRequest, openErr.Error())
		return
	}
	_ = s.dirDB.Touch(id)
	writeJSON(w, map[string]interface{}{"id": id, "status": "opened", "path": path, "terminal": termType})
}

// handleTerminalList 返回支持的终端类型列表
func (s *APIServer) handleTerminalList(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	supported := make([]map[string]interface{}, 0, len(cfg.Terminal.Types))
	for typeKey, termDef := range cfg.Terminal.Types {
		entry := map[string]interface{}{
			"type": typeKey,
			"name": termDef.Name,
			"plate": termDef.Plate,
		}
		if len(termDef.RemoteArgs) > 0 {
			entry["remote_args"] = termDef.RemoteArgs
		}
		supported = append(supported, entry)
	}
	writeJSON(w, map[string]interface{}{
		"supported": supported,
		"default":   shortcuts.DefaultTerminal(),
	})
}

// handleTerminalDetect 检测终端类型的可执行文件路径
func (s *APIServer) handleTerminalDetect(w http.ResponseWriter, r *http.Request) {
	termType := r.URL.Query().Get("type")
	if termType == "" {
		writeErr(w, http.StatusBadRequest, "type is required")
		return
	}
	path := shortcuts.DetectTerminalPath(termType)
	if path != "" {
		writeJSON(w, map[string]string{"path": path})
	} else {
		writeJSON(w, map[string]string{"path": ""})
	}
}

// handleModelList 返回模型列表（从 config.json 加载）
func (s *APIServer) handleModelList(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	writeJSON(w, map[string]interface{}{
		"cli_type_models": cfg.Models,
	})
}

// handleGetConfig 返回当前配置（从 config.json 读取）
func (s *APIServer) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	writeJSON(w, map[string]interface{}{
		// 用户偏好（顶层）
		"default_terminal":   cfg.DefaultTerminal,
		"preferred_cli":      cfg.PreferredCLI,
		"ai_loop_enabled":    cfg.AILoopEnabled,
		"aichat_default_cli": cfg.AichatDefaultCLI,
		"todo_md_path":       cfg.TodoMDPath,
		"scheduler_enabled":  cfg.SchedulerEnabled,
		"dangerously_skip_permissions": cfg.DangerouslySkipPermissions,
		// 部署级配置
		"relay": cfg.Relay,
		"terminal": map[string]interface{}{
			"detect_paths": cfg.Terminal.DetectPaths,
			"types":        cfg.Terminal.Types,
		},
		"models": cfg.Models,
	})
}

// handleSetConfig 统一入口：更新 config.json 中的偏好和部署配置，回写文件
// 任意字段非空（非零）即覆盖；bool/int 字段 0/false 也合法（用 ptr 区分"未设"）
//
// 安全语义（修复 Issue 2 + 3）：
//  1. 校验和"是否要改"先于修改：所有写入字段都必须先通过校验，
//     校验失败返 4xx 不改任何东西。
//  2. copy-on-write 修改：fn 内只改副本，全局其它 reader 仍看旧值，
//     没有"半改状态"被读到。
//  3. Save 失败回滚：磁盘写失败时内存回滚到原始快照，
//     不会出现"内存已改 / 磁盘未改"的不一致窗口。
func (s *APIServer) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	logger.Infow("set config")
	var req struct {
		// 用户偏好（字符串字段：空=未设；bool 字段：nil=未设）
		DefaultTerminal  *string `json:"default_terminal,omitempty"`
		PreferredCLI     *string `json:"preferred_cli,omitempty"`
		AILoopEnabled    *bool   `json:"ai_loop_enabled,omitempty"`
		AichatDefaultCLI *string `json:"aichat_default_cli,omitempty"`
		TodoMDPath       *string `json:"todo_md_path,omitempty"`
		SchedulerEnabled *bool   `json:"scheduler_enabled,omitempty"`
		DangerouslySkipPermissions *bool `json:"dangerously_skip_permissions,omitempty"`
		// 部署级
		TerminalType      string            `json:"terminal_type"`
		TerminalPath      string            `json:"terminal_path"`
		ModelDefaults     map[string]string `json:"model_defaults"`      // cli_type -> 执行默认 model
		EvalModelDefaults map[string]string `json:"eval_model_defaults"` // cli_type -> 评估默认 model
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := config.Get()
	if cfg == nil {
		writeErr(w, http.StatusInternalServerError, "config not initialized")
		return
	}

	// === 第一阶段：校验（不动 cfg）===
	// 任何 4xx 都直接返，不进入修改阶段。
	if req.DefaultTerminal != nil {
		v := *req.DefaultTerminal
		if v != "" && !shortcuts.IsSupportedTerminal(v) {
			writeErr(w, http.StatusBadRequest, "unsupported terminal type: "+v)
			return
		}
	}
	if req.PreferredCLI != nil {
		v := *req.PreferredCLI
		if v != "" && !config.IsValidCLI(v) {
			writeErr(w, http.StatusBadRequest, "unsupported cli: "+v+" (supported: claude, cbc)")
			return
		}
	}
	if req.TerminalType != "" {
		// 注：default_type 已上移到顶层 DefaultTerminal；此处 TerminalType 仅用于与 TerminalPath 配对更新类型路径
		if _, ok := cfg.Terminal.Types[req.TerminalType]; !ok {
			writeErr(w, http.StatusBadRequest, "unknown terminal type: "+req.TerminalType)
			return
		}
	}

	// === 第二阶段：决定是否真要改 ===
	// ModelDefaults/EvalModelDefaults 需 cliType 在 cfg.Models 里才标 changed；
	// 其它 nil/空值不算"请求改"，避免没必要的 Save。
	changed := false
	if req.DefaultTerminal != nil {
		changed = true
	}
	if req.PreferredCLI != nil {
		changed = true
	}
	if req.AichatDefaultCLI != nil {
		changed = true
	}
	if req.TodoMDPath != nil {
		changed = true
	}
	if req.AILoopEnabled != nil {
		changed = true
	}
	if req.SchedulerEnabled != nil {
		changed = true
	}
	if req.DangerouslySkipPermissions != nil {
		changed = true
	}
	if req.TerminalType != "" {
		changed = true
	}
	if req.TerminalPath != "" && req.TerminalType != "" {
		changed = true
	}
	if req.ModelDefaults != nil {
		for cliType := range req.ModelDefaults {
			if _, ok := cfg.Models[cliType]; ok {
				changed = true
				break
			}
		}
	}
	if !changed && req.EvalModelDefaults != nil {
		for cliType := range req.EvalModelDefaults {
			if _, ok := cfg.Models[cliType]; ok {
				changed = true
				break
			}
		}
	}

	if !changed {
		// 没字段需要改（可能全空 / 全是未支持的 cli_type / 默认值相同）。
		// 直接返 ok，不动 AppConfig 也不写盘。
		writeJSON(w, map[string]string{"status": "ok"})
		return
	}

	// === 第三阶段：copy-on-write 修改 + Save（失败自动回滚）===
	_, err := config.SetAndSave(func(c *config.Config) {
		// 用户偏好（字符串）
		if req.DefaultTerminal != nil {
			c.DefaultTerminal = *req.DefaultTerminal
		}
		if req.PreferredCLI != nil {
			c.PreferredCLI = config.NormalizeCLI(*req.PreferredCLI)
		}
		if req.AichatDefaultCLI != nil {
			c.AichatDefaultCLI = *req.AichatDefaultCLI
		}
		if req.TodoMDPath != nil {
			c.TodoMDPath = *req.TodoMDPath
		}
		// 用户偏好（bool）
		if req.AILoopEnabled != nil {
			c.AILoopEnabled = *req.AILoopEnabled
		}
		if req.SchedulerEnabled != nil {
			c.SchedulerEnabled = *req.SchedulerEnabled
		}
		if req.DangerouslySkipPermissions != nil {
			c.DangerouslySkipPermissions = *req.DangerouslySkipPermissions
		}
		// 部署级：TerminalPath
		if req.TerminalPath != "" && req.TerminalType != "" {
			if typeDef, ok := c.Terminal.Types[req.TerminalType]; ok {
				typeDef.Path = req.TerminalPath
				c.Terminal.Types[req.TerminalType] = typeDef
			}
		}
		// 部署级：ModelDefaults / EvalModelDefaults（仅 cliType 已存在时生效）
		if req.ModelDefaults != nil {
			for cliType, model := range req.ModelDefaults {
				if group, ok := c.Models[cliType]; ok {
					group.Default = model
					c.Models[cliType] = group
				}
			}
		}
		if req.EvalModelDefaults != nil {
			for cliType, model := range req.EvalModelDefaults {
				if group, ok := c.Models[cliType]; ok {
					group.EvalDefault = model
					c.Models[cliType] = group
				}
			}
		}
	})
	if err != nil {
		// Save 失败：内存已自动回滚，告知客户端即可。
		writeErr(w, http.StatusInternalServerError, "save config failed: "+err.Error())
		return
	}

	// 配置已落盘：广播给所有 WS 客户端，让其他 tab 即时同步 UI
	// （AI 自治开关/调度器开关等热改场景下，用户在一个 tab 改了，另一个
	// tab 打开任务详情弹窗时就能看到最新状态，不用重新加载整个页面）。
	// 重新读一次 Get() 拿刚 SetAndSave 后的最新值（cfg 是 SetAndSave 之前的快照）。
	if s.hub != nil {
		if newCfg := config.Get(); newCfg != nil {
			s.hub.Broadcast(wsmsg.ChannelConfig, map[string]any{
				"event":             "config_changed",
				"ai_loop_enabled":   newCfg.AILoopEnabled,
				"scheduler_enabled": newCfg.SchedulerEnabled,
				"preferred_cli":     newCfg.PreferredCLI,
			})
		}
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// --- Scheduled Tasks ---

func (s *APIServer) handleScheduledList(w http.ResponseWriter, r *http.Request) {
	list, err := s.schedDB.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*backend.ScheduledTask{}
	}
	// 注入下次执行时间：走 scheduler 内部 entry.Next()（触发前稳定），
	// 不用 handler 现场 Parse+Next(time.Now())——后者会随 now 漂移导致 UI 一直刷新。
	// 拿不到（未 enabled / 解析失败 / scheduler 未加载）时留 nil,前端不显示。
	for _, t := range list {
		if nxt, ok := s.sch.NextRunAt(t.ID); ok {
			t.NextRunAt = &nxt
		}
	}
	writeJSON(w, list)
}

func (s *APIServer) handleScheduledGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.schedDB.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, t)
}

func (s *APIServer) handleScheduledCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		CronExpr    string `json:"cron_expr"`
		CommandType string `json:"command_type"`
		Model       string `json:"model"`
		Prompt      string `json:"prompt"`
		WorkingDir  string `json:"working_dir"`
		Enabled     bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.CronExpr == "" || req.CommandType == "" {
		writeErr(w, http.StatusBadRequest, "name, cron_expr, command_type are required")
		return
	}
	t := &backend.ScheduledTask{
		ID:          uuid.New().String(),
		Name:        req.Name,
		CronExpr:    req.CronExpr,
		CommandType: req.CommandType,
		Model:       req.Model,
		Prompt:      req.Prompt,
		WorkingDir:  req.WorkingDir,
		Enabled:     req.Enabled,
		CreatedAt:   time.Now(),
	}
	if err := s.schedDB.Create(t); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Infow("handler: scheduled task created", "id", t.ID, "name", t.Name, "cron", t.CronExpr)
	_ = s.sch.Reload() // 热加载
	writeJSON(w, t)
}

func (s *APIServer) handleScheduledUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger.Infow("scheduled update", "id", id)
	var req struct {
		Name        string `json:"name"`
		CronExpr    string `json:"cron_expr"`
		CommandType string `json:"command_type"`
		Model       string `json:"model"`
		Prompt      string `json:"prompt"`
		WorkingDir  string `json:"working_dir"`
		Enabled     bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	t := &backend.ScheduledTask{
		ID:          id,
		Name:        req.Name,
		CronExpr:    req.CronExpr,
		CommandType: req.CommandType,
		Model:       req.Model,
		Prompt:      req.Prompt,
		WorkingDir:  req.WorkingDir,
		Enabled:     req.Enabled,
	}
	if err := s.schedDB.Update(t); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.sch.Reload()
	writeJSON(w, t)
}

func (s *APIServer) handleScheduledDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger.Infow("scheduled delete", "id", id)
	if err := s.schedDB.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Infow("handler: scheduled task deleted", "id", id)
	_ = s.sch.Reload()
	writeJSON(w, map[string]string{"id": id, "status": "deleted"})
}

// handleScheduledToggle 翻转 enabled 状态并 reload scheduler。
func (s *APIServer) handleScheduledToggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.schedDB.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	t.Enabled = !t.Enabled
	if err := s.schedDB.Update(t); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.sch.Reload()
	writeJSON(w, t)
}

func (s *APIServer) handleScheduledRunNow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger.Infow("scheduled run-now received", "id", id)
	execID, err := s.sch.RunNow(id)
	if err != nil {
		logger.Warnw("scheduled run-now failed", "id", id, "err", err)
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	logger.Infow("scheduled run-now done", "id", id, "execution_id", execID)
	writeJSON(w, map[string]string{"id": id, "execution_id": execID, "status": "triggered"})
}

// --- Scheduler ---

func (s *APIServer) handleSchedulerStart(w http.ResponseWriter, r *http.Request) {
	if err := s.sch.Start(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Infow("handler: scheduler started")
	writeJSON(w, map[string]string{"status": "running"})
}

func (s *APIServer) handleSchedulerStop(w http.ResponseWriter, r *http.Request) {
	s.sch.Stop()
	logger.Infow("handler: scheduler stopped")
	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *APIServer) handleSchedulerStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"running": s.sch.IsRunning(),
	})
}

func (s *APIServer) handleSchedulerReload(w http.ResponseWriter, r *http.Request) {
	if err := s.sch.Reload(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "reloaded"})
}

// handleAILoopStatus 返回 AI 自治能力开关状态（单一来源：config.json）。
// 前端 task-modal 打开时调这个，决定是否渲染"AI 自治"区块；也用于判断按钮是否该禁用。
func (s *APIServer) handleAILoopStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	var running []string
	for id := range s.runLoops {
		running = append(running, id)
	}
	s.mu.Unlock()
	writeJSON(w, map[string]any{"enabled": s.aiLoopEnabled(), "running": running})
}

// handleGetPreferredCLI 读优先 CLI。前端"默认 CLI" tab 调。
func (s *APIServer) handleGetPreferredCLI(w http.ResponseWriter, r *http.Request) {
	v := preferredCLI()
	writeJSON(w, map[string]any{"value": v})
}

// handleSetPreferredCLI 设置优先 CLI。写入 config.json（运行时热生效，重启后保留）。
// body: {"value": "claude" | "cbc"}；取值在 config.IsValidCLI 检查。
func (s *APIServer) handleSetPreferredCLI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !config.IsValidCLI(req.Value) {
		writeErr(w, http.StatusBadRequest, "unsupported cli type: "+req.Value+" (supported: claude, cbc)")
		return
	}
	if config.Get() == nil {
		writeErr(w, http.StatusInternalServerError, "config not initialized")
		return
	}
	v := config.NormalizeCLI(req.Value)
	if _, err := config.SetAndSave(func(c *config.Config) {
		c.PreferredCLI = v
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "save config failed: "+err.Error())
		return
	}
	logger.Infow("set preferred_cli", "value", v)
	writeJSON(w, map[string]any{"value": v})
}

// --- Todo.md ---

// todoPath 集中读 todo_md_path，统一走 config.Get()（单一来源 + 线程安全）
func todoPath() string {
	if cfg := config.Get(); cfg != nil {
		return cfg.TodoMDPath
	}
	return ""
}

func (s *APIServer) handleTodo(w http.ResponseWriter, r *http.Request) {
	path := todoPath()
	if path == "" {
		writeJSON(w, map[string]any{"path": "", "items": []*todo.Item{}})
		return
	}
	items, err := todo.ReadAndParse(path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []*todo.Item{}
	}
	writeJSON(w, map[string]any{"path": path, "items": items})
}

func (s *APIServer) handleTodoToggle(w http.ResponseWriter, r *http.Request) {
	path := todoPath()
	if path == "" {
		writeErr(w, http.StatusBadRequest, "todo_md_path not set")
		return
	}
	lineNo := parseInt(r.PathValue("line_no"), 0)
	if lineNo <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid line_no")
		return
	}
	var req struct {
		Done bool `json:"done"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // body 可选，未传则保持原状
	items, err := todo.ReadAndParse(path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 展平为扁平列表用于按 line_no 查找并回写
	flat := todo.Flatten(items)
	var found bool
	for i := range flat {
		if flat[i].LineNo == lineNo {
			flat[i].Done = req.Done
			found = true
			break
		}
	}
	if !found {
		writeErr(w, http.StatusNotFound, "line not found")
		return
	}
	if err := todo.ToggleAndWrite(path, flat); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"line_no": lineNo, "done": req.Done})
}

func (s *APIServer) handleTodoAdd(w http.ResponseWriter, r *http.Request) {
	path := todoPath()
	if path == "" {
		writeErr(w, http.StatusBadRequest, "todo_md_path not set")
		return
	}
	var req struct {
		Text    string   `json:"text"`
		DueDate string   `json:"due_date,omitempty"`
		Tags    []string `json:"tags,omitempty"`
		Note    string   `json:"note,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Text == "" {
		writeErr(w, http.StatusBadRequest, "text is required")
		return
	}
	lineNo, err := todo.AddAndWrite(path, req.Text, req.DueDate, req.Tags, req.Note)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"text": req.Text, "status": "added", "line_no": lineNo})
}

func (s *APIServer) handleTodoDelete(w http.ResponseWriter, r *http.Request) {
	path := todoPath()
	if path == "" {
		writeErr(w, http.StatusBadRequest, "todo_md_path not set")
		return
	}
	lineNo := parseInt(r.PathValue("line_no"), 0)
	logger.Infow("todo delete", "line_no", lineNo)
	if lineNo <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid line_no")
		return
	}
	if err := todo.DeleteAndWrite(path, lineNo); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"line_no": lineNo, "status": "deleted"})
}

func (s *APIServer) handleTodoAddChild(w http.ResponseWriter, r *http.Request) {
	path := todoPath()
	if path == "" {
		writeErr(w, http.StatusBadRequest, "todo_md_path not set")
		return
	}
	parentLineNo := parseInt(r.PathValue("line_no"), 0)
	if parentLineNo <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid line_no")
		return
	}
	var req struct {
		Text    string `json:"text"`
		DueDate string `json:"due_date,omitempty"`
		Done    bool   `json:"done"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Text == "" {
		writeErr(w, http.StatusBadRequest, "text is required")
		return
	}
	if err := todo.AddChildAndWrite(path, parentLineNo, req.Text, req.DueDate, req.Done); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"text": req.Text, "status": "added"})
}

func (s *APIServer) handleTodoEdit(w http.ResponseWriter, r *http.Request) {
	path := todoPath()
	if path == "" {
		writeErr(w, http.StatusBadRequest, "todo_md_path not set")
		return
	}
	lineNo := parseInt(r.PathValue("line_no"), 0)
	if lineNo <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid line_no")
		return
	}
	var req struct {
		Text    string `json:"text,omitempty"`
		DueDate string `json:"due_date,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Text == "" {
		writeErr(w, http.StatusBadRequest, "text is required")
		return
	}
	items, err := todo.ReadAndParse(path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	flat := todo.Flatten(items)
	var found bool
	for i := range flat {
		if flat[i].LineNo == lineNo {
			flat[i].Text = req.Text
			if req.DueDate != "" {
				flat[i].DueDate = req.DueDate
			}
			found = true
			break
		}
	}
	if !found {
		writeErr(w, http.StatusNotFound, "line not found")
		return
	}
	if err := todo.ToggleAndWrite(path, flat); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"line_no": lineNo, "status": "updated"})
}

func (s *APIServer) handleTodoPath(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"path": todoPath()})
}

func (s *APIServer) handleTodoPathSet(w http.ResponseWriter, r *http.Request) {
	logger.Infow("todo path set")
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if config.Get() == nil {
		writeErr(w, http.StatusInternalServerError, "config not initialized")
		return
	}
	if _, err := config.SetAndSave(func(c *config.Config) {
		c.TodoMDPath = req.Path
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "save config failed: "+err.Error())
		return
	}
	writeJSON(w, map[string]string{"path": req.Path})
}

// Helpers

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	// 获取调用者函数名
	pcs := make([]uintptr, 1)
	runtime.Callers(2, pcs)
	var funcName string
	if pcs[0] != 0 {
		fn := runtime.FuncForPC(pcs[0])
		if fn != nil {
			funcName = fn.Name()
			if idx := strings.LastIndex(funcName, "."); idx >= 0 {
				funcName = funcName[idx+1:]
			}
		}
	}
	if funcName == "" {
		funcName = "unknown"
	}
	if code >= 500 {
		logger.Errorw("http error", "status", code, "msg", msg, "handler", funcName)
	} else if code >= 400 {
		logger.Warnw("http error", "status", code, "msg", msg, "handler", funcName)
	} else {
		logger.Infow("http error", "status", code, "msg", msg, "handler", funcName)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// aiLoopEnabled 判断 AI 自治能力是否启用。
// 单一来源：config.json（ai_loop_enabled 顶层字段）。
// 3 个 handler 入口（handleTaskReevaluate / RunLoop / Learn）每次请求都查，Save() 后下一次请求生效。
func (s *APIServer) aiLoopEnabled() bool {
	if cfg := config.Get(); cfg != nil {
		return cfg.AILoopEnabled
	}
	return false
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// preferredCLI 读 preferred_cli（线程安全），空则返 "claude"。
// 单一来源是 config.json 顶层 PreferredCLI；并发安全由 Snapshot/Get 保证。
func preferredCLI() string {
	if cfg := config.Get(); cfg != nil {
		if v := config.NormalizeCLI(cfg.PreferredCLI); v != "" {
			return v
		}
	}
	return "claude"
}

// evalDefaultModel 评估入口 model 为空时的 fallback 链。
// 优先级：Models[cli].EvalDefault（评估专用）→ Models[cli].Default（执行默认）→ "sonnet"
// 注：调用方应先保证 cliType 非空（已 fallback 到 PreferredCLI/"claude"）。
func evalDefaultModel(cliType string) string {
	if cfg := config.Get(); cfg != nil {
		if group, ok := cfg.Models[cliType]; ok {
			if group.EvalDefault != "" {
				return group.EvalDefault
			}
			if group.Default != "" {
				return group.Default
			}
		}
	}
	return "sonnet"
}

// extractResumeSessionID 从 claude -p / cbc -p --output-format json 输出中解析 session_id/sessionId。
//
// claude 2.x 输出是 JSON 事件流（多 event 数组），有 session_id 字段：
//   - system/init 块：必带 session_id
//   - 最后 result 块：必带 session_id（session 内稳定，多次 --resume 都是同一个）
//
// codebuddy 输出是分段 JSON 数组，每个段都有 sessionId 字段（同一个值）。
//
// 注意：不要被 event 块中的 uuid 字段迷惑。uuid 是单次执行的事件标识，**每次都变**，
// 不是 session 标识，传给 --resume 会让 claude 报 "No conversation found with session ID"。
// 传给 --resume 的必须是 session_id（一次会话内跨多次 --resume 不变）。
//
// 解析策略：先尝试按 JSON 解析（数组或单对象）；从尾到头扫 session_id/sessionId 字段；
// JSON 解析失败则回退到字符串匹配（防止输出被截断不是合法 JSON）。
func extractResumeSessionID(output string) string {
	if output == "" {
		return ""
	}
	// 路径 1：JSON 解析
	var anyObj any
	if err := json.Unmarshal([]byte(output), &anyObj); err == nil {
		// anyObj 可能是 array 或单 object
		switch v := anyObj.(type) {
		case []any:
			// 优先取最后一个含 session_id 或 sessionId 的 event（result 块）
			for i := len(v) - 1; i >= 0; i-- {
				if m, ok := v[i].(map[string]any); ok {
					if sid, _ := m["session_id"].(string); sid != "" {
						return sid
					}
					if sid, _ := m["sessionId"].(string); sid != "" {
						return sid
					}
				}
			}
			// 退化：取第一个含 session_id 或 sessionId 的（init 块）
			for i := 0; i < len(v); i++ {
				if m, ok := v[i].(map[string]any); ok {
					if sid, _ := m["session_id"].(string); sid != "" {
						return sid
					}
					if sid, _ := m["sessionId"].(string); sid != "" {
						return sid
					}
				}
			}
			return ""
		case map[string]any:
			if sid, _ := v["session_id"].(string); sid != "" {
				return sid
			}
			if sid, _ := v["sessionId"].(string); sid != "" {
				return sid
			}
		}
	}
	// 路径 2：字符串匹配回退（输出被截断 / 非合法 JSON）
	// 优先匹配 "session_id"（claude）
	idx := strings.Index(output, `"session_id"`)
	if idx >= 0 {
		rest := output[idx+12:] // 跳过 "session_id"
		idx2 := strings.Index(rest, `"`)
		if idx2 >= 0 {
			rest = rest[idx2+1:]
			end := strings.Index(rest, `"`)
			if end >= 0 {
				return rest[:end]
			}
		}
	}
	// 回退：匹配 "sessionId"（codebuddy）
	idx = strings.Index(output, `"sessionId"`)
	if idx >= 0 {
		rest := output[idx+11:] // 跳过 "sessionId"
		idx2 := strings.Index(rest, `"`)
		if idx2 >= 0 {
			rest = rest[idx2+1:]
			end := strings.Index(rest, `"`)
			if end >= 0 {
				return rest[:end]
			}
		}
	}
	return ""
}

func main() {
	// 初始化默认 logger（stderr），后续切到文件后通过 loglib.Set 同步到全局
	zapLogger, _ := zap.NewProduction()
	sugar := zapLogger.Sugar()
	logger = sugar
	loglib.Set(sugar) // 同步到全局，使其他包（evaluator/executor 等）也写入同一目标

	dbPath := paths.ResolveDBPath()
	if cwd, err := os.Getwd(); err == nil {
		logger.Infow("db path", "path", dbPath, "cwd", cwd)
	} else {
		logger.Infow("db path", "path", dbPath)
	}

	db, err := backend.OpenDB(dbPath)
	if err != nil {
		logger.Fatalw("open db failed", "err", err)
	}
	defer db.Close()

	if err := backend.InitSchema(db); err != nil {
		logger.Fatalw("init schema failed", "err", err)
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("[config] load failed: %v, using defaults", err)
		cfg = config.DefaultConfig()
	}
	config.Set(cfg)

	// 支持 -config 指定配置文件路径
	cfgPath := flag.String("config", "", "path to config.json")
	addrFlag := flag.String("addr", "", "listen address (e.g. :8902)")
	flag.Parse()
	if *cfgPath != "" {
		if err := config.LoadFromPath(*cfgPath); err != nil {
			logger.Errorf("[config] load from %s failed: %v", *cfgPath, err)
		} else {
			logger.Infow("config loaded", "path", *cfgPath)
		}
	}

	// 日志写入文件（包含源文件行号，时间戳用友好格式）
	logDir := filepath.Join(filepath.Dir(dbPath), "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		logger.Warnw("创建日志目录失败，将仅输出到stderr", "logDir", logDir, "err", err)
	} else {
		logFile := filepath.Join(logDir, "xworkbench.log")
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			logger.Warnw("打开日志文件失败，将仅输出到stderr", "logFile", logFile, "err", err)
		} else {
			encCfg := zap.NewProductionEncoderConfig()
			encCfg.TimeKey = "time"
			encCfg.EncodeTime = zapcore.ISO8601TimeEncoder // 2026-01-02T15:04:05.000Z
			enc := zapcore.NewConsoleEncoder(encCfg)
			core := zapcore.NewCore(enc, zapcore.AddSync(f), zapcore.InfoLevel)
			zapLogger := zap.New(core, zap.AddCaller())
			logger = zapLogger.Sugar()
			logger.Infow("日志写入文件", "path", logFile, "build", BuildInfo)
		}
	}

	// 把统一配置的 logger 注入各内部包，避免各自 init 写 stderr
	loglib.Set(logger)

	taskRepo := backend.NewTaskRepo(db)
	expRepo := backend.NewExperienceRepo(db)
	execRepo := backend.NewExecutionRepo(db)
	linkRepo := backend.NewWebLinkRepo(db)
	dirRepo := backend.NewDirShortcutRepo(db)
	schedRepo := backend.NewScheduledTaskRepo(db)
	evalRepo := backend.NewEvaluationRepo(db)
	h := hub.New()
	sch := scheduler.New(schedRepo, execRepo, h)
	if err := sch.AutoStart(); err != nil {
		logger.Errorf("[scheduler] auto start failed: %v", err)
	}

	// init relay repo
	relayRepo := relay.NewSQLiteRelayRepo(db)
	if err := relayRepo.InitSchema(); err != nil {
		logger.Fatalw("init relay schema failed", "err", err)
	}

	agentRepo := backend.NewAgentRepo(db)
	eventRepo := backend.NewTaskEventRepo(db)
	cmtRepo := backend.NewTaskCommentRepo(db)
	execCmtRepo := backend.NewExecutionCommentRepo(db)

	// startup orphan cleanup：扫 status='running' 的执行(上次服务器关闭时还在跑的子进程
	// 已经随进程退出,这些 exec 在 DB 里永远卡在 running)。统一 ForceFinish 标 cancelled,
	// 前端不会再看到「永久运行中」的状态,列表/详情里能看到「服务重启时被强制结束」标记。
	// 不会自动重跑——AI 任务可能有副作用(写文件/网络调用),自动重跑有风险;让用户决定是否手动重跑。
	if orphans, err := execRepo.ListRunning(); err != nil {
		logger.Errorw("startup orphan scan failed", "error", err.Error())
	} else if len(orphans) > 0 {
		now := time.Now()
		reason := "orphaned on startup, server restarted while execution was in-flight (force-finished)"
		finished := 0
		for _, o := range orphans {
			if err := execRepo.ForceFinish(o.ID, now, reason); err != nil {
				logger.Errorw("orphan force-finish failed", "execution_id", o.ID, "task_id", o.TaskID, "error", err.Error())
				continue
			}
			logger.Warnw("orphan execution force-finished on startup", "execution_id", o.ID, "task_id", o.TaskID)
			// WS 广播 done 事件,连着的客户端能立即看到状态变化
			h.Broadcast(wsmsg.ChannelExec, map[string]any{
				"execution_id": o.ID,
				"task_id":      o.TaskID,
				"done":         true,
				"exit_code":    -1,
				"force":        true,
				"orphan":       true,
			})
			finished++
		}
		logger.Warnw("startup orphan cleanup done", "total", len(orphans), "finished", finished)
	}

	// 初始化 skill 插件注册中心（扫描 tools/ 目录）
	// 查找顺序：cwd/tools（开发，从 repo 根启动）> 二进制同级 tools/（生产部署）
	var skillToolsDir string
	var candidates []string
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "tools"))
	}
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		// binary 与 tools 同级：/opt/xworkbench/xworkbench + /opt/xworkbench/tools
		candidates = append(candidates, filepath.Join(execDir, "tools"))
	}
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			skillToolsDir = dir
			break
		}
	}
	if skillToolsDir != "" {
		logger.Infow("skill init", "dir", skillToolsDir)
		if err := skill.Init(skillToolsDir); err != nil {
			logger.Warnw("skill init failed", "err", err, "dir", skillToolsDir)
		} else {
			logger.Infow("skill registry loaded", "dir", skillToolsDir, "count", len(skill.GetAll()))
		}
	}

	srv := NewAPIServer(taskRepo, expRepo, execRepo,
		linkRepo, dirRepo, schedRepo, evalRepo, agentRepo,
		eventRepo, cmtRepo, execCmtRepo, sch, h, relayRepo)

	// 后台 goroutine：心跳超时检测
	// Agent >30s 未心跳 → 标记为 offline，并把该 agent 手上未完成的任务还回 pending 池
	// 任务 claim >10min 未完成 → 强制释放回 pending 池（防心跳还在但任务托死）
	startAgentHeartbeatChecker(agentRepo, taskRepo, eventRepo, h, 30*time.Second, 10*time.Minute)

	addr := *addrFlag
	if addr == "" {
		addr = os.Getenv("ADDR")
	}
	if addr == "" {
		addr = ":8902"
	}

	// Windows Service 模式：在此处接管，不再继续往下执行
	// startFn 会启动 HTTP server 并监听 stopCh 来优雅关闭
	if runServiceFlag(
		func() {
			// HTTP server 启动（Windows Service 的 goroutine 中执行）
			logger.Infof("Skill Factory started at http://localhost%s  build=%s", addr, BuildInfo)
			stopCh := make(chan struct{})
			serviceStopCh = stopCh

			httpSrv := &http.Server{
				Handler:     srv,
				IdleTimeout: 30 * time.Second,
				ReadTimeout: 60 * time.Second,
				WriteTimeout: 0,
			}

			go func() {
				ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
				defer stop()
				<-ctx.Done()
				logger.Infow("shutdown signal received...")
				close(stopCh)
			}()

			ln, err := net.Listen("tcp", addr)
			if err != nil {
				logger.Fatalw("listen failed", "addr", addr, "err", err)
			}

			serveErr := make(chan error, 1)
			go func() {
				serveErr <- httpSrv.Serve(ln)
			}()

			select {
			case err := <-serveErr:
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Fatalw("http serve failed", "err", err)
				}
			case <-stopCh:
				logger.Infow("stop signal received, shutting down...")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := httpSrv.Shutdown(shutdownCtx); err != nil {
					logger.Errorw("http shutdown failed", "err", err)
				}
				<-serveErr
				logger.Infow("http server stopped")
			}
		},
		nil, // stopFn: handled via stopCh closure
	) {
		return
	}

	// SO_REUSEADDR：服务重启时避免 "address already in use" 等 TIME_WAIT
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatalw("listen failed", "addr", addr, "err", err)
	}
	logger.Infof("Skill Factory started at http://localhost%s  build=%s", addr, BuildInfo)

	// 优雅关闭：stopCh 被关闭时触发 Shutdown（Windows Service 或 Ctrl+C 都会关闭它）
	stopCh := make(chan struct{})

	// 注册 service stopCh（Windows Service 模式会将 serviceStopCh 指向同一个 channel）
	serviceStopCh = stopCh

	// 非 Windows Service 模式下，监听 SIGINT/SIGTERM 信号来关闭 stopCh
	go func() {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		<-ctx.Done()
		logger.Infow("shutdown signal received...")
		close(stopCh)
	}()

	httpSrv := &http.Server{
		Handler:     srv,
		IdleTimeout: 30 * time.Second,
		ReadTimeout: 60 * time.Second,
		// WriteTimeout=0（无限制）：run-loop handler 已经异步化（立即返 202），其它
		// handler 都是 ms 级返回，不再需要短超时。原 60s 会在长任务上报文被截断。
		WriteTimeout: 0,
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpSrv.Serve(ln)
	}()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalw("http serve failed", "err", err)
		}
	case <-stopCh:
		logger.Infow("stop signal received, shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			logger.Errorw("http shutdown failed", "err", err)
		}
		<-serveErr
		logger.Infow("http server stopped")
	}
}

// stopHTTPServer gracefully shuts down the HTTP server (used by Windows Service stop).
func stopHTTPServer(srv *http.Server) {
	if srv == nil {
		return
	}
	logger.Infow("stopHTTPServer: initiating graceful shutdown...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Errorw("http shutdown failed", "err", err)
	}
}

// ---- eval-loop handlers ----

// handleTaskEvalHistory 返回任务的评估历史。
func (s *APIServer) handleTaskEvalHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.db.Get(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	execs, err := s.execDB.ListByTask(id, 50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type Step struct {
		ExecutionID    string     `json:"execution_id"`
		Score          *float64   `json:"score,omitempty"`
		Comments       string     `json:"comments,omitempty"`
		EvaluatorModel string     `json:"evaluator_model,omitempty"`
		CreatedAt      *time.Time `json:"created_at,omitempty"`
	}
	result := make([]Step, 0, len(execs))
	for _, e := range execs {
		step := Step{ExecutionID: e.ID}
		if evs, err := s.evalDB.ListByExecution(e.ID); err == nil && len(evs) > 0 {
			ev := evs[0]
			step.Score = &ev.Score
			step.Comments = ev.Comments
			step.EvaluatorModel = ev.EvaluatorModel
			step.CreatedAt = &ev.CreatedAt
		}
		result = append(result, step)
	}
	writeJSON(w, result)
}

// handleTaskReevaluate 用新模型重新评估最新 execution。
func (s *APIServer) handleTaskReevaluate(w http.ResponseWriter, r *http.Request) {
	if !s.aiLoopEnabled() {
		writeErr(w, http.StatusForbidden, "AI 自治能力未启用：在自动化页「高级设置」中开启，或设 config.json 顶层 ai_loop_enabled=true")
		return
	}
	id := r.PathValue("id")
	if _, err := s.db.Get(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct{ CliType, Model string }
	json.NewDecoder(r.Body).Decode(&req)
	// 默认值跟随 config.json 的 preferred_cli（单一来源）
	// evaluator 必须是真实 AI CLI（claude/cbc），shell 不能做评估
	if req.CliType == "" || !config.IsValidCLI(req.CliType) {
		req.CliType = preferredCLI()
	}
	if req.Model == "" {
		req.Model = evalDefaultModel(req.CliType)
	}
	execs, err := s.execDB.ListByTask(id, 1)
	if err != nil || len(execs) == 0 {
		writeErr(w, http.StatusBadRequest, "no execution to reevaluate")
		return
	}
	exec := execs[0]
	go func() {
		evaluator.RunAndSave(context.Background(), s.evalDB, s.execDB, exec, "reevaluate", req.CliType, req.Model)
	}()
	writeJSON(w, map[string]interface{}{"execution_id": exec.ID, "status": "reevaluating", "cli_type": req.CliType, "model": req.Model})
}

// handleTaskRunLoop 评估闭环：执行→评估→分数<阈值则换更强模型重试。
//
// 异步实现：handler 立即返回 202 + {task_id, status:"started"}，后台 goroutine
// 跑循环。每次迭代/完成/异常通过 wsmsg.ChannelExec 推送到前端，前端用
// handleExecStream 增量渲染进度。设计原因见 P1 修复（同步阻塞 + WriteTimeout
// 60s 双重矛盾让长任务必死）。Sync 版本已移除。
func (s *APIServer) handleTaskRunLoop(w http.ResponseWriter, r *http.Request) {
	if !s.aiLoopEnabled() {
		writeErr(w, http.StatusForbidden, "AI 自治能力未启用：在自动化页「高级设置」中开启，或设 config.json 顶层 ai_loop_enabled=true")
		return
	}
	id := r.PathValue("id")
	if _, err := s.db.Get(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct {
		Prompt        string  `json:"prompt"`
		Model         string  `json:"model"`
		CliType       string  `json:"cli_type"`
		Threshold     float64 `json:"threshold"`
		MaxIterations int     `json:"max_iterations"`
		GoalMode      bool    `json:"goal_mode"` // Goal 目标模式：/goal 前缀自动加到 prompt
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Threshold == 0 {
		req.Threshold = 7
	}
	if req.MaxIterations == 0 {
		req.MaxIterations = 3
	}
	if req.CliType == "" {
		req.CliType = preferredCLI()
	}
	models := []string{req.Model}
	if req.Model == "haiku" || req.Model == "" {
		models = []string{"haiku", "sonnet", "opus"}
	}

	// 任务级去重：同一 task 不能并发跑两个 run-loop 循环。
	// 否则 WS 推两组 iteration_started/iteration_done 事件，前端进度条错乱，
	// 数据库也会被并发写入同一 task 的多条 execution 记录。
	s.mu.Lock()
	if s.runLoops[id] {
		s.mu.Unlock()
		writeErr(w, http.StatusConflict, "run-loop already running for this task; wait for current loop to finish")
		return
	}
	s.runLoops[id] = true
	s.mu.Unlock()

	// 立即返回 202，让 client 不要再阻塞等循环跑完。
	writeJSON(w, map[string]interface{}{
		"task_id":        id,
		"status":         "started",
		"max_iterations": req.MaxIterations,
		"threshold":      req.Threshold,
	})

	// 后台 goroutine 跑循环，WS 推进度。
	// - iteration_started: 即将调 claude
	// - iteration_done: claude + evaluator 都跑完（带 score/exit_code）
	// - loop_done: 达到阈值/最大迭代/被 build 失败中断
	// - loop_error: 整个 goroutine panic
	go func() {
		// loop 开始时将 task 状态置为 running，结束/退出时恢复
		if err := s.db.UpdateStatus(id, backend.TaskStatusRunning, ""); err != nil {
			logger.Warnw("run-loop: set task running status failed", "task_id", id, "err", err.Error())
		}
		defer func() {
			s.mu.Lock()
			delete(s.runLoops, id)
			s.mu.Unlock()
			// 恢复为 pending（loop 结束后任务回到待认领状态，由用户决定下一步）
			s.db.UpdateStatus(id, backend.TaskStatusPending, "")
		}()
		s.runLoopBackground(id, req, models)
	}()
}

// runLoopBackground 是 handleTaskRunLoop 的后台执行体，单独拆出来便于测试和
// recover panic。WS payload 字段见 handleExecStream 的 run-loop 分支。
func (s *APIServer) runLoopBackground(taskID string, req struct {
	Prompt        string  `json:"prompt"`
	Model         string  `json:"model"`
	CliType       string  `json:"cli_type"`
	Threshold     float64 `json:"threshold"`
	MaxIterations int     `json:"max_iterations"`
	GoalMode      bool    `json:"goal_mode"`
}, models []string) {
	type Step struct {
		Iteration int      `json:"iteration"`
		Model     string   `json:"model"`
		ExitCode  int      `json:"exit_code"`
		Score     *float64 `json:"score,omitempty"`
		Comments  string   `json:"comments,omitempty"`
		Error     string   `json:"error,omitempty"`
	}
	history := []Step{}

	defer func() {
		if rec := recover(); rec != nil {
			logger.Errorw("run-loop background panic", "task_id", taskID, "panic", rec)
			s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
				"task_id": taskID, "event": "loop_error", "error": fmt.Sprintf("%v", rec),
			})
		}
	}()

	// 固定使用第一个模型（不再轮换）
	model := models[0]
	// session 续用：每轮拿到 resumeSessionID 后下轮继续用，保持上下文连贯
	var sessionID string
	// 跨轮累积输出：每轮 output 追加进 prompt，让模型看到完整迭代历史
	var outputs string

	for i := 0; i < req.MaxIterations; i++ {
		s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
			"task_id": taskID, "event": "iteration_started",
			"iteration": i + 1, "model": model,
		})

		// 拼 prompt：原始任务描述 + 上轮输出累积
		// Goal 模式：prompt 前加 /goal 前缀，让 Claude Code 以目标驱动模式执行
		var loopPrompt string
		if req.CliType == "claude" || req.CliType == "cbc" {
			basePrompt := req.Prompt
			if req.GoalMode {
				basePrompt = "/goal " + req.Prompt
			}
			loopPrompt = basePrompt + fmt.Sprintf(taskpkg.OutputDirHintTpl, paths.AITaskDir(taskID))
		} else {
			loopPrompt = req.Prompt
		}
		// 第二轮起：把之前所有轮次的输出追加进去，让模型看到完整的迭代历史
		if outputs != "" {
			loopPrompt += "\n\n--- 此前轮次的执行结果 ---\n" + outputs + "\n--- 请基于以上结果继续改进，直到达成目标。---\n"
		}
		skip := config.AppConfig != nil && config.AppConfig.DangerouslySkipPermissions
		var (
			cmd     []string
			stdin   string
			cleanup func()
			err     error
		)
		if skip {
			cmd, stdin, cleanup, err = runner.BuildCommand(req.CliType, model, sessionID, loopPrompt, runner.WithSkipPermissions(), runner.WithActionReport())
		} else {
			cmd, stdin, cleanup, err = runner.BuildCommand(req.CliType, model, sessionID, loopPrompt, runner.WithActionReport())
		}
		if err != nil {
			step := Step{Iteration: i + 1, Model: model, Error: err.Error()}
			history = append(history, step)
			s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
				"task_id": taskID, "event": "loop_done", "reason": "build_failed",
				"history": history,
			})
			return
		}

		exec := &backend.Execution{ID: uuid.New().String(), TaskID: taskID, Source: "loop", Command: runner.CmdStringWithPrompt(cmd, req.Prompt), Model: model, CliType: req.CliType, StartedAt: time.Now()}
		s.execDB.Create(exec)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		res, runErr := executor.Run(ctx, cmd, "", stdin, nil)
		cancel()

		if cleanup != nil {
			go func() { defer cleanup() }()
		}

		// session 冲突：claude 报 "Session ID already in use" 时，等待后用同一 session 重试
		if runErr != nil && strings.Contains(runErr.Error(), "already in use") && sessionID != "" {
			logger.Warnw("run-loop: session conflict, retrying with same session", "task_id", taskID, "iteration", i+1)
			time.Sleep(2 * time.Second) // 等待 session 释放
			cmd2, stdin2, cleanup2, err2 := runner.BuildCommand(req.CliType, model, sessionID, loopPrompt, runner.WithActionReport())
			if err2 == nil {
				ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Minute)
				res2, runErr2 := executor.Run(ctx2, cmd2, "", stdin2, nil)
				cancel2()
				if cleanup2 != nil {
					go func() { defer cleanup2() }()
				}
				res, runErr = res2, runErr2
			}
		}

		exitCode := -1
		var out, errOut string
		if res != nil {
			out = res.Output
			exitCode = res.ExitCode
			if res.ErrorOut != "" {
				errOut = res.ErrorOut
			} else if res.Err != nil {
				errOut = "executor: " + res.Err.Error()
			}
		}
		if runErr != nil && errOut == "" {
			errOut = "run: " + runErr.Error()
		}
		resumeSessionID := extractResumeSessionID(out)
		if resumeSessionID != "" {
			sessionID = resumeSessionID // 记住 session，下轮续用
		}
		s.execDB.Finish(exec.ID, out, errOut, exitCode, resumeSessionID)

		step := Step{Iteration: i + 1, Model: model, ExitCode: exitCode}
		if runErr != nil {
			step.Error = runErr.Error()
			logger.Warnw("run-loop executor failed", "task_id", taskID, "iteration", i+1, "err", runErr.Error())
			outputs += fmt.Sprintf("\n[第 %d 轮 执行失败: %s]", i+1, runErr.Error())
		} else {
			// 把本轮输出累积进 context
			// 每轮输出截断到 3000 字符，避免 prompt 无限膨胀
			outPart := out
			if len(outPart) > 3000 {
				outPart = outPart[:3000] + "\n...(输出截断)"
			}
			outputs += fmt.Sprintf("\n--- 第 %d 轮输出 ---\n%s", i+1, outPart)
			// 评估
			evalCLI := preferredCLI()
			evalModel := evalDefaultModel(req.CliType)
			if evID, err := evaluator.RunAndSave(context.Background(), s.evalDB, s.execDB, exec, req.Prompt, evalCLI, evalModel); err == nil {
				if evs, _ := s.evalDB.ListByExecution(exec.ID); len(evs) > 0 {
					step.Score = &evs[0].Score
					step.Comments = evs[0].Comments
					// 评语也累积进 context，让模型在下一轮看到完整反馈
					if evs[0].Comments != "" {
						outputs += fmt.Sprintf("\n[第 %d 轮评估] 评分: %.1f / 目标: %.1f，评语: %s", i+1, *step.Score, req.Threshold, evs[0].Comments)
					} else {
						outputs += fmt.Sprintf("\n[第 %d 轮评估] 评分: %.1f / 目标: %.1f", i+1, *step.Score, req.Threshold)
					}
				}
				_ = evID
			}
		}
		history = append(history, step)

		s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
			"task_id":      taskID,
			"event":        "iteration_done",
			"iteration":    i + 1,
			"model":        model,
			"exit_code":    exitCode,
			"execution_id": exec.ID,
			"score":        step.Score,
			"comments":     step.Comments,
			"error":        step.Error,
		})

		if step.Score != nil && *step.Score >= req.Threshold {
			s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
				"task_id": taskID, "event": "loop_done", "reason": "threshold_met",
				"history": history,
			})
			return
		}
	}

	s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
		"task_id": taskID, "event": "loop_done", "reason": "max_iterations",
		"history": history,
	})
}

// ---- Remote Agent Handlers ----

// handleAgentRegister Agent 注册。生成 agent_id 和一个随机 token（存 hash）。
func (s *APIServer) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		Capabilities string `json:"capabilities"`
		Version      string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	// 生成 agent id 和 token
	agentID := uuid.New().String()
	token := uuid.New().String()
	tokenHash := backend.HashToken(token) // SHA-256 hash，不存明文

	a := &backend.Agent{
		ID:           agentID,
		Name:         req.Name,
		TokenHash:    tokenHash,
		Capabilities: req.Capabilities,
		Version:      req.Version,
		Status:       "online",
		CreatedAt:    time.Now(),
	}
	if err := s.agentDB.Register(a); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Infow("agent registered", "agent_id", agentID, "name", req.Name)
	writeJSON(w, map[string]any{
		"agent_id":      agentID,
		"token":         token, // 仅此时返回，之后不再暴露
		"name":          req.Name,
		"status":        "online",
		"registered_at": a.CreatedAt,
	})
}

// handleAgentHeartbeat Agent 心跳。Header: Authorization: Bearer <token>
func (s *APIServer) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	token := extractBearerToken(r)
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "missing token")
		return
	}
	// 验证 token
	a, err := s.agentDB.GetByToken(token)
	if err != nil || a.ID != agentID {
		writeErr(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var req struct {
		Status        string `json:"status"`
		CurrentTaskID string `json:"current_task_id"`
	}
	json.NewDecoder(r.Body).Decode(&req) // body 可选

	updated, err := s.agentDB.UpdateHeartbeat(agentID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"ok":          true,
		"server_time": time.Now(),
		"agent":       updated,
	})
}

// handleTaskClaim 远程 Agent claim 任务。
func (s *APIServer) handleTaskClaim(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	token := extractBearerToken(r)
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "missing token")
		return
	}
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 验证 token 对应 agent
	a, err := s.agentDB.GetByToken(token)
	if err != nil || a.ID != req.AgentID {
		writeErr(w, http.StatusUnauthorized, "invalid token or agent_id mismatch")
		return
	}
	if err := s.db.ClaimTask(taskID, req.AgentID); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	task, _ := s.db.Get(taskID)
	task.ExperienceIDs, _ = s.db.ListExperienceIDsForTask(taskID)
	experiences := s.loadExperiencesForTask(task)
	// 预生成 agent 可直接用的完整 prompt（含 task 三要素 + 经验库内容 + 输出目录约定）
	// agent 端无需自己拼 prompt，直接调 claude CLI 时把这个字段作为 stdin/参数传入。
	// 输出目录约定告诉 agent「把文件写到 data/ai-task-dir/<task_id>/」。
	prompt := taskpkg.BuildTaskPromptWithOutput(task, paths.AITaskDir(taskID), experiences...)
	// 审计
	s.eventDB.Record(&backend.TaskEvent{
		TaskID: taskID, EventType: "claimed",
		Actor: "agent:" + req.AgentID, CreatedAt: time.Now(),
	})
	writeJSON(w, map[string]any{
		"status":      "claimed",
		"task":        task,
		"experiences": experiences,
		"prompt":      prompt,
	})
}

// handleTaskReport 远程 Agent 上报执行结果。
func (s *APIServer) handleTaskReport(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	token := extractBearerToken(r)
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "missing token")
		return
	}
	var req struct {
		AgentID         string   `json:"agent_id"`
		Status          string   `json:"status"`
		ResultOutput    string   `json:"result_output"`
		EvaluationScore *float64 `json:"evaluation_score"`
		LastError       string   `json:"last_error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 验证 token
	a, err := s.agentDB.GetByToken(token)
	if err != nil || a.ID != req.AgentID {
		writeErr(w, http.StatusUnauthorized, "invalid token or agent_id mismatch")
		return
	}
	if req.Status == "" {
		req.Status = backend.TaskStatusArchived
	}
	if err := s.db.ReportTask(taskID, req.AgentID, req.Status, req.ResultOutput, req.EvaluationScore, req.LastError); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	logger.Infow("task report received", "task_id", taskID, "agent_id", req.AgentID, "status", req.Status)
	// 审计
	scoreStr := "null"
	if req.EvaluationScore != nil {
		scoreStr = fmt.Sprintf("%v", *req.EvaluationScore)
	}
	s.eventDB.Record(&backend.TaskEvent{
		TaskID: taskID, EventType: "reported",
		Actor:     "agent:" + req.AgentID,
		Payload:   fmt.Sprintf(`{"status":"%s","score":%s}`, req.Status, scoreStr),
		CreatedAt: time.Now(),
	})
	// WebSocket 广播任务状态变更
	task, _ := s.db.Get(taskID)
	s.hub.Broadcast(wsmsg.ChannelTask, map[string]any{
		"task_id": taskID,
		"status":  req.Status,
		"task":    task,
	})
	writeJSON(w, map[string]any{"ok": true, "task_id": taskID})
}

// extractBearerToken 从 Authorization header 提取 Bearer token。
func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}

// startAgentHeartbeatChecker 启动后台 goroutine 定期检查 agent 心跳 + 任务超时。
// 1) 心跳超时（默认 30s）：agent 标记 offline，手上任务还回 pending
// 2) 任务超时（默认 10min）：claimed 太久未完成的任务也强制释放，避免 agent 心跳还在但任务托死
func startAgentHeartbeatChecker(agentRepo *backend.AgentRepo, taskRepo *backend.TaskRepo, eventRepo *backend.TaskEventRepo, h *hub.Hub, hbTimeout time.Duration, taskTimeout time.Duration) {
	go func() {
		ticker := time.NewTicker(hbTimeout / 2) // 每半个心跳周期检查一次
		defer ticker.Stop()
		for range ticker.C {
			// 1) agent 心跳超时
			stale, err := agentRepo.ListStaleAgents(int(hbTimeout.Seconds()))
			if err != nil {
				logger.Errorw("list stale agents failed", "err", err)
			} else {
				for _, agentID := range stale {
					if err := agentRepo.SetStatusOffline(agentID); err != nil {
						logger.Errorw("set agent offline failed", "agent_id", agentID, "err", err)
						continue
					}
					released, _ := taskRepo.ReleaseTasksFromAgent(agentID)
					logger.Warnw("agent heartbeat timeout", "agent_id", agentID, "released_tasks", released)
					eventRepo.Record(&backend.TaskEvent{
						TaskID: "", EventType: "heartbeat_lost",
						Actor:     "system:" + agentID,
						Payload:   fmt.Sprintf(`{"released_tasks":%d}`, released),
						CreatedAt: time.Now(),
					})
					h.Broadcast(wsmsg.ChannelAgent, map[string]any{
						"event":          "agent_offline",
						"agent_id":       agentID,
						"released_tasks": released,
					})
				}
			}
			// 2) 任务超时
			released, err := taskRepo.ReleaseStaleTasks(int(taskTimeout.Seconds()))
			if err != nil {
				logger.Errorw("release stale tasks failed", "err", err)
			} else if released > 0 {
				logger.Warnw("released stale tasks", "count", released, "timeout_sec", int(taskTimeout.Seconds()))
				eventRepo.Record(&backend.TaskEvent{
					TaskID: "", EventType: "task_timeout",
					Actor:     "system",
					Payload:   fmt.Sprintf(`{"count":%d,"timeout_sec":%d}`, released, int(taskTimeout.Seconds())),
					CreatedAt: time.Now(),
				})
				h.Broadcast(wsmsg.ChannelAgent, map[string]any{
					"event":  "tasks_released",
					"count":  released,
					"reason": "task_claim_timeout",
				})
			}
		}
	}()
}

// ---- 审计 + 依赖 Handlers ----

// handleTaskEvents 返回某 task 的所有审计事件（时间倒序）。
func (s *APIServer) handleTaskEvents(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	limit := parseInt(r.URL.Query().Get("limit"), 100)
	events, err := s.eventDB.ListByTask(taskID, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, events)
}

func (s *APIServer) handleCommentList(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	cmts, err := s.cmtDB.ListByTask(taskID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, cmts)
}

func (s *APIServer) handleCommentCreate(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	var req struct {
		Author   string `json:"author"`
		Content  string `json:"content"`
		Mentions string `json:"mentions"`
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Content == "" {
		writeErr(w, http.StatusBadRequest, "content is required")
		return
	}
	if req.Author == "" {
		req.Author = "user"
	}
	c := &backend.TaskComment{
		TaskID: taskID, Author: req.Author, Content: req.Content,
		Mentions: req.Mentions, ParentID: req.ParentID,
	}
	if err := s.cmtDB.Create(c); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 审计
	s.eventDB.Record(&backend.TaskEvent{
		TaskID: taskID, EventType: "commented",
		Actor: req.Author, CreatedAt: time.Now(),
	})
	writeJSON(w, c)
}

func (s *APIServer) handleCommentUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.cmtDB.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct {
		Content  string `json:"content"`
		Mentions string `json:"mentions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Content != "" {
		c.Content = req.Content
	}
	c.Mentions = req.Mentions
	if err := s.cmtDB.Update(c); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, c)
}

func (s *APIServer) handleCommentDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.cmtDB.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"id": id, "status": "deleted"})
}

// ---- ExecutionComment Handlers ----

func (s *APIServer) handleExecutionCommentList(w http.ResponseWriter, r *http.Request) {
	execID := r.PathValue("id")
	cmts, err := s.execCmtDB.ListByExecution(execID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, cmts)
}

func (s *APIServer) handleExecutionCommentCreate(w http.ResponseWriter, r *http.Request) {
	execID := r.PathValue("id")
	var req struct {
		Author   string `json:"author"`
		Content  string `json:"content"`
		Mentions string `json:"mentions"`
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Content == "" {
		writeErr(w, http.StatusBadRequest, "content is required")
		return
	}
	if req.Author == "" {
		req.Author = "user"
	}
	c := &backend.ExecutionComment{
		ExecutionID: execID, Author: req.Author, Content: req.Content,
		Mentions: req.Mentions, ParentID: req.ParentID,
	}
	if err := s.execCmtDB.Create(c); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, c)
}

func (s *APIServer) handleExecutionCommentDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.execCmtDB.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"id": id, "status": "deleted"})
}

// handleTaskClaimNext 任务优先级队列：自动领下一个最高优先级任务。
// 支持 GET (long-poll, ?timeout=30) 和 POST (即时返回)。
// 找不到可领任务时返回 204。
// (在 agentHandler 注册时被 ratelimit 包裹)
func (s *APIServer) handleTaskClaimNext(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "missing token")
		return
	}
	var agentID string
	var timeoutSec int

	if r.Method == "GET" {
		// Long-poll: agent 每隔 N 秒轮询，等有任务再返回
		agentID = r.URL.Query().Get("agent_id")
		timeoutSec = parseInt(r.URL.Query().Get("timeout"), 10)
		if timeoutSec <= 0 || timeoutSec > 60 {
			timeoutSec = 10
		}
	} else {
		var req struct{ AgentID string `json:"agent_id"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		agentID = req.AgentID
		timeoutSec = 0 // POST 即时返回，不等待
	}

	a, err := s.agentDB.GetByToken(token)
	if err != nil || a.ID != agentID {
		writeErr(w, http.StatusUnauthorized, "invalid token or agent_id mismatch")
		return
	}
	if !a.AutoClaimEnabled {
		writeErr(w, http.StatusForbidden, "auto claim is disabled for this agent")
		return
	}

	// 尝试立即 claim（无等待路径）
	doClaim := func() (taskID string, err error) {
		tid, err := s.db.NextClaimable(agentID)
		if err != nil {
			return "", err
		}
		if tid == "" {
			return "", nil
		}
		if err := s.db.ClaimTask(tid, agentID); err != nil {
			return "", err
		}
		return tid, nil
	}

	taskID, err := doClaim()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 无任务 → long-poll 等候
	if taskID == "" && timeoutSec > 0 {
		logger.Infow("claim-next: no task, starting long-poll", "agent_id", agentID, "timeout_sec", timeoutSec)
		type claimResult struct {
			taskID string
			err    error
		}
		resultCh := make(chan claimResult, 1)
		go func() {
			for elapsed := 0; elapsed < timeoutSec; elapsed += 2 {
				time.Sleep(2 * time.Second)
				tid, err := doClaim()
				if err != nil {
					resultCh <- claimResult{"", err}
					return
				}
				if tid != "" {
					resultCh <- claimResult{tid, nil}
					return
				}
			}
			resultCh <- claimResult{"", nil} // timeout 后返回空
		}()

		select {
		case res := <-resultCh:
			if res.err != nil {
				writeErr(w, http.StatusInternalServerError, res.err.Error())
				return
			}
			taskID = res.taskID
		case <-r.Context().Done():
			return // 客户端断开
		}
	}

	if taskID == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Claim 成功 → 构建响应
	task, _ := s.db.Get(taskID)
	task.ExperienceIDs, _ = s.db.ListExperienceIDsForTask(taskID)
	experiences := s.loadExperiencesForTask(task)
	prompt := taskpkg.BuildTaskPromptWithOutput(task, paths.AITaskDir(taskID), experiences...)
	s.eventDB.Record(&backend.TaskEvent{
		TaskID: taskID, EventType: "claimed_via_priority",
		Actor: "agent:" + agentID, CreatedAt: time.Now(),
	})
	writeJSON(w, map[string]any{
		"status":      "claimed",
		"task_id":     taskID,
		"task":        task,
		"experiences": experiences,
		"prompt":      prompt,
		"output_dir":  paths.AITaskDir(taskID),
	})
}

// ---- 远程 Agent 管理 API（主用户调用）----

// handleAgentsList 返回 Agent 列表。可选 ?status=online|offline 过滤。
func (s *APIServer) handleAgentsList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	agents, err := s.agentDB.List(status)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 附带每个 agent 当前手上的任务数（轻量统计，不查全量）
	type agentWithStats struct {
		*backend.Agent
		CurrentTaskCount int `json:"current_task_count"`
	}
	out := make([]agentWithStats, 0, len(agents))
	for _, a := range agents {
		n, _ := s.db.CountInProgressByAgent(a.ID)
		out = append(out, agentWithStats{Agent: a, CurrentTaskCount: n})
	}
	writeJSON(w, out)
}

// handleAgentReleaseTasks 强制释放某 agent 手上所有 in_progress 的 remote 任务回 pending 池。
// 场景：agent 卡死但心跳还在；管理员想强制回收。
func (s *APIServer) handleAgentReleaseTasks(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	a, err := s.agentDB.GetByID(agentID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	released, err := s.db.ReleaseTasksFromAgent(agentID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 审计
	s.eventDB.Record(&backend.TaskEvent{
		TaskID: "", EventType: "force_released",
		Actor:     "user",
		Payload:   fmt.Sprintf(`{"agent_id":"%s","released_tasks":%d}`, agentID, released),
		CreatedAt: time.Now(),
	})
	// ws 广播
	s.hub.Broadcast(wsmsg.ChannelAgent, map[string]any{
		"event":          "tasks_released",
		"agent_id":       agentID,
		"released_tasks": released,
	})
	logger.Infow("agent tasks force released", "agent_id", agentID, "agent_name", a.Name, "count", released)
	writeJSON(w, map[string]any{"ok": true, "released_tasks": released})
}

// handleAgentResetToken 重置 agent token，返回新明文 token（仅此次返回）。
// 旧 token 立即失效。
func (s *APIServer) handleAgentResetToken(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	a, err := s.agentDB.GetByID(agentID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	newToken, err := s.agentDB.ResetToken(agentID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 审计
	s.eventDB.Record(&backend.TaskEvent{
		TaskID: "", EventType: "token_reset",
		Actor:     "user",
		Payload:   fmt.Sprintf(`{"agent_id":"%s","agent_name":"%s"}`, agentID, a.Name),
		CreatedAt: time.Now(),
	})
	logger.Warnw("agent token reset", "agent_id", agentID, "agent_name", a.Name)
	writeJSON(w, map[string]any{
		"ok":        true,
		"agent_id":  agentID,
		"new_token": newToken,
		"warning":   "旧 token 已立即失效，请把新 token 同步到 agent 端",
	})
}

// handleAgentSetAutoClaim 切换 agent 的 auto_claim_enabled 开关。Body: {"enabled": true|false}
func (s *APIServer) handleAgentSetAutoClaim(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.agentDB.SetAutoClaimEnabled(agentID, req.Enabled); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "agent_id": agentID, "auto_claim_enabled": req.Enabled})
}

// handleAgentSetBoundDirShortcut 绑定/解绑 agent 到一个 type=remote 的 dir_shortcut。
// Body: {"dir_shortcut_id": "uuid..."}  空字符串 = 解绑（恢复本机/主动 claim 模式）。
// 绑定后，任务页选这个 agent 走 SSH 远端执行。
func (s *APIServer) handleAgentSetBoundDirShortcut(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	var req struct {
		DirShortcutID string `json:"dir_shortcut_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DirShortcutID != "" {
		// 验证 id 对得上 + type=remote
		ds, err := s.dirDB.GetByID(req.DirShortcutID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if ds == nil {
			writeErr(w, http.StatusNotFound, "dir_shortcut not found")
			return
		}
		if ds.Type != backend.DirShortcutTypeRemote {
			writeErr(w, http.StatusBadRequest, "dir_shortcut is not type=remote (need remote ssh config)")
			return
		}
	}
	if err := s.agentDB.SetBoundDirShortcut(agentID, req.DirShortcutID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Infow("agent bound to dir_shortcut", "agent_id", agentID, "dir_shortcut_id", req.DirShortcutID)
	writeJSON(w, map[string]any{"ok": true, "agent_id": agentID, "bound_dir_shortcut_id": req.DirShortcutID})
}

// handleSkillsList 返回所有已注册的 skill 技能列表（排除内部工具）。
func (s *APIServer) handleSkillsList(w http.ResponseWriter, r *http.Request) {
	skills := skill.GetPublic()
	// 转换为前端需要的格式
	type SkillInfo struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Version     string            `json:"version,omitempty"`
		Params      map[string]string `json:"params"`
		Output      map[string]string `json:"output"`
		Examples    []struct {
			Description string         `json:"description"`
			Params      map[string]any `json:"params"`
		} `json:"examples"`
	}
	var out []SkillInfo
	for _, s := range skills {
		examples := make([]struct {
			Description string         `json:"description"`
			Params      map[string]any `json:"params"`
		}, len(s.XWExamples))
		for i, ex := range s.XWExamples {
			examples[i] = struct {
				Description string         `json:"description"`
				Params      map[string]any `json:"params"`
			}{Description: ex.Description, Params: ex.Params}
		}
		out = append(out, SkillInfo{
			Name:        s.Name,
			Description: s.Description,
			Version:     s.Version,
			Params:      s.XWParams,
			Output:      s.XWOutput,
			Examples:    examples,
		})
	}
	writeJSON(w, map[string]any{"skills": out})
}

// handleSkillsExecute 执行指定的 skill 技能。
func (s *APIServer) handleSkillsExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name   string         `json:"name"`
		Params map[string]any `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	result, err := skill.Execute(req.Name, req.Params)
	if err != nil {
		// skill 执行失败，但 result 里可能有有用的输出信息（如超时、脚本错误等）
		// 只有"找不到 skill"才返回 404，其他情况返回结果让前端展示
		if result == nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
	}
	writeJSON(w, map[string]any{
		"status":  result.Status,
		"output":  result.Output,
		"raw_out": result.RawOut,
		"raw_err": result.RawErr,
	})
}

// handleSkillsCreate 根据用户输入创建新的 skill（目前支持 HTTP 请求类 skill）。
// 请求体：{name, description, url, method, headers, body, output}
// output 为 map[key]=jsonPath，如 {"ip":"ip","city":"city"}
func (s *APIServer) handleSkillsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		URL         string            `json:"url"`
		Method      string            `json:"method"`
		Headers     map[string]string `json:"headers"`
		Body        string            `json:"body"`
		Output      map[string]string `json:"output"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" || req.URL == "" {
		writeErr(w, http.StatusBadRequest, "name and url are required")
		return
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	if req.Headers == nil {
		req.Headers = map[string]string{}
	}
	if req.Output == nil {
		req.Output = map[string]string{}
	}

	// 验证目录不存在
	skillDir := filepath.Join(skill.ToolsDir, req.Name)
	if _, err := os.Stat(skillDir); err == nil {
		writeErr(w, http.StatusConflict, "skill already exists: "+req.Name)
		return
	}

	// 创建目录
	scriptsDir := filepath.Join(skillDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		writeErr(w, http.StatusInternalServerError, "mkdir failed: "+err.Error())
		return
	}

	// --- 生成 SKILL.md ---
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", req.Name))
	sb.WriteString(fmt.Sprintf("description: %s\n", req.Description))
	sb.WriteString("version: 1.0.0\n")
	sb.WriteString("xw_command: python3 scripts/check.py\n")
	sb.WriteString("xw_params:\n")
	for k := range req.Headers {
		sb.WriteString(fmt.Sprintf("  %s: 请求头 %s\n", k, k))
	}
	if req.Method == "POST" || req.Method == "PUT" {
		sb.WriteString("  body: 请求体（JSON 字符串）\n")
	}
	sb.WriteString("xw_output:\n")
	if len(req.Output) > 0 {
		for k, v := range req.Output {
			sb.WriteString(fmt.Sprintf("  %s: JSONPath %s\n", k, v))
		}
	} else {
		sb.WriteString("  raw: 完整响应 JSON\n")
	}
	sb.WriteString("xw_examples:\n")
	sb.WriteString(fmt.Sprintf("  - description: 调用 %s\n", req.Name))
	sb.WriteString("    params: {}\n---\n")

	// --- 生成 check.py ---
	// 计算 http_util 的相对路径

	var checkPy strings.Builder
	checkPy.WriteString("#!/usr/bin/env python3\n")
	checkPy.WriteString(fmt.Sprintf(`"""Auto-generated skill: %s
Description: %s
URL: %s %s
"""`, req.Name, req.Description, req.Method, req.URL))
	checkPy.WriteString("\nimport json, sys\nsys.path.insert(0, \"../http_util/http_util\")\nfrom http_util.http_util import json_request\n\ndef main():\n    params = json.load(sys.stdin)\n")

	// headers
	headersJSON, _ := json.Marshal(req.Headers)
	checkPy.WriteString(fmt.Sprintf("    headers = %s\n", headersJSON))

	// body
	if req.Body != "" {
		bodyJSON, _ := json.Marshal(req.Body)
		checkPy.WriteString(fmt.Sprintf("    body = %s\n", bodyJSON))
	}

	// request call
	checkPy.WriteString(fmt.Sprintf("    resp = json_request(%q, method=%q", req.URL, req.Method))
	if req.Body != "" {
		checkPy.WriteString(", body=body")
	}
	checkPy.WriteString(")\n\n")

	// output processing
	if len(req.Output) > 0 {
		checkPy.WriteString("    out = {}\n")
		for outKey, jsonPath := range req.Output {
			checkPy.WriteString(fmt.Sprintf("    # %s: JSONPath %s\n", outKey, jsonPath))
			checkPy.WriteString(fmt.Sprintf(`    try:
        parts = %q.split('.')
        val = resp
        for p in parts:
            if isinstance(val, dict):
                val = val[p]
            elif isinstance(val, list):
                val = val[int(p)]
            else:
                val = None
        out[%q] = val
    except:
        out[%q] = None
`, jsonPath, outKey, outKey))
		}
		checkPy.WriteString("    print(json.dumps({\"status\": \"ok\", **out}, ensure_ascii=False))\n")
	} else {
		checkPy.WriteString("    print(json.dumps({\"status\": \"ok\", \"raw\": resp}, ensure_ascii=False))\n")
	}

	checkPy.WriteString("\nif __name__ == \"__main__\":\n    main()\n")

	// 写文件
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(sb.String()), 0644); err != nil {
		writeErr(w, http.StatusInternalServerError, "write SKILL.md failed: "+err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "check.py"), []byte(checkPy.String()), 0755); err != nil {
		writeErr(w, http.StatusInternalServerError, "write check.py failed: "+err.Error())
		return
	}

	logger.Infow("skill created", "name", req.Name, "dir", skillDir)

	// 热更新 skill registry，使新创建的 skill 立即可用
	if err := skill.Reload(); err != nil {
		logger.Warnw("skill reload after create failed", "err", err)
	}
	writeJSON(w, map[string]any{"ok": true, "name": req.Name, "dir": skillDir})
}

// handleAgentDelete 删除 agent（先释放任务再删，避免遗留 in_progress）。
func (s *APIServer) handleAgentDelete(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	a, err := s.agentDB.GetByID(agentID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	released, _ := s.db.ReleaseTasksFromAgent(agentID)
	if err := s.agentDB.Delete(agentID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventDB.Record(&backend.TaskEvent{
		TaskID: "", EventType: "agent_deleted",
		Actor:     "user",
		Payload:   fmt.Sprintf(`{"agent_id":"%s","agent_name":"%s","released_tasks":%d}`, agentID, a.Name, released),
		CreatedAt: time.Now(),
	})
	logger.Warnw("agent deleted", "agent_id", agentID, "agent_name", a.Name, "released_tasks", released)
	writeJSON(w, map[string]any{"ok": true, "released_tasks": released})
}
