# xworkbench 代理层设计方案

## 1. Exec 命令执行接口

### 背景
xworkbench 作为 Windows Agent，接收 Linux 服务的请求，执行 Windows 本地操作。

### API 设计

```
POST /api/exec
```

**请求：**
```json
{
  "command": "powershell -Command 'Get-Process'",
  "cwd": "C:\\temp",
  "timeout_ms": 30000
}
```

**响应：**
```json
{
  "output": "...",
  "error_out": "...",
  "exit_code": 0,
  "duration_ms": 150
}
```

### 实现

| 文件 | 内容 |
|------|------|
| `internal/relay/exec.go` | ExecHandler，复用 `executor.Run` |

```go
// HandleExec 接收命令，在 Windows 上执行并返回结果
func (h *RelayHandler) HandleExec(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Command   string `json:"command"`
        Cwd      string `json:"cwd"`
        TimeoutMs int   `json:"timeout_ms"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    ctx, cancel := context.WithTimeout(r.Context(), time.Duration(req.TimeoutMs)*time.Millisecond)
    defer cancel()

    // 复用现有 executor.Run，支持流式输出
    result, err := executor.Run(ctx, parseShell(req.Command), req.Cwd, nil)
    // ...
}
```

---

## 2. Teambition MCP 集成

### 背景

- Teambition 官方提供 MCP 包 `@tng/teambition-openapi-mcp`
- 支持 ~40+ 工具，覆盖 project/task/worktime/contact
- xworkbench relay 层只需转发 AI 指令给 Teambition，MCP 工具抽象完美匹配

### 传输模式选择

| 模式 | 可用性 | 说明 |
|------|--------|------|
| stdio | ❌ | 需要双向管道，改动大 |
| **streamable-http** | ✅ 推荐 | MCP server 作为 HTTP 服务，xworkbench 直接 HTTP 调用 |

### 架构

```
OpenClaw / AI Agent
       ↓ HTTP
xworkbench (Windows)
       ↓ /api/mcp/call (代理)
teambition-mcp (:3000)
       ↓
Teambition OpenAPI
```

### MCP Server 启动

开机时自动拉起：
```bash
teambition-mcp mcp \
  -a <appId> \
  -s <appSecret> \
  -o <orgId> \
  -b https://open.teambition.com/api \
  -m streamable-http \
  -p 3000
```

### API 设计

```
POST /api/mcp/call
```

**请求：**
```json
{
  "method": "tools/call",
  "params": {
    "name": "createTaskV3",
    "arguments": {
      "projectId": "xxx",
      "title": "测试任务"
    }
  }
}
```

**响应：** 透传 MCP server 响应

### 实现

| 文件 | 内容 |
|------|------|
| `internal/relay/mcp.go` | MCP Handler，代理到本地 MCP server |
| `cmd/server/mcp_setup.go` | 启动时拉起 teambition-mcp 进程 |

```go
// HandleMCPCall 透传请求到本地 MCP server
func (h *MCPHandler) HandleMCPCall(w http.ResponseWriter, r *http.Request) {
    var req JSONRPCRequest
    json.NewDecoder(r.Body).Decode(&req)

    // 转发到本地 MCP server
    resp, err := http.Post("http://localhost:3000/mcp", "application/json", body)
    // ...
}
```

---

## 3. 消息转发框架（精简版）

### 保留内容

| 模块 | 说明 |
|------|------|
| `internal/relay/relay.go` | HTTP Handler（send/proxy/stats） |
| `internal/relay/repo.go` | SQLite relay_logs + RelayRepo 接口 |

### 移除内容（暂不合入）

- `internal/integration/registry.go` — adapter 全局注册表
- `internal/integration/dingtalk/` — 钉钉 adapter
- `internal/integration/teambition/` — Teambition adapter（改用 MCP）
- `internal/integration/repo.go` — RelayRepo 接口（移到 relay/repo.go）

---

## 4. 待确认

- [ ] MCP server 的 AppToken 模式是以应用身份操作
- [ ] per-user OAuth2 flow（用户扫码授权）需要确认官方是否支持
- [ ] MCP server 进程管理（拉起、监控、重启）

---

## 5. 实施顺序

1. **Exec 接口** — 最简单，先实现
2. **MCP 集成** — 等 Teambition MCP 确认
3. **消息转发框架** — relay 功能完善