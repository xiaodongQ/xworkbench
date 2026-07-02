//go:build !windows

package main

import (
	"net/http"
	"strings"
)

// handleXwcliInstall 返回 xwcli 安装脚本内容（公开，无需认证）。
func (s *APIServer) handleXwcliInstall(w http.ResponseWriter, r *http.Request) {
	serverURL := r.URL.Query().Get("server")
	if serverURL == "" {
		serverURL = "http://localhost:8902"
	}
	script := strings.Replace(installScriptTemplate, "${SERVER_URL}", serverURL, 1)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="xwcli-install.sh"`)
	w.Write([]byte(script))
}

// handleXwcliDownload 返回 xwcli.py 单文件 agent 工具源码（公开，无需认证）。
func (s *APIServer) handleXwcliDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/x-python; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="xwcli.py"`)
	w.Write([]byte(xwcliSource))
}

// installScriptTemplate 是 xwcli 安装脚本模板，${SERVER_URL} 在运行时替换。
const installScriptTemplate = `#!/bin/bash
# xwcli 安装脚本 - xworkbench 远程 agent
set -e

SERVER="${SERVER_URL}"
NAME=${1:-$HOSTNAME}
CONFIG_DIR="$HOME/.config/xwcli"

echo "==> xwcli 安装脚本"
echo "==> 服务地址: $SERVER"
echo "==> 机器名称: $NAME"

# 1. 创建配置目录
mkdir -p "$CONFIG_DIR"

# 2. 下载 xwcli.py
XWCLI_URL="$SERVER/api/xwcli/xwcli.py"
echo "==> 下载 xwcli.py..."
if command -v curl &>/dev/null; then
    curl -fsSL "$XWCLI_URL" -o "$CONFIG_DIR/xwcli.py"
elif command -v wget &>/dev/null; then
    wget -q "$XWCLI_URL" -O "$CONFIG_DIR/xwcli.py"
else
    echo "ERROR: 需要 curl 或 wget" && exit 1
fi
chmod +x "$CONFIG_DIR/xwcli.py"
mkdir -p "$HOME/bin"
ln -sf "$CONFIG_DIR/xwcli.py" "$HOME/bin/xwcli" 2>/dev/null || true
echo "==> xwcli.py 已安装到 $CONFIG_DIR/xwcli.py"

# 3. 注册 agent（如已有配置则跳过）
if [ -f "$CONFIG_DIR/agent.json" ]; then
    echo "==> 已注册，跳过注册步骤"
    echo "    配置文件: $CONFIG_DIR/agent.json"
else
    echo "==> 注册 agent 到 $SERVER..."
    RESP=$(curl -s -X POST "$SERVER/api/agents/register" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"$NAME\",\"capabilities\":\"task-execute,streaming-output\",\"version\":\"0.1.0\"}")
    AGENT_ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('agent_id',''))" 2>/dev/null || echo "")
    TOKEN=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || echo "")
    if [ -z "$AGENT_ID" ] || [ -z "$TOKEN" ]; then
        echo "ERROR: 注册失败，响应: $RESP"
        exit 1
    fi
    echo "{\"agent_id\":\"$AGENT_ID\",\"token\":\"$TOKEN\",\"server_url\":\"$SERVER\",\"machine_name\":\"$NAME\",\"version\":\"0.1.0\"}" > "$CONFIG_DIR/agent.json"
    echo "==> 注册成功!"
    echo "    Agent ID: $AGENT_ID"
    echo "    配置文件: $CONFIG_DIR/agent.json"
fi

# 4. 输出运行命令
echo ""
echo "==> 安装完成! 运行以下命令启动 xwcli:"
echo "    python3 $CONFIG_DIR/xwcli.py run"
echo ""
echo "    或后台运行:"
echo "    nohup python3 $CONFIG_DIR/xwcli.py run >> $CONFIG_DIR/xwcli.log 2>&1 &"
`

