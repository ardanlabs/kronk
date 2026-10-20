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
| Large system prefix | Calibrate a versioned synthetic handbook to 10,000–10,500 untemplated tokens using the selected tokenizer. A large prompt alone is insufficient: deliberately exercise exact replay, append, independent-session System reuse, active cancellation, and recovery. |
| Server versus direct SDK | Current prototype uses the server: it also exercises HTTP/SSE, pool acquisition, and transport cancellation and correlates local JSON logs by trace. Direct SDK remains a possible future alternative, not a second implemented runner. |
| Code ownership | Standalone implementation under `.tools/quick-check`; no code reuse from reliability. Model configurations are shared at `.tools/model_config_tools.yaml`. |
| Fixtures | Use Qwen3.6 MoE Q2_K_XL for the first cell so it can run on this machine. Reserve Qwen3.8-27B Dense for expansion. Deeper reliability Q8 fixtures remain unchanged; Q2/Q8 timings are not version-only comparisons. |
| Warm-up | Keep a small unrelated first request before the 10K workload. Measure and preserve it separately rather than hiding startup cost or mixing startup into every phase. The measured cold prompt must still attest zero cached tokens. |
| Reporting | Keep decision/comparison metrics in `summary.json`; full answers, metadata, counters, and execution evidence remain in `events.ndjson`. Compact reporting must not weaken assertions. |
| Quick inspection | The initial seven-line header was too verbose for the user's goal. Use three lines: overall result, IMC/MTP + serial isolation + cancel/recovery verdicts, and first-request/workload time. On failures only, add deeper-tool directions (IMC/MTP: hybrid and embedded-MTP tools; isolation/recovery: hybrid tool). Keep detailed metrics in summary.json. Labels summarize complete probes, not root causes; missing phases/evidence are UNVERIFIED. Setup/evidence problems must be resolved before deeper inference checks. |
| AMP analysis | Optional future contextual triage, not a required oracle for PASS and not implemented. Machine-checkable infrastructure evidence is the first gate. |

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
| `workload_seconds` | Six-phase workload wall time including idle waits; excludes first request, calibration, evidence extraction, and final unload. |

`first_request` uses the same request metrics but remains separate from `results`.
It begins after the dedicated alias is unloaded and uses `Reply OK.` with an
eight-token cap. It is **model-unloaded**, not guaranteed cold-process, cold GPU,
or cold-filesystem-cache startup. It replaces the misleading `load_seconds`:
neither its total nor its first-content latency isolates native loading alone.
The first request's assembled answer/usage/timings are preserved in diagnostics.

Missing, malformed, or negative required server durations produce findings, not
fabricated zero values. A measured `0s` is valid. Units are converted with Go's
duration parser; required metrics apply to all six phases and the first request.
No timing threshold or cross-version verdict is inferred from a single run.

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

## Roadmap and open decisions

- Implemented: first-cell workload, per-request infrastructure evidence,
  compact summary plus diagnostics, separately retained first request, normalized
  lifecycle metrics, and focused harness regression checks.
- Next: qualify variability/repetition strategy and comparison thresholds on
  controlled runs before using timing as an automated merge gate. Compare each
  phase separately, with matching prompts, quantization, config, native backend,
  hardware and machine load; preserve token counts alongside durations/rates.
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
