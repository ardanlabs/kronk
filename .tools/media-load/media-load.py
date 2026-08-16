#!/usr/bin/env python3
"""Verify media prefill does not stall concurrent streamed generation."""

from __future__ import annotations

import argparse
import base64
import json
import mimetypes
import os
from pathlib import Path
import threading
import time
import urllib.error
import urllib.request


POLL_SECONDS = 0.02
READY_CONTENT_EVENTS = 5


def arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--image", required=True, type=Path)
    parser.add_argument("--generation-max-tokens", required=True, type=int)
    parser.add_argument("--image-max-tokens", required=True, type=int)
    parser.add_argument("--max-generation-content-event-gap", required=True, type=float)
    parser.add_argument("--timeout", required=True, type=float)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    if not args.image.is_file():
        parser.error(f"--image is not a file: {args.image}")
    for name in ("generation_max_tokens", "image_max_tokens", "timeout"):
        if getattr(args, name) <= 0:
            parser.error(f"--{name.replace('_', '-')} must be positive")
    if args.max_generation_content_event_gap <= 0:
        parser.error("--max-generation-content-event-gap must be positive")
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
            "messages": [{"role": "user", "content": "Hello world."}],
        },
    )
    if not isinstance(response, dict) or not response.get("choices"):
        raise RuntimeError("model warm-up returned no completion")


def model_matches(identifier: str, configured: str) -> bool:
    return identifier == configured or identifier.endswith(configured) or configured.endswith(identifier)


def stream_request(args: argparse.Namespace, body: dict, result: dict, ready=None) -> None:
    request = urllib.request.Request(
        f"{args.host.rstrip('/')}/v1/chat/completions",
        data=json.dumps(body).encode(),
        headers=headers(),
        method="POST",
    )
    result["started_at"] = time.monotonic()
    pieces: list[str] = []
    try:
        with urllib.request.urlopen(request, timeout=args.timeout) as response:
            for raw_line in response:
                line = raw_line.decode(errors="replace").strip()
                if not line.startswith("data:"):
                    continue
                data = line.removeprefix("data:").strip()
                if data == "[DONE]":
                    break
                chunk = json.loads(data)
                if chunk.get("id"):
                    result["response_id"] = chunk["id"]
                if chunk.get("usage"):
                    result["usage"] = chunk["usage"]
                choices = chunk.get("choices") or []
                if choices and choices[0].get("finish_reason") is not None:
                    result["finish_reason"] = choices[0]["finish_reason"]
                delta = choices[0].get("delta") if choices else None
                piece = delta.get("content", "") if isinstance(delta, dict) else ""
                if piece:
                    pieces.append(piece)
                    result["content_event_times"].append(time.monotonic())
                    if ready and len(result["content_event_times"]) >= READY_CONTENT_EVENTS:
                        ready.set()
        result["content_characters"] = len("".join(pieces))
    except Exception as exc:  # Preserve errors for the summary and main thread.
        result["error"] = str(exc)
    finally:
        result["finished_at"] = time.monotonic()
        if ready:
            ready.set()


def selected_engine(slots: object, model: str) -> dict | None:
    if not isinstance(slots, list):
        return None
    return next(
        (item for item in slots if model_matches(str(item.get("model_id", "")), model)),
        None,
    )


def rel(value: float | None, origin: float) -> float | None:
    return round(value - origin, 6) if value is not None else None


