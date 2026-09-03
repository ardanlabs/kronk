#!/usr/bin/env python3
"""Check multimodal correctness, IMC reuse, and state isolation on one model slot."""

from __future__ import annotations

import argparse
import base64
import json
import mimetypes
import os
from pathlib import Path
import time
import urllib.error
import urllib.parse
import urllib.request


def arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--image", required=True, type=Path)
    parser.add_argument("--max-tokens", required=True, type=int)
    parser.add_argument("--timeout", required=True, type=float)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument(
        "--expect",
        action="append",
        default=[],
        help="expected image subject term; repeat or use comma-separated terms (default: giraffe)",
    )
    args = parser.parse_args()
    if not args.image.is_file():
        parser.error(f"--image is not a file: {args.image}")
    if args.max_tokens <= 0:
        parser.error("--max-tokens must be positive")
    if args.timeout <= 0:
        parser.error("--timeout must be positive")
    args.expect = [term.strip() for value in args.expect for term in value.split(",") if term.strip()]
    if not args.expect:
        args.expect = ["giraffe"]
    return args


def headers() -> dict[str, str]:
    result = {"Content-Type": "application/json"}
    if token := os.environ.get("KRONK_TOKEN"):
        result["Authorization"] = f"Bearer {token}"
    return result


def request_json(
    args: argparse.Namespace, path: str, body: dict | None = None
) -> dict | list:
    request = urllib.request.Request(
        f"{args.host.rstrip('/')}{path}",
        data=json.dumps(body).encode() if body is not None else None,
        headers=headers(),
        method="POST" if body is not None else "GET",
    )
    try:
        with urllib.request.urlopen(request, timeout=args.timeout) as response:
            return json.load(response)
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode(errors="replace")
        raise RuntimeError(f"{path}: HTTP {exc.code}: {detail}") from exc


def model_matches(identifier: str, configured: str) -> bool:
    return identifier == configured or identifier.endswith(configured) or configured.endswith(identifier)


def loaded_model(models: object, model: str) -> dict | None:
    if not isinstance(models, list):
        return None
    return next(
        (item for item in models if model_matches(str(item.get("id", "")), model)),
        None,
    )


def compact(text: str, limit: int = 500) -> str:
    value = " ".join(text.split())
    return value if len(value) <= limit else value[: limit - 1] + "…"


def completion(args: argparse.Namespace, content: str | list[dict]) -> dict:
    started = time.monotonic()
    response = request_json(
        args,
        "/v1/chat/completions",
        {
            "model": args.model,
            "stream": False,
            "temperature": 0,
            "seed": 42,
            "max_tokens": args.max_tokens,
            "enable_thinking": False,
            "messages": [{"role": "user", "content": content}],
        },
    )
    duration = time.monotonic() - started
    if not isinstance(response, dict) or not response.get("choices"):
        raise RuntimeError("chat completion returned no choices")
    choice = response["choices"][0]
    text = str((choice.get("message") or {}).get("content") or "")
    return {
        "text": text,
        "summary_text": compact(text),
        "duration_seconds": round(duration, 6),
        "usage": response.get("usage", {}),
        "finish_reason": choice.get("finish_reason"),
        "response_id": response.get("id"),
    }


def inspect(args: argparse.Namespace, path: str, diagnostics: dict) -> object | None:
    try:
        value = request_json(args, path)
        diagnostics[path] = value
        return value
    except Exception as exc:
        diagnostics[path] = {"unavailable": str(exc)}
        return None


def main() -> int:
    args = arguments()
    failures: list[str] = []
    responses: dict[str, dict] = {}
    diagnostics: dict = {}
    run_id = str(time.time_ns())
    try:
        before = inspect(args, "/v1/kronk/models/ps", diagnostics)
        if loaded_model(before, args.model) is None:
            print(f"model={args.model} is not loaded; loading it now")
            completion(args, "Hello. Reply with OK.")

        after = inspect(args, "/v1/kronk/models/ps", diagnostics)
        detail_path = "/v1/kronk/models/" + urllib.parse.quote(args.model, safe="")
        detail = inspect(args, detail_path, diagnostics)
        selected = loaded_model(after, args.model)
        if isinstance(after, list) and selected is None:
            failures.append(f"model did not appear after loading: {args.model}")
        for source, value in (("process listing", selected), ("model config", detail)):
            if isinstance(value, dict) and value.get("has_projection") is False:
                failures.append(f"{source} reports has_projection=false")

        media_type = mimetypes.guess_type(args.image.name)[0] or "image/jpeg"
        image_url = f"data:{media_type};base64,{base64.b64encode(args.image.read_bytes()).decode()}"
        first_prefix = f"MEDIA-OBSERVATION-{run_id}:"
        image_content = [
            {"type": "text", "text": f"Describe the main subject concisely. Start your response with exactly {first_prefix}"},
            {"type": "image_url", "image_url": {"url": image_url}},
        ]
        responses["first_image"] = completion(args, image_content)
        first_text = responses["first_image"]["text"].strip()
        if not first_text.startswith(first_prefix) or not first_text[len(first_prefix):].strip():
            failures.append(f"first image response lacks nonempty {first_prefix} prefix")
        if not any(term.casefold() in first_text.casefold() for term in args.expect):
            failures.append("first image response lacks an expected subject term")

        marker = f"TEXT-ONLY-{run_id}"
        responses["text_only"] = completion(
            args,
            f"Reply with exactly this marker and nothing else: {marker}",
        )
        text_only = responses["text_only"]["text"].strip()
        if text_only != marker:
            failures.append("text-only response did not exactly match its marker")
        if first_prefix in text_only:
            failures.append(f"text-only response leaked {first_prefix}")

        responses["repeat_image"] = completion(args, image_content)
        repeat_text = responses["repeat_image"]["text"].strip()
        if repeat_text != first_text:
            failures.append("repeat image response differs from the first deterministic response")
        if not any(term.casefold() in repeat_text.casefold() for term in args.expect):
            failures.append("repeat image response lacks an expected subject term")
        repeat_usage = responses["repeat_image"].get("usage", {})
        repeat_details = repeat_usage.get("prompt_tokens_details") or {}
        if int(repeat_details.get("cached_tokens", 0)) <= 0:
            failures.append("repeat image response reported no cached tokens")
    except Exception as exc:
        failures.append(str(exc))

    summary = {
        "passed": not failures,
        "failures": failures,
        "host": args.host,
        "model": args.model,
        "run_id": run_id,
        "image_path": str(args.image),
        "configuration": {
            "max_tokens": args.max_tokens,
            "timeout_seconds": args.timeout,
            "expected_subject_terms": args.expect,
        },
        "model_diagnostics": diagnostics,
        "responses": {
            name: {key: value for key, value in result.items() if key != "text"}
            for name, result in responses.items()
        },
    }
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(summary, indent=2) + "\n")
    print(f"{'PASS' if summary['passed'] else 'FAIL'}: saved summary to {args.out}")
    for failure in failures:
        print(f"  {failure}")
    return 0 if summary["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
