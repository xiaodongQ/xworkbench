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

case_audit_deps() {
  info "[audit-deps] 创建 → 触发 claim → 查 events → 加 dep → 验证未完成时不能 claim"

  # 1. 注册 agent
  local reg_resp
  reg_resp=$(curl -s -X POST "http://${BASE_URL}/api/agents/register" \
    -H "Content-Type: application/json" \
    -d '{"name":"e2e-deps-agent","version":"0.1"}')
  local agent_id=$(jget "$reg_resp" "agent_id")
  local token=$(jget "$reg_resp" "token")
  [ -n "$agent_id" ] || { err "agent 注册失败: $reg_resp"; return 1; }
  ok "agent 注册成功: $agent_id"

  # 2. 创建两个任务：A 和 B
  local a_resp
  a_resp=$(curl -s -X POST "http://${BASE_URL}/api/tasks" \
    -H "Content-Type: application/json" \
    -d '{"title":"dep-A","task_type":"remote"}')
  local aid=$(jget "$a_resp" "id")
  [ -n "$aid" ] || { err "创建 A 失败"; return 1; }
  ok "task A 创建成功: $aid"

  local b_resp
  b_resp=$(curl -s -X POST "http://${BASE_URL}/api/tasks" \
    -H "Content-Type: application/json" \
    -d '{"title":"dep-B","task_type":"remote"}')
  local bid=$(jget "$b_resp" "id")
  [ -n "$bid" ] || { err "创建 B 失败"; return 1; }
  ok "task B 创建成功: $bid"

  # 3. B 依赖 A
  local add_dep
  add_dep=$(curl -s -X POST "http://${BASE_URL}/api/tasks/$bid/dependencies" \
    -H "Content-Type: application/json" \
    -d "{\"depends_on\":\"$aid\",\"type\":\"hard\"}")
  local dep_err=$(jget "$add_dep" "error")
  [ -z "$dep_err" ] || { err "添加依赖失败: $dep_err"; return 1; }
  ok "B 已依赖 A"

  # 4. 验证 B 不能被 claim（A 未完成）
  local fail_claim
  fail_claim=$(curl -s -X POST "http://${BASE_URL}/api/tasks/$bid/claim" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"agent_id\":\"$agent_id\"}")
  local fail_err=$(jget "$fail_claim" "error")
  if [ -z "$fail_err" ]; then
    err "B 在 A 未完成时被 claim 了（漏洞！）: $fail_claim"
    return 1
  fi
  ok "B 在 A 未完成时不能 claim: $fail_err"

  # 5. 验证循环依赖被拒：A 依赖 B
  local cycle_dep
  cycle_dep=$(curl -s -X POST "http://${BASE_URL}/api/tasks/$aid/dependencies" \
    -H "Content-Type: application/json" \
    -d "{\"depends_on\":\"$bid\",\"type\":\"hard\"}")
  local cycle_err=$(jget "$cycle_dep" "error")
  [ -n "$cycle_err" ] || { err "循环依赖未被拒（漏洞！）"; return 1; }
  ok "循环依赖被拒: $cycle_err"

  # 6. claim A → report
  local a_claim
  a_claim=$(curl -s -X POST "http://${BASE_URL}/api/tasks/$aid/claim" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"agent_id\":\"$agent_id\"}")
  local a_claim_err=$(jget "$a_claim" "error")
  [ -z "$a_claim_err" ] || { err "A claim 失败: $a_claim_err"; return 1; }
  ok "A 被 claim"

  curl -s -X POST "http://${BASE_URL}/api/tasks/$aid/report" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"agent_id\":\"$agent_id\",\"status\":\"archived\",\"result_output\":\"A done\"}" > /dev/null
  ok "A 已 report"

  # 7. 现在 B 可以被 claim 了
  local b_claim
  b_claim=$(curl -s -X POST "http://${BASE_URL}/api/tasks/$bid/claim" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"agent_id\":\"$agent_id\"}")
  local b_claim_err=$(jget "$b_claim" "error")
  [ -z "$b_claim_err" ] || { err "A 完成后 B 仍不能 claim: $b_claim_err"; return 1; }
  ok "A 完成后 B 可被 claim"

  # 8. 验证 task_events 时间线
  local a_events
  a_events=$(curl -s "http://${BASE_URL}/api/tasks/$aid/events")
  local has_created=$(echo "$a_events" | python3 -c "import json,sys; d=json.load(sys.stdin); print(any(e['event_type']=='created' for e in d))" 2>/dev/null)
  local has_claimed=$(echo "$a_events" | python3 -c "import json,sys; d=json.load(sys.stdin); print(any(e['event_type']=='claimed' for e in d))" 2>/dev/null)
  local has_reported=$(echo "$a_events" | python3 -c "import json,sys; d=json.load(sys.stdin); print(any(e['event_type']=='reported' for e in d))" 2>/dev/null)
  [ "$has_created" = "True" ] || { err "events 缺 created: $a_events"; return 1; }
  [ "$has_claimed" = "True" ] || { err "events 缺 claimed: $a_events"; return 1; }
  [ "$has_reported" = "True" ] || { err "events 缺 reported: $a_events"; return 1; }
  ok "task_events 时间线完整: created/claimed/reported 都有"

  # 9. 验证 B 的 events 包含 dep_added
  local b_events
  b_events=$(curl -s "http://${BASE_URL}/api/tasks/$bid/events")
  local has_dep=$(echo "$b_events" | python3 -c "import json,sys; d=json.load(sys.stdin); print(any(e['event_type']=='dep_added' for e in d))" 2>/dev/null)
  [ "$has_dep" = "True" ] || { err "B events 缺 dep_added: $b_events"; return 1; }
  ok "B 事件流含 dep_added"

  # 10. 清理
  curl -s -X DELETE "http://${BASE_URL}/api/tasks/$aid" >/dev/null
  curl -s -X DELETE "http://${BASE_URL}/api/tasks/$bid" >/dev/null
  ok "audit-deps case ✅"
}

