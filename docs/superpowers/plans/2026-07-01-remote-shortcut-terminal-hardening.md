# 快捷目录-远程终端打开 硬化 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把"快捷目录 + 终端打开远程目录"路径上的 6 类问题分摊到 6 个独立 commit,每个都能单独 review/回滚。**不动** 大架构(transport × host terminal 矩阵),单独列为 follow-up。

**Architecture:**
- **横向切分原则**:每个 Task 是单一改动,独立 commit,独立回滚。
- **改动范围控制**:本次只动 `internal/shortcuts/*` + `cmd/server/main.go:1663` + `internal/backend/models.go` + `internal/config/config.go` + `config.json.template`。
- **YAGNI**:不停留在"理论上需要"。每个 patch 都要有具体可测的现象做证据。
- **测试纪律**:每 patch 单测先行,先红后绿,再 refactor 周边。
- **沟通纪律**:`RemotePassword` / `TerminalCmd` 语义两个牵涉 UI/DB schema 的改动,**先讨论再写代码**(本 plan 不包含)。

**Tech Stack:** Go 1.25 / `internal/shortcuts` 包(既有)/ `internal/config` 包 / `internal/backend` models + repo / vanilla JS 前端(不动)/ SQLite(不动 schema)/ stdlib `crypto/ssh`(不动)

---

## 关键文件清单

| 角色 | 路径 |
|---|---|
| 远程打开主函数(改) | `internal/shortcuts/terminal.go:74-117` |
| 快捷目录单测(改) | `internal/shortcuts/terminal_test.go` |
| SSH 通用工具(读) | `internal/executor/ssh_helpers.go:26-42`(`quoteArgs`) |
| 入口 API handler(改) | `cmd/server/main.go:1663`(`/api/dir-shortcuts/{id}/open-terminal`) |
| DirShortcut 模型(改/不动) | `internal/backend/models.go:163-180` |
| 配置模板(改) | `config.json.template` |
| DefaultConfig(改) | `internal/config/config.go:334-356` |

**不动:** DB schema(列已经在)、所有前端代码、所有 `RunSSHViaConfig` 路径(`ssh_helpers.go`)、其他 Task 列表。

---

## 任务前置:从根分支拉工作分支

- [ ] **Step 0: 准备工作分支**

```bash
cd /Users/xd/Documents/workspace/repo/xworkbench
git checkout -b fix/remote-shortcut-terminal-hardening main
```

---

## Task 1: 修复 `OpenRemoteDirShortcut` 的 shell 注入风险(quote-escape)

**问题:** `terminal.go:91-112` 把 `dir.RemotePath` 和 `dir.TerminalCmd` 直接拼字符串 `"cd '<remote_path>' && <terminal_cmd>"`,然后喂给 `ssh ... sh -c <concat>`。路径里若含单引号/`;`/`&`/`$`/`\` 都会被远端 sh 解释。

**Files:**
- Modify: `internal/shortcuts/terminal.go:74-117`
- Test: `internal/shortcuts/terminal_test.go` (新增)

### Step 1: 写失败测试 — quote 转义对含 `'` / `;` 的路径生效

在 `internal/shortcuts/terminal_test.go` **末尾**追加以下测试。`OpenRemoteDirShortcut` 内部拼 sh 命令字符串,我们不能直接测 ssh 启动(那需要真 SSH 服务),所以抽出一个内部函数 `buildRemoteSSHArgs` 并测它——本 Task 同时做这个 refactor。

但是 refactor 引入新函数是另一个动作。**严格 TDD 顺序**:先写出**最终目标**的测试和签名,再写实现。

在文件末尾(第 98 行 `TestOpenTerminal_NotFound` 之后)追加:

