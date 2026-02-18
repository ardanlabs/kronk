# Kronk User Manual

## Table of Contents

1. [Introduction](#chapter-1-introduction)
2. [Installation & Quick Start](#chapter-2-installation--quick-start)
3. [Model Configuration](#chapter-3-model-configuration)
4. [Batch Processing](#chapter-4-batch-processing)
5. [Message Caching](#chapter-5-message-caching)
6. [YaRN Extended Context](#chapter-6-yarn-extended-context)
7. [Model Server](#chapter-7-model-server)
8. [API Endpoints](#chapter-8-api-endpoints)
9. [Request Parameters](#chapter-9-request-parameters)
10. [Multi-Modal Models](#chapter-10-multi-modal-models)
11. [Security & Authentication](#chapter-11-security--authentication)
12. [Browser UI (BUI)](#chapter-12-browser-ui-bui)
13. [Client Integration](#chapter-13-client-integration)
14. [Observability](#chapter-14-observability)
15. [MCP Service](#chapter-15-mcp-service)
16. [Troubleshooting](#chapter-16-troubleshooting)
17. [Developer Guide](#chapter-17-developer-guide)

---

## Chapter 1: Introduction

### 1.1 What is Kronk

Kronk is a Go SDK and Model Server for running local inference with open-source
GGUF models. Built on top of llama.cpp via the [yzma](https://github.com/hybridgroup/yzma)
Go bindings (a non-CGO FFI layer), Kronk provides hardware-accelerated inference
for text generation, vision, audio, embeddings, and reranking.

**The SDK is the foundation.** The Kronk Model Server is built entirely on top
of the SDK — we "dog food" our own library. Everything the model server can do
is available to you as a SDK developer to help you write your own applications.

**You don't need a model server.** The real power of Kronk is that you can embed
model inference directly into your Go applications. Load models, run inference,
manage caching, and handle concurrent requests — all without running the models
in a separate server process. The [examples](examples/) directory demonstrates
building standalone applications with the SDK.

**The Model Server is optional.** When you do need an model server (for web UIs,
multi-client access, or OpenAI-compatible endpoints), the Kronk Model Server
provides:

- OpenAI and Anthropic compatible REST APIs
- OpenWebUI integration
- Agent and tool support for local models
- Any OpenAI-compatible client

### 1.2 Key Features

**Model Types**

- **Text Generation** - Chat completions and streaming responses with reasoning support.
- **Vision** - Image understanding and analysis.
- **Audio** - Speech-to-text and audio understanding.
- **Embeddings** - Vector embeddings for semantic search and RAG.
- **Reranking** - Document relevance scoring.

**Performance**

- **Batch Processing** - Process multiple requests concurrently within a set of partitioned KV cache sequences.
- **Message Caching** - System prompt and incremental message caching to reduce redundant computation.
- **YaRN Context Extension** - Extend context windows 2-4x beyond native training length.
- **Model Pooling** - Keep a number of models loaded in memory with configurable TTL.

**Operations**

- **Catalog System** - Curated collection of verified models with one-command downloads.
- **Browser UI (BUI)** - Web interface for model management, downloads, and configuration.
- **Authentication** - JWT-based security with key management, endpoint authorization and rate limiting.
- **Observability** - Tracing and metrics integration with Grafana support.

### 1.3 Supported Platforms and Hardware

Kronk supports full hardware acceleration across major platforms:

| **OS**  | **CPU**      | **GPU**                         |
| ------- | ------------ | ------------------------------- |
| Linux   | amd64, arm64 | CUDA, Vulkan, HIP, ROCm, SYCL   |
| macOS   | arm64        | Metal                           |
| Windows | amd64        | CUDA, Vulkan, HIP, SYCL, OpenCL |

**Hardware Requirements**

- Minimum 8GB RAM for small models (1-3B parameters)
- 16GB+ RAM recommended for medium models (7-8B parameters)
- 32GB+ RAM or dedicated GPU VRAM for large models (30B+ parameters)
- GPU with Metal, CUDA, or Vulkan support recommended for optimal performance

### 1.4 Architecture Overview

Kronk is designed as a layered architecture where the SDK provides all core
functionality and the Model Server is one application built on top of it.

![Kronk SDK Architecture](https://github.com/ardanlabs/kronk/blob/main/images/design/sdk.png?raw=true)

**Layer Breakdown:**

| Layer           | Component                            | Purpose                                    |
| --------------- | ------------------------------------ | ------------------------------------------ |
| **Application** | Kronk Model Server                   | REST API server (or your own app)          |
| **SDK Tools**   | Models, Libs, Catalog, Template APIs | High-level APIs for common tasks           |
| **SDK Core**    | Kronk SDK API, Model SDK API         | Model loading, inference, pooling, caching |
| **Bindings**    | yzma (non-CGO FFI via purego)        | Go bindings to llama.cpp without CGO       |
| **Engine**      | llama.cpp                            | Hardware-accelerated inference             |
| **Hardware**    | Metal, CUDA, Vulkan, CPU             | GPU/CPU acceleration                       |

**The Key Insight:** Your application sits at the same level as the Kronk Model
Server. You have access to the exact same SDK APIs. Whether you're building a
CLI tool, a web service, an embedded system, or a desktop app — you get the
full power of local model inference without any server overhead.

**SDK vs Server Usage:**

```go
// Direct SDK usage - no server needed
cfg := model.Config{
    ModelFiles: modelPath.ModelFiles,
    CacheTypeK: model.GGMLTypeQ8_0,
    CacheTypeV: model.GGMLTypeQ8_0,
}

krn, _ := kronk.New(cfg)
defer krn.Unload(ctx)

ch, _ := krn.ChatStreaming(ctx, model.D{
    "messages":   model.DocumentArray(model.TextMessage(model.RoleUser, "Hello")),
    "max_tokens": 2048,
})

for resp := range ch {
    fmt.Print(resp.Choice[0].Delta.Content)
}
```

```shell
# Or use the Model Server for OpenAI-compatible API
kronk server start
curl http://localhost:8080/v1/chat/completions -d '{"model":"Qwen3-8B-Q8_0","messages":[...]}'
```

---

## Chapter 2: Installation & Quick Start

### 2.1 Prerequisites

**Required**

- Go 1.26 or later
- Internet connection (for downloading libraries and models)

**Recommended**

- GPU with Metal (macOS), CUDA (NVIDIA), or Vulkan support
- 16GB+ system RAM (96GB+ Recommended)

### 2.2 Installing the CLI

Install Kronk using Go:

```shell
go install github.com/ardanlabs/kronk/cmd/kronk@latest
```

Verify the installation:

```shell
kronk --help
```

You should see output listing available commands:

```
Kronk CLI - A tool for managing Kronk models

Usage:
  kronk [command]

Available Commands:
  catalog     Manage model catalog
  libs        Install or upgrade llama.cpp libraries
  model       Manage models
  run         Run a model directly for quick testing
  security    Manage security keys and tokens
  server      Manage Kronk model server
  help        Help about any command
```

### 2.3 Installing Libraries

Before running inference, you need the llama.cpp libraries for your machine. Kronk auto-detects your hardware and downloads the appropriate binaries.

**Option A: Via the Server (Recommended)**

Start the server and use the BUI to download libraries:

```shell
kronk server start
```

Open http://localhost:8080 in your browser and navigate to the Libraries page.

**Option B: Via CLI**

```shell
kronk libs --local
```

This downloads libraries to `~/.kronk/libraries/` using auto-detected settings.

**Environment Variables for Library Installation**

```
KRONK_LIB_PATH  - Library directory (default: `~/.kronk/libraries`)
KRONK_PROCESSOR - `cpu`, `cuda`, `metal`, or `vulkan` (default: `cpu`)
KRONK_ARCH      - Architecture override: `amd64`, `arm64`
KRONK_OS        - OS override: `linux`, `darwin`, `windows`
```

**Example: Install CUDA Libraries**

```shell
KRONK_PROCESSOR=cuda kronk libs --local
```

### 2.4 Downloading Your First Model

Kronk provides a curated catalog of verified models. List available models:

```shell
kronk catalog list --local
```

Output:

```
CATALOG              MODEL ID                            PULLED   ENDPOINT
Audio-Text-to-Text   Qwen2-Audio-7B.Q8_0                 no       chat_completion
Embedding            embeddinggemma-300m-qat-Q8_0        no       embeddings
Image-Text-to-Text   gemma-3-4b-it-q4_0                  no       chat_completion
Text-Generation      Qwen3-8B-Q8_0                       no       chat_completion
Text-Generation      Llama-3.3-70B-Instruct-Q8_0         no       chat_completion
...
```

Download a model (recommended starter: Qwen3-8B):

```shell
kronk catalog pull Qwen3-8B-Q8_0 --local
```

Models are stored in `~/.kronk/models/` by default.

### 2.5 Starting the Server

Start the Kronk Model Server:

```shell
kronk server start
```

The server starts on `http://localhost:8080` by default. You'll see output like:

```
Kronk Model Server started
API: http://localhost:8080
BUI: http://localhost:8080
```

**Running in Background**

To run the server as a background process:

```shell
kronk server start -d
```

**Stopping the Server**

```shell
kronk server stop
```

### 2.6 Verifying the Installation

**Test via curl**

```shell
curl http://localhost:8080/v1/models
```

You should see a list of available models.

**Test Chat Completion**

_Note: It might take a few seconds the first time you call this because the
model needs to be loaded into memory first._

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'
```

**Test via BUI**

Open http://localhost:8080 in your browser and navigate to the Apps/Chat app. Select the model you want to try and chat away.

### 2.7 Quick Start Summary

```shell
# 1. Install Kronk
go install github.com/ardanlabs/kronk/cmd/kronk@latest

# 2. Start the server (auto-installs libraries on first run)
kronk server start

# 3. Open BUI and download a model
open http://localhost:8080

# 4. Download via the BUI Catalog/List screen or use this CLI call
kronk catalog pull Qwen3-8B-Q8_0 --local

# 5. Test the API using this curl call or the BUI App/Chat screen
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "Qwen3-8B-Q8_0", "messages": [{"role": "user", "content": "Hello!"}]}'
```

---

## Chapter 3: Model Configuration

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
  Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL:
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

| Mode | Slot Lifetime           | Cache Strategy                                                                              |
| ---- | ----------------------- | ------------------------------------------------------------------------------------------- |
| Off  | Cleared after request   | None                                                                                        |
| SPC  | Cleared after request   | System prompt decoded once, KV state stored in RAM, restored per request via StateSeqSetData |
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

| Mode | Slot Lifetime           | Cache Strategy                                                                               |
| ---- | ----------------------- | -------------------------------------------------------------------------------------------- |
| off  | Cleared after request   | None                                                                                         |
| SPC  | Cleared after request   | System prompt decoded once, KV state stored in RAM, restored per request via StateSeqSetData  |
| IMC  | Persists across requests | Full conversation cached in the slot's KV cache sequence                                     |

#### Example: Real Model Calculation

```
Model                   : Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL
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
    model-id: Qwen3-0.6B-Q8_0     # Draft model ID (must be downloaded)
    ndraft: 5                       # Candidates per step (default: 5)
    ngpu-layers: 0                  # GPU layers (0=all, -1=none)
    device: ""                      # Pin to specific GPU (e.g., "GPU1")
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

| Field        | YAML Key       | Default | Description                              |
| ------------ | -------------- | ------- | ---------------------------------------- |
| ModelID      | `model-id`     | (none)  | Draft model ID (must be downloaded)      |
| NDraft       | `ndraft`       | 5       | Number of candidate tokens per step      |
| NGpuLayers   | `ngpu-layers`  | 0 (all) | GPU layers for draft model               |
| Device       | `device`       | ""      | Pin draft model to a specific GPU        |

**Draft Model Selection**

Choose a draft model that shares the same tokenizer family as the target.
A quantized version of the same architecture at lower precision works well:

| Target Model                              | Recommended Draft                          |
| ----------------------------------------- | ------------------------------------------ |
| Qwen3-8B-Q8_0                             | Qwen3-0.6B-Q8_0                           |
| Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL  | Qwen3-Coder-30B-A3B-Instruct-UD-Q4_K_XL   |

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

## Chapter 4: Batch Processing

Batch processing allows Kronk to handle multiple concurrent requests
efficiently by sharing model resources. This chapter explains the architecture
and how to optimize for your workload.

### 4.1 Architecture Overview

For text inference models (including vision/audio), Kronk always creates a
batch engine with `NSeqMax` slots (defaulting to 1). `NSeqMax` controls how
many sequences are processed in parallel within a single model instance.

```
                    ┌───────────────────────────────────┐
   Request 1 ──────▶│                                   │
                    │          Request Queue            │
   Request 2 ──────▶│      (capacity: NSeqMax × 2)      │
                    │                                   │
   Request 3 ──────▶│                                   │
                    └────────────────┬──────────────────┘
                                     │
                                     ▼
                    ┌───────────────────────────────────┐
                    │           Batch Engine            │
                    │                                   │
                    │  ┌─────────┐  ┌─────────┐         │
                    │  │ Slot 0  │  │ Slot 1  │  ...    │
                    │  │ seqID=0 │  │ seqID=1 │         │
                    │  └────┬────┘  └────┬────┘    ┬    │
                    │       │            │         │    │
                    │       └─────────┬──┴─────────┘    │
                    │                 ▼                 │
                    │          Shared KV Cache          │
                    │  ┌─────────────────────────────┐  │
                    │  │ seq 0 │ seq 1 │ seq 2 │ ... │  │
                    │  └─────────────────────────────┘  │
                    └───────────────────────────────────┘
                                      │
                                      ▼
                    ┌───────────────────────────────────┐
                    │        llama.cpp Backend          │
                    │       (GPU/CPU Inference)         │
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

The slot/sequence layout is the same for all caching strategies:

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
4. **Prefill**: Tokenize and process remaining prompt tokens
5. **Decode**: Generate tokens one at a time, streaming to client
6. **Complete**: Release the slot:
   - Clear the entire sequence (no caching or SPC)
   - Trim generated tokens, keep cached conversation prefix (IMC)

### 4.4 Configuring Batch Processing

Batch processing is controlled primarily through the model configuration. The
key setting is `NSeqMax`, which determines how many slots the batch engine
creates and therefore how many requests can be processed in parallel. Increasing
`NSeqMax` improves concurrency but requires proportionally more KV cache memory,
so it's important to balance throughput against available VRAM.

**Enable Batch Processing**

Set `NSeqMax > 1` in your model config:

```yaml
models:
  Qwen3-8B-Q8_0:
    n_seq_max: 4 # 4 concurrent requests
```

**Queue Depth**

The request queue holds `NSeqMax × 2` requests by default. With `NSeqMax=4`,
up to 8 requests can be in-flight: 4 actively processing in slots and 4
waiting in the queue. This multiplier is configurable via `WithQueueDepth`
when using the SDK:

```go
krn, err := kronk.New(ctx, cfg, kronk.WithQueueDepth(3))
```

When all slots and queue positions are occupied, new requests block until a
slot completes or the request's context is cancelled. If the engine is
shutting down, queued requests receive an immediate error. This backpressure
mechanism prevents the system from accepting more work than it can process
within a reasonable time.

**Memory and Caching**

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

**Chat Request Flow (NSeqMax=4)**

When multiple users send chat requests at the same time, each request is
assigned to its own slot inside the batch engine. Rather than processing each
request in isolation, the engine combines tokens from all active slots into a
single GPU operation. This is what makes batch processing efficient — one
large decode call instead of several small ones. The following diagram shows
this flow with four concurrent requests:

```
Request 1 ──→ acquireModel(Qwen3-8B) ──→ ChatStreaming() ──→ batch.Submit()
Request 2 ──→ acquireModel(Qwen3-8B) ──→ ChatStreaming() ──→ batch.Submit()
Request 3 ──→ acquireModel(Qwen3-8B) ──→ ChatStreaming() ──→ batch.Submit()
Request 4 ──→ acquireModel(Qwen3-8B) ──→ ChatStreaming() ──→ batch.Submit()
                                                                   ↓
                                               ┌───────────────────────────┐
                                               │      Batch Engine         │
                                               │ ┌─────┬─────┬─────┬─────┐ │
                                               │ │Slot0│Slot1│Slot2│Slot3│ │
                                               │ │ R1  │ R2  │ R3  │ R4  │ │
                                               │ └─────┴─────┴─────┴─────┘ │
                                               │             ↓             │
                                               │   Single batched decode   │
                                               │   (all 4 in parallel)     │
                                               └───────────────────────────┘
```

From the outside, each request behaves as if it has the model to itself — it
receives its own stream of generated tokens. Internally, the batch engine is
doing the work for all four requests in lockstep, which uses the GPU far more
efficiently than handling them one at a time.

**Embedding/Rerank Request Flow (NSeqMax=4)**

Embedding and reranking models don't use the batch engine. Instead, Kronk
creates a pool of independent contexts — one per `NSeqMax` slot. When a
request arrives, it acquires a context from the pool, processes its inputs,
and releases the context back. If all contexts are in use, the request blocks
until one becomes available. The following diagram shows this flow:

```
Request 1 ──→ acquireModel() ──→ pool.acquire() ──→ Context 1 ──→ decode ──→ results
Request 2 ──→ acquireModel() ──→ pool.acquire() ──→ Context 2 ──→ decode ──→ results
Request 3 ──→ acquireModel() ──→ pool.acquire() ──→ Context 3 ──→ decode ──→ results
Request 4 ──→ acquireModel() ──→ pool.acquire() ──→ Context 4 ──→ decode ──→ results
                                       ↓
                          All 4 run in parallel
                          (separate decode calls)
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

High-throughput server configuration:

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

---

## Chapter 5: Message Caching

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

**Works best with:** Models with consistent templates where the same messages
always produce identical templated output regardless of conversation length.
Models like QWEN and Llama have consistent templates and get the fastest path
(hash-based matching). Models like GPT-OSS and GLM with non-deterministic
templates are also supported — IMC automatically falls back to token-level
partial prefix matching, salvaging as much of the KV cache as possible instead
of rebuilding from scratch (see [Token-Level Partial Prefix Matching](#token-level-partial-prefix-matching)
below).

**Best for:**

- Models with consistent templates (QWEN, Llama) — fastest hash-based path
- Models with non-deterministic templates (GPT-OSS, GLM) — token prefix fallback
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

When a request arrives, IMC scans all slots to find the best match:

1. **Scan all slots** — For each slot:
   - Skip slots with a build in-flight (pending flag set)
   - Skip empty slots (track the first empty slot as a fallback)
   - Skip slots with more cached messages than the request has total
   - Hash `messages[:slot.lastMsgIdxCached]` and compare to the slot's
     stored hash

2. **On match** — Pick the slot with the best prefix coverage (most cached
   messages). If the request has new messages to cache, extend the slot's
   cache. If the messages are identical, it's a pure cache hit.

3. **No hash match — token prefix fallback** — Tokenize the incoming messages
   and compare the resulting token sequence element-by-element against each
   non-empty slot's stored `cachedTokens`. Pick the slot with the longest
   common prefix that meets `cache_min_tokens`. Trim the KV cache from the
   divergence point (`MemorySeqRm(seq, trimPos, -1)`) and decode only the
   new tokens from there forward. This handles non-deterministic templates
   (e.g., GPT-OSS) that produce different token sequences for identical
   messages across requests — salvaging 70-80% of the cache instead of
   rebuilding from scratch.

4. **No match at all** — Pick an empty slot if one exists, otherwise evict
   the least-recently-used (LRU) slot and rebuild from scratch.

**Concurrent Build Protection:**

When two requests arrive simultaneously and both need to build a cache from
scratch, a race condition could cause both to pick the same empty slot. IMC
prevents this with a pending flag: when a slot begins a deferred cache build,
it is marked pending. Concurrent scanners skip pending slots, so the second
request picks a different slot. The pending flag is cleared after the cache
decode completes (or on error).

**Token-Level Partial Prefix Matching:**

Some model templates are non-deterministic — they produce different token
sequences for identical messages across requests. GPT-OSS, for example,
injects tool call formatting that varies between template invocations. This
causes message hash mismatches even though the semantic content is identical,
which would normally force a full cache rebuild from scratch.

IMC handles this automatically. When no hash match is found during the slot
scan, IMC falls back to comparing the actual cached token arrays against the
incoming request's tokens. It tokenizes the incoming messages, then compares
them element-by-element against each non-empty slot's stored token sequence
to find the longest common prefix.

```
Cached tokens:   [T1, T2, T3, T4, T5, T6, T7, T8]
Incoming tokens: [T1, T2, T3, T4, T5, T9, T10, T11, T12]
                                       ↑
                              Divergence point (pos 5)

Common prefix: 5 tokens (salvaged from KV cache)
Trimmed:       3 tokens (T6-T8 removed via MemorySeqRm)
New decode:    7 tokens (T5-T12, from divergence point forward)
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

**Debugging Token Prefix Matching:**

Look for these log messages to observe the token prefix path:

| Log Message                                   | Meaning                                                                 |
| --------------------------------------------- | ----------------------------------------------------------------------- |
| `no slot matched, trying token prefix match`  | Hash match failed, entering token comparison                            |
| `slot[N] common-prefix X/Y tokens (Z% salvageable)` | Per-slot comparison result                                        |
| `token prefix match found`                    | Usable prefix found, will trim and extend                               |
| `imc-trim-prefix`                             | KV cache trim in progress (shows cached_tokens, trim_pos)               |
| `imc-partial-rebuilt`                          | Rebuild complete (shows total_cached, salvaged_prefix, salvaged_pct)    |
| `no usable token prefix match`                | All prefixes below `cache_min_tokens`, falling back to empty/LRU slot   |

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

| Feature      | System Prompt Cache              | Incremental Message Cache                     |
| ------------ | -------------------------------- | --------------------------------------------- |
| Caches       | System prompt only               | All messages except last                      |
| Extends      | No                               | Yes, incrementally                            |
| Multi-user   | Single shared cache sequence     | Single-user, all slots available              |
| Sub-agents   | All share same SPC sequence      | Each gets own slot via hash matching          |
| Best for     | Chat UIs                         | Agentic workflows                             |
| Memory       | Zero extra VRAM (KV state in RAM)| Zero extra VRAM overhead                      |
| Template req | Any                              | Any (hash match or token prefix fallback)     |

**Important:** SPC and IMC are mutually exclusive. Choose based on your
workload:

- **Agentic workflows:** Use IMC — works with all templates. Deterministic
  templates (QWEN, Llama) get the fastest hash-based path. Non-deterministic
  templates (GPT-OSS, GLM) use token prefix fallback with 70-80% cache salvage.
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

**IMC Limitations:**

- Text-only requests (IMC for vision/audio is not currently supported)
- Conversations must grow monotonically (append-only)
- Editing earlier messages triggers full cache rebuild
- Designed for single-user use
- Max concurrent conversation branches = NSeqMax; when all slots are
  occupied, the least-recently-used slot is evicted

---

## Chapter 6: YaRN Extended Context

YaRN (Yet another RoPE extensioN) allows models to handle context windows
beyond their native training length. This is essential for long documents,
extended conversations, and complex agentic workflows.

### 6.1 Understanding Context Extension

Language models are trained with a fixed context length (e.g., 8K, 32K tokens).
RoPE (Rotary Position Embedding) encodes position information, but naive
extension beyond training length causes quality degradation.

YaRN applies frequency-dependent interpolation with attention scaling to
maintain quality at extended lengths.

```
Native Context:     32K tokens (training length)
Extended Context:   131K tokens (4x extension with YaRN)
```

### 6.2 When to Use YaRN

**Good candidates for YaRN:**

- Qwen3 models (trained at 32K, support 131K with YaRN)
- Llama models with RoPE scaling support
- Any model where you need 2-4x the native context

**When NOT to use YaRN:**

- If native context is sufficient for your use case
- Extensions beyond 4x (quality degrades significantly)
- Models without RoPE (older architectures)

### 6.3 Configuration

**Basic YaRN Setup:**

```yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 131072 # Extended context (131K)
    rope_scaling: yarn # Enable YaRN
```

That's often all you need—Kronk auto-calculates the other YaRN parameters
from the context extension ratio.

**Full Configuration (Advanced):**

```yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 131072
    rope_scaling: yarn
    rope_freq_base: 1000000 # Model-specific (Qwen3 uses 1M)
    rope_freq_scale: null # Auto-calculate
    yarn_ext_factor: null # Auto-calculate
    yarn_attn_factor: 1.0 # Attention scaling
    yarn_beta_fast: 32.0 # Low correction dimension
    yarn_beta_slow: 1.0 # High correction dimension
    yarn_orig_ctx: 32768 # Original training context
```

### 6.4 Scaling Types

Kronk supports three RoPE scaling methods:

**None (Default)**

```yaml
rope_scaling: none
```

Uses native context length. No scaling applied.

**Linear**

```yaml
rope_scaling: linear
```

Simple linear interpolation. Works but quality degrades faster than YaRN
at high extension ratios.

**YaRN (Recommended)**

```yaml
rope_scaling: yarn
```

Frequency-dependent interpolation with attention scaling. Maintains quality
better at 2-4x extensions.

### 6.5 Parameter Reference

| Parameter          | Default        | Description                                         |
| ------------------ | -------------- | --------------------------------------------------- |
| `rope_scaling`     | none           | Scaling method: `none`, `linear`, `yarn`            |
| `rope_freq_base`   | model default  | Base frequency (10000 for Llama, 1000000 for Qwen3) |
| `rope_freq_scale`  | auto           | Frequency scaling factor                            |
| `yarn_ext_factor`  | auto           | Extrapolation mix factor (0 = disable)              |
| `yarn_attn_factor` | 1.0            | Attention magnitude scaling                         |
| `yarn_beta_fast`   | 32.0           | Low correction dimension                            |
| `yarn_beta_slow`   | 1.0            | High correction dimension                           |
| `yarn_orig_ctx`    | model metadata | Original training context size                      |

### 6.6 Model-Specific Examples

**Qwen3 (32K → 131K)**

```yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 131072
    rope_scaling: yarn
```

Qwen3 models are specifically designed to support 131K context with YaRN.
The default parameters work well.

**Llama 3 (8K → 32K)**

```yaml
models:
  Llama-3-8B-Q8_0:
    context_window: 32768
    rope_scaling: yarn
    rope_freq_base: 10000
```

4x extension from 8K to 32K is within the recommended range.

### 6.7 Memory Impact

Extended context significantly increases memory requirements:

```
Qwen3-8B with F16 KV cache:

32K context:   ~1.6 GB KV cache
64K context:   ~3.2 GB KV cache
131K context:  ~6.5 GB KV cache
```

**Mitigation strategies:**

1. Use KV cache quantization:

```yaml
cache_type_k: q8_0
cache_type_v: q8_0
```

2. Reduce batch parallelism:

```yaml
n_seq_max: 1 # Fewer concurrent requests
```

3. Keep KV cache on CPU (slower but saves VRAM):

```yaml
offload_kqv: false
```

### 6.8 Quality Considerations

**Extension ratio guidelines:**

- 2x extension: Minimal quality loss
- 3x extension: Slight degradation, usually acceptable
- 4x extension: Noticeable but often usable
- > 4x extension: Not recommended

**Testing your configuration:**

1. Start with a known-good prompt at native context
2. Extend to your target length
3. Compare output quality
4. Adjust if needed (reduce extension or try different parameters)

### 6.9 Example: Long Document Processing

Configuration for processing long documents:

```yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 65536 # 64K context
    rope_scaling: yarn
    n_batch: 4096 # Larger batch for long prompts
    n_ubatch: 1024
    cache_type_k: q8_0
    cache_type_v: q8_0
    n_seq_max: 1 # Single request (memory intensive)
```

This configuration can process documents up to ~50K tokens while leaving
room for generation.

---

## Chapter 7: Model Server

The Kronk Model Server provides an OpenAI-compatible REST API for inference.
This chapter covers server configuration, management, and the catalog system.

**CLI Modes: Web vs Local**

Most CLI commands communicate with a running server by default:

```shell
kronk catalog list                # Talks to server at localhost:8080
kronk catalog pull Qwen3-8B-Q8_0  # Downloads via server
```

Add `--local` to run commands directly without a server:

```shell
kronk catalog list --local        # Direct file access
kronk catalog pull Qwen3-8B-Q8_0 --local
kronk libs --local
```

Use `--local` when:

- The server isn't running yet
- You're setting up on the same machine where the server will run
- You prefer direct file operations

Use web mode (no flag) when:

- The server is running
- You want progress streaming in the BUI
- You're managing a remote server via `KRONK_WEB_API_HOST`

**Environment Variables**

Every command-line flag has a corresponding environment variable. The naming
convention is `KRONK_` followed by the flag name in uppercase with hyphens
replaced by underscores:

```
--api-host        →  KRONK_WEB_API_HOST
--models-in-cache →  KRONK_MODELS_IN_CACHE
--cache-ttl       →  KRONK_CACHE_TTL
--processor       →  KRONK_PROCESSOR
--hf-token        →  KRONK_HF_TOKEN
```

Environment variables are useful for:

- Configuration in Docker/Kubernetes deployments
- Setting defaults without repeating flags
- Keeping secrets out of command history

### 7.1 Starting the Server

**Install the CLI** (if not already installed)

```shell
go install github.com/ardanlabs/kronk/cmd/kronk@latest
```

**Basic Start**

```shell
kronk server start
```

The server starts on `http://localhost:8080` by default.

**Background Mode**

Run the server as a background process:

```shell
kronk server start -d
```

**Custom Host/Port**

```shell
kronk server start --api-host=0.0.0.0:9000
```

### 7.2 Stopping the Server

```shell
kronk server stop
```

### 7.3 Server Configuration

Configuration can be set via command-line flags or environment variables. Every
flag has a corresponding environment variable using the `KRONK_` prefix with
underscores replacing hyphens.

**Web Settings**

| Flag                     | Environment Variable             | Default          | Description                  |
| ------------------------ | -------------------------------- | ---------------- | ---------------------------- |
| `--api-host`             | `KRONK_WEB_API_HOST`             | `localhost:8080` | API host address             |
| `--debug-host`           | `KRONK_WEB_DEBUG_HOST`           | `localhost:8090` | Debug/pprof host address     |
| `--read-timeout`         | `KRONK_WEB_READ_TIMEOUT`         | `30s`            | HTTP read timeout            |
| `--write-timeout`        | `KRONK_WEB_WRITE_TIMEOUT`        | `15m`            | HTTP write timeout           |
| `--idle-timeout`         | `KRONK_WEB_IDLE_TIMEOUT`         | `1m`             | HTTP idle timeout            |
| `--shutdown-timeout`     | `KRONK_WEB_SHUTDOWN_TIMEOUT`     | `1m`             | Graceful shutdown timeout    |
| `--cors-allowed-origins` | `KRONK_WEB_CORS_ALLOWED_ORIGINS` | `*`              | Comma-separated CORS origins |

**Authentication Settings**

| Flag             | Environment Variable       | Default         | Description                                               |
| ---------------- | -------------------------- | --------------- | --------------------------------------------------------- |
| `--auth-host`    | `KRONK_AUTH_HOST`          | _(empty)_       | External auth service host. Leave empty to use local auth |
| `--auth-enabled` | `KRONK_AUTH_LOCAL_ENABLED` | `false`         | Enable local JWT authentication                           |
| `--auth-issuer`  | `KRONK_AUTH_LOCAL_ISSUER`  | `kronk project` | Issuer name for local JWT tokens                          |

**Tracing Settings (Tempo)**

| Flag                   | Environment Variable       | Default          | Description                          |
| ---------------------- | -------------------------- | ---------------- | ------------------------------------ |
| `--tempo-host`         | `KRONK_TEMPO_HOST`         | `localhost:4317` | OpenTelemetry collector host         |
| `--tempo-service-name` | `KRONK_TEMPO_SERVICE_NAME` | `kronk`          | Service name for traces              |
| `--tempo-probability`  | `KRONK_TEMPO_PROBABILITY`  | `0.25`           | Trace sampling probability (0.0-1.0) |

**Catalog Settings**

| Flag                    | Environment Variable              | Default        | Description                                            |
| ----------------------- | --------------------------------- | -------------- | ------------------------------------------------------ |
| `--catalog-github-repo` | `KRONK_CATALOG_GITHUB_REPO`       | GitHub API URL | GitHub repo URL for catalog files                      |
| `--model-config-file`   | `KRONK_CATALOG_MODEL_CONFIG_FILE` | _(empty)_      | Path to model-specific config YAML file                |
| `--catalog-repo-path`   | `KRONK_CATALOG_REPO_PATH`         | _(empty)_      | Path to cloned catalog repository for publishing edits |

**Template Settings**

| Flag                      | Environment Variable          | Default        | Description                        |
| ------------------------- | ----------------------------- | -------------- | ---------------------------------- |
| `--templates-github-repo` | `KRONK_TEMPLATES_GITHUB_REPO` | GitHub API URL | GitHub repo URL for template files |

**Cache Settings**

| Flag                       | Environment Variable                 | Default | Description                               |
| -------------------------- | ------------------------------------ | ------- | ----------------------------------------- |
| `--models-in-cache`        | `KRONK_CACHE_MODELS_IN_CACHE`        | `3`     | Maximum models kept loaded in memory      |
| `--cache-ttl`              | `KRONK_CACHE_TTL`                    | `20m`   | How long unused models stay loaded        |
| `--ignore-integrity-check` | `KRONK_CACHE_IGNORE_INTEGRITY_CHECK` | `true`  | Skip SHA256 integrity check on model load |

**Runtime Settings**

| Flag                 | Environment Variable     | Default    | Description                                       |
| -------------------- | ------------------------ | ---------- | ------------------------------------------------- |
| `--base-path`        | `KRONK_BASE_PATH`        | `~/.kronk` | Base directory for all Kronk data                 |
| `--lib-path`         | `KRONK_LIB_PATH`         | _(empty)_  | Path to llama library directory                   |
| `--lib-version`      | `KRONK_LIB_VERSION`      | _(empty)_  | Specific llama library version                    |
| `--arch`             | `KRONK_ARCH`             | _(auto)_   | Architecture override (`amd64`, `arm64`)          |
| `--os`               | `KRONK_OS`               | _(auto)_   | OS override (`linux`, `darwin`, `windows`)        |
| `--processor`        | `KRONK_PROCESSOR`        | _(auto)_   | Processor type (`cpu`, `metal`, `cuda`, `vulkan`) |
| `--hf-token`         | `KRONK_HF_TOKEN`         | _(empty)_  | Hugging Face API token for gated models           |
| `--allow-upgrade`    | `KRONK_ALLOW_UPGRADE`    | `true`     | Allow automatic library upgrades                  |
| `--llama-log`        | `KRONK_LLAMA_LOG`        | `1`        | Llama log level (0=off, 1=on)                     |
| `--insecure-logging` | `KRONK_INSECURE_LOGGING` | `false`    | Log sensitive data (messages, model config)       |

**Example**

```shell
kronk server start \
  --api-host=0.0.0.0:8080 \
  --models-in-cache=5 \
  --cache-ttl=30m \
  --model-config-file=model-config.yaml \
  --catalog-repo-path=~/code/kronk_catalogs \
  --hf-token=hf_xxxxx
```

### 7.4 Model Caching

The server maintains a pool of loaded models to avoid reload latency.

**Configuration**

```shell
kronk server start \
  --models-in-cache=3 \
  --cache-ttl=5m
```

- `models-in-cache` - Maximum models kept loaded (default: 3)
- `cache-ttl` - How long unused models stay loaded (default: 5m)

When a new model is requested and the cache is full, the least recently
used model is unloaded.

### 7.5 Model Config Files

Create a YAML file to configure model-specific settings:

```yaml
# model-config.yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 32768
    n_seq_max: 4
    cache_type_k: q8_0
    cache_type_v: q8_0
    system_prompt_cache: true

  Llama-3.3-70B-Instruct-Q8_0:
    context_window: 8192
    n_gpu_layers: 0
    split_mode: row

  embeddinggemma-300m-qat-Q8_0:
    n_seq_max: 2
```

Start with the config file:

```shell
kronk server start --model-config-file=model-config.yaml
```

Or via environment variable:

```shell
export KRONK_CATALOG_MODEL_CONFIG_FILE=/path/to/model-config.yaml
kronk server start
```

**Project Reference Configuration**

The Kronk repository includes a comprehensive reference configuration with
recommended settings for various models and use cases:

```shell
export KRONK_CATALOG_MODEL_CONFIG_FILE=<clone_path>/zarf/kms/model_config.yaml
kronk server start
```

This file includes:

- Optimized configurations for coding agents (Cline, OpenCode)
- YaRN extended context examples
- SPC and IMC variants for different caching strategies
- Vision and audio model settings
- Detailed comments explaining each configuration option

Review `zarf/kms/model_config.yaml` for examples of YAML anchors, cache
configurations, and model-specific tuning.

### 7.6 Catalog System

The catalog provides a curated list of verified models with preconfigured
settings.

**List Available Models**

```shell
kronk catalog list
```

Output:

```
CATALOG              MODEL ID                         PULLED  ENDPOINT
Audio-Text-to-Text   Qwen2-Audio-7B.Q8_0              no      chat_completion
Embedding            embeddinggemma-300m-qat-Q8_0     no      embeddings
Image-Text-to-Text   gemma-3-4b-it-q4_0               no      chat_completion
Text-Generation      Qwen3-8B-Q8_0                    yes     chat_completion
Text-Generation      Llama-3.3-70B-Instruct-Q8_0      no      chat_completion
```

**Filter by Category**

```shell
kronk catalog list --filter-category=Embedding
```

**Pull a Model**

```shell
kronk catalog pull Qwen3-8B-Q8_0
```

**Show Model Details**

```shell
kronk catalog show Qwen3-8B-Q8_0
```

**Update Catalog**

_Note: We don't have a server version of this yet._

```shell
kronk catalog update --local
```

### 7.7 Custom Catalog Repository

Use a custom catalog repository:

```shell
kronk server start \
  --catalog-github-repo=https://github.com/myorg/my-catalog
```

### 7.8 Templates

Templates define chat formatting (Jinja templates) for different models.
Kronk downloads templates automatically from the offical templates repository.

https://github.com/ardanlabs/kronk_catalogs

You don't need this unless you want to maintain your own repository.

**Custom Templates Repository**

```shell
kronk server start \
  --templates-github-repo=https://github.com/myorg/my-templates
```

Templates are cached in `~/.kronk/templates/` by default.

### 7.9 Runtime Settings

**Processor Selection**

```shell
kronk server start --processor=cuda    # NVIDIA GPU
kronk server start --processor=metal   # Apple Silicon
kronk server start --processor=vulkan  # Cross-platform GPU
kronk server start --processor=cpu     # CPU only
```

**Library Path**

```shell
kronk server start \
  --lib-path=/custom/path/to/libraries \
  --lib-version=b7406
```

**Hugging Face Token**

For gated models requiring authentication:

```shell
kronk server start --hf-token=hf_xxxxx
```

Or via environment variable:

```shell
export KRONK_HF_TOKEN=hf_xxxxx
kronk server start
```

### 7.10 Logging

**llama.cpp Logging**

```shell
kronk server start --llama-log=1    # Enable llama.cpp logs
kronk server start --llama-log=0    # Disable (default)
```

**Insecure Logging**

Enable logging of message content (for debugging only):

```shell
kronk server start --insecure-logging=true
```

**Warning:** This logs sensitive data. Never use in production.

**View Server Logs**

```shell
kronk server logs
```

### 7.11 Data Paths

Default data locations:

```
~/.kronk/
├── libraries/     # llama.cpp libraries
├── models/        # Downloaded models
├── templates/     # Chat templates
└── catalog/       # Catalog cache
```

**Custom Base Path**

```shell
kronk server start --base-path=/data/kronk
```

### 7.12 Complete Example

Production-ready server configuration:

```shell
kronk server start \
  --api-host=0.0.0.0:8080 \
  --models-in-cache=2 \
  --cache-ttl=10m \
  --model-config-file=/etc/kronk/models.yaml \
  --processor=cuda \
  --auth-enabled=true \
  -d
```

With model config:

```yaml
# /etc/kronk/models.yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 32768
    n_seq_max: 4
    cache_type_k: q8_0
    cache_type_v: q8_0
    incremental_cache: true
```

---

## Chapter 8: API Endpoints

Kronk provides an OpenAI-compatible REST API. This chapter documents the
available endpoints and their usage.

### 8.1 Endpoint Overview

| Endpoint               | Method | Description                                |
| ---------------------- | ------ | ------------------------------------------ |
| `/v1/chat/completions` | POST   | Chat completions (streaming/non-streaming) |
| `/v1/responses`        | POST   | OpenAI Responses API format                |
| `/v1/messages`         | POST   | Anthropic API format                       |
| `/v1/embeddings`       | POST   | Generate embeddings                        |
| `/v1/rerank`           | POST   | Rerank documents                           |
| `/v1/tokenize`         | POST   | Tokenize text input                        |
| `/v1/models`           | GET    | List available models                      |

### 8.2 Chat Completions

Generate chat responses using the familiar OpenAI format.

**Endpoint:** `POST /v1/chat/completions`

**Basic Request:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is the capital of France?"}
    ]
  }'
```

**Request Parameters:**

```json
{
  "model": "Qwen3-8B-Q8_0",
  "messages": [
    { "role": "system", "content": "System prompt" },
    { "role": "user", "content": "User message" },
    { "role": "assistant", "content": "Previous response" },
    { "role": "user", "content": "Follow-up question" }
  ],
  "temperature": 0.8,
  "top_p": 0.9,
  "top_k": 40,
  "max_tokens": 2048,
  "stream": true
}
```

**Streaming Response:**

With `"stream": true`, responses are sent as Server-Sent Events:

```
data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk",...}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk",...}

data: [DONE]
```

**Non-Streaming Response:**

```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "Qwen3-8B-Q8_0",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "The capital of France is Paris."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "completion_tokens": 8,
    "total_tokens": 33
  }
}
```

**Reasoning Models:**

For models with thinking/reasoning support (like Qwen3):

```json
{
  "model": "Qwen3-8B-Q8_0",
  "messages": [...],
  "enable_thinking": true
}
```

The response includes `reasoning_content` in the message.

To disable thinking:

```json
{
  "enable_thinking": false
}
```

### 8.3 Responses API

OpenAI's newer Responses API format, used by some clients.

**Endpoint:** `POST /v1/responses`

**Request:**

```shell
curl http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "input": "Explain quantum computing in simple terms."
  }'
```

The `input` field can be a string or an array of message objects.

**Streaming Events:**

The Responses API uses a different event format:

```
event: response.created
data: {"type":"response.created",...}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"The",...}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":" answer",...}

event: response.completed
data: {"type":"response.completed",...}
```

### 8.4 Embeddings

Generate vector embeddings for text.

**Endpoint:** `POST /v1/embeddings`

**Request:**

```shell
curl http://localhost:8080/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "embeddinggemma-300m-qat-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog."
  }'
```

**Multiple Inputs:**

```json
{
  "model": "embeddinggemma-300m-qat-Q8_0",
  "input": [
    "First document to embed.",
    "Second document to embed.",
    "Third document to embed."
  ]
}
```

**Response:**

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [0.123, -0.456, 0.789, ...]
    }
  ],
  "model": "embeddinggemma-300m-qat-Q8_0",
  "usage": {
    "prompt_tokens": 10,
    "total_tokens": 10
  }
}
```

### 8.5 Reranking

Score and reorder documents by relevance to a query.

**Endpoint:** `POST /v1/rerank`

**Request:**

```shell
curl http://localhost:8080/v1/rerank \
  -H "Content-Type: application/json" \
  -d '{
    "model": "bge-reranker-v2-m3-Q8_0",
    "query": "What is machine learning?",
    "documents": [
      "Machine learning is a subset of artificial intelligence.",
      "The weather today is sunny.",
      "Deep learning uses neural networks.",
      "I like pizza."
    ],
    "top_n": 2
  }'
```

**Response:**

```json
{
  "object": "list",
  "results": [
    {
      "index": 0,
      "relevance_score": 0.95,
      "document": "Machine learning is a subset of artificial intelligence."
    },
    {
      "index": 2,
      "relevance_score": 0.82,
      "document": "Deep learning uses neural networks."
    }
  ],
  "model": "bge-reranker-v2-m3-Q8_0",
  "usage": {
    "prompt_tokens": 45,
    "total_tokens": 45
  }
}
```

### 8.6 Tokenize

Get the token count for a text input. Works with any model type.

**Endpoint:** `POST /v1/tokenize`

**Parameters:**

| Field                   | Type      | Required | Description                                                                                                                                                  |
| ----------------------- | --------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `model`                 | `string`  | Yes      | Model ID (e.g., `Qwen3-8B-Q8_0`). Works with any model type.                                                                                                 |
| `input`                 | `string`  | Yes      | The text to tokenize.                                                                                                                                        |
| `apply_template`        | `boolean` | No       | If true, wraps the input as a user message and applies the model's chat template before tokenizing. The count includes template overhead. Defaults to false. |
| `add_generation_prompt` | `boolean` | No       | When `apply_template` is true, controls whether the assistant role prefix is appended to the prompt. Defaults to true.                                       |

**Request (raw text):**

```shell
curl http://localhost:8080/v1/tokenize \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog"
  }'
```

**Request (with template):**

```shell
curl http://localhost:8080/v1/tokenize \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog",
    "apply_template": true
  }'
```

**Response:**

```json
{
  "object": "tokenize",
  "created": 1738857600,
  "model": "Qwen3-8B-Q8_0",
  "tokens": 11
}
```

When `apply_template` is true, the token count will be higher than raw text
because it includes template overhead (role markers, separators, and the
generation prompt).

### 8.7 Tool Calling (Function Calling)

Kronk supports OpenAI-compatible tool calling, allowing models to request
function executions that you handle in your application.

**Request with Tools:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [
      {"role": "user", "content": "What is the weather in Paris?"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get current weather for a location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "City name"
              }
            },
            "required": ["location"]
          }
        }
      }
    ],
    "tool_choice": "auto"
  }'
```

**Tool Choice Options:**

- `"auto"` - Model decides whether to call tools (default)
- `"none"` - Never call tools
- `{"type": "function", "function": {"name": "get_weather"}}` - Force specific tool

**Response with Tool Calls:**

```json
{
  "id": "chatcmpl-xxx",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": null,
        "tool_calls": [
          {
            "id": "call_abc123",
            "type": "function",
            "function": {
              "name": "get_weather",
              "arguments": "{\"location\": \"Paris\"}"
            }
          }
        ]
      },
      "finish_reason": "tool_calls"
    }
  ]
}
```

**Handling Tool Results:**

After executing the tool, send the result back:

```json
{
  "model": "Qwen3-8B-Q8_0",
  "messages": [
    { "role": "user", "content": "What is the weather in Paris?" },
    {
      "role": "assistant",
      "content": null,
      "tool_calls": [
        {
          "id": "call_abc123",
          "type": "function",
          "function": {
            "name": "get_weather",
            "arguments": "{\"location\": \"Paris\"}"
          }
        }
      ]
    },
    {
      "role": "tool",
      "tool_call_id": "call_abc123",
      "content": "{\"temperature\": 18, \"condition\": \"sunny\"}"
    }
  ]
}
```

**Streaming with Tool Calls:**

Tool call arguments stream incrementally:

```
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"loc"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ation\":"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":" \"Paris\"}"}}]}}]}
```

### 8.8 Models List

Get available models.

**Endpoint:** `GET /v1/models`

**Request:**

```shell
curl http://localhost:8080/v1/models
```

**Response:**

```json
{
  "object": "list",
  "data": [
    {
      "id": "Qwen3-8B-Q8_0",
      "object": "model",
      "owned_by": "kronk"
    },
    {
      "id": "embeddinggemma-300m-qat-Q8_0",
      "object": "model",
      "owned_by": "kronk"
    }
  ]
}
```

### 8.9 Authentication

When authentication is enabled, include the token in requests:

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-token-here" \
  -d '{...}'
```

