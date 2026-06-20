// WebSocket 客户端 - 监听任务状态变更
let ws = null;
let wsReconnectTimer = null;

function initWS() {
  if (ws && ws.readyState === WebSocket.OPEN) return;
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(protocol + '//' + location.host + '/ws');
  ws.onopen = () => {
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
      } else if (msg.channel === 'exec') {
        handleExecStream(msg.payload);
      } else if (msg.channel === 'scheduler') {
        // scheduler 启停状态变化：重拉 dashboard 和 automation
        if (currentTab === 'dashboard' && typeof loadDashboard === 'function') loadDashboard();
        if (currentTab === 'automation' && typeof loadAutomation === 'function') loadAutomation({silent: true});
      } else if (msg.channel === 'scheduled') {
        // 定时任务被触发：重拉“定时任务”列表和“最近执行”列表
        if (typeof loadScheduled === 'function') loadScheduled();
        if (typeof loadRecentExecutions === 'function') loadRecentExecutions();
      } else if (msg.channel === 'shortcut') {
        // 快捷目录被另一个终端打开：重拉 “快捷方式” 列表
        if (currentTab === 'shortcuts' && typeof loadShortcuts === 'function') loadShortcuts();
      }
    } catch (err) {
      console.error('WS parse error:', err);
    }
  };
  ws.onclose = () => {
    wsReconnectTimer = setTimeout(initWS, 3000);
  };
  ws.onerror = () => ws.close();
}

// handleExecStream 处理 execution 实时输出推送。
// payload 有两种：
//   - chunk 推送：{execution_id, task_id, chunk} — 来自 executor.Run 的 onChunk 回调
//   - done 推送：{execution_id, task_id, done:true, exit_code} — 进程退出后
//
// 行为：
//   - 如果 exec-detail-modal 打开着且就是同一个 execution：chunk 追加到 output textarea
//   - done 到达后重新 fetch 一次该 execution，走 renderExecOutput 拿到完整解析后的文本
//   - modal 未打开该 exec：静默丢弃（避免误伤）
//   - automation 页面“最近执行”列表需要在 done 后刷一下状态（交给 currentTab 检查）
function handleExecStream(payload) {
  if (!payload || !payload.execution_id) return;
  const execId = payload.execution_id;
  // chunk 推送
  if (typeof payload.chunk === 'string') {
    if (typeof currentExecId !== 'undefined' && currentExecId === execId) {
      const el = document.getElementById('exec-detail-output');
      if (el) {
        // 避免覆盖“未运行”提示
        if (el.value === '' || el.value === '(无输出)' || el.value === '⏳ 执行中…') {
          el.value = payload.chunk;
        } else {
          el.value += payload.chunk;
        }
        // 滚到底
        el.scrollTop = el.scrollHeight;
      }
    }
    return;
  }
  // done 推送：重拉完整数据并重新渲染
  if (payload.done === true) {
    if (typeof currentExecId !== 'undefined' && currentExecId === execId) {
      // 重拉走 renderExecOutput（制除流式阶段累积的 raw 字节，换成 claude JSON 解析后的友好文本）
      fetch('/api/executions/' + execId)
        .then(r => r.ok ? r.json() : null)
        .then(exec => {
          if (!exec) return;
          const el = document.getElementById('exec-detail-output');
          if (el) {
            // 保留 raw 状态 + 加尾部标记
            const raw = el.value;
            const rendered = (typeof renderExecOutput === 'function') ? renderExecOutput(exec.output) : exec.output;
            // 如果用户已经看到 claude JSON 解析后的部分，替换为完整渲染
            el.value = rendered;
            el.scrollTop = el.scrollHeight;
          }
          // 退出码徽标：调用 viewExecutionDetail 重画（会重填全部字段，包括 exit_code 提示）
          if (typeof viewExecutionDetail === 'function' && execId) {
            viewExecutionDetail(execId);
          }
        })
        .catch(err => console.error('[ws-client] fetch exec on done:', err));
    }
    // 任何 tab 看到 exec done 都刷一下“最近执行”列表
    if (currentTab === 'automation' && typeof loadRecentExecutions === 'function') {
      loadRecentExecutions();
    }
  }
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