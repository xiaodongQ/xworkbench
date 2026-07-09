# WebLink URL 规范化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让快捷链接（web_links）在添加/编辑时自动把本地路径（Windows 盘符、UNC、Unix 绝对路径、`~`）规范化为 `file://` 协议 URL,DB 与导出/导入/前端展示形态统一。

**Architecture:** 在 `cmd/server/main.go` 新增顶层函数 `normalizeLinkURL(raw)`,内部按顺序 trim → 已含 `://` 原样保留 → `~` 展开 → 本地路径调既有 `pathToFileURL`。`handleWebLinkCreate` / `handleWebLinkUpdate` 入库前各调一次；`handleLinkOpen` 内部把现有 ~ 展开 + `pathToFileURL` 调用统一替换为 `normalizeLinkURL`。DB schema 与前端不动。

**Tech Stack:** Go 1.25 stdlib (`os/user`、`strings`、`path/filepath`)、`net/http` httptest、SQLite 测试库 `backend.TestDB()`

---

## 文件结构

| 文件 | 职责 |
|---|---|
| `cmd/server/main.go` | 新增 `normalizeLinkURL` 函数；改造 `handleWebLinkCreate`/`handleWebLinkUpdate`/`handleLinkOpen` |
| `cmd/server/weblink_normalize_test.go` | 单元测试 `normalizeLinkURL` 全部 case |
| `cmd/server/weblink_url_test.go` | httptest 集成测试 POST/PUT 入口的规范化行为 |

---

## Task 1: 抽出 normalizeLinkURL 函数 + 单元测试

**Files:**
- Modify: `cmd/server/main.go`(在 `pathToFileURL` 函数定义之前插入新函数)
- Create: `cmd/server/weblink_normalize_test.go`

- [ ] **Step 1: 创建单元测试文件**

新建 `cmd/server/weblink_normalize_test.go`:

```go
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

// 确保现有 user.Current / os 包被引用,避免 import 缺失警告
var _ = user.Current
var _ = os.Getenv
```

- [ ] **Step 2: 运行测试,确认全部失败**

```bash
go test -run TestNormalizeLinkURL ./cmd/server/ -v
```

Expected: FAIL with `undefined: normalizeLinkURL`(所有用例都失败)。

- [ ] **Step 3: 在 main.go 中插入 `normalizeLinkURL` 函数**

打开 `cmd/server/main.go`,定位到 `pathToFileURL` 函数定义(main.go:1623 附近)。**在该函数定义之前**,插入以下代码:

```go
// normalizeLinkURL 将用户输入的 URL/路径规整为统一形式:
//   - 前后空白 trim
//   - 已含 "://" 的 URL(http/https/ftp/file 等)原样保留(幂等)
//   - ~ 展开为用户 home
//   - Windows 盘符 / UNC / Unix 绝对路径 → file:// URL
//   - 其他(相对路径)原样保留
func normalizeLinkURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// 已是带协议的 URL,原样保留(file:// 也走此分支,实现幂等)
	if strings.Contains(s, "://") {
		return s
	}
	// ~ 展开(仅去除首字符 ~,后续保留相对路径语义)
	if strings.HasPrefix(s, "~") {
		if usr, err := user.Current(); err == nil {
			s = filepath.Join(usr.HomeDir, strings.TrimPrefix(s, "~"))
		}
	}
	// Windows 盘符 / UNC / Unix 绝对路径 → file://
	if matchWindowsDrive(s) || strings.HasPrefix(s, "\\\\") || strings.HasPrefix(s, "/") {
		return pathToFileURL(s)
	}
	// 其他(相对路径等)原样保留
	return s
}
```

- [ ] **Step 4: 运行测试,确认全部通过**

```bash
go test -run TestNormalizeLinkURL ./cmd/server/ -v
```

Expected: PASS(全部用例 OK)。

- [ ] **Step 5: 提交**

```bash
git add cmd/server/main.go cmd/server/weblink_normalize_test.go
git commit -m "feat(weblink): 抽出 normalizeLinkURL 函数,支持本地路径→file:// 规范化

单元测试覆盖 Windows 盘符、UNC、Unix 绝对路径、~ 展开、
已规范 URL 幂等、trim、空字符串、相对路径透传等 17 个 case。

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 2: handleWebLinkCreate 接入 + 集成测试

**Files:**
- Modify: `cmd/server/main.go`(在 `handleWebLinkCreate` 函数内,`req.Name/URL` 校验通过后,赋值 `link.URL` 前)
- Modify: `cmd/server/weblink_normalize_test.go`(追加集成测试函数,放 Task 3 之后一并跑也行,但为单测独立,这里就追加)

- [ ] **Step 1: 在 weblink_normalize_test.go 追加集成测试**

在文件末尾追加:

```go
import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	// ... 已有 import

	"github.com/xiaodongQ/xworkbench/internal/backend"
)

