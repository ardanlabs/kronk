# Kronk Features

This document provides a comprehensive list of all features available in the Kronk project.

## Kronk SDK API

The SDK API (`sdk/kronk`) provides a high-level, concurrently safe interface for working with models using llama.cpp via yzma.

### Core API Features

| Feature | Description |
|---------|-------------|
| **Concurrent Model Access** | Thread-safe access to models through a pooling mechanism supporting multiple model instances |
| **Chat Completions** | Synchronous chat completions with inference models |
| **Streaming Chat** | Asynchronous streaming chat responses via Go channels |
| **HTTP Streaming** | Built-in HTTP handler support for Server-Sent Events (SSE) streaming |
| **Embeddings** | Generate embeddings from embedding models |
| **HTTP Embeddings** | Built-in HTTP handler for embedding requests |
| **Reranking** | Rerank documents by relevance to a query |
| **HTTP Reranking** | Built-in HTTP handler for reranking requests |
| **Responses API** | OpenAI Responses API support with rich event streaming |
| **HTTP Responses** | Built-in HTTP handler for Responses API requests |
| **Model Info** | Retrieve model metadata and configuration |
| **System Info** | Access llama.cpp system information (GPU, CPU features) |
| **Active Stream Tracking** | Monitor the number of active inference streams |
| **Graceful Unloading** | Safe model unloading with active stream awareness |
| **Configurable Logging** | Silent or normal logging modes for llama.cpp |

### Model Capabilities

| Capability | Description |
|------------|-------------|
| **Text Generation** | Standard language model inference |
| **Reasoning Models** | Support for models with reasoning capabilities |
| **Vision Models** | Multimodal image+text inference |
| **Audio Models** | Audio-to-text inference |
| **Embedding Models** | Text embedding generation |
| **Reranker Models** | Document relevance scoring and reordering |
| **Tool Calling** | Function/tool calling support |

### Configuration Options

| Option | Description |
|--------|-------------|
| **ModelFiles** | Path to the model files (mandatory) |
| **ProjFile** | Path to projection files for vision/audio models |
| **JinjaFile** | Optional custom Jinja template file |
| **Device** | GPU device selection (use `llama-bench --list-devices` to see available) |
| **ContextWindow** | Maximum tokens the model can process at once (default: 8192) |
| **NBatch** | Logical batch size for forward passes (default: 2048) |
| **NUBatch** | Physical batch size for prompt processing (default: 512, 2048 for vision) |
| **NThreads** | Threads for generation |
| **NThreadsBatch** | Threads for batch processing |
| **CacheTypeK** | KV cache key precision (f32, f16, q8_0, q4_0, bf16, auto) |
| **CacheTypeV** | KV cache value precision (f32, f16, q8_0, q4_0, bf16, auto) |
| **FlashAttention** | Flash Attention mode (enabled, disabled, auto) |
| **UseDirectIO** | Enable direct I/O for model loading |
| **IgnoreIntegrityCheck** | Skip model integrity verification |
| **NSeqMax** | For text models: parallel sequences for batched inference. For sequential models: number of pooled model instances |
| **OffloadKQV** | KV cache on GPU (true) or CPU (false) |
| **OpOffload** | Tensor operations on GPU (true) or CPU (false) |
| **NGpuLayers** | Layers to offload to GPU (0=all, -1=none) |
| **SplitMode** | Multi-GPU split mode (none, layer, row for MoE models) |

---

## Kronk Model Server (KMS)

The Kronk Model Server is an OpenAI-compatible model server for chat completions, responses, and embeddings, compatible with OpenWebUI and Cline.

### Server Endpoints

#### Chat Completions (`/v1/chat/completions`)

| Feature | Description |
|---------|-------------|
| **OpenAI Compatibility** | Compatible with OpenAI chat completions API format |
| **Streaming Support** | Server-Sent Events for real-time token streaming |
| **Non-Streaming** | Standard request/response mode |
| **Model Selection** | Dynamically select models per request |
| **Automatic Model Loading** | Models loaded on-demand from cache |
| **Tool Calling** | Function/tool calling support with JSON arguments |

#### Responses (`/v1/responses`)

| Feature | Description |
|---------|-------------|
| **OpenAI Responses API** | Compatible with OpenAI Responses API format |
| **Streaming Support** | Server-Sent Events with rich event types |
| **Non-Streaming** | Standard request/response mode |
| **Tool Calling** | Function call support with parallel tool calls |
| **Reasoning Support** | Support for reasoning models with summary output |
| **Input Format Conversion** | Automatic conversion from `input` to `messages` format |

