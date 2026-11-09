# Reliability coverage reference

[coverage.csv](coverage.csv) is the quick viewer of profile × configuration
coverage. This document explains its labels, selected fixtures, bounded probe
contracts and representative-coverage decisions. It is not a run schedule or a
development roadmap. Local deferred investigations can be kept in the gitignored
`development-findings.md`; that file is not required to use this reference.

These are selectable **pressure probes** for suspected regressions, significant
code changes and bug investigations. They need not reproduce quick-check's
model or workload. A selected PASS establishes its executed contract, not the
correctness of the entire codebase.

Use the [investigation guide](investigations.md) to choose packages, controls
and commands for those situations. This document explains coverage; that guide
explains how to use it.

## Reading the matrix

Each row identifies GPU count, slots, IMC, MTP, image input, routing and state.
Dense/MoE routing and Hybrid/Attention-only state are independent properties.
The CSV calls Attention-only state `Pure`. Media here means images, not audio
or video. `Nslot` means the profile's configured many-slot fixture, not an
arbitrary slot count.

The Model ID is a row-level fixture hint, not a universal model assignment to
every profile in that row. The per-profile fixture table below resolves those
differences. Routing/state labels alone do not imply image or drafting support.

| Label | Meaning |
| --- | --- |
| `covered` | A selected fixture/workload supplies the bounded profile contract below. Not exhaustive coverage or a passing-run verdict. |
| Empty | No required coverage selected for this exact pair. Not proof that it is unnecessary or runnable. |
| `not apply` | Outside this profile's input/slot contract. |
| `overlaped by T:L` | Common behavior is represented by tool column T, CSV line L, including the header. Not fixture equivalence, a redirect or a ban on investigating this pair. |
| `GAP: …` | Important coverage is unresolved. A configured probe may still be runnable and fail qualification. |
| `covered; GAP: …` | The bounded contract exists, but a selected additional behavior/path is missing. |
| `need other model` | Fixture selection is unresolved; an existing model with a suitable alias may suffice. |

Tool numbers and line references are documentation references only; keep them
valid when changing CSV order or columns. Invocation uses explicit identities,
not numbers. Single-GPU coverage is the current scope. Multi-GPU rows are
deferred, and physical GPU placement is not independently proven by these tools.
Classic full-draft speculation is excluded. Lifecycle remains separate from
the eight reliability profiles; SDK tests run independently.

## Selecting a profile × configuration

Both selectors are required: choose profiles and cells, or explicitly use `all`
on either axis. Profiles are comma-separated; cells are whitespace-separated,
written one per line. Case-sensitive cell syntax is:

`Dense|MoE` / `Hyb|Att` / `1slot|Nslot` / `IMC-|IMC+` / `MTP-|MTP+` / `img-|img+`

There is no GPU component. Cell `all` selects the 64 single-GPU configurations,
including unavailable pairs that must be reported. The selection is the cross
product of requested profiles and cells; use separate invocations when different
profiles need different cell sets.

```sh copy Copy Attention Batch Command
make test-load-all RELIABILITY_PROFILES=batch-text \
  RELIABILITY_CELLS='
    Dense/Att/Nslot/IMC-/MTP-/img-
    Dense/Att/Nslot/IMC+/MTP-/img-
  '
```

`test-load-<profile>` supplies that profile selection. The CLI equivalents are
`-profiles` and `-cells`; `-list` lists names and identities without a server.

The presentation and `all` profile order is text-state, image-state,
long-context, image-prefill, batch-text, batch-images, mtp-embedded,
mtp-companion, followed by the separate lifecycle tool in the CSV.
Explicit profile lists retain the user's chosen execution order.
The combined image Make target is `test-load-images`.
Make fixture overrides use `RELIABILITY_TEXT_STATE_MODEL` and
`RELIABILITY_IMAGE_*` names. These fixture overrides are for low-level custom
runs, not profile × cell selection.

