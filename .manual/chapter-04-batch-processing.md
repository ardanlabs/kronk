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

Kronk uses three concurrency designs:

| Workload | `nseq-max` controls | Execution design |
| --- | --- | --- |
| Text generation | Active generation slots | One model context and the generation batch engine |
| Multimodal generation | Active generation slots | The generation batch engine with specialized media prefill |
| Supported embedding/reranking | Sequences per native batch | One model context and the sequence-batch engine |
| Other embedding/reranking | Independent contexts | Context-pool fallback with shared model weights |

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

For text and multimodal generation, the batch engine creates `nseq-max`
execution slots. A slot tracks one active request's prompt position, sampler,
streaming response, and sequence ID.

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
request's tokens. Kronk also gives each sequence a fixed physical KV-cache
partition. The aggregate context allocation is:

```text
context-window × nseq-max
```

llama.cpp divides that allocation into `nseq-max` streams, so every slot gets
the full configured `context-window`; it is never divided by the slot count.
An idle slot's partition is not borrowed by another slot. Increasing
`nseq-max` therefore increases the total capacity Kronk must allocate and
budget.

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

![The four stages of a Kronk generation request](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/chapter-04/request-lifecycle.svg)

An IMC session and an execution slot are deliberately separate. A session is a
reusable conversation and model-state identity selected during Stage 2. A slot
is a temporary execution resource assigned during Stage 3 and occupied during
Stage 4. A session can therefore be restored into different slots on different
requests.

### 4.3 The Generation Inference Lifecycle

The four stages describe ownership and waiting. The next view zooms into the
generation path: Stage 2 turns portable request objects into an exact prompt
plan, Stage 3 schedules that plan, and Stage 4 executes it in one slot.

![Generation inference from request preparation through slot execution](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/chapter-04/generation-inference-lifecycle.svg)

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

An unset `queue-depth` resolves to 2. Negative values are invalid. Embedding
and reranking do not use the queue-depth multiplier; their admission capacity
is `max(nseq-max, 1)`.

If admission is full, Kronk waits for a permit for at most `admission-timeout`,
a per-model SDK setting that defaults to three minutes. This deadline is local
to the admission wait. Kronk discards that child deadline as
soon as the request is admitted, so prompt preparation, slot waiting, and
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

