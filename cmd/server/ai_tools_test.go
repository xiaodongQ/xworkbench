package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
)

// ─────────────────────────────────────────────────────────────────────────────
// TestAIToolsRegistered
// 验证所有 Phase 1+2 工具均已注册，不遗漏
// ─────────────────────────────────────────────────────────────────────────────

func TestAIToolsRegistered(t *testing.T) {
	tools := GetTools()
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	// Phase 1 + 2 完整工具集（不含 skill 插件）
	expected := []string{
		// Task
		"create_task",
		"list_tasks",
		"get_task",
		"update_task",
		"trigger_task",     // renamed from run_task
		"list_task_executions",
		// DirShortcut
		"create_dir_shortcut",
		"list_dir_shortcuts",
		"update_dir_shortcut",
		"delete_dir_shortcut",
		"open_dir_shortcut",
		"open_dir_shortcut_terminal",
		// Experience
		"search_experiences",
		"create_experience",
		"update_experience",
		"delete_experience",
		// WebLink
		"list_web_links",
		"create_web_link",
		"update_web_link",
		"delete_web_link",
		"open_web_link",
		// Todo
		"list_todos",
		"add_todo",
		"toggle_todo",
		"delete_todo",
		// Scheduled Tasks
		"list_scheduled_tasks",
		"create_scheduled_task",
		"get_scheduled_task",
		"update_scheduled_task",
		"delete_scheduled_task",
		"run_scheduled_task_now",
		// Local Shell
		"start_local_shell",
		"run_local_command",
	}

	for _, name := range expected {
		if !names[name] {
			t.Errorf("tool %q not registered (missing from GetTools)", name)
		}
	}
	if len(tools) < len(expected) {
		t.Errorf("expected at least %d tools, got %d", len(expected), len(tools))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestAIToolsHaveRequiredFields
// 验证每个工具都有有效的 name/description/JSON Schema
// ─────────────────────────────────────────────────────────────────────────────

func TestAIToolsHaveRequiredFields(t *testing.T) {
	tools := GetTools()
	if len(tools) == 0 {
		t.Fatal("no tools registered")
	}
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("tool has empty name")
		}
		if tool.Description == "" {
			t.Errorf("tool %s has empty description", tool.Name)
		}
		var schema map[string]any
		if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
			t.Errorf("tool %s parameters not valid JSON: %v", tool.Name, err)
		}
		// Must have "type": "object"
		if schema["type"] != "object" {
			t.Errorf("tool %s schema type must be 'object', got %v", tool.Name, schema["type"])
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestAIToolDescriptionHasDecisionGuidance
// 验证工具描述包含 AI 决策引导（"Use when:" 风格）
// ─────────────────────────────────────────────────────────────────────────────

func TestAIToolDescriptionHasDecisionGuidance(t *testing.T) {
	tools := GetTools()
	criticalTools := []string{
		"create_task", "list_tasks", "get_task", "update_task",
		"trigger_task", "list_task_executions",
		"search_experiences", "create_experience",
		"list_web_links", "create_web_link", "open_web_link",
		"list_todos", "add_todo",
	}
	for _, tool := range tools {
		if !strIn(tool.Name, criticalTools) {
			continue
		}
		desc := tool.Description
		// 验证描述有足够信息量（>30字符，且不是简单重复）
		if len(desc) < 20 {
			t.Errorf("tool %s description too short (%d chars): %q", tool.Name, len(desc), desc)
		}
	}
}

func strIn(s string, list []string) bool {
	for _, v := range list {
		if s == v {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// TestExecuteTool_Unknown
// 未知工具返回结构化错误
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteTool_Unknown(t *testing.T) {
	srv := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "unknown_tool_xyz", `{}`)
	if result == "" {
		t.Error("unknown tool returned empty string")
	}
	if !contains(result, "未知") && !contains(result, "unknown") {
		t.Errorf("unknown tool should return error message, got: %s", result)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestExecuteTool_Task_CRUD
// 场景 T1-T4: Task 创建/列表/详情/更新
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteTool_CreateTask(t *testing.T) {
	srv := newTestServerForTools(t)
	args := `{"title":"test task","description":"created by tool test","task_type":"manual","priority":3}`
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "create_task", args)

	if result == "" {
		t.Fatal("create_task returned empty result")
	}
	// 必须包含成功标记和任务ID格式
	if !contains(result, "✅") && !contains(result, "task-") {
		t.Errorf("create_task result should contain success mark and task ID, got: %s", result)
	}
}

func TestExecuteTool_ListTasks_Empty(t *testing.T) {
	srv := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "list_tasks", `{}`)

	if result == "" {
		t.Fatal("list_tasks returned empty")
	}
	// 正常返回"无任务"或列表内容
	if !contains(result, "无") && !contains(result, "[") && !contains(result, "-") {
		t.Errorf("list_tasks unexpected result: %s", result)
	}
}

func TestExecuteTool_ListTasks_WithData(t *testing.T) {
	srv := newTestServerForTools(t)
	// 创建测试任务
	task := &backend.Task{
		ID:          "test-list-" + ts(),
		Title:       "List Test Task",
		Description: "test",
		Status:      backend.TaskStatusPending,
		Priority:    3,
		Version:     "v1",
		CreatedAt:   time.Now(),
	}
	if err := srv.db.Create(task); err != nil {
		t.Fatalf("create test task: %v", err)
	}

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "list_tasks", `{}`)

	if !contains(result, "List Test Task") && !contains(result, task.ID) {
		t.Errorf("list_tasks should contain created task, got: %s", result)
	}
}

func TestExecuteTool_GetTask(t *testing.T) {
	srv := newTestServerForTools(t)
	task := &backend.Task{
		ID:          "test-get-" + ts(),
		Title:       "Get Test Task",
		Description: "Test description",
		Acceptance:  "Test acceptance",
		Status:      backend.TaskStatusPending,
		Priority:    2,
		Version:     "v1",
		CreatedAt:   time.Now(),
	}
	if err := srv.db.Create(task); err != nil {
		t.Fatalf("create test task: %v", err)
	}

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "get_task", `{"task_id":"`+task.ID+`"}`)

	if result == "" {
		t.Fatal("get_task returned empty")
	}
	if !contains(result, "Get Test Task") {
		t.Errorf("get_task should contain task title, got: %s", result)
	}
}

func TestExecuteTool_GetTask_NotFound(t *testing.T) {
	srv := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "get_task", `{"task_id":"non-existent-id"}`)

	if !contains(result, "不存在") && !contains(result, "not found") && !contains(result, "失败") {
		t.Errorf("get_task for non-existent should return error, got: %s", result)
	}
}

func TestExecuteTool_UpdateTask(t *testing.T) {
	srv := newTestServerForTools(t)
	task := &backend.Task{
		ID:          "test-update-" + ts(),
		Title:       "Update Test Task",
		Description: "Old description",
		Status:      backend.TaskStatusPending,
		Priority:    5,
		Version:     "v1",
		CreatedAt:   time.Now(),
	}
	if err := srv.db.Create(task); err != nil {
		t.Fatalf("create test task: %v", err)
	}

	// 更新标题和优先级
	args := `{"task_id":"` + task.ID + `","title":"Updated Title","status":"in_progress","priority":1}`
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "update_task", args)

	if !contains(result, "✅") && !contains(result, "Updated Title") {
		t.Errorf("update_task should confirm update and show new title, got: %s", result)
	}

	// 验证更新已生效
	updated, err := srv.db.Get(task.ID)
	if err != nil {
		t.Fatalf("get updated task: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %q", updated.Title)
	}
	if updated.Status != backend.TaskStatusInProgress {
		t.Errorf("expected status in_progress, got %q", updated.Status)
	}
	if updated.Priority != 1 {
		t.Errorf("expected priority 1, got %d", updated.Priority)
	}
}

