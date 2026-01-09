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
- Add support for setting the KV cache type to different formats (FP8, FP16, FP4, etc)

### TELEMETRY

- Cache Usage
- Tokens/sec reported against a bucketed list of context sizes from the incoming requests
- Maintain stats at a model level

### API

- Investigate why OpenWebUI doesn't generate a "Follow-up" compared to when using other LLM runners

- Add responses API
  - https://docs.ollama.com/api/openai-compatibility

### AI-TRAINING

- Replease Ollama for KMS
