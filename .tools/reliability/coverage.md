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
context stages and sustained load belong inside the existing `test-load-all`
invocation. One command does not mean every profile runs in every matrix cell.
Lifecycle retains its separate `example-lifecycle-load` command: its one selected
admission/cancellation case does not need matrix expansion or runner integration.

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
whole contract; several selected paths remain GAPs. This document defines extensions
to the existing command, not a replacement command.

## Matrix labels

- `covered`: the current default fixture and workload contain the profile's
  checks described below. It does not mean exhaustive coverage of the cell.
- `not apply`: outside the current profile's contract.
- Empty: uncovered; its priority is not yet decided.
- `overlaped by T:L`: the required behavior already has a `covered` cell in tool
  column T (the number in its header), CSV line L (header is line 1). No separate
  run of this profile is needed for the selected behavior. Cross-routing Hybrid
  references represent common Kronk state/load behavior, not identical native
  computation or fixture-specific backend qualification. Update references if
  lines or tool numbers move.
- `GAP: ...`: an uncovered path proposed as important.
- `covered; GAP: ...`: the current contract exists, but a selected additional
  workload/path is missing inside the same matrix cell.
- `GAP: ...; need other model`: important coverage with an outstanding model
  selection.

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
lifecycle tool does not assert those modes.

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
even when that contract has limitations. Internal gaps are recorded here; selected
additions inside a covered cell use `covered; GAP: ...`. Other `GAP` cells highlight
selected missing combinations/workloads elsewhere in the matrix.
Neither notation automatically commits us to extending every profile. Some
limitations are intentionally covered by another profile instead.

## Confirmed overlaps

| Overlapped cell (tool:line) | Covered source (tool:line) | Reason |
| --- | --- | --- |
| 1:35 | 2:35 | Dense Hybrid target-only control; IMC off. |
| 2:37 | 1:37 | MoE Hybrid target-only control; IMC off. |
| 1:51 | 2:51 | Dense Hybrid target-only control; IMC on. |
| 2:53 | 1:53 | MoE Hybrid target-only control; IMC on. |
| 3:3 | 3:5 | Representative Hybrid state recovery; MTP off, IMC off. |
| 3:11 | 3:13 | Representative Hybrid state recovery; MTP on, IMC off. |
| 3:19 | 3:21 | Representative Hybrid state recovery; MTP off, IMC on. |
| 3:27 | 3:29 | Representative Hybrid state recovery; MTP on, IMC on. |
| 5:35 | 5:37 | Representative Hybrid sustained text load; MTP off, IMC off. |
| 5:43 | 5:45 | Representative Hybrid sustained text load; MTP on, IMC off. |
| 5:51 | 5:53 | Representative Hybrid sustained text load; MTP off, IMC on. |
| 5:59 | 5:61 | Representative Hybrid sustained text load; MTP on, IMC on. |

The first four references are exact workload duplication: both MTP profiles call
the same `runMTPProfile` implementation. With MTP off,
backend/source and draft-KV evidence checks are skipped, and the profile adds no
embedded-versus-companion assertion. Each reference stays on the same model,
slot configuration, media mode, and IMC setting: running the other profile name
on that configuration would repeat its calibrated concurrent text control.
This does not remove the controls on either of the two distinct target models.

The eight state/batch references select representative coverage across Hybrid
routing types. They preserve GPU/slot category, IMC, MTP and media settings while
using the retained MoE Hybrid fixture. They do not assert that the Dense fixture
itself ran, that exact slot counts match within the many-slot category, or that
native expert computation and companion/embedded backends are interchangeable.
Dedicated speculation checks remain selected separately.

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
- **No routing-only Hybrid duplication:** remove Dense Hybrid recovery GAPs at
  3:3/11/19/27 and text-batch GAPs at 5:35/43/51/59. Both Hybrid routing variants
  use the same Kronk state-management/batching branches; a Dense label alone does
  not justify another recovery suite or deep batch run. Keep existing Hybrid
  state/context and sustained text/image workloads. Mark those eight cells
  `overlaped by T:L` using the representative sources above, not `not apply`.
  Native Dense/MoE computation differs, but this extra integration coverage is not
  mandatory in the selected suite. Keep companion/embedded checks for their distinct speculation
  paths, not their routing labels. Expert cache/offload coverage would need its
  own explicit configuration and assertions; duplicating default profiles does
  not establish it.
