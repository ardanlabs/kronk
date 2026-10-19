# Reliability coverage reference

[coverage.csv](coverage.csv) is the quick coverage viewer: combinations, selected
models, short status labels, and gap keywords. This document is the reference for
what those labels mean, what each profile actually checks, and what it leaves
unchecked. Keep detailed explanations here rather than expanding CSV cells.

This is a source-based assessment of the current workloads and default fixtures,
not a record of executions or passing results. Proposed gaps are planning items,
not authorization to implement or run the full matrix.

## Target: one-command regression assessment

The intended suite runs a fixed, representative coverage set with **one command**.
There is no operator-driven sequence of qualification, state, and deep-test runs.
Fixture validation, mode selection, loading/unloading, short state probes, selected
context stages, sustained load, and lifecycle checks belong inside that invocation.
One command does not mean every profile runs in every matrix cell.

SDK tests run separately as an existing prerequisite. The reliability command
must not invoke them again or duplicate their smoke/algorithm checks. Its first
run must collect the additional server/state evidence selected here, without an
operator deciding from that run which further coverage tests to launch. Follow-up
runs may diagnose failures, not complete an initially partial coverage selection.

Models are fixtures for Kronk paths, not independent coverage objectives. Both
attention-only and recurrent state must be represented for restore/recovery;
speculative coverage must demonstrate the intended rollback paths, not merely
positive draft counts. Media scheduling selection must distinguish shared-capable
and isolated execution, not assume Hybrid/Attention-only selects the scheduler.

Fixture/backend/capability checks are automatic prerequisites of the selected
cases, not separate runs the operator must perform. A missing fixture, unexpected
backend, unexercised required path, or skipped required context stage must prevent
a complete-coverage verdict. Optional/out-of-scope cases must remain distinct.
Timing measurements need comparable baselines and explicit regression criteria
before they support a performance-regression verdict.

A passing report should establish that the selected behaviors were exercised and
no regressions were detected in that scope, with provenance and limitations. It
cannot prove the absence of all bugs. The current tools do not yet implement this
whole contract; in particular lifecycle is separate and several selected paths
remain GAPs. This document defines the target, not a new command implementation.

## Matrix labels

- `covered`: the current default fixture and workload contain the profile's
  checks described below. It does not mean exhaustive coverage of the cell.
- `not apply`: outside the current profile's contract.
- Empty: uncovered; its priority is not yet decided.
- `overlaped by T:L`: the required behavior already has a `covered` cell in tool
  column T (the number in its header), CSV line L (header is line 1). No separate
  run of this profile is needed for that cell's behavior. Update references if
  lines or tool numbers move.
- `GAP: ...`: an uncovered path proposed as important.
- `GAP: ...; need other model`: important coverage with an outstanding model
  selection. For classic drafting this includes the separate draft model, even
  when a target is selected.

`need other model` does **not** necessarily mean downloading a new model. An
existing model may fit after selecting it for that row and configuring a suitable
alias. Candidates below are not selections; leave Model ID empty until selected.
Routing (Dense/MoE) and State (Attention-only/Hybrid) are independent properties.

## Current fixtures behind covered cells

All reliability profiles below have four MTP/IMC aliases. MTP-off aliases run
target-only controls; they do not exercise active drafting. Media means image
input in these tools, not general audio/video coverage.

| Profiles | Default model ID | Routing / State | Slots | Media workload |
| --- | --- | --- | --- | --- |
| mtp-embedded; batch-text | unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL | MoE / Hybrid | 4 | No |
| batch-images; media-correctness; media-prefill | unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL | MoE / Hybrid | 4 | Yes; media profiles include text controls |
| mtp-companion | unsloth/Qwen3.8-27B-UD-Q4_K_XL | Dense / Hybrid | 2 | No |
| hybrid; long-context | unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL | MoE / Hybrid | 1 | No |
| example-lifecycle-load | unsloth/Qwen3-0.6B-Q8_0 | Dense / Attention-only | 1 | No |

Lifecycle is separate: its fixture has IMC on and no configured MTP, but the
lifecycle tool does not assert those modes. No `classic-drafter` profile exists.

## What covered cells contain, and their internal gaps

### Shared mode assertions

