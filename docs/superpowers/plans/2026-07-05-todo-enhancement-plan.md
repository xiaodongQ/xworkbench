# Todo 增强功能实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Todo 功能添加截止日期、标签、详细备注支持，扩展 Markdown 行内语法，优化 UI 展示和交互。

**Architecture:** 基于扩展的 Markdown 行内语法（`due:YYYY-MM-DD tags:tag1,tag2`），Parser 层解析后透传到 API，前端渲染带日期标签和 tag chips 的列表，支持展开详情和过滤排序。

**Tech Stack:** Go (parser + API) / Vanilla JS + CSS (frontend) / Markdown 文件存储

---

## 文件结构

| 文件 | 职责 |
|---|---|
| `internal/todo/parser.go` | Item 结构体 + Parse + Write 逻辑 |
| `internal/todo/parser_test.go` | 单元测试 |
| `cmd/server/main.go` | handleTodo / handleTodoAdd HTTP handler |
| `cmd/server/ai_tools.go` | AI chat add_todo 工具 |
| `cmd/server/index.html` | todo widget DOM + 添加弹窗 HTML |
| `cmd/server/static/js/widgets.js` | todo 渲染 + 交互逻辑 |
| `cmd/server/static/css/base.css` | 过期红、tag chip、展开详情等样式 |

---

## Phase 1: Parser（核心解析）

### Task 1: 扩展 Item 结构体 + 基础解析

**Files:**
- Modify: `internal/todo/parser.go:12-17`

- [ ] **Step 1: 查看现有 Item 结构体和 Parse 函数**

Read `internal/todo/parser.go` lines 1-60, focusing on:
- `Item` struct 定义（行 12-17）
- `Parse` 函数签名和实现
- `itemRe` 正则表达式（行 19）

- [ ] **Step 2: 扩展 Item 结构体**

将 `Item` 改为：
```go
type Item struct {
    LineNo   int      `json:"line_no"`
    Indent   string   `json:"indent"`
    Done     bool     `json:"done"`
    Text     string   `json:"text"`
    DueDate  string   `json:"due_date,omitempty"`   // "YYYY-MM-DD"，""=未设
    Tags     []string `json:"tags,omitempty"`
    Note     string   `json:"note,omitempty"`
    Children []*Item  `json:"children,omitempty"`   // 前端用，不写入文件
}
```

- [ ] **Step 3: 添加元数据解析辅助函数**

在 `parser.go` 添加：

```go
// parseMetadata 从文本末尾提取 due: 和 tags: 元数据，返回 (cleanText, dueDate, tags)
func parseMetadata(text string) (string, string, []string) {
    var dueDate string
    var tags []string

    // 提取 due:YYYY-MM-DD 或 due:MM-DD
    if dueRe := regexp.MustCompile(`\s+due:(\d{4}-\d{2}-\d{2}|\d{2}-\d{2})\b`); dueRe.MatchString(text) {
        match := dueRe.FindStringSubmatch(text)
        dueDate = match[1]
        // 如果是 MM-DD 格式，补上当前年份
        if len(dueDate) == 5 {
            dueDate = fmt.Sprintf("%d-%s", time.Now().Year(), dueDate)
        }
        // 从原文中移除 due:... 部分
        text = dueRe.ReplaceAllString(text, "")
    }

    // 提取 tags:tag1,tag2
    if tagsRe := regexp.MustCompile(`\s+tags:([\w,]+)`); tagsRe.MatchString(text) {
        match := tagsRe.FindStringSubmatch(text)
        for _, t := range strings.Split(match[1], ",") {
            t = strings.TrimSpace(t)
            if t != "" {
                tags = append(tags, t)
            }
        }
        text = tagsRe.ReplaceAllString(text, "")
    }

    return strings.TrimSpace(text), dueDate, tags
}
```

- [ ] **Step 4: 修改 Parse 函数，调用 parseMetadata**

在 `Parse` 函数的 for 循环内，解析完 Indent/Done/Text 后，添加：
```go
text, dueDate, tags := parseMetadata(text)
item.DueDate = dueDate
item.Tags = tags
```

- [ ] **Step 5: 写测试验证**

```go
// internal/todo/parser_test.go 添加
func TestParseMetadata(t *testing.T) {
    tests := []struct {
        input    string
        expText  string
        expDue   string
        expTags  []string
    }{
        {"买牛奶 due:2026-07-08", "买牛奶", "2026-07-08", nil},
        {"购物 tags:personal,shopping", "购物", "", []string{"personal", "shopping"}},
        {"任务 due:2026-07-10 tags:work,urgent", "任务", "2026-07-10", []string{"work", "urgent"}},
        {"普通任务", "普通任务", "", nil},
    }
    for _, tt := range tests {
        text, due, tags := parseMetadata(tt.input)
        if text != tt.expText || due != tt.expDue || !equalTags(tags, tt.expTags) {
            t.Errorf("parseMetadata(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.input, text, due, tags, tt.expText, tt.expDue, tt.expTags)
        }
    }
}
```

