# xw-sshpass

xw-sshpass 是 [win-sshpass](https://github.com/chuccp/win-sshpass) 的跨平台预编译产物，用于 xworkbench 外部终端密码认证场景。

## 背景

xworkbench 点击"打开外部终端"时，需要 SSH 连接到远程目录。密钥认证方式已支持，密码认证方式通过此工具实现：

```
xworkbench → xw-sshpass → 本地 PTY ← WezTerm/iTerm2 连接并渲染
```

## 架构支持

| 文件名 | 平台 |
|--------|------|
| `xw-sshpass-darwin-amd64` | macOS Intel |
| `xw-sshpass-linux-amd64` | Linux x64 |
| `xw-sshpass-windows-amd64.exe` | Windows x64 |

## 构建

```bash
cd tools/xw-sshpass

# 构建当前平台（自动拉取源码）
./build.sh

# 构建全平台（darwin/linux/windows amd64）
./build.sh -a

# 仅拉取源码，不构建
./build.sh --clone
```

## 手动拉取源码

如果需要更新 win-sshpass，先删掉源码目录再重新构建：

```bash
rm -rf tools/xw-sshpass/win-sshpass
./build.sh
```

## 权限

构建完成后赋予执行权限（build.sh 已自动处理）：

```bash
chmod +x tools/xw-sshpass/xw-sshpass-*
```

## 部署

产物**不纳入 git**，由上线流程带入部署机器。运行时若找不到 xw-sshpass，则回退到原有密钥认证路径（不阻塞）。
