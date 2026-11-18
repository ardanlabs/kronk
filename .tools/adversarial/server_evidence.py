"""Correlate JSON/logfmt telemetry with adversarial captures, without inference."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import sys


HEADER = re.compile(r"^KRONK: (\S+): (.*?): (INFO|ERROR|WARN|DEBUG): ([0-9a-f]{32}): (.*)$")


def formatted_event(header, body):
    timestamp, source, level, trace, _ = header.groups()
    msg, _, fields = body.partition(": ")
    event = dict(time=timestamp, file=source, level=level, trace_id=trace, msg=msg)
    # Values can contain nested brackets and multiple lines. Never mine fields
    # out of prompt/output text, or reinterpret continuation lines as JSON logs.
    while fields:
        key = re.match(r"([\w-]+)\[", fields)
        if not key:
            break
        start = key.end()
        end, depth = start, 1
        while end < len(fields) and depth:
            depth += (fields[end] == "[") - (fields[end] == "]")
            end += 1
        if depth:
            break  # logfmt is not lossless for arbitrary unbalanced prompt text.
        value = fields[start:end - 1]
        try:
            value = json.loads(value)
        except ValueError:
            pass
        event[key[1]] = value
        fields = fields[end:]
        if not fields.startswith(": "):
            break
        fields = fields[2:]
    return event


def read_events(source, offset):
    if not source.is_file():
        return
    with source.open("rb") as stream:
        stream.seek(offset)
        pending, body, first = None, [], 0
        for number, raw in enumerate(stream, 1):
            line = raw.decode("utf-8", errors="replace").rstrip("\r\n")
            header = HEADER.match(line)
            if header:
                if pending:
                    yield formatted_event(pending, "\n".join(body)), first, "logfmt"
                pending, body, first = header, [header[5]], number
                continue
            if pending:
                body.append(line)
                continue
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if isinstance(event, dict):
                yield event, number, "json"
        if pending:
            yield formatted_event(pending, "\n".join(body)), first, "logfmt"


def record(label, reason, evidence=None):
    return dict(label=label, reason=reason, evidence=evidence or label)


def analyze(work, source, offset, groups, out):
    traces = {path.stem: path.read_text().splitlines() for path in work.glob("*.trace-id")}
    known = {trace for values in traces.values() for trace in values}
    by_trace, formats = {}, set()
    with (out / "server-events.jsonl").open("w") as target:
        for event, line, fmt in read_events(source, offset):
            trace = event.get("trace_id")
            if not isinstance(trace, str) or trace not in known:
                continue
            event.update(log_line=line, log_offset=offset)
            formats.add(fmt)
            by_trace.setdefault(trace, []).append(event)
            target.write(json.dumps(event, separators=(",", ":")) + "\n")

    def events_for(label):
        return [event for trace in traces.get(label, []) for event in by_trace.get(trace, [])]

    def finals(events):
        return [event for event in events if event.get("status") == "final"
                and all(key in event for key in ("buffered_tool_bytes", "tool_calls", "finish_reason"))]

    def rejected(events):
        return any(event.get("msg") == "tool-call" and event.get("status") == "parse-error-recovered"
                   and event.get("remaining") == 0 for event in events)

    def resolution(label, reason, events):
        row = record(label, reason, "server-events.jsonl")
        row["records"] = [dict(trace_id=event["trace_id"], log_line=event["log_line"],
                               log_offset=offset) for event in events]
        return row

    findings, warnings, gaps, resolved = [], [], [], []
    all_events = [event for events in by_trace.values() for event in events]
    for trace, events in by_trace.items():
        for final in finals(events):
            if (final["buffered_tool_bytes"] > 0 and final["tool_calls"] == 0
                    and final["finish_reason"] != "length" and not rejected(events)):
                findings.append(record("server-log-truncation",
                    f"trace {trace}: discarded {final['buffered_tool_bytes']} buffered tool bytes "
                    f"without a token cap or observed atomic parser rejection (log line {final['log_line']})",
                    "server-events.jsonl"))
    # Recovery middleware puts its stack in err, not msg. Do not scan arbitrary
    # prompt/output fields for panic words or forged telemetry.
    panics = [event for event in all_events if re.search(
        r"panic[:\s\[]|goroutine \d+ \[running\]",
        " ".join(str(event.get(key, "")) for key in ("msg", "err", "error")), re.IGNORECASE)]
    if panics:
        findings.append(record("server-log-panic", f"{len(panics)} correlated panic marker(s)", "server-events.jsonl"))
    errors = sum(event.get("level") == "ERROR" for event in all_events)
    if errors:
        warnings.append(record("server-log-errors", f"{errors} correlated ERROR records; compare with expected probe rejections",
                               "server-events.jsonl"))

    if "truncation" in groups:
        for cap in (128, 256, 512, 1024, 2048):
            label = f"tr-log{cap}"
            events = finals(events_for(f"tr-maxtok{cap}"))
            if events:
                resolved.append(resolution(label, "correlated final buffered-tool telemetry is available", events))
            else:
                gaps.append(record(label, "no correlated final buffered-tool evidence for this capped call"))

    events = events_for("aa-closing-tag")
    response = work / "aa-closing-tag.resp.json"
    status = work / "aa-closing-tag.status"
    if response.is_file() and status.is_file() and re.search(r"^(?:http_status=)?200(?:\s|$)", status.read_text()):
        try:
            payload = json.loads(response.read_text())
        except ValueError:
            payload = None
        choices = payload.get("choices", []) if isinstance(payload, dict) else []
        no_calls = bool(choices) and all(not choice.get("message", {}).get("tool_calls") for choice in choices)
        if no_calls and rejected(events) and any(final["tool_calls"] == 0 and final["buffered_tool_bytes"] > 0
                                                for final in finals(events)):
            resolved.append(resolution("aa-closing-tag", "parser rejected the buffered tool call; no executable call returned",
                                       [event for event in events if event.get("status") in ("parse-error-recovered", "final")]))

    for label in traces:
        if not label.startswith("cx-abort"):
            continue
        status, sse = work / (label + ".status"), work / (label + ".sse")
        if not status.is_file() or not sse.is_file():
            continue
        text = status.read_text()
        if not re.search(r"\bhttp_status=200\b", text) or not re.search(r"\bcurl_exit=28\b", text):
            continue
        if "data: [DONE]" in sse.read_text():
            continue
        events = events_for(label)
        generated = any(event.get("msg") == "batch-engine" and event.get("status") == "slot-finished"
                        and event.get("output_tokens", 0) > 0 for event in events)
        canceled = any(event.get("msg") == "request-lifecycle" and event.get("stage_name") == "execute-in-slot"
                       and event.get("status") == "cancel" for event in events)
        if generated and canceled:
            resolved.append(resolution("cx-midstream", f"{label}: client timed out during server-observed generation; execution canceled",
                                       [event for event in events if event.get("status") in ("slot-finished", "cancel")]))

    assessment = dict(formats=sorted(formats), correlated_records=len(all_events),
                      correlated_traces=len(by_trace), log_offset=offset,
                      findings=findings, warnings=warnings, no_signal=gaps, resolved=resolved,
                      analyzer_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest())
    (out / "server-evidence.json").write_text(json.dumps(assessment, indent=2) + "\n")
    return assessment


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--run", type=Path, help="reprocess a saved run without HTTP access; requires a new --out directory")
    mode.add_argument("--work", type=Path, help="internal live-harness capture directory")
    parser.add_argument("--server-log", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--offset", type=int, default=0, help="live log byte offset; line numbers are relative to this offset")
    parser.add_argument("--groups", default="", help="space-separated live-harness groups")
    args = parser.parse_args()
    os.umask(0o077)
    try:
        summary = None
        if args.run:
            if args.offset:
                raise ValueError("saved-run reprocessing scans the complete supplied log")
            summary_path = args.run / "summary.json"
            summary_bytes = summary_path.read_bytes()
            summary = json.loads(summary_bytes)
            work = args.run / (summary.get("captures") or ".work")
            if not work.is_dir() or not args.server_log.is_file():
                raise ValueError("saved captures and a server log are required")
            # mkdir refuses even an empty existing directory: never replace a run.
            args.out.mkdir(parents=True, exist_ok=False)
            groups = summary["groups"]
        else:
            work, groups = args.work, args.groups.split()
        evidence = analyze(work, args.server_log, args.offset, groups, args.out)
        if summary is None:
            return 0
        resolved = {row["label"] for row in evidence["resolved"]}
        # Client assertions were not rerun. Keep prior findings, even when the
        # supplied log does not corroborate them; absence of evidence is no fix.
        summary["findings"] += [row for row in evidence["findings"] if row not in summary["findings"]]
        summary["warnings"] = [row for row in summary["warnings"] if row["label"] != "server-log-errors"] + evidence["warnings"]
        gaps = {row["label"]: row for row in summary["no_signal"] if row["label"] not in resolved}
        gaps.update((row["label"], row) for row in evidence["no_signal"])
        summary["no_signal"] = list(gaps.values())
        summary["result"], summary["exit_code"] = ("FAIL", 1) if summary["findings"] else (
            ("INCOMPLETE", 3) if gaps else ("PASS", 0))
        summary.update(captures=str(work.resolve()), server_events="server-events.jsonl", server_evidence="server-evidence.json",
                       reprocessed_from=dict(run=str(args.run.resolve()),
                           summary_sha256=hashlib.sha256(summary_bytes).hexdigest(),
                           server_log=str(args.server_log.resolve()),
                           server_log_sha256=hashlib.sha256(args.server_log.read_bytes()).hexdigest()))
        (args.out / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
        lines = ["Offline server-evidence reprocessing; no requests rerun.", f"Original run: {args.run.resolve()}",
                 f"RESULT: {summary['result']} ({len(summary['findings'])} findings, {len(gaps)} no-signal)", ""]
        for title, rows in (("RESOLVED", evidence["resolved"]), ("FINDING", summary["findings"]),
                            ("NO SIGNAL", summary["no_signal"]), ("WARNING", summary["warnings"])):
            lines.extend(f"{title} {row['label']}: {row['reason']}" for row in rows)
        report = "\n".join(lines) + "\n"
        (args.out / "findings.txt").write_text(report)
        print(report, end="")
        return summary["exit_code"]
    except (OSError, ValueError, KeyError) as error:
        print(f"server evidence: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