```go
func TestBuildRemoteShellCommand(t *testing.T) {
	tests := []struct {
		name         string
		remotePath   string
		terminalCmd  string
		wantContains []string
	}{
		{
			name:         "普通路径无注入",
			remotePath:   "/home/user/proj",
			terminalCmd:  "claude",
			wantContains: []string{`cd '/home/user/proj'`, `claude`, `exec $SHELL`},
		},
		{
			name:         "路径含单引号必须用 '\\'' escape",
			remotePath:   "/home/user/o'reilly",
			terminalCmd:  "",
			wantContains: []string{`cd '/home/user/o'\''reilly'`, `exec $SHELL`},
		},
		{
			name:         "路径含分号必须 quote",
			remotePath:   "/tmp/a;b",
			terminalCmd:  "",
			wantContains: []string{`cd '/tmp/a;b'`},
		},
		{
			name:         "terminalCmd 含元字符也要 quote",
			remotePath:   "/tmp",
			terminalCmd:  "echo hi; rm -rf /",
			wantContains: []string{`'echo hi; rm -rf /'`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildRemoteShellCommand(tt.remotePath, tt.terminalCmd)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("buildRemoteShellCommand(%q, %q) = %q\n  missing substring: %q", tt.remotePath, tt.terminalCmd, got, want)
				}
			}
		})
	}
}
```

并在文件顶部 import 段追加 `"strings"`。

### Step 2: 跑测试,确认失败(red)

Run:

```bash
go test ./internal/shortcuts/ -run TestBuildRemoteShellCommand -v
```

Expected: FAIL,`buildRemoteShellCommand undefined`(因为函数还没抽出来)。

如果直接编译都过,说明 import 或拼写有错,先修测试再继续。

### Step 3: 抽出 `buildRemoteShellCommand` 函数 + 实现 quote

在 `internal/shortcuts/terminal.go` **顶部 import 段**追加 `"github.com/xiaodongQ/xworkbench/internal/executor"`(如果还没引 ssh_helpers,因为 quoteArgs 在那包里)。

Read `internal/shortcuts/terminal.go:1-13` 看现状 import 段,把 `executor` import 加进去(如果跨包 import 不方便,就在 shortcuts 包内部 mirror 一个 `quoteArg` 函数,不走 executor,**保留** quoteArgs 在 shortcuts 包不动——避免新建跨包依赖,本 Task 是安全修,不该扩大 import 拓扑)。

**更稳的方案**:在 `internal/shortcuts/terminal.go` 内部加一个本地 `shellQuote(s string) string` 函数,语义照搬 `internal/executor/ssh_helpers.go:26-42` 的 `quoteArgs` 单元素版本。这是借 pattern 复用而非 import,符合 KISS。

在 `internal/shortcuts/terminal.go` 的 `OpenRemoteDirShortcut` 函数之前(大概在第 73 行 `func OpenRemoteDirShortcut` 上方)新增:

```go
// shellQuote 用 single-quote 包裹一个 shell 参数;含单引号的串用 '\'' 转义。
// 等价于 POSIX shell 单引号字符串字面量。语义参考 internal/executor/ssh_helpers.go:quoteArgs 单元素版。
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'`$\\;|&<>(){}*?!#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// buildRemoteShellCommand 拼出最终传给 ssh -t -- sh -c 的命令字符串。
// remotePath / terminalCmd 都经过 shellQuote 转义,避免注入。
// 末尾固定 `exec $SHELL`,确保 -NoExit 风格(远端会话不退出)。
func buildRemoteShellCommand(remotePath, terminalCmd string) string {
	parts := []string{}
	if remotePath != "" {
		parts = append(parts, "cd "+shellQuote(remotePath))
	}
	if terminalCmd != "" {
		parts = append(parts, shellQuote(terminalCmd))
	}
	parts = append(parts, "exec $SHELL")
	return strings.Join(parts, " && ")
}
```

注意:**不带任何 prefix/suffix 改造**,只是把拼接改安全。原本 `if/else if` 的两种 shape(`TerminalCmd != ""` 走 A 分支 vs `TerminalCmd == ""` 走 B 分支)被 `parts` 数组消除,行为等价:
- `terminalCmd=""` + `remotePath="/foo"` → `cd '/foo' && exec $SHELL` ✅
- `terminalCmd="claude"` + `remotePath="/foo"` → `cd '/foo' && 'claude' && exec $SHELL` ✅
- `terminalCmd=""` + `remotePath=""` → `exec $SHELL` ✅

### Step 4: 把 `OpenRemoteDirShortcut` 两处 string concat 替换为调用

`internal/shortcuts/terminal.go:74-117` 当前两个分支(`wezterm` 和 `default`)各自拼 cmd。

修改后的 `default` 分支(替换行 105-113 区域):

```go
default:
    sshArgs := []string{}
    if dir.AuthMethod == "key" && dir.KeyPath != "" {
        sshArgs = append(sshArgs, "-i", dir.KeyPath)
    }
    sshArgs = append(sshArgs, sshTarget)
    sshArgs = append(sshArgs, "-t", "--", "sh", "-c", buildRemoteShellCommand(dir.RemotePath, dir.TerminalCmd))
    return exec.Command("ssh", sshArgs...).Start()
