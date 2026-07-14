# Markdown 渲染模块复用指南

把 xworkbench 的 markdown 渲染方案(含嵌套 fenced code block 归一化 + 安全渲染 + 代码高亮)移植到其他项目。

> **方案 A 适用**:1-2 个内部项目复用,零工具链成本。
> 3+ 个项目或公开发布需求请看文末「扩展方向」(方案 B/C)。

---

## 1. 这个方案解决什么问题

AI 输出 markdown 时常见两类坑:

1. **嵌套 fenced code block 错乱**

   典型场景(外层 python 代码块里又出现了 ```json 块):

   ```
   [外层开始] 三个反引号 + python
   example = """
   [内层开始] 三个反引号 + json   ← marked 会把这行当成外层的 close,后续整段崩
   {"a":1}
   [内层结束] 三个反引号
   """
   [外层结束] 三个反引号
   ```

   (为避免文档自身渲染冲突,这里用缩进代替反引号展示)

2. **流式输出末尾未闭合**

   ```
   三个反引号 + bash
   ls -la          ← 流式截断,没有 close,marked 把整篇当代码块
   ```

`markdown.js` 的 `normalizeFences` 在解析前预处理,栈式扫描识别嵌套,升级外层 fence 长度到 (同字符最大长度 + 1),让内层打不破外层;未闭合的在末尾补 close。

---

## 2. 文件清单

最小可用集 = **4 个 JS + 3 个 LICENSE**,共 ~109KB:

```
目标项目/static/js/
├── vendor/
│   ├── marked.umd.min.js       43 KB   Markdown 解析
│   ├── marked.LICENSE                   BSD-3-Clause
│   ├── dompurify.min.js        29 KB   XSS 过滤
│   ├── dompurify.LICENSE               Apache-2.0 / MPL-2.0
│   ├── hljs.bundle.min.js      68 KB   代码块高亮(12 种语言:bash/python/js/ts/go/json/yaml/sql/css/xml/md/shell)
│   └── highlightjs.LICENSE             BSD-3-Clause
└── markdown.js                  7.5 KB   整合层(归一化 + 安全渲染)
```

**LICENSE 必带**:BSD/MIT/Apache/MPL 协议都要求"分发时保留版权声明"。不带是协议违反,合规风险。

---

## 3. 集成步骤

### 第 1 步:拷文件

从 xworkbench 拷到目标项目:

```bash
SRC=/Users/xd/Documents/workspace/repo/xworkbench/cmd/server/static/js
DEST=/path/to/your-project/static/js   # 改成你的路径

mkdir -p "$DEST/vendor"
cp -v "$SRC/markdown.js" "$DEST/"
cp -v "$SRC/vendor/"*.js "$DEST/vendor/"
cp -v "$SRC/vendor/"*.LICENSE "$DEST/vendor/"
```

> **替代做法**:如果目标项目在 git 里且跟 xworkbench 在同一台机器,可以加为 submodule 或写同步脚本(见 §7)。

### 第 2 步:HTML 引入

**顺序敏感**:marked → dompurify → hljs → markdown.js,后者依赖前者的 window 全局。

```html
<!-- 放在 </body> 之前,业务 JS 之前 -->
<script src="/static/js/vendor/marked.umd.min.js"></script>
<script src="/static/js/vendor/dompurify.min.js"></script>
<script src="/static/js/vendor/hljs.bundle.min.js"></script>
<script src="/static/js/markdown.js"></script>
```

### 第 3 步:加 CSS

拷 `xworkbench/cmd/server/static/css/base.css` 里以 `.md-output` 开头的所有规则到你的样式文件:

```bash
SRC=/Users/xd/Documents/workspace/repo/xworkbench/cmd/server/static/css/base.css
grep -n "^/\* ===== Markdown 渲染" "$SRC"
# 输出行号,比如 1593
# 然后拷 L1593 到 L1690 左右(到 .md-output-raw 结束)
```

或者直接打开 `base.css` 搜 `.md-output`,复制整段。

**CSS 变量依赖**(可自定义):
```css
:root {
  --text-primary: #1e293b;
  --text-secondary: #64748b;
  --border: #e2e8f0;
  --hover-bg: #f1f5f9;
  --primary: #2563eb;
}
```

### 第 4 步:使用 API

```js
// 最简:渲染到 div
const html = Markdown.render(aiText);
document.getElementById('output').innerHTML = html;
```

```js
// 流式打字(每段重渲染,marked 很快)
function streamRender(el, fullText, chunkSize = 10, delayMs = 30) {
  let i = 0;
  const tick = () => {
    i = Math.min(i + chunkSize, fullText.length);
    el.innerHTML = Markdown.render(fullText.slice(0, i));
    if (i < fullText.length) setTimeout(tick, delayMs);
  };
  tick();
}
```

```js
// 只归一化 fence(拿到归一化后的 markdown,自己用其他渲染器)
const normalized = Markdown.normalizeFences(rawText);
```

### 第 5 步:可选,加单测

拷 `xworkbench/cmd/server/static/js/__tests__/markdown-fences.test.js`,跑:

```bash
node /path/to/your-project/static/js/__tests__/markdown-fences.test.js
```

应输出 `=== 10 pass, 0 fail ===`。

---

## 4. API 参考

`window.Markdown` 暴露 3 个方法:

| 方法 | 说明 | 返回 |
|---|---|---|
| `Markdown.render(text, opts?)` | 渲染成安全 HTML 字符串(嵌套 fence 归一化 + marked + DOMPurify) | `string` |
| `Markdown.renderTo(el, text, opts?)` | 直接渲染到 DOM 元素 | `void` |
| `Markdown.normalizeFences(text)` | 只归一化 fence,不渲染 | `string` |

**DOMPurify 配置**(在 `markdown.js` 内部):
- `ADD_ATTR: ['target', 'rel']` — 允许外链属性
- `FORBID_TAGS: ['style']` — 防 CSS 注入
- `FORBID_ATTR: ['onerror', 'onload', 'onclick']` — 防事件处理器

**marked 配置**(在 `markdown.js` 内部):
- `gfm: true` — GFM(表格、删除线、任务列表)
- `breaks: true` — 单换行转 `<br>`(AI 输出换行频繁,默认开启)

需要自定义可在调用前后覆盖:

```js
marked.setOptions({ breaks: false });  // 关掉单换行
const html = Markdown.render(text);    // 生效
```

---

## 5. 嵌套 fence 算法的边界

`normalizeFences` 用栈式扫描,处理了 95% 的 AI 输出场景。但有 1 个已知限制:

**字符串字面量内的 ```**(不是嵌套 fence):
```markdown
```javascript
const x = "```inner```";  ← 这两个 ``` 是字符串内容,不是 fence
```
```

算法无法区分"嵌套 fence"和"字符串里的字面字符",会误判为 fence 嵌套(升级外层)。

**目前解决方案**:无,这是 markdown 本身的语义限制。
**备选**:如果项目里 AI 经常在字符串里写 ```,可以把 `normalizeFences` 第一句 `if (!text) return text;` 后强制升级所有外层 fence 到 4+,但会让所有代码块显示成 ```` ```` ```` 多 1 个反引号(可读性微降)。

---

## 6. 安全要点

1. **必须 DOMPurify**:marked 默认不过滤 `<script>` / `<img onerror>`。`Markdown.render` 内部已调,直接用就行。
2. **不要 `Markdown.render` 后再 `innerHTML =`**:中间不要让 raw HTML 落到不可信 DOM。
3. **vendor JS 必须放在 HTTPS 或 localhost**:浏览器 Clipboard API 需要安全上下文。如果在 HTTP 内网部署,降级到 `textarea + execCommand` 的 fallback 已在 aichat 集成里示范。

---

## 7. 同步更新

xworkbench 升级(markdown.js bug fix / vendor 版本升级)时,把改动同步到所有复用的项目:

### 临时同步(单次手动)

```bash
SRC=/Users/xd/Documents/workspace/repo/xworkbench/cmd/server/static/js
DEST=/path/to/your-project/static/js

cp -v "$SRC/markdown.js" "$DEST/"
cp -v "$SRC/vendor/"*.{js,LICENSE} "$DEST/vendor/"
```

### 写一个 sync 脚本(推荐,放 ~/bin 或项目 scripts/)

```bash
#!/bin/bash
# sync-md-render.sh — 同步 xworkbench markdown 模块到目标项目
set -euo pipefail

SRC="/Users/xd/Documents/workspace/repo/xworkbench/cmd/server/static/js"
DEST="${1:?用法: $0 <目标项目的 static/js 路径>}"

if [[ ! -d "$DEST" ]]; then
  echo "❌ 目标不存在: $DEST" >&2
  exit 1
fi

mkdir -p "$DEST/vendor"
cp -v "$SRC/markdown.js" "$DEST/"
cp -v "$SRC/vendor/"*.js "$SRC/vendor/"*.LICENSE "$DEST/vendor/"
echo "✅ 已同步 $(ls "$DEST/vendor/" | wc -l | tr -d ' ') 个 vendor 文件 + markdown.js"
```

用法:
```bash
chmod +x sync-md-render.sh
./sync-md-render.sh /path/to/project-a/static/js
./sync-md-render.sh /path/to/project-b/static/js
```

### git submodule(适合 3+ 个项目)

把 `markdown.js + vendor/` 抽成独立 git 仓库 `xworkbench/xworkbench-md-render`,在主项目里:

```bash
git submodule add git@github.com:xworkbench/xworkbench-md-render.git \
                    static/js/_md-render
# HTML 引用路径:
# /static/js/_md-render/markdown.js
# /static/js/_md-render/vendor/marked.umd.min.js 等
# 升级:git submodule update --remote
```

---

## 8. 集成示例

### 8.1 React 组件

```jsx
import { useEffect, useRef } from 'react';

function MarkdownView({ text }) {
  const ref = useRef(null);
  useEffect(() => {
    if (ref.current && window.Markdown) {
      ref.current.innerHTML = window.Markdown.render(text);
    }
  }, [text]);
  return <div ref={ref} className="md-output" />;
}
```

### 8.2 Vue 3 组件

```vue
<template>
  <div class="md-output" v-html="rendered"></div>
</template>

<script setup>
import { computed } from 'vue';
const props = defineProps({ text: String });
const rendered = computed(() =>
  window.Markdown ? window.Markdown.render(props.text) : props.text
);
</script>
```

### 8.3 服务端渲染(SSR,Node)

```js
// 浏览器端的 marked/DOMPurify 用法不变,但 hljs 在 Node 里也跑得通
// 拷 markdown.js 后改写:
const FENCE_RE = ...;  // 直接复制 normalizeFences
// 不用 IIFE,改成 module.exports
module.exports = { normalizeFences, render };

// 然后 marked 在 Node 用:
// const { marked } = require('marked');
// marked.parse(text) → html
// 用 isomorphic-dompurify 替代 dompurify(Node 友好)
```

不过 SSR 场景**不建议**用这个方案,标记渲染在客户端更轻量。

### 8.4 aichat 风格(完整 aichat.js 已示范)

事件代理 + data-markdown 存原始内容 + Clipboard API + fallback execCommand。完整代码见 [aichat.js:380-440](../cmd/server/static/js/views/aichat.js)。

---

## 9. 扩展方向(简述)

### 方案 B:npm 包

适合 3+ 项目或公开发布。

```bash
mkdir xworkbench-markdown-render
cd xworkbench-markdown-render
npm init -y
npm i marked dompurify highlight.js --save
# esbuild 打包 src/index.js → dist/markdown.umd.min.js
npm publish
```

其他项目:
```bash
npm i xworkbench-markdown-render
```

### 方案 C:Monorepo / Submodule

适合 3+ 内部项目且不想走 npm。把 `markdown.js + vendor/` 抽成独立 git 仓库,主项目 git submodule 引入。

---

## 10. 故障排查

| 现象 | 原因 | 修法 |
|---|---|---|
| 页面 Console: `Markdown is not defined` | vendor JS 没加载,或顺序错 | 检查 HTML `<script>` 顺序:marked → dompurify → hljs → markdown.js |
| 代码块没高亮 | hljs 没加载 | 检查 `hljs.bundle.min.js` 是否 200,Console 看是否报 `Dynamic require` 错(说明 vendor bundle 打包坏了) |
| 渲染出 `<script>` | DOMPurify 没加载 | 检查 `dompurify.min.js` 200,加载顺序在 marked 之后 |
| 嵌套 ``` 仍然错乱 | 没调用 `Markdown.render`,直接用了 marked | 一定要走 `Markdown.render` 才能触发 normalizeFences |
| 字号过大/过小 | 没拷 `.md-output` CSS | 拷 base.css 里 `.md-output` 整段样式 |

---

## 11. 关联文件

- 源文件: [cmd/server/static/js/markdown.js](../cmd/server/static/js/markdown.js)
- 源样式: [cmd/server/static/css/base.css](../cmd/server/static/css/base.css) (搜 `.md-output`)
- 单测: [cmd/server/static/js/__tests__/markdown-fences.test.js](../cmd/server/static/js/__tests__/markdown-fences.test.js)
- 集成示例 1: [cmd/server/static/js/views/automation.js](../cmd/server/static/js/views/automation.js) (`renderExecOutputAsMarkdown`)
- 集成示例 2: [cmd/server/static/js/views/aichat.js](../cmd/server/static/js/views/aichat.js) (`formatContent` + 复制按钮)
