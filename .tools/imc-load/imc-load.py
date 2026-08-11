#!/usr/bin/env python3
"""Exercise long-running IMC conversations through a one-slot Kronk model."""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import os
from pathlib import Path
import time
import urllib.error
import urllib.request


def arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Run retained and evicting multi-turn conversations against a model "
            "configured with one slot and a bounded IMC session capacity."
        )
    )
    parser.add_argument("--host", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--turns", type=int, default=21)
    parser.add_argument("--target-tokens", type=int, default=30_000)
    parser.add_argument("--tokens-per-turn", type=int, default=1_400)
    parser.add_argument("--max-tokens", type=int, default=32)
    parser.add_argument("--capacity", type=int, default=3)
    parser.add_argument("--workers", type=int, default=2)
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
    if args.capacity < 1:
        parser.error("--capacity must be positive")
    if args.workers < 1:
        parser.error("--workers must be positive")

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


def model_matches(identifier: str, configured: str) -> bool:
    return identifier == configured or identifier.endswith(configured)


def loaded_model(args: argparse.Namespace) -> dict:
    models = request_json(args.host, "/v1/kronk/models/ps", 30)
    if not isinstance(models, list):
        raise RuntimeError("/v1/kronk/models/ps returned a non-list response")
    model = next(
        (
            item
            for item in models
            if model_matches(str(item.get("id", "")), args.model)
        ),
        None,
    )
    if model is None:
        raise RuntimeError(f"model {args.model!r} is not loaded")
    return model


def token_count(args: argparse.Namespace, text: str) -> int:
    response = request_json(
        args.host,
        "/v1/tokenize",
        30,
        {"model": args.model, "input": text, "apply_template": False},
    )
    if not isinstance(response, dict):
        raise RuntimeError("/v1/tokenize returned a non-object response")
    return int(response["tokens"])


def record_prompt(phase: str, session: int, turn: int, records: int) -> str:
    key = f"{phase.upper()}-SESSION-{session + 1}"
    lines = [
        f"Conversation key: {key}. This is turn {turn}.",
        "Retain the conversation and read these compact operational records:",
    ]
    for record in range(records):
        lines.append(
            f"record {record:04d}: latency budget retry owner archive signal metric policy."
        )
    lines.append(
        f"Reply with {key}-TURN-{turn} followed by one short sentence about record {records - 1:04d}."
    )
    return "\n".join(lines)


def calibrate_records(args: argparse.Namespace) -> int:
    low, high = 1, max(2, args.tokens_per_turn)
    while token_count(args, record_prompt("calibration", 0, 1, high)) < args.tokens_per_turn:
        high *= 2

    while low < high:
        middle = (low + high) // 2
        count = token_count(args, record_prompt("calibration", 0, 1, middle))
        if count < args.tokens_per_turn:
            low = middle + 1
        else:
            high = middle
    return low


def message_text(messages: list[dict]) -> str:
    return "\n".join(str(message.get("content", "")) for message in messages)


def imc_sessions(args: argparse.Namespace) -> list[dict]:
    sessions = request_json(args.host, "/v1/kronk/models/imc-sessions", 30)
    if not isinstance(sessions, list):
        raise RuntimeError("IMC sessions endpoint returned a non-list response")
    return [
        session
        for session in sessions
        if model_matches(str(session.get("model_id", "")), args.model)
    ]


def completion(
    args: argparse.Namespace,
    phase: str,
    session: int,
    turn: int,
    messages: list[dict],
    records: int,
) -> dict:
    user_content = record_prompt(phase, session, turn, records)
    request_messages = messages + [{"role": "user", "content": user_content}]
    body = {
        "model": args.model,
        "stream": False,
        "temperature": 0,
        "seed": args.seed,
        "max_tokens": args.max_tokens,
        "enable_thinking": False,
        "messages": request_messages,
    }

    started = time.monotonic()
    response = request_json(
        args.host, "/v1/chat/completions", args.timeout, body
    )
    elapsed = time.monotonic() - started
    if not isinstance(response, dict):
        raise RuntimeError("chat completions returned a non-object response")

    choices = response.get("choices") or []
    message = choices[0].get("message") if choices else None
    content = message.get("content", "") if isinstance(message, dict) else ""
    if not content:
        raise RuntimeError(f"{phase} session {session + 1} turn {turn}: empty response")

    usage = response.get("usage") or {}
    cached = (usage.get("prompt_tokens_details") or {}).get("cached_tokens", 0)
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
        "cached_tokens": int(cached),
        "finish_reason": choices[0].get("finish_reason") if choices else None,
    }


