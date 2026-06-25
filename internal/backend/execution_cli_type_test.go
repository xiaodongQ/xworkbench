package backend

import (
	"testing"
	"time"
)

// TestExecutionRepo_PreservesCliType 验证 executions 表新增 cli_type 字段后,
// Create + Get 能正确保留。回归保护：继续对话需要这条信息来"延续原 CLI"(之前
// handleExecutionContinue 硬编码 claude,原 exec 是 cbc/shell 时会切断运行环境)。
func TestExecutionRepo_PreservesCliType(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	repo := NewExecutionRepo(db)

	cases := []struct {
		name     string
		cliType  string
		wantType string
	}{
		{"claude", "claude", "claude"},
		{"cbc", "cbc", "cbc"},
		{"shell", "shell", "shell"},
		{"empty omitted", "", ""},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exec := &Execution{
				ID:        "exec-cli-" + c.name,
				Source:    "manual",
				Command:   "echo " + c.cliType,
				CliType:   c.cliType,
				StartedAt: time.Now(),
			}
			if err := repo.Create(exec); err != nil {
				t.Fatalf("Create: %v", err)
			}
			got, err := repo.Get(exec.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.CliType != c.wantType {
				t.Errorf("CliType round-trip = %q, want %q", got.CliType, c.wantType)
			}
		})
		_ = i
	}
}