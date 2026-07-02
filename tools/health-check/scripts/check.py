#!/usr/bin/env python3
"""health-check: 检查主机端口是否可达，返回延迟和状态"""
import sys
import json
import socket
import time

try:
    params = json.load(sys.stdin)
except Exception:
    params = {}

host = params.get("host", "")
port = params.get("port", "")
timeout = params.get("timeout", 5)

if not host or not port:
    print(json.dumps({"status": "error", "error": "host and port are required"}))
    sys.exit(1)

try:
    port = int(port)
except ValueError:
    print(json.dumps({"status": "error", "error": f"port must be integer, got: {port}"}))
    sys.exit(1)

start = time.time()
try:
    sock = socket.create_connection((host, port), timeout=float(timeout))
    latency = round((time.time() - start) * 1000)
    sock.close()
    print(json.dumps({"status": "reachable", "latency_ms": latency}))
except socket.timeout:
    print(json.dumps({"status": "unreachable", "latency_ms": -1, "error": "connection timeout"}))
    sys.exit(0)  # unreachable is valid, not an error
except ConnectionRefusedError:
    # 连接拒绝，但主机可达（服务没开）
    latency = round((time.time() - start) * 1000)
    print(json.dumps({"status": "unreachable", "latency_ms": latency, "error": "connection refused (service not running)"}))
    sys.exit(0)  # unreachable is valid, not an error
except socket.gaierror as e:
    print(json.dumps({"status": "error", "error": f"dns resolution failed: {e}"}))
    sys.exit(1)
except OSError as e:
    print(json.dumps({"status": "unreachable", "latency_ms": -1, "error": str(e)}))
    sys.exit(0)  # unreachable is a valid result, not an error
