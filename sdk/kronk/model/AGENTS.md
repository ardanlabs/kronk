# AGENTS.md - sdk/kronk/model

Low-level model inference using yzma (llama.cpp Go bindings).

## Package Overview

- `model.go` - Model type, context management, lifecycle
- `chat.go` - Chat inference loop, batch vs sequential routing
- `batch.go` - Batch engine for parallel text inference
- `config.go` - Model configuration (GPU, cache, batching)
- `models.go` - OpenAI-compatible types (ChatMessage, ToolCall, etc.)
- `embed.go` - Embedding inference
- `rerank.go` - Reranking inference
- `media.go` - Vision/audio media processing
- `processor.go` - Template-specific token processors
- `prompts.go` - Prompt formatting
- `params.go` - Sampling parameters
- `logprobs.go` - Token log probability extraction
- `check.go` - Model validation
- `caching.go` - System prompt and IMC cache management

## ChatStreaming: Batch vs Sequential Routing

`ChatStreaming` (`chat.go`) decides between two processing paths:

**Decision Logic** (`chat.go:89-120`):

```go
// Use batch engine for text-only requests when available.
if m.batch != nil && object == ObjectChatText {
    // Submit to batch engine...
    return
}
// Sequential path for media requests or when engine is not available.
m.sequentialChatRequest(...)
```

**Batch Engine Path** (text-only, `NSeqMax > 1`):

- Used when: `m.batch != nil` AND `object == ObjectChatText`
- `m.batch` is created in `NewModel` only when `NSeqMax > 1` for text models
- Job submitted to `batchEngine.requestQ` channel
- Engine runs `nSlots` parallel inference slots sharing one model context
- Each slot has its own `seqID` for isolated KV cache segments
- `batching = true` flag prevents cleanup in `ChatStreaming` defer (engine handles it)

**Sequential Path** (media or single-slot):

- Used when: `m.batch == nil` OR `object == ObjectChatMedia`
- Media requests (`ProjFile` set) always take this path—can't batch media tokens
- Calls `m.sequentialChatRequest()` directly
- `batching = false`, so defer handles `resetContext()` and channel close

**Why media can't use batch engine:**

- `mtmd.Context` (vision/audio projector) is per-request
- Media tokens are processed through separate pipeline (`mtmd.InputChunksInit`)
- Each request needs exclusive model context for media embedding

**Batch Engine Architecture** (`batch.go`):

- `batchEngine` manages `nSlots` parallel `slot` structs
- Each `slot` tracks: `seqID`, prompt tokens, decode state, sampler, response channel
- Wake channel pattern: `wakeCh chan struct{}` (buffered size 1) for coalesced wake signals
- `submit()` sends non-blocking wake after queuing; `processLoop` listens on `wakeCh`
- Polling intervals: 100µs (active), 5ms (idle)
- `llama.MemorySeqRm(mem, s.seqID, -1, -1)` clears slot's KV cache segment on finish

**Slot Optimizations** (`batch.go`):

- `slot.seqIDs []llama.SeqId`: Pre-allocated at slot creation as `[]llama.SeqId{seqID}`, reused in `batchAdd` calls to avoid per-token allocations during prefill

**Slots vs Sequences** (`batch.go`):

Slots and sequences are 1:1, but they are different concepts:

- `slot.id` = slot index (0, 1, 2...)—for logging/identification only
- `slot.seqID` = llama.cpp sequence ID—determines which KV cache partition the slot uses

Sequences are isolated partitions in the shared KV cache memory. Each request's key-value states are stored in its assigned sequence without interfering with other concurrent requests.

When caching is enabled, sequence 0 is reserved for cached prompts, so slot seqIDs are offset:

```
NSeqMax = 2
Without caching:        slot[0].seqID=0, slot[1].seqID=1
With SystemPromptCache: slot[0].seqID=1, slot[1].seqID=2  SPC cached in seqID=0
With IncrementalCache:  slot[0].seqID=1, slot[1].seqID=2  IMC cached in seqID=0
```

Note: SPC and IMC are mutually exclusive. IMC caches all messages except the last one, extending incrementally on each turn (for agentic workflows).

When a cache hit occurs, the KV states from sequence 0 are copied into the slot's sequence via copyCachesToSeq(seqID). The slot then continues from that point with nPast set to skip re-processing those tokens.