Reliability profiles check the loaded IMC configuration and scheduler MTP mode.
Completed requests must report positive prompt/completion usage. IMC off requires
zero cached tokens. MTP off requires zero draft tokens, accepted drafts, and
coverage. MTP on rejects a draft-disable reason and requires positive drafting and
coverage above eight completion tokens. Individual profiles can impose stronger
requirements, as described below.

**Limits:** cancelled streams do not necessarily carry final usage. Usage fields
show activity, not semantic equivalence to target-only decoding. IMC-on exact or
append reuse is asserted only by workloads that explicitly repeat or append.

### mtp-embedded and mtp-companion

**Covers:** distinct embedded-head and companion loading paths; configured slot
counts; calibrated concurrent text requests; successful `stop` completions. With
MTP on, every measured request must demonstrate positive drafting/coverage without
disablement. With MTP off, requests are target-only controls. When server evidence
is available, validation checks observed backend/source records and a per-request
draft-KV-cleared event.

**Internal gaps:** request count and simultaneous submission do not prove every
physical slot executed or that all requests generated concurrently. There is no
exact/append cache-reuse sequence, image workload, or target-only output-equivalence
comparison. Backend evidence validation is qwen35-own-kv-specific and optional
when the server log is unavailable; it is not general qualification of other MTP
backends. Output hashes are recorded, not compared for equivalence.

### hybrid

**Covers:** exactly one configured and diagnostic slot; deterministic cold/repeat
text; an appended instruction with a new exact marker; positive exact/append reuse
when IMC is on. A stream is cancelled after content, then a fresh request must
return its own exact marker without the cancelled marker. One slot forces recovery
to reuse the same execution capacity. Active MTP also needs positive aggregate
drafting/coverage across the completed requests.

**Internal gaps:** the profile name does not assert model architecture. It does
not deliberately trigger and identify rejected speculative rounds or rollback
paths. Cancellation is after generated content, not during prefill or cache
publication. There are no images or concurrent session-migration checks. Exact
markers and cache counts do not independently prove native snapshot correctness.

### long-context

**Covers:** staged text retrieval at 4K, 8K, 16K, 32K, 64K, and 128K targets by
default. Each stage checks three exact markers in order and rejects known foreign
stage markers. Cold/repeat output must match; IMC-on repeats must report reuse,
while IMC-off repeats perform full prefill. Stages above reported capacity skip;
at least one stage must run.

**Internal gaps:** no exact-limit/overflow assertion, concurrent long-context
interference, cancellation, media, or appended-history retrieval. These are serial
requests, not proof of many-slot capacity. A skipped stage provides no coverage;
marker retrieval is a narrow integrity check, not a general long-context quality
evaluation.

### batch-text

**Covers:** five conversations on four slots by default; at least 21 growing
history turns and a final prompt of at least 30K tokens. Every response validates
its session/turn canaries and isolation. Every turn requires all configured slots'
client-visible generation intervals to overlap, plus queue-pressure evidence
inferred from first-content/completion timing. IMC-on appends must show reuse.

**Internal gaps:** no dedicated exact replay, cancellation/recovery, or image
history. Queue evidence is a client-timing proxy, not authoritative server queue
or request-to-slot attribution. The strict generation-overlap assertion differs
from slot occupancy; it remains unchanged. A failure stops that workload, so
later turns are not exercised. Timing collection does not itself assert a
performance-regression threshold.

### batch-images

**Covers:** the same history/load/isolation assertions as batch-text, plus image
subjects distributed across conversations, a later subject switch, and exact
request replays at the initial-image and subject-switch turns. Responses must
identify the latest subject and exclude foreign/stale fixture subjects. IMC-on
replays/appends must show reuse. Overlap and queue-pressure checks also apply to
replay phases.

**Internal gaps:** no cancellation or forced same-slot recovery; the same
queue/slot-attribution limitation as text batch. Subject checks do not establish
general visual understanding. Whole-response equality is intentionally **not**
required and is not a missing assertion to add. This is sustained image-history
load, not a substitute for media-prefill responsiveness.

### media-correctness

**Covers:** image subject/prefix; text-only exact-marker isolation; deterministic
image replay; appended-image subject retention and IMC-on reuse. Cancels an image
history after generated content, waits for all slots to become idle, then checks
fresh image and text recovery with new markers. With MTP on, first/repeat/append/
recovery image requests must each demonstrate positive drafting and coverage.