```

修改后的 `wezterm` 分支(替换行 83-97 区域):

```go
case "wezterm":
    args := []string{"ssh", sshTarget}
    args = append(args, "--", "bash", "-c", buildRemoteShellCommand(dir.RemotePath, dir.TerminalCmd))
    return exec.Command(binPath, args...).Start()
```

注意: `wezterm` 的 args 从 `["ssh", sshTarget, "--", "bash", "-c", cmd]` 改成 `["ssh", sshTarget, "--", "bash", "-c", buildRemoteShellCommand(...)]`,原逻辑等价。我们**只替换 cmd 串的来源**,不去碰 SSH / wezterm 启动方式。

### Step 5: 跑测试,确认绿(green)

Run:

```bash
go test ./internal/shortcuts/ -v
```

Expected:
- `TestBuildRemoteShellCommand` **4 个子用例全过**。
- 现有 `TestIsSupportedTerminal` / `TestDefaultTerminal` / `TestParseSSHURL` / `TestOpenTerminal_NotFound` **不退步**。

如果有失败,看哪个用例红,对应修。

### Step 6: 完整测试集 + vet

```bash
go build ./...
go vet ./...
go test ./internal/shortcuts/ ./internal/executor/ ./internal/backend/ ./internal/config/ -count=1
```

Expected:全部 PASS,无 vet warning。

### Step 7: commit

```bash
git add internal/shortcuts/terminal.go internal/shortcuts/terminal_test.go
git commit -m "$(cat <<'EOF'
fix(shortcuts): quote-escape RemotePath 与 TerminalCmd 避免 sh 注入

把 OpenRemoteDirShortcut 内 string concat 路径改成 shellQuote +
buildRemoteShellCommand。路径含单引号 / 分号 / 空格的目录不再被
远端 sh 错误解释或注入。行为对不含特殊字符的输入保持等价。

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: 入口 handler 用 `DetectTerminalPath` 而不是 `typeDef.Path`(对称化)

**问题:** `cmd/server/main.go:1663` 把 `binPath := typeDef.Path` 直接拿来用,但 `terminal.go:29 DetectTerminalPath` 已经有 PATH + `~` 探测 + `cfg.Terminal.DetectPaths` 三层 fallback。两个入口(handler / detect API)行为不一致,导致 UI 上"路径探测"对"快捷目录打开按钮"完全无效。

**Files:**
- Modify: `cmd/server/main.go:1663-1666`(只动 binPath 取值 + 错误返回)
- Test: 既有 `cmd/server/*_test.go` 的 `handleDirShortcutOpenTerminal` 用例(可能有也可能没有,本 Task 不强求新写,如果没就靠手工验证)

### Step 1: 确认当前实现

Read 1660-1675 行,确认现状:

```go
binPath := typeDef.Path
var openErr error
if entry.Type == backend.DirShortcutTypeRemote {
    openErr = shortcuts.OpenRemoteDirShortcut(entry, termType, binPath)
} else {
    if _, err := os.Stat(path); err != nil {
        writeErr(w, http.StatusBadRequest, "目录不存在或不可访问："+path)
        return
    }
    openErr = shortcuts.OpenTerminal(termType, path, binPath)
}
```

