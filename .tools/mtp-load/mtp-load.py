# Releases concurrent chat requests through one barrier. The calibrated scenario
# saves request/response JSON plus summary.json when --out is set; the tic-tac-toe
# scenario validates known-good responses entirely in memory.

#!/usr/bin/env python3
"""Run deterministic concurrent chat requests against Kronk."""

from __future__ import annotations

import argparse
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import shutil
import threading
import time
import urllib.error
import urllib.request


TOPICS = (
    ("ASTRONOMY", "orbit nebula telescope galaxy comet"),
    ("COOKING", "recipe skillet rosemary pastry simmer"),
    ("GARDENING", "garden seedling compost orchard trellis"),
    ("ARCHITECTURE", "building archway masonry blueprint column"),
    ("MUSIC", "melody rhythm violin concert harmony"),
    ("SAILING", "harbor compass sailboat tide anchor"),
)

TIC_TAC_TOE_PROMPT = """Implement a complete two-player terminal tic-tac-toe game in Go using only the standard library.

Requirements:
- Represent the board as nine cells and display empty cells as positions 1 through 9.
- Alternate X and O turns, reject invalid or occupied positions, and detect all eight winning lines.
- Detect a draw only after checking for a winner.
- After each game, ask whether to play again and preserve X wins, O wins, and draw scores across games.
- Read complete input lines through one shared bufio.Reader.
- Keep the implementation concise and format it as idiomatic Go.

Return only the complete contents of main.go. Do not use tools and do not write any files."""


def arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--requests", type=int, required=True)
    parser.add_argument("--prompt-tokens", type=int)
    parser.add_argument("--max-tokens", type=int, required=True)
    parser.add_argument("--seed", type=int, required=True)
    parser.add_argument("--out", type=Path)
    parser.add_argument(
        "--scenario", choices=("calibrated", "tic-tac-toe"), default="calibrated"
    )
    args = parser.parse_args()
    if args.requests < 1:
        parser.error("--requests must be at least 1")
    if args.max_tokens < 1:
        parser.error("--max-tokens must be positive")
    if args.scenario == "calibrated" and (
        args.prompt_tokens is None or args.prompt_tokens < 1
    ):
        parser.error("--prompt-tokens must be positive for the calibrated scenario")
    return args


def headers() -> dict[str, str]:
    result = {"Content-Type": "application/json"}
    token = os.environ.get("KRONK_TOKEN")
    if token:
        result["Authorization"] = f"Bearer {token}"
    return result


def post(host: str, path: str, body: dict) -> dict:
    request = urllib.request.Request(
        f"{host.rstrip('/')}{path}",
        data=json.dumps(body).encode(),
        headers=headers(),
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=1800) as response:
            return json.load(response)
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode(errors="replace")
        raise RuntimeError(f"{path}: HTTP {exc.code}: {detail}") from exc


def get(host: str, path: str) -> dict | list:
    request = urllib.request.Request(
        f"{host.rstrip('/')}{path}", headers=headers(), method="GET"
    )
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)


def prompt_for(index: int, records: int) -> str:
    topic, vocabulary = TOPICS[index % len(TOPICS)]
    lines = [
        f"REQUEST-{index + 1} {topic}. Read these deterministic records carefully.",
    ]
    for record in range(records):
        lines.append(
            f"{topic} record {record:04d}: {vocabulary}; marker {index + 1}-{record:04d}."
        )
    lines.append(
        f"Return exactly five short bullets summarizing only the {topic.lower()} records."
    )
    return "\n".join(lines)


def token_count(host: str, model: str, prompt: str) -> int:
    response = post(
        host,
        "/v1/tokenize",
        {"model": model, "input": prompt, "apply_template": True},
    )
    return int(response["tokens"])


def calibrated_prompt(host: str, model: str, index: int, target: int) -> tuple[str, int]:
    low, high = 1, max(2, target)
    while token_count(host, model, prompt_for(index, high)) < target:
        high *= 2

    while low < high:
        middle = (low + high) // 2
        count = token_count(host, model, prompt_for(index, middle))
        if count < target:
            low = middle + 1
        else:
            high = middle

    prompt = prompt_for(index, low)
    return prompt, token_count(host, model, prompt)


