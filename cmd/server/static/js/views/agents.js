// ===== 远程 Agent 管理 =====

let agentsCache = [];

// getInstallCommand 返回 xwcli 安装命令（根据当前浏览器 origin 动态生成）。
function getInstallCommand() {
  const server = window.location.origin;
  return `curl -fsSL ${server}/api/xwcli/install.sh | bash -s -- \`hostname\``;
}

function copyXwcliInstallCmd(el) {
  const cmd = getInstallCommand();
  navigator.clipboard.writeText(cmd).then(() => {
    const orig = el.textContent;
    el.textContent = '✅ 已复制到剪贴板';
    setTimeout(() => { el.textContent = orig; }, 2000);
  }).catch(() => {
    prompt('复制命令（Ctrl+C）:', cmd);
  });
}

function populateInstallCmd() {
  const el = document.getElementById('xwcli-install-cmd');
  if (el) el.textContent = getInstallCommand();
}

function formatRelativeTime(t) {
  if (!t) return '从未';
  const ms = Date.now() - new Date(t).getTime();
  if (ms < 0) return '刚刚';
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return sec + 's 前';
  const min = Math.floor(sec / 60);
  if (min < 60) return min + 'm 前';
  const hr = Math.floor(min / 60);
  if (hr < 24) return hr + 'h 前';
  const day = Math.floor(hr / 24);
  return day + 'd 前';
}

function agentStatusTag(status) {
  if (status === 'online') return '<span style="color:green;font-weight:600">● 在线</span>';
  if (status === 'offline') return '<span style="color:#999">● 离线</span>';
  return '<span style="color:#999">● ' + escapeHtml(status || 'unknown') + '</span>';
}

async function loadAgents() {
  const container = document.getElementById('agents-table');
  if (!container) return;
  const status = document.getElementById('agent-filter-status')?.value || '';
  const params = status ? '?status=' + encodeURIComponent(status) : '';
  container.innerHTML = '<div style="color:var(--text-secondary);padding:20px;text-align:center">加载中...</div>';
  try {
    const agents = await fetchJSON('/api/agents' + params);
    agentsCache = agents || [];
    if (agentsCache.length === 0) {
      container.innerHTML = '<div style="color:var(--text-secondary);padding:20px;text-align:center">暂无 Agent<br><span style="font-size:11px">Agent 通过 <code>POST /api/agents/register</code> 注册</span></div>';
      return;
    }
    renderAgents(agentsCache);
    populateInstallCmd();
  } catch (e) {
    container.innerHTML = '<div style="color:var(--danger);padding:20px">加载失败: ' + escapeHtml(e.message || String(e)) + '</div>';
  }
}