Slots are for inference. Cache sequences are just pre-computed KV state storage.

Per-request flow:

1. Request assigned to available slot (e.g., slot 0 with seqID=1)
2. Slot clears its sequence: `MemorySeqRm(mem, seqID, -1, -1)`
3. If cache hit: copies reserved seq → slot's seq, sets `nPast` to skip re-processing
4. Tokenizes remaining prompt, prefills into slot's sequence
5. Decodes tokens, slot becomes available for next request

## Context Pooling

- `llama.Context` is created once in `NewModel` and reused across requests
- Call `resetContext()` (uses `llama.MemoryClear`) between requests to clear KV cache
- Avoids Vulkan memory fragmentation from repeated context alloc/dealloc

## KV Cache Type Configuration

- `CacheTypeK` and `CacheTypeV` fields on `Config` control cache precision
- Uses `GGMLType` constants: `GGMLTypeF16=1`, `GGMLTypeQ8_0=8`, `GGMLTypeBF16=30`, etc.
- `GGMLTypeAuto=-1` uses llama.cpp defaults

## Resource Lifecycle

- Sampler chain freed via `defer llama.SamplerFree(sampler)` in `processChatRequest`
- Media path: `mtmd.InputChunksInit()` must be freed with `mtmd.InputChunksFree(output)`

## Jinja Template Caching (`prompts.go`)

- `Model.compiledTmpl *compiledTemplate`: Cached compiled template
- `Model.templateOnce sync.Once`: Ensures single compilation per model
- Template compiles once on first use via `applyRequestJinjaTemplate()`
- Eliminates per-request template parsing overhead

## Input Mutation Handling (`chat.go`, `media.go`)

No deep copying is used. Cloning happens only at specific mutation points:

- **Text-only models**: Input `D` passed directly to Jinja without copying
- **Media models (OpenAI format)**: `prepareMediaContext()` passes `d.Clone()` to `toMediaMessage()` before mutating content
- **Media models (plain base64)**: `convertPlainBase64ToBytes()` clones `D` and messages before replacing content with `[]byte`

The shallow `Clone()` method (maps.Copy) is sufficient since only top-level keys are mutated.

## Config Fields Reference

- `NSeqMax`: For text models, max parallel sequences for batched inference. For sequential models (embed/rerank/vision/audio), creates that many model instances in a pool. (0 = default of 1)
- `OffloadKQV`: KV cache on GPU (nil/true) or CPU (false)
- `OpOffload`: Tensor ops on GPU (nil/true) or CPU (false)
- `NGpuLayers`: Layers to offload (0 = all, -1 = none, N = specific count)
- `SplitMode`: Multi-GPU split (`SplitModeNone=0`, `SplitModeLayer=1`, `SplitModeRow=2` for MoE)
- `SystemPromptCache`: Cache system prompt (role="system") KV state in sequence 0 (see below)
- `IncrementalCache`: Incremental Message Cache (IMC) for agentic workflows - caches all messages except last, extends incrementally (see below)
- `CacheMinTokens`: Minimum tokens before caching (default: 100)

## RoPE/YaRN Extended Context Configuration

RoPE (Rotary Position Embedding) scaling enables context windows beyond a model's native training length. YaRN (Yet another RoPE extensioN) is the recommended method for extending context 2-4x.

**Scaling Types** (`RopeScalingType`):

- `RopeScalingNone` (0): Disabled, use native context length
- `RopeScalingLinear` (1): Linear interpolation, simple but less effective for large extensions
- `RopeScalingYaRN` (2): Frequency-dependent interpolation with attention scaling, recommended

**Config Fields**:

- `RopeScaling`: Scaling method (`RopeScalingNone`, `RopeScalingLinear`, `RopeScalingYaRN`)
- `RopeFreqBase`: Base frequency override (nil = model default; common: 10000 for Llama, 1000000 for Qwen3)
- `RopeFreqScale`: Frequency scaling factor (nil = auto-calculate from context extension ratio)
- `YarnExtFactor`: Extrapolation mix factor (nil = auto-calculate; 0 = disable extrapolation)
- `YarnAttnFactor`: Attention magnitude scaling (nil = default 1.0)
- `YarnBetaFast`: Low correction dimension (nil = default 32.0)
- `YarnBetaSlow`: High correction dimension (nil = default 1.0)
- `YarnOrigCtx`: Original training context size (nil/0 = use model metadata)

