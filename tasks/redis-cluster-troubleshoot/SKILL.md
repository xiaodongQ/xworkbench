---
name: redis-cluster-troubleshoot
description: "定位 Redis 分布式集群常见故障：节点失联、内存碎片、主从中断、慢查询、Big Key、热 Key 分析。"
---

# Redis 分布式集群故障定位 Skill

用于定位 Redis 分布式集群常见问题，提供标准化的问题定位路径和解决方案。

## 支持场景

1. **集群节点失联**（CLUSTERDOWN）
2. **内存碎片化**（mem_fragmentation_ratio > 1.4）
3. **主从复制中断**
4. **慢查询根因分析**（slowlog > 1s）
5. **Big Key 查询**
6. **热 Key 探测**
7. **连接数打满**

## 诊断流程

### 第一步：识别问题类型

| 关键字 | 问题类型 |
|--------|---------|
| `CLUSTERDOWN` | 集群节点失联 |
| `MOVED/ASK redirect` | Slot 迁移中客户端重定向 |
| `READONLY` | 从节点只读写入冲突 |
| `slowlog` | 慢查询分析 |
| `mem_fragmentation_ratio` | 内存碎片 |
| `maxmemory` | 内存上限触发 |
| `OOM` | 内存溢出 |

### 第二步：信息收集

```sh
redis-cli cluster nodes
redis-cli cluster slots
redis-cli cluster info
redis-cli slowlog get 10
redis-cli --bigkeys
redis-cli memory stats
redis-cli memory purge
```

### 第三步：问题定位与建议

#### CLUSTERDOWN — 集群节点失联

**识别特征**：`CLUSTERDOWN The cluster is gone`

**排查步骤**：检查 cluster-node-timeout、网络连通性、节点日志

**解决方案**：
```conf
cluster-node-timeout 30000
cluster-slave-validity-factor 5
cluster-require-full-coverage no
```

#### 内存碎片化

**识别特征**：`mem_fragmentation_ratio: 1.5+`

**解决方案**：
```sh
redis-cli memory purge
# redis.conf
activedefrag yes
active-defrag-ignore-bytes 100mb
```

#### 慢查询分析

**常见慢命令**：KEYS、SMEMBERS、HGETALL、LRANGE

**解决方案**：用 SCAN 替换 KEYS，避免 O(N) 全量扫描

#### READONLY — 从节点只读冲突

**解决方案**：应用层区分主从写操作，replica 只读

## 快速命令速查

| 场景 | 命令 |
|------|------|
| 集群健康 | `redis-cli cluster info` |
| 节点列表 | `redis-cli cluster nodes` |
| 慢查询 | `redis-cli slowlog get 10` |
| Big Key | `redis-cli --bigkeys` |
| 内存统计 | `redis-cli memory stats` |

## 验收用例

### 正向用例

| 输入 | 预期输出 |
|------|---------|
| `CLUSTERDOWN The cluster is gone` | 定位 cluster-node-timeout，给出 redis.conf 调优建议 |
| `READONLY You can't write` | 识别只读场景，输出 replica 配置检查步骤 |
| slowlog 显示 KEYS > 5s | 给出 SCAN 替换方案 |
| `mem_fragmentation_ratio > 1.5` | 给出 MEMORY PURGE + activedefrag 配置 |

### 反向用例

| 输入 | 预期行为 |
|------|---------|
| 空 slowlog | 返回"无慢查询记录" |
| MySQL 日志 | 明确拒绝，输出"非 Redis 日志格式" |
| 二进制数据 | 提示"请提供文本日志" |