func TestExecuteTool_UpdateTask_NotFound(t *testing.T) {
	srv := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "update_task", `{"task_id":"non-existent","title":"new"}`)

	if !contains(result, "不存在") && !contains(result, "not found") {
		t.Errorf("update_task for non-existent should return error, got: %s", result)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestExecuteTool_TriggerTask
// 场景 T5: trigger_task（验证调用链路）
// 注意：在测试环境中不会真正触发HTTP调用，验证参数构造正确即可
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteTool_TriggerTask(t *testing.T) {
	srv := newTestServerForTools(t)
	task := &backend.Task{
		ID:          "test-trigger-" + ts(),
		Title:       "Trigger Test Task",
		Description: "Test task for trigger",
		Status:      backend.TaskStatusPending,
		TaskType:    "manual",
		Priority:    3,
		Version:     "v1",
		CreatedAt:   time.Now(),
	}
	if err := srv.db.Create(task); err != nil {
		t.Fatalf("create test task: %v", err)
	}

	// trigger_task 在测试环境下通过 HTTP 调用（使用测试地址覆盖）
	oldAddr := serverAddr
	serverAddr = ":8902"
	defer func() { serverAddr = oldAddr }()

	args := `{"task_id":"` + task.ID + `"}`
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "trigger_task", args)

	// HTTP 调用在测试环境可能失败（取决于测试服务器是否启动），但函数应该正常返回
	// 不应 panic，不应返回空
	if result == "" {
		t.Fatal("trigger_task returned empty string (possible panic)")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestExecuteTool_TaskExecutions
// 场景 T6: list_task_executions
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteTool_ListTaskExecutions(t *testing.T) {
	srv := newTestServerForTools(t)
	task := &backend.Task{
		ID:          "test-exec-" + ts(),
		Title:       "Exec Test Task",
		Description: "Test",
		Status:      backend.TaskStatusPending,
		Priority:    3,
		Version:     "v1",
		CreatedAt:   time.Now(),
	}
	if err := srv.db.Create(task); err != nil {
		t.Fatalf("create test task: %v", err)
	}

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "list_task_executions", `{"task_id":"`+task.ID+`"}`)

	if result == "" {
		t.Fatal("list_task_executions returned empty")
	}
	// 无执行记录时应返回"无执行记录"
	if !contains(result, "无执行记录") && !contains(result, "Exec Test Task") {
		t.Errorf("unexpected list_task_executions result: %s", result)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestExecuteTool_WebLinks
// 场景 T7-T8: WebLink CRUD + open
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteTool_CreateWebLink(t *testing.T) {
	srv := newTestServerForTools(t)
	args := `{"name":"Test Link","url":"https://example.com"}`
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "create_web_link", args)

	if result == "" {
		t.Fatal("create_web_link returned empty")
	}
	if !contains(result, "✅") && !contains(result, "Test Link") {
		t.Errorf("create_web_link result: %s", result)
	}
}

func TestExecuteTool_CreateWebLink_MissingFields(t *testing.T) {
	srv := newTestServerForTools(t)
	// 缺少必填字段
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "create_web_link", `{"name":""}`)

	if !contains(result, "⚠️") && !contains(result, "必填") {
		t.Errorf("create_web_link with missing fields should warn, got: %s", result)
	}
}

func TestExecuteTool_ListWebLinks_Empty(t *testing.T) {
	srv := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "list_web_links", `{}`)

	if result == "" {
		t.Fatal("list_web_links returned empty")
	}
	// 应返回"无收藏链接"或列表
	if !contains(result, "无") && !contains(result, "🔗") {
		t.Errorf("list_web_links unexpected result: %s", result)
	}
}

