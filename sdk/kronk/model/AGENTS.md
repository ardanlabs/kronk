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
- `check.go` - Model validation
- `sysprompt.go` - System prompt KV cache management

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
- Signal-based wake: sleeps until `requestQ` has jobs or slots are active
- Polling intervals: 100µs (active), 5ms (idle)
- `llama.MemorySeqRm(mem, s.seqID, -1, -1)` clears slot's KV cache segment on finish

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

## Config Fields Reference

- `NSeqMax`: For text models, max parallel sequences for batched inference. For sequential models (embed/rerank/vision/audio), creates that many model instances in a pool. (0 = default of 1)
- `OffloadKQV`: KV cache on GPU (nil/true) or CPU (false)
- `OpOffload`: Tensor ops on GPU (nil/true) or CPU (false)
- `NGpuLayers`: Layers to offload (0 = all, -1 = none, N = specific count)
- `SplitMode`: Multi-GPU split (`SplitModeNone=0`, `SplitModeLayer=1`, `SplitModeRow=2` for MoE)
- `SystemPromptCache`: Cache system prompt (role="system") KV state in sequence 0 (see below)
- `FirstMessageCache`: Cache first user message (role="user") KV state in sequence 0 (see below)
- `CacheMinTokens`: Minimum tokens before caching (default: 100)

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

## Message Caching (System Prompt / First Message)

Two mutually exclusive cache modes are available:

- **`SystemPromptCache`**: Caches messages with `role="system"`. If a subsequent request has no system message but the cache exists, the cached system prompt is used. Ideal for Open Web UI and similar clients that send the system prompt once.
- **`FirstMessageCache`**: Caches messages with `role="user"`. Ideal for clients like Cline that use a large first user message as context.

Both modes are mutually exclusive - only one can be enabled at a time.

**API Pattern** (`sysprompt.go`):

`ensureFirstMessageCached()` returns a `cacheResult` struct:

```go
type cacheResult struct {
    modifiedD D         // D with first message removed if cache was used
    prompt    string    // Templated prompt (set when caching occurs)
    media     [][]byte  // Media from templating (set when caching occurs)
    nPast     llama.Pos // Starting position for new tokens
    cached    bool      // True if cache is being used
    err       error     // Any error that occurred
}
```

**Integration with ChatStreaming** (`chat.go:105-126`):

```go
if (m.cfg.SystemPromptCache || m.cfg.FirstMessageCache) && object == ObjectChatText {
    cache := m.ensureFirstMessageCached(ctx, d)
    if cache.err != nil {
        m.sendChatError(ctx, ch, id, cache.err)
        return
    }
    d = cache.modifiedD
    sysPromptNPast = cache.nPast
    sysPromptCached = cache.cached
    prompt = cache.prompt
    media = cache.media
}

// Only call createPrompt if caching didn't already handle it.
if prompt == "" {
    prompt, media, err = m.createPrompt(ctx, d)
    ...
}
```

**How it works** (`sysprompt.go`):

1. **Cache miss (first request)**:
   - Extract first message, check role matches cache mode
   - Hash role+content, tokenize with `add_generation_prompt=false`
   - Check token count against `CacheMinTokens` (default: 100) - skip if too short
   - Decode tokens to sequence 0 via `decodeTokensToSeq0()`
   - Store hash/count in `Model.sysPromptHash` / `Model.sysPromptTokens`
   - Template full prompt, extract suffix (generation prompt portion)
   - Return `cacheResult` with `prompt` set to suffix for immediate use

2. **Cache hit (subsequent requests)**:
   - Same first message → return `cacheResult` with `modifiedD` (first message removed)
   - `nPast` set to cached token count (skip prefill)
   - `prompt` empty, so `chat.go` calls `createPrompt()` on remaining messages
   - Batch engine copies KV cache from seq 0 to slot's seqID

3. **SystemPromptCache special case**:
   - No system message but cache exists → use cached system prompt
   - Returns original D (not modified), `nPast` set to cached tokens

**Sequence ID layout:**

- Sequence 0: Reserved for cached message KV state
- Sequences 1-N: Used by batch engine slots

**NSeqMax and Caching Relationship:**

The `+1` adjustment happens at the llama.cpp level (`config.go:314-315`):

```go
nSeqMax := max(cfg.NSeqMax, 1)
ctxParams.NSeqMax = uint32(nSeqMax + 1)  // Reserve Seq0 for cache
```

With `nseq-max: 1`:

- llama.cpp allocates 2 sequences (0 and 1)
- Seq0 holds the cached KV state (read-only)
- Seq1 is the single inference slot
- `nSlots = 1` in batch engine

With `nseq-max: 2`:

- llama.cpp allocates 3 sequences (0, 1, and 2)
- Seq0 holds the cached KV state
- Seq1 and Seq2 are inference slots
- `nSlots = 2` in batch engine (2 concurrent requests)

**Minimum requirement:** `nseq-max: 1` is sufficient for caching. The +1 reservation is automatic. Use `nseq-max: 2` only if you need concurrent request handling.

**Batch engine slot assignment** (`batch.go:122-128`):

```go
for i := range slots {
    slots[i] = &slot{
        id:    i,
        seqID: llama.SeqId(i + 1),  // SeqID 0 reserved
    }
}
```

**Request flow with caching** (`batch.go:354-365`):

1. Slot clears its sequence: `llama.MemorySeqRm(mem, s.seqID, -1, -1)`
2. If cached, copies KV state: `copySystemPromptToSeq(s.seqID)` (Seq0 → slot's SeqID)
3. Sets `nPast` to skip re-processing cached tokens
4. Tokenizes remaining prompt (without cached prefix)

**Cache invalidation:**

- Hash mismatch: clears seq 0 via `llama.MemorySeqRm()`, re-evaluates new message
- Different role with same content: different hash (role is included in hash)
- `resetContext()`: clears all memory AND calls `clearSystemPromptCache()` (sequential path)

**Limitations:**

- Only works for text-only requests (`ObjectChatText`)
- Sequential path calls `resetContext()` which clears the cache
- Messages shorter than `CacheMinTokens` are not cached
- Thread-safe via `sysPromptMu` mutex (read lock for hits, write lock for misses)
