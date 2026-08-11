#!/usr/bin/env python3
"""Verify parallel batch generation remains isolated over long conversations."""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import os
import re
import threading
import time
import urllib.error
import urllib.request
from pathlib import Path


def arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Run synchronized, long-lived conversations against a multi-slot model "
            "and verify that their responses remain isolated."
        )
    )
    parser.add_argument("--host", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--turns", type=int, default=21)
    parser.add_argument("--target-tokens", type=int, default=30_000)
    parser.add_argument("--tokens-per-turn", type=int, default=1_400)
    parser.add_argument("--max-tokens", type=int, default=128)
    parser.add_argument("--slots", type=int, default=3)
    parser.add_argument("--conversations", type=int, default=3)
    parser.add_argument("--timeout", type=int, default=1_800)
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--out", type=Path)
    args = parser.parse_args()

    if args.turns < 21:
        parser.error("--turns must be at least 21")
    if args.target_tokens < 30_000:
        parser.error("--target-tokens must be at least 30000")
    if args.tokens_per_turn < 1:
        parser.error("--tokens-per-turn must be positive")
    if args.max_tokens < 1:
        parser.error("--max-tokens must be positive")
    if args.slots < 2:
        parser.error("--slots must be at least 2")
    if args.conversations != args.slots:
        parser.error("--conversations must equal --slots")

    return args


def headers() -> dict[str, str]:
    result = {"Content-Type": "application/json"}
    if token := os.environ.get("KRONK_TOKEN"):
        result["Authorization"] = f"Bearer {token}"
    return result


def request_json(
    host: str, path: str, timeout: int, body: dict | None = None
) -> dict | list:
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(
        f"{host.rstrip('/')}{path}",
        data=data,
        headers=headers(),
        method="POST" if body is not None else "GET",
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return json.load(response)
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode(errors="replace")
        raise RuntimeError(f"{path}: HTTP {exc.code}: {detail}") from exc


def request_stream(
    host: str, path: str, timeout: int, body: dict
) -> tuple[str, dict, float]:
    request = urllib.request.Request(
        f"{host.rstrip('/')}{path}",
        data=json.dumps(body).encode(),
        headers=headers(),
        method="POST",
    )
    content: list[str] = []
    usage: dict = {}
    first_content_at = 0.0
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            for raw_line in response:
                line = raw_line.decode(errors="replace").strip()
                if not line.startswith("data:"):
                    continue
                data = line.removeprefix("data:").strip()
                if data == "[DONE]":
                    break
                chunk = json.loads(data)
                if chunk_usage := chunk.get("usage"):
                    usage = chunk_usage
                choices = chunk.get("choices") or []
                delta = choices[0].get("delta") if choices else None
                piece = delta.get("content", "") if isinstance(delta, dict) else ""
                if piece:
                    if not first_content_at:
                        first_content_at = time.monotonic()
                    content.append(piece)
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode(errors="replace")
        raise RuntimeError(f"{path}: HTTP {exc.code}: {detail}") from exc

    return "".join(content), usage, first_content_at


def model_matches(identifier: str, configured: str) -> bool:
    return (
        identifier == configured
        or identifier.endswith(configured)
        or configured.endswith(identifier)
    )


def loaded_model(args: argparse.Namespace) -> dict | None:
    models = request_json(args.host, "/v1/kronk/models/ps", 30)
    if not isinstance(models, list):
        raise RuntimeError("/v1/kronk/models/ps returned a non-list response")
    return next(
        (
            item
            for item in models
            if model_matches(str(item.get("id", "")), args.model)
        ),
        None,
    )


def warm_model(args: argparse.Namespace) -> None:
    response = request_json(
        args.host,
        "/v1/chat/completions",
        args.timeout,
        {
            "model": args.model,
            "stream": False,
            "max_tokens": 16,
            "enable_thinking": False,
            "messages": [{"role": "user", "content": "Hello model."}],
        },
    )
    if not isinstance(response, dict) or not response.get("choices"):
        raise RuntimeError("model warm-up returned no completion")


def token_count(args: argparse.Namespace, text: str) -> int:
    response = request_json(
        args.host,
        "/v1/tokenize",
        args.timeout,
        {"model": args.model, "input": text, "apply_template": False},
    )
    if not isinstance(response, dict):
        raise RuntimeError("/v1/tokenize returned a non-object response")
    return int(response["tokens"])


def conversation_key(session: int) -> str:
    return f"BATCH-SESSION-{session + 1:02d}"


def record_prompt(session: int, turn: int, records: int) -> str:
    key = conversation_key(session)
    lines = [
        f"Conversation key: {key}. This is turn {turn}.",
        "Retain the conversation and read these compact operational records:",
    ]
    for record in range(records):
        lines.append(
            f"record {record:04d}: latency budget retry owner archive signal metric policy."
        )
    lines.append(
        f"Reply by repeating {key}-TURN-{turn} exactly 48 times, separated by spaces. "
        "Do not output anything else."
    )
    return "\n".join(lines)


def calibrate_records(args: argparse.Namespace) -> int:
    low, high = 1, max(2, args.tokens_per_turn)
    while token_count(args, record_prompt(0, 1, high)) < args.tokens_per_turn:
        high *= 2

    while low < high:
        middle = (low + high) // 2
        count = token_count(args, record_prompt(0, 1, middle))
        if count < args.tokens_per_turn:
            low = middle + 1
        else:
            high = middle
    return low


def message_text(messages: list[dict]) -> str:
    return "\n".join(str(message.get("content", "")) for message in messages)


def completion(
    args: argparse.Namespace,
    session: int,
    turn: int,
    messages: list[dict],
    records: int,
    barrier: threading.Barrier,
) -> dict:
    user_content = record_prompt(session, turn, records)
    request_messages = messages + [{"role": "user", "content": user_content}]
    body = {
        "model": args.model,
        "stream": True,
        "stream_options": {"include_usage": True},
        "temperature": 0,
        "seed": args.seed,
        "max_tokens": args.max_tokens,
        "enable_thinking": False,
        "messages": request_messages,
    }

    barrier.wait()
    started = time.monotonic()
    content, usage, first_content_at = request_stream(
        args.host, "/v1/chat/completions", args.timeout, body
    )
    finished_at = time.monotonic()
    elapsed = finished_at - started
    if not content:
        raise RuntimeError(f"session {session + 1} turn {turn}: empty response")

    expected_marker = f"{conversation_key(session)}-TURN-{turn}"
    observed_markers = re.findall(r"BATCH-SESSION-\d+-TURN-\d+", content)
    if expected_marker not in observed_markers:
        raise RuntimeError(
            f"session {session + 1} turn {turn}: response lost marker "
            f"{expected_marker!r}: {content!r}"
        )
    foreign_markers = set(observed_markers) - {expected_marker}
    if foreign_markers:
        raise RuntimeError(
            f"session {session + 1} turn {turn}: response contained foreign "
            f"conversation markers {sorted(foreign_markers)}: {content!r}"
        )
    if observed_markers.count(expected_marker) < 8:
        raise RuntimeError(
            f"session {session + 1} turn {turn}: response produced only "
            f"{observed_markers.count(expected_marker)} canaries, want at least 8"
        )

    messages.extend(
        [
            {"role": "user", "content": user_content},
            {"role": "assistant", "content": content},
        ]
    )
    return {
        "session": session + 1,
        "turn": turn,
        "elapsed_seconds": round(elapsed, 3),
        "prompt_tokens": int(usage.get("prompt_tokens", 0)),
        "completion_tokens": int(usage.get("completion_tokens", 0)),
        "first_content_seconds": round(first_content_at - started, 3),
        "_first_content_at": first_content_at,
        "_finished_at": finished_at,
    }


def run_load(args: argparse.Namespace, records: int) -> dict:
    count = args.conversations
    conversations = [
        [
            {
                "role": "system",
                "content": (
                    "You are participating in a deterministic long-running load test. "
                    f"Keep this conversation separate: {conversation_key(session)}."
                ),
            }
        ]
        for session in range(count)
    ]
    results: list[dict] = []

    print(f"\nrunning {count} conversations x {args.turns} synchronized turns")
    with concurrent.futures.ThreadPoolExecutor(max_workers=count) as executor:
        for turn in range(1, args.turns + 1):
            turn_records = records
            if turn == args.turns:
                current_tokens = min(
                    token_count(args, message_text(messages))
                    for messages in conversations
                )
                remaining = args.target_tokens - current_tokens
                if remaining > args.tokens_per_turn:
                    turn_records = max(records, records * remaining // args.tokens_per_turn)

            barrier = threading.Barrier(count)
            futures = [
                executor.submit(
                    completion,
                    args,
                    session,
                    turn,
                    conversations[session],
                    turn_records,
                    barrier,
                )
                for session in range(count)
            ]
            turn_results = [future.result() for future in futures]

            latest_first_content = max(
                result["_first_content_at"] for result in turn_results
            )
            earliest_finish = min(result["_finished_at"] for result in turn_results)
            if latest_first_content >= earliest_finish:
                raise RuntimeError(
                    f"turn {turn}: requests did not execute in parallel; "
                    "a response completed before every conversation produced output"
                )
            overlap_seconds = earliest_finish - latest_first_content
            for result in turn_results:
                del result["_first_content_at"]
                del result["_finished_at"]
                result["parallel_overlap_seconds"] = round(overlap_seconds, 3)
            results.extend(turn_results)

            minimum_prompt = min(result["prompt_tokens"] for result in turn_results)
            maximum_elapsed = max(result["elapsed_seconds"] for result in turn_results)
            print(
                f"turn {turn:02d}: {len(turn_results)}/{count} complete, "
                f"minimum prompt={minimum_prompt}, slowest={maximum_elapsed:.3f}s, "
                f"parallel overlap={overlap_seconds:.3f}s"
            )

    final_results = [result for result in results if result["turn"] == args.turns]
    if min(result["prompt_tokens"] for result in final_results) < args.target_tokens:
        raise RuntimeError(
            f"a final prompt did not reach {args.target_tokens} tokens"
        )

    return {
        "conversations": count,
        "turns": args.turns,
        "requests": len(results),
        "minimum_final_prompt_tokens": min(
            result["prompt_tokens"] for result in final_results
        ),
        "results": results,
    }


def main() -> int:
    args = arguments()
    model = loaded_model(args)
    if model is None:
        print(f"model={args.model} is not loaded; loading it now")
        warm_model(args)
        model = loaded_model(args)
    if model is None:
        raise RuntimeError(f"model {args.model!r} did not appear after loading")
    if model.get("slots") != args.slots:
        raise RuntimeError(
            f"expected {args.slots} loaded slots for {args.model}, "
            f"got {model.get('slots')}; configure nseq-max={args.slots}"
        )

    records = calibrate_records(args)
    calibrated = token_count(args, record_prompt(0, 1, records))
    print(
        f"model={args.model} slots={args.slots} conversations={args.conversations}; "
        f"calibrated turn payload={calibrated} tokens"
    )

    load = run_load(args, records)
    summary = {
        "host": args.host,
        "model": args.model,
        "slots": args.slots,
        "target_tokens": args.target_tokens,
        **load,
    }
    if args.out:
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(json.dumps(summary, indent=2) + "\n")
        print(f"saved summary to {args.out}")

    print(
        f"\nPASS: completed {load['requests']} requests across "
        f"{load['conversations']} long conversations on {args.slots} slots; "
        "all response markers remained isolated"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
