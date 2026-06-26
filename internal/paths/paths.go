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

// AISandboxDir 返回 AI 任务沙盒目录的绝对路径（默认 data/ai-sandbox/）。
//
// 所有由 claude/cbc 调起 AI 任务时的 CWD 都应该用这个目录：
//   - 隔离 AI 任务生成的代码、中间产物到 data/ 下，源码树（internal/ 等）保持干净
//   - data/ 已在 .gitignore，AI 写文件不会被误 commit
//
// 路径父目录会自动 MkdirAll 0755。
func AISandboxDir() string {
	return ensureParent("data/ai-sandbox")
}

func ensureParent(p string) string {
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	return p
}

// ErrEmpty 等价于 "no path configured"，保留为占位 errors sentinel 以便测试。
var ErrEmpty = errors.New("paths: empty resolved path")
