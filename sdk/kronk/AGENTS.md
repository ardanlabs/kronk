# AGENTS.md - sdk/kronk

Core API package for model loading, chat, embeddings, rerank, and metrics.

## Package Overview

- `kronk.go` - Main Kronk type, model pool management
- `chat.go` - Chat completion API
- `embedding.go` - Embedding API
- `rerank.go` - Reranking API
- `response.go` - OpenAI Responses API streaming
- `concurrency.go` - Generic streaming utilities
- `acquire.go` - Model pool acquire/release
- `init.go` - Initialization and configuration

## Streaming Architecture

**Response Streaming Pattern** (`response.go`, `concurrency.go`):

- Uses `streamingWith[T, U]` generic function for 1:N event transformation
- `streamProcessor` has three phases: `Start()`, `Process(chunk)`, `Complete(lastChunk)`
- `streamState` struct maintains response ID, sequence numbers, aggregated usage
- SSE format: `event: <type>\ndata: <json>\n\n`

**Key streaming events** (OpenAI Responses format):

- `response.created`, `response.in_progress` → emitted at start
- `response.output_text.delta`, `response.reasoning_summary_text.delta` → per chunk
- `response.function_call_arguments.delta` → for tool calls
- `*.done` events emitted at completion before `response.completed`

**FinishReason handling** (`response.go`, `model/models.go`):

- `FinishReasonPtr *string` field with `FinishReason()` accessor (returns empty string if nil)
- Constants: `FinishReasonStop="stop"`, `FinishReasonTool="tool_calls"`, `FinishReasonError="error"`
- When `FinishReasonPtr != nil`, skip text/reasoning deltas (they duplicate previous content)
- Always process tool calls even with FinishReason set (may only arrive in final chunk)

**Message and Delta structure** (`model/models.go`):

- Both `Message` and `Delta` use `*ResponseMessage` type (unified structure)
- `ResponseMessage` has: `Role`, `Content`, `Reasoning` (reasoning_content), `ToolCalls`
- Final chunk populates **both** `Message` and `Delta` with full content and tool calls
- Test helpers: use `Delta` only if streaming AND `FinishReasonPtr` is nil; otherwise use `Message`

## NSeqMax and Model Pooling Strategy

`NSeqMax` behaves differently depending on model type (`kronk.go`):

**Sequential Models** (embed, rerank, vision/audio with `ProjFile`):

- `NSeqMax` controls the **number of model instances** in the pool
- Each instance handles one request at a time (single-flight)
- Creates `NSeqMax` separate `model.Model` instances at startup
- Pooled via `krn.pool` channel for concurrent request handling
- Semaphore capacity = `NSeqMax` (1:1 with instances)

**Text Inference Models** (chat, completion):

- `NSeqMax` controls **batch parallelism within a single model instance**
- Only one `model.Model` instance is created
- Semaphore capacity = `NSeqMax * queueDepth` (default queueDepth=2)
- Allows requests to queue while current batch processes

**Detection Logic** (`kronk.go:112-117`):

```go
isSingleFlight := cfg.ProjFile != ""  // Vision/audio projector
if mi.IsEmbedModel || mi.IsRerankModel {
    isSingleFlight = true
}
```

**Why this matters:**

- Sequential models can't batch tokens across requests (each request is independent)
- Text models benefit from batched inference with shared KV cache
- Vision/audio models need separate context for media processing

## Model Acquire/Release & Cleanup

**Two-Stage Acquisition** (`acquire.go`):

1. **Backpressure slot**: Acquire semaphore slot (limits total in-flight requests)
2. **Model instance**: If pooled (`krn.pool != nil`), acquire specific model from pool

**Model Pool Pattern:**

- Sequential models: pooled via `krn.pool` channel for exclusive instance access
- Text models: single instance in `krn.models[0]`, no pool channel
- `acquireModel()` blocks until resources available, increments `activeStreams`
- `releaseModel()` returns model to pool (if pooled), releases semaphore, decrements `activeStreams`

**Cleanup Flow (ensures clean state before release):**

1. `streaming()` / `streamingWith()` acquire model, defer `releaseModel()` in wrapper goroutine
2. Wrapper calls `ChatStreaming()` which runs in its own goroutine
3. `ChatStreaming` defers `m.resetContext()` before any processing
4. When generation completes, `resetContext()` runs first:
   - `llama.Synchronize(m.lctx)` - waits for GPU operations to complete
   - `llama.MemoryClear(mem, true)` - clears KV cache
5. Then channel closes, wrapper goroutine exits, `releaseModel()` runs
6. Model returns to pool in clean state for next request

**Key invariant:** `resetContext()` always runs before model release due to defer ordering:

- Inner goroutine (`ChatStreaming`): `defer m.resetContext()` runs on exit
- Outer goroutine (concurrency wrapper): `defer krn.releaseModel()` runs after inner channel drains

## GPT-OSS Processor (processor.go)

**Token handling for gpt-oss template**:

- `<|return|>` and `<|call|>` return `io.EOF` (end of generation)
- `<|end|>` is a section terminator (continues to next section)
- `<|channel|>commentary` triggers tool call mode (`statusTooling`)
- State machine: `awaitingChannel` → `collectingName` → content collection

**Repetition penalty**: Applied via `llama.SamplerInitPenalties` with defaults `RepeatPenalty=1.1`, `RepeatLastN=64`
