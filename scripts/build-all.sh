#!/usr/bin/env bash
# 本地交叉编译：linux/darwin/windows 全打一遍。
# 用法：./scripts/build-all.sh
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="${OUT:-/tmp/sf-cross-build}"
mkdir -p "$OUT"

for goos in linux darwin windows; do
  for arch in amd64 arm64; do
    if [[ "$goos" == "windows" && "$arch" == "arm64" ]]; then
      continue  # windows/arm64 不在常规支持内，跳过
    fi
    ext=""
    [[ "$goos" == "windows" ]] && ext=".exe"
    bin="$OUT/skill-factory-$goos-$arch$ext"
    echo "==> $goos/$arch -> $bin"
    GOOS="$goos" GOARCH="$arch" CGO_ENABLED=0 \
      go build -o "$bin" ./cmd/server
  done
done

echo
echo "built:"
ls -la "$OUT"
