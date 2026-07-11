# 远程终端登录认证方案分析

> 2026-07-11

## 1. 背景与问题

xworkbench 有两种远程终端场景，均涉及 SSH 认证：

| 场景 | 描述 | 当前认证支持 |
|------|------|-------------|
| **外部终端** | 点击"打开外部终端"，唤起 WezTerm/iTerm2 等外部终端应用，由终端应用自行连接远程 | 仅密钥认证，密码方式无法工作 |
| **页面远程终端** | Web UI 内嵌 xterm.js，xworkbench 自己持有 SSH 连接并渲染 PTY | 密码认证（golang.org/x/crypto/ssh）✅ |

**核心问题**：外部终端场景下，密码认证无法工作——终端应用不知道密码，密钥方式也因为路径转义 bug 导致失败（已修复）。

---

## 2. 现有实现分析

### 2.1 外部终端路径

**入口**：`handleDirShortcutOpenTerminal`（main.go） → `OpenRemoteDirShortcut`（terminal.go） → `execRemoteTerminal`

**调用链**：
```
handleDirShortcutOpenTerminal
  → OpenRemoteDirShortcut(termType, dir)
    → executor.BuildSSHCommand(dir, termType)   // 构造 ssh 命令参数
    → execRemoteTerminal(termType, binPath, args)
      → exec.Command(bin, localArgs...).Start() // 启动终端应用
```

**BuildSSHCommand**（ssh_command_builder.go）：构造 ssh 命令，返回 `[]string`，占位符 `{key_path}` 由 `ResolveKeyPath` 解析实际私钥路径。

**execRemoteTerminal**：根据终端类型（wezterm/iterm2/wt）拼接本地唤起参数，例如 WezTerm 用 `wezterm start -- ssh -i ... user@host -t -- sh -c '...'`

**认证方式**：
- 密钥认证：构造 `-i /path/to/key` 参数（已修复路径引号 bug）
- 密码认证：❌ 不支持（终端应用不知道密码，无法自动填写）

### 2.2 页面远程终端路径

**入口**：`handlePty`（main.go） → PTY 创建 → SSH 连接在 xworkbench 进程内

**认证**：走 `RunSSHViaConfig`（ssh_runner.go），基于 `golang.org/x/crypto/ssh`，密码通过 `ssh.Password()` 传入 SSH 协议层。

**PTY**：creack/pty（Unix）或 ConPTY（Windows），xworkbench 持有连接。

---

## 3. 方案对比

### 方案 A：维持现状 + 修复密钥 bug（已完成）

**改动**：修复 `ssh_command_builder.go` 第 59 行，`shellQuote(keyPath)` 改为直接用 `keyPath`（绝对路径不需要引号包裹）。

**效果**：外部终端的密钥认证正常工作，密码认证仍不支持。

**结论**：够用，但密码认证场景缺失。

---

### 方案 B：win-sshpass 作为外部子工具（推荐）

**思路**：编译 win-sshpass（github.com/chuccp/win-sshpass）为独立二进制 `xw-ssh`，xworkbench 通过进程调用它来建立 SSH 连接，外部终端应用连接到 xw-sshpass 的本地 PTY。

**架构**：
```
xworkbench
  → 启动 xw-sshpass 子进程：xw-sshpass -p '密码' ssh user@host
  → xw-sshpass 建 SSH 连接，创建本地 PTY（golang.org/x/crypto/ssh）
  → 外部终端应用（WezTerm）连接到 xw-sshpass 的本地 PTY
  → xworkbench 只管启动子进程，不持有 SSH 连接
```

**优势**：
- SSH 连接在子进程，不影响 xworkbench 稳定性
- xworkbench 崩溃不影响已建立的终端连接（子进程独立）
- 跨平台（macOS/Linux/Windows），一次编码到处运行
- 支持密码认证和密钥认证
- 支持交互式 PTY（raw mode，正确 echo，Ctrl+C 信号转发，vim/top 等全屏应用）
- 支持 rz/sz 文件传输降级到 SFTP

**劣势**：
- 需要在构建时编译 win-sshpass 二进制
- 外部终端连接的是本地 PTY，不是远程原生 SSH（WezTerm 的多 pane、搜索等高级功能受限）
- 需要处理子进程的启停和生命周期管理