- **MTP:** retain embedded/companion activation and one-slot GAPs, without
  multiplying them over Attention-only just to populate another category.
- **Media prefill:** remove companion-only prefill GAPs. Both current Qwen
  fixtures have input-mixing support; a companion file alone does not identify
  another scheduler branch. Reuse Qwen3.6 for isolated-prefill workloads with IMC
  off/on and MTP off: cells 8:41/57 now retain `covered` with an internal GAP.
  Remove the dedicated Gemma-driven GAPs at 8:40/56; those cells become blank,
  not overlaps. Shared-capable models can isolate chunks that exceed shared batch
  capacity. Attest actual execution; Gemma remains optional for capability-driven
  isolation specifically.
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
| one-slot drafting / one-slot target-only control | mtp-embedded; mtp-companion | One active request on a multi-slot context does not test native one-slot capacities. Cover active drafting and its disabled control without a deep batch run. | **Reuse** `unsloth/Qwen3.8-27B-UD-Q4_K_XL` for companion rows, configured with one slot. For embedded GAPs, reuse `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL` with one slot, not Flash-Next: Flash-Next uses a companion, not embedded MTP. No new model or Flash-Next backend adaptation is required for this slot-count path. |
| attention-only recovery | hybrid | Establish a non-recurrent baseline for deterministic reuse and same-slot cancellation/recovery. The existing assertions are useful beyond hybrid architectures. | **Reuse** `unsloth/Qwen3-0.6B-Q8_0` for both IMC settings, with a one-slot state-probe fixture. The IMC-off row does not require a different model merely because its Model ID is empty. |
| attention-only context baseline | long-context | Separate ordinary attention/cache behavior from recurrent-state behavior during staged retrieval. | **Reuse** `unsloth/Qwen3-0.6B-Q8_0`: upstream and Unsloth document 32,768-token context; 8K is only our lifecycle configuration. Add a separate fixture and select stages within that budget with output headroom. No full-128K Attention-only fixture is selected. |
| attention-only isolation/load | batch-text | Represent attention-only multi-session state integrity alongside existing Hybrid load coverage; no additional MoE baseline is required by this distinction. | **Reuse candidate** `unsloth/Qwen3-0.6B-Q8_0` with a multi-slot, 32K fixture. The 30K minimum is not a ceiling: qualify actual accumulated history, template overhead and output against 32,768 tokens, plus canary correctness and overlap. Current 8K cannot do this. Alternatives: catalog Qwen3-1.7B-Q4_K_M or Qwen2.5-VL-3B-Instruct-Q8_0; both also have documented 32K context, not extra headroom. Keep MTP off. |
| same-slot image recovery | hybrid | Image cancellation/state cleanup must not be hidden by recovery on a spare slot, for either state representation. | **Hybrid: reuse** `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL` and its projection, with one slot and MTP on/off. **Attention-only: catalog candidate** `ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0` and its projection, with one slot and MTP off. Confirm normalized state classification when selecting this fixture. Add image-state assertions; the current state workload is text-only. |
| companion image-state integrity | media-correctness | Image replay/append and cancellation on the embedded model do not qualify the companion model's image-state path. | **Reuse** `unsloth/Qwen3.8-27B-UD-Q4_K_XL` with its projection and companion. Configure the media profile for this fixture; preserve actual image-drafting assertions when MTP is on. |
| attention-only image state | media-correctness | Represent attention-only image replay/append, text isolation, cancellation and recovery alongside Hybrid state coverage. | **Catalog candidate:** `ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0` and its projection, with a multi-slot fixture and MTP off. Use IMC off/on; state/capability validation belongs inside the eventual suite. |
| isolated media prefill | media-prefill | Exercise scheduling and text responsiveness where media embeddings cannot join a shared decode contribution. | **Reuse** existing Qwen3.6 embedded fixture and projection, at least two slots, MTP off and IMC off/on. Choose chunk size/prefill capacity that forces isolation and attest actual execution. Existing phase-level polling is insufficient. Gemma-4-26B-A4B remains optional only if model-capability-driven isolation itself is selected for model-backed coverage. |

### Latest fixture research and reuse decisions

- **Why retain Attention-only:** Hybrid contains attention layers, but exercises
  combined recurrent/attention memory rather than ordinary Attention-only memory
  in isolation. Keep representative recovery, context and multi-session load
  checks; do not multiply every Hybrid workload across architectures.
