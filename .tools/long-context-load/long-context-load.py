#!/usr/bin/env python3
"""Probe staged long-context retrieval and warm incremental-message-cache reuse."""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


DEFAULT_STAGES = "4096,8192,16384,32768,65536,131072"
MARKER_RE = re.compile(r"LONGCTX-S\d+-(?:BEGIN|MIDDLE|END)-[0-9A-F]{8}")


def arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run staged exact-needle retrievals and verify warm Kronk IMC reuse."
    )
    parser.add_argument("--host", required=True, help="Kronk server base URL")
    parser.add_argument("--model", required=True)
    parser.add_argument(
        "--stages", "--stage", dest="stages", action="append",
        help=f"repeatable or comma-separated token targets (default: {DEFAULT_STAGES})",
    )
    parser.add_argument("--max-tokens", type=int, default=96)
    parser.add_argument("--timeout", type=int, default=1800)
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--out", type=Path, default=Path("summary.json"))
    args = parser.parse_args()

    raw = args.stages or [DEFAULT_STAGES]
    try:
        args.stages = sorted({int(value.strip()) for item in raw for value in item.split(",") if value.strip()})
    except ValueError:
        parser.error("--stages values must be integers")
    if not args.stages or any(value <= 0 for value in args.stages):
        parser.error("--stages values must be positive")
    if args.max_tokens <= 0:
        parser.error("--max-tokens must be positive")
    if args.timeout <= 0:
        parser.error("--timeout must be positive")
    if args.seed < 0:
        parser.error("--seed must not be negative")
    return args


def headers() -> dict[str, str]:
    result = {"Content-Type": "application/json"}
    if token := os.environ.get("KRONK_TOKEN"):
        result["Authorization"] = f"Bearer {token}"
    return result


def request_json(args: argparse.Namespace, path: str, body: dict | None = None, timeout: int | None = None):
    request = urllib.request.Request(
        f"{args.host.rstrip('/')}{path}", data=json.dumps(body).encode() if body is not None else None,
        headers=headers(), method="POST" if body is not None else "GET",
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout or args.timeout) as response:
            return json.load(response)
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode(errors="replace")
        raise RuntimeError(f"{path}: HTTP {exc.code}: {detail}") from exc


def stream_completion(args: argparse.Namespace, prompt: str) -> dict:
    body = {
        "model": args.model, "stream": True,
        "stream_options": {"include_usage": True}, "temperature": 0,
        "seed": args.seed, "max_tokens": args.max_tokens, "enable_thinking": False,
        "messages": [{"role": "user", "content": prompt}],
    }
    request = urllib.request.Request(
        f"{args.host.rstrip('/')}/v1/chat/completions", data=json.dumps(body).encode(),
        headers=headers(), method="POST",
    )
    started = time.monotonic()
    first = None
    pieces: list[str] = []
    usage: dict = {}
    try:
        with urllib.request.urlopen(request, timeout=args.timeout) as response:
            for raw in response:
                line = raw.decode(errors="replace").strip()
                if not line.startswith("data:"):
                    continue
                data = line[5:].strip()
                if data == "[DONE]":
                    break
                chunk = json.loads(data)
                usage = chunk.get("usage") or usage
                choices = chunk.get("choices") or []
                delta = choices[0].get("delta") if choices else None
                text = delta.get("content", "") if isinstance(delta, dict) else ""
                if text:
                    first = first or time.monotonic()
                    pieces.append(text)
    except urllib.error.HTTPError as exc:
        raise RuntimeError(f"completion: HTTP {exc.code}: {exc.read().decode(errors='replace')}") from exc
    finished = time.monotonic()
    output = "".join(pieces)
    if first is None:
        raise RuntimeError("completion returned no text")
    details = usage.get("prompt_tokens_details") or {}
    completion_tokens = int(usage.get("completion_tokens", 0))
    decode_seconds = max(finished - first, 0.000001)
    return {
        "output": output,
        "prompt_tokens": int(usage.get("prompt_tokens", 0)),
        "cached_tokens": int(details.get("cached_tokens", usage.get("cached_tokens", 0))),
        "output_tokens": completion_tokens,
        "ttft_seconds": round(first - started, 4),
        "tokens_per_second": round(completion_tokens / decode_seconds, 3),
        "elapsed_seconds": round(finished - started, 4),
    }


def token_count(args: argparse.Namespace, text: str) -> int:
    response = request_json(args, "/v1/tokenize", {
        "model": args.model, "input": text, "apply_template": False,
    })
    if not isinstance(response, dict) or "tokens" not in response:
        raise RuntimeError("/v1/tokenize returned no token count")
    return int(response["tokens"])


def markers(stage: int, seed: int) -> list[str]:
    # Arithmetic, rather than random state, keeps a stage independently reproducible.
    return [
        f"LONGCTX-S{stage}-{place}-{((stage * 2654435761 + seed + index) & 0xffffffff):08X}"
        for index, place in enumerate(("BEGIN", "MIDDLE", "END"))
    ]