def main() -> int:
    args = arguments()
    origin = time.monotonic()
    generation = {"content_event_times": [], "usage": {}, "finish_reason": None}
    image = {"content_event_times": [], "usage": {}, "finish_reason": None}
    observations: list[dict] = []
    failures: list[str] = []
    summary: dict = {}
    try:
        models = request_json(args.host, "/v1/kronk/models/ps", min(args.timeout, 30))
        if not isinstance(models, list):
            raise RuntimeError("/v1/kronk/models/ps returned a non-list response")
        loaded = next(
            (item for item in models if model_matches(str(item.get("id", "")), args.model)),
            None,
        )
        if loaded is None:
            print(f"model={args.model} is not loaded; loading it now")
            warm_model(args)
            models = request_json(
                args.host, "/v1/kronk/models/ps", min(args.timeout, 30)
            )
            if not isinstance(models, list):
                raise RuntimeError("/v1/kronk/models/ps returned a non-list response")
            loaded = next(
                (
                    item
                    for item in models
                    if model_matches(str(item.get("id", "")), args.model)
                ),
                None,
            )
        if loaded is None:
            raise RuntimeError(f"model did not appear after loading: {args.model}")
        if loaded.get("has_projection") is False:
            raise RuntimeError(f"loaded model reports no multimodal projection: {args.model}")

        initial_engine = selected_engine(
            request_json(args.host, "/v1/kronk/models/slots", min(args.timeout, 30)),
            args.model,
        )
        if initial_engine is None:
            raise RuntimeError("selected model has no generation scheduler diagnostics")
        if len(initial_engine.get("slots") or []) < 2:
            raise RuntimeError("selected model must be loaded with at least 2 slots")

        run_id = str(time.time_ns())
        generation_body = {
            "model": args.model,
            "stream": True,
            "stream_options": {"include_usage": True},
            "temperature": 0,
            "seed": 42,
            "max_tokens": args.generation_max_tokens,
            "enable_thinking": False,
            "messages": [{"role": "user", "content": "Write the integers from 1 upward, one integer per line. Continue without commentary until the output limit."}],
        }
        media_type = mimetypes.guess_type(args.image.name)[0] or "image/jpeg"
        image_url = f"data:{media_type};base64,{base64.b64encode(args.image.read_bytes()).decode()}"
        image_body = {
            "model": args.model,
            "stream": True,
            "stream_options": {"include_usage": True},
            "temperature": 0,
            "seed": 42,
            "max_tokens": args.image_max_tokens,
            "enable_thinking": False,
            "messages": [{"role": "user", "content": [
                {"type": "text", "text": f"Media load probe {run_id}. Describe the main subject of this image briefly."},
                {"type": "image_url", "image_url": {"url": image_url}},
            ]}],
        }

        ready = threading.Event()
        generation_thread = threading.Thread(target=stream_request, args=(args, generation_body, generation, ready))
        generation_thread.start()
        if not ready.wait(args.timeout) or len(generation["content_event_times"]) < READY_CONTENT_EVENTS:
            failures.append("text generation ended or timed out before several content events")
        image_thread = threading.Thread(target=stream_request, args=(args, image_body, image))
        image_thread.start()

        deadline = time.monotonic() + args.timeout
        while (generation_thread.is_alive() or image_thread.is_alive()) and time.monotonic() < deadline:
            observed_at = time.monotonic()
            try:
                engine = selected_engine(request_json(args.host, "/v1/kronk/models/slots", 5), args.model)
                if engine:
                    active = [slot for slot in engine.get("slots", []) if slot.get("phase") != "idle"]
                    observations.append({
                        "at": observed_at,
                        "slots": [{"id": slot.get("id"), "phase": slot.get("phase"), "request_id": slot.get("request_id")} for slot in active],
                    })
            except Exception as exc:
                failures.append(f"slot polling failed: {exc}")
                break
            time.sleep(POLL_SECONDS)
        generation_thread.join(max(0.0, deadline - time.monotonic()))
        image_thread.join(max(0.0, deadline - time.monotonic()))
        if generation_thread.is_alive() or image_thread.is_alive():
            failures.append("requests did not finish before timeout")

        media_times = [o["at"] for o in observations if any(s["phase"] == "media-prefill" for s in o["slots"])]
        overlap_times = [o["at"] for o in observations if any(s["phase"] == "media-prefill" for s in o["slots"]) and any(s["phase"] == "generation" for s in o["slots"])]

        gaps = []
        event_times = generation["content_event_times"]
        image_started = image.get("started_at")
        image_first_content = image["content_event_times"][0] if image["content_event_times"] else None
        if image_started is None or image_first_content is None:
            failures.append("image prefill window could not be measured")
        else:
            for start, end in zip(event_times, event_times[1:]):
                if start <= image_first_content and end >= image_started:
                    gaps.append({"start": start, "end": end, "seconds": end - start})
            if not gaps:
                failures.append("no text content-event gap overlapped the image prefill window")
        max_gap = max((gap["seconds"] for gap in gaps), default=None)
        if max_gap is not None and max_gap > args.max_generation_content_event_gap:
            failures.append(f"maximum overlapping text content-event gap {max_gap:.3f}s exceeds {args.max_generation_content_event_gap:.3f}s")
        for label, result in (("generation", generation), ("image", image)):
            if result.get("error"):
                failures.append(f"{label} response failed: {result['error']}")
            if not result.get("content_characters"):
                failures.append(f"{label} response was empty")

        request_ids = sorted({s["request_id"] for o in observations for s in o["slots"] if s.get("request_id")})
        phase_observations: dict[str, int] = {}
        for observation in observations:
            for slot in observation["slots"]:
                phase = str(slot["phase"])
                phase_observations[phase] = phase_observations.get(phase, 0) + 1
        summary = {
            "passed": not failures,
            "failures": failures,
            "host": args.host,
            "model": args.model,
            "run_id": run_id,
            "image_path": str(args.image),
            "configuration": {
                "generation_max_tokens": args.generation_max_tokens,
                "image_max_tokens": args.image_max_tokens,
                "max_generation_content_event_gap_seconds": args.max_generation_content_event_gap,
                "timeout_seconds": args.timeout,
                "slot_poll_interval_seconds": POLL_SECONDS,
            },
            "diagnostic_request_ids": request_ids,
            "scheduler_observations": {
                "polls": len(observations),
                "phase_counts": phase_observations,
            },
            "media_prefill": {
                "window_start_seconds": rel(image_started, origin),
                "window_end_seconds": rel(image_first_content, origin),
                "window_duration_seconds": round(image_first_content - image_started, 6) if image_started is not None and image_first_content is not None else None,
                "observations": len(media_times),
                "first_at_seconds": rel(min(media_times), origin) if media_times else None,
                "last_at_seconds": rel(max(media_times), origin) if media_times else None,
                "generation_overlap_observations": len(overlap_times),
            },
            "generation_content_event_gap_metric": {
                "name": "maximum gap between consecutive text content events overlapping image submission through first image content",
                "maximum_seconds": round(max_gap, 6) if max_gap is not None else None,
                "evidence": [{"start": rel(gap["start"], origin), "end": rel(gap["end"], origin), "seconds": round(gap["seconds"], 6)} for gap in gaps],
            },
            "responses": {},
        }
        for label, result in (("generation", generation), ("image", image)):
            summary["responses"][label] = {
                "response_id": result.get("response_id"),
                "started_at_seconds": rel(result.get("started_at"), origin),
                "first_content_at_seconds": rel(result["content_event_times"][0], origin) if result["content_event_times"] else None,
                "finished_at_seconds": rel(result.get("finished_at"), origin),
                "duration_seconds": round(result.get("finished_at", origin) - result.get("started_at", origin), 6),
                "content_events": len(result["content_event_times"]),
                "content_characters": result.get("content_characters", 0),
                "finish_reason": result.get("finish_reason"),
                "usage": result.get("usage", {}),
                "error": result.get("error"),
            }
    except Exception as exc:
        failures.append(str(exc))
        summary = {"passed": False, "failures": failures, "host": args.host, "model": args.model}
    finally:
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(json.dumps(summary, indent=2) + "\n")

    print(f"{'PASS' if summary.get('passed') else 'FAIL'}: saved summary to {args.out}")
    for failure in failures:
        print(f"  {failure}")
    return 0 if summary.get("passed") else 1


if __name__ == "__main__":
    raise SystemExit(main())