![A chat template translates request context into the selected model's prompt protocol](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/chapter-04/chat-template-protocol.svg)

The rendered prompt—not the original message objects—is tokenized and executed.
For an IMC-eligible request, Kronk independently renders the complete prompt
with and without the generation cue. After tokenization or media-plan
construction, the no-cue plan is reusable only when it is a strict prefix of
the generation-ready plan and leaves a nonempty inference tail; a media tail
must also be text-only. Kronk then finds the longest complete safe reusable
prefix from all working sessions first. Only when none matches does it search
the model's System cache pool and preload a separately selected working
session. It never treats a
coincidental partial token overlap inside a saved snapshot as reusable state.
[Chapter 5](https://www.kronkai.com/manual#chapter-5-message-caching) zooms
further into this planning step.

Ordinary non-cached tokenization can occur when the slot starts. The exact
internal boundary does not change the ownership story: request preparation
defines the model work, while Stage 4 performs the model computation.

### 4.6 Stage 3 — Schedule the Job

Internally, the batch engine receives admitted jobs through a bounded handoff
channel and drains them into its pending-job list until slots become available.
The channel is not a second user-visible queue budget. The direct Go SDK option
`model.WithQueueDepth(n)` changes both the outer admission multiplier and the
handoff channel capacity. The handoff capacity is `NSeqMax × QueueDepth`;
`pendingJobs` remains responsible for jobs drained while every slot is busy.

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

When the scheduler assigns a slot, Kronk binds any reserved IMC session to that
slot's fixed llama sequence ID. A compatible saved prefix is restored from the
session store; otherwise the sequence starts from an empty state. The session
identity is not permanently attached to the slot.

Restoring target KV does not restore a live sampler object. Kronk primes
penalties and DRY from the complete logical prompt, including the restored
prefix, before generation. When own-KV MTP state is available, its paired draft
snapshot and final hidden row are restored with the target; shared-target-KV
MTP resumes from the restored target state. Missing or invalid draft-side state
falls back to target-only generation without discarding a valid target prefix.

#### 4.7.2 Prefill Uncached Work

For ordinary text prefill, one active slot owns prefill until its prompt is
complete. Each decode iteration first stages generated and speculative rows
from every ready output slot, then gives the remaining tray capacity to the
prefill owner. This gets one request to first-token generation quickly without
pausing output already streaming from other slots.

More precisely, `prefill-batch-size`—2048 tokens by default—caps the owner's
contribution during one scheduler visit. Kronk derives an internal logical
batch capacity that also reserves generation rows. For the current prefill
owner, Kronk adds:

```text
min(remaining prompt tokens, available logical-batch space, prefill-batch-size)
```

The scheduler retains a prefill cursor between decode iterations. It remains
on the current owner across prefill contributions and advances to the next
eligible slot only when that owner's prefill completes, is cancelled, or
otherwise becomes ineligible. A long prompt can therefore make another new
prompt wait, but it cannot block output-token generation between decode
iterations.

At debug level, each contribution emits `status[prefill-scheduled]` with
`iteration`, `slot`, `prefill_slots`, `generation_contributions`,
`chunk_tokens`, `prefill_remaining`, `prefill_complete`, `selector_start`,
`selector_selected`, `selector_next`, `generation_rows`, `tray_tokens`,
`nbatch`, and `nubatch`. `prefill_slots` lists every eligible prefill slot for
that iteration. Each `generation_contributions` entry reports its slot, staged
row count, and mode; M-RoPE direct decoding is identified separately because it
does not occupy the shared tray. The three selector fields show the persistent
cursor before selection, the selected slot-array index, and the cursor retained
for the next iteration. While an owner is still prefilling,
`selector_selected` and `selector_next` remain equal. Completion moves
`selector_next` to the following slot-array index. If generation rows consume
the complete logical tray, `status[prefill-deferred]` records the same slot and
selector snapshot without a prefill contribution. The older `next_slot` field
remains as an alias for `selector_next`.

![The two prefill phases: restore reusable IMC state, then compute missing request state](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/chapter-04/prefill-batching.svg)

Kronk adds a generation reserve to the 2048-token default prefill contribution.
Non-MTP reserves one row per slot and keeps the internal physical capacity at
2048, so llama.cpp may split the logical tray into physical ubatches. MTP
reserves `1 + ndraft` rows per slot and raises both internal capacities so a
full speculative verification round and the prefill contribution remain in one
physical batch. For four slots, the internal effective values are
`NUBatch: 2048`, `NBatch: 2052` for non-MTP and `NUBatch: 2064`,
`NBatch: 2064` for MTP with the default `ndraft: 3`.

Generation-row count comes from the generation mode, not from the configured
prefill size:
ordinary generation contributes one row, while MTP with `ndraft: 3` contributes
up to four rows—the sampled token plus three candidates. Generation and prefill
therefore continue in the same scheduler iteration for both modes. If the
prefill owner is not itself generating, some of the worst-case reserve remains
unused.

At model load, `batch-sizing status[resolved]` logs
`configured-prefill-batch-size`, `prefill-batch-size`,
`generation-rows-per-slot`, `generation-reserve`, `effective-prefill`,
`effective-nbatch`, `effective-nubatch`, `mtp`, and `automatic-padding`.
The effective values describe derived llama.cpp capacities; they are diagnostic
output rather than separate configuration controls.

#### 4.7.3 Generate Output

Once the final prefill row produces logits, ordinary non-speculative generation
repeats the decode-and-sample loop below. The first output token is sampled
directly from the final prefill logits. On later iterations, Kronk decodes the
previously selected token into that slot's sequence state, samples from the new
logits, processes and streams the resulting text, and retains the selected token
for the next iteration. A newly selected token is therefore not committed to KV
state until the following decode.

![Stage 4 ordinary token generation from batched decode through sampling and streaming](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/chapter-04/token-generation-loop.svg)

Vocabulary EOG, parser-signaled completion, and `max_tokens` end generation
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

On normal completion or error, Kronk finishes the response, clears the active
sequence, releases the slot, completes or releases any IMC reservation, and
returns the outer admission permit. The next pending job can then occupy the
slot; it does not inherit the prior request's sampler, parser, or sequence
state.

Most users should leave `prefill-batch-size` at its 2048-token default. Kronk
derives the internal batch capacities as described in
[Chapter 3 §3.5](https://www.kronkai.com/manual#35-concurrency-and-batching).

### 4.8 Embedding and Reranking

Embedding and reranking models do not use generation slots. Kronk chooses a
separate runtime from the GGUF architecture metadata. Architectures proven
safe for multi-sequence pooled evaluation use the sequence-batch engine. The
current compatibility set is Qwen3 embedding and BERT reranking models.

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

Unknown or unsafe architectures use the context-pool fallback. Each admitted
request acquires one independent single-sequence context, performs its work,
and returns the context to the pool. This avoids native assertions seen with
some architectures during multi-sequence initialization. Additional fallback
contexts require memory even though model weights are shared.

Runtime selection is conservative: a model name identifies the task, while
GGUF `general.architecture` metadata must match the compatibility allowlist to
select sequence batching. Missing or unrecognized architecture metadata uses
the fallback. Raise `nseq-max` only after measuring throughput and memory for
the selected runtime.

### 4.9 Configuration and Tuning

Configure concurrency in `~/.kronk/models/model_config.yaml`:

```yaml
mradermacher/Qwopus3.5-4B-Coder.Q8_0:
  context-window: 32768
  nseq-max: 2
  admission-timeout: 3m
  queue-depth: 2
```

The file is read at server startup. Restart the server after changing it. The
top-level key must match the model ID used by requests.

The Go SDK equivalents are `model.WithAdmissionTimeout(3*time.Minute)` and
`model.WithQueueDepth(2)`. `admission-timeout` is separate from the model
server's `KRONK_WEB_INFERENCE_TIMEOUT` (default `60m`): admission timeout only
bounds waiting for a permit, while inference timeout bounds admitted request
processing at the server/web/CLI layer.

Local representative benchmarks are available for both embedding and
reranking runtimes:

```shell
make benchmark-embedding-fallback
make benchmark-embedding-batchseq
make benchmark-rerank-fallback
make benchmark-rerank-batchseq
```

Each path uses a model known to select that runtime, so these are deployment
baselines rather than controlled measurements of scheduler overhead: model
architecture and weights differ between each fallback/sequence-batch pair.

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

While a request is active, its restored or newly built state consumes cells in
that slot's KV stream. Kronk normally snapshots a built or extended stable
prefix during slot startup, before generating the request's suffix. Exact
read-only hits can skip a redundant snapshot. Completion clears the slot's
active sequence. This allows the number of cached conversation identities to
differ from the number of concurrent execution slots.

The IMC pool contains:

```text
default session capacity = max(nseq-max, 1) × max(3, queue-depth)
```

The minimum of three sessions per execution slot preserves reusable
conversation state beyond the number of requests that can execute at once.
When `queue-depth` is greater than 3, the pool expands with admission capacity.
Set `imc-session-capacity` to a larger explicit value when the measured warm
conversation working set warrants it. An explicit value may reduce the default
but cannot be smaller than generation admission capacity.
Therefore, for generation through the Kronk SDK:

```text
session capacity ≥ admission capacity
```

This invariant prevents Kronk from admitting more concurrent generation
requests than the IMC planner has session identities available to reserve. It
avoids moving an admitted request into preparation when every session is
already owned by another admitted request.

A session is **reserved** while one request has exclusive ownership of its IMC
state and other planners must skip it. Reservation does not necessarily mean
that token generation is active. Cache append or rebuild paths can publish a
stable snapshot and release the reservation before generation finishes, while
exact, read-only, and some media paths can retain it longer.

If every IMC session is reserved, current token-based planning returns a
server-busy error rather than preempting another request's session. With the
capacity invariant, this remains a defensive path for direct low-level model
callers, leaked reservations, or an internal invariant violation rather than
the expected result of a valid SDK queue-depth configuration.

Session matching, RAM and disk stores, media caching, invalidation, and cache
settings are documented in [Chapter 5](https://www.kronkai.com/manual#chapter-5-message-caching).

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
by operation and runtime (`batchseq` or `context_pool`). Sequence batching also
publishes `batchseq_queue_wait_seconds`, `batchseq_items`, and
`batchseq_batches_total`. These distinguish outer request concurrency from the
number and width of native batches actually evaluated.

The `inference_*` metrics begin after the outer SDK admission permit is
acquired. They describe admitted model-layer work and do not count admission
wait time or requests that time out or are cancelled before admission.

Consistently increasing queue-wait time means requests are arriving faster
than slots complete them. Before raising `nseq-max`, confirm that the device
has memory headroom and that aggregate throughput improves under a realistic
concurrent load. See [Chapter 15](https://www.kronkai.com/manual#chapter-15-observability) for metrics,
tracing, and profiling.

---
