package main

// 数据管理：导入 / 导出 / 备份
// 实现以下 API：
//   GET  /api/config/export?types=shortcuts,links,experiences,tasks_manual,tasks_scheduled[,all]
//                                            → 多个类型逗号分隔；types=all 或缺省 = 全量
//                                            → 单类型且无逗号时设置 Content-Disposition 触发下载
//   POST /api/config/import/preview        → 入参 {type, items[]} → {valid:[], invalid:[]}
//   POST /api/config/import                → 入参 {type, items[], dedupe} → {created,updated,skipped,errors[]}
//                                            dedupe: skip | overwrite | append（默认 skip）
//
// AI 自然语言导入不在后端另起 API：
//   路径 A：前端把 prompt 发到 /api/pty（走 AI 对话 Tab）
//   路径 B：前端建一个 task，description = prompt，跑 /api/tasks/{id}/run（claude -p）
//   两者都复用现有机制，后端不用加新接口。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xiaodongQ/xworkbench/internal/backend"
)

// ----- 导出 -----

// handleConfigExport 导出入口
// /api/config/export?types=shortcuts,links,experiences,tasks_manual,tasks_scheduled
// 不传 types 时默认全量；types=all 显式全量；支持单个 type 单文件下载。
func (s *APIServer) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	typeParam := strings.TrimSpace(r.URL.Query().Get("types"))
	if typeParam == "" {
		typeParam = "all"
	}

	// 单类型导出时设下载文件名，便于浏览器直接保存
	if !strings.Contains(typeParam, ",") && typeParam != "all" {
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="xworkbench-%s-%s.json"`,
				typeParam, time.Now().Format("20060102-150405")))
	}

	var out map[string]json.RawMessage
	switch typeParam {
	case "all":
		out = map[string]json.RawMessage{
			"version":         jsonRaw(`"1.0"`),
			"exported_at":     jsonRaw(fmt.Sprintf(`"%s"`, time.Now().Format(time.RFC3339))),
			"dir_shortcuts":   jsonRaw(mustMarshal(s.exportDirShortcuts())),
			"web_links":       jsonRaw(mustMarshal(s.exportWebLinks())),
			"experiences":     jsonRaw(mustMarshal(s.exportExperiences())),
			"tasks_manual":    jsonRaw(mustMarshal(s.exportTasksByType("manual"))),
			"tasks_scheduled": jsonRaw(mustMarshal(s.exportScheduledTasks())),
		}
	default:
		// 多个 type 用逗号分，每类作为顶层 key（与 all 形态一致）
		types := strings.Split(typeParam, ",")
		out = map[string]json.RawMessage{
			"version":     jsonRaw(`"1.0"`),
			"exported_at": jsonRaw(fmt.Sprintf(`"%s"`, time.Now().Format(time.RFC3339))),
		}
		for _, t := range types {
			t = strings.TrimSpace(t)
			switch t {
			case "shortcuts", "dir_shortcuts":
				out["dir_shortcuts"] = jsonRaw(mustMarshal(s.exportDirShortcuts()))
			case "links", "web_links":
				out["web_links"] = jsonRaw(mustMarshal(s.exportWebLinks()))
			case "experiences":
				out["experiences"] = jsonRaw(mustMarshal(s.exportExperiences()))
			case "tasks_manual":
				out["tasks_manual"] = jsonRaw(mustMarshal(s.exportTasksByType("manual")))
			case "tasks_scheduled":
				out["tasks_scheduled"] = jsonRaw(mustMarshal(s.exportScheduledTasks()))
			default:
				writeErr(w, http.StatusBadRequest, "unknown type: "+t)
				return
			}
		}
	}
	writeJSON(w, out)
}

func (s *APIServer) exportDirShortcuts() []*backend.DirShortcut {
	list, err := s.dirDB.List()
	if err != nil {
		logger.Warnw("config export: dir shortcuts list failed", "err", err)
		return []*backend.DirShortcut{}
	}
	if list == nil {
		return []*backend.DirShortcut{}
	}
	// remote_password 仍导出；前端导出按钮处有显式提示
	return list
}

func (s *APIServer) exportWebLinks() []*backend.WebLink {
	list, _ := s.linkDB.List()
	if list == nil {
		return []*backend.WebLink{}
	}
	return list
}

func (s *APIServer) exportExperiences() []*backend.Experience {
	// 经验库本身是 list，调用 export handler 不带 module 过滤
	// repo 没有 ListAll 直接调用，需看 experience handler 怎么处理
	list, err := s.listExpsAll()
	if err != nil {
		logger.Warnw("config export: experiences list failed", "err", err)
		return []*backend.Experience{}
	}
	if list == nil {
		return []*backend.Experience{}
	}
	return list
}

func (s *APIServer) exportTasksByType(t string) []*backend.Task {
	tasks, err := s.db.List(backend.TaskFilter{TaskType: t, Limit: 10000})
	if err != nil {
		logger.Warnw("config export: tasks list failed", "type", t, "err", err)
		return []*backend.Task{}
	}
	if tasks == nil {
		return []*backend.Task{}
	}
	// 列表接口不填 ExperienceIDs，这里补一下
	for _, task := range tasks {
		ids, _ := s.db.ListExperienceIDsForTask(task.ID)
		task.ExperienceIDs = ids
	}
	return tasks
}

func (s *APIServer) exportScheduledTasks() []*backend.ScheduledTask {
	list, _ := s.schedDB.List()
	if list == nil {
		return []*backend.ScheduledTask{}
	}
	return list
}

// ----- 导入 -----

// configImportRequest 导入请求体
type configImportRequest struct {
	Type   string          `json:"type"`   // dir_shortcuts|web_links|experiences|tasks_manual|tasks_scheduled
	Items  json.RawMessage `json:"items"`  // 任意 JSON 数组；解析时按 type 做形状校验
	Dedupe string          `json:"dedupe"` // skip|overwrite|append，默认 skip
}

// configImportResult 导入结果
type configImportResult struct {
	Type      string         `json:"type"`
	Dedupe    string         `json:"dedupe"`
	Created   int            `json:"created"`
	Updated   int            `json:"updated"`
	Skipped   int            `json:"skipped"`
	Errors    []importError  `json:"errors,omitempty"`
}

type importError struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

// handleConfigImport 真正执行导入
func (s *APIServer) handleConfigImport(w http.ResponseWriter, r *http.Request) {
	var req configImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Dedupe == "" {
		req.Dedupe = "skip"
	}
	if req.Dedupe != "skip" && req.Dedupe != "overwrite" && req.Dedupe != "append" {
		writeErr(w, http.StatusBadRequest, "dedupe must be skip|overwrite|append")
		return
	}

	var items []json.RawMessage
	if err := json.Unmarshal(req.Items, &items); err != nil {
		writeErr(w, http.StatusBadRequest, "items must be array: "+err.Error())
		return
	}

	res := configImportResult{Type: req.Type, Dedupe: req.Dedupe}
	switch req.Type {
	case "dir_shortcuts":
		s.importDirShortcuts(items, &res)
	case "web_links":
		s.importWebLinks(items, &res)
	case "experiences":
		s.importExperiences(items, &res)
	case "tasks_manual":
		s.importTasks(items, "manual", &res)
	case "tasks_scheduled":
		s.importScheduledTasks(items, &res)
	default:
		writeErr(w, http.StatusBadRequest, "unknown type: "+req.Type)
		return
	}
	logger.Infow("config import done", "type", req.Type, "created", res.Created, "updated", res.Updated, "skipped", res.Skipped, "errors", len(res.Errors))
	writeJSON(w, res)
}

// configImportPreviewItem 单条预览结果
type configImportPreviewItem struct {
	Index   int    `json:"index"`
	Valid   bool   `json:"valid"`
	Reason  string `json:"reason,omitempty"`
	Summary string `json:"summary,omitempty"` // 简短摘要，便于 UI 显示
}

// handleConfigImportPreview 预览导入：不真正写入，只校验 + 去重预判
func (s *APIServer) handleConfigImportPreview(w http.ResponseWriter, r *http.Request) {
	var req configImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var items []json.RawMessage
	if err := json.Unmarshal(req.Items, &items); err != nil {
		writeErr(w, http.StatusBadRequest, "items must be array: "+err.Error())
		return
	}

	out := struct {
		Type  string                     `json:"type"`
		Total int                        `json:"total"`
		Items []configImportPreviewItem  `json:"items"`
	}{Type: req.Type, Total: len(items)}

	switch req.Type {
	case "dir_shortcuts":
		out.Items = s.previewDirShortcuts(items)
	case "web_links":
		out.Items = s.previewWebLinks(items)
	case "experiences":
		out.Items = s.previewExperiences(items)
	case "tasks_manual":
		out.Items = s.previewTasks(items, "manual")
	case "tasks_scheduled":
		out.Items = s.previewScheduledTasks(items)
	default:
		writeErr(w, http.StatusBadRequest, "unknown type: "+req.Type)
		return
	}
	writeJSON(w, out)
}

// ----- 业务实现：导入/预览 -----

func (s *APIServer) importDirShortcuts(items []json.RawMessage, res *configImportResult) {
	for i, raw := range items {
		var d backend.DirShortcut
		if err := json.Unmarshal(raw, &d); err != nil {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "parse: " + err.Error()})
			continue
		}
		if d.Name == "" || d.Path == "" {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "name and path required"})
			continue
		}
		if d.Type == "" {
			d.Type = backend.DirShortcutTypeLocal
		}
		existing, _ := s.dirDB.GetByName(d.Name)
		switch res.Dedupe {
		case "skip":
			if existing != nil {
				res.Skipped++
				continue
			}
		case "overwrite":
			if existing != nil {
				d.ID = existing.ID
				if err := s.dirDB.Update(&d); err != nil {
					res.Errors = append(res.Errors, importError{Index: i, Reason: "update: " + err.Error()})
					continue
				}
				res.Updated++
				continue
			}
		}
		d.ID = uuid.New().String()
		if d.SortOrder == 0 {
			d.SortOrder = s.dirDB.NextSortOrder()
		}
		d.CreatedAt = time.Now()
		if err := s.dirDB.Create(&d); err != nil {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "create: " + err.Error()})
			continue
		}
		res.Created++
	}
}

func (s *APIServer) previewDirShortcuts(items []json.RawMessage) []configImportPreviewItem {
	out := make([]configImportPreviewItem, 0, len(items))
	for i, raw := range items {
		var d backend.DirShortcut
		if err := json.Unmarshal(raw, &d); err != nil {
			out = append(out, configImportPreviewItem{Index: i, Valid: false, Reason: "parse: " + err.Error()})
			continue
		}
		if d.Name == "" || d.Path == "" {
			out = append(out, configImportPreviewItem{Index: i, Valid: false, Reason: "name and path required"})
			continue
		}
		existing, _ := s.dirDB.GetByName(d.Name)
		summary := d.Name + " (" + d.Path + ")"
		reason := ""
		if existing != nil {
			reason = "已存在(name), skip/overwrite 二选一"
		}
		out = append(out, configImportPreviewItem{Index: i, Valid: true, Reason: reason, Summary: summary})
	}
	return out
}

func (s *APIServer) importWebLinks(items []json.RawMessage, res *configImportResult) {
	for i, raw := range items {
		var l backend.WebLink
		if err := json.Unmarshal(raw, &l); err != nil {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "parse: " + err.Error()})
			continue
		}
		if l.Name == "" || l.URL == "" {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "name and url required"})
			continue
		}
		existing, _ := s.linkDB.GetByName(l.Name)
		switch res.Dedupe {
		case "skip":
			if existing != nil {
				res.Skipped++
				continue
			}
		case "overwrite":
			if existing != nil {
				l.ID = existing.ID
				if err := s.linkDB.Update(&l); err != nil {
					res.Errors = append(res.Errors, importError{Index: i, Reason: "update: " + err.Error()})
					continue
				}
				res.Updated++
				continue
			}
		}
		l.ID = uuid.New().String()
		if l.SortOrder == 0 {
			l.SortOrder = s.linkDB.NextSortOrder()
		}
		l.CreatedAt = time.Now()
		if err := s.linkDB.Create(&l); err != nil {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "create: " + err.Error()})
			continue
		}
		res.Created++
	}
}

func (s *APIServer) previewWebLinks(items []json.RawMessage) []configImportPreviewItem {
	out := make([]configImportPreviewItem, 0, len(items))
	for i, raw := range items {
		var l backend.WebLink
		if err := json.Unmarshal(raw, &l); err != nil {
			out = append(out, configImportPreviewItem{Index: i, Valid: false, Reason: "parse: " + err.Error()})
			continue
		}
		if l.Name == "" || l.URL == "" {
			out = append(out, configImportPreviewItem{Index: i, Valid: false, Reason: "name and url required"})
			continue
		}
		existing, _ := s.linkDB.GetByName(l.Name)
		reason := ""
		if existing != nil {
			reason = "已存在(name), skip/overwrite 二选一"
		}
		out = append(out, configImportPreviewItem{Index: i, Valid: true, Reason: reason, Summary: l.Name + " → " + l.URL})
	}
	return out
}

func (s *APIServer) importExperiences(items []json.RawMessage, res *configImportResult) {
	for i, raw := range items {
		var e backend.Experience
		if err := json.Unmarshal(raw, &e); err != nil {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "parse: " + err.Error()})
			continue
		}
		if e.Module == "" {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "module required"})
			continue
		}
		existing := s.findExperienceByModuleKeywords(e.Module, e.Keywords)
		switch res.Dedupe {
		case "skip":
			if existing != nil {
				res.Skipped++
				continue
			}
		case "overwrite":
			if existing != nil {
				e.ID = existing.ID
				if err := s.expDB.Update(&e); err != nil {
					res.Errors = append(res.Errors, importError{Index: i, Reason: "update: " + err.Error()})
					continue
				}
				res.Updated++
				continue
			}
		}
		e.ID = uuid.New().String()
		now := time.Now()
		e.CreatedAt = now
		e.UpdatedAt = now
		if e.Version == "" {
			e.Version = "v0.0.1"
		}
		if err := s.expDB.Create(&e); err != nil {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "create: " + err.Error()})
			continue
		}
		res.Created++
	}
}

func (s *APIServer) previewExperiences(items []json.RawMessage) []configImportPreviewItem {
	out := make([]configImportPreviewItem, 0, len(items))
	for i, raw := range items {
		var e backend.Experience
		if err := json.Unmarshal(raw, &e); err != nil {
			out = append(out, configImportPreviewItem{Index: i, Valid: false, Reason: "parse: " + err.Error()})
			continue
		}
		if e.Module == "" {
			out = append(out, configImportPreviewItem{Index: i, Valid: false, Reason: "module required"})
			continue
		}
		existing := s.findExperienceByModuleKeywords(e.Module, e.Keywords)
		reason := ""
		if existing != nil {
			reason = "已存在(module+keywords), skip/overwrite 二选一"
		}
		summary := e.Module
		if e.Keywords != "" {
			summary += " [" + e.Keywords + "]"
		}
		out = append(out, configImportPreviewItem{Index: i, Valid: true, Reason: reason, Summary: summary})
	}
	return out
}

func (s *APIServer) importTasks(items []json.RawMessage, defaultType string, res *configImportResult) {
	for i, raw := range items {
		var t backend.Task
		if err := json.Unmarshal(raw, &t); err != nil {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "parse: " + err.Error()})
			continue
		}
		if t.Title == "" {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "title required"})
			continue
		}
		// 强制 task_type 为目标类型，避免导入时被传入的字段污染
		t.TaskType = defaultType
		if t.Status == "" {
			t.Status = backend.TaskStatusPending
		}
		if t.Version == "" {
			t.Version = "v0.0.1"
		}
		expIDs := t.ExperienceIDs
		if len(expIDs) == 0 && t.ExperienceID != "" {
			expIDs = []string{t.ExperienceID}
		}

		existing, _ := s.findTaskByTitle(t.Title)
		switch res.Dedupe {
		case "skip":
			if existing != nil {
				res.Skipped++
				continue
			}
		case "overwrite":
			if existing != nil {
				t.ID = existing.ID
				if err := s.db.Update(&t); err != nil {
					res.Errors = append(res.Errors, importError{Index: i, Reason: "update: " + err.Error()})
					continue
				}
				if err := s.db.SetTaskExperiences(t.ID, expIDs); err != nil {
					res.Errors = append(res.Errors, importError{Index: i, Reason: "attach exp: " + err.Error()})
					continue
				}
				res.Updated++
				continue
			}
		}
		t.ID = uuid.New().String()
		t.CreatedAt = time.Now()
		if err := s.db.Create(&t); err != nil {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "create: " + err.Error()})
			continue
		}
		if len(expIDs) > 0 {
			if err := s.db.AttachExperiences(t.ID, expIDs); err != nil {
				res.Errors = append(res.Errors, importError{Index: i, Reason: "attach exp: " + err.Error()})
				continue
			}
		}
		res.Created++
	}
}

func (s *APIServer) previewTasks(items []json.RawMessage, defaultType string) []configImportPreviewItem {
	out := make([]configImportPreviewItem, 0, len(items))
	for i, raw := range items {
		var t backend.Task
		if err := json.Unmarshal(raw, &t); err != nil {
			out = append(out, configImportPreviewItem{Index: i, Valid: false, Reason: "parse: " + err.Error()})
			continue
		}
		if t.Title == "" {
			out = append(out, configImportPreviewItem{Index: i, Valid: false, Reason: "title required"})
			continue
		}
		existing, _ := s.findTaskByTitle(t.Title)
		reason := ""
		if existing != nil {
			reason = "已存在(title), skip/overwrite 二选一"
		}
		out = append(out, configImportPreviewItem{Index: i, Valid: true, Reason: reason, Summary: t.Title})
	}
	return out
}

func (s *APIServer) importScheduledTasks(items []json.RawMessage, res *configImportResult) {
	for i, raw := range items {
		var st backend.ScheduledTask
		if err := json.Unmarshal(raw, &st); err != nil {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "parse: " + err.Error()})
			continue
		}
		if st.Name == "" || st.CronExpr == "" || st.CommandType == "" {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "name, cron_expr, command_type required"})
			continue
		}
		existing, _ := s.findScheduledByName(st.Name)
		// scheduled 默认导入为禁用，避免误触发
		st.Enabled = false
		switch res.Dedupe {
		case "skip":
			if existing != nil {
				res.Skipped++
				continue
			}
		case "overwrite":
			if existing != nil {
				st.ID = existing.ID
				if err := s.schedDB.Update(&st); err != nil {
					res.Errors = append(res.Errors, importError{Index: i, Reason: "update: " + err.Error()})
					continue
				}
				res.Updated++
				_ = s.sch.Reload()
				continue
			}
		}
		st.ID = uuid.New().String()
		st.CreatedAt = time.Now()
		if err := s.schedDB.Create(&st); err != nil {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "create: " + err.Error()})
			continue
		}
		res.Created++
	}
	if s.sch != nil { _ = s.sch.Reload() }
}

func (s *APIServer) previewScheduledTasks(items []json.RawMessage) []configImportPreviewItem {
	out := make([]configImportPreviewItem, 0, len(items))
	for i, raw := range items {
		var st backend.ScheduledTask
		if err := json.Unmarshal(raw, &st); err != nil {
			out = append(out, configImportPreviewItem{Index: i, Valid: false, Reason: "parse: " + err.Error()})
			continue
		}
		if st.Name == "" || st.CronExpr == "" || st.CommandType == "" {
			out = append(out, configImportPreviewItem{Index: i, Valid: false, Reason: "name, cron_expr, command_type required"})
			continue
		}
		existing, _ := s.findScheduledByName(st.Name)
		reason := ""
		if existing != nil {
			reason = "已存在(name), skip/overwrite 二选一"
		}
		summary := st.Name + " [" + st.CronExpr + "]"
		out = append(out, configImportPreviewItem{Index: i, Valid: true, Reason: reason, Summary: summary})
	}
	return out
}

// ----- 辅助 -----

// jsonRaw 把字符串包成 JSON RawMessage（避免多一次 marshal）
func jsonRaw(s string) json.RawMessage {
	return json.RawMessage(s)
}

func mustMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		logger.Warnw("config export marshal failed", "err", err)
		return "null"
	}
	return string(b)
}

// ----- APIServer 上的便利包装 -----

func (s *APIServer) listExpsAll() ([]*backend.Experience, error) {
	return s.expDB.ListAll()
}

func (s *APIServer) findExperienceByModuleKeywords(module, keywords string) *backend.Experience {
	e, _ := s.expDB.FindByModuleKeywords(module, keywords)
	return e
}

func (s *APIServer) findTaskByTitle(title string) (*backend.Task, error) {
	return s.db.FindByTitle(title)
}

func (s *APIServer) findScheduledByName(name string) (*backend.ScheduledTask, error) {
	return s.schedDB.FindByName(name)
}
