package backend

import (
	"testing"
	"time"
)

// TestDeriveStatus 验证 status 派生逻辑：
//
//	exit_code = 0                  → "success"
//	exit_code != 0 && != -1        → "failed"（子进程报错）
//	exit_code = -1 && 含 "cancel"  → "cancelled"
//	exit_code = -1 && 含 "deadline"→ "timeout"
//	exit_code = -1 && 其他         → "failed"（兜底）
func TestDeriveStatus(t *testing.T) {
	cases := []struct {
		name     string
		exitCode int
		errOut   string
		want     string
	}{
		{"exit_0_success", 0, "", "success"},
		{"exit_1_failed", 1, "some error", "failed"},
		{"exit_2_failed", 2, "", "failed"},
		{"exit_-1_cancelled", -1, "manually cancelled (force)", "cancelled"},
		{"exit_-1_timeout", -1, "executor: context deadline exceeded", "timeout"},
		{"exit_-1_signal_killed", -1, "executor: signal: killed", "cancelled"},
		{"exit_-1_unknown", -1, "random weird error", "failed"},
		{"exit_-1_deadline_uppercase", -1, "DEADLINE EXCEEDED", "timeout"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DeriveStatus(c.exitCode, c.errOut)
			if got != c.want {
				t.Errorf("DeriveStatus(%d, %q) = %q, want %q", c.exitCode, c.errOut, got, c.want)
			}
		})
	}
}

// TestExecutionRepo_Finish_WritesStatus 验证 Finish 写入正确的 status 字段。
func TestExecutionRepo_Finish_WritesStatus(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	// 跑 InitSchema 创表
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	repo := NewExecutionRepo(db)

	// 准备一个 execution 行（Create 时 status='running'）
	exec := &Execution{
		ID:        "exec-finish-1",
		TaskID:    "",
		Source:    "manual",
		Command:   "echo hello",
		StartedAt: time.Now(),
	}
	if err := repo.Create(exec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 调 Finish（exit_code=0, 无 error）
	if err := repo.Finish(exec.ID, "hello\n", "", 0, ""); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	got, err := repo.Get(exec.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != "success" {
		t.Errorf("after Finish(exit=0), status=%q, want %q", got.Status, "success")
	}
	if got.CompletedAt == nil {
		t.Errorf("CompletedAt should be set after Finish")
	}

	// 第二次：exit_code=-1, errOut 含 "deadline" → status=timeout
	exec2 := &Execution{ID: "exec-finish-2", Source: "manual", Command: "x", StartedAt: time.Now()}
	if err := repo.Create(exec2); err != nil {
		t.Fatalf("Create2: %v", err)
	}
	if err := repo.Finish(exec2.ID, "", "context deadline exceeded", -1, ""); err != nil {
		t.Fatalf("Finish2: %v", err)
	}
	got2, _ := repo.Get(exec2.ID)
	if got2.Status != "timeout" {
		t.Errorf("after Finish(exit=-1, errOut=context deadline), status=%q, want %q", got2.Status, "timeout")
	}
}

// TestExecutionRepo_ForceFinish 验证 ForceFinish：把僵尸 execution 标完成 + status=cancelled。
// 同时验证不存在的 id 调用不会报错（SQL UPDATE 0 row）。
func TestExecutionRepo_ForceFinish(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	repo := NewExecutionRepo(db)

	exec := &Execution{ID: "exec-force-1", Source: "manual", Command: "x", StartedAt: time.Now()}
	if err := repo.Create(exec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// 此时 completed_at=NULL, status=running
	now := time.Now()
	if err := repo.ForceFinish(exec.ID, now, "manually cancelled (force, test)"); err != nil {
		t.Fatalf("ForceFinish: %v", err)
	}
	got, _ := repo.Get(exec.ID)
	if got.CompletedAt == nil {
		t.Errorf("after ForceFinish, CompletedAt should be set")
	}
	if got.Status != "cancelled" {
		t.Errorf("after ForceFinish, status=%q, want %q", got.Status, "cancelled")
	}
	if got.ExitCode != -1 {
		t.Errorf("after ForceFinish, exit_code=%d, want %d", got.ExitCode, -1)
	}

	// 重复调 ForceFinish（completed_at 已有值），不应改写
	// SQL: WHERE id=? AND completed_at IS NULL，所以不匹配、无副作用
	if err := repo.ForceFinish(exec.ID, time.Now(), "second call"); err != nil {
		t.Fatalf("ForceFinish (idempotent): %v", err)
	}
	got2, _ := repo.Get(exec.ID)
	if got2.CompletedAt == nil || !got2.CompletedAt.Equal(*got.CompletedAt) {
		t.Errorf("after second ForceFinish, CompletedAt should not change")
	}
}

// TestExecutionRepo_MigrateOldData 验证 InitSchema 后老 execution 数据被正确迁移。
// 模拟"老库"（status='success' 默认 + 不合理 exit_code），跑迁移后派生正确 status。
func TestExecutionRepo_MigrateOldData(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()

	// TestDB 已经跑过 InitSchema（包含 status 列），先把 executions 表彻底 drop，
	// 然后用"老库"schema 重建，模拟"应用启动时表是老的、没 status 列"的状态。
	if _, err := db.Exec(`DROP TABLE IF EXISTS executions`); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE executions (
		id TEXT PRIMARY KEY, task_id TEXT, scheduled_task_id TEXT, source TEXT NOT NULL,
		command TEXT NOT NULL, prompt TEXT, model TEXT, started_at DATETIME, completed_at DATETIME,
		output TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '',
		exit_code INTEGER NOT NULL DEFAULT 0, resume_uuid TEXT
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// 插 4 条老数据
	type seed struct {
		id       string
		started  time.Time
		ended    *time.Time
		exitCode int
		errOut   string
	}
	now := time.Now()
	seeds := []seed{
		{"old-success", now.Add(-4 * time.Hour), ptrTime(now.Add(-4 * time.Hour).Add(time.Second)), 0, ""},
		{"old-failed", now.Add(-3 * time.Hour), ptrTime(now.Add(-3 * time.Hour).Add(time.Second)), 1, "exit 1"},
		{"old-timeout", now.Add(-2 * time.Hour), ptrTime(now.Add(-2 * time.Hour).Add(time.Second)), -1, "context deadline exceeded"},
		{"old-running", now.Add(-1 * time.Hour), nil, 0, ""}, // 还在跑
	}
	for _, s := range seeds {
		_, err := db.Exec(`INSERT INTO executions (id,source,command,started_at,completed_at,exit_code,error) VALUES (?,?,?,?,?,?,?)`,
			s.id, "manual", "x", s.started, s.ended, s.exitCode, s.errOut)
		if err != nil {
			t.Fatalf("insert %s: %v", s.id, err)
		}
	}

	// 跑 InitSchema → 触发 ALTER ADD status + 老数据迁移
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	// 验证
	check := []struct {
		id, want string
	}{
		{"old-success", "success"},
		{"old-failed", "failed"},
		{"old-timeout", "timeout"},
		{"old-running", "running"},
	}
	for _, c := range check {
		var status string
		if err := db.QueryRow(`SELECT status FROM executions WHERE id=?`, c.id).Scan(&status); err != nil {
			t.Errorf("query %s: %v", c.id, err)
			continue
		}
		if status != c.want {
			t.Errorf("after migration, %s status=%q, want %q", c.id, status, c.want)
		}
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
