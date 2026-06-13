# AI 评估打分优化 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 AI 任务评估从"凭 stdout 文字猜测"升级为"对照自报清单 + stdout 交叉验证",同时把"解析失败"和"真低分"在 UI 上明确区分。

**Architecture:**
- **方案 1（解析失败标记）**: `parseEval` 不再把解析失败 fallback 成 `Score=0`,而是保留 `Score=-1` 表示"评估员输出无法解析"。前端识别后显示灰色"解析失败"卡片。
- **方案 2（动作清单自报 + 交叉验证）**: 任务执行 prompt 末尾固定追加"动作清单"格式要求(`- 命令: ...` / `- 退出码: ...` / `- 验证: ...`),评估员按"清单 vs stdout 是否真做了"打分。同时代码侧 grep stdout 检查清单声明的命令是否真实出现过,不出现则降分(嘴炮拦截)。

**Tech Stack:** Go 1.22+ / 现有 evaluator 包 / vanilla JS 前端 / SQLite

---

## 关键文件清单

| 角色 | 路径 |
|---|---|
| 评估主逻辑(改) | `internal/evaluator/evaluator.go` |
| 评估单测(改) | `internal/evaluator/evaluator_test.go` |
| Runner 命令构造(改) | `internal/executor/runner/build.go` |
| Runner 单测(改) | `internal/executor/runner/runner_test.go` |
| 手动执行 API(改) | `cmd/server/main.go` |
| 调度器(改) | `internal/scheduler/scheduler.go` |
| 前端弹窗(改) | `cmd/server/static/js/views/automation.js` |
| 前端样式(改) | `cmd/server/static/css/style.css` |

**不动的:** DB schema(`Score REAL` 完全支持 -1)、`Evaluation` 结构体、`BuildCommand` 签名。

---

## Task 1: 方案 1 — parseEval 不再 fallback 到 0(标记解析失败)

**Files:**
- Modify: `internal/evaluator/evaluator.go:95-114`
- Test: `internal/evaluator/evaluator_test.go:5-64`

- [ ] **Step 1: 写失败的测试 — 解析失败时 Score=-1 而不是 0**

在 `evaluator_test.go` 第 5-64 行的 `cases` 数组**追加**两个用例(在第 53 行的 `多行输出` 用例之后):

```go
{
    name:      "完全乱码时 Score=-1 表示解析失败",
    in:        "I don't know how to format this",
    wantScore: -1, // 改成 -1,原 wantScore: 0
    wantCmt:   "I don't know how to format this",
},
{
    name:      "缺评语 + 无分数行时 Score=-1",
    in:        "我做完了",
    wantScore: -1, // 改成 -1,原 wantScore: 0
    wantCmt:   "我做完了", // 原文 fallback
},
```

并把第 39-41 行的 `无评语` 用例(纯分数行,能解析出 score)的 `wantScore: 5` 保持不变(这种不算解析失败,能拿到 5 分)。

- [ ] **Step 2: 跑测试确认失败**

```bash
cd /Users/xd/Documents/workspace/repo/ai-playground/xworkbench
go test ./internal/evaluator/ -run TestParseEval -v
```

预期: 第 1 个和第 2 个新 case FAIL,报错 `score = 0, want -1`。

- [ ] **Step 3: 改 evaluator.go 的 parseEval,Score=-1 不再 fallback 到 0**

把 `evaluator.go:95-114` 的 `parseEval` 函数**完整替换**为:

```go
// parseEval 解析 claude 输出,提取"评分: X"和"评语: ..."。
// 解析失败时 Score 保持 -1,Comments 保留原始 output,方便前端识别"解析失败" vs "真低分"。
func parseEval(output string) *EvalResult {
	res := &EvalResult{Score: -1, Comments: strings.TrimSpace(output)}
	if m := scoreRe.FindStringSubmatch(output); len(m) >= 2 {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 0 && n <= 10 {
			res.Score = n
		}
	}
	if m := cmtRe.FindStringSubmatch(output); len(m) >= 2 {
		res.Comments = strings.TrimSpace(m[1])
	}
	// 解析出评语但 score 失败:把完整 output 附加便于排查
	if res.Score == -1 && res.Comments != strings.TrimSpace(output) {
		res.Comments = res.Comments + "\n[原始输出]\n" + strings.TrimSpace(output)
	}
	// 注意:不再 fallback 到 0,保留 -1 表示"无法解析"
	return res
}
```

