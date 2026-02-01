# AGENTS.md

Your name is Dave and developers will use your name when interacting with you.

For comprehensive documentation, see [MANUAL.md](MANUAL.md).

You wil want to look at `Chapter 15: Developer Guide` for detailed information about the project structure, code, and workflows.

## MANUAL.md Index

| Chapter                                                                                | Topics                                                                                         |
| -------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| [Chapter 1: Introduction](MANUAL.md#chapter-1-introduction)                            | What is Kronk, key features, supported platforms, architecture overview                        |
| [Chapter 2: Installation & Quick Start](MANUAL.md#chapter-2-installation--quick-start) | Prerequisites, CLI install, libraries, downloading models, starting server                     |
| [Chapter 3: Model Configuration](MANUAL.md#chapter-3-model-configuration)              | GPU config, KV cache, flash attention, NSeqMax, VRAM estimation, model-specific tuning         |
| [Chapter 4: Batch Processing](MANUAL.md#chapter-4-batch-processing)                    | Slots, sequences, request flow, memory overhead, batch vs sequential models                    |
| [Chapter 5: Message Caching](MANUAL.md#chapter-5-message-caching)                      | System Prompt Cache (SPC), Incremental Message Cache (IMC), multi-user IMC, cache invalidation |
| [Chapter 6: YaRN Extended Context](MANUAL.md#chapter-6-yarn-extended-context)          | RoPE scaling, YaRN configuration, context extension                                            |
| [Chapter 7: Model Server](MANUAL.md#chapter-7-model-server)                            | Server start/stop, configuration, model caching, config files, catalog system                  |
| [Chapter 8: API Endpoints](MANUAL.md#chapter-8-api-endpoints)                          | Chat completions, Responses API, embeddings, reranking, tool calling, logprobs                 |
| [Chapter 9: Multi-Modal Models](MANUAL.md#chapter-9-multi-modal-models)                | Vision models, audio models, media input formats                                               |
| [Chapter 10: Security & Authentication](MANUAL.md#chapter-10-security--authentication) | JWT auth, key management, token creation, rate limiting                                        |
| [Chapter 11: Browser UI (BUI)](MANUAL.md#chapter-11-browser-ui-bui)                    | Web interface, downloading libraries/models, key/token management                              |
| [Chapter 12: Client Integration](MANUAL.md#chapter-12-client-integration)              | OpenWebUI, Cline, Python SDK, curl, LangChain                                                  |
| [Chapter 13: Observability](MANUAL.md#chapter-13-observability)                        | Debug server, Prometheus metrics, pprof profiling, tracing                                     |
| [Chapter 14: Troubleshooting](MANUAL.md#chapter-14-troubleshooting)                    | Common issues, error messages, debugging tips                                                  |
| [Chapter 15: Developer Guide](MANUAL.md#chapter-15-developer-guide)                    | Build commands, project architecture, BUI development, code style, SDK internals               |

### Chapter 15 Sub-sections

| Section                                                                 | Topics                                                                     |
| ----------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| [15.1 Build & Test Commands](MANUAL.md#151-build--test-commands)        | Install CLI, run tests, build server, build BUI, generate docs             |
| [15.2 Developer Setup](MANUAL.md#152-developer-setup)                   | Git hooks, pre-commit configuration                                        |
| [15.3 Project Architecture](MANUAL.md#153-project-architecture)         | Directory structure, cmd/, sdk/ packages                                   |
| [15.4 BUI Frontend Development](MANUAL.md#154-bui-frontend-development) | React structure, routing, adding pages, state management, styling          |
| [15.5 Documentation Generation](MANUAL.md#155-documentation-generation) | SDK docs, CLI docs, examples generation                                    |
| [15.6 Code Style Guidelines](MANUAL.md#156-code-style-guidelines)       | Package comments, error handling, struct design, imports, control flow     |
| [15.7 SDK Internals](MANUAL.md#157-sdk-internals)                       | Package structure, streaming, model pool, batch engine, IMC implementation |
| [15.8 API Handler Notes](MANUAL.md#158-api-handler-notes)               | Input format conversion for Response APIs                                  |
| [15.9 Reference Threads](MANUAL.md#159-reference-threads)               | THREADS.md for past conversations                                          |

## Reference Threads

See `THREADS.md` for important past conversations worth preserving.
