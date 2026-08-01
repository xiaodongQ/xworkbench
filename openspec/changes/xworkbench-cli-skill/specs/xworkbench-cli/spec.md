# xworkbench-cli

CLI 工具集，提供 38 个子命令覆盖 xworkbench 工作台的完整 CRUD + 执行控制 + 配置管理。

## ADDED Requirements

### Requirement: CLI supports task subcommands
The CLI SHALL support task subcommands: list, create, get, update, run, cancel, delete.

#### Scenario: List tasks
- **WHEN** user runs `xworkbench-cli task list --status pending`
- **THEN** CLI returns JSON with `{"ok": true, "data": [...]}`

#### Scenario: Create task
- **WHEN** user runs `xworkbench-cli task create --title "xxx" --description "yyy"`
- **THEN** CLI returns JSON with `{"ok": true, "data": {"id": "..."}}`

#### Scenario: Run task
- **WHEN** user runs `xworkbench-cli task run 123`
- **THEN** CLI triggers task execution via POST /api/tasks/123/run

### Requirement: CLI supports exec subcommands
The CLI SHALL support exec subcommands: list, get, evaluate, cancel, continue.

#### Scenario: List executions
- **WHEN** user runs `xworkbench-cli exec list --task-id 123`
- **THEN** CLI returns JSON with execution records for that task

#### Scenario: Evaluate execution
- **WHEN** user runs `xworkbench-cli exec evaluate 456`
- **THEN** CLI triggers AI evaluation via POST /api/executions/456/evaluate

### Requirement: CLI supports experience subcommands
The CLI SHALL support experience subcommands: list, get, create, update, delete.

### Requirement: CLI supports scheduled subcommands
The CLI SHALL support scheduled subcommands: list, get, create, update, delete, run, toggle.

### Requirement: CLI supports todo subcommands
The CLI SHALL support todo subcommands: list, add, toggle, edit.

### Requirement: CLI supports config subcommands
The CLI SHALL support config subcommands: get, set.

#### Scenario: Get config
- **WHEN** user runs `xworkbench-cli config get`
- **THEN** CLI returns full config JSON from GET /api/config

#### Scenario: Set config
- **WHEN** user runs `xworkbench-cli config set default_terminal wezterm`
- **THEN** CLI updates config via PUT /api/config

### Requirement: CLI supports models subcommand
The CLI SHALL support models subcommand: list.

#### Scenario: List models
- **WHEN** user runs `xworkbench-cli models list`
- **THEN** CLI returns available models from GET /api/models

### Requirement: CLI supports stats command
The CLI SHALL support stats command returning dashboard statistics.

### Requirement: CLI supports dir-shortcut subcommands
The CLI SHALL support dir-shortcut subcommands: list, create, update, delete, open.

### Requirement: CLI supports web-link subcommands
The CLI SHALL support web-link subcommands: list, create, update, delete, open.

### Requirement: CLI output format
All commands SHALL return JSON with consistent format.

#### Scenario: Success response
- **WHEN** command succeeds
- **THEN** CLI returns `{"ok": true, "data": {...}}`

#### Scenario: Error response
- **WHEN** command fails
- **THEN** CLI returns `{"ok": false, "error": "message"}`

### Requirement: CLI server configuration
CLI SHALL use `http://localhost:8902` as default server address.

#### Scenario: Default server
- **WHEN** user runs command without --server flag
- **THEN** CLI connects to http://localhost:8902

#### Scenario: Custom server
- **WHEN** user runs command with `--server http://x:8902`
- **THEN** CLI connects to specified server
