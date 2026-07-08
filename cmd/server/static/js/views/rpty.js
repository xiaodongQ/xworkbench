// 远程 PTY 终端 Tab：xterm.js + WebSocket 接 /api/rpty
// 会话持久化：每个 dir shortcut 存储历史 tab session，支持选历史 / 新建 shell
// 依赖: api.js, xterm.js (via index.html CDN)

let rptyTerm = null;        // xterm.js Terminal 实例
window.__rptyTerm = () => rptyTerm; // 调试用
let rptyWs = null;          // WebSocket 连接
let rptyReady = false;
let rptyTabID = null;       // 当前 tab_id（持久化到 localStorage）
let rptyDirID = null;       // 当前 dir_id

// localStorage 键前缀
const RPTY_SESSIONS_KEY = 'sf_rterm_sessions'; // { [dirID]: RptySession[] }

// RptySession: { tabID, dirID, dirName, createdAt, lastActive }
function getStoredSessions(dirID) {
  try {
    const all = JSON.parse(localStorage.getItem(RPTY_SESSIONS_KEY) || '{}');
    return all[dirID] || [];
  } catch { return []; }
}

function saveStoredSessions(dirID, sessions) {
  try {
    const all = JSON.parse(localStorage.getItem(RPTY_SESSIONS_KEY) || '{}');
    all[dirID] = sessions;
    localStorage.setItem(RPTY_SESSIONS_KEY, JSON.stringify(all));
  } catch {}
}

// 获取某 dir shortcut 的最新会话 tabID（用于自动重连）
function getLastTabID(dirID) {
  const sessions = getStoredSessions(dirID);
  if (sessions.length === 0) return null;
  // 返回最新活跃的
  return sessions[0].tabID;
}

// 创建新 session 并保存到历史
function createRptySession(dirID, dirName) {
  const tabID = 'rpty-' + Date.now() + '-' + Math.random().toString(36).slice(2, 8);
  const sessions = getStoredSessions(dirID);
  // 去重（同名 tabID 只保留一个）
  const filtered = sessions.filter(s => s.tabID !== tabID);
  filtered.unshift({ tabID, dirID, dirName, createdAt: Date.now(), lastActive: Date.now() });
  if (filtered.length > 10) filtered.splice(10);
  saveStoredSessions(dirID, filtered);
  return tabID;
}

// 更新 session 的 lastActive（每次连接时调用）
function touchSession(dirID, tabID) {
  const sessions = getStoredSessions(dirID);
  const s = sessions.find(x => x.tabID === tabID);
  if (s) {
    s.lastActive = Date.now();
    saveStoredSessions(dirID, sessions);
  }
}

// 删除某 session
function deleteSession(dirID, tabID) {
  const sessions = getStoredSessions(dirID).filter(s => s.tabID !== tabID);
  saveStoredSessions(dirID, sessions);
}

// ===== 会话选择器 UI =====

