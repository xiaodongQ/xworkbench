package backend

import (
	"time"
)

// TaskStatus values
const (
	TaskStatusPending       = "pending"
	TaskStatusInProgress    = "in_progress" // 已认领，待执行
	TaskStatusRunning       = "running"     // 执行中
	TaskStatusArchived      = "archived"
	TaskStatusException     = "exception"
	TaskStatusWaitingInput  = "waiting_input"
)

// TaskType values
const (
	TaskTypeManual     = "manual"
	TaskTypeScheduled  = "scheduled"
	TaskTypeRemote     = "remote"
)

type Task struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	Status       string     `json:"status"`
	Priority     int        `json:"priority,omitempty"`
	ExperienceID string     `json:"experience_id,omitempty"`
	// 多经验关联：通过 task_experiences 表加载，列表/详情接口填充。空切片 == nil != 没设置
	ExperienceIDs []string `json:"experience_ids,omitempty"`
	Resources    string     `json:"resources,omitempty"`
	Acceptance   string     `json:"acceptance,omitempty"`
	Version      string     `json:"version"`
	CreatedAt    time.Time  `json:"created_at"`
	ClaimedAt    *time.Time `json:"claimed_at,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Maintainer   string     `json:"maintainer,omitempty"`
	RepoAddress  string     `json:"repo_address,omitempty"`
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
	Result       string     `json:"result,omitempty"`
	// 调度相关
	ExecutorModel       string  `json:"executor_model,omitempty"`
	CbcModel            string  `json:"cbc_model,omitempty"`
	IterationCount      int     `json:"iteration_count,omitempty"`
	MaxIterations       int     `json:"max_iterations,omitempty"`
	ImprovementThreshold float64 `json:"improvement_threshold,omitempty"`
	LastHeartbeat       *time.Time `json:"last_heartbeat,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	// 执行配置（手动任务创建时确定）
	CommandType   string `json:"command_type,omitempty"` // claude/shell/cbc
	Model         string `json:"model,omitempty"`       // haiku/sonnet/opus
	Prompt        string `json:"prompt,omitempty"`       // 执行用 prompt
	GoalMode      bool   `json:"goal_mode,omitempty"`    // 是否启用 Goal 目标模式（/goal 前缀）
	// 远程 Agent 相关
	TaskType          string   `json:"task_type,omitempty"`
	AssignedAgentID   string   `json:"assigned_agent_id,omitempty"`  // 创建时指定的目标 agent（task_type=remote）
	ClaimerAgentID    string   `json:"claimer_agent_id,omitempty"`
	ResultOutput     string   `json:"result_output,omitempty"`
	EvaluationScore *float64 `json:"evaluation_score,omitempty"`
	WaitingInput     string   `json:"waiting_input,omitempty"`   // 待交互的提示内容
	ExecutionID      string   `json:"execution_id,omitempty"`    // 当前执行的 execution id
}

