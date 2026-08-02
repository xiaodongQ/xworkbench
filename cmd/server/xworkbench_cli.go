package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// handleXworkbenchCliDownload serves the platform-specific xworkbench-cli binary.
// Path: GET /api/xworkbench-cli/download
// Returns binary matching the request's OS/Architecture.
func (s *APIServer) handleXworkbenchCliDownload(w http.ResponseWriter, r *http.Request) {
	queryOS := r.URL.Query().Get("os")
	queryArch := r.URL.Query().Get("arch")

	// Auto-detect from User-Agent if not provided
	if queryOS == "" || queryArch == "" {
		ua := r.UserAgent()
		if strings.Contains(ua, "Windows") {
			queryOS = "windows"
		} else if strings.Contains(ua, "Darwin") || strings.Contains(ua, "Mac") {
			queryOS = "darwin"
		} else {
			queryOS = "linux"
		}
		if strings.Contains(ua, "amd64") || strings.Contains(ua, "x86_64") {
			queryArch = "amd64"
		} else if strings.Contains(ua, "arm64") || strings.Contains(ua, "aarch64") {
			queryArch = "arm64"
		} else {
			queryArch = "amd64" // default
		}
	}

	filename := fmt.Sprintf("xworkbench-cli-%s-%s", queryOS, queryArch)
	downloadName := "xworkbench-cli"
	if queryOS == "windows" {
		filename += ".exe"
		downloadName += ".exe"
	}

	data, err := os.ReadFile(filepath.Join("tools", "xworkbench-cli-skill", filename))
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Sprintf("no binary for %s/%s (not built?)", queryOS, queryArch))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.Write(data)
}

// handleXworkbenchCliInstall returns an install script that downloads and installs
// the xworkbench-cli skill to the appropriate agent's skills directory.
// Path: GET /api/xworkbench-cli/install
func (s *APIServer) handleXworkbenchCliInstall(w http.ResponseWriter, r *http.Request) {
	serverURL := r.URL.Query().Get("server")
	if serverURL == "" {
		serverURL = "http://localhost:8902"
	}
	script := strings.Replace(xworkbenchCliInstallScriptTemplate, "${SERVER_URL}", serverURL, 1)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="xworkbench-cli-install.sh"`)
	w.Write([]byte(script))
}

// handleXworkbenchCliSkillMD serves the SKILL.md file.
func (s *APIServer) handleXworkbenchCliSkillMD(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(filepath.Join("tools", "xworkbench-cli-skill", "SKILL.md"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "SKILL.md not found")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

// xworkbenchCliInstallScriptTemplate is the install script for xworkbench-cli skill.
const xworkbenchCliInstallScriptTemplate = `#!/bin/bash
# xworkbench-cli 安装脚本
# 自动检测 Claude Code / CodeBuddy 并交互式选择安装到对应 skills 目录
set -e

SERVER="${SERVER_URL}"
SKILL_NAME="xworkbench-cli"

# 检查 stdin 是否来自终端（管道安装时跳过交互）
INTERACTIVE=0
[ -t 0 ] && INTERACTIVE=1

# 1. 检测可用的运行环境
HAS_CLAUDE=0
HAS_CODEBUDDY=0
TOTAL=0
if [ -d "$HOME/.claude" ]; then
    HAS_CLAUDE=1
    TOTAL=$((TOTAL + 1))
fi
if [ -d "$HOME/.codebuddy" ]; then
    HAS_CODEBUDDY=1
    TOTAL=$((TOTAL + 1))
fi

if [ $TOTAL -eq 0 ]; then
    echo "ERROR: 未检测到 Claude Code (~/.claude) 或 CodeBuddy (~/.codebuddy) 环境"
    exit 1
fi

# 2. 选择安装目标
DO_CLAUDE=0
DO_CODEBUDDY=0
if [ $INTERACTIVE -eq 1 ] && [ $TOTAL -gt 1 ]; then
    echo ""
    echo "检测到以下环境，请选择安装目标："
    echo "  1) Claude Code  ($HOME/.claude)"
    echo "  2) CodeBuddy    ($HOME/.codebuddy)"
    echo "  3) 全部安装"
    while true; do
        read -r -p "请选择 (1/2/3): " choice </dev/tty
        case "$choice" in
            1) DO_CLAUDE=1; break ;;
            2) DO_CODEBUDDY=1; break ;;
            3) DO_CLAUDE=$HAS_CLAUDE; DO_CODEBUDDY=$HAS_CODEBUDDY; break ;;
            *) echo "  无效选择，请输入 1、2 或 3" ;;
        esac
    done
else
    DO_CLAUDE=$HAS_CLAUDE
    DO_CODEBUDDY=$HAS_CODEBUDDY
fi

# 3. 检测架构
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64)  ARCH="arm64" ;;
    arm64)    ARCH="arm64" ;;
    *)        echo "ERROR: 不支持的架构: $ARCH" && exit 1 ;;
esac
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
BIN_NAME="xworkbench-cli-$OS-$ARCH"
if [ "$OS" = "windows" ]; then
    BIN_NAME="$BIN_NAME.exe"
fi

# 4. 逐个安装到目标目录
install_to() {
    local dir="$1"
    local name="$2"
    mkdir -p "$dir"

    local action="安装"
    if [ -f "$dir/SKILL.md" ]; then
        action="更新"
    fi

    echo ""
    echo "==> ${action} xworkbench-cli ($OS/$ARCH) → $dir"
    curl -fsSL "$SERVER/api/xworkbench-cli/download?os=$OS&arch=$ARCH" -o "$dir/xworkbench-cli" || {
        echo "ERROR: 下载失败，请确认服务器已构建二进制"
        exit 1
    }
    chmod +x "$dir/xworkbench-cli"

    echo "==> ${action} SKILL.md → $dir"
    curl -fsSL "$SERVER/api/xworkbench-cli/skill.md" -o "$dir/SKILL.md" || {
        echo "ERROR: 下载 SKILL.md 失败"
        exit 1
    }
}

if [ $DO_CLAUDE -eq 1 ]; then
    install_to "$HOME/.claude/skills/$SKILL_NAME" "Claude Code"
fi
if [ $DO_CODEBUDDY -eq 1 ]; then
    install_to "$HOME/.codebuddy/skills/$SKILL_NAME" "CodeBuddy"
fi

echo ""
echo "==> 完成!"
if [ $DO_CLAUDE -eq 1 ]; then
    echo "    Claude Code: $HOME/.claude/skills/$SKILL_NAME"
fi
if [ $DO_CODEBUDDY -eq 1 ]; then
    echo "    CodeBuddy:   $HOME/.codebuddy/skills/$SKILL_NAME"
fi
`
