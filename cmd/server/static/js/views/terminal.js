// terminal.js — Web 终端 Tab 专用终端模块
// 支持：左侧会话列表 + 右侧终端区、多会话并存

let term = null;         // xterm.js Terminal 实例
let termWs = null;       // WebSocket 连接
let termReady = false;
let currentTabID = null;  // 当前 tab_id
let initInProgress = false;
let activeSession = null; // 当前活跃会话 ID

// sessionIDFromParams 根据类型和 dirID 计算 session ID
function sessionIDFromParams(type, dirID) {
  if (type === 'remote') return 'remote_' + (dirID || '');
  return type;
}

// renderSessionList 渲染左侧会话列表
function renderSessionList() {
  const localEl = document.getElementById('rterm-local-sessions');
  const remoteEl = document.getElementById('rterm-remote-sessions');
  if (!localEl || !remoteEl) return;

  fetchJSON('/api/terminal/sessions').then(sessions => {
    const locals = sessions.filter(s => s.type !== 'remote');
    const remotes = sessions.filter(s => s.type === 'remote');

    localEl.innerHTML = locals.length === 0
      ? '<div style="font-size:10px;color:var(--text-secondary);padding:2px 8px">暂无</div>'
      : locals.map(s => sessionItemHTML(s)).join('');

    remoteEl.innerHTML = remotes.length === 0
      ? '<div style="font-size:10px;color:var(--text-secondary);padding:2px 8px">暂无</div>'
      : remotes.map(s => sessionItemHTML(s)).join('');

    updateNavStatus(sessions);
  }).catch(() => {});
}

function sessionItemHTML(s) {
  const statusDot = s.status === 'connected'
    ? '<span style="display:inline-block;width:6px;height:6px;border-radius:50%;background:#22c55e;flex-shrink:0" title="已连接"></span>'
    : s.status === 'connecting'
    ? '<span style="display:inline-block;width:6px;height:6px;border-radius:50%;background:#f59e0b;flex-shrink:0" title="连接中"></span>'
    : '<span style="display:inline-block;width:6px;height:6px;border-radius:50%;background:var(--text-secondary);flex-shrink:0" title="已断开"></span>';

  const isActive = activeSession === s.id;
  const bgStyle = isActive ? 'background:var(--hover);' : '';

  const disconnectBtn = s.status === 'connected'
    ? `<button class="rterm-session-close" onclick="event.stopPropagation();disconnectSession('${s.id}')" title="断开">×</button>`
    : '';

  return `<div class="rterm-session-item" style="${bgStyle}" onclick="switchSession('${s.id}', '${s.type}', '${s.dir_id || ''}')">
    ${statusDot}
    <span style="flex:1;font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">${s.label}</span>
    ${disconnectBtn}
  </div>`;
}

// switchSession 点击列表项切换到该会话
function switchSession(id, type, dirID) {
  activeSession = id;

  // 更新下拉框
  const typeSel = document.getElementById('rterm-type-select');
  if (typeSel) typeSel.value = type;

  const dirGroup = document.querySelector('.rterm-dir-group');
  if (type === 'remote') {
    if (dirGroup) dirGroup.style.display = '';
    const dirSel = document.getElementById('rpty-dir-select');
    if (dirSel && dirID) dirSel.value = dirID;
  } else {
    if (dirGroup) dirGroup.style.display = 'none';
  }

  renderSessionList();

  // 如果该会话已连接，当前终端显示的不是它，则自动重连到此会话
  const sessions = fetchJSON('/api/terminal/sessions');
  sessions.then(list => {
    const s = list.find(item => item.id === id);
    if (s && s.status === 'connected') {
      // 会话已连接：直接初始化终端连过去（后台会 CreateOrReplace 复用到同 ID）
      updateTermStatus('connecting');
      initTerminal(type, dirID);
    } else {
      // 会话未连接：更新状态提示用户点击连接
      updateTermStatus('disconnected');
    }
  }).catch(() => {
    updateTermStatus('disconnected');
  });
}

// disconnectSession 手动断开指定会话
function disconnectSession(sessionID) {
  fetch('/api/terminal/disconnect', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionID })
  }).then(() => {
    if (activeSession === sessionID) {
      activeSession = null;
      disconnectTerminal();
    }
    renderSessionList();
  }).catch(e => console.error('disconnect error:', e));
}

// updateNavStatus 更新导航栏终端状态点
function updateNavStatus(sessions) {
  const navStatus = document.getElementById('rterm-nav-status');
  if (!navStatus) return;
  const hasConnected = (sessions || []).some(s => s.status === 'connected');
  navStatus.style.display = hasConnected ? '' : 'none';
}

