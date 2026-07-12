// terminal.js — Web 终端：左侧会话列表 + 右侧终端区，多会话并存
// 使用 termPool 管理多个 xterm.js + WebSocket，切换时不中断

const termPool = {};  // sessionID → { term, termWs, fitAddon, ready, tabID, container }
let activeSession = null;
let initInProgress = false;

function sessionIDFromParams(type, dirID) {
  if (type === 'remote') return 'remote_' + (dirID || '');
  return type;
}

// ===== Session List =====

function renderSessionList() {
  const localEl = document.getElementById('rterm-local-sessions');
  const remoteEl = document.getElementById('rterm-remote-sessions');
  if (!localEl || !remoteEl) return;

  fetchJSON('/api/terminal/sessions').then(sessions => {
    const locals = sessions.filter(s => s.type !== 'remote');
    const remotes = sessions.filter(s => s.type === 'remote');

    localEl.innerHTML = `<div class="rterm-group-label" style="padding:6px 8px 2px;font-size:10px;color:var(--text-secondary);text-transform:uppercase;letter-spacing:0.5px;font-weight:600">本地终端</div>` +
      (locals.length === 0
        ? '<div style="font-size:10px;color:var(--text-secondary);padding:4px 8px">暂无</div>'
        : locals.map(s => sessionItemHTML(s)).join(''));

    remoteEl.innerHTML = `<div class="rterm-group-label" style="padding:6px 8px 2px;font-size:10px;color:var(--text-secondary);text-transform:uppercase;letter-spacing:0.5px;font-weight:600">远程 SSH</div>` +
      (remotes.length === 0
        ? '<div style="font-size:10px;color:var(--text-secondary);padding:4px 8px">暂无</div>'
        : remotes.map(s => sessionItemHTML(s)).join(''));

    updateNavStatus(sessions);
  }).catch(() => {});
}

function sessionItemHTML(s) {
  // 以前端 termPool 的实际连接状态为准（比后端 API 更实时）
  const poolInfo = termPool[s.id];
  const frontendConnected = poolInfo && poolInfo.ready;
  const displayStatus = frontendConnected ? 'connected' : s.status;
  const isConnected = displayStatus === 'connected';
  const isConnecting = displayStatus === 'connecting';
  const isActive = activeSession === s.id;
  const dotColor = isConnected ? '#22c55e' : isConnecting ? '#f59e0b' : 'var(--text-secondary)';
  const borderStyle = isActive ? 'border-left:2px solid var(--primary);' : '';

  let actionBtn = '';
  if (isConnected) {
    actionBtn = `<button class="rterm-session-close" onclick="event.stopPropagation();disconnectSession('${s.id}')" title="断开">×</button>`;
  } else if (s.type === 'remote' && s.status === 'disconnected') {
    actionBtn = `<button class="rterm-session-close" onclick="event.stopPropagation();removeSession('${s.id}')" title="删除记录">×</button>`;
  }

  return `<div class="rterm-session-item" style="${borderStyle}" onclick="switchSession('${s.id}', '${s.type}', '${s.dir_id || ''}')">
    <span style="display:inline-block;width:6px;height:6px;border-radius:50%;background:${dotColor};flex-shrink:0" title="${displayStatus}"></span>
    <span style="flex:1;font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">${s.label}</span>
    ${actionBtn}
  </div>`;
}

// ===== Session Switching =====

function switchSession(id, type, dirID) {
  if (activeSession === id) return;

  // 隐藏当前 session 的终端
  if (activeSession && termPool[activeSession]) {
    termPool[activeSession].container.style.display = 'none';
  }

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

  // 目标会话如果已经连接并已有 termPool 实例，直接显示
  if (termPool[id] && termPool[id].ready) {
    termPool[id].container.style.display = '';
    updateTermStatus('connected');
    updateColsDisplay(id);
    return;
  }

  // 目标会话在后端已连接但前端缺少实例（tab 切走又切回），自动重建前端连接
  fetchJSON('/api/terminal/sessions').then(list => {
    const s = list.find(item => item.id === id);
    if (s && s.status === 'connected') {
      updateTermStatus('connecting');
      initTerminal(type, dirID);
    } else {
      updateTermStatus('disconnected');
    }
  }).catch(() => {
    updateTermStatus('disconnected');
  });
}

