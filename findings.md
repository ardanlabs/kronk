# MTP / Speculative Decoding — Findings

2026-08-01. Verified against this repo, `.extras/llama.cpp` (b10211), `.extras/yzma`, and live
runs of `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL`. **No source was modified.**

> **yzma version note.** `.extras/yzma` is tagged **v1.21.0** — the same version `go.mod` pins.
> Only `pkg/llama/draft.go` and `draft_test.go` differ from the module cache, via an
> **uncommitted local patch** that inserts a `pMin float32` parameter into `DraftGenerate`
> between `nDraft` and `greedy`. Kronk passes 10 args at `batchgen_speculative.go:242`, so
> **adopting that local tree would break Kronk's build.** Everything else in `pkg/llama` is
> byte-identical, so §§1-2, 4-6 below hold for both trees. See §6c and §6i for the patch's
> behavioural impact.

---

## 1. MTP bonus token bypasses the sampler chain and the grammar — CRITICAL, reproduced

`batchgen_speculative.go:353-356` forces `greedy = true` for every MTP request. The in-loop
verify branch honours that by routing through `s.sampler` / `s.grammarSampler`. The
bonus-token block at `:582-602` does not:

```go
case greedy:                                  // true for every MTP request
    maskSuppressTokenLogits(targetLogits, e.model.suppressTokens)
    bonusToken = argmax(targetLogits)         // :594-595
```

Every fully-accepted round emits a raw `argmax` token — no penalties, DRY, top-k/p, min-p,
XTC, temperature, **no grammar**. At measured 0.71 acceptance with `defMTPNDraft = 2`, that
is ~15-30% of all emitted tokens decoded greedily.

- **Repeat loops**: greedy decoding is the canonical cause, and the bypass also removes
  repeat-penalty on exactly those tokens. Self-reinforcing — greedy tokens are what the MTP
  head predicts best, so acceptance stays high.
- **Broken tool calls / crash**: `response_format` and `json_schema` set `p.Grammar`
  (`params.go:469,478,490`). The unconstrained bonus token then reaches
  `grammarSampler.Accept` (`batchgen_tokens.go:60`); `llama_grammar_accept_token` **throws**
  (`llama-grammar.cpp:1517`), nothing catches it, and the C++ exception unwinds through
  libffi into Go. Verified: `yzma.SamplerAccept` (`sampling.go:586-591`) has only a nil-handle
  guard, and `grep -rn "recover()" pkg/` over all of yzma returns **zero hits**. This is not an
  oversight that a defensive `recover()` could fix — a Go `recover()` cannot catch a C++
  exception unwinding through libffi at all; `libc++abi` aborts before any Go frame runs.
  **The only possible fix is to never hand an out-of-grammar token to `Accept`.**

Reproduced — 5 identical `json_schema` requests, runs 0-1 valid JSON, run 2 killed the process:

```
libc++abi: terminating due to uncaught exception of type std::runtime_error:
  Unexpected empty grammar stack after accepting piece: } (92)
SIGABRT
  llama_grammar_accept_token → SamplerAccept → grammarSampler.Accept (grammar.go:436)
  → handleSampledToken (batchgen_tokens.go:60)
  → finalizeSpeculativeTokens (batchgen_speculative.go:762)   ← bonus-token site
```

Kills the whole process, not just the request. No workaround: MTP auto-enables from
`nextn_predict_layers` (`draft_mtp.go:460-508`) and no config key disables it.

Upstream samples every position — including the bonus — through the same chain and grammar
(`common/sampling.cpp:646-674`).

**Fix**: one branch, mirroring `:425-467`.

---

## 2. `MemorySeqRm` return value discarded everywhere — HIGH

`llama.MemorySeqRm` returns `(bool, error)`. There are **19** call sites in `sdk/kronk/`
and **none** captures either value. yzma's doc comment (`memory.go:101-105`) is explicit:
*"Returns false if a partial sequence cannot be removed. Removing a whole sequence never
fails."* Note the `error` return is only the nil-handle guard — it is never non-nil after a
successful FFI call, so the **bool** is the load-bearing value.

That doc comment splits the sites into two groups:

- **Partial-range — can genuinely return false, must be checked (6 sites):**
  `batchgen_speculative.go:64,697,703,971,1012`, `batchgen_finish.go:159`
- **Whole-sequence (`-1,-1`) — never fails, ignoring the bool is defensible (13 sites):**
  `batchgen_speculative.go:24,60,1084`, `batchgen_finish.go:163,179`,
  `batchgen_slot_start.go:296,322,330,412,421,455,457,494`

