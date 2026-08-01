# Kronk — MTP / reasoning-derailment findings

`unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL` (hybrid recurrent+attention MoE, MTP speculative decoding,
Metal), verified against llama.cpp **b10211** and yzma **v1.21.0**. llama.cpp's own `server`/`common`
code is the reference; each finding is a divergence from it. No production source was modified. Every
finding is pinned by a test that fails on purpose until fixed.

yzma defects are in **`yzma-findings.md`** (separate, standalone). Round 1 is in `findings.md`.

Reported symptoms: (1) the model contradicts what it said earlier; (2) it starts a task and drops it
mid-way. **(2) is reproduced and attributed to Kronk — §14.** (1) is partially reproduced but
statistically underpowered — §14e.

| # | Issue | Sev | Explains |
|---|---|---|---|
| §14 | Completed tool call discarded, reported as a clean `stop` | HIGH | **task abandonment (reproduced)** |
| §14a | No `length` finish reason, so truncation is undetectable | HIGH | task abandonment |
| §8a/§8b | Post-render regex mangles assistant headers, deletes bytes from user messages | HIGH | contradiction |
| §8c | In-turn `reasoning_content` deleted where the template replays it | HIGH | contradiction |
| §5 | IMC decode treats `llama_decode` failure as success → permanently truncated context | HIGH | both |
| §2 | Model's own `general.sampling.*` defaults read then discarded | HIGH | contradiction/looping |
| §9 | Every token accepted into the sampler chain twice | HIGH | looping |
| §6a/§12c | Released slot's staged rows decoded into a reassigned sequence | HIGH | both |
| §12a | Tool call held in the parser buffer discarded at EOG — no drain | HIGH | task abandonment |
| §11a | Embedding NULL result escalated to a permanent engine kill | HIGH | — |
| §10a | MTP never deactivates when Kronk's own throttle/disable logic fires | MED-HIGH | both |
| §2b/§2d/§12b | DRY unusable; penalty window never seeded with the prompt | MED-HIGH | looping |
| §3 | Draft width never clamped by remaining context or token budget | MED-HIGH | — |
| §6b | Engine-global `promptBuf` aliased onto every slot | MED-HIGH | both |
| §2e | Speculative verify samples from an unpenalized distribution | MED | looping |
| §11b | Rerank pair omits the `[EOS][SEP]` boundary | MED | — |
| §3b, §3c, §4, §8d, §9b, §10c, §12d, §12e, §1a, §1b, §5b, §6c, §8e | see sections | MED–LOW | — |

Marked **(may be intended — confirm)**: §1a, §2, §3c, §4, §8c. Kronk deliberately provides no way to
disable MTP from configuration; that is not reported (§10 covers only the deactivation defects).

Fix order: §5 and §12a/§14 first (small, localized, silently destroy user-visible work), then §8a-§8c
(one function, corrupts every agentic turn), then §2 with §9 (they interact — the anti-repetition
machinery is currently either off or corrupt), then §10a.

Per `AGENTS.md` this is a source change requiring `plan-change` and an approved plan.

---

## 0. Verified facts about the model itself

Read directly from the GGUF with a strict parser (all 55 KV pairs consumed cleanly — the
vendored `gguf-py` dump script is unusable here, it needs numpy which isn't installed):

| Key | Value |
|---|---|
| `general.sampling.top_k` | **20** |
| `general.sampling.top_p` | **0.95** |
| `general.sampling.temp` | **1.0** |
| `qwen35moe.nextn_predict_layers` | **1** |
| `qwen35moe.context_length` | 262144 |
| `qwen35moe.rope.freq_base` | 10000000.0 |
| `qwen35moe.rope.dimension_sections` | [11, 11, 10, 0] |
| `tokenizer.ggml.add_bos_token` | **False** |
| `tokenizer.ggml.eos_token_id` | 248046 |

Three of these directly contradict Kronk's behaviour and drive §2 and §3 below. There are **no
SWA / sliding-window keys in the file at all**, which is the first half of the answer to the
sliding-window hypothesis.

---

## 1. Sliding-window attention — two SWA bookkeeping defects

*This model has `n_swa == 0` (no `attention.sliding_window` key; hybrid attention + recurrent), so SWA
is not a source of wrong output here. The two defects below are real regardless: they concern how Kronk
reports and sizes SWA state, and they will matter on any model that does use a window.*

### 1a. `calculateVRAMDiag` sizes the SWA KV cache with the wrong nil-default — MED

`model.go:1331` passes `SWAFull: cfg.SWAFull()`, and that accessor (`config.go:445-448`) returns
`boolOr(PtrSWAFull, false)` — its own doc says callers needing the *effective* value must inspect
`PtrSWAFull` directly. Since `config.go:961-967` only overwrites when the pointer is non-nil, unset
means **true** at runtime. `gguf/kvcache.go:83-89` (a faithful port of the upstream allocator) then
shrinks SWA layers to `PAD(min(base, n_swa*seq+n_ubatch), 256)`, under-reporting
`ModelInfo.SlotMemory` / `VRAMTotal` — while `pool/llama.go:315-319` and
`sdk/tools/models/analyze.go:271` resolve nil to **true** for the same config, so the two estimates
disagree with each other. Pinned by `TestCalculateVRAMDiagResolvesEffectiveSWAFull`.

### 1b. Effective `n_swa` is never surfaced — LOW

`model.go:637-643` logs only the *requested* `ctxParams.SwaFull`, and `sdk/kronk/init.go:134-140`
mutes llama.cpp's own load banner by default (which prints `n_swa = %u` at
`llama-model.cpp:1785`). yzma already exports `llama.ModelNSWA`
(`.extras/yzma/pkg/llama/model.go:534`) and **nothing under `sdk/` calls it**. Establishing the
headline fact above required dumping the GGUF and reading llama.cpp's per-arch loader by hand — an
operator cannot distinguish a windowed model on a compact cache from a non-SWA model.
Pinned by `TestKronkSurfacesEffectiveNSWA`.

Adjacent, not pinned: `gguf.ParseAttentionFacts` / `CalculateKVCache` have no notion of recurrent
layers, so all 41 `qwen35moe` blocks are sized as attention KV though only ~10 are (over-estimates,
so safe).

---

## 2. Kronk ignores the model's own recommended sampling parameters — HIGH

*Distinct from `findings.md` §6a: that finding is that temperature 0 is unreachable (a clamp bug).
This one is that the model's own shipped defaults are read and then thrown away.*

`params.go:688-760` hardcodes `DefTemp = 0.8`, `DefTopK = 40`, `DefTopP = 0.9`. The model ships
`general.sampling.temp = 1.0`, `top_k = 20`, `top_p = 0.95` (§0), and
**`models.go:119-146` already loads those keys** — they are read and then never used for defaults.

llama.cpp copies GGUF sampling metadata into `common_params_sampling` on every model load unless
the caller explicitly overrode that field (`common/common.cpp:1142-1198`, called at `:1266`).