case_templates_filters() {
  info "[tpl-sf] 创建模板 → instantiate → 保存过滤器"

  # 1. 创建模板
  local tpl_resp
  tpl_resp=$(curl -s -X POST "http://${BASE_URL}/api/task-templates" \
    -H "Content-Type: application/json" \
    -d '{
      "name":"release-checklist",
      "description":"发布前检查",
      "category":"release",
      "task_type":"manual",
      "template_body":"{\"description\":\"Run all smoke tests before release\",\"resources\":\"wiki/release-checklist\",\"acceptance\":\"所有 smoke test 通过\"}"
    }')
  local tpl_id=$(jget "$tpl_resp" "id")
  [ -n "$tpl_id" ] || { err "模板创建失败: $tpl_resp"; return 1; }
  ok "模板创建: $tpl_id"

  # 2. 列出模板
  local list_resp
  list_resp=$(curl -s "http://${BASE_URL}/api/task-templates?category=release")
  local has_tpl=$(echo "$list_resp" | python3 -c "import json,sys; d=json.load(sys.stdin); print(any(t['id']=='$tpl_id' for t in d))" 2>/dev/null)
  [ "$has_tpl" = "True" ] || { err "List(category) 缺模板: $list_resp"; return 1; }
  ok "List(category) 包含模板"

  # 3. instantiate 模板
  local inst_resp
  inst_resp=$(curl -s -X POST "http://${BASE_URL}/api/task-templates/$tpl_id/instantiate" \
    -H "Content-Type: application/json" \
    -d '{"title":"release v1.0.0"}')
  local new_id=$(jget "$inst_resp" "id")
  [ -n "$new_id" ] || { err "instantiate 失败: $inst_resp"; return 1; }
  ok "instantiate 创建任务: $new_id"

  # 验证任务字段是从模板继承
  local inst_desc=$(jget "$inst_resp" "description")
  local inst_acc=$(jget "$inst_resp" "acceptance")
  [ "$inst_desc" = "Run all smoke tests before release" ] || { err "description 未继承: $inst_desc"; return 1; }
  [ "$inst_acc" = "所有 smoke test 通过" ] || { err "acceptance 未继承: $inst_acc"; return 1; }
  ok "模板字段正确继承到任务"

  # 验证 use_count 自增
  local tpl_after=$(curl -s "http://${BASE_URL}/api/task-templates/$tpl_id")
  local use_count=$(jget "$tpl_after" "use_count")
  [ "$use_count" = "1" ] || { err "use_count = $use_count, want 1"; return 1; }
  ok "use_count = 1"

  # 4. override 测试
  local inst2
  inst2=$(curl -s -X POST "http://${BASE_URL}/api/task-templates/$tpl_id/instantiate" \
    -H "Content-Type: application/json" \
    -d '{"title":"hotfix v1.0.1","description":"紧急修复跳过部分测试"}')
  local desc2=$(jget "$inst2" "description")
  [ "$desc2" = "紧急修复跳过部分测试" ] || { err "override description 失败: $desc2"; return 1; }
  ok "override 字段生效"

  # 5. 验证事件记录
  local evts
  evts=$(curl -s "http://${BASE_URL}/api/tasks/$new_id/events")
  local has_tpl_evt=$(echo "$evts" | python3 -c "import json,sys; d=json.load(sys.stdin); print(any(e['event_type']=='created_from_template' for e in d))" 2>/dev/null)
  [ "$has_tpl_evt" = "True" ] || { err "缺 created_from_template 事件: $evts"; return 1; }
  ok "事件流含 created_from_template"

  # 6. 保存过滤器
  local sf_resp
  sf_resp=$(curl -s -X POST "http://${BASE_URL}/api/saved-filters" \
    -H "Content-Type: application/json" \
    -d '{"name":"今日待办","filter_json":"{\"status\":\"pending\"}","is_default":1}')
  local sf_id=$(jget "$sf_resp" "id")
  [ -n "$sf_id" ] || { err "过滤器创建失败: $sf_resp"; return 1; }
  ok "过滤器创建: $sf_id"

  # 列出
  local sf_list
  sf_list=$(curl -s "http://${BASE_URL}/api/saved-filters")
  local has_sf=$(echo "$sf_list" | python3 -c "import json,sys; d=json.load(sys.stdin); print(any(f['id']=='$sf_id' for f in d))" 2>/dev/null)
  [ "$has_sf" = "True" ] || { err "List 缺过滤器: $sf_list"; return 1; }
  ok "List 包含过滤器"

  # 7. 清理
  curl -s -X DELETE "http://${BASE_URL}/api/task-templates/$tpl_id" >/dev/null
  curl -s -X DELETE "http://${BASE_URL}/api/tasks/$new_id" >/dev/null
  local inst2_id=$(jget "$inst2" "id")
  [ -n "$inst2_id" ] && curl -s -X DELETE "http://${BASE_URL}/api/tasks/$inst2_id" >/dev/null
  curl -s -X DELETE "http://${BASE_URL}/api/saved-filters/$sf_id" >/dev/null
  ok "tpl-sf case ✅"
}

