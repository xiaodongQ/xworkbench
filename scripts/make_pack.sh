#!/usr/bin/env bash
# 打包发布包：编译当前平台 + 组装目录结构 + 生成压缩包
# 用法：./scripts/make_pack.sh
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="${OUT:-./dist}"
rm -rf "$OUT"
mkdir -p "$OUT/bin"

# 检测当前平台
case "$(uname -s)" in
  Darwin*)  os=darwin ;;
  Linux*)  os=linux ;;
  *)       os=windows ;;
esac
arch=$(uname -m)
[[ "$arch" == "x86_64" ]] && arch=amd64
[[ "$arch" == "aarch64" ]] && arch=arm64
ext=""
[[ "$os" == "windows" ]] && ext=".exe"
bin_name="xworkbench-${os}-${arch}${ext}"

# 1. 编译当前平台
echo "==> 编译 ${os}/${arch}..."
GOOS=$os GOARCH=$arch CGO_ENABLED=0 \
  go build -ldflags "-s -w" -trimpath \
  -o "$OUT/bin/$bin_name" ./cmd/server

# 2. 拷贝配置文件和脚本
echo "==> 拷贝配置文件和脚本..."
cp config.json "$OUT/"
cp scripts/run.sh "$OUT/run.sh"
chmod +x "$OUT/run.sh"

# 3. 生成 README
cat > "$OUT/README.md" << EOF
# xworkbench — 个人工作台

> 单 Go 二进制 · 6 Tab（总览 / 任务 / 经验库 / 自动化 / AI 对话 / 代理）跨平台

## 快速启动

### macOS / Linux
\`\`\`bash
./run.sh              # 启动（默认 :8902）
./run.sh --stop       # 停止
./run.sh --status     # 查看状态
./run.sh --log        # 查看日志
\`\`\`

### Windows
\`\`\`cmd
run.bat              # 启动
run.bat --stop       # 停止
run.bat --status     # 状态
run.bat --help       # 帮助
\`\`\`

然后浏览器打开 http://localhost:8902

## 目录结构

\`\`\`
.
├── run.sh / run.bat  # 启停脚本
├── config.json       # 配置文件
├── bin/
│   └── $bin_name
└── README.md
\`\`\`

## 配置

编辑 \`config.json\` 修改监听端口、API Key 等。默认端口 \`8902\`。

## 数据

- 数据：\`data/xworkbench.db\`（首次启动自动创建）
- 日志：\`bin/xworkbench.log\`
EOF

# 4. 打包
echo "==> 打包..."
cd "$OUT"
if [[ "$os" == "windows" ]]; then
  zip -r "../xworkbench-${os}-${arch}.zip" . > /dev/null
else
  tar czf "../xworkbench-${os}-${arch}.tar.gz" .
fi
cd ..

echo
echo "==> 输出：xworkbench-${os}-${arch}.$([ "$os" == "windows" ] && echo "zip" || echo "tar.gz")"
echo "==> dist 目录结构："
find "$OUT" -type f | sort
