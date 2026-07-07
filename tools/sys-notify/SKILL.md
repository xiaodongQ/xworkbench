---
name: sys-notify
description: 发送跨平台系统通知（Windows/macOS/Linux 均支持）。当用户说"通知我"、"弹个消息"、"提醒我"时触发。
version: 1.0.0
xw_command: python3 scripts/notify.py
xw_params:
  body: 通知内容（必填）
  title: 通知标题（必填）
  urgency: 紧急程度 low=低/normal=普通/critical=紧急，默认 normal
xw_output:
  status: sent | error
  error: 错误信息（仅 error 时有）
xw_examples:
  - description: 任务完成通知
    params: { body: "代码审查已完成，共发现 3 个问题", title: "任务完成" }
  - description: 紧急告警
    params: { body: "CPU 使用率超过 90%", title: "⚠️ 服务异常", urgency: critical }
---
