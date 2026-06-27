// Package paths 提供跨平台的数据库/数据目录路径解析。
package paths

import (
	"errors"
	"os"
	"path/filepath"
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

// AITaskDir 返回单个 AI 任务的输出子目录：<root>/<taskID>/
//
// 每次 AI 任务执行都拿到独立子目录（taskID 通常取 backend.Task.ID / Execution.ID），
// 好处：
//   - 多任务并发写文件互不干扰
//   - 出错时清理某个 taskID 子目录即可
//   - 前端可以展示"这次执行产生了哪些文件"
//
// 自动 MkdirAll 0755。
func AITaskDir(taskID string) string {
	p := filepath.Join(AITaskRoot(), taskID)
	_ = os.MkdirAll(p, 0o755)
	return p
}

// ErrEmpty 等价于 "no path configured"，保留为占位 errors sentinel 以便测试。
var ErrEmpty = errors.New("paths: empty resolved path")
