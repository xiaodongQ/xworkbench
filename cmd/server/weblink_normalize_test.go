package main

import (
	"os"
	"os/user"
	"strings"
	"testing"
)

func TestNormalizeLinkURL_WindowsDrive(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"win-backslash", `D:\dfdf\sdfd\xx.html`, `file:///d:/dfdf/sdfd/xx.html`},
		{"win-slash", `D:/dfdf/sdfd/xx.html`, `file:///d:/dfdf/sdfd/xx.html`},
		{"win-lowercase-drive", `c:\a\b.html`, `file:///c:/a/b.html`},
		{"win-single-letter-dir", `C:\x.html`, `file:///c:/x.html`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := normalizeLinkURL(c.in)
			if got != c.want {
				t.Errorf("normalizeLinkURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeLinkURL_UNC(t *testing.T) {
	got := normalizeLinkURL(`\\server\share\a.html`)
	want := `file:////server/share/a.html`
	if got != want {
		t.Errorf("normalizeLinkURL(UNC) = %q, want %q", got, want)
	}
}

func TestNormalizeLinkURL_UnixAbsolute(t *testing.T) {
	got := normalizeLinkURL(`/home/user/a.html`)
	want := `file:////home/user/a.html`
	if got != want {
		t.Errorf("normalizeLinkURL(unix) = %q, want %q", got, want)
	}
}

func TestNormalizeLinkURL_Idempotent(t *testing.T) {
	cases := []string{
		`file:///D:/dfdf/sdfd/xx.html`,
		`https://example.com`,
		`http://example.com`,
		`ftp://server/file`,
		`file:///home/user/a.html`,
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			got := normalizeLinkURL(in)
			if got != in {
				t.Errorf("normalizeLinkURL(%q) = %q, want idempotent %q", in, got, in)
			}
		})
	}
}

func TestNormalizeLinkURL_Tilde(t *testing.T) {
	// `user.Current()` 在 macOS/Linux 测试环境必然成功。
	// 只断言展开后是 file:/// 开头 + /a.html 后缀,不绑死用户名。
	got := normalizeLinkURL(`~/a.html`)
	if !strings.HasPrefix(got, "file:///") {
		t.Errorf("normalizeLinkURL(~/a.html) = %q, want file:/// prefix", got)
	}
	if !strings.HasSuffix(got, "/a.html") {
		t.Errorf("normalizeLinkURL(~/a.html) = %q, want /a.html suffix", got)
	}
}

func TestNormalizeLinkURL_TrimAndEmpty(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  https://x.com  ", "https://x.com"},
		{"  ", ""},
		{"", ""},
		{"\tD:\\a\\b.html\n", "file:///d:/a/b.html"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := normalizeLinkURL(c.in)
			if got != c.want {
				t.Errorf("normalizeLinkURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeLinkURL_Passthrough(t *testing.T) {
	// 不识别为本地路径的(相对路径、Windows 盘符但无 \ 或 /)原样保留
	cases := []struct {
		in, want string
	}{
		{"foo/bar.html", "foo/bar.html"},
		{"D:foo", "D:foo"}, // matchWindowsDrive 要求第 3 字符是 \ 或 /
		{"just-text", "just-text"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := normalizeLinkURL(c.in)
			if got != c.want {
				t.Errorf("normalizeLinkURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// 确保现有 user.Current / os 包被引用,避免 import 缺失警告
var _ = user.Current
var _ = os.Getenv
