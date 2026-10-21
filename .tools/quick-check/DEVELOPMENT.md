# Quick-check development record

Living record of findings, considered options, decisions, and remaining work.
Update this document as development proceeds; use it to prepare the final design
summary rather than reconstructing the reasoning from conversation history.
Operational instructions belong in [README.md](README.md).

## Goal and boundaries — 2026-10-20

Provide an early infrastructure correctness/performance signal for new main
versions. A finding directs investigation toward the relevant deeper reliability
tool; a PASS means no detected regression in the exercised scope, not no bugs.

The intended matrix is slots one/many × IMC off/on × MTP off/on × images
absent/present × Dense/MoE. Initially implement only **MoE / one slot / IMC on /
MTP on / no images**, and qualify the workload before expanding. Both eventual
fixtures may be Hybrid. Attention-only, multi-GPU placement, and classic drafting
are not objectives here. Embedded versus companion MTP is not a quick-check axis.
The initial 30-second reference is not an enforced budget.

## Decisions and alternatives — 2026-10-20

| Topic | Decision and reason |
| --- | --- |
| OpenCode versus direct workload | Do not use OpenCode. Its agent loop, retries, tool execution, and client changes introduce variability outside Kronk and complicate attribution. Approximate a realistic agent-sized context without benchmarking an agent. |
| Generated programs versus prose | Use fictional handover conversations, not compilation/execution of generated code. Validate lightweight fact inclusion/exclusion and infrastructure evidence; do not grade the model generally. |
| Fixed-format/deterministic answers | Do not require rigid formatting or identical replay output. Temperature zero and a seed reduce variation but do not establish determinism. Content findings need investigation, not automatic attribution to the engine. |
| Large system prefix | Use a versioned synthetic handbook with a fixed 72 records, preserving the text qualified at 10,133 untemplated tokens. Tokenize once to record actual size, not to resize it. A large prompt alone is insufficient: deliberately exercise exact replay, append, independent-session System reuse, active cancellation, and recovery. |
| Server versus direct SDK | Current prototype uses the server: it also exercises HTTP/SSE, pool acquisition, and transport cancellation and correlates local JSON logs by trace. Direct SDK remains a possible future alternative, not a second implemented runner. |
| Code ownership | Standalone implementation under `.tools/quick-check`; no code reuse from reliability. Model configurations are shared at `.tools/model_config_tools.yaml`. |
| Fixtures | Use Qwen3.6 MoE Q2_K_XL for the first cell so it can run on this machine. Reserve Qwen3.8-27B Dense for expansion. Deeper reliability Q8 fixtures remain unchanged; Q2/Q8 timings are not version-only comparisons. |
| Warm-up | Keep a small unrelated first request before the 10K workload. Measure and preserve it separately rather than hiding startup cost or mixing startup into every phase. The measured cold prompt must still attest zero cached tokens. |
| Reporting | Keep decision/comparison metrics in `summary.json`; full answers, metadata, counters, and execution evidence remain in `events.ndjson`. Compact reporting must not weaken assertions. |
| Quick inspection | The initial seven-line header was too verbose for the user's goal. Use four lines: overall result, tested commit, IMC/MTP + serial isolation + cancel/recovery verdicts, and first-request/workload time. On failures only, add deeper-tool directions (IMC/MTP: hybrid and embedded-MTP tools; isolation/recovery: hybrid tool). Keep detailed metrics in summary.json. Labels summarize complete probes, not root causes; missing phases/evidence are UNVERIFIED. Setup/evidence problems must be resolved before deeper inference checks. |
| AMP analysis | Optional future contextual triage, not a required oracle for PASS and not implemented. Machine-checkable infrastructure evidence is the first gate. |

Terminal output is limited to the finalized log header, with no phase progress,
startup artifact-path announcement, or duplicate explanation of reported findings.
Keep live progress in `tool.log`; preserve nonzero failure exits. Setup/storage
errors that cannot be fully reported by the finalized summary remain visible on
stderr. The quick-check make target suppresses its recipe echo too.