**实现要点**：
- `scripts/build.sh` 中加入 `go build -o data/bin/xw-sshpass ./cmd/sshpass`（win-sshpass 的 cmd）
- `execRemoteTerminal` 改为调用 `xw-sshpass -p '密码' ssh user@host`，而不是直接调用 ssh
- 二进制放在 `data/bin/` 目录，随 xworkbench 分发

---

### 方案 C：win-sshpass 作为 Go 库引用

**思路**：直接 import `github.com/chuccp/win-sshpass`，在 xworkbench 进程内调用 `sshpass.NewClient()` 和 `client.Shell()`。

**架构**：
```
xworkbench 进程内
  → sshpass.NewClient(cfg) → ssh.Client（golang.org/x/crypto/ssh）
  → sshpass.Shell() → 创建 PTY
  → PTY fd 通过 wezterm 方式传给外部终端
  → SSH 连接和 xworkbench 进程共存亡
```

**优势**：
- 无需编译独立二进制
- 代码更集中

**劣势**：
- SSH 连接在 xworkbench 进程内，xworkbench 崩溃则连接断
- 需要重写 `execRemoteTerminal` 的整个调用方式
- PTY 处理逻辑和 xworkbench 深度耦合，调试困难

**结论**：稳定性不如方案 B，不推荐。

---

### 方案 D：expect 脚本包装（不推荐）

**思路**：用 expect 脚本包装 ssh 命令，自动填写密码。

**局限**：
- macOS 默认没有 expect，需要手动安装
- Windows 完全不支持
- 需要额外依赖

**结论**：跨平台性差，不考虑。

---

## 4. 推荐方案：方案 B 详细设计

### 4.1 构建集成

在 `scripts/build.sh` 中加入：

```bash
# 构建 xw-sshpass 子工具（跨平台 SSH 密码认证工具）
git submodule add https://github.com/chuccp/win-sshpass third_party/win-sshpass 2>/dev/null || true
GOOS=$(go env GOOS) GOARCH=$(go env GOARCH) go build -o data/bin/xw-sshpass-${GOOS}-${GOARCH} ./third_party/win-sshpass/cmd/sshpass
```

或者作为独立步骤：

```bash
# 手动构建（首次需要）
git clone https://github.com/chuccp/win-sshpass /tmp/win-sshpass
GOOS=darwin GOARCH=amd64 go build -o data/bin/xw-sshpass-darwin-amd64 /tmp/win-sshpass/cmd/sshpass
```

### 4.2 目录结构

```
xworkbench/
  tools/
    xw-sshpass/
      xw-sshpass-darwin-amd64    # macOS x64（本地编译）
      xw-sshpass-darwin-arm64    # macOS ARM64
      xw-sshpass-linux-amd64     # Linux x64
      xw-sshpass-windows-amd64.exe
```

**不纳入 git**。本地从 win-sshpass 源码编译：
```bash
cd tools/xw-sshpass
./build.sh              # 当前平台
./build.sh -a           # 全平台（darwin/linux/windows amd64）
```

运行时找不到 xw-sshpass 则回退到原有密钥认证路径（不阻塞）。

### 4.3 execRemoteTerminal 改造

**当前逻辑**（ssh_command_builder.go → execRemoteTerminal）：
```go
args, _ := executor.BuildSSHCommand(dir, termType)
// args: [ssh, -i, /path/key, user@host, -t, --, sh, -c, '...']
exec.Command(bin, localArgs...).Start()
```

**改造后**（密码方式走 xw-sshpass）：
```go
if dir.AuthMethod == "password" && dir.RemotePassword != "" {
    // 走 xw-sshpass 子进程
    bin := resolveXwSshpassBin() // e.g. data/bin/xw-sshpass-darwin-amd64
    sshArgs := buildXwSshpassArgs(dir) // [-p, password, ssh, user@host, -t, --, sh, -c, '...']
    exec.Command(bin, sshArgs...).Start()
} else {
    // 密钥方式走原有逻辑
    args, _ := executor.BuildSSHCommand(dir, termType)
    execRemoteTerminal(termType, binPath, args)
}
```

### 4.4 xw-sshpass 二进制选择逻辑

