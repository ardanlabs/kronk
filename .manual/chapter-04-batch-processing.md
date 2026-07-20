# Chapter 4: Batch Processing

## Table of Contents

- [4.1 Concurrency at a Glance](#41-concurrency-at-a-glance)
- [4.2 Generation Slots and Sequences](#42-generation-slots-and-sequences)
- [4.3 Admission, Waiting, and Cancellation](#43-admission-waiting-and-cancellation)
- [4.4 Prompt and Token Scheduling](#44-prompt-and-token-scheduling)
- [4.5 Embedding and Reranking](#45-embedding-and-reranking)
- [4.6 Configuration and Tuning](#46-configuration-and-tuning)
- [4.7 Interaction with Message Caching](#47-interaction-with-message-caching)
- [4.8 Observing Queue Behavior](#48-observing-queue-behavior)

---

Kronk can process requests concurrently while sharing one loaded copy of a
model's weights. The `nseq-max` model setting controls how much concurrency a
model instance provides, but its exact behavior depends on the model's task.

This chapter covers user-visible scheduling and configuration. Model memory,
batch sizes, and KV-cache precision are covered in
[Chapter 3](chapter-03-model-configuration.md). Message-cache session behavior
is covered in [Chapter 5](chapter-05-message-caching.md).

### 4.1 Concurrency at a Glance

Kronk uses two concurrency designs:

| Workload | `nseq-max` controls | Execution design |
| -------- | ------------------- | ---------------- |
| Text generation | Active generation slots | One model context and a shared batch engine |
| Multimodal generation | Active generation slots | The same batch engine, with specialized media prefill |
| Embedding | Independent contexts | Context pool with shared model weights |
| Reranking | Independent contexts | Context pool with shared model weights |

Multimodal generation includes requests that provide images or audio to a
compatible language model. Bucky speech transcription is a separate
whisper.cpp service and is not scheduled by this batch engine; see
[Chapter 18](chapter-18-bucky.md).

Increasing `nseq-max` allows more work to proceed concurrently. It can improve
aggregate throughput when requests overlap, but it also increases memory
capacity and gives each request a smaller share of the same compute resources.
Higher concurrency can therefore increase individual response latency. There
is no universal value that is best for every model, device, and workload.

### 4.2 Generation Slots and Sequences

For text and multimodal generation, the batch engine creates `nseq-max`
execution slots. A slot tracks one active request's prompt position, sampler,
streaming response, and sequence ID.

```diagram
┌───────────────┐       ┌──────────────────────────────────┐
│ Waiting jobs  │──────▶│ Batch engine                     │
└───────────────┘       │                                  │
                        │  Slot 0 ── sequence 0 ── request A│
                        │  Slot 1 ── sequence 1 ── request B│
                        │  Slot 2 ── sequence 2 ── request C│
                        └────────────────┬─────────────────┘
                                         │
                                         ▼
                        ┌──────────────────────────────────┐
                        │ Shared model context and weights │
                        └──────────────────────────────────┘
```

Sequence IDs isolate attention state, so one request cannot attend to another
request's tokens. They are not fixed physical KV-cache partitions. With more
than one sequence, Kronk enables a unified KV pool whose total capacity is
based on:

```text
context-window × nseq-max
```

Each slot is limited to one `context-window`, while unused capacity remains
available to active sequences. Idle slots do not permanently own a slice of
the pool. Even so, increasing `nseq-max` increases the total capacity Kronk
must allocate and budget.

When a request finishes, its slot becomes available for another waiting job.
Scheduling uses the first available slot; jobs do not reserve a particular
slot between requests.

### 4.3 Admission, Waiting, and Cancellation

The outer Kronk API applies the user-visible admission limit before a request
reaches the model. For generation, its default capacity is
`nseq-max × queue-depth`, where the default queue depth is 2.

Internally, the batch engine receives admitted jobs through a bounded handoff
channel and drains them into its pending-job list until slots become available.
The channel is not a second user-visible queue budget. The direct Go SDK option
`model.WithQueueDepth(n)` changes the outer admission multiplier; it does not
resize that internal handoff channel. Embedding and reranking use an admission
capacity of `nseq-max` rather than the queue-depth multiplier.

At the default generation admission depth, `nseq-max: 4` permits up to eight
requests through the outer admission gate. At most four can occupy execution
slots at once; the remainder wait for a slot. Additional callers block at the
admission gate until capacity is released.

Waiting honors request cancellation. If a request's context is cancelled
before admission or while submitting to the engine, the request returns that
cancellation. During model shutdown, the engine rejects new submissions and
finishes active and pending jobs with a shutdown error.

The engine does **not** cancel a long-running request merely because another
job has waited for a slot. Applications that require a maximum generation time
should use request cancellation, server timeouts, or generation limits such as
`max_tokens`.

### 4.4 Prompt and Token Scheduling

Generation work moves through these stages:

1. Prepare the request and plan any reusable cached state.
2. Submit the job and wait for an execution slot.
3. Restore or build cached state and tokenize or prefill remaining input.
4. Generate and stream output tokens.
5. Clear the active sequence and release the slot.

Some preparation and IMC tokenization occurs before submission. Ordinary
non-cached tokenization can occur when the slot starts. The exact boundary is
an implementation detail; the visible queue wait begins around engine
submission and ends when a slot is assigned.

For ordinary text prefill, active slots contribute prompt tokens in
round-robin chunks of up to `nubatch` tokens until the shared `nbatch` capacity
is reached. This prevents one large prompt from consuming every prefill pass
while other slots wait. Generated tokens from active slots can be processed in
the same shared decode loop.

Media input requires specialized encoder and prefill steps, so it is not
always combined with text work in one forward pass. Multi-Token Prediction
(MTP) also changes how some prefill and verification batches are formed. These
special cases preserve the same user-visible slot limit but should not be
treated as identical scheduling at the backend level.

Most users should leave `nbatch` and `nubatch` unset. Kronk derives their
load-time values as described in
[Chapter 3 §3.5](chapter-03-model-configuration.md#35-concurrency-and-batching).

### 4.5 Embedding and Reranking

Embedding and reranking models do not use generation slots. Kronk creates a
pool of `nseq-max` independent model contexts that share the model weights.

```diagram
┌──────────┐       ┌──────────────────────────────┐
│ Requests │──────▶│ Context pool                │
└──────────┘       │  Context 0 ── request A     │
                   │  Context 1 ── request B     │
                   │  Context 2 ── available     │
                   └──────────────────────────────┘
```

Each admitted request acquires one context, performs its work independently,
and returns the context to the pool. If every context is busy, another request
waits until one is released or its context is cancelled. Work from separate
contexts is not combined into the generation engine's shared token batch.

Additional contexts require memory even though model weights are shared. Raise
`nseq-max` only when concurrent embedding or reranking traffic benefits from
the extra contexts.

### 4.6 Configuration and Tuning

Configure concurrency in `~/.kronk/models/model_config.yaml`:

```yaml
mradermacher/Qwopus3.5-4B-Coder.Q8_0:
  context-window: 32768
  nseq-max: 2
```

The file is read at server startup. Restart the server after changing it. The
top-level key must match the model ID used by requests.

Tune from a measured baseline rather than a generic slot recommendation:

1. Start with automatic tuning or `nseq-max: 1` for a controlled baseline.
2. Run the expected number and shape of concurrent requests.
3. Measure aggregate throughput, time to first token, queue wait, and memory.
4. Increase `nseq-max` one step at a time while throughput improves acceptably.
5. Stop when memory pressure, queueing, or per-request latency becomes worse
   than the workload can tolerate.

If requests spend too long waiting for slots, possible responses include:

- increase `nseq-max` if the model and device have sufficient memory;
- reduce `context-window` when the workload does not need it;
- evaluate a smaller KV-cache type or a smaller model; or
- distribute traffic across more model-server instances.

Do not treat weight size plus a hand-calculated KV value as total VRAM. Use the
BUI's **Apps → VRAM Calculator** and retain operating headroom. See
[Chapter 3 §3.6](chapter-03-model-configuration.md#36-memory-planning-and-quantization)
for the components that affect an estimate.

### 4.7 Interaction with Message Caching

Incremental Message Caching (IMC) keeps reusable conversation state in a
logical session, not in a permanently assigned execution slot. Cached state is
externalized to a session store between requests. A later request can restore
that state into any free slot, extend it, and continue generation.

While a request is active, its restored or newly built state consumes cells in
the unified KV pool. Kronk normally snapshots a built or extended stable prefix
during slot startup, before generating the request's suffix. Exact read-only
hits can skip a redundant snapshot. Completion clears the slot's active
sequence. This allows the number of cached conversation identities to differ
from the number of concurrent execution slots.

If every IMC session has work pending, current token-based planning returns a
server-busy error rather than preempting a generation already running in a
batch slot.

Session matching, RAM and disk stores, media caching, invalidation, and cache
settings are documented in [Chapter 5](chapter-05-message-caching.md).

### 4.8 Observing Queue Behavior

Kronk records two direct indicators of generation-slot contention:

- the `queue-wait` trace span, which wraps the submit attempt and subsequent
  slot wait for successful jobs; and
- the `chat_queue_wait_seconds` Prometheus histogram, recorded when a slot is
  assigned.

For a successful job, timing starts immediately before attempting submission
to the batch engine and ends at slot assignment. It does not include time
blocked at the outer SDK admission gate or time spent preparing an IMC session
before the submit attempt. Compare it with end-to-end request duration and
time-to-first-token measurements when diagnosing latency.

Consistently increasing queue-wait time means requests are arriving faster
than slots complete them. Before raising `nseq-max`, confirm that the device
has memory headroom and that aggregate throughput improves under a realistic
concurrent load. See [Chapter 15](chapter-15-observability.md) for metrics,
tracing, and profiling.

---