### Why this workload instead of OpenCode

The original proposal was an OpenCode task that writes Hello World in Go and
then other languages. Its appeal was realistic chat traffic with a large system
prompt. We retained the large-context, multi-turn characteristics, but chose a
controlled conversation because the subject under test is **Kronk infrastructure**,
not OpenCode's agent behavior or the model's programming ability.

- **Control which paths execute.** The harness deliberately requests an uncached
  prefix, exact replay, append, independent conversation, active cancellation,
  and same-slot recovery. An agent task can succeed without exercising these
  transitions; a huge shared prompt alone does not prove their coverage.
- **Keep failures attributable.** An OpenCode run can vary its tool calls,
  retries, generated programs, and number of turns. A failure or slowdown could
  come from the client, model decisions, compiler, or environment rather than
  Kronk. Direct requests remove those extra moving parts without bypassing
  Kronk's server, admission, planning, cache, and inference pipeline.
- **Make version comparisons meaningful.** A versioned prompt and prescribed
  request sequence give each phase a defined purpose and measurable boundary.
  We still retain actual assistant replies and token counts: model output can
  vary, so a controlled workload is not a claim of deterministic answers or
  perfectly identical work across versions.
- **Check infrastructure without executing generated code.** Lightweight
  handover facts provide omission/contamination signals, while logs and usage
  attest cache reuse, drafting, cleanup, and recovery. No rigid answer format,
  exact prose equality, compilation, or general model-quality judgment is needed.
- **Leave room for comparable image variants.** The handover scenario can later
  incorporate visual evidence and explicit media-state transitions without
  changing into an unrelated coding benchmark. That image workload is not yet
  designed or qualified; text-only success does not establish image coverage.

OpenCode remains useful for a separate end-to-end coding-agent integration
benchmark. The tradeoff here is less agent realism in exchange for controlled,
diagnosable infrastructure checks. The synthetic 10K-token handbook approximates
context size, not OpenCode's complete prompt semantics or tool-use distribution.

### Why handovers rather than Hello World in several languages

The advantage is the controlled request sequence and observable state markers,
not an inherently better inference workload. Direct Hello World requests could
exercise the same Kronk paths if structured with the same replay, append,
independent-session, cancellation, and recovery transitions. This is a separate
choice from avoiding OpenCode's agent loop.

Handovers make lightweight content checks easier:

- Distinct private identifiers, owners, and destinations help expose foreign
  session facts; similar Hello World programs naturally share content.
- Updating only the destination checks that the latest turn is reflected while
  the identifier and owner remain available.
- A marker introduced only in the cancelled request should not appear in the
  fresh recovery conversation.

If generated-program content were ignored, usage and server evidence could still
check stream completion, cache reuse, MTP activity, cancellation, cleanup, and
timing. What would be lost is the lightweight content signal for stale or foreign
state. Neither subject guarantees infrastructure coverage: requests and execution
evidence establish the exercised paths, and content findings alone do not prove
a Kronk defect.

### Fixed prompt size instead of runtime calibration

The initial prototype searched for enough handbook records to reach 10,000–10,500
tokens. Replace that search with the 72 records used in the qualified runs: fewer
tokenization requests and no tokenizer-dependent resizing between versions. Keep
one untemplated tokenization call and record the actual count for timing analysis;
do not enforce the old size range. The existing prompt hash remains diagnostic
metadata, not a cross-version equality requirement. Appended histories still use
actual assistant replies, so per-request token counts can vary.

Repository precedents support both choices: the SDK pure-cache-hit test uses fixed
messages, and the Hybrid reliability scenario uses a fixed record count. MTP and
long-context reliability scenarios calibrate because token volume is an explicit
scenario parameter. Quick-check does not need that sizing machinery.

### Server identity and bounded output retention

