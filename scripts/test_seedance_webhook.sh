#!/usr/bin/env bash
#
# scripts/test_seedance_webhook.sh — Seedance webhook 模式一键自测（方案一：本地手动模拟）
#
# 测什么：
#   1. 提交任务（带 callback_url）→ 命中 webhook 门控，任务进入 webhook 模式；
#   2. 手动模拟上游回调（中间态 RUNNING → 终态 SUCCESS）→ 网关落终态、差额结算、
#      回调下游；
#   3. 本地监听器收到网关的下游回调 payload 并打印。
#
# 原理：不需要真实上游回调网关——脚本自己 curl 网关的 webhook 端点模拟上游。唯一
# 需要"骗过"的是门控里的公网 ServerAddress 校验；提供 AMUX_ADMIN_KEY 时脚本会
# 临时把 ServerAddress 设成一个公网样子的假地址，退出时自动还原。
#
# 用法：
#   AMUX_API_KEY=sk-xxx ./scripts/test_seedance_webhook.sh
#   AMUX_API_KEY=sk-xxx AMUX_ADMIN_KEY=admin-xxx ./scripts/test_seedance_webhook.sh
#
# 环境变量：
#   AMUX_BASE_URL    网关地址                默认 http://127.0.0.1:3000
#   AMUX_API_KEY     用户 token              必填（模型需已启用、账户有余额）
#   AMUX_ADMIN_KEY   admin token             可选；提供则自动开 SeedanceWebhookEnabled、
#                                             临时改 ServerAddress 并设一个随机
#                                             SeedanceWebhookSecret，退出时一并还原
#   AMUX_WEBHOOK_SECRET  回调 HMAC 签名密钥  可选；须与后台 SeedanceWebhookSecret 一致
#                                             （未提供且给了 ADMIN_KEY 时脚本自动配置随机
#                                             密钥并用于签名）
#   AMUX_MODEL       模型                    默认 seedance-2.5（官渠 2.5）；ZeroCut 渠道
#                                             请换成 seedance-2.0-api 之类
#   AMUX_PORT        下游监听端口            默认 9001
#   AMUX_FAKE_ADDR   假公网地址（仅门控用）  默认 https://amux-webhook-test.example.com
#
# 依赖：curl、python3（用来起监听器 + 解析 JSON，不依赖 jq）

set -euo pipefail

BASE="${AMUX_BASE_URL:-http://127.0.0.1:3000}"
BASE="${BASE%/}"
API_KEY="${AMUX_API_KEY:-}"
ADMIN_KEY="${AMUX_ADMIN_KEY:-}"
SECRET="${AMUX_WEBHOOK_SECRET:-}"
MODEL="${AMUX_MODEL:-seedance-2.5}"
PORT="${AMUX_PORT:-9001}"
FAKE_ADDR="${AMUX_FAKE_ADDR:-https://amux-webhook-test.example.com}"

if [ -z "$API_KEY" ]; then
  echo "缺少 AMUX_API_KEY（用户 token）。用法见脚本头部注释。" >&2
  exit 2
fi
command -v curl >/dev/null || { echo "缺少 curl" >&2; exit 2; }
command -v python3 >/dev/null || { echo "缺少 python3" >&2; exit 2; }

# 颜色（非 tty 时关掉）
if [ -t 1 ]; then
  C_OK=$'\033[32m'; C_FAIL=$'\033[31m'; C_INFO=$'\033[36m'; C_WARN=$'\033[33m'; C_RST=$'\033[0m'
else
  C_OK=""; C_FAIL=""; C_INFO=""; C_WARN=""; C_RST=""
fi
pass() { echo "${C_OK}[PASS]${C_RST} $*"; }
fail() { echo "${C_FAIL}[FAIL]${C_RST} $*"; }
info() { echo "${C_INFO}[INFO]${C_RST} $*"; }
warn() { echo "${C_WARN}[WARN]${C_RST} $*"; }

