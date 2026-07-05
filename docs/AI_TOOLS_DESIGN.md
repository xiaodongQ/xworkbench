# AI Tools 设计原则与规范

> 从第一性原理推导的 AI Function Calling 设计体系

---

## 核心原则

### P0：工具是 AI 的"四肢"，不是 API 的封装

**第一性原理**：AI 工具的本质是**扩展 AI 能采取的行动**。不是"把 HTTP API 包装成 tool"，而是"让 AI 能够执行某类动作"。

**推论**：
- 工具命名 = 动作命名，不是 URL 路径
- 工具描述 = AI 的"工具使用说明"，不是人类文档的缩写
- 工具返回 = AI 继续推理的上下文，不是操作日志

### P1：每个工具有且只有一种语义

一个 tool 应该对应 AI 能做的一件事。不能有歧义。

| 错误 | 正确 |
|------|------|
| `get_task` 同时查任务又查执行历史 | `get_task` 只查任务；`list_task_executions` 查执行历史 |
| `update_task` 能改 status/title/description | `update_task` 只做属性更新；`trigger_task` 触发执行 |
| `list_tasks` 返回中带执行统计 | `list_tasks` 只返回任务列表；`get_task_executions` 查执行 |

### P2：描述即 AI 的决策引导

工具的 `description` 不是"这个工具干什么"，而是"**在什么情况下 AI 应该调用这个工具**，以及调用后会发生什么"。

格式：
```
Use when: <场景描述>
Returns: <返回值含义>
Warning: <需要注意的边界情况>
```

---

## 工具命名规范（统一动词体系）

```
create_<noun>   → 新增资源
list_<noun>     → 列出资源列表（可过滤）
get_<noun>      → 获取单个资源详情
update_<noun>   → 更新资源属性
delete_<noun>   → 删除资源
search_<noun>   → 按关键词搜索资源
trigger_<verb>  → 触发一个动作
cancel_<verb>   → 取消一个进行中的动作
open_<noun>     → 执行一个外部可见的动作（打开文件/链接/终端）
```

**禁止**：
- `add_xxx`（用 `create_xxx`）
- `remove_xxx`（用 `delete_xxx`）
- `fetch_xxx`（用 `get_xxx`）

---

## 工具描述写作标准

每个工具的 `description` 必须包含三段：

```json
{
  "description": "Use when: <场景>\nReturns: <返回什么>\nWarning: <边界/注意点>"
}
```

### 写作原则

1. **"Use when"** — AI 决策核心。描述在什么用户话语/场景下应选这个工具
2. **"Returns"** — AI 看到返回值后如何继续推理。描述返回内容的结构和含义
3. **"Warning"** — 防止 AI 误用。边界条件、危险操作、常见错误

### 示例

**❌ 差**：
```json
"description": "搜索经验库"
```

**✅ 好**：
```json
"description": "Use when: 用户说'查一下有没有关于 X 的经验'或'有没有处理过 Y 问题'\nReturns: 匹配的经验列表，每条含 [模块] 场景描述\nWarning: 搜索无结果是正常的，不要认为系统出错"
```

---

## 返回值规范

### 成功时
返回**人类可读的结构化文本**，格式统一：

```
✅ <操作结果描述>: <关键信息>
<可选: 列表内容，每行一个>
```

**示例**：
```
✅ 任务已创建: 修复登录 bug (ID: task-20260705-xxx)
```

### 错误时
```
⚠️ <错误类型>: <具体错误原因>
💡 <可选: AI 建议的下一步>
```

### 列表类返回
```
📋 <资源类型> (<数量> 结果):
- <item1>
- <item2>
```

### 详情类返回
```
📋 <资源名称>
状态: <status> | 类型: <type> | 优先级: <priority>
<字段>: <值>
...
```

---

## 工具分类（Phase 1 + Phase 2）

### 域 A：任务 Task

| 工具 | 语义 | 危险度 |
|------|------|-------|
| `create_task` | 创建新任务 | 低 |
| `list_tasks` | 列出任务（过滤） | 低 |
| `get_task` | 查任务详情 | 低 |
| `update_task` | 更新任务属性 | 低 |
| `trigger_task` | 触发任务执行 | **中** |
| `list_task_executions` | 查执行历史 | 低 |
| `cancel_task` | 取消进行中的任务 | **中** |

### 域 B：目录快捷方式 DirShortcut

| 工具 | 语义 | 危险度 |
|------|------|-------|
| `create_dir_shortcut` | 创建快捷方式 | 低 |
| `list_dir_shortcuts` | 列出快捷方式 | 低 |
| `update_dir_shortcut` | 更新快捷方式 | 低 |
| `delete_dir_shortcut` | 删除快捷方式 | **高** |
| `open_dir_shortcut` | 文件管理器打开 | 低 |
| `open_dir_shortcut_terminal` | 终端打开 | 低 |

### 域 C：经验 Experience

| 工具 | 语义 | 危险度 |
|------|------|-------|
| `search_experiences` | 搜索经验 | 低 |
| `create_experience` | 创建经验 | 低 |
| `update_experience` | 更新经验 | 低 |
| `delete_experience` | 删除经验 | **高** |

