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
| **Tool Calling** | Function/tool calling support |

### Configuration Options

| Option | Description |
|--------|-------------|
| **Model Instances** | Configure the number of concurrent model instances |
| **Temperature** | Control randomness of outputs |
| **Top-P / Top-K** | Nucleus and top-k sampling parameters |
| **Max Tokens** | Limit response length |
| **Context Length** | Configure context window size |

---

## Kronk Model Server (KMS)

The Kronk Model Server is an OpenAI-compatible model server for chat completions and embeddings, compatible with OpenWebUI.

### Server Endpoints

#### Chat Completions (`/v1/chat/completions`)

| Feature | Description |
|---------|-------------|
| **OpenAI Compatibility** | Compatible with OpenAI chat completions API format |
| **Streaming Support** | Server-Sent Events for real-time token streaming |
| **Non-Streaming** | Standard request/response mode |
| **Model Selection** | Dynamically select models per request |
| **Automatic Model Loading** | Models loaded on-demand from cache |

#### Embeddings (`/v1/embeddings`)

| Feature | Description |
|---------|-------------|
| **OpenAI Compatibility** | Compatible with OpenAI embeddings API format |
| **Model Selection** | Dynamically select embedding models per request |

### Server Management Features

| Feature | Description |
|---------|-------------|
| **Model Caching** | Configurable number of models kept in memory |
| **TTL Management** | Automatic model unloading after inactivity |
| **Resource Management** | Efficient hardware resource utilization |

---

## Tools API

The Tools API (`cmd/server/app/domain/toolapp`) provides endpoints for managing models, libraries, and security.

### Library Management

| Feature | Description |
|---------|-------------|
| **List Libraries** | View installed llama.cpp library version information |
| **Pull Libraries** | Download and install llama.cpp libraries with streaming progress |
| **Auto-Upgrade** | Automatic upgrade support for new llama.cpp releases |
| **Platform Detection** | Automatic detection of OS, architecture, and processor type |

### Model Management

| Feature | Description |
|---------|-------------|
| **List Models** | View all locally installed models |
| **Pull Models** | Download models from URLs with streaming progress |
| **Remove Models** | Delete models from local storage |
| **Show Model** | Display detailed model information and metadata |
| **Model PS** | View currently loaded/running models |
| **Index Models** | Build model index for fast lookups |

### Catalog Management

| Feature | Description |
|---------|-------------|
| **List Catalog** | View available models from the official catalog |
| **Filter by Category** | Filter catalog by model type (Text-Generation, Embedding, Vision, Audio) |
| **Pull from Catalog** | Download models directly from the catalog by model ID |
| **Show Catalog Model** | View detailed information about a catalog model |

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

### Model Management

| Feature | Description |
|---------|-------------|
| **Model List** | View all installed models |
| **Running Models** | Monitor currently loaded/running models |
| **Model Pull** | Download models from URLs with progress tracking |
| **Model Remove** | Delete installed models |

### Catalog Management

| Feature | Description |
|---------|-------------|
| **Catalog List** | Browse available models from the official catalog |
| **Catalog Pull** | Download models from the catalog |

### Library Management

| Feature | Description |
|---------|-------------|
| **Libs Pull** | Install or upgrade llama.cpp libraries |

### Security Management

| Feature | Description |
|---------|-------------|
| **Key List** | View registered API keys |
| **Key Create** | Generate new API keys |
| **Key Delete** | Remove API keys |
| **Token Create** | Generate tokens with custom rate limits and permissions |

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
| **OpenAI SDK** | Compatible with OpenAI client libraries |
| **GGUF Models** | Support for all GGUF format models from Hugging Face |
| **yzma** | Direct integration with llama.cpp via the yzma module |

---

## Model Catalog System

The Catalog system (`sdk/tools/catalog`) provides a curated registry of verified models.

| Feature | Description |
|---------|-------------|
| **Remote Catalog** | Syncs with the official [kronk_catalogs](https://github.com/ardanlabs/kronk_catalogs) GitHub repository |
| **Category Filtering** | Filter models by type: Text-Generation, Embedding, Vision, Audio |
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
