# Chapter 3: Model Configuration

## Table of Contents

- [3.1 Configuration File](#31-configuration-file)
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
are covered in [Chapter 8](chapter-08-model-server.md).

### 3.1 Configuration File

The model server reads per-model overrides from:

```text
~/.kronk/models/model_config.yaml
```

Kronk creates this file on first use. The file is a flat YAML map keyed by the
canonical model ID. Use the same ID shown by `kronk model list` or the
`/v1/models` endpoint:

```yaml
unsloth/Qwen3-0.6B-Q8_0:
  context-window: 32768
  nseq-max: 2
```

Do not add a `models:` wrapper. Top-level setting names use kebab-case, such as
`context-window` and `nseq-max`. Keys nested under `sampling-parameters` use
the API's snake_case names, such as `top_p`.

The server reads this file during startup. Restart the server after changing
it. To test a different file without replacing the default, run:

```shell
kronk server start --model-config-file=./my-model-config.yaml
```

You can also set `KRONK_POOL_MODEL_CONFIG_FILE` to an alternative path. See
[Chapter 8 §8.5](chapter-08-model-server.md#85-model-configuration-files) for
model config file management and
[Chapter 2 §2.5](chapter-02-installation.md#25-models-and-data-paths) for all
data paths.

#### Model variants

A suffix creates another configuration for the same downloaded model:

```yaml
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
Request fields such as `temperature`, `top_p`, and `max_tokens` can override
generation behavior for an individual request. Those request fields are
documented in [Chapter 10](chapter-10-request-parameters.md).

The hardware processor (`cpu`, `metal`, `cuda`, `rocm`, or `vulkan`) selects a
native library bundle rather than a per-model setting. Kronk detects it during
library installation. Set `KRONK_PROCESSOR` before installing libraries only
when you need to override detection; see
[Chapter 2 §2.4](chapter-02-installation.md#24-libraries).

### 3.2 Automatic Tuning

The model server derives a starting configuration from GGUF metadata and the
available hardware. This analysis chooses values such as:

- context window;
- KV cache types;
- maximum parallel sequences;
- GPU layer placement;
- Flash Attention mode; and
- multi-GPU split mode.

A concrete override in `model_config.yaml` replaces the analyzed value. The
special cache type `auto` is treated as unset and therefore does not clear an
analyzed `f16` or `q8_0` choice. This makes the usual workflow:

1. Start with no override and let Kronk analyze the model.
2. Use the model normally and monitor memory and latency.
3. Override only the setting needed for the workload.

The balanced analysis limits the selected context to the model's training
context and a maximum of 128K tokens. It estimates the largest supported
context bucket that fits its GPU budget, with a 4K minimum recommendation. It
starts with an f16 KV cache and tries q8_0 if the minimum f16 configuration
does not fit. CPU-only analysis and systems without a known GPU budget cannot
perform the same fit check. All recommendations are estimates, not a guarantee
that every backend and workload will fit or have identical memory use.

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
the model may require RoPE scaling; see [Chapter 7](chapter-07-yarn-extended-context.md).

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

When unset, llama.cpp currently uses a full-size SWA cache. Explicitly setting
`false` uses the compact sliding-window cache, which can save memory but limits
context caching and shifting. Setting `true` preserves the full cache at a
higher memory cost. This key has no effect on models without SWA metadata.

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
| `row` | Split tensor rows across GPUs |

When the setting is omitted, Kronk selects `row` when more than one GPU is
present and `layer` otherwise. This is a hardware-derived default, not a rule
that one mode is always fastest for a particular model architecture.

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
sequence state is isolated, while the text engine uses a unified KV pool with
total capacity based on `context-window × nseq-max`. Idle slots do not own
permanent fixed partitions, but increasing `nseq-max` still increases the
capacity Kronk must budget and can substantially increase memory use.

Embedding and reranking models use `nseq-max` to size a pool of independent
contexts rather than text-generation slots. See
[Chapter 4](chapter-04-batch-processing.md) for request scheduling and the
differences between model types.

Two settings control prompt batching:

| Key | Load-time default | Purpose |
| --- | ----------------- | ------- |
| `nubatch` | `2048`; `4096` with MoE expert CPU offload | Physical compute chunk size |
| `nbatch` | `nubatch × nseq-max` | Maximum logical decode batch |

Most deployments should leave both unset. Larger values can improve prompt
throughput but require larger compute buffers. `nubatch` must not exceed
`nbatch`. Multimodal encoders may require an entire media token chunk to fit in
one `nubatch`, so do not lower it for a multimodal model without testing media
input.

Incremental Message Caching is configured separately with
`incremental-cache` and related cache settings. See
[Chapter 5](chapter-05-message-caching.md) rather than treating cached
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

Do not use model names or benchmark results as universal draft-selection rules.
Measure acceptance and throughput on the actual workload. See
[Chapter 6](chapter-06-speculative-decoding-mtp.md) for drafter selection,
adaptive throttling, observability, and limitations.

#### Extended context with YaRN

Do not add RoPE scaling merely because a large `context-window` fits in memory.
Scaling must match the model and its native training context. Configuration
uses `rope-scaling-type` and the `yarn-*` keys described in
[Chapter 7](chapter-07-yarn-extended-context.md).

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
[Chapter 10](chapter-10-request-parameters.md) for behavior and the full
parameter reference.

### 3.8 Complete Example and Key Reference

This example shows the file structure and naming conventions. It is not a
recommendation that every model needs these overrides:

```yaml
# ~/.kronk/models/model_config.yaml

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

Common top-level keys are summarized below. An omitted hardware-related value
is normally supplied by analysis or by the load-time defaults.

| Key | Values | Purpose |
| --- | ------ | ------- |
| `context-window` | Positive token count | Per-sequence context capacity |
| `cache-type-k`, `cache-type-v` | `f16`, `q8_0`, `q4_0`, and supported GGML types | KV-cache precision |
| `flash-attention` | `enabled`, `disabled`, `auto` | Attention implementation mode |
| `nseq-max` | Positive integer | Parallel sequences or context-pool size |
| `nubatch`, `nbatch` | Positive token counts | Physical and logical batch sizes |
| `ngpu-layers` | `-1`, `0`, or a positive count | CPU/GPU layer placement |
| `offload-kqv` | Boolean | Place KV cache on GPU when true |
| `op-offload` | Boolean | Place host tensor operations on GPU when true |
| `proj-on-cpu` | Boolean | Keep multimodal projector on CPU |
| `devices` | Device-name list | Devices available to the model |
| `split-mode` | `none`, `layer`, `row` | Multi-GPU distribution mode |
| `main-gpu` | Device index | Primary device in single-GPU mode |
| `tensor-split` | Numeric share list | Proportional multi-GPU placement |
| `swa-full` | Boolean | Full or compact SWA cache |
| `incremental-cache` | Boolean | Incremental Message Cache |
| `draft-model` | Mapping | Separate drafter or MTP draft-count override |
| `rope-scaling-type` | Supported scaling mode | Extended-context scaling |
| `sampling-parameters` | Mapping | Per-model generation defaults |
| `template` | File path | Override the model's chat template |

Prefer the automatic values until a measured workload gives you a reason to
override them. Change one setting at a time so memory, quality, and throughput
effects remain attributable.

---
