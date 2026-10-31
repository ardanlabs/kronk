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

Kronk creates this file on first use. Version 2 stores language-model overrides
under `models`, Bucky overrides under `bucky-models`, and Malina overrides under
`malina-models`. Version 1 files containing only `kms` and `models` remain
supported. Language-model entries use the canonical ID shown by
`kronk model list` or the `/v1/models` endpoint:

```yaml
version: 2
models:
  unsloth/Qwen3-0.6B-Q8_0:
    context-window: 32768
    nseq-max: 2
    admission-timeout: 3m
    queue-depth: 2
    imc-session-capacity: 8

bucky-models:
  tiny:
    nseq-max: 2
    queue-depth: 4

malina-models:
  sd-1.5:
    concurrency: 2
    queue-depth: 2
```

Files must declare `version: 1` or `version: 2`; missing or unsupported versions
prevent server startup. Migrate legacy files with top-level model IDs into the
`models:` mapping. The remaining examples in this chapter show individual entries
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
providers. Bucky entries use the short ID shown by `kronk bucky model list`,
and Malina entries use the bundle ID shown by `kronk malina model list`.

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

Applications embedding the SDK can configure model loading directly without
using the server's YAML file.
Request fields such as `temperature`, `top_p`, `seed`, and `max_tokens` can override
generation behavior for an individual request. A configured seed supplies a
repeatable sampling default; an explicit request seed takes precedence. Without
either, Kronk generates a random seed, which a matched IMC session can retain
across turns. A fixed seed does not guarantee identical output across different
backends or execution conditions.
Those request fields are documented in
[Chapter 10](https://www.kronkai.com/manual#chapter-10-request-parameters).

The hardware processor (such as `cpu`, `metal`, `cuda`, `rocm`, or `vulkan`)
selects a native library bundle rather than a per-model setting. Kronk normally
detects it automatically. To override detection, apply `KRONK_PROCESSOR` to both
library installation and server startup; a one-command setting does not persist.
See
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

For normal server model loads, Kronk resolves the template in this order:

1. The explicit `template` path in `model_config.yaml`.
2. A matching Jinja file under the data root's `jinja` directory, first matching
   the model name including quantization, then the name without its quantization
   suffix.
3. The `tokenizer.chat_template` embedded in GGUF metadata.

Relative `template` paths resolve under the Kronk data root, not beside the
configuration file. An unreadable or empty explicit template prevents model
loading. A custom in-memory template supplied by the Playground or an embedded
application takes precedence over these file-based choices.

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
context. The usual minimum candidate is 4K, but a smaller training context is
not expanded to 4K. Fit checks use both VRAM and host RAM where applicable; on
unified-memory systems, they use the combined footprint. CPU-only analysis can
check RAM when a budget is known. Without a known budget, the corresponding
memory limit cannot be checked. All
recommendations are estimates, not a guarantee that every backend and workload
will fit or have identical memory use.

Automatic tuning estimates a configuration; it does not guarantee a successful
load. A configuration that cannot fit the server's memory budget can be rejected
before loading. AutoTune does not automatically turn an oversized full-GPU
configuration into a CPU/GPU split: configure partial layer or expert placement
explicitly when needed.

Embedded applications must opt into automatic tuning. Explicit application
settings constrain the recommendation in the same way as server overrides.

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

#### KV Cache Layout

`kv-cache-mode` selects how concurrent generation slots allocate KV storage:

```yaml
unsloth/Qwen3-0.6B-Q8_0:
  context-window: 8192
  nseq-max: 4
  kv-cache-mode: shared
```

The default, `partitioned`, gives each slot a dedicated context-window-sized
partition. `shared` uses one aggregate pool. Both request total context capacity
of `context-window × nseq-max`, isolate attention between requests, and keep
Kronk's per-request context limit unchanged. Shared mode does not reserve a
physical partition for each slot.

Shared mode can improve ordinary generation batching when active sequence IDs
have gaps. It can also increase attention workspace and long-history processing
costs. Benchmark your workload rather than assuming it is always faster. This is
a model-load setting, not a chat-request parameter; restart/reload with the new
configuration.

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

Quantized value caches such as `cache-type-v: q8_0` or `q4_0` require
`flash-attention: auto` or `enabled`; combining them with `disabled` fails
configuration validation. Backend and model support must still be verified.

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

#### MoE expert placement

For MoE models, `moe.mode` controls routed expert placement separately from
ordinary layer offload:

| Value | Behavior |
| ----- | -------- |
| `auto` | Experts follow normal layer placement; no extra CPU override |
| `experts_cpu` | Keep routed expert weights on the CPU while other weights follow layer placement |
| `experts_gpu` | No extra CPU override; experts follow layer placement, so full GPU offload is still needed to put all experts on GPU |
| `keep_top_n` | Keep routed experts eligible for GPU placement in the highest-numbered N layers; force the remaining expert layers to CPU |
| `custom` | Use explicit `tensor-buft-overrides` for placement |

For example, on a discrete-GPU system with enough host RAM, this keeps the
non-expert model weights eligible for GPU offload while placing routed experts
in host memory:

```yaml
some-provider/large-moe-model:
  ngpu-layers: 0
  moe:
    mode: experts_cpu
```

To retain some expert layers on GPU, use `mode: keep_top_n` with
`keep-experts-top-n: 4`, adjusting the count to fit the available VRAM. Here
“top” means the highest-numbered layers, not the most frequently selected
experts. An explicit `tensor-buft-overrides` list takes precedence over
`moe.mode`. These settings redistribute weights; they do not reduce model size
or create additional capacity on unified-memory systems. Measure throughput
because CPU expert evaluation can become the bottleneck. The optional GPU
expert cache is described in Section 3.6.

#### Model weight loading

`load-mode` controls how Kronk reads model weights. Its default is `auto`:

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

The former `use-mmap` and `use-direct-io` keys are no longer supported. Migrate
existing configuration as follows:

| Removed configuration | Replacement |
| --------------------- | ----------- |
| `use-mmap: true` | `load-mode: mmap` |
| `use-mmap: false` | `load-mode: none` |
| `use-direct-io: true` | `load-mode: direct-io` |

#### Lazy tensor reading

`lazy-mode` controls on-demand reading of tensors that llama.cpp marks eligible,
such as the per-layer token embedding tables in Qwen4Exp and Gemma4. It is not
whole-model lazy loading; models without eligible tensors are unaffected.

| Value | Behavior |
| ----- | -------- |
| `auto` | Lazy-read eligible tensors larger than 4 GiB when all selected devices support mmap; this is the default |
| `off` | Read eligible tensors up front like ordinary weights |
| `on` | Request lazy reading regardless of tensor size; if platform mmap is unavailable, load fully into RAM and log a warning |

```yaml
some-provider/some-model:
  lazy-mode: on
  load-mode: none
```

On Metal, mmap can bring the entire table into RAM through an overlapping GPU
mapping even when the log says `lazy read enabled`. Use `load-mode: none` to
avoid that overlap unless the table is isolated in its own shard. Eligible lazy
tensors remain file-mapped even with `load-mode: none`; the other weights use
ordinary loading. Accessed pages may remain cached, and disk reads can reduce
inference speed. Verify residency and performance with the actual model.

The setting applies to the target and any separate or companion draft model.
For eligible lazy tables, Kronk's memory estimates and admission reservations
use an estimated host-memory working set instead of the full table size.

Full-table accounting remains when lazy reading is off or uncertain, when
`auto` does not meet its size/device requirements, with mlock, or when Metal
mmap may overlap a table that is not alone in its shard. The current device
capability check verifies CPU and Metal; other backends conservatively retain
the full table in `auto` mode. Explicit `on` bypasses the size and device checks,
but cannot enable lazy reading on a platform without mmap support.
This is not a hard residency cap: clean mapped pages may accumulate or be
reclaimed by the operating system. Validate actual memory and latency under
representative prompts before relying on the additional admission headroom.

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

`GGML_OP_OFFLOAD_MIN_BATCH` sets the minimum batch size for offloading supported
host operations on applicable GPU backends; the default is 32. It is
process-wide, not a per-model setting. Set it before server startup and restart
after changing it.

For multimodal models, `proj-on-cpu: true` keeps the media projector on the
CPU without changing placement of the language model itself. To place the
projector on a specific accelerator, set `proj-device` to a name reported by
`kronk devices` (for example, `CUDA1` or `MTL0`). Omit both settings for
automatic placement. `proj-device` cannot be combined with
`proj-on-cpu: true`.

#### Native backend environment settings

These optional llama.cpp settings are process-wide environment variables, not
per-model YAML fields. They affect the CUDA backend and applicable ROCm/HIP
paths. Leave them unset unless you need to override backend memory behavior.

| Variable | Effect when set to `1` |
| -------- | ---------------------- |
| `GGML_CUDA_ENABLE_UNIFIED_MEMORY` | Enable managed/unified memory allocation |
| `GGML_CUDA_NO_PINNED` | Disable pinned host-memory allocation |
| `GGML_CUDA_REGISTER_HOST` | Enable host-memory registration |

Use `1` to activate these three settings. An empty value, `0`, `true`, or `yes`
does not activate them. For example:

```shell
GGML_CUDA_ENABLE_UNIFIED_MEMORY=1 kronk server start
```

Set these variables before backend initialization. Their values are cached
on first use, so restart the server after changing them. Do not apply this
numeric rule to every native setting: `GGML_CUDA_P2P` remains presence-based,
so even `GGML_CUDA_P2P=0` enables it; unset it to disable it.

#### Multiple GPUs

`split-mode` accepts:

| Value | Behavior |
| ----- | -------- |
| `none` | Use one GPU |
| `layer` | Distribute whole layers across GPUs |
| `row` | Use legacy row-split parallelism where supported |
| `tensor` | Use experimental tensor parallelism where supported |

When the setting is omitted, Kronk selects `layer`, matching llama.cpp's
default and most compatible multi-GPU mode. Layer mode can distribute a single
GGUF file across multiple GPUs. `row` remains available for legacy
configurations.

With AutoTune enabled, Kronk selects `tensor` when exactly two selected devices
use the ROCm backend, the model architecture supports llama.cpp tensor
parallelism, Flash Attention is not disabled, and GPU offload is enabled. This
allows llama.cpp to use RCCL when available or its two-GPU HIP AllReduce path
otherwise. An explicit `split-mode`, disabled Flash Attention, CPU-only
placement, a different backend, or any other GPU count keeps the normal
selection behavior. Tensor mode is experimental and requires Flash Attention;
set `split-mode: layer` to opt out.

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

For text generation, this allows up to four active generation requests.
Requests remain isolated, and each has the configured `context-window` limit.
Kronk budgets aggregate context capacity of `context-window × nseq-max`;
`kv-cache-mode` determines whether KV storage is partitioned or shared.
Increasing concurrency can substantially increase memory use.

For embedding and reranking models, `nseq-max` limits the number of complete
inputs or query-document pairs evaluated together. Their combined token count
is also limited by `prefill-batch-size`. See
[Chapter 4](https://www.kronkai.com/manual#chapter-4-batch-processing) for request scheduling and the
differences between model types.

Text scheduling, image capacity, and image preprocessing are configured separately:

| Key | Load-time default | Purpose |
| --- | ----------------- | ------- |
| `prefill-batch-size` | `2048` | Generation prefill contribution per decode iteration, or aggregate embedding/reranking tokens per native sequence batch |
| `physical-batch-size` | Unset | Requested physical compute capacity for generation, independent of the prompt contribution; constrained by image floors, logical capacity, and MTP policy |
| `image-batch-token-capacity` | `0` (no additional floor) | Independent native capacity floor for image chunks; does not change text scheduling or image detail |
| `image-token-budget` | Unset (projector default) | Explicit preprocessing budget for supported projectors; lowering it may reduce image resolution and detail |

For generation models, a larger `prefill-batch-size` can move a long prompt to generation in
fewer decode calls, but requires larger compute buffers and each call runs
longer before already-generating slots can run again. A smaller value gives
generating slots more frequent scheduling opportunities at the cost of more
prefill decode calls.

For embedding and reranking, `prefill-batch-size` limits the combined tokens
evaluated together, not the token capacity of each input.
The batch is constrained independently by `nseq-max` complete sequences and by
`prefill-batch-size` combined tokens. For example, `nseq-max: 8` with
`prefill-batch-size: 2048` can evaluate eight 256-token sequences, four
512-token sequences, two 1024-token sequences, or one 2048-token sequence in a
single batch. Kronk does not multiply the token capacity by `nseq-max`;
larger aggregate batches require more compute memory. Increase
`prefill-batch-size` explicitly when the expected embedding or reranking
workload needs a larger aggregate batch.

Most deployments should leave the prompt contribution at its default.

For ordinary causal generation, `physical-batch-size` lets llama.cpp divide one
logical submission into smaller compute batches without changing how many prompt
tokens Kronk schedules. For example, `prefill-batch-size: 2048`,
`physical-batch-size: 512`, and `nseq-max: 1` allow a prompt contribution to
be processed in smaller compute batches. This can enable layer-split multi-GPU
pipeline overlap within a submission.

Explicit positive physical sizes override AutoTune's recommendation. AutoTune
initially recommends 512 for ordinary text models using multiple selected GPUs
with layer splitting and full GPU offload. Single-GPU and other execution modes
retain the legacy default when the setting is omitted. Image capacity and budget
floors can raise a request, and logical/native context capacity can cap it.
Embedding, reranking, and specialized decision evaluation retain their own sizing policies. Requested
and effective capacities are reported separately. Non-causal media chunks still
must fit in one physical batch; oversized chunks return an error.

Kronk adds generation capacity automatically; users do not need to calculate an
allowance for active slots or draft tokens. With active MTP, Kronk keeps the
work in one physical batch, so `physical-batch-size` cannot divide that work
into smaller compute batches. MTP may therefore require more compute memory.
See [Chapter 6](https://www.kronkai.com/manual#chapter-6-speculative-decoding-and-mtp)
for MTP configuration and limitations.

Image capacity and an explicit image token budget can increase the required
compute capacity without increasing text prefill per iteration.

There are no model-family image-capacity defaults. Supply a non-negative
`image-batch-token-capacity` when complete non-causal image chunks need more
room than text prefill provides. For example, `prefill-batch-size: 512` and
`image-batch-token-capacity: 1120` reserve room for a complete 1120-token image
chunk while keeping text prefill at 512 tokens per iteration. This does not
change image preprocessing or detail.

Only an explicit positive `image-token-budget` changes the projector's
preprocessing budget; neither text prefill nor image capacity silently sets it.
Lowering this budget can reduce image resolution and detail. When omitted,
Kronk preserves the projector's default. Not all projectors support the option
(for example, Gemma 3's fixed-resolution path does not), and resizing alignment
can make the final chunk count differ from the budget. Very small budgets may
also conflict with a projector's minimum and fail initialization. An explicit budget also contributes
to native capacity, so a separate capacity setting is unnecessary unless more
room is needed. Increasing capacity does not undo a smaller preprocessing budget.

Non-causal media chunks must fit completely in one physical decode. Kronk
checks actual chunk sizes before native decoding in both cached and uncached
paths. An oversized chunk returns an error with the required capacity instead
of being split or aborting the process. Increase `image-batch-token-capacity`
and reload the model to process that chunk.

After loading, inspect the `batch-sizing` debug logs and Slots diagnostics to
compare requested settings with effective runtime capacities. Image requirements,
active MTP, and native context limits can change the effective physical capacity.

![How two requests move from prefill to generation and how non-MTP and MTP batch sizing supports them](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/08-wait-for-execution-slot.svg)

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

#### Independent dimensions of model design

Dense, MoE, and hybrid are not three mutually exclusive model types. Describe
a model using independent dimensions:

| Dimension | Choices | What it describes |
| --- | --- | --- |
| Parameter activation | Dense or Mixture-of-Experts (MoE) | Which feed-forward weights compute for a token |
| Sequence processing | Attention-only, recurrent/state-space-only, or hybrid | How information from earlier tokens is processed |
| Architecture | A specific overall design identifier, such as the GGUF `general.architecture` value | The model's concrete layout, operations, and conventions |

An architecture identifier is not another activation or sequence-processing
category. It identifies the overall design; inspect that design and its metadata
to determine both dimensions rather than treating its name as a third axis of
mutually exclusive types.

The catalog's existing **Type** column reports parameter activation: Dense or
MoE. New downloads also store `architecture` (the GGUF architecture identifier)
and `sequence_processing` (Attention-only, Recurrent, Hybrid, or Unknown) in
catalog metadata, without adding list columns. Unknown means the available
metadata does not establish the sequence-processing topology. The runtime uses
independent MoE and sequence-processing properties for memory planning. A
hybrid MoE model combines MoE parameter activation with hybrid sequence
processing; neither property replaces the other.

Dense and Mixture-of-Experts (MoE) describe how each layer's feed-forward
weights are activated. A dense layer uses its complete feed-forward network for
every token. An MoE layer stores many expert feed-forward networks and uses a
learned router to select a small subset independently for each token and layer.
All experts remain in memory even though only the selected experts compute.

**Experimental GPU expert cache:** `moe.cache-size` reserves additional GPU
storage for frequently used experts whose original weights remain in host
memory. The value is bytes per native context; it is one global budget shared
across eligible GPUs, not a budget per GPU or per sequence slot. Unset or zero
leaves the llama.cpp default unchanged (currently disabled). Non-MoE models
ignore this setting.

```yaml
my-moe-model:
  moe:
    mode: experts_cpu
    cache-size: 1073741824 # 1 GiB of additional GPU expert-cache capacity
```

The cache primarily targets small-batch generation (up to 32 tokens per
microbatch when capacity permits). Multiple GPUs are supported; tensor
parallelism (`split-mode: tensor`) is not. Native pipeline parallelism is
disabled when caching is requested. A cache too small for the selected experts
can fail context initialization. Models without eligible host-resident experts
may run without an active cache. Benchmark before assuming a speedup.

All three BUI calculators (standalone, model details, and catalog details)
expose **MoE Expert Cache (GiB)** for MoE models and export `cache-size` in
bytes. The calculator and pool reserve this budget in addition to model
weights, KV, and compute memory. On unified-memory systems it increases the
shared physical-memory footprint; original host weights are not deducted.
Per-GPU allocation is an estimate: actual cache placement follows eligible
layers and may use free-memory proportions. Retain backend/host-metadata
headroom beyond the cache budget.

For sequence processing, **attention-only** models use attention to access prior
tokens (full, sliding-window, or sparse attention). **Recurrent/state-space-only**
models carry information forward in recurrent state without attention layers;
they are not automatically hybrid. Here, **hybrid** means a stack combining
attention with recurrent/state-space mechanisms. Mixing full and sliding-window
attention alone remains attention-only in this classification.

Each sequence-processing choice can pair with dense feed-forward networks or
MoE blocks; not every combination needs to be represented in Kronk's catalog.
Qwen 3.6 combines Gated DeltaNet and
full-attention layers with MoE; qwen4exp combines Gated DeltaNet and sparse
attention with MoE. Every token still traverses every layer. The hybrid stack
changes how context is processed, while an MoE router independently changes
which feed-forward experts activate.

![Independent model dimensions: Dense and MoE activation crossed with attention-only, recurrent/state-space-only, and hybrid sequence processing](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/03c-dense-moe-and-hybrid-inference.svg)

The estimator follows the model's per-layer attention topology when the GGUF
provides it. Full-attention and sliding-window layers can have different KV
head counts and dimensions; compact SWA includes its physical-batch headroom,
while `swa-full: true` allocates SWA against the full context. Shared KV layers
are not charged twice.

Hybrid recurrent models use both attention-cache memory and per-sequence
recurrent state. Speculative decoding can add state copies, additional context
memory, and drafter weights. Use the calculator for the intended speculative
configuration; an ordinary transformer KV-cache formula can substantially
misestimate these models.

If a configuration does not fit, consider these changes one at a time:

1. Reduce `context-window`.
2. Reduce `nseq-max`.
3. Let automatic tuning use q8_0, or explicitly compare a quantized KV cache.
4. On discrete-GPU systems with sufficient host RAM, move the KV cache, model
   layers, or MoE experts to CPU to reduce VRAM pressure.
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
model. `KRONK_BASE_PATH` or `--base-path` selects the data root containing the
`lora` directory; it does not move existing adapter files.

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

`auto` uses a configured separate drafter first; otherwise it attempts supported
MTP and can fall back to target-only generation. `disabled` runs target-only and
does not load draft resources. `classic` requires `draft-model.model-id`.
`mtp` requires a supported, compatible embedded head or companion assistant and
a native library that provides MTP; explicitly requiring unavailable MTP fails
model loading instead of silently disabling it. Do not combine `mtp` with a
separate `draft-model.model-id`.

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

version: 2
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
| `kv-cache-mode` | `partitioned` (default), `shared` | Dedicated slot partitions or shared aggregate KV capacity |
| `flash-attention` | `enabled`, `disabled`, `auto` | Attention implementation mode |
| `nseq-max` | Positive integer | Active generation requests or embedding/reranking batch width |
| `admission-timeout` | Go duration, default `3m` | Maximum SDK admission-permit wait; separate from the server's `KRONK_WEB_INFERENCE_TIMEOUT` (default `60m`) |
| `queue-depth` | Non-negative integer, default `2` | Admitted request layers: `1` allows slot occupants only; `2` adds one waiting layer |
| `imc-session-capacity` | Positive integer; derived when omitted | Maximum reusable IMC conversation identities; built-in storage is RAM |
| `prefill-batch-size` | Positive token count, default `2048` | Generation prefill contribution per decode iteration, or aggregate embedding/reranking tokens per native sequence batch |
| `physical-batch-size` | Positive token count up to `2147483647`; unset by default | Requested generation compute batch size; AutoTune can choose it, and correctness constraints determine the effective capacity |
| `image-batch-token-capacity` | Non-negative token count, default `0` | Image-capacity floor for native generation batches; does not change image detail |
| `image-token-budget` | Positive token count up to `2147483647`; unset by default | Explicit projector preprocessing budget; may reduce image detail; also contributes to native capacity |
| `ngpu-layers` | `-1`, `0`, or a positive count | CPU/GPU layer placement |
| `moe.mode` | `auto`, `experts_cpu`, `experts_gpu`, `keep_top_n`, `custom` | Routed-expert placement; explicit tensor overrides take precedence |
| `moe.keep-experts-top-n` | Non-negative layer count | Required for `keep_top_n`; highest-numbered expert layers eligible for GPU placement |
| `moe.cache-size` | Non-negative byte count | Additional experimental GPU expert-cache budget |
| `tensor-buft-overrides` | CPU-placement patterns | Advanced tensor placement overriding `moe.mode` |
| `lazy-mode` | `auto`, `off`, `on` | On-demand reading of eligible tensors; default `auto` |
| `load-mode` | `auto`, `mmap`, `none`, `mlock`, `mmap+mlock`, `direct-io` | Model weight loading strategy |
| `offload-kqv` | Boolean | Place KV cache on GPU when true |
| `op-offload` | Boolean | Place host tensor operations on GPU when true |
| `proj-on-cpu` | Boolean | Keep multimodal projector on CPU |
| `proj-device` | Device name | Place multimodal projector on a specific accelerator |
| `devices` | Device-name list | Devices available to the model |
| `split-mode` | `none`, `layer`, `row`, `tensor` | Multi-GPU distribution mode |
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
