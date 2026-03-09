# Chapter 4: Batch Processing

## Table of Contents

- [4.1 Architecture Overview](#41-architecture-overview)
- [4.2 Slots and Sequences](#42-slots-and-sequences)
- [4.3 Request Flow](#43-request-flow)
- [4.4 Configuring Batch Processing](#44-configuring-batch-processing)
- [4.5 Concurrency by Model Type](#45-concurrency-by-model-type)
- [4.6 Performance Tuning](#46-performance-tuning)
- [4.7 Example Configuration](#47-example-configuration)
- [4.8 IMC Slot Scheduling](#48-imc-slot-scheduling)
- [4.9 Model Types and State Management](#49-model-types-and-state-management)

---

Batch processing allows Kronk to handle multiple concurrent requests
efficiently by sharing model resources. This chapter explains the architecture
and how to optimize for your workload.

### 4.1 Architecture Overview

For text inference models (including vision/audio), Kronk always creates a
batch engine with `NSeqMax` slots (defaulting to 1). `NSeqMax` controls how
many sequences are processed in parallel within a single model instance.

```
                    ┌───────────────────────────────────┐
    Request 1 ─────▶│                                   │
                    │          Request Queue            │   Incoming requests are buffered.
    Request 2 ─────▶│      (capacity: NSeqMax × 2)      │   R3 waits because all slots are
                    │                                   │   occupied (NSeqMax=2).
Request 3 (WAIT) ──▶│                                   │
                    └────────────────┬──────────────────┘
                                     │
                                     ▼
                    ┌───────────────────────────────────┐
                    │            Batch Engine           │
                    │                                   │
                    │  ┌───────────┐    ┌───────────┐   │
                    │  │  Slot 0   │    │  Slot 1   │   │   Each request is assigned to a slot.
                    │  │   (R1)    │    │   (R2)    │   │   The slot tracks prompt tokens,
                    │  │  seqID=0  │    │  seqID=1  │   │   decode position, and sampler state.
                    │  └─────┬─────┘    └─────┬─────┘   │
                    │        │                │         │
                    │        ▼                ▼         │
                    │  ┌───────────┐    ┌───────────┐   │
                    │  │ KV Cache  │    │ KV Cache  │   │   Each slot writes to its own KV cache
                    │  │   (R1)    │    │   (R2)    │   │   partition, isolated by sequence ID.
                    │  │   seq0    │    │   seq1    │   │   Requests never share attention state.
                    │  └─────┬─────┘    └─────┬─────┘   │
                    │        │                │         │
                    │        └───────┬────────┘         │
                    │                ▼                  │
                    │        ┌────────────────┐         │   Tokens from all active slots are
                    │        │  Decode Loop   │         │   collected into a single batch using
                    │        │(parallel batch)│         │   round-robin n_ubatch-sized chunks
                    │        └───────┬────────┘         │   and decoded together each iteration.
                    └────────────────┼──────────────────┘
                                     │
                                     ▼
                    ┌───────────────────────────────────┐   llama.cpp processes the full batch
                    │         llama.cpp Backend         │   on the GPU, computing all sequences
                    │        (GPU/CPU Inference)        │   in parallel in one forward pass.
                    └───────────────────────────────────┘
```

### 4.2 Slots and Sequences

The batch engine divides its capacity into slots and sequences. Together they
provide the mechanism for processing multiple requests concurrently while
keeping each request's data isolated inside the shared KV cache.

**Slots** are processing units that handle individual requests. Each slot
tracks its own state: prompt tokens, decode position, sampler, and response
channel.

**Sequences** are isolated partitions in the shared KV cache. Each slot is
assigned a unique sequence ID, ensuring requests don't interfere with each
other's attention state.

The slot/sequence layout is the same for all caching strategies in Kronk:

```
NSeqMax = 4

Slot 0  →  seqID = 0  →  KV cache partition 0
Slot 1  →  seqID = 1  →  KV cache partition 1
Slot 2  →  seqID = 2  →  KV cache partition 2
Slot 3  →  seqID = 3  →  KV cache partition 3
```

How a slot uses its sequence depends on the caching strategy. Without caching,
the sequence is cleared between requests. With SPC or IMC, the sequence
retains cached tokens to avoid redundant processing. See
[Section 3.5](#35-parallel-inference-nseqmax) for details on how each caching
strategy affects slot behavior.

### 4.3 Request Flow

Each request moves through the batch engine in the following stages:

1. **Queue**: Request enters the queue (backpressure if full)
2. **Assign**: Available slot picks up the request
3. **Cache Setup**: Prepare the slot's sequence based on caching strategy:
   - Clear the sequence (no caching)
   - Clear the sequence, then copy cached KV state from dedicated SPC sequence (SPC)
   - Extend or rebuild the conversation cache in place (IMC)
4. **Prefill**: Tokenize and process remaining prompt tokens (round-robin
   across slots in `n_ubatch`-sized chunks to prevent starvation)
5. **Decode**: Generate tokens one at a time, streaming to client
6. **Complete**: Release the slot:
   - Clear the entire sequence (no caching or SPC)
   - Dense/MoE with IMC: Trim generated tokens via partial range delete, keep cached conversation prefix
   - Hybrid with IMC: Full clear + restore snapshot from byte buffer, keep cached conversation prefix (see [Section 4.9](#49-model-types-and-state-management))

### 4.4 Configuring Batch Processing

Batch processing is controlled primarily through the model configuration. The
key setting is `NSeqMax`, which determines how many slots the batch engine
creates and therefore how many requests can be processed in parallel. Increasing
`NSeqMax` improves concurrency but requires proportionally more KV cache memory,
so it's important to balance throughput against available VRAM.

#### Enable Batch Processing

By default, the batch engine runs with a single slot (`NSeqMax=1`). To enable
parallel request processing, set `NSeqMax > 1` in your model config:

```yaml
models:
  Qwen3-8B-Q8_0:
    n_seq_max: 4 # 4 concurrent requests
```

#### Queue Depth

A bounded request queue sits in front of the batch engine to absorb bursts
of incoming requests without rejecting them immediately.

The request queue holds `NSeqMax × 2` requests by default. With `NSeqMax=4`,
up to 8 requests can be in-flight: 4 actively processing in slots and 4
waiting in the queue. This multiplier is configurable via `WithQueueDepth`
when using the SDK:

```go
krn, err := kronk.New(ctx, cfg, kronk.WithQueueDepth(3))
```

When all slots and queue positions are occupied, new requests block until a
slot becomes available or the request's context is cancelled. If a queued
request waits longer than `CacheSlotTimeout` (default: 30 seconds), the
engine preempts the longest-running slot — cancelling that in-flight request
with a "preempted by queued request" error — and assigns the slot to the
waiting request. If the engine is shutting down, queued requests receive an
immediate error. This backpressure and preemption mechanism prevents any
single request from starving others indefinitely.

#### Memory and Caching

Adding slots increases throughput but costs memory. Each additional slot
allocates its own KV cache partition proportional to the full context window.

Each slot reserves its own KV cache partition, so increasing `NSeqMax`
increases VRAM usage proportionally. Neither SPC nor IMC adds extra sequences.
For details on how slot memory is allocated and how to estimate total VRAM, see
[Section 3.5](#35-parallel-inference-nseqmax) and
[Section 3.7](#37-vram-estimation).

### 4.5 Concurrency by Model Type

Not all model types achieve concurrency the same way. Text inference models
(including vision and audio) use the batch engine described in the previous
sections, where multiple slots share a single model context and their tokens
are combined into one decode call. Embedding and reranking models take a
different approach — they create a pool of independent contexts that each
process requests separately. The table below summarizes the distinction, and
the diagrams that follow show the request flow for each approach.

| Model Type              | NSeqMax Behavior  | Concurrency Method                |
| ----------------------- | ----------------- | --------------------------------- |
| Text (chat, completion) | Batch parallelism | Shared model, multiple slots      |
| Vision/Audio            | Batch parallelism | Shared model, multiple slots      |
| Embedding               | Context pool      | Shared weights, multiple contexts |
| Reranking               | Context pool      | Shared weights, multiple contexts |

#### Embedding/Rerank Request Flow (NSeqMax=4)

Embedding and reranking models don't use the batch engine. Instead, Kronk
creates a pool of independent contexts — one per `NSeqMax` slot. When a
request arrives, it acquires a context from the pool, processes its inputs,
and releases the context back. If all contexts are in use, the request blocks
until one becomes available. The following diagram shows this flow:

```
                    ┌──────────────────────────────────┐
   Request 1 ──────▶│                                  │   Requests acquire a context from the
                    │           Context Pool           │   pool. If all contexts are in use,
   Request 2 ──────▶│       (capacity: NSeqMax)        │   the request blocks until one is
                    │                                  │   released.
Request 3 (WAIT) ──▶│                                  │
                    └────────────────┬─────────────────┘
                                     │
                                     ▼
                    ┌──────────────────────────────────┐
                    │     Independent Contexts         │
                    │                                  │
                    │  ┌───────────┐    ┌───────────┐  │   Each context has its own KV cache.
                    │  │ Context 0 │    │ Context 1 │  │   Unlike the batch engine, there is
                    │  │   (R1)    │    │   (R2)    │  │   no shared state between contexts.
                    │  └─────┬─────┘    └─────┬─────┘  │
                    │        │                │        │
                    │        ▼                ▼        │
                    │  ┌───────────┐    ┌───────────┐  │   Each request runs its own decode
                    │  │  Decode   │    │  Decode   │  │   call independently. Efficiency
                    │  │   (R1)    │    │   (R2)    │  │   comes from sharing model weights,
                    │  └─────┬─────┘    └─────┬─────┘  │   not from batching work together.
                    │        │                │        │
                    └────────┼────────────────┼────────┘
                             │                │
                             ▼                ▼
                    ┌──────────────────────────────────┐   llama.cpp processes each context
                    │         llama.cpp Backend        │   separately on the GPU. Model weights
                    │        (GPU/CPU Inference)       │   are shared, only KV cache is per-ctx.
                    └──────────────────────────────────┘
```

Unlike the batch engine, each request runs its own separate decode call —
there is no combining of work across requests. The efficiency comes from
sharing the model weights across all contexts, so only the KV cache memory
is duplicated.

### 4.6 Performance Tuning

The right `NSeqMax` value depends on your workload. More slots increase
throughput by serving more requests in parallel, but each additional slot
shares the same GPU, so individual requests may take slightly longer to
complete. The goal is to find the balance point where you have enough
concurrency for your users without saturating the GPU or running out of VRAM.

**Throughput vs Latency**

- Higher `NSeqMax`: Better throughput, potentially higher per-request latency
- Lower `NSeqMax`: Lower latency, less concurrent capacity

**Recommended Settings**

- Single user, interactive: `n_seq_max: 1-2`
- Multi-user API server: `n_seq_max: 4-8`
- High-throughput batch jobs: `n_seq_max: 8-16`

**Monitoring**

Use request tracing to watch for long `queue-wait` spans, which indicate
requests are waiting for an available slot. If you see consistently long
queue waits, consider:

1. Increasing `NSeqMax` (if VRAM allows)
2. Reducing `context_window` to fit more slots
3. Using KV cache quantization (`cache_type_k/v: q8_0`)

See [Chapter 14: Observability](#chapter-14-observability) for details on
tracing and metrics.

### 4.7 Example Configuration

The following config shows a high-throughput setup that balances concurrency,
memory, and caching for a multi-user API server:

```yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 8192
    n_seq_max: 8
    n_batch: 2048
    n_ubatch: 512
    cache_type_k: q8_0
    cache_type_v: q8_0
    system_prompt_cache: true
```

This configuration handles 8 concurrent requests, uses quantized KV cache to
reduce memory, and caches the system prompt for faster prefill. Here is the
VRAM estimate (see [Section 3.7](#37-vram-estimation) for the full formula):

```
Model                   : Qwen3-8B-Q8_0
Model Weights           : ~9 GB
Context Window (n_ctx)  : 8,192
Bytes Per Element       : 1 (q8_0)
block_count (n_layers)  : 36
attention.head_count_kv : 8
attention.key_length    : 128
attention.value_length  : 128

Step 1 — Per-token-per-layer cost:

  KV_Per_Token_Per_Layer = 8 × (128 + 128) × 1 = 2,048 bytes

Step 2 — Per-sequence cost:

  KV_Per_Sequence = 8,192 × 36 × 2,048 = ~0.6 GB

Step 3 — Total KV cache (NSeqMax = 8):

  Slot_Memory = 8 × 0.6 GB = ~4.8 GB

Step 4 — Total VRAM:

  Total_VRAM = 9.0 GB + 4.8 GB = ~13.8 GB
```

### 4.8 IMC Slot Scheduling

When IMC is enabled, the batch engine uses a specialized scheduling algorithm that
handles the constraint of routing requests to specific slots. This section explains
how IMC scheduling differs from normal slot assignment and the mechanisms that
prevent requests from stalling.

#### Normal Scheduling (No Caching / SPC)

Without IMC, the algorithm assigns the next queued request to any available
slot. If all slots are busy, the request stays in the queue until a slot
finishes. This is simple and works well because requests have no slot
affinity.

#### IMC Scheduling

IMC routes each request to a specific target slot based on cache matching
(see [Section 5.3](#53-incremental-message-cache-imc)). This creates a
problem: a request's target slot may be busy generating for another request,
even though other slots are free. The algorithm handles this with two
mechanisms: **deferred jobs** and **slot preemption**.

#### Deferred Jobs

When the algorithm dequeues a request but its target slot is busy, the
request is held aside as a deferred job instead of being put back on the
queue. On the next iteration of the processing loop, the algorithm checks
the deferred job first — if the target slot has finished, the job is
assigned immediately. This avoids a critical stall: putting a job back on
the queue could cause the processing loop to go idle (no active slots, empty
queue) and never wake up until a new external request arrives.

```
Request dequeued → target slot busy → defer (not requeue)
                                          │
Next iteration → target slot free? ──Yes──→ assign to slot
                                     │
                                     No → keep deferred, check again next iteration
```

#### Slot Preemption

If a deferred job waits longer than `CacheSlotTimeout` seconds (default: 30)
for its target slot to finish, the algorithm triggers preemption. This is a
safety mechanism for pathologically long generations — under normal operation,
the target slot finishes before the timeout and the deferred job picks it up
naturally.

Preemption uses a two-phase approach for safety:

1. **Schedule** — The algorithm marks the victim slot for preemption and
   defers the waiting job. No slot state is modified yet.

2. **Execute** — At the top of the next processing loop iteration, after
   the batch is cleared but before any tokens are added, the victim slot is
   finished with a preemption error. This ordering is critical — the victim
   slot must have no tokens in the current batch, otherwise cleaning up its
   KV state could corrupt a subsequent decode.

The preempted request receives an error response and the client can retry.
The waiting job is then assigned to the freed slot.

For IMC cache-hit requests, the specific target slot is preempted. For
requests with no cache hit (assigned to any slot), the longest-running
slot is preempted.

#### CacheSlotTimeout

The `cache_slot_timeout` setting (default: 30 seconds) controls two distinct
timeout scenarios in the IMC scheduling path:

| Scenario                | Phase              | What Happens at Timeout                      |
| ----------------------- | ------------------ | -------------------------------------------- |
| Wait for slot available | Before batch queue | Error returned: "server busy"                |
| Deferred job waiting    | Inside batch queue | Target slot preempted, deferred job assigned |

```
                          CacheSlotTimeout (30s)
                          ┌──────────────────────────────────────┐
                          │                                      │
    ┌─────────────────────┼──────────────────┐   ┌───────────────┼──────────────┐
    │  Before Batch Queue │                  │   │ Inside Batch  │              │
    │                     │                  │   │ Queue         │              │
    │  All slots have     │                  │   │  Target slot  │              │
    │  cache builds       ▼                  │   │  is busy      ▼              │
    │  in-flight     ──► Error               │   │  generating ──► Preempt      │
    │                     "server busy"      │   │                victim slot   │
    └────────────────────────────────────────┘   └──────────────────────────────┘
```

The first scenario fires before the job enters the batch engine — it blocks
during cache preparation when all IMC slots have pending cache builds
in-flight. The second scenario fires inside the batch engine — the job is
already queued but its target slot is actively generating tokens for another
request.

**Important:** The preemption timeout is measured from when the job enters
the batch engine queue, not from when the HTTP request arrived. Time spent
waiting for cache builds does not count against the preemption budget. This
prevents false preemptions when a request waits for a long cache build
before entering the queue.

#### Debugging IMC Scheduling

| Log Message                           | Meaning                                            |
| ------------------------------------- | -------------------------------------------------- |
| `all slots pending, waiting for slot` | Waiting for a cache build to finish (timeout 1)    |
| `slot became available, retrying`     | A cache build finished, retrying slot scan         |
| `server busy`                         | Wait for slot timed out (timeout 1)                |
| `preempting-slot`                     | Preemption scheduled (timeout 2, shows wait time)  |
| `preempted by queued request`         | Victim slot finished with preemption error         |
| `slot-finished` (after preemption)    | Victim cleaned up, slot available for deferred job |

### 4.9 Model Types and State Management

Kronk supports three model architectures. The model type is detected
automatically at load time and determines how the batch engine manages
sequence state between requests when IMC is enabled. The caching system handles
slot matching and cache building — it is unaffected by model type. The difference
is in the batch engine's slot lifecycle code.

| Model Type | Architecture                         | State Management     | Detection                     |
| ---------- | ------------------------------------ | -------------------- | ----------------------------- |
| Dense      | Standard transformer                 | Partial range delete | Default (not MoE, not Hybrid) |
| MoE        | Mixture of Experts                   | Partial range delete | GGUF `expert_count` metadata  |
| Hybrid     | Attention + Recurrent (DeltaNet/SSM) | Snapshot/Restore     | `llama.ModelIsHybrid`         |

#### Partial Range Delete (Dense and MoE)

Dense and MoE models use the simplest and fastest cleanup strategy after a
request completes.

After a request completes, the batch engine trims the generated tokens from
the KV cache. This removes only the tokens produced during generation, leaving
the cached conversation prefix intact for the next request. This is cheap and
fast — the cached prefix is never re-decoded.

```
Before:  [cached prefix tokens] [generated tokens]
After:   [cached prefix tokens]                     ← trimmed
```

#### Snapshot/Restore (Hybrid)

Hybrid models require a different approach because their recurrent layers
maintain hidden state that cannot be partially trimmed like a KV cache.

Hybrid models mix Attention layers with recurrent layers (DeltaNet/SSM).
Recurrent layers store a hidden state that cannot be "rewound" by partial
range delete — a partial delete corrupts the recurrent state, causing decode
errors on subsequent requests.

Instead, the batch engine uses a snapshot/restore approach:

1. **Snapshot**: After the IMC cache is built or extended but before suffix
   tokens are decoded, the engine captures the full sequence state (KV cache 
   recurrent hidden state) into a byte buffer in RAM.

2. **Restore**: After the request completes, the engine performs a full
   sequence clear and restores the snapshot from the byte buffer. This
   returns the sequence to the exact state it was in after the cached
   prefix, with the recurrent hidden state perfectly preserved.

```
Standard (Dense/MoE):  Trim generated tokens (partial delete)
Hybrid:                Full clear → Restore snapshot (memory copy)
```

The snapshot/restore is a memory copy operation, typically 10-30ms depending
on conversation size.

#### Partial Prefix Rebuilds (Hybrid)

Partial prefix matches are more expensive for hybrid models because the
recurrent state must be rebuilt from the beginning.

When a request matches a partial token prefix (the Non-Deterministic fallback
path), Dense/MoE models trim from the divergence point. Hybrid models cannot
do partial trims, so the engine performs a full sequence clear and re-decodes
the entire cached token sequence from position 0. This is more expensive but
guarantees the recurrent state is built correctly.

#### MoE Performance Characteristics

While MoE models share the same state management as Dense, their architecture
introduces unique performance trade-offs worth understanding.

MoE models use the same state management as Dense (partial range delete),
but have different performance profiles that affect configuration:

- Lower tokens/sec than comparably-sized dense models on Apple Silicon
  due to scattered memory access patterns from expert routing
- Sensitive to aggressive KV cache quantization — use `f16` cache types
  if quality degrades with `q8_0`
- Use `split_mode: row` for multi-GPU setups to enable expert-parallel
  execution

#### Hybrid Constraints

Hybrid models have hard requirements that Kronk enforces at load time.

- KV cache must use `f16` — quantized cache types (e.g., `q8_0`) are
  incompatible with recurrent layers
- Flash attention is automatically disabled

#### Hybrid Guardrails

Kronk protects against corrupted state by automatically recovering when
snapshot operations fail.

If a snapshot restore fails, Kronk clears the slot's IMC metadata so the
slot is not reused with a corrupted sequence. The next request to that slot
triggers a full cache rebuild from scratch.

### 4.10 Debugging State Management

Use these log messages to diagnose how the batch engine is managing KV cache
state between requests. They are especially useful for hybrid models where
snapshot/restore failures can trigger expensive full rebuilds.

| Log Message                  | Meaning                                                     |
| ---------------------------- | ----------------------------------------------------------- |
| `imc-hybrid-snapshot`        | State captured after cache build (shows snapshot_bytes)     |
| `imc-hybrid-snapshot-failed` | StateSeqGetData returned 0 bytes                            |
| `imc-hybrid-restore`         | Snapshot restored after request (shows restored_bytes)      |
| `imc-hybrid-restore-failed`  | StateSeqSetData failed, slot metadata cleared               |
| `imc-hybrid-no-snapshot`     | No snapshot available, full clear + metadata invalidation   |
| `imc-hybrid-rebuild`         | Partial prefix: full clear + re-decode from position 0      |
| `imc-hybrid-trim-rebuild`    | Trim-only prefix: full clear + re-decode truncated sequence |

---
