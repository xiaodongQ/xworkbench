# Experience Schema

## Experience Model

| Field | Type | Required | Description |
|-------|------|---------|-------------|
| `id` | string (UUID) | auto | Unique identifier, auto-generated |
| `module` | string | ✅ yes | Module name (e.g., `redis-cluster`, `mysql-slow`) |
| `keywords` | string | no | Comma-separated keywords for programmatic matching |
| `log_paths` | string | no | Log file paths (e.g., `/var/log/redis/redis-server.log`) |
| `tool_usage` | string | no | Common CLI commands (multi-line) |
| `scene` | string | no | Applicable business scenarios |
| `log_samples` | string | no | Sample log snippets for manual comparison |
| `code_snippets` | string | no | Configuration or solution code samples |
| `version` | string | auto | Defaults to `v1.0.0` |
| `created_at` | datetime | auto | Creation timestamp |
| `updated_at` | datetime | auto | Last update timestamp |

## API Endpoints

### List Experiences
```bash
curl -s http://localhost:8901/api/experiences[?module=redis-cluster]
```
Returns: `Experience[]`

### Get Experience by ID
```bash
curl -s http://localhost:8901/api/experiences/{id}
```
Returns: `Experience`

### Create Experience
```bash
curl -s -X POST http://localhost:8901/api/experiences \
  -H "Content-Type: application/json" \
  -d '{
    "module": "redis-cluster",
    "keywords": "CLUSTERDOWN, MOVED redirect, READONLY",
    "log_paths": "/var/log/redis/redis-server.log",
    "tool_usage": "redis-cli cluster nodes\nredis-cli slowlog get 10",
    "scene": "集群节点失联定位\n内存异常增长分析",
    "log_samples": "# CLUSTERDOWN\nThe cluster is gone",
    "code_snippets": "cluster-node-timeout 5000"
  }'
```

### Delete Experience
```bash
curl -s -X DELETE http://localhost:8901/api/experiences/{id}
```