So the default serving configuration runs Qwen3.6 at a **lower temperature on a more aggressively
nucleus-clipped candidate set** than the model was tuned for. That is the documented
repetition-and-derailment regime for Qwen reasoning models, and it applies to every request that
doesn't override all three values.

Pinned by `TestSamplingDefaultsHonorGGUFMetadata`.

### 2b. DRY is a silent no-op unless explicitly given a positive window — MED-HIGH

`DefDryPenaltyLast = 0` (`params.go:37`, applied `:701-703`, used `:791`). Both "unset" and
llama.cpp's `-1` sentinel (= context size) collapse to `0`, and `0` means **DRY disabled** — a
zero-length ring buffer whose `apply()` returns immediately (`src/llama-sampler.cpp:3200-3205`,
`:2939`). Meanwhile `params.go:792` **logs DRY as active**.

The one anti-repetition sampler a user would reach for to fix a loop cannot be turned on.
Reference: `common/common.h:245`, `server-schema.cpp:151-155`, `:557-559`.
Pinned by `TestParseParamsDryPenaltyLastKeepsDryEnabled`.

### 2c. `repeat_last_n` loses both of llama.cpp's sentinels — MED

`params.go:717-719` maps anything `<= 0` to `64`. Upstream: `-1` = penalize the whole context,
`0` = penalties **off** (`common/common.h:238`, `server-schema.cpp:126-128`, `:552-555`). A caller
disabling the repeat penalty instead gets it applied over a 64-token window — which in long
reasoning output suppresses exactly the connective and identifier tokens that hold a chain of
thought together. Pinned by `TestParseParamsRepeatLastNSentinels`.

### 2d. Penalty / DRY history is never seeded with the prompt — MED

The only accept site is `batchgen_tokens.go:63`; the chain is created at
`batchgen_slot_start.go:96-97`. llama.cpp accepts **every prompt token** into the chain before
generation begins (`server-context.cpp:375-393`, `init_sampler`, called at `:3571`), so penalties
and DRY span the whole conversation.

Kronk's penalty window contains only the current turn's output. Each new turn therefore starts
penalty-free over everything it just said — so the model is free to re-say it. For a multi-turn
reasoning conversation this is a direct mechanism for the reported "I already said that" loop.
Pinned by `TestSamplerPenaltyHistoryIsSeededWithPrompt`.

(Per-request reset itself is correct: a fresh chain per request, freed at `batchgen_finish.go:444`.)

### 2e. Speculative verification samples from a different distribution than the real chain — MED

`logprobs.go:129` (`applySamplerFilters`, called from `batchgen_speculative.go:498`, `:543`, `:599`)
is a hand-rolled filter: suppress → top-k → top-p → min-p → temp. **No Penalties, no DRY, no XTC.**
The chain the request actually uses is built at `params.go:783-825`.

So `p_target` / `q_draft` and the rejection resample all come from an *unpenalized* distribution.
Enabling MTP silently disables repetition control on the verified positions. llama.cpp verifies
every draft position through the identical full chain
(`server-context.cpp:3862` → `common/sampling.cpp:646-674`).

This is **distinct from** the round-1 bonus-token bypass — that was one token per fully-accepted
round; this is every verified position. Pinned by `TestSpeculativeVerifyTargetProbsIncludePenalties`.

### 2f. Adaptive-p is mis-ordered and then discarded — MED-LOW

`params.go:813-816` appends adaptive-p, then `:819-821` appends temp and `:823-825` appends `dist`
unconditionally. adaptive-p is a *selecting* sampler that overwrites all logits with a
peak-at-target curve; llama.cpp appends it **last and instead of** `dist`
(`common/sampling.cpp:363-381`, `src/llama-sampler.cpp:3311-3373`). As written, adaptive-p's pick is
thrown away, its EMA never updates, and the emitted token is drawn from its synthetic curve by a
downstream `dist`. Pinned by `TestToSamplerAdaptivePIsTheFinalSelector`.

### 2g. DRY built with no sequence breakers — LOW-MED

`params.go:791` passes `nil`; llama.cpp defaults to `{"\n", ":", "\"", "*"}`
(`common/common.h:257`, `common/sampling.cpp:329-339`, `server-task.cpp:115`). DRY then matches
n-grams *across* line, quote and colon boundaries, so lists, repeated JSON keys and echoed
identifiers accumulate `base^(len-allowed)` penalties on structurally required tokens.
Pinned by `TestToSamplerDryUsesDefaultSequenceBreakers`.

### 2h. `top_k` cannot be disabled — LOW

`params.go:733-735` maps `<= 0` to `40`; upstream documents `0` = disabled
(`common/common.h:229`, `server-schema.cpp:89-91`). `{"top_k": 0}` from an OpenAI-style client
becomes a hard 40-token truncation on a 248320-token vocab.
Pinned by `TestParseParamsTopKDisableSentinel`.

## 3. Draft width is never clamped by remaining context or token budget — MED-HIGH

*Related to but distinct from `findings.md` §6b. Round 1 observed that nothing guards `s.nPast`
during generation. This is the specific upstream mechanism that is missing — a per-round draft-width
ceiling — and it makes the §6b overrun reachable `nDraft` tokens earlier than a plain round would.*

`chooseNDraft` (`batchgen_speculative.go:147`) branches **only** on `s.specAccEMA`. It cannot see
`s.nPast`, `ContextWindow`, or `MaxTokens`. Call sites: `batchgen_speculative.go:181`,
`batchgen_mtp.go:258`, `batchgen_mtp.go:671`.

llama.cpp computes a hard ceiling every round (`server-context.cpp:473-478`):

```cpp
int n_draft_max = n_ctx - prompt.n_tokens() - 2;   // reserves 2 cells
if (n_remaining > 0) n_draft_max = std::min(n_draft_max, n_remaining - 1);
```

A speculative round writes positions `nPast..nPast+nDraft`, so it needs `nDraft` more KV cells than
the plain round it replaces. At `nPast == CW-2` Kronk still drafts 2, `llama_decode` returns 1, and
`batchgen_engine.go:464-475` **fails every active slot** — any co-resident conversation dies with
it. The same gap makes Kronk draft a full round with one token of budget left, so the slot finishes
mid-verify and the work is discarded.

Pinned by `TestChooseNDraftClampsToRemainingContext`.

### 3b. Prompt admission gate is off by one — MED

`batchgen_slot_start.go:839` (also `:1012`, `:1112`) uses `s.nPrompt > cfg.ContextWindow()`.
llama.cpp uses `>=` (`server-context.cpp:3188`: `slot.task->n_tokens() >= slot.n_ctx` →
`ERROR_TYPE_EXCEED_CONTEXT_SIZE` + `slot.release()`) precisely because a slot needs one free cell to
decode into.

A prompt of exactly `ContextWindow` tokens is admitted, pays for a full-window prefill, and then the
first generation decode requests position `ContextWindow`; `llama_decode` returns 1 and every active
slot is failed with the generic "context window is full" (`batchgen_errors.go:81`) instead of an
actionable up-front rejection. Pinned by `TestPromptAtExactContextWindowIsRejected`.

### 3c. An explicit `ContextWindow` is never capped at the trained context — MED