def make_prompt(args: argparse.Namespace, target: int) -> tuple[str, int, list[str]]:
    needles = markers(target, args.seed)
    # Leave capacity for the chat template and requested completion when a stage
    # is exactly the configured context window.
    content_target = max(1, target - args.max_tokens - 64)
    intro = (
        "Long-context retrieval test. Memorize the three exact marker strings embedded below. "
        "Ignore the filler.\n"
    )
    ending = (
        "\nReturn only all three marker strings, each exactly once, in their original order. "
        "Do not explain.\n"
    )
    filler = " archival ledger quartz orbit telemetry cedar delta nominal."

    def candidate(repetitions: int) -> str:
        third = repetitions // 3
        parts = [filler * third, filler * (repetitions - 2 * third), filler * third]
        return intro + needles[0] + parts[0] + needles[1] + parts[1] + needles[2] + parts[2] + ending

    low, high = 0, max(1, content_target // 3)
    while token_count(args, candidate(high)) < content_target:
        high *= 2
    while low < high:
        middle = (low + high + 1) // 2
        if token_count(args, candidate(middle)) <= content_target:
            low = middle
        else:
            high = middle - 1
    prompt = candidate(low)
    return prompt, token_count(args, prompt), needles


def model_matches(left: str, right: str) -> bool:
    return left == right or left.endswith(right) or right.endswith(left)


def warm_and_inspect(args: argparse.Namespace) -> tuple[int | None, str]:
    # A completion is the supported lazy-load path.
    stream_completion(args, "Reply with OK only.")
    encoded = urllib.parse.quote(args.model, safe="")
    try:
        detail = request_json(args, f"/v1/kronk/models/{encoded}", timeout=30)
        config = detail.get("model_config", {}) if isinstance(detail, dict) else {}
        value = config.get("context-window", config.get("context_window"))
        if value:
            return int(value), "model management detail"
    except (RuntimeError, ValueError, TypeError):
        pass
    try:
        sessions = request_json(args, "/v1/kronk/models/imc-sessions", timeout=30)
        for session in sessions if isinstance(sessions, list) else []:
            if model_matches(str(session.get("model_id", "")), args.model) and session.get("context_window"):
                return int(session["context_window"]), "IMC session management"
    except (RuntimeError, ValueError, TypeError):
        pass
    return None, "unavailable; stages will run until the server rejects one"


def verify(result: dict, expected: list[str], all_known: set[str]) -> None:
    found = MARKER_RE.findall(result["output"])
    foreign = sorted((set(found) | (set(result["output"].split()) & all_known)) - set(expected))
    if found != expected or foreign:
        raise RuntimeError(
            f"marker validation failed: expected={expected}, found={found}, "
            f"foreign={foreign}, output={result['output']!r}"
        )


def main() -> int:
    args = arguments()
    summary = {"host": args.host, "model": args.model, "requested_stages": args.stages, "stages": [], "status": "FAIL"}
    try:
        context, source = warm_and_inspect(args)
        summary.update({"configured_context": context, "context_source": source})
        known = {marker for stage in args.stages for marker in markers(stage, args.seed)}
        ran = 0
        for target in args.stages:
            if context is not None and target > context:
                summary["stages"].append({"target_tokens": target, "status": "SKIP", "reason": f"target exceeds configured context {context}"})
                print(f"stage {target}: SKIP (configured context {context})")
                continue
            try:
                prompt, calibrated, expected = make_prompt(args, target)
                cold = stream_completion(args, prompt)
                verify(cold, expected, known)
                warm = stream_completion(args, prompt)
                verify(warm, expected, known)
                if warm["cached_tokens"] <= 0:
                    raise RuntimeError("warm repeat reported cached_tokens=0; IMC is not enabled or was not reused")
                if warm["output"] != cold["output"]:
                    raise RuntimeError("warm repeat output differs from the cold deterministic output")
                summary["stages"].append({"target_tokens": target, "calibrated_tokens": calibrated, "markers": expected, "status": "PASS", "cold": cold, "warm": warm})
                ran += 1
                print(f"stage {target}: PASS prompt={warm['prompt_tokens']} cached={warm['cached_tokens']} ttft={warm['ttft_seconds']:.3f}s tps={warm['tokens_per_second']:.1f}")
            except RuntimeError as exc:
                # Unknown context commonly appears as a server rejection; retain the explicit evidence.
                summary["stages"].append({"target_tokens": target, "status": "FAIL", "reason": str(exc)})
                raise
        if not ran:
            raise RuntimeError("no requested stage ran")
        summary["status"] = "PASS"
        print(f"PASS: {ran} stages completed with exact retrieval and warm IMC reuse")
        return_code = 0
    except Exception as exc:  # produce a usable artifact for operational failures
        summary["error"] = str(exc)
        print(f"FAIL: {exc}", file=sys.stderr)
        return_code = 1
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(summary, indent=2) + "\n")
    print(f"saved summary to {args.out}")
    return return_code


if __name__ == "__main__":
    raise SystemExit(main())
