// markdown.js 的 normalizeFences 单测（Node 跑，不依赖浏览器）
//
// 背景：AI 输出 markdown 时常出现两种不规范场景：
//   1) 外层 ```python 里又写 ```json...```（嵌套 fence）
//      → marked 会把内层当成外层闭合，破坏代码块
//   2) AI 流式截断导致末尾 fence 只开不闭
//      → marked 会把整篇当代码块
//
// normalizeFences 用栈式扫描 + 长度升级 + 兜底补 close 来解决这两个问题。
// 这里覆盖关键场景（嵌套 / 未闭合 / 混合 fence / 边缘 case）。
//
// 用法：node cmd/server/static/js/__tests__/markdown-fences.test.js
'use strict';

const assert = require('assert');

// 直接复制 markdown.js 里的 normalizeFences（保持与生产代码同源，
// 不通过 vm/runInContext 注入整个文件，避免 marked/DOMPurify 依赖）。
// 若生产代码改了，这里必须同步改——这是约定的同步点。
function normalizeFences(text) {
  if (!text) return text;
  var lines = text.split('\n');
  var FENCE_RE = /^(\s{0,3})(`{3,}|~{3,})\s*(.*?)\s*$/;
  var CLOSE_RE = /^(\s{0,3})(`{3,}|~{3,})\s*$/;

  var stack = [];
  var dirty = new Set();
  var allOpens = [];
  for (var i = 0; i < lines.length; i++) {
    var openM = lines[i].match(FENCE_RE);
    if (!openM) continue;
    var closeM = lines[i].match(CLOSE_RE);
    var indent = openM[1];
    var fence = openM[2];
    var info = openM[3] || '';
    var isClose = !!closeM && !info;
    if (isClose) {
      for (var k = stack.length - 1; k >= 0; k--) {
        if (stack[k].char === fence[0] && fence.length >= stack[k].len) {
          stack.splice(k, 1);
          break;
        }
      }
    } else {
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

  var maxLenByChar = {};
  for (var m = 0; m < allOpens.length; m++) {
    var c = allOpens[m].char;
    if (!maxLenByChar[c] || allOpens[m].len > maxLenByChar[c]) {
      maxLenByChar[c] = allOpens[m].len;
    }
  }

  var out = lines.slice();
  var upgradeMap = {};
  for (var d = 0; d < allOpens.length; d++) {
    if (dirty.has(allOpens[d].idx)) {
      upgradeMap[allOpens[d].idx] = (maxLenByChar[allOpens[d].char] || 3) + 1;
    }
  }
  for (var u in upgradeMap) {
    var lineIdx = parseInt(u, 10);
    var m2 = out[lineIdx].match(FENCE_RE);
    if (m2) {
      var newLen = upgradeMap[u];
      out[lineIdx] = m2[1] + m2[2][0].repeat(newLen) + m2[3];
    }
  }

  var stack2 = [];
  for (var i2 = 0; i2 < lines.length; i2++) {
    var openM2 = lines[i2].match(FENCE_RE);
    if (!openM2) continue;
    var closeM2 = lines[i2].match(CLOSE_RE);
    var char2 = openM2[2][0];
    var info2 = openM2[3] || '';
    var isClose2 = !!closeM2 && !info2;
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
      if (!matched) out[i2] = '';
    }
  }

  if (stack2.length > 0) {
    var tail = [];
    for (var t = 0; t < stack2.length; t++) {
      var u2 = stack2[t];
      tail.push(u2.indent + u2.char.repeat(u2.len));
    }
    if (out.length && out[out.length - 1].length > 0) out.push('');
    for (var p = 0; p < tail.length; p++) out.push(tail[p]);
  }
  return out.join('\n');
}

const tests = [
  {
    name: '简单代码块（无嵌套）',
    input: '```python\nprint("hi")\n```',
    expectContains: ['```python', 'print("hi")', '```'],
    expectNotContains: [],
  },
  {
    name: '嵌套 fence（用户核心痛点）',
    input: [
      '```python',
      'example = """',
      '这里有 JSON:',
      '```json',
      '{"a":1}',
      '```',
      '"""',
      '```',
    ].join('\n'),
    expectContains: ['````python'],
    expectNotContains: [],
  },
  {
    name: '未闭合 fence（AI 流式截断）',
    input: '```bash\nls -la',
    expectContains: ['```bash', 'ls -la', '\n```'],
    expectNotContains: [],
  },
  {
    name: '多段独立代码块',
    input: [
      '```js',
      'const a = 1',
      '```',
      '',
      '中间文字',
      '',
      '```py',
      'b = 2',
      '```',
    ].join('\n'),
    expectContains: ['```js', '中间文字', '```py'],
    expectNotContains: [],
  },
  {
    name: '波浪号 fence 与反引号 fence 不互通',
    input: [
      '~~~python',
      'print(1)',
      '~~~',
      '',
      '```js',
      'console.log(2)',
      '```',
    ].join('\n'),
    expectContains: ['~~~python', '```js'],
    expectNotContains: [],
  },
  {
    name: '嵌套 fence 后多段独立代码块',
    input: [
      '```python',
      '# 外层',
      '```json',
      '{"x":1}',
      '```',
      '# 后续内容',
      '```',
      '',
      '下面是独立块:',
      '```js',
      'const y = 2;',
      '```',
    ].join('\n'),
    expectContains: ['````python'],
    expectNotContains: [],
  },
  {
    name: '空输入',
    input: '',
    expectContains: [''],
    expectNotContains: [],
  },
  {
    name: '纯文本无 fence',
    input: 'hello world\n普通段落',
    expectContains: ['hello world', '普通段落'],
    expectNotContains: [],
  },
  {
    // 已知限制：AI 在代码块内的字符串字面量里写 ```（不是嵌套 fence）
    // 会被误识别为 close。这是边缘 case，无法在不主动升级所有代码块
    // 的前提下解决。文档化此行为，等真有用户反馈再决定是否默认升级。
    name: '字符串字面量内的 ``` （已知限制）',
    input: '```javascript\nconst x = "```inner```";\n```',
    expectContains: ['```javascript'],
    expectNotContains: [],
  },
  {
    name: 'info string 含空格的语言标识',
    input: '```js some hint\nconst a = 1;\n```',
    expectContains: ['js some hint', 'const a = 1;'],
    expectNotContains: [],
  },
];

let pass = 0;
let fail = 0;
const failures = [];
for (const tc of tests) {
  const actual = normalizeFences(tc.input);
  let ok = true;
  const reasons = [];
  for (const e of tc.expectContains) {
    if (actual.indexOf(e) === -1) {
      ok = false;
      reasons.push('缺: ' + JSON.stringify(e));
    }
  }
  for (const n of tc.expectNotContains) {
    if (actual.indexOf(n) !== -1) {
      ok = false;
      reasons.push('不该有: ' + JSON.stringify(n));
    }
  }
  if (ok) {
    console.log('✅ ' + tc.name);
    pass++;
  } else {
    console.log('❌ ' + tc.name);
    console.log('   输入:\n' + tc.input.split('\n').map((l) => '     | ' + l).join('\n'));
    console.log('   输出:\n' + actual.split('\n').map((l) => '     | ' + l).join('\n'));
    console.log('   原因: ' + reasons.join('; '));
    failures.push({ name: tc.name, reasons });
    fail++;
  }
}

console.log('\n=== ' + pass + ' pass, ' + fail + ' fail ===');
process.exit(fail > 0 ? 1 : 0);
