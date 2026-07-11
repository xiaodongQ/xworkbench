// terminal.js — Web 终端 Tab 专用终端模块
// 支持：本地 Shell / 本地 Claude CLI / 本地 CBC CLI / 远程 SSH（xw-sshpass）
// 依赖: api.js, xterm.js (via index.html CDN)

let term = null;         // xterm.js Terminal 实例
let termWs = null;       // WebSocket 连接
let termReady = false;
let currentTabID = null;  // 当前 tab_id
let initInProgress = false; // 防止 initTerminal 重复调用

// initTerminal 初始化 xterm.js 并连接 WebSocket
function initTerminal(type, dirID) {
  console.log('[terminal] initTerminal called', { type, dirID, initInProgress });
  const container = document.getElementById('rpty-container');
  if (!container) return;
  if (initInProgress) {
    console.log('[terminal] initTerminal already in progress, skipping');
    return;
  }
  // 关闭旧的 WebSocket（如果存在）
  if (termWs) {
    console.log('[terminal] closing existing termWs');
    termWs.onclose = null; // 避免触发旧的 onclose
    termWs.close();
    termWs = null;
  }
  if (term) { term.dispose(); term = null; }
  termReady = false;
  initInProgress = true;

  // 控制远程目录选择框显隐（onRtermTypeChange 的显隐逻辑合并到这里，避免重复调用 disconnectTerminal）
  const dirGroup = document.querySelector('.rterm-dir-group');
  if (dirGroup) dirGroup.style.display = type === 'remote' ? '' : 'none';

  const fitAddon = new FitAddon.FitAddon();
  term = new Terminal({
    fontFamily: "'SF Mono', 'Fira Code', 'Consolas', monospace",
    fontSize: 13,
    theme: { background: '#0f172a', foreground: '#e2e8f0', cursor: '#22d3ee' },
    cursorBlink: true,
    scrollback: 10000,
  });
  term.loadAddon(fitAddon);
  term.open(container);
  fitAddon.fit();

  const tabID = 'term-' + Date.now() + '-' + Math.random().toString(36).slice(2, 8);
  currentTabID = tabID;

  const wsUrl = buildWsUrl(tabID, type, dirID);
  console.log('[terminal] connecting to', wsUrl);
  termWs = new WebSocket(wsUrl);
  termWs.binaryType = 'arraybuffer';

  termWs.onopen = () => {
    console.log('[terminal] ws onopen fired');
    initInProgress = false;
    termReady = true;
    term.writeln('\x1b[32m[xworkbench] 连接就绪\x1b[0m\r\n');
    termWs.send('resize,' + term.cols + ',' + term.rows);
    updateTermStatus('connected');
    fitAddon.fit();
  };

  termWs.onmessage = (e) => {
    if (!termReady) return;
    if (e.data instanceof ArrayBuffer) {
      term.write(new Uint8Array(e.data));
    } else {
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === 'auth_required') {
          term.writeln('\r\n\x1b[33m[xworkbench] 需要授权: ' + (msg.extra || '') + '\x1b[0m\r\n');
          term.writeln('\x1b[2m（使用下方 auth 响应框输入后回车）\x1b[0m\r\n');
          showAuthPanel();
        }
      } catch {
        term.write(e.data);
      }
    }
  };

  termWs.onerror = (e) => {
    console.error('[terminal] ws onerror', e);
    initInProgress = false;
    term.writeln('\r\n\x1b[31m[WebSocket 错误]\x1b[0m\r\n');
    updateTermStatus('error');
  };

  termWs.onclose = (e) => {
    termReady = false;
    initInProgress = false;
    console.log('[terminal] ws onclose', e.code, e.reason);
    if (e.reason) {
      term.writeln('\r\n\x1b[33m[连接已关闭: ' + e.reason + ']\x1b[0m\r\n');
    } else {
      term.writeln('\r\n\x1b[33m[连接已关闭]\x1b[0m\r\n');
    }
    updateTermStatus('disconnected');
  };

  // PTY 自带回显，不需要前端 echo
  term.onData(data => {
    if (termWs && termWs.readyState === WebSocket.OPEN) termWs.send(data);
  });

  term.onResize(({ cols, rows }) => {
    if (termWs && termWs.readyState === WebSocket.OPEN) {
      termWs.send('resize,' + cols + ',' + rows);
    }
    fitAddon.fit();
    updateColsDisplay();
  });

  // ResizeObserver：容器大小变化时自动 fit
  const ro = new ResizeObserver(() => {
    if (termReady) {
      fitAddon.fit();
      if (termWs && termWs.readyState === WebSocket.OPEN) {
        termWs.send('resize,' + term.cols + ',' + term.rows);
      }
      updateColsDisplay();
    }
  });
  ro.observe(container);

  updateColsDisplay();
}

