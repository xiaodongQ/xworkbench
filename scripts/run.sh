#!/usr/bin/env bash
# xworkbench 启停脚本（只管启停，编译由 build.sh 负责）
# 用法：
#   ./scripts/run.sh                # 启动（进程已在跑则先停止再启动）
#   ./scripts/run.sh --stop        # 停止
#   ./scripts/run.sh --restart      # 重启
#   ./scripts/run.sh --port 9090   # 自定义端口
#   ./scripts/run.sh --status      # 查看运行状态
#   ./scripts/run.sh --log         # 跟踪日志
#   ./scripts/run.sh --help        # 帮助
#
# 环境变量：
#   DB_PATH  SQLite 路径（默认 ./data/xworkbench.db）
#   ADDR     监听地址（默认 :8902，可被 --port 覆盖）

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# 自动选择对应平台的二进制
case "$(uname -s)" in
  Darwin)  os=darwin ;;
  Linux)   os=linux ;;
  *)       os=windows ;;
esac
arch=$(uname -m)
case "$arch" in
  x86_64)  arch=amd64 ;;
  aarch64) arch=arm64 ;;
esac
ext=""
[ "$os" = "windows" ] && ext=".exe"
BIN="${PROJECT_ROOT}/bin/xworkbench-${os}-${arch}${ext}"
DB_PATH="${DB_PATH:-${PROJECT_ROOT}/data/xworkbench.db}"
ADDR="${ADDR:-:8902}"
CONFIG_PATH="${CONFIG_PATH:-${PROJECT_ROOT}/config.json}"

# 检测终端是否支持 ANSI 颜色（兼容 Windows cmd / PowerShell 旧版）
supports_ansi() {
  if [ "$os" != "windows" ]; then
    # Unix/Linux/macOS 一般都支持
    return 0
  fi
  # Windows 下检测
  if [ -n "${WT_SESSION:-}" ]; then
    # Windows Terminal 支持
    return 0
  fi
  if [ -n "${TERM:-}" ] && [ "$TERM" != "dumb" ]; then
    # Git Bash / MSYS2 / WSL 等
    return 0
  fi
  # 默认不支持
  return 1
}

if supports_ansi; then
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  CYAN='\033[0;36m'
  NC='\033[0m'
else
  RED='' GREEN='' YELLOW='' CYAN='' NC=''
fi

hr() { awk 'BEGIN{for(i=1;i<=50;i++) printf "="; print ""}'; }

usage() {
  cat <<EOF
Usage: $0 [option]

Options:
  (无)             启动（进程已在跑则先停止再启动）
  --stop           停止
  --restart        重启
  --port PORT      自定义端口（如 9090）
  --status         查看运行状态
  --log            跟踪日志（tail -f）
  --help           显示本帮助

环境变量：
  DB_PATH          SQLite 路径（默认 ./data/xworkbench.db）
  ADDR             监听地址（默认 :8902，可被 --port 覆盖）

文件：
  二进制：${BIN}
  日志：${PROJECT_ROOT}/data/logs/xworkbench.log
EOF
}

listen_port() {
  echo "$ADDR" | sed 's/^://'
}

# 通过端口查找 xworkbench 进程 PID（跨平台）
find_pid_by_port() {
  local port=$(listen_port)
  local pid=""
  case "$os" in
    darwin)
      pid=$(lsof -i :$port -c xworkbench 2>/dev/null | awk 'NR>1 {print $2}' | head -1) || true
      ;;
    linux)
      pid=$(ss -tp 2>/dev/null | grep ":$port" | grep xworkbench | \
        awk '{for(i=1;i<=NF;i++) if($i~/pid=[0-9]+/) {match($i,/pid=([0-9]+)/,m); print m[1]}}' | head -1) || true
      ;;
    windows)
      pid=$(netstat -ano 2>/dev/null | grep ":$port" | grep LISTEN | \
        awk '{print $NF}' | head -1) || true
      ;;
  esac
  echo "$pid"
}

# 检查端口是否被任何进程占用（跨平台）
is_port_any_in_use() {
  local port=$(listen_port)
  case "$os" in
    darwin)
      lsof -ti:$port >/dev/null 2>&1
      ;;
    linux)
      ss -tln 2>/dev/null | grep -q ":$port"
      ;;
    windows)
      netstat -ano 2>/dev/null | grep ":$port" | grep -q LISTEN
      ;;
  esac
}

