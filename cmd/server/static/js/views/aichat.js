// AI Chat Tab：单 xterm.js + 单 WebSocket 接 /api/pty（macOS/Linux 真 PTY）
// 依赖 api.js

let term;
let ptyWs;
let termReady = false;

// onCliChange 保存用户选择的 CLI 到后端（下次连接时生效）
async function onCliChange(value) {
  const disp = document.getElementById('cli-display');
  if (disp) disp.textContent = value;
  await fetchJSON('/api/config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ aichat_default_cli: value }),
  });
  if (termReady) {
    term.writeln('\r\n\x1b[33m[xworkbench] CLI 已切换为 ' + value + '，刷新页面后生效\x1b[0m\r\n');
  }
}

function initTerminal() {
  if (term) return;
  const termContainer = document.getElementById('terminal');
  if (!termContainer) return;
  // 等待 DOM 完全渲染后再 open（避免 #page-aichat 从 hidden 切换时 term.open 在不可见状态调用）
  requestAnimationFrame(() => {
    if (term) return; // double-check after frame
    term = new Terminal({
      fontFamily: "'SF Mono', 'Fira Code', 'Consolas', monospace",
      fontSize: 13,
      theme: { background: '#0f172a', foreground: '#e2e8f0', cursor: '#22d3ee' },
      cursorBlink: true,
      scrollback: 10000,
    });
    term.open(termContainer);

    ptyWs = new WebSocket('ws://' + window.location.host + '/api/pty');
    ptyWs.binaryType = 'arraybuffer';

    ptyWs.onopen = () => {
      termReady = true;
      term.writeln('\x1b[32m[xworkbench] 连接就绪\x1b[0m\r\n');
    };

    ptyWs.onmessage = (e) => {
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

    ptyWs.onclose = () => {
      term.writeln('\r\n\x1b[33m[连接已关闭] 刷新页面重连\x1b[0m');
      termReady = false;
    };
    ptyWs.onerror = () => {
      term.writeln('\r\n\x1b[31m[WebSocket 错误]\x1b[0m');
    };

    // PTY 输入 → WS
    term.onData(data => { if (ptyWs && ptyWs.readyState === 1) ptyWs.send(data); });
    term.onResize(() => {
      if (ptyWs && ptyWs.readyState === 1) {
        ptyWs.send('resize,' + term.cols + ',' + term.rows);
      }
    });

    // 终端初始化后加载 CLI 设置（确保下拉框值先恢复）
    if (typeof loadCliSetting === 'function') loadCliSetting();
  });
}

// 加载保存的 CLI 选择（页面加载时恢复下拉框值）
async function loadCliSetting() {
  const sel = document.getElementById('cli-selector');
  const disp = document.getElementById('cli-display');
  if (!sel) return;
  try {
    const cfg = await fetchJSON('/api/config');
    if (cfg?.aichat_default_cli) {
      sel.value = cfg.aichat_default_cli;
      if (disp) disp.textContent = cfg.aichat_default_cli;
    }
  } catch(e) {
    console.error('[loadCliSetting] error:', e);
  }
}
