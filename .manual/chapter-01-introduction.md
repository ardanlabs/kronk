# Chapter 1: Introduction

## Table of Contents

- [1.1 What is Kronk](#11-what-is-kronk)
- [1.2 Key Features](#12-key-features)
- [1.3 Supported Platforms and Hardware](#13-supported-platforms-and-hardware)
- [1.4 Architecture Overview](#14-architecture-overview)

---



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