// ===== Terminal Management =====

function initTerminal(type, dirID) {
  const sessionID = sessionIDFromParams(type, dirID);
  const mainContainer = document.getElementById('rpty-container');
  if (!mainContainer || initInProgress) return;
  initInProgress = true;

  // 关闭该 session 的旧连接（如果存在）
  if (termPool[sessionID]) {
    if (termPool[sessionID].termWs) {
      termPool[sessionID].termWs.onclose = null;
      termPool[sessionID].termWs.close();
    }
    if (termPool[sessionID].term) termPool[sessionID].term.dispose();
    if (termPool[sessionID].container) termPool[sessionID].container.remove();
    delete termPool[sessionID];
  }

  // 创建该 session 的专属容器
  const container = document.createElement('div');
  container.style.cssText = 'position:absolute;top:0;left:0;right:0;bottom:0';
  container.setAttribute('data-session', sessionID);
  mainContainer.appendChild(container);

  const fitAddon = new FitAddon.FitAddon();
  const term = new Terminal({
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

  const info = { term, fitAddon, ready: false, tabID, container, termWs: null };
  termPool[sessionID] = info;

  // 隐藏其他 session，显示当前
  Object.keys(termPool).forEach(id => {
    if (termPool[id].container) termPool[id].container.style.display = id === sessionID ? '' : 'none';
  });
  activeSession = sessionID;

  const wsUrl = buildWsUrl(tabID, type, dirID);
  const termWs = new WebSocket(wsUrl);
  termWs.binaryType = 'arraybuffer';
  info.termWs = termWs;

  termWs.onopen = () => {
    initInProgress = false;
    info.ready = true;
    term.writeln('\x1b[32m[xworkbench] 连接就绪\x1b[0m\r\n');
    termWs.send('resize,' + term.cols + ',' + term.rows);
    updateTermStatus('connected');
    // 延迟刷新：后端 CreateOrReplace 在 onopen 后执行，等 200ms
    setTimeout(() => renderSessionList(), 200);
    fitAddon.fit();
  };

  termWs.onmessage = (e) => {
    if (!info.ready) return;
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

  termWs.onerror = () => {
    initInProgress = false;
    term.writeln('\r\n\x1b[31m[WebSocket 错误]\x1b[0m\r\n');
    updateTermStatus('error');
    renderSessionList();
  };

  termWs.onclose = (e) => {
    info.ready = false;
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
    updateColsDisplay(sessionID);
  });

  const ro = new ResizeObserver(() => {
    const pageRterm = document.getElementById('page-rterm');
    if (!pageRterm || pageRterm.classList.contains('hidden')) return;
    if (info.ready && activeSession === sessionID) {
      fitAddon.fit();
      if (termWs && termWs.readyState === WebSocket.OPEN) {
        termWs.send('resize,' + term.cols + ',' + term.rows);
      }
      updateColsDisplay(sessionID);
    }
  });
  ro.observe(container);
  updateColsDisplay(sessionID);
}

// ===== Helpers =====

function buildWsUrl(tabID, type, dirID) {
  const host = window.location.host;
  const base = '/api/pty?tab_id=' + encodeURIComponent(tabID);
  if (type === 'remote') {
    return 'ws://' + host + base + '&dir_id=' + encodeURIComponent(dirID);
  }
  const cliMap = { local_shell: 'shell', local_claude: 'claude', local_cbc: 'cbc', local_powershell: 'powershell' };
  return 'ws://' + host + base + '&cli_type=' + encodeURIComponent(cliMap[type] || 'shell');
}

function updateColsDisplay(sessionID) {
  const info = termPool[sessionID];
  if (!info || !info.term) return;
  const el = document.getElementById('rpty-cols-display');
  if (el) el.textContent = info.term.cols + ' 列 × ' + info.term.rows + ' 行';
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

function updateNavStatus(sessions) {
  const navStatus = document.getElementById('rterm-nav-status');
  if (!navStatus) return;
  const hasConnected = (sessions || []).some(s => s.status === 'connected');
  navStatus.style.display = hasConnected ? '' : 'none';
}

// ===== Session Actions =====

function disconnectSession(sessionID) {
  fetch('/api/terminal/disconnect', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionID })
  }).then(() => {
    // 关闭前端连接
    if (termPool[sessionID]) {
      if (termPool[sessionID].termWs) {
        termPool[sessionID].termWs.onclose = null;
        termPool[sessionID].termWs.close();
      }
      termPool[sessionID].ready = false;
    }
    if (activeSession === sessionID) {
      activeSession = null;
      updateTermStatus('disconnected');
    }
    renderSessionList();
  }).catch(e => console.error('disconnect error:', e));
}

