"""
http_util - xworkbench 共享 HTTP 请求工具
供所有 skill 脚本 import 复用

用法（在任意 skill 的 scripts/check.py 中）:
    from http_util.http_util import json_request, get, post

    result = json_request("https://api.example.com/data",
                          method="GET",
                          headers={"Authorization": "Bearer xxx"})
"""
import json
import urllib.request
import urllib.error
import urllib.parse


def json_request(url, method="GET", headers=None, body=None, timeout=10):
    """
    通用 JSON HTTP 请求。

    Args:
        url: 请求 URL
        method: HTTP 方法 GET/POST/PUT/DELETE
        headers: dict，请求头
        body: dict 或 str，请求体（dict 会自动 JSON 序列化）
        timeout: 超时秒数

    Returns:
        dict，解析后的 JSON 响应
    Raises:
        Exception，错误时抛出
    """
    if headers is None:
        headers = {}
    headers = dict(headers)

    # 默认 Accept
    if "Accept" not in headers:
        headers["Accept"] = "application/json"

    data = None
    if body is not None:
        if isinstance(body, dict):
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = headers.get("Content-Type", "application/json")
        else:
            data = body.encode("utf-8") if isinstance(body, str) else body

    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read()
            charset = resp.headers.get_content_charset() or "utf-8"
            try:
                return json.loads(raw.decode(charset))
            except json.JSONDecodeError:
                return {"raw": raw.decode(charset, errors="replace")}
    except urllib.error.HTTPError as e:
        raw = e.read()
        charset = e.headers.get_content_charset() or "utf-8"
        try:
            body = json.loads(raw.decode(charset))
        except (json.JSONDecodeError, UnicodeDecodeError):
            body = {"error": raw.decode(charset, errors="replace")}
        return {"error": f"HTTP {e.code}", "body": body}
    except urllib.error.URLError as e:
        raise Exception(f"网络错误: {e.reason}")


def get(url, headers=None, timeout=10):
    """GET 请求 shortcut"""
    return json_request(url, method="GET", headers=headers, timeout=timeout)


def post(url, headers=None, body=None, timeout=10):
    """POST 请求 shortcut"""
    return json_request(url, method="POST", headers=headers, body=body, timeout=timeout)


def put(url, headers=None, body=None, timeout=10):
    """PUT 请求 shortcut"""
    return json_request(url, method="PUT", headers=headers, body=body, timeout=timeout)


def delete(url, headers=None, timeout=10):
    """DELETE 请求 shortcut"""
    return json_request(url, method="DELETE", headers=headers, timeout=timeout)