See [Chapter 11: Security & Authentication](#chapter-11-security--authentication)
for details on token management.

### 8.10 Error Responses

Errors follow a standard format:

```json
{
  "error": {
    "code": "invalid_argument",
    "message": "missing model field"
  }
}
```

**Common Error Codes:**

- `invalid_argument` - Missing or invalid request parameters
- `not_found` - Model not found
- `internal` - Server error during processing
- `unauthenticated` - Missing or invalid authentication token

---

## Chapter 9: Request Parameters

This chapter documents the request parameters available for controlling model output through both the SDK and REST API.

### 9.1 Sampling Parameters

These parameters control the randomness and diversity of generated text.

| Parameter   | JSON Key      | Type    | Default | Description                                                                                             |
| ----------- | ------------- | ------- | ------- | ------------------------------------------------------------------------------------------------------- |
| Temperature | `temperature` | float32 | 0.8     | Controls randomness of output. Higher values produce more varied text, lower values more deterministic. |
| Top-K       | `top_k`       | int32   | 40      | Limits token pool to K most probable tokens before sampling.                                            |
| Top-P       | `top_p`       | float32 | 0.9     | Nucleus sampling threshold. Only tokens with cumulative probability ≤ top_p are considered.             |
| Min-P       | `min_p`       | float32 | 0.0     | Dynamic sampling threshold. Tokens with probability < min_p × max_probability are excluded.             |

### 9.2 Repetition Control

These parameters help prevent repetitive output.

| Parameter      | JSON Key         | Type    | Default | Description                                                                 |
| -------------- | ---------------- | ------- | ------- | --------------------------------------------------------------------------- |
| Repeat Penalty | `repeat_penalty` | float32 | 1.0     | Penalty multiplier for repeated tokens. Values > 1.0 discourage repetition. |
| Repeat Last N  | `repeat_last_n`  | int32   | 64      | Window size for repetition check. Only the last N tokens are considered.    |

**DRY Parameters (Don't Repeat Yourself):**

DRY penalizes n-gram repetitions to prevent the model from repeating phrases.

| Parameter          | JSON Key             | Type    | Default | Description                                                                 |
| ------------------ | -------------------- | ------- | ------- | --------------------------------------------------------------------------- |
| DRY Multiplier     | `dry_multiplier`     | float32 | 1.05    | N-gram repetition penalty strength. Higher values penalize repetition more. |
| DRY Base           | `dry_base`           | float32 | 1.75    | Exponential penalty base for longer n-grams.                                |
| DRY Allowed Length | `dry_allowed_length` | int32   | 2       | Minimum n-gram length to consider for penalties.                            |
| DRY Penalty Last N | `dry_penalty_last_n` | int32   | 0       | Number of recent tokens to consider for DRY. 0 means all tokens.            |

### 9.3 Advanced Sampling

**XTC (eXtreme Token Culling):**

XTC probabilistically removes high-probability tokens to increase diversity.

| Parameter       | JSON Key          | Type    | Default | Description                                                  |
| --------------- | ----------------- | ------- | ------- | ------------------------------------------------------------ |
| XTC Probability | `xtc_probability` | float32 | 0.0     | Probability of activating XTC on each token. 0 disables XTC. |
| XTC Threshold   | `xtc_threshold`   | float32 | 0.1     | Probability threshold for token culling.                     |
| XTC Min Keep    | `xtc_min_keep`    | uint32  | 1       | Minimum number of tokens to keep after culling.              |

**Adaptive-P:**

Adaptive-P dynamically adjusts the sampling threshold based on output probability.

| Parameter         | JSON Key            | Type    | Default | Description                                                 |
| ----------------- | ------------------- | ------- | ------- | ----------------------------------------------------------- |
| Adaptive-P Target | `adaptive_p_target` | float32 | 0.0     | Target probability threshold. 0 disables adaptive sampling. |
| Adaptive-P Decay  | `adaptive_p_decay`  | float32 | 0.0     | Speed of threshold adjustment toward target.                |

### 9.4 Generation Control

| Parameter        | JSON Key           | Type   | Default  | Description                                                                |
| ---------------- | ------------------ | ------ | -------- | -------------------------------------------------------------------------- |
| Max Tokens       | `max_tokens`       | int    | 4096     | Maximum tokens to generate.                                                |
| Enable Thinking  | `enable_thinking`  | string | "true"   | Enable model thinking/reasoning mode. Set to "false" for direct responses. |
| Reasoning Effort | `reasoning_effort` | string | "medium" | GPT reasoning level: none, minimal, low, medium, high.                     |
| Stream           | `stream`           | bool   | false    | Stream response chunks via SSE.                                            |
| Include Usage    | `include_usage`    | bool   | true     | Include token usage statistics in streaming responses.                     |

### 9.5 Grammar Constrained Output

Grammars force the model to only produce tokens that match a specified pattern, guaranteeing structured output.

**Built-in Presets:**

| Preset              | Description                   |
| ------------------- | ----------------------------- |
| `GrammarJSON`       | Valid JSON objects or arrays  |
| `GrammarJSONObject` | JSON objects only             |
| `GrammarJSONArray`  | JSON arrays only              |
| `GrammarBoolean`    | "true" or "false"             |
| `GrammarYesNo`      | "yes" or "no"                 |
| `GrammarInteger`    | Integer values                |
| `GrammarNumber`     | Numeric values (int or float) |

**Using Grammar Presets (SDK):**

```go
d := model.D{
    "messages": model.DocumentArray(
        model.TextMessage(model.RoleUser, "List 3 languages in JSON"),
    ),
    "grammar": model.GrammarJSONObject,
}
```

**Using Grammar via API:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "List 3 languages in JSON"}],
    "grammar": "root ::= object\nvalue ::= object | array | string | number | \"true\" | \"false\" | \"null\"\nobject ::= \"{\" ws ( string \":\" ws value (\",\" ws string \":\" ws value)* )? ws \"}\"\narray ::= \"[\" ws ( value (\",\" ws value)* )? ws \"]\"\nstring ::= \"\\\"\" ([^\"\\\\] | \"\\\\\" [\"\\\\bfnrt/] | \"\\\\u\" [0-9a-fA-F]{4})* \"\\\"\"\nnumber ::= \"-\"? (\"0\" | [1-9][0-9]*) (\".\" [0-9]+)? ([eE] [+-]? [0-9]+)?\nws ::= [ \\t\\n\\r]*"
  }'
