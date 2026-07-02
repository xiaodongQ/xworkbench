---
name: ip-info
description: 查询 IP 地址信息（公网 IP 或指定域名的解析结果）。当用户说"查 IP"、"我的 IP 是多少"、"域名解析"时触发。
version: 1.0.0
xw_command: python3 scripts/check.py
xw_params:
  domain: 要查询的域名或 IP（可选，空则查本机公网 IP）
xw_output:
  ip: 查询到的 IP 地址
  location: IP 地理位置（国家/城市），如果能获取到的话
  org: IP 所属组织/ ISP
xw_examples:
  - description: 查询本机公网 IP
    params: {}
  - description: 解析域名 IP
    params: { domain: "github.com" }
---