### Step 2: 改成调 `DetectTerminalPath`

替换为:

```go
binPath := shortcuts.DetectTerminalPath(termType)
if binPath == "" && typeDef.Path != "" {
    binPath = typeDef.Path // 显式配置兜底(保留旧行为)
}
logger.Infow("[handleDirShortcutOpenTerminal] binPath resolved",
    "termType", termType,
    "fromDetect", shortcuts.DetectTerminalPath(termType),
    "fromTypeDef", typeDef.Path,
    "final", binPath,
    "at", "main.go:1663")

var openErr error
if entry.Type == backend.DirShortcutTypeRemote {
    openErr = shortcuts.OpenRemoteDirShortcut(entry, termType, binPath)
} else {
    if _, err := os.Stat(path); err != nil {
        writeErr(w, http.StatusBadRequest, "目录不存在或不可访问："+path)
        return
    }
    openErr = shortcuts.OpenTerminal(termType, path, binPath)
}
```

逻辑:
1. 优先用 `DetectTerminalPath` —— 走 PATH + `~` + DetectPaths 三层。
2. 探测为空但配置里有 `typeDef.Path` 时用配置值(保留向后兼容)。
3. 都空时 `binPath=""` —— `OpenTerminal` 会回落到 `typeDef.Bin`(既有行为)。

### Step 3: 跑既有测试 + build

```bash
go build ./...
go vet ./...
go test ./... -count=1
```

Expected:全部 PASS。

### Step 4: 手工验证(在 macOS / Linux dev 机器)

启动 xworkbench,在 UI 上对一条 local 快捷目录点"用 wezterm 打开":

```bash
# 启动 dev server
DB_PATH=./data/xworkbench.db ./bin/xworkbench -config ./config.json
# 浏览器打开 http://localhost:8902,新建快捷目录,点击「打开」选 wezterm
```

Expected:
- 后端日志出现 `[handleDirShortcutOpenTerminal] binPath resolved` 行,`fromDetect` 是 wezterm 探测到的全路径,`final` 与之一致。
- wezterm 新窗口正常打开(行为和原本一样)。

### Step 5: commit