- **Qwen3-0.6B:** both the official model and Unsloth GGUF cards document 32,768
  tokens. This establishes advertised capacity, not successful execution of our
  exact long-history workload. Start with reuse; retain the current lifecycle
  configuration and create separate reliability aliases. Qwen3-1.7B-Q4_K_M is
  an existing SDK/catalog alternative, not a larger-context replacement.
- **Attention-only images:** our existing vision fixtures are Hybrid; our current
  Attention-only text fixtures lack vision. Qwen2.5-VL-3B supplies the missing
  combination, not merely another image model. It could also consolidate
  Attention-only text fixtures if qualified. Catalog alternatives include
  Qwen2.5-Omni and Ministral-3 vision models; none is selected here.
- **No mandatory Gemma addition:** Qwen can reach isolated execution when a
  whole image chunk exceeds shared capacity; noncausal contributions also isolate.
  Gemma's unlisted mixing capability takes a different planner condition, but a
  separate model is unnecessary for the selected isolated-execution objective.
  The existing covered prefill cells do not prove either branch without evidence.

These are source/model-card findings, not inference results. Model
suggestions remain here rather than populating CSV Model IDs before selection.

## Boundaries for the follow-up work

- The table marks missing workloads/configurations, not failed executions.
- GAP marks are proposed priorities, not proof that every blank is dispensable.
  Multiple cells can be addressed by one fixture or workload improvement.
- Some model choices require capability qualification. A routing/state label
  alone does not prove MTP or media support.
- Keep existing assertions, including batch's per-turn all-slot generation
  overlap. Image batch intentionally does not require whole-output equality.
- Multi-GPU remains phase 2. Its applicable empty cells have not been promoted
  to current-phase GAPs.
- Combine short state checks, representative sustained load and selected context
  stages in the same command; do not require manual multi-step execution.

## Final review and implementation plan

### Review result

The CSV has 128 unique combinations: 64 single-GPU rows followed by 64 multi-GPU
rows. Its tool cells contain 33 covered contracts and 28 selected GAP markers
(two are inside covered cells), 12 overlap references and 480 `not apply`
markers; 601 cells remain blank. All overlap references resolve to covered cells
with matching GPU/slot categories, IMC, MTP, media and state. Eight intentionally
cross routing types as explained above. These are design counts,
not execution results or the number of distinct workloads required.
Excluding lifecycle, there are 58 distinct selected reliability cells:
32 covered and 28 GAP markers, with two cells belonging to both sets.
This is a scope refinement, not newly implemented coverage or weakened assertions
in the retained workloads.

No new requirement mismatch was identified for the current covered fixtures in
this review. Their bounded assertions and internal gaps remain as described
above. Active image drafting remains a requirement to validate, not a reason to
accept target-only fallback. Blank cells are not automatically dispensable, and
selecting a model does not itself close a GAP.

Dependencies to resolve during development, not through operator follow-up runs:

- An Attention-only fixture supporting the unchanged 30K batch workload plus
  generation. The existing 8K configuration cannot. Required context stages also
  need explicit selection per fixture; running any one small stage is inadequate.
- Request-correlated evidence for required rollback and shared/isolated media
  execution. Optional logs and phase-only polling do not yet prove these paths.
  Single-GPU execution also needs placement evidence, not just available hardware.

### Ordered development steps

These are implementation steps, not separate coverage runs for the operator.
This plan does not authorize implementation by itself.

1. **Freeze the case inventory and fixture contracts.** Map every current covered
   reliability cell and selected single-GPU GAP to a stable case ID, profile, model alias,
   slots, IMC mode, speculation mode, media input and required evidence. Preserve
   the 12 confirmed overlaps without copying their workloads. Resolve the
   dependencies above and qualify the proposed vision fixture and reused Qwen
   isolated-prefill configuration.
   Specify model/projection/drafter identities and context/output budgets. Keep
   unselected blanks and multi-GPU outside the required inventory. Completion:
   every required cell has a feasible fixture and explicit assertions.

2. **Extend selection through the existing `test-load-all`.** Reuse its scenario,
   subprofile and MTP/IMC matrix arguments wherever sufficient. Its current model
   settings select one fixture per role for the invocation, and the same selected
   profiles run across the selected mode pairs. Extend only what the final inventory
   needs: multiple fixtures/slot configurations per role and case-specific mode
   selection that avoids unnecessary cross-products. Do not build another runner
   or command. Keep lifecycle separate and do not invoke SDK suites. Keep individual
   profile commands for diagnosis. Automatically check required configurations.
   Continue independent cases after ordinary failures, but stop safely after
   cleanup/server failure and enumerate unrun cases. Missing fixtures must not
   silently shrink the suite into a PASS.

