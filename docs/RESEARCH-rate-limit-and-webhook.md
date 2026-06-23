# 调研：速率限制 + Webhook 通知

## 1. 价值

**速率限制（Rate Limiting）**：
- 借鉴：Stripe API rate limit（每秒 100 calls，per-key 限制）、GitHub API 5000 req/hour
- 价值：防止 agent 误循环（claim 死循环）、防止刷数据、保护后端资源

**Webhook 通知**：
- 借鉴：GitHub webhooks、Stripe webhooks、Jira notifications
- 价值：让外部系统（监控、IM 通知、CI/CD、cron）能被动接收任务状态变化，不用轮询

## 2. 设计

### 2.1 速率限制

**简单实现**：token-bucket per agent_id
- 每个 agent 在 1 分钟内最多 N 次调用（默认 60/min）
- 超限返回 429 Too Many Requests + Retry-After 头
- 内存维护（重启丢失无所谓，反正 ratelimit 只是 best-effort 防护）

**实现位置**：HTTP middleware

```go
// internal/ratelimit/ratelimit.go
type Limiter struct {
    rate     int           // 每分钟允许次数
    buckets  map[string]*bucket
    mu       sync.Mutex
}

type bucket struct {
    tokens  int
    updated time.Time
}

func (l *Limiter) Allow(key string) bool {
    // token bucket refill
}
```

**配置**：
- 通过环境变量 `RATE_LIMIT_PER_MIN=60` 调整
- 0 = 禁用

**挂载点**：
- 只对 agent API（/api/agents/*、/api/tasks/*/claim、/api/tasks/*/report）生效
- 不影响人类用户操作（这些不在 agent API 路径下）

### 2.2 Webhook 通知

**表结构**：
```sql
CREATE TABLE webhooks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    secret TEXT,                  -- HMAC 签名密钥
    events TEXT,                  -- 逗号分隔: created,claimed,reported,task_timeout,...
    enabled INTEGER DEFAULT 1,
    created_at DATETIME,
    last_triggered_at DATETIME,
    fail_count INTEGER DEFAULT 0
);
```

**事件类型**：
- `task.created` - 任务创建
- `task.claimed` - 任务被 claim
- `task.reported` - 任务被 report
- `task.timeout` - 任务超时释放
- `agent.offline` - Agent 心跳超时

**API**：
```
GET    /api/webhooks
POST   /api/webhooks
PUT    /api/webhooks/{id}
DELETE /api/webhooks/{id}
POST   /api/webhooks/{id}/test   # 主动触发测试事件
```

**发送实现**：
- 后台 goroutine 监听事件 → 异步 POST 到所有启用的 webhooks
- 失败重试 3 次（指数退避），最终 fail_count++
- payload 用 HMAC-SHA256 签名（X-Signature 头）
- 包含 X-Webhook-Id、X-Event-Type 头方便调试

## 3. 优先级

1. **速率限制**（简单，高价值，安全必须）
2. **Webhook 通知**（中高价值，提升集成性）

## 4. 风险

- 速率限制如果太严会影响正常 agent
- Webhook 接收方下线会导致大量失败 → fail_count 监控
- Webhook 触发需要保证不阻塞主流程 → 异步 + 队列
