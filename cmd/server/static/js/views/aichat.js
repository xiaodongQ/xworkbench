// aichat.js — AI Chat view with function calling
(function() {
  'use strict';

  const SESSION_KEY = 'sf_aichat_sessions';
  const LAST_KEY = 'sf_aichat_last_session';

  function getSessions() {
    try { return JSON.parse(localStorage.getItem(SESSION_KEY) || '[]'); } catch { return []; }
  }
  function saveSessions(s) { localStorage.setItem(SESSION_KEY, JSON.stringify(s)); }
  function getLastId() { return localStorage.getItem(LAST_KEY); }
  function setLastId(id) { localStorage.setItem(LAST_KEY, id); }

  // ── PTY terminal state ─────────────────────────────────────
  // eslint-disable-next-line no-var
  var term = null, ptyWs = null, termReady = false;
  // eslint-disable-next-line no-var
  var currentSession = null;

  // ── Public: render into #aichat-root ────────────────────────
  window.renderAICheat = function(root) {
    const sessions = getSessions();
    const activeId = getLastId();
    const active = sessions.find(s => s.id === activeId) || sessions[0] || null;

    root.innerHTML = `
      <div class="aichat-root">
        <aside class="aichat-sidebar">
          <div class="aichat-sidebar-header">
            <button class="btn btn-primary btn-sm" id="aichat-new-btn">+ 新建对话</button>
          </div>
          <div class="aichat-session-list" id="aichat-session-list"></div>
        </aside>
        <main class="aichat-main">
          <div class="aichat-toolbar">
            <span class="aichat-toolbar-title">AI对话助手</span>
            <button class="btn-icon" id="aichat-toggle-term" title="网页终端（体验功能）" aria-label="toggle terminal">⌨</button>
            <button class="btn-icon" id="aichat-open-config" title="AI对话助手配置" aria-label="open config">⚙</button>
          </div>

          <div class="aichat-chat-region" id="aichat-chat-region">
            <div class="aichat-messages" id="aichat-messages"></div>
            <div class="aichat-input-area">
              <textarea id="aichat-input" placeholder="发送消息，AI 将通过工具帮你操作任务、目录、经验库..." rows="3"></textarea>
              <div class="aichat-input-row">
                <button class="btn btn-secondary btn-sm" id="aichat-clear-btn">清空</button>
                <button class="btn btn-primary" id="aichat-send-btn">发送</button>
              </div>
            </div>
          </div>

          <div class="aichat-term-region hidden" id="aichat-term-region">
            <div class="resize-handle-h" id="aichat-term-resizer"></div>
            <div class="terminal-wrap" style="margin-bottom:0;border-radius:0">
              <div class="terminal-header">
                <div class="terminal-dot red"></div>
                <div class="terminal-dot yellow"></div>
                <div class="terminal-dot green"></div>
                <div class="terminal-title" id="shell-term-title">网页终端</div>
                <div style="margin-left:auto;display:flex;align-items:center;gap:6px;position:relative">
                  <span style="font-size:11px;color:var(--text-secondary)">CLI:</span>
                  <div id="cli-display" style="font-size:12px;padding:3px 8px;border-radius:4px;border:1px solid #475569;background:#0f172a;color:#e2e8f0;min-width:80px;text-align:center">-</div>
                  <select id="cli-selector" onchange="onCliChange(this.value)" style="position:absolute;opacity:0;width:100%;height:100%;left:0;top:0;cursor:pointer">
                    <option value="claude">claude</option>
                    <option value="cbc">cbc / codebuddy</option>
                    <option value="shell">shell</option>
                  </select>
                </div>
                <button class="btn-icon" id="aichat-close-term" title="关闭终端" aria-label="close terminal" style="margin-left:6px">✕</button>
              </div>
              <div id="shell-terminal" tabindex="0"></div>
            </div>
          </div>
        </main>
      </div>

      <div class="modal-backdrop hidden" id="aichat-config-modal">
        <div class="modal-card">
          <div class="modal-header">
            <h3>AI对话助手配置</h3>
            <button class="btn-icon" id="aichat-config-close" aria-label="close">✕</button>
          </div>
          <div class="modal-body" id="aichat-config-body">
            ${configPanelHTML()}
          </div>
        </div>
      </div>
    `;

    bindEvents(root);
    if (active) renderMessages(active.messages || []);
    else renderMessages([]);
    renderSessionList(root);
  };

  function configPanelHTML() {
    return `<div class="aichat-config">
      <h3>AI对话助手配置</h3>
      <p class="aichat-config-hint">支持 Anthropic / OpenAI 双协议共存，填好后用顶部切换选择使用哪套。</p>
      <div class="cfg-model-warning" id="cfg-model-warning" style="display:none;background:#fef3c7;border:1px solid #f59e0b;border-radius:6px;padding:10px 12px;margin-bottom:12px;font-size:13px;color:#92400e">
        ⚠️ 当前未配置模型，请先在下方填写 API Key 和 Model 后保存。
      </div>

      <!-- 当前激活 Provider -->
      <div class="form-group" id="cfg-provider-row" style="background:var(--hover);border:1px solid var(--border);border-radius:8px;padding:10px 12px;margin-bottom:12px">
        <label style="font-size:12px;font-weight:600;margin-bottom:6px;display:block;color:var(--text)">当前使用</label>
        <div style="display:flex;align-items:center;gap:8px">
          <select id="cfg-provider" onchange="onCfgProviderChange()" style="font-size:13px;padding:4px 8px;background:var(--card);border:1px solid var(--border);border-radius:4px">
            <option value="anthropic">Anthropic</option>
            <option value="openai">OpenAI</option>
          </select>
          <span id="cfg-provider-hint" style="font-size:12px"></span>
        </div>
      </div>

      <!-- Anthropic + OpenAI 并排 -->
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">
        <div class="provider-config-block anthropic" id="cfg-block-anthropic">
          <div class="provider-config-header">Anthropic</div>
          <div class="form-group"><label style="font-size:11px">API URL</label>
            <input type="text" id="cfg-anthropic-url" placeholder="留空用官方默认" style="font-size:12px;padding:4px 6px" />
          </div>
          <div class="form-group"><label style="font-size:11px">API Key</label>
            <input type="password" id="cfg-anthropic-key" placeholder="sk-ant-..." style="font-size:12px;padding:4px 6px" />
          </div>
          <div class="form-group"><label style="font-size:11px">Model</label>
            <input type="text" id="cfg-anthropic-model" placeholder="claude-sonnet-4-20250514" style="font-size:12px;padding:4px 6px" />
          </div>
        </div>

        <div class="provider-config-block openai" id="cfg-block-openai">
          <div class="provider-config-header">OpenAI</div>
          <div class="form-group"><label style="font-size:11px">API URL</label>
            <input type="text" id="cfg-openai-url" placeholder="留空用官方默认" style="font-size:12px;padding:4px 6px" />
          </div>
          <div class="form-group"><label style="font-size:11px">API Key</label>
            <input type="password" id="cfg-openai-key" placeholder="sk-..." style="font-size:12px;padding:4px 6px" />
          </div>
          <div class="form-group"><label style="font-size:11px">Model</label>
            <input type="text" id="cfg-openai-model" placeholder="gpt-4o" style="font-size:12px;padding:4px 6px" />
          </div>
        </div>
      </div>

      <div class="form-actions" style="position:relative">
        <button class="btn btn-secondary" id="cfg-test-btn" onmouseenter="showTestTip()" onmouseleave="hideTestTip()">测试连接</button>
        <button class="btn btn-primary" id="cfg-save-btn">保存配置</button>
        <div id="cfg-test-tip" style="display:none;position:absolute;bottom:100%;left:0;margin-bottom:6px;background:#1e293b;color:#e2e8f0;font-size:11px;padding:6px 10px;border-radius:6px;white-space:nowrap;z-index:10;box-shadow:0 2px 8px rgba(0,0,0,0.25)"></div>
      </div>
      <div class="cfg-status" id="cfg-status"></div>
    </div>`;
  }

  // 显示测试按钮 tooltip：当前选中 provider + model + key 状态
  window.showTestTip = function() {
    const root = document.getElementById('aichat-config-body');
    if (!root) return;
    const provider = root.querySelector('#cfg-provider').value;
    const model = (provider === 'openai'
      ? root.querySelector('#cfg-openai-model')
      : root.querySelector('#cfg-anthropic-model'))?.value.trim();
    const keyEl = (provider === 'openai'
      ? root.querySelector('#cfg-openai-key')
      : root.querySelector('#cfg-anthropic-key'));
    const hasKey = !!(keyEl?.value || (keyEl?.placeholder?.startsWith('已配置')));
    const tip = root.querySelector('#cfg-test-tip');
    if (!tip) return;
    const label = provider === 'openai' ? 'OpenAI' : 'Anthropic';
    tip.innerHTML = `<b>${label}</b> · ${model || '<span style="color:#f59e0b">缺模型</span>'} · ${hasKey ? '✅ Key已填' : '<span style="color:#f59e0b">缺Key</span>'}`;
    tip.style.display = 'block';
  };
  window.hideTestTip = function() {
    const root = document.getElementById('aichat-config-body');
    if (!root) return;
    const tip = root.querySelector('#cfg-test-tip');
    if (tip) tip.style.display = 'none';
  };

  window.onCfgProviderChange = function() {
    // 切换 active_provider，不改变页面可见性（两套始终可见）
    const provider = document.getElementById('cfg-provider').value;
    document.querySelector('#cfg-active-provider-display')?.setAttribute('data-provider', provider);
    // 根据当前表单值更新右侧状态文字颜色
    const model = (provider === 'openai'
      ? document.getElementById('cfg-openai-model')
      : document.getElementById('cfg-anthropic-model'))?.value.trim();
    const keyEl = (provider === 'openai'
      ? document.getElementById('cfg-openai-key')
      : document.getElementById('cfg-anthropic-key'));
    const hasKey = !!(keyEl?.value.trim() || (keyEl?.placeholder?.startsWith('已配置')));
    const hint = document.querySelector('#cfg-provider-hint');
    if (hint) {
      if (hasKey && model) {
        hint.textContent = '✅ 已配置'; hint.style.color = '#16a34a';
      } else if (hasKey) {
        hint.textContent = '⚠️ 缺少模型'; hint.style.color = '#ca8a04';
      } else {
        hint.textContent = '❌ 未配置'; hint.style.color = '#dc2626';
      }
    }
  };

  // ── Events ─────────────────────────────────────────────────
  function bindEvents(root) {
    // 1) Terminal 抽屉开关
    const termBtn   = root.querySelector('#aichat-toggle-term');
    const closeBtn  = root.querySelector('#aichat-close-term');
    const termRegion = () => root.querySelector('#aichat-term-region');
    const openTerm = () => {
      const r = termRegion(); if (!r) return;
      r.classList.remove('hidden');
      termBtn?.classList.add('active');
      ensureTerminalHeight();
      initShellTerminal(); // 已存在的惰性初始化（幂等：if (term) return）
    };
    const closeTerm = () => {
      const r = termRegion(); if (!r) return;
      r.classList.add('hidden');
      termBtn?.classList.remove('active');
      // xterm + WS 实例保留（体验功能定位：复用长会话）
    };
    termBtn?.addEventListener('click', () => {
      termRegion().classList.contains('hidden') ? openTerm() : closeTerm();
    });
    closeBtn?.addEventListener('click', closeTerm);

    // 2) 配置 modal
    const cfgBtn   = root.querySelector('#aichat-open-config');
    const cfgModal = root.querySelector('#aichat-config-modal');
    const cfgClose = root.querySelector('#aichat-config-close');
    cfgBtn?.addEventListener('click', () => {
      cfgModal.classList.remove('hidden');
      loadConfig(root);
    });
    const closeCfg = () => cfgModal?.classList.add('hidden');
    cfgClose?.addEventListener('click', closeCfg);
    cfgModal?.addEventListener('click', e => { if (e.target === cfgModal) closeCfg(); });
    document.addEventListener('keydown', e => {
      if (e.key === 'Escape' && cfgModal && !cfgModal.classList.contains('hidden')) closeCfg();
    });

    // 3) 其余原 handler（聊天 / 会话 / 清空 / 配置保存）
    root.querySelector('#aichat-new-btn').addEventListener('click', () => createSession(root));
    root.querySelector('#aichat-send-btn').addEventListener('click', () => sendMessage(root));
    root.querySelector('#aichat-input').addEventListener('keydown', e => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(root); }
    });
    root.querySelector('#aichat-clear-btn').addEventListener('click', () => {
      root.querySelector('#aichat-messages').innerHTML = '';
      root.querySelector('#aichat-input').value = '';
    });
    root.querySelector('#cfg-save-btn')?.addEventListener('click', () => saveConfig(root));
    root.querySelector('#cfg-test-btn')?.addEventListener('click', () => testConfig(root));

    // 4) 终端抽屉高度拖拽
    initTermResizer();
  }

  // ── Terminal 抽屉高度持久化 ─────────────────────────────────
  const TERM_H_KEY = 'sf_aichat_term_h';
  const TERM_H_DEFAULT = 280;

  function ensureTerminalHeight() {
    const r = document.getElementById('aichat-term-region');
    if (!r) return;
    const h = parseInt(localStorage.getItem(TERM_H_KEY) || TERM_H_DEFAULT, 10);
    r.style.height = h + 'px';
  }

  function initTermResizer() {
    const handle = document.getElementById('aichat-term-resizer');
    const region = document.getElementById('aichat-term-region');
    if (!handle || !region) return;
    ensureTerminalHeight();
    handle.addEventListener('mousedown', e => {
      e.preventDefault();
      document.body.style.cursor = 'row-resize';
      const startY = e.clientY;
      const startH = region.getBoundingClientRect().height;
      const onMove = ev => {
        const h = Math.max(120, Math.min(window.innerHeight - 200, startH + (startY - ev.clientY)));
        region.style.height = h + 'px';
        // 抽屉高度变化时同步 fit（shell-terminal 容器宽高随之变化）
        const termContainer = document.getElementById('shell-terminal');
        if (termContainer?._fitAddon) {
          termContainer._fitAddon.fit();
          if (ptyWs && ptyWs.readyState === 1) {
            ptyWs.send('resize,' + term.cols + ',' + term.rows);
          }
        }
      };
      const onUp = () => {
        document.removeEventListener('mousemove', onMove);
        document.removeEventListener('mouseup', onUp);
        document.body.style.cursor = '';
        localStorage.setItem(TERM_H_KEY, String(Math.round(region.getBoundingClientRect().height)));
      };
      document.addEventListener('mousemove', onMove);
      document.addEventListener('mouseup', onUp);
    });
  }

  // ── Sessions ─────────────────────────────────────────────────
  function createSession(root) {
    const sessions = getSessions();
    const s = { id: 'sess_' + Date.now(), title: '新对话', messages: [], createdAt: new Date().toISOString() };
    sessions.unshift(s);
    saveSessions(sessions);
    setLastId(s.id);
    renderSessionList(root);
    switchToSession(root, s.id);
  }

  function renderSessionList(root) {
    const sessions = getSessions();
    const lastId = getLastId();
    const list = root.querySelector('#aichat-session-list');
    if (!list) return;
    if (!sessions.length) { list.innerHTML = '<div class="empty-hint">无会话</div>'; return; }
    list.innerHTML = sessions.map(s => `
      <div class="session-item ${s.id === lastId ? 'active' : ''}" data-id="${s.id}">
        <span class="session-title">${escHtml(s.title || '新对话')}</span>
        <button class="session-del" data-id="${s.id}">✕</button>
      </div>`).join('');
    list.querySelectorAll('.session-item').forEach(item => {
      item.addEventListener('click', e => {
        if (e.target.classList.contains('session-del')) { e.stopPropagation(); deleteSession(root, item.dataset.id); }
        else { switchToSession(root, item.dataset.id); }
      });
    });
  }

  function switchToSession(root, id) {
    setLastId(id);
    const sessions = getSessions();
    const s = sessions.find(s => s.id === id);
    renderSessionList(root);
    renderMessages(s ? s.messages : []);
  }

  function deleteSession(root, id) {
    let sessions = getSessions().filter(s => s.id !== id);
    saveSessions(sessions);
    renderSessionList(root);
    if (sessions.length) switchToSession(root, sessions[0].id);
    else renderMessages([]);
  }

  // ── Messages ─────────────────────────────────────────────────
  function renderMessages(messages) {
    const container = document.getElementById('aichat-messages');
    if (!container) return;
    if (!messages.length) { container.innerHTML = welcomeHTML(); return; }
    container.innerHTML = messages.map(m => `
      <div class="aichat-msg aichat-msg-${escHtml(m.role)}">
        <div class="aichat-msg-role">${m.role === 'user' ? '你' : 'AI'}</div>
        <div class="aichat-msg-content">${formatContent(m.content)}</div>
      </div>`).join('');
    container.scrollTop = container.scrollHeight;
  }

  // welcomeHTML: 无消息时展示欢迎面板（4 张能力卡 + 提示）。
  // 能力分类来自 cmd/server/ai_tools.go 的真实工具。
  function welcomeHTML() {
    return `<div class="aichat-welcome">
      <div class="aichat-welcome-title">👋 你好，我是 AI 对话助手</div>
      <div class="aichat-welcome-sub">任务管理 · 目录快捷 · 经验库 · CLI 会话</div>
      <div class="aichat-welcome-grid">
        <div class="aichat-welcome-card">
          <div class="aichat-welcome-card-icon">📋</div>
          <div class="aichat-welcome-card-title">任务管理</div>
          <div class="aichat-welcome-card-desc">创建、查询、执行任务</div>
        </div>
        <div class="aichat-welcome-card">
          <div class="aichat-welcome-card-icon">📁</div>
          <div class="aichat-welcome-card-title">目录快捷</div>
          <div class="aichat-welcome-card-desc">本地与远程目录访问</div>
        </div>
        <div class="aichat-welcome-card">
          <div class="aichat-welcome-card-icon">💡</div>
          <div class="aichat-welcome-card-title">经验库</div>
          <div class="aichat-welcome-card-desc">搜索已有经验与知识</div>
        </div>
        <div class="aichat-welcome-card">
          <div class="aichat-welcome-card-icon">🛠️</div>
          <div class="aichat-welcome-card-title">CLI 会话</div>
          <div class="aichat-welcome-card-desc">Claude / CBC 交互式会话</div>
        </div>
      </div>
      <div class="aichat-welcome-hint">在下方输入框开始对话 ↓</div>
    </div>`;
  }

  function formatContent(content) {
    if (!content) return '';
    return escHtml(content).replace(/\n/g, '<br>');
  }

  function escHtml(s) {
    if (!s) return '';
    return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  // ── Send ─────────────────────────────────────────────────────
  async function sendMessage(root) {
    const input = root.querySelector('#aichat-input');
    const text = input.value.trim();
    if (!text) return;
    input.value = '';

    let sessions = getSessions();
    let id = getLastId();
    let session = sessions.find(s => s.id === id);
    if (!session) { createSession(root); sessions = getSessions(); id = getLastId(); session = sessions.find(s => s.id === id); }

    session.messages.push({ role: 'user', content: text });
    if (session.messages.length === 1) { session.title = text.slice(0, 30); }
    saveSessions(sessions);
    renderMessages(session.messages);
    renderSessionList(root);

    const msgContainer = document.getElementById('aichat-messages');
    const typing = document.createElement('div');
    typing.className = 'aichat-msg aichat-msg-assistant';
    typing.innerHTML = '<div class="aichat-msg-role">AI</div><div class="aichat-msg-content">思考中...</div>';
    msgContainer.appendChild(typing);
    msgContainer.scrollTop = msgContainer.scrollHeight;

    try {
      const resp = await fetch('/api/ai/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ messages: session.messages })
      });
      if (!resp.ok) {
        let errMsg = resp.statusText;
        try {
          const err = await resp.json().catch(() => null);
          if (err && err.error) errMsg = err.error;
        } catch {}
        throw new Error(errMsg);
      }
      const data = await resp.json();
      session.messages.push({ role: 'assistant', content: data.message?.content || JSON.stringify(data) });

      // Refresh sidebar widgets if AI tool modified them (same as config.js import)
      if (data.refresh_widgets && data.refresh_widgets.length > 0) {
        if (typeof loadLinks === 'function' && data.refresh_widgets.includes('links')) loadLinks();
        if (typeof loadDirs === 'function' && data.refresh_widgets.includes('dirs')) loadDirs();
        if (typeof loadTodos === 'function' && data.refresh_widgets.includes('todos')) loadTodos();
      }
    } catch (err) {
      session.messages.push({ role: 'assistant', content: '❌ 错误: ' + err.message });
    }

    saveSessions(sessions);
    typing.remove();
    renderMessages(session.messages);
  }

  // ── Config ────────────────────────────────────────────────────
  async function loadConfig(root) {
    try {
      const r = await fetch('/api/ai/config');
      const d = await r.json();
      const cfg = d.ai_chat || {};
      const provider = cfg.active_provider || 'anthropic';
      root.querySelector('#cfg-provider').value = provider;

      // Anthropic 配置
      const ant = cfg.anthropic || {};
      root.querySelector('#cfg-anthropic-url').value = ant.base_url || '';
      root.querySelector('#cfg-anthropic-model').value = ant.model || '';
      const antKeyEl = root.querySelector('#cfg-anthropic-key');
      if (antKeyEl && ant.api_key && ant.api_key !== '') {
        antKeyEl.placeholder = '已配置（修改请重新输入）';
      }

      // OpenAI 配置
      const oai = cfg.openai || {};
      root.querySelector('#cfg-openai-url').value = oai.base_url || '';
      root.querySelector('#cfg-openai-model').value = oai.model || '';
      const oaiKeyEl = root.querySelector('#cfg-openai-key');
      if (oaiKeyEl && oai.api_key && oai.api_key !== '') {
        oaiKeyEl.placeholder = '已配置（修改请重新输入）';
      }

      // 当前 provider 未配置模型时显示警告
      const active = provider === 'openai' ? oai : ant;
      const warn = root.querySelector('#cfg-model-warning');
      if (warn) warn.style.display = (!active.model || !active.api_key) ? 'block' : 'none';
      // 当前使用右侧状态文字颜色
      const hint = root.querySelector('#cfg-provider-hint');
      if (hint) {
        const hasKey = !!active.api_key;
        const hasModel = !!active.model;
        if (hasKey && hasModel) {
          hint.textContent = '✅ 已配置'; hint.style.color = '#16a34a';
        } else if (hasKey) {
          hint.textContent = '⚠️ 缺少模型'; hint.style.color = '#ca8a04';
        } else {
          hint.textContent = '❌ 未配置'; hint.style.color = '#dc2626';
        }
      }
    } catch {}
  }

  async function saveConfig(root) {
    const status = root.querySelector('#cfg-status');
    const activeProvider = root.querySelector('#cfg-provider').value;

    // 收集两个 Provider 的配置
    const antModel = root.querySelector('#cfg-anthropic-model').value.trim();
    const antURL = root.querySelector('#cfg-anthropic-url').value.trim();
    const antKey = root.querySelector('#cfg-anthropic-key').value;
    const oaiModel = root.querySelector('#cfg-openai-model').value.trim();
    const oaiURL = root.querySelector('#cfg-openai-url').value.trim();
    const oaiKey = root.querySelector('#cfg-openai-key').value;

    // 保存主配置（不含 api_key）
    try {
      await fetch('/api/ai/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          active_provider: activeProvider,
          anthropic: { model: antModel, base_url: antURL },
          openai: { model: oaiModel, base_url: oaiURL },
        })
      });
    } catch (err) {
      status.textContent = '❌ 保存失败: ' + err.message; status.style.color = 'red'; return;
    }

    // 单独保存各 Provider 的 api_key
    const saves = [];
    if (antKey) saves.push(fetch('/api/ai/config/key', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider: 'anthropic', api_key: antKey })
    }));
    if (oaiKey) saves.push(fetch('/api/ai/config/key', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider: 'openai', api_key: oaiKey })
    }));
    try {
      await Promise.all(saves);
      status.textContent = '✅ 配置已保存'; status.style.color = 'green';
      clearTimeout(status._timer);
      status._timer = setTimeout(() => { status.textContent = ''; }, 3000);
      const warn = root.querySelector('#cfg-model-warning');
      if (warn) warn.style.display = 'none';
    } catch (err) {
      status.textContent = '❌ 保存 Key 失败: ' + err.message; status.style.color = 'red';
    }
  }

  async function testConfig(root) {
    const status = root.querySelector('#cfg-status');
    clearTimeout(status._timer);
    const provider = root.querySelector('#cfg-provider').value;
    status.textContent = '🔬 测试连接中...'; status.style.color = 'blue';
    try {
      const r = await fetch('/api/ai/config/test?provider=' + encodeURIComponent(provider), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}) // 空 body，后端用已保存的 config
      });
      if (r.ok) {
        status.textContent = '✅ 连接成功'; status.style.color = '#16a34a';
      } else {
        const e = await r.json().catch(() => ({}));
        status.textContent = '❌ ' + (e.error || r.statusText); status.style.color = '#dc2626';
      }
    } catch (err) {
      status.textContent = '❌ ' + err.message; status.style.color = '#dc2626';
    }
    status._timer = setTimeout(() => { status.textContent = ''; }, 4000);
  }


  // ── Local Shell PTY ─────────────────────────────────────────
  function initShellTerminal() {
    if (term) return; // already initialized
    const termContainer = document.getElementById('shell-terminal');
    if (!termContainer) return;

    requestAnimationFrame(() => {
      const fitAddon = new FitAddon.FitAddon();
      term = new Terminal({
        fontFamily: "'SF Mono', 'Fira Code', 'Consolas', monospace",
        fontSize: 13,
        theme: { background: '#0f172a', foreground: '#e2e8f0', cursor: '#22d3ee' },
        cursorBlink: true,
        scrollback: 10000,
      });
      term.loadAddon(fitAddon);
      term.open(termContainer);
      termContainer._fitAddon = fitAddon; // 供抽屉拖动时引用
      fitAddon.fit();

      const sessId = getLastId() || ('sess_' + Date.now());
      const wsUrl = 'ws://' + window.location.host + '/api/pty?session_id=' + encodeURIComponent(sessId);

      ptyWs = new WebSocket(wsUrl);
      ptyWs.binaryType = 'arraybuffer';

      ptyWs.onopen = () => {
        termReady = true;
        term.writeln('\x1b[32m[xworkbench] 网页终端已连接\x1b[0m\r\n');
        if (typeof loadCliSetting === 'function') loadCliSetting();
        // 连接建立后立即发送正确尺寸给后端
        ptyWs.send('resize,' + term.cols + ',' + term.rows);
      };
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

      term.onData(data => {
        if (ptyWs && ptyWs.readyState === 1) ptyWs.send(data);
      });
      term.onResize(() => {
        if (ptyWs && ptyWs.readyState === 1) {
          ptyWs.send('resize,' + term.cols + ',' + term.rows);
        }
      });

      // ResizeObserver 监听容器变化，动态 fit 并通知后端
      const resizeObserver = new ResizeObserver(() => {
        fitAddon.fit();
        if (ptyWs && ptyWs.readyState === 1) {
          ptyWs.send('resize,' + term.cols + ',' + term.rows);
        }
      });
      resizeObserver.observe(termContainer);
    });
  }

  // loadCliSetting: fetch current aichat_default_cli from config and update selector/display
  window.loadCliSetting = async function() {
    try {
      const r = await fetch('/api/config');
      const d = await r.json();
      const cli = d.aichat_default_cli || 'claude';
      const sel = document.getElementById('cli-selector');
      const disp = document.getElementById('cli-display');
      if (sel) sel.value = cli;
      if (disp) disp.textContent = cli;
    } catch {}
  };

  window.onCliChange = async function(value) {
    const disp = document.getElementById('cli-display');
    if (disp) disp.textContent = value;
    try {
      await fetch('/api/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ aichat_default_cli: value })
      });
    } catch {}
  };

})();