TMP="$(mktemp -d /tmp/seedance_webhook_test.XXXXXX)"
CALLBACK_DIR="$TMP/callbacks"
mkdir -p "$CALLBACK_DIR"
LISTENER_PID=""
RESTORE_ENABLED=""   # 非空表示退出时要还原后台配置
OLD_ADDR=""; OLD_ENABLED=""; OLD_SECRET=""

cleanup() {
  # 还原后台配置（只在真的改过时才动）
  if [ -n "$RESTORE_ENABLED" ] && [ -n "$ADMIN_KEY" ]; then
    info "还原后台配置 ..."
    curl -sS -o /dev/null -X PUT "$BASE/api/option/" \
      -H "Authorization: Bearer $ADMIN_KEY" -H 'Content-Type: application/json' \
      -d "{\"key\":\"SeedanceWebhookEnabled\",\"value\":$OLD_ENABLED}" || true
    curl -sS -o /dev/null -X PUT "$BASE/api/option/" \
      -H "Authorization: Bearer $ADMIN_KEY" -H 'Content-Type: application/json' \
      -d "{\"key\":\"ServerAddress\",\"value\":\"$OLD_ADDR\"}" || true
    curl -sS -o /dev/null -X PUT "$BASE/api/option/" \
      -H "Authorization: Bearer $ADMIN_KEY" -H 'Content-Type: application/json' \
      -d "{\"key\":\"SeedanceWebhookSecret\",\"value\":\"$OLD_SECRET\"}" || true
    warn "已还原 SeedanceWebhookEnabled=$OLD_ENABLED, ServerAddress=\"$OLD_ADDR\", SeedanceWebhookSecret=$([ -z "$OLD_SECRET" ] && echo '(空)' || echo '***')"
  fi
  if [ -n "$LISTENER_PID" ]; then
    kill "$LISTENER_PID" 2>/dev/null || true
    wait "$LISTENER_PID" 2>/dev/null || true   # 收割后台任务，避免打印 Terminated 提示
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# 工具函数
# ---------------------------------------------------------------------------

# jget <json-file> <dotted-path> —— 从 JSON 文件里取字段，缺字段输出空串
jget() {
  python3 -c '
import sys, json
try:
    d = json.load(open(sys.argv[1]))
except Exception:
    sys.exit(0)
cur = d
for p in sys.argv[2].split("."):
    if isinstance(cur, dict) and p in cur:
        cur = cur[p]
    else:
        sys.exit(0)
print(cur if isinstance(cur, (str, int, float, bool)) else json.dumps(cur, ensure_ascii=False))
' "$1" "$2"
}

# opt_get <json-file> <key> —— 从 /api/option/ 响应里按 key 取值
opt_get() {
  python3 -c '
import sys, json
try:
    d = json.load(open(sys.argv[1]))
except Exception:
    sys.exit(0)
for o in d.get("data", []):
    if o.get("key") == sys.argv[2]:
        print(o.get("value", "")); sys.exit(0)
' "$1" "$2"
}

# set_option <key> <value-json> —— 调 /api/option/ 更新，返回 HTTP code
set_option() {
  curl -sS -o "$TMP/opt_out.json" -w '%{http_code}' \
    -X PUT "$BASE/api/option/" \
    -H "Authorization: Bearer $ADMIN_KEY" -H 'Content-Type: application/json' \
    -d "{\"key\":\"$1\",\"value\":$2}"
}

# ---------------------------------------------------------------------------
# 1. 预检：监听端口是否空闲
# ---------------------------------------------------------------------------
echo "=== Seedance webhook 自测 ==="
info "网关: $BASE  模型: $MODEL  下游监听端口: $PORT"
if nc -z 127.0.0.1 "$PORT" 2>/dev/null; then
  fail "端口 $PORT 已被占用，请用 AMUX_PORT 换个端口"
  exit 1
fi

# ---------------------------------------------------------------------------
# 2. 启动下游回调监听器
# ---------------------------------------------------------------------------
cat > "$TMP/listener.py" <<PYEOF
import http.server, socketserver, sys, os

PORT, OUT = int(sys.argv[1]), sys.argv[2]
os.makedirs(OUT, exist_ok=True)
n = {"v": 0}

class H(http.server.BaseHTTPRequestHandler):
    def _handle(self):
        ln = int(self.headers.get("Content-Length", 0) or 0)
        body = self.rfile.read(ln) if ln else b""
        n["v"] += 1
        with open(os.path.join(OUT, "cb_%03d.json" % n["v"]), "wb") as f:
            f.write(body)
        with open(os.path.join(OUT, "cb_%03d.headers" % n["v"]), "w") as f:
            f.write(str(dict(self.headers)))
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"code":200,"message":"ok"}')
    do_POST = _handle
    do_GET = _handle
    def log_message(self, *a):
        pass

