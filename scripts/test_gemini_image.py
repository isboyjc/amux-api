#!/usr/bin/env python3
"""
Test script for Gemini image generation via new-api proxy.

Targets two production issues:
  1. "request body too large" (HTTP 413/400) when sending reference images
     via inlineData. new-api enforces MAX_REQUEST_BODY_MB (default 128 MB)
     in common/gin.go and middleware/gzip.go.
  2. "bad response status code 524" — upstream gateway timeout. new-api
     itself uses RELAY_TIMEOUT (default 0 = no timeout), so 524 comes from
     a fronting nginx/cloudflare. The script measures end-to-end latency
     so you can see whether the request survives past common timeout marks.

Usage:
    pip install pillow requests
    python scripts/test_gemini_image.py

    # Run a single scenario:
    python scripts/test_gemini_image.py --only large-20mb

    # Run just the aspect-ratio matrix:
    python scripts/test_gemini_image.py --only aspect-matrix

The script calls the native Gemini endpoint exposed by new-api:
    POST {base_url}/v1beta/models/{model}:generateContent
"""

import argparse
import base64
import datetime
import io
import json
import os
import sys
import time
from typing import Optional

try:
    import requests
except ImportError:
    sys.exit("Missing dependency: pip install requests")

try:
    from PIL import Image
except ImportError:
    sys.exit("Missing dependency: pip install pillow")


DEFAULT_BASE_URL = "https://api.amux.ai"
DEFAULT_API_KEY = "sk-O60riamVBxWsaBYWK4MDX1ZIZ2fU5sMPGSrNO3F1j966EGEd"
DEFAULT_MODEL = "gemini-3.1-flash-image-preview"
DEFAULT_CLIENT_TIMEOUT = 600  # 10 min — long enough to observe upstream 524s

# Log file + output images: written next to the script, one dir per run.
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
LOG_DIR = os.path.join(SCRIPT_DIR, "test_gemini_image_logs")
OUTPUT_DIR = os.path.join(SCRIPT_DIR, "test_gemini_image_output")
RUN_TS = datetime.datetime.now().strftime("%Y%m%d_%H%M%S")
LOG_FILE = os.path.join(LOG_DIR, f"run_{RUN_TS}.log")
RUN_OUTPUT_DIR = os.path.join(OUTPUT_DIR, f"run_{RUN_TS}")

_log_fh = None


def log_open() -> None:
    global _log_fh
    os.makedirs(LOG_DIR, exist_ok=True)
    _log_fh = open(LOG_FILE, "w", encoding="utf-8")


def log_close() -> None:
    global _log_fh
    if _log_fh is not None:
        _log_fh.flush()
        _log_fh.close()
        _log_fh = None


def log(line: str = "") -> None:
    """Write to both stdout and the log file."""
    print(line)
    if _log_fh is not None:
        _log_fh.write(line + "\n")
        _log_fh.flush()


def log_block(title: str, content: str) -> None:
    """Write a titled block (only to log file — stdout stays tidy)."""
    if _log_fh is None:
        return
    _log_fh.write(f"\n----- {title} -----\n")
    _log_fh.write(content)
    if not content.endswith("\n"):
        _log_fh.write("\n")
    _log_fh.flush()


def slugify(label: str) -> str:
    out = []
    for ch in label:
        if ch.isalnum() or ch in ("-", "_"):
            out.append(ch)
        elif ch in (" ", "/", "\\"):
            out.append("_")
        elif ch == ":":
            out.append("-")
    return "".join(out).strip("_") or "scenario"


MIME_TO_EXT = {
    "image/jpeg": "jpg",
    "image/jpg": "jpg",
    "image/png": "png",
    "image/webp": "webp",
    "image/gif": "gif",
}


