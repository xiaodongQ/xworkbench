package backend

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTaskAttachAndListExperiences 验证 task <-> experience 多对多关联：
// AttachExperiences + ListExperienceIDsForTask + Get 自动加载 + Delete 级联清理。
func TestTaskAttachAndListExperiences(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	taskRepo := NewTaskRepo(db)
	expRepo := NewExperienceRepo(db)

	// 准备 3 条经验
	for _, mod := range []string{"redis-cluster", "k8s-oom", "mysql-deadlock"} {
		exp := &Experience{
			ID:        "exp-" + mod,
			Module:    mod,
			Scene:     "测试场景 " + mod,
			Version:   "v1.0.0",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := expRepo.Create(exp); err != nil {
			t.Fatalf("Create exp[%s]: %v", mod, err)
		}
	}

	// 创建 task，挂 3 条经验
	task := &Task{
		ID:        "task-multi-001",
		Title:     "多经验关联测试",
		Status:    TaskStatusPending,
		CreatedAt: time.Now(),
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("Create task: %v", err)
	}
	if err := taskRepo.AttachExperiences(task.ID, []string{"exp-redis-cluster", "exp-k8s-oom", "exp-mysql-deadlock"}); err != nil {
		t.Fatalf("AttachExperiences: %v", err)
	}

	// ListExperienceIDsForTask 验证顺序（按 created_at 升序）
	ids, err := taskRepo.ListExperienceIDsForTask(task.ID)
	if err != nil {
		t.Fatalf("ListExperienceIDsForTask: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("ids len = %d, want 3: %v", len(ids), ids)
	}
	want := []string{"exp-redis-cluster", "exp-k8s-oom", "exp-mysql-deadlock"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("ids[%d] = %q, want %q", i, ids[i], want[i])
		}
	}

	// Get 应该自动加载 ExperienceIDs
	got, err := taskRepo.Get(task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.ExperienceIDs) != 3 {
		t.Errorf("Get.ExperienceIDs len = %d, want 3", len(got.ExperienceIDs))
	}

	// 重复 Attach 已存在记录应被忽略（INSERT OR IGNORE）
	if err := taskRepo.AttachExperiences(task.ID, []string{"exp-redis-cluster"}); err != nil {
		t.Fatalf("AttachExperiences dup: %v", err)
	}
	ids, _ = taskRepo.ListExperienceIDsForTask(task.ID)
	if len(ids) != 3 {
		t.Errorf("after dup Attach, ids len = %d, want 3", len(ids))
	}
}

// TestTaskSetExperiences 全量替换 task 的经验列表。
func TestTaskSetExperiences(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	taskRepo := NewTaskRepo(db)
	expRepo := NewExperienceRepo(db)

	for _, mod := range []string{"a", "b", "c"} {
		expRepo.Create(&Experience{ID: "exp-" + mod, Module: mod, Version: "v1", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	}
	taskRepo.Create(&Task{ID: "task-set-001", Title: "set test", Status: TaskStatusPending, CreatedAt: time.Now()})
	taskRepo.AttachExperiences("task-set-001", []string{"exp-a", "exp-b"})

	// 替换为 [c]
	if err := taskRepo.SetTaskExperiences("task-set-001", []string{"exp-c"}); err != nil {
		t.Fatalf("SetTaskExperiences: %v", err)
	}
	ids, _ := taskRepo.ListExperienceIDsForTask("task-set-001")
	if len(ids) != 1 || ids[0] != "exp-c" {
		t.Errorf("after Set, ids = %v, want [exp-c]", ids)
	}

	// 传空数组 = 解绑全部
	if err := taskRepo.SetTaskExperiences("task-set-001", []string{}); err != nil {
		t.Fatalf("SetTaskExperiences empty: %v", err)
	}
	ids, _ = taskRepo.ListExperienceIDsForTask("task-set-001")
	if len(ids) != 0 {
		t.Errorf("after Set empty, ids = %v, want []", ids)
	}
}

// TestTaskDetachExperience 单条解绑。
func TestTaskDetachExperience(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	taskRepo := NewTaskRepo(db)
	expRepo := NewExperienceRepo(db)

	for _, mod := range []string{"a", "b"} {
		expRepo.Create(&Experience{ID: "exp-" + mod, Module: mod, Version: "v1", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	}
	taskRepo.Create(&Task{ID: "task-detach-001", Title: "detach test", Status: TaskStatusPending, CreatedAt: time.Now()})
	taskRepo.AttachExperiences("task-detach-001", []string{"exp-a", "exp-b"})

	if err := taskRepo.DetachExperience("task-detach-001", "exp-a"); err != nil {
		t.Fatalf("DetachExperience: %v", err)
	}
	ids, _ := taskRepo.ListExperienceIDsForTask("task-detach-001")
	if len(ids) != 1 || ids[0] != "exp-b" {
		t.Errorf("after Detach, ids = %v, want [exp-b]", ids)
	}
}

// TestTaskDeleteCascadesExperiences 验证删除 task 会级联清理 task_experiences。
func TestTaskDeleteCascadesExperiences(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	taskRepo := NewTaskRepo(db)
	expRepo := NewExperienceRepo(db)

	expRepo.Create(&Experience{ID: "exp-x", Module: "x", Version: "v1", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	taskRepo.Create(&Task{ID: "task-cascade-001", Title: "cascade", Status: TaskStatusPending, CreatedAt: time.Now()})
	taskRepo.AttachExperiences("task-cascade-001", []string{"exp-x"})

	if err := taskRepo.Delete("task-cascade-001"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	ids, _ := taskRepo.ListExperienceIDsForTask("task-cascade-001")
	if len(ids) != 0 {
		t.Errorf("after Delete, ids = %v, want [] (cascade should clean)", ids)
	}
	// 经验本身没被删（task_experiences 是 task 侧的级联）
	if _, err := expRepo.Get("exp-x"); err != nil {
		t.Errorf("experience itself should NOT be deleted, got err: %v", err)
	}
}

// TestTaskListIncludesExperienceIDs 验证 List 接口也会填充 ExperienceIDs。
func TestTaskListIncludesExperienceIDs(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	taskRepo := NewTaskRepo(db)
	expRepo := NewExperienceRepo(db)

	if err := expRepo.Create(&Experience{ID: "exp-y", Module: "y", Version: "v1", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("Create exp: %v", err)
	}
	if err := taskRepo.Create(&Task{ID: "task-list-001", Title: "list test", Status: TaskStatusPending, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("Create task: %v", err)
	}
	if err := taskRepo.AttachExperiences("task-list-001", []string{"exp-y"}); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	list, err := taskRepo.List(TaskFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	var found *Task
	for _, t1 := range list {
		if t1.ID == "task-list-001" {
			found = t1
			break
		}
	}
	if found == nil {
		t.Fatal("task-list-001 not found in list")
	}
	if len(found.ExperienceIDs) != 1 || found.ExperienceIDs[0] != "exp-y" {
		t.Errorf("list.ExperienceIDs = %v, want [exp-y]", found.ExperienceIDs)
	}
}

// TestReleaseTasksFromAgent 验证 agent 心跳超时后任务被正确释放回 pending 池。
func TestReleaseTasksFromAgent(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	taskRepo := NewTaskRepo(db)
	agentRepo := NewAgentRepo(db)

	// 准备 agent
	agentID := "agent-test-001"
	if err := agentRepo.Register(&Agent{
		ID: agentID, Name: "test-agent", TokenHash: "hash-1",
		Status: "online", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("Register agent: %v", err)
	}

	// 准备 2 个 remote 任务（pending 和 in_progress 状态各一）
	pendingTask := &Task{
		ID: "task-pending-1", Title: "p1", Status: TaskStatusPending,
		TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now(),
	}
	if err := taskRepo.Create(pendingTask); err != nil {
		t.Fatalf("Create pending: %v", err)
	}

	claimedTask := &Task{
		ID: "task-claimed-1", Title: "c1", Status: TaskStatusPending,
		TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now(),
	}
	if err := taskRepo.Create(claimedTask); err != nil {
		t.Fatalf("Create claimed: %v", err)
	}
	// claim
	if err := taskRepo.ClaimTask("task-claimed-1", agentID); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	// 准备一个手动任务（不应被释放）
	manualTask := &Task{
		ID: "task-manual-1", Title: "m1", Status: TaskStatusInProgress,
		TaskType: TaskTypeManual, Maintainer: "user-1", Version: "v1", CreatedAt: time.Now(),
	}
	if err := taskRepo.Create(manualTask); err != nil {
		t.Fatalf("Create manual: %v", err)
	}

	// 释放 agent 任务
	if _, err := taskRepo.ReleaseTasksFromAgent(agentID); err != nil {
		t.Fatalf("ReleaseTasksFromAgent: %v", err)
	}

	// 验证：claimed task 应该回到 pending 且 claimer 清空
	got, err := taskRepo.Get("task-claimed-1")
	if err != nil {
		t.Fatalf("Get claimed: %v", err)
	}
	if got.Status != TaskStatusPending {
		t.Errorf("claimed task status = %s, want pending", got.Status)
	}
	if got.ClaimerAgentID != "" {
		t.Errorf("claimed task claimer = %q, want empty", got.ClaimerAgentID)
	}

	// 验证：pending task 不应被影响
	got2, _ := taskRepo.Get("task-pending-1")
	if got2.Status != TaskStatusPending {
		t.Errorf("pending task status = %s, want pending", got2.Status)
	}

	// 验证：manual 任务不应被影响
	got3, _ := taskRepo.Get("task-manual-1")
	if got3.Status != TaskStatusInProgress {
		t.Errorf("manual task status = %s, want in_progress", got3.Status)
	}
}

// TestHashToken 验证 SHA-256 hash 是 deterministic 且不同 token 不撞。
func TestHashToken(t *testing.T) {
	h1 := HashToken("token-abc-123")
	h2 := HashToken("token-abc-123")
	if h1 != h2 {
		t.Errorf("HashToken not deterministic: %s != %s", h1, h2)
	}
	h3 := HashToken("token-abc-124")
	if h1 == h3 {
		t.Errorf("HashToken collision: %s == %s", h1, h3)
	}
	// hash 长度 = 64 hex chars
	if len(h1) != 64 {
		t.Errorf("HashToken length = %d, want 64", len(h1))
	}
}

// TestReleaseStaleTasks 验证超时任务被释放回 pending 池。
func TestReleaseStaleTasks(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	taskRepo := NewTaskRepo(db)
	agentRepo := NewAgentRepo(db)
	agentID := "agent-stale-001"
	agentRepo.Register(&Agent{ID: agentID, Name: "stale-test", TokenHash: "h", Status: "online", CreatedAt: time.Now()})

	// 创建 3 个 task：1 个已完成不动，1 个刚 claim，1 个老 claim
	archivedTask := &Task{ID: "task-archived-1", Title: "a", Status: TaskStatusArchived, TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now(), ClaimerAgentID: agentID}
	taskRepo.Create(archivedTask)

	freshTask := &Task{ID: "task-fresh-1", Title: "f", Status: TaskStatusPending, TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now()}
	taskRepo.Create(freshTask)
	taskRepo.ClaimTask("task-fresh-1", agentID) // claim_at = now

	// 手动 claim 一个老 task（claimed_at = 2小时前）
	oldTask := &Task{ID: "task-old-1", Title: "o", Status: TaskStatusPending, TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now()}
	taskRepo.Create(oldTask)
	taskRepo.ClaimTask("task-old-1", agentID)
	// 把 claimed_at 改到 2 小时前
	_, err = db.Exec(`UPDATE tasks SET claimed_at=datetime('now', '-2 hours') WHERE id='task-old-1'`)
	if err != nil {
		t.Fatalf("set old claimed_at: %v", err)
	}

	// 释放超过 60s 还没完成的任务
	n, err := taskRepo.ReleaseStaleTasks(60)
	if err != nil {
		t.Fatalf("ReleaseStaleTasks: %v", err)
	}
	if n != 1 {
		t.Errorf("released count = %d, want 1", n)
	}

	// 验证：老任务回到 pending
	old, _ := taskRepo.Get("task-old-1")
	if old.Status != TaskStatusPending {
		t.Errorf("old task status = %s, want pending", old.Status)
	}
	if old.ClaimerAgentID != "" {
		t.Errorf("old task claimer = %q, want empty", old.ClaimerAgentID)
	}

	// 验证：刚 claim 的没被释放
	fresh, _ := taskRepo.Get("task-fresh-1")
	if fresh.Status != TaskStatusInProgress {
		t.Errorf("fresh task status = %s, want in_progress", fresh.Status)
	}

	// 验证：archived 任务没动
	archived, _ := taskRepo.Get("task-archived-1")
	if archived.Status != TaskStatusArchived {
		t.Errorf("archived task status = %s, want archived", archived.Status)
	}
}

// TestClaimTaskConcurrent 验证并发 claim 同一 task 时只有一个 agent 能成功。
func TestClaimTaskConcurrent(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	taskRepo := NewTaskRepo(db)

	// 创建任务
	task := &Task{ID: "task-concurrent-1", Title: "race", Status: TaskStatusPending, TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now()}
	taskRepo.Create(task)

	// 10 个并发 claim
	const N = 10
	var wg sync.WaitGroup
	var successCount int32
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			agentID := fmt.Sprintf("agent-%d", idx)
			if err := taskRepo.ClaimTask("task-concurrent-1", agentID); err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("concurrent claim success count = %d, want 1", successCount)
	}

	// 验证：只有一个 agent 拿到了 claimer_agent_id
	t1, _ := taskRepo.Get("task-concurrent-1")
	if t1.Status != TaskStatusInProgress {
		t.Errorf("task status = %s, want in_progress", t1.Status)
	}
	if t1.ClaimerAgentID == "" {
		t.Errorf("task claimer_agent_id is empty")
	}
}

// TestTaskEvent 验证 task event CRUD + ListByTask 顺序。
func TestTaskEvent(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	repo := NewTaskEventRepo(db)
	for i, et := range []string{"created", "claimed", "reported"} {
		if err := repo.Record(&TaskEvent{
			TaskID: "task-evt-1", EventType: et, Actor: "test",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	events, err := repo.ListByTask("task-evt-1", 10)
	if err != nil {
		t.Fatalf("ListByTask: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("events len = %d, want 3", len(events))
	}
	// ListByTask 倒序，最新的（reported）在前
	if events[0].EventType != "reported" {
		t.Errorf("events[0] = %s, want reported", events[0].EventType)
	}
}

// TestTaskComment 验证评论 CRUD + 嵌套回复。
func TestTaskComment(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	repo := NewTaskCommentRepo(db)

	c1 := &TaskComment{TaskID: "task-cmt-1", Author: "user-1", Content: "第一条评论"}
	if err := repo.Create(c1); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c1.ID == "" {
		t.Errorf("ID auto-gen failed")
	}

	// 嵌套回复
	c2 := &TaskComment{TaskID: "task-cmt-1", Author: "user-2", Content: "回复第一条", ParentID: c1.ID}
	repo.Create(c2)

	// 列出
	cmts, _ := repo.ListByTask("task-cmt-1")
	if len(cmts) != 2 {
		t.Errorf("List len = %d, want 2", len(cmts))
	}
	if cmts[1].ParentID != c1.ID {
		t.Errorf("parent_id mismatch: %s", cmts[1].ParentID)
	}

	// Update
	c1.Content = "修改后的内容"
	repo.Update(c1)
	got, _ := repo.Get(c1.ID)
	if got.Content != "修改后的内容" {
		t.Errorf("update failed")
	}

	// Delete
	repo.Delete(c2.ID)
	cmts2, _ := repo.ListByTask("task-cmt-1")
	if len(cmts2) != 1 {
		t.Errorf("after delete len = %d, want 1", len(cmts2))
	}
}

// TestNextClaimable 验证优先级队列：priority 高的先 claim。
func TestNextClaimable(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	taskRepo := NewTaskRepo(db)

	// 创建 3 个 pending 任务，priority 不同
	tasks := []*Task{
		{ID: "low-1", Title: "low", Status: TaskStatusPending, TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now().Add(-3 * time.Second), Priority: 1},
		{ID: "high-1", Title: "high", Status: TaskStatusPending, TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now().Add(-1 * time.Second), Priority: 10},
		{ID: "mid-1", Title: "mid", Status: TaskStatusPending, TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now().Add(-2 * time.Second), Priority: 5},
	}
	for _, t1 := range tasks {
		if err := taskRepo.Create(t1); err != nil {
			t.Fatalf("Create %s: %v", t1.ID, err)
		}
	}

	// NextClaimable 应返回 high-1
	next, err := taskRepo.NextClaimable("agent-1")
	if err != nil {
		t.Fatalf("NextClaimable: %v", err)
	}
	if next != "high-1" {
		t.Errorf("NextClaimable = %s, want high-1", next)
	}

	// 领取 high-1 后，下一个应是 mid-1
	taskRepo.ClaimTask("high-1", "agent-1")
	next, _ = taskRepo.NextClaimable("agent-2")
	if next != "mid-1" {
		t.Errorf("NextClaimable after claim = %s, want mid-1", next)
	}
}

// TestAutoClaimEnabled 验证 auto_claim_enabled 开关默认 false。
func TestAutoClaimEnabled(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	agentRepo := NewAgentRepo(db)

	a := &Agent{
		ID:        "agent-toggle-test",
		Name:      "ToggleTest",
		TokenHash: HashToken("test-token-toggle"),
		Status:    "online",
		// AutoClaimEnabled 默认为 false（Go 默认零值）
	}
	if err := agentRepo.Register(a); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, _ := agentRepo.GetByID("agent-toggle-test")
	if got.AutoClaimEnabled {
		t.Errorf("AutoClaimEnabled should default to false")
	}

	// 开启开关
	agentRepo.SetAutoClaimEnabled("agent-toggle-test", true)
	got2, _ := agentRepo.GetByID("agent-toggle-test")
	if !got2.AutoClaimEnabled {
		t.Errorf("AutoClaimEnabled should be true after SetAutoClaimEnabled")
	}

	// 关闭开关
	agentRepo.SetAutoClaimEnabled("agent-toggle-test", false)
	got3, _ := agentRepo.GetByID("agent-toggle-test")
	if got3.AutoClaimEnabled {
		t.Errorf("AutoClaimEnabled should be false after second SetAutoClaimEnabled")
	}
}