class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address = True

Server(("0.0.0.0", PORT), H).serve_forever()
PYEOF
python3 "$TMP/listener.py" "$PORT" "$CALLBACK_DIR" &
LISTENER_PID=$!
sleep 0.3
if ! kill -0 "$LISTENER_PID" 2>/dev/null; then
  fail "下游监听器启动失败"
  exit 1
fi
CB_URL="http://127.0.0.1:$PORT/cb"
pass "下游监听器已启动: $CB_URL"

# ---------------------------------------------------------------------------
# 3.（可选）自动配置后台开关 + 假公网地址
# ---------------------------------------------------------------------------
if [ -n "$ADMIN_KEY" ]; then
  info "用 admin token 自动配置门控条件 ..."
  if ! curl -sS -o "$TMP/options.json" -H "Authorization: Bearer $ADMIN_KEY" "$BASE/api/option/"; then
    warn "读取后台配置失败，跳过自动配置（请手工开 SeedanceWebhookEnabled 并设公网 ServerAddress）"
  else
    OLD_ADDR="$(opt_get "$TMP/options.json" ServerAddress)"
    OLD_ENABLED="$(opt_get "$TMP/options.json" SeedanceWebhookEnabled)"
    OLD_SECRET="$(opt_get "$TMP/options.json" SeedanceWebhookSecret)"
    [ -z "$OLD_ENABLED" ] && OLD_ENABLED=false
    info "当前 ServerAddress=\"$OLD_ADDR\", SeedanceWebhookEnabled=$OLD_ENABLED, SeedanceWebhookSecret=$([ -n "$OLD_SECRET" ] && echo '(已配置)' || echo '(空)')"

    code=$(set_option SeedanceWebhookEnabled true)
    if [ "$code" != "200" ]; then
      fail "设置 SeedanceWebhookEnabled=true 失败（HTTP ${code}）: $(jget "$TMP/opt_out.json" message)"
      exit 1
    fi
    code=$(set_option ServerAddress "\"$FAKE_ADDR\"")
    if [ "$code" != "200" ]; then
      fail "临时设置 ServerAddress 失败（HTTP ${code}）: $(jget "$TMP/opt_out.json" message)"
      exit 1
    fi
    # HMAC 签名密钥：回调地址拼了 sig=HMAC(secret, taskID)，webhook 端点据此校验
    # 请求确实来自上游。脚本用临时随机密钥并同步用于模拟回调的签名，退出时还原。
    if [ -z "$SECRET" ]; then
      SECRET="seedance-webhook-$(basename "$TMP")"
    fi
    code=$(set_option SeedanceWebhookSecret "\"$SECRET\"")
    if [ "$code" != "200" ]; then
      fail "设置 SeedanceWebhookSecret 失败（HTTP ${code}）: $(jget "$TMP/opt_out.json" message)"
      exit 1
    fi
    RESTORE_ENABLED=1
    warn "已临时设置 SeedanceWebhookEnabled=true, ServerAddress=\"$FAKE_ADDR\", SeedanceWebhookSecret=***（仅门控用，退出自动还原）"
  fi