def extract_and_save_images(resp_text: str, scenario_slug: str) -> list:
    """
    Parse the Gemini response body, pull out inlineData image parts, and
    save each to RUN_OUTPUT_DIR. Returns a list of saved file paths.
    """
    saved: list = []
    try:
        data, _ = json.JSONDecoder().raw_decode(resp_text.lstrip())
    except (ValueError, json.JSONDecodeError) as exc:
        log(f"  image extract     : skipped ({type(exc).__name__}: {exc})")
        return saved

    candidates = data.get("candidates", []) if isinstance(data, dict) else []
    os.makedirs(RUN_OUTPUT_DIR, exist_ok=True)

    for ci, cand in enumerate(candidates):
        parts = (cand.get("content") or {}).get("parts", []) or []
        for pi, part in enumerate(parts):
            inline = part.get("inlineData") if isinstance(part, dict) else None
            if not isinstance(inline, dict):
                continue
            b64 = inline.get("data")
            if not isinstance(b64, str) or not b64:
                continue
            mime = inline.get("mimeType") or "image/png"
            ext = MIME_TO_EXT.get(mime.lower(), "bin")
            filename = f"{scenario_slug}__c{ci}_p{pi}.{ext}"
            path = os.path.join(RUN_OUTPUT_DIR, filename)
            try:
                raw = base64.b64decode(b64)
            except (ValueError, base64.binascii.Error) as exc:
                log(f"  image extract     : part c{ci}p{pi} decode failed ({exc})")
                continue
            with open(path, "wb") as f:
                f.write(raw)
            saved.append(path)
            log(f"  saved image       : {path} ({human_size(len(raw))}, {mime})")
    return saved


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def human_size(n: float) -> str:
    for unit in ("B", "KB", "MB", "GB"):
        if n < 1024:
            return f"{n:.1f}{unit}"
        n /= 1024
    return f"{n:.1f}TB"