**Example: Qwen3 32k → 131k**:

```go
cfg := model.Config{
    ContextWindow: 131072,
    RopeScaling:   model.RopeScalingYaRN,
    // Other YaRN params auto-calculated from context ratio
}
```

**When to use YaRN vs Linear**:

- YaRN: 2-4x context extension, maintains quality better at longer contexts
- Linear: Simple extension, quality degrades more at high ratios

## Model-Specific Tuning Guidelines

- Vision/Audio models: keep `NUBatch` high (≥2048) for image/audio token processing
- MoE models: use `SplitModeRow` for multi-GPU, be cautious with aggressive cache quantization
- Embedding models: `NBatch` can equal `ContextWindow`, align `NUBatch` with sliding window

## Tool Call Handling

**chatMessage Unmarshaling** (`models.go`):

- `Content` can be `nil` for assistant messages with tool_calls or tool role messages
- Handle `len(app.Content) == 0 || string(app.Content) == "null"` as valid empty content

**ToolCallArguments type** (`models.go`):

- Custom type that marshals to JSON string (OpenAI spec) but unmarshals from either string or object
- Used in `ResponseToolCallFunction.Arguments` field
- `MarshalJSON`: wraps `map[string]any` as a JSON-encoded string
- `UnmarshalJSON`: tries string first, falls back to object for non-compliant clients

## Logprobs Support

Token log probabilities can be returned for chat completions via the `logprobs` and `top_logprobs` request parameters.

**Request Parameters** (`params.go`):

- `logprobs` (bool): When true, returns log probability for each generated token. Default: false.
- `top_logprobs` (int): Number of most likely alternative tokens to return (0-5). Setting > 0 implicitly enables `logprobs`. Default: 0.

**Response Structure** (`models.go`):

- `Choice.Logprobs *Logprobs`: Contains token probability data when requested
- `Logprobs.Content []ContentLogprob`: Array of per-token log probability data
- `ContentLogprob`: Token string, log probability (≤0), byte representation, and optional top alternatives
- `TopLogprob`: Alternative token with its log probability and bytes

**Implementation** (`logprobs.go`):

- `extractLogprobs()`: Retrieves logits via `llama.GetLogitsIth()`, converts to log probabilities
- `logSoftmax()`: Numerically stable log-softmax using log-sum-exp trick
- `getTopKLogprobs()`: Uses min-heap for efficient O(n log k) top-k extraction

**Streaming vs Non-Streaming Behavior**:

- **Non-streaming**: All logprobs accumulated and returned in final response `Choice.Logprobs`
- **Streaming**: Per-token logprobs sent in each delta chunk; final chunk has `Logprobs: nil`

**Critical Implementation Detail**:

Logprobs must be extracted **before** `llama.SamplerAccept()` is called. After accept, the sampler may modify internal state that affects logit retrieval.

## Response Structure

**Choice and ResponseMessage** (`models.go`):

- `Choice` has `Message *ResponseMessage` and `Delta *ResponseMessage` (same type)
- `FinishReasonPtr *string` with `FinishReason()` accessor returning empty string if nil
- Constants: `FinishReasonStop="stop"`, `FinishReasonTool="tool_calls"`, `FinishReasonError="error"`

**ResponseMessage fields**:

- `Role` - message role (e.g., "assistant")
- `Content` - text content
- `Reasoning` - reasoning content (JSON field: `reasoning_content`)
- `ToolCalls []ResponseToolCall` - tool call array

**Final chunk behavior** (`chatResponseFinal`):

- Sets both `Message` and `Delta` to the same `ResponseMessage` with full content
- `FinishReasonPtr` set to `FinishReasonStop` or `FinishReasonTool` (if tool calls present)

**Delta chunk behavior** (`chatResponseDelta`):

- Only `Delta` is set (not `Message`)
- `FinishReasonPtr` is nil for intermediate chunks

**Media processing** (`media.go`):

- Handle `nil` content in `toMediaMessage` with `case nil: continue`

## Message Caching (System Prompt / Incremental Message Cache)

Two cache modes are available (mutually exclusive):