type Experience struct {
	ID        string    `json:"id"`
	Module    string    `json:"module"`    // 分类，如 redis, docker, git（必填）
	Keywords  string    `json:"keywords"`  // 关键词，逗号分隔，用于匹配问题
	Scene     string    `json:"scene"`     // 适用场景简述
	Details   string    `json:"details"`   // 详细内容，Markdown 格式（命令/日志/代码/步骤等）
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SkillVersion struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Version   string    `json:"version"`
	TestCases string    `json:"test_cases"`
	Accuracy  float64   `json:"accuracy"`
	IterCount int       `json:"iter_count"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type TaskFilter struct {
	Status   string
	TaskType string
	Offset   int
	Limit    int
}

type TaskResult struct {
	SkillFile  string `json:"skill_file,omitempty"`
	Iterations string `json:"iterations,omitempty"`
	FinalAcc   float64 `json:"final_accuracy"`
	PassCount  int     `json:"pass_count"`
	FailCount  int     `json:"fail_count"`
	Message    string `json:"message,omitempty"`
}

// ===== v2 all-in-one 新增模型 =====

// Execution 任务执行记录（一次执行 = 一次子进程跑）
type Execution struct {
	ID              string     `json:"id"`
	TaskID          string     `json:"task_id,omitempty"`
	ScheduledTaskID string     `json:"scheduled_task_id,omitempty"`
	Source          string     `json:"source"` // 'manual' | 'scheduled' | 'retry'
	Command         string     `json:"command"`
	Prompt          string     `json:"prompt,omitempty"` // 原始 prompt（scheduled task 用，评估时优先用这个）
	Model           string     `json:"model,omitempty"`
	CliType         string     `json:"cli_type,omitempty"` // CLI 类型（claude/cbc/shell），用于"继续对话"时延续原运行环境
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Output          string     `json:"output,omitempty"`
	Error           string     `json:"error,omitempty"`
	ExitCode        int        `json:"exit_code"`
	ResumeSessionID string     `json:"resume_uuid,omitempty"` // claude -p 返回的 session_id（注意：不是单次执行的 uuid，是 session 级别的稳定标识），用于 --resume 继续对话
	// Status 显式状态字段，2026-06 新增。取代之前用 completed_at+exit_code+error 拼凑的判定。
	// 取值：running / success / failed / timeout / cancelled / build_error
	//   - running:     已创建未 Finish
	//   - success:     exit_code=0
	//   - failed:      exit_code≠0 且 ≠-1（子进程报错）
	//   - timeout:     exit_code=-1 且 errOut 含 "context deadline"
	//   - cancelled:   用户手动调 /api/executions/{id}/cancel 强制结束
	//   - build_error: runner.BuildCommand 返回 err
	Status string `json:"status,omitempty"`
	// JOIN 填充的标题（非数据库字段）
	TaskTitle          string `json:"task_title,omitempty"`
	ScheduledTaskTitle string `json:"scheduled_task_title,omitempty"`
	// 最近一次评估分数（NULL = 未评估）。list 接口 JOIN 填，单 exec 接口也填。
	EvaluationScore *float64 `json:"evaluation_score,omitempty"`
	// 评估次数（查询时实时 COUNT，非持久化）
	EvalCount int `json:"evaluation_count,omitempty"`
}

type Evaluation struct {
	ID             string    `json:"id"`
	TaskID         string    `json:"task_id,omitempty"`
	ExecutionID    string    `json:"execution_id,omitempty"`
	EvaluatorModel string    `json:"evaluator_model,omitempty"`
	Score          float64   `json:"score"`
	Comments       string    `json:"comments,omitempty"`
	DurationS     float64  `json:"duration_s,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// WebLink 网页链接
type WebLink struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	IconURL   string    `json:"icon_url,omitempty"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

// DirShortcut 目录快捷
// DirShortcutType values
const (
	DirShortcutTypeLocal  = "local"
	DirShortcutTypeRemote = "remote"
)

type DirShortcut struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Path            string     `json:"path"`
	SortOrder       int        `json:"sort_order"`
	Type            string     `json:"type"` // "local" | "remote"，默认 "local"
	// 远程连接配置（Type=remote 时使用）
	RemoteHost     string `json:"remote_host,omitempty"`    // IP 或域名
	RemotePort     string `json:"remote_port,omitempty"`    // 端口（默认 22）
	RemoteUser     string `json:"remote_user,omitempty"`    // 用户名
	RemotePath     string `json:"remote_path,omitempty"`    // 远程目录路径（默认空 = 主目录）
	RemotePassword string `json:"remote_password,omitempty"` // 密码（仅演示/内部用，生产建议用 key）
	AuthMethod     string `json:"auth_method,omitempty"`    // "password" | "key"，默认 "password"
	KeyPassword   string `json:"key_password,omitempty"`  // 加密私钥密码（AuthMethod=key 时使用）
	KeyPath        string `json:"key_path,omitempty"`       // 私钥路径（AuthMethod=key 时使用；已废弃，推荐 LocalKeyPath）
	LocalKeyPath   string `json:"local_key_path,omitempty"` // 本地私钥路径（优先于 KeyPath；不填则用全局 ssh.default_key_path）
	// 本地终端配置
	UseLegacyAlgorithms bool       `json:"use_legacy_algorithms"`       // 是否启用 legacy SSH 算法（默认 false，老服务器按需开启）
	CreatedAt           time.Time  `json:"created_at"`
	LastAccessedAt      *time.Time `json:"last_accessed_at,omitempty"`
}

// ScheduledTask 定时任务
type ScheduledTask struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	CronExpr       string     `json:"cron_expr"`
	CommandType    string     `json:"command_type"` // 'claude' | 'cbc' | 'shell'
	Model          string     `json:"model,omitempty"`
	Prompt         string     `json:"prompt,omitempty"`
	WorkingDir     string     `json:"working_dir,omitempty"`
	Enabled        bool       `json:"enabled"`
	TimeoutSec     int        `json:"timeout_sec"` // 超时秒数，0=默认（AI 任务 10 分钟，shell 任务 5 分钟）
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	NextRunAt      *time.Time `json:"next_run_at,omitempty"` // 下次执行时间；仅 enabled 任务注入，nil=禁用或解析失败
	LastStatus      string     `json:"last_status,omitempty"`
	LastExecutionID string    `json:"last_execution_id,omitempty"`
	LastSessionID   string    `json:"last_session_id,omitempty"` // 跨执行续用 session_id
	ResumeCount     int       `json:"resume_count"`              // 当前连续 resume 次数，达到 MaxResumeCount 后重置会话
	CreatedAt       time.Time `json:"created_at"`
}

// AppSetting KV 设置
type AppSetting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}