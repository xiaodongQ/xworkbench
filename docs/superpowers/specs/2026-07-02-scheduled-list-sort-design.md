# 定时任务列表按下次运行时间排序

## 背景

自动化页（`#page-automation`）有两处"最近执行"：

| 位置 | 当前标题 | 说明 |
|---|---|---|
| 定时任务表格最后一列 | 最近执行 | 显示上次运行时间 + 下次运行时间（子行） |
| 表格下方独立列表 | 最近执行 | 所有 execution 记录（scheduled / manual / continue） |

两处中文名完全相同，口语中容易混淆。下方独立列表改名"最近**任务**执行"。

## 目标

1. **标题区分**：下方独立列表标题从"最近执行"改为"最近任务执行"
2. **排序功能**：定时任务表格的"最近执行"列支持点击排序，按**下次运行时间**升序（即将先跑的排前面）
3. **偏好持久化**：排序方向存入 `localStorage`（`automation.schedSortDir`），刷新后保持用户习惯

## 设计

### 1. 标题改名

**文件**：`cmd/server/index.html`

```html
<!-- 旧 -->
<h2 class="page-title" style="margin:0">最近执行</h2>

<!-- 新 -->
<h2 class="page-title" style="margin:0">最近任务执行</h2>
```

### 2. 表头可点击

**文件**：`cmd/server/static/js/views/automation.js`

定时任务表格（`loadScheduled` 函数）的"最近执行"表头 `<th>` 改为可点击：

```html
<!-- 旧 -->
<th>最近执行</th>

<!-- 新 -->
<th style="cursor:pointer;user-select:none" onclick="toggleSchedSort()">
  最近执行 <span id="sched-sort-icon">⇅</span>
</th>
```

### 3. 排序逻辑

**排序状态变量**：
```js
const SCHED_SORT_KEY = 'automation.schedSortDir'; // 'asc' | 'desc'
```

**`toggleSchedSort()` 函数**：
1. 读取 `localStorage.getItem(SCHED_SORT_KEY)`
2. 首次：无值 → 默认 `asc`
3. 已有值：切换方向（`asc` → `desc` → `asc`）
4. 保存到 `localStorage`
5. 更新图标显示（`↑` 升序 / `↓` 降序 / `⇅` 无排序）
6. 调用 `renderScheduledList(sortedList)` 重排表格

**排序规则**：
- enabled + 有 `next_run_at`：按 `next_run_at` 升序（soonest first）
- enabled + `next_run_at` 为 null（cron 解析失败）：排到最后
- disabled：排到最后

**排序后列表替换**（不重新请求后端，直接在前端数组上排）：
```js
function sortScheduledList(list) {
  const dir = localStorage.getItem(SCHED_SORT_KEY) || 'asc';
  return [...list].sort((a, b) => {
    // disabled 排最后
    if (!a.enabled && !b.enabled) return 0;
    if (!a.enabled) return 1;
    if (!b.enabled) return -1;
    // 无 next_run_at 排最后
    if (!a.next_run_at && !b.next_run_at) return 0;
    if (!a.next_run_at) return 1;
    if (!b.next_run_at) return -1;
    // 按 next_run_at 排序
    const diff = new Date(a.next_run_at) - new Date(b.next_run_at);
    return dir === 'asc' ? diff : -diff;
  });
}
```

### 4. 初始化排序图标

页面加载时（`loadScheduled` 末尾）读取 `localStorage` 状态，设置正确的排序图标：

```js
function updateSortIcon() {
  const dir = localStorage.getItem(SCHED_SORT_KEY) || 'asc';
  const icon = document.getElementById('sched-sort-icon');
  if (icon) icon.textContent = dir === 'asc' ? '↑' : dir === 'desc' ? '↓' : '⇅';
}
```

## 关键文件

| 文件 | 改动 |
|---|---|
| `cmd/server/index.html` | 标题"最近执行" → "最近任务执行" |
| `cmd/server/static/js/views/automation.js` | `loadScheduled` 表头可点击 + 排序逻辑 + 图标状态 |

## 范围

**做**：
- 标题改名
- 表头点击排序（前端排序，不改后端）
- localStorage 偏好持久化

**不做**：
- 后端改动（`next_run_at` 已由 2026-06-24 design 注入）
- 下方"最近任务执行"列表的排序（该列表按时间倒序，不受本次改动影响）
- 多列排序
