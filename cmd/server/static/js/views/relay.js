// ===== 代理页面 =====

function toggleApiDoc(id) {
  const el = document.getElementById(id);
  if (!el) return;
  const isHidden = el.classList.contains('hidden');
  if (isHidden) {
    el.classList.remove('hidden');
  } else {
    el.classList.add('hidden');
  }
}

async function runExec() {
  const cmd = document.getElementById('exec-cmd').value.trim();
  if (!cmd) { alert('命令不能为空'); return; }
  const cwd = document.getElementById('exec-cwd').value.trim();
  const timeout = parseInt(document.getElementById('exec-timeout').value) || 30000;

  const resultEl = document.getElementById('exec-result');
  resultEl.style.display = 'block';
  resultEl.textContent = '执行中...';

  try {
    const resp = await fetchJSON('/api/exec', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({command: cmd, cwd, timeout_ms: timeout}),
    });
    let text = `Exit Code: ${resp.exit_code}\nDuration: ${resp.duration_ms}ms\n`;
    if (resp.output) text += `\n=== stdout ===\n${resp.output}`;
    if (resp.error_out) text += `\n=== stderr ===\n${resp.error_out}`;
    if (resp.error) text += `\n=== error ===\n${resp.error}`;
    resultEl.textContent = text;
  } catch (e) {
    resultEl.textContent = '请求失败: ' + e.message;
  }
}

async function runProxy() {
  const url = document.getElementById('proxy-url').value.trim();
  if (!url) { alert('URL 不能为空'); return; }
  const method = document.getElementById('proxy-method').value;
  const timeout = parseInt(document.getElementById('proxy-timeout').value) || 30000;

  const resultEl = document.getElementById('proxy-result');
  resultEl.style.display = 'block';
  resultEl.textContent = '转发中...';

  try {
    const resp = await fetchJSON('/api/relay/proxy', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({method, url, timeout_ms: timeout}),
    });
    let text = `Status: ${resp.status_code}\n`;
    if (resp.headers) {
      text += '\n=== headers ===\n' + Object.entries(resp.headers).map(([k,v]) => `${k}: ${v}`).join('\n');
    }
    if (resp.body) {
      const body = typeof resp.body === 'string' ? resp.body : JSON.stringify(resp.body, null, 2);
      text += '\n=== body ===\n' + body;
    }
    if (resp.error) text += '\n=== error ===\n' + resp.error;
    resultEl.textContent = text;
  } catch (e) {
    resultEl.textContent = '请求失败: ' + e.message;
  }
}

async function loadRelayStats() {
  await Promise.all([
    loadSourceStats('exec'),
    loadSourceStats('proxy'),
  ]);
}

async function loadSourceStats(source) {
  try {
    const stats = await fetchJSON('/api/relay/stats?source=' + source);
    document.getElementById(source + '-total').textContent = stats.total_count || 0;
    document.getElementById(source + '-success').textContent = stats.success_count || 0;
    document.getElementById(source + '-failed').textContent = stats.failed_count || 0;

    const hist = stats.date_histogram || {};
    const days = Object.keys(hist).sort();
    const el = document.getElementById(source + '-histogram');
    if (el) {
      if (days.length > 0) {
        el.textContent = '近 ' + days.length + ' 天: ' + days.map(d => d + ': ' + hist[d]).join(', ');
      } else {
        el.textContent = '暂无数据';
      }
    }
  } catch (e) {
    console.error('loadSourceStats(' + source + ') error', e);
  }
}

// ===== Linux 调用脚本生成 =====
// showRelayScriptModal 打开 modal,展示一段可直接拷贝到 Linux 机器运行的 shell 脚本。
// 脚本内置 exec_cmd / proxy_http 两个函数封装,用户改 3 处(API_HOST/API_KEY/参数)即可用。
// 脚本内容是常量字符串(不依赖任何运行时变量),避免脚本里出现实际 token 等敏感信息。
function showRelayScriptModal() {
  const script = `#!/usr/bin/env bash
# xworkbench Relay 调用脚本
# 用途:在 Linux 机器上调用本服务代理(执行命令 / HTTP 转发)
# 改 3 处即可:API_HOST / API_KEY / 调用参数(command / url)

set -euo pipefail

API_HOST="\${API_HOST:-http://localhost:8902}"   # ← 改成你的 xworkbench 地址
API_KEY="\${API_KEY:-xworkbench}"                # ← 改成实际 API key(默认 "xworkbench")

# === 方式 1:执行 shell 命令(POST /api/exec) ===
# body 形如: '{"command":"ls -la /tmp","cwd":"/","timeout_ms":30000}'
exec_cmd() {
  local body="\${1:?usage: exec_cmd '<json_body>'}"
  curl -fsS -X POST "$API_HOST/api/exec" \\
    -H "Content-Type: application/json" \\
    -H "Authorization: Bearer $API_KEY" \\
    -d "$body"
}

# === 方式 2:HTTP 代理转发(POST /api/relay/proxy) ===
# body 形如: '{"method":"GET","url":"https://api.example.com/data","timeout_ms":10000}'
proxy_http() {
  local body="\${1:?usage: proxy_http '<json_body>'}"
  curl -fsS -X POST "$API_HOST/api/relay/proxy" \\
    -H "Content-Type: application/json" \\
    -H "Authorization: Bearer $API_KEY" \\
    -d "$body"
}

# === 示例(取消注释运行) ===
# exec_cmd '{"command":"ls -la /tmp","timeout_ms":10000}'
# proxy_http '{"method":"GET","url":"https://api.github.com","timeout_ms":10000}'
`;
  document.getElementById('relay-script-content').value = script;
  // 重置拷贝按钮文字(防止上次打开后被改成"✅ 已复制")
  const btn = document.getElementById('relay-script-copy-btn');
  if (btn) btn.textContent = '📋 复制全部';
  document.getElementById('relay-script-modal').classList.remove('hidden');
}

function closeRelayScriptModal() {
  document.getElementById('relay-script-modal').classList.add('hidden');
}

function copyRelayScript() {
  const text = document.getElementById('relay-script-content').value;
  const btn = document.getElementById('relay-script-copy-btn');
  if (!text || !btn) return;
  // 用 navigator.clipboard(https/localhost 都可用)。fallback 到 execCommand 兼容旧浏览器。
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(() => {
      btn.textContent = '✅ 已复制';
      setTimeout(() => { btn.textContent = '📋 复制全部'; }, 1500);
    }).catch(() => fallbackCopy(text, btn));
  } else {
    fallbackCopy(text, btn);
  }
}

function fallbackCopy(text, btn) {
  const ta = document.getElementById('relay-script-content');
  if (!ta) return;
  ta.select();
  try {
    document.execCommand('copy');
    btn.textContent = '✅ 已复制';
    setTimeout(() => { btn.textContent = '📋 复制全部'; }, 1500);
  } catch (e) {
    btn.textContent = '❌ 复制失败,请手动 Ctrl+C';
    setTimeout(() => { btn.textContent = '📋 复制全部'; }, 2000);
  }
}