- [ ] **Step 6: 运行测试**

```bash
go test ./internal/todo/ -v -run TestParseMetadata
```

Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add internal/todo/parser.go internal/todo/parser_test.go
git commit -m "feat(todo): extend Item struct with DueDate, Tags, Note fields and parseMetadata helper
```

---

### Task 2: 解析 Note（`>` 缩进行）

**Files:**
- Modify: `internal/todo/parser.go`

- [ ] **Step 1: 添加 Note 正则解析函数**

在 `parser.go` 添加：

```go
// parseNotes 扫描所有行，收集 > 开头的缩进行，关联到对应的 Item
// 返回 lineNo -> note 字符串的 map
func parseNotes(lines []string) map[int]string {
    notes := map[int]string{}
    var currentItemLine int

    for _, line := range lines {
        trimmed := strings.TrimPrefix(line, " ")
        // 检测是否是 > 开头的注释行
        if strings.HasPrefix(strings.TrimLeft(line, " "), ">") {
            noteText := strings.TrimLeft(line[len(line)-len(strings.TrimLeft(line, " ")):], " ")
            noteText = strings.TrimPrefix(noteText, ">")
            noteText = strings.TrimSpace(noteText)
            if currentItemLine > 0 {
                if existing, ok := notes[currentItemLine]; ok {
                    notes[currentItemLine] = existing + "\n" + noteText
                } else {
                    notes[currentItemLine] = noteText
                }
            }
        } else if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "-") {
            // 非空非任务行，重置（简化处理）
            // 实际上应该只在遇到新任务行时更新 currentItemLine
            // 这里用更好的方式：
        }
        // 检测是否是新的任务行（- [ ] 或 - [x]）
        if itemRe.MatchString(line) {
            // 提取行号
            for i, l := range lines {
                if l == line {
                    currentItemLine = i + 1 // 1-based
                    break
                }
            }
        }
    }
    return notes
}
```

**注意**：上述实现有问题，因为循环中无法直接知道当前行号。更好的方式是重写 Parse 函数同时返回行列表：

```go
// 重写 Parse 函数，同时收集 note 行
func Parse(content string) []*Item {
    lines := strings.Split(content, "\n")
    noteMap := parseNotes(lines)

    var items []*Item
    for lineNo, line := range lines {
        matches := itemRe.FindStringSubmatch(line)
        if len(matches) == 0 {
            continue
        }
        indent := matches[1]
        done := matches[2] != " "
        text := matches[3]

        // 提取元数据
        text, dueDate, tags := parseMetadata(text)

        item := &Item{
            LineNo:  lineNo + 1, // 1-based
            Indent:  indent,
            Done:    done,
            Text:    text,
            DueDate: dueDate,
            Tags:    tags,
            Note:    noteMap[lineNo+1],
        }
        items = append(items, item)
    }
    return items
}

