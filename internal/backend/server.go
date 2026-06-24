package backend

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/logger"
)

// OpenDB 打开 SQLite 数据库,正确配置 PRAGMA 与连接池。
//
// 关键点:SQLite PRAGMA 是 per-connection 的(database/sql 是懒连接池,
// 直接 db.Exec("PRAGMA ...") 只在池中已有连接上生效,后续 lazy 创建的连接
// 会用默认值:busy_timeout=0(立即失败)、journal_mode=DELETE(写锁粒度大),
// 极易出现 SQLITE_BUSY)。
//
// 修复:用 modernc.org/sqlite 驱动的 DSN 参数 `_pragma=key(value)`,驱动层
// 会在每个新连接打开时自动执行,确保 per-connection 生效。busy_timeout 会被
// 驱动排到第一个执行(见 modernc.org/sqlite sqlite.go:applyQueryParams)。
//
// 启动期还会 QueryRow("PRAGMA journal_mode") 验证 WAL 真的生效,失败 fail-fast
// 避免带隐患运行(文件被旧进程持有、文件系统不支持 WAL 等场景)。
func OpenDB(path string) (*sql.DB, error) {
	// _pragma 参数会被 modernc.org/sqlite 在每个新连接打开时自动执行
	// busy_timeout 必须用毫秒值;journal_mode=WAL 是 set-and-verify,实际是否生效
	// 由下面 QueryRow 的结果判断
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	// 显式限制连接池,避免无限扩,WAL 下并发写者排队可控
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)

	// 启动期校验:WAL 必须实际生效。PRAGMA journal_mode 是 set-and-verify,
	// 执行后会返回当前实际模式。如果不是 wal(常见原因:文件被旧进程持有、
	// 文件系统不支持、只读挂载等),直接 fail-fast,避免带隐患运行。
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("verify journal_mode: %w", err)
	}
	if !strings.EqualFold(mode, "wal") {
		_ = db.Close()
		return nil, fmt.Errorf("journal_mode = %q, want wal (db file may be locked by another process, or filesystem does not support WAL)", mode)
	}

	logger.Logger.Infow("db: opened",
		"path", path,
		"journal_mode", mode,
		"busy_timeout_ms", 5000,
		"max_open_conns", 8,
	)
	return db, nil
}