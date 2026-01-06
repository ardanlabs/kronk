## ROADMAP

### BUGS / ISSUES

- Poor performance compared to other LLM runners

  - E.g. ~ 8 t/s response vs ~61 t/s and degrades considerably for every new message in the chat stream
  - Possible venues to investigate
    - Performance after setting the KV cache to FP8
    - Processing of tokens in batches

---

- `KRONK_HF_TOKEN` needs to be configured in the CLI runner during `kronk server start` as
  the HF token configured via UI doesn't work. To verify this, pull a gated model from HF, e.g. gemma

  - FLORIN: I want this to be a single global settings for the server. I don't
    want people to pass this as part of the UI or as a parameter. The
    token you saw in the UI is a KRONK token not a HF token.
    If you agree then let's remove this from the ROADMAP.

---

- CLI flags are not working, env vars must be used to configure the server start

  - FLORIN: I will figure this out. I would like this to work since I want
    people to be able to start the server from the CLI.

---

- No obvious way to configure the `.kronk` storage directory. A full path, including the final name should be allowed

  - FLORIN: There is a `BaseDir` defaults function. All of the tools package
    allow you to override this. When you contruct the Kronk API, you can specify
    the `WithTemplateRetriever` implementation. This is an opportunity to use a
    different location. Same with downloading models and libs. There is always a
    `NewWithSettings` that let's you control this. I wanted the APIs for the
    defaults to be very simple.
    If you agree then let's remove this from the ROADMAP.

---

- Model download cache can be corrupted if a model download fails. The `.index.yaml` will show as `downloaded: true` even if it's not true.

  - FLORIN: This is complicated. It's hard to know if we have the full file or
    not. I will work out a solution.

---

- Model download cache can be corrupted if a model folder is manually removed. Kronk will fail to start. The solution is to remove the `.index.yaml` file

  - FLORIN: I have a functions that can rebuild the index for models and the
    catalog because of this. For the server, I can run these functions
    on startup to help. When people are using the CLI or writing
    programs, they will need to do this manually. The CLI tooling
    extends a command for this.

  - SOLUTION: On the KMS startup the model index file is rebuilt. There is a CLI
    command that can also be used to rebuild the index.

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
