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

- Investigate why OpenWebUI doesn't generate a "Follow-up" compared to when using other LLM runners

- Add responses API
  - https://docs.ollama.com/api/openai-compatibility

### AI-TRAINING

- Remove Ollama for KMS
