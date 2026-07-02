---
name: health-check
description: 检查远程主机端口是否可达，验证服务是否存活。当用户说"服务正常吗"、"检查端口"、"服务挂了没"时触发。
version: 1.0.0
xw_command: python3 scripts/check.py
xw_params:
  host: 目标主机地址（必填）
  port: 端口号（必填）
  timeout: 超时秒数，默认 5
xw_output:
  status: reachable | unreachable
  latency_ms: 延迟（毫秒），不可达时为 -1
  error: 错误信息（可选）
xw_examples:
  - description: 检查本机 8902 端口
    params: { host: localhost, port: 8902 }
  - description: 检查远程 HTTP 服务
    params: { host: example.com, port: 443, timeout: 10 }
---
