# Todo 归档功能设计方案

## 1. 背景与目标

- **问题**：已完成 todo 堆积在活跃区，干扰查看当前任务
- **目标**：支持将已完成项归档到独立区域，页面可折叠查看，Markdown 文件保持人类可读
- **约束**：仅支持最外层（顶级）todo 项的归档/取消归档，不支持单独归档子项

---

## 2. Markdown 文件格式

### 2.1 整体结构

```markdown
# Todo

## 📋 活跃中

### 2026年7月
- [ ] 任务A created:2026-07-01 due:2026-07-15
  > 这是备注
- [ ] 任务B created:2026-07-05 due:2026-07-20
  - [ ] 子任务B1 created:2026-07-06
- [x] 任务C created:2026-07-03 due:2026-07-08
  > 已完成但未归档

### 2026年6月
- [ ] 任务D created:2026-06-28

### 未标注创建日期
- [ ] 任务E

---

## 📦 已归档

### 2026年7月
- [x] 归档任务1 created:2026-07-01 archived:2026-07-11 due:2026-07-10
  - [x] 子任务1-1 created:2026-07-02 archived:2026-07-11
- [x] 归档任务2 created:2026-07-05 archived:2026-07-11 due:2026-07-08

### 2026年6月
- [x] 归档任务3 created:2026-06-15 archived:2026-06-28 due:2026-06-25

### 未标注归档日期
- [x] 归档任务4 created:2026-05-01 archived:2026-05-10
```

### 2.2 活跃区规则

| 要素 | 说明 |
|------|------|
| 标题 | `## 📋 活跃中` |
| 月份分组 | `### YYYY年MM月`，按 `created:` 日期分组 |
| 无日期项 | 归入 `### 未标注创建日期` |
| 子项 | 跟随父项移动，不单独参与归档 |
| 分隔线 | `---`，固定格式，位于活跃区和归档区之间 |

### 2.3 归档区规则

| 要素 | 说明 |
|------|------|
| 标题 | `## 📦 已归档` |
| 月份分组 | `### YYYY年MM月`，按 `archived:` 日期分组 |
| 无日期项 | 归入 `### 未标注归档日期` |
| 子项 | 跟随父项移动，保留完整缩进结构 |

---

## 3. 元数据规范

### 3.1 新增字段

| 字段 | 格式 | 写入时机 | 说明 |
|------|------|----------|------|
| `created:YYYY-MM-DD` | `created:2026-07-01` | `AddAndWrite` 自动写入当天 | 创建日期，不可修改 |
| `archived:YYYY-MM-DD` | `archived:2026-07-11` | `ArchiveItem` 自动写入当天 | 归档日期 |

### 3.2 解析规则

- 正则：`created:(\d{4}-\d{2}-\d{2})` 和 `archived:(\d{4}-\d{2}-\d{2})`
- 解析时提取，BuildLine 时原样写出
- 现有无此字段的项，解析为 `""`（空），视为"未标注"
- `due:日期` 保持不变，不参与归档分类

---

## 4. 数据模型

### 4.1 Item 结构体变更

```go
type Item struct {
    LineNo    int      `json:"line_no"`
    Indent    string   `json:"indent"`
    Done      bool     `json:"done"`
    Text      string   `json:"text"`
    DueDate   string   `json:"due_date,omitempty"`
    Tags      []string `json:"tags,omitempty"`
    Note      string   `json:"note,omitempty"`
    Created   string   `json:"created,omitempty"`   // 新增：创建日期 YYYY-MM-DD
    Archived  string   `json:"archived,omitempty"`  // 新增：归档日期 YYYY-MM-DD
    Children  []*Item  `json:"children,omitempty"`
}
```

### 4.2 Section 划分

解析后返回两个独立切片：

```go
type ParsedSections struct {
    ActiveItems   []*Item  // 活跃区项（含未勾选 + 已勾选未归档）
    ArchivedItems  []*Item  // 归档区项（archived 非空）
}
```

---

## 5. API 设计

### 5.1 新增端点

| 方法 | 路由 | 说明 |
|------|------|------|
| `PUT` | `/api/todo/{line_no}/archive` | 归档指定项（仅顶级项） |
| `PUT` | `/api/todo/{line_no}/unarchive` | 取消归档（仅顶级项） |

### 5.2 Archive 行为

1. 校验是否为顶级项（Indent 为空或缩进为 0）——子项禁止归档
2. 在原位置移除该项（含所有子孙项 + 备注行）
3. 写入 `archived:今天日期` 到该项（含所有子孙项）
4. 将该项追加到归档区对应月份分组底部
5. 重新解析并返回完整 Sections

### 5.3 Unarchive 行为

