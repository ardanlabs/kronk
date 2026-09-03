# Chapter 1: Introduction

## Table of Contents

- [1.1 What Is Kronk?](#11-what-is-kronk)
- [1.2 SDK or Model Server?](#12-sdk-or-model-server)
- [1.3 Capabilities](#13-capabilities)
- [1.4 Architecture](#14-architecture)
- [1.5 Where to Go Next](#15-where-to-go-next)

---

### 1.1 What Is Kronk?

Kronk is a Go SDK and model server for running open-source models on your own
hardware. It uses two C++ inference engines:

- [llama.cpp](https://github.com/ggml-org/llama.cpp) runs GGUF text, vision,
  embedding, and reranking models.
- [whisper.cpp](https://github.com/ggml-org/whisper.cpp) runs speech-to-text
  models through Kronk's **Bucky** subsystem.

Kronk provides hardware-accelerated text generation, image understanding,
audio transcription, embeddings, and reranking from Go without requiring
Python or a CGO build toolchain.

### 1.2 SDK or Model Server?

The model server's inference functionality is built on the same public SDK
that is available to Go applications. You can either embed inference directly
in an application or run the server for network access.

| Use the SDK when you need... | Use the Model Server when you need... |
| ---------------------------- | ------------------------------------- |
| Inference inside a Go process | HTTP APIs for one or more clients |
| Direct control over model loading and lifetime | OpenAI-compatible APIs and an Anthropic-compatible Messages API |
| No separate server process | A browser interface for managing and testing models |
| Application-specific caching and concurrency | Authentication, authorization, and rate limiting |

Standalone SDK examples are available in the
[`examples`](https://github.com/ardanlabs/kronk/tree/main/examples) directory.

### 1.3 Capabilities

**Inference**

- **Text generation** — chat completions and responses, including streaming,
  reasoning, and tool calls.
- **Vision** — image understanding with compatible multimodal models.
- **Audio transcription** — speech-to-text through Bucky and whisper.cpp. See
  [Chapter 18: Bucky](https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription).
- **Embeddings and reranking** — vector generation and document relevance
  scoring for search and retrieval systems.

**Performance**

- **Concurrent processing** — partition a model's KV cache into sequences so
  it can process multiple requests.
- **Message caching** — reuse computed prompt prefixes instead of evaluating
  them again for every request.
- **Extended context** — configure YaRN context extension for compatible
  models.
- **Model pooling** — keep a configurable number of models loaded and evict
  inactive models after a time-to-live period.

**Operation**

- **Model catalog** — discover, download, and configure a curated set of
  models through the CLI or Browser UI (BUI).
- **Client APIs** — use OpenAI-compatible endpoints, the Anthropic-compatible
  Messages endpoint, or integrations such as OpenWebUI and OpenCode.
- **Security** — protect endpoints with JWT authentication, scoped
  authorization, and rate limits.
- **Observability** — collect traces and Prometheus metrics for external
  observability systems.

Kronk stores its managed files under `~/.kronk/` by default. The location is
configurable; the official containers use `/kronk` so the directory can be
mounted as persistent storage.

Hardware support differs by operating system, architecture, inference engine,
and release artifact. Kronk can use CPU inference and GPU backends such as
Metal, CUDA, Vulkan, and ROCm where a compatible library bundle is available.
See [Chapter 2: Installation & Quick Start](https://www.kronkai.com/manual#chapter-2-installation-quick-start) for the
currently distributed native and container options.

Memory requirements depend on more than model parameter count. Quantization,
context size, KV cache type, batch size, concurrency, and multimodal
projections all affect RAM and VRAM use. See
[Chapter 3: Model Configuration](https://www.kronkai.com/manual#chapter-3-model-configuration) before
selecting a large model or context window.

### 1.4 Architecture

Kronk is layered so applications and the model server use the same inference
SDKs. The two engine paths provide different model capabilities and may have
different platform support.

![Kronk architecture: application access, SDKs, bindings, native engines, and models](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/02-kronk-architecture.svg)

The SDK layer owns model loading, inference, caching, and concurrency. The
model server adds HTTP transport, model pooling, the BUI, security, and
operational services. The model server reaches both typed SDKs through the
shared `sdk/pool` facade. Your application can call `sdk/kronk` or `sdk/bucky`
directly without starting the model server, or use the pool when it needs a
coordinated multi-model lifecycle.

For conversational generation, Incremental Message Caching (IMC) keeps a pool
of reusable conversation-prefix snapshots separate from the active execution
slots. An inactive session is externalized to the built-in RAM store or a
custom SDK store rather than remaining attached to one slot or persistently
occupying that slot's accelerator KV stream. A later request can reserve the
longest compatible complete prefix, restore it into any free slot, and prefill
only the stable work that was added. This applies to supported text and
multimodal prompts; media identity, ordering, embedding expansion, and position
accounting are part of safe reuse. See
[Chapter 4](https://www.kronkai.com/manual#chapter-4-batch-processing) for the
request lifecycle and
[Chapter 5](https://www.kronkai.com/manual#chapter-5-message-caching) for the
full IMC semantics.

### 1.5 Where to Go Next

- **Install Kronk and run your first model:**
  [Chapter 2: Installation & Quick Start](https://www.kronkai.com/manual#chapter-2-installation-quick-start)
- **Choose memory, context, and GPU settings:**
  [Chapter 3: Model Configuration](https://www.kronkai.com/manual#chapter-3-model-configuration)
- **Connect an editor, agent, or other client:**
  [Chapter 14: Client Integration](https://www.kronkai.com/manual#chapter-14-client-integration)
- **Run speech-to-text models:**
  [Chapter 18: Bucky](https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription)

---
