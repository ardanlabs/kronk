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
Retired profile names `hybrid`, `media-correctness` and `media-prefill` are not
aliases for `text-state`, `image-state` and `image-prefill`.

The presentation and `all` profile order is text-state, image-state,
long-context, image-prefill, batch-text, batch-images, mtp-embedded,
mtp-companion, followed by the separate lifecycle tool in the CSV.
Explicit profile lists retain the user's chosen execution order.
The combined image Make target is `test-load-images`, not `test-load-media`.
Make fixture overrides use `RELIABILITY_TEXT_STATE_MODEL` and
`RELIABILITY_IMAGE_*` names. These fixture overrides are for low-level custom
runs, not profile × cell selection.

The low-level scenarios are `text-state`, `images`, `long-context`, `batch` and
`mtp`. `images` accepts `-image-profile state`, `prefill` or `all`.
Retired scenario names `hybrid-state` / `media` and CLI flags `-hybrid-model` /
`-media-*` are not aliases. Use `-text-state-model`, `-image-model`,
`-image-slots`, `-image-file`, `-image-expect`, `-image-state-max-tokens`,
`-image-prefill-generation-max-tokens`, `-image-prefill-max-tokens`,
`-image-prefill-max-generation-gap` and `-image-require-isolated-prefill`.
Historical output captures, JSON detail keys and server diagnostic event names
retain their existing vocabulary; they are not invocation syntax. Prompt markers
are also unchanged so this naming cleanup does not change the inference task.

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
| `image-prefill` | Submit image after five text content events; both streams complete, scheduler-observed prefill/generation overlap and text gaps within one second by default. Require request-correlated image execution evidence mapped below. | Two concurrent requests, not all-slot saturation. No image-subject, replay/append or cancellation contract. Client content-event gaps are not server per-token timings. |
| `batch-text` | Eight conversations on four slots, or four on two slots for the Dense Hybrid alternative; at least 21 growing-history turns and 30K final prompts. Session/turn canaries, isolation and IMC-on append reuse. Per-turn full slot ownership and prepared-request wait/release coverage; workload-wide generation-during-prefill evidence as defined below. | No dedicated exact replay, cancellation/recovery or images. Slot occupancy is not generation overlap. Revised contract qualified only for the cells below. |
| `batch-images` | Same eight-conversation batch history/load/isolation and scheduling contract, plus image subjects, later subject switch, initial/switch request replays, current-subject correctness and rejection of stale/foreign subjects. IMC-on replay/append reuse remains required. | No whole-description equality: differing correct prose is allowed. No cancellation or forced same-slot recovery. Image history load does not replace prefill responsiveness. Revised contract qualified only for the cell below; duplication remains unresolved. |
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

Across the workload, successive request-correlated diagnostics samples must show
`generated_tokens` increasing for one request while a different, consistently
identified request remains in prefill across those samples (text, IMC preparation
or media prefill). This progress witness is not required on every turn or replay.
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

### Batch contract qualification

The tooling-contract correction is validated on both text and image paths by
the following November 6, 2026 captures. All use the Hybrid MoE Qwen3.6 fixture,
eight conversations and four slots; each reports behavioral PASS, scheduling
coverage OBSERVED and successful cleanup. Server metadata records build
`1.33.2-rc`, revision `56082b8050aa49cbdb1adf250d67c5fe887bcdf9`, Metal and
libraries `b12125`. This is bounded qualification, not a matrix-wide clearance.

| Profile × cell | Completed workload | Capture under `output/` |
| --- | --- | --- |
| `batch-text × MoE/Hyb/Nslot/IMC-/MTP+/img-` | 21 turns; 168 requests; 31,372-token final prompts | `fetched-batch-text-mtp-on-revalidation-20261106-075952-20261106-105101` |
| `batch-text × MoE/Hyb/Nslot/IMC-/MTP-/img-` | 21 turns; 168 requests; 31,372-token final prompts | `fetched-batch-text-contract-20261106-090438-20261106-131726` |
| `batch-text × MoE/Hyb/Nslot/IMC+/MTP+/img-` | 21 turns; 168 requests; 31,372-token final prompts | `fetched-batch-text-contract-20261106-090438-20261106-131726` |
| `batch-images × MoE/Hyb/Nslot/IMC+/MTP+/img+` | 21 turns plus initial/changed-image replays; 184 requests; final reported prompts 46,327–46,356 tokens | `fetched-batch-images-probe-revalidation-20261106-113046-20261106-134051` |

Retained lifecycle events independently establish four occupied slots without
ownership conflicts and waiting-job handoff in every recorded phase. Successive
request-correlated scheduler samples establish generation during peer prefill.
Text IMC-off/MTP-on client overlap fell to two on later turns; image overlap fell
to two at the turn-11 subject switch. Both completed with scheduling evidence,
validating that low client overlap no longer aborts the history exercise.
The image replays had no sampled generation-during-prefill witness; the required
workload-wide witness was observed in other phases, including the subject switch.

Do not extend this qualification to text IMC-on/MTP-off, Attention-only batch,
or the other image modes. The interrupted image-pair capture
`fetched-batch-images-contract-20261106-094431-20261106-131805` retained only
`tool.log`: IMC-off/MTP-on logged completion and cleanup, but no final summary or
raw events survived, so its scheduling qualification is not established.