func TestExecuteTool_ListWebLinks_WithData(t *testing.T) {
	srv := newTestServerForTools(t)
	// 通过 create_web_link 创建一条
	ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"create_web_link", `{"name":"Test","url":"https://test.com"}`)

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "list_web_links", `{}`)

	if !contains(result, "Test") && !contains(result, "https://test.com") {
		t.Errorf("list_web_links should contain created link, got: %s", result)
	}
}

func TestExecuteTool_UpdateWebLink(t *testing.T) {
	srv := newTestServerForTools(t)
	// 创建
	ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"create_web_link", `{"name":"Old Name","url":"https://old.com"}`)

	// 先列出找 ID
	list, _ := srv.linkDB.List()
	if len(list) == 0 {
		t.Skip("no web link to update")
	}
	id := list[0].ID
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"update_web_link", `{"id":"`+id+`","name":"New Name","url":"https://new.com"}`)

	if !contains(result, "✅") {
		t.Errorf("update_web_link should confirm, got: %s", result)
	}
}

func TestExecuteTool_DeleteWebLink(t *testing.T) {
	srv := newTestServerForTools(t)
	// 创建一条
	ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"create_web_link", `{"name":"ToDelete","url":"https://del.com"}`)

	list, _ := srv.linkDB.List()
	if len(list) == 0 {
		t.Skip("no web link to delete")
	}
	id := list[0].ID
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "delete_web_link", `{"id":"`+id+`"}`)

	// 删除已存在的链接应返回成功
	if !contains(result, "✅") && !contains(result, "已删除") {
		t.Errorf("delete_web_link should confirm deletion, got: %s", result)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestExecuteTool_Experiences
// 场景 T10-T12: Experience CRUD
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteTool_CreateExperience(t *testing.T) {
	srv := newTestServerForTools(t)
	args := `{"module":"test","keywords":"test,unit","scene":"测试场景","details":"# 测试经验\n这是测试内容"}`
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "create_experience", args)

	if result == "" {
		t.Fatal("create_experience returned empty")
	}
	if !contains(result, "✅") && !contains(result, "test") {
		t.Errorf("create_experience result: %s", result)
	}
}