Record the running server binary's full VCS revision and modified flag through
`/v1/liveness`. Display `<hash>`, `<hash>+develop`, or `unknown` in the existing
four-line log header and preserve structured provenance in the summary. Do not
infer server identity from the harness checkout or equate the generic server
`build: develop` label with a dirty source tree. The tools server launcher now runs
the Go package with explicit VCS stamping rather than a single Go source file.
An old/unstamped server remains unknown until rebuilt and restarted.

The second header line, directly below the overall verdict, reads
`Tested commit: <seven-character-hash>[+develop] from <commit-timestamp>`.
Keep full hashes in JSON reports and retention decisions; only the header is shortened.
Use the binary's embedded `vcs.time` (exposed as `vcs_time` by liveness), not the
build or invocation time. Preserve it as `server_commit_time` in the reports;
missing timestamps display as `unknown` without changing retention eligibility.

After writing and closing all artifacts, retain the current candidate plus the
newest prior committed run; without one, retain the newest prior candidate. A
development or unknown candidate cannot evict a committed baseline. A new committed
candidate is retained alongside the old committed baseline and becomes eligible
as the next invocation's baseline. FAIL results participate too: retention is not
an assertion that a baseline passed or that two runs are performance-comparable.
Automatic comparisons and thresholds remain future work.

Only recognized timestamped run directories with the three regular artifact files
and a valid quick-check summary are eligible for pruning, including legacy outputs.
Unknown legacy provenance is never promoted from `harness_revision`. Preserve
unrecognized/incomplete/symlinked entries. On artifact write/close failure, preserve
all prior results; interrupted or failed-to-write runs can exceed the two-result
bound. Serialize invocations sharing the output parent with an exclusive lock;
after a crash, its stale lock requires manual removal once no run is active.

Run artifacts under `output/` are trackable in Git for historical comparisons.
Keep `output/saved_local_runs/` ignored as a local archive, along with the runtime
`output/.quick-check.lock`. The archive is outside retention's timestamped-directory
selection and is never pruned. Retention still applies to trackable run directories,
including tracked ones; review resulting deletions and commit selected artifacts
to preserve them in history. Uncommitted output additions/deletions can mark the
next server build `+develop`, so commit them or move local-only artifacts into the
ignored archive before building a clean baseline.

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

### Machine provenance for timing comparisons

Write summary metadata before request blocks: machine, server/library provenance,
run identity and totals, first request, then the workload results in execution
order. Use an ordered struct for the top-level JSON instead of an alphabetized map;
preserve existing key names rather than adding numeric prefixes. Request ordering
is represented by the existing results array, not object-key sorting. This is a
reading-order change, not a change in measurements or retention semantics.

Keep the `machine` block minimal: server hostname and OS family from liveness;
RAM bytes, aggregate GPU-memory bytes, and unified-memory flag from `/v1/devices`.
The initial inventory, CPU/runtime fields, and individual device descriptions were
too verbose for this summary. Full device data stays in diagnostic events. On
unified-memory machines GPU-reported capacity is not additional dedicated VRAM;
do not sum it with RAM. Reuse the lightweight endpoint instead of the broader
`/v1/diagnose` subprocess and backend probes. Collect before the workload clock,
never substitute the local harness machine's identity for a remote server's.

These fields expose a changed environment; they do not make different machines
comparable. Inventory is not actual placement evidence, OS family is not an OS
release, and driver versions, contention, power/thermal conditions, and client
networking remain uncontrolled. Older liveness fields are null and unsupported
memory queries may report zero, never assumed hardware equivalence. Automatic
comparison compatibility checks remain future work.

## Findings from actual runs — 2026-10-20

- First run `output/20261020T175401Z-1949178444` failed only because the harness
  classified the deliberate cancellation's middleware ERROR as unexpected.
  The nested code was `canceled`, ending in `context canceled`; the engine
  acknowledged cancellation and cleared target/draft state. Narrowly recognize
  that expected disconnect only on the request the harness cancelled and only
  with engine acknowledgment. Other errors remain failures; raw events remain.
