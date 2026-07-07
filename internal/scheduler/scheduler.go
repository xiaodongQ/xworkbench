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
	"github.com/xiaodongQ/xworkbench/internal/config"
	"github.com/xiaodongQ/xworkbench/internal/executor"
	"github.com/xiaodongQ/xworkbench/internal/executor/runner"
	"github.com/xiaodongQ/xworkbench/internal/hub"
	"github.com/xiaodongQ/xworkbench/internal/wsmsg"
	"github.com/xiaodongQ/xworkbench/internal/logger"
	"golang.org/x/sync/singleflight"
)

// Scheduler 进程内 cron 引擎，加载 DB 中 enabled=1 的 scheduled_tasks。
// scheduler.enabled 状态存 config.json（顶层 scheduler_enabled 字段）。
type Scheduler struct {
	mu      sync.Mutex
	cron    *cron.Cron
	repo    *backend.ScheduledTaskRepo
	execDB  *backend.ExecutionRepo
	hub     *hub.Hub
	running bool

	// entries task ID → cron entry ID,维护 task 与 cron engine 内部 entry 的映射。
	// NextRunAt 通过这个 map 查 entry,触发前 Next 稳定(handler 之前自己
	// parser.Parse+Next(time.Now()) 会随 now 漂移)。
	entries map[string]cron.EntryID

	// nextRun task ID → 该 task 的下一次触发时间。在 Reload 时主动调用
	// Schedule.Next(time.Now()) 计算一次,生产环境 Start() 后 c.run() 会持续
	// 更新 Entry.Next;测试场景不调 Start,因此独立维护此 map 让 NextRunAt
	// 在两种场景下行为一致。nextRun 与 entries 一一对应。
	nextRun map[string]time.Time

	// sf 用于合并同 task 的重叠执行：cron 周期 < 子进程时长时,或用户点"立即运行"
	// 与 cron 触发重叠时,只跑一次,避免两个 goroutine 同时写 executions 表触发
	// SQLITE_BUSY 竞争。详见 doExecute 的 singleflight 包装。
	sf singleflight.Group
}

func New(repo *backend.ScheduledTaskRepo, execDB *backend.ExecutionRepo, h *hub.Hub) *Scheduler {
	loc, _ := time.LoadLocation("Local")
	return &Scheduler{
		cron:    cron.New(cron.WithLocation(loc)),
		repo:    repo,
		execDB:  execDB,
		hub:     h,
		entries: make(map[string]cron.EntryID),
		nextRun: make(map[string]time.Time),
	}
}

