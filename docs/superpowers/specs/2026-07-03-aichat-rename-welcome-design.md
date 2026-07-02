# AI 助手 Tab 重命名 + 欢迎页设计

**日期**: 2026-07-03
**目标**: 把「AI 对话」Tab 的语义、命名、空态做得更清晰、更准确、更美观

---

## 1. 背景

当前「AI 对话」Tab 存在 4 个体验问题：

| # | 问题 | 位置 |
|---|---|---|
| 1 | 侧边导航叫「AI 对话」，但实际是 AI 助手（具备工具调用，能管理任务/目录/经验库） | `index.html:63-64` |
| 2 | subtab 叫「本地 Shell」，但下拉框实际可切 `claude / cbc / shell`，命名误导 | `aichat.js:38, 57, 344` |
| 3 | subtab 叫「⚙️ 配置」，与系统配置 Tab 容易混淆 | `aichat.js:39` |
| 4 | 空态只有一行「发送消息开始对话」，新用户不知道这个 Tab 能做什么 | `aichat.js:195` |

---

## 2. 重命名（5 处）

| 位置 | 原文 | 新文 |
|---|---|---|
| 侧边导航 nav-text | `AI 对话` | `AI 助手` |
| 侧边导航 tooltip | `AI 对话\n终端式 Claude Code 交互（PTY）\nmacOS / Linux 实时` | `AI 助手\n对话管理任务/目录/经验库\n终端式 Claude/CBC/shell 交互` |
| subtab (line 38) | `本地 Shell` | `网页终端` |
| subtab (line 39) | `⚙️ 配置` | `⚙️ AI 助手配置` |
| terminal header (line 57) | `本地 Shell` | `网页终端` |
| WS 连接成功提示 (line 344) | `[xworkbench] 本地 Shell 已连接` | `[xworkbench] 网页终端已连接` |
| config panel `<h3>` (line 86) | `AI 配置` | `AI 助手配置` |

---

## 3. 欢迎页（替代空态文案）

当 `messages.length === 0` 时，`#aichat-messages` 容器渲染欢迎面板：

```
┌──────────────────────────────────────────────────────────┐
│                                                          │
│                   👋 你好，我是 AI 助手                   │
│                                                          │
│      我能帮你管理任务、操作目录、查询经验库，              │
│   也可以启动 Claude/CBC CLI 进行交互式操作。             │
│                                                          │
│   ┌──────────────┐  ┌──────────────┐                    │
│   │ 📋 任务管理  │  │ 📁 目录快捷  │                    │
│   │ 创建/查询/   │  │ 本地与远程   │                    │
│   │ 执行任务     │  │ 目录一键访问 │                    │
│   └──────────────┘  └──────────────┘                    │
│   ┌──────────────┐  ┌──────────────┐                    │
│   │ 💡 经验库    │  │ 🛠️ CLI 会话 │                    │
│   │ 搜索已有知识 │  │ 启动 Claude/ │                    │
│   │ 与经验       │  │ CBC 交互会话 │                    │
│   └──────────────┘  └──────────────┘                    │
│                                                          │
│          在下方输入框开始你的第一次对话 ↓                 │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

4 张能力卡来自 `cmd/server/ai_tools.go:GetTools()` 的真实工具：
- 📋 任务管理 — `create_task / list_tasks / get_task / update_task / run_task / get_task_executions`
- 📁 目录快捷 — `create_dir_shortcut / list_dir_shortcuts`
- 💡 经验库 — `search_experiences`
- 🛠️ CLI 会话 — `start_local_shell / run_local_command`（启动 Claude/CBC）

Skill 插件工具（`skill.GetAll()`）动态追加，不在欢迎页硬编码（避免每次加 skill 都要改文案）。

---

## 4. 样式（在 `base.css` 末尾新增）

```css
.aichat-welcome { padding: 32px 24px; text-align: center; }
.aichat-welcome-title {
  font-size: 22px; font-weight: 600; color: var(--text);
  margin-bottom: 8px;
}
.aichat-welcome-sub {
  font-size: 13px; color: var(--text-secondary);
  margin-bottom: 24px; line-height: 1.7;
}
.aichat-welcome-grid {
  display: grid; grid-template-columns: repeat(2, 1fr);
  gap: 12px; max-width: 520px; margin: 0 auto 24px;
}
.aichat-welcome-card {
  background: var(--card); border: 1px solid var(--border);
  border-radius: 8px; padding: 14px 16px; text-align: left;
  transition: border-color .15s, background .15s;
}
.aichat-welcome-card:hover {
  border-color: var(--primary, #22d3ee);
  background: var(--card-hover, rgba(34,211,238,0.04));
}
.aichat-welcome-card-icon { font-size: 20px; margin-bottom: 4px; }
.aichat-welcome-card-title {
  font-size: 13px; font-weight: 600; color: var(--text);
  margin-bottom: 2px;
}
.aichat-welcome-card-desc {
  font-size: 12px; color: var(--text-secondary);
  line-height: 1.5;
}
.aichat-welcome-hint {
  font-size: 12px; color: var(--text-secondary);
  opacity: 0.7;
}
```

风格与项目现有卡片一致（`var(--card)` / `var(--border)` / `var(--text-secondary)`），不引入新色板。居中布局、悬浮态边框高亮（与 `--primary` 联动）。

---

## 5. 涉及文件

| 文件 | 改动 |
|---|---|
| `cmd/server/index.html` | nav-text、tooltip（2 处） |
| `cmd/server/static/js/views/aichat.js` | 5 处文本重命名 + `renderMessages` 空态分支改为欢迎面板 HTML |
| `cmd/server/static/css/base.css` | 新增 `.aichat-welcome*` 样式块（约 30 行） |

无 schema、无路由、无后端改动。`//go:embed` 不受影响（前端文件路径不变）。

---

## 6. 验证

- 浏览器 F5 刷新 → 侧边导航显示「AI 助手」
- 进入 Tab 无对话时 → 显示欢迎页（4 张能力卡 + 提示）
- 进入对话后再清空 → 回到欢迎页
- subtab 切换：AI 助手对话 / 网页终端 / AI 助手配置
- 网页终端连接成功提示改为「网页终端已连接」
- 配置页标题改为「AI 助手配置」
- 无 console 报错；无样式溢出

无需新增测试（纯前端文本/样式改动）。

---

## 7. 不做（YAGNI）

- 不改后端 `/api/ai/*` 路由或文案
- 不动 subtab 顺序
- 不加「首次访问引导」动画（保持简洁）
- 不在欢迎页写动态 skill 插件工具列表（避免文案随 skill 变动）
- 不做国际化（项目目前全中文）