#!/usr/bin/env python3
import sys
import json
import urllib.request
import urllib.error

try:
    params = json.load(sys.stdin)
except Exception:
    params = {}

domain = params.get("domain", "").strip()

result = {}

try:
    if not domain:
        # 查询本机公网 IP（多个服务兜底）
        ip = None
        for url in ["https://api.ipify.org", "https://icanhazip.com"]:
            try:
                req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
                with urllib.request.urlopen(req, timeout=10) as r:
                    ip = r.read().decode().strip()
                    break
            except Exception:
                continue
        if not ip:
            print(json.dumps({"status": "error", "error": "无法获取公网 IP"}))
            sys.exit(1)
        result["ip"] = ip
        result["source"] = "ipify"
    else:
        # 域名解析
        import socket
        ip = socket.gethostbyname(domain)
        result["ip"] = ip
        result["source"] = f"dns:{domain}"
except Exception as e:
    print(json.dumps({"status": "error", "error": str(e)}))
    sys.exit(1)

print(json.dumps({"status": "ok", **result}))
