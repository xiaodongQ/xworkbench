// AI Chat Tab：xterm.js + WebSocket 接 /api/pty
// 会话持久化：session_id + resume_uuid 存 localStorage，支持选历史 / 新建对话
// 依赖 api.js

let term;
let ptyWs;
let termReady = false;
let currentSession = null; // 当前 SessionInfo

// localStorage 键
const AI_SESSION_KEY = 'sf_aichat_sessions';

// SessionInfo: { id, resumeUuid, title, createdAt }
function getStoredSessions() {
  try { return JSON.parse(localStorage.getItem(AI_SESSION_KEY) || '[]'); } catch { return []; }
}
function saveStoredSessions(sessions) { localStorage.setItem(AI_SESSION_KEY, JSON.stringify(sessions)); }

// 取最新有 resumeUuid 的会话（刷新后自动重连）
function getLastActiveSession() {
  return getStoredSessions().find(s => s.resumeUuid) || null;
}

function createNewSession() {
  const id = 'sess-' + Date.now() + '-' + Math.random().toString(36).slice(2, 8);
  const sess = { id, resumeUuid: '', title: '新对话', createdAt: Date.now() };
  const sessions = getStoredSessions();
  sessions.unshift(sess);
  if (sessions.length > 20) sessions.splice(20);
  saveStoredSessions(sessions);
  return sess;
}

// 用 resumeUuid 更新已有会话（首次 claude 输出后调用）
function linkResumeUuid(sessionId, resumeUuid) {
  const sessions = getStoredSessions();
  const s = sessions.find(x => x.id === sessionId);
  if (s && s.resumeUuid !== resumeUuid) {
    s.resumeUuid = resumeUuid;
    saveStoredSessions(sessions);
  }
}

// 用第一条人类消息更新标题
function updateSessionTitle(sessionId, firstMsg) {
  const sessions = getStoredSessions();
  const s = sessions.find(x => x.id === sessionId);
  if (s && s.title === '新对话' && firstMsg) {
    s.title = firstMsg.replace(/\n/g, ' ').trim().slice(0, 40) || '新对话';
    saveStoredSessions(sessions);
  }
}

// ===== 会话选择器 UI =====

function showSessionPicker() {
  const sessions = getStoredSessions();
  const itemsHtml = sessions.length === 0
    ? '<div style="color:var(--text-secondary);font-size:13px;text-align:center;padding:20px">暂无历史对话</div>'
    : sessions.map(s => `
        <div class="session-item" onclick="selectSession('${s.id}')" style="
          padding:10px 12px;border:1px solid var(--border);border-radius:8px;
          cursor:pointer;background:var(--card-bg);margin-bottom:6px;
          transition:border-color 0.15s">
          <div style="display:flex;align-items:center;justify-content:space-between">
            <span style="font-size:13px;font-weight:500">${esc(s.title)}</span>
            <span style="font-size:11px;color:var(--text-secondary)">${formatRelTime(s.createdAt)}</span>
          </div>
          ${s.resumeUuid
            ? '<div style="font-size:11px;color:#22c55e;margin-top:2px">✓ 会话已保存，可续接</div>'
            : '<div style="font-size:11px;color:var(--text-secondary);margin-top:2px">新会话</div>'}
        </div>`).join('');

  const html = `
    <div id="session-picker-overlay" style="
      position:fixed;inset:0;background:rgba(0,0,0,0.6);z-index:9999;
      display:flex;align-items:center;justify-content:center"
      onclick="if(event.target.id==='session-picker-overlay')closeSessionPicker()">
      <div style="
        background:var(--card);border:1px solid var(--border);border-radius:12px;
        padding:24px;width:440px;max-width:90vw;max-height:80vh;overflow-y:auto;box-shadow:0 20px 60px rgba(0,0,0,0.4)">
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:16px">
          <h3 style="margin:0;font-size:15px;font-weight:600">🤖 AI 对话</h3>
          <button onclick="handleNewSession()" style="
            background:var(--primary);color:#fff;border:none;border-radius:6px;
            padding:6px 14px;cursor:pointer;font-size:13px;font-weight:500">+ 新建对话</button>
        </div>
        <div id="session-list">${itemsHtml}</div>
        <div style="margin-top:12px;padding-top:12px;border-top:1px solid var(--border);display:flex;justify-content:flex-end">
          <button onclick="closeSessionPicker()" style="
            background:transparent;color:var(--text-secondary);border:1px solid var(--border);
            border-radius:6px;padding:5px 12px;cursor:pointer;font-size:12px">取消</button>
        </div>
      </div>
    </div>`;
  document.body.insertAdjacentHTML('beforeend', html);
}