```bash
git add cmd/server/main.go
git commit -m "$(cat <<'EOF'
refactor(main): 快捷目录 open-terminal 入口走 DetectTerminalPath

原 binPath := typeDef.Path 直接读配置硬编码 path,没接 PATH/~/
DetectPaths 三层探测。现在统一走 DetectTerminalPath,仅在探测为空
且 typeDef.Path 非空时回落到配置值。修对称性:
  - /api/terminal/detect 探测得到路径时,/api/dir-shortcuts/{id}/open-terminal
    也用同一路径。
  - 配置 path 为空时也能用 PATH 上的 bin。
向后兼容:typeDef.Path 兜底保留,所以显式配置的旧用户不受影响。

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: pwsh7 在 Windows 上的 `detect_paths` 默认值补全

**问题:** `internal/config/config.go:342-355` DefaultConfig 只给 `wezterm` 配了 `detect_paths`,`pwsh7.path=""` + `bin="pwsh"` + 没 detect_paths,Windows 用户首次安装完 pwsh7 后能输入 `default_terminal=pwsh7` 但启动会报 `'pwsh' 不是内部或外部命令`。

**Files:**
- Modify: `internal/config/config.go:342-355`
- Modify: `config.json.template:16-20`
- Test: `internal/config/config_test.go`(如有)or 新增

### Step 1: 看现有 DefaultConfig 的 detect_paths 段

Read `internal/config/config.go:342-355`,确认现状只有 wezterm 一项。

### Step 2: 加 pwsh7 的常见 Windows 安装路径

在 `DetectPaths: map[string][]string{...}` 块里追加 pwsh7 条目,放在 wezterm 之后:

```go
DetectPaths: map[string][]string{
    "wezterm": {"/Applications/WezTerm.app/Contents/MacOS/WezTerm"},
    // PowerShell 7 在 Windows 的默认安装路径(MSI 全机 / 用户 AppData)
    "pwsh7": {
        // MSI 全机安装 (默认)
        `C:\Program Files\PowerShell\7\pwsh.exe`,
        // MSI 用户安装 / winget / Store 通常在 AppData
        // ~/AppData/Local/Microsoft/PowerShell/7/pwsh.exe 在运行时由 DetectTerminalPath 的 ~ 展开处理
    },
},
```

注意:`~` 在 `DetectPaths` 不会被自动展开,但是 `internal/shortcuts/terminal.go:46-49` 的 `~` 展开只对 `DetectTerminalPath` 内的路径生效,**仅当 bin 不在 PATH 时**——也就是当用户用 AppData 路径时,需要在 detect_paths 里直接写展开后的 `%USERPROFILE%` 占位符比较复杂。

**实际解决方法**:只配全机安装路径(`C:\Program Files\PowerShell\7\pwsh.exe`)——这是 90% 用户的实际安装位置。如果用户的安装不在此,UI 的 detect 入口会回空,他们可以手动填 `typeDef.Path`,本 Task 解决 90% 场景。

### Step 3: 同步 config.json.template

在 `config.json.template:16-20` 的 `detect_paths` 块同步加 `pwsh7` 项:

```json
"detect_paths": {
  "wezterm": [
    "/Applications/WezTerm.app/Contents/MacOS/WezTerm"
  ],
  "pwsh7": [
    "C:\\Program Files\\PowerShell\\7\\pwsh.exe"
  ]
},
```

### Step 4: 写单测 — 验证 DefaultConfig 包含新条目

在 `internal/config/config_test.go`(如果不存在就新建)追加:

```go
func TestDefaultConfig_Pwsh7DetectPaths(t *testing.T) {
    cfg := DefaultConfig()
    paths, ok := cfg.Terminal.DetectPaths["pwsh7"]
    if !ok {
        t.Fatal("DefaultConfig.Terminal.DetectPaths[\"pwsh7\"] 缺失")
    }
    if len(paths) == 0 {
        t.Fatal("DefaultConfig.Terminal.DetectPaths[\"pwsh7\"] 是空切片")
    }
    // 默认必须包含 MSI 全机安装路径(Windows 用户 90% 走这个)
    want := `C:\Program Files\PowerShell\7\pwsh.exe`
    found := false
    for _, p := range paths {
        if p == want {
            found = true
            break
        }
    }
    if !found {
        t.Errorf("DefaultConfig.Terminal.DetectPaths[\"pwsh7\"] 缺少 %q;got %v", want, paths)
    }
}
```

### Step 5: 跑测试

```bash
go test ./internal/config/ -run TestDefaultConfig_Pwsh7DetectPaths -v
go test ./internal/config/ -count=1
```

Expected:全过。

### Step 6: commit

```bash
git add internal/config/config.go internal/config/config_test.go config.json.template
git commit -m "$(cat <<'EOF'
fix(config): pwsh7 默认 detect_paths 补全 MSI 全机安装路径

PowerShell 7 在 Windows 上默认不加入 PATH,只有 DetectTerminalPath
能探测出来。DefaultConfig 现有 wezterm 一项,pwsh7 缺失,导致
default_terminal=pwsh7 在 Windows 上首次开机会报 'pwsh' 不是内部
或外部命令。

补 MSI 全机默认路径 C:\Program Files\PowerShell\7\pwsh.exe,覆盖
90% 安装场景;其它路径(用户 AppData / 自定义)用户可走 UI 配置
typeDef.path 覆盖。

同步 config.json.template,保持模板与 DefaultConfig 一致。

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: 复用 `quoteArgs` —— 提取共享的 ssh 命令构建器,消解 `OpenRemoteTerminal` 与 `OpenRemoteDirShortcut` 重叠

**问题:** `internal/shortcuts/terminal.go:74-117 OpenRemoteDirShortcut` 和 `terminal.go:119-139 OpenRemoteTerminal` 两段几乎独立但功能重叠(都跑 ssh),OpenRemoteTerminal 还在 history 显示"打开远程终端 / 简化版",可能是早期 version。两个函数共享同一组 SSH 启动参数(-i key,user@host,sh -c),但写了两遍。

