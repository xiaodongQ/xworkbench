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

	w.Header().Set("Content-Type", "application/json")

	now := fmt.Sprintf(`"%s"`, time.Now().Format(time.RFC3339))

	switch typeParam {
	case "all":
		allOut := struct {
			Version       string `json:"version"`
			ExportedAt    string `json:"exported_at"`
			DirShortcuts  any    `json:"dir_shortcuts"`
			WebLinks      any    `json:"web_links"`
			Experiences  any    `json:"experiences"`
			TasksManual   any    `json:"tasks_manual"`
			TasksScheduled any   `json:"tasks_scheduled"`
		}{
			Version:        "1.0",
			ExportedAt:     now,
			DirShortcuts:   s.exportDirShortcuts(),
			WebLinks:       s.exportWebLinks(),
			Experiences:    s.exportExperiences(),
			TasksManual:    s.exportTasksByType("manual"),
			TasksScheduled: s.exportScheduledTasks(),
		}
		b, _ := json.MarshalIndent(allOut, "", "  ")
		w.Write(b)
		return
	case "tasks":
		manual := s.exportTasksAsUnified("manual")
		sched := s.exportScheduledAsUnified()
		merged := append(manual, sched...)
		tasksOut := struct {
			Version     string `json:"version"`
			ExportedAt  string `json:"exported_at"`
			Tasks       any    `json:"tasks"`
		}{
			Version:    "1.0",
			ExportedAt: now,
			Tasks:      merged,
		}
		b, _ := json.MarshalIndent(tasksOut, "", "  ")
		w.Write(b)
		return
	default:
		// 多 type 用逗号分，手动拼接（dir_shortcuts/web_links 用 indent，其他用 compact）
		types := strings.Split(typeParam, ",")
		parts := []string{}
		addField := func(k string, v string) {
			parts = append(parts, fmt.Sprintf(`  "%s": %s`, k, v))
		}
		addField("version", `"1.0"`)
		addField("exported_at", now)
		for _, t := range types {
			t = strings.TrimSpace(t)
			var val string
			switch t {
			case "shortcuts", "dir_shortcuts":
				val = mustMarshalIndent(s.exportDirShortcuts())
			case "links", "web_links":
				val = mustMarshalIndent(s.exportWebLinks())
			case "experiences":
				val = mustMarshal(s.exportExperiences())
			case "tasks_manual":
				val = mustMarshal(s.exportTasksByType("manual"))
			case "tasks_scheduled":
				val = mustMarshal(s.exportScheduledTasks())
			case "tasks":
				manual := s.exportTasksAsUnified("manual")
				sched := s.exportScheduledAsUnified()
				merged := append(manual, sched...)
				val = mustMarshal(merged)
			default:
				writeErr(w, http.StatusBadRequest, "unknown type: "+t)
				return
			}
			parts = append(parts, fmt.Sprintf(`  "%s": %s`, t, val))
		}
			w.Write([]byte("{\n" + strings.Join(parts[:len(parts)-1], ",\n") + ",\n" + parts[len(parts)-1] + "\n}"))
	}
}
func (s *APIServer) exportDirShortcuts() []*DirShortcutExport {
	list, err := s.dirDB.List()
	if err != nil {
		logger.Warnw("config export: dir shortcuts list failed", "err", err)
		return []*DirShortcutExport{}
	}
	if list == nil {
		return []*DirShortcutExport{}
	}
	out := make([]*DirShortcutExport, 0, len(list))
	for _, d := range list {
		if d == nil {
			continue
		}
		out = append(out, &DirShortcutExport{
			Name:     d.Name,
			Path:     d.Path,
			Type:     d.Type,
			RemoteHost:     d.RemoteHost,
			RemoteUser:     d.RemoteUser,
			RemotePath:     d.RemotePath,
			RemotePassword: d.RemotePassword,
			AuthMethod:     d.AuthMethod,
			KeyPath:        d.KeyPath,
		})
	}
	return out
}

