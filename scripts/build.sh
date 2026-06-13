#!/usr/bin/env bash
# xworkbench 构建脚本
# 用法：
#   ./scripts/build.sh              # 默认：当前平台 → ./bin/xworkbench
#   ./scripts/build.sh -a           # 三平台：./bin/xworkbench-{darwin,linux,windows}-{amd64}.exe
#   ./scripts/build.sh -c           # 清理 ./bin
#   ./scripts/build.sh -h           # 帮助
#
# 编译参数：-ldflags="-s -w"  去符号表/调试信息（节省 ~5MB）
#          -trimpath           去文件系统路径

set -euo pipefail
cd "$(dirname "$0")/.."

LDFLAGS=(-ldflags "-s -w" -trimpath)
BIN_DIR="./bin"
VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

usage() {
  cat <<EOF
Usage: $0 [option]

Options:
  (无)        编译当前平台到 ${BIN_DIR}/xworkbench
  -a, --all   编译三平台（macOS / Linux / Windows）到 ${BIN_DIR}/
  -c, --clean 清理 ${BIN_DIR}/
  -h, --help  显示本帮助

构建参数：${LDFLAGS}
版本：${VERSION}  构建时间：${BUILD_TIME}
EOF
  exit 0
}

build_current() {
  mkdir -p "$BIN_DIR"
  echo "==> 当前平台 → ${BIN_DIR}/xworkbench"
  go build "${LDFLAGS[@]}" -o "${BIN_DIR}/xworkbench" ./cmd/server
  ls -lh "${BIN_DIR}/xworkbench" | awk '{printf "    %s  %s\n", $5, $9}'
}

build_one() {
  local goos=$1 goarch=$2 ext=$3
  local bin_name="xworkbench-${VERSION}-${goos}-${goarch}${ext}"
  echo "==> ${goos}/${goarch} → ${BIN_DIR}/${bin_name}"
  GOOS=$goos GOARCH=$goarch go build "${LDFLAGS[@]}" \
    -o "${BIN_DIR}/${bin_name}" ./cmd/server
  ls -lh "${BIN_DIR}/${bin_name}" | awk '{printf "    %s  %s\n", $5, $9}'
}

build_all() {
  mkdir -p "$BIN_DIR"
  build_one darwin amd64 ""
  build_one linux amd64 ""
  build_one windows amd64 ".exe"
  echo
  echo "==> 三平台完成："
  ls -lh "${BIN_DIR}/" | awk 'NR>1 {printf "    %s  %s\n", $5, $9}'
}

clean() {
  rm -rf "${BIN_DIR}"
  echo "cleaned ${BIN_DIR}"
}

case "${1:-}" in
  -a|--all)  build_all ;;
  -c|--clean) clean ;;
  -h|--help) usage ;;
  "")        build_current ;;
  *)         echo "unknown option: $1"; usage ;;
esac
