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
- `SystemPromptCache`: Enable caching first message KV state in sequence 0 (see below)
- `SystemPromptCacheMinTokens`: Minimum tokens before caching first message (default: 100)

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

## System Prompt Caching

When `Config.SystemPromptCache` is enabled, the first message's KV cache is computed once and reused across requests with the same first message. This works for any role (system, user, or assistant), supporting both traditional system prompts and clients like Cline that use a large first user message as context.

**How it works** (`sysprompt.go`, `batch.go`):

1. On first request: Extract first message from D (any role), hash role+content, tokenize and decode to sequence 0
2. Check token count against `SystemPromptCacheMinTokens` (default: 100) - skip caching if too short
3. Store hash and token count in `Model.sysPromptHash` / `Model.sysPromptTokens`
4. Remove first message from D before creating prompt (avoids double-encoding)
5. On subsequent requests with same first message:
   - Hash matches → copy KV cache from seq 0 to slot's seqID via `MemorySeqCp`
   - Set `nPast` to cached token count (skip prefill for those tokens)
   - Tokenize remaining prompt without BOS (cached message already has it)
6. When slot finishes: clear slot's seq, then restore from seq 0 for next request

**Sequence ID layout:**

- Sequence 0: Reserved for cached first message KV state
- Sequences 1-N: Used by batch engine slots

**Cache invalidation:**

- Hash mismatch: clears seq 0, re-evaluates new first message, updates hash/count
- Different role with same content: different hash (role is included in hash)
- `resetContext()`: clears all memory AND calls `clearSystemPromptCache()` (sequential path)

**Limitations:**

- Only works for batch path (text-only requests)
- Sequential path calls `resetContext()` which clears the cache
- Messages shorter than `SystemPromptCacheMinTokens` are not cached
- Thread-safe via `sysPromptMu` mutex
