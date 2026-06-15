# Gemma4 MTP (Separate-File Assistant) — Implementation Notes

Thread: https://ampcode.com/threads/T-019ecb2b-7c2e-75d6-bdb6-1e1a6574a43a

Status: **NOT YET IMPLEMENTED (runtime).** The download/catalog/BUI plumbing for
the co-located `mtp-*.gguf` companion file is done. The runtime engine work to
actually *use* the assistant file is planned and specced below, but deliberately
deferred.

---

## Goal

When a Gemma4 model is downloaded together with its co-located MTP assistant
file (e.g. `mtp-gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf`), use that assistant file
**automatically** for MTP speculative decoding when it exists on disk next to the
main model.

### Hard constraints (from Bill)

- **Do NOT touch `DraftModelConfig`.** No new user knob. The assistant file is
  either on disk (then use it) or it isn't (then don't).
- Treat it like `ProjFile`: a top-level `Config.MTPFile` is acceptable, auto-wired
  from disk. (We may eventually move `ProjFile` to this same model later; out of
  scope now.)
- Keep MTP gating in `sdk/kronk/model` — do **not** move it into `sdk/kronk/parsers`
  (parsers are for output/prompt semantics; moving runtime gating there complicates
  imports/abstractions for no benefit).

---

## What is already DONE and verified (download / catalog / BUI)

Companion download/catalog/BUI support for the co-located `mtp-*.gguf` file mirrors
the existing `mmproj` support:

- `sdk/tools/models`:
  - `ModelPath.MTPFile`
  - `CatalogEntry.MTP / MTPOrig / MTPSize / MTPChecked`
  - `Resolution.MTP / MTPOrig / DownloadMTP / LocalMTP`
  - catalog summary exposes `files.mtp` and `has_mtp`
  - `SchemaVersion` bumped 1 -> 2 in `sdk/tools/models/catalog.go`
  - `discoverCompanions` during reconcile heals old entries (one-time MTP
    discovery via `mtp_checked`; also recovers clobbered mmproj metadata)
  - `persistURLResolution` fixed so an MTP-only pull no longer wipes existing
    mmproj metadata (and vice-versa)
- Server/BUI:
  - `cmd/server/app/domain/toolapp/model.go`: `ResolveResponse.DownloadMTP`,
    `PullRequest.MTPURL`, `PullResponse.MTPFile`
  - `cmd/server/app/domain/toolapp/catalog.go`: installed=false when MTP expected
    but missing; returns `download_mtp`
  - `cmd/server/app/domain/toolapp/models.go`: pull/download/peer paths thread `mtp_url`
  - BUI `services/api.ts` `pullModel(..., mtpUrl?)`, `types/index.ts`,
    `contexts/DownloadContext.tsx`, `components/ModelPull.tsx` (MTP drafter row +
    "Override MTP drafter URL"; removed `DownloadInfoTable` from ModelPull only),
    `components/CatalogList.tsx` (MTP Drafter URL)

Validation that passed for the above:
- `go build ./...`
- `go vet ./sdk/tools/models/ ./cmd/server/app/domain/toolapp/`
- `staticcheck ./sdk/tools/models/ ./cmd/server/app/domain/toolapp/`
- `RUN_IN_PARALLEL=yes GITHUB_WORKSPACE=<repo> go test ./sdk/tools/models/`
- BUI `tsc --noEmit`

---

## Hugging Face references

- Main:      https://huggingface.co/unsloth/gemma-4-26B-A4B-it-GGUF
- Assistant: https://huggingface.co/unsloth/gemma-4-26B-A4B-it-GGUF/blob/main/mtp-gemma-4-26B-A4B-it.gguf

The assistant ("mtp-…") file is the per-model MTP head. There is no user choice of
drafter — it ships alongside the main GGUF, so we download it with the main file
when present.

---

## Critical metadata discovery (Gemma4 assistant GGUF)

Dumped from
`~/.kronk/models/unsloth/gemma-4-26B-A4B-it-GGUF/mtp-gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf`:

- `general.architecture            = gemma4-assistant`
- `gemma4-assistant.nextn_predict_layers = 4`   (== n_layer_all: the ENTIRE model is the MTP head)
- `embedding_length     = 1024`  (assistant INTERNAL hidden width)
- `embedding_length_out = 2816`  (== TARGET n_embd; this is `n_embd_out()` = `n_embd_inp()`)

So Gemma4 `mtp-*.gguf` is a **separate-file MTP head** — NOT a vocab-matched classic
draft model, and NOT embedded in the target GGUF. It is effectively a **third
runtime mode**.

---

## How llama.cpp handles `gemma4-assistant` at runtime (authoritative)

- The assistant GGUF is loaded as its **own** `llama_model` (arch `gemma4-assistant`).
  `nextn_predict_layers = 4 = n_layer_all` => the whole model is the MTP head, no
  backbone layers.
- `embedding_length=1024` is the assistant's internal compute width;
  `embedding_length_out=2816` is the target's hidden size, so the speculator's
  dimension-match assertion (`n_embd_out(assistant) == n_embd(target)`) passes.
  **Mirror/embd buffers must be sized to TARGET n_embd (2816), not 1024.**
