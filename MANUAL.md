# Kronk Model Server User Manual

## Table of Contents

1. [Introduction](#chapter-1-introduction)
2. [Installation & Quick Start](#chapter-2-installation--quick-start)
3. [Model Configuration](#chapter-3-model-configuration)
4. [Batch Processing](#chapter-4-batch-processing)
5. [Message Caching](#chapter-5-message-caching)
6. [YaRN Extended Context](#chapter-6-yarn-extended-context)
7. [Model Server](#chapter-7-model-server)
8. [API Endpoints](#chapter-8-api-endpoints)
9. [Multi-Modal Models](#chapter-9-multi-modal-models)
10. [Security & Authentication](#chapter-10-security--authentication)
11. [Browser UI (BUI)](#chapter-11-browser-ui-bui)
12. [Client Integration](#chapter-12-client-integration)
13. [Observability](#chapter-13-observability)
14. [Troubleshooting](#chapter-14-troubleshooting)
15. [Developer Guide](#chapter-15-developer-guide)

---

## Chapter 1: Introduction

### 1.1 What is Kronk Model Server

Kronk Model Server (KMS) is an OpenAI and Anthropic compatible model server for running local inference with open-source GGUF models. Built on top of llama.cpp via the [yzma](https://github.com/hybridgroup/yzma) Go bindings, Kronk provides hardware-accelerated inference for text generation, vision, audio, embeddings, and reranking.

The server exposes a REST API that is compatible with:

- OpenAI client libraries
- OpenWebUI
- Agents that can be configured to work with local models
- Any OpenAI-compatible client

### 1.2 Key Features

**Model Types**

- **Text Generation** - Chat completions and streaming responses with reasoning support
- **Vision** - Image understanding and analysis
- **Audio** - Speech-to-text and audio understanding
- **Embeddings** - Vector embeddings for semantic search and RAG
- **Reranking** - Document relevance scoring

**Performance**

- **Batch Processing** - Process multiple requests concurrently with shared KV cache
- **Message Caching** - System prompt and incremental message caching to reduce redundant computation
- **YaRN Context Extension** - Extend context windows 2-4x beyond native training length
- **Model Pooling** - Keep models loaded in memory with configurable TTL

**Operations**

- **Catalog System** - Curated collection of verified models with one-command downloads
- **Browser UI (BUI)** - Web interface for model management, downloads, and configuration
- **Authentication** - JWT-based security with key management and endpoint authorization
- **Observability** - Tempo tracing integration and debug endpoints

### 1.3 Supported Platforms and Hardware

Kronk supports full hardware acceleration across major platforms:

| OS      | CPU          | GPU                             |
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

```
┌────────────────────────────────────────────────────────────────────┐
│                         Kronk Model Server                         │
├────────────────────────────────────────────────────────────────────┤
│                     REST API (OpenAI Compatible)                   │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │
│  │   Chat   │ │ Response │ │  Embed   │ │  Rerank  │ │   Msgs   │  │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘  │
├───────┼────────────┼────────────┼────────────┼────────────┼────────┤
│       └────────────┴──────────┬─┴────────────┴────────────┘        │
│                               ▼                                    │
│      ┌─────────────────────────────────────────────────────┐       │
│      │              Kronk SDK (Model Pool)                 │       │
│      │  ┌─────────┐  ┌─────────┐  ┌─────────┐              │       │
│      │  │ Model A │  │ Model B │  │ Model C │  (cached)    │       │
│      │  └────┬────┘  └────┬────┘  └────┬────┘              │       │
│      └───────┼────────────┼────────────┼───────────────────┘       │
├──────────────┼────────────┼────────────┼───────────────────────────│
│          .   └────────────┴─────┬──────┘                           │
│                                 ▼                                  │
│      ┌─────────────────────────────────────────────────────┐       │
│      │         yzma (llama.cpp Go Bindings)                │       │
│      └─────────────────────────────────────────────────────┘       │
├────────────────────────────────────────────────────────────────────┤
│        Hardware Acceleration: Metal │ CUDA │ Vulkan │ CPU          │
└────────────────────────────────────────────────────────────────────┘
```

**Request Flow**

1. Client sends request to REST API endpoint
2. Server routes to appropriate handler (chat, embed, rerank)
3. Model is acquired from pool (or loaded if not cached)
4. For text models with batch processing enabled, requests queue into batch slots
5. Message caching checks for reusable KV state from previous requests
6. Inference runs with hardware acceleration
7. Response streams back to client (for streaming requests)
8. Model returns to pool for reuse

---

## Chapter 2: Installation & Quick Start

### 2.1 Prerequisites

**Required**

- Go 1.25 or later
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

Before running inference, you need the llama.cpp libraries for your platform. Kronk auto-detects your hardware and downloads the appropriate binaries.

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

Open http://localhost:8080 in your browser. The Browser UI provides:

- Model management and downloads
- Library installation
- Server configuration
- Security key management

### 2.7 Quick Start Summary

```shell
# 1. Install Kronk
go install github.com/ardanlabs/kronk/cmd/kronk@latest

# 2. Start the server (auto-installs libraries on first run)
kronk server start

# 3. Open BUI and download a model
open http://localhost:8080

# 4. Or download via CLI
kronk catalog pull Qwen3-8B-Q8_0 --local

# 5. Test the API
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "Qwen3-8B-Q8_0", "messages": [{"role": "user", "content": "Hello!"}]}'
```

---

## Chapter 3: Model Configuration

Model configuration controls how Kronk loads and runs inference. Configuration
can be set via model config files, catalog templates, or programmatically
through the SDK.

### 3.1 Basic Configuration

**Context Window**

The context window defines the maximum number of tokens the model can process
in a single request. This includes both the input prompt and generated output.

```yaml
context_window: 8192 # Default: 8192 tokens
```

Larger context windows require more VRAM. A rough estimate:

- `8K context`: ~2GB additional VRAM
- `32K context`: ~8GB additional VRAM
- `128K context`: ~32GB additional VRAM (requires YaRN scaling)

**Batch Size Configuration**

Two parameters control how tokens are processed:

- `n_batch` - Maximum tokens in a single forward pass (default: 2048)
- `n_ubatch` - Physical batch size for prompt processing (default: 512)

```yaml
n_batch: 2048 # Logical batch size
n_ubatch: 512 # Physical batch size (must be ≤ n_batch)
```

**Recommended settings by workload:**

- Interactive chat (single user): `n_batch=512-1024`, `n_ubatch=512`
- Long prompts/RAG: `n_batch=2048-4096`, `n_ubatch=512-1024`
- Batch inference (multiple prompts): `n_batch=2048-4096`, `n_ubatch=512`
- Low VRAM (<8GB): `n_batch=512`, `n_ubatch=256-512`
- High VRAM (24GB+): `n_batch=4096+`, `n_ubatch=1024+`

### 3.2 Sampling Parameters

Sampling parameters control the randomness and quality of generated text.
These are set per-request in the API call.

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

### 3.3 GPU Configuration

**Layer Offloading**

Control how many model layers run on GPU:

```yaml
n_gpu_layers: 0      # 0 = all layers on GPU (default)
n_gpu_layers: -1     # All layers on CPU
n_gpu_layers: 20     # First 20 layers on GPU
```

**KV Cache Location**

The KV cache stores attention state and can consume significant VRAM:

```yaml
offload_kqv: true    # KV cache on GPU (default, faster)
offload_kqv: false   # KV cache on CPU (saves VRAM, slower)
```

**Tensor Operations Offload**

Control where tensor computations run:

```yaml
op_offload: true     # Tensor ops on GPU (default)
op_offload: false    # Tensor ops on CPU
```

Use `op_offload: false` when you need to run the model on CPU but want to
keep some layers on GPU for memory.

**Multi-GPU Split Mode**

For systems with multiple GPUs:

```yaml
split_mode: none     # Single GPU (default)
split_mode: layer    # Split layers across GPUs
split_mode: row      # Tensor parallelism (best for MoE models)
```

Use `row` for Mixture of Experts models like Qwen3-MoE, Mixtral, or DeepSeek.

**Configuration Reference**

| Field | YAML Key | Values | Default | Description |
|-------|----------|--------|---------|-------------|
| NGpuLayers | `n_gpu_layers` | 0, -1, N | 0 | Layers on GPU (0=all, -1=none) |
| OffloadKQV | `offload_kqv` | true/false | true | KV cache on GPU |
| OpOffload | `op_offload` | true/false | true | Tensor ops on GPU |
| SplitMode | `split_mode` | none/layer/row | none | Multi-GPU distribution |

### 3.4 KV Cache Quantization

Reduce VRAM usage by quantizing the KV cache:

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

### 3.5 Flash Attention

Flash Attention optimizes memory usage and speeds up attention computation:

```yaml
flash_attention: enabled   # Default: enabled
flash_attention: disabled  # Disable if causing issues
flash_attention: auto      # Let llama.cpp decide
```

Flash Attention is particularly beneficial for large context windows.

### 3.6 Parallel Inference (NSeqMax)

`NSeqMax` controls concurrent request handling, but behaves differently based
on model type:

**Text Models (Chat/Completion)**

For text models, `NSeqMax` controls batch parallelism within a single model:

```yaml
n_seq_max: 4 # Process up to 4 requests concurrently
```

Multiple requests share the model context and KV cache, with each request
getting an isolated sequence partition.

**Sequential Models (Embed/Rerank/Vision/Audio)**

For sequential models, `NSeqMax` creates multiple model instances:

```yaml
n_seq_max: 2 # Create 2 model instances in pool
```

Each instance handles one request at a time, but multiple instances allow
concurrent processing.

### 3.7 VRAM Estimation

Rough VRAM requirements for common configurations:

**Model Size (Q8_0 quantization)**

- 1-3B parameters: 2-4 GB
- 7-8B parameters: 8-10 GB
- 13B parameters: 14-16 GB
- 30B parameters: 32-36 GB
- 70B parameters: 72-80 GB

**Additional VRAM for Context**

Per 1K tokens of context (with F16 KV cache):

- 7B model: ~50 MB
- 13B model: ~80 MB
- 70B model: ~200 MB

**Example: Qwen3-8B with 32K context**

```
Model weights (Q8_0):     ~8.5 GB
KV cache (32K, F16):      ~1.6 GB
Overhead:                 ~0.5 GB
─────────────────────────────────
Total:                    ~10.6 GB
```

With Q8_0 KV cache quantization, the KV cache drops to ~0.8 GB.

### 3.8 Model Config File Example

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

### 3.9 Model-Specific Tuning

Different model architectures have specific optimization requirements.

**Vision and Audio Models**

Keep `n_ubatch` high for efficient media token processing:

```yaml
models:
  Qwen2.5-VL-3B-Instruct-Q8_0:
    n_batch: 2048
    n_ubatch: 2048    # High for image/audio token batches
    n_seq_max: 2      # Creates 2 model instances in pool
```

Vision models process image tiles as large token batches. Low `n_ubatch`
values cause multiple decode passes per image, significantly slowing
inference.

**Mixture of Experts (MoE) Models**

Use row-based tensor parallelism for multi-GPU setups:

```yaml
models:
  Qwen3-MoE-30B-A3B-Q8_0:
    split_mode: row       # Best for MoE architecture
    cache_type_k: q8_0    # Be cautious with aggressive quantization
    cache_type_v: q8_0
```

MoE models can be sensitive to aggressive KV cache quantization. If you
notice quality degradation, try `f16` cache types.

**Embedding Models**

Optimize batch size for your typical input lengths:

```yaml
models:
  embeddinggemma-300m-qat-Q8_0:
    n_batch: 8192         # Can equal context_window
    n_ubatch: 512         # Align with typical sliding window
    n_seq_max: 4          # 4 model instances for concurrency
```

Embedding models process complete inputs in a single pass, so larger
`n_batch` values improve throughput.

---

## Chapter 4: Batch Processing

Batch processing allows Kronk to handle multiple concurrent requests
efficiently by sharing model resources. This chapter explains the architecture
and how to optimize for your workload.

### 4.1 Architecture Overview

When `NSeqMax > 1` for text models, Kronk creates a batch engine that
processes multiple requests in parallel within a single model instance.

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
                    │         (GPU/CPU Inference)       │
                    └───────────────────────────────────┘
```

### 4.2 Slots and Sequences

**Slots** are processing units that handle individual requests. Each slot
tracks its state: prompt tokens, decode position, sampler, and response
channel.

**Sequences** are isolated partitions in the shared KV cache. Each slot is
assigned a unique sequence ID, ensuring requests don't interfere with each
other's attention state.

```
NSeqMax = 4 (without caching)

Slot 0  →  seqID = 0  →  KV cache partition 0
Slot 1  →  seqID = 1  →  KV cache partition 1
Slot 2  →  seqID = 2  →  KV cache partition 2
Slot 3  →  seqID = 3  →  KV cache partition 3
```

When caching is enabled, sequence 0 is reserved for cached content:

```
NSeqMax = 2 (with System Prompt Cache)

Cache   →  seqID = 0  →  Cached system prompt KV state
Slot 0  →  seqID = 1  →  KV cache partition 1
Slot 1  →  seqID = 2  →  KV cache partition 2
```

### 4.3 Request Flow

1. **Queue**: Request enters the queue (backpressure if full)
2. **Assign**: Available slot picks up the request
3. **Clear**: Slot clears its sequence partition
4. **Cache Check**: If caching enabled, copy cached KV state to slot's sequence
5. **Prefill**: Tokenize and process prompt tokens
6. **Decode**: Generate tokens one at a time, streaming to client
7. **Complete**: Clear sequence, slot becomes available

### 4.4 Configuring Batch Processing

**Enable Batch Processing**

Set `NSeqMax > 1` in your model config:

```yaml
models:
  Qwen3-8B-Q8_0:
    n_seq_max: 4 # 4 concurrent requests
```

**Queue Depth**

The request queue holds `NSeqMax × 2` requests. With `NSeqMax=4`, up to 8
requests can queue while 4 are actively processing.

**Memory Considerations**

Each slot needs its own KV cache partition. With 4 slots and 8K context:

```
KV cache per slot:  ~200 MB (for 8B model with F16)
Total KV cache:     ~800 MB (4 slots × 200 MB)
```

**Caching Memory Overhead**

When message caching is enabled, additional sequences are reserved:

| SPC | IMC | MaxIMCSessions | Reserved Seqs | Slot Start | Memory Overhead |
|-----|-----|----------------|---------------|------------|-----------------|
| off | off | -              | 0             | seq 0      | none            |
| on  | off | -              | 1             | seq 1      | +1 context window |
| off | on  | 1              | 1             | seq 1      | +1 context window |
| off | on  | 4              | 4             | seq 4      | +4 context windows |

Example with `max_imc_sessions=3` and `n_seq_max=2`:

```
seq 0: user-1 cache (IMC)
seq 1: user-2 cache (IMC)
seq 2: user-3 cache (IMC)
seq 3: slot[0] inference
seq 4: slot[1] inference
```

Each cache sequence requires one full context window of KV memory.

### 4.5 Batch vs Sequential Models

The batch engine is only used for **text-only** requests. Other model types
use sequential processing with model pooling:

| Model Type              | NSeqMax Behavior  | Concurrency Method           |
| ----------------------- | ----------------- | ---------------------------- |
| Text (chat, completion) | Batch parallelism | Shared model, multiple slots |
| Embedding               | Model pool        | Multiple model instances     |
| Reranking               | Model pool        | Multiple model instances     |
| Vision                  | Model pool        | Multiple model instances     |
| Audio                   | Model pool        | Multiple model instances     |

**Why Vision/Audio Can't Batch**

Media models require exclusive model context for processing image/audio
tokens through a separate projector pipeline. Each request needs its own
context for media embedding.

### 4.6 Performance Tuning

**Throughput vs Latency**

- Higher `NSeqMax`: Better throughput, potentially higher per-request latency
- Lower `NSeqMax`: Lower latency, less concurrent capacity

**Recommended Settings**

- Single user, interactive: `n_seq_max: 1-2`
- Multi-user API server: `n_seq_max: 4-8`
- High-throughput batch jobs: `n_seq_max: 8-16`

**Monitoring**

Watch for queue backpressure. If requests consistently queue, consider:

1. Increasing `NSeqMax` (if VRAM allows)
2. Reducing `context_window` to fit more slots
3. Using KV cache quantization (`cache_type_k/v: q8_0`)

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

This configuration:

- Handles 8 concurrent requests
- Uses quantized KV cache to reduce memory
- Caches system prompt for faster prefill

---

## Chapter 5: Message Caching

Message caching reduces redundant computation by storing and reusing KV cache
state from previous requests. Kronk provides two caching modes optimized for
different use cases.

### 5.1 Overview

When processing a chat request, the model must compute attention for every
token in the conversation. For long conversations or repeated system prompts,
this becomes wasteful—the same tokens are reprocessed on every request.

Message caching stores the computed KV state and copies it to new requests,
skipping the prefill phase for cached tokens.

```
Without Caching:
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (prefill)   │ (prefill) │ (prefill) │  (prefill)  │
└─────────────────────────────────────────────────────┘
                                              ↓
                                         Generate

With Caching:
┌─────────────────────────────────────────────────────┐
│ System Prompt │ Message 1 │ Message 2 │ New Message │
│   (cached)    │ (cached)  │ (cached)  │  (prefill)  │
└─────────────────────────────────────────────────────┘
                                              ↓
                                         Generate
```

### 5.2 System Prompt Cache (SPC)

System Prompt Cache stores the KV state of the first system message and
reuses it across all requests with the same system prompt.

**Best for:**

- OpenWebUI and similar chat interfaces
- Applications with a consistent system prompt
- Single-user or shared system prompt scenarios

**Enable SPC:**

```yaml
models:
  Qwen3-8B-Q8_0:
    system_prompt_cache: true
```

**How It Works:**

1. First request: System prompt is processed and cached in sequence 0
2. Subsequent requests: Cached KV state is copied to the slot's sequence
3. Only the new messages need prefill processing

**Cache Invalidation:**

The cache is automatically invalidated when:

- The system prompt content changes
- The system prompt role changes
- The server restarts

### 5.3 Incremental Message Cache (IMC)

Incremental Message Cache is designed for agentic workflows where
conversations grow monotonically. It caches all messages except the last
one and extends the cache incrementally on each turn.

**Best for:**

- AI coding agents (Cline, OpenCode, Aider)
- Long-running agent conversations
- Any workflow where messages are appended, not edited

**Enable IMC:**

```yaml
models:
  Qwen3-8B-Q8_0:
    incremental_cache: true
    max_imc_sessions: 4 # Support 4 concurrent users
    cache_min_tokens: 100 # Minimum tokens before caching
```

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

### 5.4 Multi-User IMC

IMC supports multiple concurrent users, each with their own cache sequence.
Users are identified by the `imc_id` parameter in requests.

**Configuration:**

```yaml
models:
  Qwen3-8B-Q8_0:
    incremental_cache: true
    max_imc_sessions: 4 # 4 concurrent user caches
```

**Passing IMC ID:**

Via HTTP header:

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "KRONK_IMC_ID: user-123" \
  -d '{"model": "Qwen3-8B-Q8_0", "messages": [...]}'
```

Or in the request body:

```json
{
  "model": "Qwen3-8B-Q8_0",
  "imc_id": "user-123",
  "messages": [...]
}
```

**Sequence Allocation:**

With `max_imc_sessions=3` and `n_seq_max=2`:

```
seq 0: user-1 cache
seq 1: user-2 cache
seq 2: user-3 cache
seq 3: slot[0] inference
seq 4: slot[1] inference
```

If all cache slots are in use, new sessions bypass IMC gracefully.

### 5.5 SPC vs IMC

| Feature    | System Prompt Cache | Incremental Message Cache |
| ---------- | ------------------- | ------------------------- |
| Caches     | System prompt only  | All messages except last  |
| Extends    | No                  | Yes, incrementally        |
| Multi-user | Shared cache        | Per-user cache            |
| Best for   | Chat UIs            | Agentic workflows         |
| Memory     | 1 extra sequence    | N extra sequences         |

**Important:** SPC and IMC are mutually exclusive. Enabling both returns a
validation error because IMC already includes the system prompt in its cache.

### 5.6 Cache Invalidation

**SPC Invalidation:**

- System prompt content changes → rebuild cache
- Different message role → rebuild cache

**IMC Invalidation:**

- Message prefix changes → rebuild cache from scratch
- User starts new conversation → new cache
- Edit earlier message → rebuild cache

**Manual Invalidation:**

The cache is cleared when:

- Model is unloaded
- Server restarts
- Sequential path processes a request (clears all caches)

### 5.7 Configuration Reference

```yaml
models:
  Qwen3-8B-Q8_0:
    # System Prompt Cache
    system_prompt_cache: true

    # OR Incremental Message Cache (mutually exclusive)
    incremental_cache: true
    max_imc_sessions: 4

    # Shared settings
    cache_min_tokens: 100 # Don't cache if < 100 tokens
```

**cache_min_tokens**

Minimum token count before caching activates. Short messages don't benefit
from caching because the overhead exceeds the prefill savings.

Default: 100 tokens

### 5.8 Performance Impact

**Prefill Time Savings:**

For a 2000-token cached prefix:

- Without cache: ~200ms prefill (varies by hardware)
- With cache: ~5ms copy + ~20ms for new tokens

**Memory Overhead:**

Each cache sequence requires one context window worth of KV cache memory:

```
8K context, F16 cache:    ~200 MB per cache sequence
8K context, Q8_0 cache:   ~100 MB per cache sequence
32K context, F16 cache:   ~800 MB per cache sequence
```

### 5.9 Limitations

- Only works for text-only requests (not vision/audio)
- Requires deterministic Jinja templates (no timestamps, random values)
- IMC requires monotonically growing conversations
- Editing earlier messages triggers full cache rebuild
- If `max_imc_sessions` slots are full, new users bypass IMC

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
kronk catalog list              # Talks to server at localhost:8080
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

Configuration can be set via command-line flags or environment variables.

**Web Settings**

```shell
kronk server start \
  --api-host=localhost:8080 \
  --debug-host=localhost:8090 \
  --read-timeout=30s \
  --write-timeout=15m \
  --idle-timeout=1m \
  --shutdown-timeout=1m \
  --cors-allowed-origins=http://localhost:3000
```

**Environment Variables**

| Variable                  | Description                                |
| ------------------------- | ------------------------------------------ |
| `KRONK_WEB_API_HOST`      | API host address (default: localhost:8080) |
| `KRONK_WEB_DEBUG_HOST`    | Debug host address                         |
| `KRONK_WEB_READ_TIMEOUT`  | HTTP read timeout                          |
| `KRONK_WEB_WRITE_TIMEOUT` | HTTP write timeout                         |

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
    max_imc_sessions: 8
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

### 8.6 Tool Calling (Function Calling)

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
    {"role": "user", "content": "What is the weather in Paris?"},
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

### 8.7 Logprobs (Token Probabilities)

Request log probabilities for generated tokens to understand model confidence
or implement custom sampling strategies.

**Request Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `logprobs` | bool | false | Return log probability for each token |
| `top_logprobs` | int | 0 | Number of top alternatives (0-5) |

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
              {"token": "4", "logprob": -0.0012, "bytes": [52]},
              {"token": "The", "logprob": -6.82, "bytes": [84, 104, 101]},
              {"token": "Four", "logprob": -7.15, "bytes": [70, 111, 117, 114]}
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

### 8.9 Using IMC with API Requests

To use Incremental Message Cache, pass the session ID via header:

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "KRONK_IMC_ID: user-123" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [...]
  }'
```

Or in the request body:

```json
{
  "model": "Qwen3-8B-Q8_0",
  "imc_id": "user-123",
  "messages": [...]
}
```

### 8.10 Authentication

When authentication is enabled, include the token in requests:

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-token-here" \
  -d '{...}'
```

See [Chapter 10: Security & Authentication](#chapter-10-security--authentication)
for details on token management.

### 8.11 Error Responses

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

## Chapter 9: Multi-Modal Models

Kronk supports vision and audio models that can process images, video frames,
and audio alongside text. This chapter covers how to use these models.

### 9.1 Overview

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

### 9.2 Vision Models

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

### 9.3 Audio Models

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

### 9.4 Plain Base64 Format

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

### 9.5 Configuration for Multi-Modal Models

Vision and audio models have specific configuration requirements:

```yaml
models:
  Qwen2.5-VL-3B-Instruct-Q8_0:
    n_ubatch: 2048 # Higher for image token processing
    n_seq_max: 2 # Creates 2 model instances (pooled)
    context_window: 8192
```

**Key Considerations:**

- `n_ubatch` should be high (≥2048) for efficient image/audio token processing
- `n_seq_max` creates model instances in a pool (not batch parallelism)
- Each request needs exclusive model context for media embedding

### 9.6 Memory Requirements

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

### 9.7 Limitations

- Vision/audio models cannot use batch processing (sequential only)
- Each request gets exclusive model context
- Message caching (SPC/IMC) not supported for media requests
- Processing time varies with image resolution and audio duration

### 9.8 Example: Image Analysis

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

### 9.9 Example: Audio Transcription

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

_Next: [Chapter 10: Security & Authentication](#chapter-10-security--authentication)_

## Chapter 10: Security & Authentication

Kronk provides JWT-based authentication and authorization with per-endpoint
rate limiting. When enabled, all API requests require a valid token.

### 10.1 Enabling Authentication

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

### 10.2 Using the Admin Token

The admin token is required for all security management operations.

**Set the Token:**

```shell
export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)
```

**Admin Capabilities:**

- Create new tokens for users
- Add and remove signing keys
- Access all endpoints without rate limits

### 10.3 Key Management

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

### 10.4 Creating User Tokens

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

### 10.5 Token Examples

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

### 10.6 Using Tokens in API Requests

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

### 10.7 Authorization Flow

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

### 10.8 Rate Limiting

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

### 10.9 Configuration Reference

**Server Flags:**

- `--auth-enabled` - Enable authentication (env: `KRONK_AUTH_ENABLED`)
- `--auth-issuer` - JWT issuer name (env: `KRONK_AUTH_ISSUER`)
- `--auth-host` - External auth service host (env: `KRONK_AUTH_HOST`)

**Environment Variables:**

- `KRONK_TOKEN` - Token for CLI commands and API requests
- `KRONK_WEB_API_HOST` - Server address for CLI web mode
  (default: `localhost:8080`)

### 10.10 Security Best Practices

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

_Next: [Chapter 11: Browser UI (BUI)](#chapter-11-browser-ui-bui)_

## Chapter 11: Browser UI (BUI)

Kronk includes a web-based interface for managing models, libraries,
security, and server configuration without using the command line.

### 11.1 Accessing the BUI

The BUI is served from the same port as the API.

**Open in Browser:**

```
http://localhost:8080
```

The BUI automatically loads when you navigate to the server root.

### 11.2 Downloading Libraries

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

### 11.3 Downloading Models

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

### 11.4 Managing Keys and Tokens

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

### 11.5 Other Screens

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

_Next: [Chapter 12: Client Integration](#chapter-12-client-integration)_

## Chapter 12: Client Integration

Kronk's OpenAI-compatible API works with popular AI clients and tools.

### 12.1 OpenWebUI

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

### 12.2 Cline

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
    max-imc-sessions: 1
```

IMC is especially beneficial for Cline's iterative coding workflow.

_Note: Don't use R1 Message formats when using KMS._

### 12.4 Python OpenAI SDK

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

### 12.5 curl and HTTP Clients

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

### 12.6 LangChain

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

_Next: [Chapter 13: Observability](#chapter-13-observability)_

## Chapter 13: Observability

Kronk provides comprehensive observability through distributed tracing,
Prometheus metrics, pprof profiling, and real-time visualizations.

### 13.1 Debug Server

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

### 13.2 Debug Endpoints

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

### 13.3 Health Check Endpoints

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

### 13.4 Prometheus Metrics

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
- `model_prefill_nonmedia_avg`, `_min`, `_max`
- `model_prefill_media_avg`, `_min`, `_max`
- `model_ttft_avg`, `_min`, `_max` (time to first token)

Token usage:

- `usage_prompt_tokens_avg`, `_min`, `_max`
- `usage_reasoning_tokens_avg`, `_min`, `_max`
- `usage_completion_tokens_avg`, `_min`, `_max`
- `usage_output_tokens_avg`, `_min`, `_max`
- `usage_total_tokens_avg`, `_min`, `_max`
- `usage_tokens_per_second_avg`, `_min`, `_max`

### 13.5 Prometheus Integration

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

### 13.6 Distributed Tracing with Tempo

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

### 13.7 Tracing Architecture

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

### 13.8 Tempo Setup with Docker

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

### 13.9 pprof Profiling

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

### 13.10 Statsviz Real-Time Monitoring

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

### 13.11 Logging

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

### 13.12 Configuration Reference

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

_Next: [Chapter 14: Troubleshooting](#chapter-14-troubleshooting)_

## Chapter 14: Troubleshooting

This chapter covers common issues, their causes, and solutions.

### 14.1 Library Issues

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

### 14.2 Model Loading Failures

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

### 14.3 Memory Errors

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

### 14.4 Request Timeouts

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

### 14.5 Authentication Errors

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

### 14.6 Streaming Issues

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

### 14.7 Performance Issues

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
    max_imc_sessions: 4
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

### 14.8 Viewing Logs

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

### 14.9 Common Error Messages

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

### 14.10 Getting Help

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

_Next: [Chapter 15: Developer Guide](#chapter-15-developer-guide)_

## Chapter 15: Developer Guide

This chapter covers development workflows, build commands, and code
conventions for contributors to the Kronk project.

### 15.1 Quick Reference

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

### 15.2 Build & Test Commands

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

### 15.3 Developer Setup

Configure git hooks for automatic pre-commit checks:

```shell
make setup
```

This enables a pre-commit hook that automatically runs:

- `make kronk-docs` - Regenerates documentation
- `make bui-build` - Rebuilds the BUI frontend

### 15.4 Project Architecture

**Directory Structure:**

| Directory                      | Purpose                                                        |
| ------------------------------ | -------------------------------------------------------------- |
| `cmd/kronk/`                   | CLI tool (subcommands: catalog, libs, model, run, security, server) |
| `cmd/server/`                  | OpenAI-compatible model server (gRPC + HTTP) with BUI frontend |
| `cmd/server/api/tooling/docs/` | Documentation generator for BUI (SDK and CLI docs)             |
| `sdk/kronk/`                   | Core API: model loading, chat, embeddings, cache, metrics      |
| `sdk/kronk/model/`             | Core inference and caching engine                              |
| `sdk/kronk/observ/`            | Observability packages (metrics/, otel/)                       |
| `sdk/tools/`                   | Support for libs, models, catalogs, templates, and defaults    |

**Core Technology:**

Kronk uses [yzma](https://github.com/hybridgroup/yzma) (llama.cpp Go bindings)
for local inference with GGUF models.

### 15.5 BUI Frontend Development

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

### 15.6 Code Style Guidelines

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

### 15.7 SDK Internals

This section documents implementation details for developers working on
the Kronk SDK packages.

#### 15.7.1 Package Structure

**sdk/kronk/** - Core API package:

| File | Purpose |
|------|---------|
| `acquire.go` | Model pool acquire/release |
| `chat.go` | Chat completion API |
| `concurrency.go` | Generic streaming utilities |
| `embedding.go` | Embedding API |
| `init.go` | Initialization and configuration |
| `kronk.go` | Main Kronk type, model pool management |
| `rerank.go` | Reranking API |
| `response.go` | OpenAI Responses API streaming |

**sdk/kronk/model/** - Low-level inference:

| File | Purpose |
|------|---------|
| `batch.go` | Batch engine for parallel text inference |
| `caching.go` | System prompt and IMC cache management |
| `chat.go` | Chat inference loop, batch vs sequential routing |
| `config.go` | Model configuration (GPU, cache, batching) |
| `embed.go` | Embedding inference |
| `logprobs.go` | Token log probability extraction |
| `media.go` | Vision/audio media processing |
| `model.go` | Model type, context management, lifecycle |
| `models.go` | OpenAI-compatible types (ChatMessage, ToolCall, etc.) |
| `params.go` | Sampling parameters |
| `processor.go` | Template-specific token processors |
| `prompts.go` | Prompt formatting |
| `rerank.go` | Reranking inference |

#### 15.7.2 Streaming Architecture

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

#### 15.7.3 Model Pool Strategy

`NSeqMax` behaves differently depending on model type:

**Sequential Models** (embed, rerank, vision/audio):

- `NSeqMax` controls the number of model instances in the pool
- Each instance handles one request at a time (single-flight)
- Pooled via `krn.pool` channel for concurrent request handling

**Text Inference Models** (chat, completion):

- `NSeqMax` controls batch parallelism within a single model instance
- Only one `model.Model` instance is created
- Semaphore capacity = `NSeqMax * queueDepth` (default queueDepth=2)

**Detection Logic** (`kronk.go`):

```go
isSingleFlight := cfg.ProjFile != ""  // Vision/audio projector
if mi.IsEmbedModel || mi.IsRerankModel {
    isSingleFlight = true
}
```

#### 15.7.4 Model Acquire/Release & Cleanup

**Two-Stage Acquisition** (`acquire.go`):

1. **Backpressure slot**: Acquire semaphore slot (limits total in-flight requests)
2. **Model instance**: If pooled, acquire specific model from pool channel

**Cleanup Flow:**

1. `streaming()` acquires model, defers `releaseModel()` in wrapper goroutine
2. `ChatStreaming` defers `m.resetContext()` before any processing
3. When generation completes, `resetContext()` runs first:
   - `llama.Synchronize(m.lctx)` - waits for GPU operations
   - `llama.MemoryClear(mem, true)` - clears KV cache
4. Channel closes, wrapper exits, `releaseModel()` runs
5. Model returns to pool in clean state

**Key invariant:** `resetContext()` always runs before model release due to defer ordering.

#### 15.7.5 Batch Engine Internals

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

If `submitToBatchEngine()` returns false, the sequential path is used:

```go
if m.submitToBatchEngine(...) {
    batching = true
    return
}
m.sequentialChatRequest(...)
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
are offset when caching is enabled (SPC uses seq 0; IMC uses seqs 0 to
MaxIMCSessions-1).

#### 15.7.6 Context Pooling

- `llama.Context` is created once in `NewModel` and reused across requests
- Call `resetContext()` between requests to clear KV cache
- Avoids Vulkan memory fragmentation from repeated context alloc/dealloc

#### 15.7.7 IMC Implementation Details

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

#### 15.7.8 Tool Call Internals

**chatMessage Unmarshaling** (`models.go`):

- `Content` can be `nil` for assistant messages with tool_calls
- Handle `len(app.Content) == 0 || string(app.Content) == "null"` as valid empty content

**ToolCallArguments Type:**

- Custom type that marshals to JSON string (OpenAI spec)
- Unmarshals from either string or object for non-compliant clients

#### 15.7.9 Logprobs Implementation

**Implementation** (`logprobs.go`):

- `extractLogprobs()`: Retrieves logits via `llama.GetLogitsIth()`
- `logSoftmax()`: Numerically stable log-softmax using log-sum-exp trick
- `getTopKLogprobs()`: Uses min-heap for efficient O(n log k) top-k extraction

**Critical:** Logprobs must be extracted **before** `llama.SamplerAccept()` is called.

### 15.8 API Handler Notes

**Input Format Conversion** (`cmd/server/app/domain/`):

Both streaming and non-streaming Response APIs must call
`convertInputToMessages(d)` to handle the OpenAI Responses `input` field
format.

### 15.9 Reference Threads

See `THREADS.md` for important past conversations and decisions worth
preserving.