**Internal gaps:** recovery is not pinned to the cancelled physical slot on the
four-slot fixture, although the all-idle check prevents a stuck slot from being
silently ignored. No concurrent image-session isolation, changed-image history,
or prefill-time cancellation. Those are distinct from this profile's short serial
image-state checks.

### media-prefill

**Covers:** starts sustained text generation, then submits an image after five
content events. Scheduler polling must observe media-prefill alongside generation.
Both streams must return content; text gaps intersecting the image-prefill window
must stay within the configured bound (one second by default). Records baseline
and overlapping gaps plus text/image request timings.

**Internal gaps:** only two concurrent requests on the four-slot fixture, not
all-slot saturation. Polling is phase-level rather than request-to-slot
correlation. It does not validate image subject semantics, cache replay/append, or
cancellation. Client content-event gaps are not per-token server timings; timing
metrics alone are not a version-regression assertion.

### example-lifecycle-load

**Covers:** one execution slot and two admission permits; an active holder, queued
cancellation, admission timeout for a further request, holder cancellation, and
successful recovery. Structured lifecycle logs provide stage-level evidence.

**Internal gaps:** fixed text fixture without a mode matrix or explicit IMC/MTP
verification. No cache/draft-state, media-state, or output-isolation assertions.
Successful recovery demonstrates released capacity, not all inference-state
invariants. It does not replace the state, batch, or media profiles.

### How internal gaps appear in the CSV

A `covered` marker stays concise: it means the bounded contract above exists,
even when that contract has limitations. Internal gaps are recorded here; `GAP`
cells highlight selected missing combinations/workloads elsewhere in the matrix.
Neither notation automatically commits us to extending every profile. Some
limitations are intentionally covered by another profile instead.

## Confirmed overlaps

| Empty cell (tool:line) | Covered source (tool:line) | Reason |
| --- | --- | --- |
| 2:35 | 3:35 | Dense Hybrid target-only control; IMC off. |
| 3:37 | 2:37 | MoE Hybrid target-only control; IMC off. |
| 2:51 | 3:51 | Dense Hybrid target-only control; IMC on. |
| 3:53 | 2:53 | MoE Hybrid target-only control; IMC on. |

Both MTP profiles call the same `runMTPProfile` implementation. With MTP off,
backend/source and draft-KV evidence checks are skipped, and the profile adds no
embedded-versus-companion assertion. Each reference stays on the same model,
slot configuration, media mode, and IMC setting: running the other profile name
on that configuration would repeat its calibrated concurrent text control.
This does not remove the controls on either of the two distinct target models.

Other empty cells remain unmarked unless the full required behavior is replaced:

- With MTP on, embedded and companion activation are distinct loading paths.
- One-slot and many-slot contexts are not interchangeable. A serial request on
  a many-slot model does not establish the one-slot recovery contract.
- Batch prompt growth is not staged long-context retrieval or exact replay;
  image batch does not replace text-only histories or deterministic media replay.
- Shared cache/usage assertions are partial overlap, not replacement of a whole
  profile's state, load, cancellation, or responsiveness checks.
- Existing coverage does not establish equivalent behavior on another routing/
  state architecture, an unselected model, or multi-GPU placement.

## Representative selections in the matrix

- **State/recovery:** retain Hybrid text coverage and Dense Attention-only
  recovery GAPs. Add Attention-only same-slot image recovery on lines 6/22 and
  media correctness on lines 38/54, with MTP off and IMC off/on. This isolates
  attention state without requiring an attention-only MTP implementation.
- **Context/load:** retain the Attention-only context baseline and Dense
  Attention-only batch GAPs. Remove the additional MoE Attention-only batch GAPs
  on lines 36/52: a second routing type is not required merely to represent
  attention-only state. Those cells are blank, not proven overlaps.
- **Classic:** retain four text-only GAPs: attention-only trim rollback on lines
  2/18 and hybrid snapshot/replay rollback on lines 3/19, with IMC off/on. Remove
  blanket classic GAPs across MoE and media combinations. Require rejected rounds
  and evidence of the selected rollback path; a Hybrid label alone does not
  guarantee the snapshot fallback executes.
- **MTP:** retain embedded/companion activation and one-slot GAPs, without
  multiplying them over Attention-only just to populate another category.
- **Media prefill:** remove companion-only prefill GAPs. Both current Qwen
  fixtures have input-mixing support; a companion file alone does not identify
  another scheduler branch. Add isolated-prefill GAPs on lines 40/56, using an
  Attention-only MoE candidate because it reaches that scheduling path, not
  because that architecture category needs another test. Shared-capable fixtures
  may also isolate particular chunks; the runner must attest which paths occur.