function showRptySessionPicker(dirID, dirName) {
  const sessions = getStoredSessions(dirID);
  const itemsHtml = sessions.length === 0
    ? '<div style="color:var(--text-secondary);font-size:13px;text-align:center;padding:16px">暂无历史会话</div>'
    : sessions.map(s => `
        <div style="display:flex;align-items:center;justify-content:space-between;padding:8px 0;border-bottom:1px solid var(--border)">
          <div>
            <div style="font-size:13px;font-weight:500">${esc(s.dirName || dirName)}</div>
            <div style="font-size:11px;color:var(--text-secondary);margin-top:2px">
              ${formatRelTime(s.lastActive)} · ${esc(s.tabID.slice(0, 20))}...
            </div>
          </div>
          <div style="display:flex;gap:6px">
            <button onclick="rptySelectSession('${dirID}','${dirName}','${s.tabID}')" style="
              background:var(--primary);color:#fff;border:none;border-radius:6px;padding:4px 10px;cursor:pointer;font-size:12px">连接</button>
            <button onclick="rptyDeleteSession('${dirID}','${s.tabID}')" style="
              background:transparent;color:var(--text-secondary);border:1px solid var(--border);border-radius:6px;padding:4px 8px;cursor:pointer;font-size:12px">×</button>
          </div>
        </div>`).join('');

  const html = `
    <div id="rpty-picker-overlay" style="
      position:fixed;inset:0;background:rgba(0,0,0,0.6);z-index:9999;
      display:flex;align-items:center;justify-content:center"
      onclick="if(event.target.id==='rpty-picker-overlay')closeRptySessionPicker()">
      <div style="
        background:var(--card);border:1px solid var(--border);border-radius:12px;
        padding:24px;width:420px;max-width:90vw;max-height:80vh;overflow-y:auto;box-shadow:0 20px 60px rgba(0,0,0,0.4)">
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:16px">
          <h3 style="margin:0;font-size:15px;font-weight:600">🖥️ ${esc(dirName)}</h3>
          <button onclick="rptyNewShell('${dirID}','${dirName}')" style="
            background:var(--primary);color:#fff;border:none;border-radius:6px;
            padding:6px 14px;cursor:pointer;font-size:13px;font-weight:500">+ 新建 Shell</button>
        </div>
        <div>${itemsHtml}</div>
        <div style="margin-top:12px;padding-top:12px;border-top:1px solid var(--border);display:flex;justify-content:flex-end">
          <button onclick="closeRptySessionPicker()" style="
            background:transparent;color:var(--text-secondary);border:1px solid var(--border);
            border-radius:6px;padding:5px 12px;cursor:pointer;font-size:12px">取消</button>
        </div>
      </div>
    </div>`;
  document.body.insertAdjacentHTML('beforeend', html);
}

function closeRptySessionPicker() {
  const el = document.getElementById('rpty-picker-overlay');
  if (el) el.remove();
}

// 选历史会话 → 连接
function rptySelectSession(dirID, dirName, tabID) {
  closeRptySessionPicker();
  // 更新该 session 的 lastActive
  touchSession(dirID, tabID);
  openRptyForDirWithTabID(dirID, dirName, tabID);
}

// 新建 shell
function rptyNewShell(dirID, dirName) {
  closeRptySessionPicker();
  const newTabID = createRptySession(dirID, dirName);
  openRptyForDirWithTabID(dirID, dirName, newTabID);
}

// 删除历史会话
function rptyDeleteSession(dirID, tabID) {
  deleteSession(dirID, tabID);
  event.stopPropagation();
  // 重新渲染 picker
  const dirName = getStoredSessions(dirID).find(s => s.tabID === tabID)?.dirName || '';
  closeRptySessionPicker();
  showRptySessionPicker(dirID, dirName || '远程终端');
}

// openRptyForDir 从 dir shortcut 侧边栏触发连接（点击 ⌷ 按钮时调用）
// 行为：如有上次的 tab_id → 重连；无 → 新建
async function openRptyForDir(dirID, dirName) {
  const lastTabID = getLastTabID(dirID);
  if (lastTabID) {
    touchSession(dirID, lastTabID);
    await openRptyForDirWithTabID(dirID, dirName, lastTabID);
  } else {
    showRptySessionPicker(dirID, dirName);
  }
}

// openRptyForDirWithTabID 使用指定 tab_id 连接远程 PTY
async function openRptyForDirWithTabID(dirID, dirName, tabID) {
  switchTab('rterm');
  await new Promise(r => setTimeout(r, 100));

  // 初始化 terminal
  initRptyTab();
  await new Promise(r => setTimeout(r, 50));

  // 更新 selector 值（如果有）
  const sel = document.getElementById('rpty-dir-select');
  if (sel) sel.value = dirID;

  rptyTabID = tabID;
  rptyDirID = dirID;
  updateRptyStatus('connecting');

  connectRPTY(tabID, dirID);
}

