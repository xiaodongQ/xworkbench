# Todo 增强功能设计

## 1. 背景与目标

当前 Todo 功能基于 Markdown 文件（`todo.md`），数据结构极简（`LineNo/Indent/Done/Text`），无法满足以下需求：
- 无法设置截止日期，过期任务无法提醒
- 无法对任务分类（标签）
- 无法记录详细描述/备注
- 任务一多难以快速定位

## 2. 扩展 Markdown 语法

### 2.1 行内元数据语法

```
- [ ] 任务标题 due:2026-07-10 tags:personal,shopping
  > 这是详细备注，可以是多行
  - [ ] 子任务 due:2026-07-08
```

**元数据位置**：在 `Text` 内容之后、换行之前，行内解析。

| 字段 | 语法 | 示例 |
|---|---|---|
| 截止日期 | `due:YYYY-MM-DD` 或 `due:MM-DD`（年可省略视为当年） | `due:2026-07-10` / `due:07-10` |
| 标签 | `tags:tag1,tag2,tag3` | `tags:work,shopping` |
| 详细备注 | 缩进 `>` 开头（独立行） | `  > 备注内容...` |

### 2.2 解析规则

1. 先按行解析出所有 Item（保留行号）
2. 对每行 Text 内容，依次提取 `due:...` 和 `tags:...`
3. `>` 开头的缩进行，关联到上一个非 `>` 的 Item 作为 Note
4. 子任务缩进继承父任务标签（子任务自身 tags 可覆盖父级）

### 2.3 向后兼容

- 现有 `todo.md` 无任何元数据，正常解析
- 新字段为空时 API 返回 `null` 或空数组，前端不展示

---

## 3. 数据结构

### 3.1 Go Model（`internal/todo/parser.go`）

```go
type Item struct {
    LineNo   int      `json:"line_no"`
    Indent   string   `json:"indent"`
    Done     bool     `json:"done"`
    Text     string   `json:"text"`
    DueDate  string   `json:"due_date,omitempty"`   // "YYYY-MM-DD"，""=未设
    Tags     []string `json:"tags,omitempty"`
    Note     string   `json:"note,omitempty"`
    Children []*Item  `json:"children,omitempty"`   // 前端折叠展示用
}
```

### 3.2 API 返回结构（`GET /api/todo`）

```json
{
  "path": "/path/to/todo.md",
  "items": [
    {
      "line_no": 1,
      "indent": "",
      "done": false,
      "text": "购买食材",
      "due_date": "2026-07-10",
      "tags": ["personal", "shopping"],
      "note": "",
      "children": [
        {
          "line_no": 2,
          "indent": "  ",
          "done": false,
          "text": "买牛奶",
          "due_date": "2026-07-08",
          "tags": [],
          "note": "",
          "children": []
        }
      ]
    }
  ]
}
```

---

## 4. UI 设计

### 4.1 Widget 列表态（默认视图）

```
┌─ 待办 ─────────────────── [+ 添加][设置][☐] ─┐
│ ☐ 购买食材      📅 07-10  [#personal]        │
│   ☐ 买牛奶      📅 07-08                    │
│   ☐ 买面包                               │
│ ☐ 过期任务      📅 07-01  [已过期标红]      │
│ ☑ 已完成       📅 07-05                    │
└───────────────────────────────────────────┘
```

**展示规则**：
- 每行：checkbox + text + (due_date 标签) + (tags chips)
- 截止日期格式：`MM-DD`，hover 显示完整 `YYYY-MM-DD`
- 过期（due < today）：日期红色 + 行背景浅红
- 标签：小型彩色 chip，`#tagname` 格式
- 子任务缩进在父任务下方，展开时显示

### 4.2 展开详情态（点击任务行）

点击任务行展开详情面板（inline 展开，不弹 modal）：

```
┌─ 任务详情 ──────────────────────────────────┐
│ ☐ 购买食材                                   │
│ 📅 截止：2026-07-10（还有 5 天）              │
│ 标签：#personal #shopping                   │
│ ────────────────────────────────────────── │
│ 备注：                                       │
│ 这是一条详细备注，可以是多行                  │
│ ────────────────────────────────────────── │
│ 子任务：                                     │
│   ☐ 买牛奶  📅 07-08                        │
│   ☐ 买面包                                   │
└─────────────────────────────────────────────┘
```

### 4.3 添加/编辑弹窗

添加新任务时弹窗包含：
- 任务标题（必填）
- 截止日期（可选，date picker）
- 标签（可选，逗号分隔或 chip 输入）
- 备注（可选，多行文本）

---

## 5. 排序逻辑

默认排序规则：
1. **overdue**（已过期未完成）→ **today** → **upcoming**（未来）
2. 同组内按 `due_date` 升序
3. 无截止日期的任务排在最后

过滤选项：
- 全部 / 仅未完成 / 仅已完成
- 按标签过滤
- 按过期状态过滤

---

## 6. 实现计划

### Phase 1：核心解析（parser.go）
- 扩展 `Item` 结构体
- 重写/扩展正则解析，支持 `due:` 和 `tags:` 行内语法
- 解析 `>` 缩进行为 Note
- 更新 `Write`/`ToggleAndWrite` 保留元数据

### Phase 2：API 适配（main.go）
- `handleTodo` 返回扩展字段
- `handleTodoAdd` 接收 `due_date`/`tags`/`note` 参数
- `handleTodoToggle` 不变

### Phase 3：前端 Widget（widgets.js + index.html）
- 列表态渲染：日期标签（标红过期）+ tag chips + 子任务缩进
- 点击展开详情态
- 添加/编辑弹窗（带日期选择器、标签输入）
- 排序和过滤逻辑

### Phase 4：AI 工具（ai_tools.go）
- `add_todo` 支持 `due_date`/`tags`/`note` 参数
- `list_todos` 返回格式不变（JSON 透传）

---

## 7. 关键文件

| 文件 | 改动 |
|---|---|
| `internal/todo/parser.go` | Item 结构体 + 解析/写回逻辑 |
| `internal/todo/parser_test.go` | 新增测试用例 |
| `cmd/server/main.go` | handleTodo* 适配扩展字段 |
| `cmd/server/ai_tools.go` | add_todo 工具扩展参数 |
| `cmd/server/index.html` | 添加弹窗 + 展开详情 DOM |
| `cmd/server/static/js/widgets.js` | 渲染 + 交互逻辑 |
| `cmd/server/static/css/base.css` | 样式（过期红、tag chip 等） |

---

## 8. 风险与注意事项

1. **Markdown 格式兼容性**：用户手动编辑 `todo.md` 时需遵循格式，否则解析失败静默忽略该字段
2. **写回一致性**：元数据写在 Text 同一行，修改时用正则替换而非重新拼接，避免格式漂移
3. **性能**：每次 `loadTodo` 全量解析文件，需确保正则高效
4. **向后兼容**：无元数据的任务正常展示，新字段为空