// buildWsUrl 根据终端类型和 dirID 构建 WebSocket URL
function buildWsUrl(tabID, type, dirID) {
  const host = window.location.host;
  const base = '/api/pty?tab_id=' + encodeURIComponent(tabID);
  if (type === 'remote') {
    return 'ws://' + host + base + '&dir_id=' + encodeURIComponent(dirID);
  }
  // 本地 shell/claude/cbc
  const cliMap = { local_shell: 'shell', local_claude: 'claude', local_cbc: 'cbc', local_powershell: 'powershell' };
  return 'ws://' + host + base + '&cli_type=' + encodeURIComponent(cliMap[type] || 'shell');
}

// updateColsDisplay 更新 header 列数显示
function updateColsDisplay() {
  if (!term) return;
  const el = document.getElementById('rpty-cols-display');
  if (el) el.textContent = term.cols + ' 列 × ' + term.rows + ' 行';
}

// updateTermStatus 更新状态文本
function updateTermStatus(status) {
  const el = document.getElementById('rpty-status');
  if (!el) return;
  const labels = {
    connected: '\x1b[32m已连接\x1b[0m',
    disconnected: '\x1b[33m未连接\x1b[0m',
    error: '\x1b[31m错误\x1b[0m',
    connecting: '\x1b[33m连接中...\x1b[0m',
  };
  el.innerHTML = labels[status] || status;
}

// showAuthPanel 显示授权响应框
function showAuthPanel() {
  const panel = document.getElementById('rpty-auth-panel');
  if (panel) panel.style.display = 'flex';
}

// onRtermTypeChange 终端类型切换
window.onRtermTypeChange = function(type) {
  const dirGroup = document.querySelector('.rterm-dir-group');
  if (dirGroup) dirGroup.style.display = type === 'remote' ? '' : 'none';
  disconnectTerminal();
};

// onRptyDirChange 目录切换
window.onRptyDirChange = function(dirID) {
  const btn = document.getElementById('rpty-connect-btn');
  if (btn) btn.disabled = !dirID;
};

// onRptyConnect 连接按钮
window.onRptyConnect = function() {
  const typeSel = document.getElementById('rterm-type-select');
  const dirSel = document.getElementById('rpty-dir-select');
  const type = typeSel ? typeSel.value : 'local_shell';
  const dirID = dirSel ? dirSel.value : '';

  if (type === 'remote' && !dirID) {
    if (term) term.writeln('\x1b[31m[错误] 请先选择远程目录\x1b[0m\r\n');
    return;
  }

  updateTermStatus('connecting');
  initTerminal(type, dirID);
};

// disconnectTerminal 断开连接
window.disconnectTerminal = function() {
  initInProgress = false;
  if (termWs) {
    termWs.onclose = null; // 避免旧连接的 close 影响新连接
    termWs.close();
    termWs = null;
  }
  termReady = false;
  updateTermStatus('disconnected');
};

// submitAuthInput 提交授权响应
window.submitAuthInput = function(input) {
  if (!currentTabID || !termReady) {
    if (term) term.writeln('\x1b[31m[错误] 无活跃连接\x1b[0m\r\n');
    return;
  }
  fetch('/api/pty/' + encodeURIComponent(currentTabID) + '/submit-input', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ input }),
  }).then(r => {
    if (!r.ok && term) r.json().then(b => term.writeln('\x1b[31m[提交失败] ' + (b.error || r.statusText) + '\x1b[0m\r\n'));
  }).catch(e => {
    if (term) term.writeln('\x1b[31m[提交失败] ' + e.message + '\x1b[0m\r\n');
  });
  const panel = document.getElementById('rpty-auth-panel');
  if (panel) panel.style.display = 'none';
};

// initTermOnFirstVisit 首次访问 Tab 时初始化
window.initRptyTabOnFirstVisit = function() {
  // 加载远程目录列表
  fetchJSON('/api/dir-shortcuts').then(dirs => {
    const sel = document.getElementById('rpty-dir-select');
    if (!sel) return;
    const remote = (dirs || []).filter(d => d.type === 'remote');
    sel.innerHTML = '<option value="">— 选择远程目录 —</option>' +
      remote.map(d => '<option value="' + d.id + '">' + d.name + ' (' + d.remote_host + ')</option>').join('');
  }).catch(() => {});
  // 初始化终端（不要在这里调用 onRtermTypeChange，它会触发 disconnectTerminal 关掉刚建的 WS）
  initTerminal('local_shell', '');
};
