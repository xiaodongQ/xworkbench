# CLI 命令格式

`internal/executor/runner/build.go` 的 `BuildCommand(typ, model, sessionID, prompt)` 构造 AI CLI 子进程命令（避免 shell 注入）。

## 三种 command_type

### 1. `claude`（Claude Code）

```bash
claude --print --verbose [--model <m>] [--session-id <sid>] "<prompt>"
```

- `--print` / `--verbose` 必带（headless 模式 + 流式输出）
- `--model <m>` 可选（`sonnet` / `opus` / `haiku`...）
- `--session-id <sid>` 可选（续接用）
- `<prompt>` 是任务的描述/输入

示例（来自 Plan § 三 Redis 样例）：
```bash
claude --print --verbose --model sonnet --session-id sess-abc "请分析 Redis 慢查询日志，给出优化建议"
```

### 2. `cbc` / `codebuddy`

```bash
cbc -p [--model <m>] "<prompt>"
# 或回落
codebuddy -p [--model <m>] "<prompt>"
```

- `-p` / `--print` headless 模式
- `--model <m>` 可选（cbc 支持的长选项）

**PATH 自动回落**：执行时 `exec.LookPath("cbc")` 找 cbc，找不到时回落 `codebuddy`，都没有时返回错误。

### 3. `shell`

```bash
sh -c "<prompt>"
```

- `<prompt>` 是任意 shell 命令
- 不要传特殊字符（已在 Go 层 list 化处理，**避免 shell 注入**）

## 4 种人工确认信号

`internal/executor/confirm.go` 检测 AI CLI 输出的 18 个中英文信号，命中时上层可调用 `/api/tasks/{id}/submit-input`：

```go
// 移植自 ai-task-system v2.4 cli_executor.py:62-115
var confirmSignals = []string{
    "?", "[Y/n]", "[是/否]", "[y/n]", "[Yes/No]",
    "是否要", "要不要", "是否需要", "请确认",
    "不确定", "需要更多信息", "请告诉我", "请选择",
    "Press Enter", "按 Enter", "输入选择",
    "Continue?", "Proceed?", "Confirm",
}

func NeedsUserInput(output string) bool { ... }
func ParseConfirmRequest(output string) map[string]any { ... }  // 提取 {"confirm_type": "single_choice", ...}
```

## 流式执行

```go
// internal/executor/exec.go
func Run(ctx context.Context, cmd []string, onChunk func(string)) (*Result, error)
// onChunk 收到 stdout/stderr 每段（stderr 加 "[err] " 前缀）
// ctx 取消会 kill 子进程（30 分钟默认超时）
```

每个 stdout 段同时通过 WebSocket 推 `exec` 频道：

```json
{"channel": "exec", "payload": {"execution_id": "...", "task_id": "...", "chunk": "line1\n"}}
```

## 平台差异

| 维度 | Unix（macOS/Linux） | Windows |
|---|---|---|
| 子进程创建 | `exec.CommandContext` | 同上，自动用 `CreateProcess` |
| 终止 | `SIGKILL` | `TerminateProcess` |
| PTY | ✅ creack/pty | ❌ stub（503） |
| Shell 命令 | `sh -c "..."` | 调 `cmd /c "..."`（后续可加） |
