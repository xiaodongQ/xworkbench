#!/usr/bin/env bash
# smoke.sh — 基于运行中服务的冒烟测试
# 用法: ./scripts/smoke.sh
# 依赖: curl, sqlite3
#
# 测试覆盖:
#   1. /version 接口
#   2. /api/executions/{id}/evaluations (created_at, duration_s)
#   3. /api/scheduled/{id}/run-now (executions 创建)
#   4. 新评估触发 (duration_s float 秒)

set -e

ADDR="${ADDR:-http://localhost:8902}"
DB_PATH="${DB_PATH:-$HOME/Library/Application Support/skill-factory/data/skill-factory.db}"
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

FAILED=0

hr() { printf '=%.0s' $(seq 1 50); printf '\n'; }

pass() { printf "${GREEN}✓${NC} %s\n" "$1"; }
fail() { printf "${RED}✗${NC} %s\n" "$1"; FAILED=1; }

http() {
  local method="${1:-GET}"
  local path="$2"
  local body="$3"
  local extra="${4:-}"
  curl -s -X "$method" "$ADDR$path" \
    -H 'Content-Type: application/json' \
    ${body:+-d "$body"} \
    $extra
}

json_get() {
  echo "$1" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('$2',''))"
}

echo
hr
printf "${YELLOW}==> 冒烟测试${NC}  addr=$ADDR\n"
hr

# 1. /version
printf "\n[1] /version 接口\n"
VER=$(http GET "/version")
BUILD=$(json_get "$VER" "build")
if [[ -n "$BUILD" && "$BUILD" != "unknown" ]]; then
  pass "返回 build=$BUILD"
else
  fail "build 为空或 unknown: $BUILD"
fi

# 2. 获取一个有 evaluations 的 execution（优先选有评估记录的）
EXEC_ID=$(sqlite3 "$DB_PATH" "SELECT execution_id FROM evaluations GROUP BY execution_id HAVING COUNT(*)>=1 LIMIT 1" 2>/dev/null)
if [[ -z "$EXEC_ID" ]]; then
  EXEC_ID=$(sqlite3 "$DB_PATH" "SELECT id FROM executions LIMIT 1" 2>/dev/null)
fi
printf "\n[2] GET /api/executions/{id}/evaluations  (exec_id=%s)\n" "$EXEC_ID"

EVALS=$(http GET "/api/executions/$EXEC_ID/evaluations")
FIRST_CA=$(echo "$EVALS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[0]['created_at'] if d else '')" 2>/dev/null)
# 检查 created_at 不是 0001-01-01
if [[ "$FIRST_CA" != *"0001-01-01"* ]] && [[ -n "$FIRST_CA" ]]; then
  pass "created_at 正确: $FIRST_CA"
else
  fail "created_at 解析失败: $FIRST_CA"
fi

# 检查 duration_s 不为 null（如果有数据）
DUR=$(echo "$EVALS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[0].get('duration_s','') if d else '')" 2>/dev/null)
if [[ -n "$DUR" ]]; then
  pass "duration_s 存在: $DUR"
else
  echo "  (无 duration_s 数据，跳过)"
fi

# 3. scheduled task run-now
ST_ID=$(sqlite3 "$DB_PATH" "SELECT id FROM scheduled_tasks LIMIT 1" 2>/dev/null)
if [[ -n "$ST_ID" ]]; then
  printf "\n[3] POST /api/scheduled/{id}/run-now  (scheduled_task_id=%s)\n" "$ST_ID"
  # 读日志行号
  BEFORE=$(wc -l < "$HOME/Library/Application Support/skill-factory/data/logs/xworkbench.log" 2>/dev/null || echo 0)
  RUN_RESP=$(http POST "/api/scheduled/$ST_ID/run-now")
  sleep 3

  # 验证日志里有 executions created
  if grep -q "executions created" "$HOME/Library/Application Support/skill-factory/data/logs/xworkbench.log" 2>/dev/null; then
    pass "executions 创建成功"
  else
    fail "executions 未创建"
  fi
  if grep -q "task execution started" "$HOME/Library/Application Support/skill-factory/data/logs/xworkbench.log" 2>/dev/null; then
    pass "任务执行启动"
  else
    fail "任务执行未启动"
  fi
else
  printf "\n[3] ${YELLOW}(跳过: 无 scheduled_tasks)${NC}\n"
fi

# 4. 触发新评估，检查 duration_s 为 float
if [[ -n "$EXEC_ID" ]]; then
  printf "\n[4] POST /api/executions/{id}/evaluate (触发新评估)\n"
  EVAL_RESP=$(http POST "/api/executions/$EXEC_ID/evaluate")
  sleep 15
  NEW_EVALS=$(http GET "/api/executions/$EXEC_ID/evaluations")
  NEW_DUR=$(echo "$NEW_EVALS" | python3 -c "
import sys,json
d=json.load(sys.stdin)
for e in d:
    m=e.get('evaluator_model','')
    if m.startswith('claude') or m.startswith('sonnet'):
        print(e.get('duration_s',''))
        break
" 2>/dev/null)
  if [[ -n "$NEW_DUR" && "$NEW_DUR" != "0" && "$NEW_DUR" != "null" ]]; then
    pass "新评估 duration_s=$NEW_DUR (float)"
  else
    fail "duration_s 为空或 0: $NEW_DUR"
  fi
  NEW_CA=$(echo "$NEW_EVALS" | python3 -c "
import sys,json
d=json.load(sys.stdin)
for e in d:
    m=e.get('evaluator_model','')
    if m.startswith('claude') or m.startswith('sonnet'):
        print(e.get('created_at',''))
        break
" 2>/dev/null)
  if [[ "$NEW_CA" != *"0001-01-01"* ]] && [[ -n "$NEW_CA" ]]; then
    pass "新评估 created_at=$NEW_CA"
  else
    fail "新评估 created_at 解析失败: $NEW_CA"
  fi
else
  printf "\n[4] ${YELLOW}(跳过: 无 executions)${NC}\n"
fi

# 5. 日志带 caller 信息
printf "\n[5] 日志 caller 信息\n"
if grep -q 'main.go:[0-9]' "$HOME/Library/Application Support/skill-factory/data/logs/xworkbench.log" 2>/dev/null; then
  pass "日志包含 caller (main.go:行号)"
else
  fail "日志缺少 caller"
fi

echo
hr
if [[ $FAILED -eq 0 ]]; then
  printf "${GREEN}✓ 全部通过${NC}\n"
else
  printf "${RED}✗ 有测试失败${NC}\n"
fi
hr
exit $FAILED
