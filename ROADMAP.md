## ROADMAP

### BUGS / ISSUES

---

Review CLI Tooling docs are updating new flags.

Fix TraceID for logs with loading model file.

---

- Poor performance compared to other LLM runners

  - E.g. ~ 8 t/s response vs ~61 t/s and degrades considerably for every new message in the chat stream
  - Possible venues to investigate

    - Performance after setting the KV cache to FP8
    - Processing of tokens in batches

    OTEL tags for metrics!
    Add Spans

---

    Add support to Release to update Proxy server

---

### MODEL SERVER / TOOLING

- Add more models to the catalog. Look at Ollama's catalog.
- Add support for setting the KV cache type to different formats (FP8, FP16, FP4, etc)

### Telemetry

- Apply OTEL Spans to critical areas beyond start/stop request
- TTFT reporting
- Cache Usage
- Tokens/sec reported against a bucketed list of context sizes from the incoming requests
- Maintain stats at a model level

### API

- Implement the Charmbracelt Interface
  https://github.com/charmbracelet/fantasy/pull/92#issuecomment-3636479873
- Investigate why OpenWebUI doesn't generate a "Follow-up" compared to when using other LLM runners

### FRONTEND
