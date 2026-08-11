#!/usr/bin/env python3
"""Decode OpenAI-compatible streaming responses and compare semantic output."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import sys


def arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("files", nargs="+", type=Path, help="SSE response files")
    return parser.parse_args()


def decode(path: Path) -> dict:
    reasoning = []
    content = []
    calls: dict[int, dict] = {}
    finish_reason = None
    usage = None

    for number, line in enumerate(path.read_text().splitlines(), start=1):
        if not line.startswith("data:"):
            continue

        data = line[5:].lstrip()
        if not data or data == "[DONE]":
            continue

        try:
            event = json.loads(data)
        except json.JSONDecodeError as exc:
            raise ValueError(f"{path}:{number}: invalid JSON: {exc}") from exc

        if event.get("usage") is not None:
            usage = event["usage"]

        for choice in event.get("choices") or []:
            if choice.get("finish_reason") is not None:
                finish_reason = choice["finish_reason"]

            delta = choice.get("delta") or {}
            reasoning_value = delta.get("reasoning_content")
            if reasoning_value is None:
                reasoning_value = delta.get("reasoning")
            if reasoning_value:
                reasoning.append(reasoning_value)
            if delta.get("content"):
                content.append(delta["content"])

            for tool_call in delta.get("tool_calls") or []:
                index = int(tool_call.get("index", 0))
                call = calls.setdefault(
                    index,
                    {"type": tool_call.get("type"), "function": {"name": "", "arguments": ""}},
                )
                if tool_call.get("type") is not None:
                    call["type"] = tool_call["type"]
                function = tool_call.get("function") or {}
                call["function"]["name"] += function.get("name") or ""
                call["function"]["arguments"] += function.get("arguments") or ""

    semantic = {
        "finish_reason": finish_reason,
        "reasoning": "".join(reasoning),
        "content": "".join(content),
        "tool_calls": [calls[index] for index in sorted(calls)],
    }
    encoded = json.dumps(semantic, sort_keys=True, separators=(",", ":")).encode()
    return {
        "file": str(path),
        "semantic_sha256": hashlib.sha256(encoded).hexdigest(),
        **semantic,
        "usage": usage,
    }


def main() -> int:
    args = arguments()
    try:
        results = [decode(path) for path in args.files]
    except (OSError, ValueError) as exc:
        print(exc, file=sys.stderr)
        return 1

    for result in results:
        print(json.dumps(result, indent=2, ensure_ascii=False))

    if len(results) > 1:
        hashes = {result["semantic_sha256"] for result in results}
        print("MATCH" if len(hashes) == 1 else "DIVERGENCE")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
