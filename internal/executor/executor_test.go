package executor

import "testing"

func TestNeedsUserInput(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"hello world", false},
		{"Do you want to continue? [Y/n]", true},
		{"请确认是否继续？[是/否]", true},
		{"Continue? Proceed? Confirm", true},
		{"  是否要删除此文件？", true},
		{"normal output without signals", false},
	}
	for _, c := range cases {
		if got := NeedsUserInput(c.in); got != c.want {
			t.Errorf("NeedsUserInput(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseConfirmRequest(t *testing.T) {
	in := `some output
{"confirm_type": "single_choice", "options": ["yes", "no"]}
trailing`
	got := ParseConfirmRequest(in)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got["confirm_type"] != "single_choice" {
		t.Errorf("confirm_type = %v, want single_choice", got["confirm_type"])
	}
	if ParseConfirmRequest("no json here") != nil {
		t.Error("expected nil for non-JSON")
	}
}