So the actionable fix is 6 sites, not 19.

On a hybrid target (`qwen35moe` is hybrid), `llama_memory_hybrid::seq_rm` tries the recurrent
cache first and **returns false having mutated nothing — not even the attention KV**
(`llama-memory-hybrid.cpp:143-150`), because `llama_memory_recurrent::seq_rm` refuses
mid-sequence ranges unless `n_rs_seq > 0`.

`MemorySeqPosMin` / `MemorySeqPosMax` (`memory.go:153,163`) return `(Pos, error)` and are
unused by Kronk. They are the cheapest runtime assertion that a trim actually landed — and
they catch the hybrid case where the bool is true but only one of the two caches was trimmed.

So on the §3 fallback paths, rejected drafts stay in the attention KV while `s.nPast` rewinds.
llama.cpp appends rather than overwrites by `(seq, pos)` — the sequence carries phantom
rejected tokens forever. Textbook repeat-loop generator.

---

## 3. Snapshot / restore failures continue speculating — HIGH (latent)

- **Capture** (`batchgen_engine.go:308-317`): logs `snapshot-error`, truncates
  `specSnapshot`, keeps speculating → `hybridRestore == false` → the §2 no-op `MemorySeqRm`.
- **Restore** (`batchgen_speculative.go:689-699`): logs `restore-error`, same fallback, then
  sets `nPast` (`:740`) and streams the bonus token (`:762`). The comment "we'll likely fail
  the slot below" is wrong — nothing below fails the slot.

`disableMTPForRequestSpec` (`:1081`) and `mtpDisabledForRequest` exist but are not wired to
either. Neither fired across ~2500 probe tokens, so these are latent, not the daily cause.

---

## 4. llama.cpp's recurrent-rollback support is unused — HIGH

- **`n_rs_seq`**: `llm_arch_supports_rs_rollback` returns true for `QWEN35`/`QWEN35MOE`
  (`llama-arch.cpp:989-997`). With `n_rs_seq >= n_draft`, `llama_memory_seq_rm` does the
  partial recurrent rollback natively and no snapshot is needed. Upstream sets it from the
  draft width (`common/common.cpp:1633`). Kronk never sets `NRsSeq` — always 0.
- **`PARTIAL_ONLY`**: `llama_memory_hybrid::state_write/read` skip the attention KV under
  `LLAMA_STATE_SEQ_FLAGS_PARTIAL_ONLY` (`llama-memory-hybrid.cpp:190-202`); upstream's server
  uses it for every speculative checkpoint. Kronk calls `StateSeq{GetSize,GetData,SetData}`
  at `batchgen_speculative.go:784,797,823` with flags = 0 — full per-seq state incl. the
  entire attention KV — **every speculative round**.

**This is far cheaper to fix than it first appears — no FFI work is needed.** Verified against
`.extras/yzma`:

- `NRsSeq` is already exposed on `llama.ContextParams` (`llama.go:340`), and the Go struct
  matches `llama_context_params` field-for-field, so it is a one-line assignment at the
  context-params construction site. `CtxOther` (`:377`) and `CtxType` (`:344`) are likewise
  correct.
- yzma **already binds all three `_ext` entry points**: `StateSeqGetSizeExt`,
  `StateSeqGetDataExt`, `StateSeqSetDataExt` (`state.go:343,353,368`), each taking
  `flags uint32`, tested at `state_test.go:297-328`. Switching the three call sites above and
  passing `uint32(1)` is a three-line change. (yzma exposes no named constant for the flag —
  pass the literal with a comment.)
- `llama.NRsSeq(ctx)` (`context.go:550`) reads back what llama.cpp actually honoured.
  `model.go:640` currently logs the *requested* `ctxParams.NRsSeq`, which is always 0;
  logging the read-back instead would report the value that determines whether partial
  recurrent rollback works.

---

## 5. Flash Attention + MTP: it works

Measured, Metal, b10211, 4096 ctx, 400-token generations:

| flash-attention | nSeqMax | K/V | result | accept | TPS |
|---|---|---|---|---|---|
| enabled | 1 | f16 | OK | 0.73 | 69.1 |
| disabled | 1 | f16 | OK | 0.69 | 65.6 |
| enabled | 2 | f16 | OK | 0.71 | 68.6 |
| disabled | 2 | f16 | OK | 0.72 | 68.5 |
| auto | 1 | f16 | OK | 0.74 | 71.0 |
| enabled | 1 | q8_0 | OK | 0.71 | 62.9 |
| **disabled** | 1 | **q8_0** | **LOAD FAILS** | — | — |

