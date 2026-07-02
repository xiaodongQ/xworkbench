// 远程 PTY 终端 Tab：xterm.js + WebSocket 接 /api/rpty
// 依赖: api.js, xterm.js (via index.html CDN)

let rptyTerm = null;        // xterm.js Terminal 实例
let rptyWs = null;          // WebSocket 连接
let rptyReady = false;
let rptyTabID = null;       // 当前 tab_id
let rptyDirID = null;       // 当前 dir_id

// initRptyTab 初始化远程终端 tab（switchTab 时调用）。
function initRptyTab() {
  if (rptyTerm) return; // 已初始化
  const container = document.getElementById('rpty-container');
  if (!container) return;

  requestAnimationFrame(() => {
    if (rptyTerm) return;

    rptyTerm = new Terminal({
      fontFamily: "'SF Mono', 'Fira Code', 'Consolas', monospace",
      fontSize: 13,
      theme: { background: '#0f172a', foreground: '#e2e8f0', cursor: '#22d3ee' },
      cursorBlink: true,
      scrollback: 10000,
    });
    rptyTerm.open(container);

    // 默认状态提示
    rptyTerm.writeln('\x1b[36m[xworkbench] 远程终端\x1b[0m 请从上方选择目录后点击"连接"\r\n');
    rptyTerm.writeln('\x1b[2m提示：在侧边栏选择一个远程目录（🌐），点击终端图标选择"Web 内嵌终端"\x1b[0m\r\n');
  });
}

// connectRPTY 连接远程 PTY。
// tabID: 前端 tab 标识符（用于 submit-input API）
// dirID: DirShortcut ID
async function connectRPTY(tabID, dirID) {
  if (!tabID || !dirID) {
    if (rptyTerm) rptyTerm.writeln('\r\n\x1b[31m[错误] 缺少 tab_id 或 dir_id\x1b[0m\r\n');
    return;
  }

  // 断开已有连接
  if (rptyWs) {
    rptyWs.close();
    rptyWs = null;
  }
  rptyReady = false;
  rptyTabID = tabID;
  rptyDirID = dirID;

  if (rptyTerm) {
    rptyTerm.clear();
    rptyTerm.writeln('\x1b[36m[xworkbench] 正在连接...\x1b[0m\r\n');
  }

  rptyWs = new WebSocket('ws://' + window.location.host + '/api/rpty?tab_id=' + encodeURIComponent(tabID) + '&dir_id=' + encodeURIComponent(dirID));
  rptyWs.binaryType = 'arraybuffer';

  rptyWs.onopen = () => {
    rptyReady = true;
    if (rptyTerm) rptyTerm.writeln('\x1b[32m[xworkbench] 连接就绪\x1b[0m\r\n');
    updateRptyStatus('connected');
  };

  rptyWs.onmessage = (e) => {
    if (!rptyReady || !rptyTerm) return;
    if (e.data instanceof ArrayBuffer) {
      rptyTerm.write(new Uint8Array(e.data));
    } else {
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === 'auth_required') {
          rptyTerm.writeln('\r\n\x1b[33m[xworkbench] 需要授权: ' + (msg.extra || '') + '\x1b[0m\r\n');
          // 提示用户可使用 submit-input 提交响应
          rptyTerm.writeln('\x1b[2m（使用下方 auth 响应框输入后回车）\x1b[0m\r\n');
        }
      } catch {
        rptyTerm.write(e.data);
      }
    }
  };

  rptyWs.onclose = () => {
    rptyReady = false;
    if (rptyTerm) rptyTerm.writeln('\r\n\x1b[33m[连接已关闭]\x1b[0m\r\n');
    updateRptyStatus('disconnected');
  };

  rptyWs.onerror = () => {
    if (rptyTerm) rptyTerm.writeln('\r\n\x1b[31m[WebSocket 错误]\x1b[0m\r\n');
    updateRptyStatus('error');
  };

  // PTY 输入 → WS
  rptyTerm.onData(data => {
    if (rptyWs && rptyWs.readyState === WebSocket.OPEN) {
      rptyWs.send(data);
    }
  });

  // PTY resize → WS
  rptyTerm.onResize(({ cols, rows }) => {
    if (rptyWs && rptyWs.readyState === WebSocket.OPEN) {
      rptyWs.send('resize,' + cols + ',' + rows);
    }
  });
}