`adjustContextWindow` (`config.go:804-807`) returns immediately for any explicitly-set window; the
GGUF `context_length` read at `:811-818` and the `min()` at `:825` are auto-pick-only, and
`validateConfig` (`config.go:525`) never inspects it. llama.cpp caps `n_ctx_slot` at `n_ctx_train`
(`server-context.cpp:1294-1297`) and assigns the capped value to every slot (`:1349`); the C API
alone only warns (`llama-context.cpp:320-322`).

That value is both the prompt limit (`batchgen_slot_start.go:839`) and the default `MaxTokens`
(`params.go:705-706`), so `WithContextWindow(N)` beyond `n_ctx_train` silently generates out to
positions RoPE never saw — no error, just degradation that reads as the model losing the thread.
This model advertises 262144, so the auto-pick (8192) is safe; the explicit path is not.
Pinned by `TestPickedContextWindowNeverExceedsTrainedContext`.

## 4. Flash Attention is forced ON, bypassing llama.cpp's capability probe — MED

**The premise that "Kronk doesn't enable FA for this model" is false.** With default config Kronk
passes `flash_attn_type = LLAMA_FLASH_ATTN_TYPE_ENABLED` (1):

- `config.go:1255-1259` — `DerefFlashAttention(nil)` → `FlashAttentionEnabled`
- `config.go:441-443` — an unset `PtrFlashAttention` derefs to that zero value
- `config.go:880-881` — mapped to `llama.FlashAttentionTypeEnabled`

llama.cpp's own default is `LLAMA_FLASH_ATTN_TYPE_AUTO` (-1) — `llama-context.cpp:3493`,
`common/common.h:496`. Enum values confirmed at `llama.h:190-194` (AUTO=-1, DISABLED=0,
ENABLED=1); yzma's constants (`llama.go:163-165`) match exactly.

**Why forcing ENABLED is not equivalent to AUTO.** `ENABLED` sets `cparams.auto_fa = false`, so
`resolve_fused_ops`' FLASH_ATTN capability probe never runs (`llama-context.cpp:246-247`,
`:549-552`). That probe compares the fused node's device against the layer's device and, on a
mismatch, logs *"Flash Attention not supported, set to disabled"* and falls back to the non-FA
graph. Forced ENABLED keeps `ggml_flash_attn_ext` in the graph and lets the scheduler place it on
whatever backend accepts it — typically CPU — while K/V live on the accelerator. The result is a
silent per-layer, per-token round trip with no error on either side. It also suppresses the
AUTO-only promotions at `:3546-3548` and `:3562-3564`.

Pinned by `TestFlashAttentionUnsetResolvesToLlamaAuto`. Note `config_test.go:104` currently pins
the *wrong* default and must be updated in the same commit as the fix.

### 4b. No pre-flight guard for quantized V cache + FA disabled — MED

`config.go:525-674` has no validation rule for the one combination llama.cpp rejects outright
(`llama-context.cpp:3561-3569`: DISABLED → error + `nullptr`, re-checked at `:458-461`).

Default config is safe (`CacheTypeV` stays `GGMLTypeAuto` → F16). But AutoTune can *generate* the
invalid pair on a CPU-only, RAM-tight host: `sdk/tools/models/analyze.go:374-381` hardcodes
`flash-attention: disabled` when no GPU is present, while an independent f16→q8_0 fallback
(`analyze.go:439`, `:462-464`, `:622-650`) sets `cache-type-v: q8_0`. Nothing validates the
combination, so it surfaces as a generic nil-context error with llama.cpp's actual explanation
buried in the C log.

Pinned by `TestValidateConfigRejectsQuantizedVCacheWithFlashAttentionDisabled`.

### 4d. Observability gap (no test possible)

`logContextParamsTrace` (`model.go:623-645`) logs only the *requested* mode, and `llama.h` exposes
no getter for the resolved value (only `llama_flash_attn_type_name`, `llama.h:196`). Once §1 is
fixed to AUTO, the effective mode will be visible only in llama.cpp's own stderr.

### 4e. Adjacent, out-of-area

`model.go:892-898` (the non-MTP draft loader) copies FA/threads/batch but drops `TypeK`/`TypeV`, so
a quantized target KV cache silently gives the draft an F16 cache.

---

## 5. IMC prompt-cache decode treats every `llama_decode` failure as success — HIGH

`caching_imc.go:118` and `batchgen_mtp.go:407` (the latter is the path actually taken for this MTP
model — dispatch at `batchgen_slot_start.go:447-452`) both write:

```go
if _, err := llama.Decode(m.lctx, batch); err != nil {
```

yzma returns the `llama_decode` status as the **first** result and a non-nil error *only* when the
context handle is 0 (`.extras/yzma/pkg/llama/context.go:381-390`). The `err != nil` guard is
therefore dead code, and both functions always return nil.

`llama.h:964-976` is explicit that the status matters: `1` = could not find a KV slot, `-1` = invalid
input batch (reachable on this hybrid IMROPE target via `llama-batch.cpp:255-270`), `2` = aborted
with partial ubatches left in memory.

On a discarded failure, `startSlot` sets `cacheIdx = job.imcNewTotalCached`
(`batchgen_slot_start.go:471`), commits that token count to the session (`:474`), and snapshots a KV
that is **missing those cells**. The session now advertises a prefix that isn't resident, the suffix
is decoded over the hole, and **every later turn restores the same truncated snapshot** — so earlier
messages are permanently gone from the model's context while the conversation transcript still shows
them. That is precisely "it contradicts what it already said" and "it drops the task mid-way".

llama.cpp instead binds `ret`, classifies it, and calls `slot.prompt_clear()`
(`server-context.cpp:3639-3691`).

The three sibling decoders in `caching_imc_media.go` (`:236-244`, `:312-323`, `:377-388`) already do
this correctly — `ret, err := llama.Decode(...)` then `if err != nil || ret != 0`. So this is an
isolated oversight on the two text paths, not a systemic pattern.

Pinned by `TestIMCCacheDecodeChecksLlamaDecodeStatus`.

### 5b. Documented `cachedRenderInputHash` pure-hit guard does not exist — LOW

The field is written at 3 sites (`model.go:94`, `caching.go:205`, `batchgen_slot_start.go:157`) and
compared at **0**. Both staleness predicates (`batchgen_slot_start.go:169-176`, `:541-551`) omit it,
and there is no `IMCPureHitSnapshotSkip` symbol. `caching_imc_pure_hit_test.go:308-323` is named for
a guard it never asserts. Not wrong output today — the token-exact prefix compare subsumes every
fingerprint input — but the next relaxation of the skip predicate will be made believing a backstop
exists. Reference: `server-common.cpp:471-518` + `server-context.cpp:3197-3207`.
Pinned by `TestIMCPureHitSkipGuardReadsCachedRenderInputHash`.

## 6. Slot / sequence cross-request contamination

### 6a. A released slot's already-staged batch rows are decoded into the sequence `finishSlot` just cleared — HIGH

