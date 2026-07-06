# Memory.md 规范

## 目的
`data/memory.md` 是 AI 与用户对话时的"记忆文件"——需要跨 session 记住的关键信息（偏好、约定、项目上下文、持续任务等）。每次 AI 对话结束时，把需要记住的内容追加到这个文件。

## 格式

```markdown
# Memory — 最后更新: 2026-07-06

## 用户 & 环境
[2026-07-06] 用户时区 Asia/Shanghai，通过飞书联系
[2026-07-01] 常用模型: minimax/MiniMax-M3

## 项目
[2026-07-03] xworkbench 路径: /home/workspace/repo/xworkbench
[2026-07-05] 博客仓库: /home/workspace/xiaodongq.github.io

## 约定
[2026-07-06] 飞书推送每条必须带日期 📅 YYYY-MM-DD
[2026-07-06] git 操作必须 fetch→pull→commit→push

## 持续任务
[2026-07-06] AI 趋势日报 cron 已设置，每天 08:00
```

## 规则

### 条目格式
- 每条 `[YYYY-MM-DD] 内容`
- 带分类标签（## 二级标题）
- 同一分类下不允许完全重复的内容

### 大小限制
- **硬上限：20KB**。接近上限时（>18KB）给出警告，>20KB 拒绝写入
- 写入前检查，超过时触发整合（dedup + 摘要）

### 去重策略
- 精确去重：完全相同的条目不写入
- 相似去重（>70% token 重叠）：写入时提示是否要合并
- 同一分类下同一天同一主题只保留最新一条

### 写入时机
- AI 对话结束（AI final response）时，由 AI 自行判断是否需要写入
- 也可通过 `memory_add` 工具手动触发
- 工具名：`memory_add`（追加条目）、`memory_list`（查看当前记忆）

### 启动加载
- 服务启动时读取 `data/memory.md`，内容注入 AI system prompt 头部
- system prompt 注入格式：`\n\n## 记忆\n<memory.md 内容>\n`

### 工具接口
```
memory_add(text: str, category: str) -> str  # 追加条目，返回结果
memory_list() -> str                          # 列出所有条目
memory_prune() -> str                         # 手动触发整合/去重
```

## 实现位置
- `internal/memory/memory.go` — 核心逻辑
- `internal/memory/memory_test.go` — TDD 测试
- `cmd/server/memory_tools.go` — AI 工具导出
- `cmd/server/main.go` — 启动时加载
- `cmd/server/ai_chat.go` — system prompt 注入
