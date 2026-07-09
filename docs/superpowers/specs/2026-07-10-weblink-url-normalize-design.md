# WebLink URL 规范化设计

## 1. 背景与目标

### 1.1 当前痛点

用户在前端「快捷链接」弹窗里粘贴 `D:\dfdf\sdfd\xx.html` 这种 Windows 路径时：

- **后端 `handleWebLinkCreate`（main.go:1465）/ `handleWebLinkUpdate`（main.go:1500）** 只校验非空就原样入库
- **前端 `submitLink`（widgets.js:83）** 只 trim 空格
- **DB 里存的就是 `D:\dfdf\sdfd\xx.html`**,与"打开时再转 file://"的逻辑脱节
- 后果：
  - 数据不规整（一个用户存 `D:\a\b`,另一个存 `file:///D:/a/b`）
  - 导出/导入/前端展示不一致
  - 前端 `openLink` 已经能识别本地路径,但只能"打的时候转",存的不是规范 URL

### 1.2 目标

入库前规范化 URL 为标准 `file://` 协议形式,统一数据形态。

支持本地路径形式(用户确认全部覆盖):

| 输入 | 输出 |
|---|---|
| `D:\dfdf\sdfd\xx.html` | `file:///D:/dfdf/sdfd/xx.html` |
| `D:/dfdf/sdfd/xx.html` | `file:///D:/dfdf/sdfd/xx.html` |
| `\\server\share\a.html` | `file:////server/share/a.html` |
| `/home/user/a.html` | `file:///home/user/a.html` |
| `~/a.html` (用户 home 已知) | `file:///home/user/a.html` |
| `file:///D:/a.html` (已规范) | `file:///D:/a.html` (幂等) |
| `https://example.com` | `https://example.com` (原样保留) |
| `http://example.com` | `http://example.com` (原样保留) |
| `ftp://...` | `ftp://...` (原样保留) |

## 2. 设计

### 2.1 抽出 `normalizeLinkURL` 顶层函数

放在 `cmd/server/main.go`,紧邻现有 `pathToFileURL`(main.go:1623)。

```go
// normalizeLinkURL 将用户输入的 URL/路径规整为统一形式:
//   - 前后空白 trim
//   - ~ 展开为 $HOME
//   - Windows 盘符 / UNC / Unix 绝对路径 → file:// URL
//   - 已是 file:// / http(s):// / 其他协议 → 原样保留(幂等)
func normalizeLinkURL(raw string) string {
    s := strings.TrimSpace(raw)
    if s == "" {
        return ""
    }
    // 已是带协议的 URL(http/https/ftp/file 等),原样保留
    if strings.Contains(s, "://") {
        return s
    }
    // ~ 展开(仅首个字符)
    if strings.HasPrefix(s, "~") {
        if usr, err := user.Current(); err == nil {
            s = filepath.Join(usr.HomeDir, strings.TrimPrefix(s, "~"))
        }
    }
    // Windows 盘符 / UNC / Unix 绝对路径 → file://
    if matchWindowsDrive(s) || strings.HasPrefix(s, "\\\\") || strings.HasPrefix(s, "/") {
        return pathToFileURL(s)
    }
    // 其他(相对路径等)原样保留,由调用方决定
    return s
}
```

### 2.2 复用 / 重构点

| 函数 | 改动 |
|---|---|
| `handleWebLinkCreate`(main.go:1465) | 入库前 `req.URL = normalizeLinkURL(req.URL)` |
| `handleWebLinkUpdate`(main.go:1500) | 入库前 `req.URL = normalizeLinkURL(req.URL)` |
| `handleLinkOpen`(main.go:1532) | 替换现有 ~ 展开 + `pathToFileURL` 调用,改为统一调 `normalizeLinkURL` 拿 URL,再走 `open`/`cmd start`/`xdg-open` |

**重点**:`handleLinkOpen` 里现有逻辑保留 `isLocal` 判定是为了决定"打开时走系统命令还是 `window.open`",这个跟"URL 规范化"是两件事。本次改造把"取最终 URL"统一到 `normalizeLinkURL` 即可,`isLocal` 判定保留独立。

### 2.3 不改的部分

- **DB schema**:`web_links` 表 URL 字段保持 `TEXT NOT NULL`,不动
- **WebLinkRepo**(`internal/backend/repo.go`):不动,数据层保持纯 SQL 落库
- **前端 `submitLink`(widgets.js:83)**:不做客户端规范化(后端权威);可保留已有 `trim()`
- **HTML 弹窗 placeholder/index.html:918**:文案已经是"支持 https:// / file:/// / 绝对路径",无需改
- **config_export_import**(`config_export.go`):导入走的也是 POST `/api/web-links`,自动享受规范化

## 3. 数据流

### 3.1 添加链接(POST)

```
用户输入 "D:\dfdf\sdfd\xx.html"
  → POST /api/web-links { name, url }
  → handleWebLinkCreate:
       req.URL = normalizeLinkURL("D:\\dfdf\\sdfd\\xx.html")
                 = "file:///D:/dfdf/sdfd/xx.html"
  → linkDB.Create(URL="file:///D:/dfdf/sdfd/xx.html")
  → 200 { id, name, url: "file:///D:/dfdf/sdfd/xx.html", ... }
```

### 3.2 编辑链接(PUT)

```
用户编辑 url = "D:/dfdf/sdfd/xx.html"
  → PUT /api/web-links/{id} { url }
  → handleWebLinkUpdate:
       req.URL = normalizeLinkURL("D:/dfdf/sdfd/xx.html")
                 = "file:///D:/dfdf/sdfd/xx.html"
  → linkDB.Update(URL=...)
  → 200 { ... url: "file:///D:/dfdf/sdfd/xx.html" }
```

