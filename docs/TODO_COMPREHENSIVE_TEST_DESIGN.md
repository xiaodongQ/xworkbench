# Todo 综合验证测试方案

## 调研背景

近期 todo 功能做了大量修改：
- `c509434` Web终端和TODO功能优化
- `81d1ebd` 归档功能
- `0dc1824` 简化月份分组机制
- `983c78c` 10项修复
- `f589cf2` 修复添加子项时重复两次的 bug
- `ae7b13d` 全面重构待办功能

## 当前已发现的 Bug

### Bug #1: `AddAndWrite` 返回的 line_no 在没有尾换行文件时错位

**复现路径：**
1. AddAndWrite(path, "Parent 1", ...) → 文件末尾无 `\n`
2. AddChildAndWrite(path, 2, "Child 1.1", ...) → 在父行后插入子项，但丢失文件末尾换行
3. AddChildAndWrite(path, 2, "Child 1.2", ...) → 后续插入恢复换行
4. AddAndWrite(path, "Parent 2", ...) → 返回 `len(lines)` = 4，但 Parent 2 实际在 line 5

**影响：** 前端用 `line_no` 给新父任务加子项时，子项加到了错误位置（嵌套到旧父任务的子项里）

### Bug #2: `AddChildAndWrite` 丢失文件末尾换行

**复现路径：**
1. 文件：`"# Todo\n- [ ] Parent\n"`（有尾换行）
2. AddChildAndWrite(path, 2, "Child", ...) → 文件变成 `"# Todo\n- [ ] Parent\n  - [ ] Child"`（无尾换行）

**影响：** 多次添加子项后文件结构不稳定，导致 `len(lines)` 计算偏差

## 测试设计

### 后端（Go）测试用例

#### A. 基础增删改查
- A1. AddAndWrite 单项到空文件
- A2. AddAndWrite 单项到有内容的文件
- A3. AddAndWrite 多次连续追加（验证 line_no 准确）
- A4. AddAndWrite 到有 `---` 分隔线的文件（活跃区末尾）
- A5. AddChildAndWrite 单子项
- A6. AddChildAndWrite 多子项（连续调用）
- A7. AddChildAndWrite 多层级（孙项）
- A8. ToggleAndWrite 切换勾选状态
- A9. DeleteAndWrite 删除顶级项及其子项
- A10. DeleteAndWrite 删除中间子项（保留其他）

#### B. 元数据
- B1. AddAndWrite 带 due_date
- B2. AddAndWrite 带 tags
- B3. AddAndWrite 带 note
- B4. AddAndWrite 同时带 due_date + tags + note
- B5. ToggleAndWrite 保留元数据
- B6. DeleteAndWrite 保留其他项元数据

#### C. 树结构（核心）
- C1. BuildTree 单层结构
- C2. BuildTree 双层结构
- C3. BuildTree 三层嵌套
- C4. BuildTree 混合（有/无子项）
- C5. Flatten 深度优先
- C6. Flatten 跨多根节点

#### D. 归档功能
- D1. ArchiveItem 顶级项
- D2. ArchiveItem 含子项的顶级项
- D3. ArchiveItem 子项单独归档（应报错）
- D4. UnarchiveItem 顶级项
- D5. UnarchiveItem 子项单独恢复（应报错）
- D6. UnarchiveItem 恢复连带子项
- D7. ParseSections 区分活跃区/归档区
- D8. WriteSections 重新生成文件

#### E. 行号一致性（**关键**）
- E1. **Bug #1 验证**：连续 AddAndWrite 返回的 line_no 始终是文件实际行号
- E2. **Bug #2 验证**：AddChildAndWrite 后文件保留尾换行
- E3. AddAndWrite 后再次 Parse/Flatten，所有 line_no 一致
- E4. AddAndWrite + AddChildAndWrite 混合操作，line_no 不漂移

#### F. 边界条件
- F1. 空 text 报错
- F2. 只有 `---` 分隔线的文件
- F3. 文件不存在时 AddAndWrite
- F4. 没有 `---` 但有归档标题的文件
- F5. 文件首行不是标题

#### G. 备注（note）
- G1. 单行 note 关联到父项
- G2. 多行 note 合并
- G3. note 行不被识别为子项
- G4. 删除父项同时删除关联 note

### 前端集成测试（http）

#### H. API 端到端
- H1. POST /api/todo + POST /api/todo/{line_no}/children 顺序调用
- H2. PUT /api/todo/{line_no} (toggle)
- H3. PUT /api/todo/{line_no}/edit
- H4. DELETE /api/todo/{line_no}
- H5. PUT /api/todo/{line_no}/archive
- H6. PUT /api/todo/{line_no}/unarchive
- H7. GET /api/todo 列表解析

### 顺序操作场景（核心 Bug 复现）

#### I. 场景1：连续添加多个父任务带子项
```
POST /api/todo {text:"P1"}  → 返回 line_no=2
POST /api/todo/2/children {text:"C1"}  → 成功
POST /api/todo/2/children {text:"C2"}  → 成功
POST /api/todo {text:"P2"}  → 应返回 line_no=5（不是 4）
POST /api/todo/5/children {text:"C3"}  → 应加到 P2 下
```

#### J. 场景2：编辑现有任务，添加/删除子项
```
POST /api/todo {text:"P1"}  → line_no=2
POST /api/todo/2/children {text:"C1"}  → C1
PUT /api/todo/2/edit {text:"P1 renamed"}  → 修改主任务
PUT /api/todo/3/edit {text:"C1 updated"}  → 修改子项
POST /api/todo/2/children {text:"C2"}  → 新增子项
DELETE /api/todo/3  → 删除 C1
```

#### K. 场景3：归档后继续添加
```
POST /api/todo {text:"P1"}  → line_no=2
POST /api/todo/2/children {text:"C1"}  → C1
PUT /api/todo/2/archive  → P1+C1 归档
POST /api/todo {text:"P2"}  → line_no 应在归档前
POST /api/todo/{p2_line}/children {text:"C2"}  → 加到 P2 下
```