def make_png_bytes(target_bytes: int) -> bytes:
    """
    Build a valid PNG whose serialized file size is approximately target_bytes.

    Uses random pixel data with PNG compress_level=0 so deflate doesn't shrink
    it — the encoded file ends up very close to W*H*3 plus a small header.
    """
    if target_bytes <= 0:
        buf = io.BytesIO()
        Image.new("RGB", (1, 1), "white").save(buf, format="PNG")
        return buf.getvalue()

    # RGB: 3 bytes/pixel. Find a square that matches target pixel volume.
    side = max(1, int(((target_bytes // 3)) ** 0.5))
    while side * side * 3 < target_bytes:
        side += 1

    raw = os.urandom(side * side * 3)
    img = Image.frombytes("RGB", (side, side), raw)
    buf = io.BytesIO()
    img.save(buf, format="PNG", compress_level=0)
    return buf.getvalue()


def build_payload(
    prompt: str,
    ref_png: Optional[bytes],
    aspect_ratio: Optional[str],
    image_size: Optional[str],
) -> dict:
    parts = [{"text": prompt}]
    if ref_png is not None:
        parts.append({
            "inlineData": {
                "mimeType": "image/png",
                "data": base64.b64encode(ref_png).decode("ascii"),
            }
        })

    generation_config: dict = {
        "responseModalities": ["IMAGE", "TEXT"],
    }

    image_config: dict = {}
    if aspect_ratio:
        image_config["aspectRatio"] = aspect_ratio
    if image_size:
        # "imageSize" param accepted by gemini-*-image-preview models
        # e.g. "1K", "2K". Harmless if the upstream ignores it.
        image_config["imageSize"] = image_size
    if image_config:
        generation_config["imageConfig"] = image_config

    return {
        "contents": [{"role": "user", "parts": parts}],
        "generationConfig": generation_config,
    }


def call_gemini(
    base_url: str,
    api_key: str,
    model: str,
    payload: dict,
    client_timeout: int,
):
    url = f"{base_url.rstrip('/')}/v1beta/models/{model}:generateContent"
    headers = {
        "Content-Type": "application/json",
        # Native Gemini auth header — new-api accepts both of these.
        "x-goog-api-key": api_key,
        "Authorization": f"Bearer {api_key}",
    }
    body_bytes = json.dumps(payload).encode("utf-8")
    body_size = len(body_bytes)

    started = time.monotonic()
    try:
        resp = requests.post(
            url,
            headers=headers,
            data=body_bytes,
            timeout=client_timeout,
        )
    except requests.RequestException as exc:
        elapsed = time.monotonic() - started
        return None, elapsed, body_size, exc, url, headers
    elapsed = time.monotonic() - started
    return resp, elapsed, body_size, None, url, headers


def redact_payload_for_log(payload: dict) -> dict:
    """Return a copy of the payload with inlineData.data replaced by a size hint."""
    clone = json.loads(json.dumps(payload))
    for content in clone.get("contents", []):
        for part in content.get("parts", []):
            inline = part.get("inlineData")
            if isinstance(inline, dict) and isinstance(inline.get("data"), str):
                data_len = len(inline["data"])
                inline["data"] = f"<base64 omitted, {data_len} chars>"
    return clone


def summarize(resp, err, body_size: int, elapsed: float, payload: dict,
              url: str, headers: dict, scenario_slug: str) -> None:
    log(f"  request body size : {human_size(body_size)}")
    log(f"  elapsed           : {elapsed:.2f}s")

    # Always write full request details to the log file.
    redacted_headers = {
        k: ("<redacted>" if k.lower() in ("authorization", "x-goog-api-key") else v)
        for k, v in headers.items()
    }
    log_block(
        "REQUEST",
        f"url: {url}\n"
        f"headers: {json.dumps(redacted_headers, ensure_ascii=False, indent=2)}\n"
        f"payload:\n{json.dumps(redact_payload_for_log(payload), ensure_ascii=False, indent=2)}",
    )

    if err is not None:
        log(f"  ERROR             : {type(err).__name__}: {err}")
        log_block("ERROR", f"{type(err).__name__}: {err}")
        return

    log(f"  status            : {resp.status_code}")
    text = resp.text or ""
    snippet = text[:500].replace("\n", " ")
    log(f"  body snippet      : {snippet}")

    resp_headers = {k: v for k, v in resp.headers.items()}
    log_block(
        "RESPONSE",
        f"status: {resp.status_code}\n"
        f"headers: {json.dumps(resp_headers, ensure_ascii=False, indent=2)}\n"
        f"body:\n{text}",
    )

    # Quick pass/fail hints tied to the two issues we care about.
    if resp.status_code in (400, 413) and "too large" in text.lower():
        log("  => HIT body-size limit (check MAX_REQUEST_BODY_MB on the server)")
    if resp.status_code == 524:
        log("  => HIT gateway timeout (check nginx/cloudflare proxy_read_timeout)")

    # Extract any inlineData image parts from the response and write them to disk.
    if resp.status_code == 200:
        extract_and_save_images(text, scenario_slug)


def run_scenario(
    label: str,
    base_url: str,
    api_key: str,
    model: str,
    ref_target_bytes: int,
    prompt: str,
    aspect_ratio: Optional[str],
    image_size: Optional[str],
    client_timeout: int,
) -> None:
    log(f"\n===== {label} =====")
    log(f"  prompt            : {prompt}")
    log(f"  aspect_ratio      : {aspect_ratio or '(default)'}")
    log(f"  image_size        : {image_size or '(default)'}")

    ref_png = None
    if ref_target_bytes > 0:
        log(f"  building reference PNG (~{human_size(ref_target_bytes)}) ...")
        ref_png = make_png_bytes(ref_target_bytes)
        log(f"  reference PNG     : {human_size(len(ref_png))}")

    payload = build_payload(prompt, ref_png, aspect_ratio, image_size)
    resp, elapsed, body_size, err, url, headers = call_gemini(
        base_url, api_key, model, payload, client_timeout
    )
    summarize(resp, err, body_size, elapsed, payload, url, headers,
              scenario_slug=slugify(label))


# ---------------------------------------------------------------------------
# Scenarios
# ---------------------------------------------------------------------------


MB = 1024 * 1024

# (target_raw_ref_png_bytes, prompt, aspect_ratio, image_size)
SIZE_SCENARIOS = {
    "text-only": (
        0,
        "A cyberpunk cat riding a neon motorbike through a rainy Shanghai street at night",
        "1:1",
        "1K",
    ),
    "small-1mb": (
        1 * MB,
        "Transform the attached image into a vivid oil painting",
        "16:9",
        "1K",
    ),
    "large-20mb": (
        20 * MB,
        "Restyle the attached image as a watercolor artwork",
        "9:16",
        "2K",
    ),
    "mid-25mb": (
        25 * MB,
        "Restyle the attached image in a Van Gogh style",
        "1:1",
        "2K",
    ),
    "mid-30mb": (
        30 * MB,
        "Restyle the attached image in a Monet style",
        "1:1",
        "2K",
    ),
    "mid-35mb": (
        35 * MB,
        "Restyle the attached image in a Cezanne style",
        "1:1",
        "2K",
    ),
    "mid-37mb": (
        37 * MB,
        "Restyle the attached image in a Gauguin style",
        "1:1",
        "2K",
    ),
    "mid-39mb": (
        39 * MB,
        "Restyle the attached image in a Matisse style",
        "1:1",
        "2K",
    ),
    "mid-40mb": (
        40 * MB,
        "Restyle the attached image in a Picasso style",
        "1:1",
        "2K",
    ),
    "xl-50mb": (
        50 * MB,
        "Turn the attached image into a detailed pencil sketch",
        "4:3",
        "2K",
    ),
    # Output resolution + aspect ratio matrix (no reference image, isolate the params).
    "res-1k-1-1": (
        0,
        "A serene Japanese zen garden with raked sand patterns and a single cherry blossom tree",
        "1:1",
        "1K",
    ),
    "res-2k-16-9": (
        0,
        "A sweeping cinematic landscape of a futuristic megacity at golden hour",
        "16:9",
        "2K",
    ),
    "res-4k-9-16": (
        0,
        "A tall vertical portrait of a cyberpunk samurai in neon-lit alleyway, Tokyo night",
        "9:16",
        "4K",
    ),
}

# Dedicated aspect-ratio + image-size matrix, run on a small ref image so
# the upstream failures you see are tied to the params, not to body size.
ASPECT_MATRIX = [
    ("1:1",  "1K"),
    ("16:9", "1K"),
    ("9:16", "1K"),
    ("4:3",  "2K"),
    ("3:4",  "2K"),
]


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Gemini image-generation body-size / timeout tester for new-api",
    )
    parser.add_argument(
        "--base-url",
        default=DEFAULT_BASE_URL,
        help=f"new-api base URL (default: {DEFAULT_BASE_URL})",
    )
    parser.add_argument(
        "--api-key",
        default=DEFAULT_API_KEY,
        help="API key / token",
    )
    parser.add_argument(
        "--model",
        default=DEFAULT_MODEL,
        help=f"model name (default: {DEFAULT_MODEL})",
    )
    parser.add_argument(
        "--only",
        help=(
            "comma-separated scenario names to run. "
            f"Available: {','.join(SIZE_SCENARIOS)},aspect-matrix"
        ),
    )
    parser.add_argument(
        "--client-timeout",
        type=int,
        default=DEFAULT_CLIENT_TIMEOUT,
        help="client-side HTTP timeout in seconds (default: 600)",
    )
    args = parser.parse_args()

    selected = None
    if args.only:
        selected = {s.strip() for s in args.only.split(",") if s.strip()}

    log_open()
    log(f"run start : {datetime.datetime.now().isoformat(timespec='seconds')}")
    log(f"log file  : {LOG_FILE}")
    log(f"output dir: {RUN_OUTPUT_DIR}")
    log(f"target    : {args.base_url}")
    log(f"model     : {args.model}")
    log(f"timeout   : {args.client_timeout}s")
    if selected:
        log(f"scenarios : {','.join(sorted(selected))}")

    # Run size scenarios.
    for name, (ref_bytes, prompt, ratio, size) in SIZE_SCENARIOS.items():
        if selected and name not in selected:
            continue
        run_scenario(
            label=f"size / {name}",
            base_url=args.base_url,
            api_key=args.api_key,
            model=args.model,
            ref_target_bytes=ref_bytes,
            prompt=prompt,
            aspect_ratio=ratio,
            image_size=size,
            client_timeout=args.client_timeout,
        )

    # Run aspect-ratio matrix (small ref image, so failures isolate the params).
    if selected is None or "aspect-matrix" in selected:
        for ratio, size in ASPECT_MATRIX:
            run_scenario(
                label=f"aspect-matrix / {ratio} @ {size}",
                base_url=args.base_url,
                api_key=args.api_key,
                model=args.model,
                ref_target_bytes=1 * MB,
                prompt="Redraw the attached image in a Studio Ghibli style",
                aspect_ratio=ratio,
                image_size=size,
                client_timeout=args.client_timeout,
            )

    log("\nAll done.")
    log(f"run end   : {datetime.datetime.now().isoformat(timespec='seconds')}")
    log_close()


if __name__ == "__main__":
    main()
