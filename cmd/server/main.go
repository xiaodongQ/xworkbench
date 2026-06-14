package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
	"github.com/xiaodongQ/xworkbench/internal/evaluator"
	"github.com/xiaodongQ/xworkbench/internal/executor"
	"github.com/xiaodongQ/xworkbench/internal/executor/runner"
	"github.com/xiaodongQ/xworkbench/internal/hub"
	"github.com/xiaodongQ/xworkbench/internal/paths"
	"github.com/xiaodongQ/xworkbench/internal/scheduler"
	"github.com/xiaodongQ/xworkbench/internal/shortcuts"
	taskpkg "github.com/xiaodongQ/xworkbench/internal/task"
	"github.com/xiaodongQ/xworkbench/internal/todo"
	"github.com/xiaodongQ/xworkbench/internal/httplog"
	"github.com/xiaodongQ/xworkbench/internal/relay"
	"github.com/xiaodongQ/xworkbench/internal/wsmsg"
)

//go:embed index.html static
var FS embed.FS

type APIServer struct {
	db     *backend.TaskRepo
	expDB  *backend.ExperienceRepo
	execDB *backend.ExecutionRepo
	linkDB *backend.WebLinkRepo
	dirDB  *backend.DirShortcutRepo
	schedDB *backend.ScheduledTaskRepo
	setDB  *backend.AppSettingsRepo
	evalDB *backend.EvaluationRepo
	sch    *scheduler.Scheduler
	hub    *hub.Hub
	relayHandler *relay.RelayHandler
	mux       *http.ServeMux
	wrapped   http.Handler // mux + httplog.Middleware

	// 进程内运行中的执行（task_id → cancel func）
	mu      sync.Mutex
	running map[string]context.CancelFunc
}

func NewAPIServer(
	db *backend.TaskRepo, expDB *backend.ExperienceRepo, execDB *backend.ExecutionRepo,
	linkDB *backend.WebLinkRepo, dirDB *backend.DirShortcutRepo,
	schedDB *backend.ScheduledTaskRepo, setDB *backend.AppSettingsRepo,
	evalDB *backend.EvaluationRepo, sch *scheduler.Scheduler, h *hub.Hub,
	relayRepo relay.Repo,
) *APIServer {
	s := &APIServer{
		db: db, expDB: expDB, execDB: execDB,
		linkDB: linkDB, dirDB: dirDB, schedDB: schedDB, setDB: setDB, evalDB: evalDB,
		sch: sch, hub: h,
		relayHandler: relay.NewRelayHandler(relayRepo),
		mux: http.NewServeMux(), running: map[string]context.CancelFunc{},
	}
	s.routes()
	s.wrapped = httplog.Middleware(s.mux)
	return s
}

