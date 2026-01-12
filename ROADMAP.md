## ROADMAP

### Automation

- Look at what Llama.cpp vs Yzma vs Kronk and identify changes.

### Yzma / LLama Mutli Request Support

Start from this example:
https://github.com/ggml-org/llama.cpp/blob/537d4240d4f4dbd7f2eac1e3bf0452194fbb8e39/examples/parallel/parallel.cpp

Generate 128 client requests (-ns 128), simulating 8 concurrent clients (-np 8). The system prompt is shared (-pps), meaning that it is computed once at the start. The client requests consist of up to 10 junk questions (--junk 10) followed by the actual question.
llama-parallel -m model.gguf -np 8 -ns 128 --top-k 1 -pps --junk 10 -c 16384

Speculative Decoding:
https://github.com/ggml-org/llama.cpp/tree/537d4240d4f4dbd7f2eac1e3bf0452194fbb8e39/examples/speculative

### BUGS / ISSUES

- New a github workflow for released: add support to Release to update Proxy server

### SDK

- Add model_config defaults to the catalog which can be overridden by model_config
  or through the config with kronk.New

- Use the catalog for known models to check if they support things for the call
  they are being used for. ie images/audio/embedding

- Missing some potential samplers we could use.
  std::vector<enum common_sampler_type> samplers = {
  X COMMON_SAMPLER_TYPE_DRY,
  X COMMON_SAMPLER_TYPE_XTC,
  };

### TESTING

- Missing tool call tests in api.

### MCP and TOOL CALLING

- Support making tool calls on behalf of the user.
- Add a set of tools like web_search and web_fetch.
- Allow users to register/configure MCP tools.

### OLLAMA FEATURE PARITY

- **Anthropic API Compatibility** - `/v1/messages` endpoint enables tools like Claude Code to work with Kronk

- **Logprobs** - Return token log probabilities for prompt engineering and debugging

  Yzma exposes raw logits via GetLogits() and GetLogitsIth() in pkg/llama/context.go, returning []float32 arrays. You would need to manually apply log-softmax to convert these to log probabilities.

  What's missing: No direct access to llama_sampler_get_data() or convenience wrappers for per-token log probabilities during sampling. So implementing Logprobs in kronk is possible but would require additional work to expose and compute the values from raw logits.

- **Structured Outputs (JSON Schema)** - Support `format` as a JSON schema, not just `json` boolean

- **`suffix` Parameter** - Fill-in-the-middle completion support

  - yzma exposes FIM token functions: `VocabFIMPre()`, `VocabFIMSuf()`, `VocabFIMMid()`, etc.
  - Implementation: construct prompt as `<FIM_PRE>{prefix}<FIM_SUF>{suffix}<FIM_MID>`, model generates the middle
  - Caveat: FIM must be trained into the model; only certain models support it (CodeLlama, StarCoder, CodeGemma, etc.)

- **`kronk push`** - Push custom models to a registry

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