```

**JSON Schema Auto-Conversion:**

```go
schema := model.D{
    "type": "object",
    "properties": model.D{
        "name": model.D{"type": "string"},
        "year": model.D{"type": "integer"},
    },
    "required": []string{"name", "year"},
}

d := model.D{
    "messages": model.DocumentArray(...),
    "json_schema": schema,
    "enable_thinking": false,
}
```

Via API with `json_schema` field:

```json
{
  "model": "Qwen3-8B-Q8_0",
  "messages": [...],
  "json_schema": {
    "type": "object",
    "properties": {
      "name": {"type": "string"},
      "year": {"type": "integer"}
    },
    "required": ["name", "year"]
  },
  "enable_thinking": false
}
```

**Custom GBNF Grammars:**

```go
sentimentGrammar := `root ::= sentiment
sentiment ::= "positive" | "negative" | "neutral"`

d := model.D{
    "messages": model.DocumentArray(...),
    "grammar": sentimentGrammar,
    "enable_thinking": false,
}
```

**Important:** When using grammar constraints, set `enable_thinking: false` because the grammar applies from the first output token.

### 9.6 Logprobs (Token Probabilities)

Request log probabilities for generated tokens to understand model confidence
or implement custom sampling strategies.

**Request Parameters:**

| Parameter      | Type | Default | Description                           |
| -------------- | ---- | ------- | ------------------------------------- |
| `logprobs`     | bool | false   | Return log probability for each token |
| `top_logprobs` | int  | 0       | Number of top alternatives (0-5)      |

Setting `top_logprobs > 0` implicitly enables `logprobs`.

**Request:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [
      {"role": "user", "content": "What is 2+2?"}
    ],
    "logprobs": true,
    "top_logprobs": 3,
    "max_tokens": 10
  }'
```

