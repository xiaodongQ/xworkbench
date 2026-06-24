# 定时任务下次执行时间显示 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在自动化页的定时任务列表里，对启用的任务显示"下次执行时间"，不新增列；后端在 `handleScheduledList` 注入 `next_run_at`，前端双行展示。

**Architecture:** 后端用现有 `robfig/cron/v3`（与 scheduler 同一 parser）解析 + 算下次时间，注入到 `ScheduledTask.NextRunAt` 字段；前端在"最近执行"列下追加"下次"小字。

**Tech Stack:** Go 1.25 / `robfig/cron/v3` / SQLite / vanilla JS（无构建）

---

## 文件结构

| 文件 | 职责 |
|---|---|
| [internal/backend/models.go](internal/backend/models.go) | struct 加 `NextRunAt` 字段（数据层） |
| [cmd/server/main.go:1637-1647](cmd/server/main.go#L1637) | `handleScheduledList` 注入 next（业务层） |
| `cmd/server/main_scheduled_test.go`（新建） | 单元 + 集成测试 |
| [cmd/server/static/js/views/automation.js:246-272](cmd/server/static/js/views/automation.js#L246) | 表格"最近执行"列改双行（UI） |

每个文件单一职责：模型只持数据，handler 只做注入，view 只渲染，测试只验证。

---

## Task 1: 给 `ScheduledTask` 加 `NextRunAt` 字段

**Files:**
- Modify: [internal/backend/models.go:172-187](internal/backend/models.go#L172)

- [ ] **Step 1: 编辑 struct 加字段**

把 [internal/backend/models.go:172-187](internal/backend/models.go#L172) 改成：

```go
// ScheduledTask 定时任务
type ScheduledTask struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	CronExpr       string     `json:"cron_expr"`
	CommandType    string     `json:"command_type"` // 'claude' | 'cbc' | 'shell'
	Model          string     `json:"model,omitempty"`
	Prompt         string     `json:"prompt,omitempty"`
	WorkingDir     string     `json:"working_dir,omitempty"`
	Enabled        bool       `json:"enabled"`
	TimeoutSec     int        `json:"timeout_sec"` // 超时秒数，0=默认（AI任务1小时，shell 5分钟）
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	NextRunAt      *time.Time `json:"next_run_at,omitempty"` // 下次执行时间；仅 enabled 任务注入，nil=禁用或解析失败
	LastStatus     string     `json:"last_status,omitempty"`
	LastExecutionID string    `json:"last_execution_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}
```

（沿用原文逐字段手写风格，不强求 type 列对齐——gofmt 不管 struct 字段对齐。）

- [ ] **Step 2: 编译验证**

Run: `go build ./...`
Expected: 通过（仅加字段，不影响行为）

- [ ] **Step 3: 跑现有测试，确保无回归**

Run: `go test ./cmd/server/... ./internal/backend/... 2>&1 | tail -30`
Expected: 全部通过

- [ ] **Step 4: 提交**

```bash
git add internal/backend/models.go
git commit -m "feat(backend): ScheduledTask 加 NextRunAt 字段（暂时未注入）"
```

---

## Task 2: 写 enabled + 合法 cron 的失败测试

**Files:**
- Create: `cmd/server/main_scheduled_test.go`

- [ ] **Step 1: 创建测试文件**

创建 `cmd/server/main_scheduled_test.go`：

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"github.com/xiaodongQ/xworkbench/internal/config"
)

// TestHandleScheduledList_NextRunAt_Enabled 验证 enabled 任务的 next_run_at 被注入且为未来时间。
func TestHandleScheduledList_NextRunAt_Enabled(t *testing.T) {
	s := newTestServer(t)
	now := time.Now()
	enabled := &backend.ScheduledTask{
		ID:          "sched-enabled-1",
		Name:        "每 5 分一次",
		CronExpr:    "*/5 * * * *",
		CommandType: "shell",
		Enabled:     true,
		TimeoutSec:  60,
		CreatedAt:   now,
	}
	if err := s.schedDB.Create(enabled); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/scheduled", s.handleScheduledList)
	req := httptest.NewRequest("GET", "/api/scheduled", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var list []*backend.ScheduledTask
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	got := list[0]
	if got.NextRunAt == nil {
		t.Fatalf("NextRunAt is nil; want non-nil for enabled task")
	}
	// 期望在 6 分钟以内（5 分 + 误差）
	if d := time.Until(*got.NextRunAt); d < 0 || d > 6*time.Minute {
		t.Errorf("NextRunAt in %v; want within 0..6min from now", d)
	}

	// 用 config 防止其他测试污染
	_ = config.AppConfig
}
```

- [ ] **Step 2: 跑测试，验证它失败（handler 还没实现注入）**

Run: `go test -run TestHandleScheduledList_NextRunAt_Enabled ./cmd/server/... -v`
Expected: FAIL — `NextRunAt is nil; want non-nil for enabled task`

- [ ] **Step 3: 提交失败的测试**

```bash
git add cmd/server/main_scheduled_test.go
git commit -m "test(server): 加 enabled 任务 next_run_at 注入测试（红）"
```

---

## Task 3: 在 `handleScheduledList` 注入 next_run_at

**Files:**
- Modify: [cmd/server/main.go:1637-1647](cmd/server/main.go#L1637)

- [ ] **Step 1: 编辑 handler 注入逻辑**

把 [cmd/server/main.go:1637-1647](cmd/server/main.go#L1637) 改成：

```go
func (s *APIServer) handleScheduledList(w http.ResponseWriter, r *http.Request) {
	list, err := s.schedDB.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*backend.ScheduledTask{}
	}
	// 注入下次执行时间（仅 enabled 任务）。复用 robfig/cron，与调度器同一 parser。
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	now := time.Now()
	for _, t := range list {
		if !t.Enabled {
			continue
		}
		sched, err := parser.Parse(t.CronExpr)
		if err != nil {
			continue // 解析失败留 nil，不阻断整列表
		}
		nxt := sched.Next(now)
		t.NextRunAt = &nxt
	}
	writeJSON(w, list)
}
```

- [ ] **Step 2: 确认 import 已有 `time` 和 `github.com/robfig/cron/v3`**

Run: `grep -nE '"time"|robfig/cron' cmd/server/main.go | head -5`
Expected: 两行都存在。如果 `time` 或 `cron` 未 import，在 import 块加上。

若 `cron` 未 import，import 块加：
```go
"github.com/robfig/cron/v3"
```

若 `time` 未 import，import 块加：
```go
"time"
```

- [ ] **Step 3: 跑 Task 2 的测试，验证它通过**

Run: `go test -run TestHandleScheduledList_NextRunAt_Enabled ./cmd/server/... -v`
Expected: PASS

- [ ] **Step 4: 跑全量测试确认无回归**

Run: `go test ./... 2>&1 | tail -20`
Expected: 全部通过

- [ ] **Step 5: 提交**

```bash
git add cmd/server/main.go
git commit -m "feat(server): handleScheduledList 注入 next_run_at（仅 enabled）"
```

---

## Task 4: 加 disabled 任务不显示 next_run_at 的测试

**Files:**
- Modify: `cmd/server/main_scheduled_test.go`

- [ ] **Step 1: 追加测试**

在 `cmd/server/main_scheduled_test.go` 末尾追加：

```go
// TestHandleScheduledList_NextRunAt_Disabled 验证 disabled 任务的 next_run_at 为 nil（字段不出现）。
func TestHandleScheduledList_NextRunAt_Disabled(t *testing.T) {
	s := newTestServer(t)
	disabled := &backend.ScheduledTask{
		ID:          "sched-disabled-1",
		Name:        "已禁用",
		CronExpr:    "*/5 * * * *",
		CommandType: "shell",
		Enabled:     false,
		TimeoutSec:  60,
		CreatedAt:   time.Now(),
	}
	if err := s.schedDB.Create(disabled); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/scheduled", s.handleScheduledList)
	req := httptest.NewRequest("GET", "/api/scheduled", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// 用 map 解码以验证字段是否真的"不出现"（omitempty 行为）
	var list []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}
	if _, ok := list[0]["next_run_at"]; ok {
		t.Errorf("next_run_at should be omitted for disabled task; got %v", list[0]["next_run_at"])
	}
}
```

- [ ] **Step 2: 跑测试**

Run: `go test -run TestHandleScheduledList_NextRunAt_Disabled ./cmd/server/... -v`
Expected: PASS（Task 3 的实现已正确处理 disabled 分支）

- [ ] **Step 3: 提交**

```bash
git add cmd/server/main_scheduled_test.go
git commit -m "test(server): disabled 任务 next_run_at 字段不出现"
```

---

## Task 5: 加非法 cron 不阻断整列表的测试

**Files:**
- Modify: `cmd/server/main_scheduled_test.go`

- [ ] **Step 1: 追加测试**

在 `cmd/server/main_scheduled_test.go` 末尾追加：

```go
// TestHandleScheduledList_NextRunAt_InvalidCron 验证非法 cron 不阻断整列表。
func TestHandleScheduledList_NextRunAt_InvalidCron(t *testing.T) {
	s := newTestServer(t)
	now := time.Now()
	bad := &backend.ScheduledTask{
		ID:          "sched-bad-1",
		Name:        "非法 cron",
		CronExpr:    "not a cron",
		CommandType: "shell",
		Enabled:     true,
		TimeoutSec:  60,
		CreatedAt:   now,
	}
	good := &backend.ScheduledTask{
		ID:          "sched-good-1",
		Name:        "合法 cron",
		CronExpr:    "0 9 * * 1-5",
		CommandType: "shell",
		Enabled:     true,
		TimeoutSec:  60,
		CreatedAt:   now,
	}
	if err := s.schedDB.Create(bad); err != nil {
		t.Fatalf("Create bad: %v", err)
	}
	if err := s.schedDB.Create(good); err != nil {
		t.Fatalf("Create good: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/scheduled", s.handleScheduledList)
	req := httptest.NewRequest("GET", "/api/scheduled", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var list []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}

	// bad 应该没有 next_run_at；good 应该有
	for _, item := range list {
		id, _ := item["id"].(string)
		_, has := item["next_run_at"]
		switch id {
		case "sched-bad-1":
			if has {
				t.Errorf("bad task should not have next_run_at; got %v", item["next_run_at"])
			}
		case "sched-good-1":
			if !has {
				t.Errorf("good task should have next_run_at")
			}
		}
	}
}
```

- [ ] **Step 2: 跑测试**

Run: `go test -run TestHandleScheduledList_NextRunAt_InvalidCron ./cmd/server/... -v`
Expected: PASS

- [ ] **Step 3: 提交**

```bash
git add cmd/server/main_scheduled_test.go
git commit -m "test(server): 非法 cron 不阻断整列表"
```

---

## Task 6: 加 `@every` 描述符支持的测试

**Files:**
- Modify: `cmd/server/main_scheduled_test.go`

- [ ] **Step 1: 追加测试**

在 `cmd/server/main_scheduled_test.go` 末尾追加：

```go
// TestHandleScheduledList_NextRunAt_EveryDescriptor 验证 @every 描述符被识别。
func TestHandleScheduledList_NextRunAt_EveryDescriptor(t *testing.T) {
	s := newTestServer(t)
	task := &backend.ScheduledTask{
		ID:          "sched-every-1",
		Name:        "每 30 秒",
		CronExpr:    "@every 30s",
		CommandType: "shell",
		Enabled:     true,
		TimeoutSec:  10,
		CreatedAt:   time.Now(),
	}
	if err := s.schedDB.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/scheduled", s.handleScheduledList)
	req := httptest.NewRequest("GET", "/api/scheduled", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var list []*backend.ScheduledTask
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}
	if list[0].NextRunAt == nil {
		t.Fatalf("NextRunAt is nil; @every should be parsed")
	}
	// 期望 30 秒 + 误差
	if d := time.Until(*list[0].NextRunAt); d < 25*time.Second || d > 35*time.Second {
		t.Errorf("NextRunAt in %v; want ~30s", d)
	}
}
```

- [ ] **Step 2: 跑测试**

Run: `go test -run TestHandleScheduledList_NextRunAt_EveryDescriptor ./cmd/server/... -v`
Expected: PASS

- [ ] **Step 3: 提交**

```bash
git add cmd/server/main_scheduled_test.go
git commit -m "test(server): @every 描述符解析"
```

---

## Task 7: 跑全量测试做最终验证

- [ ] **Step 1: 跑所有后端测试**

Run: `go test ./... 2>&1 | tail -30`
Expected: 全部通过；新加的 4 个 scheduled_list 测试都出现在列表里。

- [ ] **Step 2: 跑 build 确认无 lint 错误**

Run: `go build ./... 2>&1 | tail -10`
Expected: 无输出（成功）

- [ ] **Step 3: 跑 vet**

Run: `go vet ./... 2>&1 | tail -10`
Expected: 无输出

- [ ] **Step 4: 提交（如有自动 fix）**

如果 gofmt 提示格式问题：
```bash
gofmt -w cmd/server/main.go cmd/server/main_scheduled_test.go internal/backend/models.go
git add -u
git commit -m "style: gofmt"
```

---

## Task 8: 前端双行渲染"上次 / 下次"

**Files:**
- Modify: [cmd/server/static/js/views/automation.js:246-272](cmd/server/static/js/views/automation.js#L246)

- [ ] **Step 1: 改 loadScheduled 的表格行**

把 [cmd/server/static/js/views/automation.js:246-272](cmd/server/static/js/views/automation.js#L246) 的相关部分（`lastRun` 计算和"最近执行" `<td>`）改成：

替换前（line 247 和 265）：
```js
    const lastRun = s.last_run_at ? new Date(s.last_run_at).toLocaleString() : '-';
```

替换后：
```js
    const lastRun = s.last_run_at ? new Date(s.last_run_at).toLocaleString() : '-';
    const nextRun = (s.enabled && s.next_run_at)
      ? `<div style="font-size:10px;color:var(--text-secondary);margin-top:2px">下次 ${esc(new Date(s.next_run_at).toLocaleString())}</div>`
      : '';
```

替换前（line 265）：
```js
      <td style="font-size:11px;color:var(--text-secondary)">${lastRun}</td>
```

替换后：
```js
      <td style="font-size:11px;color:var(--text-secondary);vertical-align:top">${lastRun}${nextRun}</td>
```

- [ ] **Step 2: 手测：启动服务、创建任务、刷新页面看效果**

Run: `./scripts/run.sh --restart`
然后：
1. 浏览器打开自动化页（[http://localhost:8902](http://localhost:8902) → 自动化 tab）
2. 创建一个 enabled 定时任务（cron 设为 `*/5 * * * *`）
3. 列表应显示"上次 -"（新任务无 last_run_at）+ "下次 12:35:00" 之类
4. 停用该任务，刷新列表 → "下次" 消失，只剩"上次 -"

- [ ] **Step 3: 提交**

```bash
git add cmd/server/static/js/views/automation.js
git commit -m "feat(ui): 定时任务列表显示下次执行时间（仅 enabled）"
```

---

## Task 9: 端到端冒烟（可选但推荐）

- [ ] **Step 1: 跑项目自带 e2e 脚本**

Run: `./scripts/e2e.sh basic 2>&1 | tail -30`
Expected: 全部 PASS（验证现有 scheduled API 不被破坏）

- [ ] **Step 2: 如果有 fail，先看是否与本改动相关**

若 fail 与 next_run_at 相关，按错误信息修。若不相关，记录在案即可（可能是已有 flakiness）。

---

## 完成标准

- [ ] `go test ./...` 全部通过
- [ ] `go build ./...` 成功
- [ ] 浏览器手测 enabled 任务显示"下次"，disabled 不显示
- [ ] git log 显示 7 个原子 commit（每个 Task 一个）
- [ ] 无残留 TODO / 调试代码