// parseNotes 扫描所有行，收集 > 开头的缩进行，关联到对应的 Item
func parseNotes(lines []string) map[int]string {
    notes := map[int]string{}
    lastItemLine := 0

    for lineNo, line := range lines {
        trimmed := strings.TrimLeft(line, " ")
        isNote := strings.HasPrefix(trimmed, ">")
        isItem := itemRe.MatchString(line)

        if isItem {
            lastItemLine = lineNo + 1 // 1-based
        } else if isNote && lastItemLine > 0 {
            noteText := strings.TrimPrefix(trimmed, ">")
            noteText = strings.TrimSpace(noteText)
            if existing, ok := notes[lastItemLine]; ok {
                notes[lastItemLine] = existing + "\n" + noteText
            } else {
                notes[lastItemLine] = noteText
            }
        } else if !isItem && !isNote && strings.TrimSpace(line) != "" {
            // 其他非空行（如普通文本段落），不改变 lastItemLine
        }
    }
    return notes
}
```

- [ ] **Step 2: 运行现有测试确保未破坏**

```bash
go test ./internal/todo/ -v
```

Expected: 所有现有测试 PASS

- [ ] **Step 3: 添加 Note 解析测试**

```go
func TestParseNotes(t *testing.T) {
    content := `- [ ] 任务一
  > 这是备注第一行
  > 这是备注第二行
- [x] 任务二`

    items := Parse(content)
    if len(items) != 2 {
        t.Fatalf("expected 2 items, got %d", len(items))
    }
    if items[0].Note != "这是备注第一行\n这是备注第二行" {
        t.Errorf("item[0].Note = %q, want %q", items[0].Note, "这是备注第一行\n这是备注第二行")
    }
    if items[1].Note != "" {
        t.Errorf("item[1].Note = %q, want %q", items[1].Note, "")
    }
}
```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/todo/ -v -run TestParseNotes
```

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/todo/parser.go internal/todo/parser_test.go
git commit -m "feat(todo): parse note lines (indented > prefix) and associate with parent item
```

---

### Task 3: 构建父子任务树（Children）

**Files:**
- Modify: `internal/todo/parser.go`

- [ ] **Step 1: 添加 BuildTree 函数**

在 `parser.go` 添加：

```go
// BuildTree 将扁平 Item 列表按缩进构建为树结构
func BuildTree(items []*Item) []*Item {
    if len(items) == 0 {
        return items
    }

    var roots []*Item
    var stack []*Item // 用栈跟踪父级嵌套层次

    for _, item := range items {
        // 计算当前 indent 级别（2 空格 = 1 级）
        level := len(item.Indent) / 2

        // 弹出栈中深度 >= 当前级别的项
        for len(stack) > level {
            stack = stack[:len(stack)-1]
        }

        if len(stack) == 0 {
            // 顶级任务
            roots = append(roots, item)
        } else {
            // 作为栈顶的子任务
            parent := stack[len(stack)-1]
            parent.Children = append(parent.Children, item)
        }

        // 当前 item 入栈（它的子任务会在后面挂在它下面）
        stack = append(stack, item)
    }

    return roots
}
```

- [ ] **Step 2: 修改 ReadAndParse 调用 BuildTree**

```go
func ReadAndParse(path string) ([]*Item, error) {
    content, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    items := Parse(string(content))
    return BuildTree(items), nil
}
```

- [ ] **Step 3: 同样修改 Parse 函数返回 BuildTree 结果（或保持 Parse 返回扁平，ReadAndParse 调用 BuildTree）**

建议：保持 `Parse` 返回扁平列表（单元测试方便），`ReadAndParse` 调用 `BuildTree` 封装一层。

- [ ] **Step 4: 添加测试**

```go
func TestBuildTree(t *testing.T) {
    items := []*Item{
        {LineNo: 1, Indent: "", Text: "任务1", Children: nil},
        {LineNo: 2, Indent: "  ", Text: "子任务1.1", Children: nil},
        {LineNo: 3, Indent: "    ", Text: "子任务1.1.1", Children: nil},
        {LineNo: 4, Indent: "  ", Text: "子任务1.2", Children: nil},
        {LineNo: 5, Indent: "", Text: "任务2", Children: nil},
    }

    tree := BuildTree(items)

    if len(tree) != 2 {
        t.Errorf("expected 2 root items, got %d", len(tree))
    }
    if len(tree[0].Children) != 2 {
        t.Errorf("expected 2 children for task1, got %d", len(tree[0].Children))
    }
    if len(tree[0].Children[0].Children) != 1 {
        t.Errorf("expected 1 child for subtask 1.1, got %d", len(tree[0].Children[0].Children))
    }
}
```

- [ ] **Step 5: 运行测试**

```bash
go test ./internal/todo/ -v -run TestBuildTree
```

Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/todo/parser.go internal/todo/parser_test.go
git commit -m "feat(todo): build parent-child tree from indentation hierarchy
```

---

### Task 4: 更新 Write 逻辑保留元数据

**Files:**
- Modify: `internal/todo/parser.go`

- [ ] **Step 1: 查看现有 Write 相关函数**

Read `internal/todo/parser.go` lines 50-120, focusing on:
- `ToggleAndWrite` 函数
- `AddAndWrite` 函数
- `DeleteAndWrite` 函数
- 写回格式字符串

- [ ] **Step 2: 添加 itemToLine 函数（将 Item 转回 Markdown 行）**

```go
// itemToLine 将 Item 转换回 Markdown 行，保留元数据
func itemToLine(item *Item) string {
    done := " "
    if item.Done {
        done = "x"
    }

    text := item.Text
    // 追加 due date
    if item.DueDate != "" {
        text += " due:" + item.DueDate
    }
    // 追加 tags
    if len(item.Tags) > 0 {
        text += " tags:" + strings.Join(item.Tags, ",")
    }

    return fmt.Sprintf("%s- [%s] %s", item.Indent, done, text)
}
```

- [ ] **Step 3: 修改 ToggleAndWrite 保留元数据**

现有 `ToggleAndWrite` 大致逻辑是：
1. 解析文件
2. 找到对应行号的 item，更新 Done 状态
3. 重新拼接写回

修改第 3 步，用 `itemToLine` 而不是手动拼接：

```go
func ToggleAndWrite(path string, items []*Item) error {
    // ... 读取解析同上 ...

    // 重新生成内容
    lines := strings.Split(content, "\n")
    for i, item := range items {
        if item.LineNo-1 < len(lines) {
            lines[item.LineNo-1] = itemToLine(item)
        }
    }

    return atomicWrite(path, strings.Join(lines, "\n"))
}
```

- [ ] **Step 4: 修改 AddAndWrite**

`AddAndWrite` 目前是追加 `- [ ] text\n`。修改为支持元数据：

