package evaluator

import "testing"

func TestParseEval(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantScore int
		wantCmt   string
	}{
		{
			name:      "标准 X/10",
			in:        "评分: 8/10\n评语: 完美完成",
			wantScore: 8,
			wantCmt:   "完美完成",
		},
		{
			name:      "无 /10 后缀",
			in:        "评分: 10\n评语: 全部完成",
			wantScore: 10,
			wantCmt:   "全部完成",
		},
		{
			name:      "中文冒号",
			in:        "评分：7\n评语：还行",
			wantScore: 7,
			wantCmt:   "还行",
		},
		{
			name:      "0 分 + 评语（实际 claude 自相矛盾场景）",
			in:        "评分: 0\n评语: 完美完成",
			wantScore: 0,
			wantCmt:   "完美完成",
		},
		{
			name:      "无评语",
			in:        "评分: 5",
			wantScore: 5,
			wantCmt:   "评分: 5", // 全文 fallback
		},
		{
			name:      "完全乱码",
			in:        "I don't know how to format this",
			wantScore: -1, // 解析失败,不再 fallback 到 0
			wantCmt:   "I don't know how to format this",
		},
		{
			name:      "多行输出（claude 常见）",
			in:        "我先分析一下...\n评分: 9\n评语: 一次性通过",
			wantScore: 9,
			wantCmt:   "一次性通过",
		},
		{
			name:      "缺评语 + 无分数行时 Score=-1",
			in:        "我做完了",
			wantScore: -1,     // 改:旧 fallback 到 0,新行为保留 -1 表示解析失败
			wantCmt:   "我做完了", // 原文 fallback
		},
	}
	for _, c := range cases {
		got := parseEval(c.in)
		if got.Score != c.wantScore {
			t.Errorf("%s: score = %d, want %d", c.name, got.Score, c.wantScore)
		}
		if got.Comments != c.wantCmt {
			t.Errorf("%s: comments = %q, want %q", c.name, got.Comments, c.wantCmt)
		}
	}
}

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

func TestParseJSONExecution(t *testing.T) {
	stdout := `{"type":"result","is_error":false,"num_turns":3,"result":"hello","stop_reason":"end_turn","duration_ms":12000,"permission_denials":["WebFetch"]}`
	meta, ok := ParseJSONExecution(stdout)
	if !ok {
		t.Fatal("expected ok")
	}
	if meta.NumTurns != 3 {
		t.Errorf("NumTurns = %d, want 3", meta.NumTurns)
	}
	if meta.Result != "hello" {
		t.Errorf("Result = %q, want hello", meta.Result)
	}
	if !meta.ToolUseLikely() {
		t.Error("ToolUseLikely should be true (num_turns >= 2)")
	}
}

func TestParseJSONExecutionNotJSON(t *testing.T) {
	_, ok := ParseJSONExecution("plain text output")
	if ok {
		t.Error("expected !ok for plain text")
	}
}

func TestParseJSONExecutionToolUseLikelyFalse(t *testing.T) {
	meta := &ExecutionMeta{NumTurns: 1, PermissionDenials: nil}
	if meta.ToolUseLikely() {
		t.Error("ToolUseLikely should be false for num_turns=1, no denials")
	}
}

func TestParseJSONExecutionNil(t *testing.T) {
	var m *ExecutionMeta
	if m.ToolUseLikely() {
		t.Error("nil receiver should return false")
	}
}
