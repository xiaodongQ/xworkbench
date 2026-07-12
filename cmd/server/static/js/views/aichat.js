// aichat.js — 浮动 AI 对话挂件
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

  // eslint-disable-next-line no-var
  var pinned = false;

  // ── Init ─────────────────────────────────────────────────
  window.initFloatingChat = function() {
    if (document.getElementById('floating-chat-btn')) return; // already initialized

    // 浮动按钮
    const btn = document.createElement('button');
    btn.id = 'floating-chat-btn';
    btn.className = 'floating-chat-btn';
    btn.title = 'AI 助手';
    btn.innerHTML = '💬';
    document.body.appendChild(btn);

    // 遮罩
    const backdrop = document.createElement('div');
    backdrop.id = 'floating-chat-backdrop';
    backdrop.className = 'floating-chat-backdrop';
    document.body.appendChild(backdrop);

    // 面板
    const panel = document.createElement('div');
    panel.id = 'floating-chat-panel';
    panel.className = 'floating-chat-panel';
    panel.innerHTML = panelHTML();
    document.body.appendChild(panel);

    // 配置 Modal（在面板内）
    addConfigModal(panel);

    // 高度拖拽调节
    initResize(panel);

    bindEvents();
    refreshPanel();
  };

  function panelHTML() {
    const sessions = getSessions();
    const lastId = getLastId();
    return `
      <div class="floating-chat-resize-handle" id="floating-chat-resize-handle" title="拖拽调节高度"></div>
      <div class="floating-chat-header">
        <span class="floating-chat-header-title">AI 助手</span>
        <div class="floating-chat-header-actions">
          <button class="btn-icon" id="aichat-open-config" title="配置">⚙</button>
          <button class="btn-icon" id="aichat-pin-btn" title="点击不收起（自动收起）">📌</button>
          <button class="btn-icon" id="aichat-close-btn" title="收起">◀</button>
        </div>
      </div>
      <div class="floating-chat-sessions">
        <select id="aichat-session-select">
          ${sessions.length === 0 ? '<option value="">-- 无会话 --</option>' : ''}
          ${sessions.map(s => `<option value="${s.id}" ${s.id === lastId ? 'selected' : ''}>${escHtml(s.title || '新对话')}</option>`).join('')}
        </select>
        <button class="btn btn-primary btn-sm" id="aichat-new-btn">+ 新建</button>
        <button class="btn btn-secondary btn-sm" id="aichat-del-btn" title="删除选中会话">删除</button>
      </div>
      <div class="floating-chat-body">
        <div class="aichat-messages" id="aichat-messages"></div>
        <div class="aichat-input-area">
          <textarea id="aichat-input" placeholder="发送消息，AI 将通过工具帮你操作任务、目录..." rows="2"></textarea>
          <div class="aichat-input-row">
            <button class="btn btn-secondary btn-sm" id="aichat-clear-btn">清空</button>
            <button class="btn btn-primary" id="aichat-send-btn">发送</button>
          </div>
        </div>
      </div>
    `;
  }

  function addConfigModal(panel) {
    const modal = document.createElement('div');
    modal.id = 'aichat-config-modal';
    modal.className = 'modal-backdrop hidden';
    modal.innerHTML = `
      <div class="modal-card">
        <div class="modal-header">
          <h3>AI 助手配置</h3>
          <button class="btn-icon" id="aichat-config-close" aria-label="close">✕</button>
        </div>
        <div class="modal-body" id="aichat-config-body">
          ${configPanelHTML()}
        </div>
      </div>
    `;
    panel.appendChild(modal);
  }

  // ── Open / Close ─────────────────────────────────────────
  function open() {
    document.getElementById('floating-chat-panel').classList.add('open');
    document.getElementById('floating-chat-backdrop').classList.add('open');
    document.getElementById('floating-chat-btn').style.display = 'none';
    refreshPanel();
    const input = document.getElementById('aichat-input');
    if (input) setTimeout(() => input.focus(), 300);
  }

  function close() {
    document.getElementById('floating-chat-panel').classList.remove('open');
    document.getElementById('floating-chat-backdrop').classList.remove('open');
    document.getElementById('floating-chat-btn').style.display = '';
  }

  function toggle() {
    const panel = document.getElementById('floating-chat-panel');
    if (panel.classList.contains('open')) close();
    else open();
  }

  // ── Resize ───────────────────────────────────────────────
  function initResize(panel) {
    const handle = panel.querySelector('#floating-chat-resize-handle');
    let startY, startTop;

    function onMove(e) {
      const clientY = e.touches ? e.touches[0].clientY : e.clientY;
      const dy = clientY - startY;
      const newTop = Math.max(0, Math.min(window.innerHeight - 320, startTop + dy));
      panel.style.top = newTop + 'px';
      panel.style.height = (window.innerHeight - newTop) + 'px';
    }

    function onUp() {
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
      document.removeEventListener('touchmove', onMove);
      document.removeEventListener('touchend', onUp);
      document.body.style.userSelect = '';
    }

    handle.addEventListener('mousedown', e => {
      startY = e.clientY;
      startTop = panel.offsetTop;
      document.addEventListener('mousemove', onMove);
      document.addEventListener('mouseup', onUp);
      document.body.style.userSelect = 'none';
      e.preventDefault();
    });

    handle.addEventListener('touchstart', e => {
      startY = e.touches[0].clientY;
      startTop = panel.offsetTop;
      document.addEventListener('touchmove', onMove, { passive: false });
      document.addEventListener('touchend', onUp);
      document.body.style.userSelect = 'none';
    }, { passive: true });
  }

  // ── Events ───────────────────────────────────────────────
  function bindEvents() {
    document.getElementById('floating-chat-btn').addEventListener('click', toggle);
    document.getElementById('floating-chat-backdrop').addEventListener('click', () => { if (!pinned) close(); });
    document.getElementById('aichat-close-btn').addEventListener('click', close);

    // 图钉锁定
    const pinBtn = document.getElementById('aichat-pin-btn');
    pinBtn.addEventListener('click', () => {
      pinned = !pinned;
      pinBtn.classList.toggle('pinned', pinned);
      pinBtn.title = pinned ? '已锁定不收起（点击取消）' : '点击不收起（自动收起）';
    });

    // 配置
    document.getElementById('aichat-open-config').addEventListener('click', () => {
      const modal = document.getElementById('aichat-config-modal');
      modal.classList.remove('hidden');
      const panel = document.getElementById('floating-chat-panel');
      loadConfig(panel);
    });
    const cfgModal = document.getElementById('aichat-config-modal');
    document.getElementById('aichat-config-close').addEventListener('click', () => {
      cfgModal.classList.add('hidden');
    });
    cfgModal.addEventListener('click', e => {
      if (e.target === cfgModal) cfgModal.classList.add('hidden');
    });

    // 会话
    document.getElementById('aichat-new-btn').addEventListener('click', () => createSession());
    document.getElementById('aichat-del-btn').addEventListener('click', () => deleteSession());
    document.getElementById('aichat-session-select').addEventListener('change', function() {
      const id = this.value;
      if (id) switchToSession(id);
    });

    // 消息
    document.getElementById('aichat-send-btn').addEventListener('click', sendMessage);
    document.getElementById('aichat-input').addEventListener('keydown', e => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); }
    });
    document.getElementById('aichat-clear-btn').addEventListener('click', () => {
      document.getElementById('aichat-messages').innerHTML = '';
      document.getElementById('aichat-input').value = '';
    });

    // 配置保存/测试
    const panel = document.getElementById('floating-chat-panel');
    panel.querySelector('#cfg-save-btn')?.addEventListener('click', () => saveConfig(panel));
    panel.querySelector('#cfg-test-btn')?.addEventListener('click', () => testConfig(panel));

    // ESC
    document.addEventListener('keydown', e => {
      if (e.key !== 'Escape') return;
      const modal = document.getElementById('aichat-config-modal');
      if (modal && !modal.classList.contains('hidden')) {
        modal.classList.add('hidden');
        return;
      }
      if (!pinned) close();
    });
  }

  // ── Panel refresh ────────────────────────────────────────
  function refreshPanel() {
    const sessions = getSessions();
    const lastId = getLastId();
    const select = document.getElementById('aichat-session-select');
    if (select) {
      select.innerHTML = sessions.length === 0
        ? '<option value="">-- 无会话 --</option>'
        : sessions.map(s => `<option value="${s.id}" ${s.id === lastId ? 'selected' : ''}>${escHtml(s.title || '新对话')}</option>`).join('');
    }
    const active = sessions.find(s => s.id === lastId);
    renderMessages(active ? active.messages : []);
  }

  // ── Config Panel HTML ────────────────────────────────────
  function configPanelHTML() {
    return `<div class="aichat-config">
      <p class="aichat-config-hint">支持 Anthropic / OpenAI 双协议共存，填好后用顶部切换选择使用哪套。</p>
      <div class="cfg-model-warning" id="cfg-model-warning" style="display:none;background:#fef3c7;border:1px solid #f59e0b;border-radius:6px;padding:10px 12px;margin-bottom:12px;font-size:13px;color:#92400e">
        ⚠️ 当前未配置模型，请先在下方填写 API Key 和 Model 后保存。
      </div>

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
      <div id="cfg-test-output" style="display:none;margin-top:8px;background:var(--bg-secondary,#0f172a);border:1px solid var(--border,#334155);border-radius:6px;padding:8px 10px;font-size:12px;color:var(--text,#e2e8f0);max-height:120px;overflow-y:auto;white-space:pre-wrap;word-break:break-all;line-height:1.5"></div>
    </div>`;
  }

  // ── Global helpers (called from inline onchange) ─────────
  window.showTestTip = function() {
    const root = document.getElementById('floating-chat-panel');
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
    const root = document.getElementById('floating-chat-panel');
    if (!root) return;
    const tip = root.querySelector('#cfg-test-tip');
    if (tip) tip.style.display = 'none';
  };
  window.onCfgProviderChange = function() {
    const root = document.getElementById('floating-chat-panel');
    if (!root) return;
    const provider = root.querySelector('#cfg-provider').value;
    const model = (provider === 'openai'
      ? root.querySelector('#cfg-openai-model')
      : root.querySelector('#cfg-anthropic-model'))?.value.trim();
    const keyEl = (provider === 'openai'
      ? root.querySelector('#cfg-openai-key')
      : root.querySelector('#cfg-anthropic-key'));
    const hasKey = !!(keyEl?.value.trim() || (keyEl?.placeholder?.startsWith('已配置')));
    const hint = root.querySelector('#cfg-provider-hint');
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

  // ── Sessions ─────────────────────────────────────────────
  function createSession() {
    const sessions = getSessions();
    const s = { id: 'sess_' + Date.now(), title: '新对话', messages: [], createdAt: new Date().toISOString() };
    sessions.unshift(s);
    saveSessions(sessions);
    setLastId(s.id);
    refreshPanel();
  }

  function switchToSession(id) {
    setLastId(id);
    const sessions = getSessions();
    const s = sessions.find(s => s.id === id);
    refreshPanel();
  }

  function deleteSession() {
    const select = document.getElementById('aichat-session-select');
    const id = select ? select.value : '';
    if (!id) return;
    let sessions = getSessions().filter(s => s.id !== id);
    saveSessions(sessions);
    if (sessions.length === 0) {
      setLastId('');
    } else {
      setLastId(sessions[0].id);
    }
    refreshPanel();
  }

  // ── Messages ─────────────────────────────────────────────
  function renderMessages(messages) {
    const container = document.getElementById('aichat-messages');
    if (!container) return;
    if (!messages || !messages.length) { container.innerHTML = welcomeHTML(); return; }
    container.innerHTML = messages.map(m => `
      <div class="aichat-msg aichat-msg-${escHtml(m.role)}">
        <div class="aichat-msg-role">${m.role === 'user' ? '你' : 'AI'}</div>
        <div class="aichat-msg-content">${formatContent(m.content)}</div>
      </div>`).join('');
    container.scrollTop = container.scrollHeight;
  }

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

  // ── Send ─────────────────────────────────────────────────
  async function sendMessage() {
    const input = document.getElementById('aichat-input');
    const text = input.value.trim();
    if (!text) return;
    input.value = '';

    let sessions = getSessions();
    let id = getLastId();
    let session = sessions.find(s => s.id === id);
    if (!session) { createSession(); sessions = getSessions(); id = getLastId(); session = sessions.find(s => s.id === id); }

    session.messages.push({ role: 'user', content: text });
    if (session.messages.length === 1) { session.title = text.slice(0, 30); }
    saveSessions(sessions);
    refreshPanel();

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
        try { const err = await resp.json().catch(() => null); if (err && err.error) errMsg = err.error; } catch {}
        throw new Error(errMsg);
      }
      const data = await resp.json();
      session.messages.push({ role: 'assistant', content: data.message?.content || JSON.stringify(data) });

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

  // ── Config ───────────────────────────────────────────────
  async function loadConfig(root) {
    try {
      const r = await fetch('/api/ai/config');
      const d = await r.json();
      const cfg = d.ai_chat || {};
      const provider = cfg.active_provider || 'anthropic';
      root.querySelector('#cfg-provider').value = provider;

      const ant = cfg.anthropic || {};
      root.querySelector('#cfg-anthropic-url').value = ant.base_url || '';
      root.querySelector('#cfg-anthropic-model').value = ant.model || '';
      const antKeyEl = root.querySelector('#cfg-anthropic-key');
      if (antKeyEl && ant.api_key && ant.api_key !== '') {
        antKeyEl.placeholder = '已配置（修改请重新输入）';
      }

      const oai = cfg.openai || {};
      root.querySelector('#cfg-openai-url').value = oai.base_url || '';
      root.querySelector('#cfg-openai-model').value = oai.model || '';
      const oaiKeyEl = root.querySelector('#cfg-openai-key');
      if (oaiKeyEl && oai.api_key && oai.api_key !== '') {
        oaiKeyEl.placeholder = '已配置（修改请重新输入）';
      }

      const active = provider === 'openai' ? oai : ant;
      const warn = root.querySelector('#cfg-model-warning');
      if (warn) warn.style.display = (!active.model || !active.api_key) ? 'block' : 'none';

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
    const antModel = root.querySelector('#cfg-anthropic-model').value.trim();
    const antURL = root.querySelector('#cfg-anthropic-url').value.trim();
    const antKey = root.querySelector('#cfg-anthropic-key').value;
    const oaiModel = root.querySelector('#cfg-openai-model').value.trim();
    const oaiURL = root.querySelector('#cfg-openai-url').value.trim();
    const oaiKey = root.querySelector('#cfg-openai-key').value;

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
    const output = root.querySelector('#cfg-test-output');
    try {
      const r = await fetch('/api/ai/config/test?provider=' + encodeURIComponent(provider), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({})
      });
      output.style.display = 'none';
      if (r.ok) {
        const data = await r.json().catch(() => ({}));
        status.textContent = '✅ 连接成功';
        status.style.color = '#16a34a';
        if (data.reply) {
          output.textContent = 'AI 回复：' + data.reply;
          output.style.display = 'block';
        }
      } else {
        const e = await r.json().catch(() => ({}));
        status.textContent = '❌ 连接失败';
        status.style.color = '#dc2626';
        output.textContent = e.error || r.statusText;
        output.style.display = 'block';
      }
    } catch (err) {
      status.textContent = '❌ ' + err.message; status.style.color = '#dc2626';
      if (output) { output.textContent = err.message; output.style.display = 'block'; }
    }
    status._timer = setTimeout(() => { status.textContent = ''; if (output) output.style.display = 'none'; }, 6000);
  }

})();
