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
| [Chapter 12: Browser UI (BUI)](MANUAL.md#chapter-12-browser-ui-bui)                    | Web interface, downloading libraries/models, key/token management                                        |
| [Chapter 13: Client Integration](MANUAL.md#chapter-13-client-integration)              | OpenWebUI, Cline, Python SDK, curl, LangChain                                                            |
| [Chapter 14: Observability](MANUAL.md#chapter-14-observability)                        | Debug server, Prometheus metrics, pprof profiling, tracing                                               |
| [Chapter 15: MCP Service](MANUAL.md#chapter-15-mcp-service)                            | Brave Search, MCP configuration, Cline/Kilo client setup, curl testing                                   |
| [Chapter 16: Troubleshooting](MANUAL.md#chapter-16-troubleshooting)                    | Common issues, error messages, debugging tips                                                            |
| [Chapter 17: Developer Guide](MANUAL.md#chapter-17-developer-guide)                    | Build commands, project architecture, BUI development, code style, SDK internals                         |

### Chapter 17 Sub-sections

| Section                                                                 | Topics                                                                     |
| ----------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| [17.1 Build & Test Commands](MANUAL.md#171-build--test-commands)        | Install CLI, run tests, build server, build BUI, generate docs             |
| [17.2 Developer Setup](MANUAL.md#172-developer-setup)                   | Git hooks, pre-commit configuration                                        |
| [17.3 Project Architecture](MANUAL.md#173-project-architecture)         | Directory structure, cmd/, sdk/ packages                                   |
| [17.4 BUI Frontend Development](MANUAL.md#174-bui-frontend-development) | React structure, routing, adding pages, state management, styling          |
| [17.5 Documentation Generation](MANUAL.md#175-documentation-generation) | SDK docs, CLI docs, examples generation                                    |
| [17.6 Code Style Guidelines](MANUAL.md#176-code-style-guidelines)       | Package comments, error handling, struct design, imports, control flow     |
| [17.7 SDK Internals](MANUAL.md#177-sdk-internals)                       | Package structure, streaming, model pool, batch engine, IMC implementation |
| [17.8 API Handler Notes](MANUAL.md#178-api-handler-notes)               | Input format conversion for Response APIs                                  |
| [17.9 Goroutine Budget](MANUAL.md#179-goroutine-budget)                 | Baseline goroutines, per-request goroutines, expected counts               |
| [17.10 Request Tracing Spans](MANUAL.md#1710-request-tracing-spans)     | Span hierarchy, queue wait, prepare-request vs process-request             |
| [17.11 Reference Threads](MANUAL.md#1711-reference-threads)             | THREADS.md for past conversations                                          |

## Reference Threads

See `THREADS.md` for important past conversations worth preserving.
