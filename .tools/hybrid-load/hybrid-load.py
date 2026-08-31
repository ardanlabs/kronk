#!/usr/bin/env python3
"""Probe single-slot hybrid/MTP generation and incremental message caching."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import time
import urllib.error
import urllib.parse
import urllib.request


CHAT_PATH = "/v1/chat/completions"
CONTENT_LIMIT = 2_000


def arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--timeout", required=True, type=float)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    if args.timeout <= 0:
        parser.error("--timeout must be positive")
    return args


def headers() -> dict[str, str]:
    result = {"Content-Type": "application/json"}
    if token := os.environ.get("KRONK_TOKEN"):
        result["Authorization"] = f"Bearer {token}"
    return result


def request_json(
    host: str, path: str, timeout: float, body: dict | None = None
) -> dict | list:
    request = urllib.request.Request(
        f"{host.rstrip('/')}{path}",
        data=json.dumps(body).encode() if body is not None else None,
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
    return (
        identifier == configured
        or identifier.endswith(configured)
        or configured.endswith(identifier)
    )


def selected(items: object, model: str, key: str) -> dict | None:
    if not isinstance(items, list):
        return None
    return next(
        (
            item
            for item in items
            if isinstance(item, dict)
            and model_matches(str(item.get(key, "")), model)
        ),
        None,
    )


def warm_model(args: argparse.Namespace) -> None:
    response = request_json(
        args.host,
        CHAT_PATH,
        args.timeout,
        {
            "model": args.model,
            "stream": False,
            "max_tokens": 8,
            "enable_thinking": False,
            "messages": [{"role": "user", "content": "Reply OK."}],
        },
    )
    if not isinstance(response, dict) or not response.get("choices"):
        raise RuntimeError("model lazy-load request returned no completion")


def completion(args: argparse.Namespace, body: dict) -> dict:
    started = time.monotonic()
    response = request_json(args.host, CHAT_PATH, args.timeout, body)
    if not isinstance(response, dict):
        raise RuntimeError("chat completion returned a non-object response")
    choices = response.get("choices") or []
    message = choices[0].get("message", {}) if choices else {}
    content = message.get("content", "") if isinstance(message, dict) else ""
    return {
        "response_id": response.get("id"),
        "elapsed_seconds": round(time.monotonic() - started, 3),
        "content": content if isinstance(content, str) else str(content),
        "finish_reason": choices[0].get("finish_reason") if choices else None,
        "usage": response.get("usage") or {},
    }


def cancel_after_content(args: argparse.Namespace, body: dict) -> dict:
    request = urllib.request.Request(
        f"{args.host.rstrip('/')}{CHAT_PATH}",
        data=json.dumps(body).encode(),
        headers=headers(),
        method="POST",
    )
    started = time.monotonic()
    pieces: list[str] = []
    response_id = None
    try:
        response = urllib.request.urlopen(request, timeout=args.timeout)
        try:
            for raw_line in response:
                line = raw_line.decode(errors="replace").strip()
                if not line.startswith("data:"):
                    continue
                data = line.removeprefix("data:").strip()
                if data == "[DONE]":
                    break
                chunk = json.loads(data)
                response_id = chunk.get("id") or response_id
                choices = chunk.get("choices") or []
                delta = choices[0].get("delta") if choices else None
                piece = delta.get("content", "") if isinstance(delta, dict) else ""
                if piece:
                    pieces.append(piece)
                    break
        finally:
            response.close()  # Deliberately disconnect before stream completion.
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode(errors="replace")
        raise RuntimeError(f"{CHAT_PATH}: HTTP {exc.code}: {detail}") from exc
    return {
        "response_id": response_id,
        "elapsed_seconds": round(time.monotonic() - started, 3),
        "content_received": "".join(pieces),
        "cancelled_by_client": bool(pieces),
    }


def cached_tokens(result: dict) -> int:
    details = result.get("usage", {}).get("prompt_tokens_details") or {}
    return int(details.get("cached_tokens", 0))


def audit_result(result: dict) -> dict:
    copy = dict(result)
    for key in ("content", "content_received"):
        if key in copy:
            text = copy.pop(key)
            copy[f"{key}_characters"] = len(text)
            copy[f"{key}_preview"] = text[:CONTENT_LIMIT]
            copy[f"{key}_truncated"] = len(text) > CONTENT_LIMIT
    return copy


def metadata_record(detail: dict | None) -> dict:
    if not detail:
        return {"available": False}
    metadata = detail.get("metadata") or {}
    config = detail.get("model_config") or {}
    architecture = next(
        (v for k, v in metadata.items() if k.endswith(".architecture")),
        metadata.get("general.architecture"),
    )
    metadata_context = next(
        (v for k, v in metadata.items() if k.endswith(".context_length")), None
    )
    return {
        "available": True,
        "id": detail.get("id"),
        "architecture": architecture,
        "context_window": config.get("context-window") or metadata_context,
        "model_config": config,
    }


def main() -> int:
    args = arguments()
    failures: list[str] = []
    summary: dict = {
        "passed": False,
        "failures": failures,
        "host": args.host,
        "model": args.model,
        "configuration": {"timeout_seconds": args.timeout, "seed": 42},
    }
    try:
        processes = request_json(args.host, "/v1/kronk/models/ps", min(args.timeout, 30))
        loaded = selected(processes, args.model, "id")
        if loaded is None:
            print(f"model={args.model} is not loaded; loading it now")
            warm_model(args)
            processes = request_json(
                args.host, "/v1/kronk/models/ps", min(args.timeout, 30)
            )
            loaded = selected(processes, args.model, "id")
        if loaded is None:
            raise RuntimeError(f"model did not appear after loading: {args.model}")
        if loaded.get("slots") != 1:
            failures.append(
                f"selected model must have exactly one slot; got {loaded.get('slots')!r}"
            )
        summary["loaded_model"] = loaded

        diagnostics = request_json(
            args.host, "/v1/kronk/models/slots", min(args.timeout, 30)
        )
        engine = selected(diagnostics, args.model, "model_id")
        if engine is None:
            raise RuntimeError("selected model has no scheduler diagnostics")
        slots = engine.get("slots")
        if not isinstance(slots, list):
            failures.append("scheduler diagnostics do not contain a slot list")
            slots = []
        for field in ("mtp", "ndraft"):
            if field not in engine:
                failures.append(f"scheduler diagnostics omit {field}")
        if len(slots) != 1:
            failures.append(f"scheduler must report exactly one slot; got {len(slots)}")
        summary["scheduler"] = {
            "model_id": engine.get("model_id"),
            "mtp": engine.get("mtp"),
            "ndraft": engine.get("ndraft"),
            "slot_count": len(slots),
        }

        # Encode the entire identifier as one path segment. Some configured IDs
        # contain repository separators, so failure here is informational rather
        # than a reason to skip the model-backed integrity checks.
        try:
            encoded_model = urllib.parse.quote(args.model, safe="")
            detail = request_json(
                args.host,
                f"/v1/kronk/models/{encoded_model}",
                min(args.timeout, 30),
            )
            summary["model_metadata"] = metadata_record(
                detail if isinstance(detail, dict) else None
            )
        except Exception as exc:
            summary["model_metadata"] = {
                "available": False,
                "error": str(exc),
                "model_family": loaded.get("model_family"),
            }

        run_id = str(time.time_ns())
        base_marker = f"HYBRID-BASE-{run_id}"
        base_prompt = (
            "Study this stable context, then obey the final instruction.\n"
            + "\n".join(
                f"Context record {number:02d}: amber cedar delta harbor quartz."
                for number in range(48)
            )
            + f"\nReply with exactly {base_marker} and nothing else."
        )
        messages = [{"role": "user", "content": base_prompt}]
        body = {
            "model": args.model,
            "stream": False,
            "temperature": 0,
            "seed": 42,
            "max_tokens": 64,
            "enable_thinking": False,
            "messages": messages,
        }
        cold = completion(args, body)
        repeat = completion(args, body)
        if not cold["content"]:
            failures.append("cold deterministic response was empty")
        if cold["content"].strip() != base_marker:
            failures.append("cold deterministic response did not exactly match its marker")
        if cold["content"] != repeat["content"]:
            failures.append("exact deterministic repeat content differed from cold content")
        if cached_tokens(repeat) <= 0:
            failures.append("exact repeat reported no cached tokens")

        appended_marker = f"HYBRID-APPEND-{run_id}"
        appended_messages = messages + [
            {"role": "assistant", "content": cold["content"]},
            {
                "role": "user",
                "content": f"Now reply with exactly {appended_marker} and nothing else.",
            },
        ]
        appended_body = {**body, "messages": appended_messages}
        appended = completion(args, appended_body)
        if not appended["content"]:
            failures.append("appended-turn response was empty")
        if appended["content"].strip() != appended_marker:
            failures.append("appended-turn response did not exactly match its marker")
        if cached_tokens(appended) <= 0:
            failures.append("appended turn reported no cached tokens")

        stale_marker = f"HYBRID-STALE-{run_id}"
        cancel_body = {
            **body,
            "stream": True,
            "stream_options": {"include_usage": True},
            "max_tokens": 1024,
            "messages": [
                {
                    "role": "user",
                    "content": (
                        f"Output {stale_marker} on every line and continue until the "
                        "output limit. Do not output anything else."
                    ),
                }
            ],
        }
        cancelled = cancel_after_content(args, cancel_body)
        if not cancelled["content_received"]:
            failures.append("cancel stream produced no content before disconnect")

        recovery_marker = f"HYBRID-RECOVERY-{run_id}"
        recovery_body = {
            **body,
            "messages": [
                {
                    "role": "user",
                    "content": f"Reply with exactly {recovery_marker} and nothing else.",
                }
            ],
        }
        recovery = completion(args, recovery_body)
        if recovery["content"].strip() != recovery_marker:
            failures.append("recovery response did not exactly match its marker")
        if stale_marker in recovery["content"]:
            failures.append("recovery response leaked the cancelled stream marker")

        generation_results = [cold, repeat, appended, recovery]
        draft_tokens = sum(int(r["usage"].get("draft_tokens", 0)) for r in generation_results)
        completion_tokens = sum(
            int(r["usage"].get("completion_tokens", 0)) for r in generation_results
        )
        weighted_coverage_tokens = sum(
            float(r["usage"].get("draft_coverage", 0))
            * int(r["usage"].get("completion_tokens", 0))
            for r in generation_results
        )
        aggregate_coverage = (
            weighted_coverage_tokens / completion_tokens if completion_tokens else 0.0
        )
        mtp = engine.get("mtp") is True
        if mtp and draft_tokens <= 0:
            failures.append("MTP scheduler produced no aggregate draft tokens")
        if mtp and aggregate_coverage <= 0:
            failures.append("MTP scheduler produced no aggregate draft coverage")
        summary["generation_usage"] = {
            "mode": "mtp" if mtp else "target-only",
            "requests": len(generation_results),
            "completion_tokens": completion_tokens,
            "draft_tokens": draft_tokens,
            "draft_coverage": aggregate_coverage,
        }
        summary["audit"] = {
            "requests": {
                "cold": body,
                "repeat": body,
                "appended": appended_body,
                "cancelled_stream": cancel_body,
                "recovery": recovery_body,
            },
            "results": {
                "cold": audit_result(cold),
                "repeat": audit_result(repeat),
                "appended": audit_result(appended),
                "cancelled_stream": audit_result(cancelled),
                "recovery": audit_result(recovery),
            },
        }
        summary["passed"] = not failures
    except Exception as exc:
        failures.append(str(exc))
    finally:
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(json.dumps(summary, indent=2) + "\n")

    print(f"{'PASS' if summary['passed'] else 'FAIL'}: saved summary to {args.out}")
    for failure in failures:
        print(f"  {failure}")
    return 0 if summary["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