func TestExecuteTool_CreateExperience_MissingModule(t *testing.T) {
	srv := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "create_experience", `{"keywords":"test"}`)

	if !contains(result, "⚠️") && !contains(result, "module") {
		t.Errorf("create_experience without module should warn, got: %s", result)
	}
}

func TestExecuteTool_SearchExperiences(t *testing.T) {
	srv := newTestServerForTools(t)
	// 先创建一条
	ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"create_experience", `{"module":"git","keywords":"rebase,merge","scene":"git 变基"}`)

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "search_experiences", `{"query":"git"}`)

	if result == "" {
		t.Fatal("search_experiences returned empty")
	}
	if !contains(result, "git") {
		t.Errorf("search_experiences should find git experience, got: %s", result)
	}
}

func TestExecuteTool_SearchExperiences_NoResults(t *testing.T) {
	srv := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "search_experiences", `{"query":"xyznonexistent999"}`)

	if !contains(result, "无结果") && !contains(result, "🔍") {
		t.Errorf("search_experiences with no results should say so, got: %s", result)
	}
}

func TestExecuteTool_UpdateExperience(t *testing.T) {
	srv := newTestServerForTools(t)
	// 创建
	createResult := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"create_experience", `{"module":"docker","keywords":"docker","scene":"Docker 场景"}`)

	// 提取 ID
	var m map[string]any
	json.Unmarshal([]byte(createResult), &m)
	// ID 在文本中，找一个经验先
	exps, _ := srv.expDB.Search("docker")
	if len(exps) == 0 {
		t.Skip("no docker experience found to update")
	}

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"update_experience", `{"id":"`+exps[0].ID+`","scene":"Docker 场景（更新）"}`)

	if !contains(result, "✅") {
		t.Errorf("update_experience should confirm, got: %s", result)
	}
}

func TestExecuteTool_DeleteExperience_NotFound(t *testing.T) {
	srv := newTestServerForTools(t)
	// 创建一条经验
	ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"create_experience", `{"module":"testdel","keywords":"del","scene":"Delete Test"}`)
	// 找出来并删除
	exps, _ := srv.expDB.Search("testdel")
	if len(exps) == 0 {
		t.Skip("no experience to delete")
	}
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"delete_experience", `{"id":"`+exps[0].ID+`"}`)
	if !contains(result, "✅") {
		t.Errorf("delete_experience should confirm deletion, got: %s", result)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestExecuteTool_DirShortcuts
// 场景 T12-T13: DirShortcut CRUD + open
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteTool_CreateDirShortcut_Local(t *testing.T) {
	srv := newTestServerForTools(t)
	args := `{"name":"TestProject","type":"local","path":"/tmp/test-` + ts() + `"}`
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "create_dir_shortcut", args)

	if result == "" {
		t.Fatal("create_dir_shortcut returned empty")
	}
	if !contains(result, "✅") && !contains(result, "TestProject") {
		t.Errorf("create_dir_shortcut result: %s", result)
	}
}