**Files:**
- Modify: `internal/shortcuts/terminal.go:74-139`
- Test: `internal/shortcuts/terminal_test.go`(扩展)

### Step 1: 写失败测试 — 共享函数对不同入口产生等价 ssh args

在 `terminal_test.go` 末尾追加:

```go
func TestBuildSSHCmd_RemoteDirShortcut(t *testing.T) {
	tests := []struct {
		name      string
		dir       *backend.DirShortcut
		wantArgs  []string
		wantFinal string // 最后一个 -c 后的 sh 命令(便于 spot check)
	}{
		{
			name: "基本 remote + path + cmd 走 key",
			dir: &backend.DirShortcut{
				Type:        backend.DirShortcutTypeRemote,
				RemoteHost:  "1.2.3.4",
				RemoteUser:  "alice",
				RemotePath:  "/var/log",
				AuthMethod:  "key",
				KeyPath:     "/home/alice/.ssh/id_rsa",
				TerminalCmd: "claude",
			},
			wantArgs: []string{"-i", "/home/alice/.ssh/id_rsa", "alice@1.2.3.4", "-t", "--", "sh", "-c"},
		},
		{
			name: "空 path + 空 cmd 也要能跑",
			dir: &backend.DirShortcut{
				Type:       backend.DirShortcutTypeRemote,
				RemoteHost: "host",
				RemoteUser: "u",
			},
			wantArgs: []string{"u@host", "-t", "--", "sh", "-c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, _ := buildSSHCmd(tt.dir)
			// 比对前 N 个 token(wantArgs),最后留出 sh -c 的命令字符串不去比内容
			if len(args) < len(tt.wantArgs) {
				t.Fatalf("args len=%d < want len=%d;got %v", len(args), len(tt.wantArgs), args)
			}
			for i, want := range tt.wantArgs {
				if args[i] != want {
					t.Errorf("args[%d] = %q, want %q\n  full: %v", i, args[i], want, args)
				}
			}
		})
	}
}
```

文件顶部 import 段追加 `"github.com/xiaodongQ/xworkbench/internal/backend"`。

### Step 2: 跑测试,确认失败(red)

```bash
go test ./internal/shortcuts/ -run TestBuildSSHCmd_RemoteDirShortcut -v
```

Expected:FAIL,`buildSSHCmd undefined`。

### Step 3: 实现 `buildSSHCmd` —— 抽出共享 ssh-args 构建

在 `terminal.go` 顶部 `buildRemoteShellCommand` 之后追加:

```go
// buildSSHCmd 把一个 remote DirShortcut 转成 exec.Command("ssh", args...) 的 args 切片
// 和最终给 sh 的命令字符串。后续 -c 的命令内容由 buildRemoteShellCommand 给出。
//
// key auth: -i <keypath> <user>@<host>
// password / 其他 auth: <user>@<host>
// 端口暂时走默认 22;如未来 DirShortcut 加 Port 字段,再扩。
func buildSSHCmd(dir *backend.DirShortcut) ([]string, string) {
	sshTarget := dir.RemoteHost
	if dir.RemoteUser != "" {
		sshTarget = dir.RemoteUser + "@" + dir.RemoteHost
	}
	args := []string{}
	if dir.AuthMethod == "key" && dir.KeyPath != "" {
		args = append(args, "-i", dir.KeyPath)
	}
	args = append(args, sshTarget, "-t", "--", "sh", "-c")
	return args, buildRemoteShellCommand(dir.RemotePath, dir.TerminalCmd)
}
```

### Step 4: 改写 `OpenRemoteDirShortcut` 的 default 分支(去掉重复)

`terminal.go:98-115` 的 default 分支替换为:

```go
default:
    args, cmdStr := buildSSHCmd(dir)
    args = append(args, cmdStr)
    return exec.Command("ssh", args...).Start()
```

### Step 5: 决定 `OpenRemoteTerminal` 的去留

