#!/usr/bin/env bash
# 打包发布包：编译当前平台 + 组装目录结构 + 生成压缩包
# 用法：./scripts/make_pack.sh
set -euo pipefail
cd "$(dirname "$0")/.."

# 检测当前平台
case "$(uname -s)" in
  Darwin*)  os=darwin ;;
  Linux*)   os=linux ;;
  *)        os=windows ;;
esac
arch=$(uname -m)
[[ "$arch" == "x86_64" ]] && arch=amd64
[[ "$arch" == "aarch64" ]] && arch=arm64
ext=""
[[ "$os" == "windows" ]] && ext=".exe"
bin_name="xworkbench-${os}-${arch}${ext}"

# 产物目录：dist/xworkbench_YYYYMMDD
# 支持环境变量 DATE_TAG 覆盖（用于 release 时传入版本号，如 DATE_TAG=1.0.0）
# 去掉 v 前缀以匹配 Git tag 格式
version_tag="${DATE_TAG:-$(date +%Y%m%d)}"
version_tag="${version_tag#v}"
pkg_dir="xworkbench_${version_tag}"
OUT="${OUT:-./dist}/${pkg_dir}"
rm -rf "$OUT"
mkdir -p "$OUT/bin" "$OUT/data" "$OUT/scripts"

# 1. 编译当前平台
echo "==> 编译 ${os}/${arch}..."
GOOS=$os GOARCH=$arch CGO_ENABLED=0 \
  go build -ldflags "-s -w" -trimpath \
  -o "$OUT/bin/$bin_name" ./cmd/server

# 2. 拷贝配置文件模板、脚本和工具目录（与仓库结构保持一致）
echo "==> 拷贝配置文件模板和脚本..."
# 配置文件模板放 data/ 下（run.sh 默认读取 data/config.json）
cp config.template.json "$OUT/data/config.json"
cp scripts/run.sh "$OUT/scripts/"
chmod +x "$OUT/scripts/run.sh"

# 拷贝 Windows PowerShell 脚本
cp scripts/run_background.ps1 "$OUT/scripts/" 2>/dev/null || true

# 3. 拷贝 tools 工具目录
if [ -d "tools" ]; then
  echo "==> 拷贝 tools 工具目录..."
  cp -r tools "$OUT/"
fi

# 4. 生成 README
cat > "$OUT/README.md" << 'READEOF'
# xworkbench — 个人工作台

> 单 Go 二进制 · 7 Tab（总览 / 任务 / 经验库 / 自动化 / 系统配置 / AI 对话 / 代理）跨平台

## 快速启动

### macOS / Linux
```bash
./scripts/run.sh        # 启动（默认 :8902）
./scripts/run.sh --stop # 停止
./scripts/run.sh --status # 查看状态
./scripts/run.sh --log   # 查看日志
```

### Windows
```powershell
# 启动（后台运行，可关闭终端）
powershell -ExecutionPolicy Bypass -File scripts/run_background.ps1
powershell -ExecutionPolicy Bypass -File scripts/run_background.ps1 -Action stop  # 停止
powershell -ExecutionPolicy Bypass -File scripts/run_background.ps1 -Action status  # 状态
```

然后浏览器打开 http://localhost:8902

## 目录结构

```
.
├── bin/                # 程序目录
│   └── BINPLACEHOLDER
├── data/               # 数据目录
│   ├── config.json     # 配置文件（首次启动自动读取模板）
│   └── xworkbench.db   # 数据库（首次启动自动创建）
├── scripts/            # 脚本目录
│   ├── run.sh          # macOS/Linux 启动脚本
│   └── run_background.ps1  # Windows 后台运行脚本
├── tools/              # 工具目录（健康检查/通知等）
└── README.md
```

## 配置

编辑 `data/config.json` 修改监听端口、API Key 等。默认端口 `8902`。

## 数据

- 数据库：`data/xworkbench.db`（首次启动自动创建）
- 日志：`data/logs/`
READEOF

# 修正 README 中的二进制文件名
sed -i "s|BINPLACEHOLDER|${bin_name}|g" "$OUT/README.md"

# 5. 打包（对 xworkbench_日期 目录打包）
echo "==> 打包..."
dist_dir="$(dirname "$OUT")"
pkg_file="${pkg_dir}-${os}-${arch}.$([ "$os" == "windows" ] && echo "zip" || echo "tar.gz")"
if [[ "$os" == "windows" ]]; then
  (cd "$dist_dir" && zip -r "$pkg_file" "$pkg_dir" > /dev/null)
else
  tar czf "$dist_dir/$pkg_file" -C "$dist_dir" "$pkg_dir"
fi

echo
echo "==> 输出：$dist_dir/$pkg_file"
echo "==> 产物目录结构："
find "$OUT" -type f -o -type d | sort
