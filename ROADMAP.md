## ROADMAP

### BUGS / ISSUES

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

- No obvious way to configure the `.kronk` storage directory. A full path, including the final name should be allowed

  - FLORIN: There is a `BaseDir` defaults function. All of the tools package
    allow you to override this. When you contruct the Kronk API, you can specify
    the `WithTemplateRetriever` implementation. This is an opportunity to use a
    different location. Same with downloading models and libs. There is always a
    `NewWithSettings` that let's you control this. I wanted the APIs for the
    defaults to be very simple.
    If you agree then let's remove this from the ROADMAP.

  - BILL: I will add support to the CLI and SERVER to override default locations
    for models, libs, catalog, and templates. Use a unified ENV VAR / flag

---

- Model download cache can be corrupted if a model download fails. The `.index.yaml` will show as `downloaded: true` even if it's not true.

  - FLORIN: This is complicated. It's hard to know if we have the full file or
    not. I will work out a solution.

    switch out `resolve` for `raw` and check the size in bytes
    Perform the sha256 and check that
    Add SHA256 to the catalog

    https://huggingface.co/unsloth/gpt-oss-20b-GGUF/raw/main/gpt-oss-20b-Q8_0.gguf
    https://huggingface.co/Qwen/Qwen3-8B-GGUF/raw/main/Qwen3-8B-Q8_0.gguf
    https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/raw/main/embeddinggemma-300m-qat-Q8_0.gguf

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