#### Embeddings (`/v1/embeddings`)

| Feature | Description |
|---------|-------------|
| **OpenAI Compatibility** | Compatible with OpenAI embeddings API format |
| **Model Selection** | Dynamically select embedding models per request |

#### Rerank (`/v1/rerank`)

| Feature | Description |
|---------|-------------|
| **Document Reranking** | Score and reorder documents by relevance to a query |
| **Model Selection** | Dynamically select reranker models per request |
| **Top-N Filtering** | Return only the top N most relevant results |
| **Document Return** | Optionally include document text in results |

### Server Management Features

| Feature | Description |
|---------|-------------|
| **Model Caching** | Configurable number of models kept in memory |
| **TTL Management** | Automatic model unloading after inactivity |
| **Resource Management** | Efficient hardware resource utilization |

---

#### Tools API Endpoints

The Tools API provides endpoints for managing models, libraries, security, and catalog.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/libs` | GET | List installed llama.cpp library version information |
| `/v1/libs/pull` | POST | Download and install llama.cpp libraries (admin) |
| `/v1/models` | GET | List all locally installed models |
| `/v1/models/{model}` | GET | Show detailed model information and metadata |
| `/v1/models/ps` | GET | View currently loaded/running models |
| `/v1/models/index` | POST | Build model index for fast lookups (admin) |
| `/v1/models/pull` | POST | Download models from URLs with streaming progress (admin) |
| `/v1/models/{model}` | DELETE | Remove a model from local storage (admin) |
| `/v1/catalog` | GET | List available models from the official catalog |
| `/v1/catalog/filter/{filter}` | GET | Filter catalog by model type |
| `/v1/catalog/{model}` | GET | Show detailed catalog model information |
| `/v1/catalog/pull/{model}` | POST | Download models from the catalog |
| `/v1/security/token/create` | POST | Create API tokens |
| `/v1/security/keys` | GET | List registered API keys |
| `/v1/security/keys/add` | POST | Add a new API key |
| `/v1/security/keys/remove/{keyid}` | POST | Remove an API key |

---

## Security Features

Kronk includes a comprehensive security system with JWT-based authentication and endpoint-level rate limiting.

### JWT Authentication

| Feature | Description |
|---------|-------------|
| **JWT Token Generation** | Generate signed RS256 JWT tokens for API access |
| **Token Authentication** | Validate bearer tokens on protected endpoints |
| **Token Authorization** | Check claims for admin and endpoint permissions |
| **Key ID (KID) Support** | Multiple signing keys with key rotation support |
| **Configurable Issuer** | Set token issuer for validation |
| **OPA Policy Evaluation** | Open Policy Agent integration for authentication and authorization rules |

### Rate Limiting

| Feature | Description |
|---------|-------------|
| **Endpoint-Level Limits** | Configure rate limits per endpoint (chat completions, embeddings) |
| **Time Windows** | Support for day, month, year, and unlimited rate windows |
| **Per-Token Configuration** | Each token can have unique rate limit settings |
| **Admin Bypass** | Admin tokens can bypass rate limiting |

### API Key Management

| Feature | Description |
|---------|-------------|
| **List Keys** | View all registered API keys |
| **Create Keys** | Generate new API keys |
| **Delete Keys** | Remove API keys from the system |
| **Key-Based Token Generation** | Generate tokens associated with specific keys |

### Token Management

| Feature | Description |
|---------|-------------|
| **Create Tokens** | Generate tokens with custom permissions and rate limits |
| **Admin Tokens** | Create tokens with administrative privileges |
| **Configurable Duration** | Set token expiration periods |
| **Endpoint Permissions** | Specify which endpoints a token can access |

---

## Browser App (BUI)

Kronk includes a built-in browser-based management interface for administering the system.

### Features

| Feature | Description |
|---------|-------------|
| **Chat Interface** | Interactive chat interface for testing models |
| **Model List** | View all installed models |
| **Running Models** | Monitor currently loaded/running models |
| **Model Pull** | Download models from URLs with progress tracking |
| **Catalog Browser** | Browse and download from the official model catalog |
| **Library Management** | Install or upgrade llama.cpp libraries |
| **API Key Management** | Create, list, and delete API keys |
| **Token Management** | Generate tokens with custom rate limits and permissions |
| **Settings** | Configure server connection and authentication |

### Documentation

| Feature | Description |
|---------|-------------|
| **SDK Documentation** | Auto-generated docs for kronk and model packages |
| **CLI Documentation** | Reference for all CLI commands |
| **API Documentation** | OpenAPI-style docs for Chat, Responses, Embeddings, and Tools endpoints |
| **Code Examples** | Working examples from the examples directory |

---

## Kronk CLI

The Kronk CLI (`cmd/kronk`) provides command-line access to all management features.

### Commands Overview

```
kronk [command]