- Strengthened checks require executed reuse counters to agree with the plan
  and client usage, paired target/own-KV draft snapshots/restores, resumed draft
  KV, and terminal cleanup/recovery on the configured physical slot. Successful
  answers or server-wide MTP enablement alone are insufficient.
- Run `output/20261020T182229Z-1060445797` passed all six phases. The system text
  was 10,133 tokens; the cold rendered prompt was 10,216. Exact replay reused
  10,209 stable tokens; append reused those tokens and extended by 306;
  independent/recovery requests reused only the 10,137-token System prefix.
- That run took 39.996 seconds for the warmed workload. The former combined
  `load_seconds` was 2.428 seconds. These are observations, not thresholds or a
  statistically qualified baseline.
- Compacting that run's summary reduced it from the earlier format's roughly
  37 KB to 3.2 KB. Lifecycle metrics now add useful comparison data back, without
  returning full diagnostic payloads to the summary.
- Lifecycle-metric verification run `output/20261020T190438Z-1293979891` passed
  with zero findings and all six required server durations for the first request
  and each workload phase. First-request total was 7.632 seconds, first content
  7.572 seconds; warmed workload was 40.221 seconds. The first-use total differs
  from the earlier run's 2.428 seconds, but these uncontrolled single runs do not
  establish a regression or its cause. No pure loading duration is inferred.
- Focused race-enabled harness tests and package vet/Staticcheck passed. Timing
  tests use distinct µs/ms/s values, accept a measured zero queue wait, reject
  missing/negative durations, and verify first-request metrics survive reporting.
- Quick-inspection verification run `output/20261020T192603Z-1837175713` passed;
  its `tool.log` begins with the seven-line verdict/timing block and preserves
  phase progress below it. Harness checks also verify early failures leave
  uncompleted probes UNVERIFIED and cancellation cleanup failures appear as FAIL.

## Timing contract — 2026-10-20

The manual's `04-generation-inference-lifecycle.svg` depicts per-request work.
Loaded weights do not eliminate admission, prompt planning, IMC selection, or
scheduling. The server acquires a resident model (or loads a missing one) before
the SDK chat call. SDK admission and waiting for a physical slot are different
capacity boundaries.

Source owners inspected:

- `cmd/server/app/domain/chatapp/chatapp.go`: HTTP decoding, pool acquisition,
  then `ChatStreamingHTTP`.
- `sdk/pool/engine/acquire.go`: resident-handle hit versus plan/reserve/load.
- `sdk/kronk/admission.go` and `concurrency.go`: per-call admission and release.
- `sdk/kronk/model/chat.go`: preparation, token/cache plan, job submission.
- `sdk/kronk/model/batchgen_slot_start.go`: queue wait and prefill clock.
- `sdk/kronk/model/batchgen_tokens.go`: first sampled token and generation clock.
- `sdk/kronk/model/batchgen_finish.go`: generation elapsed, cleanup, slot terminal.
- `client.go`: client start, first nonempty SSE content, and terminal observation.

All duration metrics are numeric **seconds**; rates are tokens/second. Client
metrics and server metrics use their own clocks. Never subtract timestamps from
different machines or label their timing difference as pure preprocessing.

