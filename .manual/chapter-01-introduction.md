# Chapter 1: Introduction

## Table of Contents

- [1.1 What Is Kronk?](#11-what-is-kronk)
- [1.2 SDK or Model Server?](#12-sdk-or-model-server)
- [1.3 Capabilities](#13-capabilities)
- [1.4 Architecture](#14-architecture)
- [1.5 Where to Go Next](#15-where-to-go-next)

---

### 1.1 What Is Kronk

Kronk is a Go SDK and model server for running open-source models on your own
hardware. It uses three C++ inference engines:

- [llama.cpp](https://github.com/ggml-org/llama.cpp) runs GGUF text, vision,
  embedding, and reranking models.
- [whisper.cpp](https://github.com/ggml-org/whisper.cpp) runs speech-to-text
  models through Kronk's **Bucky** subsystem.
- [stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp) runs
  image-generation models through Kronk's experimental **Malina** subsystem.

Kronk provides hardware-accelerated text generation, image understanding,
audio transcription, image generation, embeddings, and reranking from Go
without requiring Python or a CGO build toolchain. Runtime inference still
requires compatible native libraries, which Kronk's library managers can
download for supported platforms.

### 1.2 SDK or Model Server

The model server's inference functionality is built on the same public SDK
that is available to Go applications. You can either embed inference directly
in an application or run the server for network access.

| Use the SDK when you need...                   | Use the Model Server when you need...                           |
| ---------------------------------------------- | --------------------------------------------------------------- |
| Inference inside a Go process                  | HTTP APIs for one or more clients                               |
| Direct control over model loading and lifetime | OpenAI-compatible APIs and an Anthropic-compatible Messages API |
| No separate server process                     | A browser interface for managing and testing models             |
| Application-specific caching and concurrency   | Authentication, authorization, and rate limiting                |

Standalone SDK examples are available in the
[`examples`](https://github.com/ardanlabs/kronk/tree/main/examples) directory.

### 1.3 Capabilities

**Inference**

- **Text generation** — chat completions and responses, including streaming,
  reasoning, and tool calls.
- **Vision** — image understanding with compatible multimodal models.
- **Audio transcription** — speech-to-text through Bucky and whisper.cpp. See
  [Chapter 18: Bucky](https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription).
- **Image generation and editing** — text-to-image and image-to-image through
  experimental Malina and stable-diffusion.cpp. See
  [Chapter 19: Malina](https://www.kronkai.com/manual#chapter-19-malina-image-generation)
  for server capabilities and additional SDK-only operations.
- **Embeddings and reranking** — vector generation and document relevance
  scoring for search and retrieval systems.
- **Structured decisions** — evaluate choice, score, and calibrated yes/no
  questions against shared application state with supported decision models.

**Performance**

- **Concurrent processing** — serve multiple requests against a loaded model
  within configured concurrency and memory limits.
- **Message caching** — reuse computed prompt prefixes instead of evaluating
  them again for every request.
- **Extended context** — configure YaRN context extension for compatible
  models.
- **Model pooling** — retain loaded models within per-backend count limits
  and shared RAM/VRAM budgets, evicting idle models when space is needed.
  Optional time-to-live expiration can also unload idle models; it is
  disabled by default.

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
SDKs. The three engine paths provide different model capabilities and may have
different platform support.

![Kronk architecture: application access, SDKs, bindings, native engines, and models](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/talk/02-kronk-architecture.svg)

Embedded applications control model loading and inference directly. The
model server provides network access to those capabilities and manages model
retention, memory admission, concurrent requests, and waiting requests. It also
provides the BUI, security, and operational services. Supported operations
differ by model and backend; some SDK operations are not exposed over HTTP.

For conversational generation, Incremental Message Caching (IMC) can avoid
reprocessing a compatible prompt prefix from an earlier request.
Chat-completion clients still send the complete conversation; caching does not
replace conversation history management. An inactive conversation does not
hold an execution slot, but its cached state still consumes storage. Changes
to messages, tools, templates, or media can reduce or prevent reuse. See
[Chapter 4](https://www.kronkai.com/manual#chapter-4-batch-processing) for
concurrency and request handling, and
[Chapter 5](https://www.kronkai.com/manual#chapter-5-message-caching) for cache
behavior and limitations.

### 1.5 Where to Go Next

- **Install Kronk and run your first model:**
  [Chapter 2: Installation & Quick Start](https://www.kronkai.com/manual#chapter-2-installation-quick-start)
- **Choose memory, context, and GPU settings:**
  [Chapter 3: Model Configuration](https://www.kronkai.com/manual#chapter-3-model-configuration)
- **Connect an editor, agent, or other client:**
  [Chapter 14: Client Integration](https://www.kronkai.com/manual#chapter-14-client-integration)
- **Run speech-to-text models:**
  [Chapter 18: Bucky](https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription)
- **Generate or edit images:**
  [Chapter 19: Malina](https://www.kronkai.com/manual#chapter-19-malina-image-generation)

---