关键变化:
- 删掉 `if res.Score == -1 { res.Score = 0 }`(原 110-112 行)
- 解析出 comment 但 score 仍是 -1 时,只在 comments 不是原文时追加原始 output(避免重复)

- [ ] **Step 4: 跑测试确认通过**

```bash
cd /Users/xd/Documents/workspace/repo/ai-playground/xworkbench
go test ./internal/evaluator/ -v
```

预期: 全部 9 个 case 通过(7 旧 + 2 新)。

- [ ] **Step 5: 跑全量测试确认没破坏其他包**

```bash
go test ./...
```

预期: 6 个包全通过。

- [ ] **Step 6: Commit**

```bash
git add internal/evaluator/evaluator.go internal/evaluator/evaluator_test.go
git commit -m "feat(evaluator): 区分解析失败(-1)与真低分,不再 fallback 到 0"
```

---

## Task 2: 方案 2 准备 — 定义 ActionReportSuffix 常量

**Files:**
- Modify: `internal/executor/runner/build.go`(末尾追加)
- Test: `internal/executor/runner/runner_test.go`(追加)

- [ ] **Step 1: 写失败的测试 — BuildCommand 对 claude 类型追加动作清单后缀**

在 `runner_test.go` 末尾追加(最后一个 `}` 之后):

```go
func TestBuildCommandClaudeWithActionReport(t *testing.T) {
	got, err := BuildCommand("claude", "haiku", "", "用 osascript 通知我", WithActionReport())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("cmd too short: %v", got)
	}
	// 最后一个元素应该包含原 prompt + 动作清单后缀
	last := got[len(got)-1]
	if !strings.Contains(last, "用 osascript 通知我") {
		t.Errorf("missing original prompt in: %s", last)
	}
	if !strings.Contains(last, "## 动作清单") {
		t.Errorf("missing action report suffix in: %s", last)
	}
}

func TestBuildCommandShellNoActionReport(t *testing.T) {
	got, err := BuildCommand("shell", "", "", "echo hi", WithActionReport())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// shell 类型不加动作清单
	if strings.Join(got, " ") != "sh -c echo hi" {
		t.Errorf("shell cmd changed: %v", got)
	}
}
```

并在 `runner_test.go` 顶部 `import` 块加 `"strings"`(已存在则跳过)。

- [ ] **Step 2: 跑测试确认失败**

```bash
cd /Users/xd/Documents/workspace/repo/ai-playground/xworkbench
go test ./internal/executor/runner/ -v
```

预期: `TestBuildCommandClaudeWithActionReport` 和 `TestBuildCommandShellNoActionReport` 编译失败(`WithActionReport` 未定义)。

- [ ] **Step 3: 在 build.go 末尾追加 ActionReportSuffix 常量和选项**

把 `build.go` 末尾(第 57 行 `CmdString` 之后)追加:

```go
// ActionReportSuffix 追加到 AI 任务执行 prompt 末尾,要求 AI 自报动作清单,
// 便于后续 evaluator 交叉验证"嘴上说做了 vs 实际执行了"。
// shell 类型不适用。
const ActionReportSuffix = `

## 任务完成后必须输出"动作清单"(便于自动评估)
请严格按以下 Markdown 格式输出,**必须用真实可执行命令,不允许用 \`...\` 占位符**:

## 动作清单
- 命令: <实际执行的命令,完整可复制>
- 退出码: <命令退出码,无命令填 N/A>
- 工具调用: <Bash / Read / Write / Edit / 其他 / 无>
- 验证步骤: <如何确认结果正确,无验证填 N/A>
`

// WithActionReport 返回一个选项,启用动作清单自报后缀(仅对 claude/cbc 生效)。
func WithActionReport() func(*buildOpts) { return func(o *buildOpts) { o.actionReport = true } }

type buildOpts struct {
	actionReport bool
}
```

并改 `BuildCommand` 签名: `func BuildCommand(typ, model, sessionID, prompt string, opts ...func(*buildOpts)) ([]string, error)`。

在 `BuildCommand` 函数体最开头加一行初始化 options:

```go
	o := &buildOpts{}
	for _, opt := range opts {
		opt(o)
	}
