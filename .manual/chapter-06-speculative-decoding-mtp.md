# Chapter 6: Speculative Decoding & MTP

## Table of Contents

- [6.1 Overview & The Two Draft Modes](#61-overview-the-two-draft-modes)
- [6.2 Separate-GGUF Draft (Recap)](#62-separate-gguf-draft-recap)
- [6.3 MTP Drafts (Multi-Token Prediction)](#63-mtp-drafts-multi-token-prediction)
- [6.4 Auto-Detection: `selectAndLoadDraft`](#64-auto-detection-selectandloaddraft)
- [6.5 MTP Requirements & Skip Reasons](#65-mtp-requirements-skip-reasons)
- [6.6 Pre-Norm Hidden-State Plumbing](#66-pre-norm-hidden-state-plumbing)
- [6.7 The Mirror Step & AR Draft Loop](#67-the-mirror-step-ar-draft-loop)
- [6.8 Verification on the MTP Path](#68-verification-on-the-mtp-path)
- [6.9 Hybrid Target Rollback: Snapshot/Restore](#69-hybrid-target-rollback-snapshotrestore)
- [6.10 Adaptive `nDraft` (Acceptance EMA)](#610-adaptive-ndraft-acceptance-ema)
- [6.11 Per-Slot State Added for MTP](#611-per-slot-state-added-for-mtp)
- [6.12 Configuration](#612-configuration)
- [6.13 Observability](#613-observability)
- [6.14 Code Map](#614-code-map)
- [6.15 Testing](#615-testing)
- [6.16 Known Limitations](#616-known-limitations)

---

This chapter documents Kronk's speculative-decoding stack with a focus
on the **MTP (Multi-Token Prediction)** drafter shipped in
[PR #593](https://github.com/ardanlabs/kronk/pull/593). It assumes you
have read the introductory speculative-decoding section in
[Chapter 3 §3.12](chapter-03-model-configuration.md#312-speculative-decoding),
which covers the conventional separate-GGUF drafter and the
Leviathan-style verify math at a user level. This chapter goes deeper
into the engine internals and explains the auto-detected MTP path
that the separate-GGUF discussion does not cover.

### 6.1 Overview & The Two Draft Modes

Kronk supports two interchangeable sources of draft tokens for
speculative decoding. The drafter sits behind a single `*draftModel`
type ([model.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/model.go)),
selected once at model load by `selectAndLoadDraft`
([draft_mtp.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/draft_mtp.go)).

| Mode                | When used                                                                                            | Drafter GGUF                                                  | Driver                                                                              |
| ------------------- | ---------------------------------------------------------------------------------------------------- | ------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| **Separate-GGUF**   | `cfg.DraftModel != nil` (explicit user config — `draft-model:` block in YAML or `WithDraftModel`).   | A second, smaller GGUF loaded into its own `llama_model`.     | `llama.DraftGenerate` token-only loop in `generateDraftTokens`.                     |
| **MTP**             | Auto-enabled when the target GGUF carries a Multi-Token-Prediction head (`nextn_predict_layers > 0`) and a few sanity gates pass. | None — the MTP head lives inside the TARGET GGUF; the draft context **shares the target's `llama_model`**. | Bespoke `generateDraftTokensMTP` AR loop that feeds the head `(token_id, pre_norm_hidden_state)` per step. |

A model can have at most one drafter active. If the user configures
`DraftModel` explicitly, that wins; the MTP head — even when present —
is ignored on that load.

### 6.2 Separate-GGUF Draft (Recap)

Configure via `draft-model:` in `model_config.yaml`
(Chapter 3 §3.12 covers the YAML shape and field list).

Requirements:

- Draft and target share the same tokenizer (vocabulary).
- `nseq-max: 1` (single-slot) on the target.
- Draft GGUF is downloaded locally.

Runtime characteristics:

- Loaded by `loadDraftModel` in `batch_speculative.go`.
- Tokens drafted by `generateDraftTokens` which delegates to
  `llama.DraftGenerate` — a tight FFI loop that does decode → sample →
  capture in one C call per step.
- Verified by `verifySpeculativeTokens` using either greedy argmax
  (temperature = 0) or the sparse-candidate probabilistic verify
  (temperature > 0); see `speculative_sparse.go`.
- KV rollback on rejection is a single `MemorySeqRm` on the target.

The remainder of the chapter is about the MTP path.

### 6.3 MTP Drafts (Multi-Token Prediction)

MTP heads ship inside certain modern GGUFs (Qwen3.5 / Qwen3.6
architecture `qwen35`, `qwen35moe`, and future architectures that
populate the same metadata key). The head is not a standalone language
model — it is a few extra layers grafted onto the target that predict
the **next N tokens** of the target's continuation in a single forward
pass, given:

1. The token id at position `t`.
2. The target's **pre-norm hidden state** at position `t` — i.e. the
   residual-stream activation immediately before the final layer norm.

Because the head shares the target's weights and tokenizer, there is no
extra file to download and no vocabulary mismatch to worry about. The
trade-off is more invasive plumbing: every target `llama_decode` must
**mirror** its pre-norm hidden buffer into the draft context, and the
draft context's auto-regressive loop must feed back both the sampled
token and the previously emitted hidden state on every step.

Reference: `common/speculative.cpp common_speculative_impl_draft_mtp`
in upstream llama.cpp. Kronk's implementation lives in
[`draft_mtp.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/draft_mtp.go)
(load), [`batch_mtp.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_mtp.go)
(mirror + AR loop), and integration changes in
[`batch_engine.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_engine.go),
[`batch_slot.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot.go),
[`batch_slot_start.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot_start.go),
[`batch_speculative.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_speculative.go),
[`batch_prefill_text.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_prefill_text.go),
[`batch_finish.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go),
and the FFI bindings in
[`yzma.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/yzma.go).

### 6.4 Auto-Detection: `selectAndLoadDraft`

`selectAndLoadDraft` runs once during model initialization
(`initGenerationRuntime` in `model.go`) and decides which drafter, if
any, to load. The decision tree:

```diagram
                       ╭───────────────────────╮
                       │ cfg.DraftModel != nil │──── yes ──▶  loadDraftModel  (separate-GGUF)
                       ╰──────────┬────────────╯
                                  │ no
                                  ▼
                       ╭───────────────────────╮
                       │ mtpNextNLayers(target)│──── 0 ──▶  return (nil, nil) — no drafter
                       ╰──────────┬────────────╯
                                  │ > 0
                                  ▼
                       ╭───────────────────────╮
                       │     MTPAvailable()    │──── false ──▶ skip (log reason: old llama.cpp)
                       ╰──────────┬────────────╯
                                  │ true
                                  ▼
                       ╭───────────────────────╮
                       │ ctxParams.NSeqMax == 1│──── no ──▶ skip (log reason: multi-slot)
                       ╰──────────┬────────────╯
                                  │ yes
                                  ▼
                       loadDraftModelMTP  (auto-enabled)
```

`mtpNextNLayers` looks up the GGUF metadata key
`<arch>.nextn_predict_layers` (a uint32). Kronk matches by the unique
substring `nextn_predict_layers` so the same lookup works for every
architecture variant without first reading `general.architecture`.

`MTPAvailable()` probes whether the loaded llama.cpp library exports
the three pre-norm symbols listed in §6.6. Older builds (pre
`src/llama-ext.h`) won't have them — Kronk logs and starts up without
MTP rather than crashing on a missing symbol mid-request.

### 6.5 MTP Requirements & Skip Reasons

| Requirement                                   | Why                                                                                                                                            |
| --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| Target GGUF has `nextn_predict_layers > 0`    | No MTP head exists otherwise; nothing to load.                                                                                                 |
| llama.cpp build exports pre-norm symbols      | Kronk reads hidden states via `llama_get_embeddings_pre_norm{,_ith}` and toggles them on via `llama_set_embeddings_pre_norm`. See §6.6.        |
| `nseq-max: 1` on the target                   | MTP + multi-slot triggers a `GGML_ASSERT(logits != nullptr)` in `llama_sampler_sample` on the shared-batch decode that mixes one slot's MTP spec tokens with another slot's fresh prefill. Belt-and-suspenders gate in both `selectAndLoadDraft` and `loadDraftModelMTP`. |

When any of those fail, `selectAndLoadDraft` logs the specific reason
and returns `(nil, nil)`. The target still loads and serves traffic —
just without speculation.

### 6.6 Pre-Norm Hidden-State Plumbing

The MTP path needs three llama.cpp C symbols that yzma upstream does
not yet bind. Kronk adds them locally in
[`yzma.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/yzma.go)
via the `jupiterrider/ffi` package:

| Symbol                                | Go wrapper                                            | Purpose                                                                                              |
| ------------------------------------- | ----------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `llama_set_embeddings_pre_norm`       | `SetEmbeddingsPreNorm(ctx, value, masked)`            | Toggle pre-norm extraction on a context. `masked=false` = dense (all rows); `masked=true` = sparse (logit-flagged rows only). |
| `llama_get_embeddings_pre_norm`       | `GetEmbeddingsPreNorm(ctx, nRows, nEmbd) []float32`   | Return the dense buffer produced by the most recent `llama_decode`. Used on the target.              |
| `llama_get_embeddings_pre_norm_ith`   | `GetEmbeddingsPreNormIth(ctx, i, nEmbd) []float32`    | Return a single row by output-table index. Used on the draft (masked) context.                       |

Two binding details worth highlighting:

- **Symbol probing is dual.** Each prep tries the C-linkage name first
  and falls back to the Itanium C++ ABI mangled form (e.g.
  `_Z29llama_set_embeddings_pre_normP13llama_contextbb`) so kronk
  binds against llama.cpp builds compiled with or without `LLAMA_API`
  on these declarations.
- **Best-effort init.** `InitYzmaWorkarounds` never fails on a missing
  pre-norm symbol. The corresponding `ffi.Fun` stays zero-valued and
  `MTPAvailable()` returns false, gating §6.4.

At load time `loadDraftModelMTP` sets:

- `SetEmbeddingsPreNorm(targetCtx, true, false)` — dense, every row
  accessible by raw batch index. Required for the mirror step (§6.7),
  which reads arbitrary rows from each completed target batch.
- `SetEmbeddingsPreNorm(draftCtx, true, true)` — sparse, only
  logits-flagged rows stored. The draft only needs the single output
  row of each AR step.

The flag is consumed at graph-build time, so it must be set **before**
the first decode on either context.

### 6.7 The Mirror Step & AR Draft Loop

Two functions in [`batch_mtp.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_mtp.go)
do the heavy lifting.

#### Mirror: `mirrorTargetBatchToMTPDraft`

After every successful target `llama_decode` + `llama_synchronize`, the
post-decode pass in `processBatch` calls the mirror to replay the
slot's just-decoded range into the draft context with `batch.embd`
populated from the target's pre-norm buffer.

Per-position alignment is **shift-right-by-1**, matching
`common_speculative_impl_draft_mtp`:

```
mirror[0]   : token = tgt[start+0],  embd = pendingH (slot's pre-batch h)
mirror[k>0] : token = tgt[start+k],  embd = h_tgt[start+k-1]
```

`pendingH` is a per-slot copy of the hidden row at the last committed
target position. On the very first decode of a sequence, no `h` has
been observed yet — that slot of the mirror batch is zeroed (the MTP
head's first prediction at position 0 is on a BOS / instruction
sentinel where exact `h` does not matter).

After the mirror succeeds, `pendingH` is updated to the last
target-batch row so it's ready as the slot-0 input of the next
mirror.

A few non-obvious correctness points enforced in the function:

- **Chunking by mirror capacity.** The mirror batch is allocated at
  `NBatch` capacity. When `effectiveCount > NBatch` the mirror is run
  in chunks, with `llama.Synchronize(draft)` **inside the chunk loop**
  before the next chunk overwrites `mirror.Embd`. Without the per-chunk
  sync, the next chunk's `copy()` into the Go-owned embd slice races
  the still-in-flight C read on async backends (Metal/CUDA) and
  corrupts the input.
- **`effectiveCount` is caller-provided.** Prefill chunks and plain
  gen-token decodes mirror `targetBatchCount` positions; the spec
  path mirrors only `1 + accepted` rows so rejected draft tokens are
  never reflected into draft KV.
- **`logits=true` only on the last row.** The mirror only needs the
  pre-norm row of the very last position (as the next `pendingH`), so
  only the last row is logits-flagged.

#### AR Draft: `generateDraftTokensMTP`

The drafter runs an auto-regressive loop on the MTP context. Each
iteration:

1. Build a single-token batch with `(curToken, pos, seqIDs)` and copy
   `curEmbd` into the embd slot.
2. `llama.Decode` + `llama.Synchronize` (async-backend safety again).
3. `llama.SamplerSample(greedy, ctx, -1)` to pick the next draft token.
4. `GetEmbeddingsPreNormIth(ctx, 0, nEmbd)` to read back the next
   hidden state.
5. EOG check; copy `nextEmbd` into `pendingH`; advance.

The loop stops on `chooseNDraft(s, draft.nDraft)` rounds (see §6.10),
or earlier on EOG or decode failure.

**Why MTP-only batches?** `llama.BatchInit(N, embd, nSeqMax)` allocates
**either** the token buffer **or** the embd buffer — never both — based
on its `embd` arg. MTP needs both per position. Kronk works around this
by calling `BatchInit(N, 0, 1)` to get a token-only batch (with `pos`,
`seq_id`, and `logits` arrays sized to `N`) and then attaching a
Go-allocated `[]float32` of size `N*nEmbd` as the embd buffer. The Go
slice is pinned (`runtime.Pinner`) for the batch's lifetime and the
`Batch.Embd` pointer is cleared **before** `BatchFree` so llama.cpp's
unconditional `free(batch.embd)` doesn't `free()` a Go heap allocation.

These two MTP-only batches live on `draftModel`:

- `draftBatchMTP` — capacity 1, used by `generateDraftTokensMTP` per
  step.
- `mirrorBatchMTP` — capacity `NBatch`, used by the mirror step.

### 6.8 Verification on the MTP Path

`verifySpeculativeTokens` is shared between separate-GGUF and MTP, but
the MTP path forces **greedy verification** unconditionally because
the MTP head currently runs only greedy sampling (`SamplerInitGreedy`)
and the AR loop does not capture sparse draft distributions. Running
the probabilistic verify path without a draft distribution would fall
through to `sampleFromProbs(target)` at every position and reject
every draft token unconditionally.

To compensate, the greedy branch is taught — only on the MTP path
(`mtpGreedy == true`) — to invoke the slot's **full sampler** at each
position instead of taking the raw target argmax. That preserves the
user's `temperature` / `top_k` / `top_p` shape on the emitted
sequence. The mathematical guarantee of distribution-equivalent
output (Leviathan et al., 2023) is lost on the MTP path — it is the
standard approximation when the draft distribution is unavailable.

`originalSampled` is also snapshotted before the verify loop, because
`handleSampledToken` mutates `s.sampled` as each accepted draft token
flows through the streaming pipeline. The hybrid re-decode path
(§6.9) needs the **original** sampled token at the base position;
using the mutated value would re-decode the wrong token and corrupt
every subsequent round.

After verify, the MTP mirror runs again over `1 + accepted` rows to
overwrite the AR-loop draft KV entries with target-derived hidden
states. That update is what makes the next round's `pendingH` reflect
reality.

`rollbackDraft` for MTP is also different from the separate-GGUF
path: it `MemorySeqRm`s the **entire** drafted range from the draft
KV before the post-verify mirror runs. llama.cpp's transformer KV
does not overwrite by `(seq, pos)` on re-decode — it appends another
slot, leaving duplicate entries that corrupt subsequent attention.
The mirror then writes the correct target-derived entries into clean
slots.

### 6.9 Hybrid Target Rollback: Snapshot/Restore

Hybrid target models (transformer + recurrent layers) introduce a
problem the regular `MemorySeqRm` rollback cannot solve: the
recurrent layer has been **advanced through all `1+nDraft` decoded
positions** and there is no per-position trim. A partial-rejection
round would leave the recurrent state advanced past the accepted
boundary, and the next `llama_decode` would fail with `-1`.

Two new helpers in `batch_speculative.go` solve this:

| Helper                          | What it does                                                                                                                                                |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `captureTargetSpecSnapshot(s)`  | Sizes `s.specSnapshot` via `StateSeqGetSize` and reads the full per-sequence state with `StateSeqGetData`. Called **before** the spec batch is decoded.     |
| `restoreTargetSpecSnapshot(s)`  | `StateSeqSetData` to rewind, then re-decode `(sampledAtBase + first accepted drafts)` so the seq ends at exactly `basePast + 1 + accepted` correct positions. |

The snapshot buffer is lazy-grow / never-shrink on the slot
(`s.specSnapshot`). Size scales with current KV occupancy, so the cost
grows with context length. Dense / pure-attention targets skip this
path entirely — `MemorySeqRm` is correct and much cheaper for them.

The captureTarget/restoreTarget hooks are gated on
`e.model.modelInfo.Type == ModelTypeHybrid` so the dense fast path is
untouched. If `captureTargetSpecSnapshot` errors, `verifySpeculativeTokens`
clears `s.specSnapshot` and falls through to `MemorySeqRm`. The fallback
is broken on hybrid partial-reject rounds, but full-accept rounds still
work, and the next request begins with a fresh sequence anyway.

### 6.10 Adaptive `nDraft` (Acceptance EMA)

Drafting `N` tokens that all get rejected wastes a forward pass on the
draft model. `chooseNDraft(s, maxDraft)` in
[`batch_speculative.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_speculative.go)
scales down based on the slot's exponential moving average of
acceptance rate (`specAccEMA`):

| EMA range  | `nDraft`         |
| ---------- | ---------------- |
| `< 0.30`   | `min(1, max)`    |
| `< 0.50`   | `min(1, max)`    |
| `< 0.70`   | `min(2, max)`    |
| `< 0.85`   | `min(3, max)`    |
| `≥ 0.85`   | `max` (configured) |

`specAccEMA` is updated per spec round with the formula
`0.9*old + 0.1*(accepted/nDraft)` and **persists across requests** on
the slot, so a long quiet streak with poor acceptance keeps draft
overhead low even when a new request begins on the same slot.

When the EMA collapses to ~0, `chooseNDraft` returns 0 and the spec
path is bypassed for that round — but the draft-tokens / accepted /
acceptance-rate fields are still emitted on the final slot log line
so dashboards see a stable schema. See `finishSlot` and
`sendFinalResponse`.

### 6.11 Per-Slot State Added for MTP

PR #593 added the following fields to `slot` in
[`batch_slot.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot.go).
All are reset in `slot.reset()` with lazy-grow / never-shrink
buffer policy.

| Field                                                 | Purpose                                                                                                                                   |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `pendingH []float32`                                  | Copy of the most-recently committed target pre-norm row. Slot-0 input of the next mirror batch.                                            |
| `targetBatchStart / Count / BasePos`                  | Slot's contiguous range inside the shared target batch — captured at batch-add time so the post-decode mirror knows where its rows live. |
| `mtpHasBatch`                                         | True between `batch.Add()` and the post-decode mirror; cleared by the mirror.                                                              |
| `mtpDisabledForRequest`                               | Set at `startSlot` to disable MTP for this request (currently used as a diagnostic switch on IMC cache hits — see §6.16).                  |
| `specSnapshot []byte`                                 | Pre-spec target state buffer for hybrid rollback (§6.9). Lazy-grow.                                                                        |
| `specRounds`                                          | Counter used to throttle per-round verify logging (logs first round, then every 32nd).                                                     |

### 6.12 Configuration

MTP is **auto-enabled** — you do not configure it. To get an MTP
drafter:

1. Pull a target GGUF that ships an MTP head (e.g. the Qwen3.6 MTP
   builds — the test suite uses
   `unsloth/Qwen3.6-35B-A3B-MTP-GGUF/Qwen3.6-35B-A3B-MTP-UD-Q2_K_XL.gguf`).
2. Configure the target with `nseq-max: 1`.
3. Do **not** set `draft-model:` on that entry (an explicit
   separate-GGUF draft wins over auto-detected MTP).
4. Make sure your llama.cpp library is recent enough to export the
   pre-norm API — Kronk's libs ship with a sufficiently new build by
   default; only matters for users running with a pinned older lib.

Minimal `model_config.yaml` snippet:

```yaml
unsloth/Qwen3.6-35B-A3B-MTP-UD-Q2_K_XL:
  context-window: 8192
  nbatch: 2048
  nubatch: 512
  cache-type-k: f16
  cache-type-v: f16
  nseq-max: 1
  incremental-cache: true
```

On a successful load you will see a log line like:

```
draft-model-mtp status=loaded source=auto-detected nDraft=4 nextn-layers=1 nEmbd=2048 nCtx=8192
```

The default `nDraft` for MTP is `4` (`defMTPNDraft` in
`draft_mtp.go`) — conservative because MTP heads typically have high
acceptance for the first 1–3 tokens and rapidly decay beyond that.
The adaptive EMA in §6.10 scales further down when acceptance is
poor.

### 6.13 Observability

MTP-specific log events (all at the same level as the surrounding
batch-engine logs):

| Event                                          | Where                              | When                                                                                       |
| ---------------------------------------------- | ---------------------------------- | ------------------------------------------------------------------------------------------ |
| `draft-model-mtp status=loading / loaded`      | `loadDraftModelMTP`                | Once at model startup.                                                                     |
| `draft-model-mtp status=auto-detect-skipped`   | `selectAndLoadDraft`               | Once when the gate fails (no metadata, no pre-norm API, multi-slot).                       |
| `speculative status=mtp-mirror-error`          | `processBatch` / `verifySpeculativeTokens` | Mirror decode failed; slot is finished or its draft KV is desync'd.                |
| `speculative status=mtp-imc-hit-diagnostic`    | `startSlotText`                    | Per-request diagnostic (see §6.16 — IMC interaction).                                      |
| `speculative status=verify-done`               | `verifySpeculativeTokens`          | Throttled: first round + every 32nd. Carries `round`, `accepted`, `nDraft`, `acc_ema`.    |
| `speculative status=restore-error`             | `verifySpeculativeTokens`          | Hybrid snapshot restore failed; falls back to broken `MemorySeqRm`.                        |
| `speculative status=snapshot-error`            | `processBatch`                     | Hybrid snapshot capture failed; spec round will fall back to `MemorySeqRm` on rejection.   |
| `chat-completion ... draft_tokens=N ...`       | `sendFinalResponse`                | Always present once the model has a drafter, even when speculation produced 0 tokens.      |

The `finishSlot` log line follows the same rule — `draft_tokens`,
`draft_accepted_tokens`, and `acceptance_rate` fields are emitted
whenever `e.model.draft != nil`, so log schemas stay stable across
requests where the EMA collapsed mid-stream.

### 6.14 Code Map

| File                                                                                                                                         | Role for MTP                                                                                                                       |
| -------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| [`sdk/kronk/model/draft_mtp.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/draft_mtp.go)                     | `mtpNextNLayers`, `loadDraftModelMTP`, `selectAndLoadDraft`. Sole source for MTP load + detect.                                    |
| [`sdk/kronk/model/batch_mtp.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_mtp.go)                     | `mirrorTargetBatchToMTPDraft`, `generateDraftTokensMTP`, helpers (`batchTokensAt`, `unsafeFloatSlice`, `mirrorBatchCapacity`).     |
| [`sdk/kronk/model/yzma.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/yzma.go)                               | FFI bindings for the three pre-norm symbols; `MTPAvailable`, `SetEmbeddingsPreNorm`, `GetEmbeddingsPreNorm{,Ith}`.                 |
| [`sdk/kronk/model/model.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/model.go)                             | `draftModel` struct extended with MTP fields (`mtp`, `nEmbd`, MTP batches, pinned embd slices). `Unload` skips shared `ModelFree`. |
| [`sdk/kronk/model/batch_slot.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot.go)                   | `slot` struct extended with per-slot MTP state (`pendingH`, target-batch range, `mtpHasBatch`, `mtpDisabledForRequest`, `specSnapshot`, `specRounds`). |
| [`sdk/kronk/model/batch_slot_start.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot_start.go)       | Skips separate-draft-prefill on MTP; diagnostic IMC-hit logging.                                                                   |
| [`sdk/kronk/model/batch_engine.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_engine.go)               | `processBatch` integration: claims slot's target-batch range at every add site, mirrors after every successful decode, dispatches MTP vs separate-GGUF draft generation. |
| [`sdk/kronk/model/batch_prefill_text.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_prefill_text.go)   | `addPrefillChunk` claims (or extends) the slot's MTP target-batch range so prefill rows get mirrored.                              |
| [`sdk/kronk/model/batch_speculative.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_speculative.go)     | Greedy-only MTP verify path; `originalSampled` snapshot; hybrid snapshot/restore; post-verify mirror; throttled `verify-done` log; MTP-specific `rollbackDraft`. |
| [`sdk/kronk/model/batch_finish.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go)               | Always-emit draft metrics when a drafter is configured.                                                                            |
| [`sdk/kronk/model/params.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/params.go)                           | `top_p == 0 || == 1` from the request is treated as unset so the model-config `top_p` survives.                                    |

### 6.15 Testing

Test package: [`sdk/kronk/tests/mtp/`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/tests/mtp).

The suite is a smoke test against the
`unsloth/Qwen3.6-35B-A3B-MTP-UD-Q2_K_XL` target via `testlib.CfgMTPChat()`.
A successful `Chat` and `ChatStreaming` response implicitly verifies
that:

- The MTP draft context loaded (auto-detection passed).
- Pre-norm extraction is wired correctly on both contexts.
- The mirror step is in sync after every target decode.
- Speculation produced valid drafts and the target accepted and
  emitted clean text.

`TestMain` skips the whole suite when the MTP model file is not
downloaded, so contributors without the GGUF locally still get a green
run.

Run from the project root:

```shell
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE=$(pwd)
go test -v -count=1 ./sdk/kronk/tests/mtp/...
```

### 6.16 Known Limitations

| Limitation                                          | Why                                                                                                                                                                   |
| --------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `nseq-max: 1` only                                  | See §6.5 — the shared-batch decode mixing one slot's MTP spec tokens with another slot's prefill triggers a `GGML_ASSERT` in `llama_sampler_sample`.                  |
| Greedy verify only                                  | The MTP head's AR loop runs greedy sampling and does not capture sparse draft distributions; probabilistic verify would always reject. See §6.8.                      |
| IMC + MTP interaction is a measurement              | IMC restores only the target KV; the draft context has no snapshot, so an IMC cache-hit request runs MTP against an empty / stale draft context. The original guard would have disabled MTP on IMC hits; PR #593 leaves it enabled and logs `mtp-imc-hit-diagnostic` to measure real-world acceptance. If acceptance collapses to ~0%, the safe-default disable will be restored. |
| No vision / audio                                   | Speculative decoding in general is text-only in Kronk.                                                                                                                |
| `defMTPNDraft = 4` is a fixed cap                   | The adaptive EMA scales down from 4, but there is no per-model config to raise the ceiling for an exceptionally well-behaved MTP head.                                |

---