func (s *APIServer) exportWebLinks() []*WebLinkExport {
	list, _ := s.linkDB.List()
	if list == nil {
		return []*WebLinkExport{}
	}
	out := make([]*WebLinkExport, 0, len(list))
	for _, l := range list {
		if l == nil {
			continue
		}
		out = append(out, &WebLinkExport{
			Name:   l.Name,
			URL:    l.URL,
			IconURL: l.IconURL,
		})
	}
	return out
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

// TaskExportItem "统一任务"导出格式。合并 manual + scheduled 到同一 "tasks" 数组用。
// 导入时按 task_type 路由到不同表。
type TaskExportItem struct {
	backend.Task
	CommandType    string `json:"command_type,omitempty"`     // claude/cbc/shell
	Model          string `json:"model,omitempty"`
	CronExpr       string `json:"cron_expr,omitempty"`        // 仅 scheduled
	WorkingDir     string `json:"working_dir,omitempty"`      // 仅 scheduled
	Enabled        bool   `json:"enabled,omitempty"`          // scheduled专用，导入时强制 false
	TimeoutSec     int    `json:"timeout_sec,omitempty"`
}

// exportScheduledAsUnified 把 ScheduledTask 转为 *TaskExportItem 格式（ task_type=scheduled），
// 给“合并导出 tasks”用。导入时按 task_type 路由到不同表。
func (s *APIServer) exportScheduledAsUnified() []*TaskExportItem {
	sched := s.exportScheduledTasks()
	out := make([]*TaskExportItem, 0, len(sched))
	for _, st := range sched {
		if st == nil { continue }
		out = append(out, &TaskExportItem{
			Task: backend.Task{
				ID:          st.ID,
				Title:       st.Name,
				Description: st.Prompt,
				Status:      backend.TaskStatusPending,
				Priority:    5,
				TaskType:    backend.TaskTypeScheduled,
			},
			CommandType: st.CommandType,
			Model:       st.Model,
			CronExpr:    st.CronExpr,
			WorkingDir:  st.WorkingDir,
			Enabled:     st.Enabled,
			TimeoutSec:  st.TimeoutSec,
		})
	}
	return out
}

// exportTasksAsUnified 把 manual tasks 转 *TaskExportItem。补 experience_ids。
func (s *APIServer) exportTasksAsUnified(t string) []*TaskExportItem {
	tasks := s.exportTasksByType(t)
	out := make([]*TaskExportItem, 0, len(tasks))
	for _, task := range tasks {
		if task == nil { continue }
		out = append(out, &TaskExportItem{Task: *task})
	}
	return out
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
	case "tasks":
		// “tasks” 是合并导入：按每个 item 的 task_type 路由到不同表。
		manualItems, schedItems := s.splitTasksByType(items)
		s.importTasks(manualItems, "manual", &res)
		s.importScheduledTasks(schedItems, &res)
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
	case "tasks":
		// “tasks” 是合并预览：按 item.task_type 路由，每个 index 返回 1 条 preview。
		// 理论上同 index 上 manual + scheduled 是两种独立记录，这里为保型一律给“同一条 preview”
		//（最后取为“只要 type=manual 走 manual preview，type=scheduled 走 scheduled preview”）。
		manualItems, schedItems := s.splitTasksByType(items)
		manualPreview := s.previewTasks(manualItems, "manual")
		schedPreview := s.previewScheduledTasks(schedItems)
		// 合并: 重新按原始 index 映射
		out.Items = s.mergeTasksPreview(items, manualPreview, schedPreview)
	default:
		writeErr(w, http.StatusBadRequest, "unknown type: "+req.Type)
		return
	}
	writeJSON(w, out)
}

// ----- 业务实现：导入/预览 -----

