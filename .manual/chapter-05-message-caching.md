# Chapter 5: Message Caching

## Table of Contents

- [5.1 Overview](#51-overview)
- [5.2 System Prompt Cache (SPC)](#52-system-prompt-cache-spc)
- [5.3 Incremental Message Cache (IMC)](#53-incremental-message-cache-imc)
  - [IMC Deterministic](#imc-deterministic)
  - [IMC Non-Deterministic](#imc-non-deterministic)
  - [Model Type Interactions](#model-type-interactions)
- [5.4 Single-User Caching](#54-single-user-caching)
- [5.5 SPC vs IMC](#55-spc-vs-imc)
- [5.6 Cache Invalidation](#56-cache-invalidation)
- [5.7 Configuration Reference](#57-configuration-reference)
- [5.8 Performance and Limitations](#58-performance-and-limitations)

---

Message caching reduces redundant computation by storing and reusing KV cache
state from previous requests. Kronk provides two caching modes (SPC and IMC)
optimized for different use cases.

### 5.1 Overview

When processing a chat request, the model must compute attention for
every token in the conversation. Without caching, the entire prompt is
prefilled on every request — even tokens the model has already seen.

_Note: Prefill is the phase where the model processes all input tokens
(system prompt, conversation history, and the new message) before it
begins generating a response. This is the most computationally
expensive part of a request, and its cost grows with the number of
input tokens._

Kronk provides two caching modes that reduce redundant prefill work:

- SPC (System Prompt Cache) decodes the system prompt once, externalizes the KV state to a byte buffer in RAM, and restores it into each slot via StateSeqSetData per request.

- IMC (Incremental Message Cache) dedicates each slot to a user and caches the full conversation in the slot's KV cache sequence, so only the new message needs to be prefilled.

```
No Caching:
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (prefill)   │ (prefill) │ (prefill) │  (prefill)  │
└─────────────────────────────────────────────────────┘
                                              ↓
                                           Generate

SPC (System Prompt Cache):
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (cached)    │ (prefill) │ (prefill) │  (prefill)  │
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

### 5.2 System Prompt Cache (SPC)

System Prompt Cache decodes the system prompt once into a temporary sequence,
externalizes the KV state to a byte buffer in RAM, and frees the sequence. On
each request, the KV state is restored into the slot's working sequence via
StateSeqSetData. This avoids re-decoding the system prompt on every request.
No dedicated cache sequence is permanently occupied, so SPC does not add any
extra sequences to the VRAM allocation.

**Best for:**

- OpenWebUI and similar chat interfaces
- Applications with a consistent system prompt
- Multi-user scenarios with different system prompts

**Enable SPC:**

```yaml
models:
  Qwen3-8B-Q8_0:
    system_prompt_cache: true
```

**How It Works:**

1. First request: System prompt is templated, tokenized, and decoded into a
   temporary sequence
2. The KV state is extracted into a byte buffer in RAM and the sequence is freed
3. The KV state is restored into the slot's working sequence via StateSeqSetData
4. Remaining messages are prefilled after the cached system prompt tokens
5. Subsequent requests: KV state is restored from the RAM buffer (no
   re-decoding needed)

**Cache Invalidation:**

The cache is automatically invalidated when:

- The system prompt content changes (detected by hash comparison)
- The system prompt role changes
- The server restarts

### 5.3 Incremental Message Cache (IMC)

Incremental Message Cache is designed for agentic workflows where
conversations grow monotonically. It caches all messages except the last
one and extends the cache incrementally on each turn.

IMC has two matching strategies. You don't choose a strategy — Kronk always
tries hash matching first and automatically falls back to token prefix
matching when the hash doesn't match.

| Strategy          | When It Runs                         | How It Matches        |
| ----------------- | ------------------------------------ | --------------------- |
| Deterministic     | Hash of cached messages matches      | Hash-based            |
| Non-Deterministic | Hash fails, falls back automatically | Token prefix fallback |

The matching strategy is independent of the model type (Dense, MoE, Hybrid).
Any model type can use either strategy — it depends on the template, not the
architecture. What changes per model type is how the batch engine manages
state between requests — see [Section 4.9](#49-model-types-and-state-management).

The table below shows the real models and which strategy their templates
produce. Note that MoE appears in both columns — Qwen3-VL (MoE) has a
deterministic template, while GPT-OSS (also MoE) has a non-deterministic one.

| Model                          | Architecture | Template          | Modality |
| ------------------------------ | ------------ | ----------------- | -------- |
| Qwen3-8B-Q8_0                  | Dense        | Deterministic     | Text     |
| Qwen2.5-VL-3B-Instruct-Q8_0    | Dense        | Deterministic     | Vision   |
| Qwen3-VL-30B-A3B-Instruct-Q8_0 | MoE          | Deterministic     | Vision   |
| Qwen3.5-35B-A3B-Q8_0           | Hybrid       | Deterministic     | Vision   |
| gpt-oss-20b-Q8_0               | MoE          | Non-Deterministic | Text     |

- **Deterministic** is the fastest path — hash matching finds the right slot
  instantly with no tokenization overhead.
- **Non-Deterministic** exists because some model templates produce different
  token sequences for the same messages. Hash matching fails, so IMC falls
  back to token-level comparison to salvage as much of the cache as possible.

**Best for:**

- AI coding agents
- Long-running agent conversations
- Any workflow where messages are appended, not edited
- Sub-agent architectures with multiple concurrent agents

**Enable IMC:**

```yaml
models:
  Qwen3-8B-Q8_0:
    incremental_cache: true
    cache_min_tokens: 100 # Minimum tokens before caching
```

**Multi-Slot Architecture:**

All `NSeqMax` slots are available for IMC. Each slot independently tracks its
own conversation branch — its own message hash, token count, and message
index. Sub-agents are routed to different slots via hash matching, allowing
them to maintain independent caches and run concurrently.

With `n_seq_max: 3`, three sub-agents can each have their own cached
conversation branch. Without multi-slot IMC, every sub-agent request would
cause a prefix mismatch and rebuild the cache from scratch because different
sub-agents send different system prompts and conversation content.

**Important:** Set `n_seq_max` to at least the number of concurrent
sub-agents your agent framework spawns. If `n_seq_max` is smaller than
the number of sub-agents, cache thrashing occurs — each new sub-agent
evicts a slot, and when the evicted sub-agent returns, it evicts another.
Every request triggers a full rebuild from scratch, eliminating the
caching benefit entirely. The trade-off is VRAM — each additional slot
reserves its full KV cache partition at model load time.

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

**Slot Selection Algorithm:**

When a request arrives, IMC scans all slots to find the best match. Steps 1-2
apply to both strategies. Step 3 is the Non-Deterministic fallback path.
Step 4 is the universal last resort.

1. **Scan all slots** — For each slot:
   - Skip slots with a build in-flight (pending flag set)
   - Skip empty slots (track the first empty slot as a fallback)
   - Skip slots with more cached messages than the request has total
   - Hash `messages[:slot.lastMsgIdxCached]` and compare to the slot's
     stored hash

2. **On match** — Pick the slot with the best prefix coverage (most cached
   messages). If the request has new messages to cache, extend the slot's
   cache. If the messages are identical, it's a pure cache hit.

3. **No hash match — token prefix fallback (Non-Deterministic mode)** —
   Tokenize the incoming messages and compare the resulting token sequence
   element-by-element against each non-empty slot's stored `cachedTokens`.
   Pick the slot with the longest common prefix that meets `cache_min_tokens`.
   Trim the KV cache from the divergence point and decode only the new tokens
   from there forward. See [IMC Non-Deterministic](#imc-non-deterministic)
   for details.

4. **No match at all** — Pick an empty slot if one exists, otherwise evict
   the least-recently-used (LRU) slot and rebuild from scratch.

**Concurrent Build Protection:**

When two requests arrive simultaneously and both need to build a cache from
scratch, a race condition could cause both to pick the same empty slot. IMC
prevents this with a pending flag: when a slot begins a deferred cache build,
it is marked pending. Concurrent scanners skip pending slots, so the second
request picks a different slot. The pending flag is cleared after the cache
decode completes (or on error).

#### IMC Deterministic

The default and fastest matching strategy. Used automatically for models with
consistent templates — where the same messages always produce identical token
sequences regardless of conversation length.

**Why this strategy exists:** Most models have deterministic templates (see
the model table above). When the template is consistent, a simple hash of
the message prefix is enough to identify a matching slot. This avoids
tokenization overhead entirely.

**When it's used:** Automatically when `incremental_cache: true` and the
template produces consistent token sequences. This is the default path.

**How it works:**

1. Hash the incoming message prefix
2. Compare against each slot's stored hash
3. On match: extend the cache with new messages, or return a pure cache hit

**Models:** QWEN, Llama, Qwen3-Coder (MoE), Qwen3-Coder-Next (Hybrid),
and most model architectures.

#### IMC Non-Deterministic

A fallback mode that activates automatically when hash matching fails due
to template non-determinism. Some model templates produce different token
sequences for identical messages across requests — even though the semantic
content is the same.

**Why this mode exists:** GPT-OSS, for example, injects tool call formatting
that varies between template invocations. This causes message hash mismatches
even though the conversation hasn't changed, which would normally force a full
cache rebuild from scratch. The Non-Deterministic mode salvages 70-80% of the
cached tokens instead.

**When it's used:** Automatically when no hash match is found during the slot
scan. IMC falls back to comparing the actual cached token arrays against the
incoming request's tokens. Only candidates with compatible message counts are
considered — the request must have at least as many messages as the slot
cached.

**How it works — Token-Level Partial Prefix Matching:**

When no hash match is found, IMC tokenizes the incoming messages and compares
them element-by-element against each non-empty slot's stored token sequence to
find the longest common prefix.

```
Cached tokens:   [T1, T2, T3, T4, T5, T6, T7, T8]
Incoming tokens: [T1, T2, T3, T4, T5, T9, T10, T11, T12]
                                       ↑
                              Divergence point (pos 5)

Common prefix: 5 tokens (salvaged from KV cache)
Trimmed:       3 tokens (T6-T8 removed via MemorySeqRm)
New decode:    4 tokens (T9-T12, from divergence point forward)
```

If the common prefix meets the `cache_min_tokens` threshold, IMC:

1. Reserves the matching slot (marks it pending)
2. Trims the divergent suffix from the KV cache (`MemorySeqRm(seq, trimPos, -1)`)
3. Decodes only the new tokens from the divergence point forward
4. Updates the slot's hash and cached token sequence

Once the partial rebuild completes, subsequent requests in the same
conversation use normal hash-based extending. The token prefix path is only
triggered at conversation boundaries — when the template non-determinism
causes the initial mismatch.

Real-world testing with GPT-OSS showed 77-80% cache salvage rates when
switching conversations. Instead of decoding ~8400 tokens from scratch,
the system kept ~6800 cached and only decoded ~1600.

**Models:** GPT-OSS, GLM, and any model whose template produces variable
token sequences for identical messages (see the model table above).

**Debugging Non-Deterministic IMC:**

| Log Message                                         | Meaning                                                               |
| --------------------------------------------------- | --------------------------------------------------------------------- |
| `no slot matched, trying token prefix match`        | Hash match failed, entering token comparison                          |
| `slot[N] common-prefix X/Y tokens (Z% salvageable)` | Per-slot comparison result                                            |
| `token prefix match found`                          | Usable prefix found, will trim and extend                             |
| `imc-trim-prefix`                                   | KV cache trim in progress (shows cached_tokens, trim_pos)             |
| `imc-partial-rebuilt`                               | Rebuild complete (shows total_cached, salvaged_prefix, salvaged_pct)  |
| `no usable token prefix match`                      | All prefixes below `cache_min_tokens`, falling back to empty/LRU slot |

#### Model Type Interactions

The IMC matching strategy (Deterministic or Non-Deterministic) is independent
of the model type (Dense, MoE, Hybrid). The caching system works the same way
for all model types — only the batch engine's state management differs. See
[Section 4.9](#49-model-types-and-state-management) for how each model type
manages state between requests.

| Model Type | Matching Strategy        | State Management     | Configuration Notes               |
| ---------- | ------------------------ | -------------------- | --------------------------------- |
| Dense      | Deterministic or Non-Det | Partial range delete | No special requirements           |
| MoE        | Deterministic or Non-Det | Partial range delete | f16 cache, split_mode: row        |
| Hybrid     | Deterministic or Non-Det | Snapshot/Restore     | f16 cache required, no flash attn |

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

### 5.4 Single-User Caching

IMC is designed for single-user use. All `NSeqMax` slots are available, with
each slot independently tracking its own conversation branch via hash matching.
This design is optimized for agentic workflows where multiple sub-agents send
independent conversations (different system prompts, different message
histories).

**SPC:** All requests share the same externalized KV state buffer. The cached
KV state is restored into each slot via StateSeqSetData. If the system prompt
changes, the cache is rebuilt automatically.

### 5.5 SPC vs IMC

Both caching modes eliminate redundant work, but they target different parts
of the prompt and suit different workloads. SPC is the simpler option — it
caches just the system prompt and works with any model template. IMC is more
aggressive — it caches the entire conversation history and works with all
templates (deterministic templates use fast hash matching, non-deterministic
templates fall back to token prefix matching). The table below summarizes the
trade-offs to help you choose.

| Feature      | System Prompt Cache               | Incremental Message Cache                 |
| ------------ | --------------------------------- | ----------------------------------------- |
| Caches       | System prompt only                | All messages except last                  |
| Extends      | No                                | Yes, incrementally                        |
| Multi-user   | Single shared cache sequence      | Single-user, all slots available          |
| Sub-agents   | All share same SPC sequence       | Each gets own slot via hash matching      |
| Best for     | Chat UIs                          | Agentic workflows                         |
| Memory       | Zero extra VRAM (KV state in RAM) | Zero extra VRAM overhead                  |
| Template req | Any                               | Any (hash match or token prefix fallback) |

**Important:** SPC and IMC are mutually exclusive. Choose based on your
workload:

- **Agentic workflows:** Use IMC — works with all templates. Most models get
  the fastest hash-based path. Non-deterministic templates (GPT-OSS, GLM) use
  token prefix fallback with 70-80% cache salvage.
- **Chat UIs / multi-client:** Use SPC — simpler model, no slot dedication

### 5.6 Cache Invalidation

Cached state doesn't last forever. Kronk uses hash comparisons to detect
when cached tokens no longer match the incoming request, and automatically
rebuilds the cache when a mismatch is found. Understanding what triggers
invalidation helps you avoid unexpected prefill costs.

**SPC Invalidation:**

- System prompt content changes → cache rebuilt
- System prompt hash mismatch → cache rebuilt

**IMC Invalidation:**

- Message prefix hash mismatch → token prefix fallback attempted first. If a
  common prefix ≥ `cache_min_tokens` is found, only the divergent suffix is
  trimmed and rebuilt. Otherwise, cache is rebuilt from scratch.
- User starts new conversation → token prefix fallback salvages shared prefix
  (e.g., system prompt tokens), then extends from there
- Earlier message edited → cache rebuilt (hash and token prefix both fail)

**Automatic Invalidation:**

Caches are cleared when:

- Model is unloaded
- Server restarts

### 5.7 Configuration Reference

Both caching modes are enabled through the model configuration. Remember that
SPC and IMC are mutually exclusive — enable one or the other, not both.

```yaml
models:
  Qwen3-8B-Q8_0:
    # System Prompt Cache
    system_prompt_cache: true

    # OR Incremental Message Cache (mutually exclusive)
    incremental_cache: true

    # Shared settings
    cache_min_tokens: 100 # Don't cache if < 100 tokens
```

**cache_min_tokens**

Minimum token count threshold. For SPC, caching doesn't activate if the
system prompt is shorter than this. For IMC, this is the minimum common
prefix length required for token-level partial prefix matching — if no
slot's cached tokens share at least this many tokens with the incoming
request, the fallback is skipped and the cache is rebuilt from scratch.

Default: 100 tokens

### 5.8 Performance and Limitations

Caching improves request latency by skipping redundant prefill work, but
each mode has its own costs and constraints. SPC trades a small per-request
decode cost for broad compatibility. IMC delivers larger savings but imposes
restrictions on template behavior and session management.

**SPC Performance:**

SPC restores the externalized KV state into each slot via StateSeqSetData.
This is a memory copy from RAM into the KV cache, typically taking 10-30ms
depending on system prompt size and memory bus load. No extra VRAM is consumed
since the KV state lives in regular RAM.

**IMC Prefill Savings:**

For a 2000-token cached conversation prefix:

- Without cache: ~200ms prefill (varies by hardware)
- With IMC: ~5ms for new tokens only

Cache extensions (adding new messages to an existing cached prefix) are
especially fast because only the delta tokens are decoded. In production
logs, sequential extensions typically take ~3ms each.

**IMC Memory Overhead:**

IMC adds no extra VRAM. llama.cpp partitions the KV cache across
sequences, so each slot gets `context_window / NSeqMax` tokens:

```
8K context, n_seq_max=4, IMC:
  KV cache per slot: ~200 MB (8B model, F16)
  Total KV cache: 4 × 200 MB = ~800 MB
```

**IMC Token Prefix Fallback Performance:**

When IMC falls back to token-level prefix matching (non-deterministic
templates), there is a one-time cost to tokenize the incoming messages for
comparison. This is typically fast (< 5ms for most conversations). The
savings from salvaging 70-80% of the cached tokens far outweigh this cost
compared to a full rebuild.

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

- Conversations must grow monotonically (append-only)
- Editing earlier messages triggers full cache rebuild
- Designed for single-user use
- Max concurrent conversation branches = NSeqMax; when all slots are
  occupied, the least-recently-used slot is evicted

---
