# Kronk Manual

This manual covers everything you need to use and develop with Kronk — from installing the CLI to tuning model configuration and building your own applications with the SDK.

> **Getting Started?** Jump to [Chapter 2: Installation & Quick Start](chapter-02-installation.md) to install Kronk and run your first model, then [Chapter 7: Model Server](chapter-07-model-server.md) to launch the server.

## Chapters

| # | Chapter | Summary |
|---|---------|---------|
| 1 | [Introduction](chapter-01-introduction.md) | What Kronk is, key features, supported platforms, and a high-level architecture overview of the SDK and Model Server. |
| 2 | [Installation & Quick Start](chapter-02-installation.md) | Prerequisites, installing the CLI via Homebrew or Go, downloading libraries and models, starting the server, and verifying everything works. |
| 3 | [Model Configuration](chapter-03-model-configuration.md) | GPU and processor selection, KV cache quantization, flash attention, context windows, parallel inference, GGUF quantization formats, VRAM estimation, and speculative decoding. |
| 4 | [Batch Processing](chapter-04-batch-processing.md) | How slots, sequences, and the batch engine handle concurrent requests. Covers request flow, IMC slot scheduling, and performance tuning. |
| 5 | [Message Caching](chapter-05-message-caching.md) | The Incremental Message Cache (IMC) — how it reduces redundant prefill, the two-tier hash design, KV pressure eviction, and when to enable or disable caching. |
| 6 | [YaRN Extended Context](chapter-06-yarn-extended-context.md) | Extending model context windows beyond training length using YaRN (RoPE scaling). Configuration, scaling types, memory impact, and quality trade-offs. |
| 7 | [Model Server](chapter-07-model-server.md) | Starting and stopping the server, configuration options, model caching, the resource manager, catalog system, and runtime settings. |
| 8 | [API Endpoints](chapter-08-api-endpoints.md) | OpenAI-compatible REST API reference — chat completions, Responses API, embeddings, reranking, tokenization, tool calling, and error responses. |
| 9 | [Request Parameters](chapter-09-request-parameters.md) | Sampling controls (temperature, top-k, top-p), repetition penalties, grammar-constrained output, logprobs, and a full parameter reference table. |
| 10 | [Multi-Modal Models](chapter-10-multi-modal-models.md) | Using vision and audio models — supported media formats, configuration, memory requirements, caching behavior, and worked examples. |
| 11 | [Security & Authentication](chapter-11-security-authentication.md) | Enabling JWT authentication, key management, creating user tokens, per-endpoint rate limiting, and security best practices. |
| 12 | [Browser UI (BUI)](chapter-12-browser-ui.md) | The built-in web interface for managing models, browsing the catalog, downloading libraries, running the playground, and managing security tokens. |
| 13 | [Client Integration](chapter-13-client-integration.md) | Connecting Kronk to OpenCode (with agent bundles), OpenWebUI, the Python OpenAI SDK, curl, and LangChain. |
| 14 | [Observability](chapter-14-observability.md) | Debug server, Prometheus metrics, distributed tracing with Tempo, pprof profiling, Statsviz real-time monitoring, and logging configuration. |
| 15 | [MCP Service](chapter-15-mcp-service.md) | The built-in Model Context Protocol service — Brave web search and fuzzy edit tools, embedded vs standalone modes, and client configuration. |
| 16 | [Troubleshooting](chapter-16-troubleshooting.md) | Common issues and solutions for library loading, model failures, memory errors, timeouts, authentication, streaming, and catalog problems. |
| 17 | [Developer Guide](chapter-17-developer-guide.md) | Build commands, project architecture, BUI frontend development, code style guidelines, and SDK internals for contributors. |
