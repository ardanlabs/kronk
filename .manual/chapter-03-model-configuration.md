# Chapter 3: Model Configuration

## Table of Contents

- [3.1 Basic Configuration](#31-basic-configuration)
- [3.2 GPU Configuration](#32-gpu-configuration)
- [3.3 KV Cache Quantization](#33-kv-cache-quantization)
- [3.4 Flash Attention](#34-flash-attention)
- [3.5 Parallel Inference (NSeqMax)](#35-parallel-inference-nseqmax)
- [3.6 Understanding GGUF Quantization](#36-understanding-gguf-quantization)
  - [What is Quantization?](#what-is-quantization)
  - [What are K-Quants?](#what-are-k-quants)
  - [Standard Quantization Formats](#standard-quantization-formats)
  - [IQ (Importance Matrix) Quantization](#iq-importance-matrix-quantization)
  - [UD (Ultra-Dynamic) Quantization](#ud-ultra-dynamic-quantization)
  - [Choosing the Right Quantization](#choosing-the-right-quantization)
- [3.7 VRAM Estimation](#37-vram-estimation)
  - [Slots and Sequences](#slots-and-sequences)
  - [What Affects KV Cache Memory Per Sequence](#what-affects-kv-cache-memory-per-sequence)
  - [What Affects Total KV Cache (Slot Memory)](#what-affects-total-kv-cache-slot-memory)
  - [Caching Modes (SPC / IMC)](#caching-modes-spc-imc)
  - [Example: Real Model Calculation](#example-real-model-calculation)
- [3.8 Model-Specific Tuning](#38-model-specific-tuning)
- [3.9 Speculative Decoding](#39-speculative-decoding)
- [3.10 Sampling Parameters](#310-sampling-parameters)
- [3.11 Model Config File Example](#311-model-config-file-example)

---

Model configuration controls how Kronk configures models to run inference.
Configuration can be set via model config files, catalog templates, or
programmatically through the SDK.

### 3.1 Basic Configuration

For most models you will want to touch these basic settings. There are many more
which will be presented later. Each model has GGUF metadata that Kronk can read
for defaults like setting the context window size when not provided. Kronk also
has default settings for things like temperature and top_p when not provided.

**Context Window**

The context window defines the maximum number of tokens the model can process
in a single request. This would be the sum of all input tokens being provided
at any given time.

```yaml
context_window: 8192 # 8192 tokens (Kronk default if not specifed by the model and you)
```

_Note: A common rule of thumb is that 1 token ≈ 0.75 words (or roughly 4
characters in English). So an 8K context window can handle approximately 6,000
words of combined input and output._

Larger context windows require more VRAM. A rough estimate:

- `8K context`: ~2GB additional VRAM
- `32K context`: ~8GB additional VRAM
- `128K context`: ~32GB additional VRAM (requires YaRN scaling)

_Note: YaRN is a way to extend the natural size of context windows for
small models. Kronk supports YaRN and talked about in Chapter 6._

**Batch Size Configuration**

When you send a prompt to a model, the model doesn't process all your input
tokens at once. It breaks them into smaller chunks and processes each chunk
through the GPU in a series of steps called forward passes. These two
parameters control the size of those chunks:

- `n_batch` - Maximum tokens in a single forward pass (kronk default: 2048)
- `n_ubatch` - Physical batch size for prompt processing (kronk default: 512)

Think of it like reading a book aloud. `n_batch` is how many words you're
willing to look at on the page at once, and `n_ubatch` is how many words you
actually read in one breath. You might glance at 2048 words, but you read
them 512 at a time.

For example, if you send a 4096-token prompt with the default settings, the
model will process it in chunks: it takes up to 2048 tokens per forward pass
(`n_batch`), and within each pass, it physically processes 512 tokens at a
time (`n_ubatch`). Larger values mean faster prompt processing but use more
VRAM. The `n_ubatch` value must always be less than or equal to `n_batch`.

```yaml
n_batch: 2048 # Logical batch size
n_ubatch: 512 # Physical batch size (must be ≤ n_batch)
```

**Recommended settings by workload:**

| Workload                           | n_batch   | n_ubatch |
| ---------------------------------- | --------- | -------- |
| Interactive chat (single user)     | 512-1024  | 512      |
| Long prompts/RAG                   | 2048-4096 | 512-1024 |
| Batch inference (multiple prompts) | 2048-4096 | 512      |
| Low VRAM (<8GB)                    | 512       | 256-512  |
| High VRAM (24GB+)                  | 4096+     | 1024+    |

### 3.2 GPU Configuration

A model is made up of layers, and each layer contains the weights (numbers)
the model learned during training. When you run inference, the model processes
your input through these layers one at a time. The key performance question is:
where do those layers live — on the GPU or the CPU?

GPUs are dramatically faster at the math required for inference, but they have
limited memory (VRAM). If your model doesn't fit entirely in VRAM, you can
split the work: keep some layers on the GPU for speed and let the rest run on
the CPU. This section covers how to control that split and other GPU-related
settings.

**Layer Offloading**

A typical model might have anywhere from 28 to 80+ layers depending on its
size. For example, a 7B parameter model usually has around 32 layers, while a
70B model might have 80. Each layer you place on the GPU runs significantly
faster, but consumes VRAM. If your GPU doesn't have enough VRAM to hold every
layer, you can choose how many to offload — the rest will run on the CPU,
which is slower but has access to your full system RAM.

The goal is to put as many layers on the GPU as your VRAM allows. If you run
out of VRAM, lower this number until the model fits.

Control how many model layers run on GPU:

```yaml
n_gpu_layers: 0      # 0 = all layers on GPU (default)
n_gpu_layers: -1     # All layers on CPU
n_gpu_layers: 20     # First 20 layers on GPU
```

**KV Cache Location**

As the model processes your conversation, it builds up a cache of intermediate
calculations called the KV (Key-Value) cache. Think of it as the model's
short-term memory — it stores what the model has already "read" so it doesn't
have to reprocess the entire conversation for every new token it generates.
The longer the conversation, the larger this cache grows.

By default the KV cache lives on the GPU for speed, but it can consume a
significant amount of VRAM — especially with large context windows or multiple
concurrent requests. If you're running low on VRAM, moving the KV cache to
the CPU frees up GPU memory at the cost of slower inference.

Control where the KV cache is stored:

```yaml
offload_kqv: true    # KV cache on GPU (default, faster)
offload_kqv: false   # KV cache on CPU (saves VRAM, slower)
```

**Tensor Operations Offload**

Beyond the model layers and KV cache, there are additional math operations
(called tensor operations) that happen during inference — things like
matrix multiplications and attention score calculations. These operations
are separate from the layer weights themselves and can independently be
placed on the GPU or CPU. By default they run on the GPU, but if VRAM is
tight you can move them to the CPU while still keeping your model layers
on the GPU.

_Note: Use `op_offload: false` when you need to run the model on CPU but want to
keep some layers on GPU for memory._

Control where these tensor computations run:

```yaml
op_offload: true     # Tensor ops on GPU (default)
op_offload: false    # Tensor ops on CPU
```

**Multi-GPU Split Mode**

If you have more than one GPU in your system, you can spread a model across
them. This is useful when a model is too large to fit in a single GPU's VRAM.
There are two strategies: `layer` mode assigns entire layers to different GPUs
(simple and works well for most models), while `row` mode splits individual
tensor operations across GPUs in parallel (better for Mixture of Experts
models like Qwen3-MoE, Mixtral, or DeepSeek where different "experts" can
run simultaneously on different GPUs).

_Note: Use `row` for Mixture of Experts models like Qwen3-MoE, Mixtral, or
DeepSeek._

Control how the model is distributed across GPUs:

```yaml
split_mode: none     # Single GPU (default)
split_mode: layer    # Split layers across GPUs
split_mode: row      # Tensor parallelism (best for MoE models)
```

**Configuration Reference**

| Field      | YAML Key       | Values         | Default | Description                    |
| ---------- | -------------- | -------------- | ------- | ------------------------------ |
| NGpuLayers | `n_gpu_layers` | 0, -1, N       | 0       | Layers on GPU (0=all, -1=none) |
| OffloadKQV | `offload_kqv`  | true/false     | true    | KV cache on GPU                |
| OpOffload  | `op_offload`   | true/false     | true    | Tensor ops on GPU              |
| SplitMode  | `split_mode`   | none/layer/row | none    | Multi-GPU distribution         |

### 3.3 KV Cache Quantization

As discussed in the previous section, the KV cache is the model's short-term
memory of your conversation. By default it stores values in half precision
(f16), which gives the best accuracy but uses the most VRAM. Quantization
reduces the precision of those stored values — using fewer bits to represent
each number. It's a trade-off: you lose a small amount of accuracy in
exchange for meaningful VRAM savings. For most use cases, `q8_0` (8-bit)
gives nearly identical output quality while cutting KV cache memory by about
25%. More aggressive options like `q4_0` save even more but can start to
affect generation quality.

Control the precision of the key and value caches independently:

```yaml
cache_type_k: q8_0 # Key cache precision
cache_type_v: q8_0 # Value cache precision
```

**Available types:**

- `f16` - Half precision (default, best quality)
- `q8_0` - 8-bit quantization (good balance)
- `q4_0` - 4-bit quantization (aggressive, may affect quality)
- `bf16` - Brain float 16 (for supported hardware)

**VRAM savings with Q8_0 cache:**

- 8K context: ~25% reduction
- 32K context: ~25% reduction
- Larger contexts benefit proportionally

**When to Use F16 Cache (No Quantization):**

Certain model architectures are sensitive to KV cache quantization and
perform significantly better with `f16` precision:

- **Mixture of Experts (MoE) models** - Models like Qwen3-MoE, DeepSeek-MoE,
  and Mixtral use sparse expert routing. The routing decisions depend on
  subtle attention patterns that degrade when the KV cache is quantized.

- **Long-context reasoning** - Tasks requiring attention across many thousands
  of tokens (legal documents, codebases, multi-turn conversations) accumulate
  small precision errors that compound over the sequence length.

- **Code generation** - Precise variable tracking and syntax coherence benefit
  from higher cache precision, especially in larger codebases.

- **Math and logic** - Multi-step reasoning chains are sensitive to accumulated
  quantization noise in earlier attention states.

**Example: MoE Model with F16 Cache**

```yaml
models:
  # MoE models benefit from f16 cache for routing accuracy
  Qwen_Qwen3.5-35B-A3B-Q8_0:
    context_window: 32768
    cache_type_k: f16 # Preserve routing precision
    cache_type_v: f16
    split_mode: row # Best for MoE multi-GPU

  # Dense models can often use q8_0 cache without issues
  Qwen3-8B-Q8_0:
    context_window: 32768
    cache_type_k: q8_0
    cache_type_v: q8_0
```

**Recommendation:** If you notice quality degradation (incoherent outputs,
reasoning failures, or code bugs) with quantized cache, try `f16` first
before adjusting other parameters. The VRAM cost is typically 25-50% more
for the cache, but the quality improvement for sensitive workloads is
substantial.

### 3.4 Flash Attention

Attention is the core mechanism that lets a model figure out which parts of
your input are relevant to each other. For example, in the sentence "The cat
sat on the mat because it was tired," attention is how the model connects
"it" back to "the cat." The standard attention algorithm needs to hold a
large matrix of scores in memory — one score for every pair of tokens in your
input. As context windows grow, this matrix grows quadratically and can
become both slow and memory-hungry.

Flash Attention is an optimized implementation that computes the same result
but processes the matrix in small tiles that fit in the GPU's fast on-chip
memory (SRAM) instead of slower VRAM. The result is lower memory usage and
faster computation — especially noticeable with large context windows (32K+).
It's enabled by default and should rarely need to be changed.

Control whether Flash Attention is used:

```yaml
flash_attention: enabled   # Default: enabled
flash_attention: disabled  # Disable if causing issues
flash_attention: auto      # Let llama.cpp decide
```

### 3.5 Parallel Inference (NSeqMax)

When multiple users (or applications) send requests to the same model at the
same time, the model needs a way to handle them concurrently. That's what
`NSeqMax` controls — it determines how many requests the model can process in
parallel.

Behind the scenes, Kronk creates a processing slot for each concurrent
request. Each slot gets its own isolated partition in the KV cache (the
model's short-term memory from earlier sections). All slots share the same
model weights and GPU, but each one maintains its own conversation state
independently. When a batch decode runs on the GPU, tokens from all active
slots are combined into a single operation — so the GPU does one large matrix
multiply instead of several small ones. This is what makes parallel inference
efficient.

The trade-off is VRAM. Each slot reserves its full KV cache partition when the
model loads, whether or not it's actively handling a request. Setting
`n_seq_max: 4` means four KV cache partitions are allocated upfront. If each
partition costs 3 GB, that's 12 GB of VRAM just for the cache — on top of the
model weights. More slots means more concurrency but more VRAM.

Control how many requests can be processed in parallel:

```yaml
n_seq_max: 4 # Process up to 4 requests concurrently
```

**How Caching Strategy Affects Slot Behavior**

All three caching strategies allocate the same number of slots with the same VRAM
cost. The difference is what happens to the cached data in each slot between
requests:

**No Caching** — The simplest mode. When a request finishes, the slot's KV
cache is cleared. The next request that lands in that slot starts from
scratch, processing the full prompt from the beginning. Every request pays
the full cost of prompt processing regardless of how similar it is to a
previous one.

**SPC (System Prompt Cache)** — In many applications, every request starts with
the same system prompt (the instructions that tell the model how to behave).
SPC decodes the system prompt once into a temporary sequence, then externalizes
the KV state to a byte buffer in RAM and frees the sequence. When a new request
arrives, the KV state is restored into the slot's working sequence via
StateSeqSetData before the rest of the prompt is processed. The slot is still
cleared between requests. No dedicated cache sequence is permanently occupied,
so SPC does not add any extra sequences to the VRAM allocation.

**IMC (Incremental Message Cache)** — Designed for multi-turn conversations. A
slot becomes dedicated to a conversation. The entire conversation history stays
in the slot's KV cache between requests. When the user sends a new message,
only the new tokens need to be processed — the model doesn't re-read the
entire conversation. This gives the best performance for chat applications,
but each active conversation permanently occupies a slot.

| Mode | Slot Lifetime            | Cache Strategy                                                                               |
| ---- | ------------------------ | -------------------------------------------------------------------------------------------- |
| Off  | Cleared after request    | None                                                                                         |
| SPC  | Cleared after request    | System prompt decoded once, KV state stored in RAM, restored per request via StateSeqSetData |
| IMC  | Persists across requests | Full conversation cached in the slot's KV cache sequence                                     |

**Embedding and Reranking Models**

Embedding and reranking models work differently. Instead of slots sharing a
single context, `NSeqMax` creates a pool of independent contexts. When a
request contains multiple inputs (for example, 100 sentences to embed), those
inputs are spread across the pool contexts and processed in parallel. Model
weights are shared, but each context has its own KV cache memory.

### 3.6 Understanding GGUF Quantization

GGUF models come in various quantization formats that trade off between file
size, VRAM usage, and output quality. Understanding these formats helps you
choose the right model variant for your hardware and use case.

#### What is Quantization?

Quantization reduces model precision from the original 16-bit or 32-bit
floating-point weights to lower bit representations. This dramatically
decreases:

- **File size** - A 7B model goes from ~14GB (FP16) to ~3GB (Q4)
- **VRAM usage** - More aggressive quantization allows larger models on limited hardware
- **Inference speed** - Smaller models load faster and may run faster on memory-constrained systems

The tradeoff is **quality degradation** - lower precision means less accurate
representations of the original weights, which can affect output coherence,
reasoning ability, and factual accuracy.

#### What are K-Quants?

K-quants (introduced by llama.cpp) use **per-block scaling** with importance
weighting. Instead of applying uniform quantization across all weights, K-quants:

1. Divide weights into small blocks (typically 32 or 256 values)
2. Calculate optimal scale factors per block
3. Preserve more precision for important weights

This produces better quality than naive quantization at the same bit rate.
K-quant variants include size suffixes:

- **S** (Small) - Smallest file size, lowest quality within that bit level
- **M** (Medium) - Balanced size and quality
- **L** (Large) - Larger file, better quality

#### Standard Quantization Formats

| Format     | Bits/Weight | Quality     | VRAM (7B Model) | Use Case                                     |
| ---------- | ----------- | ----------- | --------------- | -------------------------------------------- |
| **Q4_0**   | 4.5         | Low         | ~4 GB           | Maximum compression, quality loss noticeable |
| **Q4_1**   | 5.0         | Low-Med     | ~4.3 GB         | Slightly better than Q4_0                    |
| **Q4_K_S** | 4.5         | Medium      | ~4 GB           | K-quant, good balance for limited VRAM       |
| **Q4_K_M** | 4.8         | Medium      | ~4.5 GB         | K-quant, recommended 4-bit option            |
| **Q5_K_S** | 5.5         | Medium-High | ~5 GB           | Good quality, moderate size                  |
| **Q5_K_M** | 5.7         | High        | ~5.3 GB         | Recommended for most users                   |
| **Q6_K**   | 6.5         | High        | ~6 GB           | Near-original quality                        |
| **Q8_0**   | 8.5         | Highest     | ~8 GB           | Best quality, largest size                   |

#### IQ (Importance Matrix) Quantization

IQ formats use **learned importance matrices** to determine which weights
matter most. They achieve extreme compression with minimal quality loss by:

1. Analyzing weight importance during quantization
2. Allocating more bits to critical weights
3. Aggressively compressing less important weights

| Format      | Bits/Weight | Quality     | Use Case                          |
| ----------- | ----------- | ----------- | --------------------------------- |
| **IQ1_S**   | ~1.5        | Very Low    | Extreme compression, experimental |
| **IQ1_M**   | ~1.75       | Low         | Extreme compression, experimental |
| **IQ2_XXS** | ~2.0        | Low         | Ultra-low VRAM situations         |
| **IQ2_XS**  | ~2.3        | Low-Med     | Very constrained hardware         |
| **IQ2_S**   | ~2.5        | Medium      | Constrained hardware              |
| **IQ3_XXS** | ~3.0        | Medium      | Good balance for low VRAM         |
| **IQ3_XS**  | ~3.3        | Medium-High | Better quality low-bit option     |
| **IQ4_XS**  | ~4.0        | High        | Alternative to Q4_K variants      |

#### UD (Ultra-Dynamic) Quantization

UD quantization applies **different precision levels per layer**. Neural
network layers have varying sensitivity to quantization:

- Early layers (embeddings, first attention blocks) - More sensitive
- Middle layers - Moderately sensitive
- Later layers - Often more tolerant of compression

UD variants analyze each layer and assign optimal bit depths, achieving
better quality than uniform quantization at similar average bits per weight.

Common UD naming: `UD-Q5_K_XL` means Ultra-Dynamic with Q5 K-quant base, XL quality tier.

#### Choosing the Right Quantization

**By Available VRAM:**

| VRAM   | 7B Model | 13B Model | 30B Model | 70B Model |
| ------ | -------- | --------- | --------- | --------- |
| 6 GB   | Q4_K_M   | IQ3_XXS   | -         | -         |
| 8 GB   | Q6_K     | Q4_K_M    | IQ2_XXS   | -         |
| 12 GB  | Q8_0     | Q5_K_M    | IQ3_XXS   | -         |
| 16 GB  | Q8_0     | Q8_0      | Q4_K_M    | -         |
| 24 GB  | Q8_0     | Q8_0      | Q6_K      | IQ3_XXS   |
| 48 GB  | Q8_0     | Q8_0      | Q8_0      | Q4_K_M    |
| 64 GB+ | Q8_0     | Q8_0      | Q8_0      | Q6_K/Q8_0 |

**By Use Case:**

- **Production/Quality-Critical**: Q8_0 or Q6_K - Minimal quality loss
- **General Use**: Q5_K_M - Best balance of quality and efficiency
- **VRAM-Constrained**: Q4_K_M - Good quality at low VRAM cost
- **Experimental/Testing**: IQ3_XXS or IQ2_XS - Run larger models on limited hardware

**Quality Guidelines:**

1. **Start with Q5_K_M** - It's the sweet spot for most use cases
2. **Use Q8_0 for reasoning-heavy tasks** - Math, code, complex logic benefit from higher precision
3. **Q4_K_M is the floor** - Below this, quality degrades noticeably for most models
4. **IQ formats are specialized** - Great for running models that wouldn't otherwise fit, but expect some quality loss
5. **Larger models at lower quant often beat smaller models at higher quant** - A 70B Q4 may outperform a 7B Q8

**Example Configuration:**

```yaml
models:
  # Quality-focused: Q8_0 for a model that fits in VRAM
  Qwen3-8B-Q8_0:
    context_window: 32768
    cache_type_k: q8_0
    cache_type_v: q8_0

  # VRAM-constrained: Q4_K_M to fit larger model
  Llama-3.3-70B-Instruct-Q4_K_M:
    context_window: 8192
    split_mode: row
    n_gpu_layers: 0
```

### 3.7 VRAM Estimation

Before loading a model, you need to know whether it will fit in your GPU's
memory. VRAM usage comes from two things: the model weights (fixed cost
determined by the model you chose) and the KV cache (variable cost determined
by your configuration choices from the previous sections — context window size,
number of slots, and cache precision). If the total exceeds your available
VRAM, the model either won't load or will partially fall back to the CPU,
which significantly slows inference. This section walks through how to
estimate the total.

**Total VRAM = Model Weights + KV Cache**

Model weights are determined by the GGUF file size (e.g., ~8GB for a 7B Q8_0
model). The KV cache is the variable cost you control through configuration.

**Model Weights (Q8_0 quantization)**

- 1-3B parameters: 2-4 GB
- 7-8B parameters: 8-10 GB
- 13B parameters: 14-16 GB
- 30B parameters: 32-36 GB
- 70B parameters: 72-80 GB

#### Slots and Sequences

A slot is a processing unit that handles one request at a time. Each slot is
assigned a unique sequence ID that maps to an isolated partition in the shared
KV cache. The mapping is always 1:1:

```
NSeqMax = 4 (set via n_seq_max in model config)

Slot 0  →  Sequence 0  →  KV cache partition 0
Slot 1  →  Sequence 1  →  KV cache partition 1
Slot 2  →  Sequence 2  →  KV cache partition 2
Slot 3  →  Sequence 3  →  KV cache partition 3
```

`NSeqMax` controls how many slots (and sequences) are created. More slots means
more concurrent requests, but each slot reserves its own KV cache partition in
VRAM whether or not it is actively used.

#### What Affects KV Cache Memory Per Sequence

Each sequence's KV cache partition size is determined by three factors:

1. **Context Window (`n_ctx`)** — The maximum number of tokens the sequence can
   hold. Larger context windows linearly increase memory. 32K context uses 4×
   the memory of 8K context.

2. **Number of Layers (`block_count`)** — Every transformer layer stores its own
   key and value tensors per token. More layers means more memory per token. A
   70B model with 80 layers uses ~2.5× more per-token memory than a 7B model
   with 32 layers.

3. **KV Cache Precision (`bytes_per_element`)** — The data type used to store
   cached keys and values:
   - `f16` = 2 bytes per element (default, best quality)
   - `q8_0` = 1 byte per element (50% VRAM savings, good quality)

   The head geometry (`head_count_kv`, `key_length`, `value_length`) is fixed by
   the model architecture and read from the GGUF header.

The formula:

```
KV_Per_Token_Per_Layer = head_count_kv × (key_length + value_length) × bytes_per_element
KV_Per_Sequence        = n_ctx × n_layers × KV_Per_Token_Per_Layer
```

#### What Affects Total KV Cache (Slot Memory)

Total KV cache (Slot Memory) is the per-sequence cost multiplied by the number
of slots:

```
Slot_Memory = NSeqMax × KV_Per_Sequence
Total_VRAM  = Model_Weights + Slot_Memory
```

Memory is statically allocated upfront when the model loads. All slots reserve
their full KV cache partition regardless of whether they are actively processing
a request.

#### Caching Modes (SPC / IMC)

Neither SPC nor IMC adds extra sequences to the VRAM calculation.

SPC (System Prompt Cache) externalizes the decoded system prompt KV state to a
byte buffer in RAM. On each request, the KV state is restored into the slot's
sequence via StateSeqSetData. No dedicated cache sequence is permanently
occupied.

IMC (Incremental Message Cache) uses dedicated slot/seq binding — each slot's
sequence IS the cache. No separate cache sequences.

| Mode | Slot Lifetime            | Cache Strategy                                                                               |
| ---- | ------------------------ | -------------------------------------------------------------------------------------------- |
| off  | Cleared after request    | None                                                                                         |
| SPC  | Cleared after request    | System prompt decoded once, KV state stored in RAM, restored per request via StateSeqSetData |
| IMC  | Persists across requests | Full conversation cached in the slot's KV cache sequence                                     |

#### Example: Real Model Calculation

```
Model                   : Qwen_Qwen3.5-35B-A3B-Q8_0
Model Weights           : 36.0 GB
Context Window (n_ctx)  : 131,072 (128K)
Bytes Per Element       : 1 (q8_0)
block_count (n_layers)  : 48
attention.head_count_kv : 4
attention.key_length    : 128
attention.value_length  : 128

Step 1 — Per-token-per-layer cost:

  KV_Per_Token_Per_Layer = 4 × (128 + 128) × 1 = 1,024 bytes

Step 2 — Per-sequence cost:

  KV_Per_Sequence = 131,072 × 48 × 1,024 = ~6.4 GB

Step 3 — Total KV cache (NSeqMax = 2):

  Slot_Memory = 2 × 6.4 GB = ~12.8 GB

Step 4 — Total VRAM:

  Total_VRAM = 36.0 GB + 12.8 GB = ~48.8 GB
```

### 3.8 Model-Specific Tuning

The previous sections covered general configuration that applies to all
models. However, different model architectures — vision, audio, Mixture of
Experts (MoE), and embedding models — each have their own characteristics
that benefit from specific tuning. A vision model processes images as large
batches of tokens, which needs different batch settings than a text-only chat
model. An MoE model routes tokens through specialized "expert" sub-networks,
which affects how you split work across GPUs. This section provides
recommended configurations for each model type so you can get the best
performance out of the box.

**Vision and Audio Models**

Vision models process image tiles as large token batches. Low `n_ubatch`
values cause multiple decode passes per image, significantly slowing
inference.

Keep `n_ubatch` high for efficient media token processing:

```yaml
models:
  Qwen2.5-VL-3B-Instruct-Q8_0:
    n_batch: 2048
    n_ubatch: 2048 # High for image/audio token batches
    n_seq_max: 2 # Process up to 2 requests concurrently
```

**Mixture of Experts (MoE) Models**

MoE models like `Qwen3-Coder-30B-A3B` have many total parameters but only
activate a small subset per token — the "A3B" means 30B total, 3B active.
This saves compute on GPU clusters but has important implications for
Apple Silicon and other memory-bandwidth-bound systems.

On Apple Silicon (unified memory), inference speed is determined by how many
**bytes must be read from memory per generated token**, not compute. MoE
models create **scattered memory access patterns** — the routing layer must
select which experts to activate, and those expert weights are spread across
memory. This scattered access underutilizes memory bandwidth compared to the
sequential access pattern of dense models.

A dense model at Q4 quantization may outperform a smaller-active MoE model
at Q8 quantization on Apple Silicon, even though the MoE activates fewer
parameters. The dense model reads weights sequentially (ideal for bandwidth
saturation) at 0.5 bytes per parameter, while the MoE reads scattered expert
weights at 1 byte per parameter. The total bytes moved per token can be
comparable — but the dense model's sequential pattern is more efficient.

MoE also tends to produce lower quality per-token than a dense model with the
same number of active parameters, because only a fraction of the model's
knowledge is engaged per token. A dense model at Q4 with all parameters
active has far more model capacity per token than an MoE with 3B active,
even at Q8 precision.

MoE models can be sensitive to aggressive KV cache quantization. If you
notice quality degradation, try `f16` cache types.

Use row-based tensor parallelism for multi-GPU setups:

```yaml
models:
  Qwen3-MoE-30B-A3B-Q8_0:
    split_mode: row # Best for MoE architecture
    cache_type_k: q8_0 # Be cautious with aggressive quantization
    cache_type_v: q8_0
```

**Hybrid Models (Attention + Recurrent)**

Hybrid models like Qwen3-Coder-Next mix traditional Attention layers with
recurrent layers (DeltaNet or SSM/Mamba). Unlike MoE models, hybrid models
are dense — every parameter participates in every token. Kronk detects hybrid
models automatically at load time.

Hybrid models have specific constraints:

- **KV cache must be f16** — quantized cache types (`q8_0`) are incompatible
  with the recurrent layers. This doubles KV cache memory compared to models
  that support `q8_0`.
- **Flash attention is disabled** — Kronk automatically disables flash
  attention for hybrid models.
- **IMC uses snapshot/restore** — see [IMC Hybrid](#imc-hybrid) for details
  on how caching works with recurrent state.

When choosing between a hybrid model and an MoE model for Apple Silicon,
consider: the hybrid model's sequential memory access pattern and dense
activation give it both better quality per token and better bandwidth
utilization. The trade-off is total model size — hybrid models use all
parameters, so you need enough unified memory to hold them plus the larger
f16 KV cache.

```yaml
models:
  Qwen3-Coder-Next-UD-Q4_K_XL:
    cache_type_k: f16 # Required for hybrid models
    cache_type_v: f16 # Required for hybrid models
    incremental_cache: true
```

**Embedding Models**

Embedding models process complete inputs in a single pass, so larger
`n_batch` values improve throughput.

Optimize batch size for your typical input lengths:

```yaml
models:
  embeddinggemma-300m-qat-Q8_0:
    n_batch: 8192 # Can equal context_window
    n_ubatch: 512 # Align with typical sliding window
    n_seq_max: 4 # 4 model instances for concurrency
```

### 3.9 Speculative Decoding

Speculative decoding uses a small, fast "draft" model to predict candidate
tokens, then verifies them against the full "target" model in a single forward
pass. When the draft model's predictions match the target's, multiple tokens
are accepted per decode step — improving throughput without changing output
quality. The output distribution is mathematically guaranteed to match the
target model exactly, regardless of draft quality (Leviathan et al., 2023).

**How It Works**

1. The draft model auto-regressively generates N candidate tokens (default 5)
2. All candidates plus the last accepted token are added to the target model's
   batch and decoded in one forward pass
3. Each candidate is verified using speculative sampling: accepted with
   probability `min(1, p_target / q_draft)`, where `p_target` is the target's
   probability and `q_draft` is the draft's probability for that token
4. On rejection, a corrective token is sampled from `max(0, p_target - q_draft)`
   normalized, and remaining candidates are discarded
5. If all candidates are accepted, a bonus token is sampled from the target

The speedup depends on the draft model's acceptance rate. Higher acceptance
means more tokens per forward pass. Acceptance rates depend on:

- **Draft model quality** — A larger, more capable draft produces better
  predictions. A Q4 quantization of the same architecture tends to outperform
  a much smaller model.
- **Temperature** — Lower temperatures (more deterministic) yield higher
  acceptance rates. At temperature 0.8, expect ~30% of steps to accept zero
  draft tokens.
- **Task type** — Predictable text (boilerplate, common patterns) accepts
  more often than creative or reasoning-heavy output.

**Requirements**

- Draft and target models must share the **same vocabulary** (same tokenizer)
- `n_seq_max` must be `1` (single-slot mode only)
- The draft model must be downloaded and available locally
- Only text generation is supported (not vision/audio)

**Configuration**

Speculative decoding is configured via the `draft-model` block in catalog
YAML or `model_config.yaml`:

```yaml
# In a catalog YAML file
config:
  context-window: 32768
  nbatch: 2048
  nubatch: 512
  cache-type-k: q8_0
  cache-type-v: q8_0
  nseq-max: 1
  incremental-cache: true
  draft-model:
    model-id: Qwen3-0.6B-Q8_0 # Draft model ID (must be downloaded)
    ndraft: 5 # Candidates per step (default: 5)
    ngpu-layers: 0 # GPU layers (0=all, -1=none)
    device: "" # Pin to specific GPU (e.g., "GPU1")
```

```yaml
# In model_config.yaml
Qwen3-8B-Q8_0:
  incremental-cache: true
  nseq-max: 1
  draft-model:
    model-id: Qwen3-0.6B-Q8_0
    ndraft: 5
```

| Field      | YAML Key      | Default | Description                         |
| ---------- | ------------- | ------- | ----------------------------------- |
| ModelID    | `model-id`    | (none)  | Draft model ID (must be downloaded) |
| NDraft     | `ndraft`      | 5       | Number of candidate tokens per step |
| NGpuLayers | `ngpu-layers` | 0 (all) | GPU layers for draft model          |
| Device     | `device`      | ""      | Pin draft model to a specific GPU   |

**Draft Model Selection**

Choose a draft model that shares the same tokenizer family as the target.
A quantized version of the same architecture at lower precision works well:

| Target Model              | Recommended Draft                       |
| ------------------------- | --------------------------------------- |
| Qwen3-8B-Q8_0             | Qwen3-0.6B-Q8_0                         |
| Qwen_Qwen3.5-35B-A3B-Q8_0 | Qwen3-Coder-30B-A3B-Instruct-UD-Q4_K_XL |

The second example uses the same MoE architecture at lower quantization,
which shares more of the target's weight structure and produces higher
acceptance rates than a smaller dense model.

**Performance Characteristics**

Speculative decoding helps most when the target model is large relative to the
draft. For dense models where the target is already fast (e.g., 8B at 33+ TPS),
the overhead of running a draft model may not provide a net speedup. MoE models
with large parameter counts but sparse activation (e.g., 30B-A3B) are better
candidates, but only when using a high-quality draft.

The `ndraft` parameter controls how many candidates to generate. Higher values
increase the potential speedup but also increase wasted work when predictions
are rejected. The default of 5 is a good starting point; tune based on your
observed acceptance rates.

### 3.10 Sampling Parameters

Sampling parameters control the randomness and quality of generated text.
These are set per-request in the API call.

For most models you will want to touch these basic sampling parameters. There
are many more which will be presented later.

**Temperature**

Controls randomness. Lower values produce more deterministic output.

```json
{
  "temperature": 0.8
}
```

- `0.0-0.3` - Focused, deterministic (good for code, factual Q&A)
- `0.5-0.8` - Balanced (good for general chat)
- `0.9-1.2` - Creative (good for storytelling, brainstorming)

**Top-K and Top-P**

Limit the token selection pool:

```json
{
  "top_k": 40,
  "top_p": 0.9
}
```

- `top_k` - Consider only the K most probable tokens (default: 40)
- `top_p` - Consider tokens until cumulative probability reaches P (default: 0.9)

**Repetition Control**

Reduce repetitive output:

```json
{
  "repeat_penalty": 1.1,
  "repeat_last_n": 64
}
```

- `repeat_penalty` - Penalty for repeated tokens (1.0 = off, 1.1 = mild)
- `repeat_last_n` - How many recent tokens to check (default: 64)

**DRY Sampler (Don't Repeat Yourself)**

Advanced n-gram repetition penalty:

```json
{
  "dry_multiplier": 1.05,
  "dry_base": 1.75,
  "dry_allowed_length": 2
}
```

**Max Tokens**

Limit the response length:

```json
{
  "max_tokens": 2048
}
```

### 3.11 Model Config File Example

Create a YAML config file for custom model settings:

```yaml
# model-config.yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 32768
    n_batch: 2048
    n_ubatch: 512
    n_seq_max: 2
    cache_type_k: q8_0
    cache_type_v: q8_0
    flash_attention: enabled
    system_prompt_cache: true

  Llama-3.3-70B-Instruct-Q8_0:
    context_window: 8192
    n_gpu_layers: 0
    split_mode: row
    offload_kqv: true
```

Start the server with custom config:

```shell
kronk server start --model-config-file=model-config.yaml
```

---