The low-level scenarios are `text-state`, `images`, `long-context`, `batch` and
`mtp`. `images` accepts `-image-profile state`, `prefill` or `all`.
Use `-text-state-model`, `-image-model`,
`-image-slots`, `-image-file`, `-image-expect`, `-image-state-max-tokens`,
`-image-prefill-generation-max-tokens`, `-image-prefill-max-tokens`,
`-image-prefill-max-generation-gap` and `-image-require-isolated-prefill`.

The selection display shows only resulting profile × cell/status pairs:
`runnable`, `not apply` or `coverage gap`. Nothing is silently omitted or
redirected to an overlap representative. No runnable pair means NOT RUN, not
PASS. Missing requested coverage returns a nonzero exit. Runnable means a
mapping exists, not that the fixture has passed qualification. The selector
does not interpret CSV labels; [selection.go](selection.go) owns actual mappings.

Normal selections resolve fixtures, pressure defaults and required prefill
phases automatically. Deliberate pressure/evidence overrides remain available
through `RELIABILITY_ARGS`. Fixture overrides require the low-level interface
with explicit scenario and modes instead of profile × cell selectors.

## Probe contracts and limits

All selected fixtures must match the requested state, loaded modes and slot
configuration; image profiles require a projection. Completed requests require
positive prompt/completion usage. IMC off requires zero cached tokens. MTP off
requires zero draft usage; MTP on requires drafting rather than silent fallback,
with usage/coverage checks appropriate to each workload. Cache-hit requirements
belong to explicit repeat/append workloads, not every IMC-on request.

Behavior assertions concern answers, isolation, reuse, completion and progress.
Backend names, cleanup messages and rollback strategy names are diagnostics,
not correctness contracts. Explicit path objectives are reported separately as
OBSERVED or INCOMPLETE. Missing logs or an untriggered path cannot prove coverage
but do not alone make a correct answer a behavioral failure. Required incomplete
coverage still returns nonzero. Timing reports alone are not performance
regression verdicts; those need comparable baselines and criteria.

| Profile | What covered cells exercise | Important limits / complementary probes |
| --- | --- | --- |
| `text-state` | One slot; three cold/repeat/append/cancel/recovery cycles without unloading. Exact task answers, new append marker, IMC-on exact/append reuse, cancellation after content and clean recovery without the cancelled marker. | Same execution capacity, not concurrent-session isolation. No images, prefill cancellation or independently identified rollback strategy. Use batch for sustained histories. |
| `image-state` | Three image/text/replay/append/cancel/recovery cycles without unloading. Subject and requested prefixes, text exact-marker isolation, IMC-on reuse, cancellation after content, all slots idle and fresh image/text recovery. MTP-on measured image requests require drafting. | One-slot fixtures force same-slot recovery; many-slot fixtures do not pin recovery to the cancelled slot. No concurrent image sessions, subject-switch history or prefill cancellation. No byte-identical prose requirement. |
| `long-context` | Serial staged retrieval of three ordered markers, rejecting foreign-stage markers; cold/repeat correctness and IMC-on warm reuse. Flash-Next stages 4K–128K; Attention-only baseline 4K/8K/16K. Unknown capacity or skipped requested stages cannot pass. | No exact-limit/overflow, append, cancellation, images or concurrent long-context interference. Marker retrieval is not general model-quality evaluation. |
| `image-prefill` | Submit image after five text content events; both streams complete and scheduler-observed prefill/generation overlap. Text gaps above one second produce a UX warning; only an explicitly supplied gap limit is enforced. Require request-correlated image execution evidence mapped below. | Two concurrent requests, not all-slot saturation. No image-subject, replay/append or cancellation contract. Client content-event gaps are not server per-token timings. |
| `batch-text` | Eight conversations on four slots, or four on two slots for the Dense Hybrid alternative; at least 21 growing-history turns and 30K final prompts. Session/turn canaries, isolation and IMC-on append reuse. Per-turn full slot ownership and prepared-request wait/release coverage; generation-during-prefill is a warning-only diagnostic as defined below. | No dedicated exact replay, cancellation/recovery or images. Slot occupancy is not generation overlap. |
| `batch-images` | Same eight-conversation batch history/load/isolation and scheduling contract, plus image subjects, later subject switch, initial/switch request replays, current-subject correctness and rejection of stale/foreign subjects. IMC-on replay/append reuse remains required. | No whole-description equality: differing correct prose is allowed. No cancellation or forced same-slot recovery. Image history load does not replace prefill responsiveness or prove absence of duplicated decoding. |
| `mtp-embedded` / `mtp-companion` | Embedded-head or companion fixture; three request waves on the same loaded model, supplied-value correctness, own session key and no foreign key, normal stop, selected drafting mode, then fresh-marker recovery. One/many-slot fixtures are distinct. | No appended-history cache-hit requirement, images, target-only output equivalence or proof that all physical slots generated concurrently. No inherent minimum rejection count or acceptance rate. |
| `example-lifecycle-load` | Separate command: one execution slot, two admission permits, queued cancellation, admission timeout, active cancellation and successful recovery; structured stage evidence. | Fixed text fixture; no IMC/MTP matrix or cache/draft/media integrity assertions. Admission recovery does not establish inference-state integrity. |

