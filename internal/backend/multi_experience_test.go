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
	if err != nil { t.Fatalf("TestDB: %v", err) }
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
	if err != nil { t.Fatalf("ListByTask: %v", err) }
	if len(events) != 3 { t.Fatalf("events len = %d, want 3", len(events)) }
	// ListByTask 倒序，最新的（reported）在前
	if events[0].EventType != "reported" { t.Errorf("events[0] = %s, want reported", events[0].EventType) }
}

// TestTaskDependency 验证依赖添加 + 硬依赖未完成时阻挡 claim。
func TestTaskDependency(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil { t.Fatalf("TestDB: %v", err) }
	defer cleanup()
	taskRepo := NewTaskRepo(db)
	depRepo := NewTaskDependencyRepo(db)

	// 准备 A、B 两个 remote 任务，都 pending
	for _, t1 := range []Task{
		{ID: "A", Title: "A", Status: TaskStatusPending, TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now()},
		{ID: "B", Title: "B", Status: TaskStatusPending, TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now()},
	} {
		if err := taskRepo.Create(&t1); err != nil { t.Fatalf("Create: %v", err) }
	}
	// B 依赖 A（hard）
	if err := depRepo.Add(&TaskDependency{TaskID: "B", DependsOn: "A", Type: "hard"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// 循环依赖：再 A 依赖 B，应失败
	if err := depRepo.Add(&TaskDependency{TaskID: "A", DependsOn: "B", Type: "hard"}); err == nil {
		t.Errorf("cycle dep not detected")
	}
	// 自依赖：应失败
	if err := depRepo.Add(&TaskDependency{TaskID: "A", DependsOn: "A", Type: "hard"}); err == nil {
		t.Errorf("self-dep not rejected")
	}
	// B 不能 claim（A 未完成）
	if err := taskRepo.ClaimTask("B", "agent-1"); err == nil {
		t.Errorf("B claimed despite unmet hard dep")
	}
	// A 可以 claim
	if err := taskRepo.ClaimTask("A", "agent-1"); err != nil {
		t.Errorf("A claim failed: %v", err)
	}
	// A 完成后，B 就可以 claim 了
	A, _ := taskRepo.Get("A")
	A.Status = TaskStatusArchived
	// 直接 UPDATE 而不是走 ReportTask（ReportTask 验证 claimer，不在这里重造）
	db.Exec(`UPDATE tasks SET status='archived' WHERE id='A'`)
	if err := taskRepo.ClaimTask("B", "agent-2"); err != nil {
		t.Errorf("B claim after A done failed: %v", err)
	}
	// 列出 B 的依赖
	deps, _ := depRepo.ListByTask("B")
	if len(deps) != 1 || deps[0].DependsOn != "A" {
		t.Errorf("B deps = %+v, want [A]", deps)
	}
	// DeleteByTask 清理
	if err := depRepo.DeleteByTask("A"); err != nil { t.Errorf("DeleteByTask: %v", err) }
	deps, _ = depRepo.ListByTask("B")
	if len(deps) != 0 { t.Errorf("after cleanup deps = %+v, want []", deps) }
}

// TestTaskTemplate 验证任务模板 CRUD + instantiate use_count 自增。
func TestTaskTemplate(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil { t.Fatalf("TestDB: %v", err) }
	defer cleanup()
	repo := NewTaskTemplateRepo(db)
	taskRepo := NewTaskRepo(db)

	tpl := &TaskTemplate{
		Name: "release-checklist", Category: "release", TaskType: "manual",
		TemplateBody: `{"description":"release前的检查清单","resources":"wiki/release"}`,
	}
	if err := repo.Create(tpl); err != nil { t.Fatalf("Create: %v", err) }
	if tpl.ID == "" { t.Errorf("ID auto-gen failed") }

	// 列出
	tpls, _ := repo.List("")
	if len(tpls) != 1 { t.Errorf("List len = %d, want 1", len(tpls)) }

	// 按 category 过滤
	tpls2, _ := repo.List("release")
	if len(tpls2) != 1 { t.Errorf("List(release) len = %d, want 1", len(tpls2)) }
	tpls3, _ := repo.List("dev")
	if len(tpls3) != 0 { t.Errorf("List(dev) len = %d, want 0", len(tpls3)) }

	// Update
	tpl.Description = "updated"
	repo.Update(tpl)
	got, _ := repo.Get(tpl.ID)
	if got.Description != "updated" { t.Errorf("update failed") }

	// Instantiate: 创建任务
	task := &Task{
		ID: "tpl-task-1", Title: "release v1.0",
		Status: TaskStatusPending, Version: "v1", CreatedAt: time.Now(),
	}
	taskRepo.Create(task)
	repo.IncrementUseCount(tpl.ID)
	got, _ = repo.Get(tpl.ID)
	if got.UseCount != 1 { t.Errorf("use_count = %d, want 1", got.UseCount) }

	// Delete
	if err := repo.Delete(tpl.ID); err != nil { t.Errorf("Delete: %v", err) }
	_, err = repo.Get(tpl.ID)
	if err == nil { t.Errorf("template should be deleted") }
}

// TestSavedFilter 验证 saved filter CRUD。
func TestSavedFilter(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil { t.Fatalf("TestDB: %v", err) }
	defer cleanup()
	repo := NewSavedFilterRepo(db)

	f := &SavedFilter{
		Name: "我今天要做的", Description: "pending + 高优",
		FilterJSON: `{"status":"pending","priority":{"$gte":7}}`,
		IsDefault: 1, SortOrder: 0,
	}
	if err := repo.Create(f); err != nil { t.Fatalf("Create: %v", err) }

	list, _ := repo.List()
	if len(list) != 1 { t.Errorf("List len = %d, want 1", len(list)) }

	f.Name = "我今天要做的（改）"
	repo.Update(f)
	got, _ := repo.Get(f.ID)
	if got.Name != "我今天要做的（改）" { t.Errorf("Update failed") }

	repo.Delete(f.ID)
	_, err = repo.Get(f.ID)
	if err == nil { t.Errorf("should be deleted") }
}

// TestWebhook 验证 webhook CRUD + 触发标记。
func TestWebhook(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil { t.Fatalf("TestDB: %v", err) }
	defer cleanup()
	repo := NewWebhookRepo(db)

	w := &Webhook{
		Name: "test-hook", URL: "http://example.com/hook",
		Secret: "test-secret", Events: "task.created,task.claimed",
		Enabled: 1,
	}
	if err := repo.Create(w); err != nil { t.Fatalf("Create: %v", err) }
	if w.ID == "" { t.Errorf("ID auto-gen failed") }

	list, _ := repo.List()
	if len(list) != 1 { t.Errorf("List len = %d, want 1", len(list)) }

	// Update
	w.URL = "http://example.com/v2"
	repo.Update(w)
	got, _ := repo.Get(w.ID)
	if got.URL != "http://example.com/v2" { t.Errorf("update failed") }

	// MarkTriggered
	repo.MarkTriggered(w.ID)
	got, _ = repo.Get(w.ID)
	if got.LastTriggeredAt == nil { t.Errorf("LastTriggeredAt not set") }
	if got.FailCount != 0 { t.Errorf("FailCount = %d, want 0", got.FailCount) }

	// IncrementFail
	repo.IncrementFail(w.ID)
	repo.IncrementFail(w.ID)
	got, _ = repo.Get(w.ID)
	if got.FailCount != 2 { t.Errorf("FailCount = %d, want 2", got.FailCount) }

	// ListEnabled
	enabled, _ := repo.ListEnabled()
	if len(enabled) != 1 { t.Errorf("ListEnabled len = %d, want 1", len(enabled)) }
	w.Enabled = 0
	repo.Update(w)
	enabled, _ = repo.ListEnabled()
	if len(enabled) != 0 { t.Errorf("after disable ListEnabled = %d, want 0", len(enabled)) }

	// Delete
	repo.Delete(w.ID)
	_, err = repo.Get(w.ID)
	if err == nil { t.Errorf("should be deleted") }
}
