# 调度器（Scheduler）

`internal/scheduler/scheduler.go` 包装 [`robfig/cron/v3`](https://github.com/robfig/cron)，进程内调度，**不依赖 OS scheduler**（launchd / systemd / Task Scheduler 都用不上）。

## 启动 / 停止 / 重载

```bash
# 启动
curl -X POST localhost:8901/api/scheduler/start
# 停止
curl -X POST localhost:8901/api/scheduler/stop
# 状态
curl localhost:8901/api/scheduler/status
# {"running": true}
# 重载（增删定时任务后自动调用，也可手动）
curl -X POST localhost:8901/api/scheduler/reload
```

UI 路径：Dashboard 4 号 widget 或 Automation Tab 顶部。

## Cron 表达式

支持 `robfig/cron` 的所有语法（5 字段 + 预设）：

### 标准 5 字段

```
分 时 日 月 周
*  *  *  *  *     每分钟
0  9  *  *  *     每天 9 点
*/5 *  *  *  *     每 5 分钟
0  0  1  *  *     每月 1 日 0 点
0  9  *  *  1-5   工作日 9 点
```

### 预设（descriptor）

| 表达式 | 等价 |
|---|---|
| `@yearly` | `0 0 1 1 *` |
| `@monthly` | `0 0 1 * *` |
| `@weekly` | `0 0 * * 0` |
| `@daily` / `@midnight` | `0 0 * * *` |
| `@hourly` | `0 * * * *` |
| `@every 30s` | 每 30 秒 |
| `@every 5m` | 每 5 分钟 |
| `@every 1h30m` | 每 1.5 小时 |

注意：`robfig/cron/v3` 默认是**分钟级**（6 字段语法可加秒，但本项目用 5 字段 + `@every` 实现秒级）。

## 创建定时任务

```bash
curl -X POST localhost:8901/api/scheduled -H "Content-Type: application/json" -d '{
  "name": "heartbeat",
  "cron_expr": "@every 30s",
  "command_type": "shell",
  "prompt": "echo tick && date",
  "enabled": true
}'
```

字段说明：
- `name` 必填，显示名
- `cron_expr` 必填，标准 5 字段 或 `@every`
- `command_type` 必填：`shell` / `claude` / `cbc`
- `model` 可选（claude/cbc 时用 `--model`）
- `prompt` 必填（claude/cbc 时为 prompt；shell 时为命令）
- `working_dir` 可选（暂未启用）
- `enabled` 默认 true

## 立即触发

```bash
# 不依赖 cron，立即跑一次
curl -X POST localhost:8901/api/scheduled/{id}/run-now
```

## 执行流程

```
cron 触发
  ↓
makeHandler(t)
  ↓
BuildCommand(typ, model, "", prompt)
  ↓
写 executions 表 (source='scheduled')
  ↓
executor.Run(ctx, cmd, onChunk) 流式执行
  ↓
onChunk 调 hub.Broadcast(wsmsg.ChannelScheduled, ...)
  ↓
写 executions.output/error/exit_code
  ↓
scheduled_tasks.last_run_at / last_status / last_execution_id
```

## 跨平台调度

`robfig/cron` 用 Go stdlib `time` 包，**跨平台**。`time.LoadLocation("Local")` 加载系统时区。

- macOS：✅
- Linux：✅
- Windows：✅（进程内 cron，不需要 Task Scheduler）

⚠️ 进程退出 → 调度停。要 7×24 跑需要：
- macOS / Linux：launchd / systemd
- Windows：可选 `kardianos/service` 把二进制注册成 Windows Service（v0.2 计划）

## 故障排查

```bash
# 1. 看 cron 解析日志
tail -f server.log | grep scheduler
# [scheduler] loaded heartbeat cron="@every 10s" next=2026-06-12T15:00:10 id=1
# [scheduler] parse xxx ("bad expr"): invalid syntax

# 2. 看最近执行
curl 'localhost:8901/api/executions?limit=20' | jq '.[] | {command, exit_code, started_at, output}'

# 3. 看 last_status
curl localhost:8901/api/scheduled | jq '.[] | {name, last_status, last_run_at}'
```

`last_status` 取值：
- `success` — exit 0
- `failed` — exit ≠ 0
- `timeout` — 30 分钟 ctx 超时
- `build_error` — BuildCommand 报错（如 cbc/codebuddy 都不在 PATH）
