//go:build !windows

package main

import (
	"embed"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed tools/xwcli/xwcli-linux-amd64 tools/xwcli/xwcli-linux-arm64 tools/xwcli/xwcli-darwin-amd64 tools/xwcli/xwcli-darwin-arm64
var xwcliFS embed.FS

// handleXwcliInstall returns the install script that downloads and runs the Go binary.
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

// handleXwcliDownload serves the platform-specific xwcli binary.
// Path: /api/xwcli/xwcli-{os}-{arch} (e.g. /api/xwcli/xwcli-linux-amd64)
// Legacy /api/xwcli/xwcli.py → 410 Gone.
func (s *APIServer) handleXwcliDownload(w http.ResponseWriter, r *http.Request) {
	// e.g. /api/xwcli/xwcli-linux-amd64 → xwcli-linux-amd64
	base := filepath.Base(r.URL.Path)
	if base == "xwcli.py" {
		writeErr(w, http.StatusGone, "xwcli.py is no longer available; use xwcli-install.sh to install the Go binary")
		return
	}
	data, err := xwcliFS.ReadFile("tools/xwcli/" + base)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Sprintf("no binary for %s (not built?)", base))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, base))
	w.Write(data)
}

// installScriptTemplate downloads and installs the Go xwcli binary.
const installScriptTemplate = `#!/bin/bash
# xwcli 安装脚本 - xworkbench 远程 agent (Go 版)
set -e

SERVER="${SERVER_URL}"
NAME=${1:-$HOSTNAME}
CONFIG_DIR="$HOME/.config/xwcli"

echo "==> xwcli 安装脚本"
echo "==> 服务地址: $SERVER"
echo "==> 机器名称: $NAME"

# 1. 检测架构
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64)  ARCH="arm64" ;;
    arm64)    ARCH="arm64" ;;
    *)        echo "ERROR: 不支持的架构: $ARCH" && exit 1 ;;
esac
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
BIN_NAME="xwcli-${OS}-${ARCH}"

# 2. 创建配置目录
mkdir -p "$CONFIG_DIR"

# 3. 下载 xwcli 二进制
echo "==> 下载 xwcli (${OS}/${ARCH})..."
curl -fsSL "$SERVER/api/xwcli/$BIN_NAME" -o "$CONFIG_DIR/xwcli" || {
    echo "ERROR: 下载失败，请确认服务器已构建 xwcli 二进制"
    exit 1
}
chmod +x "$CONFIG_DIR/xwcli"

# 4. 建立 $HOME/bin 软链接
mkdir -p "$HOME/bin"
if [ ! -L "$HOME/bin/xwcli" ] || [ "$(readlink -f "$HOME/bin/xwcli")" != "$CONFIG_DIR/xwcli" ]; then
    ln -sf "$CONFIG_DIR/xwcli" "$HOME/bin/xwcli"
fi

# 5. 注册（如已有配置则跳过）
if [ -f "$CONFIG_DIR/agent.json" ]; then
    echo "==> 已注册，跳过"
    echo "    配置文件: $CONFIG_DIR/agent.json"
else
    echo "==> 注册 agent 到 $SERVER..."
    RESP=$(curl -s -X POST "$SERVER/api/agents/register" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"$NAME\",\"capabilities\":\"task-execute\",\"version\":\"1.0.0\"}")
    AGENT_ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('agent_id',''))" 2>/dev/null || echo "")
    TOKEN=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || echo "")
    if [ -z "$AGENT_ID" ] || [ -z "$TOKEN" ]; then
        echo "ERROR: 注册失败，响应: $RESP"
        exit 1
    fi
    echo "{\"agent_id\":\"$AGENT_ID\",\"token\":\"$TOKEN\",\"server_url\":\"$SERVER\",\"machine_name\":\"$NAME\",\"version\":\"1.0.0\"}" > "$CONFIG_DIR/agent.json"
    echo "==> 注册成功! Agent ID: $AGENT_ID"
fi

echo ""
echo "==> 安装完成! 运行以下命令启动 xwcli:"
echo "    xwcli run"
echo "    后台运行: nohup xwcli run >> $CONFIG_DIR/xwcli.log 2>&1 &"
`