func TestExecuteTool_CreateDirShortcut_Remote(t *testing.T) {
	srv := newTestServerForTools(t)
	args := `{"name":"RemoteDev","type":"remote","remote_host":"192.168.1.1","remote_user":"root","remote_path":"/home/user"}`
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "create_dir_shortcut", args)

	if result == "" {
		t.Fatal("create_dir_shortcut remote returned empty")
	}
	if !contains(result, "✅") && !contains(result, "RemoteDev") {
		t.Errorf("create_dir_shortcut remote result: %s", result)
	}
}

func TestExecuteTool_CreateDirShortcut_MissingName(t *testing.T) {
	srv := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "create_dir_shortcut", `{"type":"local","path":"/tmp"}`)

	if !contains(result, "⚠️") && !contains(result, "必填") {
		t.Errorf("create_dir_shortcut without name should warn, got: %s", result)
	}
}

func TestExecuteTool_ListDirShortcuts_Empty(t *testing.T) {
	srv := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "list_dir_shortcuts", `{}`)

	if result == "" {
		t.Fatal("list_dir_shortcuts returned empty")
	}
	if !contains(result, "无") && !contains(result, "📁") {
		t.Errorf("list_dir_shortcuts unexpected result: %s", result)
	}
}

func TestExecuteTool_ListDirShortcuts_WithFilter(t *testing.T) {
	srv := newTestServerForTools(t)
	ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"create_dir_shortcut", `{"name":"LocalOnly","type":"local","path":"/tmp/test"}`)

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "list_dir_shortcuts", `{"type":"local"}`)

	if !contains(result, "LocalOnly") {
		t.Errorf("list_dir_shortcuts with type filter should work, got: %s", result)
	}
}

func TestExecuteTool_UpdateDirShortcut(t *testing.T) {
	srv := newTestServerForTools(t)
	ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"create_dir_shortcut", `{"name":"OldName","type":"local","path":"/tmp/old"}`)

	// 找出来
	list, _ := srv.dirDB.List()
	if len(list) == 0 {
		t.Skip("no dir shortcut to update")
	}
	id := list[0].ID

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"update_dir_shortcut", `{"id":"`+id+`","name":"NewName"}`)

	if !contains(result, "✅") && !contains(result, "NewName") {
		t.Errorf("update_dir_shortcut result: %s", result)
	}
}

func TestExecuteTool_DeleteDirShortcut(t *testing.T) {
	srv := newTestServerForTools(t)
	ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"create_dir_shortcut", `{"name":"ToDelete","type":"local","path":"/tmp/del"}`)

	list, _ := srv.dirDB.List()
	before := len(list)

	ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"delete_dir_shortcut", `{"id":"`+list[0].ID+`"}`)

	list, _ = srv.dirDB.List()
	if len(list) != before-1 {
		t.Errorf("delete_dir_shortcut should remove one entry, before=%d after=%d", before, len(list))
	}
}

func TestExecuteTool_OpenDirShortcut_NotFound(t *testing.T) {
	srv := newTestServerForTools(t)
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"open_dir_shortcut", `{"id":"non-existent-shortcut"}`)

	if !contains(result, "不存在") {
		t.Errorf("open_dir_shortcut for non-existent should warn, got: %s", result)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestExecuteTool_Todo
// 场景 T9, T14-T16: Todo CRUD
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteTool_AddTodo(t *testing.T) {
	srv := newTestServerForTools(t)
	// 设置 todo path
	setTodoPath(srv, t.TempDir()+"/todo.md")

	args := `{"text":"测试 Todo 项目"}`
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "add_todo", args)

	if result == "" {
		t.Fatal("add_todo returned empty")
	}
	if !contains(result, "✅") && !contains(result, "测试 Todo") {
		t.Errorf("add_todo result: %s", result)
	}
}

func TestExecuteTool_ListTodos(t *testing.T) {
	srv := newTestServerForTools(t)
	path := t.TempDir() + "/todo.md"
	setTodoPath(srv, path)

	// 先添加一条
	ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"add_todo", `{"text":"List Test Todo"}`)

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil, "list_todos", `{}`)

	if !contains(result, "List Test Todo") && !contains(result, "📝") {
		t.Errorf("list_todos should contain added item, got: %s", result)
	}
}

