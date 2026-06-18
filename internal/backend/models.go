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
	// 远程 Agent 相关
	TaskType         string   `json:"task_type,omitempty"`
	ClaimerAgentID   string   `json:"claimer_agent_id,omitempty"`
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
	ID             string     `json:"id"`
	TaskID         string     `json:"task_id,omitempty"`
	ScheduledTaskID string    `json:"scheduled_task_id,omitempty"`
	Source         string     `json:"source"` // 'manual' | 'scheduled' | 'retry'
	Command        string     `json:"command"`
	Prompt         string     `json:"prompt,omitempty"` // 原始 prompt（scheduled task 用，评估时优先用这个）
	Model          string     `json:"model,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	Output         string     `json:"output,omitempty"`
	Error          string     `json:"error,omitempty"`
	ExitCode       int        `json:"exit_code"`
	// JOIN 填充的标题（非数据库字段）
	TaskTitle         string `json:"task_title,omitempty"`
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
	RemoteUser     string `json:"remote_user,omitempty"`    // 用户名
	RemotePath     string `json:"remote_path,omitempty"`    // 远程目录路径（默认空 = 主目录）
	RemotePassword string `json:"remote_password,omitempty"` // 密码（仅演示/内部用，生产建议用 key）
	AuthMethod     string `json:"auth_method,omitempty"`    // "password" | "key"，默认 "password"
	KeyPath        string `json:"key_path,omitempty"`       // 私钥路径（AuthMethod=key 时使用）
	// 本地终端配置
	TerminalCmd    string `json:"terminal_cmd,omitempty"`   // 启动终端的命令，默认从 AppSettings 读取
	CreatedAt      time.Time  `json:"created_at"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
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
	TimeoutSec     int        `json:"timeout_sec"` // 超时秒数，0=默认（AI任务1小时，shell 5分钟）
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	LastStatus     string     `json:"last_status,omitempty"`
	LastExecutionID string    `json:"last_execution_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// AppSetting KV 设置
type AppSetting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}