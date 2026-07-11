# 快捷链接/目录分类功能设计

## 目标
当条目数量多时（链接 > 10、目录 > 5），全部平铺难快速定位。需支持按分类组织。

## 数据模型

### 新表
- `link_categories` - 链接分类
- `dir_categories` - 目录分类

### 字段
分类表共字段：
- `id` TEXT PRIMARY KEY
- `name` TEXT NOT NULL - 分类名（如"工作"、"开发"、"文档"）
- `icon` TEXT - emoji 或图标 URL（可选）
- `sort_order` INTEGER - 排序
- `created_at` DATETIME

### 修改表
- `web_links` 新增 `category_id` TEXT（可空，外键）
- `dir_shortcuts` 新增 `category_id` TEXT（可空，外键）

## API 设计

### 分类管理
- `GET /api/link-categories` - 列出分类
- `POST /api/link-categories` - 创建分类
- `PUT /api/link-categories/{id}` - 更新分类
- `DELETE /api/link-categories/{id}` - 删除分类（默认分类不可删）

### 目录分类
- `GET /api/dir-categories`
- `POST /api/dir-categories`
- `PUT /api/dir-categories/{id}`
- `DELETE /api/dir-categories/{id}`

### 修改现有 API
- 创建/更新链接时接受 `category_id`
- 创建/更新目录时接受 `category_id`

## 前端 UI

### 链接面板
- 默认显示"全部"
- 顶部一行分类标签（chips），点击切换
- 选中分类时只显示该分类下的链接
- "全部"显示所有链接

### 目录面板
- 同样的分类标签 + 过滤逻辑
- 目录面板已有"+"按钮，新增分类按钮

### 添加/编辑表单
- 增加"分类"下拉框
- 显示已有分类 + "+ 新建分类"选项

## 迭代计划

### Round 1: Schema + 模型 + 迁移
- 数据库 schema 新增 category 表
- model 添加新结构
- 增量迁移 web_links/dir_shortcuts 加 category_id

### Round 2: Repository 层
- LinkCategoryRepo CRUD
- DirCategoryRepo CRUD
- 修改 WebLinkRepo/DirShortcutRepo 支持 category_id

### Round 3: API 处理器
- 分类 CRUD 路由
- 修改现有链接/目录 API 返回 category 信息

### Round 4: 前端加载逻辑
- 加载分类列表
- 修改 loadLinks/loadDirs 支持按分类过滤

### Round 5: 前端 UI - 分类 chips
- 顶部分类标签栏
- 切换分类过滤

### Round 6: 前端 UI - 添加/编辑表单
- 添加分类字段
- 支持新建分类

### Round 7: 数据迁移兼容
- 旧数据自动归入"默认"分类

### Round 8-15: 细节优化
- 拖动分类排序
- 拖动条目到不同分类
- 删除分类时条目归入默认
- 空分类隐藏
- CSS 优化

### Round 16-20: 测试 + 文档
- E2E 测试
- API 文档
- README 更新
