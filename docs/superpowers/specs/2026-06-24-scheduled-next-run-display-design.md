# 定时任务下次执行时间显示设计

## 背景

自动化页（`#page-automation`）的定时任务列表（[cmd/server/static/js/views/automation.js:246-272](cmd/server/static/js/views/automation.js#L246-L272)）当前 6 列：名称 / Cron / 类型 / 状态 / 最近执行 / 操作。

调度器已用 `robfig/cron/v3`（[internal/scheduler/scheduler.go:12](internal/scheduler/scheduler.go#L12)），但 `ScheduledTask` struct 没有 `NextRunAt` 字段，前端也无从展示。

用户场景：用户想快速看到"这个任务下一次什么时候跑"，尤其在排查"为什么没执行 / 是不是漏跑"时。

## 目标

对**启用**的定时任务，在不新增列的前提下，列表里直观显示下次执行时间。已禁用任务不显示（cron 里没 entry，下次时间无意义）。

## 约束

- 不新增列
- 不引第三方 JS 库（vanilla JS 项目，无构建）
- 不改首页 widget（YAGNI——widget 是摘要预览，已含 cron 表达式和状态）

## 设计

### 1. 数据层（后端）

`internal/backend/models.go`：
```go
type ScheduledTask struct {
    ID             string     `json:"id"`
    Name           string     `json:"name"`
    CronExpr       string     `json:"cron_expr"`
    CommandType    string     `json:"command_type"`
    Model          string     `json:"model,omitempty"`
    Prompt         string     `json:"prompt,omitempty"`
    WorkingDir     string     `json:"working_dir,omitempty"`
    Enabled        bool       `json:"enabled"`
    TimeoutSec     int        `json:"timeout_sec"`
    LastRunAt      *time.Time `json:"last_run_at,omitempty"`
    NextRunAt      *time.Time `json:"next_run_at,omitempty"` // 新增：仅 enabled 任务注入
    LastStatus     string     `json:"last_status,omitempty"`
    LastExecutionID string    `json:"last_execution_id,omitempty"`
    CreatedAt      time.Time  `json:"created_at"`
}
```

`cmd/server/main.go:handleScheduledList`（[main.go:1637-1647](cmd/server/main.go#L1637)）在返回前注入 next：
```go
parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
now := time.Now()
for _, t := range list {
    if !t.Enabled { continue }
    sched, err := parser.Parse(t.CronExpr)
    if err != nil { continue } // 解析失败留 nil，不阻断整列表
    nxt := sched.Next(now)
    t.NextRunAt = &nxt
}
```

复用 `robfig/cron` 现有依赖，**零新依赖**。与调度器用同一 parser，行为一致。

### 2. 前端渲染（主表格）

[cmd/server/static/js/views/automation.js:246-272](cmd/server/static/js/views/automation.js#L246-L272) 的"最近执行"列改为双行小字：

```js
// 旧
<td style="font-size:11px;color:var(--text-secondary)">${lastRun}</td>

// 新
const lastRunHtml = `${lastRun}`;
const nextRunHtml = (s.enabled && s.next_run_at)
  ? `<div style="font-size:10px;color:var(--text-secondary);margin-top:2px">下次 ${esc(new Date(s.next_run_at).toLocaleString())}</div>`
  : '';
// ...
<td style="font-size:11px;color:var(--text-secondary);vertical-align:top">${lastRunHtml}${nextRunHtml}</td>
```

显示规则：
- `enabled=true` 且 `next_run_at` 非空：显示"下次 ${本地化时间}"
- 其他情况（含已禁用、解析失败）：只显示"上次"

样式继承现有 11px / 10px secondary 色调，**不引新 CSS 类**。

### 3. 错误处理 / 边界

| 场景 | `NextRunAt` 值 | 前端显示 |
|---|---|---|
| `Enabled=false` | `nil` | 不显示"下次" |
| `Enabled=true` 且 `CronExpr` 解析失败 | `nil` | 不显示"下次" |
| `Enabled=true` 且 `CronExpr` 合法 | `time.Time` | 显示"下次" |
| `CronExpr = "@every 30s"` | `time.Time` | 显示"下次"（`robfig/cron` 支持） |
| 列表为空 | 不影响 | "暂无定时任务" 占位文案 |

cron 解析失败不返回错误——不阻断整列表（部分坏任务不应让接口 500）。`NextRunAt=nil` 配合 `omitempty` 自然不出现字段。

### 4. 测试

**单元测试**（`cmd/server/main_scheduled_test.go`，新文件）：
- `enabled=true` + 合法 cron（`*/5 * * * *`）→ 响应含 `next_run_at` 且在未来 6 分钟左右
- `enabled=true` + 合法 cron（`@every 30s`）→ 响应含 `next_run_at` 且在未来 35 秒内
- `enabled=true` + 非法 cron（`not a cron`）→ 响应不含 `next_run_at` 字段
- `enabled=false` → 响应不含 `next_run_at` 字段
- 解析失败不阻断整列表（一条坏 cron + 一条好 cron 都创建，列表应含一条 next_run_at + 一条不含）

**集成测试**：
- `GET /api/scheduled` 返回 JSON 的 shape 正确（所有 enabled 任务有 `next_run_at`，所有 disabled 任务无）

**端到端**（手动 + 可选 `scripts/e2e.sh`）：
- 启服务 → 创建 enabled 定时任务 → 进自动化页 → 看到"上次 / 下次"双行
- 停用该任务 → 刷新 → 只看到"上次"

## 范围

**做**：
- `internal/backend/models.go` 加 `NextRunAt` 字段
- `cmd/server/main.go:handleScheduledList` 注入计算
- `cmd/server/static/js/views/automation.js:loadScheduled` 双行渲染
- 单元测试 + 集成测试

**不做**：
- 首页 widget（`loadScheduledSummary`）改动
- 倒计时 / 相对时间显示（避免每秒重渲染）
- 自动刷新（依赖现有 WS 频道 `scheduled` + 手动刷新）
- 启用/禁用过渡动画
- cron 表达式的人话解释（"每 5 分钟"）—— YAGNI

## 关键文件

| 文件 | 改动 |
|---|---|
| [internal/backend/models.go:172-187](internal/backend/models.go#L172) | struct 加 `NextRunAt` 字段 |
| [cmd/server/main.go:1637-1647](cmd/server/main.go#L1637) | handler 注入 next |
| [cmd/server/static/js/views/automation.js:246-272](cmd/server/static/js/views/automation.js#L246) | 表格"最近执行"列双行 |
| `cmd/server/main_scheduled_test.go` | 新建单元测试 |