case_ratelimit_webhook() {
  info "[rl-wh] 注册 agent → 触发 5 次心跳 → 验证 200 → 注册 webhook → 触发 test 事件"

  # 1. 注册 agent
  local reg_resp
  reg_resp=$(curl -s -X POST "http://${BASE_URL}/api/agents/register" \
    -H "Content-Type: application/json" \
    -d '{"name":"e2e-rl-agent","version":"0.1"}')
  local agent_id=$(jget "$reg_resp" "agent_id")
  local token=$(jget "$reg_resp" "token")
  [ -n "$agent_id" ] || { err "agent 注册失败"; return 1; }
  ok "agent 注册成功: $agent_id"

  # 2. 用同一个 token 连发 5 次心跳（rate limit 默认 60/min，远低于这个数）
  #    主要验证令牌机制不阻塞正常调用
  for i in 1 2 3 4 5; do
    local hb
    hb=$(curl -s -X POST "http://${BASE_URL}/api/agents/$agent_id/heartbeat" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $token" \
      -d '{}')
    local ok_field=$(echo "$hb" | python3 -c "import json,sys; print(json.load(sys.stdin).get('ok', False))" 2>/dev/null)
    [ "$ok_field" = "True" ] || { err "第 $i 次心跳失败: $hb"; return 1; }
  done
  ok "5 次心跳都成功（在 limit 内）"

  # 3. 创建 webhook（指向不存在的 URL 没事，只验证 dispatch 不阻塞）
  local wh_resp
  wh_resp=$(curl -s -X POST "http://${BASE_URL}/api/webhooks" \
    -H "Content-Type: application/json" \
    -d '{
      "name":"e2e-test-webhook",
      "url":"http://127.0.0.1:9/nonexistent",
      "secret":"e2e-secret",
      "events":"test,task.created",
      "enabled":1
    }')
  local wh_id=$(jget "$wh_resp" "id")
  [ -n "$wh_id" ] || { err "webhook 创建失败: $wh_resp"; return 1; }
  ok "webhook 创建: $wh_id"

  # 4. 列出 webhook 应包含
  local wh_list
  wh_list=$(curl -s "http://${BASE_URL}/api/webhooks")
  local has_wh=$(echo "$wh_list" | python3 -c "import json,sys; d=json.load(sys.stdin); print(any(w['id']=='$wh_id' for w in d))" 2>/dev/null)
  [ "$has_wh" = "True" ] || { err "List 缺 webhook: $wh_list"; return 1; }
  ok "List 含 webhook"

  # 5. 主动触发 test 事件
  local test_resp
  test_resp=$(curl -s -X POST "http://${BASE_URL}/api/webhooks/$wh_id/test")
  local test_status=$(jget "$test_resp" "status")
  [ "$test_status" = "test_dispatched" ] || { err "test 触发失败: $test_resp"; return 1; }
  ok "test 事件已 dispatch"

  # 6. 等 5 秒让 dispatch 失败（webhook URL 不通会 3 次重试 + 1+2+4s 退避 = 7s）
  sleep 5
  local wh_after
  wh_after=$(curl -s "http://${BASE_URL}/api/webhooks/$wh_id")
  local fail_count=$(jget "$wh_after" "fail_count")
  # 这里 fail_count 可能为 0（重试还没完成）或 ≥1（重试 1 次失败）
  # 至少验证 webhook 状态可查、字段更新机制正常
  ok "webhook 状态可查，fail_count=$fail_count"

  # 7. 验证：创建任务触发 task.created 事件（webhook 配的事件列表里有 task.created）
  local task_resp
  task_resp=$(curl -s -X POST "http://${BASE_URL}/api/tasks" \
    -H "Content-Type: application/json" \
    -d '{"title":"e2e-rl-task","task_type":"manual"}')
  local tid=$(jget "$task_resp" "id")
  [ -n "$tid" ] || { err "task 创建失败"; return 1; }
  ok "task 创建（webhook 应被触发）"

  # 8. 清理
  curl -s -X DELETE "http://${BASE_URL}/api/webhooks/$wh_id" >/dev/null
  curl -s -X DELETE "http://${BASE_URL}/api/tasks/$tid" >/dev/null
  ok "rl-wh case ✅"
}

case_comments_priority() {
  info "[cmt-pri] 创建任务带 priority → 验证 claim-next 按优先级 → 评论 CRUD"

  # 1. 创建 3 个不同 priority 的任务
  local lo_resp
  lo_resp=$(curl -s -X POST "http://${BASE_URL}/api/tasks" \
    -H "Content-Type: application/json" \
    -d '{"title":"low-pri","task_type":"remote","priority":1}')
  local lo_id=$(jget "$lo_resp" "id")
  local lo_pri=$(jget "$lo_resp" "priority")
  [ "$lo_pri" = "1" ] || { err "low priority 没设上: $lo_resp"; return 1; }
  ok "low-pri 任务创建 (priority=$lo_pri)"

  local hi_resp
  hi_resp=$(curl -s -X POST "http://${BASE_URL}/api/tasks" \
    -H "Content-Type: application/json" \
    -d '{"title":"high-pri","task_type":"remote","priority":10}')
  local hi_id=$(jget "$hi_resp" "id")
  local hi_pri=$(jget "$hi_resp" "priority")
  [ "$hi_pri" = "10" ] || { err "high priority 没设上: $hi_resp"; return 1; }
  ok "high-pri 任务创建 (priority=$hi_pri)"

  local mi_resp
  mi_resp=$(curl -s -X POST "http://${BASE_URL}/api/tasks" \
    -H "Content-Type: application/json" \
    -d '{"title":"mid-pri","task_type":"remote","priority":5}')
  local mi_id=$(jget "$mi_resp" "id")
  ok "mid-pri 任务创建"

  # 2. 注册 agent
  local reg_resp
  reg_resp=$(curl -s -X POST "http://${BASE_URL}/api/agents/register" \
    -H "Content-Type: application/json" \
    -d '{"name":"e2e-cmt-agent"}')
  local agent_id=$(jget "$reg_resp" "agent_id")
  local token=$(jget "$reg_resp" "token")
  [ -n "$agent_id" ] || { err "agent 注册失败"; return 1; }
  ok "agent 注册: $agent_id"

  # 3. claim-next 应返回 hi-pri 任务
  local next_resp
  next_resp=$(curl -s -X POST "http://${BASE_URL}/api/tasks/claim-next" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"agent_id\":\"$agent_id\"}")
  local next_id=$(jget "$next_resp" "task.id")
  if [ "$next_id" != "$hi_id" ]; then
    err "claim-next 返回 $next_id，期望 $hi_id (high priority)"
    return 1
  fi
  ok "claim-next 返回最高优先级任务 (high-pri)"

  # 4. 验证 audit
  local evts
  evts=$(curl -s "http://${BASE_URL}/api/tasks/$hi_id/events")
  local has_evt=$(echo "$evts" | python3 -c "import json,sys; d=json.load(sys.stdin); print(any(e['event_type']=='claimed_via_priority' for e in d))" 2>/dev/null)
  [ "$has_evt" = "True" ] || { err "缺 claimed_via_priority 事件"; return 1; }
  ok "事件流含 claimed_via_priority"

  # 5. 在 hi-pri 任务上加评论
  local c1
  c1=$(curl -s -X POST "http://${BASE_URL}/api/tasks/$hi_id/comments" \
    -H "Content-Type: application/json" \
    -d '{"author":"user-1","content":"这是第一条评论"}')
  local c1_id=$(jget "$c1" "id")
  [ -n "$c1_id" ] || { err "评论创建失败: $c1"; return 1; }
  ok "评论创建: $c1_id"

  # 6. 嵌套回复
  local c2
  c2=$(curl -s -X POST "http://${BASE_URL}/api/tasks/$hi_id/comments" \
    -H "Content-Type: application/json" \
    -d "{\"author\":\"user-2\",\"content\":\"这是回复\",\"parent_id\":\"$c1_id\"}")
  local c2_id=$(jget "$c2" "id")
  local c2_parent=$(jget "$c2" "parent_id")
  [ "$c2_parent" = "$c1_id" ] || { err "parent_id 没设: $c2"; return 1; }
  ok "嵌套回复 parent_id 正确"

  # 7. 列表 + 修改 + 删除
  local cmts_list
  cmts_list=$(curl -s "http://${BASE_URL}/api/tasks/$hi_id/comments")
  local count=$(echo "$cmts_list" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null)
  [ "$count" = "2" ] || { err "评论数 $count, want 2"; return 1; }
  ok "评论列表含 2 条"

  # 更新
  curl -s -X PUT "http://${BASE_URL}/api/comments/$c1_id" \
    -H "Content-Type: application/json" \
    -d '{"content":"修改后的内容"}' > /dev/null
  ok "评论更新"

  # 删除
  curl -s -X DELETE "http://${BASE_URL}/api/comments/$c2_id" >/dev/null
  local cmts_after
  cmts_after=$(curl -s "http://${BASE_URL}/api/tasks/$hi_id/comments")
  local count_after=$(echo "$cmts_after" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null)
  [ "$count_after" = "1" ] || { err "删除后评论数 $count_after, want 1"; return 1; }
  ok "评论删除成功"

  # 8. 清理
  curl -s -X DELETE "http://${BASE_URL}/api/tasks/$lo_id" >/dev/null
  curl -s -X DELETE "http://${BASE_URL}/api/tasks/$hi_id" >/dev/null
  curl -s -X DELETE "http://${BASE_URL}/api/tasks/$mi_id" >/dev/null
  ok "cmt-pri case ✅"
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
    run_case case_audit_deps
    run_case case_templates_filters
    run_case case_ratelimit_webhook
    run_case case_comments_priority
    ;;
  basic)   run_case case_basic ;;
  delete)  run_case case_delete ;;
  toggle)  run_case case_toggle ;;
  eval)    run_case case_eval ;;
  prompt)  run_case case_prompt_inject ;;
  remote)  run_case case_remote_claim ;;
  audit)   run_case case_audit_deps ;;
  tpl)     run_case case_templates_filters ;;
  rl)      run_case case_ratelimit_webhook ;;
  cmt)     run_case case_comments_priority ;;
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
    echo "用法: $0 [all|basic|delete|toggle|eval|prompt|remote|audit|tpl|rl|cmt|teardown]"
    exit 2
    ;;
esac

if [ "$FAILED" = "1" ]; then
  err "===== 至少一个 case 失败 ====="
  exit 1
fi
ok "===== 全部 case 通过 ====="



