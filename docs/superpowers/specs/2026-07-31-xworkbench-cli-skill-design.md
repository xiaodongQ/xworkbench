# xworkbench-cli Skill 设计

> 为 Claude Code 等 Agent 提供操作 xworkbench 工作台的工具调用接口

## 1. 背景与目标

**问题**：xworkbench 已有 91 个 HTTP API + Skill 系统，但缺乏面向 Agent 的友好 CLI 工具。Claude Code 等 Agent 难以直接调用工作台能力。

**方案**：实现 `xworkbench-cli` CLI 工具 + Skill 定义，使 Agent 通过标准工具调用协议操作工作台。

## 2. 整体架构

```
Claude Code (Agent)
    │
    ▼
xworkbench-cli skill (tools/xworkbench-cli-skill/SKILL.md)
    │
    ▼
xworkbench-cli CLI 工具
    │
    ├── xworkbench-cli task list / create / get / update / run
    ├── xworkbench-cli exec list / evaluate
    ├── xworkbench-cli experience list / get
    ├── xworkbench-cli scheduled list / run / enable / disable
    ├── xworkbench-cli stats
    ├── xworkbench-cli dir-shortcut list
    └── xworkbench-cli web-link list
    │
    ▼
xworkbench HTTP API (:8902)
```

## 3. CLI 输出格式

所有命令返回 JSON：

```json
// 成功
{"ok": true, "data": {...}}

// 失败
{"ok": false, "error": "task not found"}
```

## 4. 命令设计

### 4.1 Task 命令

| 命令 | 功能 | HTTP 映射 |
|------|------|-----------|
| `xworkbench-cli task list [--status] [--limit]` | 任务列表 | GET /api/tasks |
| `xworkbench-cli task create --title <s> --description <s>` | 创建任务 | POST /api/tasks |
| `xworkbench-cli task get <id>` | 获取任务详情 | GET /api/tasks/{id} |
| `xworkbench-cli task update <id> --status <s> --priority <n>` | 更新任务 | PUT /api/tasks/{id}/status |
| `xworkbench-cli task run <id>` | 运行任务 | POST /api/tasks/{id}/run |

### 4.2 Exec 命令

| 命令 | 功能 | HTTP 映射 |
|------|------|-----------|
| `xworkbench-cli exec list [--task-id] [--limit]` | 执行记录 | GET /api/executions |
| `xworkbench-cli exec evaluate <id>` | AI 评估 | POST /api/executions/{id}/evaluate |

### 4.3 Experience 命令

| 命令 | 功能 | HTTP 映射 |
|------|------|-----------|
| `xworkbench-cli experience list [--module]` | 经验库列表 | GET /api/experiences |
| `xworkbench-cli experience get <id>` | 获取经验详情 | GET /api/experiences/{id} |

### 4.4 Scheduled 命令

| 命令 | 功能 | HTTP 映射 |
|------|------|-----------|
| `xworkbench-cli scheduled list` | 定时任务列表 | GET /api/scheduled |
| `xworkbench-cli scheduled run <id>` | 立即运行 | POST /api/scheduled/{id}/run-now |
| `xworkbench-cli scheduled enable <id>` | 启用调度 | PUT /api/scheduled/{id} |
| `xworkbench-cli scheduled disable <id>` | 禁用调度 | PUT /api/scheduled/{id} |

### 4.5 其他命令

| 命令 | 功能 | HTTP 映射 |
|------|------|-----------|
| `xworkbench-cli stats` | 统计信息 | GET /api/stats |
| `xworkbench-cli dir-shortcut list` | 目录快捷 | GET /api/dir-shortcuts |
| `xworkbench-cli web-link list` | 链接快捷 | GET /api/web-links |

## 5. 目录结构

```
tools/xworkbench-cli-skill/
├── SKILL.md              # Skill 定义（xw_command: xworkbench-cli）
├── xworkbench-cli       # Go 二进制
└── README.md            # 使用说明
```

## 6. 分发机制

复用现有的 `GET /api/xwcli/{filename}` 下载端点：
- 新增 `xworkbench-cli-{os}-{arch}` 二进制下载路径
- Agent 可通过标准方式下载并使用

## 7. 实现要点

- **单二进制**：Go 实现，无外部依赖
- **配置文件**：通过 `--config` 或 `XWORKBENCH_CONFIG` 指定 config.json 路径
- **默认服务地址**：`http://localhost:8902`，可通过 `--server` 覆盖
- **超时处理**：各命令独立超时，API 超时返回错误
- **错误格式**：统一 `{"ok": false, "error": "..."}`
