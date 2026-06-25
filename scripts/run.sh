#!/bin/bash
# 注意：Windows 下如果使用 Gow bash 可能有问题，建议使用 Git Bash
# 如果遇到问题，请直接运行：& "C:\Program Files\Git\bin\bash.exe" scripts/run.sh
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
      # lsof 找到端口对应的 pid，再通过 ps 确认进程名含 xworkbench
      pid=$(lsof -i :$port 2>/dev/null | awk 'NR>1 {print $2}' | sort -u | \
        while read p; do
          name=$(ps -p "$p" -o comm= 2>/dev/null || true)
          if echo "$name" | grep -q "xworkbench"; then echo "$p"; fi
        done) || true
      ;;
    linux)
      pid=$(ss -tlnp 2>/dev/null | grep ":$port " | grep xworkbench | \
        awk '{for(i=1;i<=NF;i++) if($i~/pid=[0-9]+/) {match($i,/pid=([0-9]+)/,m); print m[1]}}' | sort -u) || true
      ;;
    windows)
      # 用 netstat 找端口对应的 pid，再用 PowerShell 确认进程名含 xworkbench
      # 注意：Windows netstat 输出是 LISTENING 而不是 LISTEN
      pid=$(netstat -ano 2>/dev/null | grep ":$port" | grep LISTENING | \
        awk '{print $NF}' | sort -u | \
        while read p; do
          name=$(powershell -NoProfile -Command \
"(Get-Process -Id $p -ErrorAction SilentlyContinue).ProcessName" 2>/dev/null) || true
          if echo "$name" | grep -qi "xworkbench"; then echo "$p"; fi
        done) || true
      ;;
  esac
  echo "$pid"
}

# 检查进程是否存活（跨平台）
is_process_alive() {
  if [ "$os" = "windows" ]; then
    powershell -NoProfile -Command \
"Get-Process -Id $1 -ErrorAction SilentlyContinue" >/dev/null 2>&1
    return $?
  fi
  kill -0 "$1" 2>/dev/null
}

# 端口占用调试输出（跨平台）
dump_port_info() {
  local port=$1
  case "$os" in
    darwin) lsof -i :$port 2>/dev/null || echo "  (无)" ;;
    linux)  ss -tlnp 2>/dev/null | grep ":$port " || echo "  (无)" ;;
    *)      netstat -ano 2>/dev/null | grep ":$port" || echo "  (无)" ;;
  esac
}

# 检查端口是否被任何进程 LISTENING（跨平台）
# 注意：只检查 LISTEN 状态，CLOSE_WAIT 等其他状态的连接不影响重新绑定
is_port_in_use() {
  local port=$(listen_port)
  case "$os" in
    darwin)
      # -sTCP:LISTEN 只匹配 LISTENING 状态，忽略 CLOSE_WAIT/TIME_WAIT 等
      lsof -i :$port -sTCP:LISTEN >/dev/null 2>&1
      ;;
    linux)
      ss -tln 2>/dev/null | grep -q ":$port "
      ;;
    windows)
      netstat -ano 2>/dev/null | grep ":$port" | grep -q LISTENING
      ;;
  esac
}