MTP auto-detected and loaded in every passing case. The only failure is FA *disabled* with a
quantized V cache, which llama.cpp rejects by design (`llama-context.cpp:458-462, 3561-3570`).

No rule anywhere in `config.go` / `analyze.go` / `kronkresolve.go` / catalog disables FA for
hybrid or MTP. The belief traces to a comment — `testlib.go:394-395`, *"Hybrid target requires
f16 KV and disabled flash-attention (see config.go)"* — which is wrong in both halves, and the
config it annotates doesn't set `PtrFlashAttention` at all, so it runs with FA enabled.

---

## 6. Other correctness issues

| # | Issue | Severity |
|---|---|---|
| 6a | **Temperature 0 unreachable.** `params.go:724` rewrites `temperature <= 0` → `0.8`; `params.go:372` drops a zero when converting `Params`→`D`. Blocks deterministic decoding, makes the classic-drafter `greedy` path dead code, and blocks any temp-0 equivalence test. | MED |
| 6b | **No context guard during generation.** `MaxTokens` defaults to the full window (`params.go:706`); `s.nPast` is only checked on the prompt. Overrun → `llama_decode` fails → `batchgen_engine.go:464-475` **fails every active slot**, not just the offender. | MED |
| 6c | **`rollbackDraft` off-by-one after EOG-truncated draft.** `DraftGenerate` increments `nPast` before the EOG check but breaks before `drafted++` (yzma `draft.go:121-126`), so `draftBasePast` at `:1002` is one too high. Draft KV keeps a stale cell; acceptance collapses for the rest of the request. Details in §6c-detail. | MED |
| 6d | **`verifyH` fallback reads the wrong buffer.** On capture failure the mirror falls back to the live pre-norm buffer, which on hybrid holds the *rebatch* rows after a **successful** restore. Wrong hidden states into draft KV; comment at `:396-399` has the reasoning backwards. | MED |
| 6e | **Stale logits index for bonus token** (`:762`) after a hybrid restore — valid indices are `[0, 1+accepted)`, not spec-batch offsets. Logprobs only. | LOW |
| 6f | **`loadDraftModelMTP` drops `KVUnified`/`SwaFull`** (`draft_mtp.go:86-98`). At `nSeqMax>1` target gets `n_ctx_seq = 2*window` unified, MTP draft gets `window` non-unified. Shared-KV loader copies them and explains why. | LOW |
| 6g | **`finishSlot` trims the target memory for shared-KV MTP.** `draft.core().mem` *is* the target's memory for `*sharedMTPDrafter`; `trimPos == 0` wipes the whole target seq. Harmless only because `:179` clears it anyway. | LOW |
| 6h | **Dead code**: `s.specDraftProbs` is only ever `nil`, so `:556-574` and `sampleAdjustedInto` (`:885`) are unreachable. | LOW |
| 6i | **`DraftGenerate` corrupts sampler state, not just `nPast`, on truncated non-greedy drafts.** The EOG `break` (`draft.go:123`) is *after* `samplerAcceptFunc.Call` (`:115`), so the chain accepts a token the caller never receives — repeat-penalty/DRY/grammar state advances past a phantom token. Currently masked because MTP forces `greedy = true` and the accept is gated on `if !greedy` (`:86`). **Becomes live the moment §1 or §6a is fixed.** | MED (latent) |

### §6c-detail — exact contract

Per-exit-path trace of `DraftGenerate`:

| Exit path | `nPast++`? | `drafted++`? | `finalPast` vs `start+drafted` |
|---|---|---|---|
| decode error | No (`break` precedes it) | No | **exact — not affected** |
| EOG hit | Yes | No | **+1 (leaks)** |
| normal completion | Yes ×n | Yes ×n | exact |
| *p_min stop (local patch only)* | Yes | No | **+1 (leaks)** |

So the bug fires only on EOG today — and additionally on p_min if the uncommitted
`.extras/yzma` patch is ever adopted.

**This cannot be fixed by upgrading yzma.** Its own `draft_test.go:97-101` asserts
`finalPast - start ∈ [drafted, drafted+1]` — upstream treats the ambiguity as contract, and a
caller cannot distinguish the two cases from the return values.