### 3.3 打开链接(POST `/api/links/open`,复用)

```
存的是 "file:///D:/dfdf/sdfd/xx.html"
  → POST /api/links/open { url: "file:///D:/dfdf/sdfd/xx.html" }
  → handleLinkOpen:
       url = normalizeLinkURL("file:///D:/dfdf/sdfd/xx.html")
           = "file:///D:/dfdf/sdfd/xx.html"  (含 :// 原样保留)
  → 系统命令 open / cmd start / xdg-open
```

## 4. 错误处理

| 场景 | 处理 |
|---|---|
| URL 为空 | `handleWebLinkCreate`/`Update` 原有 400 "name and url are required" 保留 |
| `user.Current()` 失败(~ 展开) | 静默 fallback 到原路径,不入 `file://`,由调用方决定(理论上 macOS/Linux 一定能拿到 home) |
| 输入是相对路径如 `foo/bar.html` | 不识别为本地路径,原样保留(DB 里存的就是相对路径) |
| 输入是 `D:foo`(无 `\` 或 `/`) | 不匹配 `matchWindowsDrive`,原样保留 |
| 已是 `file://...` 输入 | `strings.Contains(s, "://")` 命中,原样保留(幂等) |
| 已经是 `https://...` 输入 | 同上,原样保留 |
| URL 极长/超长输入 | 不限制;Go string 无限制,SQLite TEXT 无限制 |

## 5. 测试

### 5.1 单元测试 `cmd/server/weblink_normalize_test.go`

```go
func TestNormalizeLinkURL(t *testing.T) {
    cases := []struct {
        name string
        in   string
        want string
    }{
        // Windows 盘符
        {"win-backslash", `D:\dfdf\sdfd\xx.html`, `file:///D:/dfdf/sdfd/xx.html`},
        {"win-slash", `D:/dfdf/sdfd/xx.html`, `file:///D:/dfdf/sdfd/xx.html`},
        {"win-lowercase-drive", `c:\a\b.html`, `file:///c:/a/b.html`},
        // UNC
        {"unc", `\\server\share\a.html`, `file:////server/share/a.html`},
        // Unix
        {"unix-root", `/home/user/a.html`, `file:///home/user/a.html`},
        // 幂等
        {"file-url-idempotent", `file:///D:/dfdf/sdfd/xx.html`, `file:///D:/dfdf/sdfd/xx.html`},
        {"https-idempotent", `https://example.com`, `https://example.com`},
        {"http-idempotent", `http://example.com`, `http://example.com`},
        {"ftp-idempotent", `ftp://server/file`, `ftp://server/file`},
        // ~ 展开(macOS 上 usr.HomeDir 通常非空,这里只断言 prefix 是 file:///home 或展开后非空)
        {"tilde-home", `~/a.html`, /* 见测试断言 */},
        // 边界
        {"empty", ``, ``},
        {"whitespace", `   `, ``},
        {"trim", `  https://x.com  `, `https://x.com`},
        {"relative-keep", `foo/bar.html`, `foo/bar.html`},
        // drive letter 但无 \ 或 /(matchWindowsDrive 不通过)
        {"drive-no-sep", `D:foo`, `D:foo`},
    }
    // ... 断言
}
```

> `~` 展开断言:`user.Current()` 在 macOS/Linux 上必然非空,用 `os.Setenv("HOME", ...)` 不影响 `user.Current`(mac 上 user.Current 走 directory service)。改用直接断言:`strings.HasPrefix(got, "file:///") && strings.HasSuffix(got, "/a.html")`,或者写 mock。

### 5.2 集成测试 `cmd/server/weblink_url_test.go`

覆盖 `handleWebLinkCreate` 和 `handleWebLinkUpdate` 的 HTTP 入口:

```go
func TestWebLinkCreateNormalizeWinPath(t *testing.T) {
    // 起 httptest server(参考 terminal_api_test.go 模式)
    body := `{"name":"test","url":"D:\\dfdf\\sdfd\\xx.html"}`
    // POST /api/web-links
    // 断言响应 body 里 url == "file:///D:/dfdf/sdfd/xx.html"
    // GET /api/web-links 再次确认 DB 里也是规范 URL
}

func TestWebLinkUpdateNormalizeWinPath(t *testing.T) {
    // 先 POST 一个原始路径链接,再 PUT 更新为 D:\xxx\yyy.html
    // 断言 PUT 后 url 字段被规范化
}

func TestWebLinkCreateIdempotent(t *testing.T) {
    // POST file:///D:/a.html,断言响应 url 一致
}
```

### 5.3 兼容性回归

- `handleLinkOpen` 行为不变(只是内部调用从两段换成一个函数),无需新增测试,但 `go test ./cmd/server/...` 应全绿

## 6. 影响面

| 文件 | 改动行数 |
|---|---|
| `cmd/server/main.go` | +25 (新函数) / -8 (复用) ≈ 净 +17 行 |
| `cmd/server/weblink_normalize_test.go` | 新增,约 80 行 |
| `cmd/server/weblink_url_test.go` | 新增,约 100 行 |
| 其他 | 0 |

**完全不动**:`internal/backend/*`、前端 JS/CSS/HTML、`config.json`。

## 7. 风险与回退

- **风险 1**:`handleLinkOpen` 重构后行为变化 → 已识别 `isLocal` 判定独立保留,且 `go test ./...` 跑全量回归
- **风险 2**:旧 DB 里残留的 `D:\a\b.html` 形态 → 仅在"打开/编辑"时重新规范化;DB 里存的历史数据保持不动,不影响功能(打开逻辑仍然能识别 Windows 路径)
- **回退**:本次改动可一次性 git revert,无 schema 变更,无破坏性升级路径