Available Commands:
  catalog     Manage model catalog
  libs        Install or upgrade llama.cpp libraries
  model       Manage models
  run         Run an interactive chat session with a model
  security    Manage security
  server      Manage model server
```

### Server Commands

| Command | Description |
|---------|-------------|
| `kronk server start` | Start the Kronk model server |
| `kronk server stop` | Stop the Kronk model server |
| `kronk server logs` | View server logs |

### Library Commands

| Command | Description |
|---------|-------------|
| `kronk libs` | Install or upgrade llama.cpp libraries |

### Run Commands

| Command | Description |
|---------|-------------|
| `kronk run <MODEL>` | Run an interactive chat session with a model (REPL mode) |

### Model Commands

| Command | Description |
|---------|-------------|
| `kronk model list` | List all installed models |
| `kronk model pull` | Download a model from URL |
| `kronk model remove` | Remove an installed model |
| `kronk model show` | Display model details |
| `kronk model ps` | Show currently running models |
| `kronk model index` | Rebuild model index |

### Catalog Commands

| Command | Description |
|---------|-------------|
| `kronk catalog list` | List models in the catalog |
| `kronk catalog pull` | Download a model from the catalog |
| `kronk catalog show` | Show catalog model details |
| `kronk catalog update` | Update the local catalog |

### Security Commands

| Command | Description |
|---------|-------------|
| `kronk security key` | Manage API keys |
| `kronk security token` | Manage API tokens |
| `kronk security sec` | Security configuration |

---

## Platform Support

| OS | CPU | GPU |
|----|-----|-----|
| Linux | amd64, arm64 | CUDA, Vulkan, HIP, ROCm, SYCL |
| macOS | arm64 | Metal |
| Windows | amd64 | CUDA, Vulkan, HIP, SYCL, OpenCL |

---

## Integration Support

| Integration | Description |
|-------------|-------------|
| **OpenWebUI** | Full compatibility with OpenWebUI for browser-based chat interface |
| **Cline** | Compatible with Cline AI coding assistant |
| **OpenAI SDK** | Compatible with OpenAI client libraries |
| **GGUF Models** | Support for all GGUF format models from Hugging Face |
| **yzma** | Direct integration with llama.cpp via the yzma module |

---

## Model Catalog System

The Catalog system (`sdk/tools/catalog`) provides a curated registry of verified models.

| Feature | Description |
|---------|-------------|
| **Remote Catalog** | Syncs with the official [kronk_catalogs](https://github.com/ardanlabs/kronk_catalogs) GitHub repository |
| **Category Filtering** | Filter models by type: Text-Generation, Embedding, Rerank, Vision, Audio |
| **Model Metadata** | Stores model info including endpoint type, capabilities, and download URLs |
| **Index Management** | Local index file for fast lookups without network requests |
| **Custom Catalog Repos** | Support for custom GitHub repository sources |

---

## Template System

The Template system (`sdk/tools/templates`) provides prompt template management for models.

| Feature | Description |
|---------|-------------|
| **Remote Templates** | Syncs templates from the official kronk_catalogs repository |
| **Model-Specific Templates** | Templates tied to specific model architectures |
| **Local Caching** | Templates cached locally with SHA-based change detection |
| **Custom Template Repos** | Support for custom GitHub repository sources |

---

## Defaults & Configuration

The Defaults system (`sdk/tools/defaults`) provides environment-aware configuration.

| Feature | Description |
|---------|-------------|
| **Base Directory** | Configurable Kronk home directory (default: `~/.kronk`) |
| **Architecture Detection** | Auto-detect or override via `KRONK_ARCH` environment variable |
| **OS Detection** | Auto-detect or override via `KRONK_OS` environment variable |
| **Processor Selection** | Select hardware backend (cpu, cuda, metal, vulkan) via `KRONK_PROCESSOR` |