`batchgen_engine.go:375`/`:381` release a slot from *inside* the prefill staging loop (`:366-396`),
which makes multiple passes staging rows into the shared `e.batch` via `addPrefillChunk`.

A client disconnect between passes releases the slot mid-iteration. `finishSlot` does
`MemorySeqRm(seq,-1,-1)` (`batchgen_finish.go:179`) but **cannot un-stage the rows already in
`e.batch`**, and `e.batch` is only cleared at the top of the *next* iteration. So the single
`llama.Decode` at `batchgen_engine.go:458` writes the dead request's prompt back into the
just-wiped sequence. Worse, `fillSlots` (`:418`) runs before that decode and may already have started
the next conversation on the same slot/seq — both row sets then land in one sequence. Because KV
appends by slot rather than by `(seq,pos)`, the stale cells are never overwritten, and the causal
mask admits them as generation walks past.

llama.cpp avoids this three ways: `prompt_clear()` wipes KV and token bookkeeping atomically
(`server-context.cpp:291-296`), cancellations are applied *before* `update_slots` builds the batch
(`:1449-1450`), and it re-trims past the validated prefix (`:3418-3423`) — Kronk has no equivalent.
Pinned by `TestPrefillStagingLoopDoesNotReleaseSlots`.

### 6b. Engine-global `draftCore.promptBuf` is aliased onto every slot's `draftPromptTokens` — MED-HIGH

`batchgen_slot_start.go:935` (refill at `:916-932`) hands out a reslice of one engine-global array
(field at `model.go:137`). `fillSlots` starts every pending job in one pass, so two `startSlotText`
calls run back-to-back and the second refills the *same* array in place — slot 0's
`draftPromptTokens` then reads job B's tokens.

`prefillDraft` (deferred to the next iteration, gated on `prefillDone`) decodes that mixed sequence
into slot 0's draft KV and persists it as `draftCachedTokens`, which `reset()` deliberately keeps —
so a wrong-conversation prefix is "reused" on later requests, and the persisting `specAccEMA` latches
speculation off. Upstream keeps prompt tokens in the per-slot `server_slot::task`
(`server-context.cpp:3116`), reset at `:364`.

Scope limit: classic separate-GGUF drafting only. **MTP skips `prefillDraft`**, so this does not fire
on the MTP path. Pinned by `TestDraftPromptTokensAreSlotOwned`.

### 6c. `slot.reset()` omits `startTime` and `specSnapshot` — LOW

`batchgen_slot.go:300` (fields at `:293`, `:263`). `startTime` is set only on the first output token
(`batchgen_tokens.go:97`) but read unconditionally at `batchgen_finish.go:149-150`, so a request that
dies before its first token reports the *previous* request's `elapsed` / `TokensPerSecond`.
`specSnapshot` survives as a restorable serialized copy of the previous conversation's sequence
state; `len(specSnapshot) > 0` is the "snapshot exists" gate at `batchgen_speculative.go:684` and
those bytes are fed to `StateSeqSetData` at `:823`. Upstream clears both
(`server-context.cpp:3124`, `:351`). Pinned by `TestSlotResetClearsPerRequestClockAndSpecSnapshot`.

## 8. Post-render reasoning strip corrupts the prompt — HIGH

**Two independent agents converged on this from opposite directions** (reasoning-history and
tokenization/template). It is the strongest explanation found for the *reasoning-specific* nature of
the symptom, and it is a plain-text bug with no KV cache involved.

Ground truth established first: the GGUF's embedded `tokenizer.chat_template` is **byte-identical**
to `.extras/templates/qwen3.6.jinja` (sha256 `55d4931433…`), `general.architecture = qwen35moe` (so
`parsers/qwen` is selected correctly — the `mtp-` id prefix is never consulted), `<think>`/`</think>`
are single tokens 248068/248069, and the generation prompt force-opens `<think>\n` when
`enable_thinking` — which Kronk already handles correctly at `batchgen_slot_start.go:37-53`.
Kronk's Jinja engine was verified byte-for-byte against Jinja2 3.1.6 across 21 conversation shapes,
so **the template engine itself is faithful**. The damage is done *after* rendering.

`prompts.go:177-181` rewrites the finished prompt with `StripEmptyReasoning` →
`parsers/standard/reasoning.go:35` (`StripEmptyThink`) → `:43` (`StripExceptTrailing`), a bare regex
pass over the whole string. llama.cpp's only post-render edit is BOS/EOS de-duplication
(`common/chat.cpp:926-940`); it never rewrites template output.

### 8a. Assistant role headers mangled in agentic history — HIGH

Template line 104 emits `'<|im_start|>' + role + '\n<think>\n' + reasoning + '\n</think>\n\n' + content`
as one literal, for every turn where `loop.index0 > ns.last_query_index`. The regex deletes only the
`<think>…</think>` span and leaves the surrounding `\n\n`, so the turn reaches the model as:

```
<|im_start|>assistant\n\n\n<tool_call>        ← actual
<|im_start|>assistant\n<think>\n\n</think>\n\n<tool_call>   ← what the template emitted
```

Two special tokens (248068/248069) degrade into ordinary newlines on **every assistant turn of a
tool-calling loop**. Gated only on `IncrementalCache()` (default on), so no client cooperation is
needed to trigger it. Pinned by `TestQwen36PostRenderReasoningStripBreaksAssistantTurns` (3 failing
subtests; a plain-chat control subtest passes, which is what proves the Jinja engine is innocent) and
by `TestQwen36EmptyThinkScaffoldStrippedFromInTurnAssistant`.

### 8b. The same pass deletes bytes out of USER message content — HIGH

`StripExceptTrailing` has **no role awareness** — it regexes the entire rendered prompt. A user who
writes `explain <think>\n</think> markers` has those bytes silently removed and receives
`explain  markers`. Unlogged, and nothing upstream escapes user content against this because upstream
has no such pass. Pinned by `TestQwen36PostRenderReasoningStripDeletesUserContent`.

### 8c. In-turn `reasoning_content` deleted where the template would replay it — MED-HIGH

`normalizeHistoryReasoning` (`reasoning.go:91-92`, gated at `:36-38`) deletes `reasoning` and
`reasoning_content` from **every** assistant message. But template line 103 deliberately replays
reasoning when `loop.index0 > ns.last_query_index`, and llama.cpp passes `inputs.messages` verbatim
(`common/chat.cpp:895-900`) with no stripping pass at all.

In a multi-step tool loop, the reasoning that justified a tool call belongs to the *same logical
turn* as the tool result. Kronk deletes it, so the model resumes having lost its own justification
and re-derives a different one — **this is the "I already said that / that's a contradiction" mode
directly.** The doc comment at `reasoning.go:8-18` assumes templates always drop reasoning once a
newer turn arrives; false for this template. Pinned by
`TestQwen36InTurnReasoningContentNotReplayed`.

### 8d. `addBOSToken` re-derived from a metadata string instead of the vocab flag — MED (latent here)

