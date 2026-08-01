## Context

xworkbench 工作台已有 91 个 HTTP API，提供 task/exec/experience/scheduled/todo 等完整 CRUD + 执行控制能力。但这些 API 缺乏面向 AI Agent 的友好接口。

**现状**：
- Agent 需要理解 API 结构才能调用
- 缺乏统一的 CLI 工具封装
- 现有的 `xwcli` 是 remote agent daemon，不适合直接工具调用

**约束**：
- 自完备分发（手动拷贝到 Agent tools 目录）
- 无需 HTTP API 分发机制
- Go 单二进制实现，无外部依赖

## Goals / Non-Goals

**Goals:**
- 提供 38 个 CLI 子命令，覆盖完整 CRUD + 执行控制 + 配置管理
- 统一的 JSON 输出格式
- 简单部署：拷贝目录即可

**Non-Goals:**
- 不修改现有 HTTP API
- 不实现远程 agent 协议
- 不提供 HTTP 下载接口（自完备分发）

## Decisions

### 1. CLI 框架选择

**决定**：使用 Go 标准库 `flag` + 手动子命令解析

**理由**：
- 避免引入外部依赖（cobra 等）
- 命令结构简单，子命令数量固定（38 个）
- 保持二进制体积最小

**替代考虑**：
- Cobra/CLI：功能丰富但增加依赖，本项目不需要

### 2. HTTP 客户端

**决定**：使用 Go 标准库 `net/http`

**理由**：
- 无需引入第三方 HTTP 库
- API 调用简单（GET/POST/PUT/DELETE）
- JSON 解析用 `encoding/json`

### 3. 输出格式

**决定**：统一 JSON 格式

```json
{"ok": true, "data": {...}}
{"ok": false, "error": "message"}
```

**理由**：
- Agent 易于解析
- 结构简单，无嵌套复杂结构
- 与现有 skill 系统一致

### 4. 子命令组织

**决定**：按业务领域归类子命令

```
task / exec / experience / scheduled / todo / config / models / dir-shortcut / web-link
```

**理由**：
- 38 个命令需要分组降低复杂度
- 与 xworkbench 业务领域一致
- Agent 易于发现和理解

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| API 字段变化导致 CLI 不兼容 | 保持最小依赖，API 变化时更新 CLI 即可 |
| 手动拷贝分发繁琐 | 提供构建脚本，一次性生成多平台二进制 |
| 38 个命令参数记忆成本高 | 通过 SKILL.md 提供完整描述给 Agent |

## Open Questions

1. **跨平台二进制命名**：`xworkbench-cli-{os}-{arch}`（如 `xworkbench-cli-darwin-arm64`）—— 与 Go 官方命名惯例一致
2. **skill 执行方式**：命令行参数模式（已确认）
