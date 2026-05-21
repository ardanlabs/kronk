# Chapter 6: Speculative Decoding & MTP

## Table of Contents

- [6.1 Overview — The Two Draft Modes](#61-overview-the-two-draft-modes)
- [6.2 When to Use a Separate Draft Model](#62-when-to-use-a-separate-draft-model)
- [6.3 When MTP Is Used](#63-when-mtp-is-used)
- [6.4 Acceptance, `nDraft`, and the Adaptive Throttle](#64-acceptance-ndraft-and-the-adaptive-throttle)
- [6.5 Configuration Recap](#65-configuration-recap)
- [6.6 Observability](#66-observability)
- [6.7 Known Limitations](#67-known-limitations)

---

This chapter is the user-facing operation guide for speculative decoding
in Kronk. It assumes you have read the introductory speculative-decoding
section in [Chapter 3 §3.12](#312-speculative-decoding), which covers
both draft modes at the configuration level and shows the `model_config.yaml`
shapes. This chapter focuses on **when to choose each mode**, **how to
read the acceptance metrics**, and **what to look for in the logs**. The
engine internals (FFI plumbing, mirror step, three-pass dispatch,
hybrid snapshot/restore) live in
[Chapter 18 §18.12](#1812-mtp-internals).

### 6.1 Overview — The Two Draft Modes

Kronk supports two interchangeable sources of draft tokens for
speculative decoding. The drafter is selected once at model load and
behaves the same way to a caller:

| Mode              | When used                                                                                          | Drafter origin                                          |
| ----------------- | -------------------------------------------------------------------------------------------------- | ------------------------------------------------------- |
| **Separate-GGUF** | You explicitly configure a `draft-model:` block on the target entry.                                | A second, smaller GGUF you download separately.         |
| **MTP**           | Auto-enabled when the target GGUF ships a Multi-Token-Prediction head and the platform supports it. | The MTP head shipped inside the target GGUF — no extra download. |

A model can have at most one drafter active. If you configure
`draft-model:` explicitly, that wins — the MTP head, even when present,
is ignored on that load. If neither is available, the target serves
traffic without speculation; nothing else changes.

Behavior an operator can rely on for both modes:

- The user-facing API (`/v1/chat/completions`, the Responses API, the
  SDK) does not change. Speculation is invisible to the caller.
- The same `draft_tokens`, `draft_accepted_tokens`, and
  `acceptance_rate` fields appear on every final log line and in the
  usage payload once a drafter is configured, even when speculation
  produces zero tokens in a round.
- A failed drafter never blocks a request. If draft generation fails
  mid-stream, the slot continues target-only and the request still
  completes correctly.

### 6.2 When to Use a Separate Draft Model

A separate draft model is the classic speculative-decoding setup: a
small, fast sibling of the target that proposes the next few tokens,
and the target verifies them in a single forward pass.

**The conceptual model.** Think of the target as the authoritative
writer and the draft as a fast typist who tries to anticipate the next
few words. When the typist guesses right, the target accepts those
words "for free" — they cost only one verification pass instead of
one forward pass per token. When the typist guesses wrong, the target
corrects the mistake and the wasted draft work is the price of the
attempt. Net throughput depends entirely on how often the typist is
right.

#### Picking a draft model

The draft must share the target's tokenizer (same vocabulary). Within
that constraint, the best practical choice is almost always **a
quantized version of the same architecture** rather than a smaller
dense model from a different family:

| Target Model            | Recommended Draft          | Why                                                              |
| ----------------------- | -------------------------- | ---------------------------------------------------------------- |
| Qwen3-8B-Q8_0           | Qwen3-0.6B-Q8_0            | Same Qwen3 family; same tokenizer; small enough to be cheap.      |
| Qwen3.5-35B-A3B-Q8_K_XL | Qwen3.5-35B-A3B-UD-Q2_K_XL | Same MoE architecture at lower quantization — much higher acceptance than a smaller dense model. |

The second pattern (same architecture, lower quant) is usually the
winner on MoE targets because the draft sees the same weight structure
the target uses to make decisions, just rounded down. A tiny dense
model from another family will tokenize the same words but reason
very differently, and acceptance suffers accordingly.

#### When a separate draft is the right choice

- The target ships **no MTP head**, so MTP is not an option.
- You want **predictable behavior across architectures** — a separate
  draft works the same way regardless of what the target is.
- You have a **known-good draft** you trust more than an auto-detected
  head — for example, a quantized sibling you have already benchmarked
  on your workload.
- You need **single-slot semantics** (`nseq-max: 1`) anyway. Separate-GGUF
  drafts only work at single-slot today, so this constraint is a
  no-op for you.
- You are running at **temperature near zero** on a large MoE target.
  This is the sweet spot for separate-GGUF speculation: low
  temperature drives acceptance up, and the MoE target's sparse
  activation makes verification cheap.

#### When a separate draft is the wrong choice

- The target is **small and already fast** (e.g. an 8B dense model at
  30+ TPS on your hardware). Drafting overhead can erase the win or
  go net negative.
- Your workload is **highly creative or reasoning-heavy** at high
  temperature. Acceptance collapses and the adaptive throttle (§6.4)
  will gate speculation off anyway — you are paying the draft load
  cost for nothing.
- You cannot find a draft that shares the target's tokenizer. There
  is no workaround; speculation requires vocabulary parity.
- You need **multi-slot batching** on this entry. Use MTP, or run
  without speculation.

#### Reading the acceptance signal

Speculative decoding is a throughput bet, and the metric that matters
is the running **acceptance rate**. It shows up in every final
response log line as `acceptance_rate=` along with `draft_tokens=` and
`draft_accepted_tokens=`. Rules of thumb:

| Acceptance | Interpretation                                                                                                              |
| ---------- | --------------------------------------------------------------------------------------------------------------------------- |
| > 0.70     | Strong win. Speculation is clearly paying off; the target is doing meaningfully fewer forward passes than tokens generated. |
| 0.50–0.70  | Net positive on most workloads. Worth keeping enabled.                                                                      |
| 0.30–0.50  | Marginal. The adaptive throttle (§6.4) will already be scaling `nDraft` down.                                                |
| < 0.30     | Speculation is bypassed for that round. Check temperature, draft choice, and task type.                                     |

### 6.3 When MTP Is Used

MTP (Multi-Token Prediction) is the second draft mode, auto-enabled by
Kronk when the target GGUF itself ships an MTP head. You do not
configure it; Kronk decides at model-load time.

**Conceptually**, the MTP head is a small set of extra layers grafted
onto the target that predict the **next N tokens** of the target's
continuation given the most recent token and the target's own internal
state. Because the head ships inside the same GGUF as the target and
uses the target's tokenizer:

- There is **nothing extra to download**.
- There is **no risk of a vocabulary mismatch**.
- The drafter is always perfectly architecture-matched to the target.

Kronk turns MTP on when all of these hold:

- The target GGUF metadata declares `nextn_predict_layers > 0`. This
  is the marker for "this file ships an MTP head."
- The loaded llama.cpp library exposes the pre-norm hidden-state API
  that the MTP path needs. Kronk's shipped libraries are current
  enough; this matters only if you pin an older library.
- You have **not** set a `draft-model:` block on this entry. Explicit
  config always wins.

Both single-slot (`nseq-max: 1`) and multi-slot (`nseq-max: 2+`) loads
are supported.

#### When MTP is the right choice

- The target is a Qwen3.5 / Qwen3.6 family GGUF (or any future
  architecture that adopts the same metadata key) and you do not want
  to manage a second GGUF download.
- You want **multi-slot batching** on a target that has an MTP head.
  This is the only way to get speculation under `nseq-max > 1` today.
- You are running a target where the in-file MTP head is known to
  have high first-token acceptance, and you are fine with the
  greedy-verify trade-off (see §6.7).

#### When MTP is the wrong choice

- The target does not ship an MTP head — there is nothing to enable.
- You require strict Leviathan-style distribution equivalence at
  `temperature > 0`. The MTP path runs greedy verify; see §6.7.
- You are hitting IMC cache often and the MTP-disabled-on-cache-hit
  behavior in §6.7 is removing most of your speculation opportunity.
  In that case a separate-GGUF draft, which is unaffected by IMC, may
  serve you better.

### 6.4 Acceptance, `nDraft`, and the Adaptive Throttle

Both modes generate `nDraft` candidate tokens per round and pay a
fixed verification cost on the target. If acceptance is high, the win
scales with `nDraft`. If acceptance is low, every wasted draft token
costs you a target forward pass that produced nothing.

To avoid burning forward passes when acceptance collapses (a long
creative passage at high temperature, an unusual prompt the draft
hasn't seen), Kronk runs an **adaptive throttle** on every slot. It
tracks an exponential moving average of the slot's recent acceptance
rate and scales `nDraft` for the next round accordingly:

| Recent acceptance EMA | `nDraft` used next round |
| --------------------- | ------------------------ |
| `< 0.30`              | `0` — speculation bypassed for this round |
| `< 0.50`              | `min(1, configured max)` |
| `< 0.70`              | `min(2, configured max)` |
| `< 0.85`              | `min(3, configured max)` |
| `≥ 0.85`              | `configured max`         |

Two operator-visible consequences:

- A slot whose acceptance falls into the gutter (`< 0.30`) will skip
  speculation entirely for that round. You will still see the
  `draft_tokens=0` / `draft_accepted_tokens=0` / `acceptance_rate=...`
  fields on the final log line — they are emitted on every request
  the moment the model has any drafter configured, so log schemas
  stay stable.
- The EMA **persists across requests on the same slot**. If a slot
  has been seeing poor acceptance, it will start the next request
  cautiously rather than re-paying the discovery cost.

The default `nDraft` is **5 for separate-GGUF drafts** and **4 for
MTP** (MTP heads typically have high acceptance for the first 1–3
tokens and decay rapidly beyond that, so a lower cap is safer).

### 6.5 Configuration Recap

There is nothing speculative-decoding-specific to configure in this
chapter. The YAML shapes live in
[Chapter 3 §3.12](#312-speculative-decoding):

- **Separate-GGUF** — add a `draft-model:` block under the target
  entry in `model_config.yaml` (or set `model.Config.DraftModel` from
  the SDK). Requires `nseq-max: 1`.
- **MTP** — do nothing. Pull a target GGUF that ships an MTP head,
  and make sure you have not set `draft-model:` on that entry.

### 6.6 Observability

Kronk emits a consistent set of log events for both draft modes.
Operators should be able to answer "is speculation actually helping?"
from logs alone.

**Always-present fields on the final response log line:**

| Field                   | Meaning                                                              |
| ----------------------- | -------------------------------------------------------------------- |
| `draft_tokens`          | Total candidate tokens proposed by the drafter during the request.   |
| `draft_accepted_tokens` | Of those, how many the target accepted.                              |
| `acceptance_rate`       | `draft_accepted_tokens / draft_tokens`, or 0 when no drafts were run. |

These fields are emitted whenever the model has any drafter
configured — even on requests where the adaptive throttle bypassed
speculation — so dashboards and log parsers see a stable schema.

**MTP-specific log events:**

| Event                                            | When                                                                                                                                            |
| ------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `draft-model-mtp status=loading / loaded`        | Once at model startup when the MTP head is auto-detected and loaded.                                                                            |
| `draft-model-mtp status=auto-detect-skipped`     | Once at model startup when MTP could not be enabled (no metadata, no pre-norm API).                                                             |
| `speculative status=mtp-mirror-error`            | An MTP draft step failed. The slot continues target-only for the remainder of the request.                                                      |
| `speculative status=mtp-disabled-imc-hit`        | MTP disabled for this request because the IMC cache hit didn't carry a draft-seq snapshot (no draft state on the matched session, or the restore returned 0 bytes). MTP-aware IMC builds — the default since this fix — snapshot the draft seq state and pendingH alongside the target so cache hits keep MTP running. (See §6.7.) |
| `start-slot status=imc-draft-snapshot-done`      | The cache-build path snapshotted both the target seq state and the MTP draft seq state into the session. Carries `snapshot_bytes`, `kv_alloc`, `buf_action`, `pending_h`.                  |
| `start-slot status=imc-draft-restore-done`       | A cache hit restored both the target seq and the MTP draft seq state from the session, plus the slot's `pendingH`. Carries `restored_bytes`, `pending_h`, `elapsed`.                       |
| `start-slot status=imc-draft-restore-failed`     | The session had a draft snapshot but `StateSeqSetData(draft)` returned 0 bytes. The slot continues target-only; MTP is disabled for this request via `mtp-disabled-imc-hit`.               |
| `start-slot status=imc-draft-restore-skip-empty` | The session has no draft snapshot (e.g., built before this fix or build-time draft snapshot failed). MTP is disabled for this request via `mtp-disabled-imc-hit`.                          |
| `speculative status=mtp-disabled-mirror-error`   | MTP disabled for the remainder of the request after a post-verify mirror failure. The slot continues target-only.                               |
| `speculative status=verify-prenorm-capture-error` | Phase A failed to capture the slot's pre-norm rows into `verifyH`. Phase B's mirror falls back to the live target buffer; on hybrid targets this may force a `mirror-error` disable if a restore also ran. |

**Shared speculative-decoding log events:**

| Event                                | When                                                                                                                  |
| ------------------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| `speculative status=verify-done`     | Throttled per slot: first verify round, then every 32nd. Carries `round`, `accepted`, `nDraft`, and `acc_ema` fields. |
| `speculative status=restore-error`   | Hybrid snapshot restore failed during partial-reject rollback.                                                        |
| `speculative status=snapshot-error`  | Hybrid snapshot capture failed before a spec round.                                                                   |

### 6.7 Known Limitations

| Limitation                                          | Notes                                                                                                                                                |
| --------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| Separate-GGUF: single-slot only                     | `nseq-max` must be `1` on the target entry. If you need multi-slot speculation, use MTP on a target that ships the head.                              |
| MTP: greedy verify only                             | The MTP path always runs greedy verification, so strict Leviathan-style distribution equivalence at `temperature > 0` is not guaranteed. The full slot sampler (temperature / top-k / top-p) is still applied at each accepted position, so output shape is preserved. |
| MTP + IMC: draft state must come from an MTP-aware build | IMC cache hits keep MTP running by restoring the draft seq KV + `pendingH` snapshotted alongside the target during the cache build. Sessions whose cache was built before this fix (no draft snapshot on disk/RAM) fall back to disabling MTP for the cache-hit request only — they re-enable on the next request once the cache is rebuilt by an MTP-aware path. |
| MTP + hybrid targets: f16 KV cache + no Flash Attention required | Kronk forces `cache-type-k/v: f16` and disables Flash Attention on hybrid models. Throughput on hybrid + MTP is meaningfully lower than dense / MoE targets regardless of `nseq-max`. |
| MTP: `nDraft` ceiling is fixed at 4                 | The adaptive throttle scales down from 4 but there is no per-model knob to raise the ceiling on exceptionally well-behaved MTP heads.                  |
| Speculative decoding is text-only                   | Neither draft mode applies to vision or audio requests.                                                                                              |

---
