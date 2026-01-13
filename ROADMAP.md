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

---

### NEW WORK FOR BATCHING REQUESTS

## Thread Summary: Parallel Inference Research & Implementation

### LLM Inference Engine Comparison

**vLLM vs llama.cpp vs Ollama:**

| Engine        | Backend         | Key Features                                             | Best For                                   |
| ------------- | --------------- | -------------------------------------------------------- | ------------------------------------------ |
| **vLLM**      | Own Python/CUDA | PagedAttention, continuous batching, efficient scheduler | High-concurrency production serving        |
| **llama.cpp** | C/C++           | Quantization (GGUF), CPU/GPU hybrid, portability         | Single-user, edge devices, low-concurrency |
| **Ollama**    | Wraps llama.cpp | Easy CLI, model management, OpenAI-compatible API        | Developer convenience                      |

**Key Insight:** vLLM achieves up to 3x higher throughput at high concurrency due to PagedAttention and continuous batching. llama.cpp (and thus Ollama/kronk) does NOT support PagedAttention.

### What is PagedAttention?

PagedAttention is a memory management technique for LLM inference inspired by OS virtual memory paging:

- **Problem:** Traditional KV cache pre-allocates memory for max sequence length, wasting 60-80% of memory
- **Solution:** Break KV cache into small fixed-size blocks (e.g., 16 tokens), allocate on-demand
- **Benefits:**
  - Reduces memory waste from 60-80% → ~4%
  - Enables memory sharing across requests with common prefixes
  - Up to 24x throughput improvement in benchmarks

### Parallel Inference Implementation

Created two example programs demonstrating parallel inference with yzma/llama.cpp:

**Step 1: `examples/yzma-parallel/step1/main.go`**

- Direct port of llama.cpp's parallel.cpp example
- Simulates N clients with M total sequences
- Uses continuous batching and shared system prompt

**Step 2: `examples/yzma-parallel/step2/main.go`**

- Web server architecture with HTTP API
- Request queue → Batch processor (single goroutine) → Response channels
- Endpoints: `POST /v1/completions`, `GET /v1/stats`, `GET /health`

**Architecture:**

```
HTTP Handlers ──► Request Queue ──► Batch Processor ──► Response Channels
    (many)          (chan 100)      (single goroutine)     (per request)
                                           │
                                    llama.Decode(batch)
                                    (processes multiple
                                     sequences in parallel)
```

### Key Concepts

**n_parallel vs n_sequences:**

- `n_parallel`: Max concurrent requests (KV cache slots) at any moment
- `n_sequences`: Total requests to process (workload queue size)

**Batching independent requests:**

- Requests do NOT need any commonality
- Each gets its own sequence ID (KV cache slot)
- Only optimization: shared system prompt can be computed once and copied

**n_parallel constraints:**

- **KV Cache Memory:** Each sequence needs `context_length × layers × head_dim` memory
- **VRAM:** Rough estimate for 8B model: ~0.5 MB per token per sequence
- **Practical limits:** Consumer GPU (8-12GB): 2-4 parallel; RTX 4090: 4-8; A100: 16-32+

**n_predict:**

- Default max tokens to generate per request (like OpenAI's `max_tokens`)
- Prevents runaway generation

### Bug Fix: hasPromptDone Flag

Initial step2 implementation had a bug where `s.sampled` was added to batch before prompt decode completed. Fixed by adding `hasPromptDone` flag:

```go
type slot struct {
    // ...
    hasPromptDone bool // true after initial prompt decode
}

// Only add sampled token for slots that completed prompt decode
for _, s := range bp.slots {
    if !s.active || !s.hasPromptDone {
        continue
    }
    batchAdd(&bp.batch, s.sampled, ...)
}
```

### Running the Examples

```bash
# Step 1: Batch simulation
make example-yzma-parallel-step1

# Step 2: Web server
make example-yzma-parallel-step2

# Test with curl
curl -X POST http://localhost:8090/v1/completions \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Hello", "max_tokens": 50}'

# Concurrent load test
seq 1 10 | xargs -P 10 -I {} curl -s -X POST http://localhost:8090/v1/completions \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Request {}", "max_tokens": 30}'
```
