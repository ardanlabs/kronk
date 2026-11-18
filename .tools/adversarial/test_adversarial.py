"""Harness contract tests against a fake HTTP server; no Kronk or inference."""

import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


SCRIPT = Path(__file__).with_name("adversarial.sh")
PERSON = {"name": "Ada", "age": 37, "hobbies": ["reading"]}


class HarnessTests(unittest.TestCase):
    def run_harness(self, group, mode="valid", keep="1"):
        with tempfile.TemporaryDirectory() as directory:
            out = Path(directory)
            log = out / "server.log"
            log.write_text('{"trace_id":"old","tool_calls":0,"buffered_tool_bytes":90}\n')
            requests = []
            interrupted = []
            release = threading.Event()

            class Handler(BaseHTTPRequestHandler):
                def log_message(self, *args):
                    pass

                def do_GET(self):
                    self.reply()

                def do_POST(self):
                    self.reply()

                def reply(self):
                    body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
                    requests.append((self.path, dict(self.headers), body))
                    trace = self.headers.get("traceparent", "").split("-")[1]
                    event = dict(trace_id=trace, status="final", tool_calls=1,
                                 buffered_tool_bytes=0, finish_reason="tool_calls")
                    if mode in ("drop", "formatted-drop", "rejected", "formatted-rejected"):
                        event.update(tool_calls=0, buffered_tool_bytes=90, finish_reason="stop")
                    with log.open("a") as stream:
                        # An unrelated concurrent failure must not affect this run.
                        stream.write(json.dumps(dict(event, trace_id="unrelated", tool_calls=0,
                                                     buffered_tool_bytes=90)) + "\n")
                        if mode != "no-logs":
                            events = [event]
                            if mode.endswith("rejected"):
                                events.insert(0, dict(trace_id=trace, msg="tool-call", status="parse-error-recovered", remaining=0))
                            if mode == "cancel-buffered" and body.get("max_tokens") == 4000 and not interrupted:
                                events = [dict(trace_id=trace, msg="batch-engine", status="slot-finished", output_tokens=119),
                                          dict(trace_id=trace, msg="request-lifecycle", status="cancel", stage_name="execute-in-slot")]
                            for item in events:
                                if mode.startswith("formatted"):
                                    msg = item.get("msg", "chat-completion")
                                    fields = ": ".join(f"{key}[{value}]" for key, value in item.items() if key not in ("msg", "trace_id"))
                                    stream.write(f"KRONK: 2026-11-18T08:27:22-05:00: model.go:1: INFO: {trace}: {msg}: {fields}\n")
                                else:
                                    stream.write(json.dumps(item) + "\n")
                    status = 200
                    response = {}
                    sse = False
                    if self.path == "/v1/models":
                        response = {"object": "list", "data": [{"id": "fixture/model"}]}
                    elif self.path == "/v1/liveness" and mode == "unhealthy":
                        status = 503
                    elif self.path == "/v1/chat/completions":
                        rf = body.get("response_format", {})
                        schema = rf.get("json_schema", {})
                        grammar = body.get("grammar", "")
                        if rf.get("type") == "banana" or (rf.get("type") == "json_schema" and not schema) or "unterminated" in grammar:
                            status = 400
                        elif grammar and mode == "unsupported":
                            status = 422
                        else:
                            content = json.dumps(PERSON)
                            if schema.get("name") == "verdict":
                                content = '{"verdict":"yes"}'
                            if grammar:
                                content = "red"
                            if group == "determinism":
                                content = str(len(requests))
                            message = {"content": content}
                            if group == "truncation" or (group == "determinism" and body.get("max_tokens") == 400):
                                message["tool_calls"] = [{"function": {"name": "write_file", "arguments": "{}"}}]
                            response = {"choices": [{"message": message, "finish_reason": "stop"}],
                                        "usage": {"completion_tokens": 7, "prompt_tokens_details": {"cached_tokens": 3}}}
                            if body.get("stream"):
                                sse = True
                                finish = "stop"
                                if mode in ("wrong-type", "wrong-type-length"):
                                    content = json.dumps(dict(PERSON, age="37"))
                                elif mode == "missing-key":
                                    content = '{"name":"Ada","age":37}'
                                elif mode == "extra-key":
                                    content = json.dumps(dict(PERSON, extra=True))
                                elif mode == "fractional":
                                    content = json.dumps(dict(PERSON, age=37.5))
                                elif mode == "bad-array":
                                    content = json.dumps(dict(PERSON, hobbies=[3]))
                                elif mode == "truncated":
                                    content = '{"name":'
                                if mode in ("wrong-type-length", "truncated"):
                                    finish = "length"
                                response = "data: " + json.dumps({"choices": [{"delta": {"content": content}, "finish_reason": finish}]}) + "\n\ndata: [DONE]\n\n"
                                if mode == "stream-error":
                                    status, sse, response = 429, False, {"error": "rate limited"}
                    pause = mode in ("cancel-active", "cancel-role", "cancel-buffered") and body.get("stream") and body.get("max_tokens") == 4000 and not interrupted
                    if pause:
                        interrupted.append(True)
                        delta = {"content": "working"} if mode == "cancel-active" else {"role": "assistant"}
                        response = "data: " + json.dumps({"choices": [{"delta": delta}]}) + "\n\n"
                    payload = response.encode() if sse else json.dumps(response).encode()
                    self.send_response(status)
                    self.send_header("Content-Type", "text/event-stream" if sse else "application/json")
                    self.send_header("Content-Length", str(len(payload) + (1000 if pause else 0)))
                    self.end_headers()
                    self.wfile.write(payload)
                    if pause:
                        self.wfile.flush()
                        release.wait(10)

            server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
            thread = threading.Thread(target=server.serve_forever)
            thread.start()
            env = dict(os.environ, HOST="http://127.0.0.1:" + str(server.server_port),
                       MODEL="fixture/model", OUT=directory, SERVER="0", SERVERLOG=str(log),
                       KRONK_TOKEN="fake-test-token", KEEP_ALL=keep, TIMEOUT="5")
            try:
                result = subprocess.run(["/bin/bash", str(SCRIPT), group], env=env,
                                        capture_output=True, text=True, timeout=45)
            finally:
                release.set()
                server.shutdown()
                thread.join()
                server.server_close()
            self.assertTrue((out / "summary.json").exists(), result.stdout + result.stderr)
            summary = json.loads((out / "summary.json").read_text())
            self.assertEqual(result.returncode, summary["exit_code"], result.stdout)
            traces = []
            for _, headers, _ in requests:
                self.assertEqual(headers.get("Authorization"), "Bearer fake-test-token")
                self.assertRegex(headers.get("traceparent", ""), r"^00-[0-9a-f]{32}-[0-9a-f]{16}-01$")
                traces.append(headers["traceparent"].split("-")[1])
            self.assertEqual(len(traces), len(set(traces)))
            self.assertEqual(set(traces), {trace for request in summary["requests"] for trace in request["trace_ids"]})
            events = [json.loads(line) for line in (out / "server-events.jsonl").read_text().splitlines()]
            self.assertTrue(all(event["trace_id"] in traces for event in events))
            files = {str(path.relative_to(out)): path.read_text() for path in out.rglob("*")
                     if path.is_file()}
            self.assertNotIn("fake-test-token", "".join(files.values()))
            return summary, files

    def test_clean_and_failed_health(self):
        summary, files = self.run_harness("health", keep="0")
        self.assertEqual(summary["result"], "PASS")
        self.assertEqual(summary["exit_code"], 0)
        self.assertIsNone(summary["captures"])
        self.assertFalse(any(name.startswith(".work/") for name in files))
        summary, files = self.run_harness("health", "unhealthy", keep="0")
        self.assertEqual(summary["result"], "FAIL")
        self.assertEqual(summary["exit_code"], 1)
        self.assertIn(".work/health-live.resp.json", files)

    def test_streamed_schema_checks_types_keys_and_items(self):
        summary, _ = self.run_harness("structured")
        self.assertEqual(summary["result"], "PASS")
        for mode in ("wrong-type", "missing-key", "extra-key", "fractional", "bad-array", "wrong-type-length"):
            with self.subTest(mode=mode):
                summary, files = self.run_harness("structured", mode, keep="0")
                self.assertEqual([finding["label"] for finding in summary["findings"]], ["sr-stream"])
                self.assertEqual(summary["exit_code"], 1)
                self.assertIn(".work/sr-stream.req.json", files)
                self.assertIn(".work/sr-stream.sse", files)
                self.assertIn("http_status=200", files[".work/sr-stream.status"])

    def test_incomplete_is_not_clean(self):
        for mode, label in (("unsupported", "sr-grammar"), ("truncated", "sr-stream")):
            with self.subTest(mode=mode):
                summary, files = self.run_harness("structured", mode, keep="0")
                self.assertEqual(summary["result"], "INCOMPLETE")
                self.assertEqual(summary["exit_code"], 3)
                self.assertEqual(summary["findings"], [])
                self.assertEqual(summary["no_signal"][0]["label"], label)
                self.assertIn(".work/" + label + ".req.json", files)

    def test_stream_http_error_is_not_missing_content(self):
        summary, files = self.run_harness("structured", "stream-error")
        self.assertEqual(summary["result"], "FAIL")
        self.assertEqual(summary["findings"][0]["label"], "sr-stream")
        self.assertEqual(summary["no_signal"], [])
        self.assertIn("http_status=429", files[".work/sr-stream.status"])

    def test_reproducibility_differences_are_warnings(self):
        summary, _ = self.run_harness("determinism", keep="0")
        self.assertEqual(summary["result"], "PASS")
        self.assertTrue(summary["warnings"])
        self.assertEqual(summary["captures"], ".work")

    def test_cancellation_requires_interrupted_generation_not_just_bytes(self):
        for mode, expected in (("valid", "INCOMPLETE"), ("cancel-role", "INCOMPLETE"),
                               ("cancel-active", "PASS"), ("cancel-buffered", "PASS")):
            with self.subTest(mode=mode):
                summary, _ = self.run_harness("cancel", mode)
                self.assertEqual(summary["result"], expected)
                self.assertEqual(summary["findings"], [])
                if expected == "INCOMPLETE":
                    self.assertEqual(summary["no_signal"][0]["label"], "cx-midstream")

    def test_truncation_requires_correlated_final_evidence(self):
        for mode in ("valid", "formatted"):
            with self.subTest(mode=mode):
                summary, _ = self.run_harness("truncation", mode)
                self.assertEqual(summary["result"], "PASS")
        summary, _ = self.run_harness("truncation", "no-logs")
        self.assertEqual(summary["result"], "INCOMPLETE")
        self.assertEqual(len(summary["no_signal"]), 5)
        for mode in ("drop", "formatted-drop"):
            with self.subTest(mode=mode):
                summary, _ = self.run_harness("truncation", mode)
                self.assertEqual(summary["result"], "FAIL")
                self.assertEqual(summary["findings"][0]["label"], "server-log-truncation")

    def test_parser_rejection_is_not_silent_tool_loss(self):
        for mode in ("rejected", "formatted-rejected"):
            with self.subTest(mode=mode):
                summary, _ = self.run_harness("structured", mode)
                self.assertEqual(summary["result"], "PASS")
                self.assertEqual(summary["findings"], [])