func (s *APIServer) routes() {
	mux := s.mux
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
	mux.HandleFunc("GET /api/executions", s.handleExecutionsRecent)
	mux.HandleFunc("GET /api/executions/{id}", s.handleExecutionGet)
	mux.HandleFunc("POST /api/executions/{id}/evaluate", s.handleExecutionEvaluate)
	mux.HandleFunc("GET /api/executions/{id}/evaluations", s.handleExecutionEvaluations)
	mux.HandleFunc("GET /api/experiences", s.handleExperiences)
	mux.HandleFunc("POST /api/experiences", s.handleExpCreate)
		mux.HandleFunc("PUT /api/experiences/{id}", s.handleExpUpdate)
		mux.HandleFunc("DELETE /api/experiences/{id}", s.handleExpDelete)
	mux.HandleFunc("GET /api/experiences/{id}", s.handleExpGet)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/pty", s.handlePty)
	mux.HandleFunc("POST /api/pty/{tab_id}/submit-input", s.handlePtyInput)
	mux.HandleFunc("GET /ws", s.handleWS)
	// /static/* 用 embed.FS serve 拆分 CSS/JS 文件
	mux.Handle("GET /static/", http.FileServer(http.FS(FS)))
	mux.HandleFunc("GET /", s.handleIndex)

	// 5 个新功能
	mux.HandleFunc("GET /api/web-links", s.handleWebLinks)
	mux.HandleFunc("POST /api/web-links", s.handleWebLinkCreate)
	mux.HandleFunc("PUT /api/web-links/{id}", s.handleWebLinkUpdate)
	mux.HandleFunc("DELETE /api/web-links/{id}", s.handleWebLinkDelete)

	mux.HandleFunc("GET /api/dir-shortcuts", s.handleDirShortcuts)
	mux.HandleFunc("POST /api/dir-shortcuts", s.handleDirShortcutCreate)
	mux.HandleFunc("PUT /api/dir-shortcuts/{id}", s.handleDirShortcutUpdate)
	mux.HandleFunc("DELETE /api/dir-shortcuts/{id}", s.handleDirShortcutDelete)
	mux.HandleFunc("POST /api/dir-shortcuts/{id}/open", s.handleDirShortcutOpen)
	mux.HandleFunc("POST /api/dir-shortcuts/{id}/open-terminal", s.handleDirShortcutOpenTerminal)
	mux.HandleFunc("GET /api/terminals", s.handleTerminalList)
	mux.HandleFunc("GET /api/terminals/detect", s.handleTerminalDetect)
	mux.HandleFunc("GET /api/models", s.handleModelList)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handleSetConfig)
	mux.HandleFunc("GET /api/settings/default_terminal", s.handleGetDefaultTerminal)
	mux.HandleFunc("PUT /api/settings/default_terminal", s.handleSetDefaultTerminal)

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

	mux.HandleFunc("GET /api/todo", s.handleTodo)
	mux.HandleFunc("POST /api/todo", s.handleTodoAdd)
	mux.HandleFunc("PUT /api/todo/{line_no}", s.handleTodoToggle)
	mux.HandleFunc("DELETE /api/todo/{line_no}", s.handleTodoDelete)
	mux.HandleFunc("GET /api/todo/path", s.handleTodoPath)
	mux.HandleFunc("PUT /api/todo/path", s.handleTodoPathSet)

	mux.HandleFunc("GET /api/settings", s.handleSettingsList)
	mux.HandleFunc("PUT /api/settings/{key}", s.handleSettingsSet)

	// relay 代理功能
	mux.HandleFunc("POST /api/exec", relay.HandleExec)
	mux.HandleFunc("POST /api/relay/proxy", s.relayHandler.HandleRelayProxy)
	mux.HandleFunc("GET /api/relay/stats", s.relayHandler.HandleRelayStats)
}

func (s *APIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.wrapped.ServeHTTP(w, r)
}

// Tasks

func (s *APIServer) handleTasks(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	offset := parseInt(r.URL.Query().Get("offset"), 0)
	limit := parseInt(r.URL.Query().Get("limit"), 50)

	tasks, err := s.db.List(backend.TaskFilter{Status: status, Offset: offset, Limit: limit})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, tasks)
}

func (s *APIServer) handleTaskCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title        string   `json:"title"`
		Description  string   `json:"description"`
		ExperienceID string   `json:"experience_id"` // 旧字段（单值），保留向后兼容
		ExperienceIDs []string `json:"experience_ids"` // 新字段：多经验关联
		Resources    string   `json:"resources"`
		Acceptance   string   `json:"acceptance"`
		Module       string   `json:"module"`
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
		Resources:    req.Resources,
		Acceptance:   req.Acceptance,
		Status:       backend.TaskStatusPending,
		Version:      "v0.0.1",
		CreatedAt:    time.Now(),
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
	writeJSON(w, task)
}

func (s *APIServer) handleTaskUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Title        string   `json:"title"`
		Description  string   `json:"description"`
		ExperienceID string   `json:"experience_id"`
		ExperienceIDs []string `json:"experience_ids"`
		Resources    string   `json:"resources"`
		Acceptance   string   `json:"acceptance"`
		Module       string   `json:"module"`
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
	task.Resources = req.Resources
	task.Acceptance = req.Acceptance
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
		Status string            `json:"status"`
		Maintainer string            `json:"maintainer"`
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
	var req struct {
		Module       string `json:"module"`
		Keywords     string `json:"keywords"`
		LogPaths     string `json:"log_paths"`
		ToolUsage   string `json:"tool_usage"`
		Scene       string `json:"scene"`
		LogSamples  string `json:"log_samples"`
		CodeSnippets string `json:"code_snippets"`
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
	exp.LogPaths = req.LogPaths
	exp.ToolUsage = req.ToolUsage
	exp.Scene = req.Scene
	exp.LogSamples = req.LogSamples
	exp.CodeSnippets = req.CodeSnippets
	if err := s.expDB.Update(exp); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, exp)
}

func (s *APIServer) handleExpDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.expDB.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

// Stats