- **Image batch/lifecycle:** keep existing coverage. Additional architecture
  combinations remain blank rather than mandatory GAPs. They are not claimed to
  be fully covered by other profiles.

All existing `covered`, `not apply`, and overlap labels are preserved. Multi-GPU
remains phase 2. The one Model ID per row describes the current fixture, not a
requirement that every future tool on that row use that same model; suggestions
below are per profile. In particular the embedded one-slot GAPs use Qwen3.6,
not the Flash-Next state/context fixture shown on those rows.

## Important gaps

### Model suggestions for `need other model` cells

These are fixture suggestions for reaching Kronk paths, not additional model
tests or selections. Reuse means a model already used by
the reliability/lifecycle tools; repository candidates come from the catalog or
SDK test fixtures and are not assumed installed. An empty Model ID alone is not
a reason to bring in another model.

Use the same suggestion for both IMC settings unless noted. Configuration and
workload changes remain necessary; a suggested model does not close a GAP.

| Gap keywords | Tool columns | Why important | Model fit / selection needed |
| --- | --- | --- | --- |
| classic trim rollback | classic-drafter | Cover classic loading, verification, rejection and attention-only position trimming. No dedicated profile exists. | **Repository candidate:** target `unsloth/Qwen3-1.7B-Q4_K_M` + reused draft `unsloth/Qwen3-0.6B-Q8_0`, the pair defined by `CfgClassicDraftChat`. Use one-slot, text-only configurations with IMC off/on. |
| classic snapshot rollback | classic-drafter | Cover hybrid pre-verification snapshot restoration and accepted-prefix replay when native rollback is insufficient. | **Reused target candidate:** `unsloth/Qwen3.8-27B-UD-Q4_K_XL` with MTP disabled. No compatible full classic draft is established here. A pair remains a required fixture-design dependency, not an operator follow-up. The workload must force and attest this rollback branch. |
| one-slot drafting / one-slot target-only control | mtp-embedded; mtp-companion | One active request on a multi-slot context does not test native one-slot capacities. Cover active drafting and its disabled control without a deep batch run. | **Reuse** `unsloth/Qwen3.8-27B-UD-Q4_K_XL` for companion rows, configured with one slot. For embedded GAPs, reuse `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL` with one slot, not Flash-Next: Flash-Next uses a companion, not embedded MTP. No new model or Flash-Next backend adaptation is required for this slot-count path. |
| dense hybrid recovery | hybrid | Current one-slot state coverage uses MoE Hybrid. Add dense recurrent-state restore and cancellation/recovery. | **Reuse** `unsloth/Qwen3.8-27B-UD-Q4_K_XL` with a one-slot state-probe fixture, for MTP on/off. Its existing two-slot fixture is not sufficient. |
| attention-only recovery | hybrid | Establish a non-recurrent baseline for deterministic reuse and same-slot cancellation/recovery. The existing assertions are useful beyond hybrid architectures. | **Reuse** `unsloth/Qwen3-0.6B-Q8_0` for both IMC settings, with a one-slot state-probe fixture. The IMC-off row does not require a different model merely because its Model ID is empty. |
| attention-only context baseline | long-context | Separate ordinary attention/cache behavior from recurrent-state behavior during staged retrieval. | **Reuse** `unsloth/Qwen3-0.6B-Q8_0` for the small stages under its current 8K configuration, for both IMC settings. Native context support above 8K has not been established here. If larger stages are necessary, qualify its GGUF context limit first; no full-128K attention-only candidate is selected by this assessment. |
| attention-only isolation/load | batch-text | Represent attention-only multi-session state integrity alongside existing Hybrid load coverage; no additional MoE baseline is required by this distinction. | **Reuse candidate** `unsloth/Qwen3-0.6B-Q8_0`, conditional on native support for the unchanged 30K prompt workload plus generation, and a larger-context, multi-slot fixture. Its current 8K fixture cannot do this. Establish supported context during fixture design and validate it automatically in the suite. Both selected rows keep MTP off. |
| dense hybrid isolation/load | batch-text | Companion activation checks do not cover growing histories, canary isolation, oversubscription, or all-slot generation overlap. | Qwen3.8-27B is already selected and fits Dense Hybrid. Add a matching batch fixture; its current two slots differ from the batch default of four. |
| same-slot image recovery | hybrid | Image cancellation/state cleanup must not be hidden by recovery on a spare slot, for either state representation. | **Hybrid: reuse** `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL` and its projection, with one slot and MTP on/off. **Attention-only: catalog candidate** `ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0` and its projection, with one slot and MTP off. Confirm normalized state classification when selecting this fixture. Add image-state assertions; the current state workload is text-only. |
| companion image-state integrity | media-correctness | Image replay/append and cancellation on the embedded model do not qualify the companion model's image-state path. | **Reuse** `unsloth/Qwen3.8-27B-UD-Q4_K_XL` with its projection and companion. Configure the media profile for this fixture; preserve actual image-drafting assertions when MTP is on. |
| attention-only image state | media-correctness | Represent attention-only image replay/append, text isolation, cancellation and recovery alongside Hybrid state coverage. | **Catalog candidate:** `ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0` and its projection, with a multi-slot fixture and MTP off. Use IMC off/on; state/capability validation belongs inside the eventual suite. |
| isolated media prefill | media-prefill | Exercise scheduling and text responsiveness where media embeddings cannot join a shared decode contribution. | **Repository candidate:** `unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M`, already used by SDK vision tests, with its projection, at least two slots and MTP off. Gemma4 is unlisted in the input-mixing allowlist and therefore uses isolated embedding contributions. Attest actual execution mode and preserve meaningful responsiveness assertions; current phase-level polling alone does not identify this branch. |