# 停止进程
stop() {
  local port=$(listen_port)
  local pid=$(find_pid_by_port)

  if [ -z "$pid" ]; then
    echo "${YELLOW}==> 停止${NC}  未运行"
    return
  fi

  echo "${YELLOW}==> 停止${NC}  端口 $port  pid=$pid"
  printf "  SIGTERM → ... "
  if kill "$pid" 2>/dev/null; then
    echo "${GREEN}OK${NC}"
  else
    echo "${YELLOW}失败${NC}"
  fi

  # 等待优雅退出
  local remaining=""
  for i in 1 2 3 4 5; do
    remaining=$(find_pid_by_port)
    [ -z "$remaining" ] && break
    sleep 0.5
  done

  # 仍未退出则强制
  remaining=$(find_pid_by_port)
  if [ -n "$remaining" ]; then
    echo "  ${YELLOW}SIGKILL →${NC}  pid=$remaining"
    kill -9 "$remaining" 2>/dev/null || true
  fi

  if [ -z "$remaining" ]; then
    echo "${GREEN}✓ 已停止${NC}"
  else
    echo "${GREEN}✓ 已停止（强制）${NC}"
  fi
}

start() {
  echo
  hr
  echo "${CYAN}==> 启动 xworkbench${NC}"
  hr

  if [ ! -f "$BIN" ]; then
    echo "${RED}✗ 二进制不存在：$BIN${NC}"
    echo "  请先运行 ${CYAN}./scripts/build.sh${NC} 编译"
    exit 1
  fi

  local port=$(listen_port)

  # 端口已被占用
  if is_port_any_in_use; then
    local existing_pid=$(find_pid_by_port)
    if [ -n "$existing_pid" ]; then
      echo "${YELLOW}⚠ 端口 $port 已被 xworkbench (pid=$existing_pid) 占用${NC}，先停止..."
      stop
      sleep 1
    else
      echo "${RED}✗ 端口 $port 被其他进程占用，无法启动${NC}"
      echo "  查看：${YELLOW}lsof -i :$port${NC}"
      exit 1
    fi
  fi

  mkdir -p "$(dirname "$DB_PATH")"

  echo "  端口：${port}"
  echo "  日志：${PROJECT_ROOT}/data/logs/xworkbench.log"
  echo "  数据：${DB_PATH}"
  echo "  配置：${CONFIG_PATH}"
  echo

  local start_time=$(date +%s)
  DB_PATH="$DB_PATH" ADDR="$ADDR" nohup "$BIN" -config "$CONFIG_PATH" &
  local pid=$!
  sleep 1

  if kill -0 "$pid" 2>/dev/null; then
    local end_time=$(date +%s)
    echo "${GREEN}✓ 启动成功${NC}  pid=$pid"
    echo "  耗时：$((end_time - start_time))s"
    echo
    echo "  浏览器：${CYAN}http://localhost${ADDR}${NC}"
  else
    echo "${RED}✗ 启动失败${NC}"
    echo "  查看日志：${YELLOW}tail -f ${PROJECT_ROOT}/data/logs/xworkbench.log${NC}"
    tail -10 "${PROJECT_ROOT}/data/logs/xworkbench.log"
    exit 1
  fi
}

status() {
  local pid=$(find_pid_by_port)
  echo
  hr
  echo "${CYAN}==> xworkbench 状态${NC}"
  hr

  if [ -n "$pid" ]; then
    echo "  ${GREEN}● 运行中${NC}"
    echo "  pid：$pid"
    echo "  端口：$(listen_port)"
    echo "  二进制：$(basename "$BIN")"
  else
    echo "  ${YELLOW}○ 未运行${NC}"
    echo "  端口：$(listen_port)"
    echo "  二进制：$(basename "$BIN")"
  fi
}

case "${1:-}" in
  --stop)           stop ;;
  --restart)        echo "${CYAN}==> 重启${NC}"; stop; echo; start ;;
  --port)           ADDR=":$2"; start ;;
  --status)         status ;;
  --log)            tail -f "${PROJECT_ROOT}/data/logs/xworkbench.log" ;;
  --help|-h)        usage ;;
  "")               start ;;
  *)                echo "${RED}✗ 未知选项：$1${NC}"; usage ;;
esac