def main() -> int:
    args = arguments()
    if args.out:
        if args.out.exists():
            shutil.rmtree(args.out)
        args.out.mkdir(parents=True, exist_ok=False)

    prepared = []
    for index in range(args.requests):
        if args.scenario == "tic-tac-toe":
            prompt = TIC_TAC_TOE_PROMPT
            count = token_count(args.host, args.model, prompt)
        else:
            prompt, count = calibrated_prompt(
                args.host, args.model, index, args.prompt_tokens
            )
        body = {
            "model": args.model,
            "stream": False,
            "temperature": 0,
            "seed": args.seed,
            "max_tokens": args.max_tokens,
            "enable_thinking": False,
            "messages": [{"role": "user", "content": prompt}],
        }
        if args.out:
            request_path = args.out / f"request-{index + 1}.json"
            request_path.write_text(json.dumps(body, indent=2) + "\n")
        prepared.append((index, count, body))
        print(f"prepared request {index + 1}: {count} templated prompt tokens")

    barrier = threading.Barrier(args.requests)

    def send(item: tuple[int, int, dict]) -> dict:
        index, calibrated_tokens, body = item
        barrier.wait()
        started = time.monotonic()
        try:
            response = post(args.host, "/v1/chat/completions", body)
        except Exception as exc:
            return {
                "request": index + 1,
                "calibrated_prompt_tokens": calibrated_tokens,
                "elapsed_seconds": round(time.monotonic() - started, 3),
                "passed": False,
                "error": str(exc),
                "usage": {},
            }
        elapsed = time.monotonic() - started
        if args.out:
            (args.out / f"response-{index + 1}.json").write_text(
                json.dumps(response, indent=2) + "\n"
            )
        usage = response.get("usage") or {}
        choices = response.get("choices") or []
        finish_reason = choices[0].get("finish_reason") if choices else None
        message = choices[0].get("message") if choices else None
        content = message.get("content", "") if isinstance(message, dict) else ""
        passed = bool(content) and finish_reason == "stop"
        if args.scenario == "tic-tac-toe":
            passed = passed and all(
                marker in content for marker in ("package main", "func main", "X", "O")
            )
        encoded = json.dumps(message, sort_keys=True, separators=(",", ":")).encode()
        return {
            "request": index + 1,
            "calibrated_prompt_tokens": calibrated_tokens,
            "elapsed_seconds": round(elapsed, 3),
            "finish_reason": finish_reason,
            "passed": passed,
            "output_sha256": hashlib.sha256(encoded).hexdigest(),
            "usage": usage,
        }

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.requests) as executor:
        futures = [executor.submit(send, item) for item in prepared]
        results = [future.result() for future in futures]

    results.sort(key=lambda result: result["request"])
    summary = {
        "host": args.host,
        "model": args.model,
        "requests": args.requests,
        "scenario": args.scenario,
        "requested_prompt_tokens": args.prompt_tokens,
        "max_tokens": args.max_tokens,
        "seed": args.seed,
        "results": results,
    }
    if args.out:
        (args.out / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")

    for result in results:
        usage = result["usage"]
        if error := result.get("error"):
            print(
                f"request {result['request']}: FAIL elapsed={result['elapsed_seconds']}s error={error}"
            )
            continue
        print(
            "request {request}: {verdict} elapsed={elapsed_seconds}s prompt={prompt} "
            "completion={completion} drafted={drafted} accepted={accepted} "
            "acceptance={acceptance} coverage={coverage} finish={finish}".format(
                request=result["request"],
                verdict="PASS" if result["passed"] else "FAIL",
                elapsed_seconds=result["elapsed_seconds"],
                prompt=usage.get("prompt_tokens"),
                completion=usage.get("completion_tokens"),
                drafted=usage.get("draft_tokens", 0),
                accepted=usage.get("draft_accepted_tokens", 0),
                acceptance=usage.get("draft_acceptance_rate", 0),
                coverage=usage.get("draft_coverage", 0),
                finish=result["finish_reason"],
            )
        )
    if args.out:
        print(f"saved experiment artifacts to {args.out}")

    passed = sum(result["passed"] for result in results)
    models = get(args.host, "/v1/kronk/models/ps")
    loaded = next(
        (
            model
            for model in models
            if model.get("id") == args.model or model.get("id", "").endswith(args.model)
        ),
        None,
    )
    slots = loaded.get("slots") if loaded else None
    one_slot = slots == 1
    print(f"completed {passed}/{args.requests} meaningful responses; model slots={slots}")
    if not one_slot:
        print(f"FAIL: expected one loaded slot for {args.model}, got {slots}")
    return 0 if passed == args.requests and one_slot else 1


if __name__ == "__main__":
    raise SystemExit(main())