class SavedEvidenceTests(unittest.TestCase):
    def reprocess(self, directory, events, formatted=False, status="http_status=200 curl_exit=28 bytes=445",
                  calls=None, findings=None):
        root = Path(directory)
        run = root / "original"
        work = run / ".work"
        work.mkdir(parents=True)
        labels = ["tr-maxtok128", "aa-closing-tag", "cx-abort1s-1"]
        traces = {label: str(index + 1) * 32 for index, label in enumerate(labels)}
        for label, trace in traces.items():
            (work / (label + ".trace-id")).write_text(trace + "\n")
        (work / "cx-abort1s-1.status").write_text(status + "\n")
        (work / "cx-abort1s-1.sse").write_text('data: {"choices":[{"delta":{"role":"assistant"}}]}\n\n')
        (work / "aa-closing-tag.status").write_text("200\n")
        (work / "aa-closing-tag.resp.json").write_text(json.dumps({"choices": [
            {"message": {"content": "Invalid tool call", "tool_calls": calls or []}}
        ]}))
        gaps = ["tr-log128", "aa-closing-tag", "cx-midstream", "tl-required", "cc-sess1"]
        summary = dict(run_id="saved-fixture", result="INCOMPLETE", exit_code=3,
                       groups=["truncation", "advargs", "cancel"], captures=".work",
                       findings=findings or [], warnings=[], no_signal=[dict(label=label, reason="missing", evidence=label)
                                                            for label in gaps])
        (run / "summary.json").write_text(json.dumps(summary))
        log = run / "kronk.log"
        rows = []
        for event in events:
            event = dict(event)
            label = event.pop("_request", "aa-closing-tag")
            event.setdefault("trace_id", traces[label])
            if formatted:
                msg = event.pop("msg", "chat-completion")
                tid = event.pop("trace_id")
                fields = ": ".join(f"{key}[{value}]" for key, value in event.items())
                rows.append(f"KRONK: 2026-11-18T08:27:22-05:00: model.go:1: INFO: {tid}: {msg}: {fields}")
            else:
                rows.append(json.dumps(event))
        log.write_text("\n".join(rows) + "\n")
        before = {path: path.read_bytes() for path in run.rglob("*") if path.is_file()}
        out = root / "reprocessed"
        command = ["python3", str(SCRIPT.with_name("server_evidence.py")), "--run", str(run),
                   "--server-log", str(log), "--out", str(out)]
        result = subprocess.run(command, capture_output=True, text=True, timeout=10)
        self.assertEqual(before, {path: path.read_bytes() for path in run.rglob("*") if path.is_file()})
        self.assertTrue((out / "summary.json").exists(), result.stdout + result.stderr)
        summary = json.loads((out / "summary.json").read_text())
        self.assertEqual(result.returncode, summary["exit_code"], result.stdout + result.stderr)
        evidence = json.loads((out / "server-evidence.json").read_text())
        # A repeated analysis must never overwrite an existing destination.
        before_out = {path: path.read_bytes() for path in out.rglob("*") if path.is_file()}
        repeated = subprocess.run(command, capture_output=True, text=True, timeout=10)
        self.assertEqual(repeated.returncode, 2)
        self.assertEqual(before_out, {path: path.read_bytes() for path in out.rglob("*") if path.is_file()})
        return summary, evidence

    def test_saved_json_and_formatted_logs_resolve_only_observed_paths(self):
        events = [dict(_request="tr-maxtok128", status="final", buffered_tool_bytes=568, tool_calls=0, finish_reason="length"),
                  dict(msg="tool-call", status="parse-error-recovered", remaining=0),
                  dict(status="final", buffered_tool_bytes=158, tool_calls=0, finish_reason="stop"),
                  dict(_request="cx-abort1s-1", msg="chat-completion", status="channel-transition", to="tool", tool_buffering="started"),
                  dict(_request="cx-abort1s-1", msg="batch-engine", status="slot-finished", output_tokens=119),
                  dict(_request="cx-abort1s-1", msg="request-lifecycle", status="cancel", stage_name="execute-in-slot")]
        for formatted in (False, True):
            with self.subTest(formatted=formatted), tempfile.TemporaryDirectory() as directory:
                summary, evidence = self.reprocess(directory, events, formatted)
                self.assertEqual(summary["findings"], [])
                self.assertEqual(summary["result"], "INCOMPLETE")
                self.assertEqual({row["label"] for row in evidence["resolved"]},
                                 {"tr-log128", "aa-closing-tag", "cx-midstream"})
                self.assertIn("tl-required", {row["label"] for row in summary["no_signal"]})
                self.assertIn("cc-sess1", {row["label"] for row in summary["no_signal"]})

    def test_silent_drop_is_still_a_finding(self):
        for formatted in (False, True):
            with self.subTest(formatted=formatted), tempfile.TemporaryDirectory() as directory:
                summary, _ = self.reprocess(directory, [dict(trace_id="f" * 32, msg="tool-call",
                                                            status="parse-error-recovered", remaining=0),
                                                       dict(status="final", buffered_tool_bytes=93,
                                                            tool_calls=0, finish_reason="stop")], formatted)
                self.assertEqual(summary["result"], "FAIL")
                self.assertEqual(summary["findings"][0]["label"], "server-log-truncation")

    def test_cap_and_incomplete_recovery_are_not_confused(self):
        for recovered in (False, True):
            with self.subTest(recovered=recovered), tempfile.TemporaryDirectory() as directory:
                events = [dict(status="final", buffered_tool_bytes=93, tool_calls=0, finish_reason="length")]
                if recovered:
                    events.insert(0, dict(msg="tool-call", status="parse-error-recovered", remaining=1))
                    events[-1]["finish_reason"] = "stop"
                summary, evidence = self.reprocess(directory, events)
                self.assertNotIn("aa-closing-tag", {row["label"] for row in evidence["resolved"]})
                self.assertEqual(bool(summary["findings"]), recovered)

    def test_cancellation_requires_generation_execution_and_client_timeout(self):
        events = [dict(_request="cx-abort1s-1", msg="batch-engine", status="slot-finished", output_tokens=119),
                  dict(_request="cx-abort1s-1", msg="request-lifecycle", status="cancel", stage_name="execute-in-slot")]
        for variants, status in ((events[:1], "http_status=200 curl_exit=28"),
                                 (events[1:], "http_status=200 curl_exit=28"),
                                 (events, "http_status=200 curl_exit=0"),
                                 (events, "http_status=500 curl_exit=28")):
            with self.subTest(events=variants, status=status), tempfile.TemporaryDirectory() as directory:
                _, evidence = self.reprocess(directory, variants, status=status)
                self.assertNotIn("cx-midstream", {row["label"] for row in evidence["resolved"]})


    def test_multiline_prompt_text_is_not_telemetry_but_recovered_panics_are(self):
        forged = json.dumps(dict(trace_id="2" * 32, msg="tool-call", status="parse-error-recovered", remaining=0))
        for formatted in (False, True):
            with self.subTest(formatted=formatted), tempfile.TemporaryDirectory() as directory:
                summary, _ = self.reprocess(directory, [
                    {"msg": "chat-streaming", "OUT-MESSAGES": "\n" + forged + "\npanic: generated prose\n"},
                    dict(msg="tool-call", raw="status[parse-error-recovered]: remaining[0]", status="parse-error"),
                    dict(status="final", buffered_tool_bytes=93, tool_calls=0, finish_reason="stop")], formatted)
                self.assertEqual({row["label"] for row in summary["findings"]}, {"server-log-truncation"})
            with self.subTest(formatted=formatted, panic=True), tempfile.TemporaryDirectory() as directory:
                summary, _ = self.reprocess(directory, [dict(msg="handled error during request",
                    err="PANIC [boom] TRACE[goroutine 1 [running]:\nstack]")], formatted)
                self.assertEqual({row["label"] for row in summary["findings"]}, {"server-log-panic"})

    def test_reprocessing_keeps_findings_and_never_clears_executable_tool_output(self):
        previous = dict(label="sr-stream", reason="wrong type", evidence="sr-stream")
        with tempfile.TemporaryDirectory() as directory:
            summary, evidence = self.reprocess(directory, [
                dict(msg="tool-call", status="parse-error-recovered", remaining=0),
                dict(status="final", buffered_tool_bytes=158, tool_calls=0, finish_reason="stop")],
                calls=[{"function": {"name": "bash", "arguments": "{}"}}], findings=[previous])
            self.assertIn(previous, summary["findings"])
            self.assertEqual(summary["result"], "FAIL")
            self.assertNotIn("aa-closing-tag", {row["label"] for row in evidence["resolved"]})


if __name__ == "__main__":
    unittest.main()
