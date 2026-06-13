package backend

import (
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