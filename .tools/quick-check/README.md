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

The default is **three complete repetitions**, not three consecutive copies of
each phase. Override with `QUICK_CHECK_ARGS='-repetitions N'`. The alias is unloaded
at startup, the small warm-up request is measured once, and the model stays resident
for all repetitions. Each repetition executes all six workload phases and validates
its own correlated evidence. The alias is unloaded once at the end.
The first failure stops the invocation; requested/executed counts and findings
identify remaining unrun repetitions. All requested repetitions must pass for an
overall PASS. All repetitions share one output directory, so retention still
keeps two invocations.

`KRONK_TOKEN` supplies authentication for inference and administrative endpoints.
The server is managed separately; the probe never downloads models or starts it.
Use a dedicated test server: the probe unloads **only its QUICK-CHECK alias** before
and after the invocation, but its workload can still contend with other loaded models.
The local JSON server log is required, including when HTTP connects remotely.
Restart after changing `.tools/model_config_tools.yaml`.

### Testing another commit or branch

The tested version is the running server—not the checkout from which you launch
quick-check. Switching commits alone is insufficient; you must rebuild and restart
the server.

If the target revision already contains our quick-check tool and configuration:

```sh
# Stop the current server before switching.
make kronk-server-stop

# First preserve any uncommitted work, then choose ONE:
git switch <branch>
# OR:
git switch --detach <commit>

make server-for-tools
make test-quick-check
```

Check the resulting `Tested commit:` line. It should show the selected commit;
`+develop` means the server was built with uncommitted changes. The timestamp is
the commit's timestamp, not the test time.

Copy any results you want to keep long-term outside the managed run directories
before testing several revisions, for example into `output/saved_local_runs/`;
automatic retention preserves only the current run and its selected baseline.

## Compare two saved runs

### Quick start

Run from the repository root after saving two quick-check runs:

```sh
make compare-quick-check
```

No running server is needed. With exactly two finalized runs directly under
`output/`, the older run is the baseline and the newer run is the candidate.
The terminal starts with:

```text
Comparison status: PASS
Saved-run checks: baseline PASS / candidate PASS
```

Then inspect the warnings and warmed-workload comparison: a positive time change
means the candidate was slower. PASS means the saved checks and evidence passed,
not that timing changes are acceptable. The full report is written automatically
to `.tools/quick-check/output/comparison.json`, outside both run folders, and
replaced on the next comparison.

To compare archived runs or select a different pair, supply **both**
`QUICK_CHECK_BASELINE` and `QUICK_CHECK_CANDIDATE` as shown below. If either run
failed, the comparator still reports its status/problems and saves the report,
but omits timing comparisons and exits nonzero. Missing or inconsistent input
artifacts stop comparison before a new report is written; an existing JSON file
then still belongs to the previous comparison.

### Options and interpretation

The offline comparison subcommand reads `summary.json`, `events.ndjson`, and
`tool.log` from two saved run directories. By default it selects the two finalized
timestamped runs directly under `.tools/quick-check/output/`, with the older run
as baseline and the newer as candidate. Ordering uses the directory timestamps,
not file modification times or commit dates. It ignores `saved_local_runs/`,
unrelated entries, symlinks, and incomplete directories. Fewer/more than two runs
or tied timestamps require explicit paths rather than guessing. It does not start
inference, contact the server, rewrite input artifacts, or apply retention. It saves
the full report as `.tools/quick-check/output/comparison.json`, outside both run
directories, replacing the previous comparison. `-out DIR` changes this output
directory as well as the parent used for automatic run selection.

```sh
make compare-quick-check

# Optional explicit pair (supply both paths or neither):
make compare-quick-check \
  QUICK_CHECK_BASELINE=.tools/quick-check/output/<baseline-run> \
  QUICK_CHECK_CANDIDATE=.tools/quick-check/output/<candidate-run>

# Full machine-readable output for CI (stdout only; errors go to stderr):
make compare-quick-check QUICK_CHECK_COMPARE_ARGS=-json > comparison.json

# Custom automatic-selection parent:
make compare-quick-check QUICK_CHECK_COMPARE_ARGS='-out /path/to/output'

# Direct invocation, optionally with explicit paths:
go run ./.tools/quick-check compare
go run ./.tools/quick-check compare -baseline /path/to/baseline -candidate /path/to/candidate -json
```

The report separates **saved-run checks**, **comparability warnings**, and
**observed metric changes**. It checks verdict/provenance consistency between the
artifacts, requested/completed repetitions, the six ordered phases, request
evidence, and required timings. It does not independently rerun inference
assertions. Failed/partial runs do not receive passing timing aggregates.

`Saved-run checks: baseline PASS / candidate FAIL` exposes each run's recorded
status separately. JSON stores them in `saved_run_checks.baseline` and
`saved_run_checks.candidate`; `comparison_status` separately records whether the
comparison found two complete, consistent passing runs. The terminal's first line,
`Comparison status: PASS/FAIL`, displays that overall result.

