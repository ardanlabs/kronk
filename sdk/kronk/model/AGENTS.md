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

**Media processing** (`media.go`):

- Handle `nil` content in `toMediaMessage` with `case nil: continue`