function removeSession(sessionID) {
  fetch('/api/terminal/remove', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionID })
  }).then(() => {
    if (termPool[sessionID]) {
      if (termPool[sessionID].termWs) { termPool[sessionID].termWs.onclose = null; termPool[sessionID].termWs.close(); }
      if (termPool[sessionID].term) termPool[sessionID].term.dispose();
      if (termPool[sessionID].container) termPool[sessionID].container.remove();
      delete termPool[sessionID];
    }
    if (activeSession === sessionID) activeSession = null;
    renderSessionList();
  }).catch(e => console.error('remove error:', e));
}

// ===== Window Hooks =====

// toggleRtermSidebar 收起/展开左侧栏
window.toggleRtermSidebar = function() {
  const sidebar = document.getElementById('rterm-sidebar');
  if (!sidebar) return;
  const hidden = sidebar.style.width === '0px';
  sidebar.style.width = hidden ? '160px' : '0px';
  sidebar.style.borderRight = hidden ? '' : 'none';
  sidebar.style.overflow = hidden ? '' : 'hidden';
  // 更新按钮图标
  const btn = document.querySelector('#page-rterm .btn-small');
  if (btn) btn.textContent = hidden ? '☰' : '☰';
};

window.onRtermTypeChange = function(type) {
  const dirGroup = document.querySelector('.rterm-dir-group');
  if (dirGroup) dirGroup.style.display = type === 'remote' ? '' : 'none';
  const connectBtn = document.getElementById('rpty-connect-btn');
  if (connectBtn) connectBtn.disabled = false;
  renderSessionList();
};

window.onRptyDirChange = function(dirID) {
  const btn = document.getElementById('rpty-connect-btn');
  if (btn) btn.disabled = !dirID;
};

window.onRptyConnect = function() {
  const typeSel = document.getElementById('rterm-type-select');
  const dirSel = document.getElementById('rpty-dir-select');
  const type = typeSel ? typeSel.value : 'local_shell';
  const dirID = dirSel ? dirSel.value : '';

  if (type === 'remote' && !dirID) {
    const info = termPool[activeSession];
    if (info && info.term) info.term.writeln('\x1b[31m[错误] 请先选择远程目录\x1b[0m\r\n');
    renderSessionList();
    return;
  }

  updateTermStatus('connecting');
  initTerminal(type, dirID);
  setTimeout(() => renderSessionList(), 100);
  setTimeout(() => renderSessionList(), 100);
};

window.disconnectTerminal = function() {
  if (activeSession && termPool[activeSession]) {
    if (termPool[activeSession].termWs) {
      termPool[activeSession].termWs.onclose = null;
      termPool[activeSession].termWs.close();
    }
    termPool[activeSession].ready = false;
  }
  activeSession = null;
  updateTermStatus('disconnected');
  renderSessionList();
};

window.submitAuthInput = function(input) {
  if (!activeSession || !termPool[activeSession] || !termPool[activeSession].ready) {
    const info = termPool[activeSession];
    if (info && info.term) info.term.writeln('\x1b[31m[错误] 无活跃连接\x1b[0m\r\n');
    return;
  }
  const tabID = termPool[activeSession].tabID;
  fetch('/api/pty/' + encodeURIComponent(tabID) + '/submit-input', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ input }),
  }).then(r => {
    if (!r.ok && termPool[activeSession] && termPool[activeSession].term)
      r.json().then(b => termPool[activeSession].term.writeln('\x1b[31m[提交失败] ' + (b.error || r.statusText) + '\x1b[0m\r\n'));
  }).catch(e => {
    if (termPool[activeSession] && termPool[activeSession].term)
      termPool[activeSession].term.writeln('\x1b[31m[提交失败] ' + e.message + '\x1b[0m\r\n');
  });
  document.getElementById('rpty-auth-panel').style.display = 'none';
};

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