### Classic fixture boundary

The Qwen3-1.7B / Qwen3-0.6B pair is suitable for the selected Attention-only fixture,
not the Hybrid fixture. A compatible Hybrid pair still needs to be established
as part of suite development. Separate MTP companion heads are not full classic
draft models; matching vocabulary sizes alone is insufficient. The removed MoE
and media classic GAPs are not claims of overlap or unsupported combinations:
they are outside the currently selected regression set.

### Existing classic SDK coverage versus reliability additions

This audit concerns classic separate-GGUF drafting, not a coverage audit of every
SDK model family. `tests/draft` is the only model-backed SDK suite found using
`CfgClassicDraftChat` / a classic `DraftModel` configuration. Other hybrid/IMC/MTP
suites cannot be counted as exercising that separate draft context.

| Existing test | Actual assertion | Boundary |
| --- | --- | --- |
| `draft/TestSuite/Chats/DraftChat` | Load the Qwen3-1.7B target and Qwen3-0.6B draft; complete an SDK `Chat`; validate response identity/structure and applicable nonempty content/reasoning fields; require a usage block. | One short echo prompt, not growing histories or a controlled rejection scenario. Missing the requested word `Gorilla` is a warning, not a failure. |
| `draft/TestSuite/Chats/DraftStreamingChat` | Drain SDK `ChatStreaming`, validate response chunks, accumulate nonempty content or reasoning, and require final usage. | No HTTP/SSE server path, client disconnect, or post-cancellation request. Missing `Gorilla` is also warning-only. |
| `draft/TestSuite/DraftAcceptance` | At least one request across the suite reports accepted drafts. | Per-request zero drafting/acceptance is allowed. This assertion is skipped on ROCm. It does not establish partial rejection, rollback correctness, acceptance rate, or acceleration. |
| Shared setup/cleanup | Create the model and check `Unload` returns without error. Default configuration has one slot, 8K context, four draft candidates and IMC enabled by default. | No IMC on/off matrix, cache-reuse assertion, draft resource accounting, reload/recovery assertion, or leak bound. Concurrent callers can queue on one slot; they are not multi-slot drafting. |
| Classic algorithm unit tests | Synthetic greedy partial rejection, probabilistic acceptance/residual rejection sampling, draft sizing/recovery-probe selection, request-state reset, and keep-position arithmetic. | Supplied tokens/probabilities and callbacks, not native target/draft memory. No end-to-end adaptive-throttle recovery assertion. |
| Engine/controller unit tests | Draft/context/output budget boundaries, snapshot-needed selection, snapshot failure handling, replay-batch token/position construction, and fake-host controller sequencing. | Do not restore a real hybrid sequence and verify the resulting continuation. |

The chat helpers retry returned errors, so eventual success can hide transient
attempt failures in the overall PASS; the failed attempts remain in logs.
Missing model files skip the whole SDK draft suite. These are evidence limits,
not a request to rerun it from reliability or modify SDK tests in this task.

