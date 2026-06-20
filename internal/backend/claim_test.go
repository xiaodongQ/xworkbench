package backend

import (
	"testing"
	"time"
)

// TestClaimTask_NoDeps_Succeeds 回归保护：删 TaskDependencyRepo 后，remote task
// 的 claim 流程不应被破坏。HasUnmetHardDeps 在没有依赖记录时必须返回 false。
//
// 背景：
// - 修复 fix/remove-unused-abilities 分支删了 TaskDependency / TaskTemplate / Webhook
//   三个后端能力。
// - 删之前 TaskRepo.ClaimTask 调 depRepo.HasUnmetHardDeps() 阻挡有未完成硬依赖的 task。
// - 删之后：remote task 的 claim 应直接成功（无依赖即无阻挡）。
//
// 这个测试保证上述假设在生产代码里被正确实现。
func TestClaimTask_NoDeps_Succeeds(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil { t.Fatalf("TestDB: %v", err) }
	defer cleanup()
	taskRepo := NewTaskRepo(db)

	// 准备一个 pending + remote task，无任何依赖
	task := &Task{
		ID: "claim-no-deps-1", Title: "no deps", Status: TaskStatusPending,
		TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now(),
	}
	if err := taskRepo.Create(task); err != nil { t.Fatalf("Create: %v", err) }

	// claim 应成功（无依赖 = 无阻挡）
	if err := taskRepo.ClaimTask(task.ID, "agent-1"); err != nil {
		t.Errorf("ClaimTask failed: %v (expected success for task with no deps)", err)
	}

	// verify 状态确实变成 in_progress
	got, err := taskRepo.Get(task.ID)
	if err != nil { t.Fatalf("Get: %v", err) }
	if got.Status != TaskStatusInProgress {
		t.Errorf("status = %q, want %q", got.Status, TaskStatusInProgress)
	}
}

// TestClaimTask_NonRemote_Fails 回归保护：本地手动任务不应被 claim
// （这跟删 depDB 无关，但顺手加一个 claim 路径的基础测试）
func TestClaimTask_NonRemote_Fails(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil { t.Fatalf("TestDB: %v", err) }
	defer cleanup()
	taskRepo := NewTaskRepo(db)

	task := &Task{
		ID: "claim-manual-1", Title: "manual", Status: TaskStatusPending,
		TaskType: TaskTypeManual, Version: "v1", CreatedAt: time.Now(),
	}
	if err := taskRepo.Create(task); err != nil { t.Fatalf("Create: %v", err) }

	if err := taskRepo.ClaimTask(task.ID, "agent-1"); err == nil {
		t.Errorf("ClaimTask on manual task should fail (only remote can be claimed)")
	}
}