// splitTasksByType 按 task_type 拆分 items 数组为 (manual items, scheduled items)。
// task_type 为空或“manual”走 manual，其他值都走 scheduled。
// 返回的两个 slice 中的 raw 保持原状，让后续 previewTasks/importTasks 复用。
func (s *APIServer) splitTasksByType(items []json.RawMessage) (manual, scheduled []json.RawMessage) {
	for _, raw := range items {
		var t struct {
			TaskType string `json:"task_type"`
		}
		_ = json.Unmarshal(raw, &t)
		switch t.TaskType {
		case "scheduled":
			scheduled = append(scheduled, raw)
		default:
			manual = append(manual, raw)
		}
	}
	return manual, scheduled
}

// mergeTasksPreview 按原 items 顺序逐个调对应 preview，重置 Index 为 0..N-1。
// 避免出现 manualPreview[1] 被当成原数组 index=1 的情况（顺序错位 bug）。
func (s *APIServer) mergeTasksPreview(items []json.RawMessage, manualPreview, schedPreview []configImportPreviewItem) []configImportPreviewItem {
	manualIdx, schedIdx := 0, 0
	out := make([]configImportPreviewItem, len(items))
	for newI, raw := range items {
		var t struct {
			TaskType string `json:"task_type"`
		}
		_ = json.Unmarshal(raw, &t)
		var pv configImportPreviewItem
		if t.TaskType == "scheduled" {
			if schedIdx < len(schedPreview) {
				pv = schedPreview[schedIdx]
			}
			schedIdx++
		} else {
			if manualIdx < len(manualPreview) {
				pv = manualPreview[manualIdx]
			}
			manualIdx++
		}
		pv.Index = newI // 重置为“合并后顺序”的 index
		out[newI] = pv
	}
	return out
}

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
		// 兼容合并导入时用 "title" 字段（统一任务格式）；否则从 "name" 读
		if st.Name == "" {
			var alt struct{ Title string `json:"title"` }
			_ = json.Unmarshal(raw, &alt)
			st.Name = alt.Title
		}
		// 兼容 prompt / description 两种 prompt 字段
		if st.Prompt == "" {
			var alt struct{ Description string `json:"description"` }
			_ = json.Unmarshal(raw, &alt)
			st.Prompt = alt.Description
		}
		if st.Name == "" || st.CronExpr == "" || st.CommandType == "" {
			res.Errors = append(res.Errors, importError{Index: i, Reason: "name/title, cron_expr, command_type required"})
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
		// 兼容合并导入时用 "title" 字段（统一任务格式）
		if st.Name == "" {
			var alt struct{ Title string `json:"title"` }
			_ = json.Unmarshal(raw, &alt)
			st.Name = alt.Title
		}
		if st.Prompt == "" {
			var alt struct{ Description string `json:"description"` }
			_ = json.Unmarshal(raw, &alt)
			st.Prompt = alt.Description
		}
		if st.Name == "" || st.CronExpr == "" || st.CommandType == "" {
			out = append(out, configImportPreviewItem{Index: i, Valid: false, Reason: "name/title, cron_expr, command_type required"})
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

func mustMarshalIndent(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		logger.Warnw("config export marshal indent failed", "err", err)
		return "null"
	}
	return string(b)
}

// DirShortcutExport 导出用简化结构（去掉了 id/created_at/sort_order/updated_at 等无关字段）
type DirShortcutExport struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type,omitempty"`
	RemoteHost     string `json:"remote_host,omitempty"`
	RemoteUser     string `json:"remote_user,omitempty"`
	RemotePath     string `json:"remote_path,omitempty"`
	RemotePassword string `json:"remote_password,omitempty"`
	AuthMethod     string `json:"auth_method,omitempty"`
	KeyPath        string `json:"key_path,omitempty"`
}

// WebLinkExport 导出用简化结构（去掉了 id/created_at/sort_order 等无关字段）
type WebLinkExport struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	IconURL string `json:"icon_url,omitempty"`
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