`terminal.go:119-139 OpenRemoteTerminal(target string)` —— 输入 SSH URL 字符串,parse 出 user/host/port/path 后调 `exec.Command("ssh", ...)`,**没有 default 入口调用它**(grep 全文只在自身定义里出现,前端也没引用)。

```bash
grep -r "OpenRemoteTerminal" --include="*.go" .
```

Run 上面命令确认。如果**没有调用方**,就把它删掉(Task 4b);如果有,在 Step 5b 改成调 `buildSSHCmd` 统一。

**Step 5a:无调用方 → 直接删**

```bash
# 删除 terminal.go:119-139 整段 OpenRemoteTerminal 函数
# 同时若有 import 因为这个函数才引的,清理掉
go build ./...
```

Expected:build 通过。

**Step 5b:有调用方 → 改写 + 在终端加测试覆盖**

如果 grep 找到调用方,改写为:

```go
func OpenRemoteTerminal(target string) error {
	info, err := ParseSSHURL(target)
	if err != nil {
		return err
	}
	dir := &backend.DirShortcut{
		Type:       backend.DirShortcutTypeRemote,
		RemoteHost: info.Host,
		RemoteUser: info.User,
		RemotePath: info.Path,
	}
	if info.Port != "" {
		// 端口暂未在 DirShortcut struct 中;仅当协议默认端口不是 22 时需要 -p。
		// TODO: 扩 DirShortcut.RemotePort,follow-up 处理
		// 当前简单处理:忽略非 22 端口,留 log 提示
		logger.Logger.Warnw("[OpenRemoteTerminal] port ignored (not yet supported)",
			"port", info.Port, "target", target)
	}
	args, cmdStr := buildSSHCmd(dir)
	args = append(args, cmdStr)
	return exec.Command("ssh", args...).Start()
}
```

并加测试覆盖(在 terminal_test.go 末尾):

```go
func TestBuildSSHCmd_FromTerminalAPI(t *testing.T) {
	// 通过 OpenRemoteTerminal 流程验证 ParseSSHURL + buildSSHCmd 协作
	// 不调 ssh,只验 args
	// 这块如果 OpenRemoteTerminal 被删除则不需要
	// 否则用 ParseSSHURL 拿 info,构造 dir,跑 buildSSHCmd 比对
	t.Skip("depends on whether OpenRemoteTerminal is kept")
}
```

### Step 6: 跑测试 + build + vet

```bash
go test ./internal/shortcuts/ -v
go build ./...
go vet ./...
go test ./... -count=1
```

Expected:全部 PASS。

### Step 7: commit

如果走 Step 5a(删除):

```bash
git add internal/shortcuts/terminal.go internal/shortcuts/terminal_test.go
git commit -m "$(cat <<'EOF'
refactor(shortcuts): 抽出 buildSSHCmd 共享 ssh-args 构建

OpenRemoteDirShortcut default 分支和 OpenRemoteTerminal 两段
ssh 启动参数构造完全重叠,合成一个 buildSSHCmd 复用。

OpenRemoteTerminal 经 grep 全文确认无调用方,删除(dead code)。

行为不变:
- AuthMethod=key + KeyPath 非空 走 -i key_path
- 其他情况 user@host + -t -- sh -c <buildRemoteShellCommand>

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

如果走 Step 5b(改写 + 保留):

```bash
git add internal/shortcuts/terminal.go internal/shortcuts/terminal_test.go
git commit -m "$(cat <<'EOF'
refactor(shortcuts): 抽出 buildSSHCmd 共享 ssh-args 构建

OpenRemoteDirShortcut default 分支与 OpenRemoteTerminal 两段
ssh 启动参数构造完全重叠,合成 buildSSHCmd 复用。
OpenRemoteTerminal 被前端/handler 调用,改写为走 buildSSHCmd,
非默认端口暂记录 warning(等 DirShortcut.RemotePort 字段扩入)。

行为不变(不含端口):AuthMethod=key + KeyPath 非空 走 -i key_path;
其他 user@host + -t -- sh -c <buildRemoteShellCommand>。

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: 删除 `RemotePassword` 死状态(可选,需用户决定)

