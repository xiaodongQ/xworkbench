package main

import (
	"strings"
	"testing"
)

// TestExtractResumeSessionID_FromFullArray 测试：claude 2.x 输出是 JSON 数组流，
// 抓 session_id 字段（init 块 + result 块都有），不要抓 uuid（中间事件块的 uuid 不是 session 标识）。
//
// 真实 claude 2.1.168 输出（节选）：先有 system/init 块带 session_id，然后中间各种
// assistant/user/tool 事件带 uuid，最后 result 块带 session_id + uuid。
// 期望：返回 result 块的 session_id（= init 块的 session_id，session 内稳定）。
func TestExtractResumeSessionID_FromFullArray(t *testing.T) {
	output := `[
		{"type":"system","subtype":"init","cwd":"/tmp","session_id":"56340acc-f803-4a42-b6ce-548352ad3760","tools":["Bash"]},
		{"type":"user","message":{"role":"user","content":"hi"},"uuid":"a6eb7a45-dc27-4b3b-8bc3-f49cc82e7663"},
		{"type":"assistant","message":{"id":"msg_01","role":"assistant","content":[{"type":"text","text":"hello"}]},"uuid":"ce193a70-3f79-4313-bc62-b740ec6125f7"},
		{"type":"result","subtype":"success","is_error":false,"duration_ms":1234,"result":"hello world","session_id":"56340acc-f803-4a42-b6ce-548352ad3760","uuid":"ecd8f920-4305-40c2-a002-db9a1ad9163e"}
	]`
	got := extractResumeSessionID(output)
	want := "56340acc-f803-4a42-b6ce-548352ad3760"
	if got != want {
		t.Errorf("extractResumeSessionID = %q, want %q", got, want)
	}
}

// TestExtractResumeSessionID_OnlyResultBlock 测试：init 块被截断（流式输出尾部），
// 只剩 result 块，仍能从 result.session_id 抓到。
func TestExtractResumeSessionID_OnlyResultBlock(t *testing.T) {
	output := `{"type":"result","subtype":"success","result":"hi","session_id":"56340acc-f803-4a42-b6ce-548352ad3760","uuid":"ecd8f920-4305-40c2-a002-db9a1ad9163e"}`
	got := extractResumeSessionID(output)
	want := "56340acc-f803-4a42-b6ce-548352ad3760"
	if got != want {
		t.Errorf("extractResumeSessionID (truncated) = %q, want %q", got, want)
	}
}

// TestExtractResumeSessionID_NoSessionID 测试：输出完全没有 session_id 字段（旧版本 / 错误格式）
// → 返回空串（让 caller 走"占位 group_id = execution.ID"路径）。
func TestExtractResumeSessionID_NoSessionID(t *testing.T) {
	output := `{"type":"result","result":"hi","uuid":"abcd"}`
	got := extractResumeSessionID(output)
	if got != "" {
		t.Errorf("extractResumeSessionID (no session_id) = %q, want empty", got)
	}
}

// TestExtractResumeSessionID_Empty 测试：空输出。
func TestExtractResumeSessionID_Empty(t *testing.T) {
	if got := extractResumeSessionID(""); got != "" {
		t.Errorf("extractResumeSessionID (empty) = %q, want empty", got)
	}
	if got := extractResumeSessionID("not json at all"); got != "" {
		t.Errorf("extractResumeSessionID (garbage) = %q, want empty", got)
	}
}

// TestExtractResumeSessionID_NotConfusedByUUID 测试：回归保护——
// 即使有 uuid 字段（数量还更多），返回值必须取 session_id，不能被 uuid 干扰。
// 这就是历史上 extractResumeUUID 的 bug：抓了第一个 uuid 而不是 session_id。
func TestExtractResumeSessionID_NotConfusedByUUID(t *testing.T) {
	output := `[{"uuid":"WRONG-UUID-1","session_id":"RIGHT-SESSION"},{"uuid":"WRONG-UUID-2","session_id":"RIGHT-SESSION"}]`
	got := extractResumeSessionID(output)
	if got != "RIGHT-SESSION" {
		t.Errorf("extractResumeSessionID = %q, want %q (uuid should NOT be picked)", got, "RIGHT-SESSION")
	}
	if strings.Contains(got, "WRONG-UUID") {
		t.Errorf("extractResumeSessionID picked a uuid instead of session_id: %q", got)
	}
}
