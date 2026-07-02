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
          <div class="aichat-subtabs">
            <button class="subtab active" data-tab="chat">AI 对话</button>
            <button class="subtab" data-tab="shell">本地 Shell</button>
            <button class="subtab" data-tab="config">⚙️ 配置</button>
          </div>
          <div class="aichat-panel" id="panel-chat">
            <div class="aichat-messages" id="aichat-messages"></div>
            <div class="aichat-input-area">
              <textarea id="aichat-input" placeholder="发送消息，AI 将通过工具帮你操作任务、目录、经验库..." rows="3"></textarea>
              <div class="aichat-input-row">
                <button class="btn btn-secondary btn-sm" id="aichat-clear-btn">清空</button>
                <button class="btn btn-primary" id="aichat-send-btn">发送</button>
              </div>
            </div>
          </div>
          <div class="aichat-panel hidden" id="panel-shell">
            <div class="aichat-shell-info">
              <p>本地 Claude/CBC Shell 入口。在「AI 对话」标签中发送自然语言请求，AI 会自动调用工具。</p>
              <p>直接执行命令 → <code>run_local_command</code>；启动交互式会话 → <code>start_local_shell</code></p>
            </div>
          </div>
          <div class="aichat-panel hidden" id="panel-config">
            ${configPanelHTML()}
          </div>
        </main>
      </div>
    `;

    bindEvents(root);
    if (active) renderMessages(active.messages || []);
    else renderMessages([]);
    renderSessionList(root);
  };

  function configPanelHTML() {
    return `<div class="aichat-config">
      <h3>AI 配置</h3>
      <div class="form-group"><label>Provider</label>
        <select id="cfg-provider"><option value="openai">OpenAI</option><option value="anthropic">Anthropic</option></select>
      </div>
      <div class="form-group"><label>API Key</label>
        <input type="password" id="cfg-api-key" placeholder="sk-... / sk-ant-..." />
      </div>
      <div class="form-group"><label>Model</label>
        <input type="text" id="cfg-model" placeholder="gpt-4o / claude-sonnet-4" />
      </div>
      <div class="form-group"><label>Base URL（可选）</label>
        <input type="text" id="cfg-base-url" placeholder="https://api.openai.com/v1" />
      </div>
      <div class="form-group"><label>Temperature</label>
        <input type="number" id="cfg-temp" value="0.7" min="0" max="2" step="0.1" />
      </div>
      <div class="form-group"><label>Max Tokens</label>
        <input type="number" id="cfg-max-tokens" value="4096" min="100" max="128000" />
      </div>
      <div class="form-actions">
        <button class="btn btn-secondary" id="cfg-test-btn">测试连接</button>
        <button class="btn btn-primary" id="cfg-save-btn">保存配置</button>
      </div>
      <div class="cfg-status" id="cfg-status"></div>
    </div>`;
  }

  // ── Events ─────────────────────────────────────────────────
  function bindEvents(root) {
    // Subtab switching
    root.querySelectorAll('.subtab').forEach(btn => {
      btn.addEventListener('click', () => {
        root.querySelectorAll('.subtab').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        root.querySelectorAll('.aichat-panel').forEach(p => p.classList.add('hidden'));
        root.querySelector('#panel-' + btn.dataset.tab).classList.remove('hidden');
        if (btn.dataset.tab === 'config') loadConfig(root);
      });
    });

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
    // Ensure chat subtab active
    root.querySelectorAll('.subtab').forEach(b => b.classList.remove('active'));
    root.querySelector('[data-tab="chat"]')?.classList.add('active');
    root.querySelectorAll('.aichat-panel').forEach(p => p.classList.add('hidden'));
    root.querySelector('#panel-chat')?.classList.remove('hidden');
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
    if (!messages.length) { container.innerHTML = '<div class="aichat-empty">发送消息开始对话</div>'; return; }
    container.innerHTML = messages.map(m => `
      <div class="aichat-msg aichat-msg-${escHtml(m.role)}">
        <div class="aichat-msg-role">${m.role === 'user' ? '你' : 'AI'}</div>
        <div class="aichat-msg-content">${formatContent(m.content)}</div>
      </div>`).join('');
    container.scrollTop = container.scrollHeight;
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

    // Typing indicator
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
        const err = await resp.json().catch(() => ({ error: resp.statusText }));
        throw new Error(err.error || resp.statusText);
      }
      const data = await resp.json();
      session.messages.push({ role: 'assistant', content: data.message?.content || JSON.stringify(data) });
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
      root.querySelector('#cfg-provider').value = cfg.provider || 'openai';
      root.querySelector('#cfg-model').value = cfg.model || '';
      root.querySelector('#cfg-base-url').value = cfg.base_url || '';
      root.querySelector('#cfg-temp').value = cfg.temperature || 0.7;
      root.querySelector('#cfg-max-tokens').value = cfg.max_tokens || 4096;
    } catch {}
  }

  async function saveConfig(root) {
    const status = root.querySelector('#cfg-status');
    const provider = root.querySelector('#cfg-provider').value;
    const model = root.querySelector('#cfg-model').value;
    const baseURL = root.querySelector('#cfg-base-url').value;
    const temp = parseFloat(root.querySelector('#cfg-temp').value);
    const maxTok = parseInt(root.querySelector('#cfg-max-tokens').value);
    const apiKey = root.querySelector('#cfg-api-key').value;
    try {
      await fetch('/api/ai/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider, model, base_url: baseURL, temperature: temp, max_tokens: maxTok })
      });
      if (apiKey) {
        await fetch('/api/ai/config/key', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ api_key: apiKey })
        });
      }
      status.textContent = '✅ 配置已保存';
      status.style.color = 'green';
    } catch (err) {
      status.textContent = '❌ 保存失败: ' + err.message;
      status.style.color = 'red';
    }
  }

  async function testConfig(root) {
    const status = root.querySelector('#cfg-status');
    status.textContent = '测试中...'; status.style.color = 'blue';
    try {
      const r = await fetch('/api/ai/config/test', { method: 'POST' });
      if (r.ok) { status.textContent = '✅ 连接成功'; status.style.color = 'green'; }
      else {
        const e = await r.json().catch(() => ({}));
        status.textContent = '❌ ' + (e.error || r.statusText); status.style.color = 'red';
      }
    } catch (err) {
      status.textContent = '❌ ' + err.message; status.style.color = 'red';
    }
  }

})();