func (s *APIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	type Stats struct {
		TotalTasks       int `json:"total_tasks"`
		PendingTasks     int `json:"pending_tasks"`
		InProgressTasks  int `json:"in_progress_tasks"`
		ArchivedTasks int `json:"archived_tasks"`
		ExceptionTasks   int `json:"exception_tasks"`
		TotalExp         int `json:"total_exp"`
	}
	all, _ := s.db.List(backend.TaskFilter{Limit: 10000, Offset: 0})
	st := Stats{TotalTasks: len(all)}
	for _, t := range all {
		switch t.Status {
		case backend.TaskStatusPending: st.PendingTasks++
		case backend.TaskStatusInProgress: st.InProgressTasks++
		case backend.TaskStatusArchived: st.ArchivedTasks++
		case backend.TaskStatusException: st.ExceptionTasks++
		}
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
		if id == "" { continue }
		exp, err := s.expDB.Get(id)
		if err != nil {
			slog.Warn("loadExperiencesForTask: missing experience",
				slog.String("task_id", t.ID),
				slog.String("experience_id", id),
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
		CommandType string `json:"command_type"`
		Model       string `json:"model"`
		Prompt      string `json:"prompt"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // body 可选
	if req.CommandType == "" {
		req.CommandType = "claude"
	}
	// 构造 rich prompt: task 全字段 + 多经验内容注入
	var prompt string
	if req.Prompt != "" {
		// 显式传了 prompt 就用显式的(保留原有行为)
		prompt = req.Prompt
	} else {
		// 没用 body.prompt,自动从 task + 多 experience 组装
		prompt = taskpkg.BuildTaskPrompt(task, s.loadExperiencesForTask(task)...)
		if prompt == "" {
			slog.Warn("task run rejected: empty prompt after BuildTaskPrompt",
				slog.String("task_id", id),
				slog.String("command_type", req.CommandType),
			)
			writeErr(w, http.StatusBadRequest, "task has no description and no experience content")
			return
		}
	}
	req.Prompt = prompt

	cmd, cleanup, err := runner.BuildCommand(req.CommandType, req.Model, "", req.Prompt, runner.WithActionReport())
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
		Command:   runner.CmdString(cmd),
		Model:     req.Model,
		StartedAt: time.Now(),
	}
	if err := s.execDB.Create(exec); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.db.UpdateStatus(id, backend.TaskStatusInProgress, "factory-agent")

	slog.Info("task run started",
		slog.String("task_id", id),
		slog.String("execution_id", exec.ID),
		slog.String("command_type", req.CommandType),
		slog.String("model", req.Model),
		slog.Int("prompt_chars", len(req.Prompt)),
		slog.String("cmd", exec.Command),
	)

	// 异步跑
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
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
		res, runErr := executor.Run(ctx, cmd, "", func(chunk string) {
			s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
				"execution_id": exec.ID,
				"task_id":      id,
				"chunk":        chunk,
			})
		})
		status := backend.TaskStatusArchived
		if res != nil && res.ExitCode != 0 {
			status = backend.TaskStatusException
		}
		out, errOut := "", ""
		exitCode := -1
		if res != nil {
			out, errOut = res.Output, res.ErrorOut
			exitCode = res.ExitCode
		}
		_ = s.execDB.Finish(exec.ID, out, errOut, exitCode)
		_ = s.db.UpdateStatus(id, status, "factory-agent")
		s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
			"execution_id": exec.ID,
			"task_id":      id,
			"done":         true,
			"exit_code":    exitCode,
		})
		lvl := slog.LevelInfo
		if exitCode != 0 || runErr != nil {
			lvl = slog.LevelError
		}
		slog.LogAttrs(context.Background(), lvl, "task run finished",
			slog.String("task_id", id),
			slog.String("execution_id", exec.ID),
			slog.Int("exit_code", exitCode),
			slog.String("status", status),
			slog.Int64("dur_ms", time.Since(started).Milliseconds()),
			slog.String("err", errStr(runErr)),
		)
	}()

	writeJSON(w, map[string]any{
		"execution_id": exec.ID,
		"task_id":      id,
		"command":      exec.Command,
		"status":       "started",
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
		writeErr(w, http.StatusNotFound, "no running execution for task")
		return
	}
	cancel()
	writeJSON(w, map[string]any{"task_id": id, "cancelled": true})
}

// handleTaskDelete 硬删 task + 关联 executions + evaluations（不可恢复）。
func (s *APIServer) handleTaskDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	list, err := s.execDB.ListRecent(limit)
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

// handleExecutionEvaluate 异步调 claude 给 execution 打分。
func (s *APIServer) handleExecutionEvaluate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	exec, err := s.execDB.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct {
		CliType string `json:"cli_type"`
		Model   string `json:"model"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.CliType == "" {
		req.CliType = "claude"
	}
	if req.Model == "" {
		req.Model = "sonnet"
	}
	// 找 task prompt：用 BuildTaskPrompt 注入完整 task + 多 experience 信息
	prompt := exec.Command
	if exec.TaskID != "" {
		if t, err := s.db.Get(exec.TaskID); err == nil {
			prompt = taskpkg.BuildTaskPrompt(t, s.loadExperiencesForTask(t)...)
		}
	}
	// 异步执行（避免 HTTP 阻塞 30s+）
	go func() {
		slog.Info("evaluator: dispatched",
			slog.String("execution_id", id),
			slog.String("cli", req.CliType),
			slog.String("model", req.Model),
		)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		_, err := evaluator.RunAndSave(ctx, s.evalDB, s.execDB, exec, prompt, req.CliType, req.Model)
		if err != nil {
			log.Printf("evaluator: %v", err)
		}
	}()
	writeJSON(w, map[string]string{"execution_id": id, "status": "evaluating", "cli_type": req.CliType, "model": req.Model})
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
		log.Printf("ws upgrade: %v", err)
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
	if err := s.linkDB.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"id": id, "status": "deleted"})
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
		Name      string `json:"name"`
		Path      string `json:"path"`
		SortOrder int    `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.Path == "" {
		writeErr(w, http.StatusBadRequest, "name and path are required")
		return
	}
	if req.SortOrder == 0 {
		req.SortOrder = s.dirDB.NextSortOrder()
	}
	d := &backend.DirShortcut{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Path:      req.Path,
		SortOrder: req.SortOrder,
		CreatedAt: time.Now(),
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
		Name      string `json:"name"`
		Path      string `json:"path"`
		SortOrder int    `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	d := &backend.DirShortcut{ID: id, Name: req.Name, Path: req.Path, SortOrder: req.SortOrder}
	if err := s.dirDB.Update(d); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, d)
}

