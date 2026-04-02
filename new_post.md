# Understanding Kronk's Caching Systems: SPC and IMC

## Introduction

If you're running a local LLM, every request pays a hidden tax called **prefill**. Prefill is the phase where the model processes all the input tokens — your system prompt, the entire conversation history, and the new message — before it can start generating a response. It's the most computationally expensive part of any request, and its cost grows linearly with the number of input tokens.

Without caching, the model re-processes the entire conversation from scratch on every single request. Imagine pasting the same 2,000-word system prompt into ChatGPT over and over, and the model reading it fresh each time. That's what's happening under the hood.

Kronk solves this with two caching systems: **System Prompt Cache (SPC)** and **Incremental Message Cache (IMC)**. Both eliminate redundant prefill work, but they target different parts of the prompt and are designed for different workloads.

## What Gets Cached and Why It Matters

Here's the simplest way to visualize the difference. Consider a conversation with a system prompt, two prior messages, and a new message:

```
No Caching:
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (prefill)   │ (prefill) │ (prefill) │  (prefill)  │
└─────────────────────────────────────────────────────┘
Everything is recomputed. Every request.

SPC (System Prompt Cache):
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (cached)    │ (prefill) │ (prefill) │  (prefill)  │
└─────────────────────────────────────────────────────┘
The system prompt is computed once and reused.

IMC (Incremental Message Cache):
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (cached)    │ (cached)  │ (cached)  │  (prefill)  │
└─────────────────────────────────────────────────────┘
Everything except the latest message is cached.
```

SPC saves you from re-processing the system prompt. IMC saves you from re-processing *everything* — the system prompt and the entire conversation history up to the latest message.

## System Prompt Cache (SPC)

SPC is the simpler of the two modes. It targets the system prompt — the instructions you give the model at the start of every conversation.

### How It Works

1. On the first request, Kronk takes the system prompt, tokenizes it, and decodes it into a temporary sequence inside the model.
2. The resulting KV state (the attention computations for those tokens) is extracted into a byte buffer in regular RAM. The temporary sequence is freed.
3. On that request and every future request, the KV state is restored from the RAM buffer into the slot's working sequence. No re-decoding needed.
4. The remaining messages (conversation history + new message) are then prefilled on top of the cached system prompt state.

The restoration step — copying the KV state from RAM into the slot — typically takes 10-30ms depending on system prompt size. That's nothing compared to re-decoding hundreds or thousands of system prompt tokens.

### When To Use SPC

SPC works best when you have a consistent system prompt and multiple users or sessions sharing it:

- **Chat UIs** like OpenWebUI where many users send different conversations but share the same system prompt
- **Multi-user environments** where the system prompt stays the same across requests
- **Any application** where the system prompt is significantly larger than individual messages

### Configuration

```yaml
models:
  Qwen3-8B-Q8_0:
    system_prompt_cache: true
```

That's it. SPC activates automatically and invalidates itself when the system prompt content changes (detected by hash comparison) or the server restarts.

### Memory Impact

SPC adds **zero extra VRAM**. The cached KV state lives in regular RAM, not GPU memory. No dedicated cache sequence is permanently occupied, so SPC doesn't affect your slot count or KV cache partitioning.

## Incremental Message Cache (IMC)

IMC is the more aggressive caching mode. Instead of caching just the system prompt, it caches the **entire conversation** except for the last message. As the conversation grows, the cache extends incrementally — only the newest message needs to be prefilled.

### How It Works

IMC caches all messages except the last one, and extends the cache on each turn:

**First request** (2 messages: system + user):
```
Messages: [system, user]
Cache:    [system]           ← Cache all except last
Prefill:  [user + gen_prompt]
```

**Second request** (4 messages):
```
Messages: [system, user, assistant, user2]
Cache:    [system, user, assistant]  ← Extend cache
Prefill:  [user2 + gen_prompt]
```

**Third request** (6 messages):
```
Messages: [system, user, assistant, user2, assistant2, user3]
Cache:    [system, user, assistant, user2, assistant2]  ← Extend
Prefill:  [user3 + gen_prompt]
```

Notice the pattern: on each turn, the cache grows to include everything the model has already seen. Only the brand new message (plus the generation prompt) needs to be prefilled. For a 2,000-token cached conversation, this means going from ~200ms of prefill down to ~5ms.

### The Two Matching Strategies

When a request arrives, IMC needs to find the right slot — the one that already has this conversation's history cached. It does this through two strategies, tried in order:

**1. Deterministic (Hash Matching)** — This is the default, fast path. IMC hashes the incoming message prefix and compares it against each slot's stored hash. If there's a match, it knows exactly which slot to use and extends the cache with any new messages. No tokenization overhead. Most models use this path — Qwen, Llama, and most others have templates that produce identical token sequences for the same messages every time.