- **`SystemPromptCache` (SPC)**: Caches the first message with `role="system"`. If a subsequent request has no system message but the cache exists, the cached system prompt is used. Ideal for Open Web UI and similar clients that send the system prompt once.
- **`IncrementalCache` (IMC)**: Incremental Message Cache for agentic workflows. Caches all messages except the last one, extending incrementally on each turn. Ideal for agents like Amp, Cline, or Aider where conversations grow monotonically.
- **`MaxIMCSessions`**: Maximum number of concurrent IMC sessions (users). Each session gets a dedicated cache sequence identified by `imc_id` in the request. Default: 1.

SPC and IMC are mutually exclusive; IMC includes the system prompt in its cache, so enabling both is redundant and will return a validation error.

**Multi-User IMC:**

IMC supports multiple users, each with their own dedicated cache sequence. Clients must pass the `KRONK_IMC_ID` header (or `imc_id` in the request D) to activate IMC. Each unique ID gets its own session up to `MaxIMCSessions`. If all slots are in use, requests gracefully bypass IMC.

**IMC Session State** (`model.go`):

```go
type imcSession struct {
    hash      string      // Hash of all cached messages
    tokens    int         // Total tokens in cache
    msgCount  int         // Number of messages cached
    promptLen int         // Length of templated prefix (for extension)
    seqID     llama.SeqId // Assigned cache sequence ID
    lastUsed  time.Time   // Last access time (for future eviction)
}

// Model fields for IMC
imcSessions map[string]*imcSession // Sessions keyed by imc_id
imcNextSeq  llama.SeqId            // Next available cache sequence
imcMaxSeqs  int                    // Max sessions from config
```

**IMC Algorithm** (`caching.go`):

1. **First request** (cache empty): Cache `messages[0:len-1]`, generate from last message
2. **Subsequent requests** (prefix match): Extend cache with `messages[cachedCount:len-1]`
3. **New thread** (prefix mismatch): Rebuild cache from scratch

**API Pattern** (`caching.go`):

`processCache()` returns a `cacheResult` struct:

```go
type cacheResult struct {
    modifiedD D         // D with cached messages removed
    prompt    string    // Templated prompt (set when caching occurs)
    media     [][]byte  // Media from templating (set when caching occurs)
    nPast     llama.Pos // Cumulative starting position from all cache hits
    cached    bool      // True if any cache is being used
    err       error     // Any error that occurred
}
```

**IMC Functions** (`caching.go`):

- `handleIncrementalMessageCache()`: Entry point, decides build vs extend vs hit
- `buildIMCCache()`: Builds cache from scratch for new conversations
- `extendIMCCache()`: Extends existing cache with new messages incrementally
- `generateIMCSuffix()`: Extracts suffix from full template for immediate use
- `hashMessages()`: Computes SHA-256 hash of message slice for cache validation
- `decodeExtensionTokens()`: Decodes extension tokens without clearing sequence

**How IMC works**:

1. **Cache miss (first request)**:
   - Hash `messages[0:len-1]` with `hashMessages()`
   - Template with `add_generation_prompt=false`
   - Check token count against `CacheMinTokens` (default: 100)
   - Decode tokens to seq 0 via `decodeTokensToSeq()`
   - Store `imcHash`, `imcTokens`, `imcMsgCount`, `imcPromptLen`
   - Generate suffix via `generateIMCSuffix()` for immediate use

2. **Cache hit with extension**:
   - Verify `messages[0:imcMsgCount]` matches `imcHash`
   - Template new prefix `messages[0:len-1]`
   - Extract extension: `newPrefix[imcPromptLen:]`
   - Decode extension tokens via `decodeExtensionTokens()` (no sequence clear)
   - Update cache state with new totals
   - Return suffix for generation

3. **Cache hit (no extension)**:
   - Prefix matches and covers all cacheable messages
   - Return `nPast = imcTokens`, no templating needed

4. **Prefix mismatch (new thread)**:
   - Detected when `hashMessages(messages[:imcMsgCount]) != imcHash`
   - Clear seq 0 and rebuild cache via `buildIMCCache()`

**SystemPromptCache special case**:

- No system message but cache exists → use cached system prompt
- Returns original D (not modified), `nPast` set to cached tokens

**Sequence ID layout (dynamic based on config):**

