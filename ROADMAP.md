## ROADMAP

### BUGS / ISSUES

- Poor performance compared to other LLM runners

  - E.g. ~ 8 t/s response vs ~61 t/s and degrades considerably for every new message in the chat stream
  - Possible venues to investigate
    - Performance after setting the KV cache to FP8
    - Processing of tokens in batches

- Add support to Release to update Proxy server

### MODEL SERVER / TOOLING

- Add more models to the catalog. Look at Ollama's catalog.

- We need to figure out a way to configure the model setting for a specific model.

### TELEMETRY

- Tokens/sec reported against a bucketed list of context sizes from the incoming requests
- Maintain stats at a model level

- Cache Usage
  Yes, yzma provides some memory information:
  Available APIs:
  llama.ModelSize(model) - Returns total tensor size in bytes. You're already using this in models.go to populate ModelInfo.Size.
  llama.GetMemory(ctx) - Returns a Memory handle for KV cache management (used in your resetContext() function).
  - Not available in yzma:
    Real-time VRAM usage per GPU
    Memory breakdown by component (weights vs. KV cache)
    Allocated vs. free memory stats
    For detailed runtime memory monitoring, you'd need OS-level tools or Go's runtime.MemStats for system RAM.

### API

- Use the catalog for known models to check if they support things for the call
  they are being used for. ie images/audio/embedding

- Investigate why OpenWebUI doesn't generate a "Follow-up" compared to when using other LLM runners

### AI-TRAINING

- Remove Ollama for KMS

### OLLAMA FEATURE PARITY

- **Anthropic API Compatibility** - `/v1/messages` endpoint enables tools like Claude Code to work with Kronk

- **`/v1/completions` Endpoint** - Raw text completion (non-chat) API for legacy tool compatibility

- **Logprobs** - Return token log probabilities for prompt engineering and debugging

- **Structured Outputs (JSON Schema)** - Support `format` as a JSON schema, not just `json` boolean

- **Web Search/Fetch API** - Provide `api/web_search` and `api/web_fetch` endpoints for RAG augmentation

- **`suffix` Parameter** - Fill-in-the-middle completion support

- **Embedding `dimensions`** - Allow reducing embedding dimensions on request

- **Embedding `truncate`** - Auto-truncate long inputs to fit context length

- **`kronk push`** - Push custom models to a registry

- **`kronk signin/signout`** - User authentication for cloud features