func (s *APIServer) handleDirShortcutDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.dirDB.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"id": id, "status": "deleted"})
}

func (s *APIServer) handleDirShortcutOpen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// 取 path
	list, err := s.dirDB.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var path string
	for _, d := range list {
		if d.ID == id {
			path = d.Path
			break
		}
	}
	if path == "" {
		writeErr(w, http.StatusNotFound, "shortcut not found")
		return
	}
	if err := shortcuts.OpenDir(path); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.dirDB.Touch(id)
	writeJSON(w, map[string]string{"id": id, "status": "opened", "path": path})
}

// handleDirShortcutOpenTerminal 打开指定终端类型的工作目录
func (s *APIServer) handleDirShortcutOpenTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	termType := r.URL.Query().Get("type") // 可选，默认用 config.json 的 default_type
	if termType == "" {
		termType = shortcuts.DefaultTerminal()
	}
	slog.Info("[handleDirShortcutOpenTerminal]",
		slog.String("id", id),
		slog.String("termType", termType),
		slog.String("at", "main.go:928"))

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
	typeDef := config.AppConfig.Terminal.Types[strings.ToLower(termType)]
	slog.Info("[handleDirShortcutOpenTerminal] opening",
		slog.String("dir", path),
		slog.String("termType", termType),
		slog.String("binPath", typeDef.Path),
		slog.String("bin", typeDef.Bin),
		slog.String("at", "main.go:964"))
		binPath := typeDef.Path
	var openErr error
	if strings.HasPrefix(path, "ssh://") || strings.Contains(path, "@:") {
		openErr = shortcuts.OpenRemoteTerminal(termType, path)
	} else {
		// 本地路径先检查目录是否存在
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
	supported := []map[string]string{
		{"type": "wezterm", "name": "WezTerm", "platform": "macOS/Linux/Windows"},
		{"type": "wt", "name": "Windows Terminal", "platform": "Windows"},
		{"type": "powershell", "name": "PowerShell", "platform": "Windows"},
		{"type": "pwsh", "name": "PowerShell Core", "platform": "Windows/macOS/Linux"},
		{"type": "terminal", "name": "Terminal.app", "platform": "macOS"},
		{"type": "gnome", "name": "GNOME Terminal", "platform": "Linux"},
		{"type": "xterm", "name": "xterm", "platform": "Linux"},
		{"type": "cmd", "name": "CMD", "platform": "Windows"},
	}
	defaultType, _ := s.setDB.Get("default_terminal")
	if defaultType == "" {
		defaultType = string(shortcuts.DefaultTerminal())
	}
	writeJSON(w, map[string]interface{}{
		"supported": supported,
		"default":   defaultType,
	})
}

