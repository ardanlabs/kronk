# Kronk quick-check — first cell

Standalone server-path probe, with no code dependency on the reliability tools.
The only implemented cell is **Qwen3.6 MoE / one slot / IMC on / MTP on /
no images**. The Dense alias is reserved in the shared configuration, not run.

See [DEVELOPMENT.md](DEVELOPMENT.md) for the living decision log, source-backed
timing definitions, measured findings, considered alternatives, and roadmap.

The selected model is `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL/QUICK-CHECK`.
The deeper reliability tools retain their separate Q8 fixtures. Quantization
changes are fixture changes: do not treat Q2 and Q8 timings as version-only
performance comparisons.

## Run

Install the model and compatible native libraries beforehand. Start the current
source server using the shared tools configuration, then run the probe:

```sh
make server-for-tools
make test-quick-check
# Optional connection/log settings:
make test-quick-check QUICK_CHECK_ARGS='-host http://localhost:11435 -server-log /path/to/kronk.log'
```

`KRONK_TOKEN` supplies authentication for inference and administrative endpoints.
The server is managed separately; the probe never downloads models or starts it.
Use a dedicated test server: the probe unloads **only its QUICK-CHECK alias** before
and after the run, but its workload can still contend with other loaded models.
The local JSON server log is required, including when HTTP connects remotely.
Restart after changing `.tools/model_config_tools.yaml`.

## Workload and assertions

The versioned synthetic operations handbook is calibrated through the selected
model's tokenizer to 10,000–10,500 **untemplated system-text tokens**. This is not
an OpenCode prompt; actual request usage also includes the template and messages.
Calibration and a tiny loading request are excluded from workload elapsed time.
The initial uncached 10K prefill is included. Temperature is zero, seed 42,
thinking disabled, and each measured reply has a 384-token maximum. No 30-second
performance limit is imposed; `-timeout` defaults to five minutes per request.

Six serial phases exercise:

1. Cold conversation: uncached cache build and generation.
2. Identical request replay: exact working-cache reuse; no output-equality rule.
3. Continuation using the actual first assistant reply: working-cache append,
   updated destination with unchanged owner and identifier.
4. Independent conversation under the same system prompt: System-prefix reuse
   and no facts leaking from the first conversation.
5. Cancellation after receiving content, followed by one-slot idle confirmation.
6. Fresh request from the first conversation: usable same-capacity recovery and
   no foreign or interrupted-session facts.

Completed measured replies must finish normally, have more than eight output
tokens, positive drafting/coverage, and no draft-disable reason. Content checks
are case-insensitive fact inclusion/exclusion, not rigid formatting or general
answer grading. A content finding is not proof of an infrastructure defect.
Cache build/exact/append/System reuse must be attested by trace-correlated
`imc/plan-ready` events, actual executed reuse counters, and request usage. The
probe checks the full expected reuse extent, stable-prefix/extension/tail token
accounting, paired target/MTP snapshots and restores, and resumed drafting from
restored draft KV. Exact replay restores its read-only snapshot without requiring
a new publication. Model metadata must identify the selected MoE architecture;
the configuration, scheduler, and load trace must attest IMC, one slot, and MTP.

Every phase requires terminal execution and target/draft cleanup on the configured
slot. Cancellation must be acknowledged by the engine, and recovery must execute
after it on that same slot. Missing evidence cannot PASS. Correlated server ERROR
events produce findings except for the middleware's structured `canceled` error
on the deliberately cancelled request, with engine cancellation acknowledged.
That expected disconnect remains recorded and counted; other errors still fail.
WARN events are retained for investigation without automatically failing the run.

## Results and limitations

Each invocation creates a unique directory beneath `.tools/quick-check/output`
(`-out` overrides the parent), preserving previous runs:

