## ROADMAP

### AUTOMATION

- Look at what Llama.cpp vs Yzma vs Kronk and identify changes.

- New a github workflow for released: add support to Release to update Proxy server.

- Our own machine for running test.

### SDK

- Add Tokenize API to SDK and MKS

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

#### Anthropic API Compatibility

`/v1/messages` endpoint enables tools like Claude Code to work with Kronk

#### Logprobs - Return token log probabilities for prompt engineering and debugging

Yzma exposes raw logits via GetLogits() and GetLogitsIth() in pkg/llama/context.go, returning []float32 arrays. You would need to manually apply log-softmax to convert these to log probabilities.

What's missing: No direct access to llama_sampler_get_data() or convenience wrappers for per-token log probabilities during sampling. So implementing Logprobs in kronk is possible but would require additional work to expose and compute the values from raw logits.

#### Structured Outputs (JSON Schema) - Support `format` as a JSON schema, not just `json` boolean

Yes, the OpenAI Chat Completions API supports Structured Outputs via the response_format parameter. You can set response_format: { type: "json_schema", json_schema: {...}, strict: true } to ensure model outputs match your supplied JSON schema exactly. This works with gpt-4o-mini, gpt-4o-2024-08-06, and later models.

Based on the search results, Structured Outputs works with OpenAI's GPT models and their reasoning models (o1, o3, o3-pro, o4-mini). It's an OpenAI API feature that requires models specifically trained for schema adherence.

For local models via llama.cpp (like kronk uses), you'd need to implement constrained decoding using grammars—llama.cpp supports GBNF grammars to constrain output to valid JSON matching a schema, which achieves similar results through a different mechanism.

#### `suffix` Parameter - Fill-in-the-middle completion support

yzma exposes FIM token functions: `VocabFIMPre()`, `VocabFIMSuf()`, `VocabFIMMid()`, etc.

Implementation: construct prompt as `<FIM_PRE>{prefix}<FIM_SUF>{suffix}<FIM_MID>`, model generates the middle

Caveat: FIM must be trained into the model; only certain models support it (CodeLlama, StarCoder, CodeGemma, etc.)

#### `kronk push`

Push custom models to a registry

### SGLANG FEATURE PARITY

https://medium.com/@aadishagrawal/sglang-how-a-secret-weapon-is-turbocharging-llm-inference-b9a7bd9ea43e

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
