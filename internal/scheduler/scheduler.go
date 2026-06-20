// Package scheduler 包装 robfig/cron，按 scheduled_tasks 表触发执行。
package scheduler

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/executor"
	"github.com/xiaodongQ/xworkbench/internal/executor/runner"
	"github.com/xiaodongQ/xworkbench/internal/hub"
	"github.com/xiaodongQ/xworkbench/internal/wsmsg"
	"github.com/xiaodongQ/xworkbench/internal/logger"
)



const schedulerEnabledKey = "scheduler.enabled"

// Scheduler 进程内 cron 引擎，加载 DB 中 enabled=1 的 scheduled_tasks。
type Scheduler struct {
	mu      sync.Mutex
	cron    *cron.Cron
	repo    *backend.ScheduledTaskRepo
	execDB  *backend.ExecutionRepo
	hub     *hub.Hub
	settings *backend.AppSettingsRepo
	running bool
}

func New(repo *backend.ScheduledTaskRepo, execDB *backend.ExecutionRepo, h *hub.Hub) *Scheduler {
	loc, _ := time.LoadLocation("Local")
	return &Scheduler{
		cron:   cron.New(cron.WithLocation(loc)),
		repo:   repo,
		execDB: execDB,
		hub:    h,
	}
}

// WithSettings 设置持久化 repo（用于自动启动/状态保存）。
func (s *Scheduler) WithSettings(settings *backend.AppSettingsRepo) *Scheduler {
	s.settings = settings
	return s
}

// AutoStart 根据保存的状态自动启动调度器。
func (s *Scheduler) AutoStart() error {
	if s.settings == nil {
		return nil
	}
	val, err := s.settings.Get(schedulerEnabledKey)
	if err != nil || val != "true" {
		return nil
	}
	return s.Start()
}

// Start 加载 enabled=1 的所有任务并启动 cron。
func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	if err := s.Reload(); err != nil {
		logger.Logger.Errorw("scheduler: reload failed on start", "err", err)
		return err
	}
	s.cron.Start()
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
	logger.Logger.Info("scheduler: started")
	// 持久化状态
	if s.settings != nil {
		_ = s.settings.Set(schedulerEnabledKey, "true")
	}
	s.hub.Broadcast(wsmsg.ChannelScheduler, map[string]any{"status": "running"})
	return nil
}

// Stop 停止 cron（不删除已加载的任务）。
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.running = false
	logger.Logger.Info("scheduler: stopped")
	// 持久化状态
	if s.settings != nil {
		_ = s.settings.Set(schedulerEnabledKey, "false")
	}
	s.hub.Broadcast(wsmsg.ChannelScheduler, map[string]any{"status": "stopped"})
}