- `summary.json`: overall and per-phase status/findings, matrix cell, model,
  workload identity and prompt hash, available build identifiers, elapsed and
  first-content timings, output rate, token counts, cache path/reuse, and MTP
  activity. A phase is `UNVERIFIED` if evidence validation did not run; a failed
  assertion is `FAIL`. Run-level failures remain in the top-level findings.
  Cancelled streams omit final usage/output rate because no final usage arrives.
- `events.ndjson`: full requests, client stream events, correlated server logs,
  and a final `source: "report"` record holding full model/configuration metadata,
  assembled answers, trace IDs, diagnostic timestamps, execution evidence, and
  all counters. To extract the detailed report: `jq 'select(.source == "report")
  | .report' events.ndjson`. Detailed evidence is still collected and evaluated;
  compacting the summary does not weaken any assertion.
- `tool.log`: live phase progress, with a brief verdict block prepended when the
  run finishes: three lines for overall result, probe verdicts, and first-request/
  workload time. Failures add deeper-tool commands: IMC/MTP → `test-load-hybrid`
  and `test-load-mtp-embedded`; isolation/recovery → `test-load-hybrid`.
  Inspect `summary.json` first to distinguish setup/evidence failures from
  inference findings. These are triage suggestions, not automatic executions or
  root-cause diagnoses. Missing/unvalidated phases are `UNVERIFIED`, never PASS;
  unavailable timings are `n/a`. Detailed metrics remain in `summary.json`.

`first_request` records the model-unloaded `Reply OK.` request separately from
the six warmed `results`, replacing the ambiguous `load_seconds` field. Its
total and first-content times include acquisition/loading and inference; neither
is pure model-load time. Unloading does not guarantee a cold server process or
cold filesystem/GPU caches. Historical output directories are not rewritten.

Every request exposes numeric seconds in `server_timings`: `admission_seconds`,
`prepare_seconds`, `queue_wait_seconds`, `slot_prefill_ttft_seconds`,
`generation_seconds`, and `slot_execution_seconds`. These come from explicit
trace-correlated server duration fields, not client/server timing subtraction.
Missing/invalid/negative required durations fail the run; measured zero is valid.
Slot execution includes prefill, generation, and slot cleanup: do not sum these
overlapping intervals. Client first-content/elapsed/generation timings remain
separate; completed replies also expose the server's output-token rate alongside
the client's rate. The [timing contract](DEVELOPMENT.md#timing-contract--2026-10-20)
defines each boundary and its limitations.

`cancel_release_seconds` in the summary measures the wait from client cancellation
returning until the scheduler is observed idle; it includes polling/HTTP overhead
and is not an exact engine cleanup duration. `installed_library` is the library
endpoint's current version, not proof of the bytes loaded by the running server.
These artifacts contain only synthetic workload content, but may contain server
diagnostic metadata: review before sharing.
Exit 0 means the implemented assertions passed; exit 1 means findings or an
execution/evidence failure. All phases are required; an early failure is partial,
not a reduced passing scope.

This is a **workload prototype**, not proof that a Kronk version has no bugs.
One slot does not test concurrent scheduling, queue pressure, or cross-slot
isolation. It does not prove speculative rollback, images, attention-only state,
or other matrix cells. Performance is recorded, not automatically compared or
gated: compare identical workload hashes/settings and actual server/native builds
on the same idle hardware. The harness Git revision is not the server revision;
liveness/library metadata cannot independently prove the loaded binary's exact
source or library bytes. Qualification on the actual model is required before
using these results as a merge gate.

Failure triage: cache/state/recovery → `test-load-hybrid`; drafting →
`test-load-mtp-embedded`; future many-slot isolation/overlap → `test-load-batch-text`.
These commands are diagnostic suggestions, never automatically launched.

Focused harness checks (no models or running server required):

```sh
RUN_IN_PARALLEL=yes GITHUB_WORKSPACE="$PWD" go test ./.tools/quick-check
```

These HTTP/SSE fixtures test the quick-check tool, not Kronk inference.