3. **Validate MTP expectations and establish branch evidence.** Distinguish
   target-only, embedded MTP and companion MTP. Preserve zero-drafting assertions
   for target-only controls. Validate intended backends and request-level activity,
   including active image drafting where selected. Reuse existing evidence where
   sufficient; identify the smallest instrumentation needed where it is not. Missing required logs or
   unobserved required branches cannot be treated as optional evidence.

4. **Extend existing short state, MTP and media profiles.** Add selected one-slot
   embedded/companion active and disabled controls, Attention-only text
   state recovery, same-slot image recovery, companion image correctness and
   Attention-only image correctness. Add isolated-media prefill and attest the
   actual scheduler branch for both selected scheduling paths. Preserve active
   image-drafting, bounded text gaps and recovery assertions. One-slot image-state
   cases must not inherit the prefill profile's two-slot concurrency requirement.

5. **Add representative load/context cases, not another Cartesian product.** Add
   an Attention-only text batch fixture and context baseline; retain existing
   Hybrid sustained workloads without a routing-only duplicate. Preserve batch's
   minimum turns/history, oversubscription,
   every-turn all-slot generation overlap, canaries and reuse assertions. Preserve
   image batch without whole-output equality. Retain deep Hybrid context coverage
   and require explicit stages per fixture. Estimate requests and prefill volume;
   default to one repetition, keeping repetitions configurable. Reduce repeated
   loading only where clean-state preconditions remain intact. Do not weaken
   assertions or remove selected cases to achieve an arbitrary runtime target.

6. **Report completeness and regression evidence together.** Account for planned,
   executed and unrun cases; assertions, branches, missing evidence and required
   stages; fixture/configuration identities, server/native-library provenance and
   GPU placement. Distinguish failures, incomplete coverage and deliberate
   exclusions; none may masquerade as a complete PASS. Retain timings and existing
   performance aggregates. Automated timing-regression verdicts additionally need
   comparable baseline inputs, repetition policy and thresholds; without those,
   report timings as analysis material rather than a performance PASS. Update CSV
   labels only when the documented workload/evidence contracts exist.

7. **Validate the combined command end to end.** Check that missing fixtures,
   wrong modes/backends, absent branch evidence, skipped required stages and
   interrupted cleanup cannot yield complete PASS. Execute the planned single-GPU
   inventory, validating observable continuation, isolation, recovery and report
   completeness. Check individual profile commands retain their assertions.
   Measure total/per-case cost and remove only demonstrated duplication. Document
   the command, prerequisites and artifact interpretation. Multi-GPU remains a
   separate phase requiring real placement and cross-device execution evidence.

Completion means one `test-load-all` invocation accounts for all selected single-GPU
reliability cases and gathers sufficient evidence for analysis. Lifecycle remains
a separate fixed check. A passing report establishes no detected
regressions within that scope, not proof that Kronk has no bugs.

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
- [SDK fixture selections](../../sdk/kronk/tests/testlib/testlib.go)
- [Input-mixing capabilities](../../sdk/kronk/modelprofile/batch.go) and [shared/isolated scheduling](../../sdk/kronk/model/batchgen_plan.go)
- [Hybrid/attention rollback selection](../../sdk/kronk/model/speculation_runtime.go)
- [Speculation mode selection](../../sdk/kronk/model/internal/speculation/speculation.go)
- [Snapshot depth condition](../../sdk/kronk/model/batchgen_engine.go) and [native rollback-depth readback](../../sdk/kronk/model/model.go)
- [Media chunk requirements](../../sdk/kronk/model/batchgen_prefill_media.go)
- [Qwen3-0.6B official capacity](https://huggingface.co/Qwen/Qwen3-0.6B) and [Unsloth GGUF card](https://huggingface.co/unsloth/Qwen3-0.6B-GGUF)
- [Qwen3-1.7B capacity](https://huggingface.co/Qwen/Qwen3-1.7B-GGUF) and [Qwen2.5-VL image/context capabilities](https://huggingface.co/Qwen/Qwen2.5-VL-3B-Instruct)