**Response with Logprobs:**

```json
{
  "id": "chatcmpl-xxx",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "4"
      },
      "logprobs": {
        "content": [
          {
            "token": "4",
            "logprob": -0.0012,
            "bytes": [52],
            "top_logprobs": [
              { "token": "4", "logprob": -0.0012, "bytes": [52] },
              { "token": "The", "logprob": -6.82, "bytes": [84, 104, 101] },
              {
                "token": "Four",
                "logprob": -7.15,
                "bytes": [70, 111, 117, 114]
              }
            ]
          }
        ]
      },
      "finish_reason": "stop"
    }
  ]
}
```

**Response Structure:**

- `logprobs.content[]` - Array of per-token probability data
- `token` - The generated token string
- `logprob` - Log probability (always ≤ 0; closer to 0 = higher confidence)
- `bytes` - UTF-8 byte representation of the token
- `top_logprobs[]` - Alternative tokens with their probabilities

**Streaming Behavior:**

- **Streaming**: Logprobs sent in each delta chunk
- **Non-streaming**: All logprobs in final response

**Use Cases:**

- Confidence scoring for model outputs
- Detecting hallucinations (low probability sequences)
- Custom rejection sampling
- Token-level analysis for debugging

### 9.7 Parameter Reference

| Parameter          | JSON Key             | Type    | Default  | Description                          |
| ------------------ | -------------------- | ------- | -------- | ------------------------------------ |
| Temperature        | `temperature`        | float32 | 0.8      | Controls randomness of output        |
| Top-K              | `top_k`              | int32   | 40       | Limits token pool to K most probable |
| Top-P              | `top_p`              | float32 | 0.9      | Nucleus sampling threshold           |
| Min-P              | `min_p`              | float32 | 0.0      | Dynamic sampling threshold           |
| Max Tokens         | `max_tokens`         | int     | 4096     | Maximum tokens to generate           |
| Repeat Penalty     | `repeat_penalty`     | float32 | 1.0      | Penalty for repeated tokens          |
| Repeat Last N      | `repeat_last_n`      | int32   | 64       | Window for repetition check          |
| DRY Multiplier     | `dry_multiplier`     | float32 | 1.05     | N-gram repetition penalty            |
| DRY Base           | `dry_base`           | float32 | 1.75     | Exponential penalty base             |
| DRY Allowed Length | `dry_allowed_length` | int32   | 2        | Min n-gram length for DRY            |
| DRY Penalty Last N | `dry_penalty_last_n` | int32   | 0        | Recent tokens for DRY (0=all)        |
| XTC Probability    | `xtc_probability`    | float32 | 0.0      | XTC activation probability           |
| XTC Threshold      | `xtc_threshold`      | float32 | 0.1      | XTC probability threshold            |
| XTC Min Keep       | `xtc_min_keep`       | uint32  | 1        | Min tokens after XTC                 |
| Adaptive-P Target  | `adaptive_p_target`  | float32 | 0.0      | Adaptive sampling target             |
| Adaptive-P Decay   | `adaptive_p_decay`   | float32 | 0.0      | Adaptive adjustment speed            |
| Enable Thinking    | `enable_thinking`    | string  | "true"   | Enable model thinking                |
| Reasoning Effort   | `reasoning_effort`   | string  | "medium" | GPT reasoning level                  |
| Grammar            | `grammar`            | string  | ""       | GBNF grammar constraint              |
| Logprobs           | `logprobs`           | bool    | false    | Return token probabilities           |
| Top Logprobs       | `top_logprobs`       | int     | 0        | Number of top alternatives           |
| Stream             | `stream`             | bool    | false    | Stream response                      |
| Include Usage      | `include_usage`      | bool    | true     | Include usage in streaming           |
| Return Prompt      | `return_prompt`      | bool    | false    | Include prompt in response           |

