#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
令牌分组链（多分组 + 跨分组回落）测试脚本。

只依赖 Python 3 标准库，不用装任何东西：

    python3 scripts/group_chain_test.py -u http://localhost:3000 -k sk-xxx -m gpt-4

原理：发完请求后，用同一个令牌查 GET /api/log/token（该接口用令牌自身鉴权），
按响应头里的 X-Oneapi-Request-Id 精确匹配到每次请求的消费日志，读出「实际命中的
分组」和「扣费」。命中哪个分组、按哪个分组的倍率计费，脚本自己就能判定。

两个已知限制：
  · 日志接口对非管理员屏蔽渠道 ID，所以看不到具体命中哪个渠道。要看「一次请求
    试了哪几个渠道」，去服务端日志搜「重试：」。
  · 该接口限流 20 次 / 20 分钟，所以脚本把所有请求发完之后只批量查一次日志
    （最多重试 3 次），不做逐条轮询。
"""

import argparse
import json
import ssl
import sys
import time
import urllib.error
import urllib.request
from collections import Counter

# ─────────────────────────────────────────────────────────────
# 改这三项就能跑
# ─────────────────────────────────────────────────────────────
BASE_URL = "http://localhost:3000"
TOKEN = "sk-9SBrFz1MFWPiJ5G3QFbF3qhHfHz15UoTVL4VXsjbDgBzm5wx"
MODEL = "gpt-5.6-luna"

# 可选：只存在于链上靠后分组的模型，用来验证「跨分组开关不影响模型发现」。
# 留空则跳过该项检查。
MODEL_ONLY_IN_LATER_GROUP = ""
# ─────────────────────────────────────────────────────────────

REQUEST_ID_HEADER = "x-oneapi-request-id"
LOG_FETCH_MAX_ATTEMPTS = 3


def http_json(method, url, token, payload=None, timeout=180):
    """返回 (状态码, 响应体 dict 或原始文本, 响应头 dict)。网络异常转成状态码 0。"""
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {token}")
    if data:
        req.add_header("Content-Type", "application/json")
    ctx = ssl.create_default_context()  # 自建环境常用自签证书
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    try:
        with urllib.request.urlopen(req, timeout=timeout, context=ctx) as resp:
            body = resp.read().decode("utf-8", "replace")
            headers = {k.lower(): v for k, v in resp.headers.items()}
            try:
                return resp.status, json.loads(body), headers
            except json.JSONDecodeError:
                return resp.status, body, headers
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", "replace")
        headers = {k.lower(): v for k, v in (e.headers or {}).items()}
        try:
            return e.code, json.loads(body), headers
        except json.JSONDecodeError:
            return e.code, body, headers
    except Exception as e:  # 连接失败 / 超时
        return 0, {"error": {"message": str(e)}}, {}


def extract_error(body):
    if isinstance(body, dict):
        err = body.get("error")
        if isinstance(err, dict):
            return err.get("message") or json.dumps(err, ensure_ascii=False)
        if isinstance(body.get("message"), str):
            return body["message"]
        return json.dumps(body, ensure_ascii=False)
    return str(body)


def chat(base_url, token, model, timeout, max_tokens):
    """发一次最小 chat 请求，返回 (状态码, 耗时毫秒, request_id, 错误信息)。

    max_tokens 不能给太小：推理模型会先产出推理 token，额度不够就会以
    400「Could not finish the message because max_tokens ... was reached」
    失败，看起来像渠道有问题，其实是探测请求自己的问题。
    """
    payload = {
        "model": model,
        "messages": [{"role": "user", "content": "只回复两个字：收到"}],
        "max_tokens": max_tokens,
        "stream": False,
    }
    started = time.time()
    status, body, headers = http_json(
        "POST", f"{base_url}/v1/chat/completions", token, payload, timeout
    )
    elapsed = int((time.time() - started) * 1000)
    return status, elapsed, headers.get(REQUEST_ID_HEADER, ""), (
        "" if status == 200 else extract_error(body)
    )


def fetch_logs(base_url, token, wanted_ids, settle_wait):
    """
    批量取一次消费日志并按 request_id 建索引。

    /api/log/token 限流 20 次 / 20 分钟，所以这里绝不逐条轮询：先等结算落库，
    再整体查，缺了才补查，最多 LOG_FETCH_MAX_ATTEMPTS 次。
    """
    found = {}
    for attempt in range(LOG_FETCH_MAX_ATTEMPTS):
        time.sleep(settle_wait if attempt == 0 else 1.5)
        status, body, _ = http_json("GET", f"{base_url}/api/log/token", token, timeout=30)
        if status == 429:
            return found, "日志接口被限流（20 次 / 20 分钟），稍后再跑或减少运行次数"
        if status != 200 or not isinstance(body, dict) or not body.get("success"):
            return found, f"查日志失败：HTTP {status} {extract_error(body)[:80]}"
        for item in body.get("data") or []:
            rid = item.get("request_id")
            if rid in wanted_ids:
                found[rid] = item
        if all(rid in found for rid in wanted_ids if rid):
            break
    return found, ""


def describe(status, error, log, missing_reason):
    if status != 200:
        return error.replace("\n", " ")[:70]
    if log:
        return ""
    return missing_reason or "日志未匹配到（可能未开启消费日志）"


def run(args):
    base_url = args.base_url.rstrip("/")
    print(f"网关 : {base_url}")
    print(f"令牌 : {args.token[:8]}...{args.token[-4:]}")
    print(f"模型 : {args.model}   次数: {args.count}")
    print()

    attempts = []
    total = args.count + (1 if args.discover_model else 0)
    for i in range(1, args.count + 1):
        status, elapsed, rid, error = chat(
            base_url, args.token, args.model, args.timeout, args.max_tokens
        )
        print(f"  [{i}/{total}] {args.model}  HTTP {status}  {elapsed}ms")
        attempts.append((f"{i}", args.model, status, elapsed, rid, error))
        if i < args.count and args.interval > 0:
            time.sleep(args.interval)

    if args.discover_model:
        status, elapsed, rid, error = chat(
            base_url, args.token, args.discover_model, args.timeout, args.max_tokens
        )
        print(f"  [{total}/{total}] {args.discover_model}  HTTP {status}  {elapsed}ms")
        attempts.append(("发现", args.discover_model, status, elapsed, rid, error))

    wanted = {rid for _, _, _, _, rid, _ in attempts if rid}
    print("\n  查询消费日志...")
    logs, warn = fetch_logs(base_url, args.token, wanted, args.settle_wait)

    print()
    print("-" * 86)
    print(f"{'#':<4} {'模型':<20} {'HTTP':>4} {'耗时':>8} {'命中分组':<20} {'扣费':>7}  说明")
    print("-" * 86)
    groups = Counter()
    failures = []
    for label, model, status, elapsed, rid, error in attempts:
        log = logs.get(rid)
        group = (log or {}).get("group") or "-"
        quota = (log or {}).get("quota")
        if log and group != "-":
            groups[group] += 1
        if status != 200:
            failures.append((label, status, error))
        print(
            f"{label:<4} {model:<20} {status:>4} {elapsed:>6}ms "
            f"{group:<20} {(quota if quota is not None else '-'):>7}  "
            f"{describe(status, error, log, warn)}"
        )
    print("-" * 86)

    if warn:
        print(f"注意：{warn}")
    if groups:
        print("命中分组分布：")
        for g, n in groups.most_common():
            print(f"  {g:<24} {n} 次")
        if len(groups) > 1:
            print("  ↑ 命中了多个分组：发生过跨分组回落，或不同模型分布在不同分组")
        print("  扣费差异对应各分组倍率不同，可据此确认计费按实际命中分组结算")
    elif not failures:
        print("请求都成功了，但没取到分组信息 —— 检查是否开启了「记录消费日志」")

    if failures:
        print(f"\n失败 {len(failures)}/{len(attempts)} 次：")
        for label, status, error in failures[:5]:
            print(f"  [{label}] HTTP {status}: {error[:140]}")

    if args.discover_model:
        disc = next((a for a in attempts if a[0] == "发现"), None)
        if disc and disc[2] == 200:
            print(
                f"\n模型发现：{args.discover_model} 命中 "
                f"{(logs.get(disc[4]) or {}).get('group', '-')}"
            )
            print("  ↑ 即使关闭跨分组重试也应成功 —— 该开关只管失败回落，不管模型发现")

    print("\n看「一次请求试了哪几个渠道」：服务端日志搜「重试：」（日志接口对非管理员屏蔽渠道 ID）")
    return 1 if failures and len(failures) == len(attempts) else 0


def main():
    parser = argparse.ArgumentParser(
        description="令牌分组链测试",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="示例：\n"
        "  python3 scripts/group_chain_test.py -u http://localhost:3000 -k sk-xxx -m gpt-4\n"
        "  python3 scripts/group_chain_test.py -n 6 -i 0        # 连发不间隔，便于触发熔断\n"
        "  python3 scripts/group_chain_test.py -d only-in-b     # 顺带验证模型发现\n",
    )
    parser.add_argument("-u", "--base-url", default=BASE_URL, help="网关地址")
    parser.add_argument("-k", "--token", default=TOKEN, help="令牌（sk- 开头）")
    parser.add_argument("-m", "--model", default=MODEL, help="测试模型")
    parser.add_argument("-n", "--count", type=int, default=3, help="请求次数，默认 3")
    parser.add_argument("-i", "--interval", type=float, default=1.0, help="请求间隔秒，默认 1")
    parser.add_argument("-t", "--timeout", type=int, default=180, help="单次超时秒数")
    parser.add_argument(
        "--max-tokens",
        type=int,
        default=256,
        help="探测请求的 max_tokens，默认 256。给太小会让推理模型以 400 失败，"
        "看起来像渠道故障其实是探测请求本身的问题",
    )
    parser.add_argument(
        "-w", "--settle-wait", type=float, default=2.0, help="查日志前等待结算的秒数"
    )
    parser.add_argument(
        "-d",
        "--discover-model",
        default=MODEL_ONLY_IN_LATER_GROUP,
        help="只存在于链上靠后分组的模型，用于验证模型发现",
    )
    args = parser.parse_args()
    if "xxxx" in args.token:
        parser.error("请先在脚本顶部填 TOKEN，或用 -k 传入")
    sys.exit(run(args))


if __name__ == "__main__":
    main()