else
  warn "未提供 AMUX_ADMIN_KEY，请确认后台已手工开启 SeedanceWebhookEnabled、ServerAddress 是公网地址、且 SeedanceWebhookSecret 已配置（否则任务不会进入 webhook 模式或回调 404）；并设置 AMUX_WEBHOOK_SECRET 与后台 SeedanceWebhookSecret 一致，供模拟回调签名"
fi

# ---------------------------------------------------------------------------
# 4. 提交任务
# ---------------------------------------------------------------------------
info "提交任务（callback_url=${CB_URL}） ..."
code=$(curl -sS -o "$TMP/submit.json" -w '%{http_code}' \
  -X POST "$BASE/v1/video/generations" \
  -H "Authorization: Bearer $API_KEY" -H 'Content-Type: application/json' \
  -d "{\"model\":\"$MODEL\",\"prompt\":\"seedance webhook 自测\",\"callback_url\":\"$CB_URL\"}")
if [ "$code" != "200" ]; then
  fail "提交失败（HTTP ${code}）: $(jget "$TMP/submit.json" message) $(jget "$TMP/submit.json" error.message)"
  warn "常见原因：token 无此模型权限 / 模型未启用 / 余额不足 / 渠道未配置"
  exit 1
fi
TASK_ID="$(jget "$TMP/submit.json" id)"
[ -z "$TASK_ID" ] && TASK_ID="$(jget "$TMP/submit.json" task_id)"
if [ -z "$TASK_ID" ]; then
  fail "提交成功但响应里没有 task_id: $(cat "$TMP/submit.json")"
  exit 1
fi
pass "任务已提交: $TASK_ID"

# ---------------------------------------------------------------------------
# 5. 模拟上游回调：中间态 RUNNING
# ---------------------------------------------------------------------------
echo
# HMAC 签名：webhook 端点校验回调地址里的 sig=HMAC(SeedanceWebhookSecret, task_id)，
# 模拟上游回调必须带上正确的 sig，否则 404。无密钥可签名时直接说明原因退出。
if [ -z "$SECRET" ]; then
  fail "缺少回调签名密钥：未提供 AMUX_WEBHOOK_SECRET 且无 AMUX_ADMIN_KEY 无法自动配置。"
  echo "   • 请设置 AMUX_WEBHOOK_SECRET 与后台 SeedanceWebhookSecret 一致后重跑；"
  echo "   • 或提供 AMUX_ADMIN_KEY 让脚本自动配置随机密钥。"
  exit 1
fi
SIG="$(python3 -c 'import hmac,hashlib,sys;print(hmac.new(sys.argv[1].encode(),sys.argv[2].encode(),hashlib.sha256).hexdigest())' "$SECRET" "$TASK_ID")"
info "模拟上游回调签名 sig=${SIG:0:16}...（HMAC-SHA256）"

info "模拟上游中间态回调（RUNNING） ..."
code=$(curl -sS -o "$TMP/intermediate.json" -w '%{http_code}' \
  -X POST "$BASE/api/v1/webhook/seedance/$TASK_ID?sig=$SIG" \
  -H 'Content-Type: application/json' \
  -d '{"code":200,"message":"ok","data":{"id":123,"status":"RUNNING"},"timestamp":"2026-08-12T00:00:00Z"}')
if [ "$code" = "404" ]; then
  fail "回调被拒（404）——任务没有进入 webhook 模式或签名不符。检查："
  echo "   • 后台 SeedanceWebhookEnabled 是否已开启"
  echo "   • ServerAddress 是否公网地址（脚本用假地址需要 AMUX_ADMIN_KEY）"
  echo "   • 提交请求是否真的带了 callback_url"
  echo "   • 回调 URL 的 sig 是否与后台 SeedanceWebhookSecret 一致（AMUX_WEBHOOK_SECRET）"
  exit 1