```go
func resolveXwSshpassBin() string {
    goos := runtime.GOOS
    goarch := runtime.GOARCH
    // 优先用内嵌的，无则尝试 PATH 中的 xw-sshpass
    base := filepath.Join(paths.DataDir(), "bin", "xw-sshpass")
    ext := ""
    if goos == "windows" {
        ext = ".exe"
    }
    bin := fmt.Sprintf("%s-%s-%s%s", base, goos, goarch, ext)
    if _, err := os.Stat(bin); err == nil {
        return bin
    }
    // fallback 到 PATH
    if bin, err := exec.LookPath("xw-sshpass"); err == nil {
        return bin
    }
    return bin // 让错误信息清晰
}
```

### 4.5 密码安全注意事项

- 密码通过命令行参数传入子进程（`ps aux` 可能可见），和 sshpass 的设计一致
- 建议未来改用 `SSHPASS` 环境变量方式传入，减少命令行暴露
- 远程服务器日志中仍会记录登录信息（这是 SSH 协议本身的行为）

---

## 5. win-sshpass 简介

### 5.1 项目信息

- **仓库**：github.com/chuccp/win-sshpass
- **协议**：MIT / Apache 2.0
- **Go 版本**：1.23+
- **跨平台**：Windows/macOS/Linux，x64 + ARM64

### 5.2 核心能力

| 能力 | 说明 |
|------|------|
| 密码认证 | `-p password` 或 `SSHPASS=password xw-sshpass -e ssh ...` |
| 密钥认证 | `-i /path/to/key` |
| 交互式 PTY | raw mode，正确 echo，Ctrl+C 转发，vim/top 支持 |
| 终端自适应 | SIGWINCH（Unix）/ 250ms 轮询（Windows）动态调整窗口大小 |
| SFTP 传输 | 上传/下载，带进度条，`rz`/`sz` 命令自动降级 |
| 连接重试 | 指数退避，默认 3 次 |

### 5.3 依赖（与 xworkbench 重合）

```
golang.org/x/crypto/ssh   # SSH 协议 ← xworkbench 已在用
github.com/pkg/sftp       # SFTP     ← xworkbench 已在用
golang.org/x/term         # 终端尺寸
```

**无新增外部依赖**。

---

## 6. 实施计划

### Phase 1：基础设施（0.5 天）
- [ ] 从 win-sshpass releases 下载预编译二进制，存入 `tools/`（xw-sshpass-darwin-amd64 / xw-sshpass-darwin-arm64 / xw-sshpass-linux-amd64 / xw-sshpass-linux-arm64 / xw-sshpass-windows-amd64.exe）
- [ ] 实现 `resolveXwSshpassBin()` 和 `buildXwSshpassArgs()`（找不到 binary 时 SKIP，不阻塞）

### Phase 2：execRemoteTerminal 改造（0.5 天）
- [ ] 在 `execRemoteTerminal` 中增加密码方式分支
- [ ] 密钥方式保持走原有 `BuildSSHCommand` 路径
- [ ] 端到端测试：WezTerm 密码方式打开远程目录

### Phase 3：清理与文档（0.5 天）
- [ ] 删除或标注不再使用的旧代码路径
- [ ] 更新 DESIGN.md 的远程终端架构图
- [ ] 测试 macOS/Linux/Windows 三平台

---

## 7. 附录：两种终端场景的架构差异

```
┌─────────────────────────────────────────────────────────────┐
│                      外部终端（方案 B）                       │
│                                                             │
│  xworkbench    xw-sshpass 子进程      WezTerm/iTerm2        │
│  ──────────    ──────────────────     ──────────────────    │
│  启动子进程 ──►│ 建 SSH 连接           │                     │
│               ││ 创建本地 PTY          │                     │
│               │                       │◄── 连接本地 PTY ────►│
│  不持有 SSH   │                       │  WezTerm 渲染 PTY    │
│  连接         │                       │                     │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                    页面远程终端（已有）                       │
│                                                             │
│  xworkbench 进程内                                          │
│  ─────────────────                                          │
│  golang.org/x/crypto/ssh ← 密码通过 SSH 协议传入             │
│         │                                                   │
│         ▼                                                   │
│  creack/pty ← PTY 由 xworkbench 持有                        │
│         │                                                   │
│         ▼                                                   │
│  WebSocket ──► xterm.js（浏览器渲染）                        │
│                                                             │
│  xworkbench 崩溃则连接断                                     │
└─────────────────────────────────────────────────────────────┘
```

---

## 8. 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-11 | 创建文档 |
| 2026-07-11 | 分析 win-sshpass 与 xworkbench 的集成方案，确定方案 B（外部子工具）为推荐方案 |
