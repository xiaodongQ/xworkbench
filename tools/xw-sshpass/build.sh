#!/usr/bin/env bash
# xw-sshpass 构建脚本
# 用法：
#   ./build.sh              # 构建当前平台
#   ./build.sh -a           # 构建全平台（darwin/linux/windows amd64）
#   ./build.sh --clone      # 仅拉取源码，不构建
#
# 产物输出到当前目录
# 依赖：Go，CGO_ENABLED=0（纯 Go，无 C 依赖）

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TMP_DIR="${SCRIPT_DIR}/win-sshpass_tmp-source"
REPO="https://github.com/chuccp/win-sshpass.git"

do_clone() {
    echo "==> 拉取 win-sshpass latest"
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

do_build() {
    detect_platform
    echo "=============================================="
    echo "xw-sshpass 构建"
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
    build_one "$goos" "$goarch" "$ext"

    echo
    echo "==> 设置执行权限"
    chmod +x "${SCRIPT_DIR}"/xw-sshpass-${goos}-${goarch}${ext}

    echo
    echo "==> 清理源码目录"
    rm -rf "${TMP_DIR}"

    echo
    echo "=============================================="
    echo "构建完成："
    ls -lh "${SCRIPT_DIR}"/xw-sshpass-* | awk '{print "  " $9 "  " $5}'
    echo "=============================================="
}

do_build_all() {
    echo "=============================================="
    echo "xw-sshpass 全平台构建"
    echo "  win-sshpass: latest"
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

    build_one() {
        local goos=$1 goarch=$2 suffix=$3
        local output="xw-sshpass-${goos}-${goarch}${suffix}"
        printf "  %-12s %-8s ... " "$goos" "$goarch"
        (cd "${TMP_DIR}" && CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build \
            -ldflags="-s -w" \
            -trimpath \
            -o "${SCRIPT_DIR}/${output}" \
            ./cmd/sshpass) 2>&1
        if [ $? -eq 0 ]; then
            local size=$(ls -lh "${SCRIPT_DIR}/${output}" | awk '{print $5}')
            echo "[OK] $size"
        else
            echo "[FAIL]"
            exit 1
        fi
    }

    build_one darwin amd64 ""
    build_one linux amd64 ""
    # build_one darwin arm64 ""
    # build_one linux arm64 ""
    build_one windows amd64 ".exe"

    echo
    echo "==> 设置执行权限"
    chmod +x "${SCRIPT_DIR}"/xw-sshpass-*

    echo
    echo "==> 清理源码目录"
    rm -rf "${TMP_DIR}"

    echo
    echo "=============================================="
    echo "构建完成："
    ls -lh "${SCRIPT_DIR}"/xw-sshpass-* | awk '{print "  " $9 "  " $5}'
    echo "=============================================="
}

case "${1:-}" in
    -a|--all)  do_build_all ;;
    --clone)   do_clone ;;
    *)         do_build ;;
esac
