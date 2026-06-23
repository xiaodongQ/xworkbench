# 调研：任务模板 + 保存过滤器

## 1. 价值

当前 xworkbench 创建任务时需要填一堆字段：title/description/resources/acceptance/experience_ids/task_type... 很多任务是同质化的（例如每日站会任务、release 任务、checklist 任务）。任务模板可以：
- 一键复用，减少重复填写
- 标准化模式（团队对"release 任务该填什么"达成一致）
- 给 AI agent 提供预设骨架

另外随着任务数量增长，前端一个下拉过滤完全不够用。Linear/Jira 的"保存视图"功能让用户把常用 filter 组合存起来随时切换：
- "我今天要做的"
- "本模块未完成"
- "所有 remote 任务"
- "上周完成"

## 2. 借鉴来源

- **Jira Task Templates** - 模板化任务创建
- **Linear Saved Views** - 视图保存
- **Notion Templates** - 模板结构
- **GitHub Filters** - 搜索式过滤

## 3. 设计

### 3.1 任务模板（task_templates）

**表结构：**
```sql
CREATE TABLE task_templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    category TEXT,            -- 分类（release/checklist/manual/...）
    task_type TEXT,           -- 默认 manual/scheduled/remote
    template_body TEXT,       -- JSON: {description, resources, acceptance, module, experience_ids, ...}
    use_count INTEGER DEFAULT 0,
    created_at DATETIME,
    updated_at DATETIME
);
```

**API：**
```
GET    /api/task-templates                  # 列出所有模板
GET    /api/task-templates?category=release # 按分类过滤
POST   /api/task-templates                  # 创建模板
GET    /api/task-templates/{id}             # 模板详情
PUT    /api/task-templates/{id}             # 更新模板
DELETE /api/task-templates/{id}             # 删除模板
POST   /api/task-templates/{id}/instantiate # 用模板创建任务
```

**instantiate 返回值**：
- 创建出的 task 完整对象
- use_count 自增

### 3.2 保存过滤器（saved_filters）

**表结构：**
```sql
CREATE TABLE saved_filters (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    filter_json TEXT NOT NULL,    -- JSON: {status, task_type, priority, search, ...}
    is_default INTEGER DEFAULT 0, -- 是否为默认
    sort_order INTEGER DEFAULT 0,
    created_at DATETIME,
    updated_at DATETIME
);
```

**API：**
```
GET    /api/saved-filters
POST   /api/saved-filters
DELETE /api/saved-filters/{id}
PUT    /api/saved-filters/{id}        # 更新 / 切换 default
```

**前端集成**（P3 候选）：
- 工具栏加"+"按钮 → 弹窗输入名字 + 选择 filter → 保存
- 列表顶部下拉"视图"显示所有 saved filter + "默认"

## 4. 优先级

1. **任务模板** - 直接减少重复工作，价值高
2. **保存过滤器** - 用户量大了才有价值，先做后端 API

## 5. 实现

- 先做后端（model + repo + handler + e2e）
- 前端 UI 留给下一轮（v2）

## 6. 风险

- 模板 body 是 JSON 字符串，结构变化时需要兼容（加 version 字段）
- saved_filters 删除时如果有任务依赖了此 filter，要提示用户
