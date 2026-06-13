// Package scheduler 包装 robfig/cron，按 scheduled_tasks 表触发执行。
package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/executor"
	"github.com/xiaodongQ/xworkbench/internal/executor/runner"
	"github.com/xiaodongQ/xworkbench/internal/hub"
	"github.com/xiaodongQ/xworkbench/internal/wsmsg"
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
		return err
	}
	s.cron.Start()
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
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
			log.Printf("[scheduler] parse %s (%q): %v", t.Name, t.CronExpr, err)
			continue
		}
		log.Printf("[scheduler] loaded %s cron=%q next=%v id=%d", t.Name, t.CronExpr, s.cron.Entry(id).Next, id)
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
	cmd, cleanup, err := runner.BuildCommand(t.CommandType, t.Model, "", t.Prompt, runner.WithActionReport())
	if err != nil {
		log.Printf("[scheduler] build cmd for %s: %v", t.Name, err)
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
		Command:   runner.CmdString(cmd),
		Model:     t.Model,
		StartedAt: time.Now(),
	}
	if err := s.execDB.Create(exec); err != nil {
		log.Printf("[scheduler] create execution: %v", err)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	res, _ := executor.Run(ctx, cmd, t.WorkingDir, func(chunk string) {
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
	_ = s.execDB.Finish(exec.ID, out, errOut, exitCode)
	_ = s.repo.UpdateAfterRun(t.ID, status, exec.ID)
	s.hub.Broadcast(wsmsg.ChannelScheduled, map[string]any{
		"scheduled_task_id": t.ID,
		"execution_id":      exec.ID,
		"event":             "done",
		"status":            status,
		"exit_code":         exitCode,
	})
}