// handleGetDefaultTerminal 读取默认终端类型
func (s *APIServer) handleGetDefaultTerminal(w http.ResponseWriter, r *http.Request) {
	val, _ := s.setDB.Get("default_terminal")
	if val == "" {
		val = string(shortcuts.DefaultTerminal())
	}
	writeJSON(w, map[string]string{"value": val})
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

// handleSetDefaultTerminal 设置默认终端类型
func (s *APIServer) handleSetDefaultTerminal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !shortcuts.IsSupportedTerminal(req.Value) {
		writeErr(w, http.StatusBadRequest, "unsupported terminal type: "+req.Value)
		return
	}
	if err := s.setDB.Set("default_terminal", req.Value); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"value": req.Value})
}
// handleModelList 返回模型列表（从 config.json 加载）
func (s *APIServer) handleModelList(w http.ResponseWriter, r *http.Request) {
	cfg := config.AppConfig
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	writeJSON(w, map[string]interface{}{
		"cli_type_models": cfg.Models,
	})
}

// handleGetConfig 返回当前配置（从 config.json 读取）
func (s *APIServer) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.AppConfig
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	resp := map[string]interface{}{
		"terminal": map[string]interface{}{
			"default_type":  cfg.Terminal.DefaultType,
			"detect_paths":  cfg.Terminal.DetectPaths,
			"types":         cfg.Terminal.Types,
		},
		"models": cfg.Models,
	}
	writeJSON(w, resp)
}

// handleSetConfig 保存用户配置（回写 config.json）
func (s *APIServer) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TerminalType string `json:"terminal_type"`
		TerminalPath string `json:"terminal_path"`
		ModelDefaults map[string]string `json:"model_defaults"` // cli_type -> default model
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := config.AppConfig
	changed := false
	if req.TerminalType != "" && cfg != nil {
		cfg.Terminal.DefaultType = req.TerminalType
		changed = true
	}
	if req.TerminalPath != "" && cfg != nil && req.TerminalType != "" {
		if typeDef, ok := cfg.Terminal.Types[req.TerminalType]; ok {
			typeDef.Path = req.TerminalPath
			cfg.Terminal.Types[req.TerminalType] = typeDef
			changed = true
		}
	}
	if req.ModelDefaults != nil && cfg != nil {
		for cliType, model := range req.ModelDefaults {
			if group, ok := cfg.Models[cliType]; ok {
				group.Default = model
				cfg.Models[cliType] = group
				changed = true
			}
		}
	}
	if changed {
		config.Save()
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
	slog.Info("handler: scheduled task created", "id", t.ID, "name", t.Name, "cron", t.CronExpr)
	_ = s.sch.Reload() // 热加载
	writeJSON(w, t)
}

func (s *APIServer) handleScheduledUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	if err := s.schedDB.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("handler: scheduled task deleted", "id", id)
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
	if _, err := s.sch.RunNow(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	slog.Info("handler: scheduled task run-now triggered", "id", id)
	writeJSON(w, map[string]string{"id": id, "status": "triggered"})
}

// --- Scheduler ---

func (s *APIServer) handleSchedulerStart(w http.ResponseWriter, r *http.Request) {
	if err := s.sch.Start(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("handler: scheduler started")
	writeJSON(w, map[string]string{"status": "running"})
}

func (s *APIServer) handleSchedulerStop(w http.ResponseWriter, r *http.Request) {
	s.sch.Stop()
	slog.Info("handler: scheduler stopped")
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

// --- Todo.md ---

func (s *APIServer) handleTodo(w http.ResponseWriter, r *http.Request) {
	path, _ := s.setDB.Get("todo_md_path")
	if path == "" {
		writeJSON(w, map[string]any{"path": "", "items": []todo.Item{}})
		return
	}
	items, err := todo.ReadAndParse(path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []todo.Item{}
	}
	writeJSON(w, map[string]any{"path": path, "items": items})
}

func (s *APIServer) handleTodoToggle(w http.ResponseWriter, r *http.Request) {
	path, _ := s.setDB.Get("todo_md_path")
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
	var found bool
	for i := range items {
		if items[i].LineNo == lineNo {
			items[i].Done = req.Done
			found = true
			break
		}
	}
	if !found {
		writeErr(w, http.StatusNotFound, "line not found")
		return
	}
	if err := todo.ToggleAndWrite(path, items); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"line_no": lineNo, "done": req.Done})
}

