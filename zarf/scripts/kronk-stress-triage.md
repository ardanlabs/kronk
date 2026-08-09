Triage the stress run into a verified bug report.

Input:
- `zarf/tmp/kronk-stress/findings.txt` — probe results, verdict block, flagged calls
- `zarf/tmp/kronk-stress/kronk-server.log` — server + llama.cpp log
- `zarf/scripts/kronk-stress.sh` — probe source

Code, in ownership order:
1. Kronk — `cmd/server/app/domain/*app/`, `cmd/server/foundation/web/`,
   `sdk/kronk/model/` (batch, sampling, grammar, IMC, MTP), `sdk/kronk/parsers/<family>/`
2. yzma — `.extras/yzma/pkg/`, `.extras/yzma/lib/`
3. llama.cpp — `.extras/llama.cpp/src/`, `.extras/llama.cpp/common/`, `.extras/llama.cpp/ggml/`

Steps:
1. Candidates = every flagged probe + every NO SIGNAL. Dedupe by root cause.
2. Verify each independently. Subagents optional — one per candidate when there are
   many. Each verification:
   - Read the flagged call's request/response in `findings.txt` and its probe in
     `kronk-stress.sh`; confirm the probe asserts a real invariant.
   - Correlate by timestamp with `kronk-server.log`.
   - Trace to owning code, cite `file:line`, name the layer. Follow Kronk → yzma →
     llama.cpp when the cause is not in Kronk.
   - Verdict: CONFIRMED (code path proves it) | EXPECTED (server right, probe or spec
     reading wrong — say which) | UNPROVEN (not pinnable to code).
   - Uncertain → EXPECTED or UNPROVEN. No defect without a mechanism shown in source.
3. Report CONFIRMED only.

Output — bullets, no prose, no preamble:

- **<symptom>** — `<probe-id>` · `path/file.go:LINE` (kronk|yzma|llama.cpp)
  - Expected: <invariant> · Actual: <observed>
  - Cause: <one sentence, cites code path>
  - Repro: <single command from findings.txt>

Then one line per non-confirmed flag: `Dismissed: <probe-id> — <reason>`.
