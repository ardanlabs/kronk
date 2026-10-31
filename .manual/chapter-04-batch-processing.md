# Chapter 4: Batch Processing

## Table of Contents

- [4.1 Runtime Mental Model](#41-runtime-mental-model)
- [4.2 The Four-Stage Request Lifecycle](#42-the-four-stage-request-lifecycle)
- [4.3 The Generation Inference Lifecycle](#43-the-generation-inference-lifecycle)
- [4.4 Stage 1 — Admit the Request](#44-stage-1-—-admit-the-request)
- [4.5 Stage 2 — Prepare Model Work](#45-stage-2-—-prepare-model-work)
- [4.6 Stage 3 — Schedule the Job](#46-stage-3-—-schedule-the-job)
- [4.7 Stage 4 — Execute in the Slot](#47-stage-4-—-execute-in-the-slot)
  - [4.7.1 Bind and Restore](#471-bind-and-restore)
  - [4.7.2 Prefill Uncached Work](#472-prefill-uncached-work)
  - [4.7.3 Generate Output](#473-generate-output)
  - [4.7.4 Finish and Release Resources](#474-finish-and-release-resources)
- [4.8 Embedding and Reranking](#48-embedding-and-reranking)
- [4.9 Configuration and Tuning](#49-configuration-and-tuning)
- [4.10 Interaction with Message Caching](#410-interaction-with-message-caching)
- [4.11 Observing Queue Behavior](#411-observing-queue-behavior)

---

Kronk can process requests concurrently while sharing one loaded copy of a
model's weights. The `nseq-max` model setting controls how much concurrency a
model instance provides, but its exact behavior depends on the model's task.

This chapter provides the runtime story for generation: how a request is
admitted, turned into model work, scheduled, and executed. Model memory,
batch-size configuration, and KV-cache precision are covered in
[Chapter 3](https://www.kronkai.com/manual#chapter-3-model-configuration). Message-cache session behavior
is covered in [Chapter 5](https://www.kronkai.com/manual#chapter-5-message-caching).

### 4.1 Runtime Mental Model

Concurrency has different meanings for generation and complete-input evaluation:

| Workload | `nseq-max` controls | Behavior |
| --- | --- | --- |
| Text generation | Active generation slots | Requests share model computation while keeping conversation state isolated |
| Multimodal generation | Active generation slots | The same slot limit, with additional media processing |
| Embedding/reranking | Complete sequences evaluated together | Inputs from queued requests can share a batch within sequence and token limits |

Multimodal generation includes requests that provide images or audio to a
compatible language model. Bucky speech transcription is a separate
whisper.cpp service and is not scheduled by this batch engine; see
[Chapter 18](https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription).

Increasing `nseq-max` allows more work to proceed concurrently. It can improve
aggregate throughput when requests overlap, but it also increases memory
capacity and gives each request a smaller share of the same compute resources.
Higher concurrency can therefore increase individual response latency. There
is no universal value that is best for every model, device, and workload.

#### Generation slots and sequences

For text and multimodal generation, `nseq-max` sets the number of execution
slots. Each slot serves one active request. Requests share the loaded weights
but keep prompt state, sampling, and output separate.

```diagram
┌───────────────┐       ┌────────────────────────────────────┐
│ Waiting jobs  │──────▶│ Batch engine                       │
└───────────────┘       │                                    │
                        │  Slot 0 ── sequence 0 ── request A │
                        │  Slot 1 ── sequence 1 ── request B │
                        │  Slot 2 ── sequence 2 ── request C │
                        └────────────────┬───────────────────┘
                                         │
                                         ▼
                        ┌──────────────────────────────────┐
                        │ Shared model context and weights │
                        └──────────────────────────────────┘
```

Sequence IDs isolate attention state, so one request cannot attend to another
request's tokens in either KV layout. With the default
`kv-cache-mode: partitioned`, Kronk gives each sequence a fixed physical KV-cache partition.
Both layouts request aggregate context capacity of:

```text
context-window × nseq-max
```

In partitioned mode, llama.cpp divides that allocation into `nseq-max` streams,
so every slot gets the full configured `context-window`; it is never divided
by the slot count. An idle slot's partition is not borrowed by another slot.
With `kv-cache-mode: shared`, sequences use one aggregate pool instead, while
Kronk enforces the configured context-window limit per request. There is no
reserved per-slot partition in shared mode. Increasing `nseq-max` increases
the total capacity Kronk must allocate and budget in both modes.

Partitioned ordinary generation may require separate native computations when
active sequence IDs have gaps. Shared mode avoids that restriction for ordinary
KV batching, but can cost more with long concurrent histories. See Chapter 3's
KV Cache Layout setting for the tradeoff.

When a request finishes, its slot becomes available for another waiting job.
Scheduling uses the first available slot; jobs do not reserve a particular
slot between requests.

### 4.2 The Four-Stage Request Lifecycle

Every generation request passes through the same four lifecycle stages:

1. **Admit request** — apply the route deadline and acquire SDK admission
   capacity.
2. **Prepare request and reserve session** — validate the request, render and
   tokenize the prompt, and reserve compatible IMC state when eligible.
3. **Schedule job and wait for slot** — submit prepared work to the batch
   scheduler and wait for the first inactive execution slot.
4. **Execute in assigned slot and release resources** — bind state to the
   slot's sequence, restore or prefill model state, generate output, then clear
   the sequence and release ownership.

![The four stages of a Kronk generation request](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/05-acquire-admission.svg)

An IMC session and an execution slot are deliberately separate. A session is a
reusable conversation and model-state identity selected during Stage 2. A slot
is a temporary execution resource assigned during Stage 3 and occupied during
Stage 4. A session can therefore be restored into different slots on different
requests.

### 4.3 The Generation Inference Lifecycle

The four stages describe ownership and waiting. The next view zooms into the
generation path: Stage 2 turns portable request objects into an exact prompt
plan, Stage 3 schedules that plan, and Stage 4 executes it in one slot.

![Generation inference from request preparation through slot execution](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/04-generation-inference-lifecycle.svg)

This lifecycle is the map for the detailed views below. Chat-template rendering
is part of Stage 2. Prefill batching and token generation are parts of Stage 4.
IMC and speculative decoding specialize those stages without changing the
top-level four-stage request lifecycle.

### 4.4 Stage 1 — Admit the Request

The outer Kronk API applies the user-visible admission limit before a request
reaches model preparation. For generation, the capacity is:

```text
admission capacity = max(nseq-max, 1) × queue-depth
```

`queue-depth` counts admitted request layers rather than a literal number of
waiting requests. A depth of `1` admits only enough requests to occupy the
execution slots, so `nseq-max: 2` admits two requests. The default depth of `2`
admits one active layer and one additional waiting layer, so the same two-slot
model admits four requests. An unset or zero value resolves to `2`, and negative values
are invalid. Embedding and reranking do not use this multiplier; their
admission capacity is `max(nseq-max, 1)`.

If admission is full, Kronk waits for a permit for at most `admission-timeout`,
a per-model setting that defaults to three minutes. This deadline applies only
to the admission wait, so prompt preparation, slot waiting, and
generation do **not** inherit a three-minute completion deadline. A caller's
own earlier cancellation or deadline still applies throughout the request.

The admission permit remains held until the request finishes. It therefore
bounds the total number of requests that can be preparing, waiting for an
execution slot, or generating—not merely the number in the handoff channel.

At the default generation admission depth, `nseq-max: 4` permits up to eight
requests through the outer admission gate. At most four can occupy execution
slots at once; the remainder wait for a slot. Additional callers block at the
admission gate until capacity is released.

The model server applies a total inference timeout of 60 minutes by default.
It bounds the entire inference route, including admission, IMC session
reservation, execution-slot waiting, and generation. The admission-specific
three-minute limit normally expires first in Stage 1. Direct SDK callers
should use request cancellation or generation limits such as `max_tokens`.

### 4.5 Stage 2 — Prepare Model Work

After admission, Kronk validates generation parameters and translates the
portable request into exact model work. A model-specific chat template renders
messages, roles, tools, media markers, reasoning controls, and the assistant
generation cue into the protocol the selected model learned during training.

![A chat template translates request context into the selected model's prompt protocol](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/06-render-plan-prompt.svg)

The rendered prompt—not the original message objects—is tokenized and executed.
When IMC is enabled, preparation determines whether a compatible complete
cached prefix can be reused. Kronk prefers a matching conversation cache, then
a compatible System preload; otherwise it prepares a new cache. Partial
overlap alone does not qualify for reuse. Cache planning identifies reusable
work; model computation happens after slot assignment. See
[Chapter 5](https://www.kronkai.com/manual#chapter-5-message-caching) for
eligibility and matching rules.

### 4.6 Stage 3 — Schedule the Job

Prepared requests wait for the first available execution slot. `queue-depth`
sets the admitted-request budget described in Stage 1; it is not another queue
of that size after preparation. Raising it allows more requests to remain in
the system but does not increase execution concurrency. `nseq-max` remains the
maximum number of requests that can occupy slots simultaneously.

Waiting honors request cancellation. If a request's context is cancelled
while waiting for admission, preparing, submitting, waiting for a slot, or
generating, the request returns that cancellation. During model shutdown, the
engine rejects new submissions and finishes active and pending jobs with a
shutdown error.

The engine does **not** cancel a long-running request merely because another
job has waited for a slot. The visible queue wait begins around engine
submission and ends when the first inactive slot is assigned. It remains
bounded by the route deadline and caller cancellation.

### 4.7 Stage 4 — Execute in the Slot

#### 4.7.1 Bind and Restore

A request runs in an available slot. If preparation selected a compatible
cached prefix, Kronk restores it before processing the remaining prompt; a
cache miss starts with a fresh sequence. Cache identities are not permanently
assigned to slots. An expected snapshot that cannot be restored can fail the
request; it is not silently treated as a cache miss.

Cache reuse preserves the prompt's sampling context. Unavailable
speculative-decoding state does not require discarding a valid target-model
prefix; generation can continue without speculation.

#### 4.7.2 Prefill Uncached Work

Prefill computes the prompt state that is not already reusable from a cache.
For ordinary text, one slot retains prefill priority until its prompt is
complete, cancelled, or no longer eligible. `prefill-batch-size`—2048 tokens
by default—caps its text contribution per scheduling iteration. A long prompt
can therefore delay another new prompt even when both have execution slots.

Kronk interleaves this work with output generation from ready slots. A new
prompt does not normally need to finish all of its prefill before existing
responses can continue, but each compute batch still takes time. Larger prefill
batches can increase pauses between streamed output tokens. This is a
responsiveness tradeoff, not a fixed latency guarantee.

Eligible text-only, layer-split multi-GPU workloads can overlap prompt
computation across GPUs. This includes building or extending a text IMC prefix.
Actual overlap depends on the backend; media and speculative workloads should
not be assumed to have the same behavior.

Media requires additional encoding and may require isolated processing rather
than sharing a text batch. Supported causal media can share computation when it
fits; other media uses a separate decode path. In particular, non-causal image
chunks must fit completely in one physical batch. Increase
`image-batch-token-capacity` and reload the model if an oversized chunk is
rejected. This capacity setting does not increase image detail; an explicit
`image-token-budget` controls preprocessing on supported projectors.

![The two prefill phases: restore reusable IMC state, then compute missing request state](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/09-restore-session-state.svg)

Kronk automatically includes capacity for generation and speculative work.
Operators do not need to calculate extra output rows. Active MTP keeps work in
one physical batch, so it cannot use `physical-batch-size` to split computation
into smaller batches. Chapter 3 describes these controls and their memory
tradeoffs; Chapter 6 covers speculative decoding.

#### 4.7.3 Generate Output

Once prompt processing completes, Kronk samples output tokens and continues
generation using each request's own history and sampling settings. Ready
requests share model computation; one request does not receive another's
generated tokens. Streaming clients receive output as it becomes available.

![Stage 4 ordinary token generation from batched decode through sampling and streaming](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/10-run-generation-inference.svg)

The model's end-of-generation signal, parsed completion, and `max_tokens` end generation
normally. Cancellation, context or decode failure, streaming failure, and
engine shutdown use the error path. Sampling controls and generation limits
are documented in [Chapter 10](https://www.kronkai.com/manual#chapter-10-request-parameters),
while endpoint-specific stream framing is documented in
[Chapter 9](https://www.kronkai.com/manual#chapter-9-api-endpoints).

Media input requires specialized encoder and prefill steps, so it is not
always combined with text work in one forward pass. Multi-Token Prediction
(MTP) also changes how some prefill and verification batches are formed. These
special cases preserve the same user-visible slot limit but should not be
treated as identical scheduling at the backend level.
[Chapter 6](https://www.kronkai.com/manual#chapter-6-speculative-decoding-and-mtp)
zooms into proposal, verification, acceptance, and state synchronization.

#### 4.7.4 Finish and Release Resources

On completion, cancellation, or failure, Kronk releases the request's
execution resources, any IMC reservation it still holds, and its admission
permit. The slot's active sequence is cleared before reuse. A cache already
saved by the request can remain available after the slot is released; the
next request does not inherit the previous request's sampling, parsing, or live
sequence state.

Most users should leave `prefill-batch-size` at its 2048-token default. Kronk
derives the internal batch capacities as described in
[Chapter 3 §3.5](https://www.kronkai.com/manual#35-concurrency-and-batching).

### 4.8 Embedding and Reranking

Embedding and reranking models do not use generation slots. They use sequence
batching: complete inputs from one or more queued requests can be evaluated
together. This applies to all models identified as embedding or reranking
models; it is not limited to an architecture allowlist.

```diagram
┌──────────────┐       ┌────────────────────────────────┐
│ Request jobs │──────▶│ Sequence-batch scheduler       │
└──────────────┘       │ seq 0 ── request A, input 0    │
                       │ seq 1 ── request A, input 1    │
                       │ seq 2 ── request B, input 0    │
                       └───────────────┬────────────────┘
                                       ▼
                       ┌───────────────────────────────┐
                       │ One model context and weights │
                       └───────────────────────────────┘
```

The scheduler keeps each embedding input or reranking query-document pair as a
complete sequence. It coalesces already queued requests, fills a native batch
up to the `nseq-max` and token limits, and schedules requests round-robin when
one request cannot fit in a single batch. The engine is intentionally separate
from generation because it does not own long-lived generation slots, samplers,
or streaming state.

The two limits control independent dimensions of a sequence batch:

```text
number of sequences <= nseq-max
sum of sequence tokens <= prefill-batch-size
```

For these models, `prefill-batch-size` is an aggregate batch token budget,
not a per-sequence allowance. With `nseq-max: 8` and
`prefill-batch-size: 2048`, one batch can hold eight 256-token sequences or two
1024-token sequences, but not eight 2048-token sequences. Kronk deliberately
does not multiply `prefill-batch-size` by `nseq-max`; larger batches require
more compute memory. Increase `prefill-batch-size`
explicitly when measured workloads benefit from a larger aggregate token
budget.

Raise `nseq-max` only after measuring throughput and memory for the actual
model and workload.

### 4.9 Configuration and Tuning

Configure concurrency in `~/.kronk/models/model_config.yaml`:

```yaml
version: 2
models:
  mradermacher/Qwopus3.5-4B-Coder.Q8_0:
    context-window: 32768
    nseq-max: 2
    admission-timeout: 3m
    queue-depth: 2
```

The file is read at server startup. Restart the server after changing it. The
entry under `models` must match the model ID used by requests.

`admission-timeout` is separate from the model
server's `KRONK_WEB_INFERENCE_TIMEOUT` (default `60m`): admission timeout only
bounds waiting for a permit, while inference timeout bounds admitted request
processing at the server/web/CLI layer.

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
[Chapter 3 §3.6](https://www.kronkai.com/manual#36-memory-planning-and-quantization)
for the components that affect an estimate.

### 4.10 Interaction with Message Caching

Incremental Message Caching (IMC) keeps reusable conversation state in a
logical session, not in a permanently assigned execution slot. Cached state is
externalized to a session store between requests. A later request can restore
that state into any free slot, extend it, and continue generation.

While a request executes, its restored or newly prepared prefix occupies the
slot's model state. Cache builds and extensions run as scheduled preparation
work, in bounded steps, before processing the remaining prompt. Once the stable
prefix is ready, Kronk saves it for reuse; exact read-only hits can avoid saving
the same state again. Completion clears the live sequence, not the reusable
cache. Cached conversations can therefore outnumber execution slots.

The IMC pool contains:

```text
default session capacity = max(nseq-max, 1) × queue-depth
```

The default session pool matches generation admission capacity. Set
`imc-session-capacity` to a larger explicit value when the measured warm
conversation working set warrants it. An explicit value cannot be smaller than
generation admission capacity.
Therefore, for generation through the Kronk SDK:

```text
session capacity ≥ admission capacity
```

This invariant prevents Kronk from admitting more concurrent generation
requests than the IMC planner has session identities available to reserve. It
avoids moving an admitted request into preparation when every session is
already owned by another admitted request.

A reservation gives one request exclusive access to a cache entry, including
while it waits for a slot. It is not a measure of active generation. A newly
built or extended cache may become available to other requests before its owner
finishes generating. Exact read-only reuse and some media operations retain
the reservation until completion.

Kronk's built-in cache store uses host RAM, and System preloads always remain
in RAM. Embedded applications can supply a custom working-session store;
externalizing state does not by itself make it durable across restarts. Cache
matching, media reuse, invalidation, and settings are covered in
[Chapter 5](https://www.kronkai.com/manual#chapter-5-message-caching).

### 4.11 Observing Queue Behavior

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

Embedding and reranking expose `inference_requests_total`,
`inference_request_duration_seconds`, and `inference_active_requests`, labeled
by operation and runtime (currently `batchseq`). Sequence batching also
publishes `batchseq_queue_wait_seconds`, `batchseq_items`, and
`batchseq_batches_total`. These distinguish outer request concurrency from the
number and width of native batches actually evaluated.

The `inference_*` metrics begin after the outer SDK admission permit is
acquired. They describe admitted model-layer work and do not count admission
wait time or requests that time out or are cancelled before admission.

Consistently increasing queue-wait time indicates contention for execution
capacity; compare request volume, prompt lengths, and generation lengths.
Before raising `nseq-max`, confirm that the device
has memory headroom and that aggregate throughput improves under a realistic
concurrent load. See [Chapter 15](https://www.kronkai.com/manual#chapter-15-observability) for metrics,
tracing, and profiling.

---