// Reload 重新从 DB 加载任务（add new, remove old）。
func (s *Scheduler) Reload() error {
	tasks, err := s.repo.ListEnabled()
	if err != nil {
		logger.Logger.Errorw("scheduler: list enabled tasks failed", "err", err)
		return err
	}
	// robfig/cron 没有 RemoveAll 公开方法；重建
	s.mu.Lock()
	oldCtx := s.cron.Stop()
	<-oldCtx.Done()
	loc, _ := time.LoadLocation("Local")
	s.cron = cron.New(cron.WithLocation(loc))
	for _, t := range tasks {
		id, err := s.cron.AddFunc(t.CronExpr, s.makeHandler(t))
		if err != nil {
			logger.Logger.Warnw("scheduler: parse cron expr failed", "task", t.Name, "cron", t.CronExpr, "err", err)
			continue
		}
		logger.Logger.Infow("scheduler: task loaded", "task", t.Name, "cron", t.CronExpr, "next", s.cron.Entry(id).Next)
	}
	wasRunning := s.running
	if wasRunning {
		s.cron.Start()
	}
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// RunNow 立即执行某个 scheduled_task（不依赖 cron）。
func (s *Scheduler) RunNow(id string) (string, error) {
	t, err := s.repo.Get(id)
	if err != nil {
		return "", err
	}
	go s.execute(t)
	return t.ID, nil
}

func (s *Scheduler) makeHandler(t *backend.ScheduledTask) func() {
	return func() {
		s.execute(t)
	}
}

func (s *Scheduler) execute(t *backend.ScheduledTask) {
	cmd, stdin, cleanup, err := runner.BuildCommand(t.CommandType, t.Model, "", t.Prompt,
		runner.WithStdin(),
		runner.WithActionReport(),
		runner.WithAllowedTools("Bash", "Write", "Edit", "Read"),
	)
	if err != nil {
		logger.Logger.Errorw("scheduler: build command failed", "task", t.Name, "err", err)
		_ = s.repo.UpdateAfterRun(t.ID, "build_error", "")
		return
	}
	if cleanup != nil {
		defer cleanup()
	}
	exec := &backend.Execution{
		ID:        uuid.New().String(),
		ScheduledTaskID: t.ID,
		Source:    "scheduled",
		Command:   runner.CmdStringWithPrompt(cmd, t.Prompt),
		Prompt:    t.Prompt,
		Model:     t.Model,
		StartedAt: time.Now(),
	}
	if err := s.execDB.Create(exec); err != nil {
		logger.Logger.Errorw("scheduler: create execution record failed", "task", t.Name, "exec_id", exec.ID, "err", err)
		return
	}
	s.hub.Broadcast(wsmsg.ChannelScheduled, map[string]any{
		"scheduled_task_id": t.ID,
		"execution_id":      exec.ID,
		"event":             "started",
	})

	// 计算超时：任务配置 > 默认（AI任务1小时，shell任务5分钟）
	timeout := t.TimeoutSec
	if timeout <= 0 {
		if t.CommandType == "shell" {
			timeout = 5 * 60 // 5分钟
		} else {
			timeout = 60 * 60 // 1小时
		}
	}
	logger.Logger.Infow("scheduler: task execution started", "task", t.Name, "exec_id", exec.ID, "timeout_sec", timeout)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	res, _ := executor.Run(ctx, cmd, t.WorkingDir, stdin, func(chunk string) {
		s.hub.Broadcast(wsmsg.ChannelScheduled, map[string]any{
			"scheduled_task_id": t.ID,
			"execution_id":      exec.ID,
			"chunk":             chunk,
		})
	})

	out, errOut := "", ""
	exitCode := -1
	status := "failed"
	if res != nil {
		out, errOut = res.Output, res.ErrorOut
		exitCode = res.ExitCode
		if exitCode == 0 {
			status = "success"
		} else if res.Err != nil {
			status = "timeout"
		}
	}
	// 解析 claude/cbc -p --output-format json 输出中的 session_id/sessionId（用于 --resume 继续对话）
	resumeSessionID := extractSessionID(out)
	_ = s.execDB.Finish(exec.ID, out, errOut, exitCode, resumeSessionID)
	_ = s.repo.UpdateAfterRun(t.ID, status, exec.ID)
	logger.Logger.Infow("scheduler: task finished", "task", t.Name, "exec_id", exec.ID, "session_id", resumeSessionID, "status", status, "exit_code", exitCode)
	s.hub.Broadcast(wsmsg.ChannelScheduled, map[string]any{
		"scheduled_task_id": t.ID,
		"execution_id":      exec.ID,
		"event":             "done",
		"status":            status,
		"exit_code":         exitCode,
	})
}

// extractSessionID 从 claude/cbc -p --output-format json 输出中解析 session_id/sessionId。
// 优先匹配 session_id（claude），未找到则匹配 sessionId（codebuddy）。
func extractSessionID(output string) string {
	if output == "" {
		return ""
	}
	// 路径 1：JSON 解析
	var anyObj interface{}
	if err := json.Unmarshal([]byte(output), &anyObj); err == nil {
		switch v := anyObj.(type) {
		case []any:
			// 优先取最后一个含 session_id 或 sessionId 的
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
			// 退化：取第一个
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
	// 路径 2：字符串匹配回退
	idx := strings.Index(output, `"session_id"`)
	if idx >= 0 {
		rest := output[idx+12:]
		idx2 := strings.Index(rest, `"`)
		if idx2 >= 0 {
			rest = rest[idx2+1:]
			end := strings.Index(rest, `"`)
			if end >= 0 {
				return rest[:end]
			}
		}
	}
	idx = strings.Index(output, `"sessionId"`)
	if idx >= 0 {
		rest := output[idx+11:]
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