function renderAgents(agents) {
  const container = document.getElementById('agents-table');
  const rows = agents.map(a => {
    const hb = formatRelativeTime(a.last_heartbeat);
    const cap = a.capabilities || '-';
    const ver = a.version || '-';
    const created = a.created_at ? new Date(a.created_at).toLocaleString() : '-';
    const isOnline = a.status === 'online';
    const taskCount = a.current_task_count || 0;
    const acChecked = a.auto_claim_enabled ? 'checked' : '';
    // 绑定的远程机器（类型=remote 的 dir_shortcut）
    const boundId = a.bound_dir_shortcut_id || '';
    const boundLabel = boundId ? `<span style="color:var(--primary)" title="${escapeHtml(boundId)}">🔗 ${escapeHtml(boundId.substring(0,8))}…</span>` : '<span style="color:var(--text-secondary);font-size:10px">未绑定</span>';
    return `
      <tr style="border-bottom:1px solid var(--border)">
        <td style="padding:8px">${agentStatusTag(a.status)}</td>
        <td style="padding:8px"><strong>${escapeHtml(a.name)}</strong><br><span style="font-size:10px;color:var(--text-secondary)">${escapeHtml(a.id.substring(0, 8))}…</span></td>
        <td style="padding:8px;font-size:11px">${escapeHtml(cap)}</td>
        <td style="padding:8px;font-size:11px">${escapeHtml(ver)}</td>
        <td style="padding:8px;font-size:11px">${hb}</td>
        <td style="padding:8px;font-size:11px;text-align:center">${taskCount > 0 ? `<strong style="color:var(--primary)">${taskCount}</strong>` : '0'}</td>
        <td style="padding:8px;font-size:11px">
          <label style="cursor:pointer;font-size:11px"><input type="checkbox" ${acChecked} onchange="toggleAutoClaim('${a.id}', this.checked)"> auto_claim</label>
        </td>
        <td style="padding:8px;font-size:11px" id="agent-bound-${a.id}">${boundLabel}</td>
        <td style="padding:8px;font-size:11px">${created}</td>
        <td style="padding:8px;white-space:nowrap">
          <button class="btn btn-small" onclick="bindAgentRemote('${a.id}', '${escapeHtml(a.name)}', '${escapeHtml(boundId)}')" title="绑定/解绑 远程机器（type=remote 的 dir_shortcut）">${boundId ? '🔗 改绑' : '🔗 绑定'}</button>
          <button class="btn btn-small" onclick="releaseAgentTasks('${a.id}', '${escapeHtml(a.name)}')" title="释放该 agent 手上所有 in_progress 任务回 pending 池" ${!isOnline && taskCount === 0 ? 'disabled' : ''} style="margin-left:4px">🔓 释放任务</button>
          <button class="btn btn-small" onclick="resetAgentToken('${a.id}', '${escapeHtml(a.name)}')" title="重置 token（立即作废旧 token）" style="margin-left:4px">🔑 重置 Token</button>
          <button class="btn btn-small" onclick="deleteAgent('${a.id}', '${escapeHtml(a.name)}')" title="删除 agent（会先释放任务）" style="margin-left:4px;color:var(--danger)">🗑️</button>
        </td>
      </tr>
    `;
  }).join('');
  container.innerHTML = `
    <table style="width:100%;border-collapse:collapse;font-size:12px">
      <thead>
        <tr style="background:var(--hover);text-align:left">
          <th style="padding:8px">状态</th>
          <th style="padding:8px">名称</th>
          <th style="padding:8px">能力</th>
          <th style="padding:8px">版本</th>
          <th style="padding:8px">最后心跳</th>
          <th style="padding:8px;text-align:center">手头任务</th>
          <th style="padding:8px">自动领任务</th>
          <th style="padding:8px">绑定远程机器</th>
          <th style="padding:8px">注册时间</th>
          <th style="padding:8px">操作</th>
        </tr>
      </thead>
      <tbody>${rows}</tbody>
    </table>
  `;
}

async function toggleAutoClaim(agentID, enabled) {
  try {
    const resp = await fetchJSON(`/api/agents/${agentID}/auto-claim`, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({enabled}),
    });
    if (resp.ok) {
      console.log('auto_claim toggled', agentID, enabled);
    } else {
      alert('切换失败');
      loadAgents();
    }
  } catch (e) {
    alert('切换失败: ' + (e.message || e));
    loadAgents();
  }
}

// bindAgentRemote 弹出一个选择器，选已有 type=remote 的 dir_shortcut 来绑 agent。
// 空串 = 解绑。绑定后任务页选这个 agent 会走 SSH 远端执行（-p / -r 都行）。
async function bindAgentRemote(agentID, agentName, currentBoundId) {
  try {
    const dirs = await fetchJSON(API + '/api/dir-shortcuts');
    const remoteDirs = (dirs || []).filter(d => d.type === 'remote');
    if (remoteDirs.length === 0) {
      alert('没有 type=remote 的 dir_shortcut。\n请先在"⚡ 快捷"页添加一个远程目录。');
      return;
    }
    let lines = ['选一个远程机器绑定到 agent「' + agentName + '」：', ''];
    remoteDirs.forEach((d, i) => {
      const mark = (d.id === currentBoundId) ? ' ← 当前绑定' : '';
      lines.push(`  ${i+1}. ${d.name}  (${d.remote_user || '?'}@${d.remote_host || '?'})${mark}`);
    });
    lines.push('', '  0. 解绑（恢复本机/主动 claim 模式）', '  c. 取消', '');
    const ans = prompt(lines.join('\n'), '1');
    if (ans == null || ans === 'c') return;
    let chosenId = '';
    if (ans !== '0') {
      const idx = parseInt(ans, 10) - 1;
      if (isNaN(idx) || idx < 0 || idx >= remoteDirs.length) {
        alert('序号不合法');
        return;
      }
      chosenId = remoteDirs[idx].id;
    }
    const resp = await fetchJSON(`/api/agents/${agentID}/bind-dir-shortcut`, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({dir_shortcut_id: chosenId}),
    });
    if (resp.ok) {
      loadAgents();
    } else {
      alert('绑定失败：' + (resp.error || 'unknown'));
    }
  } catch (e) {
    alert('绑定失败: ' + (e.message || e));
  }
}