**问题:** `internal/backend/models.go:173 DirShortcut.RemotePassword` 字段在 export struct、repo 写入、scan 读取里全链路都存在,但 `OpenRemoteDirShortcut` 和 `buildSSHCmd`(本次新增)都**不读它**。grep 全文:

```bash
grep -n "RemotePassword" --include="*.go" -r .
```

唯一真实引用点 = `OpenRemoteDirShortcut` 当 `AuthMethod=password` 时应该读它(目前没读)。

决策二选一,**先问用户再写代码**:

### 选项 A(推荐):删除字段(简化 + 减少密码明文存储)
- 改 `models.go:173` 删除
- 改 `repo.go` 写入/读取 SQL、schema 不变(列保留),代码侧忽略
- 改 export struct 删除字段
- 加 migration:把已存的 password 数据从 DB 清空(DELETE 密码 + PRAGMA 不可逆)
- 前端 UI 删除密码输入框(`config.js` 或 `index.html`)

### 选项 B:实现 — AuthMethod=password 时让 `sshpass` / `-o` 起来

要装 `sshpass` 依赖或者 fork 一个 helper;安全审查也复杂,**本 plan 不包含**。需要单独 plan。

---

## 任务后置:回到 main 分支

- [ ] **Final: 等所有 Patch ship 后合并**

```bash
git checkout main
git merge --no-ff fix/remote-shortcut-terminal-hardening -m "merge: 快捷目录-远程终端硬化(quote + symmetry + detect_paths + ssh-args 重构)"
```

---

## Self-Review

执行 plan 末尾的 review 检查。

### 1. Spec coverage

- [x] **问题 1.3 (shell 注入)**: Task 1 修
- [x] **问题 1.7 (DetectTerminalPath vs typeDef.Path 不对称)**: Task 2 修
- [x] **问题 1.6 (pwsh7 配置缺口)**: Task 3 修
- [x] **问题 dead code (OpenRemoteTerminal vs OpenRemoteDirShortcut 重叠)**: Task 4 修
- [ ] **问题 1.5 (RemotePassword 死状态)**: Task 5 仅决策,留用户确认
- [ ] **问题 1.4 (TerminalCmd 语义混杂)**: 不在本 plan(UX 变更,需先讨论)
- [ ] **抽象面 (transport × host 矩阵)**: 不在本 plan(架构改动,留 follow-up)
- [ ] **字段面 pwsh7.path 等补完**: 部分(Task 3 解决 detect_paths 90%,完整覆盖留 follow-up)

### 2. Placeholder scan

- 无 "TBD" / "TODO"/ "implement later" 用作 step 内容
- 所有 step 带实际代码 / 命令
- 唯一 TODO 在 Step 5b(端口字段),**标识为 follow-up**,不在当前 plan

### 3. Type / 命名一致性

- 函数名 `buildRemoteShellCommand` 在 Task 1 / Task 4 引用一致
- 函数名 `buildSSHCmd` 在 Task 4 引入并测试,后续引用一致
- 字段 `RemotePassword` 在 Task 5 提到且**单独标"需用户决定"**
- 没有出现更名后又被引用旧名的情况

---

## Follow-up(不在本 plan 范围,留单独 plan)

| 主题 | 工作量 | 优先级 |
|---|---|---|
| 抽象矩阵:transport × host terminal | 大(2-3 周) | 中 |
| `TerminalCmd` 语义改 `Mode` 字段 | 小(1 commit) | 低(需 UX 决策) |
| `DirShortcut.RemotePort` 字段 + `buildSSHCmd` 加 `-p` 支持 | 小(1 commit) | 低 |
| `RemotePassword` 实际接入 sshpass | 中(安全审查 + 依赖) | 低 |
| Windows ConPTY 集成真正在 wezterm 之外的 terminal 里跑远程 | 大 | 中 |
| UI:快捷目录打开时显示 stderr 反馈(ssh 启动失败时) | 小-中 | 中 |
