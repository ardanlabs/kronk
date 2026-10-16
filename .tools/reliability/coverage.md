# Reliability coverage reference

[coverage.csv](coverage.csv) is the quick coverage viewer: combinations, selected
models, short status labels, and gap keywords. This document is the reference for
what those labels mean, what each profile actually checks, and what it leaves
unchecked. Keep detailed explanations here rather than expanding CSV cells.

This is a source-based assessment of the current workloads and default fixtures,
not a record of executions or passing results. Proposed gaps are planning items,
not authorization to implement or run the full matrix.

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

## Important gaps

| Gap keywords | Tool columns | Why important | Model fit / selection needed |
| --- | --- | --- | --- |
| separate-model drafting | classic-drafter | No dedicated classic-drafter profile exists. Target and draft synchronization need their own coverage. | **need other model**: no compatible target/draft pair selected. A selected target alone is insufficient. Qualify tokenizer compatibility and any media/recurrent-state support before assigning a pair. |
| one-slot drafting / one-slot target-only control | mtp-embedded; mtp-companion | One active request on a multi-slot context does not test native one-slot capacities. Cover active drafting and its disabled control without a deep batch run. | Companion rows: **need other model** selected for the one-slot fixture; Qwen3.8-27B is an existing candidate. Embedded rows already select Flash-Next, but the MTP probe's backend evidence validator expects qwen35-own-kv. Adapt that check for its backend or select a one-slot Qwen3.6 fixture; existing coverage must not simply be bypassed. |
| dense hybrid recovery | hybrid | Current one-slot state coverage uses MoE Hybrid. Add dense recurrent-state restore and cancellation/recovery. | **need other model** selected for these rows. Qwen3.8-27B is a Dense Hybrid candidate; its current tools fixture has two slots, not one. |
| attention-only recovery | hybrid | Establish a non-recurrent baseline for deterministic reuse and same-slot cancellation/recovery. The existing assertions are useful beyond hybrid architectures. | The IMC-on row selects Qwen3-0.6B for lifecycle; it fits Dense Attention-only but needs a state-probe fixture. The IMC-off row has **need other model** because no model is selected for that row. |
| attention-only context baseline | long-context | Separate ordinary attention/cache behavior from recurrent-state behavior during staged retrieval. | The selected Qwen3-0.6B fixture has only 8K context, so it can cover the small stages, not the full sweep. The IMC-off row has **need other model**. Select a larger-context model if larger stages become required. |
| attention-only isolation/load | batch-text | Current batch coverage uses MoE Hybrid. Add attention-only Dense and MoE baselines for sequence isolation and scheduling. | **need other model** for both routing types. No matching multi-slot attention-only fixtures are selected. Start with MTP off so the baseline does not depend on speculative capability. |
| dense hybrid isolation/load | batch-text | Companion activation checks do not cover growing histories, canary isolation, oversubscription, or all-slot generation overlap. | Qwen3.8-27B is already selected and fits Dense Hybrid. Add a matching batch fixture; its current two slots differ from the batch default of four. |
| same-slot image recovery | hybrid | Image cancellation/state cleanup must not be hidden by recovery on a spare slot. | **need other model** selected for these rows. Existing media-capable MoE Hybrid models are candidates, but the current same-slot workload has no image requests. A one-slot fixture and image-state assertions are also missing. |
| companion image-state integrity | media-correctness | Image replay/append and cancellation on the embedded model do not qualify the companion model's image-state path. | **need other model** selected for these rows. Qwen3.8-27B has a projection and companion support and is an existing candidate; configure the media profile to use it. MTP-on checks must demonstrate actual image drafting. |
| companion prefill responsiveness | media-prefill | Image prefill must not excessively interrupt text generation on the companion configuration; throughput alone cannot establish this. | **need other model** selected for these rows. Qwen3.8-27B's two-slot fixture is a candidate. Preserve scheduler-observed overlap and bounded text-gap assertions. |

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
- Prefer short qualification/state probes before extending expensive batch or
  maximum-context execution to more fixtures.

## Sources

- [Tool defaults and profile targets](../../.make/tools.mk)
- [Current model fixtures](model_config_tools.yaml)
- [MTP workload](scenario_mtp.go) and [backend evidence validation](main.go)
- [Same-slot state workload](scenario_hybrid.go)
- [Context workload](scenario_long_context.go)
- [Batch workload](scenario_batch.go) and [media workloads](scenario_media.go)
- [Shared mode checks](scenario_common.go)
- [Lifecycle workload](../lifecycle-load/main.go)
- [Classic configuration restrictions](../../sdk/kronk/model/config.go)
- [Speculation mode selection](../../sdk/kronk/model/internal/speculation/speculation.go)
