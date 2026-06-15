// WebSocket 客户端 - 监听任务状态变更
let ws = null;
let wsReconnectTimer = null;

function initWS() {
  if (ws && ws.readyState === WebSocket.OPEN) return;
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(protocol + '//' + location.host + '/ws');
  ws.onopen = () => {
    console.log('WebSocket connected');
    if (wsReconnectTimer) {
      clearTimeout(wsReconnectTimer);
      wsReconnectTimer = null;
    }
  };
  ws.onmessage = (e) => {
    try {
      const msg = JSON.parse(e.data);
      if (msg.channel === 'task') {
        handleTaskUpdate(msg.payload);
      }
    } catch (err) {
      console.error('WS parse error:', err);
    }
  };
  ws.onclose = () => {
    console.log('WebSocket closed, reconnecting...');
    wsReconnectTimer = setTimeout(initWS, 3000);
  };
  ws.onerror = () => ws.close();
}

function handleTaskUpdate(payload) {
  // 刷新当前 tab 的数据
  if (currentTab === 'dashboard' && typeof loadDashboard === 'function') {
    loadDashboard();
  }
  if (currentTab === 'tasks' && typeof loadTasks === 'function') {
    loadTasks();
  }
  // waiting_input 状态弹出提示
  if (payload.status === 'waiting_input' && payload.waiting_input) {
    showWaitingInputNotice(payload);
  }
}

function showWaitingInputNotice(payload) {
  // 已存在弹窗则跳过
  if (document.getElementById('waiting-input-notice')) return;
  const task = payload.task || {};
  const div = document.createElement('div');
  div.id = 'waiting-input-notice';
  div.style.cssText = 'position:fixed;top:20px;right:20px;z-index:9999;background:#fff3cd;border:1px solid #ffc107;border-radius:8px;padding:16px;max-width:400px;box-shadow:0 4px 12px rgba(0,0,0,0.15);';
  div.innerHTML = `
    <div style="font-weight:600;margin-bottom:8px;">⚠️ 任务需要人工介入</div>
    <div style="font-size:13px;margin-bottom:4px;">任务: ${esc(task.title || payload.task_id)}</div>
    <div style="font-size:12px;color:#666;background:#fff8e1;padding:8px;border-radius:4px;white-space:pre-wrap;max-height:150px;overflow-y:auto;">${esc(payload.waiting_input)}</div>
    <button onclick="this.parentElement.remove()" style="margin-top:10px;padding:4px 12px;border:none;background:#ffc107;border-radius:4px;cursor:pointer;">知道了</button>
  `;
  document.body.appendChild(div);
  // 5秒后自动消失
  setTimeout(() => div.remove(), 5000);
}

// 页面加载时初始化 WebSocket
document.addEventListener('DOMContentLoaded', initWS);