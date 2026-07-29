#!/usr/bin/env bash
# 令牌并发限制的端到端测试。
#
# 用法:
#   scripts/test_token_concurrency.sh <令牌> [并发数] [服务地址] [模型]
#
# 例:
#   scripts/test_token_concurrency.sh sk-xxxx 3
#   scripts/test_token_concurrency.sh sk-xxxx 3 http://127.0.0.1:3000 gpt-4o-mini
#
# 判定标准：发 N 个并发请求（N = 并发上限 + 3），期望恰好有「上限」个不是 429，
# 其余为 429 且带 Retry-After。全部请求结束后再发一个，应该能通过（槽位已释放）。

set -uo pipefail

# 令牌也可以用 API_KEY 环境变量传，避免出现在 shell 历史和 ps 输出里
TOKEN="${1:-${API_KEY:-}}"
LIMIT="${2:-3}"
BASE="${3:-http://127.0.0.1:3000}"
MODEL="${4:-gpt-4o-mini}"

if [ -z "$TOKEN" ]; then
  echo "用法: $0 <令牌> [并发上限] [服务地址] [模型]" >&2
  echo "  也可用环境变量: API_KEY=sk-xxx $0 \"\" 3" >&2
  exit 1
fi
case "$TOKEN" in
  sk-*) ;;
  *) TOKEN="sk-$TOKEN" ;;
esac

TOTAL=$((LIMIT + 3))
OUT=$(mktemp -d)
trap 'rm -rf "$OUT"' EXIT

echo "服务地址 : $BASE"
echo "模型     : $MODEL"
echo "令牌上限 : $LIMIT"
echo "并发发出 : $TOTAL 个请求"
echo "------------------------------------------------"

# 用一个能让上游耗时几秒的请求，确保槽位在并发期间真的被占住。
# max_tokens 给大一些 + 要求长输出，避免请求太快结束导致测不出并发。
payload() {
  cat <<JSON
{"model":"$MODEL","messages":[{"role":"user","content":"Count slowly from 1 to 80, one number per line."}],"max_tokens":400,"stream":false}
JSON
}

for i in $(seq 1 "$TOTAL"); do
  (
    code=$(curl -s -o "$OUT/body.$i" -w '%{http_code}' \
      --max-time 120 \
      -X POST "$BASE/v1/chat/completions" \
      -H "Authorization: Bearer $TOKEN" \
      -H 'Content-Type: application/json' \
      -D "$OUT/hdr.$i" \
      -d "$(payload)" 2>/dev/null)
    echo "$code" > "$OUT/code.$i"
  ) &
done
wait

pass=0; limited=0; other=0
for i in $(seq 1 "$TOTAL"); do
  code=$(cat "$OUT/code.$i" 2>/dev/null || echo "000")
  case "$code" in
    429)
      limited=$((limited + 1))
      ra=$(grep -i '^retry-after:' "$OUT/hdr.$i" 2>/dev/null | tr -d '\r' | head -1)
      msg=$(python3 -c "
import json,sys
try:
    d=json.load(open('$OUT/body.$i'))
    e=d.get('error',{})
    print((e.get('code') or '') + ' | ' + (e.get('message') or '')[:70])
except Exception: print('(无法解析响应)')
" 2>/dev/null)
      echo "  #$i 429  ${ra:-（缺 Retry-After!）}  $msg"
      ;;
    200)
      pass=$((pass + 1)); echo "  #$i 200  通过"
      ;;
    *)
      other=$((other + 1)); echo "  #$i $code  其他（可能是上游错误/余额/模型不可用，见 $OUT/body.$i）"
      ;;
  esac
done

echo "------------------------------------------------"
echo "通过: $pass   被限流(429): $limited   其他: $other"
echo

fail=0
if [ "$other" -gt 0 ]; then
  echo "⚠  有非 200/429 响应，说明请求没打到并发逻辑（先确认令牌/模型/余额可用）"
  fail=1
elif [ "$pass" -eq "$LIMIT" ] && [ "$limited" -eq $((TOTAL - LIMIT)) ]; then
  echo "✓ 并发限制生效：恰好放行 $LIMIT 个，其余 $limited 个被限流"
else
  echo "✗ 与预期不符：期望放行 $LIMIT 个、限流 $((TOTAL - LIMIT)) 个"
  echo "  若放行数偏多，可能是多实例各自计数（无 Redis 时的已知降级行为）"
  fail=1
fi

# 槽位释放检查：并发批次已全部结束，此时再发一个应该能过
echo
echo "槽位释放检查（并发结束后再发 1 个请求）..."
code=$(curl -s -o "$OUT/after" -w '%{http_code}' --max-time 120 \
  -X POST "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "$(payload)" 2>/dev/null)
if [ "$code" = "429" ]; then
  echo "✗ 仍返回 429 —— 槽位没有被释放（泄漏）"
  fail=1
else
  echo "✓ 返回 $code，槽位已正常释放"
fi

exit "$fail"