| Metric | Exact interpretation/source |
| --- | --- |
| `elapsed_seconds` | Client chat invocation starts (including local request/event serialization) to `[DONE]`, error, or deliberate cancellation return. |
| `first_content_seconds` | Same client start to first nonempty content received; includes transport, server work, and delivery. Not first response header or necessarily first sampled token. |
| `generation_seconds` | Client elapsed minus first-content duration; includes delivery/finalization, not pure decode. Omitted for deliberately cancelled streams. |
| `client_output_tokens_per_second` | Final completion tokens divided by client generation duration. |
| `server_output_tokens_per_second` | Server usage TPS: output tokens minus one divided by post-first-sampled-token elapsed; excludes the prefill interval. Not pure GPU execution time. |
| `server_timings.admission_seconds` | Stage 1 `admit-request`, completed `elapsed`; includes admission bookkeeping/wait, not permit holding lifetime. |
| `server_timings.prepare_seconds` | Stage 2 `prepare-model-work`, completed `elapsed`; validates/prepares request, renders/tokenizes/plans cache. These are grouped, not independently timed. |
| `server_timings.queue_wait_seconds` | Stage 3 completed `queue_wait`, recorded from queued-at until slot pickup. |
| `server_timings.slot_prefill_ttft_seconds` | `batch-engine/prefill-done` `ttft`: slot prefill clock to first sampled token, including slot setup/restore/snapshot work before that token. Excludes stages 1–3. Corresponds to usage TTFT, which is serialized with lower precision in milliseconds. |
| `server_timings.generation_seconds` | `batch-engine/slot-finished` `elapsed`: first sampled token to start of finish processing, before subsequent KV cleanup/final response work. Available from logs even on active cancellation. |
| `server_timings.slot_execution_seconds` | Stage 4 terminal `elapsed`: prefill clock through slot finishing/resource cleanup. Includes prefill and generation: do not add those intervals again. Does not include outer admission-release/HTTP completion. |
| `cancel_release_seconds` | Client cancellation returns to observed scheduler-idle confirmation, including HTTP/poll overhead. Not an exact engine cleanup duration. |
| `workload_seconds` | Six-phase workload wall time including idle waits; excludes first request, prompt tokenization, evidence extraction, and final unload. |

`warmup_request` uses the same request metrics but remains separate from `results`.
It begins after the dedicated alias is unloaded and uses `Reply OK.` with an
eight-token cap. It is **model-unloaded**, not guaranteed cold-process, cold GPU,
or cold-filesystem-cache startup. It replaces the misleading `load_seconds`:
neither its total nor its first-content latency isolates native loading alone.
The first request's assembled answer/usage/timings are preserved in diagnostics.

Missing, malformed, or negative required server durations produce findings, not
fabricated zero values. A measured `0s` is valid. Units are converted with Go's
duration parser; required metrics apply to all six phases and the first request.
No timing threshold or cross-version verdict is inferred from a single run.

### Startup investigation and retained diagnostics — 2026-10-21