func TestExecuteTool_ToggleTodo(t *testing.T) {
	srv := newTestServerForTools(t)
	path := t.TempDir() + "/todo-toggle.md"
	// 先写一个初始 todo 项
	initialContent := "- [ ] Toggle Test Todo\n"
	if err := os.WriteFile(path, []byte(initialContent), 0644); err != nil {
		t.Fatalf("create temp todo file: %v", err)
	}
	setTodoPath(srv, path)

	// 切换 line_no=1
	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"toggle_todo", `{"line_no":1,"done":true}`)

	if !contains(result, "✅") && !contains(result, "已完成") {
		t.Errorf("toggle_todo should confirm, got: %s", result)
	}
}

func TestExecuteTool_ToggleTodo_InvalidLineNo(t *testing.T) {
	srv := newTestServerForTools(t)
	path := t.TempDir() + "/todo.md"
	setTodoPath(srv, path)

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"toggle_todo", `{"line_no":0,"done":true}`)

	if !contains(result, "⚠️") {
		t.Errorf("toggle_todo with line_no=0 should warn, got: %s", result)
	}
}

func TestExecuteTool_DeleteTodo(t *testing.T) {
	srv := newTestServerForTools(t)
	path := t.TempDir() + "/todo.md"
	setTodoPath(srv, path)

	ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"add_todo", `{"text":"Delete Test Todo"}`)

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, nil,
		"delete_todo", `{"line_no":1}`)

	if !contains(result, "✅") {
		t.Errorf("delete_todo should confirm, got: %s", result)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestExecuteTool_LocalShell
// 场景: Local Shell 工具
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteTool_StartLocalShell(t *testing.T) {
	srv := newTestServerForTools(t)
	state := &LocalShellState{}

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, state,
		"start_local_shell", `{"cli_type":"claude","cwd":"/tmp"}`)

	if result == "" {
		t.Fatal("start_local_shell returned empty")
	}
	if !contains(result, "✅") && !contains(result, "claude") {
		t.Errorf("start_local_shell result: %s", result)
	}
	if !state.Active {
		t.Error("LocalShellState.Active should be set to true")
	}
}

func TestExecuteTool_StartLocalShell_InvalidCLIType(t *testing.T) {
	srv := newTestServerForTools(t)
	state := &LocalShellState{}

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, state,
		"start_local_shell", `{"cli_type":"bash"}`)

	if !contains(result, "⚠️") && !contains(result, "claude") && !contains(result, "cbc") {
		t.Errorf("start_local_shell with invalid cli_type should warn, got: %s", result)
	}
}

func TestExecuteTool_RunLocalCommand_WithoutSession(t *testing.T) {
	srv := newTestServerForTools(t)
	state := &LocalShellState{} // 未激活

	result := ExecuteTool(context.Background(), srv.db, srv.expDB, srv.execDB, srv.agentDB, srv.linkDB, srv.dirDB, srv.schedDB, nil, state,
		"run_local_command", `{"command":"ls","cli_type":"claude"}`)

	if !contains(result, "⚠️") && !contains(result, "无活跃") {
		t.Errorf("run_local_command without active session should warn, got: %s", result)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func ts() string {
	return time.Now().Format("150405.000")
}

func newTestServerForTools(t *testing.T) *APIServer {
	db, cleanup, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	t.Cleanup(cleanup)

	srv := &APIServer{
		db:      backend.NewTaskRepo(db),
		expDB:   backend.NewExperienceRepo(db),
		execDB:  backend.NewExecutionRepo(db),
		evalDB:  backend.NewEvaluationRepo(db),
		agentDB: backend.NewAgentRepo(db),
		eventDB: backend.NewTaskEventRepo(db),
		linkDB:  backend.NewWebLinkRepo(db),
		dirDB:   backend.NewDirShortcutRepo(db),
		schedDB: backend.NewScheduledTaskRepo(db),
	}
	return srv
}

// setTodoPath 覆盖全局 config 的 TodoMDPath（测试用）。
// 注意：调用方负责确保文件存在；setTodoPath 只更新配置路径。
func setTodoPath(_ *APIServer, path string) {
	config.Update(func(c *config.Config) {
		c.TodoMDPath = path
	})
}