The correction does not resolve server efficiency concerns. Image turn-11 usage
again exceeded the earlier IMC-off control by 13,700 prompt tokens; the historical
physical history-duplication defect remains open. This PASS does not assert that
cached history was decoded only once. Text MTP-on/IMC-off took 5.8% longer than
MTP-off/IMC-off over matched 21-turn workloads in single sequential captures;
that observation is not a repeated benchmark, version regression or causal
attribution. Preserve historical FAILs under their original contracts.

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
| `mtp-companion`, Hybrid Dense | `unsloth/Qwen3.8-27B-UD-Q6_K` | 1 or 2; 128K; companion drafter when MTP on; qualification pending |
| `text-state`, `long-context`, Hybrid MoE | `unsloth/Qwen3.8-Flash-Next-UD-Q2_K_XL` | 1; context stages through 128K |
| `text-state`, Hybrid Dense alternative | Qwen3.8-27B above | 1; 128K; three recovery cycles; all MTP/IMC modes selectable, qualification pending |
| `text-state`, `long-context`, Attention-only Dense | `unsloth/Qwen3-0.6B-Q8_0` | 1; MTP off; 32K configured context, retrieval through 16K |
| `batch-text`, `batch-images`, Hybrid MoE | Qwen3.6 above | 4; 128K; original growth settings |
| `batch-text`, Hybrid Dense alternative | Qwen3.8-27B above | 2; 128K; four conversations, 21 turns / 30K target; all MTP/IMC modes selectable, qualification pending |
| `batch-text`, Attention-only Dense | `ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0` | 4; MTP off; 64K per sequence, 1,300 tokens/turn, 128 completion budget |
| `image-state`, Hybrid | Qwen3.6 MoE / Qwen3.8-27B Dense above | 1 or 4 / 1 or 2 respectively |
| `image-state`, Attention-only Dense | `unsloth/gemma-4-E4B-it-qat-UD-Q4_K_XL` | 1 or 2; 32K; MTP off; both IMC modes qualified |
| `image-prefill`, Hybrid | Qwen3.6 MoE / Qwen3.8-27B Dense above | Per phase below |
| `image-prefill`, Attention-only Dense | Qwen2.5-VL above | 2; 32K; MTP off |
| Lifecycle | Qwen3-0.6B above | 1; IMC on, no configured MTP; modes not asserted by lifecycle |

The Attention-only batch fixture was historically qualified for five conversations,
21 turns, 105 requests and
30,163-token final prompts in both IMC modes, including four-way overlap and
reuse when enabled (`output/qwen25vl-batch-imc`). This does not qualify the revised
eight-conversation workload or its scheduling evidence contract for this
Attention-only fixture. Hybrid MoE qualification is listed above. Qwen3-0.6B failed the batch
marker-count task and is retained only for its smaller state/context/lifecycle
contracts. Image-state now selects Gemma E4B QAT, while text batch and prefill
retain Qwen2.5-VL. Qwen2.5-VL previously failed image-state prefix assertions;
Ministral passed IMC-off image-state but fell back to uncached execution with
IMC on. Gemma qualifies all four image-state combinations (CSV 2:6/22/38/54)
in `output/fetched-gemma4-image-state-20261106-115711-20261106-140249`:
three cycles each, image/text correctness, cancellation slot release, recovery
and successful unload. Repeat and append requests reuse 174 tokens in every
IMC-on cycle; IMC-off controls report zero reuse. The many-slot fixture has two
slots. Server build `1.33.2-rc`, revision
`7986f0b2948ca5d772007a4cd47f9d29f8292e85`, Metal libraries `b12125`.
This closes the selected image-state fixture/reuse gaps without establishing
concurrent image-session behavior or fixing Ministral's template limitation.

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

Every phase retains completion, overlap and bounded-text-progress assertions.
The forced aliases set prefill contribution limit 32 and image decode capacity
2,048; qualification observed 345-token image chunks. Shared-capable models can
isolate oversized whole chunks, so a separate Gemma fixture is unnecessary for
this selected objective. Capacity alone does not establish execution; require
the measured request's successful encoding/preparation. A cache hit alone does
not establish cold image preparation. Evidence adapters may change with logging
without changing the behavioral contract.

These mappings have inference qualification, including shared/forced phases
41/57 (`output/prefill-mapping-20261104-135745`) and Attention-only 38/54.
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
modes are mapped for each profile. These eight pairs await inference
qualification; their overlap labels retain the representative-coverage rationale,
not a passing result on the alternate. The four duplicate MTP-off references
remain unmapped; select the corresponding mapped control profile instead.

An investigation-package reference and its companion skill should choose the
appropriate behavior and fixture for the reported situation, using this document
for the overlap rationale and the actual selector for availability. They should
prefer an available representative for common behavior, retain the suspect
fixture/backend when that distinction matters, and expose missing mappings or
qualification rather than silently substituting. An overlap label is neither
an instruction to always skip that path nor permission to run an unsupported
combination. Changes to coverage or mappings must also update that guidance.

## Remaining qualification and limitations

The four Gemma image-state combinations are qualified; no image-state fixture
or reuse GAP remains in the selected coverage. The eight Dense Hybrid
text-state/batch-text alternatives are mapped but still need qualification, as
explained above. Revised batch qualification remains limited to the four
profile × cells listed in the batch qualification section; other batch modes
and the Attention-only batch fixture do not inherit those passing results.

Historical batch FAILs remain historical
FAILs, including the image IMC/MTP-on capture; removing the universal client
overlap requirement does not retroactively pass them. Image duplication remains
unresolved, as does the text MTP timing concern. Revised batch qualification is
limited to the four profile × cells listed above. Other selected expansion contracts are qualified; this
does not prove all blank combinations unnecessary. Additional local chronology
may be retained in the gitignored `development-findings.md`, but is not required
to understand these limitations.

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