| SPC | IMC | MaxIMCSessions | Reserved Seqs | Slot Start | Memory Overhead |
| --- | --- | -------------- | ------------- | ---------- | --------------- |
| off | off | -              | 0             | seq 0      | none            |
| on  | off | -              | 1 (seq 0)     | seq 1      | +1 ctx window   |
| off | on  | 1              | 1 (seq 0)     | seq 1      | +1 ctx window   |
| off | on  | 4              | 4 (seq 0-3)   | seq 4      | +4 ctx windows  |

Note: SPC and IMC are mutually exclusive, so both cannot be enabled together.

Example with `MaxIMCSessions=3, NSeqMax=2`:
- seq 0: imc_id="user-1" cache
- seq 1: imc_id="user-2" cache
- seq 2: imc_id="user-3" cache
- seq 3: slot[0] inference
- seq 4: slot[1] inference

**NSeqMax and Caching Relationship:**

Dynamic sequence allocation in `config.go`:

```go
nSeqMax := max(cfg.NSeqMax, 1)
cacheSeqs := 0
if cfg.SystemPromptCache {
    cacheSeqs = 1
} else if cfg.IncrementalCache {
    cacheSeqs = max(cfg.MaxIMCSessions, 1)
}
ctxParams.NSeqMax = uint32(nSeqMax + cacheSeqs)
```

**Batch engine slot assignment** (`batch.go`):

```go
cacheSeqs := 0
if m.cfg.SystemPromptCache {
    cacheSeqs = 1
} else if m.cfg.IncrementalCache {
    cacheSeqs = m.imcMaxSeqs
}

for i := range slots {
    slots[i] = &slot{
        id:    i,
        seqID: llama.SeqId(i + cacheSeqs),
    }
}
```

**Request flow with caching** (`batch.go`):

1. Slot clears its sequence: `llama.MemorySeqRm(mem, s.seqID, -1, -1)`
2. If SPC cached: copies KV state via `copySystemPromptToSeq(s.seqID)` from seq 0
3. If IMC cached: copies KV state via `copyCachesToSeq(s.seqID, job.imcSeqID)` from session's seq
4. Sets `nPast` to skip re-processing cached tokens
5. Tokenizes remaining prompt (only the suffix/last message)

**Cache invalidation:**

- Prefix mismatch (IMC): clears session's seq, rebuilds cache from new conversation
- Hash mismatch (SPC): clears seq 0, re-evaluates new system message
- Different role with same content: different hash (role is included in hash)
- `resetContext()`: clears all memory AND calls `clearCaches()` (sequential path)
- `clearCaches()`: Resets SPC fields and clears all IMC sessions from `imcSessions` map

**Critical Implementation Details:**

1. **Extension tokenization must use `special=true`**: When tokenizing the extension string in `extendIMCCache()`, use `llama.Tokenize(vocab, extension, false, true)`. The `special=true` parameter ensures ChatML control tokens like `<|im_start|>` and `<|im_end|>` are recognized as special tokens. Using `special=false` causes these tokens to be tokenized as literal text, which leaks into model output as garbage.

2. **Prefix mismatch detection via `strings.HasPrefix`**: Both `extendIMCCache()` and `generateIMCSuffix()` verify that the full templated prompt starts with the cached prefix using `strings.HasPrefix(fullPrompt, prefixPrompt)`. If this check fails, it indicates Jinja template nondeterminism (e.g., timestamps, random values) and triggers a cache rebuild. Without this check, the suffix extraction would produce incorrect results.

3. **Extension string extraction**: The extension is `newPrefixPrompt[imcPromptLen:]` - the substring of the new prefix after the cached prefix length. This relies on template determinism: the same messages must always produce the same prefix.

4. **`add_generation_prompt=false` for cached prefixes**: When templating messages for caching, always set `add_generation_prompt=false`. This creates a valid prefix that can be extended. The generation prompt (`<|im_start|>assistant\n`) is only added when templating the final suffix.

**Limitations:**

- Only works for text-only requests (`ObjectChatText`)
- Sequential path calls `resetContext()` which clears all caches
- Messages shorter than `CacheMinTokens` are not cached
- Requires monotonically growing conversations (editing earlier messages triggers rebuild)
- Thread-safe via `cacheMu` mutex (read lock for hits, write lock for misses)
- Template must be deterministic (no timestamps, random values, etc.)
- IMC requires `imc_id` in request (via `KRONK_IMC_ID` header) - requests without it bypass IMC
- If all `MaxIMCSessions` slots are in use, new sessions gracefully bypass IMC
