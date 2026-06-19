package experience

import (
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	_ "modernc.org/sqlite"
)

func TestExperienceCreateAndSearch(t *testing.T) {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	repo := backend.NewExperienceRepo(db)

	exp := &backend.Experience{
		ID:       "exp-redis-cluster",
		Module:   "redis-cluster",
		Keywords: "CLUSTERDOWN,MOVED,ASK,READONLY",
		Scene:    "集群节点失联定位",
		Version:  "v1.0.0",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := repo.Create(exp); err != nil {
		t.Fatalf("Create: %v", err)
	}

	results, err := repo.Search("redis-cluster")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("search count = %d, want 1", len(results))
	}
	if results[0].Module != "redis-cluster" {
		t.Errorf("Module = %q, want %q", results[0].Module, "redis-cluster")
	}
}