def run_phase(
    args: argparse.Namespace, phase: str, count: int, records: int
) -> dict:
    conversations = [
        [
            {
                "role": "system",
                "content": (
                    "You are participating in a deterministic long-running load test. "
                    f"Keep this conversation separate: {phase.upper()}-SESSION-{session + 1}."
                ),
            }
        ]
        for session in range(count)
    ]
    results: list[dict] = []

    print(f"\n{phase}: running {count} conversations x {args.turns} turns")
    with concurrent.futures.ThreadPoolExecutor(
        max_workers=min(args.workers, count)
    ) as executor:
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

            futures = [
                executor.submit(
                    completion,
                    args,
                    phase,
                    session,
                    turn,
                    conversations[session],
                    turn_records,
                )
                for session in range(count)
            ]
            turn_results = [future.result() for future in futures]
            results.extend(turn_results)

            cached = sum(result["cached_tokens"] > 0 for result in turn_results)
            minimum_prompt = min(result["prompt_tokens"] for result in turn_results)
            print(
                f"{phase} turn {turn:02d}: {len(turn_results)}/{count} complete, "
                f"cache hits={cached}, minimum prompt={minimum_prompt}"
            )

    final_results = [result for result in results if result["turn"] == args.turns]
    if min(result["prompt_tokens"] for result in final_results) < args.target_tokens:
        raise RuntimeError(
            f"{phase}: a final prompt did not reach {args.target_tokens} tokens"
        )

    # The first request's reusable prefix is only the small system message and
    # can fall below cache-min-tokens. Turn two builds a substantial reusable
    # prefix; retained conversations must hit that prefix from turn three on.
    later_results = [result for result in results if result["turn"] > 2]
    if count <= args.capacity and any(
        result["cached_tokens"] <= 0 for result in later_results
    ):
        misses = sum(result["cached_tokens"] <= 0 for result in later_results)
        raise RuntimeError(f"{phase}: expected IMC reuse, observed {misses} later-turn misses")

    sessions = imc_sessions(args)
    if len(sessions) != min(count, args.capacity):
        raise RuntimeError(
            f"{phase}: got {len(sessions)} IMC sessions for model, "
            f"want {min(count, args.capacity)}; verify imc-session-capacity={args.capacity}"
        )

    return {
        "phase": phase,
        "conversations": count,
        "turns": args.turns,
        "requests": len(results),
        "cache_hits": sum(result["cached_tokens"] > 0 for result in results),
        "imc_sessions_after": len(sessions),
        "minimum_final_prompt_tokens": min(
            result["prompt_tokens"] for result in final_results
        ),
        "results": results,
    }


def main() -> int:
    args = arguments()
    model = loaded_model(args)
    if model.get("slots") != 1:
        raise RuntimeError(
            f"expected one loaded slot for {args.model}, got {model.get('slots')}"
        )

    records = calibrate_records(args)
    calibrated = token_count(args, record_prompt("calibration", 0, 1, records))
    print(
        f"model={args.model} slots=1 capacity={args.capacity}; "
        f"calibrated turn payload={calibrated} tokens"
    )

    retained = run_phase(args, "retained", args.capacity, records)
    evicting = run_phase(args, "evicting", args.capacity + 2, records)
    summary = {
        "host": args.host,
        "model": args.model,
        "slots": 1,
        "capacity": args.capacity,
        "target_tokens": args.target_tokens,
        "phases": [retained, evicting],
    }
    if args.out:
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(json.dumps(summary, indent=2) + "\n")
        print(f"saved summary to {args.out}")

    total_requests = retained["requests"] + evicting["requests"]
    total_conversations = retained["conversations"] + evicting["conversations"]
    print(
        f"\nPASS: completed {total_requests} requests across "
        f"{total_conversations} long conversations; "
        f"retained IMC reuse and capacity-bound eviction both remained operational"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
