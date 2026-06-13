// Package paths 提供跨平台的数据库/数据目录路径解析。
package paths

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// ResolveDBPath 返回 SQLite 数据库文件应使用的绝对路径。
//
// 解析顺序（首个命中即返回）：
//  1. $DB_PATH — 显式覆盖（dev/测试常用）
//  2. $SKILL_FACTORY_HOME/data/skill-factory.db — 项目级自建 HOME
//  3. <UserConfigDir>/skill-factory/data/skill-factory.db
//     - Windows: %AppData%\skill-factory\...
//     - macOS:   ~/Library/Application Support/skill-factory/...
//     - Linux:   $XDG_CONFIG_HOME/skill-factory/...（默认 ~/.config）
//  4. "data/skill-factory.db" — 兼容老开发模式（cwd 相对）
//
// 返回的路径父目录会自动 MkdirAll 0755；sqlite 在文件不存在时自己创建。
// 父目录创建失败不会被静默吞掉 — 后续 OpenDB 会报错。
func ResolveDBPath() string {
	if v := os.Getenv("DB_PATH"); v != "" {
		return ensureParent(v)
	}
	if home := os.Getenv("SKILL_FACTORY_HOME"); home != "" {
		return ensureParent(filepath.Join(home, "data", "skill-factory.db"))
	}
	if base, err := os.UserConfigDir(); err == nil {
		newPath := filepath.Join(base, "skill-factory", "data", "skill-factory.db")
		return ensureParent(maybeMigrateLegacy(newPath))
	}
	return ensureParent("data/skill-factory.db")
}

func ensureParent(p string) string {
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	return p
}

// maybeMigrateLegacy 若 cwd 下有 data/skill-factory.db 且新位置还没建过，
// 自动复制过去并 warn 一次，避免老用户在新机器上"看起来像丢数据"。
// 可通过设置 SKILL_FACTORY_HOME=$(pwd) 或 DB_PATH 跳过。
func maybeMigrateLegacy(newPath string) string {
	legacy := "data/skill-factory.db"
	if _, err := os.Stat(legacy); err != nil {
		return newPath
	}
	if _, err := os.Stat(newPath); err == nil {
		return newPath // 新位置已有，不动
	}
	if err := copyFile(legacy, newPath); err != nil {
		slog.Warn("paths: legacy DB found but copy failed; continuing with new path",
			slog.String("from", legacy), slog.String("to", newPath), slog.String("err", err.Error()))
		return newPath
	}
	slog.Warn("paths: migrated legacy DB",
		slog.String("from", legacy), slog.String("to", newPath))
	return newPath
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ErrEmpty 等价于 "no path configured"，保留为占位 errors sentinel 以便测试。
var ErrEmpty = errors.New("paths: empty resolved path")