```

在 `case "claude":` 和 `case "cbc", "codebuddy":` 分支里,**cmd 拼接 prompt 之前**(原 `cmd = append(cmd, prompt)` 那一行),把 prompt 改写:

```go
		finalPrompt := prompt
		if o.actionReport {
			finalPrompt = prompt + ActionReportSuffix
		}
		cmd = append(cmd, finalPrompt)
```

- [ ] **Step 4: 跑测试确认通过**

```bash
cd /Users/xd/Documents/workspace/repo/ai-playground/xworkbench
go test ./internal/executor/runner/ -v
```

预期: 全部测试通过(老的 4 个 + 新 2 个),且 `TestBuildCommandClaude` 不传 `WithActionReport` 时行为不变(回归测试)。

- [ ] **Step 5: 跑全量测试**

```bash
go test ./...
```

预期: 全通过(其他包调用 `BuildCommand` 没传 options,行为不变)。

- [ ] **Step 6: Commit**

```bash
git add internal/executor/runner/build.go internal/executor/runner/runner_test.go
git commit -m "feat(runner): BuildCommand 支持 WithActionReport 选项,AI 任务追加自报清单"
```

---

## Task 3: 方案 2 — 手动执行 API 和调度器启用动作清单

**Files:**
- Modify: `cmd/server/main.go:316`
- Modify: `internal/scheduler/scheduler.go:122`

- [ ] **Step 1: 改 main.go 的手动执行入口**

找到 `cmd/server/main.go:316` 附近,原代码:

```go
	cmd, err := runner.BuildCommand(req.CommandType, req.Model, "", req.Prompt)
```

改为:

```go
	cmd, err := runner.BuildCommand(req.CommandType, req.Model, "", req.Prompt, runner.WithActionReport())
```

- [ ] **Step 2: 改 scheduler.go 的调度执行入口**

找到 `internal/scheduler/scheduler.go:122` 附近,原代码:

```go
	cmd, err := runner.BuildCommand(t.CommandType, t.Model, "", t.Prompt)
```

改为:

```go
	cmd, err := runner.BuildCommand(t.CommandType, t.Model, "", t.Prompt, runner.WithActionReport())
```

- [ ] **Step 3: 编译确认**

```bash
cd /Users/xd/Documents/workspace/repo/ai-playground/xworkbench
go build -o /tmp/xworkbench-test ./cmd/server
```

预期: 编译成功(两个调用点都已传 `WithActionReport()`)。

- [ ] **Step 4: 跑全量测试**

```bash
go test ./...
```

预期: 全通过。

- [ ] **Step 5: Commit**

```bash
git add cmd/server/main.go internal/scheduler/scheduler.go
git commit -m "feat: 手动执行 + 定时调度启用 AI 任务动作清单自报"
```

---

## Task 4: 方案 2 — evaluator 加 ExtractActionReport 函数(代码侧交叉验证)

**Files:**
- Modify: `internal/evaluator/evaluator.go`(末尾追加)
- Modify: `internal/evaluator/evaluator_test.go`(末尾追加)

- [ ] **Step 1: 写失败的测试 — ExtractActionReport 能从 stdout 提取命令清单**

在 `evaluator_test.go` 末尾追加(最后一个 `}` 之后):

```go
func TestExtractActionReport(t *testing.T) {
	stdout := `我先执行了通知:
命令: osascript -e 'display notification "test" with title "hi"'
退出码: 0

然后我又跑了 pwd:
命令: pwd
退出码: 0
`
	report := ExtractActionReport(stdout)
	if len(report.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %+v", len(report.Commands), report.Commands)
	}
	if report.Commands[0] != `osascript -e 'display notification "test" with title "hi"'` {
		t.Errorf("cmd[0] = %q", report.Commands[0])
	}
	if report.Commands[1] != "pwd" {
		t.Errorf("cmd[1] = %q", report.Commands[1])
	}
	if report.ExitCodes[0] != 0 {
		t.Errorf("exit[0] = %d", report.ExitCodes[0])
	}
}

func TestActionReportVerify(t *testing.T) {
	// stdout 里包含清单声明的命令 → 真做了
	stdout := "osascript -e 'display notification test'"
	report := &ActionReport{
		Commands:  []string{`osascript -e 'display notification test'`},
		ExitCodes: []int{0},
	}
	res := VerifyActionReport(report, stdout)
	if !res.AllExecuted {
		t.Errorf("expected AllExecuted=true, got %+v", res)
	}
	if res.MissingCount != 0 {
		t.Errorf("expected MissingCount=0, got %d", res.MissingCount)
	}
}