Three cycles/waves are a bounded pressure budget, not a proven stability
threshold. Whole-run repetitions default to one and reload models; cycles/waves
retain the loaded context. The low-level interface retains single-cycle/wave
defaults. Optional `-mtp-min-rollback-rounds` requests correlated rejection/
recovery observations, not a universal MTP behavior requirement. Too few means
incomplete requested coverage. Observing one strategy does not prove all others.

### Batch scheduling evidence

Text and image batch profiles use eight conversations on four slots, except
Dense Hybrid text batch, which uses four conversations on two slots. Both use
twice as many conversations as execution slots. With `queue-depth: 2`, admission
capacity is eight or four requests respectively; this does **not** guarantee
simultaneous admission of every request. Correctness still requires the
21-turn/30K histories, answers, isolation and applicable cache assertions above.

Scheduling coverage is separate from behavioral PASS/FAIL. For each turn,
request-correlated server evidence must establish both:

- Concurrent ownership of all configured slots, reconstructed from physical
  slot assignment through execution finish, with no same-slot ownership conflicts.
- A prepared request waiting under full slot pressure, then taking a released
  slot and completing. Client response timing does not establish queue acceptance.

Across the workload, the runner looks for successive request-correlated
diagnostics samples showing `generated_tokens` increasing for one request while a different, consistently
identified request remains in prefill across those samples (text, IMC preparation
or media prefill). This progress witness is diagnostic, not required coverage.
If absent, it is reported as `warning` in the coverage entries and as `WARNING`
in the tool log, without making coverage INCOMPLETE or causing a nonzero exit.
Short prefills may complete between samples; absence does not establish a stall.
`coverage_status: OBSERVED` means required paths were observed, not that every
diagnostic witness was obtained. Inspect warnings separately.
A single text prefill owner is legal while other slots generate; four occupied
slots are not four generation streams.

`maximum_concurrency` remains a diagnostic of overlapping client response
intervals, not server slot ownership or simultaneous token generation. There is
no client-inferred queue metric or universal all-four client-stream overlap
requirement. Low response overlap must not truncate growing histories or replays.
Retain request-correlated queue entry, slot assignment, server queue wait and
execution timelines; do not subtract client/server clocks across machines.
Missing, untriggered or contradictory required evidence means coverage
INCOMPLETE and a nonzero exit, separately from behavioral PASS/FAIL. Contradictory
ownership evidence cannot establish coverage through an aggregate.
Failed image-state cycles retain response outputs without changing assertions.

## Current qualification

Qualification applies to the selected fixture, modes, capacity and bounded
contract, not the entire model or codebase. `IMC±` / `MTP±` means both choices;
expand them into explicit identities in commands.

