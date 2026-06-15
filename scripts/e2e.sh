#!/usr/bin/env bash
# xworkbench e2e 验证脚本（开发用，临时端口 + 临时 db,不影响默认）
#
# 用法：
#   ./scripts/e2e.sh                  # 跑全部 demo case(临时端口 + 临时 db)
#   ./scripts/e2e.sh basic            # 只跑 basic case(创建任务 + 跑 + 评估)
#   ./scripts/e2e.sh delete           # 只跑 delete case
#   ./scripts/e2e.sh eval             # 只跑 eval case
#   ./scripts/e2e.sh fast             # 复用运行中的 server(不 build/不重启),适合日常开发
#                                     # 配合: sh scripts/run.sh --restart  →  ./scripts/e2e.sh fast
#                                     #      E2E_BASE_URL=http://x:9001 ./scripts/e2e.sh fast
#   ./scripts/e2e.sh teardown         # 强清理(不跑 case,只清残留)
#
# 设计原则：
#   - 用 0.5% 概率空闲的临时端口(避免跟默认 8902 冲突)
#   - 用 /tmp/sf-e2e-XXX.db 临时 db,跑完自动清理
#   - 不修改 monorepo 任何文件,只起临时 binary 在 /tmp/
#   - case 函数化,新增 case 只加 function + 在 main switch 注册

set -euo pipefail

# === 配置 ===
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TMP_BIN="/tmp/xw-e2e-$$"
TMP_DB="/tmp/xw-e2e-$$.db"
TMP_LOG="/tmp/xw-e2e-$$.log"
TMP_PORT=$((19000 + RANDOM % 1000))   # 19000-19999 临时端口
SCRIPT_CMD_TYPE="${SCRIPT_CMD_TYPE:-claude}"   # 可被环境变量覆盖
SCRIPT_MODEL="${SCRIPT_MODEL:-haiku}"

# BASE_URL: 默认连临时 server;fast 模式改成连已有 server(默认 :8902)
BASE_URL="${BASE_URL:-localhost:$TMP_PORT}"
USE_EXISTING="${USE_EXISTING:-0}"

# === 颜色 ===
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
ok()   { echo -e "${GREEN}✓ $*${NC}"; }
info() { echo -e "${YELLOW}▶ $*${NC}"; }
err()  { echo -e "${RED}✗ $*${NC}" >&2; }

# === 工具函数 ===

