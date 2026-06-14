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
  try {
    const stats = await fetchJSON('/api/relay/stats');
    document.getElementById('relay-total').textContent = stats.total_count || 0;
    document.getElementById('relay-success').textContent = stats.success_count || 0;
    document.getElementById('relay-failed').textContent = stats.failed_count || 0;

    const hist = stats.date_histogram || {};
    const days = Object.keys(hist).sort();
    if (days.length > 0) {
      document.getElementById('relay-histogram').textContent =
        '近 ' + days.length + ' 天: ' + days.map(d => d + ': ' + hist[d]).join(', ');
    } else {
      document.getElementById('relay-histogram').textContent = '暂无数据';
    }
  } catch (e) {
    console.error('loadRelayStats error', e);
  }
}