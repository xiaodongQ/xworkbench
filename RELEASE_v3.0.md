# v3.0-stable Release Notes

## 🎉 Major Features & Improvements

### 📝 Markdown Rendering Infrastructure
- **Markdown 基础设施完整集成**：引入 marked.js / DOMPurify / highlight.js
- **AI 输出渲染优化**：解决代码块嵌套、未闭合 fence 等边缘问题
- `normalizeFences` 预处理算法：栈式扫描，自动修复和补全
- 单元测试覆盖 10+ 场景，纯 Node 环境验证

### 🤖 AI Chat Multi-Round Protocol
- **多轮对话完整支持**：修复 Anthropic + OpenAI 协议差异
- Assistant message blocks 正确传递，tool_use_id 精确匹配
- 超时管理：总预算 5min / 单轮 60s / 工具执行 30s / 最大轮次 20
- 504 GatewayTimeout 返回，前端自动取消超期请求

### 💻 Terminal Session Management
- **多会话并存架构**：termPool 替代全局 term/ws
- 独立 xterm.js + WebSocket，会话切换不中断
- 跨标签页持久化连接
- 会话列表按创建时间排序，UI 稳定

### 🐧 Windows PTY 完整支持
- PTY 会话注册、资源清理、Auth 检测
- SIGTERM/SIGKILL 分阶段清理：graceful → force kill
- 修复 Windows 下 task stop() 后目录无法删除问题
- pty_session_manager.go 跨平台编译

### 🎨 UI/UX Polish
- **执行详情卡片**：Markdown 渲染 + 原文切换，避免 JSON 堆砌
- **AI 助手面板**：浮动侧栏，Markdown 消息 + 原文复制按钮
- 字体/间距统一优化，信息密度提升
- 终端侧栏可收起 (☰ 按钮)

### 📊 Dashboard & Statistics
- 统计改用 SQL 聚合查询，每日统计按日期排序
- 总览紧凑化：缩小 padding/gap/margin，提高信息密度
- 最新任务仅显示手动任务 (manual type)

### 🔧 Build & Release Pipeline
- 跨平台构建工作流：macOS/Linux/Windows 并行编译
- 本地快捷、Darwin amd64 修复、Shell 兼容性改进
- config.template.json 替代旧 .conf 格式

## 🐛 Bug Fixes
- 修复 AI 对话 tool_result 协议错误
- 修复 Anthropic blocks 序列化和 OpenAI ToolCalls 字段
- 修复会话 session_id 解析失败时的重建逻辑
- 修复 Windows 子进程清理残留问题
- 修复 xwcli 安装命令从不显示的 bug
- SQLite DATE() 函数替换为 substr()，避免 monotonic clock 格式异常

## 🏗️ Architecture Highlights
- 16 数据表 + 7 大 Tab + 4 Widget + 97 HTTP 路由
- ~16,600 行 Go 代码，单二进制部署
- 跨 macOS / Linux / Windows，集成本地快捷 + AI 任务 + 定时调度 + 代理

## ⬆️ Upgrade from v2.0
- ✅ 数据库自动迁移（无需手动操作）
- ✅ 配置文件向前兼容
- 建议：重启服务后清浏览器缓存（Ctrl+Shift+Delete）

---

**完整功能介绍**：[xworkbench-intro-standalone.html](https://xiaodongq.github.io/assets/xworkbench/xworkbench-intro-standalone.html)

**文档导航**：
- [docs/DESIGN.md](docs/DESIGN.md) — 架构设计
- [docs/MARKDOWN_RENDER_REUSE.md](docs/MARKDOWN_RENDER_REUSE.md) — Markdown 渲染集成指南
- [docs/SCHEDULER.md](docs/SCHEDULER.md) — Cron 调度
- [CHANGELOG.md](CHANGELOG.md) — 完整变更记录
