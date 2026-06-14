// AI Chat Tab：多 Tab PTY 终端（每 Tab 独立 WebSocket + xterm.js）
// 最多 5 个 Tab；任务完成或需要授权时 Tab 标红点
// 依赖 api.js

const MAX_TABS = 5;

// tabRegistry: id -> { id, name, term, ws, needsAuth, wsConnected }
let tabRegistry = {};
let activeTabId = null;
let tabCounter = 0;

// createTab 新建一个 Tab，立即连接 PTY
async function createTab() {
  if (Object.keys(tabRegistry).length >= MAX_TABS) {
    alert('最多只能开 ' + MAX_TABS + ' 个 AI 对话 Tab');
    return;
  }
  tabCounter++;
  const id = 'tab-' + tabCounter;
  const tab = { id, name: 'Tab ' + tabCounter, term: null, ws: null, needsAuth: false, wsConnected: false };
  tabRegistry[id] = tab;
  await connectTab(tab);
  renderTabBar();
  switchToTab(id);
}

// connectTab 建立 WebSocket + xterm.js 连接
async function connectTab(tab) {
  // 创建 xterm.js 实例
  const term = new Terminal({
    fontFamily: "'SF Mono', 'Fira Code', 'Consolas', monospace",
    fontSize: 13,
    theme: { background: '#0f172a', foreground: '#e2e8f0', cursor: '#22d3ee' },
    cursorBlink: true,
    scrollback: 10000,
  });
  tab.term = term;

  // 清空并挂载 terminal 到容器
  const container = document.getElementById('terminal');
  container.innerHTML = '';
  term.open(container);

  // 建立 WebSocket，URL 带上 tab_id
  const wsUrl = 'ws://' + window.location.host + '/api/pty?tab_id=' + encodeURIComponent(tab.id);
  const ws = new WebSocket(wsUrl);
  tab.ws = ws;

  ws.binaryType = 'arraybuffer';
  ws.onopen = () => {
    tab.wsConnected = true;
    term.writeln('\x1b[32m[xworkbench] 连接就绪\x1b[0m\r\n');
    updateTabAuthState(tab.id, false);
  };

  ws.onmessage = (e) => {
    if (e.data instanceof ArrayBuffer) {
      term.write(new Uint8Array(e.data));
    } else {
      // 文本消息，可能是 JSON 控制消息
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === 'auth_required') {
          updateTabAuthState(tab.id, true);
        }
      } catch {
        term.write(e.data);
      }
    }
  };

  ws.onclose = () => {
    tab.wsConnected = false;
    term.writeln('\r\n\x1b[33m[连接已关闭]\x1b[0m\r\n');
  };
  ws.onerror = () => {
    term.writeln('\r\n\x1b[31m[WebSocket 错误]\x1b[0m\r\n');
  };

  // PTY 输入 → WS
  term.onData(data => { if (ws && ws.readyState === 1) ws.send(data); });

  // resize 事件
  term.onResize(({ cols, rows }) => {
    if (ws && ws.readyState === 1) {
      ws.send('resize,' + cols + ',' + rows);
    }
  });
}

// switchToTab 切换到指定 Tab
function switchToTab(id) {
  if (!tabRegistry[id]) return;
  activeTabId = id;

  // 卸载当前 terminal（隐藏，不 destroy）
  const container = document.getElementById('terminal');
  container.innerHTML = '';

  const tab = tabRegistry[id];
  if (tab.term) {
    tab.term.open(container);
  }

  renderTabBar();
}

// closeTab 关闭指定 Tab
function closeTab(id) {
  const tab = tabRegistry[id];
  if (!tab) return;

  if (tab.ws) {
    tab.ws.close();
    tab.ws = null;
  }
  if (tab.term) {
    tab.term.dispose();
    tab.term = null;
  }
  delete tabRegistry[id];

  // 如果关掉的是当前 Tab，切换到最后一个
  if (activeTabId === id) {
    const remaining = Object.keys(tabRegistry);
    if (remaining.length > 0) {
      switchToTab(remaining[remaining.length - 1]);
    } else {
      activeTabId = null;
      document.getElementById('terminal').innerHTML = '';
    }
  }
  renderTabBar();
}

// updateTabAuthState 更新 Tab 的 needsAuth 状态，刷新 tab bar
function updateTabAuthState(id, needsAuth) {
  if (!tabRegistry[id]) return;
  tabRegistry[id].needsAuth = needsAuth;
  renderTabBar();
}

// renderTabBar 渲染 Tab 栏
function renderTabBar() {
  const bar = document.getElementById('tab-bar');
  if (!bar) return;

  const tabs = Object.values(tabRegistry);
  bar.innerHTML = tabs.map(tab => {
    const isActive = tab.id === activeTabId;
    const needsAuthClass = tab.needsAuth ? ' tab-auth' : '';
    const activeClass = isActive ? ' tab-active' : '';
    return `<div class="tab-btn${activeClass}${needsAuthClass}" onclick="switchToTab('${tab.id}')" title="${esc(tab.name)}">
      <span class="tab-name">${esc(tab.name)}</span>
      <span class="tab-auth-dot" title="需要授权" style="display:${tab.needsAuth?'inline':'none'}">🔴</span>
      <span class="tab-close" onclick="event.stopPropagation();closeTab('${tab.id}')" title="关闭">×</span>
    </div>`;
  }).join('') + `<button class="tab-add-btn" onclick="createTab()" title="新建 Tab（最多${MAX_TABS}个）">+</button>`;
}

// initTerminal 首次初始化：创建第一个 Tab
async function initTerminal() {
  if (Object.keys(tabRegistry).length > 0) return;
  await createTab();
  if (typeof loadCliSetting === 'function') loadCliSetting();
}

// onCliChange 切换默认 CLI
async function onCliChange(value) {
  const disp = document.getElementById('cli-display');
  if (disp) disp.textContent = value;
  await fetchJSON('/api/settings/aichat_default_cli', {
    method: 'PUT',
    body: JSON.stringify({ value }),
  });
  const tab = tabRegistry[activeTabId];
  if (tab?.term) {
    tab.term.writeln('\r\n\x1b[33m[xworkbench] CLI 已切换为 ' + value + '，新建 Tab 时生效\x1b[0m\r\n');
  }
}

// loadCliSetting 恢复 CLI 选择
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