elif [ "$code" != "200" ]; then
  fail "中间态回调失败（HTTP ${code}）: $(cat "$TMP/intermediate.json")"
  exit 1
fi
pass "中间态回调已接受（HTTP 200）"

# ---------------------------------------------------------------------------
# 6. 模拟上游回调：终态 SUCCESS（带用量，触发差额结算）
# ---------------------------------------------------------------------------
echo
info "模拟上游终态回调（SUCCESS + usage） ..."
code=$(curl -sS -o "$TMP/terminal.json" -w '%{http_code}' \
  -X POST "$BASE/api/v1/webhook/seedance/$TASK_ID?sig=$SIG" \
  -H 'Content-Type: application/json' \
  -d '{"code":200,"message":"ok","data":{"id":123,"status":"SUCCESS","output":{"video_url":"https://cdn.example.com/webhook-test.mp4","usage":{"total_tokens":120000,"completion_tokens":108000}}},"timestamp":"2026-08-12T00:00:01Z"}')
if [ "$code" != "200" ]; then
  fail "终态回调失败（HTTP ${code}）: $(cat "$TMP/terminal.json")"
  exit 1
fi
pass "终态回调已接受（HTTP 200）"

# ---------------------------------------------------------------------------
# 7. 查询任务，确认已落终态
# ---------------------------------------------------------------------------
echo
info "查询任务状态 ..."
for _ in 1 2 3 4 5; do
  code=$(curl -sS -o "$TMP/query.json" -w '%{http_code}' \
    -X GET "$BASE/v1/video/generations/$TASK_ID" \
    -H "Authorization: Bearer $API_KEY")
  if [ "$code" = "200" ]; then break; fi
  sleep 0.5
done
if [ "$code" != "200" ]; then
  fail "查询失败（HTTP ${code}）"
  exit 1
fi
STATUS="$(jget "$TMP/query.json" data.status)"
PROGRESS="$(jget "$TMP/query.json" data.progress)"
RESULT_URL="$(jget "$TMP/query.json" data.result_url)"
info "查询结果: status=$STATUS progress=$PROGRESS"
if [ "$STATUS" != "SUCCESS" ] || [ "$PROGRESS" != "100%" ]; then
  fail "任务未落成功终态: status=$STATUS progress=$PROGRESS"
  exit 1
fi
pass "任务已落终态 SUCCESS / 100%  (result_url=$RESULT_URL)"

# ---------------------------------------------------------------------------
# 8. 校验下游回调是否送达
# ---------------------------------------------------------------------------
echo
info "等待下游回调送达监听器 ..."
CALLBACK_FILE=""
for _ in $(seq 1 20); do
  CALLBACK_FILE="$(ls "$CALLBACK_DIR"/cb_*.json 2>/dev/null | head -1 || true)"
  [ -n "$CALLBACK_FILE" ] && break
  sleep 0.25
done
if [ -z "$CALLBACK_FILE" ]; then
  fail "监听器没有收到下游回调（NotifyTaskCallback 未触发或发送失败）"
  exit 1
fi
pass "下游回调已送达: $CALLBACK_FILE"
echo "  回调 payload:"
python3 -c '
import json, sys
d = json.load(open(sys.argv[1]))
print("  " + json.dumps(d, ensure_ascii=False, indent=2).replace("\n", "\n  "))
' "$CALLBACK_FILE"

# ---------------------------------------------------------------------------
# 9. 汇总
# ---------------------------------------------------------------------------
echo
echo "=============================================="
pass "全部通过"
echo
info "下一步可做："
echo "   • 用真实 ZeroCut/Ark 渠道 + 公网网关做全链路 e2e（上游真实回调）"
echo "   • 到后台看任务记录 / 结算日志，核对 token 用量与差额结算"
echo "   • 观察网关日志关键字: webhook: task xxx updated to / skip polling"