// (追加到 import 块后,作为新测试函数)

func TestWebLinkCreateNormalizeWinPath(t *testing.T) {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
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
```

> 注意:`weblink_normalize_test.go` 现有 import 块需要追加 `"database/sql"`、`"net/http"`、`"net/http/httptest"`、`"github.com/xiaodongQ/xworkbench/internal/backend"`。**Task 3 也会加 httptest 的 import**,这里一次性把 import 块扩好。

- [ ] **Step 2: 运行测试,确认集成测试失败**

```bash
go test -run TestWebLinkCreateNormalizeWinPath ./cmd/server/ -v
```

Expected: FAIL with `create: url="D:\\dfdf\\sdfd\\xx.html", want "file:///D:/dfdf/sdfd/xx.html"`。

- [ ] **Step 3: 修改 handleWebLinkCreate**

打开 `cmd/server/main.go`,找到 `handleWebLinkCreate`(行 1465 附近)。把:

```go
link := &backend.WebLink{
    ID:        uuid.New().String(),
    Name:      req.Name,
    URL:       req.URL,
    IconURL:   req.IconURL,
    SortOrder: req.SortOrder,
    CreatedAt: time.Now(),
}
```

替换为:

```go
link := &backend.WebLink{
    ID:        uuid.New().String(),
    Name:      req.Name,
    URL:       normalizeLinkURL(req.URL),
    IconURL:   req.IconURL,
    SortOrder: req.SortOrder,
    CreatedAt: time.Now(),
}
```

- [ ] **Step 4: 运行测试,确认通过**

```bash
go test -run TestWebLinkCreateNormalizeWinPath ./cmd/server/ -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add cmd/server/main.go cmd/server/weblink_normalize_test.go
git commit -m "feat(weblink): handleWebLinkCreate 入库前规范化 URL

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 3: handleWebLinkUpdate 接入 + 集成测试

**Files:**
- Modify: `cmd/server/main.go`(`handleWebLinkUpdate` 内)
- Modify: `cmd/server/weblink_normalize_test.go`(追加 PUT 集成测试)

- [ ] **Step 1: 在 weblink_normalize_test.go 追加 PUT 集成测试**

```go
func TestWebLinkUpdateNormalizeWinPath(t *testing.T) {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	srv := &APIServer{linkDB: backend.NewWebLinkRepo(db)}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/web-links", srv.handleWebLinkCreate)
	mux.HandleFunc("PUT /api/web-links/{id}", srv.handleWebLinkUpdate)
	mux.HandleFunc("GET /api/web-links", srv.handleWebLinks)

	// 1. 先 POST 一个原始 Unix 路径链接
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

	// 2. PUT 一个 Windows 路径,期望被规范化
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

func TestWebLinkCreateIdempotent(t *testing.T) {
	db, _, err := backend.TestDB()
	if err != nil {
		t.Fatalf("TestDB: %v", err)
	}
	srv := &APIServer{linkDB: backend.NewWebLinkRepo(db)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/web-links", srv.handleWebLinkCreate)

	body := `{"name":"idemp","url":"file:///D:/a.html"}`
	req := httptest.NewRequest("POST", "/api/web-links", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var got backend.WebLink
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.URL != "file:///D:/a.html" {
		t.Errorf("idempotent: url=%q, want unchanged", got.URL)
	}
}
```

- [ ] **Step 2: 运行测试,确认 PUT/Idempotent 用例失败**

```bash
go test -run "TestWebLinkUpdateNormalizeWinPath|TestWebLinkCreateIdempotent" ./cmd/server/ -v
```

Expected: FAIL(`TestWebLinkUpdateNormalizeWinPath` 失败因为 PUT 未规范化;`TestWebLinkCreateIdempotent` 应该已经 PASS 因为 Task 2 已加 create 规范化)。

- [ ] **Step 3: 修改 handleWebLinkUpdate**

打开 `cmd/server/main.go`,找到 `handleWebLinkUpdate`(行 1500 附近)。把:

```go
link := &backend.WebLink{ID: id, Name: req.Name, URL: req.URL, IconURL: req.IconURL, SortOrder: req.SortOrder}
```

替换为:

```go
link := &backend.WebLink{ID: id, Name: req.Name, URL: normalizeLinkURL(req.URL), IconURL: req.IconURL, SortOrder: req.SortOrder}
```

- [ ] **Step 4: 运行测试,确认通过**

```bash
go test -run "TestWebLinkUpdateNormalizeWinPath|TestWebLinkCreateIdempotent" ./cmd/server/ -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add cmd/server/main.go cmd/server/weblink_normalize_test.go
git commit -m "feat(weblink): handleWebLinkUpdate 入库前规范化 URL

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 4: handleLinkOpen 重构复用 normalizeLinkURL

**Files:**
- Modify: `cmd/server/main.go`(`handleLinkOpen` 函数体内)

- [ ] **Step 1: 替换 handleLinkOpen 中的本地路径处理逻辑**

打开 `cmd/server/main.go`,找到 `handleLinkOpen`(行 1532 附近)。把现有 ~ 展开 + isFileURL/isLocalPath + pathToFileURL 块:

```go
url := req.URL
isLocal := false

// 展开 ~ 为用户 home 目录
if strings.HasPrefix(url, "~") {
    if usr, err2 := user.Current(); err2 == nil {
        url = filepath.Join(usr.HomeDir, url[1:])
        isLocal = true
    }
} else if isFileURL(url) || isLocalPath(url) {
    isLocal = true
}

// 将本地路径转换为 file:// URL
if isLocal && !isFileURL(url) {
    url = pathToFileURL(url)
}
```

替换为:

```go
url := normalizeLinkURL(req.URL)
// isLocal 判断用于决定走系统命令(open/cmd start/xdg-open)还是 window.open
// normalizeLinkURL 已统一产出 file:// URL;file:/// 即视为本地
isLocal := strings.HasPrefix(url, "file://")
```

> 注意:本 task 不删除 `isFileURL`、`isLocalPath`、`pathToFileURL` 等辅助函数,`normalizeLinkURL` 内部还在用 `matchWindowsDrive`、`pathToFileURL`。仅 `isFileURL`/`isLocalPath` 如果在 main.go 别处不再使用,可以保留(可能在外部代码引用,但实际只在 `handleLinkOpen` 用过,删除是可选的清理)。

- [ ] **Step 2: 全量回归测试,确认未破坏既有行为**

```bash
go test ./cmd/server/ -v -count=1
```

Expected: 全绿(若有 link 打开相关测试,确认未破坏)。

- [ ] **Step 3: 编译并 vet 检查**

```bash
go build -o /tmp/xworkbench-test ./cmd/server && go vet ./cmd/server/...
```

Expected: 编译成功 + vet 无警告。

- [ ] **Step 4: 提交**

```bash
git add cmd/server/main.go
git commit -m "refactor(weblink): handleLinkOpen 复用 normalizeLinkURL 统一规范化逻辑

删除原 inline 的 ~ 展开 + pathToFileURL 调用,改为调用
normalizeLinkURL 一处搞定,isLocal 判定保留(用于决定走
系统命令还是 window.open)。

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 5: 全量验证 + 文档同步

**Files:**
- Modify: 无(纯验证)

- [ ] **Step 1: 全量单元 + 集成测试**

```bash
go test ./... -count=1
```

Expected: PASS(整个项目测试全绿,耗时 ~5-10s)。

- [ ] **Step 2: 跑 e2e 脚本(可选,本机未跑则跳过)**

```bash
./scripts/build.sh
```

Expected: 编译成功。

- [ ] **Step 3: 更新 CLAUDE.md(可选,只在有"快捷链接"专门章节时才需要)**

本项目 CLAUDE.md 没有"快捷链接"独立章节,无需更新。

- [ ] **Step 4: 最终提交(若 step 1/2 有未提交改动)**

```bash
git status
```

如果没有未提交改动,跳过。否则:

```bash
git add -A && git commit -m "chore: post-implementation cleanup"
```

---

## Self-Review Checklist

- [x] Spec coverage:
  - [x] §1.2 输入/输出映射表 → Task 1 (TestNormalizeLinkURL_* 子测试)
  - [x] §2.1 normalizeLinkURL 函数 → Task 1 step 3
  - [x] §2.2 handleWebLinkCreate 接入 → Task 2
  - [x] §2.2 handleWebLinkUpdate 接入 → Task 3
  - [x] §2.2 handleLinkOpen 复用 → Task 4
  - [x] §5.1 单元测试 → Task 1
  - [x] §5.2 集成测试 → Task 2 + Task 3
  - [x] §5.3 handleLinkOpen 兼容性 → Task 4 (go test 全量回归)

- [x] Placeholder scan: 无 TBD/TODO/"see above"
- [x] Type consistency: `normalizeLinkURL(raw string) string` 全 plan 一致;`backend.WebLink` 全 plan 一致;`backend.NewWebLinkRepo(db)` 全 plan 一致