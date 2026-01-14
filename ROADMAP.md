## ROADMAP

### AUTOMATION

- Look at what Llama.cpp vs Yzma vs Kronk and identify changes.

- New a github workflow for released: add support to Release to update Proxy server.

- Our own machine for running test.

### PARALLEL INFERENCE RESEARCH & IMPLEMENTATION

- Multimode for Images (look before for details).

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

## BATCHING CHANGES

### Prefil Chunking

GOING TO DO Option 1 later

The issue is clear. In fillSlots() → startSlot(), when a new job is assigned to a slot, all prompt tokens are added to the batch in a single decode call (lines 280-284).

The batch was initialized with llama.BatchInit(int32(nCtx), 0, int32(nSlots)) at line 104, which uses the context size as the batch capacity. However, llama.cpp's llama_decode has a separate constraint: n_tokens_all <= cparams.n_batch (the NBatch config parameter, default 2048).

Root cause: When nseq-max=2 with 2 concurrent requests, if both prompts are prefilled in the same batch iteration, the combined tokens can exceed NBatch (e.g., two 1500-token prompts = 3000 tokens > 2048).

Potential fixes:

Chunk prefill - Add prompt tokens in chunks of NBatch / nSlots per iteration
Sequential prefill - Only prefill one slot per batch iteration
Check batch capacity - In startSlot, check remaining batch capacity before adding all tokens
The simplest fix is to chunk the prefill or only start one new slot per iteration when prefill hasn't completed.

You are saying 1 is the simplest fix?

The user is asking if option 1 (chunked prefill) is the simplest fix. Let me reconsider.

Actually, looking at this more carefully:

Chunk prefill - Requires tracking prefill progress per slot, multiple iterations to complete prefill, more complex state management
Sequential prefill - Simple: only call startSlot for one slot per fillSlots() iteration. Easy to implement.
Check batch capacity - Still need a way to defer starting a slot if capacity insufficient
Option 2 (sequential prefill) is actually simpler - just add a break or limit to only starting one slot per iteration. But this reduces parallelism during prefill.

The truly correct fix is option 1 (chunked prefill) because it allows parallel prefill while respecting NBatch limits, but it's more complex.

Let me reconsider - the user asked if I'm saying option 1 is simplest. No, I said "simplest fix is to chunk the prefill OR only start one new slot per iteration." Option 2 is simpler to implement but option 1 is more correct for performance.

No—option 2 (sequential prefill) is simpler to implement. Just add a break after startSlot in fillSlots():

case job := <-e.requestQ:
e.startSlot(s, job, buf)
return // Only start one slot per iteration
This ensures only one prompt prefills at a time, avoiding the combined token overflow.

Option 1 (chunked prefill) is the correct long-term solution for parallel prefill throughput, but requires tracking per-slot prefill progress and splitting tokens across multiple decode iterations—more complex state management.

### Multimode for Images

Good news: The yzma bindings DO support sequence IDs for multimodal!

func HelperEvalChunks(ctx Context, lctx llama.Context, chunks InputChunks,
nPast llama.Pos, seqID llama.SeqId, ...) // <-- seqID parameter exists!
The underlying llama.cpp API mtmd_helper_eval_chunks accepts a llama_seq_id parameter (line 82 comment shows this).

However, there's a mutex lock:

muHelperEvalChunks.Lock()
defer muHelperEvalChunks.Unlock()
This means the Go bindings serialize all HelperEvalChunks calls—only one can run at a time. This is likely for thread safety but prevents true parallel vision inference at the Go level.

Conclusion:

llama.cpp's mtmd does support sequence IDs for parallel inference
yzma's Go bindings serialize calls with a mutex (limiting parallelism)
To enable parallel vision, the mutex would need to be removed or the binding reworked
The current validation check is appropriate. Parallel vision would require changes to yzma.