---

## Chapter 10: Multi-Modal Models

Kronk supports vision and audio models that can process images, video frames,
and audio alongside text. This chapter covers how to use these models.

### 10.1 Overview

Multi-modal models combine a language model with a media projector that
converts images or audio into tokens the model can understand.

**Supported Media Types:**

- **Vision**: JPEG, PNG, GIF images
- **Audio**: WAV audio files

**Available Models (from catalog):**

```shell
kronk catalog list --filter-category=Image
kronk catalog list --filter-category=Audio
```

Example models:

- `Qwen2.5-VL-3B-Instruct-Q8_0` - Vision model
- `gemma-3-4b-it-q4_0` - Vision model
- `Qwen2-Audio-7B.Q8_0` - Audio model

### 10.2 Vision Models

Vision models analyze images and answer questions about their content.

**Download a Vision Model:**

```shell
kronk catalog pull Qwen2.5-VL-3B-Instruct-Q8_0
```

**API Request with Image (OpenAI Format):**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2.5-VL-3B-Instruct-Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "text",
            "text": "What do you see in this image?"
          },
          {
            "type": "image_url",
            "image_url": {
              "url": "data:image/jpeg;base64,/9j/4AAQSkZJRg..."
            }
          }
        ]
      }
    ]
  }'
```

**Content Array Structure:**

The `content` field is an array of content parts:

```json
{
  "content": [
    { "type": "text", "text": "Describe this image" },
    {
      "type": "image_url",
      "image_url": { "url": "data:image/jpeg;base64,..." }
    }
  ]
}
```

**Supported image_url Formats:**

- Base64 data URL: `data:image/jpeg;base64,/9j/4AAQSkZJRg...`
- Base64 data URL: `data:image/png;base64,iVBORw0KGgo...`

### 10.3 Audio Models

Audio models transcribe and understand spoken content.

**Download an Audio Model:**

```shell
kronk catalog pull Qwen2-Audio-7B.Q8_0
```

**API Request with Audio:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2-Audio-7B.Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "text",
            "text": "Transcribe this audio and summarize what is said."
          },
          {
            "type": "input_audio",
            "input_audio": {
              "data": "UklGRi...",
              "format": "wav"
            }
          }
        ]
      }
    ]
  }'
```

**Audio Format:**

- `data` - Base64-encoded audio data
- `format` - Audio format (currently `wav` supported)

### 10.4 Plain Base64 Format

For simpler integrations, Kronk also accepts plain base64 as the message
content (without the structured OpenAI format):

```json
{
  "model": "Qwen2.5-VL-3B-Instruct-Q8_0",
  "messages": [
    {
      "role": "user",
      "content": "/9j/4AAQSkZJRgABAQEASABIAAD..."
    }
  ]
}
```

Kronk auto-detects the media type from the binary header:

- JPEG: starts with `FF D8 FF`
- PNG: starts with `89 50 4E 47`
- WAV: starts with `RIFF`

### 10.5 Configuration for Multi-Modal Models

Vision and audio models have specific configuration requirements:

```yaml
models:
  Qwen2.5-VL-3B-Instruct-Q8_0:
    n_ubatch: 2048 # Higher for image token processing
    n_seq_max: 2 # Process up to 2 requests concurrently
    context_window: 8192
```

**Key Considerations:**

- `n_ubatch` should be high (≥2048) for efficient image/audio token processing
- `n_seq_max` controls batch parallelism (multiple slots in shared context)
- Vision/audio models use the same batch engine as text models

### 10.6 Memory Requirements

Vision and audio models require additional memory for the projector:

**Vision Model Example (Qwen2.5-VL-3B):**

```
Model weights:     ~3.5 GB
Projector:         ~0.5 GB
KV cache (8K):     ~0.4 GB
─────────────────────────
Total:             ~4.4 GB
```

**Audio Model Example (Qwen2-Audio-7B):**

```
Model weights:     ~8 GB
Projector:         ~0.8 GB
KV cache (8K):     ~0.6 GB
─────────────────────────
Total:             ~9.4 GB
```

### 10.7 Limitations

- Message caching (SPC/IMC) is not currently supported for vision/audio requests
- Processing time varies with image resolution and audio duration

### 10.8 Example: Image Analysis

Complete example analyzing an image:

```shell
# Encode image to base64
IMAGE_B64=$(base64 -i photo.jpg)

# Send request
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2.5-VL-3B-Instruct-Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Describe this image in detail."},
          {
            "type": "image_url",
            "image_url": {"url": "data:image/jpeg;base64,${IMAGE_B64}"}
          }
        ]
      }
    ],
    "max_tokens": 1024
  }'
```

### 10.9 Example: Audio Transcription

Complete example transcribing audio:

```shell
# Encode audio to base64
AUDIO_B64=$(base64 -i recording.wav)

# Send request
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2-Audio-7B.Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Transcribe this audio."},
          {
            "type": "input_audio",
            "input_audio": {"data": "${AUDIO_B64}", "format": "wav"}
          }
        ]
      }
    ],
    "max_tokens": 2048
  }'
```

---

_Next: [Chapter 11: Security & Authentication](#chapter-11-security--authentication)_

## Chapter 11: Security & Authentication

Kronk provides JWT-based authentication and authorization with per-endpoint
rate limiting. When enabled, all API requests require a valid token.

### 11.1 Enabling Authentication

**Start Server with Auth Enabled:**

```shell
kronk server start --auth-enabled
```

Or via environment variable:

```shell
export KRONK_AUTH_ENABLED=true
kronk server start
```

**First-Time Setup:**

On first startup with authentication enabled, Kronk automatically:

1. Creates a `keys/` directory in `~/.kronk/`
2. Generates a master private key (`master.pem`)
3. Creates an admin token (`master.jwt`) valid for 10 years
4. Generates an additional signing key for user tokens

The admin token is stored at `~/.kronk/keys/master.jwt`.

### 11.2 Using the Admin Token

The admin token is required for all security management operations.

**Set the Token:**

```shell
export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)
```

**Admin Capabilities:**

- Create new tokens for users
- Add and remove signing keys
- Access all endpoints without rate limits

### 11.3 Key Management

Private keys sign JWT tokens. Multiple keys allow token rotation without
invalidating all existing tokens.

**List Keys:**

```shell
kronk security key list
```

Output:

```
KEY ID                                  CREATED
master                                  2024-01-15T10:30:00Z
a1b2c3d4-e5f6-7890-abcd-ef1234567890    2024-01-15T10:30:00Z
```

**Create a New Key:**

```shell
kronk security key create
```

This generates a new UUID-named key for signing tokens.

**Delete a Key:**

```shell
kronk security key delete --keyid a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

**Important:** The master key cannot be deleted. Deleting a key invalidates
all tokens signed with that key.

**Local Mode:**

All key commands support `--local` to operate without a running server:

```shell
kronk security key list --local
kronk security key create --local
kronk security key delete --keyid <keyid> --local
```

### 11.4 Creating User Tokens

Create tokens with specific endpoint access and optional rate limits.

**Basic Syntax:**

```shell
kronk security token create \
  --duration <duration> \
  --endpoints <endpoint-list>
```

**Parameters:**

- `--duration` - Token lifetime (e.g., `1h`, `24h`, `720h`, `8760h`)
- `--endpoints` - Comma-separated list of endpoints with optional limits

**Endpoint Format:**

- `endpoint` - Unlimited access (default)
- `endpoint:unlimited` - Unlimited access (explicit)
- `endpoint:limit/window` - Rate limited

**Rate Limit Windows:**

- `day` - Resets daily
- `month` - Resets monthly
- `year` - Resets yearly

**Available Endpoints:**

- `chat-completions` - Chat completions API
- `responses` - Responses API
- `embeddings` - Embeddings API
- `rerank` - Reranking API
- `messages` - Anthropic Messages API

### 11.5 Token Examples

**Unlimited Access to All Endpoints (24 hours):**

```shell
kronk security token create \
  --duration 24h \
  --endpoints chat-completions,embeddings,rerank,responses,messages
```

**Rate-Limited Chat Token (30 days):**

```shell
kronk security token create \
  --duration 720h \
  --endpoints "chat-completions:1000/day,embeddings:500/day"
```

**Monthly Quota Token:**

```shell
kronk security token create \
  --duration 8760h \
  --endpoints "chat-completions:10000/month,embeddings:50000/month"
```

**Mixed Limits:**

```shell
kronk security token create \
  --duration 720h \
  --endpoints "chat-completions:100/day,embeddings:unlimited"
```

**Output:**

```
Token create
  Duration: 720h0m0s
  Endpoints: map[chat-completions:{1000 day} embeddings:{0 unlimited}]
TOKEN:
eyJhbGciOiJSUzI1NiIsImtpZCI6ImExYjJjM2Q0Li4uIiwidHlwIjoiSldUIn0...
```

### 11.6 Using Tokens in API Requests

Pass the token in the `Authorization` header with the `Bearer` prefix.

**curl Example:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer eyJhbGciOiJS..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

**Environment Variable Pattern:**

```shell
export KRONK_TOKEN="eyJhbGciOiJS..."

curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{...}'
```

**Python Example:**

```python
import openai

client = openai.OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="eyJhbGciOiJS..."  # Your Kronk token
)

