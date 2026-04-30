# AGENTS.md

Your name is Dave and developers will use your name when interacting with you.

The manual has been split into separate chapter files in the `.manual/` directory. Read only the chapters relevant to your task. Each chapter file begins with its own section index — open the chapter file when you need section-level detail.

You will want to look at `Chapter 17: Developer Guide` for detailed information about the project structure, code, and workflows.

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
| [Chapter 2: Installation & Quick Start](.manual/chapter-02-installation.md)            | Prerequisites, CLI install, libraries, downloading models, starting server, model config file                                                |
| [Chapter 3: Model Configuration](.manual/chapter-03-model-configuration.md)            | GPU config, KV cache, flash attention, NSeqMax, VRAM estimation, GGUF quantization, MoE vs dense vs hybrid performance, speculative decoding |
| [Chapter 4: Batch Processing](.manual/chapter-04-batch-processing.md)                  | Slots, sequences, request flow, memory overhead, concurrency by model type                                                                   |
| [Chapter 5: Message Caching](.manual/chapter-05-message-caching.md)                    | Incremental Message Cache (IMC), hybrid model IMC, multi-user IMC, cache invalidation                                                        |
| [Chapter 6: YaRN Extended Context](.manual/chapter-06-yarn-extended-context.md)        | RoPE scaling, YaRN configuration, context extension                                                                                          |
| [Chapter 7: Model Server](.manual/chapter-07-model-server.md)                          | Server start/stop, configuration, model caching, config files, catalog system                                                                |
| [Chapter 8: API Endpoints](.manual/chapter-08-api-endpoints.md)                        | Chat completions, Responses API, embeddings, reranking, tool calling                                                                         |
| [Chapter 9: Request Parameters](.manual/chapter-09-request-parameters.md)              | Sampling, repetition control, generation control, grammar, logprobs, cache ID                                                                |
| [Chapter 10: Multi-Modal Models](.manual/chapter-10-multi-modal-models.md)             | Vision models, audio models, media input formats                                                                                             |
| [Chapter 11: Security & Authentication](.manual/chapter-11-security-authentication.md) | JWT auth, key management, token creation, rate limiting                                                                                      |
| [Chapter 12: Browser UI (BUI)](.manual/chapter-12-browser-ui.md)                       | Web interface, downloading libraries/models, catalog browsing, model management, key/token management, apps, model playground                |
| [Chapter 13: Client Integration](.manual/chapter-13-client-integration.md)             | Coding agent setup (Cline, Kilo, OpenCode, Goose), model config, OpenWebUI, Python SDK, curl, LangChain                                      |
| [Chapter 14: Observability](.manual/chapter-14-observability.md)                       | Debug server, Prometheus metrics, pprof profiling, tracing                                                                                   |
| [Chapter 15: MCP Service](.manual/chapter-15-mcp-service.md)                           | Brave Search, MCP configuration, Cline/Kilo client setup, curl testing                                                                       |
| [Chapter 16: Troubleshooting](.manual/chapter-16-troubleshooting.md)                   | Common issues, error messages, debugging tips                                                                                                |
| [Chapter 17: Developer Guide](.manual/chapter-17-developer-guide.md)                   | Build commands, project architecture, BUI development, code style, SDK internals                                                             |