func (s *APIServer) handleTodoAdd(w http.ResponseWriter, r *http.Request) {
	path, _ := s.setDB.Get("todo_md_path")
	if path == "" {
		writeErr(w, http.StatusBadRequest, "todo_md_path not set")
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Text == "" {
		writeErr(w, http.StatusBadRequest, "text is required")
		return
	}
	if err := todo.AddAndWrite(path, req.Text); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"text": req.Text, "status": "added"})
}

func (s *APIServer) handleTodoDelete(w http.ResponseWriter, r *http.Request) {
	path, _ := s.setDB.Get("todo_md_path")
	if path == "" {
		writeErr(w, http.StatusBadRequest, "todo_md_path not set")
		return
	}
	lineNo := parseInt(r.PathValue("line_no"), 0)
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

func (s *APIServer) handleTodoPath(w http.ResponseWriter, r *http.Request) {
	path, _ := s.setDB.Get("todo_md_path")
	writeJSON(w, map[string]string{"path": path})
}

func (s *APIServer) handleTodoPathSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.setDB.Set("todo_md_path", req.Path); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"path": req.Path})
}

// --- Settings ---

func (s *APIServer) handleSettingsList(w http.ResponseWriter, r *http.Request) {
	all, err := s.setDB.All()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, all)
}

func (s *APIServer) handleSettingsSet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.setDB.Set(key, req.Value); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"key": key, "value": req.Value})
}

// Helpers

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	slog.LogAttrs(nil,
		func() slog.Level {
			if code >= 500 { return slog.LevelError }
			if code >= 400 { return slog.LevelWarn }
			return slog.LevelInfo
		}(),
		"http error",
		slog.Int("status", code),
		slog.String("msg", msg),
	)
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

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func main() {
	dbPath := paths.ResolveDBPath()
	if cwd, err := os.Getwd(); err == nil {
		slog.Info("db path", slog.String("path", dbPath), slog.String("cwd", cwd))
	} else {
		slog.Info("db path", slog.String("path", dbPath))
	}

	db, err := backend.OpenDB(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := backend.InitSchema(db); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Printf("[config] load failed: %v, using defaults", err)
		cfg = config.DefaultConfig()
	}
	config.AppConfig = cfg

		// 支持 -config 指定配置文件路径
		cfgPath := flag.String("config", "", "path to config.json")
		flag.Parse()
		if *cfgPath != "" {
			if err := config.LoadFromPath(*cfgPath); err != nil {
				log.Printf("[config] load from %s failed: %v", *cfgPath, err)
			} else {
				slog.Info("config loaded", slog.String("path", *cfgPath))
			}
		}

	taskRepo := backend.NewTaskRepo(db)
	expRepo := backend.NewExperienceRepo(db)
	execRepo := backend.NewExecutionRepo(db)
	linkRepo := backend.NewWebLinkRepo(db)
	dirRepo := backend.NewDirShortcutRepo(db)
	schedRepo := backend.NewScheduledTaskRepo(db)
	settingsRepo := backend.NewAppSettingsRepo(db)
	evalRepo := backend.NewEvaluationRepo(db)
	h := hub.New()
	sch := scheduler.New(schedRepo, execRepo, h).WithSettings(settingsRepo)
	if err := sch.AutoStart(); err != nil {
		log.Printf("[scheduler] auto start failed: %v", err)
	}

	// init relay repo
	relayRepo := relay.NewSQLiteRelayRepo(db)
	if err := relayRepo.InitSchema(); err != nil {
		log.Fatalf("init relay schema: %v", err)
	}

	srv := NewAPIServer(taskRepo, expRepo, execRepo,
		linkRepo, dirRepo, schedRepo, settingsRepo, evalRepo, sch, h, relayRepo)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8901"
	}
	// SO_REUSEADDR：服务重启时避免 "address already in use" 等 TIME_WAIT
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Skill Factory started at http://localhost%s", addr)
	if err := (&http.Server{Handler: srv}).Serve(ln); err != nil {
		log.Fatal(err)
	}
}
