package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaodongQ/xworkbench/internal/backend"
	"go.uber.org/zap"
)

func TestNormalizeLinkURL_WindowsDrive(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"win-backslash", `D:\dfdf\sdfd\xx.html`, `file:///D:/dfdf/sdfd/xx.html`},
		{"win-slash", `D:/dfdf/sdfd/xx.html`, `file:///D:/dfdf/sdfd/xx.html`},
		{"win-lowercase-drive", `c:\a\b.html`, `file:///c:/a/b.html`},
		{"win-single-letter-dir", `C:\x.html`, `file:///C:/x.html`},
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
	want := `file:///home/user/a.html`
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
		{"\tD:\\a\\b.html\n", "file:///D:/a/b.html"},
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

// TestWebLinkCreateNormalizeWinPath verifies POST /api/web-links normalizes Windows paths
func TestWebLinkCreateNormalizeWinPath(t *testing.T) {
	if logger == nil {
		zl, _ := zap.NewProduction()
		logger = zl.Sugar()
	}
	db, cleanup, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	srv := &APIServer{linkDB: backend.NewWebLinkRepo(db)}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/web-links", srv.handleWebLinkCreate)
	mux.HandleFunc("GET /api/web-links", srv.handleWebLinks)

	body := `{"name":"test","url":"D:\\dfdf\\sdfd\\xx.html"}`
	req := httptest.NewRequest("POST", "/api/web-links", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/web-links: status=%d body=%s", w.Code, w.Body.String())
	}

	var got backend.WebLink
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := "file:///D:/dfdf/sdfd/xx.html"
	if got.URL != want {
		t.Errorf("create: url=%q, want %q", got.URL, want)
	}

	// 再次 GET 确认 DB 里也是规范 URL
	req2 := httptest.NewRequest("GET", "/api/web-links", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET /api/web-links: status=%d", w2.Code)
	}
	var list []backend.WebLink
	if err := json.NewDecoder(w2.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0].URL != want {
		t.Errorf("after GET: list=%+v, want single item url=%q", list, want)
	}

	// 防止 unused import 警告
	var _ sql.DB
}

// TestWebLinkUpdateNormalizeWinPath verifies PUT /api/web-links/{id} normalizes Windows paths
func TestWebLinkUpdateNormalizeWinPath(t *testing.T) {
	if logger == nil {
		zl, _ := zap.NewProduction()
		logger = zl.Sugar()
	}
	db, cleanup, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	srv := &APIServer{linkDB: backend.NewWebLinkRepo(db)}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/web-links", srv.handleWebLinkCreate)
	mux.HandleFunc("PUT /api/web-links/{id}", srv.handleWebLinkUpdate)
	mux.HandleFunc("GET /api/web-links", srv.handleWebLinks)

	// 1. POST a raw Unix path link, expect normalization
	createBody := `{"name":"orig","url":"/tmp/orig.html"}`
	req := httptest.NewRequest("POST", "/api/web-links", bytes.NewReader([]byte(createBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST: status=%d body=%s", w.Code, w.Body.String())
	}
	var created backend.WebLink
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.URL != "file:///tmp/orig.html" {
		t.Fatalf("create did not normalize: got %q", created.URL)
	}

	// 2. PUT a Windows path, expect normalization
	updateBody := `{"name":"updated","url":"D:\\new\\path\\page.html"}`
	req2 := httptest.NewRequest("PUT", "/api/web-links/"+created.ID, bytes.NewReader([]byte(updateBody)))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("PUT: status=%d body=%s", w2.Code, w2.Body.String())
	}
	var updated backend.WebLink
	if err := json.NewDecoder(w2.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated: %v", err)
	}
	want := "file:///D:/new/path/page.html"
	if updated.URL != want {
		t.Errorf("update: url=%q, want %q", updated.URL, want)
	}
}

// TestWebLinkCreateIdempotent verifies already-normalized URLs are not double-converted
func TestWebLinkCreateIdempotent(t *testing.T) {
	if logger == nil {
		zl, _ := zap.NewProduction()
		logger = zl.Sugar()
	}
	db, cleanup, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	defer cleanup()
	srv := &APIServer{linkDB: backend.NewWebLinkRepo(db)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/web-links", srv.handleWebLinkCreate)

	body := `{"name":"idemp","url":"file:///D:/a.html"}`
	req := httptest.NewRequest("POST", "/api/web-links", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got backend.WebLink
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.URL != "file:///D:/a.html" {
		t.Errorf("idempotent: url=%q, want unchanged", got.URL)
	}
}