| Profile | Qualified configuration families |
| --- | --- |
| `text-state`, `long-context` | Selected MoE Hybrid one-slot modes and Dense Attention-only one-slot MTP-off modes. |
| `image-state` | MoE Hybrid one/many-slot modes; Dense Hybrid many-slot Q6 modes; Dense Attention-only one/many-slot Gemma modes with MTP off. Both IMC choices are qualified. |
| `image-prefill` | Selected MoE Hybrid and Dense Attention-only many-slot modes, including required automatic phases below. Dense Hybrid alternatives are not selected matrix coverage. |
| `batch-text` | `MoE/Hyb/Nslot/IMC±/MTP±/img-` and `Dense/Att/Nslot/IMC±/MTP-/img-`; full eight-conversation histories and required scheduling evidence. |
| `batch-images` | `MoE/Hyb/Nslot/IMC±/MTP±/img+`; full histories, image-switch/replay phases and required scheduling evidence. History duplication remains a separate limitation. |
| `mtp-embedded` | Selected MoE Hybrid one/many-slot modes. |
| `mtp-companion` | `Dense/Hyb/1slot/IMC±/MTP±/img-` and `Dense/Hyb/Nslot/IMC±/MTP±/img-` on Q6; three waves and recovery. |

The Dense Hybrid `batch-text` alternative is also qualified for
`Dense/Hyb/Nslot/IMC+/MTP+/img-` (four conversations / two slots).
Other runnable alternatives do not inherit qualification from these families.
See the overlap and limitation sections for paths outside selected coverage.

## Selected fixtures

Model IDs below omit mode-specific alias suffixes. The shared
[tools model configuration](../../zarf/kms/tools_model_config.yaml) defines exact aliases
for quick-check and reliability. `make server-for-tools` loads this file;
normal server targets retain `zarf/kms/model_config.yaml` and its `/AGENT` entries.
Image fixtures resolve projections from the catalog. MTP-off variants are
target-only controls, not drafting coverage.

| Profiles / state | Model | Slots / configuration |
| --- | --- | --- |
| `mtp-embedded`, Hybrid MoE | `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL` | 1 or 4; 128K |
| `mtp-companion`, Hybrid Dense | `unsloth/Qwen3.8-27B-UD-Q6_K` | 1 or 2; 128K; companion drafter when MTP on |
| `text-state`, `long-context`, Hybrid MoE | `unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL` | 1; context stages through 128K |
| `text-state`, Hybrid Dense alternative | Qwen3.8-27B above | 1; 128K; three recovery cycles; all MTP/IMC modes selectable, qualification pending |
| `text-state`, `long-context`, Attention-only Dense | `unsloth/Qwen3-0.6B-Q8_0` | 1; MTP off; 32K configured context, retrieval through 16K |
| `batch-text`, `batch-images`, Hybrid MoE | Qwen3.6 above | 4; 128K; eight conversations, 21 turns / 30K target |
| `batch-text`, Hybrid Dense alternative | Qwen3.8-27B above | 2; 128K; four conversations, 21 turns / 30K target; IMC+/MTP+ qualified, other modes pending |
| `batch-text`, Attention-only Dense | `ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0` | 4; MTP off; 64K per sequence, 1,300 tokens/turn, 128 completion budget |
| `image-state`, Hybrid | Qwen3.6 MoE / Qwen3.8-27B Dense above | 1 or 4 / 1 or 2 respectively |
| `image-state`, Attention-only Dense | `unsloth/gemma-4-E4B-it-qat-UD-Q4_K_XL` | 1 or 2; 32K; MTP off; both IMC modes qualified |
| `image-prefill`, Hybrid | Qwen3.6 MoE / Qwen3.8-27B Dense above | Per phase below |
| `image-prefill`, Attention-only Dense | Qwen2.5-VL above | 2; 32K; MTP off |
| Lifecycle | Qwen3-0.6B above | 1; IMC on, no configured MTP; modes not asserted by lifecycle |

Attention-only image-state uses Gemma E4B QAT; batch and prefill use Qwen2.5-VL.
They supply different contracts and are not interchangeable merely because both
accept images. Qwen3-0.6B is the small text-state/context/lifecycle fixture.