# 停止进程
stop() {
  local port=$(listen_port)
  local pid=$(find_pid_by_port)

  if [ -z "$pid" ]; then
    echo "${YELLOW}==> 停止${NC}  未运行"
    return 0
  fi

  echo "${YELLOW}==> 停止${NC}  端口 $port  pid=$pid"

  # 发送终止信号
  printf "  SIGTERM → ... "
  local term_ok=0
  if [ "$os" = "windows" ]; then
    # Windows 下控制台程序不响应 WM_CLOSE，直接用 /F 强制终止
    # Git Bash 会把 /F 转换成路径，通过 cmd.exe 调用避免此问题
    cmd.exe //c "taskkill /F /PID $pid" >/dev/null 2>&1 && term_ok=1
  else
    kill "$pid" 2>/dev/null && term_ok=1
  fi
  if [ "$term_ok" -eq 1 ]; then
    echo "${GREEN}OK${NC}"
  else
    echo "${YELLOW}失败${NC}"
  fi

  # 等待优雅退出（进程退出且端口释放）
  local remaining=""
  for i in 1 2 3 4 5 6 7 8 9 10; do
    remaining=$(find_pid_by_port)
    [ -z "$remaining" ] && break
    sleep 0.5
  done

  # 仍未退出则强制（杀掉所有找到的进程，最多重试3次）
  all_pids=$(find_pid_by_port)
  if [ -n "$all_pids" ]; then
    for i in 1 2 3; do
      echo "  ${YELLOW}SIGKILL ($i/3) →${NC}  pids=$all_pids"
      for p in $all_pids; do
        if [ "$os" = "windows" ]; then
          # Git Bash 路径转换问题：通过 cmd.exe 调用
          cmd.exe //c "taskkill /F /PID $p" >/dev/null 2>&1
        else
          kill -9 "$p" 2>/dev/null
        fi
      done
      sleep 0.3
      all_pids=$(find_pid_by_port)
      if [ -z "$all_pids" ]; then
        echo "  ${GREEN}✓ 已强制终止${NC}"
        break
      fi
    done
    if [ -n "$all_pids" ]; then
      echo "  ${RED}✗ 强制终止失败，残留 pids=$all_pids${NC}"
      dump_port_info "$port"
    fi
  fi

  # 最终确认（进程退出且端口释放）
  final_check=$(find_pid_by_port)
  if [ -z "$final_check" ]; then
    # 再等一下确保端口释放（CLOSE_WAIT 需要等待对方完成 TCP 关闭握手）
    for i in 1 2 3 4 5; do
      printf "  等待端口释放 ($i/5)... "
      sleep 1
      if ! is_port_in_use; then
        echo "${GREEN}OK${NC}"
        echo "${GREEN}✓ 已停止${NC}"
        return 0
      fi
      echo "${YELLOW}仍占用${NC}"
    done
    echo "${RED}✗ 进程已终止但端口未释放（CLOSE_WAIT 等待中）${NC}"
    dump_port_info "$port"
    echo "  提示：CLOSE_WAIT 连接会在几分钟后自动消失，可直接运行 start 启动服务"
    return 1
  else
    echo "${RED}✗ 停止失败，残留 pid=$final_check${NC}"
    dump_port_info "$port"
    return 1
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
  if is_port_in_use; then
    local existing_pid=$(find_pid_by_port)
    if [ -n "$existing_pid" ]; then
      echo "${YELLOW}⚠ 端口 $port 已被 xworkbench (pid=$existing_pid) 占用${NC}，先停止..."
      stop
      sleep 1
    else
      echo "${RED}✗ 端口 $port 被其他进程占用，无法启动${NC}"
      case "$os" in
        darwin) echo "  查看：${YELLOW}lsof -i :$port${NC}" ;;
        linux)  echo "  查看：${YELLOW}ss -tlnp | grep :$port${NC}" ;;
        *)      echo "  查看：${YELLOW}netstat -ano | findstr :$port${NC}" ;;
      esac
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

  if is_process_alive "$pid"; then
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
  --restart)
    echo "${CYAN}==> 重启${NC}"
    before_pid=$(find_pid_by_port)
    if ! stop; then
      echo "${RED}✗ 停止失败，不继续启动${NC}"
      exit 1
    fi
    sleep 1
    start
    after_pid=$(find_pid_by_port)
    echo "  ${GREEN}✓ 重启完成${NC}  pid=$after_pid (前: $before_pid)"
    ;;
  --port)           ADDR=":$2"; start ;;
  --status)         status ;;
  --log)            tail -f "${PROJECT_ROOT}/data/logs/xworkbench.log" ;;
  --help|-h)        usage ;;
  "")               start ;;
  *)                echo "${RED}✗ 未知选项：$1${NC}"; usage ;;
esac
