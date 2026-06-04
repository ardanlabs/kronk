# Chapter 5: Message Caching

## Table of Contents

- [5.1 Overview](#51-overview)
- [5.2 Incremental Message Cache (IMC)](#52-incremental-message-cache-imc)
  - [Two-Tier Hash Design](#two-tier-hash-design)
  - [Session Pool (Decoupled from Slots)](#session-pool-decoupled-from-slots)
  - [Pure Hit Snapshot Skip](#pure-hit-snapshot-skip)
  - [KV Pressure Eviction](#kv-pressure-eviction)
  - [Token Prefix Fallback](#token-prefix-fallback)
  - [Model Type Interactions](#model-type-interactions)
- [5.3 Single-User Caching](#53-single-user-caching)
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

Incremental Message Cache is designed for agentic workflows. It caches all
messages except the last one and extends the cache incrementally on each turn.
When a client or agent mutates the conversation history, IMC uses a two-tier
hash to preserve the system prompt KV state and only rebuild the conversation
body.

**Key Terminology:**

- **Session** — logical IMC conversation branch with its own metadata (hash,
  token count, message index). Decoupled from physical slots.
- **Slot** — physical batch-engine execution lane. Any session (text or media)
  can run on any available slot.
- **Sequence / seqID** — llama.cpp KV cache partition attached to the active
  slot during request processing.

#### Two-Tier Hash Design

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

For a pure hit on a text-only session, step 2 is a byte-for-byte round
trip of the bytes that were just restored in step 1 — pure I/O with no
information change. The **pure-hit snapshot-skip** optimization detects
this case and skips `StateSeqGetData` entirely.

Qualification (all must hold):

- Text-only session — media sessions also externalize KV, but the
  optimization gates on `!hasMedia` to keep the predicate small.
- No prefix mutation in this request: no extension tokens, no media
  build, no trim, no clear.
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

When the skip fires, the log emits `imc-snapshot-skip-pure-hit` and the
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

#### Token Prefix Fallback

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
| Hybrid     | Snapshot/Restore   | f16 cache required, no flash attn |

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
  cache-type-k: f16   # Required for hybrid models
  cache-type-v: f16   # Required for hybrid models
```

### 5.3 Single-User Caching

IMC is designed for single-user use. The session pool (sized at `NSeqMax × 3`,
see [Session Pool (Decoupled from Slots)](#session-pool-decoupled-from-slots))
gives multiple conversation branches their own cache identity; each branch
independently tracks its own hash, system prompt, and cached tokens. Any
session can run on any execution slot. This design is optimized for agentic
workflows where multiple sub-agents send independent conversations (different
system prompts, different message histories) without saturating the GPU's
parallel decode capacity.

### 5.4 When to Use IMC

IMC caches the entire conversation history and uses hash matching with
automatic token prefix fallback when changes are detected. It is best suited
for:

- **Agentic workflows** — hash matching handles the common case, and token
  prefix fallback automatically salvages 70-80% of cached tokens when changes
  are detected
- **AI coding agents** — long-running conversations with growing context
- **Sub-agent architectures** — each sub-agent gets its own session via hash
  matching, maintaining independent caches

| Feature      | Behavior                                                                       |
| ------------ | ------------------------------------------------------------------------------ |
| Caches       | All messages except last                                                       |
| Extends      | Yes, incrementally                                                             |
| Sessions     | Session pool sized at `NSeqMax × 3`, single-user                               |
| Slot routing | Any available slot (no session/slot affinity)                                  |
| Sub-agents   | Each gets own session via hash matching                                        |
| Pure hits    | Snapshot-skip fast path on text-only exact-match (no round-trip `GetData`)     |
| Best for     | Agentic workflows                                                              |
| VRAM         | Unified `n_ctx` pool, not multiplied by `nseq-max`                             |
| RAM          | One externalized KV snapshot per active session (lazy-grow / never-shrink)     |

### 5.5 Cache Invalidation

Cached state doesn't last forever. Kronk uses hash comparisons to detect
when cached tokens no longer match the incoming request, and automatically
rebuilds the cache when a mismatch is found. Understanding what triggers
invalidation helps you avoid unexpected prefill costs.

**IMC Invalidation:**

- Message prefix hash mismatch with same system prompt → system prompt KV
  preserved, conversation body trimmed and re-decoded (Step 4 of the session
  selection algorithm)
- Message prefix hash mismatch with no system prompt match → token prefix
  fallback attempted (see [Token Prefix Fallback](#token-prefix-fallback)).
  If a common prefix ≥ `cache-min-tokens` is found, only the divergent suffix
  is trimmed and rebuilt. Otherwise, cache is rebuilt from scratch.
- System prompt changed → full cache rebuild from scratch
- Conversation shrinks (client dropped messages or reasoning blocks) → system
  prompt preserved if unchanged, conversation body re-decoded

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

**IMC Token Prefix Fallback Performance:**

When IMC falls back to token-level prefix matching, there is a one-time cost
to tokenize the incoming messages for comparison. This is typically fast
(< 5ms for most conversations). The savings from salvaging 70-80% of the
cached tokens far outweigh this cost compared to a full rebuild.

**IMC with Vision/Audio Models:**

IMC fully supports vision and audio models (models configured with a projection
file). Text-only requests are cached normally. When a message containing media
(image, video, or audio) appears in the conversation history, IMC caches the
entire conversation — including the media embeddings — in the KV cache. The
image or audio is encoded through the projection model once. After the request,
the entire cached prefix (text + media KV) is snapshotted to RAM and restored
on the next request — media is never re-encoded unless the cache is rebuilt
from scratch. Text-only follow-up messages extend the cache without
re-encoding the media.

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

When an image appears mid-conversation (after text-only messages), IMC
preserves the existing text cache and extends it with media instead of
rebuilding from scratch:

```
Text-only conversation, then image appears mid-conversation:

Requests 1–3 (text-only):
[system]       →  cached by IMC (text tokens)
[user]         →  cached / extended normally
[assistant]    →  cached / extended normally
...            →  conversation grows, all text cached incrementally

Request 4 (image appears mid-conversation):
[system]       →  cached (text tokens skipped via imcMediaSkipTextTokens)
[earlier msgs] →  cached (text tokens skipped)
[asst + user]  →  media extend from text (new text decoded from skip point)
[user + image] →  media extend from text (image encoded through projection model)
[user]         →  prefill (generation target)

Request 5 (text follow-up about the image):
[all prior]    →  restored from RAM (image KV preserved, no re-encode)
[assistant]    →  extended (text tokens only, no image re-encode)
[user]         →  prefill (generation target)
```

**How media caching works internally:**

1. When `buildIMCCacheFromScratch` detects media content, it defers the build
   to `startSlot` where the mtmd pipeline (projection model) is available. The
   cache result carries `imcMediaBuild: true`.

2. When media first appears in a conversation that started text-only,
   `extendIMCTextCacheWithMedia` preserves the existing text prefix in the
   KV cache. It sets `imcMediaSkipTextTokens` to the number of already-cached
   text tokens, so `decodeMediaIntoCache` skips them and only decodes the new
   text plus media embeddings. This avoids re-decoding potentially tens of
   thousands of cached text tokens when an image is first introduced
   mid-conversation.

3. `decodeMediaIntoCache` processes the prompt as interleaved chunks — text
   chunks are tokenized and decoded normally, while image/audio chunks are
   encoded through the projection model and their embeddings are decoded into
   the KV cache. When `imcMediaSkipTextTokens` is set, the first text chunk
   is partially skipped (only tokens beyond the skip point are decoded). For
   models using M-RoPE (e.g., Qwen2.5-VL), 2D spatial positions are assigned
   to image tokens.

4. The session tracks `mediaKVCounts` — the number of KV positions consumed
   by each media chunk. This is needed because media embeddings occupy a
   different number of KV positions than the text marker tokens they replace
   in the tokenized prompt.

5. On text-only follow-ups, `extendIMCMediaSlotWithText` uses the
   `mediaKVCounts` to compute the correct offset between text token indices
   and KV positions, then decodes only the new text tokens at the right
   position — no image re-encoding occurs.

6. If a new message being added contains media (a second image, for example),
   `rebuildIMCWithMedia` triggers a full rebuild through the mtmd pipeline.

7. Token prefix matching is skipped when the incoming request contains media
   messages, since the tokenization path would mutate media content and corrupt
   downstream processing.

**IMC Limitations:**

- Editing earlier messages requires a partial rebuild (system prompt KV is
  preserved when the system prompt hasn't changed; conversation body is
  re-decoded)
- Changing the system prompt triggers a full cache rebuild
- Designed for single-user use
- Max concurrent conversation branches = `NSeqMax × 3` (session pool size);
  when all sessions are occupied, the least-recently-used session is evicted
- Cache hits include a RAM→VRAM restore step (typically 10-30ms depending
  on conversation size). The pure-hit snapshot-skip fast path avoids the
  subsequent VRAM→RAM round trip when the request is text-only and the
  cached prefix is not mutated — see
  [Pure Hit Snapshot Skip](#pure-hit-snapshot-skip).
- When a new media message appears in the conversation, the cache is
  rebuilt through the mtmd pipeline (projection model encodes image/audio
  into embeddings)

---