**The fix is already sitting in Kronk**: `generateDraftTokens` captures
`draftStartPast := s.draftNPast` at `batchgen_speculative.go:238` and then uses it only for
logging. `rollbackDraft` should take that saved base instead of recomputing
`s.draftNPast - llama.Pos(nDraft)` at `:1002`.

---

## 7. Test coverage

No unit test in `sdk/kronk/model/` touches `batchgen_speculative.go`, `batchgen_mtp.go`, or
`draft*.go` — only `config_test.go:347` (config arithmetic) and `lora_test.go:23` (type switch).

Integration = three smoke suites asserting the response contains "Gorilla", under
`WithRetry`. A repetition loop containing "Gorilla" passes.

`checkMTPUsage` *does* `t.Errorf` on `DraftTokens == 0`; only `DraftAcceptedTokens == 0` is a
non-failing `t.Logf` — still the one signal that would catch silent MTP collapse.

**Worse: the MTP suite has never run at all.** `testlib.go:72` resolved
`mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL`, and that ID **does not exist in the catalog** —
`grep -c Q2_K_XL sdk/tools/defaults/yaml/catalog.yaml` returns 0. The only MTP entry is
`unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL` (catalog line 321). So `testlib.MPMTP` was always
empty and `tests/mtp/main_test.go` always took the `os.Exit(0)` skip — silently, behind a
"not downloaded" message that reads like normal local-box behaviour.

`tests/gemma4mtp` and `tests/draft` deserve the same audit.

**Nothing exercises MTP with a grammar / `response_format`.** Combined with the above, that
is why §1 shipped.

---

## 8. Stale or wrong comments

| Location | Problem |
|---|---|
| `batchgen_speculative.go:348-352` | Claims the greedy branch uses the full sampler — false for the bonus token (§1). |
| `batchgen_speculative.go:694-695` | "we'll likely fail the slot below" — nothing does. |
| `batchgen_speculative.go:757-761` | Confuses sequence position with batch output index (§6e). |
| `batchgen_speculative.go:396-399` | Failsafe reasoning backwards; the hazard is a *successful* restore (§6d). |
| `batchgen_speculative.go:546-554` | Describes a live fallback that is dead code (§6h). |
| `batchgen_speculative.go:948-954` | Mirror moved to `finalizeSpeculativeTokens` in the 2A/2B split. |
| `batchgen_speculative.go:134-136` | References `batchEngine.newSlot`; no such method (it's `newBatchEngine`). |
| `batchgen_engine.go:313-314` | "full-accept rounds still work" — they never call `MemorySeqRm`; partial rejects silently no-op (§2). |
| `batchgen_slot.go:252-257` | "next llama_decode fails with -1" — it doesn't fail, it silently corrupts (§2). |
| `models.go:47-49` | Partial delete *refuses* atomically; and `n_rs_seq` makes it work on qwen35 (§4). |
| `draft_mtp.go:352` | "MemorySeqRm ... is a no-op for the shared layers" — it hits the target's memory (§6g). |
| `testlib.go:394-395` | FA/f16 claim is wrong and unsourced (§5). |
| `tests/mtp/suite_test.go:10-11` | "NRsSeq is logged by the loader" — never set (§4). |
| `batchgen_tokens.go:183-184` | Hardcoded "line 55"; actual site is line 66. |

---

## 9. Suggested order

1. **§1** — route the bonus token through `s.grammarSampler` / `s.sampler`. One branch; fixes
   the crash, the grammar violations, and the dominant loop mechanism.
2. **§4** — *moved up*: it needs no FFI work (yzma already binds everything). Set
   `NRsSeq = nDraft` on the target context, switch the three `StateSeq*` calls to the `_ext`
   forms with `flags = 1`, and checkpoint only when `n_rollback > n_rs_seq`. This also
   partially subsumes §2 by making the hybrid partial `seq_rm` succeed natively.
3. **§2** — check the bool at the 6 partial-range sites; fail the slot or disable speculation
   on false. Consider a debug-mode `MemorySeqPosMax` assertion alongside it.
4. **§3** — wire `disableMTPForRequestSpec` into `snapshot-error` / `restore-error`.
5. **§6c** — use the already-captured `draftStartPast`; one line. Do it before §6a/§1 land,
   since §6i goes live once non-greedy drafting becomes reachable.
6. **§6a**, then a temp-0 MTP-on/off equivalence test and an MTP + `json_schema` regression test.
7. Comments in §8.

Per `AGENTS.md` all of this is a source change requiring `plan-change` and an approved plan.