async function releaseAgentTasks(agentID, name) {
  if (!confirm(`确定要强制释放 Agent「${name}」手上所有 in_progress 任务回 pending 池？\n\n（释放后 agent 重新心跳时会通过 claim-next 重新认领）`)) return;
  try {
    const resp = await fetchJSON(`/api/agents/${agentID}/release-tasks`, {method: 'POST'});
    if (resp.ok) {
      alert(`已释放 ${resp.released_tasks} 个任务`);
      loadAgents();
    } else {
      alert('释放失败: ' + (resp.error || '未知错误'));
    }
  } catch (e) {
    alert('释放失败: ' + (e.message || e));
  }
}

async function resetAgentToken(agentID, name) {
  if (!confirm(`确定要重置 Agent「${name}」的 token？\n\n⚠️ 旧 token 立即失效。\n请准备好把新 token 同步到 agent 端。`)) return;
  try {
    const resp = await fetchJSON(`/api/agents/${agentID}/reset-token`, {method: 'POST'});
    if (resp.ok) {
      // 弹窗展示新 token（用户必须复制走）
      const tokenBox = document.createElement('div');
      tokenBox.style.cssText = 'position:fixed;top:50%;left:50%;transform:translate(-50%,-50%);background:var(--card-bg);padding:20px;border-radius:8px;box-shadow:0 4px 16px rgba(0,0,0,0.2);z-index:9999;max-width:600px;width:90%';
      tokenBox.innerHTML = `
        <h3 style="margin-top:0">🔑 新 Token（${escapeHtml(name)}）</h3>
        <p style="color:var(--danger);font-size:12px">⚠️ 旧 token 已失效，agent 端必须更新为下方 token</p>
        <textarea readonly style="width:100%;height:80px;font-family:monospace;font-size:12px;padding:8px;border:1px solid var(--border);border-radius:4px;background:var(--hover)" onclick="this.select()">${escapeHtml(resp.new_token)}</textarea>
        <div style="margin-top:12px;text-align:right">
          <button class="btn" onclick="navigator.clipboard.writeText('${resp.new_token.replace(/'/g, "\\'")}').then(()=>{this.textContent='✅ 已复制'})">📋 复制</button>
          <button class="btn" onclick="this.closest('div[style*=fixed]').remove()">关闭</button>
        </div>
      `;
      document.body.appendChild(tokenBox);
      loadAgents();
    } else {
      alert('重置失败: ' + (resp.error || '未知错误'));
    }
  } catch (e) {
    alert('重置失败: ' + (e.message || e));
  }
}

async function deleteAgent(agentID, name) {
  if (!confirm(`⚠️ 危险操作！\n\n确定要删除 Agent「${name}」？\n\n删除后该 agent 占用任务会先释放回 pending 池，agent 记录不可恢复。`)) return;
  if (!confirm('再次确认：删除操作不可撤销。')) return;
  try {
    const resp = await fetchJSON(`/api/agents/${agentID}`, {method: 'DELETE'});
    if (resp.ok) {
      alert(`已删除（释放了 ${resp.released_tasks} 个任务）`);
      loadAgents();
    } else {
      alert('删除失败: ' + (resp.error || '未知错误'));
    }
  } catch (e) {
    alert('删除失败: ' + (e.message || e));
  }
}