```go
func AddAndWrite(path string, text string, dueDate string, tags []string, note string) error {
    items, err := ReadAndParse(path)
    if err != nil {
        return err
    }

    newItem := &Item{
        LineNo:  len(items) + 1, // 将在最后一行后
        Indent:  "",
        Done:    false,
        Text:    text,
        DueDate: dueDate,
        Tags:    tags,
        Note:    note,
    }

    var newLines []string
    for _, item := range items {
        newLines = append(newLines, itemToLine(item))
        // 追加 note（如果有）
        if item.Note != "" {
            for _, noteLine := range strings.Split(item.Note, "\n") {
                newLines = append(newLines, item.Indent+"  > "+noteLine)
            }
        }
    }
    newLines = append(newLines, itemToLine(newItem))
    if note != "" {
        for _, noteLine := range strings.Split(note, "\n") {
            newLines = append(newLines, "  > "+noteLine)
        }
    }

    return atomicWrite(path, strings.Join(newLines, "\n"))
}
```

- [ ] **Step 5: 修改 DeleteAndWrite**

Delete 逻辑不变（按行号删），但写回时要保留元数据：

```go
func DeleteAndWrite(path string, lineNo int) error {
    items, err := ReadAndParse(path)
    if err != nil {
        return err
    }

    // 过滤掉目标行（但要保留原始行号对应的 item）
    var filtered []*Item
    for _, item := range items {
        if item.LineNo != lineNo {
            // 重新计算行号（删除行之后的下移）
            filtered = append(filtered, item)
        }
    }

    // 重新生成内容
    var lines []string
    for _, item := range filtered {
        lines = append(lines, itemToLine(item))
        if item.Note != "" {
            for _, noteLine := range strings.Split(item.Note, "\n") {
                lines = append(lines, item.Indent+"  > "+noteLine)
            }
        }
    }

    return atomicWrite(path, strings.Join(lines, "\n"))
}
```

**注意**：Delete 时 Note 行的处理比较复杂（Note 不是独立 Item 但占用行号）。更好的方式是：按行号删除，而非按 Item 删除。但这需要回到原始思路。

**简化方案**：Delete 直接操作行号，不走 Parse：

```go
func DeleteAndWrite(path string, lineNo int) error {
    content, err := os.ReadFile(path)
    if err != nil {
        return err
    }
    lines := strings.Split(string(content), "\n")

    // 找到 lineNo 行，删除它以及后续的 note 行（缩进更深的 > 开头行）
    if lineNo < 1 || lineNo > len(lines) {
        return fmt.Errorf("invalid line number: %d", lineNo)
    }

    targetIndent := ""
    // 第一次扫描确定目标行的 indent
    for _, line := range lines {
        if itemRe.MatchString(line) {
            targetIndent = itemRe.FindStringSubmatch(line)[1]
            break
        }
    }

    var newLines []string
    skip := false
    for i, line := range lines {
        lineNo_ := i + 1
        if lineNo_ == lineNo {
            skip = true
            continue
        }
        if skip {
            trimmed := strings.TrimLeft(line, " ")
            // 如果是 note 行（> 开头）且 indent 更深，跳过
            if strings.HasPrefix(trimmed, ">") && len(line)-len(trimmed) > len(targetIndent) {
                continue
            }
            // 如果是新任务行，停止 skip
            if itemRe.MatchString(line) {
                skip = false
            }
        }
        if !skip {
            newLines = append(newLines, line)
        }
    }

    return atomicWrite(path, strings.Join(newLines, "\n"))
}
```

这比较复杂，建议保持现有 DeleteAndWrite 逻辑（按行号删），因为 Delete 相对少见且用户通常不会在 Note 行操作。

- [ ] **Step 6: 添加元数据写回测试**

```go
func TestItemToLine(t *testing.T) {
    item := &Item{
        Indent: "",
        Done:   false,
        Text:   "买牛奶",
        DueDate: "2026-07-08",
        Tags:   []string{"personal", "shopping"},
    }
    line := itemToLine(item)
    expected := "- [ ] 买牛奶 due:2026-07-08 tags:personal,shopping"
    if line != expected {
        t.Errorf("itemToLine() = %q, want %q", line, expected)
    }
}
```

- [ ] **Step 7: 运行测试**

```bash
go test ./internal/todo/ -v
```

Expected: ALL PASS

- [ ] **Step 8: 提交**

```bash
git add internal/todo/parser.go internal/todo/parser_test.go
git commit -m "feat(todo): preserve due_date and tags on write operations, refactor itemToLine helper
```

---

## Phase 2: API Handler

### Task 5: 更新 handleTodo 返回扩展结构

**Files:**
- Modify: `cmd/server/main.go:2278-2310`（handleTodo 函数附近）

- [ ] **Step 1: 查看 handleTodo 函数**

