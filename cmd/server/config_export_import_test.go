package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"go.uber.org/zap"
)

// newConfigExportTestServer 构造带全 repo 的 APIServer，覆盖导出/导入链路所需的全部字段。
func newConfigExportTestServer(t *testing.T) *APIServer {
	t.Helper()
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
		dirDB:   backend.NewDirShortcutRepo(db),
		linkDB:  backend.NewWebLinkRepo(db),
		schedDB: backend.NewScheduledTaskRepo(db),
		running: map[string]context.CancelFunc{},
	}
}

func newConfigExportMux(s *APIServer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config/export", s.handleConfigExport)
	mux.HandleFunc("POST /api/config/import/preview", s.handleConfigImportPreview)
	mux.HandleFunc("POST /api/config/import", s.handleConfigImport)
	return mux
}

func doRequest(t *testing.T, mux http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// =========================
// Round-trip：导出 → 拿到 JSON → 清空 → 导入 → 比对
// =========================

func TestExportImport_RoundTrip_DirShortcuts(t *testing.T) {
	s := newConfigExportTestServer(t)
	mux := newConfigExportMux(s)

	// seed：2 条快捷目录（含 remote + password 验证不丢失）
	d1 := &backend.DirShortcut{ID: "d-1", Name: "本地-1", Path: "/tmp/a", Type: "local"}
	d2 := &backend.DirShortcut{ID: "d-2", Name: "远端-1", Path: "/home/x", Type: "remote",
		RemoteHost: "10.0.0.1", RemoteUser: "u", RemotePassword: "secret",
		AuthMethod: "password", TerminalCmd: "ssh"}
	if err := s.dirDB.Create(d1); err != nil {
		t.Fatal(err)
	}
	if err := s.dirDB.Create(d2); err != nil {
		t.Fatal(err)
	}

	// export
	w := doRequest(t, mux, "GET", "/api/config/export?types=dir_shortcuts", nil)
	if w.Code != 200 {
		t.Fatalf("export status=%d body=%s", w.Code, w.Body.String())
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	items := jsonExtractArray(t, got, "dir_shortcuts")
	if len(items) != 2 {
		t.Fatalf("want 2 dir_shortcuts, got %d", len(items))
	}
	// 检查 remote_password 字段在导出里
	if !strings.Contains(string(items[1]), "secret") {
		t.Errorf("remote_password 没被导出: %s", items[1])
	}

	// 清空
	all, _ := s.dirDB.List()
	for _, x := range all {
		_ = s.dirDB.Delete(x.ID)
	}

	// import（dedupe=append 让两条都重新进）
	w = doRequest(t, mux, "POST", "/api/config/import", configImportRequest{
		Type:   "dir_shortcuts",
		Items:  jsonExtractArrayRaw(t, got, "dir_shortcuts"),
		Dedupe: "append",
	})
	if w.Code != 200 {
		t.Fatalf("import status=%d body=%s", w.Code, w.Body.String())
	}
	var res configImportResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Created != 2 {
		t.Errorf("created=%d want 2; errors=%v", res.Created, res.Errors)
	}

	// 校验内容还原
	list, _ := s.dirDB.List()
	if len(list) != 2 {
		t.Fatalf("after import list=%d want 2", len(list))
	}
	var foundRemote bool
	for _, x := range list {
		if x.Name == "远端-1" {
			foundRemote = true
			if x.RemotePassword != "secret" {
				t.Errorf("remote_password 丢失: got %q", x.RemotePassword)
			}
		}
	}
	if !foundRemote {
		t.Error("远端-1 没导入")
	}
}

func TestExportImport_RoundTrip_WebLinks(t *testing.T) {
	s := newConfigExportTestServer(t)
	mux := newConfigExportMux(s)

	for i, name := range []string{"GitHub", "Notion"} {
		id := "wl-" + name
		_ = s.linkDB.Create(&backend.WebLink{ID: id, Name: name, URL: "https://" + strings.ToLower(name) + ".com"})
		_ = i
	}

	w := doRequest(t, mux, "GET", "/api/config/export?types=web_links", nil)
	var got map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	items := jsonExtractArray(t, got, "web_links")
	if len(items) != 2 {
		t.Fatalf("want 2, got %d", len(items))
	}

	// 清空再导入
	all, _ := s.linkDB.List()
	for _, x := range all {
		_ = s.linkDB.Delete(x.ID)
	}

	w = doRequest(t, mux, "POST", "/api/config/import", configImportRequest{
		Type: "web_links", Items: mustRawArray(t, items), Dedupe: "append",
	})
	var res configImportResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Created != 2 {
		t.Errorf("created=%d want 2", res.Created)
	}
}

func TestExportImport_RoundTrip_Experiences(t *testing.T) {
	s := newConfigExportTestServer(t)
	mux := newConfigExportMux(s)

	// 1 条带 markdown 细节的经验
	longDetails := "# Title\n\n- step 1\n- step 2\n\n```bash\necho hello\n```\n\nEnd."
	_ = s.expDB.Create(&backend.Experience{
		ID: "exp-long-1", Module: "git", Keywords: "rebase,merge", Scene: "feature 收尾",
		Details: longDetails, Version: "v1.0.0",
	})

	w := doRequest(t, mux, "GET", "/api/config/export?types=experiences", nil)
	var got map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	items := jsonExtractArray(t, got, "experiences")

	// 清空
	all, _ := s.expDB.ListAll()
	for _, x := range all {
		_ = s.expDB.Delete(x.ID)
	}

	w = doRequest(t, mux, "POST", "/api/config/import", configImportRequest{
		Type: "experiences", Items: mustRawArray(t, items), Dedupe: "append",
	})
	var res configImportResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Created != 1 {
		t.Errorf("created=%d want 1; errors=%v", res.Created, res.Errors)
	}

	// 校验 details 全文没丢
	after, _ := s.expDB.ListAll()
	if len(after) != 1 {
		t.Fatalf("after import list=%d", len(after))
	}
	if after[0].Details != longDetails {
		t.Errorf("details 文本不一致:\nwant:\n%s\n\ngot:\n%s", longDetails, after[0].Details)
	}
}

func TestExportImport_RoundTrip_Tasks(t *testing.T) {
	s := newConfigExportTestServer(t)
	mux := newConfigExportMux(s)

	// 关联一条经验
	exp := &backend.Experience{ID: "exp-rb-1", Module: "git", Scene: "merge"}
	_ = s.expDB.Create(exp)
	task := &backend.Task{
		ID: "task-rb-1", Title: "rb task", Description: "desc",
		Status: backend.TaskStatusPending, TaskType: "manual", Priority: 7,
	}
	_ = s.db.Create(task)
	_ = s.db.AttachExperiences(task.ID, []string{exp.ID})

	w := doRequest(t, mux, "GET", "/api/config/export?types=tasks_manual", nil)
	var got map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	items := jsonExtractArray(t, got, "tasks_manual")

	// 清空
	allTasks, _ := s.db.List(backend.TaskFilter{Limit: 1000})
	for _, x := range allTasks {
		_ = s.db.SetTaskExperiences(x.ID, nil)
		_ = s.db.Delete(x.ID)
	}

	w = doRequest(t, mux, "POST", "/api/config/import", configImportRequest{
		Type: "tasks_manual", Items: mustRawArray(t, items), Dedupe: "append",
	})
	var res configImportResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Created != 1 {
		t.Errorf("created=%d want 1; errors=%v", res.Created, res.Errors)
	}

	// 校验经验关联恢复
	after, _ := s.db.FindByTitle("rb task")
	if after == nil {
		t.Fatal("task not imported")
	}
	if after.TaskType != "manual" {
		t.Errorf("task_type=%q want manual", after.TaskType)
	}
	ids, _ := s.db.ListExperienceIDsForTask(after.ID)
	if len(ids) != 1 || ids[0] != exp.ID {
		t.Errorf("experience_ids not attached: %v", ids)
	}
}

func TestExportImport_RoundTrip_ScheduledTasks(t *testing.T) {
	s := newConfigExportTestServer(t)
	mux := newConfigExportMux(s)

	_ = s.schedDB.Create(&backend.ScheduledTask{
		ID: "sch-1", Name: "daily-collect", CronExpr: "0 9 * * *", CommandType: "claude",
		Prompt: "do stuff", Enabled: true,
	})

	w := doRequest(t, mux, "GET", "/api/config/export?types=tasks_scheduled", nil)
	var got map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	items := jsonExtractArray(t, got, "tasks_scheduled")

	all, _ := s.schedDB.List()
	for _, x := range all {
		_ = s.schedDB.Delete(x.ID)
	}

	w = doRequest(t, mux, "POST", "/api/config/import", configImportRequest{
		Type: "tasks_scheduled", Items: mustRawArray(t, items), Dedupe: "append",
	})
	var res configImportResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Created != 1 {
		t.Errorf("created=%d want 1; errors=%v", res.Created, res.Errors)
	}

	after, _ := s.schedDB.List()
	if len(after) != 1 {
		t.Fatalf("after import list=%d", len(after))
	}
	// 导入必须强制 enabled=false
	if after[0].Enabled {
		t.Errorf("scheduled task imported enabled=true, want false (避免误触发)")
	}
	if after[0].Prompt != "do stuff" {
		t.Errorf("prompt lost: %q", after[0].Prompt)
	}
}

// =========================
// 去重策略
// =========================

func TestImport_Dedupe_Skip(t *testing.T) {
	s := newConfigExportTestServer(t)
	mux := newConfigExportMux(s)

	_ = s.linkDB.Create(&backend.WebLink{ID: "wl-gh", Name: "GitHub", URL: "https://github.com"})
	item := mustRaw(t, map[string]any{"name": "GitHub", "url": "https://github.com/x"})

	w := doRequest(t, mux, "POST", "/api/config/import", configImportRequest{
		Type: "web_links", Items: json.RawMessage("[" + string(item) + "]"), Dedupe: "skip",
	})
	var res configImportResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Skipped != 1 || res.Created != 0 {
		t.Errorf("skip 路径: skipped=%d created=%d", res.Skipped, res.Created)
	}
}

func TestImport_Dedupe_Overwrite(t *testing.T) {
	s := newConfigExportTestServer(t)
	mux := newConfigExportMux(s)

	_ = s.linkDB.Create(&backend.WebLink{ID: "wl-gh", Name: "GitHub", URL: "https://github.com", SortOrder: 5})
	item := mustRaw(t, map[string]any{"name": "GitHub", "url": "https://github.com/new", "sort_order": 99})

	w := doRequest(t, mux, "POST", "/api/config/import", configImportRequest{
		Type: "web_links", Items: json.RawMessage("[" + string(item) + "]"), Dedupe: "overwrite",
	})
	var res configImportResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Updated != 1 {
		t.Errorf("updated=%d want 1; errors=%v", res.Updated, res.Errors)
	}
	all, _ := s.linkDB.List()
	if len(all) != 1 {
		t.Fatalf("overwrite 后 list=%d", len(all))
	}
	if all[0].URL != "https://github.com/new" || all[0].SortOrder != 99 {
		t.Errorf("overwrite 未生效: %+v", all[0])
	}
}

func TestImport_Dedupe_Append(t *testing.T) {
	s := newConfigExportTestServer(t)
	mux := newConfigExportMux(s)

	_ = s.linkDB.Create(&backend.WebLink{ID: "wl-gh", Name: "GitHub", URL: "https://github.com"})
	item := mustRaw(t, map[string]any{"name": "GitHub", "url": "https://github.com/alt"})

	w := doRequest(t, mux, "POST", "/api/config/import", configImportRequest{
		Type: "web_links", Items: json.RawMessage("[" + string(item) + "]"), Dedupe: "append",
	})
	var res configImportResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Created != 1 {
		t.Errorf("created=%d want 1", res.Created)
	}
	all, _ := s.linkDB.List()
	if len(all) != 2 {
		t.Fatalf("append 后 list=%d want 2", len(all))
	}
}

// =========================
// Preview：必填字段校验
// =========================

func TestImportPreview_RejectsBadInput(t *testing.T) {
	s := newConfigExportTestServer(t)
	mux := newConfigExportMux(s)

	w := doRequest(t, mux, "POST", "/api/config/import/preview", configImportRequest{
		Type:  "dir_shortcuts",
		Items: json.RawMessage(`[{"name":"a"},{"path":"/no-name"},{}]`),
	})
	if w.Code != 200 {
		t.Fatalf("preview status=%d", w.Code)
	}
	var got struct {
		Items []configImportPreviewItem `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Items[0].Valid {
		t.Errorf("空 path 应判无效: %+v", got.Items[0])
	}
	if got.Items[1].Valid {
		t.Errorf("空 name 应判无效: %+v", got.Items[1])
	}
	// 第三条是 name+path 都缺
	if got.Items[2].Valid {
		t.Errorf("空 name+path 应判无效: %+v", got.Items[2])
	}
}

func TestImportPreview_AllExportTypes(t *testing.T) {
	s := newConfigExportTestServer(t)
	mux := newConfigExportMux(s)

	// all 导出
	w := doRequest(t, mux, "GET", "/api/config/export", nil)
	if w.Code != 200 {
		t.Fatalf("export all status=%d", w.Code)
	}
	var got map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	for _, k := range []string{"dir_shortcuts", "web_links", "experiences", "tasks_manual", "tasks_scheduled"} {
		if _, ok := got[k]; !ok {
			t.Errorf("all 导出缺字段 %s", k)
		}
	}
}

func TestImportPreview_TaskTypeForceManual(t *testing.T) {
	s := newConfigExportTestServer(t)
	mux := newConfigExportMux(s)

	// 即使输入带 task_type=scheduled，导入 manual 应被覆盖
	item := mustRaw(t, map[string]any{"title": "x", "task_type": "scheduled"})
	w := doRequest(t, mux, "POST", "/api/config/import", configImportRequest{
		Type: "tasks_manual", Items: json.RawMessage("[" + string(item) + "]"), Dedupe: "append",
	})
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	got, _ := s.db.FindByTitle("x")
	if got == nil {
		t.Fatal("not imported")
	}
	if got.TaskType != "manual" {
		t.Errorf("task_type 没被强制成 manual: %q", got.TaskType)
	}
}

// =========================
// 工具函数
// =========================

func mustRaw(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustRawArray(t *testing.T, items []json.RawMessage) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func jsonExtractArray(t *testing.T, m map[string]json.RawMessage, key string) []json.RawMessage {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %s", key)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(v, &arr); err != nil {
		t.Fatalf("decode %s: %v", key, err)
	}
	return arr
}

func jsonExtractArrayRaw(t *testing.T, m map[string]json.RawMessage, key string) json.RawMessage {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %s", key)
	}
	return v
}