`model.go:297-302`, `:336` initialise `addBOSToken := true` and lower it only when the GGUF string is
literally `"false"`. llama.cpp's `add_bos` defaults **false** and is raised only for SPM/WPM
(`src/llama-vocab.cpp:2374-2394`, `:2547-2568`), and it force-overrides `add_bos = true` for Gemma 4 —
which a string check cannot see. So a gpt2/BPE GGUF that omits the key gets a spurious BOS from
Kronk. Kronk already calls `llama.VocabGetAddBOS` correctly on the media path
(`prompt_plan.go:35`), so the two paths can disagree for one model.

Latent for *this* model (`add_bos_token = false` is present explicitly, so both implementations skip
BOS). Pinned by `TestAddBOSTokenUsesTheVocabFlag`.

### 8e. Streaming and non-streaming messages disagree — LOW-MED

`batchgen_tokens.go:212-222` writes content to `finalContent`/`finalReasoning` *before* `:225-230`
decides via `isUnnecessaryCRLF` whether to stream it, so a suppressed newline survives only in the
non-streaming message: `Chat()` returns `"\n\nB"` where `ChatStreaming()` delivers `"B"` (and
`"\nA"` vs `"A"` on the reasoning channel). Upstream streams diffs of the accumulated parsed message
(`server-task.cpp:1021` → `compute_diffs` at `:177`), so deltas concatenate to the final message by
construction. Pinned by `TestQwen36StreamedAndFinalMessageAgree`.

## 9. Every emitted token is accepted into the sampler chain TWICE — HIGH

`llama_sampler_sample` **ends with `llama_sampler_accept(smpl, token)`**
(`src/llama-sampler.cpp:810-875`, accept at `:873`). That is exactly why upstream's
`common_sampler_sample_and_accept_n` uses `llama_sampler_apply` plus exactly **one** explicit accept
per emitted token (`common/sampling.cpp:646-676`).

*Distinct from `findings.md` §6i, which is about `DraftGenerate` accepting a token internally that
the caller never receives. This is Kronk's own call sites accepting a token that llama.cpp already
accepted for them — and unlike §6i it is not latent.*

Kronk calls `llama.SamplerSample` (yzma `sampling.go:574`, a thin wrapper on
`llama_sampler_sample`) and then calls `llama.SamplerAccept` **again** in `handleSampledToken`
(`batchgen_tokens.go:63`).

**Not speculative-specific** — it reaches the plain decode path at three sites:
`batchgen_tokens.go:25`→`:28`, `batchgen_tokens.go:265`→`:271`, and
`batchgen_engine.go:259`→`:261`, in addition to the verify loop
(`batchgen_speculative.go:433`, `:442`, `:493`, `:536`, `:590`).

**Exactly what it corrupts — both the window length and the weights, but not uniformly:**

- The penalty ring buffer's capacity *is* `penalty_last_n` (`llama-sampler.cpp:2782`,
  `ring_buffer::push_back` `:55-68`), so `penalty_last_n = 64` covers only **32 real output tokens**.
- `token_count[token]` reaches 2 (`:2663`), which doubles only the **frequency** penalty
  (`float(count) * penalty_freq`, `:2715`). Repeat and presence penalties are membership-only and are
  therefore unaffected in weight.
- **DRY is hit worst** (`:2937-2944`, ring at `:3245`): its window is halved *and* its contents are
  poisoned into `a a b b c c`, **fabricating repeats that never occurred** — an anti-repetition
  sampler actively being told the model is repeating itself.
- **Grammar is NOT double-accepted.** `grammar.go:390-427` only calls `SamplerApply`, never
  `SamplerAccept`, so the stack-machine corruption / process-abort hypothesis is **dropped**.

It is also **non-uniform** across tokens — the fully-accepted-round MTP bonus token comes from
`argmax` and is accepted only once (`batchgen_speculative.go:595`), so the penalty window advances at
a rate that varies with the acceptance rate.

**Not unconditional:** `params.go:785`/`:792` omit penalties and DRY under defaults
(`RepeatPenalty = 1.0`, freq/presence/dry `= 0`) and every remaining default sampler has
`.accept == nullptr`. So this bites only callers who enable a penalty — which, combined with §2b
(DRY cannot be enabled at all), means the anti-repetition machinery is either off or corrupt, never
simply working. Pinned by `TestVerifyLoopSamplerAcceptIsNotDoubled` and
`TestSamplerAcceptedTwicePerTokenCorruptsPenaltyWindow`.

### 9b. Phase-A verify loop sets `s.nPast` one KV cell short — MED

`batchgen_speculative.go:454` (also `:507`, `:564`) vs the authoritative `:740`. The spec batch puts
`s.sampled` at `basePast` and `draft[k]` at `basePast+1+k` (`batchgen_engine.go:284-287`), so
accepting `draft[i]` (⇒ `accepted == i+1`) leaves the next write position at `basePast+2+i`.
Phase A writes `basePast+1+i`; Phase B writes `basePast+1+accepted`, which is what upstream's
`prompt.tokens.pos_next()` yields (`server-context.cpp:3936-3942`) and what the KV is actually
trimmed to.

Masked today only because Phase B overwrites it — but **Phase B is skipped whenever Phase A's
`handleSpeculativeToken` (`:455`) reaches `finishSlot`** (EOG, MaxTokens, stream error). It also
contradicts Phase A's own documented "does NOT advance s.nPast" contract.
Pinned by `TestVerifyLoopNPastAgreesWithFinalizeNPast`.

## 10. MTP deactivation and coverage defects

*Scope note: Kronk deliberately provides no way to disable MTP from configuration — that is intended
design and is not reported here. The defects below are different: Kronk's own internal logic decides to
stand speculation down and then doesn't, plus two coverage/upgrade defects that are independent of the
design decision.*

### 10a. MTP does not actually deactivate when Kronk's own logic throttles it — MED-HIGH, IN SCOPE

*This is **not** the config question. Kronk has internal machinery whose stated purpose is to stand
speculation down — the `chooseNDraft` EMA throttle, and `disableMTPForRequestSpec` /
`mtpDisabledForRequest`. When that machinery fires, MTP does not actually stand down.*

`specAccEMA` is initialised to **1.0** (`batchgen_slot.go`), so early rounds always draft at the full
ceiling. A zero-draft round then skips only the verify/rollback/hybrid-snapshot block (gated on
`len(draftTokens) > 0`, `batchgen_engine.go:277-455`) while **still** claiming an MTP target-batch
range and running `syncAfterTargetDecode` mirror-replay every round
(`batchgen_engine.go:328-332`, `:533-541`), with the target context permanently in pre-norm mode and
`NOutputsMax` still sized `nSeqMax*(1+nDraft)`. The recovery probe
(`mtpProbeInterval`, `batchgen_speculative.go:126-166`) re-enables drafting every 32 rounds, so it can
never latch off.

Round 1's §3 is the same class and reinforces it: `disableMTPForRequestSpec`
(`batchgen_speculative.go:1081`) and `mtpDisabledForRequest` **exist but are wired to nothing**, so a
`snapshot-error` or `restore-error` — the exact conditions under which speculation should be abandoned
for the request — leaves MTP running.