Read `cmd/server/main.go` lines 2278-2310, focusing on:
- 当前返回的 JSON 结构
- `handleTodo` 函数的完整实现

- [ ] **Step 2: 确认 Parse 返回结构无需改动**

因为 `Item` 结构体已添加 JSON tag，序列化自动包含新字段。

- [ ] **Step 3: 确认 API 返回格式**

现有返回格式已经是 `{path, items}`，新字段会通过 `json:"due_date,omitempty"` 等自动包含。

- [ ] **Step 4: 验证编译**

```bash
go build ./cmd/server
```

Expected: 编译成功（无错误）

- [ ] **Step 5: 提交**

```bash
git add cmd/server/main.go
git commit -m "chore(todo): API already returns extended Item fields via JSON tags
```

---

### Task 6: 更新 handleTodoAdd 接收新字段

**Files:**
- Modify: `cmd/server/main.go:2316-2360`（handleTodoAdd 函数附近）

- [ ] **Step 1: 查看 handleTodoAdd 函数**

Read `cmd/server/main.go` lines 2316-2360, focusing on:
- 当前 request body 结构
- 调用 `todo.AddAndWrite` 的方式

- [ ] **Step 2: 修改 request body 解析**

将：
```go
var req struct { Text string `json:"text"` }
```

改为：
```go
var req struct {
    Text    string   `json:"text"`
    DueDate string   `json:"due_date"`
    Tags    []string `json:"tags"`
    Note    string   `json:"note"`
}
```

- [ ] **Step 3: 更新 AddAndWrite 调用**

将：
```go
if err := todo.AddAndWrite(path, req.Text); err != nil {
```

改为：
```go
if err := todo.AddAndWrite(path, req.Text, req.DueDate, req.Tags, req.Note); err != nil {
```

- [ ] **Step 4: 验证编译**

```bash
go build ./cmd/server
```

Expected: 编译成功

- [ ] **Step 5: 提交**

```bash
git add cmd/server/main.go
git commit -m "feat(todo): handleTodoAdd accepts due_date, tags, note fields
```

---

## Phase 3: AI 工具

### Task 7: 更新 add_todo AI 工具

**Files:**
- Modify: `cmd/server/ai_tools.go`（`add_todo` 工具定义和 execAddTodo 函数）

- [ ] **Step 1: 查看 add_todo 工具定义**

Read `cmd/server/ai_tools.go` lines 325-367（工具注册部分）和 lines 1133-1170（execAddTodo 实现）

- [ ] **Step 2: 更新工具 schema**

将 `add_todo` 的 schema 从：
```go
"add_todo": {
    Description: "...",
    Parameters: &jsonschema.Schema{
        Type: jsonschema.Object,
        Properties: map[string]jsonschema.Schema{
            "text": {Type: jsonschema.String, Description: "..."},
        },
        Required: []string{"text"},
    },
},
```

改为：
```go
"add_todo": {
    Description: "添加新任务到 todo.md...",
    Parameters: &jsonschema.Schema{
        Type: jsonschema.Object,
        Properties: map[string]jsonschema.Schema{
            "text":    {Type: jsonschema.String, Description: "任务标题"},
            "due_date": {Type: jsonschema.String, Description: "截止日期 YYYY-MM-DD 或 MM-DD"},
            "tags":    {Type: jsonschema.String, Description: "逗号分隔的标签，如 personal,shopping"},
            "note":    {Type: jsonschema.String, Description: "详细备注（可选）"},
        },
        Required: []string{"text"},
    },
},
```

- [ ] **Step 3: 更新 execAddTodo 实现**

修改 `execAddTodo` 函数，解析 tags 字符串为切片：

```go
func execAddTodo(ctx context.Context, argsJSON string) string {
    var args struct {
        Text    string `json:"text"`
        DueDate string `json:"due_date"`
        Tags    string `json:"tags"` // 逗号分隔字符串
        Note    string `json:"note"`
    }
    json.Unmarshal([]byte(argsJSON), &args)
    if args.Text == "" {
        return "⚠️ text 是必填字段"
    }

    // 解析 tags
    var tagsList []string
    if args.Tags != "" {
        for _, t := range strings.Split(args.Tags, ",") {
            t = strings.TrimSpace(t)
            if t != "" {
                tagsList = append(tagsList, t)
            }
        }
    }

    path := todoMDPath()
    if path == "" {
        return "⚠️ Todo 路径未配置（todo_md_path）"
    }
    if err := todo.AddAndWrite(path, args.Text, args.DueDate, tagsList, args.Note); err != nil {
        return fmt.Sprintf("添加失败: %v", err)
    }
    return fmt.Sprintf("✅ 已添加: %s", args.Text)
}
```

- [ ] **Step 4: 验证编译**

```bash
go build ./cmd/server
```

Expected: 编译成功

- [ ] **Step 5: 提交**

```bash
git add cmd/server/ai_tools.go
git commit -m "feat(todo): add_todo AI tool supports due_date, tags, note parameters
```

