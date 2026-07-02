# 定时任务列表按下次运行时间排序 — 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在自动化页的定时任务表格"最近执行"列加点击排序功能，按下次运行时间升序排列；同时区分两个"最近执行"标题。

**Architecture:** 纯前端改动，在 `loadScheduled` 函数内实现前端排序。排序方向存 `localStorage`，页面刷新后保持偏好。

**Tech Stack:** Vanilla JS / localStorage，无新依赖。

---

## 文件清单

| 文件 | 改动 |
|---|---|
| `cmd/server/index.html` | 标题"最近执行" → "最近任务执行" |
| `cmd/server/static/js/views/automation.js` | 表头可点击 + 排序函数 + 图标状态 |

---

## Task 1: 标题改名

**Files:**
- Modify: `cmd/server/index.html:255`

- [ ] **Step 1: 改标题**

```html
<!-- 旧 -->
<h2 class="page-title" style="margin:0">最近执行</h2>

<!-- 新 -->
<h2 class="page-title" style="margin:0">最近任务执行</h2>
```

- [ ] **Step 2: 提交**

```bash
git add cmd/server/index.html
git commit -m "feat(automation): rename '最近执行' to '最近任务执行' for disambiguation"
```

---

## Task 2: 表头可点击 + 排序状态初始化

**Files:**
- Modify: `cmd/server/static/js/views/automation.js:294`（loadScheduled 函数内表格表头部分）

- [ ] **Step 1: 在文件顶部添加排序常量和状态**

在 `automation.js` 顶部（在 `SCHED_STATUS_TEXT` 之后）添加：

```js
// 定时任务表格排序
const SCHED_SORT_KEY = 'automation.schedSortDir'; // 'asc' | 'desc'
```

- [ ] **Step 2: 添加 updateSchedSortIcon 函数**

在 `SCHED_STATUS_TEXT` 定义之后、`loadScheduledSummary` 之前添加：

```js
// 更新定时任务表格排序图标状态
function updateSchedSortIcon() {
  const dir = localStorage.getItem(SCHED_SORT_KEY) || 'asc';
  const icon = document.getElementById('sched-sort-icon');
  if (icon) icon.textContent = dir === 'asc' ? '↑' : dir === 'desc' ? '↓' : '⇅';
}

// 切换定时任务表格排序方向
function toggleSchedSort() {
  const prev = localStorage.getItem(SCHED_SORT_KEY) || 'asc';
  const next = prev === 'asc' ? 'desc' : 'asc';
  localStorage.setItem(SCHED_SORT_KEY, next);
  updateSchedSortIcon();
  loadScheduled();
}
```

- [ ] **Step 3: 修改 loadScheduled 中的表头**

找到 `loadScheduled` 函数中的表头行（约为第 294 行）：

```js
// 旧
el.innerHTML = `<table><thead><tr><th>名称</th><th>Cron</th><th>类型</th><th>状态</th><th>最近执行</th><th>操作</th></tr></thead><tbody>` + list.map(s => {
```

改为：

```js
// 新（表头 onclick + 排序图标）
const initSortIcon = () => {
  const dir = localStorage.getItem(SCHED_SORT_KEY) || 'asc';
  return dir === 'asc' ? '↑' : dir === 'desc' ? '↓' : '⇅';
};
el.innerHTML = `<table><thead><tr><th>名称</th><th>Cron</th><th>类型</th><th>状态</th><th style="cursor:pointer;user-select:none" onclick="toggleSchedSort()">最近执行 <span id="sched-sort-icon">${initSortIcon()}</span></th><th>操作</th></tr></thead><tbody>` + list.map(s => {
```

- [ ] **Step 4: 在 loadScheduled 末尾调用 updateSchedSortIcon**

在表格渲染完毕后、`el.innerHTML = ...` 行之后、`loadScheduled` 函数末尾附近添加：

```js
// 页面加载时恢复排序图标状态
updateSchedSortIcon();
```

- [ ] **Step 5: 提交**

```bash
git add cmd/server/static/js/views/automation.js
git commit -m "feat(automation): add click-to-sort on scheduled table by next run time"
```

---

## Task 3: 排序逻辑实现

**Files:**
- Modify: `cmd/server/static/js/views/automation.js`（loadScheduled 函数内排序部分）

- [ ] **Step 1: 在 loadScheduled 中，渲染前对 list 排序**

找到 `loadScheduled` 函数中 `list.map(s => {` 这一行，在其前面插入排序逻辑：

```js
// 按下次运行时间排序（next_run_at 升序，disabled/null 排最后）
const sortDir = localStorage.getItem(SCHED_SORT_KEY) || 'asc';
const sortedList = [...list].sort((a, b) => {
  // disabled 排最后
  if (!a.enabled && !b.enabled) return 0;
  if (!a.enabled) return 1;
  if (!b.enabled) return -1;
  // 无 next_run_at（cron 解析失败）排最后
  if (!a.next_run_at && !b.next_run_at) return 0;
  if (!a.next_run_at) return 1;
  if (!b.next_run_at) return -1;
  const diff = new Date(a.next_run_at) - new Date(b.next_run_at);
  return sortDir === 'asc' ? diff : -diff;
});
```

然后将后续 `list.map(s => {` 改为 `sortedList.map(s => {`。

- [ ] **Step 2: 验证代码逻辑**

确认改动后的 `loadScheduled` 函数结构：

1. `fetchJSON('/api/scheduled')` 获取 list
2. 对 list 做排序 → sortedList
3. `fetchJSON('/api/executions?limit=50')` 获取 execs（这行不受排序影响）
4. `sortedList.map(s => { ... })` 渲染表格

- [ ] **Step 3: 提交**

```bash
git add cmd/server/static/js/views/automation.js
git commit -m "feat(automation): implement sort by next_run_at with localStorage persistence"
```

---

## Task 4: 验证

- [ ] **Step 1: 启动服务**

```bash
./scripts/run.sh --restart
```

- [ ] **Step 2: 浏览器打开** `http://localhost:8902`，进入自动化页

- [ ] **Step 3: 验证标题已改名**

页面中段"最近执行"标题已变为"最近任务执行"

- [ ] **Step 4: 验证排序图标**

定时任务表格"最近执行"列头显示 `↑`（默认升序）

- [ ] **Step 5: 点击排序**

点击表头，图标变为 `↓`，列表按下次运行时间**降序**排列；再次点击变回 `↑`

- [ ] **Step 6: 刷新页面验证偏好持久化**

刷新页面，确认图标保持 `↑` 或 `↓`（取决于上次选择），列表顺序符合预期

- [ ] **Step 7: 提交**

```bash
git add cmd/server/index.html cmd/server/static/js/views/automation.js
git commit -m "feat(automation): sort scheduled tasks by next run time with localStorage preference"
```