response = client.chat.completions.create(
    model="Qwen3-8B-Q8_0",
    messages=[{"role": "user", "content": "Hello"}]
)
```

### 11.7 Authorization Flow

When a request arrives:

1. **Token Extraction** - Bearer token parsed from Authorization header
2. **Signature Verification** - Token signature verified against known keys
3. **Expiration Check** - Token must not be expired
4. **Endpoint Authorization** - Token must include the requested endpoint
5. **Rate Limit Check** - Request counted against endpoint quota
6. **Request Processing** - If all checks pass, request proceeds

**Error Responses:**

- `401 Unauthorized` - Missing, invalid, or expired token
- `403 Forbidden` - Token lacks access to the endpoint
- `429 Too Many Requests` - Rate limit exceeded

### 11.8 Rate Limiting

Rate limits are enforced per token (identified by the token's subject claim).

**How Limits Work:**

- Each token has a unique subject (UUID)
- Requests are counted per endpoint per subject
- Counters reset at window boundaries (day/month/year)

**Limit Storage:**

Rate limit counters are stored in a BadgerDB database at `~/.kronk/badger/`.
Counters persist across server restarts.

**Bypassing Rate Limits:**

Admin tokens (like `master.jwt`) bypass all rate limiting.

### 11.9 Configuration Reference

**Server Flags:**

- `--auth-enabled` - Enable authentication (env: `KRONK_AUTH_ENABLED`)
- `--auth-issuer` - JWT issuer name (env: `KRONK_AUTH_ISSUER`)
- `--auth-host` - External auth service host (env: `KRONK_AUTH_HOST`)

**Environment Variables:**

- `KRONK_TOKEN` - Token for CLI commands and API requests
- `KRONK_WEB_API_HOST` - Server address for CLI web mode
  (default: `localhost:8080`)

### 11.10 Security Best Practices

**Token Management:**

- Store admin tokens securely; treat `master.jwt` like a password
- Create separate tokens for different applications/users
- Use short durations for development tokens
- Rotate keys periodically for production deployments

**Rate Limiting:**

- Set appropriate limits based on expected usage
- Use daily limits for interactive applications
- Use monthly limits for batch processing

**Key Rotation:**

1. Create a new key: `kronk security key create`
2. Issue new tokens using the new key
3. Wait for old tokens to expire
4. Delete the old key: `kronk security key delete --keyid <old-keyid>`

**Production Checklist:**

- Enable authentication: `--auth-enabled`
- Secure the `~/.kronk/keys/` directory (mode 0700)
- Back up `master.pem` and `master.jwt` securely
- Distribute user tokens, never the admin token
- Monitor rate limit usage in logs

---

_Next: [Chapter 12: Browser UI (BUI)](#chapter-12-browser-ui-bui)_

## Chapter 12: Browser UI (BUI)

Kronk includes a web-based interface for managing models, libraries,
security, and server configuration without using the command line.

### 12.1 Accessing the BUI

The BUI is served from the same port as the API.

**Open in Browser:**

```
http://localhost:8080
```

The BUI automatically loads when you navigate to the server root.

### 12.2 Downloading Libraries

Before running inference, you need the llama.cpp libraries.

**Steps:**

1. Navigate to the **Libraries** page from the menu
2. Click **Pull Libraries**
3. Wait for the download to complete

The BUI auto-detects your platform (OS, architecture, GPU) and downloads
the appropriate binaries to `~/.kronk/libraries/`.

**Override Detection:**

If auto-detection is incorrect, you can specify:

- Processor type (CPU, CUDA, Metal, Vulkan)
- Architecture (amd64, arm64)
- Operating system

### 12.3 Downloading Models

**Browse the Catalog:**

1. Navigate to the **Catalog** page
2. Browse available models by category:
   - Text-Generation
   - Image-Text-to-Text (Vision)
   - Audio-Text-to-Text
   - Embedding
   - Reranking
3. Click **Pull** next to a model to download it

**Monitor Progress:**

The BUI shows real-time download progress including:

- Download percentage
- Transfer speed
- Estimated time remaining

**View Pulled Models:**

Navigate to the **Models** page to see all downloaded models and their
status.

### 12.4 Managing Keys and Tokens

When authentication is enabled, use the BUI to manage security.

**Keys Page:**

- View all signing keys with their IDs and creation dates
- Create new signing keys
- Delete keys (except master key)

**Tokens Page:**

- Generate new tokens with specific:
  - Duration (hours, days)
  - Endpoint access (chat-completions, embeddings, etc.)
  - Rate limits (requests per day/month/year)
- Copy generated tokens to clipboard

**Note:** You must provide an admin token in the BUI settings to access
security management features.

### 12.5 Other Screens

**Dashboard:**

Overview of server status, loaded models, and system information.

**Documentation:**

Built-in SDK and CLI documentation accessible from the menu:

- SDK API reference
- CLI command reference
- Example code

**Settings:**

Configure BUI preferences:

- API token for authenticated requests
- Theme preferences

---

_Next: [Chapter 13: Client Integration](#chapter-13-client-integration)_

## Chapter 13: Client Integration

Kronk's OpenAI-compatible API works with popular AI clients and tools.

### 13.1 OpenWebUI

OpenWebUI is a self-hosted chat interface that works with Kronk.

**Configure OpenWebUI:**

1. Open OpenWebUI settings
2. Navigate to Connections → OpenAI API
3. Set the base URL:

```
http://localhost:8080/v1
```

4. Set API key to your Kronk token (or any value (123) if auth is disabled)
5. Save and refresh models

**Features that work:**

- Chat completions with streaming
- Model selection from available models
- System prompts
- Conversation history

### 13.2 Cline

Cline is a VS Code extension for AI-assisted coding.

**Configure Cline for Kronk:**

1. Open VS Code settings
2. Search for "Cline"
3. Set API Provider to "OpenAI Compatible"
4. Configure:

```
Base URL: http://localhost:8080/v1
API Key: <your-kronk-token> or 123 for anything
Model: Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL/IMC
```

**Recommended Model Settings:**

For coding tasks, configure your model with:

```yaml
models:
    Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL:
    &base_Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL
    context-window: 131072
    nbatch: 2048
    nubatch: 512
    cache-type-k: q8_0
    cache-type-v: q8_0
    flash-attention: enabled
    nseq-max: 2
    insecure-logging: true
    sampling-parameters:
        temperature: 0.7
        top_p: 0.8
        top_k: 20

    Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL/IMC:
    <<: *base_Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL
    nseq-max: 1
    incremental-cache: true
```

IMC is especially beneficial for Cline's iterative coding workflow.

_Note: Don't use R1 Message formats when using KMS._

### 13.4 Python OpenAI SDK

Use the official OpenAI Python library with Kronk.

**Installation:**

```shell
pip install openai
```

**Usage:**

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="your-kronk-token"  # Or any string if auth disabled
)

response = client.chat.completions.create(
    model="Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL/IMC",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello!"}
    ],
    stream=True
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")
```

### 13.5 curl and HTTP Clients

Any HTTP client can call Kronk's REST API directly.

**Basic Request:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -d '{
    "model": "Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'
```

**Streaming Response:**

Streaming responses use Server-Sent Events (SSE) format:

```
data: {"id":"...","choices":[{"delta":{"content":"Hello"}}],...}

data: {"id":"...","choices":[{"delta":{"content":"!"}}],...}

data: [DONE]
```

### 13.6 LangChain

Use LangChain with Kronk via the OpenAI integration.

**Installation:**

```shell
pip install langchain-openai
```

**Usage:**

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:8080/v1",
    api_key="your-kronk-token",
    model="Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL",
    streaming=True
)

response = llm.invoke("Explain quantum computing briefly.")
print(response.content)
```

---

