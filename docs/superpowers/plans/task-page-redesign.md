# 任务页面简化 + 分类支持 + 收起展开

## 目标
- 状态从 6 个简化到 5 个（pending/in_progress/running → 待处理/进行中，waiting_input/archived/exception 保留）
- 支持任务分类（自由输入标签，一个任务可多分类）
- 按分类分组显示，每组可折叠/展开

## 状态映射

| 旧状态 | 新显示 |
|---|---|
| pending | 🔴 待处理 |
| in_progress | 🟡 进行中 |
| running | 🟡 进行中（合并） |
| waiting_input | 🔵 等待输入 |
| archived | ✅ 已完成 |
| exception | ⚠️ 异常 |

## 数据模型

- `tasks.category`: TEXT, 存储逗号分隔的多个分类名（如 "前端,需求"）
- 后端: 迁移添加 category 列，CRUD 支持 category 字段

## 前端改动 (tasks.js + index.html)

### 1. 分类分组视图
- 顶部 filter 改为: 全部 / 待处理 / 进行中 / 等待输入 / 已完成 / 异常 + 分类筛选
- 列表按 category 分组，每组一个可折叠块
- 组头部显示: 分类名 + 任务数 + 展开/收起按钮
- 无分类的任务归入 "未分类" 组

### 2. 任务行显示
- 移除 task_type 列（原 manual/remote 意义不大）
- 增加 category chips（最多显示2个，超出折叠）
- 简化状态 badge（4种颜色对应上面5态）

### 3. 新建/编辑任务
- 新增 category 输入框（逗号分隔多分类）
- 已有任务编辑时解析并回显

### 4. 折叠交互
- 点击组头部切换折叠状态
- 折叠状态用 localStorage 记忆

## 后端改动

### 1. DB 迁移
```sql
ALTER TABLE tasks ADD COLUMN category TEXT;
```

### 2. API
- POST/PUT /api/tasks body 增加 `category` 字段
- GET /api/tasks 返回 `category` 字段
- 前端分类筛选: `GET /api/tasks?category=前端`

## 实现步骤

1. [ ] 后端: DB 迁移添加 category 列
2. [ ] 后端: API 支持 category CRUD
3. [ ] 前端: task modal 增加 category 输入
4. [ ] 前端: renderTaskTable 改为分类分组视图
5. [ ] 前端: 分类折叠交互 + localStorage 记忆
6. [ ] 前端: 简化状态显示（合并 running 到 进行中）
7. [ ] 前端: 更新 filter 逻辑

## 验收标准

- [ ] 任务列表按分类分组显示
- [ ] 每组可独立折叠/展开，状态记忆
- [ ] 新建/编辑任务可设置多个分类
- [ ] 状态从 6 个简化为 5 个显示（running 不独立显示）
- [ ] 分类筛选正常
