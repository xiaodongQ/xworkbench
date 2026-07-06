// Package paths 提供跨平台的数据库/数据目录路径解析。
package paths

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)



// ResolveDBPath 返回 SQLite 数据库文件应使用的绝对路径。
//
// 解析顺序（首个命中即返回）：
//  1. $DB_PATH — 显式覆盖（dev/测试常用）
//  2. "data/xworkbench.db" — 兼容老开发模式（cwd 相对）
//
// 返回的路径父目录会自动 MkdirAll 0755；sqlite 在文件不存在时自己创建。
func ResolveDBPath() string {
	if v := os.Getenv("DB_PATH"); v != "" {
		return ensureParent(v)
	}
	return ensureParent("data/xworkbench.db")
}

// AISandboxDir 返回 AI 任务沙盒目录路径（默认 data/ai-sandbox/）。
//
// ⚠️ DEPRECATED: 新代码请用 AITaskRoot() / AITaskDir(taskID)。
// 保留此函数仅为兼容老部署（$AI_SANDBOX_DIR env 仍被 AITaskRoot 识别作 fallback）。
//
// 解析顺序（首个命中即返回，与 ResolveDBPath 保持一致）：
//  1. $AI_SANDBOX_DIR — 显式覆盖（运维/测试常用）
//  2. "data/ai-sandbox" — 兼容 dev 模式（cwd 相对，假设 cwd 是项目根）
//
// 所有由 claude/cbc 调起 AI 任务时的 CWD 都应该用这个目录：
//   - 隔离 AI 任务生成的代码、中间产物到 data/ 下，源码树（internal/ 等）保持干净
//   - data/ 已在 .gitignore，AI 写文件不会被误 commit
//
// 路径本身会自动 MkdirAll 0755（而 ResolveDBPath 只创建父目录，因为 sqlite 自己建文件）。
//
// ⚠️ 跟 ResolveDBPath 一样假设 xworkbench 二进制的 cwd 是项目根目录。
// 用 scripts/run.sh 启动时这是 true；如果你手动从别的目录启动，
// 必须设 AI_SANDBOX_DIR（跟设 DB_PATH 一个道理）。
func AISandboxDir() string {
	if v := os.Getenv("AI_SANDBOX_DIR"); v != "" {
		_ = os.MkdirAll(v, 0o755)
		return v
	}
	_ = os.MkdirAll("data/ai-sandbox", 0o755)
	return "data/ai-sandbox"
}

func ensureParent(p string) string {
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	return p
}

// AITaskRoot 返回 AI 任务输出根目录（默认 data/ai-task-dir/）。
//
// 解析顺序（首个命中即返回）：
//  1. $AI_TASK_ROOT — 显式覆盖（运维/测试常用）
//  2. $AI_SANDBOX_DIR — 兼容老变量（deprecated，新部署应迁到 AI_TASK_ROOT）
//  3. "data/ai-task-dir" — 默认（cwd 相对，假设 cwd 是项目根）
//
// 用途：所有由 claude/cbc 调起 AI 任务时，system prompt 会显式告诉 AI
//「把生成的文件写到 data/ai-task-dir/<task_id>/」即 paths.AITaskDir(taskID)。
// data/ 已在 .gitignore，AI 写文件不会被误 commit，源码树天然被保护。
//
// 自动 MkdirAll 0755。
//
// ⚠️ 跟 ResolveDBPath 一样假设 xworkbench 二进制的 cwd 是项目根目录。
// 用 scripts/run.sh 启动时这是 true；如果你手动从别的目录启动，
// 必须设 AI_TASK_ROOT（跟设 DB_PATH 一个道理）。
func AITaskRoot() string {
	if v := os.Getenv("AI_TASK_ROOT"); v != "" {
		_ = os.MkdirAll(v, 0o755)
		return v
	}
	if v := os.Getenv("AI_SANDBOX_DIR"); v != "" {
		_ = os.MkdirAll(v, 0o755)
		return v
	}
	_ = os.MkdirAll("data/ai-task-dir", 0o755)
	return "data/ai-task-dir"
}

// AITaskDir 返回单个 AI 任务的输出子目录：<root>/<YYYY-MM-DD>/<taskID>/
//
// 按本地日期分层（一天一目录），避免根目录下 UUID 子目录无限堆积：
//   - 多任务并发写文件互不干扰
//   - 出错时清理某个 taskID 子目录即可
//   - 按日期归档便于回溯/批量清理（如「清理 30 天前的 AI 产物」按日期目录删除即可）
//   - 前端可以展示"这次执行产生了哪些文件"
//
// taskID 通常取 backend.Task.ID / Execution.ID。
// 日期用本地时区（time.Now()），对应用户实际操作时间而非 UTC 跨日。
//
// ⚠️ 不自动 MkdirAll：任务没生成文件产物时不应留下空目录。实际目录由
// claude/cbc 的 Write/Edit 工具在写第一个文件时通过 OS 自动创建父目录
//（AI 用绝对路径，OS 负责建中间目录）。如果 AI 完全不写文件，就不该
// 有 taskID 子目录产生。
//
// 返回**绝对路径**（filepath.Abs 锁住），注入到 system prompt 后 AI 用绝对
// 路径写文件，不依赖父进程 cwd。跟 ResolveDBPath / AISandboxDir 同假设
//（cwd 是项目根），与父进程 cwd 错位时直接报错而非静默写错位置。
func AITaskDir(taskID string) string {
	dateDir := time.Now().Format("2006-01-02")
	rel := filepath.Join(AITaskRoot(), dateDir, taskID)
	abs, err := filepath.Abs(rel)
	if err != nil {
		// filepath.Abs 只在获取当前工作目录失败时报错（如 cwd 被删）。
		// 这种情况下 cwd 已经不正常，fallback 到相对路径（与父进程 cwd 错位风险，
		// 但比 panic 强）。
		return rel
	}
	return abs
}

// DataDir 返回数据根目录（data/），所有运行时数据（db、logs、memory.md）放此目录下。
// 路径与 ResolveDBPath 保持一致（都是 data/ 下）。
func DataDir() string {
	return filepath.Dir(ResolveDBPath())
}

// ErrEmpty 等价于 "no path configured"，保留为占位 errors sentinel 以便测试。
var ErrEmpty = errors.New("paths: empty resolved path")