Comparability checks include model/workload/cell, machine metadata, library,
harness revision, model metadata/configuration, logged native parameters, request
bodies/order, per-repetition system tokens/hash, and per-request token/cache/draft
counts. Differences or missing metadata are warnings, not proof of a regression.
Different generated assistant histories may legitimately change later inputs.
Matching metadata is not proof of matching loaded bytes, actual placement,
machine load, or OS release. Unset configuration fields are compared as reported;
a newly exposed field can warn even when effective native parameters match.

The terminal shows run paths/commits, actionable warnings, a compact cell label,
and warmed workload median, sample SD, sample count, absolute delta, and percentage
change. Harness revision differences remain in JSON only. Configuration-only
warnings stay in JSON when effective native parameters and request bodies match;
otherwise they remain visible. The saved JSON includes all latency/rate metrics,
server stages, token counts, min/max, matching checks, and limitations.
Positive delta means the candidate's numeric value increased (slower
for durations, faster for output rate). A zero baseline has a null percentage;
missing metrics have null values/deltas, never fabricated zeros. Warm-up remains a
single sample, separate from workload repetitions. Older runs' startup intervals
are reconstructed from their own recorded warm-up trace when available, without
rewriting the files.

Exit 0 means both recorded checks passed completely and comparison succeeded,
**not** that performance is equivalent. Warnings and timing changes alone do not
fail CI. Exit 1 means failed/incomplete checks, missing/invalid/inconsistent
artifacts, invalid arguments, or an output error. Valid failed-run comparisons
still emit their report before exiting nonzero. No calibrated performance gate is
implemented. Preserve both inputs as CI artifacts;
AMP is optional investigation help, not required to compute or judge this report.

## Workload and assertions

The versioned synthetic operations handbook has a fixed 72 records (10,133
untemplated tokens before repetition markers were added). Workload `operations-v2`
prepends `Quick-check repetition N.` to the same handbook. The marker is fixed for
each repetition index across invocations and versions, not random. Different system
prefixes prevent cross-repetition working/System snapshot hits without reloading
weights; logs must still attest a zero-reuse cold build in every repetition.
One tokenization request per repetition records the resulting size without resizing
it or enforcing a token-size range. This is not an OpenCode prompt; actual request
usage also includes the template and messages. Prompt tokenization and the tiny
loading request are excluded from workload elapsed time.
The initial uncached 10K prefill is included. Temperature is zero, seed 42,
thinking disabled, and reviews request about 80 words with a 384-token maximum.
There is no exact word-count check; completed replies must still pass the existing
usage, MTP, and lightweight fact checks. No 30-second
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
(`-out` overrides the parent). After artifacts are successfully written and
closed, retain the new candidate and the newest prior committed run. If no prior
committed run exists, retain the newest prior candidate instead. Older recognized
run directories are deleted, including historical outputs from before this policy.
This keeps at most two finalized runs; a running or failed-to-write invocation can
temporarily add another directory. Failed checks count as candidates too.

The second line of `tool.log`, directly below `Quick-check: PASS/FAIL`, identifies
the **running server binary** using a seven-character commit hash, with `+develop`
when built with uncommitted changes, or `unknown` when provenance is unavailable:
`Tested commit: <short-hash>[+develop] from <commit-timestamp>`.
The timestamp is the commit time in UTC, not build/run time; unavailable timestamps
display as `unknown`. `summary.json` records `server_version`, `server_revision`,
`server_commit_time`, and `server_modified`. These come from Go VCS build metadata
and retain the full hash; shortening is only for the displayed header. Metadata is
exposed by `/v1/liveness`, not the tool checkout's `harness_revision` or the server's
generic `build: develop` label. `make server-for-tools` enables VCS stamping; restart
the server after rebuilding to expose these fields. Older servers and binaries
built without VCS metadata are unknown, never committed baselines.

Development/unknown runs cannot evict the newest committed result. When a new
committed candidate completes, the previous committed result remains alongside it;
the next invocation can use that new committed result as its baseline. This is a
storage policy, not automatic performance comparison or a requirement that the
baseline passed.

Run artifacts under `output/` are trackable in Git so selected results can travel
with commits. `output/saved_local_runs/` is an ignored local archive: copy runs there
to keep them outside automatic retention without committing them. Retention does
not inspect or prune that directory. The transient `output/.quick-check.lock` is
also ignored. Tracking outputs does not disable retention; it can delete tracked
run directories, leaving Git deletions to review. Commit selected results to keep
them in history. New or deleted output artifacts can make the checkout dirty and
cause the next server build to report `+develop`; commit them or move local-only
artifacts into the ignored archive before building a clean baseline.

Unrecognized, incomplete, and symlinked directories are left untouched. Invocations
sharing an output parent are serialized with `.quick-check.lock`; after a crash,
confirm no run is active before removing a stale lock. Artifact write/close failures
preserve all previous results rather than triggering retention.

Each retained run contains:

- `summary.json`: overall and per-phase status/findings, matrix cell, model,
  workload identity and prompt hash, available build identifiers, elapsed and
  first-content timings, output rate, token counts, cache path/reuse, and MTP
  activity. A phase is `UNVERIFIED` if evidence validation did not run; a failed
  assertion is `FAIL`. Run-level failures remain in the top-level findings.
  Cancelled streams omit final usage/output rate because no final usage arrives.
  Top-level fields are written in reading order: machine/server/library metadata,
  run identity, requested/executed repetition counts, the one-time `warmup_request`,
  aggregate `measurements`, then ordered `repetitions`. Each repetition holds its
  status/findings, measured system-token count and prompt hash, workload/cancellation
  times, and `results`.
  Each results array follows execution order: cold, replay, append, fresh-session,
  cancel, recovery. Request field names are unchanged; consumers of the previous
  single-run schema must now read them inside `repetitions[]`.
  Complete passing invocations report sample count, median, minimum, maximum,
  and sample standard deviation (`sample_stddev`, using the n−1 denominator)
  for each phase's elapsed/first-content time and completed workload replies'
  server output rate, plus workload total and cancellation-release wait. Startup
  has one sample, kept separate and never included in these aggregates. Raw server
  stage durations and token counts remain per repetition. Failed/partial invocations
  omit aggregates, but preserve their individual measurements and findings.
  With `-repetitions 1`, sample standard deviation is undefined: JSON records
  `null` and the header displays `SD n/a`, not a misleading zero.
- `events.ndjson`: full requests, client stream events, correlated server logs,
  and a final `source: "report"` record holding full model/configuration metadata,
  assembled answers, trace IDs, diagnostic timestamps, execution evidence, and
  all counters per repetition. Request/client/server records include a `repetition`
  index. To extract the detailed report: `jq 'select(.source == "report")
  | .report' events.ndjson`. Detailed evidence is still collected and evaluated;
  compacting the summary does not weaken any assertion.
- `tool.log`: live phase progress, with a brief verdict block prepended when the
  run finishes: four lines for overall result, tested commit, probe verdicts, and warm-up-request/
  workload median times, workload sample SD, and executed/requested repetition count. Failures add
  deeper-tool commands: IMC/MTP → `test-load-hybrid`
  and `test-load-mtp-embedded`; isolation/recovery → `test-load-hybrid`.
  Inspect `summary.json` first to distinguish setup/evidence failures from
  inference findings. These are triage suggestions, not automatic executions or
  root-cause diagnoses. Missing/unvalidated phases are `UNVERIFIED`, never PASS;
  unavailable timings are `n/a`. Detailed metrics remain in `summary.json`.

The terminal stays silent during the workload and prints only the finalized
`tool.log` header, including its failure-only investigation directions. Phase
progress remains in `tool.log`, not the terminal. Setup/storage errors that cannot
be fully represented by a finalized report still print their explanation to stderr.
Failures continue to return a nonzero exit status.

`machine` contains only `host`, `os`, `system_ram_bytes`, `gpu_total_bytes`, and
`unified_memory`. GPU memory is the aggregate capacity reported by the backend;
on unified-memory hardware it is not separate dedicated VRAM and must not be added
to RAM. Data comes from server liveness and the existing `/v1/devices` endpoint,
not the machine running quick-check. Full device inventory and volatile free-memory
values remain in diagnostic events. Collection is outside workload timing and does
not run `/v1/diagnose` or a benchmark.

This is device inventory, not proof of the model's actual placement, driver/OS
release equality, idle machine state, or comparable client networking. Missing
fields from older servers remain null; a reported zero memory size can mean the
platform does not support that query. Rebuild/restart to expose the new liveness
fields. Hardware metadata does not normalize timing or automatically gate comparisons.

`warmup_request` records the model-unloaded `Reply OK.` request separately from
the six warmed `results`, replacing the ambiguous `load_seconds` field. The
displayed label is **warm-up request**: it warms the model for the workload,
not a request that starts with an already-warm model. Its
elapsed time runs from starting that client chat call to its completed response,
not time before issuing the request. Preceding unload/setup is excluded.
Its total and first-content times include acquisition/loading and inference; neither
is pure model-load time. Unloading does not guarantee a cold server process or
cold filesystem/GPU caches. Unloading between repetitions would not make first
requests equally cold, so the probe avoids that overhead and keeps only one
model-unloaded warm-up-request measurement. This warning and the initial
three-repetition choice **must be reviewed** once results from different commits
and other matrix cells are available; see
[the review requirement](DEVELOPMENT.md#repetitions-and-cache-state--2026-10-21).
Historical output directories are not rewritten.

Startup diagnostics: `warmup_request.startup_timings` records
`server_request_seconds` (server request start to completion), `pre_chat_seconds`
(server request start to the post-acquisition `chat-completions` log),
`native_model_load_seconds`, `projection_load_seconds`, and `mtp_load_seconds`
(each loader's started/loading to completed/loaded log interval). These are
same-trace, server-timestamp differences, not client/server clock subtraction.
Missing, malformed, or reversed boundaries omit the affected metric; they do not
change correctness verdicts. Intervals overlap: loader intervals are inside
pre-Chat time, which is inside server request time. Keep the existing slot timings
and client warm-up total separate; do not sum all these metrics. The terminal
summary and workload are unchanged; no extra requests or loads are added.

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
