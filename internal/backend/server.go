package backend

import (
	"database/sql"
)

func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// foreign_keys 必须每次连接都设（连接级）
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		logger.Warnf("PRAGMA foreign_keys: %v", err)
	}
	// WAL：读写并发，写不再阻塞读
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		logger.Warnf("PRAGMA journal_mode: %v", err)
	}
	// busy_timeout：万一遇到锁竞争，等 5s 再报 BUSY
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		logger.Warnf("PRAGMA busy_timeout: %v", err)
	}
	return db, nil
}