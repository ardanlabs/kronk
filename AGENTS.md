# AGENTS.md

Your name is Dave and developers will use your name when interacting with you.

The manual has been split into separate chapter files in the `.manual/` directory. Read only the chapters relevant to your task. Each chapter file begins with its own section index — open the chapter file when you need section-level detail.

You will want to look at `Chapter 19: Developer Guide` for detailed information about the project structure, code, and workflows.

## Basic Rules

- After modifying any `.go` file, always run `go vet` and `gofmt -s -w` on the changed files.
- After modifying any `.go` file, always run `staticcheck` and `go fix` on the changed package.
- You need these env vars to run test
  - export RUN_IN_PARALLEL=yes
  - export GITHUB_WORKSPACE=<Root Location Of Kronk Project>
- Never perform a full repo test run and never run tests from this location `sdk/kronk/tests`.

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

## MANUAL Index

Open a chapter file to see its section-level table of contents.

| Chapter                                                                                | Topics                                                                                                                                       |
| -------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| [Chapter 1: Introduction](.manual/chapter-01-introduction.md)                          | What is Kronk (SDK + Server), key features, supported platforms, architecture overview                                                       |
| [Chapter 2: Installation & Quick Start](.manual/chapter-02-installation.md)            | Prerequisites, CLI install (Homebrew, `go install`, binary, Docker / OCI), libraries, downloading models, starting server, model config file |
| [Chapter 3: Model Configuration](.manual/chapter-03-model-configuration.md)            | GPU config, KV cache, flash attention, NSeqMax, VRAM estimation, GGUF quantization, MoE vs dense vs hybrid performance, speculative decoding |
| [Chapter 4: Batch Processing](.manual/chapter-04-batch-processing.md)                  | Slots, sequences, request flow, memory overhead, concurrency by model type                                                                   |
| [Chapter 5: Message Caching](.manual/chapter-05-message-caching.md)                    | Incremental Message Cache (IMC), hybrid model IMC, multi-user IMC, cache invalidation                                                        |
| [Chapter 6: Speculative Decoding & MTP](.manual/chapter-06-speculative-decoding-mtp.md) | User guide for speculative decoding: separate-GGUF vs auto-detected MTP drafters, when to choose each, acceptance/adaptive throttle, observability, known limitations |
| [Chapter 7: YaRN Extended Context](.manual/chapter-07-yarn-extended-context.md)        | RoPE scaling, YaRN configuration, context extension                                                                                          |
| [Chapter 8: Model Server](.manual/chapter-08-model-server.md)                          | Server start/stop, configuration, model caching, config files, catalog system                                                                |
| [Chapter 9: API Endpoints](.manual/chapter-09-api-endpoints.md)                        | Chat completions, Responses API, embeddings, reranking, tool calling                                                                         |
| [Chapter 10: Request Parameters](.manual/chapter-10-request-parameters.md)              | Sampling, repetition control, generation control, grammar, logprobs, cache ID                                                                |
| [Chapter 11: Multi-Modal Models](.manual/chapter-11-multi-modal-models.md)             | Vision models, audio models, media input formats                                                                                             |
| [Chapter 12: Security & Authentication](.manual/chapter-12-security-authentication.md) | JWT auth, key management, token creation, rate limiting                                                                                      |
| [Chapter 13: Browser UI (BUI)](.manual/chapter-13-browser-ui.md)                       | Web interface, downloading libraries/models, catalog browsing, model management, key/token management, apps, model playground                |
| [Chapter 14: Client Integration](.manual/chapter-14-client-integration.md)             | OpenCode install + agent bundles in `.agents/` (default + rote), make targets, model config and swapping models, OpenWebUI, Python SDK, curl, LangChain |
| [Chapter 15: Observability](.manual/chapter-15-observability.md)                       | Debug server, Prometheus metrics, pprof profiling, tracing                                                                                   |
| [Chapter 16: MCP Service](.manual/chapter-16-mcp-service.md)                           | Brave Search, MCP configuration, OpenCode client setup, curl testing                                                                         |
| [Chapter 17: Troubleshooting](.manual/chapter-17-troubleshooting.md)                   | Common issues, error messages, debugging tips                                                                                                |
| [Chapter 18: Bucky (Audio Transcription)](.manual/chapter-18-bucky.md)                 | whisper.cpp-based transcription: install/libs, model catalog, server/pool config, CLI, BUI, API endpoint, SDK quick-start, languages, troubleshooting |
| [Chapter 19: Developer Guide](.manual/chapter-19-developer-guide.md)                   | Build commands, project architecture, BUI development, code style, SDK internals, MTP engine internals, Bucky internals, CI workflows (lint/test, goreleaser, container build & publish), version stamping |
