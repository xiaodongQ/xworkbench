#!/usr/bin/env bash
# xworkbench 构建脚本
# 用法：
#   ./scripts/build.sh              # 编译当前平台
#   ./scripts/build.sh -a           # 编译全平台
#   ./scripts/build.sh -c           # 清理 bin/
#   ./scripts/build.sh -h           # 帮助
#
# 编译参数：-ldflags="-s -w"  去符号表/调试信息（节省 ~5MB）
#          -trimpath           去文件系统路径

cd "$(dirname "$0")/.."

LDFLAGS=("-ldflags=-s -w" -trimpath)
BIN_DIR="./bin"
VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

hr() { printf '=%.0s' $(seq 1 50); printf '\n'; }

usage() {
  cat <<EOF
Usage: $0 [option]

Options:
  (无)        编译当前平台
  -a, --all   编译全平台（darwin/linux/windows 各 amd64+arm64）
  -c, --clean 清理 bin/
  -h, --help  显示本帮助

构建参数：${LDFLAGS}
版本：${VERSION}  构建时间：${BUILD_TIME}
EOF
  exit 0
}


# 检测当前平台
detect_platform() {
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
}

build_current() {
  echo "build_current..."
  detect_platform
  local bin_name="xworkbench-${os}-${arch}${ext}"
  echo "bin_name: ${bin_name}"
  mkdir -p "$BIN_DIR"

  echo
  hr
  printf '%b\n' "${CYAN}==> 开始编译${NC}  ${os}/${arch}"
  printf '%b\n' "    输出：${BIN_DIR}/${bin_name}"
  printf '%b\n' "    版本：${VERSION}"
  printf '%b\n' "    时间：${BUILD_TIME}"
  hr

  local start_time=$(date +%s)
  if GOOS=$os GOARCH=$arch go build "${LDFLAGS[@]}" -o "${BIN_DIR}/${bin_name}" ./cmd/server; then
    local end_time=$(date +%s)
    local size=$(ls -lh "${BIN_DIR}/${bin_name}" | awk '{print $5}')
    echo
    printf '%b\n' "${GREEN}✓ 编译成功${NC}  ${size}  ${bin_name}"
    printf '%b\n' "    耗时：$((end_time - start_time))s"
  else
    echo
    printf '%b\n' "${RED}✗ 编译失败${NC}"
    exit 1
  fi
}

build_one() {
  local goos=$1 goarch=$2 ext=$3
  local bin_name="xworkbench-${goos}-${goarch}${ext}"
  local start_time=$(date +%s)

  printf "  %-12s %-8s ... " "$goos" "$goarch"
  if GOOS=$goos GOARCH=$goarch go build "${LDFLAGS[@]}" -o "${BIN_DIR}/${bin_name}" ./cmd/server 2>&1; then
    local end_time=$(date +%s)
    local size=$(ls -lh "${BIN_DIR}/${bin_name}" | awk '{print $5}')
    printf '%b\n' "${GREEN}[OK]${NC}  $size"
  else
    printf '%b\n' "${RED}[FAIL]${NC}"
    return 1
  fi
}

build_all() {
  mkdir -p "$BIN_DIR"

  echo
  hr
  printf '%b\n' "${CYAN}==> 全平台编译${NC}  版本=${VERSION}"
  hr

  local start_time=$(date +%s)
  local failed=0

  build_one darwin amd64 "" || failed=1
  build_one darwin arm64 "" || failed=1
  build_one linux amd64 "" || failed=1
  build_one linux arm64 "" || failed=1
  build_one windows amd64 ".exe" || failed=1

  local end_time=$(date +%s)
  echo
  hr
  printf '%b\n' "${CYAN}==> 编译完成${NC}  耗时：$((end_time - start_time))s"
  hr
  ls -lh "${BIN_DIR}/" | awk 'NR>1 {printf "  %-12s %s\n", $9, $5}'

  if [ $failed -eq 1 ]; then
    echo
    printf '%b\n' "${RED}✗ 部分平台编译失败${NC}"
    exit 1
  fi
}

clean() {
  printf '%b\n' "${YELLOW}==> 清理 bin/ 目录${NC}"
  rm -rf "${BIN_DIR}"
  printf '%b\n' "${GREEN}✓ 已清理${NC}"
}

case "${1:-}" in
  -a|--all)  build_all ;;
  -c|--clean) clean ;;
  -h|--help) usage ;;
  "")        build_current ;;
  *)         printf '%b\n' "${RED}✗ 未知选项：$1${NC}"; usage ;;
esac
