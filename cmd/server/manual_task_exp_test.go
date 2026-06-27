package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/hub"
	"github.com/xiaodongQ/xworkbench/internal/paths"
	taskpkg "github.com/xiaodongQ/xworkbench/internal/task"
	"go.uber.org/zap"
)

func newManualTaskTestServer(t *testing.T) *APIServer {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	if logger == nil {
		z, _ := zap.NewProduction()
		logger = z.Sugar()
	}
	return &APIServer{
		db:      backend.NewTaskRepo(db),
		expDB:   backend.NewExperienceRepo(db),
		execDB:  backend.NewExecutionRepo(db),
		evalDB:  backend.NewEvaluationRepo(db),
		agentDB: backend.NewAgentRepo(db),
		eventDB: backend.NewTaskEventRepo(db),
		hub:     hub.New(),
		running: map[string]context.CancelFunc{},
	}
}

func seedExperienceB(t *testing.T, s *APIServer, module, scene, keywords, details string) string {
	t.Helper()
	exp := &backend.Experience{
		ID: "exp-b-" + module + "-" + scene, Module: module, Scene: scene,
		Keywords: keywords, Details: details, Version: "v1", CreatedAt: time.Now(),
	}
	if err := s.expDB.Create(exp); err != nil {
		t.Fatalf("create exp: %v", err)
	}
	return exp.ID
}

// TestCreateTask_MultipleExperienceIDs 验证 POST /api/tasks 用 experience_ids（数组）
// 能正确创建多经验关联。当前前端用旧 experience_id 单值字段是 bug。
func TestCreateTask_MultipleExperienceIDs(t *testing.T) {
	s := newManualTaskTestServer(t)

	// 准备 3 条经验
	expIDs := []string{
		seedExperienceB(t, s, "git", "merge", "kw1", "exp1 details"),
		seedExperienceB(t, s, "docker", "build", "kw2", "exp2 details"),
		seedExperienceB(t, s, "k8s", "deploy", "kw3", "exp3 details"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks", s.handleTaskCreate)
	mux.HandleFunc("GET /api/tasks/{id}", s.handleTaskGet) // 暂用 List 查

	body, _ := json.Marshal(map[string]any{
		"title":          "多经验任务",
		"description":    "做 ABC",
		"experience_ids": expIDs, // ✅ 新字段：数组
		"acceptance":     "结果正确",
	})
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create: status %d, body %s", w.Code, w.Body.String())
	}
	var task backend.Task
	if err := json.Unmarshal(w.Body.Bytes(), &task); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// 验证所有 3 条经验都关联上了
	if len(task.ExperienceIDs) != 3 {
		t.Errorf("expected 3 experience_ids, got %d (%v)", len(task.ExperienceIDs), task.ExperienceIDs)
	}
}

// TestCreateTask_LegacyCommaStringExperienceID 旧前端用 "id1,id2" 字符串提交。
// 当前后端把它当单值 → 只有第一条生效。这是已知 bug。
// 期望行为：要么兼容解析（拆成数组），要么前端修（用数组）。
// 这里我们记录当前 bug 行为：旧字符串被当单 ID 处理，导致关联不全。
// 当前端修好后此测试应替换为验证正常路径。
func TestCreateTask_LegacyCommaStringExperienceID_RecordsBuggyBehavior(t *testing.T) {
	s := newManualTaskTestServer(t)

	expIDs := []string{
		seedExperienceB(t, s, "git", "merge", "kw1", "exp1 details"),
		seedExperienceB(t, s, "docker", "build", "kw2", "exp2 details"),
	}
	commaStr := strings.Join(expIDs, ",")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tasks", s.handleTaskCreate)
	body, _ := json.Marshal(map[string]any{
		"title":         "旧字段任务",
		"description":   "做 ABC",
		"experience_id": commaStr, // 🐛 旧前端用逗号字符串
	})
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var task backend.Task
	if err := json.Unmarshal(w.Body.Bytes(), &task); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// BUG 行为：逗号字符串被当单 ID，AttachExperiences 找不到会 warn 跳过
	// 这里只关心 task.ExperienceIDs 数量 — 0 或 1 都说明 bug
	t.Logf("legacy comma string produces ExperienceIDs: %v (len=%d)", task.ExperienceIDs, len(task.ExperienceIDs))
	// 文档化现状，不强制 pass
	if len(task.ExperienceIDs) > 1 {
		t.Logf("UNEXPECTED: comma string was correctly split into multiple IDs")
	}
}

// TestTaskRunPrompt_IncludesExperiences 验证手动任务执行时 prompt 应含经验库内容。
// 修复（2026-06）：handleTaskRun 改用 taskpkg.BuildTaskPromptWithOutput（含经验 +
// 输出目录约定），不再用 BuildTaskPromptShort（不含经验，已删除）。
//
// 这里直接调真实的 taskpkg.BuildTaskPromptWithOutput，验证 prompt 含经验库内容。
func TestTaskRunPrompt_IncludesExperiences(t *testing.T) {
	s := newManualTaskTestServer(t)
	expID := seedExperienceB(t, s, "git", "merge-conflict", "rebase",
		"用 git rebase --continue 解决冲突后 commit")

	taskID := "manual-task-1"
	task := &backend.Task{
		ID: taskID, Title: "解决冲突", Description: "main 分支冲突",
		Acceptance: "冲突解决", Status: backend.TaskStatusPending,
		TaskType: backend.TaskTypeManual, Priority: 5, Version: "v1",
		CreatedAt: time.Now(),
	}
	if err := s.db.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := s.db.AttachExperiences(taskID, []string{expID}); err != nil {
		t.Fatalf("attach: %v", err)
	}

	// 模拟 handleTaskRun 内部构造 prompt 的逻辑
	got, _ := s.db.Get(taskID)
	exps := s.loadExperiencesForTask(got)
	prompt := taskpkg.BuildTaskPromptWithOutput(got, paths.AITaskDir(taskID), exps...)

	// 验证含经验库内容
	for _, must := range []string{"git", "merge-conflict", "rebase", "用 git rebase"} {
		if !strings.Contains(prompt, must) {
			t.Errorf("prompt missing experience content %q\n---prompt---\n%s", must, prompt)
		}
	}

	// 验证含输出目录约定（2026-06 新增）
	if !strings.Contains(prompt, "## 输出目录约定") {
		t.Errorf("prompt missing output dir hint\n---prompt---\n%s", prompt)
	}
	if !strings.Contains(prompt, taskID) {
		t.Errorf("prompt missing taskID in output dir path\n---prompt---\n%s", prompt)
	}
}