- Context creation **REQUIRES** `params.ctx_other = target_context` (throws if null).
  The assistant graph pulls `model_other->tok_embd` from the target via `ctx_other`.
- The target exposes post-final-LN hidden states via `t_h_nextn` /
  `llama_set_embeddings_nextn(ctx_tgt, true, masked=false)`; draft set with
  `masked=true`.
- `common/speculative.cpp`: `is_mem_shared = (llama_get_ctx_other(ctx_dft) == ctx_tgt)`
  = **true** for Gemma4. In that path:
  - SKIP the catch-up decode that mirrors the target batch into the draft KV.
  - All draft tokens decode at the **same** KV position `dp.n_past`
    (Gemma4 position-sharing convention).

### KV-memory sharing semantics (this is the key safety finding)

When the assistant context is created with `ctx_other = target` it shares the
target's `llama_memory` (`mem_other`). Inside the `iswa` KV cache:

- The assistant's own 4 transformer layers (1024-dim) get **freshly allocated**
  K/V tensors — decode writes only into the assistant's own memory.
- `share(il) >= 0` layers **alias the target's K/V tensor pointers directly** and
  the cell-occupancy table (`v_cells_impl`) is **shared** with the target.

Safety of draft-side ops (verified against the fork):
- `MemorySeqRm(draft_mem, ...)` → **SAFE**. For shared sub-caches `seq_rm` is an
  immediate no-op (`if (other) return true;`); for the assistant's own sub-cache it
  only touches the assistant's own cells. Will NOT corrupt the target.
- `StateSeqGetData(draft_ctx, ...)` → **SAFE** (read-only).
- `StateSeqSetData(draft_ctx, ...)` → **NOT SAFE**. For shared layers it writes
  through aliased tensor pointers into the **target's** KV buffer. Restore state on
  the TARGET context only.
- `common/speculative.cpp` never does `seq_rm`/state-restore on the draft in the
  `is_mem_shared` path; it relies entirely on the target for shared-layer KV.

---

## Current runtime MTP gating (today)

`sdk/kronk/model/draft_mtp.go` -> `selectAndLoadDraft`. Two modes today:

1. `cfg.DraftModel != nil && cfg.DraftModel.IsSeparate()` -> classic separate
   draft via `loadDraftModel`.
2. else if target GGUF has an MTP head (`mtpNextNLayers(targetModel) > 0`) and
   `MTPAvailable()` -> embedded MTP via `loadDraftModelMTP` (Qwen3.5 path).

There is **no family-name gating** today. The embedded path creates the MTP context
from the SAME target model via `InitFromModel(targetModel)` with its OWN KV, and the
engine (`batch_mtp.go`) mirror-replays the target batch into the draft KV (the
`is_mem_shared == false` path). This is the working Qwen path and must stay intact.

`Path.MTPFile` resolution exists in `sdk/tools/models` but is **not wired into the
runtime `model.Config` yet**.

---

## yzma v1.17.1 — APIs available (already verified present)

- `llama.ContextParams.CtxOther` (llama_context*) — comment: "required for
  GEMMA4_ASSISTANT (MTP draft model), where ctx_other must be the target context"
- `llama.ModelNEmbdOut(model)` -> `llama_model_n_embd_out`
- `llama.ContextTypeMTP`
- `SetEmbeddingsPreNorm` / `GetEmbeddingsPreNorm` / `GetEmbeddingsPreNormIth`
  wrappers in `sdk/kronk/model/yzma.go`

---

## Planned implementation (runtime) — minimal correct path

A second MTP mode is required; the existing mirror path CANNOT be reused as-is
because Gemma4 is `is_mem_shared` (no catch-up decode, fixed draft position, shared
KV). Add `mtpSharedKV bool` + `ownsModel bool` on `draftModel` rather than touching
`DraftModelConfig`.

### Files to change

1. `sdk/kronk/model/config.go`
   - Add top-level `MTPFile string` to `Config`.
   - Add `WithMTPFile(v string) Option`.
   - Include in `Config.String()`.
   - `validateConfig`: `CheckModel(cfg.MTPFile, true)` when set.

2. `sdk/tools/models/kronkresolve.go`
   - Wire `out.MTPFile = fp.MTPFile` (next to `out.ProjFile = fp.ProjFile`).

3. `sdk/kronk/model/model.go`
   - `draftModel`: add `mtpSharedKV bool` and `ownsModel bool`.
     - separate classic draft: `ownsModel=true, mtp=false`
     - embedded MTP (Qwen): `ownsModel=false, mtp=true, mtpSharedKV=false`
     - Gemma4 assistant file: `ownsModel=true, mtp=true, mtpSharedKV=true`
   - `Unload`: free model when `ownsModel` (replace `if !m.draft.mtp` ModelFree).
   - IMC draft KV store init: gate to `mtp && !mtpSharedKV` (shared mode has no
     separate draft KV to snapshot; keep `pendingH` only).