**2. Non-Deterministic (Token Prefix Fallback)** — Some models, like GPT-OSS, have templates that produce slightly different token sequences for identical messages across requests. The hash won't match even though the conversation hasn't changed. When this happens, IMC falls back to comparing the actual token arrays element-by-element to find the longest common prefix:

```
Cached tokens:   [T1, T2, T3, T4, T5, T6, T7, T8]
Incoming tokens: [T1, T2, T3, T4, T5, T9, T10, T11, T12]
                                       ↑
                              Divergence point (pos 5)

Common prefix: 5 tokens (salvaged from KV cache)
Trimmed:       3 tokens (T6-T8 removed from KV cache)
New decode:    4 tokens (T9-T12, from divergence point forward)
```

Instead of rebuilding 8,400 tokens from scratch, IMC keeps ~6,800 cached and only decodes ~1,600. That's a 77-80% cache salvage rate.

You don't choose a strategy — Kronk always tries hash matching first and automatically falls back to token prefix matching when the hash doesn't match.

### Multi-Slot Architecture

IMC uses all available slots (`NSeqMax`). Each slot independently tracks its own conversation — its own message hash, token count, and message index. This is critical for agentic workflows where multiple sub-agents operate concurrently.

With `n_seq_max: 3`, three sub-agents can each maintain their own cached conversation branch. Each sub-agent is routed to its own slot via hash matching, so their caches don't interfere with each other.

**Tip:** Set `n_seq_max` to at least the number of concurrent sub-agents your framework spawns. If you have fewer slots than sub-agents, you'll get cache thrashing — each new sub-agent evicts a slot, and when the evicted sub-agent returns, it evicts another. Every request triggers a full rebuild, and the caching benefit disappears entirely.

### When To Use IMC

IMC is built for workloads where conversations grow over time:

- **AI coding agents** like Cline that build up long conversation histories
- **Long-running agent conversations** with many back-and-forth turns
- **Sub-agent architectures** where multiple agents maintain independent conversations
- **Any workflow** where messages are appended, not edited

### Configuration

```yaml
models:
  Qwen3-8B-Q8_0:
    incremental_cache: true
    cache_min_tokens: 100  # Minimum tokens before caching activates
```

### Memory Impact

Like SPC, IMC adds **zero extra VRAM overhead**. The model's KV cache is already partitioned across sequences by llama.cpp. IMC just uses those partitions more efficiently by keeping cached state alive between requests rather than throwing it away.

### Vision and Audio Support

IMC fully supports multi-modal models. When a message containing an image or audio appears in the conversation, IMC caches the media embeddings in the KV cache alongside the text tokens. On subsequent requests, the image stays in the KV cache — no re-encoding through the projection model. Text-only follow-up messages simply extend the cache without touching the media.

## SPC vs IMC: Which One Should I Use?

SPC and IMC are **mutually exclusive** — you enable one or the other, not both. The choice comes down to your workload:

| Feature      | SPC                               | IMC                                       |
| ------------ | --------------------------------- | ----------------------------------------- |
| Caches       | System prompt only                | All messages except last                  |
| Extends      | No                                | Yes, incrementally                        |
| Multi-user   | Single shared cache               | Single-user, all slots available          |
| Sub-agents   | All share same SPC                | Each gets own slot via hash matching      |
| Best for     | Chat UIs                          | Agentic workflows                         |
| Memory       | Zero extra VRAM (KV state in RAM) | Zero extra VRAM overhead                  |
| Template req | Any                               | Any (hash match or token prefix fallback) |

**Use SPC** if you're running a chat UI or serving multiple users with different conversations but a shared system prompt. SPC is simpler, has no slot dedication, and works universally.

**Use IMC** if you're running agentic workflows — coding agents, long-running conversations, sub-agent architectures. IMC delivers dramatically larger prefill savings because it caches the entire conversation history, not just the system prompt.

## Cache Invalidation

Both systems handle invalidation automatically:

- **SPC** rebuilds when the system prompt content changes (detected by hash comparison) or the server restarts.
- **IMC** detects message prefix hash mismatches and tries token prefix fallback first. If a usable common prefix is found, only the divergent portion is rebuilt. If earlier messages are edited (breaking both hash and token matching), the cache rebuilds from scratch.

Both caches are cleared when the model is unloaded or the server restarts.

## Conclusion

Caching in Kronk eliminates the silent performance tax of redundant prefill. SPC gives you a quick win by caching the system prompt — simple to enable, works everywhere, and costs nothing in VRAM. IMC goes further by caching your entire conversation history, turning multi-second prefills into single-digit millisecond operations — exactly what agentic workflows need to stay responsive.

The best part: both modes are a single line of YAML to enable. Pick the one that matches your workload, and you're done.
