// Package wsmsg 定义 WebSocket 消息类型与 6 频道常量。
package wsmsg

// Channel 6 个 WebSocket 频道。
const (
	ChannelScheduler = "scheduler" // 调度器状态/启停
	ChannelTask      = "task"      // 任务状态变更
	ChannelExec      = "exec"      // 任务执行的 stdout/stderr 流
	ChannelScheduled = "scheduled" // 定时任务触发事件（started/done；输出流走 ChannelExec）
	ChannelShortcut  = "shortcut"  // 快捷方式打开通知
	ChannelTodo      = "todo"      // todo.md 解析结果
	ChannelAgent     = "agent"     // 远程 Agent 状态变更（心跳丢失/任务释放/token 重置/删除）
)

// Message 通用 WS 消息结构。
type Message struct {
	Channel string `json:"channel"`
	Payload any    `json:"payload"`
}