function closeSessionPicker() {
  const el = document.getElementById('session-picker-overlay');
  if (el) el.remove();
}

function selectSession(sessionId) {
  closeSessionPicker();
  const sessions = getStoredSessions();
  currentSession = sessions.find(s => s.id === sessionId) || createNewSession();
  initTerminalWithSession(currentSession);
}

function handleNewSession() {
  closeSessionPicker();
  currentSession = createNewSession();
  initTerminalWithSession(currentSession);
}

// ===== 终端初始化 =====

function initTerminalWithSession(session) {
  if (term) { term.dispose(); term = null; }
  if (ptyWs) { ptyWs.close(); ptyWs = null; }
  termReady = false;

  const termContainer = document.getElementById('terminal');
  if (!termContainer) return;

  requestAnimationFrame(() => {
    term = new Terminal({
      fontFamily: "'SF Mono', 'Fira Code', 'Consolas', monospace",
      fontSize: 13,
      theme: { background: '#0f172a', foreground: '#e2e8f0', cursor: '#22d3ee' },
      cursorBlink: true,
      scrollback: 10000,
    });
    term.open(termContainer);

    // 构建 WS URL（带 session_id 和可选的 resume_uuid）
    let wsUrl = 'ws://' + window.location.host + '/api/pty?session_id=' + encodeURIComponent(session.id);
    if (session.resumeUuid) {
      wsUrl += '&resume_uuid=' + encodeURIComponent(session.resumeUuid);
    }

    ptyWs = new WebSocket(wsUrl);
    ptyWs.binaryType = 'arraybuffer';

    ptyWs.onopen = () => {
      termReady = true;
      if (session.resumeUuid) {
        term.writeln('\x1b[32m[xworkbench] 会话已恢复\x1b[0m\r\n');
      } else {
        term.writeln('\x1b[32m[xworkbench] 新对话开始\x1b[0m\r\n');
      }
      updateSessionTitleDisplay(session.title);
    };

    let firstUserInput = null;

    ptyWs.onmessage = (e) => {
      if (!termReady) return;
      if (e.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(e.data));
      } else {
        try {
          const msg = JSON.parse(e.data);
          if (msg.type === 'auth_required') {
            term.writeln('\r\n\x1b[33m[xworkbench] 需要授权: ' + (msg.extra || '') + '\x1b[0m\r\n');
          }
          // 尝试从 JSON 输出中解析 resume uuid（格式: "sessionId": "uuid-xxx"）
          if (msg.sessionId && currentSession && !currentSession.resumeUuid) {
            linkResumeUuid(currentSession.id, msg.sessionId);
            currentSession.resumeUuid = msg.sessionId;
          }
        } catch {
          term.write(e.data);
        }
      }
    };

    ptyWs.onclose = () => {
      term.writeln('\r\n\x1b[33m[连接已关闭]\x1b[0m');
      termReady = false;
    };
    ptyWs.onerror = () => {
      term.writeln('\r\n\x1b[31m[WebSocket 错误]\x1b[0m');
    };

    // PTY 输入 → WS
    term.onData(data => {
      if (firstUserInput === null && data.trim()) firstUserInput = data.trim();
      if (ptyWs && ptyWs.readyState === 1) ptyWs.send(data);
    });

    // PTY resize → WS
    term.onResize(() => {
      if (ptyWs && ptyWs.readyState === 1) {
        ptyWs.send('resize,' + term.cols + ',' + term.rows);
      }
    });

    if (typeof loadCliSetting === 'function') loadCliSetting();
  });
}

function updateSessionTitleDisplay(title) {
  const titleEl = document.querySelector('#page-aichat .page-title');
  if (titleEl) titleEl.textContent = '🤖 ' + (title || '新对话');
}

// initTerminal: 切换到 aichat tab 时调用
// 有上次会话 → 自动重连；无 → 弹选择器
function initTerminal() {
  if (term) return;
  const termContainer = document.getElementById('terminal');
  if (!termContainer) return;

  const lastSession = getLastActiveSession();
  if (lastSession) {
    currentSession = lastSession;
    initTerminalWithSession(currentSession);
  } else {
    showSessionPicker();
  }
}

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
  } catch(e) { console.error('[loadCliSetting]', e); }
}

// 暴露给 index.html 标题栏的"切换会话"按钮
window.showSessionPicker = showSessionPicker;

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