So the defect is: **every internal path that is supposed to stand speculation down is either a no-op or
self-reversing.** A fix here needs a real deactivation path (a `nDraft == 0` bail in
`selectAndLoadDraft` at `draft_mtp.go:493-499`, skipping the mirror-replay, and honouring
`mtpDisabledForRequest`) — none of which requires adding a config key, so none of it conflicts with the
design decision above.

### 10c. Two integration suites have never executed a single test — MED

*This closes the open question `findings.md` §7 left ("`tests/gemma4mtp` and `tests/draft` deserve
the same audit"). The `tests/mtp` fact itself is round 1's; what is new here is the discarded error
that hides it, the verdict on the other two suites, and a regression test.*

- **`tests/mtp`**: `sdk/kronk/tests/testlib/testlib.go:97` resolves
  `mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL`, absent from the catalog (only
  `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL` at `catalog.yaml:321`). `resolveModel`
  (`testlib.go:110-116`) **discards the error**, leaving a zero `models.Path`, and
  `tests/mtp/main_test.go:20` reads that as "not downloaded" and `os.Exit(0)`. Green, silent, zero
  tests — which is how the MTP divergences from `common/speculative.cpp` shipped.
  Pinned by `TestTestlibResolvesOnlyCatalogModelIDs`.
- **`tests/gemma4mtp`**: `main_test.go:19` requires `MPMoEVision.MTPFile`, but `MPMoEVision` is
  `gemma-4-26B-A4B-it-UD-Q4_K_M` (`testlib.go:89`) whose catalog record
  (`catalog.yaml:127-145`) has no `mtp:` key — that sidecar is declared only on the Q8_K_XL entry
  (`catalog.yaml:352`), and `MTPFile` comes solely from that key
  (`sdk/tools/models/catalog.go:787-789`). So the only coverage of the shared-KV drafter
  (`draft_mtp.go:438-458`) never runs. Pinned by `TestGemma4MTPSuiteTargetShipsMTPSidecar`.
- **`tests/draft` is clean** — `Qwen3-0.6B-Q8_0` and `Qwen3-8B-Q8_0` both exist in the catalog.

---

## 11. `batchseq` engine (embed/rerank only) — two defects

`batchseq` is **not on the chat path**. It is reached only by
`(*Model).Embeddings` (`embed.go:91`) and `(*Model).Rerank` (`rerank.go:95`), selected at
`model.go:376` via `useBatchSeq` → `supportsBatchSeq` (`batchseq_compat.go:7`,
`mi.IsEmbedModel || mi.IsRerankModel`). `Chat` / `ChatStreaming` / batch-pooled generation never touch
it, so **neither reported symptom can originate here.** The two findings below are
embedding/reranking-quality bugs, reported because the audit was asked for.

### 11a. No `LLAMA_POOLING_TYPE_NONE` path; a NULL result permanently kills the engine — HIGH

`batchseq_engine.go:476` calls `llama.GetEmbeddingsSeq` unconditionally, treats a nil result as
`fatal = true` (`:478`), and fatal → `terminate` with the engine never rebuilt (`:343-347`).

Kronk sets `ctxParams.PoolingType` for **rerank only** (`config.go:844-850`), so any embed GGUF
missing `<arch>.pooling_type` — a key only written when the HF repo ships `1_Pooling/config.json`
(`conversion/base.py:2062-2075`) — resolves to NONE (`llama-context.cpp:232-236`,
`llama-hparams.h:272`). llama.cpp fills `embd_seq` only for MEAN/CLS/LAST/RANK
(`llama-context.cpp:1489-1531`, `:1934-1981`) and returns nullptr otherwise (`:924-931`).

Upstream handles both cases: it branches `NONE → llama_get_embeddings_ith`
(`server-context.cpp:2202-2207`) and degrades a NULL to a zero vector for that one request
(`:2209-2214`). Kronk instead reports "returned 0 outputs" as fatal, and **every subsequent
`Embeddings()` call fails permanently** with the stored engine error. `llama.GetEmbeddingsIth` and
`GetPoolingType` are bound by yzma but referenced nowhere under `sdk/`.
Pinned by `TestBatchSeqEmbeddingHandlesNonePooling`.

### 11b. Rerank pair omits the `[EOS][SEP]` segment boundary — MED

`rerank.go:261-263` builds the pair as `fmt.Sprintf("%s %s", query, document)`, consumed at `:157-158`
and `:209-211`. Upstream's contract is `[BOS]query[EOS][SEP]doc[EOS]`
(`server-common.h:378`), implemented by `format_prompt_rerank`
(`server-common.cpp:1553-1594`), which prefers the model's own `"rerank"` chat template and otherwise
builds it at token level gated on `llama_vocab_get_add_bos/_eos/_sep`.

`llama.Tokenize(vocab, pairText, addBOS, true)` adds only BOS/EOS at the ends, never an interior SEP,
so a `bge-reranker-v2-m3` XLM-RoBERTa cross-encoder — trained on `<s> q </s></s> d </s>` — is fed one
run-on sentence. RANK pooling still emits a score and `sigmoid` still maps it to [0,1], so **the
ordering is confidently wrong with no error**. A model-side `"rerank"` template is ignored.
Pinned by `TestFormatRerankPairSeparatesQueryFromDocument`.

## 12. Plain non-speculative path defects

**Answer to "would disabling MTP avoid this?": no — and per §10 you cannot disable it anyway.** A
plain `Chat()` always runs the `batchgen_*` engine. The defects below need no speculation to fire.

### 12a. Tool calls held in the parser buffer are silently discarded at end-of-generation — HIGH

The Qwen/standard state machines buffer tool-call bytes until they see `</tool_call>`
(`parsers/qwen/state_machine.go:88-89`, `:47-68`). `parser.go:73-80` exposes only Classify + Reset —
there is **no drain**. `finishSlot` flushes only `utf8Buf` (`batchgen_finish.go:261-278`), and
`batchgen_slot.go:379-381` calls `Reset()`, which discards.

So if EOG or MaxTokens arrives before the closing tag, the **entire tool call is thrown away**:
`finishSlot`'s tool branch (`batchgen_finish.go:282`) runs over an empty `finalTooling` and produces
an empty response with **no error**. The model announced and began an action; the caller sees nothing.
Upstream treats `generated_text` as authoritative and ships the remainder in `send_final_response`
(`server-context.cpp:1868-1898`). Pinned by `TestGenerationEndDrainsParserHeldBackContent`.

### 12b. Prompt tokens are never primed into the sampler chain — MED

`batchgen_tokens.go:63` is the **only** `SamplerAccept` on a request sampler in the whole package;
nothing in `startSlotText` (`batchgen_slot_start.go:791`) or `addPrefillChunk` primes it. Upstream's
`init_sampler()` accepts every prompt token (`server-context.cpp:375-397`).

The penalty/DRY window therefore starts empty on **every turn**, so in a long multi-turn conversation
the entire prior transcript is invisible to the repetition machinery — and it compounds with the
halved window from §9. Pinned by `TestPromptTokensArePrimedIntoTheSamplerChain`.
(Independently found as §2d; same defect, two agents.)

### 12c. Released slot's rows stay in the pending batch while its seqID is reassigned — HIGH

Independent confirmation of §6a from a second agent, with a sharper mechanism: round-robin prefill
runs multiple passes per `processBatch`, so a cancel landing between passes releases a slot whose rows
are already in `e.batch` (`batchgen_engine.go:373-383`). `finishSlot` clears the KV sequence but not
the batch (`batchgen_finish.go:176-182`), then `fillSlots` (`batchgen_engine.go:418`) hands the same
slot/seqID to a queued request which adds its own rows **at overlapping positions**. One
`llama_decode` (`:458`) commits two conversations into one sequence, and the new request attends to the
cancelled one's tokens. Also fires on an IMC hit, since restore runs inside `startSlot`, pre-decode.

Upstream guarantees assignment strictly precedes batch building — *"all tasks in the current loop is
processed, slots data is now ready"* (`server-queue.cpp:139-163`, `server-context.cpp:137`).
Pinned by `TestReleasedSlotRowsAreRemovedFromPendingBatch`.

### 12d. The token that reaches MaxTokens has its text discarded — LOW-MED

`batchgen_tokens.go:205-210` (and the partial-UTF-8 twin at `:146-151`) finish the slot **before** the
content store at `:213-222` and the delta at `:233`. The token is counted in `Usage` but never
delivered on either transport. Upstream emits the token, then checks the budget
(`server-context.cpp:1895-1899` before `:1913-1918`).
Pinned by `TestMaxTokensKeepsTheLimitReachingTokensText`.

### 12e. M-RoPE generation applies grammar during reasoning but never accepts it — LOW

`batchgen_engine.go:255-260` lacks the `&& s.reasonFlag == 0` guard that `batchgen_tokens.go:21` and
`:261` have (the Accept at `:59` *is* gated). With vision + JSON schema + reasoning, thinking tokens
are sampled from grammar-masked logits (`grammar.go:419-425` writes the −inf mask back into the
context row) while the grammar stack never advances. Upstream governs apply and accept with the same
`grammar_should_apply` predicate (`common/sampling.cpp:630-643`).
Pinned by `TestMRoPEGenerationGrammarIsGatedOnReasonFlag`.

## 13. Reproduction status — plain-conversation differential

Plain multi-turn conversation does **not** reproduce either symptom, on Kronk or upstream, so §1-§12
are verified defects with **no measured link** to observed behaviour. Only §14 (tool-calling) reproduced.

Identical GGUF/quant/build — upstream `llama-server` at b10211 from
`~/.kronk/libraries/darwin/arm64/metal/`, the same libs Kronk loads. n_ctx 16384, probes P1-P5,
temp 0.7 / top-k 40 / top-p 0.9 / min-p 0 / rep-pen 1.0, f16 KV, `-fa off`, n_seq_max 1. A second run at
n_ctx 131072, 32 turns, ~19k depth scored identically across all three legs.

| Leg | Derailments | Acceptance |
|---|---|---|
| `upstream --spec-type none` (MTP off) | 0/5 | — |
| `upstream --spec-type draft-mtp` (MTP on) | 0/5 | 0.62-0.66 |
| `kronk` (MTP auto-on) | 0/10 | 0.59-0.80 |

Upstream MTP-on vs off was byte-identical on 12/18 turns, so **speculative decoding as a technique is
not implicated**; Kronk's acceptance sits in the same band.

Three Kronk replies initially scored as "forgot the standing marker" were **false positives** — each hit
`max_tokens` exactly and ended mid-sentence. Scoring is now truncation-aware. Relevant beyond this
harness: it is the same misreading a human makes from a transcript, and §14a means the API gives a client
no way to tell the difference.

### 13a. Two measurement traps

1. Kronk's `Usage.CompletionTokens` **excludes** reasoning tokens (they are in `ReasoningTokens`);
   llama-server's includes them — comparing raw understates Kronk ~20x on thinking turns.
2. With thinking enabled both sides strip reasoning from replayed history, so turn count grows while used
   context stays small — a reasoning-heavy chat never builds a long context unless the probe forces it.

### 13b. Harness

`sdk/kronk/tests/mtp/differential_test.go`. Opt-in: build tag `kronkdiff` **and** `KRONK_MTP_DIFF=1`.
Launches `llama-server` for the upstream legs itself; skips cleanly if the GGUF or binary is missing.

```
KRONK_MTP_DIFF=1 go test -tags kronkdiff -timeout 3h -v -count=1 \
  -run 'TestMTPDifferential$' ./sdk/kronk/tests/mtp/
```

Knobs: `KRONK_MTP_DIFF_CTX`, `_PROBES`, `_REPEATS`, `_OUT`, `_SERVER`, `_UPSTREAM=0`. Set
`KRONK_MTP_DIFF_OUT` outside `/private/tmp` to keep transcripts. A `//go:build !kronkdiff` guard was
added to `sdk/kronk/tests/mtp/main_test.go`, required because its `TestMain` force-exits on the missing
catalog id (§10c).

---

## 14. A completed tool call is silently discarded at `max_tokens` — HIGH, reproduced live

Measured on the 35B MTP target, n_ctx=16384, `tools=[get_weather]`,
`enable_thinking=false`, prompt *"What is the weather in London, United Kingdom? Use celsius."*

| `max_tokens` | result |
|---|---|
| 4, 8, 12, 16, 20, 24, 32 | `finish_reason="stop"`, `content=""`, `reasoning=""`, `tool_calls=[]`, `error=nil`, `usage.output_tokens=**39**` |
| 48, 64 | `finish_reason="tool_calls"`, `get_weather` delivered, `usage.output_tokens=39` |

Note `usage.output_tokens = 39` for **every** cap, including `max_tokens=4`. That reveals two distinct
defects:

**(a) `MaxTokens` is not enforced at all while the parser is buffering a tool call.** Every tool-body
token classifies as `ChannelNone` and returns early at `batchgen_tokens.go:200-203`, **before** the
MaxTokens check at `:205-210`. A cap of 4 therefore generated 39 tokens — a ~10x overrun. On a
long tool body this is unbounded generation against an explicit caller limit.

**(b) The completed call is then destroyed by its own arrival.** The whole payload arrives as a single
`Result` at the closing tag (`parsers/qwen/state_machine.go:80-89` returns the entire buffer). At that
token `outputTokens = 39 >= MaxTokens`, so `:205-210` calls `finishSlot` **before** the content store at
`:213-222` — `finalTooling` stays empty and the caller gets an empty response.

**The model produced a well-formed, complete tool call and Kronk discarded it.** This is §12d
(max-tokens token's text discarded) amplified by the parser's single-shot flush: it destroys the
*entire* call rather than one token. Confirmed: **the failure boundary is the token count, not the
buffer state** — cap ≤ 39 loses the call, cap ≥ 40 delivers it.

**Correction to the original §12a hypothesis.** The audit predicted the loss came from the parser still
holding bytes at the cut. That route is **unreachable via `max_tokens`** — it is reached only by the EOG
check at `batchgen_tokens.go:66-69`. Both routes are real and are pinned separately: the EOG route loses
98 of 98 bytes, also with `toolFlag = 0`. Note `s.toolFlag` stays 0 because `ChannelNone` never
increments it (`batchgen_tokens.go:177-180`), so `finishSlot` never even enters the tool branch at
`batchgen_finish.go:282` — **no tool call, no log line, no error.**

### 14a. Kronk has no `length` finish reason — this is what makes §14 silent — HIGH

`models.go:36-38` defines only `FinishReasonStop` / `Tool` / `Error`, and `chatResponseFinal`
(`models.go:926-929`) picks `stop` unless tool calls exist. llama.cpp does the opposite: it **starts**
from `"length"` and downgrades only on `STOP_TYPE_WORD`/`EOS`
(`.extras/llama.cpp/tools/server/server-task.cpp:410`, `:443`, `:492`).

So a `max_tokens`-truncated turn is reported to the caller as a clean `stop`. A client has **no way to
detect truncation** — which is exactly why §14 presents as "the model just stopped doing the task"
rather than as an error. Fixing §14 without adding `length` would still leave callers blind.

### 14b. Prompt damage on a tool loop — confirmed through the real code path

Two-step tool loop, real path (`normalizeHistoryReasoning` → `applyJinjaTemplate`), IMC on (default).
**Every in-turn assistant header degrades**, on every step:

```
template says:  <|im_start|>assistant\n<think>\nOne file. Read svc/queue.go…\n</think>\n\n
Kronk sends:    <|im_start|>assistant\n\n\n
```

`<think>` openers 3→1, `</think>` closers 2→0, tokens 248068/248069 become newlines, and **both
`reasoning_content` strings are deleted outright** (`reasoning.go:91-92` + `prompts.go:177-181`).
Gated only on `IncrementalCache()`. This confirms §8a and §8c on the shape that actually triggers them,
and it is the leading unproven mechanism for the self-contradiction symptom.

### 14d. Upstream comparison — llama.cpp loses nothing, so this is Kronk's — HIGH

Identical requests, same GGUF/quant/build b10211/n_ctx 16384:

| | Kronk | upstream `llama-server` |
|---|---|---|
| silent-empty responses | **7 of 9** | **0 of 9** |
| honours `max_tokens` | no — 39 tokens regardless of cap | yes — `completion == cap` |
| `finish_reason` on truncation | `"stop"` (0/9 `"length"`) | `"length"` on 7/9 |
| tool call delivered from a truncated generation | no | **yes** — parsed `get_weather` returned, with the raw partial text at cap 4 |

Upstream recovers a *parsed tool call out of a generation it truncated at 4 tokens*. Kronk, given the
same weights and the same library, returns an empty turn.

Pinned by `TestToolCallCutOffAtMaxTokensIsInvisibleToTheCaller`,
`TestMaxTokensIsCheckedAroundBufferedToolCallTokens`,
`TestToolCallHeldByTheParserAtEOGIsUnrecoverable`, and live by `TestToolCallDifferential`:

```
KRONK_MTP_DIFF=1 KRONK_MTP_TOOL_PROBES=trunc KRONK_MTP_DIFF_CTX=16384 \
  go test -tags kronkdiff -timeout 40m -v -run TestToolCallDifferential ./sdk/kronk/tests/mtp/
```

(12s with both legs; add `KRONK_MTP_DIFF_UPSTREAM=0` for the Kronk leg alone.)

The `max_tokens` boundary was probed at caps 4, 8, 12, 16, 20, 24, 32, 36, 38, 39 (all lost) and 40, 41,
44, 48, 64 (all delivered) — the cliff is exactly at the 39 tokens the model actually generates.

### 14e. Mid-task abandonment in an agentic loop — reproduced, but UNDERPOWERED — MED confidence

Separately from the `max_tokens` artefact, an 8-file agentic audit loop was run repeatedly:

| | abandoned mid-task | samples |
|---|---|---|
| Kronk | **2** | 6 |
| upstream | 0 | 7 |

The clearest Kronk failure: it stopped at **4 of 8 files**,
declared *"## Audit Complete"*, and then — when challenged — **agreed that it had skipped them.** That is
"goes down a path, flips a switch, contradicts itself" in one sample.

2/6 vs 0/7 is Fisher p ≈ 0.19 — **suggestive, not significant**. It is
consistent with the §8/§14b prompt corruption causing real degradation, but it does not establish it.
Note also that upstream had 2/7 failures of its own, in a *different* mode Kronk never showed (emitting
`list_files` as plain text at step 0 and never entering the loop), so the two stacks fail differently
rather than one simply being worse.

Unlike §14/§14d — which is deterministic, Kronk-only, and needs no statistics — **this result needs more
samples before anyone acts on it.** The harness supports repeats:
`KRONK_MTP_TOOL_KRONK=0` + `KRONK_MTP_TOOL_SEED=<n>` for single-leg runs.

---

---

## 15. Unexplained Go heap corruption — open

The two libffi width defects found in yzma (see `yzma-findings.md` #3 and #5) are **not** armed by
`Init()` — one needs a model load, the other is unreachable — so neither explains why deferring `Init()`
worked around the crash. The `Init()`-registered `purego.NewCallback`
log callback appears **GC-safe**: purego retains it in a package-level `cbs.funcs` array
(`syscall_sysv.go:54-103`), and callbacks from foreign threads are handled because purego wires up
`iscgo`/`_cgo_thread_start` via fakecgo or real `runtime/cgo`. It does leak one callback slot per
`Init` against a hard cap of 2000.

A 40s reproduction attempt (`GODEBUG=clobberfree=1,invalidptr=1 GOGC=1`, `Init()` + 4 goroutines of
net/http+JSON + a `runtime.GC()` loop, no model) **survived 115,886 request round-trips.**

So `found pointer to free object` remains **unexplained**. The yzma defects are not its resolution.

**Severity caveat:** the yzma width defects are weak candidates for the reported symptom, because they
fire at **model-load time, not per-token during generation**. The one exception worth tracking is
`yzma-findings.md` #2 (`size_t min_keep` passed as a `uint32`), which fires per-sampler-call, is
nondeterministic at a measured 5-14%, and silently disables top-p / min-p / XTC — that one *is* a
plausible contributor and Kronk reaches it at `params.go:800`, `:804`, `:809`.

### 15a. COVERAGE GAP — the Kronk-side pointer-lifetime audit was never completed

The investigation covered yzma's binding layer. It did **not** cover the Kronk side: retained slices over
C-owned memory (logits, embeddings, pre-norm buffers), missing `runtime.KeepAlive`, and Go pointers
handed to C and held across calls. A subagent tasked with that call-site audit never reported back.

This matters because llama.h documents that `llama_get_logits` / `get_embeddings` pointers are
**invalidated by the next decode** — and Kronk reads those in the speculative path
(`batchgen_speculative.go`, `logprobs.go`, `batchgen_mtp.go`). Round 1 verified that
`captureVerifyPreNorm` copies rather than retaining, but that is one site out of many. **This is the
most likely remaining home for the unexplained crash**, and it is the obvious next step.
