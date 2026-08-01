## Why

xworkbench 工作台目前缺乏面向 AI Agent 的友好接口。虽然已有 91 个 HTTP API，但 Claude Code 等 Agent 难以直接调用这些能力。实现 xworkbench-cli 工具，使 Agent 能通过标准命令行接口操作工作台。

## What Changes

- 新增 `xworkbench-cli` CLI 工具，支持 38 个子命令覆盖完整 CRUD + 执行控制 + 配置管理
- 新增 `xworkbench-cli-skill` Skill 定义包，放在 `tools/xworkbench-cli-skill/`
- 自完备分发：手动拷贝到 Agent tools 目录即可使用
- 无需 HTTP API 分发机制

## Capabilities

### New Capabilities

- `xworkbench-cli`: CLI 工具集，支持 task/exec/experience/scheduled/todo/config/models/web-link/dir-shortcut 等子命令
- `xworkbench-cli-skill`: Skill 定义包，封装 xworkbench-cli 为可被 Agent 调用的 Skill

### Modified Capabilities

（无 - 新增实现，不修改现有功能）

## Impact

- 新增 `tools/xworkbench-cli-skill/` 目录（含 SKILL.md、二进制、README.md）
- Go 实现，单二进制，无外部依赖
- 默认连接 `http://localhost:8902`，支持 `--server` 参数覆盖
