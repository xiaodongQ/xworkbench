---
name: xworkbench
description: "从 xworkbench 后台管理系统拉取任务、TDD 开发迭代、结果回写归档的全流程自动化 Skill。"
---

# Skill Factory — 自动化开发工厂

本 Skill 实现「拉取任务 → TDD 开发 → 多轮迭代验收 → 结果回写归档」的全流程闭环。

## 前置要求

1. **后台管理系统**已运行（默认 http://localhost:8902）
2. **经验库**已初始化目标模块知识
3. **验收样例**已录入（正向 + 反向用例）

## 运行流程

### Step 1: 拉取待认领任务

```bash
curl -s http://localhost:8902/api/tasks?status=pending | jq '.[0]'
```

### Step 2: 任务认领

```bash
curl -s -X PUT http://localhost:8902/api/tasks/{task_id}/status \
  -H "Content-Type: application/json" \
  -d '{"status":"in_progress","maintainer":"factory-agent"}'
```

### Step 3: 获取前置经验库

```bash
curl -s http://localhost:8902/api/experiences?module=redis-cluster | jq '.'
```

### Step 4: TDD 开发循环（最多 20 轮）

```
for iter in $(seq 1 20); do
  test_cases=$(parse_acceptance "$task.acceptance")
  skill_impl=$(develop_skill "$task.description" "$experience_ctx" "$test_cases")
  result=$(run_tests "$skill_impl" "$test_cases")

  pass_rate=$(echo "$result" | jq '.pass_rate')
  false_pos=$(echo "$result" | jq '.false_pos_count')
  false_neg=$(echo "$result" | jq '.false_neg_count')

  if [ "$pass_rate" = "1.0" ] && [ "$false_pos" = "0" ] && [ "$false_neg" = "0" ]; then
    break  # 验收通过
  fi

  # 迭代优化
  task.description="$task.description [ITER $iter FAILED: $result]"
done
```

### Step 5: 上传产物并归档

```bash
curl -s -X PUT http://localhost:8902/api/tasks/{task_id}/status \
  -H "Content-Type: application/json" \
  -d '{"status":"archived","result":{"skill_file":"skills/xxx/SKILL.md","final_accuracy":1.0}}'
```

### Step 6: 不达标终止

达到 20 轮未达标 → `status=exception`。

## 验收标准

| 维度 | 要求 |
|------|------|
| 准确率 | 正向用例全部通过（pass_rate = 1.0） |
| 误判（false_pos） | 0 |
| 漏判（false_neg） | 0 |
| 迭代上限 | 20 轮 |