---

## Phase 4: 前端 Widget

### Task 8: 更新 Todo 列表渲染（日期标签 + Tag Chips）

**Files:**
- Modify: `cmd/server/static/js/widgets.js:388-400`（loadTodo 函数渲染部分）
- Modify: `cmd/server/static/css/base.css`

- [ ] **Step 1: 查看现有 loadTodo 渲染逻辑**

Read `cmd/server/static/js/widgets.js` lines 388-410, focusing on:
- `loadTodo` 函数渲染逻辑
- `todo-item` 的 DOM 结构

- [ ] **Step 2: 更新渲染逻辑**

将渲染部分从：
```js
`<div class="todo-item ${i.done?'done':''}">
  <input type="checkbox" ${i.done?'checked':''} onchange="toggleTodo(${i.line_no}, this.checked)">
  <span class="todo-text">${esc(i.text)}</span>
  <span class="todo-del" onclick="deleteTodoItem(${i.line_no})" title="删除">×</span>
</div>`
```

改为：
```js
// 计算过期状态
const today = new Date().toISOString().split('T')[0];
const isOverdue = i.due_date && !i.done && i.due_date < today;
const dueLabel = i.due_date ? i.due_date.slice(5) : ''; // MM-DD 格式

let extraHtml = '';
if (i.due_date) {
    extraHtml += `<span class="todo-due ${isOverdue ? 'overdue' : ''}" title="${i.due_date}">📅 ${dueLabel}</span>`;
}
if (i.tags && i.tags.length) {
    extraHtml += i.tags.map(t => `<span class="todo-tag">#${esc(t)}</span>`).join('');
}

