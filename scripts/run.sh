#!/usr/bin/env bash
# xworkbench 启动脚本
# 用法：
#   ./scripts/run.sh                # 启动（先停旧进程 → 删二进制 → 重新编译 → 启动，确保总跑最新程序）
#   ./scripts/run.sh --stop         # 停止
#   ./scripts/run.sh --port 9090    # 自定义端口（同样重新编译）
#   ./scripts/run.sh --status       # 查看运行状态
#   ./scripts/run.sh --log         # 跟踪日志
#   ./scripts/run.sh --help        # 帮助
#
# 环境变量：
#   DB_PATH      SQLite 路径（默认 ./data/xworkbench.db）
#   ADDR         监听地址（默认 :8902，可被 --port 覆盖）
#   SKIP_BUILD   非空则跳过编译（用于只想启动已编译好的二进制）

set -euo pipefail
cd "$(dirname "$0")/.."

BIN="./bin/xworkbench"
PID_FILE="./bin/xworkbench.pid"
LOG_FILE="./bin/xworkbench.log"
DB_PATH="${DB_PATH:-./data/xworkbench.db}"
ADDR="${ADDR:-:8902}"
CONFIG_PATH="${CONFIG_PATH:-./config.json}"

usage() {
  cat <<EOF
Usage: $0 [option]

Options:
  (无)             启动（停止旧进程 → 重新编译 → 启动最新程序）
  --stop           停止
  --port PORT      自定义端口（如 9090）
  --status         查看运行状态
  --log            跟踪日志（tail -f）
  --help           显示本帮助

环境变量：
  DB_PATH      SQLite 路径（默认 ./data/xworkbench.db）
  ADDR         监听地址（默认 :8902）
  SKIP_BUILD   非空则跳过编译（用于只想启动已编译好的二进制）

文件：
  二进制：${BIN}
  PID  ：${PID_FILE}
  日志：${LOG_FILE}
EOF
}

# stop：如果进程在跑则杀掉
do_stop() {
  if [ -f "$PID_FILE" ]; then
    PID=$(cat "$PID_FILE")
    if kill -0 "$PID" 2>/dev/null; then
      echo "  stop pid=$PID"
      kill "$PID" 2>/dev/null || true
      for i in 1 2 3; do
        kill -0 "$PID" 2>/dev/null || break
        sleep 0.3
      done
      kill -0 "$PID" 2>/dev/null && kill -9 "$PID" 2>/dev/null || true
    fi
    rm -f "$PID_FILE"
  fi
}

# build：重新编译二进制
do_build() {
  if [ -n "${SKIP_BUILD:-}" ]; then
    if [ ! -f "$BIN" ]; then
      echo "SKIP_BUILD=1 但二进制不存在，无法继续"
      exit 1
    fi
    echo "  跳过编译（SKIP_BUILD=1）"
    return
  fi
  echo "  编译..."
  ./scripts/build.sh
}

# start：确保最新程序跑起来
start() {
  echo "==> 准备启动  DB_PATH=$DB_PATH  ADDR=$ADDR"

  # 1. 停旧进程
  if [ -f "$PID_FILE" ]; then
    PID=$(cat "$PID_FILE")
    if kill -0 "$PID" 2>/dev/null; then
      echo "  停止旧进程 pid=$PID"
      do_stop
    else
      rm -f "$PID_FILE"
    fi
  fi

  # 2. 删旧二进制（强制重新编译，确保总跑最新程序）
  if [ -f "$BIN" ]; then
    echo "  删除旧二进制 $BIN"
    rm -f "$BIN"
  fi

  # 3. 重新编译
  do_build

  # 4. 启动
  mkdir -p "$(dirname "$PID_FILE")"
  echo "  启动  $BIN  DB_PATH=$DB_PATH  ADDR=$ADDR"
  DB_PATH="$DB_PATH" ADDR="$ADDR" nohup "$BIN" -config "$CONFIG_PATH" > "$LOG_FILE" 2>&1 &
  PID=$!
  echo "$PID" > "$PID_FILE"
  sleep 1
  if kill -0 "$PID" 2>/dev/null; then
    echo "  ✓ pid=$PID  log=$LOG_FILE"
    echo "  浏览器: http://localhost${ADDR}"
  else
    echo "  ✗ 启动失败，查看日志：tail -f $LOG_FILE"
    tail -10 "$LOG_FILE"
    rm -f "$PID_FILE"
    exit 1
  fi
}

# status：查看运行状态
status() {
  if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
    PID=$(cat "$PID_FILE")
    echo "运行中  pid=$PID  port=$ADDR"
  else
    echo "未运行"
  fi
}

case "${1:-}" in
  --stop)           do_stop ;;
  --port)           ADDR=":$2"; start ;;
  --status)         status ;;
  --log)            tail -f "$LOG_FILE" ;;
  --help|-h)        usage ;;
  "")               start ;;
  *)                echo "unknown: $1"; usage ;;
esac