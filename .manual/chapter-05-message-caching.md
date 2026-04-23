# Chapter 5: Message Caching

## Table of Contents

- [5.1 Overview](#51-overview)
- [5.2 Incremental Message Cache (IMC)](#52-incremental-message-cache-imc)
  - [Two-Tier Hash Design](#two-tier-hash-design)
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
prefill work. IMC dedicates each slot to a conversation and caches the
full message history in the slot's KV cache sequence, so only the new
message needs to be prefilled.

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

#### Two-Tier Hash Design

IMC tracks two independent hashes per slot:

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
   slot's stored hash. Instant, zero-tokenization overhead. This is the common
   case when the conversation grows normally (messages appended, nothing edited).

2. **System prompt preservation** — If the full hash mismatches but the system
   prompt hash (Tier 1) still matches, keep the system prompt KV in place and
   re-decode only the conversation body. This handles the common case where the
   client edits or drops messages while keeping the same system prompt.

3. **Token prefix fallback** — If no hash matches at all, tokenize the incoming
   messages and compare element-by-element against cached slots to find the
   longest common prefix. Trim the divergent suffix and decode only the new
   tokens. This salvages 70-80% of cached tokens when templates, tool call
   formatting, or client behavior causes token-level differences even though
   the conversation is logically the same.

4. **Full rebuild** — No usable match found. Pick an empty slot or evict the
   LRU slot and build the cache from scratch.

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
models:
  Qwen3-8B-Q8_0:
    incremental_cache: true
    cache_min_tokens: 100 # Minimum tokens before caching (default)
```

#### Multi-Slot Architecture

All `NSeqMax` slots are available for IMC. Each slot independently tracks its
own conversation branch — its own message hash, system prompt hash, token
count, and message index. Sub-agents are routed to different slots via hash
matching, allowing them to maintain independent caches and run concurrently.

With `n_seq_max: 3`, three sub-agents can each have their own cached
conversation branch. Without multi-slot IMC, every sub-agent request would
cause a prefix mismatch and rebuild the cache from scratch because different
sub-agents send different system prompts and conversation content.

**Important:** Set `n_seq_max` to at least the number of concurrent
sub-agents your agent framework spawns. If `n_seq_max` is smaller than
the number of sub-agents, cache thrashing can occur — each new sub-agent
evicts a slot, and when the evicted sub-agent returns, it evicts another.
Every request triggers a full rebuild from scratch, eliminating the
caching benefit entirely. With unified KV cache, all slots share the same
`n_ctx` pool, so adding more slots does not multiply VRAM usage. However,
more slots means more concurrent cached conversations competing for the
shared pool. KV pressure eviction automatically clears stale slots when
space gets tight — see [KV Pressure Eviction](#kv-pressure-eviction).

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

#### Slot Selection Algorithm

When a request arrives, IMC scans all slots to find the best match. The
algorithm has five steps, tried in order.

1. **Scan all slots** — For each slot:
   - Skip slots with a build in-flight (pending flag set)
   - Skip empty slots (track them as fallback candidates)
   - Skip slots with more cached messages than the request has total
   - Hash `messages[:slot.cachedMsgCount]` and compare to the slot's
     stored hash
   - On mismatch: check if the system prompt hash (Tier 1) still matches.
     Track the slot as a system-prompt-match candidate if it does.
   - Track mismatched slots as eviction candidates

2. **KV pressure eviction** — When a matching slot is found and the total
   KV usage across all slots exceeds the context window, evict mismatched
   slots (largest first) to reclaim space. See
   [KV Pressure Eviction](#kv-pressure-eviction) for details.

3. **On full match** — Pick the slot with the best prefix coverage (most
   cached messages). If the request has new messages to cache, extend the
   slot's cache. If the messages are identical, it's a pure cache hit.

4. **System prompt preservation (two-tier hash)** — No full match, but a
   slot has the same system prompt cached. Keep the system prompt KV in
   place, trim everything after the system prompt token boundary, and
   re-template and re-decode only the conversation body. Before preserving,
   IMC verifies the system prompt token boundary is consistent after
   re-templating — if the template produces a different token count for the
   system prompt, it falls back to a full rebuild. Media slots are excluded
   from this path because token-level trim is unsafe for image/audio
   embeddings.

5. **Token prefix fallback** — Tokenize the incoming messages and compare
   the resulting token sequence element-by-element against each non-empty
   slot's stored `cachedTokens`. Pick the slot with the longest common
   prefix that meets `cache_min_tokens`. Trim the KV cache from the
   divergence point and decode only the new tokens from there forward. See
   [Token Prefix Fallback](#token-prefix-fallback) for details.

6. **No match at all** — Pick an empty slot if one exists, otherwise evict
   the least-recently-used (LRU) slot and rebuild from scratch.

**Concurrent Build Protection:**

When two requests arrive simultaneously and both need to build a cache from
scratch, a race condition could cause both to pick the same empty slot. IMC
prevents this with a pending flag: when a slot begins a deferred cache build,
it is marked pending. Concurrent scanners skip pending slots, so the second
request picks a different slot. The pending flag is cleared after the cache
decode completes (or on error).

**Decode Failure Recovery:**

If a cache decode fails at any point (extend, rebuild, trim, or media build),
IMC clears the entire KV sequence and resets the session metadata. This
ensures the slot never advertises cached content that doesn't exist in the
KV cache.

#### KV Pressure Eviction

With `n_seq_max > 1`, Kronk enables a unified KV cache (`KVUnified=1`) so that
all sequences share the full `n_ctx` pool. Any single sequence can grow up to the
full context window, but the **total** KV usage across all sequences cannot exceed
`n_ctx`.

This matters when an agent framework (like Kilo or Cline) sends multiple
concurrent requests for the same conversation. Each request may land on a
different slot. As the conversation grows, the active slot accumulates a large
cache while older slots hold stale snapshots of earlier conversation states.
Those stale slots consume KV cells that the active slot needs.

**Example:** With `n_seq_max: 3` and `context_window: 131072`:

```
Slot 0: 854 tokens    (stale — 2 cached messages, hash mismatch)
Slot 1: 46,541 tokens (stale — 17 cached messages, hash mismatch)
Slot 2: 86,682 tokens (active — 49 cached messages, hash match)
Total:  134,077 tokens > 131,072 → context window full!
```

Without KV pressure eviction, the next decode would fail with "context window
is full" even though the active conversation only uses ~87k of the 131k window.

**How It Works:**

After the slot scan finds a matching slot (Step 1), IMC checks whether the
projected total KV usage across all slots exceeds the context window. If it
does, mismatched slots are evicted largest-first until the total fits:

1. Sum `totalTokensCached` across all non-empty, non-pending slots
2. If the sum exceeds `context_window`, sort mismatched slots by token count
   (descending)
3. Evict slots one at a time — clear the KV sequence (`MemorySeqRm`) and
   reset the session metadata — until the projected total is within bounds

In the example above, evicting Slot 1 (46,541 tokens) brings the total to
87,536 — well within the 131,072 limit. Slot 0 (854 tokens) may or may not
need eviction depending on the remaining headroom.

**Key Points:**

- Eviction only targets **mismatched** slots — the active slot and any other
  matching slots are never evicted
- Pending slots (with a build in-flight) are never evicted
- Evicted slots become empty and are available for future cache builds
- The eviction check runs before the extend/hit path, so the active slot
  always has room to grow
- No configuration needed — eviction triggers automatically when KV pressure
  is detected

#### Token Prefix Fallback

When hash matching fails — whether because the client edited messages, a
template produced slightly different tokens, or the agent didn't send exactly
the same conversation — IMC falls back to token-level prefix matching to
salvage as much of the cached KV state as possible.

**When it activates:** Automatically when no hash match and no system prompt
match is found during the slot scan (Step 5 of the
[Slot Selection Algorithm](#slot-selection-algorithm)). IMC compares the
actual cached token arrays against the incoming request's tokens. Only
candidates with compatible message counts are considered — the request must
have at least as many messages as the slot cached.

**How it works:**

IMC tokenizes the incoming messages and compares them element-by-element
against each non-empty slot's stored token sequence to find the longest
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

If the common prefix meets the `cache_min_tokens` threshold, IMC:

1. Reserves the matching slot (marks it pending)
2. Trims the divergent suffix from the KV cache
3. Decodes only the new tokens from the divergence point forward
4. Updates the slot's hash and cached token sequence

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
| `no usable token prefix match`                      | All prefixes below `cache_min_tokens`, falling back to empty/LRU slot |

#### Model Type Interactions

The IMC matching algorithm is the same for all model types (Dense, MoE,
Hybrid). Only the batch engine's state management differs. See
[Section 4.9](#49-model-types-and-state-management) for how each model type
manages state between requests.

| Model Type | State Management     | Configuration Notes               |
| ---------- | -------------------- | --------------------------------- |
| Dense      | Partial range delete | No special requirements           |
| MoE        | Partial range delete | f16 cache, split_mode: row        |
| Hybrid     | Snapshot/Restore     | f16 cache required, no flash attn |

**MoE Configuration:**

```yaml
models:
  Qwen3-Coder-30B-A3B-Q8_0:
    incremental_cache: true
    split_mode: row # Best for MoE architecture
    cache_type_k: f16 # Safer for MoE routing accuracy
    cache_type_v: f16
```

**Hybrid Configuration:**

```yaml
models:
  Qwen3-Coder-Next-UD-Q4_K_XL:
    incremental_cache: true
    cache_type_k: f16 # Required for hybrid models
    cache_type_v: f16 # Required for hybrid models
```

### 5.3 Single-User Caching

IMC is designed for single-user use. All `NSeqMax` slots are available, with
each slot independently tracking its own conversation branch via hash matching.
This design is optimized for agentic workflows where multiple sub-agents send
independent conversations (different system prompts, different message
histories).

### 5.4 When to Use IMC

IMC caches the entire conversation history and uses hash matching with
automatic token prefix fallback when changes are detected. It is best suited
for:

- **Agentic workflows** — hash matching handles the common case, and token
  prefix fallback automatically salvages 70-80% of cached tokens when changes
  are detected
- **AI coding agents** — long-running conversations with growing context
- **Sub-agent architectures** — each sub-agent gets its own slot via hash
  matching, maintaining independent caches

| Feature      | Behavior                                  |
| ------------ | ----------------------------------------- |
| Caches       | All messages except last                  |
| Extends      | Yes, incrementally                        |
| Slots        | All slots available, single-user          |
| Sub-agents   | Each gets own slot via hash matching      |
| Best for     | Agentic workflows                         |
| Memory       | Zero extra VRAM overhead                  |

### 5.5 Cache Invalidation

Cached state doesn't last forever. Kronk uses hash comparisons to detect
when cached tokens no longer match the incoming request, and automatically
rebuilds the cache when a mismatch is found. Understanding what triggers
invalidation helps you avoid unexpected prefill costs.

**IMC Invalidation:**

- Message prefix hash mismatch with same system prompt → system prompt KV
  preserved, conversation body trimmed and re-decoded (Step 4 of the slot
  selection algorithm)
- Message prefix hash mismatch with no system prompt match → token prefix
  fallback attempted (see [Token Prefix Fallback](#token-prefix-fallback)).
  If a common prefix ≥ `cache_min_tokens` is found, only the divergent suffix
  is trimmed and rebuilt. Otherwise, cache is rebuilt from scratch.
- System prompt changed → full cache rebuild from scratch
- Conversation shrinks (client dropped messages or reasoning blocks) → system
  prompt preserved if unchanged, conversation body re-decoded

**Automatic Invalidation:**

Caches are cleared when:

- Model is unloaded
- Server restarts

### 5.6 Configuration Reference

IMC is enabled through the model configuration.

```yaml
models:
  Qwen3-8B-Q8_0:
    incremental_cache: true
    cache_min_tokens: 100 # Don't cache if < 100 tokens
```

**cache_min_tokens**

Minimum common prefix length required for token-level partial prefix
matching. If no slot's cached tokens share at least this many tokens with
the incoming request, the fallback is skipped and the cache is rebuilt from
scratch.

Default: 100 tokens

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
With `n_seq_max > 1`, Kronk enables a unified KV cache where all sequences
share the full `n_ctx` pool. The total KV cache size is determined by
`context_window`, not multiplied by the number of slots:

```
131K context, n_seq_max=3, IMC (unified KV cache):
  Total KV cache: ~3.2 GB (8B model, F16)
  Any single slot can use up to the full 131K tokens
  Total across all slots cannot exceed 131K tokens
```

KV pressure eviction ensures that stale slots are cleared when the shared
pool gets tight, so the active conversation always has access to the full
context window.

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
image or audio is encoded through the projection model once and remains in the
KV cache across subsequent requests. Text-only follow-up messages extend the
cache without re-encoding the media.

For example, in a conversation like:

```
Request 1 (image request):
[system]       →  cached by IMC (text tokens)
[user + image] →  cached by IMC (text + image embeddings via mtmd pipeline)
[user]         →  prefill (generation target)

Request 2 (text follow-up about the image):
[system]       →  cached (KV cache hit)
[user + image] →  cached (image stays in KV cache, no re-encode)
[assistant]    →  extended (new text tokens decoded into cache)
[user]         →  prefill (generation target)

Request 3 (unrelated text question):
[system]       →  cached (KV cache hit)
[user + image] →  cached (image stays in KV cache)
[assistant]    →  cached (KV cache hit)
[user]         →  extended (new text tokens decoded into cache)
[assistant]    →  extended
[user]         →  prefill (generation target)

Request 4 (back to asking about the image):
[system]       →  cached (KV cache hit)
[user + image] →  cached (image STILL in KV cache, no re-encode)
[assistant]    →  cached (KV cache hit)
[user]         →  cached (KV cache hit)
[assistant]    →  cached (KV cache hit)
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
[all prior]    →  cached (KV cache hit, image stays in KV cache)
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

4. The slot tracks `mediaKVCounts` — the number of KV positions consumed by
   each media chunk. This is needed because media embeddings occupy a different
   number of KV positions than the text marker tokens they replace in the
   tokenized prompt.

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
- Max concurrent conversation branches = NSeqMax; when all slots are
  occupied, the least-recently-used slot is evicted
- System prompt preservation is text-only; media slots always use hash
  matching or full rebuild

---
