#!/usr/bin/env bash
# 发 N 个并发请求，统计放行/限流分布。用于验证令牌级/账户级并发限制。
#
# 用法:
#   concurrency_probe.sh <令牌> <并发数> <预期上限> [服务地址] [模型]
#
# 令牌也可以用环境变量传，避免出现在 shell 历史和 ps 输出里：
#   API_KEY=sk-xxx concurrency_probe.sh "" 10 3
#
# 预期上限用于结果对照，脚本本身不校验它和服务端配置是否一致。
set -uo pipefail
TOKEN="${1:-${API_KEY:-}}"; N="${2:?并发数}"; LIMIT="${3:?预期上限}"
if [ -z "$TOKEN" ]; then
  echo "错误：需要令牌。作为第一个参数传入，或用 API_KEY 环境变量。" >&2
  exit 1
fi
BASE="${4:-http://127.0.0.1:3000}"; MODEL="${5:-gpt-5.6-luna}"
case "$TOKEN" in sk-*) ;; *) TOKEN="sk-$TOKEN";; esac

OUT=$(mktemp -d); trap 'rm -rf "$OUT"' EXIT
BODY="{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"Count slowly from 1 to 80, one number per line.\"}],\"max_tokens\":400,\"stream\":false}"

# 用文件屏障让所有 curl 尽可能同时发出，避免串行错开导致测不到并发
GO="$OUT/go"
for i in $(seq 1 "$N"); do
  (
    while [ ! -f "$GO" ]; do sleep 0.01; done
    start=$(python3 -c 'import time;print(time.time())')
    code=$(curl -s -o "$OUT/b.$i" -D "$OUT/h.$i" -w '%{http_code}' --max-time 180 \
      -X POST "$BASE/v1/chat/completions" \
      -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
      -d "$BODY" 2>/dev/null)
    end=$(python3 -c 'import time;print(time.time())')
    printf '%s %s %s\n' "$code" "$start" "$end" > "$OUT/r.$i"
  ) &
done
sleep 1; touch "$GO"; wait

pass=0; limited=0; other=0
first_start=""; last_end=""
for i in $(seq 1 "$N"); do
  read -r code st en < "$OUT/r.$i" 2>/dev/null || { code=000; st=0; en=0; }
  [ -z "$first_start" ] && first_start="$st"
  first_start=$(python3 -c "print(min($first_start,$st))")
  last_end=$(python3 -c "print(max(${last_end:-0},$en))")
  dur=$(python3 -c "print(f'{$en-$st:.2f}')")
  case "$code" in
    200) pass=$((pass+1)); printf '  #%-2s 200  %ss\n' "$i" "$dur" ;;
    429) limited=$((limited+1))
         ra=$(grep -i '^retry-after:' "$OUT/h.$i" 2>/dev/null | tr -d '\r' | cut -d' ' -f2)
         ec=$(python3 -c "
import json
try: print(json.load(open('$OUT/b.$i')).get('error',{}).get('code',''))
except Exception: print('?')" 2>/dev/null)
         printf '  #%-2s 429  %ss  Retry-After=%s  code=%s\n' "$i" "$dur" "${ra:-缺失}" "$ec" ;;
    *) other=$((other+1)); printf '  #%-2s %s  %ss  %s\n' "$i" "$code" "$dur" "$(head -c 120 "$OUT/b.$i" 2>/dev/null)" ;;
  esac
done
window=$(python3 -c "print(f'{$last_end-$first_start:.2f}')")
echo "  并发窗口跨度: ${window}s"
echo "  结果: 放行=$pass  限流=$limited  其他=$other"
printf '%s\n' "$pass $limited $other"> "$OUT/../summary.$N" 2>/dev/null || true
exit 0