4. `sdk/kronk/model/draft_mtp.go`
   - Add `probeGemma4AssistantMTPFile(file)` (read GGUF header:
     `general.architecture == gemma4-assistant` + `nextn_predict_layers > 0`).
   - Add `loadDraftModelMTPFromFile`:
     - load assistant via `buildModelParams`/`loadModelWithEnvGuard` with
       `mtpCfg.ModelFiles = []string{cfg.MTPFile}` (target hardware placement,
       NOT DraftModelConfig).
     - validate `ModelNEmbdOut(assistant) == ModelNEmbd(target)` (2816).
     - context params: `CtxType=MTP`, `CtxOther=targetCtx`, inherit
       NCtx/NBatch/NUbatch/NSeqMax/threads/FA/cache types/offload.
     - embd buffers sized to TARGET n_embd (2816).
     - `SetEmbeddingsPreNorm(targetCtx, true, false)` before any decode;
       `SetEmbeddingsPreNorm(dCtx, true, true)`.
     - do NOT `MemoryClear` shared draft memory.
     - set `mtp=true, mtpSharedKV=true, ownsModel=true`.
   - `selectAndLoadDraft` priority:
     1. explicit `cfg.DraftModel.IsSeparate()` (unchanged)
     2. `cfg.MTPFile` present & probes as gemma4-assistant -> `loadDraftModelMTPFromFile`
     3. embedded `mtpNextNLayers(targetModel) > 0` (unchanged)

5. `sdk/kronk/model/batch_mtp.go`
   - `mirrorTargetBatchToMTPDraft`: if `mtpSharedKV` -> new
     `captureTargetBatchForSharedMTP` (no catch-up decode; just copy last target
     pre-norm row into `s.pendingH`, set `s.draftNPast`).
   - `generateDraftTokensMTP`: if `mtpSharedKV` -> new
     `generateDraftTokensMTPShared` (decode each draft token at the SAME fixed
     `s.draftNPast`; read next `pendingH` via `GetEmbeddingsPreNormIth`).
   - `mirrorBuildChunkToMTPDraft` (IMC build): if `mtpSharedKV` -> capture
     `pendingH` only, no draft decode.

6. `sdk/kronk/model/batch_speculative.go`
   - `rollbackDraft`: in shared mode, no separate draft-KV rollback needed
     (`MemorySeqRm(draft_mem)` is a safe no-op for shared layers anyway, but skip
     to be explicit). Target rollback path unchanged.
   - Audit all `MemorySeqRm(draft.mem, ...)` / `StateSeqSetData(draft.lctx, ...)`
     sites: `StateSeqSetData(draft.lctx)` is UNSAFE in shared mode (writes target);
     must be guarded to `!mtpSharedKV`.

7. `sdk/kronk/model/batch_slot_start.go` and `batch_finish.go`
   - Guard every draft-side KV snapshot/restore/trim
     (`StateSeqGetData`/`StateSeqSetData`/`MemorySeqRm` on `draft.lctx`/`draft.mem`)
     with `!mtpSharedKV`. In shared mode keep only `pendingH` snapshot/restore, and
     fall back to `mtpDisabledForRequest` on IMC cache hits (same as today's
     "no draft snapshot" rationale).

### Safety invariant

In `mtpSharedKV` mode, NEVER call `StateSeqSetData(draft.lctx, ...)` (writes through
aliased pointers into the target KV). `MemorySeqRm(draft.mem, ...)` and
`StateSeqGetData(draft.lctx, ...)` are safe but should be skipped in shared mode for
clarity. Restore state on the TARGET context only.

### What CANNOT be verified without GPU + the real model

- `InitFromModel(assistant, {CtxType=MTP, CtxOther=targetCtx})` actually succeeds
  and yzma's `CtxOther` binding matches llama.cpp pointer semantics.
- Exact `general.architecture` spelling in the released GGUF (normalize `_`/`-`).
- `ModelNEmbdOut(assistant) == 2816`.
- Fixed-position drafting correctness; acceptance > 0.
- No KV corruption under: partial reject / all accept / EOG / IMC restore /
  `NSeqMax > 1`.
- Acceptance/perf on Metal/CUDA.

### Post-edit chain (per AGENTS.md / writing-go)

```
gofmt -s -w <changed .go files>
go vet ./sdk/kronk/model ./sdk/tools/models
staticcheck ./sdk/kronk/model ./sdk/tools/models
go build ./...
# tests (never repo-wide, never from sdk/kronk/tests):
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE=<repo root>
go test ./sdk/tools/models/...
```

---

## Local on-disk example state

`~/.kronk/models/unsloth/gemma-4-26B-A4B-it-GGUF/`:
- `gemma-4-26B-A4B-it-UD-Q4_K_M.gguf`
- `gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf`
- `mmproj-gemma-4-26B-A4B-it-UD-Q4_K_M.gguf`
- `mmproj-gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf`
- `mtp-gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf`

Catalog: `~/.kronk/catalog/catalog.yaml` (schema v2).
