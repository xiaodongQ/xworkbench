package task

import (
	"fmt"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	_ "modernc.org/sqlite"
)

func TestTaskCreateAndGet(t *testing.T) {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	repo := backend.NewTaskRepo(db)

	task := &backend.Task{
		ID:          "test-task-001",
		Title:       "Redis 集群节点失联定位",
		Description: "实现 Redis 集群节点失联场景的问题定位 Skill",
		Status:      backend.TaskStatusPending,
		Version:     "v0.0.1",
		CreatedAt:   time.Now(),
	}

	if err := repo.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get("test-task-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != task.Title {
		t.Errorf("Title = %q, want %q", got.Title, task.Title)
	}
	if got.Status != backend.TaskStatusPending {
		t.Errorf("Status = %q, want %q", got.Status, backend.TaskStatusPending)
	}
}

func TestTaskUpdateStatus(t *testing.T) {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	repo := backend.NewTaskRepo(db)

	task := &backend.Task{
		ID:        "test-task-002",
		Title:     "Redis 内存碎片分析",
		Status:    backend.TaskStatusPending,
		CreatedAt: time.Now(),
	}
	repo.Create(task)

	err = repo.UpdateStatus("test-task-002", backend.TaskStatusInProgress, "agent-001")
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err := repo.Get("test-task-002")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != backend.TaskStatusInProgress {
		t.Errorf("Status = %q, want %q", got.Status, backend.TaskStatusInProgress)
	}
	if got.Maintainer != "agent-001" {
		t.Errorf("Maintainer = %q, want %q", got.Maintainer, "agent-001")
	}
}

func TestTaskListFilter(t *testing.T) {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	repo := backend.NewTaskRepo(db)

	statuses := []string{
		backend.TaskStatusPending,
		backend.TaskStatusPending,
		backend.TaskStatusInProgress,
		backend.TaskStatusArchived,
	}
	for i, status := range statuses {
		task := &backend.Task{
			ID:        fmt.Sprintf("test-task-list-%d", i),
			Title:     fmt.Sprintf("Task %d", i),
			Status:    status,
			CreatedAt: time.Now(),
		}
		repo.Create(task)
	}

	pending, err := repo.List(backend.TaskFilter{Status: backend.TaskStatusPending, Offset: 0, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("pending count = %d, want 2", len(pending))
	}
}