// connectRPTY 连接远程 PTY（复用原有逻辑，tabID 外部传入）
async function connectRPTY(tabID, dirID) {
  if (!tabID || !dirID) {
    if (rptyTerm) rptyTerm.writeln('\r\n\x1b[31m[错误] 缺少 tab_id 或 dir_id\x1b[0m\r\n');
    return;
  }

  if (rptyWs) { rptyWs.close(); rptyWs = null; }
  rptyReady = false;

  if (rptyTerm) { rptyTerm.clear(); rptyTerm.writeln('\x1b[36m[xworkbench] 正在连接...\x1b[0m\r\n'); }

  rptyWs = new WebSocket('ws://' + window.location.host + '/api/rpty?tab_id=' + encodeURIComponent(tabID) + '&dir_id=' + encodeURIComponent(dirID));
  rptyWs.binaryType = 'arraybuffer';

  rptyWs.onopen = () => {
    rptyReady = true;
    if (rptyTerm) {
      rptyTerm.writeln('\x1b[32m[xworkbench] 连接就绪\x1b[0m\r\n');
      // WS 打开后，延迟一点确保 SSH 准备好，再发 resize 并更新边界线
      setTimeout(() => {
        if (rptyWs && rptyWs.readyState === WebSocket.OPEN) {
          rptyWs.send('resize,' + rptyTerm.cols + ',' + rptyTerm.rows);
        }
        updateRptyBoundaryLine();
      }, 100);
    }
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
  if (rptyTerm) {
    // SSH 关闭了 ECHO（服务器不回显），前端负责本地 echo。
    // onData 回调在数据发往 SSH 之前先写到终端（打字立即可见）。
    rptyTerm.onData(data => {
      if (rptyWs && rptyWs.readyState === WebSocket.OPEN) rptyWs.send(data);
      rptyTerm.write(data);
    });
    rptyTerm.onResize(({ cols, rows }) => {
      if (rptyWs && rptyWs.readyState === WebSocket.OPEN) {
        rptyWs.send('resize,' + cols + ',' + rows);
      }
      if (rptyReady) updateRptyBoundaryLine();
    });
  }
}

// disconnectRPTY 断开当前连接。
function disconnectRPTY() {
  if (rptyWs) { rptyWs.close(); rptyWs = null; }
  rptyReady = false;
  if (rptyTerm) { rptyTerm.clear(); rptyTerm.writeln('\x1b[36m[xworkbench] 远程终端\x1b[0m 已断开\r\n'); }
  updateRptyStatus('disconnected');
  hideRptyBoundaryLine();
}

// updateRptyBoundaryLine 在终端容器内显示边界红线（absolute 相对 .terminal-wrap，切到其他 Tab 自动隐藏）
// 基于终端实际列数（rptyTerm.cols）计算，而非硬编码值
function updateRptyBoundaryLine() {
  let line = document.getElementById('rpty-boundary');
  const container = document.getElementById('rpty-container');
  if (!container || !rptyTerm) return;
  const wrap = container.closest('.terminal-wrap');
  if (!wrap) return;
  if (!line) {
    line = document.createElement('div');
    line.id = 'rpty-boundary';
    wrap.appendChild(line);
  }
  const wrapRect = wrap.getBoundingClientRect();
  const containerRect = container.getBoundingClientRect();
  const dims = rptyTerm._core?._renderService?.dimensions?.css?.cell;
  const cellW = dims?.width || 8;
  // 使用终端实际列数（rptyTerm.cols），而非硬编码的 RPTY_COLS
  const actualCols = rptyTerm.cols;
  // 边界线相对 .terminal-wrap 的 left = 容器距 wrap 左边的偏移 + cellW * 列数
  const boundaryX = (containerRect.left - wrapRect.left) + cellW * actualCols;
  line.style.left = boundaryX + 'px';
  // 设置 data-cols 属性，用于 ::before 伪元素显示列数标签
  line.setAttribute('data-cols', actualCols);
  line.classList.add('visible');

  // 同时更新 header 中的列数显示
  const colsDisplay = document.getElementById('rpty-cols-display');
  if (colsDisplay) {
    colsDisplay.textContent = actualCols + ' 列 × ' + rptyTerm.rows + ' 行';
  }
}

function hideRptyBoundaryLine() {
  const line = document.getElementById('rpty-boundary');
  if (line) line.classList.remove('visible');
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

// initRptyTab 初始化远程终端 tab（switchTab 时调用）。
function initRptyTab() {
  if (rptyTerm) return;
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

    // 动态计算容器实际能容纳的列数（基于容器宽度 / 字符宽度）
    const dims = rptyTerm._core?._renderService?.dimensions?.css?.cell;
    const cellW = dims?.width || 8;
    const cellH = dims?.height || 20;
    const containerW = container.clientWidth || 800;
    const containerH = container.clientHeight || 500;
    // 计算实际能容纳的列数和行数，留 8px 右内边距余量
    const RPTY_COLS = Math.max(80, Math.floor((containerW - 16) / cellW));
    const RPTY_ROWS = Math.max(24, Math.floor((containerH - 16) / cellH));

    // 强制 resize 到计算出的实际列数
    rptyTerm.resize(RPTY_COLS, RPTY_ROWS);

    // 动态设 CSS var，用于 ::after 边界线定位
    const wrap = container.closest('.terminal-wrap');
    if (wrap) {
      wrap.style.setProperty('--rpty-cell-w', cellW + 'px');
      wrap.style.setProperty('--rpty-cols', RPTY_COLS);
    }

    // ResizeObserver 监听容器变化，动态 fit 并通知后端
    const resizeObserver = new ResizeObserver(() => {
      // 重新计算容器能容纳的列数
      const newDims = rptyTerm._core?._renderService?.dimensions?.css?.cell;
      const newCellW = newDims?.width || cellW;
      const newCellH = newDims?.height || cellH;
      const newContainerW = container.clientWidth;
      const newContainerH = container.clientHeight;
      const newCols = Math.max(80, Math.floor((newContainerW - 16) / newCellW));
      const newRows = Math.max(24, Math.floor((newContainerH - 16) / newCellH));

      // 仅在列数变化时 resize（避免频繁重绘）
      if (newCols !== RPTY_COLS || newRows !== RPTY_ROWS) {
        rptyTerm.resize(newCols, newRows);
        // 更新 wrap CSS var
        const w = container.closest('.terminal-wrap');
        if (w) {
          w.style.setProperty('--rpty-cell-w', newCellW + 'px');
          w.style.setProperty('--rpty-cols', newCols);
        }
      }

      if (rptyWs && rptyWs.readyState === WebSocket.OPEN) {
        rptyWs.send('resize,' + rptyTerm.cols + ',' + rptyTerm.rows);
      }
      if (rptyReady) updateRptyBoundaryLine();
    });
    resizeObserver.observe(container);

    rptyTerm.writeln('\x1b[36m[xworkbench] 远程终端\x1b[0m 请选择目录后点击"连接"\r\n');
    rptyTerm.writeln('\x1b[2m提示：在侧边栏选择一个远程目录（🌐），点击终端图标选择"Web 内嵌终端"\x1b[0m\r\n');
  });
}

