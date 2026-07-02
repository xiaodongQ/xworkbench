---
name: process-check
description: 检查进程是否存活，获取进程 PID/CPU/内存信息。当用户说"进程还活着吗"、"检查进程"、"服务进程号"时触发。
version: 1.0.0
xw_command: python3 scripts/check.py
xw_params:
  name: 进程名关键字（必填，模糊匹配）
xw_output:
  status: running | not_found | error
  pid: 进程 PID（仅 running 时有）
  cpu_percent: CPU 使用率（%）
  mem_mb: 内存占用（MB）
  cmdline: 完整命令行
  error: 错误信息（仅 error 时有）
xw_examples:
  - description: 检查 xworkbench 进程
    params: { name: xworkbench }
  - description: 检查 nginx
    params: { name: nginx }
---
