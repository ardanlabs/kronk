# Chapter 5: Message Caching

## Table of Contents

- [5.1 Overview](#51-overview)
- [5.2 Incremental Message Cache (IMC)](#52-incremental-message-cache-imc)
  - [Complete-Prompt Token Design](#complete-prompt-token-design)
  - [Session Pool (Decoupled from Slots)](#session-pool-decoupled-from-slots)
  - [Pure Hit Snapshot Skip](#pure-hit-snapshot-skip)
  - [KV Pressure Eviction](#kv-pressure-eviction)
  - [Media and M-RoPE](#media-and-m-rope)
  - [Model Type Interactions](#model-type-interactions)
- [5.3 Multi-Session Caching](#53-multi-session-caching)
- [5.4 When to Use IMC](#54-when-to-use-imc)
- [5.5 Cache Invalidation](#55-cache-invalidation)
- [5.6 Configuration Reference](#56-configuration-reference)
- [5.7 Performance and Limitations](#57-performance-and-limitations)

---

Message caching reduces redundant computation by storing and reusing KV cache
state from previous requests.

### 5.1 Overview

When processing a chat request, the model must compute attention for
every token in the conversation. Without caching, the entire prompt is
prefilled on every request — even tokens the model has already seen.

_Note: Prefill is the phase where the model processes all input tokens
(system prompt, conversation history, and the new message) before it
begins generating a response. This is the most computationally
expensive part of a request, and its cost grows with the number of
input tokens._

Kronk provides the Incremental Message Cache (IMC) to reduce redundant
prefill work. **IMC is enabled by default for all models.** IMC maintains logical sessions — one per conversation
branch — and caches the full message history so only the new message
needs to be prefilled. All sessions (text and media) externalize their
cached KV state to RAM after each request and restore it into any
available slot on the next request. `StateSeqGetData` captures the raw
KV bytes regardless of whether they originated from text tokens or media
embeddings.

```
No Caching:
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (prefill)   │ (prefill) │ (prefill) │  (prefill)  │
└─────────────────────────────────────────────────────┘
                                              ↓
                                           Generate

IMC (Incremental Message Cache):
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (cached)    │ (cached)  │ (cached)  │  (prefill)  │
└─────────────────────────────────────────────────────┘
                                              ↓
                                           Generate
```

### 5.2 Incremental Message Cache (IMC)

Incremental Message Cache is designed for agentic workflows. For text requests
it renders the complete conversation twice from independent request copies:
the generation-ready form and a stable form without the generation prompt.
Both are tokenized with BOS exactly once. Kronk caches a complete stable token
prefix and always leaves at least one generation-ready token for inference.
Cached token sequences are authoritative: the longest session whose entire
sequence prefixes the new target is reused; divergence rebuilds an empty or
LRU session. Tool-result turns therefore need neither synthetic user messages
nor a separately rendered suffix.

**Key Terminology:**

- **Session** — logical IMC conversation branch with its own metadata (hash,
  token count, message index). Decoupled from physical slots.
- **Slot** — physical batch-engine execution lane. Any session (text or media)
  can run on any available slot.
- **Sequence / seqID** — llama.cpp KV cache partition attached to the active
  slot during request processing.

#### Complete-Prompt Token Design

The token-v2 path supersedes the older two-tier message-hash and arbitrary
token-trimming design described in earlier releases. Text IMC now uses the
complete rendered token stream as its cache authority:

1. Clone and render the complete request twice: generation-ready, and stable
   with `add_generation_prompt=false`.
2. Tokenize both with BOS exactly once.
3. Require the stable tokens to be a strict prefix of the generation-ready
   tokens, leaving a nonempty inference tail.
4. Select the longest session whose entire cached sequence prefixes the stable
   tokens.
5. Restore an exact prefix, append only at the end of a complete prefix, or
   rebuild an empty/LRU session when content diverges.

IMC never renders an independent message suffix and never trims at an arbitrary
internal token boundary. Tool-call and tool-result turns therefore remain one
template-valid conversation and require no synthetic user message. The
nonempty tail also captures the fresh hidden state Gemma4's shared-KV MTP needs
before drafting after a restore.

##### Legacy design (pre token-v2)

The following description is retained only to explain older log files. It is
not the current matching algorithm.

IMC tracks two independent hashes per session:

| Tier   | What It Covers                                 | Purpose                                |
| ------ | ---------------------------------------------- | -------------------------------------- |
| Tier 1 | System prompt (`messages[0]` when role=system) | Preserved across conversation edits    |
| Tier 2 | All cached messages (`messages[0:N]`)          | Detects any change in the conversation |

When a request arrives, IMC first checks the full prefix hash (Tier 2). If it
matches, the cache is extended as normal. If the full hash mismatches but the
system prompt hash (Tier 1) still matches, IMC keeps the system prompt KV in
place and only re-decodes the conversation body after it. This is the most
common mutation scenario — the client edits conversation history while keeping
the same system prompt.

```
Normal append (full hash match):
┌─────────────────────────────────────────────────────────┐
│ System Prompt │ Msg 1  │ Msg 2  │ Msg 3  │  New Message │
│   (cached)    │(cached)│(cached)│(cached)│  (prefill)   │
└─────────────────────────────────────────────────────────┘

Conversation edit (sys prompt hash match, full hash mismatch):
┌─────────────────────────────────────────────────────────────────┐
│ System Prompt │ Msg 1'    │ Msg 2'    │ Msg 3'    │ New Message │
│   (cached)    │(re-decode)│(re-decode)│(re-decode)│(prefill)    │
└─────────────────────────────────────────────────────────────────┘
   ↑ kept in KV     ↑ trimmed and rebuilt from sys prompt boundary
```

**How IMC Detects Changes:**

IMC uses a cascading match algorithm. It always tries the fastest path first
and automatically falls back to slower-but-more-resilient strategies when the
fast path fails:

1. **Hash match** — Hash the incoming message prefix and compare against each
   session's stored hash. Instant, zero-tokenization overhead. This is the common
   case when the conversation grows normally (messages appended, nothing edited).

2. **System prompt preservation** — If the full hash mismatches but the system
   prompt hash (Tier 1) still matches, keep the system prompt KV in place and
   re-decode only the conversation body. This handles the common case where the
   client edits or drops messages while keeping the same system prompt.

3. **Token prefix fallback** — If no hash matches at all, tokenize the incoming
   messages and compare element-by-element against cached sessions to find the
   longest common prefix. Trim the divergent suffix and decode only the new
   tokens. This salvages 70-80% of cached tokens when templates, tool call
   formatting, or client behavior causes token-level differences even though
   the conversation is logically the same.

4. **Full rebuild** — No usable match found. Pick an empty session or evict the
   LRU session and build the cache from scratch.

The matching algorithm is independent of the model type (Dense, MoE, Hybrid).
What changes per model type is how the batch engine manages state between
requests — see [Section 4.9](#49-model-types-and-state-management).

**IMC is Best for:**

- AI coding agents
- Long-running agent conversations
- Agentic workflows where conversations grow or are edited
- Sub-agent architectures with multiple concurrent agents

**Enable IMC:**

```yaml
# ~/.kronk/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  incremental-cache: true
  cache-min-tokens: 100 # Minimum tokens before caching (default)
```

#### Session Pool (Decoupled from Slots)

The IMC session pool is **decoupled** from the batch engine's execution
slots. The pool is sized at `NSeqMax × 3` (the `imcSessionMultiplier`
constant). With `nseq-max: 2`, six cache identities can stay warm; with
`nseq-max: 4`, twelve. Idle session structs cost only a few hundred bytes
each — the `SessionStore` backing buffer is allocated lazily on first use.

Each session independently tracks its own conversation branch (message
hash, system prompt hash, token count, message index, cached tokens) and
externalizes its KV bytes to host RAM between requests via
`StateSeqGetData`. On the next request the matched session is bound to
the first free execution slot and its bytes are restored via
`StateSeqSetData`. Sessions and slots have no static affinity — any
session can run on any slot.

Why a multiplier instead of `NSeqMax` sessions? In agentic workloads a
driver loop plus a handful of sub-agents plus the occasional side
conversation easily exceeds the number of parallel decode lanes you can
afford to run on a single GPU. Sizing the cache identity pool above the
execution slot count keeps the LRU eviction path quiet during normal
multi-agent operation without forcing you to raise `nseq-max` (and pay
the VRAM cost) just to avoid thrashing.

```
nseq-max = 2           → 6 IMC sessions (cache identities), 2 slots (parallel decodes)
nseq-max = 4           → 12 IMC sessions, 4 slots
nseq-max = 8           → 24 IMC sessions, 8 slots
```

With unified KV cache, all slots share the same `n_ctx` pool, so adding
slots does not multiply VRAM usage. Adding sessions does not allocate KV
memory either — only the RAM-side `SessionStore` grows as conversations
accumulate. KV pressure eviction automatically clears stale sessions
when space gets tight — see [KV Pressure Eviction](#kv-pressure-eviction).

**Sizing guidance:** Set `nseq-max` to the level of decode parallelism
you want (concurrent in-flight requests). The session pool will be 3×
that, which is the right shape for typical sub-agent fan-out. If you
know your workload spawns more than 3× sub-agents, raise `nseq-max`
deliberately so both the decode parallelism and the cache pool keep up.

#### Pure Hit Snapshot Skip

A **pure hit** is the strongest possible match: the incoming request's
cacheable messages are byte-for-byte identical to what the session
already cached (`cachedMsgCount == len(messages) - 1`). Nothing needs to
be re-decoded beyond the suffix.

On every IMC cache hit, the engine normally:

1. Restores the externalized `kvState` into the slot's sequence via
   `StateSeqSetData`.
2. After build/extend (or no-op for a pure hit), serializes the KV state
   back out via `StateSeqGetData` so the next request can restore it.

For a pure hit, step 2 is a byte-for-byte round trip of the bytes that were
just restored in step 1 — pure I/O with no information change. The
**pure-hit snapshot-skip** optimization detects this case for exact text and
media plans and skips `StateSeqGetData` entirely.

Qualification (all must hold):

- No prefix mutation in this request: no extension tokens, no media
  build, no trim, no clear.
- For media sessions, the stored logical prompt plan exactly equals the
  request's stable plan, including ordered media SHA-256 identities.
- The session's committed render-input fingerprint
  (`cachedRenderInputHash`) equals the current request's fingerprint
  (template, tools, `add_generation_prompt`, `preserve_thinking`, exact
  cacheable messages). This guards against template or top-level
  parameter changes that would silently invalidate the cached prefix.
- The session has not been mutated by a concurrent request between
  `processIMC` and `startSlot` (re-validated under `cacheMu` at the
  decode boundary).
- For models with an MTP drafter, the draft sequence's state was
  restored successfully alongside the target.

When the skip fires, the log emits `imc-snapshot-skip-read-only` with
`snapshot_action=skip-read-only`, and the
`imc_snapshot_skipped_total` counter increments. When a pure-hit
candidate races a concurrent extend, the request is failed with
`imc-pure-hit-stale` and the client retries — the next attempt sees the
newer session version and goes through the normal extend path. The
optimization is safe because `llama_state_seq_get_data` is a host-side
serializer: skipping it cannot leave KV state in a bad shape.

**How It Works:**

First request (2 messages: system + user):

```
Messages: [system, user]
Cache:    [system]           ← Cache all except last
Prefill:  [user + gen_prompt]
```

Second request (4 messages):

```
Messages: [system, user, assistant, user2]
Cache:    [system, user, assistant]  ← Extend cache
Prefill:  [user2 + gen_prompt]
```

Third request (6 messages):

```
Messages: [system, user, assistant, user2, assistant2, user3]
Cache:    [system, user, assistant, user2, assistant2]  ← Extend
Prefill:  [user3 + gen_prompt]
```

Fourth request (conversation edited — assistant response removed):

```
Messages: [system, user, user3]
Cache:    [system]                   ← System prompt KV preserved
Rebuild:  [user, user3]              ← Only conversation body re-decoded
Prefill:  [user3 + gen_prompt]
```

#### Session Selection Algorithm

> **Current token-v2 behavior:** text sessions are selected by longest complete
> token-prefix match, not by the legacy message-hash cascade below. Exact,
> append, and rebuild selections reserve their session immediately. The
> reservation is released on submission failure, cancellation, decode failure,
> panic before batch handoff, or normal slot completion. Media sessions are
> reused on exact logical-plan identity or when the stored media plan is an
> immutable prefix followed only by new text. Changed, reordered, removed, or
> newly appended media rebuilds through the authoritative mtmd pipeline.

The algorithm below documents pre token-v2 behavior for interpreting old logs.

When a request arrives, IMC scans all sessions to find the best match.
The algorithm has five steps, tried in order. After a session is selected,
the batch engine assigns the request to the first available slot. The
session's KV state is restored from RAM into the assigned slot.

1. **Scan all sessions** — For each session:
   - Skip sessions with a build in-flight (pending flag set)
   - Skip empty sessions (track them as fallback candidates)
   - Skip sessions with more cached messages than the request has total
   - Hash `messages[:session.cachedMsgCount]` and compare to the session's
     stored hash
   - On mismatch: check if the system prompt hash (Tier 1) still matches.
     Track the session as a system-prompt-match candidate if it does.
   - Track mismatched sessions as eviction candidates

2. **KV pressure eviction** — When a matching session is found and the total
   KV usage across all sessions exceeds the context window, evict mismatched
   sessions (largest first) to reclaim space. Sessions with externalized
   `kvState` do not count against VRAM KV pressure because their VRAM
   sequences are already cleared. See
   [KV Pressure Eviction](#kv-pressure-eviction) for details.

3. **On full match** — Pick the session with the best prefix coverage (most
   cached messages). Two sub-paths:
   - **Extend** — request has new messages beyond what the session cached:
     decode the extension and snapshot the new KV state back to the session.
   - **Pure cache hit** — cached messages exactly equal the request's
     cacheable prefix (`cachedMsgCount == len(messages) - 1`). The
     session's externalized KV is restored into the slot and the suffix is
     decoded directly. Text-only pure hits additionally qualify for the
     snapshot-skip fast path (see [Pure Hit Snapshot Skip](#pure-hit-snapshot-skip))
     — skipping the round-trip `StateSeqGetData` call cuts noticeable CPU
     and RAM bandwidth off cache-hit-heavy workloads.

4. **System prompt preservation (two-tier hash)** — No full match, but a
   session has the same system prompt cached. Keep the system prompt KV in
   place, trim everything after the system prompt token boundary, and
   re-template and re-decode only the conversation body. Before preserving,
   IMC verifies the system prompt token boundary is consistent after
   re-templating — if the template produces a different token count for the
   system prompt, it falls back to a full rebuild.

5. **Token prefix fallback** — Tokenize the incoming messages and compare
   the resulting token sequence element-by-element against each non-empty
   session's stored `cachedTokens`. Pick the session with the longest
   common prefix that meets `cache-min-tokens`. Trim the KV cache from the
   divergence point and decode only the new tokens from there forward. See
   [Token Prefix Fallback](#token-prefix-fallback) for details.

6. **No match at all** — Pick an empty session if one exists, otherwise
   evict the least-recently-used (LRU) session and rebuild from scratch.

**Concurrent Build Protection:**

When two requests arrive simultaneously and both need to build a cache from
scratch, a race condition could cause both to pick the same empty session.
IMC prevents this with a pending flag: when a session begins a deferred cache
build, it is marked pending. Concurrent scanners skip pending sessions, so
the second request picks a different session. The pending flag is cleared
after the cache decode completes (or on error).

The publish path is split into **two phases** to close a second race in
which a concurrent reader could observe fresh metadata but stale or empty
externalized KV bytes:

1. `imcCommitSession` updates the session's metadata (hash, token count,
   cached tokens, render-input fingerprint) under `cacheMu`. The `pending`
   flag stays set — concurrent scanners still skip the session.
2. `imcPublishSession` clears `pending` and broadcasts availability — but
   only after `startSlot` has externalized the KV state via
   `StateSeqGetData` into `session.kvState`. Now metadata and bytes are
   guaranteed consistent.

**Decode Failure Recovery:**

If a cache decode fails at any point (extend, rebuild, trim, or media build),
IMC clears the entire KV sequence and resets the session metadata. This
ensures the slot never advertises cached content that doesn't exist in the
KV cache.

#### KV Pressure Eviction

With `nseq-max > 1`, Kronk enables a unified KV cache (`KVUnified=1`) so that
all sequences share the full `n_ctx` pool. Any single sequence can grow up to the
full context window, but the **total** KV usage across all sequences cannot exceed
`n_ctx`.

All sessions externalize their KV state to RAM after each request and clear
their VRAM sequence, so they do not contribute to VRAM KV pressure between
requests. However, during active processing, a session's restored KV does
consume VRAM cells until the request completes and the state is externalized
again.

**Example:** With `nseq-max: 3` and `context-window: 131072`:

```
Session 0: 854 tokens    (stale media — 2 cached messages, hash mismatch)
Session 1: 46,541 tokens (stale media — 17 cached messages, hash mismatch)
Session 2: 86,682 tokens (active media — 49 cached messages, hash match)
Total VRAM-resident: 134,077 tokens > 131,072 → context window full!
```

Without KV pressure eviction, the next decode would fail with "context window
is full" even though the active conversation only uses ~87k of the 131k window.

**How It Works:**

After the session scan finds a matching session (Step 1), IMC checks whether
the projected total KV usage across all sessions exceeds the context window.
If it does, mismatched sessions are evicted largest-first until the total
fits:

1. Sum `totalTokensCached` across all non-empty, non-pending sessions
   (sessions with externalized `kvState` are excluded since their VRAM
   is already freed)
2. If the sum exceeds `context-window`, sort mismatched sessions by token
   count (descending)
3. Evict sessions one at a time — clear the KV sequence (`MemorySeqRm`) and
   reset the session metadata — until the projected total is within bounds

In the example above, evicting Session 1 (46,541 tokens) brings the total to
87,536 — well within the 131,072 limit. Session 0 (854 tokens) may or may not
need eviction depending on the remaining headroom.

**Key Points:**

- Eviction only targets **mismatched** sessions — the active session and any
  other matching sessions are never evicted
- Pending sessions (with a build in-flight) are never evicted
- Sessions with externalized `kvState` do not count toward VRAM pressure
  and are not eviction candidates (their VRAM is already freed)
- Evicted sessions become empty and are available for future cache builds
- The eviction check runs before the extend/hit path, so the active session
  always has room to grow
- No configuration needed — eviction triggers automatically when KV pressure
  is detected

#### Media and M-RoPE

IMC supports mtmd image, audio, and multipart requests. A media prompt plan
contains text-token units and ordered media identities derived from immutable
input bytes. BOS is added globally exactly once; vocabularies that automatically
append EOS conservatively fall back to mtmd rebuilds because an old terminal EOS
cannot safely become an interior prefix token. Raw media and identities are not
logged.

Exact plans restore the externalized KV state without re-encoding media and
skip redundant snapshot serialization. When the stored plan is a complete
prefix of the new stable plan and everything after it is text, IMC selects an
**anchor** match. It restores the media KV, decodes only the authoritative text
extension, snapshots the enlarged prefix into a separate `SessionStore`, and
atomically swaps the snapshot and metadata. A failed decode or staged snapshot
leaves the previous anchor valid. Changed, reordered, removed, or newly appended
media rebuilds through mtmd so model-specific chunk boundaries and embeddings
remain authoritative.

M-RoPE handling distinguishes physical image embeddings (`n_tokens`) from
logical position advancement (`n_pos`). Images use `[t, y, x, 0]` positions;
following text broadcasts each logical position across all four planes. An
exact or anchor-restored request resumes suffix decoding at the saved logical position.
Sessions persist both the physical KV-cell count (for capacity and prompt
accounting) and the next logical position (for M-RoPE continuation).
M-RoPE layouts whose embedding count is not the rectangular `nx*ny` grid are
rejected rather than decoded with guessed coordinates; they require mtmd's
per-token decoder-position API to be exposed by the Go binding.

Useful structured events are:

| Event | Important fields |
| ----- | ---------------- |
| `imc status=plan-ready` | `cache_mode=token-v2`, `match_kind`, `match_reason`, `reusable_tokens`, `extension_tokens`, `tail_tokens`, `actual_tokens`, `stable_tokens` |
| `imc-media-cache status=plan-ready` | `media_count`, `logical_units`, `text_tokens`, `match_kind`, `match_reason`, `anchor_physical_kv`, `anchor_logical_position`, `extension_text` |
| `start-slot status=imc-restore-done` | `next_logical_position`, `physical_kv_cells`, `restored_bytes`, `elapsed` |
| `start-slot status=imc-snapshot-done` | `next_logical_position`, `physical_kv_cells`, `snapshot_bytes`, `buf_action`, `elapsed` |
| `start-slot status=imc-snapshot-skip-read-only` | exact prefix reused without reserializing it; `snapshot_action=skip-read-only` |
| `start-slot status=imc-media-anchor-advanced-in-slot` | appended stable text decoded without re-encoding media |
| `start-slot status=imc-media-anchor-committed` | staged snapshot and matching physical/logical metadata swapped atomically |
| `imc-media-cache status=complete` | `logical_positions`, `physical_kv_cells`, ordered `media_kv_cells` counts |
| `slot-finished` | `imc_active` reports IMC participation; `imc_cache_hit` reports successful prior-snapshot restoration; also carries `imc_cache_mode`, `imc_match_kind`, `imc_tail_tokens`, and MTP totals |

##### Legacy token-prefix fallback (pre token-v2)

The following section describes the retired trim-at-divergence fallback and is
retained only for interpreting old logs.

When hash matching fails — whether because the client edited messages, a
template produced slightly different tokens, or the agent didn't send exactly
the same conversation — IMC falls back to token-level prefix matching to
salvage as much of the cached KV state as possible.

**When it activates:** Automatically when no hash match and no system prompt
match is found during the session scan (Step 5 of the
[Session Selection Algorithm](#session-selection-algorithm)). IMC compares the
actual cached token arrays against the incoming request's tokens. Only
candidates with compatible message counts are considered — the request must
have at least as many messages as the session cached.

**How it works:**

IMC tokenizes the incoming messages and compares them element-by-element
against each non-empty session's stored token sequence to find the longest
common prefix.

```
Cached tokens:   [T1, T2, T3, T4, T5, T6, T7, T8]
Incoming tokens: [T1, T2, T3, T4, T5, T9, T10, T11, T12]
                                       ↑
                              Divergence point (pos 5)

Common prefix: 5 tokens (salvaged from KV cache)
Trimmed:       3 tokens (T6-T8 removed from KV cache)
New decode:    4 tokens (T9-T12, from divergence point forward)
```

If the common prefix meets the `cache-min-tokens` threshold, IMC:

1. Reserves the matching session (marks it pending)
2. Trims the divergent suffix from the KV cache
3. Decodes only the new tokens from the divergence point forward
4. Updates the session's hash and cached token sequence

Once the partial rebuild completes, subsequent requests in the same
conversation use normal hash-based extending.

Real-world testing showed 77-80% cache salvage rates. Instead of decoding
~8400 tokens from scratch, the system kept ~6800 cached and only decoded
~1600.

**Debugging token prefix fallback:**

| Log Message                                         | Meaning                                                               |
| --------------------------------------------------- | --------------------------------------------------------------------- |
| `no slot matched, trying token prefix match`        | Hash match failed, entering token comparison                          |
| `slot[N] common-prefix X/Y tokens (Z% salvageable)` | Per-slot comparison result                                            |
| `token prefix match found`                          | Usable prefix found, will trim and extend                             |
| `imc-trim-prefix`                                   | KV cache trim in progress (shows cached_tokens, trim_pos)             |
| `imc-partial-rebuilt`                               | Rebuild complete (shows total_cached, salvaged_prefix, salvaged_pct)  |
| `no usable token prefix match`                      | All prefixes below `cache-min-tokens`, falling back to empty/LRU slot |

#### Model Type Interactions

The IMC matching algorithm is the same for all model types (Dense, MoE,
Hybrid). Only the batch engine's state management differs. See
[Section 4.9](#49-model-types-and-state-management) for how each model type
manages state between requests.

| Model Type | State Management   | Configuration Notes               |
| ---------- | ------------------ | --------------------------------- |
| Dense      | Snapshot/Restore   | No special requirements           |
| MoE        | Snapshot/Restore   | f16 cache, split-mode: row        |
| Hybrid     | Snapshot/Restore   | quantized KV needs flash attn on  |

**MoE Configuration:**

```yaml
# ~/.kronk/model_config.yaml
unsloth/Qwen3.6-35B-A3B-UD-Q4_K_M:
  incremental-cache: true
  split-mode: row     # Best for MoE architecture
  cache-type-k: f16   # Safer for MoE routing accuracy
  cache-type-v: f16
```

**Hybrid Configuration:**

```yaml
# ~/.kronk/model_config.yaml
unsloth/LFM2-700M-Q8_0:
  incremental-cache: true
  flash-attention: auto   # Falls back to disabled when the backend lacks FA
  cache-type-k: f16       # Use f16 unless flash attention is active
  cache-type-v: f16
```

### 5.3 Multi-Session Caching

The session pool (sized at `NSeqMax × 3`, see
[Session Pool (Decoupled from Slots)](#session-pool-decoupled-from-slots)) gives
multiple conversation branches independent cached token sequences or media
plans. Any session can run on any execution slot. This supports concurrent
clients and agent branches within the configured pool; clients do not need
sticky slot routing.

### 5.4 When to Use IMC

IMC caches a complete stable rendering of conversation history and uses full
token-prefix matching. It is best suited for:

- **Agentic workflows** — tool-result turns can append to a complete cached
  rendering without independently templating a suffix
- **AI coding agents** — long-running conversations with growing context
- **Sub-agent architectures** — each active branch can retain an independent
  externalized session

| Feature      | Behavior                                                                       |
| ------------ | ------------------------------------------------------------------------------ |
| Caches       | Complete stable rendering (`add_generation_prompt=false`)                      |
| Extends      | Yes, incrementally                                                             |
| Sessions     | Session pool sized at `NSeqMax × 3`                                            |
| Slot routing | Any available slot (no session/slot affinity)                                  |
| Sub-agents   | Active branches select independent token-prefix sessions                       |
| Pure hits    | Snapshot-skip fast path on exact text or media plans                           |
| Best for     | Agentic workflows                                                              |
| VRAM         | Unified `n_ctx` pool, not multiplied by `nseq-max`                             |
| RAM          | One externalized KV snapshot per active session (lazy-grow / never-shrink)     |

### 5.5 Cache Invalidation

Cached state doesn't last forever. Kronk compares complete rendered token
sequences (or exact media plans) and rebuilds when no complete prefix is safe
to reuse.

**IMC Invalidation:**

- Exact complete token prefix → restore without rebuilding
- Complete append-only token prefix → restore and decode the extension
- Earlier text changed, removed, or reordered → full rebuild in empty/LRU session
- Media changed, removed, reordered, or appended → full rebuild through mtmd
- Stable render is not a strict prefix of the generation-ready render → IMC is
  rejected for that request rather than decoding an independently rendered suffix

**Automatic Invalidation:**

Caches are cleared when:

- Model is unloaded
- Server restarts

### 5.6 Configuration Reference

IMC is enabled by default for all models. No configuration is needed to use it. To disable IMC for a specific model, set `incremental-cache: false` in your `model_config.yaml`:

```yaml
Qwen/Qwen3-8B-Q8_0:
  incremental-cache: false   # Disable IMC for this model
```

You can also tune the minimum cache threshold:

```yaml
Qwen/Qwen3-8B-Q8_0:
  cache-min-tokens: 100   # Don't cache if < 100 tokens (default: 100)
```

**cache-min-tokens**

Minimum common prefix length required for token-level partial prefix
matching. If no session's cached tokens share at least this many tokens with
the incoming request, the fallback is skipped and the cache is rebuilt from
scratch.

Default: 100 tokens

**session-store-kind / session-store-dir**

Selects the backend used to externalize each IMC session's KV cache
bytes between requests. Each backend lives in its own subpackage
under `sdk/kronk/kvstorage/<kind>/`, mirroring the parser-plugin
layout under `sdk/kronk/parsers/<name>/`.

```yaml
# Default — keep KV snapshots in process RAM
Qwen/Qwen3-8B-Q8_0:
  session-store-kind: ram

# Persist KV snapshots to disk (required when RAM is the bottleneck)
Qwen/Qwen3-8B-Q8_0:
  session-store-kind: disk
  session-store-dir: /var/lib/kronk/sessions
```

| Kind   | Subpackage       | Description                                                                                                                                                                |
| ------ | ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ram`  | `kvstorage/ram`  | Default. One Go-allocated `[]byte` per session with lazy-grow / never-shrink semantics. Zero configuration.                                                                |
| `disk` | `kvstorage/disk` | Per-session file under `session-store-dir`. Trades RAM for disk I/O on each snapshot/restore. Use when (NSeqMax × peak-conversation-KV) bytes of RAM is more than you can spare. |

Default: `ram` (used when the field is empty).

The disk backend creates each session's file via `os.CreateTemp` so
file names are unique within the directory; files are removed on
`Model.Unload`. On a process crash the per-session files are leaked
under `session-store-dir` and must be reclaimed out-of-band (cron,
`systemd-tmpfiles`, or a manual sweep). The directory must already
exist and be writable; it is not created on demand.

Additional backends (network-attached, NVMe-direct) are reserved for
future phases.

### 5.7 Performance and Limitations

IMC improves request latency by skipping redundant prefill work. It delivers
large savings for multi-turn conversations but imposes restrictions on
template behavior and session management.

**IMC Prefill Savings:**

For a 2000-token cached conversation prefix:

- Without cache: ~200ms prefill (varies by hardware)
- With IMC: ~5ms for new tokens only

Cache extensions (adding new messages to an existing cached prefix) are
especially fast because only the delta tokens are decoded. In production
logs, sequential extensions typically take ~3ms each.

**IMC Memory Overhead:**

IMC adds no extra VRAM beyond what the context window already requires.
With `nseq-max > 1`, Kronk enables a unified KV cache where all sequences
share the full `n_ctx` pool. The total KV cache size is determined by
`context-window`, not multiplied by the number of sessions:

```
131K context, nseq-max=3, IMC (unified KV cache):
  Total KV cache: ~3.2 GB (8B model, F16)
  Any single slot can use up to the full 131K tokens
  Total across all slots cannot exceed 131K tokens
```

Sessions do not pin their prefix KV in VRAM between requests — the
cached prefix is snapshotted to RAM and the VRAM sequence is cleared.
This means sessions consume **RAM** (one KV snapshot per active session)
but no VRAM KV cells between requests. The RAM cost varies by
conversation length and model size. The default `SessionStore` backend
(`kvstorage/ram`) uses lazy-grow / never-shrink semantics: a session's
buffer grows to its peak conversation size and stays there, so
subsequent turns reuse the backing array without allocation churn.
With a session pool of `NSeqMax × 3`, plan the RAM budget around peak
conversation size times the number of branches you expect to keep warm
concurrently — idle sessions cost only the struct (a few hundred bytes)
because the buffer is allocated lazily on first use.

KV pressure eviction only considers sessions whose cached KV is still
resident in VRAM (sessions without an externalized `kvState`). Sessions
with externalized state are excluded from VRAM pressure calculations.

**IMC Planning Cost:**

Text IMC renders and tokenizes two complete prompts for planning. This adds a
small host-side cost but avoids unsafe suffix rendering and typically saves far
more time by restoring the model's cached prefill state.

**IMC with Vision/Audio Models:**

IMC supports mtmd image, audio, and multipart requests. Exact complete media
plans restore text plus media KV from RAM without re-encoding. Text-only
follow-ups use an anchor restore and atomically advance the stored snapshot.
Any changed, reordered, removed, or newly appended media rebuilds through mtmd;
this rule keeps mtmd's model-specific token, embedding, and M-RoPE stream
authoritative.

For example, in a conversation like:

```
Request 1 (image request):
[system]       →  cached by IMC (text tokens)
[user + image] →  cached by IMC (text + image embeddings via mtmd pipeline)
[user]         →  prefill (generation target)

Request 2 (text follow-up about the image):
[system]       →  restored from RAM (no re-encode)
[user + image] →  restored from RAM (image KV preserved, no re-encode)
[assistant]    →  extended (new text tokens decoded into cache)
[user]         →  prefill (generation target)

Request 3 (unrelated text question):
[system]       →  restored from RAM
[user + image] →  restored from RAM (image KV preserved)
[assistant]    →  restored from RAM
[user]         →  extended (new text tokens decoded into cache)
[assistant]    →  extended
[user]         →  prefill (generation target)

Request 4 (back to asking about the image):
[system]       →  restored from RAM
[user + image] →  restored from RAM (image KV preserved, no re-encode)
[assistant]    →  restored from RAM
[user]         →  restored from RAM
[assistant]    →  restored from RAM
[user]         →  extended (new text tokens decoded into cache)
[assistant]    →  extended
[user]         →  prefill (generation target)
```

When an image first appears mid-conversation, the complete stable media plan is
built through mtmd. Once that media snapshot exists, subsequent text-only turns
anchor to it and do not rerun projection:

```
Text-only conversation, then image appears mid-conversation:

Requests 1–3 (text-only):
[system]       →  cached by IMC (text tokens)
[user]         →  cached / extended normally
[assistant]    →  cached / extended normally
...            →  conversation grows, all text cached incrementally

Request 4 (image appears mid-conversation):
[system]       →  rebuilt through the stable media plan
[earlier msgs] →  rebuilt through the stable media plan
[user + image] →  image encoded once through the projection model
[user]         →  prefill (generation target)

Request 5 (text follow-up about the image):
[all prior]    →  restored from RAM (image KV preserved, no re-encode)
[assistant]    →  extended (text tokens only, no image re-encode)
[user]         →  prefill (generation target)
```

**How media caching works internally:**

1. `buildPromptPlan` splits the stable and generation-ready renders at mtmd's
   marker, tokenizes each text segment without per-segment special tokens, adds
   BOS globally once, and inserts ordered SHA-256 media units.

2. `processIMCMediaTokenPlan` selects `exact`, `anchor`, or `rebuild`. Anchors
   require a nonempty valid snapshot, unchanged ordered media, a complete plan
   prefix, and a text-only extension.

3. A rebuild defers to `startSlot`, where `decodeMediaIntoCache` processes the
   prompt as interleaved chunks — text
   chunks are tokenized and decoded normally, while image/audio chunks are
   encoded through the projection model and their embeddings are decoded into
   the KV cache. For models using M-RoPE, 2D spatial positions are assigned to
   image tokens.

4. The session tracks `mediaKVCounts` — the number of KV positions consumed
   by each media chunk. This is needed because media embeddings occupy a
   different number of KV positions than the text marker tokens they replace
   in the tokenized prompt.

5. On text-only follow-ups, the engine restores the target snapshot at the
   stored logical position and decodes `imcNewCacheTokens`. Linear models use
   normal text positions; M-RoPE models broadcast each new logical text
   position across all four planes. No marker-token offset arithmetic or image
   re-encoding is involved.

6. The advanced target state is serialized into a new `SessionStore`. Only a
   successful snapshot atomically replaces `kvState`, `promptPlan`, physical
   KV count, logical position, message/hash metadata, and render fingerprint.

7. If a new message adds or changes media, prefix identity fails and the full
   stable plan rebuilds through mtmd.

**IMC Limitations:**

- Editing earlier messages triggers a full rebuild; arbitrary internal token
  trimming is intentionally not used
- Max concurrent conversation branches = `NSeqMax × 3` (session pool size);
  when all sessions are occupied, the least-recently-used session is evicted
- Cache hits include a RAM→VRAM restore step (typically 10-30ms depending
  on conversation size). The pure-hit snapshot-skip fast path avoids the
  subsequent VRAM→RAM round trip when an exact text or media prefix is not
  mutated — see
  [Pure Hit Snapshot Skip](#pure-hit-snapshot-skip).
- When a new media message appears in the conversation, the cache is
  rebuilt through the mtmd pipeline (projection model encodes image/audio
  into embeddings)

---
