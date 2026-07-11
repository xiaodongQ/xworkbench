#!/usr/bin/env bash
# xw-ssh-mcp-server 构建脚本
# 用法：
#   ./build.sh              # 构建当前平台
#   ./build.sh -a           # 构建全平台（darwin/linux/windows amd64）
#   ./build.sh --clone      # 仅拉取源码，不构建
#
# 产物输出到当前目录
# 依赖：Go，CGO_ENABLED=0（纯 Go，无 C 依赖）

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TMP_DIR="${SCRIPT_DIR}/ssh-mcp-server_tmp-source"
REPO="git@github.com:xiaodongQ/ssh-mcp-server.git"

do_clone() {
    echo "==> 拉取 ssh-mcp-server latest"
    if [ -d "$TMP_DIR" ]; then
        echo "  已存在，可手动 rm -rf ${TMP_DIR} 重新拉取"
    else
        git clone --depth 1 "$REPO" "$TMP_DIR"
        echo "✓ 完成"
    fi
}

detect_platform() {
    case "$(uname -s)" in
        Darwin)  goos=darwin ;;
        Linux)   goos=linux ;;
        *)       goos=windows ;;
    esac
    arch=$(uname -m)
    case "$arch" in
        x86_64)  goarch=amd64 ;;
        aarch64) goarch=arm64 ;;
    esac
    ext=""
    [ "$goos" = "windows" ] && ext=".exe"
}

# 单平台构建函数
xw_build_one() {
    local goos=$1 goarch=$2 suffix=$3
    local output="xw-ssh-mcp-server-${goos}-${goarch}${suffix}"
    printf "  %-12s %-8s ... " "$goos" "$goarch"
    (cd "${TMP_DIR}" && CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build \
        -ldflags="-s -w" \
        -trimpath \
        -o "${SCRIPT_DIR}/${output}" \
        ./cmd/server) 2>&1
    if [ $? -eq 0 ]; then
        local size=$(ls -lh "${SCRIPT_DIR}/${output}" | awk '{print $5}')
        echo "[OK] $size"
    else
        echo "[FAIL]"
        return 1
    fi
}

do_build() {
    detect_platform
    echo "=============================================="
    echo "xw-ssh-mcp-server 构建"
    echo "  平台: ${goos}/${goarch}"
    echo "  输出目录: ${SCRIPT_DIR}"
    echo "  CGO: disabled（纯 Go）"
    echo "=============================================="

    if [ ! -d "$TMP_DIR" ]; then
        echo
        echo "==> 源码不存在，自动拉取"
        do_clone
    else
        echo
        echo "==> 使用已有源码: ${TMP_DIR}"
    fi

    echo
    echo "==> 开始构建"
    xw_build_one "$goos" "$goarch" "$ext"

    echo
    echo "==> 设置执行权限"
    chmod +x "${SCRIPT_DIR}"/xw-ssh-mcp-server-${goos}-${goarch}${ext}

    echo
    echo "=============================================="
    echo "构建完成："
    ls -lh "${SCRIPT_DIR}"/xw-ssh-mcp-server-* | awk '{print "  " $9 "  " $5}'
    echo "=============================================="
}

do_build_all() {
    echo "=============================================="
    echo "xw-ssh-mcp-server 全平台构建"
    echo "  ssh-mcp-server: latest"
    echo "  输出目录: ${SCRIPT_DIR}"
    echo "  CGO: disabled（纯 Go）"
    echo "=============================================="

    if [ ! -d "$TMP_DIR" ]; then
        echo
        echo "==> 源码不存在，自动拉取"
        do_clone
    else
        echo
        echo "==> 使用已有源码: ${TMP_DIR}"
    fi

    echo
    echo "==> 开始构建"

    xw_build_one darwin amd64 ""
    xw_build_one linux amd64 ""
    # xw_build_one darwin arm64 ""
    # xw_build_one linux arm64 ""
    xw_build_one windows amd64 ".exe"

    echo
    echo "==> 设置执行权限"
    chmod +x "${SCRIPT_DIR}"/xw-ssh-mcp-server-*

    echo
    echo "=============================================="
    echo "构建完成："
    ls -lh "${SCRIPT_DIR}"/xw-ssh-mcp-server-* | awk '{print "  " $9 "  " $5}'
    echo "=============================================="
}

case "${1:-}" in
    -a|--all)  do_build_all ;;
    --clone)   do_clone ;;
    *)         do_build ;;
esac
