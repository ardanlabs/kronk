# AGENTS.md

Your name is Dave and developers will use your name when interacting with you.

For comprehensive documentation, see [MANUAL.md](MANUAL.md).

You wil want to look at `Chapter 17: Developer Guide` for detailed information about the project structure, code, and workflows.

## Basic Rules

- After modifying any `.go` file, always run `gofmt -s -w` on the changed files
- You need these env vars to run test
  - export RUN_IN_PARALLEL=yes
  - export GITHUB_WORKSPACE=<Root Location Of Kronk Project>

## MCP Services

Kronk has an MCP service and these are settings:

```
"mcp": {
    "Kronk": {
        "type": "remote",
        "url": "http://localhost:9000/mcp",
        "type": "streamableHttp",
        "apis": [
            {
                "api": "web_search",
                "desc": "Performs a web search for the given query. Returns a list of relevant web pages with titles, URLs, and descriptions. Use this for general information gathering, research, and finding specific web resources."
            }
        ],
    }
}
```

## MANUAL.md Index

| Chapter                                                                                | Topics                                                                                                   |
| -------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| [Chapter 1: Introduction](MANUAL.md#chapter-1-introduction)                            | What is Kronk (SDK + Server), key features, supported platforms, architecture overview                   |
| [Chapter 2: Installation & Quick Start](MANUAL.md#chapter-2-installation--quick-start) | Prerequisites, CLI install, libraries, downloading models, starting server                               |
| [Chapter 3: Model Configuration](MANUAL.md#chapter-3-model-configuration)              | GPU config, KV cache, flash attention, NSeqMax, VRAM estimation, GGUF quantization, MoE vs dense vs hybrid performance, speculative decoding |
| [Chapter 4: Batch Processing](MANUAL.md#chapter-4-batch-processing)                    | Slots, sequences, request flow, memory overhead, concurrency by model type                               |
| [Chapter 5: Message Caching](MANUAL.md#chapter-5-message-caching)                      | System Prompt Cache (SPC), Incremental Message Cache (IMC), hybrid model IMC, multi-user IMC, cache invalidation |
| [Chapter 6: YaRN Extended Context](MANUAL.md#chapter-6-yarn-extended-context)          | RoPE scaling, YaRN configuration, context extension                                                      |
| [Chapter 7: Model Server](MANUAL.md#chapter-7-model-server)                            | Server start/stop, configuration, model caching, config files, catalog system                            |
| [Chapter 8: API Endpoints](MANUAL.md#chapter-8-api-endpoints)                          | Chat completions, Responses API, embeddings, reranking, tool calling                                     |
| [Chapter 9: Request Parameters](MANUAL.md#chapter-9-request-parameters)                | Sampling, repetition control, generation control, grammar, logprobs, cache ID                            |
| [Chapter 10: Multi-Modal Models](MANUAL.md#chapter-10-multi-modal-models)              | Vision models, audio models, media input formats                                                         |
| [Chapter 11: Security & Authentication](MANUAL.md#chapter-11-security--authentication) | JWT auth, key management, token creation, rate limiting                                                  |
| [Chapter 12: Browser UI (BUI)](MANUAL.md#chapter-12-browser-ui-bui)                    | Web interface, downloading libraries/models, key/token management, model playground                      |
| [Chapter 13: Client Integration](MANUAL.md#chapter-13-client-integration)              | OpenWebUI, Cline, Python SDK, curl, LangChain                                                            |
| [Chapter 14: Observability](MANUAL.md#chapter-14-observability)                        | Debug server, Prometheus metrics, pprof profiling, tracing                                               |
| [Chapter 15: MCP Service](MANUAL.md#chapter-15-mcp-service)                            | Brave Search, MCP configuration, Cline/Kilo client setup, curl testing                                   |
| [Chapter 16: Troubleshooting](MANUAL.md#chapter-16-troubleshooting)                    | Common issues, error messages, debugging tips                                                            |
| [Chapter 17: Developer Guide](MANUAL.md#chapter-17-developer-guide)                    | Build commands, project architecture, BUI development, code style, SDK internals                         |

### Chapter 1 Sub-sections

| Section                                                                              | Topics                                                       |
| ------------------------------------------------------------------------------------ | ------------------------------------------------------------ |
| [1.1 What is Kronk](MANUAL.md#11-what-is-kronk)                                     | SDK + Server description, dog-fooding, embedded inference    |
| [1.2 Key Features](MANUAL.md#12-key-features)                                       | Hardware acceleration, API compatibility, caching, vision/audio |
| [1.3 Supported Platforms and Hardware](MANUAL.md#13-supported-platforms-and-hardware) | macOS, Linux, Metal, CUDA, Vulkan                            |
| [1.4 Architecture Overview](MANUAL.md#14-architecture-overview)                     | SDK → yzma → llama.cpp stack, model server                   |

### Chapter 2 Sub-sections

| Section                                                                            | Topics                                         |
| ---------------------------------------------------------------------------------- | ---------------------------------------------- |
| [2.1 Prerequisites](MANUAL.md#21-prerequisites)                                   | Go, GPU drivers, disk space                    |
| [2.2 Installing the CLI](MANUAL.md#22-installing-the-cli)                         | go install, binary setup                       |
| [2.3 Installing Libraries](MANUAL.md#23-installing-libraries)                     | llama.cpp shared libraries, platform-specific  |
| [2.4 Downloading Your First Model](MANUAL.md#24-downloading-your-first-model)     | Model download, GGUF files                     |
| [2.5 Starting the Server](MANUAL.md#25-starting-the-server)                       | Server startup, basic config                   |
| [2.6 Verifying the Installation](MANUAL.md#26-verifying-the-installation)         | Health check, test requests                    |
| [2.7 Quick Start Summary](MANUAL.md#27-quick-start-summary)                       | Step-by-step recap                             |
| [2.8 NixOS Setup](MANUAL.md#28-nixos-setup)                                       | Nix flake, dev shell, Vulkan, troubleshooting  |

### Chapter 3 Sub-sections

| Section                                                                                    | Topics                                                          |
| ------------------------------------------------------------------------------------------ | --------------------------------------------------------------- |
| [3.1 Basic Configuration](MANUAL.md#31-basic-configuration)                               | Context window, batch size, basic model settings                |
| [3.2 GPU Configuration](MANUAL.md#32-gpu-configuration)                                   | GPU layers, processor selection, multi-GPU                      |
| [3.3 KV Cache Quantization](MANUAL.md#33-kv-cache-quantization)                           | f16, q8_0, cache type selection                                 |
| [3.4 Flash Attention](MANUAL.md#34-flash-attention)                                       | Flash attention modes, auto-detection                           |
| [3.5 Parallel Inference (NSeqMax)](MANUAL.md#35-parallel-inference-nseqmax)                | Slots, concurrent requests, NSeqMax tuning                     |
| [3.6 Understanding GGUF Quantization](MANUAL.md#36-understanding-gguf-quantization)       | K-quants, IQ, UD formats, choosing quantization                |
| [3.7 VRAM Estimation](MANUAL.md#37-vram-estimation)                                       | VRAM formula, model weights + KV cache                         |
| [3.8 Model-Specific Tuning](MANUAL.md#38-model-specific-tuning)                           | Vision, MoE, hybrid, embedding model configs, MoE vs dense performance |
| [3.9 Speculative Decoding](MANUAL.md#39-speculative-decoding)                             | Draft models, acceptance rates, configuration                  |
| [3.10 Sampling Parameters](MANUAL.md#310-sampling-parameters)                             | Temperature, top-p, top-k, min-p                               |
| [3.11 Model Config File Example](MANUAL.md#311-model-config-file-example)                 | Complete YAML config example                                   |

### Chapter 4 Sub-sections

| Section                                                                          | Topics                                                     |
| -------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| [4.1 Architecture Overview](MANUAL.md#41-architecture-overview)                 | Batch engine, decode loop, slot lifecycle                  |
| [4.2 Slots and Sequences](MANUAL.md#42-slots-and-sequences)                     | Slot-sequence mapping, KV partitioning                     |
| [4.3 Request Flow](MANUAL.md#43-request-flow)                                   | Request lifecycle, queue → slot → decode → finish          |
| [4.4 Configuring Batch Processing](MANUAL.md#44-configuring-batch-processing)   | n_batch, n_ubatch tuning                                   |
| [4.5 Concurrency by Model Type](MANUAL.md#45-concurrency-by-model-type)         | Dense, MoE, vision, embedding concurrency                  |
| [4.6 Performance Tuning](MANUAL.md#46-performance-tuning)                       | Throughput vs latency trade-offs                           |
| [4.7 Example Configuration](MANUAL.md#47-example-configuration)                 | Complete batch config examples                             |
| [4.8 IMC Slot Scheduling](MANUAL.md#48-imc-slot-scheduling)                     | Slot wait queue, pending slots, scheduling                 |
| [4.9 Model Types and State Management](MANUAL.md#49-model-types-and-state-management) | Dense/MoE/Hybrid, trim vs snapshot/restore, config constraints |

### Chapter 5 Sub-sections

| Section                                                                                        | Topics                                                        |
| ---------------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| [5.1 Overview](MANUAL.md#51-overview)                                                         | SPC vs IMC overview, when to use each                         |
| [5.2 System Prompt Cache (SPC)](MANUAL.md#52-system-prompt-cache-spc)                         | SPC mechanism, externalized KV state                          |
| [5.3 Incremental Message Cache (IMC)](MANUAL.md#53-incremental-message-cache-imc)             | 2 IMC strategies, slot selection, shared algorithm            |
| — [IMC Deterministic](MANUAL.md#imc-deterministic)                                            | Hash-based matching, consistent templates                     |
| — [IMC Non-Deterministic](MANUAL.md#imc-non-deterministic)                                    | Token prefix fallback, variable templates, GPT-OSS/GLM       |
| — [Model Type Interactions](MANUAL.md#model-type-interactions)                                 | Dense/MoE/Hybrid config, cross-reference to 4.9              |
| [5.4 Single-User Caching](MANUAL.md#54-single-user-caching)                                   | Single-user design, slot dedication                           |
| [5.5 SPC vs IMC](MANUAL.md#55-spc-vs-imc)                                                     | Feature comparison, workload selection                        |
| [5.6 Cache Invalidation](MANUAL.md#56-cache-invalidation)                                     | Hash mismatch, rebuild triggers                               |
| [5.7 Configuration Reference](MANUAL.md#57-configuration-reference)                           | YAML settings, cache_min_tokens                               |
| [5.8 Performance and Limitations](MANUAL.md#58-performance-and-limitations)                   | Prefill savings, memory overhead, constraints                 |

### Chapter 6 Sub-sections

| Section                                                                                    | Topics                                      |
| ------------------------------------------------------------------------------------------ | ------------------------------------------- |
| [6.1 Understanding Context Extension](MANUAL.md#61-understanding-context-extension)       | RoPE, native vs extended context            |
| [6.2 When to Use YaRN](MANUAL.md#62-when-to-use-yarn)                                     | Good candidates, model compatibility        |
| [6.3 Configuration](MANUAL.md#63-configuration)                                           | YaRN YAML settings                          |
| [6.4 Scaling Types](MANUAL.md#64-scaling-types)                                           | Linear, YaRN scaling modes                  |
| [6.5 Parameter Reference](MANUAL.md#65-parameter-reference)                               | rope_freq_base, rope_freq_scale, rope_scaling |
| [6.6 Model-Specific Examples](MANUAL.md#66-model-specific-examples)                       | Qwen3, Llama YaRN configs                   |
| [6.7 Memory Impact](MANUAL.md#67-memory-impact)                                           | Extended context KV cache cost              |
| [6.8 Quality Considerations](MANUAL.md#68-quality-considerations)                         | Quality at extended lengths                 |
| [6.9 Example: Long Document Processing](MANUAL.md#69-example-long-document-processing)     | Full working example                        |

### Chapter 7 Sub-sections

| Section                                                                                | Topics                                    |
| -------------------------------------------------------------------------------------- | ----------------------------------------- |
| [7.1 Starting the Server](MANUAL.md#71-starting-the-server)                           | CLI flags, startup sequence               |
| [7.2 Stopping the Server](MANUAL.md#72-stopping-the-server)                           | Graceful shutdown                         |
| [7.3 Server Configuration](MANUAL.md#73-server-configuration)                         | Host, port, TLS, timeouts                 |
| [7.4 Model Caching](MANUAL.md#74-model-caching)                                       | Warm models, model pool                   |
| [7.5 Model Config Files](MANUAL.md#75-model-config-files)                             | model_config.yaml structure               |
| [7.6 Catalog System](MANUAL.md#76-catalog-system)                                     | Model catalogs, templates                 |
| [7.7 Custom Catalog Repository](MANUAL.md#77-custom-catalog-repository)               | Custom catalog repos                      |
| [7.8 Templates](MANUAL.md#78-templates)                                               | Jinja templates, chat templates           |
| [7.9 Runtime Settings](MANUAL.md#79-runtime-settings)                                 | Environment variables, runtime config     |
| [7.10 Logging](MANUAL.md#710-logging)                                                 | Log levels, log output                    |
| [7.11 Data Paths](MANUAL.md#711-data-paths)                                           | Model directory, data directory           |
| [7.12 Complete Example](MANUAL.md#712-complete-example)                               | Full server config example                |

### Chapter 8 Sub-sections

| Section                                                                                  | Topics                                    |
| ---------------------------------------------------------------------------------------- | ----------------------------------------- |
| [8.1 Endpoint Overview](MANUAL.md#81-endpoint-overview)                                 | API routes summary                        |
| [8.2 Chat Completions](MANUAL.md#82-chat-completions)                                   | /v1/chat/completions, streaming           |
| [8.3 Responses API](MANUAL.md#83-responses-api)                                         | /v1/responses, Anthropic-compatible       |
| [8.4 Embeddings](MANUAL.md#84-embeddings)                                               | /v1/embeddings, vector output             |
| [8.5 Reranking](MANUAL.md#85-reranking)                                                 | /v1/rerank, document scoring              |
| [8.6 Tokenize](MANUAL.md#86-tokenize)                                                   | /v1/tokenize, token counting              |
| [8.7 Tool Calling (Function Calling)](MANUAL.md#87-tool-calling-function-calling)       | Tool definitions, function calling        |
| [8.8 Models List](MANUAL.md#88-models-list)                                             | /v1/models, model listing                 |
| [8.9 Authentication](MANUAL.md#89-authentication)                                       | Bearer tokens, API auth                   |
| [8.10 Error Responses](MANUAL.md#810-error-responses)                                   | Error format, status codes                |

### Chapter 9 Sub-sections

| Section                                                                                    | Topics                                          |
| ------------------------------------------------------------------------------------------ | ----------------------------------------------- |
| [9.1 Sampling Parameters](MANUAL.md#91-sampling-parameters)                               | Temperature, top-p, top-k, min-p                |
| [9.2 Repetition Control](MANUAL.md#92-repetition-control)                                 | Repetition penalty, frequency penalty           |
| [9.3 Advanced Sampling](MANUAL.md#93-advanced-sampling)                                   | DRY, XTC, Mirostat                              |
| [9.4 Generation Control](MANUAL.md#94-generation-control)                                 | max_tokens, stop sequences                      |
| [9.5 Grammar Constrained Output](MANUAL.md#95-grammar-constrained-output)                 | GBNF grammars, JSON schema                      |
| [9.6 Logprobs (Token Probabilities)](MANUAL.md#96-logprobs-token-probabilities)           | Token log probabilities, top logprobs           |
| [9.7 Parameter Reference](MANUAL.md#97-parameter-reference)                               | Complete parameter table                        |

### Chapter 10 Sub-sections

| Section                                                                                                  | Topics                                    |
| -------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| [10.1 Overview](MANUAL.md#101-overview)                                                                 | Multi-modal capabilities                  |
| [10.2 Vision Models](MANUAL.md#102-vision-models)                                                       | Image input, vision architectures         |
| [10.3 Audio Models](MANUAL.md#103-audio-models)                                                         | Audio input, speech models                |
| [10.4 Plain Base64 Format](MANUAL.md#104-plain-base64-format)                                           | Base64 media encoding                     |
| [10.5 Configuration for Multi-Modal Models](MANUAL.md#105-configuration-for-multi-modal-models)         | Projection files, batch settings          |
| [10.6 Memory Requirements](MANUAL.md#106-memory-requirements)                                           | Vision/audio VRAM overhead                |
| [10.7 Limitations](MANUAL.md#107-limitations)                                                           | Multi-modal constraints                   |
| [10.8 Example: Image Analysis](MANUAL.md#108-example-image-analysis)                                   | Vision API example                        |
| [10.9 Example: Audio Transcription](MANUAL.md#109-example-audio-transcription)                         | Audio API example                         |

### Chapter 11 Sub-sections

| Section                                                                                    | Topics                                    |
| ------------------------------------------------------------------------------------------ | ----------------------------------------- |
| [11.1 Enabling Authentication](MANUAL.md#111-enabling-authentication)                     | Auth setup, admin token                   |
| [11.2 Using the Admin Token](MANUAL.md#112-using-the-admin-token)                         | Admin token usage                         |
| [11.3 Key Management](MANUAL.md#113-key-management)                                       | Creating, listing, revoking keys          |
| [11.4 Creating User Tokens](MANUAL.md#114-creating-user-tokens)                           | JWT token creation                        |
| [11.5 Token Examples](MANUAL.md#115-token-examples)                                       | Token usage examples                      |
| [11.6 Using Tokens in API Requests](MANUAL.md#116-using-tokens-in-api-requests)           | Bearer token headers                      |
| [11.7 Authorization Flow](MANUAL.md#117-authorization-flow)                               | Request auth pipeline                     |
| [11.8 Rate Limiting](MANUAL.md#118-rate-limiting)                                         | Rate limit configuration                  |
| [11.9 Configuration Reference](MANUAL.md#119-configuration-reference)                     | Auth YAML settings                        |
| [11.10 Security Best Practices](MANUAL.md#1110-security-best-practices)                   | Security recommendations                  |

### Chapter 12 Sub-sections

| Section                                                                            | Topics                                    |
| ---------------------------------------------------------------------------------- | ----------------------------------------- |
| [12.1 Accessing the BUI](MANUAL.md#121-accessing-the-bui)                         | URL, browser access                       |
| [12.2 Downloading Libraries](MANUAL.md#122-downloading-libraries)                 | BUI library download                      |
| [12.3 Downloading Models](MANUAL.md#123-downloading-models)                       | BUI model download                        |
| [12.4 Managing Keys and Tokens](MANUAL.md#124-managing-keys-and-tokens)           | BUI key/token management                  |
| [12.5 Other Screens](MANUAL.md#125-other-screens)                                 | Additional BUI pages                      |
| [12.6 Model Playground](MANUAL.md#126-model-playground)                           | Automated testing, sampling/config sweeps, manual chat, tool calling, prompt inspector |

### Chapter 13 Sub-sections

| Section                                                                    | Topics                                    |
| -------------------------------------------------------------------------- | ----------------------------------------- |
| [13.1 OpenWebUI](MANUAL.md#131-openwebui)                                 | OpenWebUI integration                     |
| [13.2 Cline](MANUAL.md#132-cline)                                         | Cline AI agent setup                      |
| [13.4 Python OpenAI SDK](MANUAL.md#134-python-openai-sdk)                 | Python client usage                       |
| [13.5 curl and HTTP Clients](MANUAL.md#135-curl-and-http-clients)         | curl examples, HTTP usage                 |
| [13.6 LangChain](MANUAL.md#136-langchain)                                 | LangChain integration                     |

### Chapter 14 Sub-sections

| Section                                                                                        | Topics                                    |
| ---------------------------------------------------------------------------------------------- | ----------------------------------------- |
| [14.1 Debug Server](MANUAL.md#141-debug-server)                                               | Debug server setup                        |
| [14.2 Debug Endpoints](MANUAL.md#142-debug-endpoints)                                         | Debug API routes                          |
| [14.3 Health Check Endpoints](MANUAL.md#143-health-check-endpoints)                           | /healthz, /readyz                         |
| [14.4 Prometheus Metrics](MANUAL.md#144-prometheus-metrics)                                   | Available metrics                         |
| [14.5 Prometheus Integration](MANUAL.md#145-prometheus-integration)                           | Prometheus setup                          |
| [14.6 Distributed Tracing with Tempo](MANUAL.md#146-distributed-tracing-with-tempo)           | OpenTelemetry, Tempo                      |
| [14.7 Tracing Architecture](MANUAL.md#147-tracing-architecture)                               | Span hierarchy, trace structure           |
| [14.8 Tempo Setup with Docker](MANUAL.md#148-tempo-setup-with-docker)                         | Docker Tempo config                       |
| [14.9 pprof Profiling](MANUAL.md#149-pprof-profiling)                                         | CPU/memory profiling                      |
| [14.10 Statsviz Real-Time Monitoring](MANUAL.md#1410-statsviz-real-time-monitoring)           | Real-time Go runtime stats                |
| [14.11 Logging](MANUAL.md#1411-logging)                                                       | Log configuration                         |
| [14.12 Configuration Reference](MANUAL.md#1412-configuration-reference)                       | Observability YAML settings               |

### Chapter 15 Sub-sections

| Section                                                                        | Topics                                    |
| ------------------------------------------------------------------------------ | ----------------------------------------- |
| [15.1 Architecture](MANUAL.md#151-architecture)                               | MCP server design                         |
| [15.2 Prerequisites](MANUAL.md#152-prerequisites)                             | Brave API key                             |
| [15.3 Configuration](MANUAL.md#153-configuration)                             | MCP YAML settings                         |
| [15.4 Available Tools](MANUAL.md#154-available-tools)                         | web_search tool                           |
| [15.5 Client Configuration](MANUAL.md#155-client-configuration)               | Cline, Kilo MCP setup                     |
| [15.6 Testing with curl](MANUAL.md#156-testing-with-curl)                     | MCP curl examples                         |

### Chapter 16 Sub-sections

| Section                                                                        | Topics                                    |
| ------------------------------------------------------------------------------ | ----------------------------------------- |
| [16.1 Library Issues](MANUAL.md#161-library-issues)                           | Shared library problems                   |
| [16.2 Model Loading Failures](MANUAL.md#162-model-loading-failures)           | Load errors, VRAM issues                  |
| [16.3 Memory Errors](MANUAL.md#163-memory-errors)                             | OOM, VRAM exhaustion                      |
| [16.4 Request Timeouts](MANUAL.md#164-request-timeouts)                       | Timeout configuration                     |
| [16.5 Authentication Errors](MANUAL.md#165-authentication-errors)             | Auth troubleshooting                      |
| [16.6 Streaming Issues](MANUAL.md#166-streaming-issues)                       | SSE, streaming problems                   |
| [16.7 Performance Issues](MANUAL.md#167-performance-issues)                   | Slow inference, TPS                       |
| [16.8 Viewing Logs](MANUAL.md#168-viewing-logs)                               | Log access, filtering                     |
| [16.9 Common Error Messages](MANUAL.md#169-common-error-messages)             | Error message reference                   |
| [16.10 Getting Help](MANUAL.md#1610-getting-help)                             | Support channels                          |

### Chapter 17 Sub-sections

| Section                                                                 | Topics                                                                     |
| ----------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| [17.1 Quick Reference](MANUAL.md#171-quick-reference)                   | Command cheat sheet                                                        |
| [17.2 Build & Test Commands](MANUAL.md#172-build--test-commands)        | Install CLI, run tests, build server, build BUI, generate docs             |
| [17.3 Developer Setup](MANUAL.md#173-developer-setup)                   | Git hooks, pre-commit configuration                                        |
| [17.4 Project Architecture](MANUAL.md#174-project-architecture)         | Directory structure, cmd/, sdk/ packages                                   |
| [17.5 BUI Frontend Development](MANUAL.md#175-bui-frontend-development) | React structure, routing, adding pages, state management, styling          |
| [17.6 Code Style Guidelines](MANUAL.md#176-code-style-guidelines)       | Package comments, error handling, struct design, imports, control flow     |
| [17.7 SDK Internals](MANUAL.md#177-sdk-internals)                       | Package structure, streaming, model pool, batch engine, IMC implementation |
| [17.8 API Handler Notes](MANUAL.md#178-api-handler-notes)               | Input format conversion for Response APIs                                  |
| [17.9 Goroutine Budget](MANUAL.md#179-goroutine-budget)                 | Baseline goroutines, per-request goroutines, expected counts               |
| [17.10 Request Tracing Spans](MANUAL.md#1710-request-tracing-spans)     | Span hierarchy, queue wait, prepare-request vs process-request             |
| [17.11 Reference Threads](MANUAL.md#1711-reference-threads)             | THREADS.md for past conversations                                          |

## Reference Threads

See `THREADS.md` for important past conversations worth preserving.
