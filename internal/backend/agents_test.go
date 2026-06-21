package backend

import (
	"testing"
	"time"
)

// helper：建测试 agent
func newTestAgent(t *testing.T, name string) *Agent {
	t.Helper()
	a := &Agent{
		ID:               "test-agent-" + name,
		Name:             name,
		TokenHash:        HashToken("plain-token-" + name),
		Capabilities:     "remote-task",
		Version:          "test-v0.1",
		Status:           "online",
		AutoClaimEnabled: false,
		CreatedAt:        time.Now(),
	}
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	t.Cleanup(cleanup)
	if err := NewAgentRepo(db).Register(a); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return a
}

func TestAgent_RegisterAndGetByID(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	repo := NewAgentRepo(db)

	a := &Agent{
		ID: "agent-r1", Name: "reg1", TokenHash: HashToken("tok1"),
		Status: "online", CreatedAt: time.Now(),
	}
	if err := repo.Register(a); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := repo.GetByID("agent-r1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "reg1" || got.Status != "online" {
		t.Errorf("GetByID mismatch: %+v", got)
	}
}

func TestAgent_GetByToken(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	repo := NewAgentRepo(db)

	const plainToken = "secret-token-xyz"
	a := &Agent{
		ID: "agent-tok1", Name: "tok1", TokenHash: HashToken(plainToken),
		Status: "online", CreatedAt: time.Now(),
	}
	if err := repo.Register(a); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := repo.GetByToken(plainToken)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got.ID != "agent-tok1" {
		t.Errorf("GetByToken ID mismatch: %s", got.ID)
	}

	// 错误 token 应失败
	if _, err := repo.GetByToken("wrong-token"); err == nil {
		t.Error("GetByToken with wrong token should fail")
	}
}

func TestAgent_List_AllAndFiltered(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	repo := NewAgentRepo(db)

	// 插 3 个 agent：2 online + 1 offline
	for _, a := range []*Agent{
		{ID: "a1", Name: "a1", TokenHash: "h1", Status: "online", CreatedAt: time.Now()},
		{ID: "a2", Name: "a2", TokenHash: "h2", Status: "online", CreatedAt: time.Now().Add(1 * time.Second)},
		{ID: "a3", Name: "a3", TokenHash: "h3", Status: "offline", CreatedAt: time.Now().Add(2 * time.Second)},
	} {
		if err := repo.Register(a); err != nil {
			t.Fatalf("Register %s: %v", a.ID, err)
		}
	}

	all, err := repo.List("")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List all should return 3, got %d", len(all))
	}

	online, err := repo.List("online")
	if err != nil {
		t.Fatalf("List online: %v", err)
	}
	if len(online) != 2 {
		t.Errorf("List online should return 2, got %d", len(online))
	}
	for _, a := range online {
		if a.Status != "online" {
			t.Errorf("filtered list contains non-online: %+v", a)
		}
	}
}

func TestAgent_ResetToken(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	repo := NewAgentRepo(db)

	a := &Agent{ID: "agent-rst", Name: "rst", TokenHash: HashToken("old"), Status: "online", CreatedAt: time.Now()}
	if err := repo.Register(a); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 旧 token 能查到
	if _, err := repo.GetByToken("old"); err != nil {
		t.Fatalf("old token should work before reset: %v", err)
	}

	// reset
	newToken, err := repo.ResetToken("agent-rst")
	if err != nil {
		t.Fatalf("ResetToken: %v", err)
	}
	if newToken == "" || newToken == "old" {
		t.Errorf("new token should be different non-empty string, got %q", newToken)
	}

	// 旧 token 立即失效
	if _, err := repo.GetByToken("old"); err == nil {
		t.Error("old token should be invalid after reset")
	}
	// 新 token 能查到
	got, err := repo.GetByToken(newToken)
	if err != nil {
		t.Fatalf("new token should work: %v", err)
	}
	if got.ID != "agent-rst" {
		t.Errorf("new token ID mismatch: %s", got.ID)
	}
}

func TestAgent_Delete(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	repo := NewAgentRepo(db)

	a := &Agent{ID: "agent-del", Name: "del", TokenHash: "h", Status: "online", CreatedAt: time.Now()}
	if err := repo.Register(a); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := repo.Delete("agent-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID("agent-del"); err == nil {
		t.Error("GetByID after Delete should fail")
	}
}

func TestAgent_CountInProgressByAgent(t *testing.T) {
	db, cleanup, err := TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	agentRepo := NewAgentRepo(db)
	taskRepo := NewTaskRepo(db)

	agentID := "agent-cnt"
	if err := agentRepo.Register(&Agent{ID: agentID, Name: "cnt", TokenHash: "h", Status: "online", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 插 3 个 remote task，用真实 ClaimTask 走 claim 流程（保证 claimer_agent_id 写进去）
	for i := 1; i <= 3; i++ {
		task := &Task{
			ID: "task-cnt-" + string(rune('0'+i)),
			Title: "t", Status: TaskStatusPending,
			TaskType: TaskTypeRemote, Version: "v1", CreatedAt: time.Now(),
		}
		if err := taskRepo.Create(task); err != nil {
			t.Fatalf("Create task %d: %v", i, err)
		}
		// 前 2 个让该 agent claim，第 3 个保持 pending
		if i <= 2 {
			if err := taskRepo.ClaimTask(task.ID, agentID); err != nil {
				t.Fatalf("ClaimTask %d: %v", i, err)
			}
		}
	}

	n, err := taskRepo.CountInProgressByAgent(agentID)
	if err != nil {
		t.Fatalf("CountInProgressByAgent: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 in_progress, got %d", n)
	}
}
