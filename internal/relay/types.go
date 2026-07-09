package relay

// RelayLog is a log entry for relay activity.
type RelayLog struct {
	ID           int64  `json:"id,omitempty"`
	Source       string `json:"source"`
	Destination  string `json:"destination"`
	Summary      string `json:"summary"`
	Direction    string `json:"direction"`
	Status       string `json:"status"`
	ErrorMsg     string `json:"error_msg,omitempty"`
	RequestSize  int    `json:"request_size"`
	ResponseSize int    `json:"response_size"`
	CreatedAt    string `json:"created_at,omitempty"`
}

// RelayStats aggregates relay statistics.
type RelayStats struct {
	TotalCount    int            `json:"total_count"`
	SuccessCount  int            `json:"success_count"`
	FailedCount   int            `json:"failed_count"`
	BySource      map[string]int `json:"by_source"`
	ByDestination map[string]int `json:"by_destination"`
	DateHistogram map[string]int `json:"date_histogram"`
}

// Repo defines the interface for relay log persistence.
type Repo interface {
	Log(log *RelayLog) error
	Stats(from, to, source string) (*RelayStats, error)
	InitSchema() error
}