1. 校验该项在归档区（archived 非空）
2. 从归档区移除该项（含所有子孙项 + 备注行）
3. 清除 `archived:xxx` 标签（子孙项同步清除）
4. 将该项追加到活跃区末尾
5. 重新解析并返回完整 Sections

### 5.4 错误处理

| 场景 | HTTP 状态码 | 错误信息 |
|------|-------------|----------|
| 子项尝试归档 | 400 | "子项不能单独归档" |
| 归档区项再次归档 | 400 | "该项已归档" |
| 活跃项取消归档 | 400 | "该项不在归档区" |
| line_no 不存在 | 404 | "todo 项不存在" |

---

## 6. 前端交互

### 6.1 页面布局

```
┌─ 活跃中 ──────────────────────────────────┐
│ [☐ 仅未完成] [▶ 展开所有] [+ 新增]         │
│                                           │
│ ▼ 2026年7月                                │
│   ☐ 任务A  📅 07-15                       │
│   ☐ 任务B  📅 07-20                       │
│     ☐ 子任务B1                            │
│                                           │
│ ▼ 2026年6月                                │
│   ☑ 任务C  📅 07-08  [归档]              │
│                                           │
│ ▼ 未标注创建日期                           │
│   ☐ 任务D                                 │
└───────────────────────────────────────────┘

┌─ 已归档 ───────────────────────────────────┐
│ [📦 显示/隐藏归档区]                        │
│                                           │
│ ▼ 2026年7月 (2)                           │
│   ☑ 归档任务1  📅 07-10  [↩ 恢复] [×]    │
│     ☑ 子任务1-1  [↩ 恢复] [×]            │
│   ☑ 归档任务2  📅 07-08  [↩ 恢复] [×]    │
│                                           │
│ ▼ 2026年6月 (1)                            │
│   ☑ 归档任务3  📅 06-25  [↩ 恢复] [×]    │
└───────────────────────────────────────────┘
```

### 6.2 操作按钮

| 按钮 | 位置 | 触发条件 |
|------|------|----------|
| 📦 归档 | 活跃区，已勾选项行末 | 仅顶级项（子项不显示） |
| ↩ 恢复 | 归档区，每项行末 | 任意归档项 |
| × 删除 | 归档区，每项行末 | 任意项 |

### 6.3 过滤状态

- **归档区默认折叠**，点标题栏展开/收起
- 展开时按 `archived:` 月份分组，显示该项数和操作按钮

---

## 7. 实现要点

### 7.1 解析层变更

- 新增 `ParseSections(content string) (*ParsedSections, error)`：返回 `ActiveItems` + `ArchivedItems`
- `Parse` 保持兼容，内部调用 `ParseSections`
- `ParseMetadata` 扩展：提取 `created:` 和 `archived:` 字段

### 7.2 写入层变更

- `WriteSections(path string, active []*Item, archived []*Item) error`
  - 写活跃区：按原顺序，保留月份分组标题
  - 写分隔线 + 归档标题
  - 写归档区：按 `archived:` 月份分组
- `ArchiveItem(path string, lineNo int) error`：整体移动，含子孙项
- `UnarchiveItem(path string, lineNo int) error`：整体恢复，清除 archived 标签
- `AddAndWrite` 变更：自动写入 `created:今天日期`

### 7.3 月份分组写入

写入时根据 `created:` / `archived:` 日期决定放到哪个月份分组：
- 有日期：找对应月份分组，不存在则新建
- 无日期：归到「未标注创建日期」或「未标注归档日期」分组
- 月份分组不存在时插入，存在时追加到该组底部

### 7.4 禁止子项单独归档

归档 API 入口检查：
```go
if item.Indent != "" {
    writeErr(w, 400, "子项不能单独归档")
    return
}
```

---

## 8. 兼容性

| 场景 | 处理方式 |
|------|----------|
| 现有 todo.md 无 created/archived 字段 | 解析时为空字符串，按"未标注"处理 |
| 旧文件无分隔线和分区标题 | 视为全为活跃区，归档时追加分隔线+归档区 |
| 混合结构（部分在归档区、部分不在） | 统一刷新：按规则重新写入，确保一致性 |

---

## 9. 交付物

- [ ] `internal/todo/parser.go` — 新增 `ParseSections`，扩展 `ParseMetadata`
- [ ] `internal/todo/writer.go` — 新增 `WriteSections` / `ArchiveItem` / `UnarchiveItem`
- [ ] `cmd/server/main.go` — 新增 `/api/todo/{line_no}/archive` 和 `/unarchive` handler
- [ ] `cmd/server/static/js/widgets.js` — 前端归档/恢复按钮 + 归档区折叠 UI
- [ ] `docs/TODO_ARCHIVE_DESIGN.md` — 本文档
