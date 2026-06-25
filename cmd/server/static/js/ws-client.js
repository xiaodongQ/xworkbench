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
        // 区分 run-loop 事件（payload.event 存在）和原生 exec 流（payload.chunk/done）
        const p = msg.payload || {};
        if (p.event) {
          handleRunLoopProgress(p);
        } else {
          handleExecStream(p);
        }
      } else if (msg.channel === 'scheduler') {
        // scheduler 启停状态变化：重拉 dashboard 和 automation
        if (currentTab === 'dashboard' && typeof loadDashboard === 'function') loadDashboard();
        if (currentTab === 'automation' && typeof loadAutomation === 'function') loadAutomation({silent: true});
      } else if (msg.channel === 'scheduled') {
        // 定时任务状态变化（started / done）：重拉列表。
        // chunk 推送已改走 exec 频道（handleExecStream），这里不再无差别重拉，
        // 避免 task 执行中每次 chunk 触发 loadScheduled() → handler 重算 next_run_at 漂移
        const evt = msg.payload && msg.payload.event;
        if (evt === 'started' || evt === 'done') {
          if (typeof loadScheduled === 'function') loadScheduled();
          if (typeof loadRecentExecutions === 'function') loadRecentExecutions();
        }
      } else if (msg.channel === 'shortcut') {
        // 快捷目录被另一个终端打开：重拉 “快捷方式” 列表
        if (currentTab === 'shortcuts' && typeof loadShortcuts === 'function') loadShortcuts();
      } else if (msg.channel === 'agent') {
        // 远程 Agent 状态变更（心跳丢失/任务释放）：重拉 agent 列表
        if (currentTab === 'relay' && typeof loadAgents === 'function') loadAgents();
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

// handleRunLoopProgress 消费 handleTaskRunLoop 后端 goroutine 推的进度事件。
// payload 字段（与 cmd/server/main.go runLoopBackground 对应）：
//   - task_id: 哪个 task（用 _currentTaskId() 过滤，避免弹窗关闭/切换时误渲染）
//   - event: iteration_started | iteration_done | loop_done | loop_error
//   - iteration: 1-based（iteration_started/iteration_done）
//   - model: "haiku"/"sonnet"/"opus"
//   - score: number（iteration_done/loop_done）
//   - exit_code: int（iteration_done）
//   - error: string（loop_error / iteration_done 失败时）
//   - reason: "threshold_met" | "max_iterations" | "build_failed"（loop_done）
//   - history: Step[]（loop_done 时一次性给完整 history）
function handleRunLoopProgress(payload) {
  if (!payload || !payload.task_id) return;
  // 只渲染当前打开的 task 的进度（避免多个 task 弹窗互窜）
  const cur = (typeof _currentTaskId === 'function') ? _currentTaskId() : '';
  if (cur && payload.task_id !== cur) return;
  if (typeof _aiLoopProgressShow !== 'function') return; // tasks.js 还没加载

  switch (payload.event) {
    case 'iteration_started':
      _aiLoopProgressShow(
        `迭代 ${payload.iteration} 启动 (model: ${payload.model})...`
      );
      break;
    case 'iteration_done': {
      const scoreStr = payload.score != null ? ` · score=${payload.score.toFixed(2)}` : '';
      const errStr = payload.error ? ` · ❌ ${payload.error}` : '';
      _aiLoopProgressShow(
        `迭代 ${payload.iteration} 完成 (exit=${payload.exit_code}${scoreStr}${errStr})`
      );
      // 历史区增量重渲染：iteration_done 不带 history,所以从累计中加这一步
      const histEl = document.getElementById('ai-loop-progress-history');
      if (histEl) {
        const cur = histEl.getAttribute('data-history') ? JSON.parse(histEl.getAttribute('data-history')) : [];
        cur.push({
          iteration: payload.iteration, model: payload.model,
          exit_code: payload.exit_code, score: payload.score, error: payload.error,
        });
        histEl.setAttribute('data-history', JSON.stringify(cur));
        // 复用 _aiLoopProgressRenderHistory 的渲染逻辑
        if (typeof _aiLoopProgressRenderHistory === 'function') {
          _aiLoopProgressRenderHistory(cur);
        }
      }
      break;
    }
    case 'loop_done': {
      const reasonMap = {
        threshold_met: '✓ 达到阈值,循环结束',
        max_iterations: '⏸ 达到最大迭代次数',
        build_failed: '⚠ 命令构建失败',
      };
      _aiLoopProgressShow(reasonMap[payload.reason] || '✓ 循环结束');
      if (Array.isArray(payload.history) && typeof _aiLoopProgressRenderHistory === 'function') {
        _aiLoopProgressRenderHistory(payload.history);
        const histEl = document.getElementById('ai-loop-progress-history');
        if (histEl) histEl.setAttribute('data-history', JSON.stringify(payload.history));
      }
      break;
    }
    case 'loop_error':
      _aiLoopProgressShow('❌ 后端 panic: ' + (payload.error || 'unknown'));
      break;
    default:
      // 未知 event:忽略,不影响其他 handler
      break;
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