// disconnectRPTY 断开当前连接。
function disconnectRPTY() {
  if (rptyWs) {
    rptyWs.close();
    rptyWs = null;
  }
  rptyReady = false;
  rptyTabID = null;
  rptyDirID = null;
  if (rptyTerm) {
    rptyTerm.clear();
    rptyTerm.writeln('\x1b[36m[xworkbench] 远程终端\x1b[0m 已断开\r\n');
  }
  updateRptyStatus('disconnected');
}

// submitAuthInput 向当前 session 发送 auth 响应（用于 Password:/yes 等）。
async function submitAuthInput(input) {
  if (!rptyTabID || !rptyReady) {
    if (rptyTerm) rptyTerm.writeln('\x1b[31m[错误] 无活跃连接\x1b[0m\r\n');
    return;
  }
  try {
    const r = await fetch('/api/rpty/' + encodeURIComponent(rptyTabID) + '/submit-input', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ input }),
    });
    if (!r.ok && rptyTerm) {
      r.json().then(body => rptyTerm.writeln('\x1b[31m[提交失败] ' + (body.error || r.statusText) + '\x1b[0m\r\n'));
    }
  } catch (e) {
    if (rptyTerm) rptyTerm.writeln('\x1b[31m[提交失败] ' + e.message + '\x1b[0m\r\n');
  }
}

// updateRptyStatus 更新连接状态显示。
function updateRptyStatus(status) {
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

// openRptyForDir 从 dir shortcut 侧边栏触发连接。
// dirID: DirShortcut ID
// dirName: 显示用名称
async function openRptyForDir(dirID, dirName) {
  // 切换到 rterm tab
  switchTab('rterm');
  // 等 tab 渲染
  await new Promise(r => setTimeout(r, 100));

  // 初始化 terminal（如果还没）
  initRptyTab();
  await new Promise(r => setTimeout(r, 50));

  // 填充 dir selector（如果有）
  const sel = document.getElementById('rpty-dir-select');
  if (sel) sel.value = dirID;

  // 生成唯一 tab_id
  const tabID = 'rpty-' + Date.now();

  // 更新状态
  const statusEl = document.getElementById('rpty-status');
  if (statusEl) statusEl.innerHTML = '\x1b[33m连接中...\x1b[0m';

  connectRPTY(tabID, dirID);
}

// onRptyDirChange 切换目录时更新连接按钮状态。
function onRptyDirChange(dirID) {
  const btn = document.getElementById('rpty-connect-btn');
  if (btn) btn.disabled = !dirID;
}

// onRptyConnect 从 select 读取 dirID，触发连接。
async function onRptyConnect() {
  const sel = document.getElementById('rpty-dir-select');
  const dirID = sel ? sel.value : '';
  if (!dirID) {
    if (rptyTerm) rptyTerm.writeln('\x1b[31m[错误] 请先选择一个远程目录\x1b[0m\r\n');
    return;
  }
  // dirName 取 option text
  const opt = sel.options[sel.selectedIndex];
  const dirName = opt ? opt.text : dirID;
  await openRptyForDir(dirID, dirName);
}

// loadRptyDirList 从 dirDB 加载远程 shortcut 到 select（切换到 rterm tab 时调用一次）。
async function loadRptyDirList() {
  const sel = document.getElementById('rpty-dir-select');
  if (!sel) return;
  try {
    const dirs = await fetchJSON('/api/dir-shortcuts');
    const remoteDirs = (dirs || []).filter(d => d.type === 'remote');
    sel.innerHTML = '<option value="">— 选择远程目录 —</option>' +
      remoteDirs.map(d =>
        '<option value="' + esc(d.id) + '">' + esc(d.name) + ' (' + esc(d.remote_host) + ')</option>'
      ).join('');
  } catch (e) {
    console.error('[loadRptyDirList]', e);
  }
}

// initRptyTabOnFirstVisit 首次进入 rterm tab 时加载 dir list 并初始化 terminal。
function initRptyTabOnFirstVisit() {
  loadRptyDirList();
  initRptyTab();
}

// esc HTML 转义（最小实现，避免 XSS）。
function esc(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// 暴露给外部调用（widgets.js 远程目录"内嵌终端"按钮）
window.openRptyForDir = openRptyForDir;