// initTerminal 初始化 xterm.js 并连接 WebSocket
function initTerminal(type, dirID) {
  const container = document.getElementById('rpty-container');
  if (!container) return;
  if (initInProgress) return;

  // 关闭旧的 WebSocket
  if (termWs) {
    termWs.onclose = null;
    termWs.close();
    termWs = null;
  }
  if (term) { term.dispose(); term = null; }
  termReady = false;
  initInProgress = true;

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
  activeSession = sessionIDFromParams(type, dirID);

  const wsUrl = buildWsUrl(tabID, type, dirID);
  termWs = new WebSocket(wsUrl);
  termWs.binaryType = 'arraybuffer';

  termWs.onopen = () => {
    initInProgress = false;
    termReady = true;
    term.writeln('\x1b[32m[xworkbench] 连接就绪\x1b[0m\r\n');
    termWs.send('resize,' + term.cols + ',' + term.rows);
    updateTermStatus('connected');
    renderSessionList();
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
    renderSessionList();
  };

  termWs.onclose = (e) => {
    termReady = false;
    initInProgress = false;
    if (e.reason) {
      term.writeln('\r\n\x1b[33m[连接已关闭: ' + e.reason + ']\x1b[0m\r\n');
    } else {
      term.writeln('\r\n\x1b[33m[连接已关闭]\x1b[0m\r\n');
    }
    updateTermStatus('disconnected');
    renderSessionList();
  };

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

  const ro = new ResizeObserver(() => {
    const pageRterm = document.getElementById('page-rterm');
    if (!pageRterm || pageRterm.classList.contains('hidden')) return;
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

// buildWsUrl 构建 WebSocket URL
function buildWsUrl(tabID, type, dirID) {
  const host = window.location.host;
  const base = '/api/pty?tab_id=' + encodeURIComponent(tabID);
  if (type === 'remote') {
    return 'ws://' + host + base + '&dir_id=' + encodeURIComponent(dirID);
  }
  const cliMap = { local_shell: 'shell', local_claude: 'claude', local_cbc: 'cbc', local_powershell: 'powershell' };
  return 'ws://' + host + base + '&cli_type=' + encodeURIComponent(cliMap[type] || 'shell');
}

function updateColsDisplay() {
  if (!term) return;
  const el = document.getElementById('rpty-cols-display');
  if (el) el.textContent = term.cols + ' 列 × ' + term.rows + ' 行';
}

function updateTermStatus(status) {
  const el = document.getElementById('rpty-status');
  if (!el) return;
  const labels = {
    connected: '<span style="color:#22c55e">已连接</span>',
    disconnected: '<span style="color:#f59e0b">未连接</span>',
    error: '<span style="color:#ef4444">错误</span>',
    connecting: '<span style="color:#f59e0b">连接中...</span>',
  };
  el.innerHTML = labels[status] || status;
}

function showAuthPanel() {
  const panel = document.getElementById('rpty-auth-panel');
  if (panel) panel.style.display = 'flex';
}

// onRtermTypeChange 终端类型切换（不自动连接，不中断现有连接）
window.onRtermTypeChange = function(type) {
  const dirGroup = document.querySelector('.rterm-dir-group');
  if (dirGroup) dirGroup.style.display = type === 'remote' ? '' : 'none';
  const connectBtn = document.getElementById('rpty-connect-btn');
  if (connectBtn) connectBtn.disabled = false;
};

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
    renderSessionList();
    return;
  }

  updateTermStatus('connecting');
  initTerminal(type, dirID);
};

window.disconnectTerminal = function() {
  initInProgress = false;
  if (termWs) {
    termWs.onclose = null;
    termWs.close();
    termWs = null;
  }
  termReady = false;
  activeSession = null;
  updateTermStatus('disconnected');
  renderSessionList();
};

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

// initTermOnFirstVisit 首次访问 Tab 时加载会话列表
window.initRptyTabOnFirstVisit = function() {
  fetchJSON('/api/dir-shortcuts').then(dirs => {
    const sel = document.getElementById('rpty-dir-select');
    if (!sel) return;
    const remote = (dirs || []).filter(d => d.type === 'remote');
    sel.innerHTML = '<option value="">— 选择远程目录 —</option>' +
      remote.map(d => '<option value="' + d.id + '">' + d.name + ' (' + d.remote_host + ')</option>').join('');
  }).catch(() => {});
  renderSessionList();
};
