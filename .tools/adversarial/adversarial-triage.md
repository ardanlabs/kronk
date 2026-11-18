Triage the adversarial run into a verified bug report.

Input:

- The run's `OUT` directory (default `.tools/adversarial/output`): `summary.json`, `findings.txt`, `.work/`, `server-events.jsonl`, and `server-evidence.json`
- `SERVERLOG`, when available — original server + llama.cpp log, not necessarily inside `OUT`
- `.tools/adversarial/adversarial.sh` — probe source

Source code, in ownership order:

1. Kronk — `cmd/server/app/domain/*app/`, `cmd/server/foundation/web/`,
   `sdk/kronk/model/` (batch, sampling, grammar, IMC, MTP), `sdk/kronk/parsers/<family>/`
2. yzma — `.extras/yzma/pkg/`, `.extras/yzma/lib/`
3. llama.cpp — `.extras/llama.cpp/src/`, `.extras/llama.cpp/common/`, `.extras/llama.cpp/ggml/`

Steps:

1. Read `summary.json`. Separate findings from NO SIGNAL coverage gaps and diagnostic warnings. A model name does not prove MTP execution; independent requests need not produce byte-identical output. Dedupe findings by root cause. Do not classify missing coverage as a server defect.
2. Verify each independently. Subagents optional — one per candidate when there are
   many. Each verification:
   - Read the exact request, endpoint, response/SSE, status, and trace ID under the summary's `captures` path (normally `.work/`), using the finding's evidence label. `findings.txt` contains clipped excerpts, not replay-complete bodies. Read the probe in `adversarial.sh` and confirm it asserts a real invariant.
   - Correlate by trace ID with `server-events.jsonl` and check the resolved coverage and source record locations in `server-evidence.json`. Use timestamps only as supporting evidence in the original log; older or unrelated records do not establish this run's failure. Missing correlated logs leave the server-side path unproven. A parser recovery that rejects all calls is not silent tool-output loss. Buffered generation can establish mid-generation cancellation without client-visible argument deltas, but cancellation alone does not prove slot release or cache correctness.
   - Trace to owning code, cite `file:line`, name the layer. Follow Kronk → yzma →
     llama.cpp when the cause is not in Kronk.
   - Verdict: CONFIRMED (code path proves it) | EXPECTED (server right, probe or spec
     reading wrong — say which) | UNPROVEN (not pinnable to code).
   - Uncertain → UNPROVEN, not EXPECTED. No defect without a mechanism shown in source.
3. If a server log was supplied after the run, use `python3 .tools/adversarial/server_evidence.py --run RUN_DIR --server-log LOG_FILE --out NEW_DIR` from the repository root to analyze it without inference; JSON and formatted Kronk logs are supported. Use a new destination and retain the original captures referenced by the reprocessed summary. Prior findings remain until independently verified; do not treat a missing log record as disproving a finding.
4. Report confirmed defects separately from unproven findings, coverage gaps, and diagnostics. Analysis does not authorize rerunning probes, restarting servers, or downloading models; obtain explicit permission for those actions.

Output — bullets, no prose, no preamble:

- **<symptom>** — `<probe-id>` · `path/file.go:LINE` (kronk|yzma|llama.cpp)
  - Expected: <invariant> · Actual: <observed>
  - Cause: <one sentence, cites code path>
  - Repro: <request body file, endpoint, model/configuration, and necessary setup; do not invent a successful reproduction>

Then one line per remaining item: `EXPECTED`, `UNPROVEN`, `COVERAGE GAP`, or `DIAGNOSTIC`: `<probe-id> — <reason>`. Do not silently dismiss an unproven finding.