// xwcliSource 是 xwcli.py 单文件 agent 工具的完整 Python 源码。
// 零外部依赖，仅用 Python3 标准库。
const xwcliSource = `#!/usr/bin/env python3
"""
xwcli - xworkbench 远程 agent（单文件，零依赖）

用法:
    xwcli.py register --server <url> --name <machine-name>
    xwcli.py run          # 启动常驻 agent（claim → execute → report 循环）
    xwcli.py status       # 查看当前状态
    xwcli.py stop         # 停止 agent
"""

import json, os, pathlib, subprocess, sys, time, urllib.request, urllib.error

def _windows_detach():
    if sys.platform != 'win32':
        return
    try:
        DETACHED = 0x00000008
        CREATE_PGROUP = 0x00000200
        si = subprocess.STARTUPINFO()
        si.dwFlags |= subprocess.STARTF_USESHOWWINDOW
        si.wShowWindow = 0
        subprocess.Popen(
            [sys.executable, __file__, 'run-inner'],
            stdin=subprocess.DEVNULL,
            stdout=open(os.devnull, 'w'),
            stderr=open(os.devnull, 'w'),
            creationflags=DETACHED | CREATE_PGROUP,
            startupinfo=si,
        )
        print('[xwcli] Windows background started')
        sys.exit(0)
    except Exception as e:
        print('[xwcli] background failed: %s' % e)

CONFIG_PATH = pathlib.Path.home() / ".config" / "xwcli" / "agent.json"
HEARTBEAT_INTERVAL = 15   # 秒
CLAIM_INTERVAL = 10       # 秒（无任务时轮询间隔）
LONG_POLL_TIMEOUT = 30    # 秒（claim-next long-poll 超时）


def load_config():
    if not CONFIG_PATH.exists():
        print(f"ERROR: 未注册，请先运行: xwcli.py register --server <url> --name <name>")
        sys.exit(1)
    return json.loads(CONFIG_PATH.read_text())


def api_request(method, url, data=None, token=None):
    body = json.dumps(data).encode() if data else None
    req = urllib.request.Request(url, data=body, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            raw = resp.read()
            try:
                return resp.status, json.loads(raw)
            except json.JSONDecodeError:
                return resp.status, {"raw": raw.decode("utf-8", errors="replace")}
    except urllib.error.HTTPError as e:
        try:
            err_body = json.loads(e.read())
        except:
            err_body = {"error": e.reason}
        return e.code, err_body
    except Exception as e:
        return 0, {"error": str(e)}


def register(server_url, name, version="0.1.0"):
    url = f"{server_url}/api/agents/register"
    status, resp = api_request("POST", url, {
        "name": name,
        "capabilities": "task-execute,streaming-output",
        "version": version,
    })
    if status != 200:
        print(f"ERROR: 注册失败 ({status}): {resp}")
        sys.exit(1)
    CONFIG_PATH.parent.mkdir(parents=True, exist_ok=True)
    config = {
        "agent_id": resp["agent_id"],
        "token": resp["token"],
        "server_url": server_url,
        "machine_name": name,
        "version": version,
    }
    CONFIG_PATH.write_text(json.dumps(config, indent=2))
    print(f"OK: 注册成功! agent_id={resp['agent_id']}")
    print(f"    配置文件: {CONFIG_PATH}")


def do_heartbeat(token, server_url, status="idle", current_task_id=None):
    url = f"{server_url}/api/agents/heartbeat"
    status_code, _ = api_request("POST", url, {
        "status": status,
        "current_task_id": current_task_id or "",
    }, token)
    return status_code == 200


def do_claim_next(token, server_url, agent_id):
    """Long-poll claim-next GET，返回 task dict 或 None"""
    url = f"{server_url}/api/tasks/claim-next?agent_id={agent_id}&timeout={LONG_POLL_TIMEOUT}"
    status, resp = api_request("GET", url, token=token)
    if status == 204 or status == 0:
        return None
    if status == 200 and resp.get("status") == "claimed":
        return resp
    return None


def do_report(token, server_url, task_id, result_output, status="archived",
              score=None, last_error="", duration_sec=None):
    url = f"{server_url}/api/tasks/{task_id}/report"
    payload = {
        "status": status,
        "result_output": result_output,
        "last_error": last_error,
    }
    if score is not None:
        payload["evaluation_score"] = score
    if duration_sec is not None:
        payload["duration_sec"] = duration_sec
    status_code, _ = api_request("POST", url, payload, token)
    return status_code == 200


def run_claude(prompt):
    """尝试用 claude CLI 执行 prompt，返回 (stdout, stderr, exit_code, duration_sec)"""
    for cli in ["claude", "hermes"]:
        try:
            start = time.time()
            # 优先用 --print 模式（无交互），否则用 stdin
            for args in [
                [cli, "--print", prompt],
                [cli],
            ]:
                try:
                    proc = subprocess.Popen(
                        args,
                        stdin=subprocess.DEVNULL,
                        stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE,
                    )
                    stdout, stderr = proc.communicate(timeout=600)
                    duration = int(time.time() - start)
                    out = stdout.decode("utf-8", errors="replace")
                    err = stderr.decode("utf-8", errors="replace")
                    if proc.returncode == 0 or args[1] == "--print":
                        return out, err, proc.returncode, duration
                except FileNotFoundError:
                    continue
                except subprocess.TimeoutExpired:
                    proc.kill()
                    return "", f"{cli} 执行超时（10分钟）", 124, 600
        except Exception as e:
            continue
    return "", f"ERROR: claude/hermes 均未找到", 127, 0


def run_loop():
    """主循环：claim → execute → report"""
    pid_file = CONFIG_PATH.parent / "xwcli.pid"
    if sys.platform == "win32":
        pid_file.write_text(str(os.getpid()))
    config = load_config()
    token = config["token"]
    agent_id = config["agent_id"]
    server_url = config["server_url"]
    print(f"[xwcli] 启动 agent, server={server_url}, machine={config['machine_name']}")

    # 初始心跳
    do_heartbeat(token, server_url, "idle")

    while True:
        try:
            # 1. claim-next（long-poll，等任务）
            result = do_claim_next(token, server_url, agent_id)
            if not result:
                time.sleep(CLAIM_INTERVAL)
                do_heartbeat(token, server_url, "idle")
                continue

            task_id = result.get("task_id") or (result.get("task") or {}).get("id", "")
            prompt = result.get("prompt", "")
            if not task_id or not prompt:
                time.sleep(CLAIM_INTERVAL)
                continue

            print(f"[xwcli] 领到任务 task_id={task_id}")
            do_heartbeat(token, server_url, "busy", task_id)

            # 2. 执行
            start = time.time()
            stdout, stderr, exit_code, duration = run_claude(prompt)
            output = stdout + ("\n[STDERR] " + stderr if stderr else "")

            # 3. 上报
            last_error = f"exit_code={exit_code}" if exit_code != 0 else ""
            do_report(token, server_url, task_id, output, "archived",
                      last_error=last_error, duration_sec=duration)
            print(f"[xwcli] 任务完成 task_id={task_id}, duration={duration}s, exit={exit_code}")
            do_heartbeat(token, server_url, "idle")

        except KeyboardInterrupt:
            print("\n[xwcli] 收到中断，退出")
            break
        except Exception as e:
            print(f"[xwcli] 错误: {e}")
            time.sleep(5)


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(0)
    cmd = sys.argv[1]

    if cmd == "register":
        server_url = name = None
        i = 2
        while i < len(sys.argv):
            if sys.argv[i] == "--server" and i + 1 < len(sys.argv):
                server_url = sys.argv[i + 1]
                i += 2
            elif sys.argv[i] == "--name" and i + 1 < len(sys.argv):
                name = sys.argv[i + 1]
                i += 2
            else:
                i += 1
        if not server_url or not name:
            print("ERROR: 需要 --server <url> --name <machine-name>")
            sys.exit(1)
        register(server_url, name)

    elif cmd == "run-inner":
        run_loop()
        pid_file.unlink(missing_ok=True)
        sys.exit(0)
    elif cmd == "run":
        _windows_detach()
        run_loop()

    elif cmd == "status":
        config = load_config()
        print(json.dumps(config, indent=2))

    elif cmd == "stop":
        pid_file = CONFIG_PATH.parent / "xwcli.pid"
        if pid_file.exists():
            import signal as sig
            pid = int(pid_file.read_text().strip())
            os.kill(pid, sig.SIGTERM)
            print(f"已发送 SIGTERM 到 PID {pid}")
        else:
            print("stop 功能需配合 systemd 或 kill pid（见 xwcli.pid）")

    else:
        print(f"Unknown command: {cmd}")
        print(__doc__)
        sys.exit(1)


if __name__ == "__main__":
    main()
`