Ramon observed roughly 2–9-second warm-up totals within the original code version
while warmed workloads stayed consistent. Those historical details are no longer
available; the retained cross-version pair cannot explain that same-version range.
Add diagnostic-only `warmup_request.startup_timings` using the existing warm-up
trace's server timestamps: full server request, pre-Chat acquisition/initialization,
native model-file load, projection load, and MTP load. Existing slot timings remain
separate. These intervals overlap, missing/invalid/reversed boundaries are omitted,
and no new correctness gate, inference request, load, or terminal line is added.
Collect repeated invocations against the same running binary/library before drawing
conclusions; do not restart the server between those invocations. Archive each run
in `output/saved_local_runs/` before retention can remove it. The pre-instrumentation
baseline is [this commit](https://github.com/ardanlabs/kronk/commit/33cf0229adbff3c4b6b72fe604c17a594f70e471).
The instrumentation was initially temporary. After reviewing the five runs below,
Ramon chose to retain its compact five-metric block for future comparisons: it
adds useful loading diagnostics without extra requests or model loads. Keep its
measured findings and decisions here rather than assuming startup variability is
harmless or attributing it to a specific cache without evidence.

Five saved invocations on the same reported revision (`33cf022` +develop), library
`b11738`, machine, model, and `operations-v2` workload all passed three repetitions.
System tokens/hashes matched per repetition. Ramon restarted the server between
runs 3 and 4; this restart boundary is operator-reported, not independently attested
by the report metadata. Run timestamps below identify directories in
`output/saved_local_runs/` (UTC):

| Run timestamp | Warm-up total | Native model load | Projection load | Workload median |
| --- | ---: | ---: | ---: | ---: |
| 20261021T171314Z | 5.555 s | 3.669 s | 0.675 s | 41.042 s |
| 20261021T172213Z | 3.668 s | 1.965 s | 0.540 s | 42.273 s |
| 20261021T172607Z | 2.456 s | 0.960 s | 0.388 s | 42.195 s |
| 20261021T173000Z (after restart) | 2.502 s | 0.949 s | 0.395 s | 41.193 s |
| 20261021T173858Z | 3.106 s | 1.442 s | 0.520 s | 40.875 s |

Recomputed server request and native-load intervals directly from each saved trace;
they match the new summary metrics. Client totals exceed server request intervals
by only 0.276–1.101 ms. Every warm-up trace attests a pool cache miss and native
model-file load, not accidental use of an already-resident alias. Native loading
accounts for about 2.709 s of the 3.099 s warm-up decrease from run 1 to run 3;
projection loading accounts for another 0.287 s. Warm-up slot execution remains
0.259–0.324 s. Thus these runs locate most variation in server loading rather than
the client timer or chat scheduling. They do not reproduce the full historical
2–9 s range. Restarting did not restore the initial slow loading: run 4 is close
to run 3, while run 5 is slower without another reported restart. OS/file caching,
memory pressure, and native/backend state remain hypotheses, not established
causes; no disk/page-fault or cache-state measurements were collected. Keep startup
separate from resident workload comparisons and do not impose a startup regression
threshold from these five samples. Dirty-build provenance also cannot establish
byte-identical binaries across a rebuild.

Historical example from the 18:22Z run (warm model):

| Measurement | Uncached prompt | Exact replay |
| --- | ---: | ---: |
| Admission | 6.5 µs | 14.4 µs |
| Preparation | 42.80 ms | 47.35 ms |
| Queue wait | 20.7 µs | 14.6 µs |
| Slot-side first-token time | 16,684.45 ms | 62.89 ms |
| Client first-content time | 16,730.28 ms | 111.76 ms |

This establishes that host preparation still runs on an exact cache replay.
It does not establish a persistent performance difference between cache modes.

## Repetitions and cache state — 2026-10-21

Start with **three complete workload repetitions**, configurable with
`-repetitions`. Repeat the full six-phase sequence, not each phase consecutively.
All repetitions must pass; stop on the first failure and explicitly report the
requested/executed counts and remaining unrun repetitions. Do not average away
an intermittent correctness failure. Keep all repetitions within one invocation's
artifacts, preserving the committed-baseline/latest-candidate retention policy.

**Warning: unloading between repetitions does not reset filesystem, GPU, or
process caches, so repeated first requests are not equally cold.** Keep
first-request measurements separate from workload measurements; do not call
model-unloaded latency a pure loading time or a guaranteed cold start.

Ramon rejected unnecessary unload/reload overhead. The implementation therefore
resets the alias only at invocation startup, measures `Reply OK.` once, keeps
weights resident through all repetitions, and unloads once at the end.

An unchanged workload would otherwise reuse a previous repetition's working or
System snapshot. Inspection of `caching_imc_tokens.go` shows that working reuse
requires a complete token-prefix match and System reuse requires equal system
tokens. `operations-v2` prepends a fixed `Quick-check repetition N.` marker to
the unchanged 72-record handbook. Each index has the same prompt across versions;
markers differ between repetitions without a random nonce or size calibration.
Measure the actual token count and retain prompt identity for each repetition.
Keep the existing zero-reuse build/publication checks: a marker is not evidence
that the intended path actually executed. No public full-cache-reset API exists;
the private `clearCaches` only resets working entries, not the shared System tier.
Avoid adding a production API solely to support this harness.

Report per-phase elapsed/first-content latency and completed workload replies'
server output rate as sample count, median, minimum, maximum, and sample standard
deviation (`sample_stddev`, n−1 denominator); also summarize workload total and
cancellation-release wait. The compact terminal/log header includes workload SD.
With one repetition, SD is undefined: JSON null and header `SD n/a`, not zero.
The one-time warm-up-request time measures the actual `Reply OK.` client call through
its completed response, including model acquisition/loading, processing, inference,
and delivery. It is not time before issuing that request; preceding unload/setup
is excluded. Use **warm-up request** consistently: `warmup_request` in summary and
diagnostic JSON, `warmup` for the event phase, and "warm-up request" in the header.
It warms the model for subsequent work but starts with the alias unloaded.
Preserve individual server stage durations,
token counts, findings, and detailed evidence per repetition. Publish
aggregates only for a complete passing invocation; startup remains one separate
sample. These are descriptive measurements, not confidence intervals or automatic
regression thresholds. `operations-v1` timings are not strictly comparable to
`operations-v2`, which adds repetition markers and keeps weights resident.

The first resident-model trial with the original 160-word review target passed
repetition one (38.430 seconds), then stopped at repetition two's first reply:
zero cache reuse was correctly attested, but generation reached the existing
384-token limit (`finish_reason: length`). Repetition three was unrun and no
performance aggregate was published. This finding does not establish an engine
regression. Reduce the requested reviews to about 80 words rather than increasing
the cap or accepting truncated replies. The input remains approximately 10K
tokens, and the same cache/state/isolation/cancellation/MTP assertions apply.
This also avoids spending extra generation time without exercising another path.

Validation of the revised workload passed all three repetitions on the running
Q2 fixture: 43.227, 41.145, and 39.747 seconds (median 41.145); the one-time first
request took 2.887 seconds. Events contained one loading request, one MTP backend
load, and 19 total chat requests. Each repetition measured 10,140 untemplated
system tokens, attested zero reuse on its cold phase, and passed all six phases.
Completed workload replies still varied from 221 to 321 tokens: requesting fewer
words does not guarantee shorter output or establish a speedup. The run verifies
resident-model repetition, not cross-version performance stability. Focused
race-enabled harness tests, package vet, go fix/gofmt, and Staticcheck passed.

**Mandatory review, not an optional follow-up:** once we have results from
different commit versions **and** other matrix cells, revisit this cache-state
warning using those results. Review whether three repetitions are sufficient,
whether repetition order or startup effects bias timings, and which comparisons
need a different measurement protocol. Do not assume this initial strategy is
permanently sufficient or establish merge-gate thresholds before that review.

## Roadmap and open decisions

- Implemented: first-cell workload, per-request infrastructure evidence,
  compact summary plus diagnostics, separately retained first request, normalized
  lifecycle metrics, three resident-model repetitions with per-phase median/range
  reporting, and focused harness regression checks.
- Next: qualify variability/repetition strategy and comparison thresholds on
  controlled runs before using timing as an automated merge gate. Compare each
  phase separately, with matching prompts, quantization, config, native backend,
  hardware and machine load; preserve token counts alongside durations/rates.
  The cross-version/other-cell review above is required once those data exist.
- Establish trustworthy actual server/native-build provenance. Harness HEAD,
  liveness `develop`, and the library endpoint's installed version do not attest
  the exact running binary or loaded library bytes.
- Decide whether finer loading/rendering/restore/cleanup attribution is needed.
  Current grouped durations must not be presented as independent substep timings;
  use existing reliable spans or deliberately add instrumentation if warranted.
- Expand to the Dense fixture and other matrix cells after qualifying this cell.
  Many-slot cases need actual concurrent progress/isolation evidence; one-slot
  results do not validate that scheduler contract. Images need attested media
  work/state transitions, not an attachment that is silently ignored.
- Deliberately attested speculative rejection/rollback is not currently covered.
- CI invocation, automated baseline comparison, and optional AMP triage remain
  future work, not delivered behavior. Deeper tools remain diagnostic resources.
