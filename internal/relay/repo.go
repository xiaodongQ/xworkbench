package relay

import (
	"database/sql"
	"fmt"
	"strings"
)

// SQLiteRelayRepo implements Repo using SQLite.
type SQLiteRelayRepo struct {
	db *sql.DB
}

// NewSQLiteRelayRepo creates a new SQLite-backed Repo.
func NewSQLiteRelayRepo(db *sql.DB) *SQLiteRelayRepo {
	return &SQLiteRelayRepo{db: db}
}

// InitSchema creates the relay_logs table if it doesn't exist.
func (r *SQLiteRelayRepo) InitSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS relay_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source VARCHAR(50),
		destination VARCHAR(500),
		summary VARCHAR(500),
		direction VARCHAR(10),
		status VARCHAR(20),
		error_msg TEXT,
		request_size INTEGER,
		response_size INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_relay_logs_created_at ON relay_logs(created_at);
	CREATE INDEX IF NOT EXISTS idx_relay_logs_source ON relay_logs(source);
	CREATE INDEX IF NOT EXISTS idx_relay_logs_destination ON relay_logs(destination);
	`
	_, err := r.db.Exec(schema)
	return err
}

// Log inserts a relay log entry.
func (r *SQLiteRelayRepo) Log(log *RelayLog) error {
	_, err := r.db.Exec(`
		INSERT INTO relay_logs (source, destination, summary, direction, status, error_msg, request_size, response_size)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		log.Source, log.Destination, log.Summary, log.Direction,
		log.Status, log.ErrorMsg, log.RequestSize, log.ResponseSize)
	return err
}

// Stats returns aggregated relay statistics.
func (r *SQLiteRelayRepo) Stats(from, to string) (*RelayStats, error) {
	stats := &RelayStats{
		BySource:      make(map[string]int),
		ByDestination: make(map[string]int),
		DateHistogram: make(map[string]int),
	}

	whereClause := ""
	args := make([]any, 0, 2)
	if from != "" {
		whereClause += " AND created_at >= ?"
		args = append(args, from)
	}
	if to != "" {
		whereClause += " AND created_at <= ?"
		args = append(args, to)
	}
	if whereClause != "" {
		whereClause = " WHERE " + strings.TrimPrefix(whereClause, " AND ")
	}

	// Total counts
	row := r.db.QueryRow(`SELECT COUNT(*), SUM(CASE WHEN status='success' THEN 1 ELSE 0 END), SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END) FROM relay_logs` + whereClause, args...)
	var total, success, failed sql.NullInt64
	if err := row.Scan(&total, &success, &failed); err != nil {
		return nil, fmt.Errorf("stats count: %w", err)
	}
	stats.TotalCount = int(total.Int64)
	stats.SuccessCount = int(success.Int64)
	stats.FailedCount = int(failed.Int64)

	// By source
	rows, err := r.db.Query(`SELECT source, COUNT(*) FROM relay_logs`+whereClause+` GROUP BY source`, args...)
	if err != nil {
		return nil, fmt.Errorf("stats by source: %w", err)
	}
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.BySource[source] = count
	}
	rows.Close()

	// By destination
	rows, err = r.db.Query(`SELECT destination, COUNT(*) FROM relay_logs`+whereClause+` GROUP BY destination`, args...)
	if err != nil {
		return nil, fmt.Errorf("stats by destination: %w", err)
	}
	for rows.Next() {
		var dest string
		var count int
		if err := rows.Scan(&dest, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.ByDestination[dest] = count
	}
	rows.Close()

	// Date histogram (date part of created_at)
	rows, err = r.db.Query(`SELECT DATE(created_at) as day, COUNT(*) FROM relay_logs`+whereClause+` GROUP BY day ORDER BY day`, args...)
	if err != nil {
		return nil, fmt.Errorf("stats histogram: %w", err)
	}
	for rows.Next() {
		var day string
		var count int
		if err := rows.Scan(&day, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.DateHistogram[day] = count
	}
	rows.Close()

	return stats, nil
}

// Ensure SQLiteRelayRepo implements Repo.
var _ Repo = (*SQLiteRelayRepo)(nil)