// AutoStart 根据 config.json 的 scheduler_enabled 自动启动调度器。
func (s *Scheduler) AutoStart() error {
	cfg := config.Get()
	if cfg == nil || !cfg.SchedulerEnabled {
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
	// 持久化状态到 config.json（线程安全 + Save 失败容错）
	if _, err := config.SetAndSave(func(c *config.Config) {
		c.SchedulerEnabled = true
	}); err != nil {
		logger.Logger.Warnw("scheduler: persist enabled failed", "err", err)
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
	// 持久化状态到 config.json（线程安全 + Save 失败容错）
	if _, err := config.SetAndSave(func(c *config.Config) {
		c.SchedulerEnabled = false
	}); err != nil {
		logger.Logger.Warnw("scheduler: persist disabled failed", "err", err)
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
	// 只在 cron 运行时停掉旧 ctx(没运行时 Stop 返回 nil ctx 会死锁)
	if s.running {
		oldCtx := s.cron.Stop()
		<-oldCtx.Done()
	}
	loc, _ := time.LoadLocation("Local")
	s.cron = cron.New(cron.WithLocation(loc))
	s.entries = make(map[string]cron.EntryID)
	s.nextRun = make(map[string]time.Time)
	now := time.Now()
	for _, t := range tasks {
		id, err := s.cron.AddFunc(t.CronExpr, s.makeHandler(t))
		if err != nil {
			logger.Logger.Warnw("scheduler: parse cron expr failed", "task", t.Name, "cron", t.CronExpr, "err", err)
			continue
		}
		s.entries[t.ID] = id
		// 主动计算 Next:cron engine 的 Entry.Next 只在 Start() 后由 c.run() 填充,
		// 不调 Start 时保持零值。我们独立维护 nextRun,NextRunAt 直接查它,
		// 行为对调用方一致——Reload 后稳定,handler 多次调用不会漂移。
		// 生产环境 Start() 后 cron 会持续刷新 Entry.Next,但我们读 nextRun,
		// 这是 Reload 时刻的快照(handler 用于展示"下次运行"已经够用)。
		nxt := s.cron.Entry(id).Schedule.Next(now)
		s.nextRun[t.ID] = nxt
		logger.Logger.Infow("scheduler: task loaded", "task", t.Name, "cron", t.CronExpr, "next", nxt)
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

// NextRunAt 返回 task 在 cron engine 里的下次触发时间。返回 (zero, false) 表示:
// task 未在 cron 中(未 enabled / 解析失败 / scheduler 未加载)。
//
// 这就是 scheduler 真正会触发的时间,触发前稳定(handler 不应再自己
// parser.Parse+Next(time.Now())——time.Now() 漂移会导致 UI 一直刷新)。
// 该值在 Reload 时计算并缓存,handler 多次调用返回相同结果,直到下一次 Reload。
func (s *Scheduler) NextRunAt(taskID string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nxt, ok := s.nextRun[taskID]
	if !ok {
		return time.Time{}, false
	}
	return nxt, true
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

// execute 是 singleflight 包装入口。同 task 的并发触发(cron + RunNow,或 cron
// 周期 < 子进程时长)合并为一次实际执行,避免重复写 executions 表。
func (s *Scheduler) execute(t *backend.ScheduledTask) {
	_, _, _ = s.sf.Do(t.ID, func() (any, error) {
		s.doExecute(t)
		return nil, nil
	})
}

func (s *Scheduler) doExecute(t *backend.ScheduledTask) {
	// 是否完全放开 CLI 权限（根据配置开关注定，默认关闭）
	skip := config.AppConfig != nil && config.AppConfig.DangerouslySkipPermissions

	// 是否为 AI 任务（claude/cbc），只有 AI 任务才支持 resume session
	isAI := t.CommandType == "claude" || t.CommandType == "cbc"

	// 获取 MaxResumeCount 配置
	maxResume := 20
	if cfg := config.Snapshot(); cfg != nil && cfg.Scheduler.MaxResumeCount > 0 {
		maxResume = cfg.Scheduler.MaxResumeCount
	}

	// 决定是否使用 resume session
	// - AI 任务：ResumeCount < MaxResumeCount 时使用 resume
	// - 非 AI 任务（shell）：不使用 resume
	var sessionID string
	if isAI && t.LastSessionID != "" && t.ResumeCount < maxResume {
		sessionID = t.LastSessionID
	}

	var (
		cmd     []string
		stdin   string
		cleanup func()
		err     error
	)

	// 根据权限和 session 配置构建命令
	if skip {
		cmd, stdin, cleanup, err = runner.BuildCommand(t.CommandType, t.Model, sessionID, t.Prompt,
			runner.WithStdin(),
			runner.WithActionReport(),
			runner.WithSkipPermissions(),
		)
	} else {
		cmd, stdin, cleanup, err = runner.BuildCommand(t.CommandType, t.Model, sessionID, t.Prompt,
			runner.WithStdin(),
			runner.WithActionReport(),
			runner.WithAllowedTools("Bash", "Write", "Edit", "Read", "Grep"),
		)
	}
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
		CliType:   t.CommandType, // scheduled task 的 CommandType = claude/cbc/shell
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

	// 计算超时：任务配置 > 默认（AI 任务 10 分钟，shell 任务 5 分钟）
	// AI 默认从 1 小时改成 10 分钟，与 handleTaskRun / handleExecutionContinue 对齐；
	// shell 保持 5 分钟不变（命令通常秒级完成，5 分钟足够）
	timeout := t.TimeoutSec
	if timeout <= 0 {
		if t.CommandType == "shell" {
			timeout = 5 * 60 // 5 分钟
		} else {
			timeout = 10 * 60 // 10 分钟
		}
	}
	logger.Logger.Infow("scheduler: task execution started", "task", t.Name, "exec_id", exec.ID, "timeout_sec", timeout)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	res, _ := executor.Run(ctx, cmd, t.WorkingDir, stdin, func(chunk string) {
		// chunk 推送走 exec 频道（handleExecStream 在前端已存在），
		// 与 started/done 频道分离，避免每次 chunk 触发前端 loadScheduled() 重算 next_run_at 漂移。
		s.hub.Broadcast(wsmsg.ChannelExec, map[string]any{
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

	// 更新任务的 session 信息：成功后更新 session_id 和 resume_count
	// 失败时也继续（不重置 resume_count），让下次执行还有机会继续
	newResumeCount := t.ResumeCount
	if resumeSessionID != "" {
		newResumeCount = t.ResumeCount + 1
		_ = s.repo.UpdateSessionInfo(t.ID, resumeSessionID, newResumeCount)
	}

	_ = s.repo.UpdateAfterRun(t.ID, status, exec.ID)
	// 刷新 nextRun map:trigger 后 cron library 的 Schedule.Next 已推进到下一次触发时间,
	// 写回 s.nextRun 让 NextRunAt 返回新值(否则前端 loadScheduled 拿到的 next_run_at
	// 永远是 Reload 时算的初始值,跟 last_run 严重对不上)
	if entryID, ok := s.entries[t.ID]; ok {
		if entry := s.cron.Entry(entryID); entry.ID != 0 {
			s.mu.Lock()
			s.nextRun[t.ID] = entry.Schedule.Next(time.Now())
			s.mu.Unlock()
		}
	}
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
