// AI Chat Tab：单 xterm.js + 单 WebSocket 接 /api/pty（macOS/Linux 真 PTY）
// 依赖 api.js

let term;
let ws;
let termReady = false;

// onCliChange 保存用户选择的 CLI 到后端（下次连接时生效）
async function onCliChange(value) {
  const disp = document.getElementById('cli-display');
  if (disp) disp.textContent = value;
  await fetchJSON('/api/settings/aichat_default_cli', {
    method: 'PUT',
    body: JSON.stringify({ value }),
  });
  if (termReady) {
    term.writeln('\r\n\x1b[33m[xworkbench] CLI 已切换为 ' + value + '，刷新页面后生效\x1b[0m\r\n');
  }
}

function initTerminal() {
  if (term) return;
  term = new Terminal({
    fontFamily: "'SF Mono', 'Fira Code', 'Consolas', monospace",
    fontSize: 13,
    theme: { background: '#0f172a', foreground: '#e2e8f0', cursor: '#22d3ee' },
    cursorBlink: true,
    scrollback: 10000,
  });
  term.open(document.getElementById('terminal'));

  ws = new WebSocket('ws://' + window.location.host + '/api/pty');
  ws.binaryType = 'arraybuffer';

  ws.onopen = () => {
    termReady = true;
    term.writeln('\x1b[32m[xworkbench] 连接就绪\x1b[0m\r\n');
  };

  ws.onmessage = (e) => {
    if (!termReady) return;
    if (e.data instanceof ArrayBuffer) {
      // PTY 二进制输出
      term.write(new Uint8Array(e.data));
    } else {
      // 文本消息：可能是 JSON 控制消息(auth_required)或普通字符串
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === 'auth_required') {
          term.writeln('\r\n\x1b[33m[xworkbench] 需要授权: ' + (msg.extra || '') + '\x1b[0m\r\n');
        }
        // 其它 JSON 消息忽略(暂时没有其它类型)
      } catch {
        // 非 JSON 文本(例如 PTY 启动 banner)直接写
        term.write(e.data);
      }
    }
  };

  ws.onclose = () => {
    term.writeln('\r\n\x1b[33m[连接已关闭] 刷新页面重连\x1b[0m');
    termReady = false;
  };
  ws.onerror = () => {
    term.writeln('\r\n\x1b[31m[WebSocket 错误]\x1b[0m');
  };

  // PTY 输入 → WS
  term.onData(data => { if (ws && ws.readyState === 1) ws.send(data); });
  term.onResize(() => {
    if (ws && ws.readyState === 1) {
      ws.send('resize,' + term.cols + ',' + term.rows);
    }
  });

  // 终端初始化后加载 CLI 设置（确保下拉框值先恢复）
  if (typeof loadCliSetting === 'function') loadCliSetting();
}

// 加载保存的 CLI 选择（页面加载时恢复下拉框值）
async function loadCliSetting() {
  const sel = document.getElementById('cli-selector');
  const disp = document.getElementById('cli-display');
  if (!sel) return;
  try {
    const settings = await fetchJSON('/api/settings');
    if (settings?.aichat_default_cli) {
      sel.value = settings.aichat_default_cli;
      if (disp) disp.textContent = settings.aichat_default_cli;
    }
  } catch(e) {
    console.error('[loadCliSetting] error:', e);
  }
}
