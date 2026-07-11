# xw-ssh-mcp-server

基于 [ssh-mcp-server](https://github.com/xiaodongQ/ssh-mcp-server) 的构建工具。

## 使用方法

```bash
# 构建当前平台
./build.sh

# 构建全平台（darwin/linux/windows amd64）
./build.sh -a

# 仅拉取源码，不构建
./build.sh --clone
```

## 输出

构建完成后，当前目录会生成以下文件：

- `xw-ssh-mcp-server-darwin-amd64` - macOS amd64
- `xw-ssh-mcp-server-linux-amd64` - Linux amd64
- `xw-ssh-mcp-server-windows-amd64.exe` - Windows amd64

## 依赖

- Go 1.21+
- CGO_ENABLED=0（纯 Go 构建，无 C 依赖）