`<div class="todo-item ${i.done?'done':''} ${isOverdue?'overdue-row':''}" onclick="toggleTodoExpand(this, ${i.line_no})">
  <input type="checkbox" ${i.done?'checked':''} onchange="event.stopPropagation(); toggleTodo(${i.line_no}, this.checked)">
  <span class="todo-text">${esc(i.text)}</span>
  ${extraHtml}
  <span class="todo-del" onclick="event.stopPropagation(); deleteTodoItem(${i.line_no})" title="删除">×</span>
</div>`
```

- [ ] **Step 3: 添加 CSS 样式**

在 `base.css` 添加：

```css
/* Todo 增强样式 */
.todo-due {
    font-size: 11px;
    color: var(--text-secondary);
    background: var(--bg-tertiary);
    padding: 1px 5px;
    border-radius: 3px;
    margin-left: 4px;
}
.todo-due.overdue {
    color: #e53e3e;
    background: #fed7d7;
}
.overdue-row {
    background: #fff5f5;
}
.todo-tag {
    font-size: 11px;
    color: var(--accent-color, #3182ce);
    background: var(--bg-tertiary);
    padding: 1px 5px;
    border-radius: 3px;
    margin-left: 4px;
}
```

- [ ] **Step 4: 提交**

```bash
git add cmd/server/static/js/widgets.js cmd/server/static/css/base.css
git commit -m "feat(todo): render due date labels and tag chips in widget list
```

---

### Task 9: 添加 Todo 展开详情态

**Files:**
- Modify: `cmd/server/static/js/widgets.js`（添加展开逻辑）
- Modify: `cmd/server/index.html`（添加详情展开 DOM 模板）
- Modify: `cmd/server/static/css/base.css`

- [ ] **Step 1: 添加 toggleTodoExpand 函数**

在 `widgets.js` 添加：

```js
// 存储当前展开的行号
let _expandedTodoLine = null;

function toggleTodoExpand(el, lineNo) {
    // 关闭已展开的
    const existing = document.querySelector('.todo-detail-expanded');
    if (existing) {
        existing.remove();
        if (_expandedTodoLine === lineNo) {
            _expandedTodoLine = null;
            return;
        }
    }

    _expandedTodoLine = lineNo;

    // 找到对应 item 数据（需要从之前加载的数据中找）
    const itemData = findTodoItem(lineNo);
    if (!itemData) return;

    const dueInfo = itemData.due_date
        ? `📅 截止：${itemData.due_date}${isOverdue(itemData.due_date) ? '（已过期）' : '（还有 ' + daysUntil(itemData.due_date) + ' 天）'}`
        : '';
    const tagsInfo = itemData.tags && itemData.tags.length
        ? '标签：' + itemData.tags.map(t => '#' + t).join(' ')
        : '';
    const noteInfo = itemData.note
        ? `<div class="todo-detail-note"><div class="todo-detail-label">备注：</div><div class="todo-detail-note-text">${esc(itemData.note)}</div></div>`
        : '';
    const childrenHtml = itemData.children && itemData.children.length
        ? itemData.children.map(c => {
            const cDue = c.due_date ? `<span class="todo-due ${isOverdue(c.due_date) ? 'overdue' : ''}">📅 ${c.due_date.slice(5)}</span>` : '';
            return `<div class="todo-child-item">
              <input type="checkbox" ${c.done?'checked':''} onchange="toggleTodo(${c.line_no}, this.checked)">
              <span>${esc(c.text)}</span>${cDue}
            </div>`;
          }).join('')
        : '';

    const detailHtml = `
    <div class="todo-detail-expanded">
      <div class="todo-detail-content">
        ${dueInfo ? `<div class="todo-detail-due">${dueInfo}</div>` : ''}
        ${tagsInfo ? `<div class="todo-detail-tags">${tagsInfo}</div>` : ''}
        ${noteInfo}
        ${childrenHtml ? `<div class="todo-detail-children"><div class="todo-detail-label">子任务：</div>${childrenHtml}</div>` : ''}
      </div>
    </div>`;

    el.insertAdjacentHTML('afterend', detailHtml);
}

function findTodoItem(lineNo) {
    // items 数据需要从 loadTodo 时保存下来
    if (!window._todoItems) return null;
    for (const item of window._todoItems) {
        if (item.line_no === lineNo) return item;
        // 搜索 children
        if (item.children) {
            for (const child of item.children) {
                if (child.line_no === lineNo) return child;
            }
        }
    }
    return null;
}

function isOverdue(dueDate) {
    return dueDate && dueDate < new Date().toISOString().split('T')[0];
}

function daysUntil(dueDate) {
    const due = new Date(dueDate);
    const today = new Date();
    today.setHours(0,0,0,0);
    due.setHours(0,0,0,0);
    return Math.ceil((due - today) / (1000 * 60 * 60 * 24));
}
```

- [ ] **Step 2: 修改 loadTodo 保存数据**

在 `loadTodo` 函数开头添加：
```js
window._todoItems = data.items || [];
```

- [ ] **Step 3: 添加 CSS 样式**

```css
.todo-detail-expanded {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 4px;
    padding: 10px;
    margin: 4px 0 4px 24px;
}
.todo-detail-content {
    font-size: 13px;
}
.todo-detail-due {
    color: var(--text-secondary);
    margin-bottom: 4px;
}
.todo-detail-tags {
    color: var(--accent-color, #3182ce);
    margin-bottom: 4px;
}
.todo-detail-note {
    margin: 8px 0;
    padding-top: 8px;
    border-top: 1px solid var(--border-color);
}
.todo-detail-label {
    color: var(--text-secondary);
    font-size: 12px;
    margin-bottom: 4px;
}
.todo-detail-note-text {
    white-space: pre-wrap;
    line-height: 1.5;
}
.todo-detail-children {
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--border-color);
}
.todo-child-item {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 2px 0;
    margin-left: 12px;
}
```

- [ ] **Step 4: 提交**

```bash
git add cmd/server/static/js/widgets.js cmd/server/static/css/base.css
git commit -m "feat(todo): add inline expand detail view with due date, tags, note, children
```

---

### Task 10: 更新添加弹窗（支持截止日期、标签、备注）

**Files:**
- Modify: `cmd/server/index.html`（添加弹窗表单）
- Modify: `cmd/server/static/js/widgets.js`（表单提交逻辑）
- Modify: `cmd/server/static/css/base.css`

- [ ] **Step 1: 查看现有添加弹窗 HTML**

Read `cmd/server/index.html` lines 943-953（`#todo-add-modal`）

- [ ] **Step 2: 扩展添加弹窗表单**

将单 input 扩展为：
```html
<div id="todo-add-modal" class="modal hidden">
  <div class="modal-content" style="min-width:400px">
    <div class="modal-header">添加任务<span class="modal-close" onclick="closeTodoAddModal()">×</span></div>
    <div class="modal-body">
      <div class="form-group">
        <label>任务标题 <span style="color:red">*</span></label>
        <input id="todo-add-title" placeholder="任务内容" style="width:100%;box-sizing:border-box">
      </div>
      <div class="form-group">
        <label>截止日期</label>
        <input id="todo-add-due" type="date" style="width:100%;box-sizing:border-box">
      </div>
      <div class="form-group">
        <label>标签 <span style="color:var(--text-secondary);font-weight:normal">(逗号分隔)</span></label>
        <input id="todo-add-tags" placeholder="personal, shopping" style="width:100%;box-sizing:border-box">
      </div>
      <div class="form-group">
        <label>备注</label>
        <textarea id="todo-add-note" placeholder="详细描述..." rows="3" style="width:100%;box-sizing:border-box"></textarea>
      </div>
    </div>
    <div class="modal-footer">
      <button onclick="submitTodoAdd()" class="btn-primary">添加</button>
      <button onclick="closeTodoAddModal()" class="btn-secondary">取消</button>
    </div>
  </div>
</div>
```

- [ ] **Step 3: 更新 submitTodoAdd 函数**

```js
async function submitTodoAdd() {
    const text = document.getElementById('todo-add-title').value.trim();
    if (!text) { alert('请输入任务标题'); return; }

    const dueDate = document.getElementById('todo-add-due').value;
    const tags = document.getElementById('todo-add-tags').value.trim();
    const note = document.getElementById('todo-add-note').value.trim();

    const body = { text };
    if (dueDate) body.due_date = dueDate;
    if (tags) body.tags = tags;
    if (note) body.note = note;

    const r = await fetch('/api/todo', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    });

    closeTodoAddModal();
    loadTodo();
}
```

- [ ] **Step 4: 更新 closeTodoAddModal**

```js
function closeTodoAddModal() {
    document.getElementById('todo-add-modal').classList.add('hidden');
    // 清空表单
    document.getElementById('todo-add-title').value = '';
    document.getElementById('todo-add-due').value = '';
    document.getElementById('todo-add-tags').value = '';
    document.getElementById('todo-add-note').value = '';
}
```

- [ ] **Step 5: 提交**

```bash
git add cmd/server/index.html cmd/server/static/js/widgets.js
git commit -m "feat(todo): add modal supports due_date, tags, note input fields
```

---

### Task 11: 添加排序和过滤

**Files:**
- Modify: `cmd/server/static/js/widgets.js`

- [ ] **Step 1: 在 widget 标题栏添加过滤按钮**

在 index.html 的 todo widget 标题栏添加过滤按钮（现有 ☑ 按钮旁边）：
```html
<button id="todo-filter-btn" onclick="toggleTodoFilter()" title="过滤">🔍</button>
```

- [ ] **Step 2: 添加过滤状态和过滤逻辑**

```js
let _todoFilter = { showDone: false, tag: '', overdueOnly: false };

function toggleTodoFilter() {
    // 简单循环切换：all -> only overdue -> only with tag -> all
    // 可以用 dropdown menu 实现更复杂逻辑
}

function getFilteredItems(items) {
    let filtered = [];
    for (const item of items) {
        if (!_todoFilter.showDone && item.done) continue;
        if (_todoFilter.overdueOnly && (!item.due_date || item.due_date >= today)) continue;
        filtered.push(item);
    }
    return filtered;
}
```

- [ ] **Step 3: 在 loadTodo 中应用过滤**

在渲染前调用 `getFilteredItems`，排序后渲染。

- [ ] **Step 4: 添加排序**

```js
function sortTodoItems(items) {
    const today = new Date().toISOString().split('T')[0];
    return [...items].sort((a, b) => {
        // overdue 优先
        const aOverdue = a.due_date && !a.done && a.due_date < today;
        const bOverdue = b.due_date && !b.done && b.due_date < today;
        if (aOverdue && !bOverdue) return -1;
        if (!aOverdue && bOverdue) return 1;

        // 按日期排序
        if (a.due_date && b.due_date) {
            if (a.due_date !== b.due_date) return a.due_date.localeCompare(b.due_date);
        }
        if (a.due_date && !b.due_date) return -1;
        if (!a.due_date && b.due_date) return 1;

        // 无日期的按创建顺序
        return a.line_no - b.line_no;
    });
}
```

- [ ] **Step 5: 提交**

```bash
git add cmd/server/static/js/widgets.js cmd/server/index.html
git commit -m "feat(todo): add sort by overdue/date and basic filter options
```

---

## 验证

### 手动测试步骤

1. **启动服务**
```bash
./scripts/run.sh
```

2. **创建带元数据的任务**
- 打开 todo widget，点击 `+ 添加`
- 填写标题、截止日期（选后天日期）、标签（`personal,shopping`）、备注
- 点击添加

3. **验证列表显示**
- 任务行显示：checkbox + 标题 + 📅 MM-DD + #personal #shopping
- 点击任务行展开详情，显示截止日期、标签、备注、子任务

4. **验证过期效果**
- 将截止日期设为昨天，刷新后应显示红色日期 + 浅红背景

5. **验证 AI 工具**
- 在 AI Chat 中说"添加一个明天截止的购物任务，标签 shopping"
- 验证 todo.md 文件中该任务行包含 `due:... tags:...`

### 回归测试

```bash
go test ./internal/todo/ -v
go test ./cmd/server/ -v -run Todo
```

---

## 实施检查清单

- [ ] Task 1: Item 结构体 + parseMetadata
- [ ] Task 2: Note 解析（> 缩进行）
- [ ] Task 3: BuildTree 父子任务
- [ ] Task 4: Write 保留元数据
- [ ] Task 5: handleTodo 返回扩展字段
- [ ] Task 6: handleTodoAdd 接收新字段
- [ ] Task 7: add_todo AI 工具扩展
- [ ] Task 8: 列表渲染日期+标签
- [ ] Task 9: 展开详情态
- [ ] Task 10: 添加弹窗表单
- [ ] Task 11: 排序过滤