### 域 D：链接 WebLink

| 工具 | 语义 | 危险度 |
|------|------|-------|
| `list_web_links` | 列出收藏链接 | 低 |
| `create_web_link` | 添加链接 | 低 |
| `update_web_link` | 更新链接 | 低 |
| `delete_web_link` | 删除链接 | **高** |
| `open_web_link` | 浏览器打开链接 | 中 |

### 域 E：轻量 Todo

| 工具 | 语义 | 危险度 |
|------|------|-------|
| `list_todos` | 列出 Todo | 低 |
| `add_todo` | 添加 Todo | 低 |
| `toggle_todo` | 切换完成状态 | 低 |
| `delete_todo` | 删除 Todo | **高** |

### 域 F：本地 Shell

| 工具 | 语义 | 危险度 |
|------|------|-------|
| `start_local_shell` | 启动本地 CLI 会话 | 低 |
| `run_local_command` | 在会话中执行命令 | **高** |

---

## AI 场景测试用例规范

每个工具必须有对应的 AI 场景测试。测试格式：

```go
// Scenario: <用户说法>
// Expected tool: <工具名>
// Expected params: <参数>
// Validation: <如何验证AI正确使用了工具>
```

### 测试用例矩阵（Phase 1 + 2）

| # | 场景（用户说） | 期望工具 | 期望行为 |
|---|-------------|---------|---------|
| T1 | "帮我建一个任务，标题是 XXX" | `create_task` | 任务被创建，返回 ID |
| T2 | "看看我有哪些任务" | `list_tasks` | 返回任务列表 |
| T3 | "任务 XXX 进展怎么样了" | `get_task` | 返回任务详情+执行历史 |
| T4 | "把任务状态改成进行中" | `update_task` | 状态更新成功 |
| T5 | "跑一下这个任务" | `trigger_task` | 执行触发，返回 execution_id |
| T6 | "执行历史给我看看" | `list_task_executions` | 返回执行列表 |
| T7 | "添加一个链接 XXX" | `create_web_link` | 链接创建成功 |
| T8 | "打开 XXX 链接" | `open_web_link` | 浏览器打开链接 |
| T9 | "新建一个 Todo" | `add_todo` | Todo 添加成功 |
| T10 | "帮我搜索下有没有 X 的经验" | `search_experiences` | 返回匹配经验 |
| T11 | "把这个经验记下来" | `create_experience` | 经验创建成功 |
| T12 | "创建一个目录快捷方式" | `create_dir_shortcut` | 快捷方式创建成功 |
| T13 | "在终端里打开这个目录" | `open_dir_shortcut_terminal` | 终端打开 |
| T14 | "更新任务 XXX 的优先级" | `update_task` | 优先级更新 |
| T15 | "删除这条经验" | `delete_experience` | 经验被删除 |
| T16 | "切换 Todo 第 3 项的状态" | `toggle_todo` | 完成状态切换 |

---

## 命名统一重构（当前→目标）

| 当前名称 | 目标名称 | 原因 |
|---------|---------|------|
| `run_task` | `trigger_task` | `run` 语义模糊，`trigger` 明确表示"启动执行" |
| `add_todo` | `add_todo` | 保持（`add` 在 Todo 域是标准用语） |
| `get_task_executions` | `list_task_executions` | 列表操作统一用 `list_` 前缀 |

---

## 危险操作处理

以下工具标记为 `dangerous=true`，前端应向用户确认：

- `delete_dir_shortcut` — 删除不可恢复
- `delete_web_link` — 删除不可恢复
- `delete_experience` — 删除不可恢复
- `delete_todo` — 删除不可恢复
- `run_local_command` — 执行任意命令
- `trigger_task` — 启动可能耗时/有副作用的执行
- `open_web_link` — 触发外部浏览器跳转

**实施方式**：在 Tool struct 中增加 `Dangerous bool` 字段，AI 系统提示词中明确告知这些需要先询问用户。

---

## System Prompt 指南

每个 AI 对话都应在 system prompt 中包含：

```
你有一个工具集，可以帮助你完成任务管理、链接收藏、经验沉淀等操作。

工具使用原则：
1. 优先用工具而非猜测。如果你不确定任务状态，先查再行动
2. 危险操作（删除/执行）必须先向用户确认
3. 列表类操作不带过滤条件时默认 limit=20
4. 搜索无结果是正常的，不要返回"系统错误"
5. 工具返回的信息有限时，用 get_<resource> 获取完整详情

危险工具（需确认）: delete_* 系列, trigger_task, open_web_link, run_local_command
```

---

## 未来扩展方向

1. **动态工具注册**：新 API 加 `@tool` 注解，自动生成工具定义（注解驱动的工具发现）
2. **工具使用审计**：记录 AI 调用了哪些工具、参数、结果，用于优化工具设计
3. **按场景裁剪工具集**：复杂任务只暴露相关工具子集，减少 context 压力
4. **结构化返回**：从文本返回迁移到 JSON 返回，配合轻量 schema 定义

---

*Last updated: 2026-07-05*
