# Chapter 3: Model Configuration

## Table of Contents

- [3.1 Configuration File](#31-configuration-file)
  - [3.1.1 Chat Templates Define the Model Protocol](#311-chat-templates-define-the-model-protocol)
- [3.2 Automatic Tuning](#32-automatic-tuning)
- [3.3 Core Runtime Settings](#33-core-runtime-settings)
- [3.4 GPU and Memory Placement](#34-gpu-and-memory-placement)
- [3.5 Concurrency and Batching](#35-concurrency-and-batching)
- [3.6 Memory Planning and Quantization](#36-memory-planning-and-quantization)
- [3.7 Advanced Features](#37-advanced-features)
- [3.8 Complete Example and Key Reference](#38-complete-example-and-key-reference)

---

Kronk analyzes each model and the available hardware before loading it. Most
models run well without manual tuning. Use per-model configuration when you
need a different context window, more concurrent requests, explicit device
placement, or an advanced feature such as speculative decoding.

This chapter documents model runtime configuration. Server settings such as
the listen address, authentication, and the number of models kept in the pool
are covered in [Chapter 8](https://www.kronkai.com/manual#chapter-8-model-server).

### 3.1 Configuration File

The model server reads per-model overrides from:

```text
~/.kronk/models/model_config.yaml
```

Kronk creates this file on first use. Version 1 stores per-model overrides
under `models`, keyed by the canonical model ID. Use the same ID shown by
`kronk model list` or the `/v1/models` endpoint:

```yaml
version: 1
models:
  unsloth/Qwen3-0.6B-Q8_0:
    context-window: 32768
    nseq-max: 2
    admission-timeout: 3m
    queue-depth: 2
    imc-session-capacity: 8
```

Files without `version` use the legacy version-0 shape, where model IDs are
top-level keys. The remaining examples in this chapter show individual entries
as they appear below `models:`. Model setting names use kebab-case, such as
`context-window` and `nseq-max`. Keys nested under `sampling-parameters` use the
API's snake_case names, such as `top_p`.

The server reads this file during startup. Restart the server after changing
it. To test a different file without replacing the default, run:

```shell
kronk server start --model-config-file=./my-model-config.yaml
```

You can also set `KRONK_POOL_MODEL_CONFIG_FILE` to an alternative path. See
[Chapter 8 §8.5](https://www.kronkai.com/manual#85-model-configuration-files) for
model config file management and
[Chapter 2 §2.5](https://www.kronkai.com/manual#25-models-and-data-paths) for all
data paths.

Inference requests require the canonical `provider/modelID` shown by
`/v1/models`. Bare model IDs are rejected rather than searched across a list of
providers.

#### Model variants

A suffix creates another configuration for the same downloaded model:

```yaml
models:
  unsloth/Qwen3-0.6B-Q8_0:
    context-window: 32768

  unsloth/Qwen3-0.6B-Q8_0/LONG:
    context-window: 65536
```

Select the variant by sending the complete name, including `/LONG`, as the API
request's `model` value. Variants let applications use different runtime
settings without keeping duplicate model files.

#### Other configuration surfaces

Applications embedding the Go SDK can construct a `model.Config` directly.
Request fields such as `temperature`, `top_p`, `seed`, and `max_tokens` can override
generation behavior for an individual request. A configured seed makes requests
that omit `seed` repeatable; omit it from both places to use random sampling.
Those request fields are documented in
[Chapter 10](https://www.kronkai.com/manual#chapter-10-request-parameters).

The hardware processor (`cpu`, `metal`, `cuda`, `rocm`, or `vulkan`) selects a
native library bundle rather than a per-model setting. Kronk detects it during
library installation. Set `KRONK_PROCESSOR` before installing libraries only
when you need to override detection; see
[Chapter 2 §2.4](https://www.kronkai.com/manual#24-libraries).

#### 3.1.1 Chat Templates Define the Model Protocol

A chat template is not cosmetic string formatting. It is executable,
model-specific protocol logic that converts portable request data—messages,
roles, tools, optional media, and supported reasoning controls—into the exact
prompt syntax the model learned during training. The model never receives the
original message objects.

The correct template is therefore part of model compatibility, much like the
tokenizer. A mismatched template can use the wrong role markers, serialize tool
definitions incorrectly, omit media placeholders, or fail to append the cue
that tells the model to begin an assistant response. These failures can be
subtle: the model may still generate fluent text while instruction following,
tool calling, reasoning, or multimodal behavior degrades.

Kronk resolves the template when the model loads, in this order:

1. The explicit `template` path in `model_config.yaml`.
2. A matching Jinja file discovered under Kronk's Jinja directory.
3. The `tokenizer.chat_template` embedded in GGUF metadata.

Prefer the model's supplied template unless you are correcting known-bad
metadata or deliberately testing a custom protocol. A template override must
remain compatible with the model and with any tool-call output parser.
Sampling controls such as `temperature`, `top_p`, penalties, and `max_tokens`
do not become prompt text; they govern token selection and stopping after the
template has rendered.

[Chapter 4 §4.5](https://www.kronkai.com/manual#45-stage-2-—-prepare-model-work)
shows this template protocol as part of Stage 2 request preparation, where the
portable request becomes the exact prompt that Kronk tokenizes and executes.
[Chapter 5](https://www.kronkai.com/manual#chapter-5-message-caching) explains
why IMC renders the complete conversation both with and without the generation
suffix. [Chapter 9](https://www.kronkai.com/manual#chapter-9-api-endpoints)
documents the portable API message formats that templates consume.

### 3.2 Automatic Tuning

The Kronk Model Server runs automatic tuning by default for every model it
loads. No `auto-tune` key is required in `model_config.yaml`. The server derives
a starting configuration from GGUF metadata and the available hardware. This
analysis chooses values such as:

- context window;
- KV cache types;
- maximum parallel sequences;
- GPU layer placement;
- Flash Attention mode; and
- multi-GPU split mode.

Automatic tuning does not select the model weight loading mode. The default
`auto` mode uses mmap when every selected device supports it and otherwise uses
ordinary loading. Storage, filesystem support, page-cache reuse, memory-lock
limits, and workload lifetime still determine whether an explicit mode is
appropriate.

A concrete override in `model_config.yaml` constrains the analysis. Explicit
`context-window` and `nseq-max` values are fixed: automatic tuning includes
them in its memory calculation and never silently reduces them. The special
cache type `auto` is treated as unset and therefore does not clear an analyzed
`f16` or `q8_0` choice. This makes the usual workflow:

1. Start with no override and let Kronk analyze the model.
2. Use the model normally and monitor memory and latency.
3. Override only the setting needed for the workload.

When `context-window` and `nseq-max` are specified but the KV cache types are
omitted, automatic tuning uses this priority:

1. Preserve the configured context window and concurrency.
2. Try `f16` for both KV caches.
3. If `f16` does not fit, try `q8_0` at the same context and concurrency.
4. If `q8_0` does not fit, keep the configured values and report that the
   recommendation does not fit. Automatic tuning does not select a cache type
   below `q8_0` and does not reduce the configured context or concurrency.

This is the recommended server configuration style:

```yaml
ornith-ai/Ornith-1.5-35B-Q8_0/AGENT:
  context-window: 262144
  nseq-max: 2
```

Leave `cache-type-k` and `cache-type-v` out unless the workload requires a
specific cache format. In this example, Kronk sizes the exact 262144-token,
two-sequence configuration with `f16` first and then `q8_0` if necessary.

When context and concurrency are not specified, the balanced analysis limits
the selected context to the model's training context and a maximum of 128K
tokens. It searches supported context buckets from largest to smallest. At
each context it tries `f16` first and then `q8_0` before considering a smaller
context. The minimum recommendation is 4K. CPU-only analysis and systems
without a known GPU budget cannot perform the same fit check. All
recommendations are estimates, not a guarantee that every backend and workload
will fit or have identical memory use.

In the Go SDK, the same analysis is opt-in through `WithAutoTune`. It is not
applied when an application uses the low-level `model` package directly.
Explicit SDK options still take precedence over analyzed values.

### 3.3 Core Runtime Settings

#### Context window

`context-window` is the maximum number of tokens available to one sequence.
Input and generated tokens both consume this capacity.

```yaml
unsloth/Qwen3-0.6B-Q8_0:
  context-window: 32768
```

A larger window increases KV-cache memory and can reduce the number of parallel
sequences that fit. It also cannot create model capability that was absent
during training. If the requested window exceeds the model's native context,
the model may require RoPE scaling; see [Chapter 7](https://www.kronkai.com/manual#chapter-7-yarn-extended-context).

#### KV cache types

The KV cache stores attention state for tokens already processed. Configure
the key and value caches independently:

```yaml
unsloth/Qwen3-0.6B-Q8_0:
  cache-type-k: q8_0
  cache-type-v: q8_0
```

Common choices are:

| Value | Meaning |
| ----- | ------- |
| `f16` | Higher precision and larger cache |
| `q8_0` | Smaller quantized cache |
| `q4_0` | More aggressive compression |

The actual memory reduction includes block and alignment overhead, so it is not
an exact ratio for every model and backend. Start with automatic tuning. If an
explicit quantized cache changes output quality, compare the same workload with
`f16` before changing unrelated settings.

#### Flash Attention

Flash Attention can reduce attention memory traffic and improve performance,
especially at longer contexts:

```yaml
unsloth/Qwen3-0.6B-Q8_0:
  flash-attention: auto
```

Valid values are `enabled`, `disabled`, and `auto`. Automatic tuning uses
`auto` when a GPU is available and disables it for CPU-only analysis. Set an
explicit value only for backend compatibility or controlled benchmarking.

#### Sliding Window Attention

Kronk reads the sliding-window size from model metadata. `swa-full` controls
the cache allocation used by models with sliding window attention:

```yaml
some-provider/some-swa-model:
  swa-full: false
```

When unset, Kronk leaves the choice to llama.cpp. Explicitly setting `false`
uses the compact sliding-window cache, which can save memory but limits context
caching and shifting. Setting `true` requests the full cache at a higher memory
cost. This key has no effect on models without SWA metadata.

### 3.4 GPU and Memory Placement

Automatic tuning normally places model layers and operations. Use these
settings when a model does not fit or when a multi-GPU deployment needs an
explicit layout.

#### Model layers

`ngpu-layers` controls how many model layers are offloaded to the GPU:

```yaml
some-provider/some-model:
  ngpu-layers: 20
```

| Value | Behavior |
| ----- | -------- |
| `0` | Offload all layers to the GPU |
| `-1` | Keep all layers on the CPU |
| Positive integer | Offload that many layers |

Partial offload can make a model fit in limited VRAM, but CPU-resident layers
usually reduce inference speed. On unified-memory systems, CPU and GPU do not
have separate memory pools, although placement can still affect performance.

#### Model weight loading

`load-mode` controls how Kronk reads model weights. Its default and Go zero
value are `auto`:

```yaml
some-provider/some-model:
  load-mode: auto
```

| Value | Behavior |
| ----- | -------- |
| `auto` | Use mmap when every selected device supports it; otherwise use ordinary loading; this is the default |
| `mmap` | Force memory-mapped model weights and use the operating system page cache |
| `none` | Load weights without mmap, mlock, or direct I/O |
| `mlock` | Load weights and request that their pages remain resident in RAM without forcing mmap |
| `mmap+mlock` | Memory-map weights and request that their pages remain resident in RAM |
| `direct-io` | Bypass the operating system page cache where the platform and filesystem support it |

Use `none` when mmap is unsuitable or when direct allocation is needed for a
measured NUMA placement issue. Use `mlock` only when the host has enough
physical and lockable RAM for the model plus the rest of the workload; process
resource limits may prevent all pages from being locked. Use `mmap+mlock` when
the model should be both memory mapped and kept resident. Direct I/O can avoid
page-cache pollution for some local-storage and GPU-loading workloads, but it
can also make repeated loads slower and is not supported by every filesystem.
Benchmark the actual model path before selecting it.

Kronk applies one load mode to the target model and any separate draft model.
The Go SDK equivalent is `model.WithLoadMode`, using `LoadModeAuto`,
`LoadModeMMap`, `LoadModeNone`, `LoadModeMLock`, `LoadModeMMapMLock`, or
`LoadModeDirectIO`.

The former `use-mmap` and `use-direct-io` keys are no longer supported. Migrate
existing configuration as follows:

| Removed configuration | Replacement |
| --------------------- | ----------- |
| `use-mmap: true` | `load-mode: mmap` |
| `use-mmap: false` | `load-mode: none` |
| `use-direct-io: true` | `load-mode: direct-io` |

#### KV cache and operations

The KV cache and host tensor operations are offloaded to the GPU by default:

```yaml
some-provider/some-model:
  offload-kqv: false
  op-offload: false
```

`offload-kqv: false` keeps the KV cache on the CPU. `op-offload: false` keeps
host tensor operations on the CPU. These options can reduce discrete-GPU VRAM
pressure at a performance cost. They do not reduce total memory requirements.

For multimodal models, `proj-on-cpu: true` keeps the media projector on the
CPU without changing placement of the language model itself.

#### Multiple GPUs

`split-mode` accepts:

| Value | Behavior |
| ----- | -------- |
| `none` | Use one GPU |
| `layer` | Distribute whole layers across GPUs |
| `row` | Use deprecated row-split tensor parallelism where supported |

When the setting is omitted, Kronk selects `layer`, matching llama.cpp's
default and most compatible multi-GPU mode. Layer mode can distribute a single
GGUF file across multiple GPUs. `row` remains available for explicit legacy
configurations but is not recommended for new deployments.

For explicit placement, `devices` names the devices and `tensor-split` gives
their proportional shares:

```yaml
some-provider/some-model:
  devices: [CUDA0, CUDA1]
  split-mode: layer
  tensor-split: [0.6, 0.4]
```

The number of `tensor-split` values must match the number of devices. Omit the
split to let the backend derive it from available memory. `main-gpu` selects
the primary device when `split-mode` is `none`.

### 3.5 Concurrency and Batching

`nseq-max` controls model concurrency:

```yaml
unsloth/Qwen3-0.6B-Q8_0:
  nseq-max: 4
```

For text generation, this creates up to four batch-engine slots. Their
sequence state and KV capacity are isolated. Kronk allocates an aggregate
context of `context-window × nseq-max`, which llama.cpp divides into one fixed
`context-window` stream per slot. Increasing `nseq-max` therefore increases
the capacity Kronk must budget and can substantially increase memory use.

For supported embedding and reranking architectures, `nseq-max` is the maximum
number of complete inputs or query-document pairs in one sequence batch on a
shared context. Architectures that have not been proven safe for that runtime
use `nseq-max` to size a pool of independent single-sequence contexts instead.
See
[Chapter 4](https://www.kronkai.com/manual#chapter-4-batch-processing) for request scheduling and the
differences between model types.

One setting controls prompt batching and sequence-batch token capacity:

| Key | Load-time default | Purpose |
| --- | ----------------- | ------- |
| `prefill-batch-size` | `2048` | Generation prefill contribution per decode iteration, or aggregate embedding/reranking tokens per native sequence batch |

For generation models, a larger value can move a long prompt to generation in
fewer decode calls, but requires larger compute buffers and each call runs
longer before already-generating slots can run again. A smaller value gives
generating slots more frequent scheduling opportunities at the cost of more
prefill decode calls.

For embedding and reranking models, Kronk sets both internal `NBatch` and
`NUBatch` to `prefill-batch-size`. This is the aggregate token capacity of one
native sequence batch, not a capacity granted separately to every sequence.
The batch is constrained independently by `nseq-max` complete sequences and by
`prefill-batch-size` combined tokens. For example, `nseq-max: 8` with
`prefill-batch-size: 2048` can evaluate eight 256-token sequences, four
512-token sequences, two 1024-token sequences, or one 2048-token sequence in a
single native batch. Kronk does not multiply the token capacity by `nseq-max`;
doing so would allocate a much larger physical compute batch. Increase
`prefill-batch-size` explicitly when the expected embedding or reranking
workload needs a larger aggregate batch.

Most deployments should use the default.

Kronk derives llama.cpp's internal physical and logical batch capacities from
this value. It reserves one output row per slot for non-MTP generation. MTP
reserves `1 + ndraft` rows per slot because speculative verification includes
the sampled token and its draft candidates. With four slots and the default
`ndraft: 3`, the internal effective sizes are `NUBatch: 2048`, `NBatch: 2052`
for non-MTP and `NUBatch: 2064`, `NBatch: 2064` for MTP. Both can stage
generation rows and then add a complete 2048-token prefill contribution from
the current owner in the same logical decode. Non-MTP may split that tray into
physical ubatches; MTP keeps it in one physical batch so dense NextN rows retain
their expected mapping.

The `status[resolved]` `batch-sizing` debug log reports the configured prefill
size, generation reserve, effective internal sizes, and MTP state. The Slots
diagnostic screen also displays effective `NBatch / NUBatch` as read-only
runtime values. Multimodal encoders may require an entire media-token chunk to
fit in one physical batch, so do not lower `prefill-batch-size` for a multimodal
model without testing media input.

![How two requests move from prefill to generation and how non-MTP and MTP batch sizing supports them](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/chapter-04/batch-sizing-mtp-vs-non-mtp.svg)

Incremental Message Caching is configured separately with
`incremental-cache` and related cache settings. See
[Chapter 5](https://www.kronkai.com/manual#chapter-5-message-caching) rather than treating cached
conversations as dedicated physical slots.

### 3.6 Memory Planning and Quantization

Model memory is not just the GGUF file size plus a simple KV-cache formula.
Depending on the model and backend, memory use can include:

- model weights placed on each device;
- KV cache or recurrent state;
- compute and output buffers;
- multimodal projector weights and buffers;
- speculative drafter weights and state; and
- backend allocations and safety margins.

Context length, `nseq-max`, cache precision, layer placement, SWA, and model
architecture all affect the result. Use **Apps → VRAM Calculator** in the BUI
to inspect the GGUF metadata and estimate a specific configuration. Treat the
result as planning guidance and retain headroom for the backend and other
processes.

The estimator follows the model's per-layer attention topology when the GGUF
provides it. Full-attention and sliding-window layers can have different KV
head counts and dimensions; compact SWA includes its physical-batch headroom,
while `swa-full: true` allocates SWA against the full context. Shared KV layers
are not charged twice.

Hybrid recurrent models such as Qwen 3.5 do not allocate ordinary K/V tensors
for recurrent layers. Kronk instead estimates their float32 recurrent state per
sequence and, when the active speculative configuration is known, the current
state plus required rollback copies. Appended embedded-MTP/NextN layers are
excluded from the target context's attention-state allocation because they are
not target trunk layers. Their weights remain part of model size, and embedded
MTP can increase recurrent rollback-state memory. These distinctions are why a
generic transformer KV formula can substantially over- or under-estimate a
hybrid model.

If a configuration does not fit, consider these changes one at a time:

1. Reduce `context-window`.
2. Reduce `nseq-max`.
3. Let automatic tuning use q8_0, or explicitly compare a quantized KV cache.
4. Move the KV cache or some model layers to CPU.
5. Choose a smaller or more heavily quantized GGUF.

#### Weight quantization versus KV-cache quantization

The quantization in a GGUF filename describes the model's stored weights. It
is selected when downloading the model and cannot be changed in
`model_config.yaml`. Lower-bit files generally use less storage and memory,
but the quality and speed trade-offs depend on the model, quantizer, and
hardware. Parameter count alone is not enough to predict whether a model fits.

`cache-type-k` and `cache-type-v` quantize runtime attention state instead.
They do not change model weights. Evaluate weight format and KV-cache format as
separate choices.

### 3.7 Advanced Features

#### LoRA adapters

Kronk can apply one or more user-provided, llama.cpp-compatible LoRA or QLoRA
adapter GGUF files when it loads a base model. The adapter must be compatible
with that base model. An incompatible or invalid adapter prevents the model
from loading.

An `id` resolves beneath the Kronk data root's `lora` directory. Do not include
the `.gguf` extension:

```yaml
some-provider/base-model:
  adapters:
    - id: acme/support
    - id: concise
      scale: 0.5
```

With the default data root, these IDs resolve to:

```text
~/.kronk/lora/acme/support.gguf
~/.kronk/lora/concise.gguf
```

Create the directories as needed and place the files there before loading the
model. `KRONK_BASE_PATH` or `--base-path` moves the `lora` directory with the
rest of the Kronk data root.

To keep an adapter elsewhere, specify its absolute path instead:

```yaml
some-provider/base-model:
  adapters:
    - path: /opt/adapters/support.gguf
      scale: 1.0
```

Each entry must set exactly one of `id` or `path`. The file must exist, be a
regular file, and have a `.gguf` extension. The optional `scale` must be a
finite, non-negative number and defaults to `1.0`; an explicit `0` disables
that adapter's contribution while keeping the configured set unchanged.
Multiple adapters compose additively using their configured scales.

Adapter files and scales are fixed for the lifetime of the loaded model. After
changing them, restart the server or otherwise unload and reload that model.
Kronk does not download adapters, resolve them through the model catalog, or
accept per-request adapter changes.

#### Speculative decoding and MTP

Kronk supports a separate draft GGUF and Multi-Token Prediction (MTP). MTP may
be embedded in the target GGUF or supplied as a model-specific companion file
that Kronk's catalog and download flow associates with the target. A separate
classic draft must already be downloaded, must have a compatible vocabulary,
and requires `nseq-max: 1`:

```yaml
some-provider/target-model:
  nseq-max: 1
  draft-model:
    model-id: some-provider/compatible-draft-model
    ndraft: 5
```

MTP is detected automatically from the downloaded target and its companion
files. To override only its starting draft-token count, omit `model-id`:

```yaml
some-provider/mtp-target-model:
  draft-model:
    ndraft: 6
```

Use `speculation` to select the implementation for a model:

```yaml
some-provider/mtp-target-model:
  speculation: disabled # auto, disabled, classic, or mtp
```

`auto` preserves automatic selection. `disabled` runs target-only and does not
load draft resources. `classic` requires `draft-model.model-id`; `mtp` requires
a compatible embedded head or companion assistant.

Do not use model names or benchmark results as universal draft-selection rules.
Measure acceptance and throughput on the actual workload. See
[Chapter 6](https://www.kronkai.com/manual#chapter-6-speculative-decoding-and-mtp) for drafter selection,
adaptive throttling, observability, and limitations.

#### Extended context with YaRN

Do not add RoPE scaling merely because a large `context-window` fits in memory.
Scaling must match the model and its native training context. Configuration
uses `rope-scaling-type` and the `yarn-*` keys described in
[Chapter 7](https://www.kronkai.com/manual#chapter-7-yarn-extended-context).

#### Per-model sampling defaults

`sampling-parameters` supplies defaults for requests using one model:

```yaml
unsloth/Qwen3-0.6B-Q8_0:
  sampling-parameters:
    temperature: 0.7
    top_p: 0.8
    top_k: 20
```

The nested keys use snake_case because they match request parameter names.
Clients can provide request-specific values. See
[Chapter 10](https://www.kronkai.com/manual#chapter-10-request-parameters) for behavior and the full
parameter reference.

#### Per-model chat-template defaults

`chat-template-kwargs` supplies model-specific Jinja variables independently
from sampling defaults. Request-level `chat_template_kwargs` override matching
model defaults. First-class request parameters such as `reasoning_effort`
remain top-level and are resolved separately:

```yaml
unsloth/gemma-4-26B-A4B-it-UD-Q8_K_XL:
  chat-template-kwargs:
    preserve_thinking: true
```

`preserve_thinking` is template-only. Requests should send it inside
`chat_template_kwargs`; it is not a sampling parameter.

Only templates that reference a key respond to it. For example, Gemma 4 uses
`preserve_thinking` to retain reasoning from earlier assistant messages that
contain tool calls. This can make rendered prompts more stable across user
turns, but it increases input-token use and does not change Gemma's separate
look-ahead behavior at some assistant/tool boundaries.

`MODEL-CONFIG` and per-request `FINAL-PARAMS` logs include the effective
template-kwarg keys. Boolean values are shown directly so settings such as
`preserve_thinking=true` can be verified; arbitrary non-boolean values are
reported as `configured` instead of being written to logs.

### 3.8 Complete Example and Key Reference

This example shows the file structure and naming conventions. It is not a
recommendation that every model needs these overrides:

```yaml
# ~/.kronk/models/model_config.yaml

version: 1
kms: {}
models:
  unsloth/Qwen3-0.6B-Q8_0:
    context-window: 32768
    nseq-max: 2
    cache-type-k: q8_0
    cache-type-v: q8_0
    flash-attention: auto
    incremental-cache: true
    sampling-parameters:
      temperature: 0.7
      top_p: 0.8

  unsloth/Qwen3-0.6B-Q8_0/LONG:
    context-window: 65536
    nseq-max: 1

  some-provider/large-model:
    context-window: 16384
    ngpu-layers: 20
    offload-kqv: false
```

Common model-entry keys are summarized below. An omitted hardware-related value
is normally supplied by analysis or by the load-time defaults.

| Key | Values | Purpose |
| --- | ------ | ------- |
| `context-window` | Positive token count | Per-sequence context capacity |
| `cache-type-k`, `cache-type-v` | `f16`, `q8_0`, `q4_0`, and supported GGML types | KV-cache precision |
| `flash-attention` | `enabled`, `disabled`, `auto` | Attention implementation mode |
| `nseq-max` | Positive integer | Generation slots, sequence-batch width, or fallback context-pool size |
| `admission-timeout` | Go duration, default `3m` | Maximum SDK admission-permit wait; separate from the server's `KRONK_WEB_INFERENCE_TIMEOUT` (default `60m`) |
| `queue-depth` | Non-negative integer, default `2` | Generation admission and handoff capacity multiplier |
| `imc-session-capacity` | Positive integer; derived when omitted | Reusable IMC conversation identities retained in RAM or on disk |
| `prefill-batch-size` | Positive token count, default `2048` | Generation prefill contribution per decode iteration, or aggregate embedding/reranking tokens per native sequence batch |
| `ngpu-layers` | `-1`, `0`, or a positive count | CPU/GPU layer placement |
| `load-mode` | `auto`, `mmap`, `none`, `mlock`, `mmap+mlock`, `direct-io` | Model weight loading strategy |
| `offload-kqv` | Boolean | Place KV cache on GPU when true |
| `op-offload` | Boolean | Place host tensor operations on GPU when true |
| `proj-on-cpu` | Boolean | Keep multimodal projector on CPU |
| `devices` | Device-name list | Devices available to the model |
| `split-mode` | `none`, `layer`, `row` | Multi-GPU distribution mode |
| `main-gpu` | Device index | Primary device in single-GPU mode |
| `tensor-split` | Numeric share list | Proportional multi-GPU placement |
| `swa-full` | Boolean | Full or compact SWA cache |
| `incremental-cache` | Boolean | Incremental Message Cache |
| `adapters` | List of `id` or absolute `path`, plus optional `scale` | Fixed load-time LoRA adapters |
| `draft-model` | Mapping | Separate drafter or MTP draft-count override |
| `speculation` | `auto`, `disabled`, `classic`, `mtp` | Select speculative-decoding implementation |
| `rope-scaling-type` | Supported scaling mode | Extended-context scaling |
| `sampling-parameters` | Mapping | Per-model generation defaults |
| `chat-template-kwargs` | Mapping | Per-model Jinja template defaults; request values override matching keys |
| `template` | File path | Override the model's chat template |

Prefer the automatic values until a measured workload gives you a reason to
override them. Change one setting at a time so memory, quality, and throughput
effects remain attributable.

---