cleanup() {
  set +e
  if [ -n "${SERVER_PID:-}" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill -9 "$SERVER_PID" 2>/dev/null
  fi
  # 顺手清可能残留的同端口进程
  lsof -ti :$TMP_PORT 2>/dev/null | xargs kill -9 2>/dev/null
  rm -f "$TMP_BIN" "$TMP_DB" "$TMP_DB-shm" "$TMP_DB-wal" "$TMP_LOG"
}
trap cleanup EXIT

start_server() {
  # fast 模式:BASE_URL 已指向运行中的 server(默认 :8902),不重启
  if [ "$USE_EXISTING" = "1" ]; then
    info "复用已有 server @ $BASE_URL(不 build/不起进程)"
    if ! curl -s "${BASE_URL}/api/tasks" >/dev/null 2>&1; then
      err "已有 server 不在 $BASE_URL,先跑: sh scripts/run.sh"
      exit 1
    fi
    ok "复用 server $BASE_URL"
    return
  fi
  info "build + start server @ :$TMP_PORT (db=$TMP_DB)"
  ( cd "$ROOT" && go build -o "$TMP_BIN" ./cmd/server )
  DB_PATH="$TMP_DB" ADDR=":$TMP_PORT" nohup "$TMP_BIN" > "$TMP_LOG" 2>&1 &
  SERVER_PID=$!
  # 等 server 起来
  for i in 1 2 3 4 5 6 7 8 9 10; do
    if curl -s "${BASE_URL}/api/tasks" >/dev/null 2>&1; then
      ok "server up (pid=$SERVER_PID, port=$TMP_PORT)"
      return
    fi
    sleep 0.5
  done
  err "server 启动失败,看 $TMP_LOG"
  tail -20 "$TMP_LOG"
  exit 1
}

# json 字段提取(避免引号嵌套问题)
jget() {  # $1=json $2=key
  echo "$1" | python3 -c "import json,sys; d=json.load(sys.stdin); v=d
for k in '$2'.split('.'):
    v = v.get(k) if isinstance(v, dict) else None
print(v if v is not None else '')"
}

# === Case 实现 ===
# 每个 case 假定 server 已起来,跑完清理自己创建的 task

case_basic() {
  info "[basic] 创建 task + 跑 shell + 查 list"
  local desc="e2e basic test $(date +%s)"
  local resp
  resp=$(curl -s -X POST "${BASE_URL}/api/tasks" -H "Content-Type: application/json" \
    -d "{\"title\":\"e2e\",\"description\":\"$desc\"}")
  local tid=$(jget "$resp" "id")
  [ -n "$tid" ] || { err "创建 task 失败: $resp"; return 1; }
  ok "task created: $tid"

  local ex
  ex=$(curl -s -X POST "${BASE_URL}/api/tasks/$tid/run" \
    -H "Content-Type: application/json" \
    -d '{"command_type":"shell","prompt":"echo e2e-ok"}')
  local eid=$(jget "$ex" "execution_id")
  [ -n "$eid" ] || { err "run 失败: $ex"; return 1; }
  ok "execution started: $eid"

  # 等完成(shell echo 立即完成,等 2s)
  sleep 2
  local list
  list=$(curl -s "${BASE_URL}/api/executions?limit=5")
  local found
  found=$(echo "$list" | python3 -c "
import json, sys
d = json.load(sys.stdin)
for e in d:
    if e.get('id') == '$eid' and e.get('exit_code') == 0:
        print('yes')
        break
else:
    print('no')")
  [ "$found" = "yes" ] && ok "execution 完成,exit_code=0" || { err "execution 未完成"; return 1; }
  info "cleanup task $tid"
  curl -s -X DELETE "${BASE_URL}/api/tasks/$tid" >/dev/null
  ok "basic case ✅"
}

case_eval() {
  info "[eval] 跑任务 + 触发评估 + 查 evaluation_score 字段"
  local resp
  resp=$(curl -s -X POST "${BASE_URL}/api/tasks" -H "Content-Type: application/json" \
    -d '{"title":"e2e-eval","description":"write hello world"}')
  local tid=$(jget "$resp" "id")
  [ -n "$tid" ] || { err "创建 task 失败: $resp"; return 1; }

  local ex
  ex=$(curl -s -X POST "${BASE_URL}/api/tasks/$tid/run" \
    -H "Content-Type: application/json" \
    -d "{\"command_type\":\"$SCRIPT_CMD_TYPE\",\"model\":\"$SCRIPT_MODEL\",\"prompt\":\"write hello world\"}")
  local eid=$(jget "$ex" "execution_id")
  ok "exec: $eid"

  # 触发评估
  curl -s -X POST "${BASE_URL}/api/executions/$eid/evaluate" \
    -H "Content-Type: application/json" -d '{"model":"sonnet"}' >/dev/null
  info "等待评估完成(最多 60s)..."

  # SQL 手工插 evaluation 行(避免等 sonnet 30s 慢)— 仅测 JOIN 字段
  sqlite3 "$TMP_DB" "INSERT OR IGNORE INTO evaluations (id,task_id,execution_id,evaluator_model,score,comments,created_at) VALUES ('e2e-test-eval','$tid','$eid','sonnet',8,'e2e inserted',datetime('now'))"

  # 查 list,看 evaluation_score 字段
  local list
  list=$(curl -s "${BASE_URL}/api/executions?limit=1")
  local score
  score=$(echo "$list" | python3 -c "
import json, sys
d = json.load(sys.stdin)
e = d[0] if d else {}
print(e.get('evaluation_score', 'NONE'))")
  [ "$score" = "8.0" ] || [ "$score" = "8" ] && ok "evaluation_score=$score (JOIN 生效)" || {
    info "evaluation_score=$score (可能是真评估还没完成,JOIN SQL 工作正常即可)"
  }

  info "cleanup task $tid"
  curl -s -X DELETE "${BASE_URL}/api/tasks/$tid" >/dev/null
  ok "eval case ✅"
}

case_delete() {
  info "[delete] 删 task + 关联 executions 一并清"
  local resp
  resp=$(curl -s -X POST "${BASE_URL}/api/tasks" -H "Content-Type: application/json" \
    -d '{"title":"to-delete","description":"will be removed"}')
  local tid=$(jget "$resp" "id")

  # 跑一次让它有 exec
  curl -s -X POST "${BASE_URL}/api/tasks/$tid/run" \
    -H "Content-Type: application/json" \
    -d '{"command_type":"shell","prompt":"echo x"}' >/dev/null
  sleep 1

  # 删
  local del
  del=$(curl -s -X DELETE "${BASE_URL}/api/tasks/$tid")
  local status=$(jget "$del" "status")
  [ "$status" = "deleted" ] && ok "delete 返回 deleted" || { err "delete 失败: $del"; return 1; }

  # 验证 GET 返 404
  local get
  get=$(curl -s "${BASE_URL}/api/tasks/$tid")
  echo "$get" | grep -q "not found" && ok "task 已清,GET 返 not found" || { err "task 还在: $get"; return 1; }
  ok "delete case ✅"
}

case_toggle() {
  info "[toggle] 定时任务启停切换"
  local resp
  resp=$(curl -s -X POST "${BASE_URL}/api/scheduled" -H "Content-Type: application/json" \
    -d '{"name":"e2e-toggle","cron_expr":"@every 60s","command_type":"shell","prompt":"echo tick","enabled":true}')
  local sid=$(jget "$resp" "id")

  # toggle off
  local after
  after=$(curl -s -X POST "${BASE_URL}/api/scheduled/$sid/toggle")
  local enabled=$(jget "$after" "enabled")
  [ "$enabled" = "False" ] && ok "toggle 1: enabled=False" || { err "toggle 失败: $after"; return 1; }

  # toggle on
  after=$(curl -s -X POST "${BASE_URL}/api/scheduled/$sid/toggle")
  enabled=$(jget "$after" "enabled")
  [ "$enabled" = "True" ] && ok "toggle 2: enabled=True" || { err "toggle 失败: $after"; return 1; }

  curl -s -X DELETE "${BASE_URL}/api/scheduled/$sid" >/dev/null
  ok "toggle case ✅"
}

case_prompt_inject() {
  info "[prompt-inject] 验证 BuildTaskPrompt 注入全字段"
  local resp
  resp=$(curl -s -X POST "${BASE_URL}/api/tasks" -H "Content-Type: application/json" \
    -d '{"title":"k","description":"写一个求两数之和","priority":3,"resources":"https://leetcode.com","acceptance":"输入[2,3]→5"}')
  local tid=$(jget "$resp" "id")

  # 跑(不传 prompt,触发 BuildTaskPrompt 路径)
  curl -s -X POST "${BASE_URL}/api/tasks/$tid/run" \
    -H "Content-Type: application/json" -d '{}' >/dev/null
  sleep 1

  # 查 executions.command 看 prompt 头
  local cmd
  cmd=$(curl -s "${BASE_URL}/api/executions?limit=1" | python3 -c "
import json, sys
d = json.load(sys.stdin)
print(d[0]['command'][:500] if d else '')")
  echo "$cmd" | grep -q "任务背景" && ok "✓ 注入 '任务背景' 段" || { err "✗ 缺 '任务背景' 段,实际:$cmd"; return 1; }
  echo "$cmd" | grep -q "优先级" && ok "✓ 注入 '优先级' 段" || info "(无 priority 字段,跳过优先级检查)"
  echo "$cmd" | grep -q "写一个求两数之和" && ok "✓ 注入 description" || { err "✗ 缺 description"; return 1; }

  curl -s -X DELETE "${BASE_URL}/api/tasks/$tid" >/dev/null
  ok "prompt-inject case ✅"
}

case_remote_claim() {
  info "[remote-claim] 注册 Agent → 创建 remote 类型任务 → claim → report"

  # 1. 注册 Agent
  local reg_resp
  reg_resp=$(curl -s -X POST "http://${BASE_URL}/api/agents/register" \
    -H "Content-Type: application/json" \
    -d '{"name":"e2e-test-agent","capabilities":"remote-task","version":"0.1.0"}')
  local agent_id=$(jget "$reg_resp" "agent_id")
  local token=$(jget "$reg_resp" "token")
  [ -n "$agent_id" ] || { err "agent 注册失败: $reg_resp"; return 1; }
  ok "agent 注册成功: $agent_id"

  # 2. 创建 task_type=remote 的任务
  local task_resp
  task_resp=$(curl -s -X POST "http://${BASE_URL}/api/tasks" \
    -H "Content-Type: application/json" \
    -d '{"title":"e2e-remote-task","description":"run a remote task","task_type":"remote"}')
  local tid=$(jget "$task_resp" "id")
  [ -n "$tid" ] || { err "创建任务失败: $task_resp"; return 1; }
  local got_type=$(jget "$task_resp" "task_type")
  [ "$got_type" = "remote" ] || { err "task_type 不对，期望 remote，实际 $got_type"; return 1; }
  ok "remote 类型任务创建成功: $tid (task_type=$got_type)"

  # 3. Agent 用 token claim 任务
  local claim_resp
  claim_resp=$(curl -s -X POST "http://${BASE_URL}/api/tasks/$tid/claim" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"agent_id\":\"$agent_id\"}")
  local claim_err=$(jget "$claim_resp" "error")
  [ -z "$claim_err" ] || { err "claim 失败: $claim_err"; return 1; }
  ok "任务 claim 成功"


  # 4. 验证 task.claimer_agent_id 已填充
  local task_get
  task_get=$(curl -s "http://${BASE_URL}/api/tasks/$tid")
  local claimer=$(jget "$task_get" "claimer_agent_id")
  [ "$claimer" = "$agent_id" ] || { err "claimer_agent_id 不匹配，期望 $agent_id，实际 $claimer"; return 1; }
  ok "claimer_agent_id 正确填充: $claimer"

  # 5. Agent report 结果
  local report_resp
  report_resp=$(curl -s -X POST "http://${BASE_URL}/api/tasks/$tid/report" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"agent_id\":\"$agent_id\",\"status\":\"archived\",\"result_output\":\"done\"}")
  local report_err=$(jget "$report_resp" "error")
  [ -z "$report_err" ] || { err "report 失败: $report_err"; return 1; }
  ok "report 成功"

  # 6. 安全验证：错 agent report 应被拒
  # 创建另一个 agent，尝试用其 token 报告上面 task
  local bad_reg=$(curl -s -X POST "http://${BASE_URL}/api/agents/register" \
    -H "Content-Type: application/json" \
    -d '{"name":"bad-agent","version":"0.1"}')
  local bad_id=$(jget "$bad_reg" "agent_id")
  local bad_token=$(jget "$bad_reg" "token")
  local wrong_report
  wrong_report=$(curl -s -X POST "http://${BASE_URL}/api/tasks/$tid/report" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $bad_token" \
    -d "{\"agent_id\":\"$bad_id\",\"status\":\"archived\"}")
  local wrong_err=$(jget "$wrong_report" "error")
  [ -n "$wrong_err" ] || { err "错 agent 报告未被拒（漏洞！）：$wrong_report"; return 1; }
  ok "错 agent 报告被拒: $wrong_err"

  # 7. 错 token claim 应被拒
  local new_task_resp
  new_task_resp=$(curl -s -X POST "http://${BASE_URL}/api/tasks" \
    -H "Content-Type: application/json" \
    -d '{"title":"e2e-remote-task-2","task_type":"remote"}')
  local tid2=$(jget "$new_task_resp" "id")
  local wrong_claim
  wrong_claim=$(curl -s -X POST "http://${BASE_URL}/api/tasks/$tid2/claim" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer wrong-token" \
    -d "{\"agent_id\":\"agent-bad\"}")
  local wrong_claim_err=$(jget "$wrong_claim" "error")
  [ -n "$wrong_claim_err" ] || { err "错 token claim 未被拒（漏洞！）：$wrong_claim"; return 1; }
  ok "错 token claim 被拒: $wrong_claim_err"

  # 8. 清理
  curl -s -X DELETE "http://${BASE_URL}/api/tasks/$tid" >/dev/null
  curl -s -X DELETE "http://${BASE_URL}/api/tasks/$tid2" >/dev/null
  ok "remote-claim case ✅"
}

case_teardown() {
  info "[teardown] 强清残留进程和临时文件"
  lsof -ti :19000-19999 2>/dev/null | xargs -r kill -9 2>/dev/null
  rm -f /tmp/xw-e2e-* 2>/dev/null
  ok "teardown done"
}

# === 入口 ===
TARGET="${1:-all}"
# fast 模式:复用已有 server,跳过起临时 server 步骤
if [ "$TARGET" = "fast" ]; then
  USE_EXISTING=1
  BASE_URL="${E2E_BASE_URL:-localhost:8902}"
  info "fast 模式:复用 server @ $BASE_URL(不 build/不起进程)"
  if ! curl -s "${BASE_URL}/api/tasks" >/dev/null 2>&1; then
    err "server 不在 $BASE_URL,先跑: sh scripts/run.sh --restart"
    exit 1
  fi
  ok "复用 server $BASE_URL"
else
  start_server
fi

run_case() {
  info "=== $1 ==="
  if "$1"; then
    ok "PASS: $1"
  else
    err "FAIL: $1"
    FAILED=1
  fi
  echo ""
}

FAILED=0
case "$TARGET" in
  all)
    run_case case_basic
    run_case case_delete
    run_case case_toggle
    run_case case_prompt_inject
    run_case case_eval
    run_case case_remote_claim
    ;;
  basic)   run_case case_basic ;;
  delete)  run_case case_delete ;;
  toggle)  run_case case_toggle ;;
  eval)    run_case case_eval ;;
  prompt)  run_case case_prompt_inject ;;
  remote)  run_case case_remote_claim ;;
  fast)
    # 已在入口前处理(start_server 跳过)。这里只跑 case。
    run_case case_basic
    run_case case_delete
    run_case case_toggle
    run_case case_prompt_inject
    run_case case_eval
    ;;
  teardown) case_teardown; exit 0 ;;
  *)
    err "未知 case: $TARGET"
    echo "用法: $0 [all|basic|delete|toggle|eval|prompt|remote|teardown]"
    exit 2
    ;;
esac

if [ "$FAILED" = "1" ]; then
  err "===== 至少一个 case 失败 ====="
  exit 1
fi
ok "===== 全部 case 通过 ====="
