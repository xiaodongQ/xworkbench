// markdown.js — markdown 渲染工具,解决 AI 输出中嵌套 ``` 错乱的问题。
//
// 设计要点:
//   1. 嵌套 fence 预处理:AI 经常在外层 ```python 中又写 ```json,marked 会
//      误把内层当外层闭合。算法:栈式扫描识别"被内嵌污染的外层",长度升级到
//      (同字符最大长度 + 1),让内层 3 反引号打不破外层。
//   2. 升级后引发的"未匹配 close"问题:外层升级后,内层 3 反引号 close 不再
//      匹配外层,会被误当成无效 close。处理:这些无效 close 直接删除(让它们
//      当成段落里的字面字符)。
//   3. 未闭合兜底:残留的开 fence 在末尾补一个等长 close,让 fenced code block
//      能正常结束,而不是延伸到文档末尾。
//   4. 安全渲染:marked 不防 XSS,所有输出经 DOMPurify 处理后才挂到 DOM。
//
// 暴露:
//   window.Markdown = { normalizeFences, render, renderTo }
//
// 依赖(由 index.html 在本文件前加载):
//   window.marked       — marked.umd.min.js
//   window.DOMPurify    — dompurify.min.js
//   window.hljs         — hljs.bundle.min.js(可选,代码块高亮)
(function () {
  'use strict';

  // 开 fence 允许 info string(语言标识)含空格,所以 info 部分用 .*? 非贪婪
  var FENCE_RE = /^(\s{0,3})(`{3,}|~{3,})\s*(.*?)\s*$/;
  var CLOSE_RE = /^(\s{0,3})(`{3,}|~{3,})\s*$/;

  /**
   * normalizeFences 预处理嵌套和未闭合 fence(详见文件头注释)。
   *
   * @param {string} text 原始 markdown
   * @returns {string} 归一化后的 markdown
   */
  function normalizeFences(text) {
    if (!text) return text;
    var lines = text.split('\n');

    // === 第一遍:模拟栈,识别被嵌套污染的开 fence ===
    var stack = [];
    var dirty = new Set();        // 被污染的开 fence 行号
    var allOpens = [];            // 所有开 fence:[{idx, char, len, indent, info}]
    for (var i = 0; i < lines.length; i++) {
      var openM = lines[i].match(FENCE_RE);
      if (!openM) continue;
      var closeM = lines[i].match(CLOSE_RE);
      var indent = openM[1];
      var fence = openM[2];
      var info = openM[3] || '';
      var isClose = !!closeM && !info; // 无语言 → 可能是 close
      if (isClose) {
        // 闭:从栈顶往下找匹配(同字符 + 长度 >= 开 fence)
        for (var k = stack.length - 1; k >= 0; k--) {
          if (stack[k].char === fence[0] && fence.length >= stack[k].len) {
            stack.splice(k, 1);
            break;
          }
        }
      } else {
        // 开:检查栈里同字符未闭 → 内嵌,标记外层 dirty
        for (var j = stack.length - 1; j >= 0; j--) {
          if (stack[j].char === fence[0] && fence.length <= stack[j].len) {
            dirty.add(stack[j].idx);
            break;
          }
        }
        var entry = { idx: i, char: fence[0], len: fence.length, indent: indent, info: info };
        stack.push(entry);
        allOpens.push(entry);
      }
    }

    // 计算每字符最大已用长度
    var maxLenByChar = {};
    for (var m = 0; m < allOpens.length; m++) {
      var c = allOpens[m].char;
      if (!maxLenByChar[c] || allOpens[m].len > maxLenByChar[c]) {
        maxLenByChar[c] = allOpens[m].len;
      }
    }

    // === 第二遍:输出最终文本 ===
    var out = lines.slice();
    var upgradeMap = {}; // 行号 → 升级后长度
    for (var d = 0; d < allOpens.length; d++) {
      if (dirty.has(allOpens[d].idx)) {
        upgradeMap[allOpens[d].idx] = (maxLenByChar[allOpens[d].char] || 3) + 1;
      }
    }

    // 应用升级到开 fence 行(保留原 info string 文本,只把 fence 部分拉长)
    for (var u in upgradeMap) {
      var lineIdx = parseInt(u, 10);
      var m2 = out[lineIdx].match(FENCE_RE);
      if (m2) {
        var newLen = upgradeMap[u];
        out[lineIdx] = m2[1] + m2[2][0].repeat(newLen) + m2[3];
      }
    }

    // 重跑栈(基于升级后长度),决定哪些 close 还有效,哪些该删
    var stack2 = [];
    for (var i2 = 0; i2 < lines.length; i2++) {
      var openM2 = lines[i2].match(FENCE_RE);
      if (!openM2) continue;
      var closeM2 = lines[i2].match(CLOSE_RE);
      var char2 = openM2[2][0];
      var info2 = openM2[3] || '';
      var isClose2 = !!closeM2 && !info2;
      // 用升级后长度
      var len2 = upgradeMap[i2] || openM2[2].length;
      if (!isClose2) {
        stack2.push({ idx: i2, char: char2, len: len2, indent: openM2[1] });
      } else {
        var matched = false;
        for (var k2 = stack2.length - 1; k2 >= 0; k2--) {
          if (stack2[k2].char === char2 && len2 >= stack2[k2].len) {
            stack2.splice(k2, 1);
            matched = true;
            break;
          }
        }
        // 未匹配的 close:因为外层升级导致内层闭不上,删除此行避免被 marked 当段落渲染
        if (!matched) {
          out[i2] = '';
        }
      }
    }

    // 兜底:残留未闭的开 fence → 末尾补等长 close fence
    // 补在文末空行之后,确保 fenced code block 能正常结束
    if (stack2.length > 0) {
      var tail = [];
      for (var t = 0; t < stack2.length; t++) {
        var u2 = stack2[t];
        tail.push(u2.indent + u2.char.repeat(u2.len));
      }
      // 确保前面有空行
      if (out.length && out[out.length - 1].length > 0) {
        out.push('');
      }
      for (var p = 0; p < tail.length; p++) out.push(tail[p]);
    }

    return out.join('\n');
  }

  /**
   * 配置 marked(只调一次)。hljs 可选:存在则启用代码高亮。
   */
  function _configureMarked() {
    if (!window.marked || window.marked.__xw_configured) return;
    var opts = {
      gfm: true,        // GitHub Flavored Markdown(表格、删除线、任务列表)
      breaks: true,     // 单换行转 <br>(AI 输出换行频繁)
    };
    if (window.hljs) {
      opts.highlight = function (code, lang) {
        try {
          if (lang && window.hljs.getLanguage(lang)) {
            return window.hljs.highlight(code, { language: lang }).value;
          }
          return window.hljs.highlightAuto(code).value;
        } catch (_) {
          return code; // 高亮失败也不阻塞渲染
        }
      };
    }
    window.marked.setOptions(opts);
    window.marked.__xw_configured = true;
  }

  /**
   * render 把 markdown 文本渲染成安全 HTML 字符串。
   * @param {string} text
   * @param {object} [opts] 透传给 DOMPurify
   * @returns {string} 安全 HTML
   */
  function render(text, opts) {
    if (text == null) return '';
    _configureMarked();
    var normalized = normalizeFences(text);
    var html = window.marked.parse(normalized);
    if (window.DOMPurify) {
      return window.DOMPurify.sanitize(html, Object.assign({
        ADD_ATTR: ['target', 'rel'],
        FORBID_TAGS: ['style'],
        FORBID_ATTR: ['onerror', 'onload', 'onclick'],
      }, opts || {}));
    }
    // DOMPurify 未加载:返回原 HTML,调用方负责不在不可信上下文使用
    return html;
  }

  /**
   * renderTo 把 markdown 渲染到指定 DOM 元素。
   * @param {Element} el 目标 div/span
   * @param {string} text markdown 源
   * @param {object} [opts] 透传给 render
   */
  function renderTo(el, text, opts) {
    if (!el) return;
    el.innerHTML = render(text, opts);
  }

  window.Markdown = {
    normalizeFences: normalizeFences,
    render: render,
    renderTo: renderTo,
  };
})();
