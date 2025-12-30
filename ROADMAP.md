## ROADMAP

### BUGS / ISSUES

- Logs still use the "SALES" service name
- Poor performance compared to other LLM runners
  - E.g. ~ 8 t/s response vs ~61 t/s and degrades considerably for every new message in the chat stream
  - Possible venues to investigate
    - Performance after setting the KV cache to FP8
    - Processing of tokens in batches
- Model download page will break when navigating away during a download in progress
- `KRONK_HF_TOKEN` needs to be configured in the CLI runner during `kronk server start` as
  the HF token configured via UI doesn't work. To verify this, pull a gated model from HF, e.g. gemma
- CLI flags are not working, env vars must be used to configure the server start
- No obvious way to configure the `.kronk` storage directory. A full path, including the final name should be allowed
- Model download cache can be corrupted if a model download fails. The `.index.yaml` will show as `downloaded: true` even if it's not true.
- Model download cache can be corrupted if a model folder is manually removed. Kronk will fail to start. The solution is to remove the `.index.yaml` file

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
