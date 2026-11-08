# Reliability investigation guide

Use this guide to choose **pressure probes for a question**, not to schedule the
whole matrix. A package below is a documented selection, not a new runner alias.
Commands use the existing profile × cell interface. Start with the relevant
package or a justified subset; do not run every package by default.
Packages are convenient starting selections, not the boundary of useful tests.
Use [paths outside the package commands](#paths-outside-the-package-commands)
when the investigation calls for a different selection.

[coverage.csv](coverage.csv) shows coverage labels;
[coverage.md](coverage.md) defines contracts, fixtures, execution phases and
limits. [selection.go](selection.go) determines what is actually selectable.
If these disagree, report the discrepancy before promising a run. This guide
does not make an unavailable fixture available or turn a historical FAIL into PASS.

For agent-assisted planning and evidence analysis, use the project skill
[`probing-kronk`](../../.agents/skills/probing-kronk/SKILL.md).
Loading it does not authorize running any Kronk reliability tool.

### Skill access and discovery

Amp sees the skill's name/description and can load it for relevant requests;
users need not name it or add a warning against remote access. The skill defaults
to analysis and proposed commands. Local file reads, code searches and evidence
analysis can proceed as needed for the request. Running any Kronk reliability
tool requires an explicit user request and permission for that execution scope;
the agent must propose unrequested runs and wait. Remote access retains its
separate permission requirement. If probes are merely a possible follow-up,
suggest them without expanding the request.

The skill uses the portable Agent Skills `SKILL.md` format, with no Amp-specific
tools or bundled services required by its workflow. Other compatible harnesses
can use it, but automatic discovery depends on their supported locations and
configuration; the format does not require a universal discovery path. They also
need the referenced repository documents/code and appropriate tools to execute
commands. Skill instructions are not a sandbox: enforce tool/network permissions
in the harness as well when a hard access boundary is required.

## The three entry points

| Why we are investigating | Read first | How to choose and order probes | What the conclusion should answer |
| --- | --- | --- | --- |
| **Quick-check regression signal** | Failed assertion or regressed phase, selected modes, requests/responses, comparison baseline and environment. A slow request and a wrong answer are different signals. | Select the implicated behavior below, then its nearest useful control. Add sustained load or a related state path when it answers the signal. Reliability uses purpose-built fixtures/workloads; reproducing quick-check's exact configuration is not a prerequisite. | Does the suspected feature remain correct under this pressure? Does the signal also affect a related path? A PASS on a different fixture does not clear the original quick-check failure. |
| **Significant feature/code change** | Changed behavior, callers, state ownership and execution branches; include negative/off-mode behavior the change must preserve. | Plan the affected packages and controls up front. Include interactions touched by the change: state family, drafting backend, images, reuse or concurrency. A wider change justifies a wider plan, not automatically `all × all`. | Which changed contracts and adjacent invariants were exercised, and which remain untested? Compare baseline/candidate when making a regression claim. |
| **Bug report or captured failure** | Exact failing phase, trace IDs, workload progress, fixture/modes and relevant code. Read existing artifacts before rerunning. | Prefer the existing workload that exposed the failure. Keep the implicated backend/state/media distinction; use a related control to isolate the hypothesis. Only add a different pressure probe if it tests a stated explanation. | What is observed, what cause is established, and what remains a hypothesis? Has the unchanged failing contract been retested after a fix? |

For a planned code-change assessment, all selected packages can be chosen before
execution; there is no required run/analyze/run gate. For an open-ended bug
investigation, follow-ups depend on evidence rather than a mandatory full suite.
Write the complete initial plan, its order and its cost before asking to execute.

## Choosing the paths, not just the labels

1. **State:** reuse/recovery/context changes warrant Hybrid and Attention-only
   representatives. Hybrid's attention layers do not replace an Attention-only
   state baseline. A narrowly backend-specific bug need not expand to both.
2. **MTP backend:** Dense/MoE is not a backend selector in general. Current
   dedicated fixtures happen to map MoE Hybrid to embedded Qwen3.6 and Dense
   Hybrid to companion Qwen3.8-27B. Do not map a Dense quick-check failure to
   companion drafting merely because it says Dense: read the backend evidence.
3. **Slots:** `1slot` targets serial/same-slot behavior; `Nslot` targets the
   profile's configured capacity (two or four), not an arbitrary number. A
   many-slot recovery PASS does not establish same-slot recovery. Batch adds
   growing histories and oversubscription; it is not a substitute for cancellation.
4. **Controls:** pair IMC on/off for reuse questions and MTP on/off for drafting
   questions. Match the fixture/workload as closely as the mappings allow.
   Changing state family or model supplies related coverage, not a causal A/B.
5. **Images:** state correctness, concurrent prefill responsiveness and growing
   image histories are different contracts. Choose the one implicated by the
   symptom; a correct image description does not prove reuse or bounded progress.
6. **Pressure:** selected state profiles use three cycles and MTP profiles three
   waves; batch reaches 30K histories; context stages are fixture-specific. Do
   not raise repetition counts by habit. Long-context/batch selections can be
   expensive; no fixed duration is promised.

### Overlap choices and availability

Prefer the covered representative for a question about common orchestration.
Prefer the alternate target/backend when the failure or changed code makes that
distinction material. **Do not silently substitute one for the other.**

The eight Dense Hybrid text-state/batch-text overlap alternatives are directly
mapped to Qwen3.8-27B Q6, but still await inference qualification. Text-state uses
one slot and three recovery cycles; batch-text uses four conversations on two
slots, retaining the 21-turn/30K history and scheduling-evidence contract.
Choose them for target-specific recovery, reuse, native execution or companion
interactions, not as proven equivalents to the MoE representatives. Commands
are listed under runnable alternatives below.

The other four overlap references duplicate an available MTP-off control on the
same target/configuration. Choose `mtp-embedded` for the Qwen3.6 control or
`mtp-companion` for the Qwen3.8-27B control; changing the label adds no behavior.
Blank cells also need a source/fixture check before recommendation.

## Package index

Commands follow in the same order. Packages are building blocks, not compulsory
tiers. Off-mode selections deliberately isolate non-drafting behavior; use the
drafting interaction selections when MTP is implicated.

| Package | Useful situations / question | Selection and useful next step | Does not establish |
| --- | --- | --- | --- |
| **Text state** | Wrong/stale answer after reuse, append or cancellation; IMC/state ownership changes | `text-state`, Hybrid + Attention-only, IMC off/on, MTP off. For sustained interference add text load. | Concurrent-session isolation or companion-target state coverage. |
| **Hybrid image state** | Image/text contamination, replay/append reuse, cancellation/recovery; multimodal state changes | `image-state`, same-slot Hybrid MoE, IMC off/on. For companion target use the explicit alternate below. | Image-history pressure or prefill responsiveness. |
| **Attention image state** | Attention-only image recovery or template compatibility | `image-state`, same-slot Attention-only, IMC off/on. Gemma E4B QAT is qualified in both modes and slot configurations. | Ministral template compatibility or concurrent image-session isolation. |
| **Context retrieval** | Lost/foreign markers as history grows; context/IMC changes | `long-context`, Hybrid + Attention-only, IMC off/on. Add text load for simultaneous growing sessions. | Overflow limits or concurrent long-context retrieval. |
| **Hybrid prefill** | Text stalls when an image arrives; shared/isolated preparation changes | `image-prefill`, Hybrid MoE, IMC off/on; includes automatically required forced phases. Companion alternative below. | Subject correctness, cancellation or all-slot saturation. |
| **Attention prefill** | Attention-only image arrival/progress; native-isolated or IMC preparation changes | `image-prefill`, Attention-only, IMC off/on. Uses Qwen2.5-VL, not the Gemma image-state fixture. | Attention image-state reuse. |
| **Hybrid text load** | Queue/stream progress, cross-session contamination, long-running reuse or MTP interactions | `batch-text`, Hybrid MoE, all four MTP/IMC modes. Select the implicated pair first for a narrow bug. | Same-slot recovery or attribution of a slowdown to a single feature by itself. |
| **Attention text load** | Ordinary attention state isolation/reuse under growing concurrent histories | `batch-text`, Attention-only, IMC off/on, MTP off. Qwen2.5-VL has historical five-conversation qualification; revised eight-conversation contract awaits revalidation. | Vision correctness merely because the fixture can accept images. |
| **Image history load** | Subject switch, stale image/history state, growing IMC image prefix or image batch overlap | `batch-images`, Hybrid MoE, all four MTP/IMC modes. IMC/MTP on passed the revised contract; historical FAIL and history duplication remain separate evidence. | Recovery after cancellation, absence of duplicated decoding or universal scheduling guarantees. |
| **Embedded drafting** | Drafting stops, corrupts values/session identity or fails recovery; embedded MTP changes | `mtp-embedded`, many slots, MTP off/on × IMC off/on. Use one-slot variation for serial paths. | Appended-history reuse or every-slot generation overlap. |
| **Companion drafting** | Own-KV/companion state, loading or recovery concerns; companion MTP changes | `mtp-companion`, same contrasts as embedded, on the companion target. | Equivalent results to the embedded fixture or classic full-drafter coverage. |
| **Drafting + state/images** | Failure requires both speculation and reuse/cancellation or image input | MTP-on `text-state` / `image-state` additions below, alongside relevant off-mode package. For response stalls add MTP-on prefill. | Every backend's state interaction; Dense text-state mapping remains pending. |
| **Admission/lifecycle** | Queued cancellation, admission timeout, leaked admission permit | Separate `example-lifecycle-load`; use only when those stages are implicated. | Cache/draft/image-state integrity; not part of the eight-profile selector. |

## Commands

Run from the repository root on the authorized test machine, against a dedicated
server built from the intended revision with the shared aliases and required
models/libraries installed. The tool does not start/rebuild the server. Do not
infer the running server revision from the local branch alone. Defaults use
`http://localhost:11435`; override `RELIABILITY_HOST` and
`RELIABILITY_SERVER_LOG` deliberately when needed. A remote API alone does not
make its server log locally readable.

**Each invocation deletes/replaces its output directory.** Commands use a new
timestamped directory per package; preserve it before rerunning, and do not run
two invocations against the same server concurrently: the tools load/unload
selected models. Both selectors are explicit; each cell is on its own line.
The selection is a cross product. Separate commands avoid unintended pairs.
These are execution commands, not dry-runs: the initial selection display does
not wait for approval. `-list` lists identities, not runnable-pair validation.

### Text state

```sh copy Copy Text State Command
make test-load-text-state \
  RELIABILITY_OUT=".tools/reliability/output/text-state-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    MoE/Hyb/1slot/IMC-/MTP-/img-
    MoE/Hyb/1slot/IMC+/MTP-/img-
    Dense/Att/1slot/IMC-/MTP-/img-
    Dense/Att/1slot/IMC+/MTP-/img-
  '
```

### Hybrid image state

```sh copy Copy Hybrid Image State Command
make test-load-image-state \
  RELIABILITY_OUT=".tools/reliability/output/hybrid-image-state-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    MoE/Hyb/1slot/IMC-/MTP-/img+
    MoE/Hyb/1slot/IMC+/MTP-/img+
  '
```

For the companion-capable target, this is an available alternative, not a
requirement to run both routing labels. With MTP off it tests native target state.
Its selected/qualified image-state fixture is many-slot, unlike the same-slot
MoE example; do not treat that comparison as a backend-only contrast.

```sh copy Copy Companion Target Image State Command
make test-load-image-state \
  RELIABILITY_OUT=".tools/reliability/output/companion-image-state-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    Dense/Hyb/Nslot/IMC-/MTP-/img+
    Dense/Hyb/Nslot/IMC+/MTP-/img+
  '
```

For multi-slot image recovery, replace `1slot` with `Nslot` in the selected
image-state command. That is a distinct available workload capacity, not proof
that recovery used the cancelled physical slot. The selector also accepts Dense
Hybrid one-slot image-state, but its CSV cells are blank and no qualification is
claimed here. Do not infer same-slot companion coverage from the many-slot runs.

### Attention image state

This selects Gemma E4B QAT, qualified in both IMC modes with one and two slots:
three cycles per combination passed image/text correctness, cancellation,
slot release and recovery. IMC-on replay/append requests reused 174 tokens
in every cycle; IMC-off controls reported zero. See the capture and qualification
boundary in [coverage.md](coverage.md#selected-fixtures).
Use IMC-off as the state/answer control and IMC-on to exercise replay/append
reuse as well. Nslot forms are also available and qualified.
The previous Ministral fixture passed IMC-off but its template caused uncached
fallback with IMC on. Gemma qualification does not fix or clear that limitation.

```sh copy Copy Attention Image State Command
make test-load-image-state \
  RELIABILITY_OUT=".tools/reliability/output/attention-image-state-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    Dense/Att/1slot/IMC-/MTP-/img+
    Dense/Att/1slot/IMC+/MTP-/img+
  '
```

### Context retrieval

```sh copy Copy Context Retrieval Command
make test-load-long-context \
  RELIABILITY_OUT=".tools/reliability/output/context-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    MoE/Hyb/1slot/IMC-/MTP-/img-
    MoE/Hyb/1slot/IMC+/MTP-/img-
    Dense/Att/1slot/IMC-/MTP-/img-
    Dense/Att/1slot/IMC+/MTP-/img-
  '
```

The two state families have different stage budgets; compare each fixture
against itself, not 128K Hybrid timings against 16K Attention-only timings.
Hybrid MTP-on context forms are available when drafting/context interaction is
the question; add the same two Hybrid identities with `MTP+` explicitly.

### Hybrid prefill

```sh copy Copy Hybrid Prefill Command
make test-load-image-prefill \
  RELIABILITY_OUT=".tools/reliability/output/hybrid-prefill-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    MoE/Hyb/Nslot/IMC-/MTP-/img+
    MoE/Hyb/Nslot/IMC+/MTP-/img+
  '
```

The first cell runs ordinary shared encoding and an oversized isolated phase;
the second runs oversized IMC preparation. IMC preparation is **not** proof of
the non-IMC isolated scheduler branch. The mapping supplies fixtures and phases;
do not add an isolation flag just to reach them.

**Optional unqualified alternative:** the selector maps the companion target's
shared/IMC prefill path, but these CSV cells are blank, not selected covered
contracts. Use the following only when that target/path is material to the
investigation; treat its inference run as qualification as well as investigation.
It does not include the oversized Qwen3.6 phases. Do not present it as equivalent
to the selected/qualified Hybrid MoE package above.

```sh copy Copy Companion Target Prefill Command
make test-load-image-prefill \
  RELIABILITY_OUT=".tools/reliability/output/companion-prefill-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    Dense/Hyb/Nslot/IMC-/MTP-/img+
    Dense/Hyb/Nslot/IMC+/MTP-/img+
  '
```

### Attention prefill

```sh copy Copy Attention Prefill Command
make test-load-image-prefill \
  RELIABILITY_OUT=".tools/reliability/output/attention-prefill-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    Dense/Att/Nslot/IMC-/MTP-/img+
    Dense/Att/Nslot/IMC+/MTP-/img+
  '
```

### Batch load evidence contract

Text and image load use eight conversations / four slots, except the Dense
Hybrid text alternative, which uses four conversations / two slots. Both retain
21 turns and 30K final-prompt targets. Queue depth two gives eight or four
admission permits respectively, not guaranteed simultaneous admission.
The tooling-contract correction has inference validation on both paths; retain
historical PASS/FAIL evidence with its original workload identity.

November 6 qualification is limited to Hybrid MoE `batch-text` IMC-/MTP-,
IMC-/MTP+ and IMC+/MTP+, plus `batch-images` IMC+/MTP+. All completed the full
eight-conversation histories with behavioral PASS, scheduling coverage OBSERVED
and cleanup PASS. See [qualification and capture identities](coverage.md#batch-contract-qualification).
This does not qualify text IMC+/MTP-, Attention-only batch or other image modes.
The earlier interrupted image-pair log is useful behavioral evidence, but lacks
the summary/raw events required to qualify its IMC-/MTP+ scheduling coverage.

For each turn, require request-correlated assignment-to-execution-finish evidence
of concurrent full slot ownership without same-slot conflicts, plus a prepared
request waiting under full slot pressure, taking a released slot and completing.
Across the workload, retain successive diagnostics samples where one request's
`generated_tokens` increases while a different, consistently identified request
remains in text/IMC/media prefill. An absent witness is warning-only: short
prefills may not persist across samples. It does not make required coverage
INCOMPLETE or cause a nonzero exit. Keep the warning visible; do not claim that
generation-during-prefill was established. One text prefill owner is legal;
other slots may generate, and four occupied slots do not mean four generation
streams.

`maximum_concurrency` measures client response-interval overlap diagnostically.
Do not infer queue acceptance from client timing, report an inferred queue metric
or require universal all-four client-stream overlap. Low overlap must not stop
histories or replays. Missing, untriggered or contradictory required evidence is
coverage INCOMPLETE with a nonzero exit, separate from behavioral PASS/FAIL.
Use the [batch contract](coverage.md#batch-scheduling-evidence) when interpreting
these packages; required coverage OBSERVED can coexist with a diagnostic warning.
Do not substitute occupancy for generation-progress evidence.

The validated text MTP-on/IMC-off workload reached client overlap two on later
turns; the image IMC/MTP-on subject switch also reached two. Both retained full
slot ownership, waiting-job handoff and generation-during-prefill evidence and
completed their remaining histories. This validates the correction, not a claim
that the scheduler is maximally efficient. Single sequential text captures showed
MTP-on taking 5.8% longer than MTP-off with IMC off; investigate repeatability and
phase attribution separately rather than treating probe PASS as resolving it.

### Hybrid text load

```sh copy Copy Hybrid Text Load Command
make test-load-batch-text \
  RELIABILITY_OUT=".tools/reliability/output/hybrid-text-load-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    MoE/Hyb/Nslot/IMC-/MTP-/img-
    MoE/Hyb/Nslot/IMC+/MTP-/img-
    MoE/Hyb/Nslot/IMC-/MTP+/img-
    MoE/Hyb/Nslot/IMC+/MTP+/img-
  '
```

### Attention text load

```sh copy Copy Attention Text Load Command
make test-load-batch-text \
  RELIABILITY_OUT=".tools/reliability/output/attention-text-load-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    Dense/Att/Nslot/IMC-/MTP-/img-
    Dense/Att/Nslot/IMC+/MTP-/img-
  '
```

### Image history load

```sh copy Copy Image History Load Command
make test-load-batch-images \
  RELIABILITY_OUT=".tools/reliability/output/image-load-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    MoE/Hyb/Nslot/IMC-/MTP-/img+
    MoE/Hyb/Nslot/IMC+/MTP-/img+
    MoE/Hyb/Nslot/IMC-/MTP+/img+
    MoE/Hyb/Nslot/IMC+/MTP+/img+
  '
```

The existing image IMC/MTP-on FAIL remains historical evidence, not a pass under
the revised scheduling contract. A new full IMC/MTP-on run passed that contract,
including turn-11 subject switch and both replay phases, but again reported a
13,700-token prompt excess at the switch versus the earlier IMC-off control.
Image history duplication remains unresolved: behavioral/scheduling PASS and
positive cache usage do not prove cached text was decoded only once. Preserve
subject-switch, replay, isolation and cache assertions; do not drop the
changed-image phase. After a server fix, rerun the affected cell and an IMC-off
control, retaining physical-decode evidence as well as behavior and scheduling
results; the tooling correction alone cannot close the duplication issue.

### Embedded drafting

```sh copy Copy Embedded Drafting Command
make test-load-mtp-embedded \
  RELIABILITY_OUT=".tools/reliability/output/embedded-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    MoE/Hyb/Nslot/IMC-/MTP-/img-
    MoE/Hyb/Nslot/IMC+/MTP-/img-
    MoE/Hyb/Nslot/IMC-/MTP+/img-
    MoE/Hyb/Nslot/IMC+/MTP+/img-
  '
```

### Companion drafting

```sh copy Copy Companion Drafting Command
make test-load-mtp-companion \
  RELIABILITY_OUT=".tools/reliability/output/companion-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    Dense/Hyb/Nslot/IMC-/MTP-/img-
    Dense/Hyb/Nslot/IMC+/MTP-/img-
    Dense/Hyb/Nslot/IMC-/MTP+/img-
    Dense/Hyb/Nslot/IMC+/MTP+/img-
  '
```

For either dedicated drafting package, replace `Nslot` with `1slot` to select
its available serial fixtures; run both capacities when the changed code spans
both paths. Do not change only the fixture's slot count behind an existing cell.
An optional `RELIABILITY_ARGS='-mtp-min-rollback-rounds 1'` requests correlated
rejection/recovery evidence **only on an MTP-on selection**. It is not a normal
package requirement; if rejection is not observed, report incomplete evidence,
not incorrect MTP behavior. Do not add it to the mixed on/off commands above.

### Drafting + state/images

These add MTP-on interaction coverage to the relevant off-mode packages, rather
than asserting that dedicated drafting waves cover reuse or cancellation.

```sh copy Copy Drafting Text State Command
make test-load-text-state \
  RELIABILITY_OUT=".tools/reliability/output/drafting-text-state-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    MoE/Hyb/1slot/IMC-/MTP+/img-
    MoE/Hyb/1slot/IMC+/MTP+/img-
  '
```

This text-state fixture is Flash-Next; it is not the dedicated Qwen3.6 target
and cannot prove that target's cancellation behavior. For image state, both
embedded and companion target fixtures have selected coverage, at different
capacities (one slot below for embedded, many for companion):

```sh copy Copy Drafting Image State Command
make test-load-image-state \
  RELIABILITY_OUT=".tools/reliability/output/drafting-image-state-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    MoE/Hyb/1slot/IMC-/MTP+/img+
    MoE/Hyb/1slot/IMC+/MTP+/img+
    Dense/Hyb/Nslot/IMC-/MTP+/img+
    Dense/Hyb/Nslot/IMC+/MTP+/img+
  '
```

Select only the implicated backend if the question is backend-specific. For
drafting-time image arrival/stalls, use the Hybrid/companion prefill command
with `MTP+` instead of `MTP-`. These available forms require active drafting
and their mapped shared/IMC execution, not the forced oversized phases. The
companion-prefill forms remain unselected/unqualified alternatives; the MoE
forms have selected coverage.

### Admission/lifecycle

```sh copy Copy Lifecycle Command
make example-lifecycle-load \
  LIFECYCLE_LOAD_OUT=".tools/lifecycle-load/output/admission-$(date +%Y%m%d-%H%M%S)"
```

This has its own fixed fixture, admission configuration and artifacts. Consult
[the lifecycle tool](../lifecycle-load/main.go); do not pass reliability cells
or claim its permit recovery validates inference-state recovery.

## Paths outside the package commands

Not appearing in a package command means **not included in that starting
selection**, not unnecessary, unavailable or covered by a different capacity.
Choose these paths directly when their behavior is implicated by a quick-check
signal, changed responsibility or bug. They do not need a package of their own.

### Selected coverage reached through other selections

These are the selected CSV paths absent from the explicit package commands.
They are already selectable; the earlier variation instructions remain valid.
Here `IMC±` / `MTP±` means both mode choices, **not literal invocation syntax**.
Expand only the needed combinations into ordinary identities, one per line.

| Profile | Path family | When to choose it | Why the package selection does not replace it |
| --- | --- | --- | --- |
| `mtp-embedded` | `MoE/Hyb/1slot/IMC±/MTP±/img-` | Serial drafting/recovery failures, or changes affecting the one-sequence execution case. Include MTP-off controls when distinguishing drafting from target-only behavior. | Many-slot waves exercise concurrent sessions; a PASS there does not establish the one-slot configuration. |
| `mtp-companion` | `Dense/Hyb/1slot/IMC±/MTP±/img-` | Serial companion/own-KV state or recovery, with the same mode controls as embedded. | Neither concurrent companion waves nor the embedded target prove this target's serial contract. |
| `image-state` | `MoE/Hyb/Nslot/IMC±/MTP±/img+` | Image/text recovery with multiple slots available; slot release/selection changes; interactions between image state, reuse and drafting at that capacity. | The same-slot package deliberately removes slot-selection ambiguity. Many-slot state is useful but does not guarantee recovery on the cancelled physical slot or add batch-style concurrent sessions. |
| `image-state` | `Dense/Att/Nslot/IMC±/MTP-/img+` | Attention-only image recovery/reuse with multiple slots available, or changes affecting slot selection and release. | Gemma E4B QAT is qualified in both modes with two slots. One-slot recovery alone does not validate this capacity; neither form establishes concurrent image-session isolation or Ministral compatibility. |
| `long-context` | `MoE/Hyb/1slot/IMC±/MTP+/img-` | Retrieval or reuse fails only with drafting, or a context/state change affects speculation. Pair with the MTP-off context selection. | Dedicated drafting waves do not exercise staged long-context retrieval; MTP-off retrieval does not exercise drafting. |
| `image-prefill` | `MoE/Hyb/Nslot/IMC±/MTP+/img+` | Text progress degrades when images arrive during drafting, or a change touches the drafting/prefill interaction. Pair with the matching off-mode selection. | MTP-off shared/oversized phases do not exercise active drafting. These MTP-on cells require their mapped shared/IMC execution, not forced oversized isolation. |

These paths test meaningful configurations/interactions and should remain
available. They are **not mandatory additions to every investigation**. Retain
the relevant state/backend/slot distinctions, choose a control that answers the
question, and avoid running both merely to fill the table.

### Runnable alternatives without selected coverage

| Profile / path family | When it could help | Availability / qualification boundary |
| --- | --- | --- |
| `text-state`: `Dense/Hyb/1slot/IMC±/MTP±/img-` | Recovery/reuse on the Qwen3.8-27B target, including companion drafting interactions. | Mapped to one-slot aliases with three cycles; all four modes await qualification. CSV overlap labels retain the MoE representative rationale. |
| `batch-text`: `Dense/Hyb/Nslot/IMC±/MTP±/img-` | Growing histories and cross-session interference on that same target. | Mapped to two slots / four conversations; all four modes await behavioral and scheduling-evidence qualification. Not a four-slot MoE capacity test. |
| `image-state`: `Dense/Hyb/1slot/IMC±/MTP±/img+` | Same-slot image reuse/cancellation/recovery on the companion-capable target, when many-slot recovery would obscure the question. | Mapped, but CSV cells are blank and this guide claims no inference qualification. The existing Dense Hybrid image-state package uses many slots. |
| `image-prefill`: `Dense/Hyb/Nslot/IMC±/MTP+/img+` | Companion drafting plus image-arrival responsiveness; select the MTP-off companion-prefill command as its control. | Mapped, but blank/unqualified like the MTP-off companion-prefill alternatives already shown above. Does not supply Qwen3.6's oversized phases. |

Do not promote an alternative to established coverage without qualification.
Select only the modes needed for an investigation. To qualify all eight newly
mapped alternatives, use these separate invocations sequentially, not a cross
product that also requests inapplicable slot/profile pairs:

```sh copy Copy Dense Hybrid State Command
make test-load-text-state \
  RELIABILITY_OUT=".tools/reliability/output/dense-hybrid-state-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    Dense/Hyb/1slot/IMC-/MTP-/img-
    Dense/Hyb/1slot/IMC-/MTP+/img-
    Dense/Hyb/1slot/IMC+/MTP-/img-
    Dense/Hyb/1slot/IMC+/MTP+/img-
  '
```

```sh copy Copy Dense Hybrid Batch Command
make test-load-batch-text \
  RELIABILITY_OUT=".tools/reliability/output/dense-hybrid-batch-$(date +%Y%m%d-%H%M%S)" \
  RELIABILITY_CELLS='
    Dense/Hyb/Nslot/IMC-/MTP-/img-
    Dense/Hyb/Nslot/IMC-/MTP+/img-
    Dense/Hyb/Nslot/IMC+/MTP-/img-
    Dense/Hyb/Nslot/IMC+/MTP+/img-
  '
```
The four duplicate MTP-off profile references add no distinct workload on the
same fixture; choose the existing mapped control rather than rerunning its label.

### Other blank or not-applicable combinations

A blank is not a conclusion that a path is unnecessary. Before proposing it,
identify the distinct behavior or risk, inspect the actual mapping and fixture,
and state any missing workload or qualification. Dense/MoE routing alone is not
sufficient reason to duplicate a common orchestration probe; a concrete target,
backend or configuration concern can be. Respect workload boundaries: one-slot
batch/prefill and image input for text-only probes are not missing variants of
those contracts. Multi-GPU and classic full-drafter coverage remain out of scope.

## Running, collecting and judging evidence

Quick-check and reliability share `zarf/kms/tools_model_config.yaml`.
`make server-for-tools` loads that dedicated configuration from the current
checkout; restart the tools server after changing it. Normal server targets and
the codegen benchmark retain `zarf/kms/model_config.yaml` and its `/AGENT` entries.

- Obtain permission before accessing any remote test machine, including SSH discovery, fetching
  files or starting/stopping anything there. An instruction to analyze already
  fetched results is not permission for remote access. Present the selected
  commands/order first; do not change code, libraries, fixtures or server state
  on that machine without authorization for those actions.
- For authorized remote runs, execute these normal Make commands in its checkout;
  no special remote runner or package alias is required. Confirm the intended
  revision/configuration and available capacity before spending inference time.
- Fetch only the named run directory, preserving it locally under
  `.tools/reliability/output/`. Once the exact remote directory is known and
  fetching is authorized, use the template below. Do not assume “latest,” rename
  old results into a new qualification, or overwrite an existing local run.

```sh copy Copy Result Fetch Template
# Supply connection details locally; do not commit private host/user/path values.
REMOTE='USER@HOST'
REMOTE_OUTPUT='/ABSOLUTE/PATH/TO/CHECKOUT/.tools/reliability/output'
# Set RUN to the exact directory produced by the authorized run.
RUN='REPLACE-WITH-EXACT-RUN-DIRECTORY'
mkdir -p .tools/reliability/output
rsync -avh --progress \
  "$REMOTE:$REMOTE_OUTPUT/$RUN/" \
  ".tools/reliability/output/$RUN/"
```

Keep `summary.json`, `events.ndjson` and `tool.log` together. Preserve effective
model configuration, tested server/tool revisions and native library versions
with the evidence; configured modes alone do not prove observed modes. Events
may contain prompts/outputs: review for sensitive content before sharing.

Read final summary/coverage first, then failing phase details, captured outputs
and correlated events. Scenario names can differ from profile names (`images`,
`batch`, `mtp`); use `selection`, cell, models and phase to identify the workload.
Historical captures may use numeric cells or old names: translate via the matrix,
never interpret an old tool number using a reordered column without checking.

| Result or signal | Interpretation / next action |
| --- | --- |
| Behavioral FAIL | Record the violated contract and actual output/progress. Investigate fixture capability, unsupported compatibility and server behavior; do not guess the cause from the verdict. |
| Coverage INCOMPLETE / missing, untriggered or contradictory required evidence | The requested path has not been established; nonzero exit is separate from behavioral PASS/FAIL. Check encoding, rejection and batch scheduling/progress evidence as applicable; missing evidence is not itself wrong-answer behavior. |
| GAP / NOT APPLY / NOT RUN | No passing execution of that pair. Explain the selection/fixture boundary; do not silently redirect to a representative. |
| Behavioral PASS with complete required evidence | The executed bounded contract passed. Does not clear unrelated quick-check failures, unrun history turns or other models/backends. |
| Timing change without failure | Compare the same fixture, workload, stages, modes, hardware/libraries and measured phase against an appropriate baseline. No package supplies a universal performance threshold or causal attribution. |

Ordinary scenario failures allow later selected pairs/phases to run; cleanup
failure stops further execution. A behavioral batch failure may leave later turns
untested; low response overlap or incomplete scheduling coverage must not truncate
histories or replays.
Inspect completed selections rather than assuming a nonzero exit means nothing
ran, or that every requested workload finished. Separate Make commands also
need deliberate failure handling if placed in a shell script; do not accidentally
use fail-fast execution for an intended overnight evidence collection.

## Companion skill and maintenance contract

The [companion skill](../../.agents/skills/probing-kronk/SKILL.md)
uses this reference, not a second hard-coded matrix. Its proposed plan should state:

1. Entry point, observed symptom/change, and uncertainty; distinguish user theory
   from established evidence.
2. Selected packages or individual profile × identities, why each tests the
   question, controls and order; identify fixture substitutions and known
   failing/unavailable paths. Consult paths outside the package commands too;
   do not treat package membership as a prerequisite for selecting a useful path.
3. Exact current commands, separate output destinations, expected scope/cost and
   required access/server permissions. No claim that cross-product selections
   execute only selected diagonal pairs.
4. What evidence will answer the question and what cannot be concluded. Read
   existing evidence first; fetch/run only within explicit authorization.
5. After execution, results per selected contract, missing phases, established
   causes versus hypotheses, and only evidence-justified follow-ups.

When coverage, profile names, fixtures, mappings or evidence contracts change,
update CSV + coverage reference + affected packages/path guidance together, and check
the skill's instructions. Validate command syntax and every referenced pair
against the current selector without inference before publishing new commands.
Do not move development chronology into this reference. The local ignored
`development-findings.md` is optional background, never a dependency for choosing
a package or understanding a known limitation described here.