## Image-prefill timing policy

A maximum text content-event gap above one second overlapping image submission
through first image content produces a `warning` entry and a `WARNING` in the
tool log. It does not fail the run or make required coverage INCOMPLETE. One
second is a [general UX flow-interruption heuristic](https://www.nngroup.com/articles/response-times-3-important-limits/),
not a calibrated Kronk performance-regression limit or an LLM streaming standard.
The full intersecting event gap is measured, not just its portion inside that
window; it does not isolate image-encoding duration.

An explicitly supplied positive `-image-prefill-max-generation-gap` duration
enables a hard limit, including an explicit `1s`. Make's
`RELIABILITY_IMAGE_PREFILL_MAX_GENERATION_GAP` is empty by default and opts into
the same hard limit when set, for both selected cells and low-level scenarios.
The one-second UX warning remains independent of that limit. Choose an enforced
budget from the deployment's latency requirements and comparable baselines.
Stream errors, missing content, unmeasurable overlap and required execution
evidence retain their existing checks.

Concurrent-prefill details record `generation_gap_enforced`, the configured
`allowed_maximum_gap_seconds` and measured gaps in either mode.

## Image-prefill execution mapping

All entries are tool 4, many-slot image configurations. Numbers are CSV lines,
not invocation syntax. Selected MTP/IMC modes remain unchanged in every phase.

| Lines | Fixture / automatic phases | Required request-correlated execution |
| --- | --- | --- |
| 38 | Qwen2.5-VL, 2 slots, IMC off | Native isolated image encoding; no oversized-chunk requirement. |
| 39, 47 | Qwen3.8-27B, 2 slots, IMC off | Shared image encoding; active drafting for 47. |
| 41 | Qwen3.6 ordinary 4-slot phase, then `TOOLS-ISOLATED` 2-slot phase | Shared encoding, then isolated encoding with image chunk strictly above the reported contribution limit. Both phases required. |
| 49 | Qwen3.6, 4 slots, IMC off | Shared encoding with active drafting. |
| 54 | Qwen2.5-VL, 2 slots, IMC on | Cold IMC whole-image preparation, not shared-tray execution. |
| 55, 63 | Qwen3.8-27B, 2 slots, IMC on | Cold IMC whole-image preparation; active drafting for 63. |
| 57 | Qwen3.6 `TOOLS-ISOLATED`, 2 slots, IMC on | IMC whole-image preparation strictly above the contribution limit; not evidence of the non-IMC isolated scheduler branch. |
| 65 | Qwen3.6, 4 slots, IMC on | Cold IMC whole-image preparation with active drafting. |

Every phase retains completion and overlap assertions and the timing policy above.
The forced aliases set prefill contribution limit 32 and image decode capacity
2,048. Shared-capable models can
isolate oversized whole chunks, so a separate Gemma fixture is unnecessary for
this selected objective. Capacity alone does not establish execution; require
the measured request's successful encoding/preparation. A cache hit alone does
not establish cold image preparation. Evidence adapters may change with logging
without changing the behavioral contract.

The runner unloads between phases and keeps their performance measurements
separate. Ordinary failures do not skip subsequent phases; cleanup failures
stop execution. One-slot cells do not apply to this concurrent profile.

## Why these representative cells

- Hybrid recurrent/attention memory and ordinary Attention-only memory warrant
  separate reuse, recovery, context and isolation checks. Hybrid's attention
  layers do not replace an Attention-only baseline.
- Embedded and companion drafting have distinct loading/state paths. One and
  many slots, IMC off/on and image/text inputs also expose different behavior.
- State transitions, growing histories, retrieval and prefill responsiveness
  stress different risks; no one profile replaces all the others.
- Dense/MoE labels alone do not justify duplicating every common Kronk
  orchestration check. Expert offload/cache behavior would require its own
  configuration and assertions, not merely another routing label.
- The blanks are unselected combinations, not a mathematically proven minimum.
  A concrete code change, failure or investigation may justify selecting one.

### Overlaps: representative coverage, not interchangeable fixtures

| Overlap → representative (tool:line) | Scope of the overlap |
| --- | --- |
| 1:3/11/19/27 → 1:5/13/21/29 respectively | Common Hybrid text-state orchestration, represented by MoE rather than Dense. |
| 5:35/43/51/59 → 5:37/45/53/61 respectively | Common Hybrid sustained text orchestration, represented by MoE rather than Dense. |
| 7:35 → 8:35; 8:37 → 7:37 | MTP-off concurrent text control, IMC off; same target and configuration. |
| 7:51 → 8:51; 8:53 → 7:53 | Same target-only control, IMC on. |

The four MTP-profile references duplicate a workload: with MTP off, both dedicated
profiles call the same target-only probe without a backend-specific assertion. Changing the
profile label provides no additional behavior on that same fixture.

The other eight are **representative substitutions**, not equivalence. A Dense
Hybrid state/batch fixture may be preferable when investigating that target,
its native execution, or its companion interaction. The MoE run does not prove
that Dense fixture works. Conversely, a Dense label alone need not trigger a
second investigation of common state/batch behavior.

**Availability:** the eight Dense Hybrid alternatives are directly selectable:
`text-state` uses Qwen3.8-27B one-slot aliases and three recovery cycles;
`batch-text` uses its two-slot aliases and four conversations, retaining the
21-turn/30K histories and current scheduling-evidence contract. All four MTP/IMC
modes are mapped for each profile. Only the IMC+/MTP+ batch alternative is
qualified; the other seven pairs remain pending. Overlap labels describe the
representative-coverage rationale, not qualification. The four duplicate MTP-off
references remain unmapped; select the corresponding mapped control instead.

An investigation-package reference and its companion skill should choose the
appropriate behavior and fixture for the reported situation, using this document
for the overlap rationale and the actual selector for availability. They should
prefer an available representative for common behavior, retain the suspect
fixture/backend when that distinction matters, and expose missing mappings or
qualification rather than silently substituting. An overlap label is neither
an instruction to always skip that path nor permission to run an unsupported
combination. Changes to coverage or mappings must also update that guidance.

## Remaining qualification and limitations

- Dense Hybrid text-state alternatives and the three batch-text alternatives
  other than IMC+/MTP+ remain pending; companion/image-state passes do not qualify them.
- IMC image-history duplication remains unresolved. Correct answers, cache usage
  and scheduling PASS do not establish that cached text was decoded only once.
- MTP-on is not a performance guarantee. Compare matched workloads and measured
  phases; acceptance/coverage rates alone do not establish a speedup.
- Many-slot image recovery does not prove concurrent image-session isolation or
  recovery on the cancelled physical slot.
- Gemma image-state qualification does not establish support for templates lacking
  a separate generation tail; such templates can fall back to uncached execution.
- Blank combinations are unselected, not proven unnecessary. Multi-GPU placement
  and classic full-drafter speculation remain outside the selected scope.

## Sources of truth

- [Selection and automatic fixture/phase mapping](selection.go)
- [Make targets/defaults](../../.make/tools.mk) and [tools aliases](../../zarf/kms/tools_model_config.yaml)
- [Shared mode assertions](scenario_common.go) and [MTP workload](scenario_mtp.go)
- [Text-state](scenario_hybrid.go), [context](scenario_long_context.go), [batch](scenario_batch.go) and [image workloads](scenario_media.go)
- [Lifecycle workload](../lifecycle-load/main.go)
- [Input-mixing capabilities](../../sdk/kronk/modelprofile/arch/batch.go) and [scheduler planning](../../sdk/kronk/model/batchgen_plan.go)

Keep CSV labels concise. Update this reference when probe contracts, fixtures,
overlap decisions or required evidence change; keep investigation guidance in
sync in [investigations.md](investigations.md). Execution chronology and
implementation plans do not belong here.