_Next: [Chapter 14: Observability](#chapter-14-observability)_

## Chapter 14: Observability

Kronk provides comprehensive observability through distributed tracing,
Prometheus metrics, pprof profiling, and real-time visualizations.

### 14.1 Debug Server

Kronk runs a separate debug server for observability endpoints, isolated
from the main API for security.

**Default Ports:**

- Main API: `localhost:8080`
- Debug server: `localhost:8090`

**Configure Debug Host:**

```shell
kronk server start --debug-host localhost:9090
```

Or via environment variable:

```shell
export KRONK_DEBUG_HOST=localhost:9090
kronk server start
```

### 14.2 Debug Endpoints

The debug server exposes these endpoints:

**Prometheus Metrics:**

```
http://localhost:8090/metrics
```

**pprof Profiling:**

- `http://localhost:8090/debug/pprof/` - Index page
- `http://localhost:8090/debug/pprof/profile` - CPU profile
- `http://localhost:8090/debug/pprof/heap` - Heap profile
- `http://localhost:8090/debug/pprof/goroutine` - Goroutine stacks
- `http://localhost:8090/debug/pprof/trace` - Execution trace

**Statsviz (Real-time Visualizations):**

```
http://localhost:8090/debug/statsviz
```

Provides live charts for memory, goroutines, GC, and more.

### 14.3 Health Check Endpoints

Available on the main API port (no authentication required):

**Liveness Check:**

```shell
curl http://localhost:8080/v1/liveness
```

Response:

```json
{
  "status": "up",
  "build": "v1.0.0",
  "host": "hostname",
  "GOMAXPROCS": 8
}
```

**Readiness Check:**

```shell
curl http://localhost:8080/v1/readiness
```

Returns 200 OK when the server is ready to accept requests.

### 14.4 Prometheus Metrics

Kronk exposes detailed inference metrics in Prometheus format.

**Fetch Metrics:**

```shell
curl http://localhost:8090/metrics
```

**Available Metrics:**

System metrics:

- `goroutines` - Current goroutine count
- `requests` - Total request count
- `errors` - Total error count
- `panics` - Total panic count

Model loading (in seconds):

- `model_load_avg`, `model_load_min`, `model_load_max`
- `model_load_proj_avg`, `model_load_proj_min`, `model_load_proj_max`

Inference timing (in seconds):

- `model_prompt_creation_avg`, `_min`, `_max`
- `model_prefill_avg`, `_min`, `_max`
- `model_ttft_avg`, `_min`, `_max` (time to first token)

Token usage:

- `usage_prompt_tokens_avg`, `_min`, `_max`
- `usage_reasoning_tokens_avg`, `_min`, `_max`
- `usage_completion_tokens_avg`, `_min`, `_max`
- `usage_output_tokens_avg`, `_min`, `_max`
- `usage_total_tokens_avg`, `_min`, `_max`
- `usage_tokens_per_second_avg`, `_min`, `_max`

### 14.5 Prometheus Integration

**Example Prometheus Configuration:**

```yaml
# prometheus.yml
scrape_configs:
  - job_name: "kronk"
    static_configs:
      - targets: ["localhost:8090"]
    scrape_interval: 15s
```

**Grafana Dashboard Query Examples:**

Time to first token:

```promql
model_ttft_avg
```

Tokens per second throughput:

```promql
usage_tokens_per_second_avg
```

Request rate:

```promql
rate(requests[5m])
```

Error rate:

```promql
rate(errors[5m]) / rate(requests[5m])
```

### 14.6 Distributed Tracing with Tempo

Kronk supports OpenTelemetry tracing with Grafana Tempo integration.

**Enable Tracing:**

```shell
kronk server start \
  --tempo-host localhost:4317 \
  --tempo-service-name kronk \
  --tempo-probability 0.25
```

Or via environment variables:

```shell
export KRONK_TEMPO_HOST=localhost:4317
export KRONK_TEMPO_SERVICE_NAME=kronk
export KRONK_TEMPO_PROBABILITY=0.25
kronk server start
```

**Configuration Options:**

- `--tempo-host` - Tempo collector address (OTLP gRPC endpoint)
- `--tempo-service-name` - Service name in traces (default: `kronk`)
- `--tempo-probability` - Sampling probability 0.0-1.0 (default: `0.25`)

**Sampling Probability:**

- `1.0` - Trace every request (development only)
- `0.25` - Trace 25% of requests (recommended for production)
- `0.05` - Trace 5% of requests (high-traffic production)

**Excluded Routes:**

Health check endpoints are automatically excluded from tracing:

- `/v1/liveness`
- `/v1/readiness`

### 14.7 Tracing Architecture

**Request Flow with Tracing:**

```
Client Request
      │
      ▼
┌─────────────────────────────┐
│  Kronk Server               │
│  ┌───────────────────────┐  │
│  │ Inject Trace Context  │  │
│  │ (trace_id, span_id)   │  │
│  └───────────┬───────────┘  │
│              ▼              │
│  ┌───────────────────────┐  │
│  │ Handler Span          │  │
│  │ (chat, embed, etc.)   │  │
│  └───────────┬───────────┘  │
│              ▼              │
│  ┌───────────────────────┐  │
│  │ Inference Span        │  │
│  │ (model operations)    │  │
│  └───────────────────────┘  │
└─────────────────────────────┘
      │
      ▼
   Tempo Collector (OTLP gRPC)
      │
      ▼
   Grafana (Visualization)
```

**What Gets Traced:**

- HTTP request handling
- Model acquisition from pool
- Prefill and generation phases
- Token streaming

### 14.8 Tempo Setup with Docker

**Run Tempo Locally:**

```shell
docker run -d --name tempo \
  -p 3200:3200 \
  -p 4317:4317 \
  grafana/tempo:latest \
  -config.file=/etc/tempo/tempo.yaml
```

**Run Grafana:**

```shell
docker run -d --name grafana \
  -p 3000:3000 \
  grafana/grafana:latest
```

**Configure Grafana:**

1. Open http://localhost:3000 (admin/admin)
2. Add data source → Tempo
3. Set URL: `http://tempo:3200`
4. Save and explore traces

### 14.9 pprof Profiling

Use Go's pprof tools for performance analysis.

**Capture CPU Profile (30 seconds):**

```shell
go tool pprof http://localhost:8090/debug/pprof/profile?seconds=30
```

**Capture Heap Profile:**

```shell
go tool pprof http://localhost:8090/debug/pprof/heap
```

**View Goroutine Stacks:**

```shell
curl http://localhost:8090/debug/pprof/goroutine?debug=2
```

**Generate Flame Graph:**

```shell
go tool pprof -http=:8081 \
  http://localhost:8090/debug/pprof/profile?seconds=30
```

Opens interactive web UI with flame graph visualization.

### 14.10 Statsviz Real-Time Monitoring

Statsviz provides live runtime visualizations in your browser.

**Access Statsviz:**

```
http://localhost:8090/debug/statsviz
```

**Available Charts:**

- Heap size and allocations
- Goroutine count
- GC pause times
- CPU scheduler latency
- Memory by size class

Useful for real-time monitoring during load testing or debugging
memory issues.

### 14.11 Logging

Kronk logs structured JSON to stdout by default.

**Log Levels:**

Logs include context like trace IDs, request details, and timing.

**Insecure Logging:**

For debugging, enable verbose logging that includes message content:

```shell
kronk server start --insecure-logging
```

**Warning:** Insecure logging exposes user prompts and model responses.
Never enable in production.

**Environment Variable:**

```shell
export KRONK_INSECURE_LOGGING=true
```

### 14.12 Configuration Reference

**Debug Server:**

- `--debug-host` - Debug server address (env: `KRONK_DEBUG_HOST`,
  default: `localhost:8090`)

**Tracing:**

- `--tempo-host` - Tempo collector address (env: `KRONK_TEMPO_HOST`,
  default: `localhost:4317`)
- `--tempo-service-name` - Service name (env: `KRONK_TEMPO_SERVICE_NAME`,
  default: `kronk`)
- `--tempo-probability` - Sampling rate 0.0-1.0
  (env: `KRONK_TEMPO_PROBABILITY`, default: `0.25`)

**Logging:**

- `--insecure-logging` - Log message content
  (env: `KRONK_INSECURE_LOGGING`, default: `false`)
- `--llama-log` - llama.cpp log level, 0=off, 1=on
  (env: `KRONK_LLAMA_LOG`, default: `1`)

---

_Next: [Chapter 15: MCP Service](#chapter-15-mcp-service)_

## Chapter 15: MCP Service

Kronk includes a built-in [Model Context Protocol (MCP)](https://modelcontextprotocol.io/)
service that exposes tools to MCP-compatible clients. The initial tool
provided is `web_search`, powered by the [Brave Search API](https://brave.com/search/api/).

MCP is an open standard that lets AI agents call external tools over a
simple JSON-RPC protocol. By running the MCP service, any MCP-compatible
client (Cline, Kilo Code, Cursor, etc.) can discover and invoke tools
served by Kronk.

### 15.1 Architecture

The MCP service can run in two modes:

**Embedded (default)** — When the Kronk model server starts and no external
MCP host is configured (`--mcp-host` is empty), it automatically starts an
embedded MCP server on `localhost:9000`. No extra process is needed.

**Standalone** — Run the MCP service as its own process for independent
scaling or when you don't need the full model server:

```shell
make mcp-server
```

Or directly:

```shell
go run cmd/server/api/services/mcp/main.go
```

Both modes serve the same MCP protocol on the same default port (`9000`).

### 15.2 Prerequisites

The `web_search` tool requires a Brave Search API key. Get a free key at
[https://brave.com/search/api/](https://brave.com/search/api/).

### 15.3 Configuration

**Environment Variables:**

| Variable          | Description                                 | Default          |
| ----------------- | ------------------------------------------- | ---------------- |
| `MCP_MCP_HOST`    | MCP listen address (standalone mode)        | `localhost:9000` |
| `MCP_MCP_BRAVEAPIKEY` | Brave Search API key (standalone mode)  | —                |
| `KRONK_MCP_HOST`  | External MCP host (empty = embedded mode)   | —                |
| `KRONK_MCP_BRAVEAPIKEY` | Brave Search API key (embedded mode)  | —                |

**Embedded mode** — Pass the Brave API key when starting the Kronk server:

```shell
export KRONK_MCP_BRAVEAPIKEY=<your-brave-api-key>
kronk server start
```

The embedded MCP server will start automatically on `localhost:9000`.

**Standalone mode** — Start the MCP service as a separate process:

```shell
export MCP_MCP_BRAVEAPIKEY=<your-brave-api-key>
make mcp-server
```

### 15.4 Available Tools

#### web_search

Performs a web search and returns a list of relevant web pages with titles,
URLs, and descriptions.

**Parameters:**

| Parameter    | Type   | Required | Description                                                   |
| ------------ | ------ | -------- | ------------------------------------------------------------- |
| `query`      | string | Yes      | Search query                                                  |
| `count`      | int    | No       | Number of results to return (default 10, max 20)              |
| `country`    | string | No       | Country code for search context (e.g. `US`, `GB`, `DE`)       |
| `freshness`  | string | No       | Filter by freshness: `pd` (past day), `pw` (past week), `pm` (past month), `py` (past year) |
| `safesearch` | string | No       | Safe search filter: `off`, `moderate`, `strict` (default `moderate`) |

### 15.5 Client Configuration

The MCP service uses the Streamable HTTP transport. Configure your
MCP-compatible client to connect to `http://localhost:9000`.

#### Cline

Add the following to your Cline MCP settings:

```json
{
  "mcpServers": {
    "Kronk": {
      "autoApprove": [
        "web_search"
      ],
      "disabled": false,
      "timeout": 60,
      "type": "streamableHttp",
      "url": "http://localhost:9000"
    }
  }
}
```

#### Kilo Code

Add the following to your Kilo Code MCP settings:

```json
{
  "mcpServers": {
    "Kronk": {
      "type": "streamable-http",
      "url": "http://localhost:9000",
      "disabled": true,
      "alwaysAllow": [
        "web_search"
      ],
      "timeout": 60
    }
  }
}
```

### 15.6 Testing with curl

You can test the MCP service manually using curl. See the makefile targets
for convenience commands.

**Initialize a session:**

```shell
make curl-mcp-init
```

This returns the `Mcp-Session-Id` header needed for subsequent requests.

**List available tools:**

```shell
make curl-mcp-tools-list SESSIONID=<session-id>
```

**Call web_search:**

```shell
make curl-mcp-web-search SESSIONID=<session-id>
```

---

_Next: [Chapter 16: Troubleshooting](#chapter-16-troubleshooting)_

## Chapter 16: Troubleshooting

This chapter covers common issues, their causes, and solutions.

### 16.1 Library Issues

**Error: "unable to load library"**

The llama.cpp libraries are missing or incompatible.

**Solution:**

```shell
kronk libs --local
```

Or download via the BUI Libraries page.

**Error: "unknown device"**

The specified GPU device is not available.

**Causes:**

- Wrong `--device` flag (e.g., `cuda` on a Mac)
- GPU drivers not installed
- Library mismatch (CPU library with GPU device setting)

**Solution:**

Check your hardware and install matching libraries:

```shell
# For Mac with Apple Silicon
KRONK_PROCESSOR=metal kronk libs --local

# For NVIDIA GPU
KRONK_PROCESSOR=cuda kronk libs --local

# For CPU only
KRONK_PROCESSOR=cpu kronk libs --local
```

### 16.2 Model Loading Failures

**Error: "unable to load model"**

The model file is missing, corrupted, or incompatible.

**Check model exists:**

```shell
ls ~/.kronk/models/
```

**Re-download the model:**

```shell
kronk catalog pull <model-name> --local
```

**Verify model integrity:**

By default, Kronk skips integrity checks. To force verification:

```shell
kronk server start --ignore-integrity-check=false
```

**Error: "failed to retrieve model template"**

The model's chat template is missing.

**Solution:**

Ensure templates are downloaded:

```shell
kronk catalog pull-templates --local
```

### 16.3 Memory Errors

**Error: "unable to init context" or "unable to get memory"**

Insufficient memory for the model configuration.

**Causes:**

- Context window too large
- Too many batch slots
- Model too large for available RAM/VRAM

**Solutions:**

Reduce context window:

```yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 8192 # Reduce from 32768
```

Reduce batch parallelism:

```yaml
models:
  Qwen3-8B-Q8_0:
    n_seq_max: 1 # Single request at a time
```

Use quantized KV cache:

```yaml
models:
  Qwen3-8B-Q8_0:
    cache-type-k: q8_0 # Saves ~50% KV cache memory
    cache-type-v: q8_0
```

**Error: "context window is full"**

The request plus context exceeds the configured context window.

**Solutions:**

- Reduce input size (fewer messages or shorter prompts)
- Increase `context_window` in model config
- Enable YaRN for extended context (see Chapter 6)

### 16.4 Request Timeouts

**Error: "context deadline exceeded"**

The request took longer than the configured timeout.

**Causes:**

- Model too slow for the request size
- Large prefill with many tokens
- Server under heavy load

**Solutions:**

Increase HTTP timeouts:

```shell
kronk server start \
  --read-timeout 5m \
  --write-timeout 30m
```

Or via environment variables:

```shell
export KRONK_READ_TIMEOUT=5m
export KRONK_WRITE_TIMEOUT=30m
```

### 16.5 Authentication Errors

**Error: "unauthorized: no authorization header"**

Authentication is enabled but no token was provided.

**Solution:**

Include the Authorization header:

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $(cat ~/.kronk/keys/master.jwt)" \
  -H "Content-Type: application/json" \
  -d '{...}'
```

**Error: "invalid token"**

The token is malformed, expired, or signed with an unknown key.

**Causes:**

- Token has expired (check `--duration` when created)
- Signing key was deleted
- Token is corrupted

**Solution:**

Create a new token:

```shell
export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)
kronk security token create \
  --duration 720h \
  --endpoints chat-completions,embeddings
```

**Error: "endpoint not authorized"**

The token doesn't include the requested endpoint.

**Solution:**

Create a new token with the required endpoints:

```shell
kronk security token create \
  --duration 720h \
  --endpoints chat-completions,embeddings,rerank,responses,messages
```

**Error: "rate limit exceeded"**

The token has exceeded its rate limit.

**Solution:**

Wait for the rate limit window to reset, or create a new token with
higher limits:

```shell
kronk security token create \
  --duration 720h \
  --endpoints "chat-completions:10000/day"
```

### 16.6 Streaming Issues

**Problem: Streaming stops mid-response**

**Causes:**

- Client disconnected
- Request timeout
- Model generated stop token

**Check server logs:**

```shell
# Look for errors in server output
kronk server start  # Run in foreground to see logs
```

**Problem: SSE events not parsing correctly**

Ensure your client handles Server-Sent Events format:

```
data: {"id":"...","choices":[...]}\n\n
```

Each event is prefixed with `data: ` and ends with two newlines.

### 16.7 Performance Issues

**Problem: Slow time to first token (TTFT)**

**Causes:**

- Large system prompt not cached
- No message caching enabled
- Cold model load

**Solutions:**

Enable system prompt caching:

```yaml
models:
  Qwen3-8B-Q8_0:
    system_prompt_cache: true
```

Or enable incremental message cache for agents:

```yaml
models:
  Qwen3-8B-Q8_0:
    incremental_cache: true
```

**Problem: Slow token generation (tokens/second)**

**Causes:**

- CPU inference instead of GPU
- Insufficient GPU layers
- Large model for available hardware

**Solutions:**

Check GPU is being used:

```shell
# On macOS, check Metal usage
sudo powermetrics --samplers gpu_power

# On Linux with NVIDIA
nvidia-smi
```

Increase GPU layers:

```yaml
models:
  Qwen3-8B-Q8_0:
    gpu_layers: 99 # Offload all layers to GPU
```

### 16.8 Viewing Logs

**Run server in foreground:**

```shell
kronk server start
```

All logs print to stdout with structured JSON format.

**Enable verbose logging:**

```shell
kronk server start --insecure-logging
```

This logs full message content (never use in production).

**Enable llama.cpp logging:**

```shell
kronk server start --llama-log 1
```

Shows low-level inference engine messages.

**Disable llama.cpp logging:**

```shell
kronk server start --llama-log 0
```

### 16.9 Common Error Messages

| Error                  | Cause                  | Solution               |
| ---------------------- | ---------------------- | ---------------------- |
| `Init() not called`    | Missing initialization | Call `kronk.Init()`    |
| `unknown device`       | Invalid GPU setting    | Check `--device` flag  |
| `context deadline`     | Request timeout        | Increase timeouts      |
| `unable to load model` | Missing/corrupt model  | Re-download model      |
| `no authorization`     | Missing token          | Add Bearer token       |
| `rate limit exceeded`  | Quota exhausted        | Wait or increase limit |
| `context window full`  | Input too large        | Reduce input size      |
| `NBatch overflow`      | Batch too large        | Reduce `n_batch`       |

### 16.10 Getting Help

**Check server status:**

```shell
curl http://localhost:8080/v1/liveness
```

**List loaded models:**

```shell
curl http://localhost:8080/v1/models
```

**Check metrics:**

```shell
curl http://localhost:8090/metrics
```

**View goroutine stacks (for hangs):**

```shell
curl http://localhost:8090/debug/pprof/goroutine?debug=2
```

**Report issues:**

Include the following when reporting bugs:

- Kronk version (`kronk --version`)
- Operating system and architecture
- GPU type and driver version
- Model name and configuration
- Full error message and stack trace
- Steps to reproduce

---

_Next: [Chapter 17: Developer Guide](#chapter-17-developer-guide)_

## Chapter 17: Developer Guide

This chapter covers development workflows, build commands, and code
conventions for contributors to the Kronk project.

### 17.1 Quick Reference

Here is a quick chart of some of the more imporant make commands.

| Task            | Command                                             |
| --------------- | --------------------------------------------------- |
| Install CLI     | `make install-kronk`.                               |
| Run all tests   | `make test`                                         |
| Single test     | `go test -v -count=1 -run TestName ./sdk/kronk/...` |
| Run server      | `make kronk-server`                                 |
| Build BUI       | `make bui-build`                                    |
| Generate docs   | `make kronk-docs`                                   |
| Tidy modules    | `make tidy`                                         |
| Update deps     | `make deps-upgrade`                                 |
| Lint            | `staticcheck ./...`                                 |
| Developer setup | `make setup` (configures git hooks)                 |

### 17.2 Build & Test Commands

**Install CLI locally:**

```shell
go install ./cmd/kronk
```

**Run all tests:**

```shell
make test
```

Tests require prerequisites and environment variables:

```shell
# Install dependencies first
make install-libraries install-models

# Set required environment variables
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE=/path/to/kronk  # project root

# Run from project root directory
make test
```

**Run a single test:**

```shell
go test -v -count=1 -run TestName ./sdk/kronk/...
```

### 17.3 Developer Setup

Configure git hooks for automatic pre-commit checks:

```shell
make setup
```

This enables a pre-commit hook that automatically runs:

- `make kronk-docs` - Regenerates documentation
- `make bui-build` - Rebuilds the BUI frontend

### 17.4 Project Architecture

**Directory Structure:**

| Directory                      | Purpose                                                             |
| ------------------------------ | ------------------------------------------------------------------- |
| `cmd/kronk/`                   | CLI tool (subcommands: catalog, libs, model, run, security, server) |
| `cmd/server/`                  | OpenAI-compatible model server (gRPC + HTTP) with BUI frontend      |
| `cmd/server/api/tooling/docs/` | Documentation generator for BUI (SDK and CLI docs)                  |
| `sdk/kronk/`                   | Core API: model loading, chat, embeddings, cache, metrics           |
| `sdk/kronk/model/`             | Core inference and caching engine                                   |
| `sdk/kronk/observ/`            | Observability packages (metrics/, otel/)                            |
| `sdk/tools/`                   | Support for libs, models, catalogs, templates, and defaults         |

**Core Technology:**

Kronk uses [yzma](https://github.com/hybridgroup/yzma) (llama.cpp Go bindings)
for local inference with GGUF models.

### 17.5 BUI Frontend Development

The Browser UI is a React application located at:

```
cmd/server/api/frontends/bui/src/
```

**Directory Structure:**

| Directory/File | Purpose                                         |
| -------------- | ----------------------------------------------- |
| `components/`  | React components (pages and UI elements)        |
| `contexts/`    | React context providers for shared state        |
| `services/`    | API client (`api.ts`)                           |
| `types/`       | TypeScript type definitions                     |
| `App.tsx`      | Main app with routing configuration             |
| `index.css`    | Global styles (CSS variables, component styles) |

**Routing:**

Uses `react-router-dom` with `BrowserRouter`. Routes are defined in
`routeMap` in `App.tsx`.

**Adding New Pages:**

1. Create component in `components/` (e.g., `DocsSDKKronk.tsx`)
2. Add page type to `Page` union in `App.tsx`
3. Add route path to `routeMap` in `App.tsx`
4. Add `<Route>` element in `App.tsx`
5. Add `<Link>` entry to menu in `components/Layout.tsx`

**Menu Structure (`Layout.tsx`):**

Uses `MenuCategory[]` with properties:

- `id` - Unique identifier
- `label` - Display text
- `items` - Array of leaf pages
- `subcategories` - Nested menu categories

**State Management:**

| Context            | Purpose                                               |
| ------------------ | ----------------------------------------------------- |
| `TokenContext`     | Stores API token in localStorage (key: `kronk_token`) |
| `ModelListContext` | Caches model list data with invalidation support      |

Access via hooks: `useToken()`, `useModelList()`

**API Service (`services/api.ts`):**

- `ApiService` class with methods for all endpoints
- Streaming support for pull operations (models, catalog, libs)
- Auth-required endpoints accept token parameter

**Styling Conventions:**

- CSS variables defined in `:root` (colors: `--color-orange`, `--color-blue`, etc.)
- Common classes: `.card`, `.btn`, `.btn-primary`, `.form-group`, `.alert`, `.table-container`
- No CSS modules or styled-components; use global CSS classes

**Documentation Generation:**

| Type     | Generator Location                                            |
| -------- | ------------------------------------------------------------- |
| SDK docs | `cmd/server/api/tooling/docs/sdk/` (uses `go doc` output)     |
| CLI docs | `cmd/server/api/tooling/docs/cli/` (from command definitions) |
| Examples | Auto-generated from `examples/` directory                     |

Generate all documentation:

```shell
go run ./cmd/server/api/tooling/docs -pkg=all
```

### 17.6 Code Style Guidelines

**Package Comments:**

```go
// Package kronk provides the core inference API.
```

**Error Handling:**

```go
// Wrap errors with lowercase context prefix
return fmt.Errorf("loading model: %w", err)

// Declare package-level sentinel errors
var ErrModelNotFound = errors.New("model not found")
```

**Struct Design:**

- Use unexported fields with exported types
- Use `Config` pattern for constructors

```go
type Config struct {
    Host string
    Port int
}

func New(cfg Config) *Server {
    // ...
}
```

**Testing:**

Disable CGO in tests:

```shell
CGO_ENABLED=0 go test ./...
```

**Import Order (goimports):**

1. Standard library
2. External packages
3. Internal packages

**Control Flow:**

- Avoid `else` and `else if` clauses
- Prefer `switch` statements or early returns

```go
// Preferred: early return
if err != nil {
    return err
}
// continue with main logic

// Preferred: switch over if-else chains
switch state {
case "active":
    // ...

case "pending":
    // ...

default:
    // ...
}
```

### 17.7 SDK Internals

This section documents implementation details for developers working on
the Kronk SDK packages.

#### 17.7.1 Package Structure

**sdk/kronk/** - Core API package:

| File             | Purpose                                |
| ---------------- | -------------------------------------- |
| `acquire.go`     | Model pool acquire/release             |
| `chat.go`        | Chat completion API                    |
| `concurrency.go` | Generic streaming utilities            |
| `embedding.go`   | Embedding API                          |
| `init.go`        | Initialization and configuration       |
| `kronk.go`       | Main Kronk type, model pool management |
| `rerank.go`      | Reranking API                          |
| `response.go`    | OpenAI Responses API streaming         |

**sdk/kronk/model/** - Low-level inference:

| File           | Purpose                                               |
| -------------- | ----------------------------------------------------- |
| `batch.go`     | Batch engine for parallel text inference              |
| `caching.go`   | System prompt and IMC cache management                |
| `chat.go`      | Chat inference loop, batch routing                    |
| `config.go`    | Model configuration (GPU, cache, batching)            |
| `embed.go`     | Embedding inference                                   |
| `logprobs.go`  | Token log probability extraction                      |
| `media.go`     | Vision/audio media processing                         |
| `model.go`     | Model type, context management, lifecycle             |
| `models.go`    | OpenAI-compatible types (ChatMessage, ToolCall, etc.) |
| `params.go`    | Sampling parameters                                   |
| `processor.go` | Template-specific token processors                    |
| `prompts.go`   | Prompt formatting                                     |
| `rerank.go`    | Reranking inference                                   |

#### 17.7.2 Streaming Architecture

**Response Streaming Pattern** (`response.go`, `concurrency.go`):

- Uses `streamingWith[T, U]` generic function for 1:N event transformation
- `streamProcessor` has three phases: `Start()`, `Process(chunk)`, `Complete(lastChunk)`
- `streamState` struct maintains response ID, sequence numbers, aggregated usage
- SSE format: `event: <type>\ndata: <json>\n\n`

**FinishReason Handling:**

- `FinishReasonPtr *string` field with `FinishReason()` accessor
- Constants: `FinishReasonStop="stop"`, `FinishReasonTool="tool_calls"`, `FinishReasonError="error"`
- When `FinishReasonPtr != nil`, skip text/reasoning deltas (they duplicate previous content)
- Always process tool calls even with FinishReason set (may only arrive in final chunk)

#### 17.7.3 Concurrency Strategy

`NSeqMax` behaves differently depending on model type:

**Embedding and Reranking Models**:

- `NSeqMax` controls the internal context pool size
- Model weights are shared, only KV cache memory is multiplied
- Inputs within a request are partitioned across pool contexts for parallel processing
- Semaphore capacity = `NSeqMax`

**Text Inference Models** (chat, completion, vision, audio):

- `NSeqMax` controls batch parallelism within the batch engine
- Only one `model.Model` instance is created with multiple slots
- Semaphore capacity = `NSeqMax * queueDepth` (default queueDepth=2)

**Detection Logic** (`kronk.go`):

```go
switch {
case mi.IsEmbedModel || mi.IsRerankModel:
    semCapacity = max(cfg.NSeqMax, 1)
default:
    semCapacity = max(cfg.NSeqMax, 1) * o.queueDepth
}
```

#### 16.7.4 Model Acquire/Release & Cleanup

**Acquisition** (`acquire.go`):

1. **Backpressure slot**: Acquire semaphore slot (limits total in-flight requests)
2. **Return model**: Return the single model instance

**Cleanup Flow:**

1. `streaming()` acquires model, defers `releaseModel()` in wrapper goroutine
2. `ChatStreaming` defers `m.resetContext()` before any processing
3. When generation completes, `resetContext()` runs first:
   - `llama.Synchronize(m.lctx)` - waits for GPU operations
   - `llama.MemoryClear(mem, true)` - clears KV cache
4. Channel closes, wrapper exits, `releaseModel()` runs

**Key invariant:** `resetContext()` always runs before model release due to defer ordering.

#### 16.7.5 Batch Engine Internals

**ChatStreaming Decision Logic** (`chat.go`):

The `submitToBatchEngine()` function decides the processing path:

```go
// submitToBatchEngine returns false if batch not available.
if m.batch == nil || object != ObjectChatText {
    return false
}
// Submit job to batch engine...
return true
```

All chat requests (including vision/audio) are submitted to the batch engine:

```go
m.submitToBatchEngine(...)
batching = true
```

**Batch Engine Architecture** (`batch.go`):

- `batchEngine` manages `nSlots` parallel `slot` structs
- Each slot tracks: `seqID`, prompt tokens, decode state, sampler, response channel, logprobs, prefill state
- Signal-based wake pattern: `wakeCh chan struct{}` (buffered size 1) wakes immediately on new requests
- Polling intervals: 100µs (active slots generating), 5ms (idle, no active slots)

**Slots vs Sequences:**

- `slot.id` = slot index (for logging)
- `slot.seqID` = llama.cpp sequence ID (determines KV cache partition)
- `slot.seqIDs` = pre-allocated slice for efficient `batchAdd` calls

Sequences are isolated partitions in the shared KV cache memory. Slot seqIDs
always start at 0 — no sequences are reserved for caching. SPC decodes
saved tokens directly into slot sequences. IMC binds each slot's sequence
to a conversation.

#### 16.7.6 Context Pooling

- `llama.Context` is created once in `NewModel` and reused across requests
- Call `resetContext()` between requests to clear KV cache
- Avoids Vulkan memory fragmentation from repeated context alloc/dealloc

#### 16.7.7 IMC Implementation Details

**Critical Implementation Details:**

1. **Extension tokenization must use `special=true`**: Use `llama.Tokenize(vocab, extension, false, true)` to ensure ChatML tokens like `<|im_start|>` are recognized.

2. **Prefix mismatch detection**: Use `strings.HasPrefix(fullPrompt, prefixPrompt)` to detect Jinja template nondeterminism.

3. **`add_generation_prompt=false` for cached prefixes**: Creates valid prefix for extension. Generation prompt added only for final suffix.

**IMC Algorithm:**

1. First request (cache empty): Cache `messages[0:len-1]`, generate from last message
2. Subsequent requests (prefix match): Extend cache with `messages[cachedCount:len-1]`
3. New thread (prefix mismatch): Rebuild cache from scratch

**IMC Session State:**

```go
type imcSession struct {
    hash      string      // Hash of all cached messages
    tokens    int         // Total tokens in cache
    msgCount  int         // Number of messages cached
    promptLen int         // Length of templated prefix
    seqID     llama.SeqId // Assigned cache sequence ID
    lastUsed  time.Time   // For future eviction
}
```

#### 16.7.8 Tool Call Internals

**chatMessage Unmarshaling** (`models.go`):

- `Content` can be `nil` for assistant messages with tool_calls
- Handle `len(app.Content) == 0 || string(app.Content) == "null"` as valid empty content

**ToolCallArguments Type:**

- Custom type that marshals to JSON string (OpenAI spec)
- Unmarshals from either string or object for non-compliant clients

#### 16.7.9 Logprobs Implementation

**Implementation** (`logprobs.go`):

- `extractLogprobs()`: Retrieves logits via `llama.GetLogitsIth()`
- `logSoftmax()`: Numerically stable log-softmax using log-sum-exp trick
- `getTopKLogprobs()`: Uses min-heap for efficient O(n log k) top-k extraction

**Critical:** Logprobs must be extracted **before** `llama.SamplerAccept()` is called.

### 17.8 API Handler Notes

**Input Format Conversion** (`cmd/server/app/domain/`):

Both streaming and non-streaming Response APIs must call
`convertInputToMessages(d)` to handle the OpenAI Responses `input` field
format.

### 17.9 Goroutine Budget

A running Kronk server typically shows ~25 baseline goroutines before any
requests arrive. When requests are active, expect roughly 3-5 additional
goroutines per in-flight request. For example, 3 concurrent requests for the
same model will show ~40 goroutines total. This is normal.

**Baseline goroutines (~25, always running):**

| Source                                         | Goroutines | Location                                 |
| ---------------------------------------------- | ---------- | ---------------------------------------- |
| Go runtime (GC, finalizer, netpoller, etc.)    | ~4-6       | runtime internals                        |
| API `http.Server` (listener + idle conns)      | ~3         | `cmd/server/api/services/kronk/kronk.go` |
| Debug `http.Server` (pprof, metrics, statsviz) | ~3         | `cmd/server/api/services/kronk/kronk.go` |
| `statsviz.Register` (websocket handler)        | ~2         | `cmd/server/app/sdk/debug/debug.go`      |
| gRPC auth server (`gs.Serve`)                  | ~2-3       | `cmd/server/app/domain/authapp/start.go` |
| OTEL background collector probe                | 1          | `sdk/kronk/observ/otel/otel.go`          |
| `otelhttp.NewHandler` internals                | ~1-2       | `cmd/server/foundation/web/web.go`       |
| Batch engine `processLoop`                     | 1          | `sdk/kronk/model/batch.go`               |

**Per-request goroutines (~3-5 each):**

| Source                                                    | Location                   |
| --------------------------------------------------------- | -------------------------- |
| `http.Server` connection handler                          | Go stdlib                  |
| `ChatStreaming` request goroutine                         | `sdk/kronk/model/chat.go`  |
| `streaming()` wrapper goroutine                           | `sdk/kronk/concurrency.go` |
| `wrapChannelForLogging` (only if `InsecureLogging` is on) | `sdk/kronk/model/chat.go`  |

The goroutine metric is a point-in-time snapshot from `runtime.NumGoroutine()`
captured every 10th request by the metrics middleware. It includes everything
in the process, including Go runtime internals. After active requests complete,
the count drops back to the baseline.

### 17.10 Request Tracing Spans

Each chat completion request produces the following trace hierarchy:

```
POST /v1/chat/completions
├── prepare-request              Validation, caching, and prompt creation
│   ├── process-cache            Cache lookup/update (SPC or IMC, when enabled)
│   │   └── cache-tokenize-*     Tokenization for cache (spc, imc-extend, imc-scratch)
│   └── create-prompt            Jinja template application
│
│        ← queue wait →          Job sits in requestQ channel until batch engine picks it up
│
└── process-request              Batch engine slot processing
    ├── prefill                  Tokenization + KV cache fill (ends at first output token)
    └── token-generation         Decode loop producing output tokens
```

**Phase 1: prepare-request** runs in the `ChatStreaming` goroutine. It
validates the document, processes caches (SPC/IMC), and creates the prompt
via the Jinja template. When caching is enabled, `process-cache` and its
child `cache-tokenize-*` spans appear here.

**Queue wait** is the gap between `prepare-request` ending and
`process-request` starting. The job has been submitted to the batch engine's
`requestQ` channel and is waiting for the `processLoop` goroutine to wake up
and assign it to a slot. The exact duration is recorded as a `queue-wait`
attribute on the `process-request` span.

**Phase 2: process-request** runs in the batch engine's `processLoop`
goroutine. The `prefill` span covers tokenization and KV cache filling. Time
to first token (TTFT) is measured from prefill start to the first output
token. The `token-generation` span covers the decode loop that produces
output tokens.

Additional spans that may appear at the top level:

| Span                   | When                      | Description                            |
| ---------------------- | ------------------------- | -------------------------------------- |
| `model-file-load-time` | First request for a model | Loading the GGUF model file            |
| `proj-file-load-time`  | Vision/audio requests     | Loading the multimodal projection file |

### 17.11 Reference Threads

See `THREADS.md` for important past conversations and decisions worth
preserving.