func TestActionReportVerifyLie(t *testing.T) {
	// 清单说有命令,但 stdout 里没有 → 嘴炮
	report := &ActionReport{
		Commands:  []string{`osascript -e 'display notification "test"'`},
		ExitCodes: []int{0},
	}
	res := VerifyActionReport(report, "我没做任何事,直接告诉你完成了")
	if res.AllExecuted {
		t.Errorf("expected AllExecuted=false, got %+v", res)
	}
	if res.MissingCount != 1 {
		t.Errorf("expected MissingCount=1, got %d", res.MissingCount)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cd /Users/xd/Documents/workspace/repo/ai-playground/xworkbench
go test ./internal/evaluator/ -run "TestExtractActionReport|TestActionReportVerify" -v
```

预期: 编译失败(`ExtractActionReport` / `ActionReport` / `VerifyActionReport` 未定义)。

- [ ] **Step 3: 在 evaluator.go 末尾追加 ActionReport 解析 + 验证**

在 `evaluator.go` 末尾(第 156 行 `GetByExecution` 之后)追加:

```go
// ActionReport 从 AI 任务输出末尾的"动作清单"段提取的结构化数据。
type ActionReport struct {
	Commands  []string // AI 声明执行的命令
	ExitCodes []int    // 对应退出码(N/A 用 -1)
}

var (
	cmdLineRe  = regexp.MustCompile(`(?m)^-?\s*命令\s*[:：]\s*(.+?)\s*$`)
	exitLineRe = regexp.MustCompile(`(?m)^-?\s*退出码\s*[:：]\s*(\d+|N/A)\s*$`)
)

// ExtractActionReport 从 stdout 提取 AI 自报的动作清单(简单 Markdown 解析)。
// 找不到"## 动作清单"段时返回空报告(NotPresent=true),不报错。
func ExtractActionReport(stdout string) *ActionReport {
	r := &ActionReport{}
	for _, m := range cmdLineRe.FindAllStringSubmatch(stdout, -1) {
		cmd := strings.TrimSpace(m[1])
		// 过滤占位符嘴炮:`...` / `<...>` / `(待填)` / `TODO` / `xxx`
		if isPlaceholder(cmd) {
			continue
		}
		r.Commands = append(r.Commands, cmd)
	}
	for _, m := range exitLineRe.FindAllStringSubmatch(stdout, -1) {
		if m[1] == "N/A" {
			r.ExitCodes = append(r.ExitCodes, -1)
		} else {
			n, _ := strconv.Atoi(m[1])
			r.ExitCodes = append(r.ExitCodes, n)
		}
	}
	return r
}

// ActionVerifyResult 验证结果。
type ActionVerifyResult struct {
	AllExecuted   bool // 清单中所有命令在 stdout 中都出现过
	MissingCount  int  // 缺失(嘴炮)命令数
	MissingCmds   []string
}

// VerifyActionReport 用 stdout 验证清单中的命令是否真实执行过。
// 判定标准:命令字符串在 stdout 中出现过(子串匹配,容忍换行/缩进差异)。
func VerifyActionReport(report *ActionReport, stdout string) *ActionVerifyResult {
	res := &ActionVerifyResult{AllExecuted: true}
	if report == nil || len(report.Commands) == 0 {
		return res
	}
	// 标准化:去多余空白,方便子串匹配
	norm := strings.Join(strings.Fields(stdout), " ")
	for _, cmd := range report.Commands {
		normCmd := strings.Join(strings.Fields(cmd), " ")
		if !strings.Contains(norm, normCmd) {
			res.AllExecuted = false
			res.MissingCount++
			res.MissingCmds = append(res.MissingCmds, cmd)
		}
	}
	return res
}

func isPlaceholder(s string) bool {
	placeholders := []string{"...", "…", "TODO", "xxx", "XXX", "<...>", "(待填)", "(占位)"}
	for _, p := range placeholders {
		if s == p || strings.Contains(s, p) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 跑测试确认通过**

```bash
cd /Users/xd/Documents/workspace/repo/ai-playground/xworkbench
go test ./internal/evaluator/ -v
```

预期: 全部通过(老的 7 个 parseEval + 2 个新解析失败 + 3 个 ActionReport)。

- [ ] **Step 5: Commit**

```bash
git add internal/evaluator/evaluator.go internal/evaluator/evaluator_test.go
git commit -m "feat(evaluator): 加 ExtractActionReport + VerifyActionReport,代码侧拦截嘴炮"
```

---

## Task 5: 方案 2 — 改写 evalPromptTpl 让评估员对照清单 + 交叉验证

**Files:**
- Modify: `internal/evaluator/evaluator.go:20-47` + 改 `Evaluate` 函数

- [ ] **Step 1: 改写 evalPromptTpl,加"动作清单"段和交叉验证要求**

把 `evaluator.go:20-47` 的 `evalPromptTpl` **完整替换**为:

```go
// 评分 prompt:要求 claude 基于"指令 vs AI 自报动作清单 vs 实际 stdout"三方对照打分。
const evalPromptTpl = `你是一个严格的 AI 任务结果评估员。请基于"原始指令"、"AI 自报的动作清单"和"实际 stdout"三方对照,判断任务是否真正完成。

## 任务原始指令
%s

## AI 自报的动作清单(从任务输出末尾提取)
%s

## 任务实际输出(stdout,截断前 3000 字符)
%s

## 任务错误输出(stderr,截断前 500 字符)
%s

## 任务退出码
%d

## 评估要求
1. **交叉验证**:动作清单里声明的每条命令,在 stdout 中是否找到真实执行痕迹(子串匹配)? 找不到 = 嘴炮
2. **占位符检测**:动作清单里含 \`...\` / \`<...>\` / \`TODO\` 等占位符的,视为未真实执行
3. **指令匹配**:动作清单里的动作是否真的回应了原始指令要求
4. 输出严格按以下 2 行格式(便于程序解析):
   评分: <0-10 的整数>
   评语: <一句话评语,50 字以内,优先点出嘴炮/缺验证/指令不匹配>

评分参考:
  9-10 完美完成,清单真实执行,无占位符
  7-8  大体完成,小瑕疵(如未验证副作用)
  5-6  部分完成或有 1-2 处占位符/缺验证
  3-4  明显嘴炮:清单声明但 stdout 无执行证据
  0-2  完全失败,清单为空或全是占位符
`
```

- [ ] **Step 2: 改 Evaluate 函数,提取动作清单并注入 prompt**

把 `evaluator.go:63-91` 的 `Evaluate` 函数**完整替换**为:

```go
// Evaluate 调 claude --print 给 execution 打分。
// 注意:execution 必须有 output;model 留空用默认 claude。
func Evaluate(ctx context.Context, exec *backend.Execution, taskPrompt string, model string) (*EvalResult, error) {
	if exec == nil {
		return nil, fmt.Errorf("execution is nil")
	}
	if model == "" {
		model = "haiku" // 默认用 haiku 快+便宜
	}
	stdout := exec.Output
	stderr := exec.Error
	report := ExtractActionReport(stdout)

	// 把动作清单渲染成可读文本注入 prompt
	reportText := "(AI 未输出动作清单)"
	if len(report.Commands) > 0 {
		var b strings.Builder
		b.WriteString("| # | 命令 | 退出码 |\n|---|------|--------|\n")
		for i, cmd := range report.Commands {
			exit := "N/A"
			if i < len(report.ExitCodes) {
				if report.ExitCodes[i] == -1 {
					exit = "N/A"
				} else {
					exit = strconv.Itoa(report.ExitCodes[i])
				}
			}
			fmt.Fprintf(&b, "| %d | `%s` | %s |\n", i+1, cmd, exit)
		}
		reportText = b.String()
	}

	prompt := strings.TrimSpace(fmt.Sprintf(evalPromptTpl,
		taskPrompt,
		reportText,
		truncate(stdout, 3000),
		truncate(stderr, 500),
		exec.ExitCode,
	))

	cmd, err := runner.BuildCommand("claude", model, "", prompt)
	if err != nil {
		return nil, fmt.Errorf("build cmd: %w", err)
	}
	ctx2, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	res, runErr := executor.Run(ctx2, cmd, nil)
	if runErr != nil && res == nil {
		return nil, fmt.Errorf("run: %w", runErr)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("claude returned exit %d, stderr: %s", res.ExitCode, truncate(res.ErrorOut, 200))
	}
	return parseEval(res.Output), nil
}
```

**注意:** 这里**不传** `WithActionReport()`,因为评估员不应该自报清单(避免无限递归)。

- [ ] **Step 3: 跑全量测试**

```bash
cd /Users/xd/Documents/workspace/repo/ai-playground/xworkbench
go test ./...
```

预期: 全通过(prompt 模板只是字符串,单测只测 parseEval,不影响)。

- [ ] **Step 4: 编译并冒烟**

```bash
go build -o /tmp/xworkbench-test ./cmd/server && echo OK
```

预期: OK。

- [ ] **Step 5: Commit**

```bash
git add internal/evaluator/evaluator.go
git commit -m "feat(evaluator): 评分 prompt 改为'动作清单 + stdout'三方交叉验证"
```

---

## Task 6: 前端 — renderEvalCard 区分"解析失败"vs"真低分"

**Files:**
- Modify: `cmd/server/static/js/views/automation.js:183-193`

- [ ] **Step 1: 改 renderEvalCard,识别 Score=-1 显示灰色"解析失败"卡**

把 `automation.js:183-193` 的 `renderEvalCard` 函数**完整替换**为:

```js
function renderEvalCard(ev) {
  const score = ev.score;
  const isParseFailed = score < 0; // -1 表示评估员输出无法解析
  const color = isParseFailed
    ? 'var(--text-secondary)'
    : score >= 8 ? 'var(--archived)' : score >= 5 ? 'var(--warning)' : 'var(--exception)';
  const scoreDisplay = isParseFailed ? '解析失败' : `${score}/10`;
  const cardStyle = isParseFailed
    ? 'font-size:13px;color:var(--text-secondary);font-style:italic'
    : 'font-size:13px';
  document.getElementById('exec-detail-eval').innerHTML = `
    <div style="${cardStyle}">
      📊 AI 评估: <b style="color:${color};font-size:18px">${scoreDisplay}</b>
      <span style="color:var(--text-secondary);font-size:11px;margin-left:8px">${esc(ev.evaluator_model || '')} · ${esc(new Date(ev.created_at).toLocaleString())}</span>
    </div>
    ${ev.comments ? `<div style="margin-top:6px;color:var(--text-secondary);font-size:12px;white-space:pre-wrap">${esc(ev.comments)}</div>` : ''}
  `;
}
```

关键变化:
- `score < 0` 走"解析失败"分支,显示灰色斜体"解析失败"
- comments 加 `white-space:pre-wrap` 让 `[原始输出]` 换行显示
- 真低分(0-3)继续走红色 exception 颜色

- [ ] **Step 2: 跑全量测试(无 JS 测试,跳过)**

无 JS 自动化测试,改完用浏览器手测。手动跑 server 后开浏览器走一遍:
1. 点一条 execution 的 📊 按钮
2. 故意给 haiku 一个刁钻 prompt(让它输出乱码)
3. 看卡片是否显示"解析失败"灰色

- [ ] **Step 3: Commit**

```bash
git add cmd/server/static/js/views/automation.js
git commit -m "feat(ui): 评估卡区分'解析失败'(灰) vs '真低分'(红)"
```

---

## Task 7: 端到端验证

**Files:** 无(只跑命令)

- [ ] **Step 1: 跑全量测试**

```bash
cd /Users/xd/Documents/workspace/repo/ai-playground/xworkbench
go test ./...
```

预期: 6 包全通过。

- [ ] **Step 2: 编译并启服务**

```bash
go build -o /tmp/xworkbench-test ./cmd/server
DB_PATH=./data/xworkbench.db ADDR=:8081 /tmp/xworkbench-test &
SERVER_PID=$!
sleep 2
echo "server pid=$SERVER_PID"
```

- [ ] **Step 3: 端到端测试 — 跑任务 + 评估**

```bash
# 1. 创建任务: 故意让 AI 嘴炮(用 haiku)
curl -X POST localhost:8081/api/tasks \
  -H "Content-Type: application/json" \
  -d '{"title":"嘴炮测试","description":"用 osascript 弹一个通知"}'

# 假设返回 task_id, 触发执行
TASK_ID=<返回的id>
curl -X POST localhost:8081/api/tasks/$TASK_ID/run \
  -H "Content-Type: application/json" \
  -d '{"command_type":"claude","prompt":"用 osascript 弹一个通知","model":"haiku"}'

# 等几秒后,拿到 execution_id 然后评估
sleep 15
EXECS=$(curl -s localhost:8081/api/executions)
EXEC_ID=$(echo $EXECS | python3 -c "import json,sys; d=json.load(sys.stdin); print([e['id'] for e in d if e.get('task_id')=='$TASK_ID'][0])")
echo "exec id: $EXEC_ID"

# 触发评估(用 sonnet 提高质量,不要用 haiku)
curl -X POST localhost:8081/api/executions/$EXEC_ID/evaluate \
  -H "Content-Type: application/json" \
  -d '{"model":"sonnet"}'

# 等评估完成(最多 3 分钟)
for i in {1..30}; do
  EVALS=$(curl -s localhost:8081/api/executions/$EXEC_ID/evaluations)
  COUNT=$(echo $EVALS | python3 -c "import json,sys; print(len(json.load(sys.stdin)))")
  if [ "$COUNT" -gt 0 ]; then
    echo "评估完成: $EVALS" | python3 -m json.tool
    break
  fi
  sleep 5
done
```

预期: 评估结果 score 应该是低分(嘴炮被识别),comments 包含"占位符"或"清单声明但无执行证据"字样。

- [ ] **Step 4: 端到端测试 — 真执行任务(对照)**

```bash
# 1. 创建任务: 让 AI 真实执行命令
TASK2=$(curl -X POST localhost:8081/api/tasks \
  -H "Content-Type: application/json" \
  -d '{"title":"真执行测试","description":"打印 hello world"}' | python3 -c "import json,sys; print(json.load(sys.stdin)['id'])")

# shell 类型任务,会真跑 echo
curl -X POST localhost:8081/api/tasks/$TASK2/run \
  -H "Content-Type: application/json" \
  -d '{"command_type":"shell","prompt":"echo hello world"}'

sleep 3
EXEC2=$(curl -s localhost:8081/api/executions | python3 -c "import json,sys; d=json.load(sys.stdin); print([e['id'] for e in d if e.get('task_id')=='$TASK2'][0])")

curl -X POST localhost:8081/api/executions/$EXEC2/evaluate \
  -H "Content-Type: application/json" \
  -d '{"model":"sonnet"}'

sleep 30
curl -s localhost:8081/api/executions/$EXEC2/evaluations | python3 -m json.tool
```

预期: shell 类型任务评估 score 应在 8-10(因为是 `echo`,真实执行了)。

- [ ] **Step 5: 清理后台 server**

```bash
kill $SERVER_PID 2>/dev/null
```

- [ ] **Step 6: 决定是否合并**

人工看两个 evaluation 的 score + comments 表现:
- 嘴炮 case: 应该低分(< 4)且评语命中"占位符/无执行证据"
- 真执行 case: 应该高分(8+)

如果都符合预期 → 进入 Task 8 commit。如果不符合 → 回头调整 prompt 模板。

- [ ] **Step 7: Commit(端到端测试日志,可选)**

如果 step 3-4 有 logs 写到文件,可以一并 commit 进 docs 目录,作为行为回归参考。无强求。

---

## Self-Review

### 1. Spec coverage

- ✅ **方案 1**: Task 1 改 `parseEval` 不 fallback + Task 6 前端识别
- ✅ **方案 2 prompt 改造**: Task 2 定义 suffix + Task 3 启用 suffix + Task 5 改 evalPromptTpl
- ✅ **方案 2 交叉验证**: Task 4 ExtractActionReport + VerifyActionReport
- ✅ **端到端验证**: Task 7

### 2. Placeholder scan

- 没有 "TBD" / "fill in details"
- 所有代码片段都完整可复制
- 所有命令带预期输出

### 3. Type consistency

- `EvalResult{Score int, Comments string}` — 全程一致
- `ActionReport{Commands []string, ExitCodes []int}` — 全程一致
- `ActionVerifyResult{AllExecuted bool, MissingCount int, MissingCmds []string}` — 全程一致
- `WithActionReport() func(*buildOpts)` — 签名一致
- `buildOpts{actionReport bool}` — 内部一致
- `Evaluation.Score float64` 接受 -1(DDL 是 REAL,合法)

### 4. 边界覆盖

- ✅ 解析失败 fallback(方案 1)
- ✅ 占位符过滤(isPlaceholder: `...` / `…` / `TODO` / `xxx` 等)
- ✅ shell 类型不加 action report 后缀
- ✅ 评估员不传 WithActionReport(避免自指)
- ✅ 退出码 N/A → -1
- ✅ 动作清单为空时显示"(AI 未输出动作清单)"不报错
- ✅ 旧 4 个 runner_test 回归测试不动
