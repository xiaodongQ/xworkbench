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
	if queryOS == "windows" {
		filename += ".exe"
	}

	data, err := os.ReadFile(filepath.Join("tools", "xworkbench-cli-skill", filename))
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Sprintf("no binary for %s/%s (not built?)", queryOS, queryArch))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
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
# 自动检测 Claude Code / CodeBuddy 并安装到对应 skills 目录
set -e

SERVER="${SERVER_URL}"
SKILL_NAME="xworkbench-cli"
SKILL_DIR=""

# 1. 检测运行环境
if [ -n "$CLAUDE_CODE" ] || [ -d "$HOME/.claude" ]; then
    SKILL_DIR="$HOME/.claude/skills/$SKILL_NAME"
    echo "==> 检测到 Claude Code，安装到 ~/.claude/skills/"
elif [ -d "$HOME/.codebuddy" ]; then
    SKILL_DIR="$HOME/.codebuddy/skills/$SKILL_NAME"
    echo "==> 检测到 CodeBuddy，安装到 ~/.codebuddy/skills/"
else
    echo "ERROR: 未检测到 Claude Code 或 CodeBuddy 环境"
    exit 1
fi

# 2. 创建目录
mkdir -p "$SKILL_DIR"

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

# 4. 下载二进制
echo "==> 下载 xworkbench-cli ($OS/$ARCH)..."
curl -fsSL "$SERVER/api/xworkbench-cli/download?os=$OS&arch=$ARCH" -o "$SKILL_DIR/xworkbench-cli" || {
    echo "ERROR: 下载失败，请确认服务器已构建二进制"
    exit 1
}
chmod +x "$SKILL_DIR/xworkbench-cli"

# 5. 下载 SKILL.md
echo "==> 下载 SKILL.md..."
curl -fsSL "$SERVER/api/xworkbench-cli/skill.md" -o "$SKILL_DIR/SKILL.md" || {
    echo "ERROR: 下载 SKILL.md 失败"
    exit 1
}

echo ""
echo "==> 安装完成! Skill 目录: $SKILL_DIR"
echo "    可以使用 xworkbench-cli 命令了"
`
