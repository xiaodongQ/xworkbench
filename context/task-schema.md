# Task Schema

## Task Model

| Field | Type | Required | Description |
|-------|------|---------|-------------|
| `id` | string (UUID) | auto | Unique identifier, auto-generated |
| `title` | string | ✅ yes | Task title |
| `description` | string | no | Detailed description of the task |
| `status` | string | auto | One of: `pending`, `in_progress`, `archived`, `exception` |
| `experience_id` | string | no | ID of related experience record |
| `resources` | string | no | Repository address or documentation link |
| `acceptance` | string | no | Acceptance criteria (positive + negative test cases) |
| `version` | string | auto | Defaults to `v0.0.1` |
| `created_at` | datetime | auto | Creation timestamp |
| `claimed_at` | datetime | auto | When task was claimed |
| `maintainer` | string | no | Who claimed the task |
| `repo_address` | string | no | Repository address |
| `archived_at` | datetime | auto | When task was archived |
| `result` | string | no | Result JSON or summary |

## API Endpoints

### List Tasks
```bash
curl -s http://localhost:8902/api/tasks[?status=pending]
```
Returns: `Task[]`

### Get Task by ID
```bash
curl -s http://localhost:8902/api/tasks/{id}
```
Returns: `Task`

### Create Task
```bash
curl -s -X POST http://localhost:8902/api/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "title": "任务标题",
    "description": "详细描述",
    "experience_id": "",
    "resources": "",
    "acceptance": "正向用例:\n输入... → 输出...\n\n反向用例:\n输入... → 预期行为..."
  }'
```

### Update Task Status
```bash
curl -s -X PUT http://localhost:8902/api/tasks/{id}/status \
  -H "Content-Type: application/json" \
  -d '{
    "status": "in_progress|archived|exception",
    "maintainer": "factory-agent"
  }'
```

### Get Stats
```bash
curl -s http://localhost:8902/api/stats
```
Returns: `{ total_tasks, pending_tasks, in_progress_tasks, archived_tasks, exception_tasks, total_exp }`