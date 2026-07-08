# SSH Command Builder 重构设计

> 状态：设计中 · 2026-07-09

## 1. 背景与动机

### 1.1 当前症状

用户点击"快捷目录 → 打开远程终端"时，远程会话无法建立。服务端日志与 wezterm 输出：

```
ssh -o 'KexAlgorithms=+diffie-hellman-group1-sha1,...' \
    -o 'HostKeyAlgorithms=+ssh-rsa,ssh-dss' \
    -o 'Ciphers=+3des-cbc,...' \
    ssh -i '/Users/xd/.ssh/xworkbench_id_ed25519' \
    root@192.168.1.150 -t -- sh -c 'cd '/home/workspace' && exec $SHELL -l'

Warning: Identity file '/Users/xd/.ssh/xworkbench_id_ed25519' not accessible:
         No such file or directory.
ssh: Could not resolve hostname ssh: nodename nor servname provided, or not known
```

退出码 255。

### 1.2 根因

源码 [`internal/shortcuts/terminal.go:101-105`](../../internal/shortcuts/terminal.go#L101)：

```go
if ok && len(termDef.RemoteArgs) > 0 {
    template = append([]string{"ssh"}, baseArgs...)
    template = append(template, termDef.RemoteArgs...)
}
```

而 [`config.template.conf:26`](../../config.template.conf#L26) 的 `remote_args` 模板自身首元素就是 `"ssh"`：

```jsonc
"remote_args": ["ssh", "-i", "{key_path}", "{user}@{host}", "-t", "--", "sh", "-c", "{shell_cmd}"]
```

两个 `"ssh"` 拼接后变 `[ssh, -o ..., ssh, -i, ..., root@host, ...]`，第二个 `ssh` 被 ssh 进程当作主机名 → `Could not resolve hostname ssh`。

附带两个次要问题：
1. **私钥不存在时仍传 `-i`**：兜底分支（[`terminal.go:107`](../../internal/shortcuts/terminal.go#L107)）有 `fileExists` 保护，模板分支没有。
2. **SSH 兼容算法硬编码**：[`terminal.go:95-99`](../../internal/shortcuts/terminal.go#L95) 的 `baseArgs`（diffie-hellman-group1-sha1 / ssh-rsa / 3des-cbc 等）写死在 Go 源码里，用户无法关闭。

### 1.3 目标

1. 修掉当前的 `ssh ssh ...` 双 binary bug
2. 把 SSH 命令拼装职责从 `shortcuts/terminal.go` 拆出到独立的 `executor/ssh_command_builder.go`
3. SSH 兼容算法挪到 `config.json` 可配置，默认保持现状兼容
4. 全程不破坏现有 config 模板（首元素仍可为 `"ssh"`，builder 自适应去重）

## 2. 架构 / 模块边界

```
internal/executor/ssh_command_builder.go    (新建)
    BuildSSHCommand(dir *DirShortcut, termType string) ([]string, error)
    // 纯函数：输入快捷 + 终端类型，输出 []string
    // 不调用 exec.Command、不依赖终端实现

internal/shortcuts/terminal.go              (改造)
    OpenRemoteDirShortcut()
        → BuildSSHCommand()              // 拿 []string
        → buildLocalArgsForRemote()      // 套终端壳
        → exec.Command(bin, ...).Start() // 执行

internal/config/config.go                   (新增字段)
    SSHConfig {
        DefaultKeyPath    string
        CompatAlgorithms  SSHCompatAlgorithms { Kex, HostKey, Cipher }
    }
```

**职责切分原则**：
- **SSH builder** 只懂 ssh 命令本身（怎么拼 args、怎么选 binary、怎么管密钥）
- **terminal.go** 只懂"用这个终端程序启动一条命令"（wezterm start / iterm2 osascript / wt new-tab / ...）
- **config.go** 只负责字段定义 + 默认值 + 持久化

两者的连接点是 `[]string` —— 一个干净的纯数据接口。

## 3. BuildSSHCommand 行为契约

### 3.1 函数签名

```go
// BuildSSHCommand 根据 DirShortcut + 终端类型构建 ssh 唤起的完整参数列表。
// 返回的 []string 形如 [ssh, -o, Kex=..., ..., root@host, -t, --, sh, -c, '...']。
// 调用方负责将其传递给终端程序（wezterm / iTerm2 / Windows Terminal ...）。
func BuildSSHCommand(dir *backend.DirShortcut, termType string) ([]string, error)
```

### 3.2 处理流程

1. **查 termDef**
   - `config.Get().Terminal.Types[strings.ToLower(termType)]` 不存在 → 返回 `fmt.Errorf("unsupported terminal type: %s", termType)`
2. **决定 ssh binary**
   - 默认 `"ssh"`
   - 若 `termDef.RemoteBin != ""` → 用 `termDef.RemoteBin`
   - 若 `termDef.RemoteArgs` 非空且 `[0]` 不是 `"ssh"`（例如 `"/custom/ssh"`）→ 用 `[0]` 覆盖 binary
3. **解析 keyPath**
   - 调用 `executor.ResolveKeyPath(dir)`（已有，优先级 LocalKeyPath > KeyPath > `config.ssh.default_key_path` > `~/.ssh/xworkbench_id_ed25519`）
4. **处理模板 `-i` 段**
   - 拿到模板后扫描整个 args 切片；若发现某个连续三元组 `["-i", "{key_path}", ...]`，且 `keyPath == ""` 或 `!fileExists(keyPath)` → 从切片里删除 `"-i"` 及其后跟的 `{key_path}` 元素（无论该 `{key_path}` 占位符在哪个位置，只要跟前一个 `-i` 紧邻就移除）
   - 反之保留三元组并在后续变量替换阶段把 `{key_path}` 替换为 `shellQuote(keyPath)`
5. **拼装 baseArgs（兼容算法）**
   - 读 `config.SSH.CompatAlgorithms.{Kex, HostKey, Cipher}`
   - 任一非空 → 加 `-o`, `"KexAlgorithms=+x,+y"`（同 `-o` 风格，逗号 join）
   - 全空 → 不传 `-o`
6. **处理 RemoteArgs 模板**
   - `RemoteArgs[0] == "ssh"` 且 binary == `"ssh"` → 跳过 `[0]`（去重）
   - `RemoteArgs[0] != "ssh"`（用户自定义 binary 路径）→ 保留 `[0]`，并已在步骤 2 覆盖 binary
7. **变量替换**（顺序无关）
   - `{key_path}` → `shellQuote(keyPath)`（仅当 keyPath 存在且文件存在）
   - `{user}` → `dir.RemoteUser`
   - `{host}` → `dir.RemoteHost`
   - `{user}@{host}` → `dir.RemoteUser + "@" + dir.RemoteHost`（RemoteUser 空时仅 `dir.RemoteHost`）
   - `{shell_cmd}` → `shellQuote(buildShellCmd(dir))`
8. **返回** `[binary, -o, ..., {user}@{host}, -t, --, sh, -c, shell_cmd]`

### 3.3 关键不变量

- 返回的 `[]string` 永远以 ssh binary 起头，**不重复**
- `-i` 永远紧跟一个**存在的**文件路径（或完全不存在）
- 兼容算法可被用户 0 配置关掉（`"compat_algorithms": {}`）
- `shell_cmd` 中的特殊字符（空格、单引号、$）不会破坏外层 ssh 命令

### 3.4 buildShellCmd 规则

```
parts = []
if dir.RemotePath != "": parts += ["cd '" + dir.RemotePath + "'"]
if dir.TerminalCmd != "": parts += [dir.TerminalCmd]
parts += ["exec $SHELL -l"]
shell_cmd = strings.Join(parts, " && ")
```

`shellQuote()`：单引号包裹 + 内单引号转义为 `'\''`（既有实现，保留）。

## 4. config 字段定义

### 4.1 字段

```go
// internal/config/config.go
// （Config.SSH 字段类型由原 SSHKeyConfig 升级为 SSHConfig）

type SSHCompatAlgorithms struct {
    Kex     []string `json:"kex,omitempty"`      // 对应 -o KexAlgorithms=+...
    HostKey []string `json:"host_key,omitempty"` // 对应 -o HostKeyAlgorithms=+...
    Cipher  []string `json:"cipher,omitempty"`   // 对应 -o Ciphers=+...
}

type SSHConfig struct {
    DefaultKeyPath   string              `json:"default_key_path,omitempty"`
    CompatAlgorithms SSHCompatAlgorithms `json:"compat_algorithms"`
}
```

> 字段命名：config 顶层 `default_key_path` 跟现有兼容；`compat_algorithms` 嵌套对象；子字段 `kex`/`host_key`/`cipher` 单数（每项对应 ssh `-o` 一个 flag 名）。`SSHKeyConfig` 旧类型在合并到 `SSHConfig` 后删除，Config struct 中 `SSH` 字段类型直接改为 `SSHConfig`。

### 4.2 默认值（保留现状）

```go
CompatAlgorithms: SSHCompatAlgorithms{
    Kex: []string{
        "+diffie-hellman-group1-sha1",
        "+diffie-hellman-group-exchange-sha1",
    },
    HostKey: []string{"+ssh-rsa", "+ssh-dss"},
    Cipher: []string{
        "+3des-cbc", "+aes128-cbc", "+aes192-cbc", "+aes256-cbc",
    },
},
```

### 4.3 config.template.conf 同步

新增节段：

```jsonc
"ssh": {
    "default_key_path": "~/.ssh/xworkbench_id_ed25519",
    "compat_algorithms": {
        "kex":      ["+diffie-hellman-group1-sha1", "+diffie-hellman-group-exchange-sha1"],
        "host_key": ["+ssh-rsa", "+ssh-dss"],
        "cipher":   ["+3des-cbc", "+aes128-cbc", "+aes192-cbc", "+aes256-cbc"]
    }
}
```

用户写 `"compat_algorithms": {}` 全部禁用（连 `-o` 都不传）。

## 5. terminal.go 改造细节

### 5.1 删除项

- `terminal.go:94-99` 硬编码 `baseArgs` —— 删除，挪到 SSH builder
- `terminal.go:101-116` 模板分支与兜底分支 —— 合并到 `BuildSSHCommand()`
- `terminal.go:131-135` `fileExists` 闭包 —— 移到 `ssh_command_builder.go`（builder 内部用）
- `terminal.go:151-158` `shellQuote` —— 移到 `ssh_command_builder.go`
- `terminal.go:137-149` `buildShellCmd` —— 移到 `ssh_command_builder.go`（与 builder 紧耦合）

### 5.2 保留项

- `IsSupportedTerminal` / `DetectTerminalPath` / `DefaultTerminal`（terminal 工具函数）
- `OpenRemoteDirShortcut` / `OpenRemoteDirShortcutWithKeyAuth`（入口函数）
- `execRemoteTerminal`（终端壳包装 + `exec.Command().Start()`）
- `buildLocalArgsForRemote`（终端类型 → 本地唤起 args 映射）
- `OpenTerminal` / `buildOpenTerminalCmd` / `openTerminalCmd`（本地打开逻辑）
- `OpenRemoteTerminal` / `ParseSSHURL` / `SSHInfo`（URL 风格入口）

### 5.3 简化后 OpenRemoteDirShortcut 主体

```go
func openRemoteDirShortcutImpl(ctx context.Context, dir *DirShortcut, termType, binPath string, ensureKeyAuth bool) error {
    if dir.Type != DirShortcutTypeRemote {
        return fmt.Errorf("not a remote shortcut: type=%s", dir.Type)
    }
    if ensureKeyAuth {
        if _, err := executor.EnsureKeyAuthAvailable(ctx, dir); err != nil {
            logger.Logger.Warnw(...)
        }
    }
    args, err := executor.BuildSSHCommand(dir, termType)
    if err != nil {
        return err
    }
    return execRemoteTerminal(termType, binPath, args)
}
```

## 6. 测试计划

### 6.1 单元测试 `internal/executor/ssh_command_builder_test.go`

| 用例 | 输入 | 期望 |
|---|---|---|
| `TestBuildSSHCommand_StandardWezterm` | dir=完整字段, termType="wezterm", key 文件存在 | 返回 `[ssh, -o, Kex=..., -i, key_path, root@host, -t, --, sh, -c, 'cd /home/workspace && exec $SHELL -l']` |
| `TestBuildSSHCommand_DedupSSH` | RemoteArgs[0]="ssh" | 返回的 []string 不含两个连续的 "ssh" |
| `TestBuildSSHCommand_CustomBin` | RemoteArgs[0]="/custom/ssh" | 返回的 [0] 是 "/custom/ssh"，不再有原生 "ssh" |
| `TestBuildSSHCommand_MissingKeyFile` | LocalKeyPath 指向不存在文件 | 返回不含 `-i` 三元组 |
| `TestBuildSSHCommand_EmptyKeyPath` | LocalKeyPath/KeyPath/default_key_path 全空 + 默认文件不存在 | 返回不含 `-i` 三元组（仅告警） |
| `TestBuildSSHCommand_CompatAlgorithmsEmpty` | config.SSH.CompatAlgorithms 全空 | 返回不含任何 `-o` |
| `TestBuildSSHCommand_CompatAlgorithmsPartial` | config.SSH.CompatAlgorithms.Kex 非空、其余空 | 仅 KexAlgorithms 一项 `-o` |
| `TestBuildSSHCommand_RemotePathWithSpace` | RemotePath="/home/my path" | shell_cmd 内 `'/home/my path'` 单引号正确 |
| `TestBuildSSHCommand_TerminalCmdSet` | TerminalCmd="claude" | shell_cmd 包含 `&& claude && exec $SHELL -l` |
| `TestBuildSSHCommand_UnsupportedTerminal` | termType="unknown" | 返回 error |

### 6.2 回归测试

复用 [`internal/executor/feat-ssh_refactor_validation_test.go`](../../internal/executor/feat-ssh_refactor_validation_test.go) 的：
- `TestResolveKeyPathPriority`（密钥解析路径不变）
- `TestBuildSSHConfigFromDirShortcut`（SSHConfig 字段映射不变）

### 6.3 端到端验证

1. `./scripts/build.sh` 构建
2. 启动 server, 浏览器打开 xworkbench
3. 添加一个远程目录快捷（remote_user/remote_host/remote_path 填真实信息）
4. 点击"打开终端" → 确认 wezterm 弹出 ssh 会话
5. 终端内 `pwd` 应等于 remote_path
6. 切换不同 termType（iterm2 / wt / gnome）确认都正常
7. 在 config.json 设 `"compat_algorithms": {}` → 重启 → 确认 ssh 命令不再带 `-o`

## 7. 改动清单（预计）

| 文件 | 改动 |
|---|---|
| `internal/executor/ssh_command_builder.go` | 新建：`BuildSSHCommand` + `buildShellCmd` + `shellQuote` + `fileExists` 闭包 |
| `internal/executor/ssh_command_builder_test.go` | 新建：10 个表驱动测试 |
| `internal/shortcuts/terminal.go` | 改造：`buildRemoteArgs` 删除，主体简化；删除移走的辅助函数 |
| `internal/shortcuts/terminal_remote_test.go` | 既有测试调整（可能需要 mock config 或指向新函数） |
| `internal/config/config.go` | 新增 `SSHCompatAlgorithms` 类型 + `SSHConfig.CompatAlgorithms` 字段 + 默认值 |
| `config.template.conf` | 新增 `ssh.compat_algorithms` 节段 |

## 8. 非目标

本次重构**不**包含：

- 自动 SSH 算法探测（方案 C，搁置后续）
- `ssh://` URL 风格入口（已有 `OpenRemoteTerminal` / `ParseSSHURL`，本次不动）
- `RemoteBin` 字段新增（builder 借用 `RemoteArgs[0]` 自适应，不引入新 config 字段）
- Windows ConPTY 适配（不影响终端唤起逻辑）
- 前端 UI 改动（`dir_shortcuts` CRUD 表单字段不变）

## 9. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 老 config 缺 `compat_algorithms` 字段 | `DefaultConfig()` + `mergeConfig()` 处理空对象 → 套用默认值 |
| 模板首元素是 "ssh" 的兼容代码逻辑遗漏 | 显式单元测试 `TestBuildSSHCommand_DedupSSH` |
| `shellQuote` 行为变化影响老路径 | shellQuote 实现直接搬运，无逻辑变更 |
| `terminal_remote_test.go` 期望打中已删除函数 | 改造测试改指向 `BuildSSHCommand`，验证最终 args 切片 |