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

# 5. 写入 SKILL.md
cat > "$SKILL_DIR/SKILL.md" << 'SKILLEOF'
---
name: xworkbench-cli
description: 操作 xworkbench 工作台（任务/执行/经验库/调度/todo/配置管理）
version: 1.0.0
xw_command: xworkbench-cli
xw_params:
  task_list: "列出任务 (--status 状态 --limit 数量)"
  task_create: "创建任务 (--title 标题 --description 描述)"
  task_get: "获取任务详情 (task_id)"
  task_update: "更新任务 (task_id --title --description --status --priority)"
  task_run: "运行任务 (task_id)"
  task_cancel: "取消任务 (task_id)"
  task_delete: "删除任务 (task_id)"
  exec_list: "列出执行记录 (--task-id --limit)"
  exec_get: "获取执行详情 (execution_id)"
  exec_evaluate: "AI 评估执行 (execution_id)"
  exec_cancel: "取消执行 (execution_id)"
  exec_continue: "继续对话 (execution_id)"
  experience_list: "列出经验库 (--module)"
  experience_get: "获取经验详情 (experience_id)"
  experience_create: "创建经验 (--module --scene --keywords ...)"
  experience_update: "更新经验 (experience_id ...)"
  experience_delete: "删除经验 (experience_id)"
  scheduled_list: "列出定时任务"
  scheduled_get: "获取定时任务详情 (scheduled_id)"
  scheduled_create: "创建定时任务 (--name --cron ...)"
  scheduled_update: "更新定时任务 (scheduled_id ...)"
  scheduled_delete: "删除定时任务 (scheduled_id)"
  scheduled_run: "立即运行定时任务 (scheduled_id)"
  scheduled_toggle: "切换启用/禁用 (scheduled_id)"
  todo_list: "列出 todo"
  todo_add: "添加 todo (--content)"
  todo_toggle: "切换完成状态 (line_no)"
  todo_edit: "编辑 todo (line_no content)"
  config_get: "获取配置 (key 可选)"
  config_set: "设置配置 (key value)"
  models_list: "列出可用模型"
  stats: "获取统计信息"
  dir_shortcut_list: "列出目录快捷"
  dir_shortcut_create: "创建目录快捷 (--name --path)"
  dir_shortcut_update: "更新目录快捷 (id --name --path)"
  dir_shortcut_delete: "删除目录快捷 (id)"
  dir_shortcut_open: "打开目录 (id)"
  web_link_list: "列出链接快捷"
  web_link_create: "创建链接快捷 (--title --url)"
  web_link_update: "更新链接快捷 (id --title --url)"
  web_link_delete: "删除链接快捷 (id)"
  web_link_open: "打开链接 (id)"
xw_output:
  ok: "成功标志 (true/false)"
  data: "API 返回的数据"
  error: "错误信息"
---

# xworkbench-cli Skill
SKILLEOF

echo ""
echo "==> 安装完成! Skill 目录: $SKILL_DIR"
echo "    可以使用 xworkbench-cli 命令了"
`