// loadRptyDirList 加载远程目录列表到 select。
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
  } catch (e) { console.error('[loadRptyDirList]', e); }
}

function onRptyDirChange(dirID) {
  const btn = document.getElementById('rpty-connect-btn');
  if (btn) btn.disabled = !dirID;
}

async function onRptyConnect() {
  const sel = document.getElementById('rpty-dir-select');
  const dirID = sel ? sel.value : '';
  if (!dirID) {
    if (rptyTerm) rptyTerm.writeln('\x1b[31m[错误] 请先选择一个远程目录\x1b[0m\r\n');
    return;
  }
  const opt = sel.options[sel.selectedIndex];
  const dirName = opt ? opt.text : dirID;
  await openRptyForDir(dirID, dirName);
}

function initRptyTabOnFirstVisit() {
  loadRptyDirList();
  initRptyTab();
}

// 暴露给 widgets.js 的 dir shortcut 按钮
window.openRptyForDir = openRptyForDir;

// ===== Helpers =====

function esc(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\"/g,'&quot;');
}

function formatRelTime(ts) {
  const diff = Date.now() - ts;
  if (diff < 60000) return '刚刚';
  if (diff < 3600000) return Math.floor(diff/60000) + '分钟前';
  if (diff < 86400000) return Math.floor(diff/3600000) + '小时前';
  if (diff < 604800000) return Math.floor(diff/86400000) + '天前';
  return new Date(ts).toLocaleDateString('zh-CN');
}