**Keep outside reliability:** another load/chat/accepted-token smoke test,
duplicate probability arithmetic, or a benchmark claiming this pair must be
faster. Those do not justify a new profile.

**Add to reliability as one bounded classic state workload:**

1. **Native rejection/continuation:** demonstrate rejected drafts and validate
   continued output, for attention-only trimming and hybrid snapshot/replay.
   Record which branch actually executed. Use independently specified output
   expectations and a controlled greedy target-only comparison where applicable;
   do not demand identical stochastic text merely because seeds match.
2. **State reuse:** a long-enough cold/exact/append history with IMC off/on;
   verify expected output, reuse only when enabled, and no stale-session content.
   A configured IMC flag or repeated short echo prompt is not reuse evidence.
3. **Cancellation/recovery:** disconnect during active classic generation, then
   verify a fresh request on the only slot completes correctly without stale
   output. Check both target and separate draft state recover; include queued
   callers without multiplying this into multi-slot classic configurations.
4. **Native termination boundaries:** small output budgets and a near-context
   case must terminate within their limits and permit a clean subsequent request.
   Unit-tested budget arithmetic alone does not validate native finalization.

Exercise these through the server in one planned invocation, retaining existing
SDK smoke coverage as a separate prerequisite. Several checks can share the same
requests; this is not a request for four additional tools or a full deep-batch
matrix. Reliable branch evidence and a compatible Hybrid pair are still design
dependencies, so the current classic GAP cells remain GAPs. End-to-end throttle
recovery is a lower-priority follow-up, not a reason to duplicate its existing
unit cases or add an unbounded performance workload.

## Boundaries for the follow-up work

- The table marks missing workloads/configurations, not failed executions.
- GAP marks are proposed priorities, not proof that every blank is dispensable.
  Multiple cells can be addressed by one fixture or workload improvement.
- Classic drafting is separate from MTP: it requires one slot and occupies
  MTP-off rows. Classic itself must be enabled; MTP off does not mean all
  speculation is disabled. Multi-slot and MTP-on combinations remain `not apply`
  in that column.
- Some model choices require capability qualification. A routing/state label
  alone does not prove MTP, classic drafting, or media support.
- Keep existing assertions, including batch's per-turn all-slot generation
  overlap. Image batch intentionally does not require whole-output equality.
- Multi-GPU remains phase 2. Its applicable empty cells have not been promoted
  to current-phase GAPs.
- Combine short state checks, representative sustained load and selected context
  stages in the same command; do not require manual multi-step execution.

## Sources

- [Tool defaults and profile targets](../../.make/tools.mk)
- [Current model fixtures](model_config_tools.yaml)
- [MTP workload](scenario_mtp.go) and [backend evidence validation](main.go)
- [Same-slot state workload](scenario_hybrid.go)
- [Context workload](scenario_long_context.go)
- [Batch workload](scenario_batch.go) and [media workloads](scenario_media.go)
- [Shared mode checks](scenario_common.go)
- [Lifecycle workload](../lifecycle-load/main.go)
- [Catalog model candidates](../../sdk/tools/defaults/yaml/catalog.yaml)
- [SDK fixture selections and classic pair](../../sdk/kronk/tests/testlib/testlib.go)
- [Classic SDK smoke suite](../../sdk/kronk/tests/draft/suite_test.go), [skip conditions](../../sdk/kronk/tests/draft/main_test.go), and [response/retry assertions](../../sdk/kronk/tests/testlib/helpers.go)
- [Classic algorithm tests](../../sdk/kronk/model/internal/speculation/classic/classic_test.go) and [engine boundary/replay tests](../../sdk/kronk/model/batchgen_speculative_test.go)
- [Input-mixing capabilities](../../sdk/kronk/modelprofile/batch.go) and [shared/isolated scheduling](../../sdk/kronk/model/batchgen_plan.go)
- [Hybrid/attention rollback selection](../../sdk/kronk/model/speculation_runtime.go)
- [Classic drafter compatibility requirements](../../.manual/chapter-06-speculative-decoding-mtp.md#631-classic-separate-draft)
- [Classic configuration restrictions](../../sdk/kronk/model/config.go)
- [Speculation mode selection](../../sdk/kronk/model/internal/speculation/speculation.go)
