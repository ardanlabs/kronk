# Chapter 17: Developer Guide

## Table of Contents

- [17.1 Quick Reference](#171-quick-reference)
- [17.2 Build & Test Commands](#172-build-test-commands)
- [17.3 Developer Setup](#173-developer-setup)
- [17.4 Project Architecture](#174-project-architecture)
- [17.5 BUI Frontend Development](#175-bui-frontend-development)
- [17.6 Code Style Guidelines](#176-code-style-guidelines)
- [17.7 SDK Internals](#177-sdk-internals)
  - [17.7.1 Package Structure](#1771-package-structure)
  - [17.7.2 Streaming Architecture](#1772-streaming-architecture)
  - [17.7.3 Concurrency Strategy](#1773-concurrency-strategy)
  - [17.7.4 Model Acquire/Release & Cleanup](#1774-model-acquirerelease-cleanup)
  - [17.7.5 Batch Engine Internals](#1775-batch-engine-internals)
  - [17.7.6 Context Pooling](#1776-context-pooling)
  - [17.7.7 IMC Implementation Details](#1777-imc-implementation-details)
  - [17.7.8 Tool Call Internals](#1778-tool-call-internals)
  - [17.7.9 Logprobs Implementation](#1779-logprobs-implementation)
- [17.8 API Handler Notes](#178-api-handler-notes)
- [17.9 Goroutine Budget](#179-goroutine-budget)
- [17.10 Request Tracing Spans](#1710-request-tracing-spans)
- [17.11 Inference Code Path](#1711-inference-code-path)
- [17.12 Inference Code Path (Detailed)](#1712-inference-code-path-detailed)
- [17.13 Reference Threads](#1713-reference-threads)

---

This chapter covers development workflows, build commands, and code
conventions for contributors to the Kronk project.

### 17.1 Quick Reference

Here is a quick chart of some of the more imporant make commands.

| Task            | Command                                             |
| --------------- | --------------------------------------------------- |
| Install CLI     | `make install-kronk`.                               |
| Run all tests   | `make test`                                         |
| Single test     | `go test -v -count=1 -run TestName ./sdk/kronk/...` |
| Run server      | `make kronk-server`                                 |
| Build BUI       | `make bui-build`                                    |
| Generate docs   | `make kronk-docs`                                   |
| Tidy modules    | `make tidy`                                         |
| Update deps     | `make deps-upgrade`                                 |
| Lint            | `staticcheck ./...`                                 |
| Developer setup | `make setup` (configures git hooks)                 |

### 17.2 Build & Test Commands

**Install CLI locally:**

```shell
go install ./cmd/kronk
```

**Run all tests:**

```shell
make test
```

Tests require prerequisites and environment variables:

```shell
# Install dependencies first
make install-libraries install-models

# Set required environment variables
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE=/path/to/kronk  # project root

# Run from project root directory
make test
```

**Run a single test:**

```shell
go test -v -count=1 -run TestName ./sdk/kronk/...
```

### 17.3 Developer Setup

Configure git hooks for automatic pre-commit checks:

```shell
make setup
```

This enables a pre-commit hook that automatically runs:

- `make kronk-docs` - Regenerates documentation
- `make bui-build` - Rebuilds the BUI frontend

### 17.4 Project Architecture

**Directory Structure:**

| Directory                      | Purpose                                                             |
| ------------------------------ | ------------------------------------------------------------------- |
| `cmd/kronk/`                   | CLI tool (subcommands: catalog, libs, model, run, security, server) |
| `cmd/server/`                  | OpenAI-compatible model server (gRPC + HTTP) with BUI frontend      |
| `cmd/server/api/tooling/docs/` | Documentation generator for BUI (SDK and CLI docs)                  |
| `sdk/kronk/`                   | Core API: model loading, chat, embeddings, cache, metrics           |
| `sdk/kronk/model/`             | Core inference and caching engine                                   |
| `sdk/kronk/observ/`            | Observability packages (metrics/, otel/)                            |
| `sdk/tools/`                   | Support for libs, models, catalogs, templates, and defaults         |

**Core Technology:**

Kronk uses [yzma](https://github.com/hybridgroup/yzma) (llama.cpp Go bindings)
for local inference with GGUF models.

### 17.5 BUI Frontend Development

The Browser UI is a React application located at:

```
cmd/server/api/frontends/bui/src/
```

**Directory Structure:**

| Directory/File | Purpose                                         |
| -------------- | ----------------------------------------------- |
| `components/`  | React components (pages and UI elements)        |
| `contexts/`    | React context providers for shared state        |
| `services/`    | API client (`api.ts`)                           |
| `types/`       | TypeScript type definitions                     |
| `App.tsx`      | Main app with routing configuration             |
| `index.css`    | Global styles (CSS variables, component styles) |

**Routing:**

Uses `react-router-dom` with `BrowserRouter`. Routes are defined in
`routeMap` in `App.tsx`.

**Adding New Pages:**

1. Create component in `components/` (e.g., `DocsSDKKronk.tsx`)
2. Add page type to `Page` union in `App.tsx`
3. Add route path to `routeMap` in `App.tsx`
4. Add `<Route>` element in `App.tsx`
5. Add `<Link>` entry to menu in `components/Layout.tsx`

**Menu Structure (`Layout.tsx`):**

Uses `MenuCategory[]` with properties:

- `id` - Unique identifier
- `label` - Display text
- `items` - Array of leaf pages
- `subcategories` - Nested menu categories

**State Management:**

| Context            | Purpose                                               |
| ------------------ | ----------------------------------------------------- |
| `TokenContext`     | Stores API token in localStorage (key: `kronk_token`) |
| `ModelListContext` | Caches model list data with invalidation support      |

Access via hooks: `useToken()`, `useModelList()`

**API Service (`services/api.ts`):**

- `ApiService` class with methods for all endpoints
- Streaming support for pull operations (models, catalog, libs)
- Auth-required endpoints accept token parameter

**Styling Conventions:**

- CSS variables defined in `:root` (colors: `--color-orange`, `--color-blue`, etc.)
- Common classes: `.card`, `.btn`, `.btn-primary`, `.form-group`, `.alert`, `.table-container`
- No CSS modules or styled-components; use global CSS classes

**Documentation Generation:**

| Type     | Generator Location                                            |
| -------- | ------------------------------------------------------------- |
| SDK docs | `cmd/server/api/tooling/docs/sdk/` (uses `go doc` output)     |
| CLI docs | `cmd/server/api/tooling/docs/cli/` (from command definitions) |
| Examples | Auto-generated from `examples/` directory                     |

Generate all documentation:

```shell
go run ./cmd/server/api/tooling/docs -pkg=all
```

### 17.6 Code Style Guidelines

**Package Comments:**

```go
// Package kronk provides the core inference API.
```

**Error Handling:**

```go
// Wrap errors with lowercase context prefix
return fmt.Errorf("loading model: %w", err)

// Declare package-level sentinel errors
var ErrModelNotFound = errors.New("model not found")
```

**Struct Design:**

- Use unexported fields with exported types
- Use `Config` pattern for constructors

```go
type Config struct {
    Host string
    Port int
}

func New(cfg Config) *Server {
    // ...
}
```

**Testing:**

Disable CGO in tests:

```shell
go test ./...
```

**Import Order (goimports):**

1. Standard library
2. External packages
3. Internal packages

**Control Flow:**

- Avoid `else` and `else if` clauses
- Prefer `switch` statements or early returns

```go
// Preferred: early return
if err != nil {
    return err
}
// continue with main logic

// Preferred: switch over if-else chains
switch state {
case "active":
    // ...

case "pending":
    // ...

default:
    // ...
}
```

### 17.7 SDK Internals

This section documents implementation details for developers working on
the Kronk SDK packages.

#### 17.7.1 Package Structure

**sdk/kronk/** - Core API package:

| File             | Purpose                                |
| ---------------- | -------------------------------------- |
| `acquire.go`     | Model pool acquire/release             |
| `chat.go`        | Chat completion API                    |
| `concurrency.go` | Generic streaming utilities            |
| `embedding.go`   | Embedding API                          |
| `init.go`        | Initialization and configuration       |
| `kronk.go`       | Main Kronk type, model pool management |
| `rerank.go`      | Reranking API                          |
| `response.go`    | OpenAI Responses API streaming         |

**sdk/kronk/model/** - Low-level inference:

| File           | Purpose                                               |
| -------------- | ----------------------------------------------------- |
| `batch.go`     | Batch engine for parallel text inference              |
| `batch_finish.go` | Request completion, KV cleanup per model type      |
| `batch_schedule.go` | Slot assignment (first-available for all sessions)     |
| `batch_slot_start.go` | Slot initialization, KV restore from RAM, KV snapshot to RAM |
| `caching.go`   | Cache orchestration and routing                       |
| `caching_imc.go` | IMC session matching, hash scanning, and cache operations |
| `caching_imc_media.go` | IMC media cache build and extend (vision/audio) |
| `chat.go`      | Chat inference loop, batch routing                    |
| `config.go`    | Model configuration (GPU, cache, batching)            |
| `embed.go`     | Embedding inference                                   |
| `logprobs.go`  | Token log probability extraction                      |
| `media.go`     | Vision/audio media processing                         |
| `model.go`     | Model type, context management, lifecycle             |
| `models.go`    | OpenAI-compatible types (ChatMessage, ToolCall, etc.) |
| `params.go`    | Sampling parameters                                   |
| `processor.go` | Template-specific token processors                    |
| `prompts.go`   | Prompt formatting                                     |
| `rerank.go`    | Reranking inference                                   |

#### 17.7.2 Streaming Architecture

**Response Streaming Pattern** (`response.go`, `concurrency.go`):

- Uses `streamingWith[T, U]` generic function for 1:N event transformation
- `streamProcessor` has three phases: `Start()`, `Process(chunk)`, `Complete(lastChunk)`
- `streamState` struct maintains response ID, sequence numbers, aggregated usage
- SSE format: `event: <type>\ndata: <json>\n\n`

**FinishReason Handling:**

- `FinishReasonPtr *string` field with `FinishReason()` accessor
- Constants: `FinishReasonStop="stop"`, `FinishReasonTool="tool_calls"`, `FinishReasonError="error"`
- When `FinishReasonPtr != nil`, skip text/reasoning deltas (they duplicate previous content)
- Always process tool calls even with FinishReason set (may only arrive in final chunk)

#### 17.7.3 Concurrency Strategy

`NSeqMax` behaves differently depending on model type:

**Embedding and Reranking Models**:

- `NSeqMax` controls the internal context pool size
- Model weights are shared, only KV cache memory is multiplied
- Inputs within a request are partitioned across pool contexts for parallel processing
- Semaphore capacity = `NSeqMax`

**Text Inference Models** (chat, completion, vision, audio):

- `NSeqMax` controls batch parallelism within the batch engine
- Only one `model.Model` instance is created with multiple slots
- Semaphore capacity = `NSeqMax * queueDepth` (default queueDepth=2)

**Detection Logic** (`kronk.go`):

```go
switch {
case mi.IsEmbedModel || mi.IsRerankModel:
    semCapacity = max(cfg.NSeqMax, 1)
default:
    semCapacity = max(cfg.NSeqMax, 1) * o.queueDepth
}
```

#### 17.7.4 Model Acquire/Release & Cleanup

**Acquisition** (`acquire.go`):

1. **Backpressure slot**: Acquire semaphore slot (limits total in-flight requests)
2. **Return model**: Return the single model instance

**Cleanup Flow:**

1. `streaming()` acquires model, defers `releaseModel()` in wrapper goroutine
2. `ChatStreaming` defers `m.resetContext()` before any processing
3. When generation completes, `resetContext()` runs first:
   - `llama.Synchronize(m.lctx)` - waits for GPU operations
   - `llama.MemoryClear(mem, true)` - clears KV cache
4. Channel closes, wrapper exits, `releaseModel()` runs

**Key invariant:** `resetContext()` always runs before model release due to defer ordering.

#### 17.7.5 Batch Engine Internals

**ChatStreaming Decision Logic** (`chat.go`):

The `submitToBatchEngine()` function decides the processing path:

```go
// submitToBatchEngine returns false if batch not available.
if m.batch == nil || object != ObjectChatText {
    return false
}
// Submit job to batch engine...
return true
```

All chat requests (including vision/audio) are submitted to the batch engine:

```go
m.submitToBatchEngine(...)
batching = true
```

**Batch Engine Architecture** (`batch.go`):

- `batchEngine` manages `nSlots` parallel `slot` structs
- Each slot tracks: `seqID`, prompt tokens, decode state, sampler, response channel, logprobs, prefill state
- Signal-based wake pattern: `wakeCh chan struct{}` (buffered size 1) wakes immediately on new requests
- Polling intervals: 100µs (active slots generating), 5ms (idle, no active slots)

**Slots, Sequences, and Sessions:**

- `slot.id` = slot index (batch-engine execution lane)
- `slot.seqID` = llama.cpp sequence ID (KV cache partition for the active slot)
- `slot.seqIDs` = pre-allocated slice for efficient `batchAdd` calls
- `imcSession` = logical cached conversation branch (hash, tokens, KV state)

Sequences are isolated partitions in the shared KV cache memory. Slot seqIDs
always start at 0. IMC sessions are decoupled from slots: session state is
externalized to RAM after each request and restored into any available slot
on the next request via `StateSeqSetData`. `StateSeqGetData` captures raw KV
bytes regardless of whether they originated from text tokens or media
embeddings.

#### 17.7.6 Context Pooling

- `llama.Context` is created once in `NewModel` and reused across requests
- Call `resetContext()` between requests to clear KV cache
- Avoids Vulkan memory fragmentation from repeated context alloc/dealloc

#### 17.7.7 IMC Implementation Details

**Critical Implementation Details:**

1. **Extension tokenization must use `special=true`**: Use `llama.Tokenize(vocab, extension, false, true)` to ensure ChatML tokens like `<|im_start|>` are recognized.

2. **Prefix mismatch detection**: Use `strings.HasPrefix(fullPrompt, prefixPrompt)` to detect Jinja template nondeterminism.

3. **`add_generation_prompt=false` for cached prefixes**: Creates valid prefix for extension. Generation prompt added only for final suffix.

**IMC Algorithm:**

1. First request (cache empty): Cache `messages[0:len-1]`, generate from last message
2. Subsequent requests (prefix match): Extend cache with `messages[cachedCount:len-1]`
3. New thread (prefix mismatch): Rebuild cache from scratch

**IMC Lifecycle (All Sessions):**

1. `processIMC()` scans **sessions** (not slots) for a hash match
2. `fillSlots()` assigns the job to the **first available slot**
3. `startSlot()` restores cached KV from RAM via `StateSeqSetData`
4. Cache is extended/rebuilt as needed, then snapshotted back to RAM via `StateSeqGetData`
5. Suffix tokens are decoded and generation runs
6. `finishSlot()` clears the full VRAM sequence (cached prefix already lives in RAM)

**IMC Session State:**

```go
type imcSession struct {
    slotID            int           // Slot index (transitional)
    seqID             llama.SeqId   // KV cache sequence ID (transitional)
    cachedMsgsHash    string        // Hash of all cached messages
    cachedTokens      []llama.Token // Full token sequence in KV cache
    totalTokensCached int           // Total KV positions cached
    cachedMsgCount    int           // Number of messages cached
    kvState           []byte        // Externalized KV state (RAM buffer)
    kvStateBytes      int           // Size of kvState in bytes
    lastUsed          time.Time     // Last access time (for eviction)
    pending           bool          // True when build/extend in-flight
    hasMedia          bool          // True if cached content includes media
    useMRoPE          bool          // True if cached media used M-RoPE
    mediaKVCounts     []int         // KV positions per media chunk
    sysPromptHash     string        // Hash of system prompt message
    sysPromptTokens   int           // Token count of system prompt
}
```

#### 17.7.8 Tool Call Internals

**chatMessage Unmarshaling** (`models.go`):

- `Content` can be `nil` for assistant messages with tool_calls
- Handle `len(app.Content) == 0 || string(app.Content) == "null"` as valid empty content

**ToolCallArguments Type:**

- Custom type that marshals to JSON string (OpenAI spec)
- Unmarshals from either string or object for non-compliant clients

#### 17.7.9 Logprobs Implementation

**Implementation** (`logprobs.go`):

- `extractLogprobs()`: Retrieves logits via `llama.GetLogitsIth()`
- `logSoftmax()`: Numerically stable log-softmax using log-sum-exp trick
- `getTopKLogprobs()`: Uses min-heap for efficient O(n log k) top-k extraction

**Critical:** Logprobs must be extracted **before** `llama.SamplerAccept()` is called.

### 17.8 API Handler Notes

**Input Format Conversion** (`cmd/server/app/domain/`):

Both streaming and non-streaming Response APIs must call
`convertInputToMessages(d)` to handle the OpenAI Responses `input` field
format.

### 17.9 Goroutine Budget

A running Kronk server typically shows ~25 baseline goroutines before any
requests arrive. When requests are active, expect roughly 3-5 additional
goroutines per in-flight request. For example, 3 concurrent requests for the
same model will show ~40 goroutines total. This is normal.

**Baseline goroutines (~25, always running):**

| Source                                         | Goroutines | Location                                 |
| ---------------------------------------------- | ---------- | ---------------------------------------- |
| Go runtime (GC, finalizer, netpoller, etc.)    | ~4-6       | runtime internals                        |
| API `http.Server` (listener + idle conns)      | ~3         | `cmd/server/api/services/kronk/kronk.go` |
| Debug `http.Server` (pprof, metrics, statsviz) | ~3         | `cmd/server/api/services/kronk/kronk.go` |
| `statsviz.Register` (websocket handler)        | ~2         | `cmd/server/app/sdk/debug/debug.go`      |
| gRPC auth server (`gs.Serve`)                  | ~2-3       | `cmd/server/app/domain/authapp/start.go` |
| OTEL background collector probe                | 1          | `sdk/kronk/observ/otel/otel.go`          |
| `otelhttp.NewHandler` internals                | ~1-2       | `cmd/server/foundation/web/web.go`       |
| Batch engine `processLoop`                     | 1          | `sdk/kronk/model/batch.go`               |

**Per-request goroutines (~3-5 each):**

| Source                                                    | Location                   |
| --------------------------------------------------------- | -------------------------- |
| `http.Server` connection handler                          | Go stdlib                  |
| `ChatStreaming` request goroutine                         | `sdk/kronk/model/chat.go`  |
| `streaming()` wrapper goroutine                           | `sdk/kronk/concurrency.go` |
| `wrapChannelForLogging` (only if `InsecureLogging` is on) | `sdk/kronk/model/chat.go`  |

The goroutine metric is a point-in-time snapshot from `runtime.NumGoroutine()`
captured every 10th request by the metrics middleware. It includes everything
in the process, including Go runtime internals. After active requests complete,
the count drops back to the baseline.

### 17.10 Request Tracing Spans

Each chat completion request produces the following trace hierarchy:

```
POST /v1/chat/completions
├── prepare-request              Validation, caching, and prompt creation
│   ├── process-cache            Cache lookup/update (IMC, when enabled)
│   │   └── cache-tokenize-*     Tokenization for cache (imc-extend, imc-scratch)
│   └── create-prompt            Jinja template application
│
│        ← queue wait →          Job sits in requestQ channel until batch engine picks it up
│
└── process-request              Batch engine slot processing
    ├── prefill                  Tokenization + KV cache fill (ends at first output token)
    └── token-generation         Decode loop producing output tokens
```

**Phase 1: prepare-request** runs in the `ChatStreaming` goroutine. It
validates the document, processes the IMC cache, and creates the prompt
via the Jinja template. When caching is enabled, `process-cache` and its
child `cache-tokenize-*` spans appear here.

**Queue wait** is the gap between `prepare-request` ending and
`process-request` starting. The job has been submitted to the batch engine's
`requestQ` channel and is waiting for the `processLoop` goroutine to wake up
and assign it to a slot. The exact duration is recorded as a `queue-wait`
attribute on the `process-request` span.

**Phase 2: process-request** runs in the batch engine's `processLoop`
goroutine. The `prefill` span covers tokenization and KV cache filling. Time
to first token (TTFT) is measured from prefill start to the first output
token. The `token-generation` span covers the decode loop that produces
output tokens.

Additional spans that may appear at the top level:

| Span                   | When                      | Description                            |
| ---------------------- | ------------------------- | -------------------------------------- |
| `model-file-load-time` | First request for a model | Loading the GGUF model file            |
| `proj-file-load-time`  | Vision/audio requests     | Loading the multimodal projection file |

### 17.11 Inference Code Path

This section describes the high-level steps that occur when a chat inference
request is processed. For the corresponding function-level trace with file
locations, see [section 17.12](#1712-inference-code-path-detailed).

#### Step 1: Receive the Request

The caller provides a document containing messages and sampling parameters.
The system validates that the request includes a timeout deadline to prevent
unbounded processing.

#### Step 2: Acquire the Model

A semaphore controls how many requests can be in-flight at once. The request
blocks here until a slot in the semaphore opens up, providing backpressure
when the system is under load. The model instance is returned once a slot is
acquired.

#### Step 3: Validate the Document

The request document is validated to ensure it contains properly structured
messages. Sampling parameters (temperature, top_p, top_k, min_p, max_tokens,
grammar, etc.) are extracted and resolved against model defaults. The document
is shallow-cloned so downstream processing can modify it without affecting the
caller.

#### Step 4: Prepare the Context

The system determines whether this is a text-only or media (vision/audio)
request:

- **Text**: Multi-part content arrays are flattened into plain strings.
- **Media**: The projection model is loaded, media content (images or audio)
  is detected and converted into raw bytes for the encoder pipeline.

#### Step 5: Process the Cache

If caching is enabled, the system checks whether any portion of the
conversation is already in the KV cache to avoid redundant computation:

- **Incremental Message Cache (IMC)**: Hashes all messages except the last
  and scans slots for a matching conversation prefix. The best match
  determines the strategy: pure cache hit (nothing to decode), extend
  (decode only new messages), partial prefix trim (salvage a common prefix),
  or rebuild from scratch.

Tool response messages are also enriched with their originating function names
so templates can render tool results correctly.

#### Step 6: Apply the Chat Template

The remaining (non-cached) messages are run through the model's Jinja2 chat
template. This converts the structured message array into the exact prompt
string the model expects, including any special tokens, role markers, and
tool definitions. For media requests, raw media bytes are returned alongside
the text prompt.

#### Step 7: Submit to the Batch Engine

The fully prepared request — prompt string, media bytes, sampling parameters,
and cache state — is packaged into a job and placed on the batch engine's
request queue. A wake signal is sent so the batch engine picks it up
immediately rather than waiting for its next poll cycle.

#### Step 8: Assign to a Slot

The batch engine's processing loop wakes up and checks for pending work. It
dequeues the job and assigns it to the first available processing slot. All
IMC sessions (text and media) use first-available slot assignment. If all
slots are busy, the longest-running slot is preempted after a configurable
timeout.

#### Step 9: Initialize the Slot

The assigned slot is prepared for this request:

1. **Restore cached KV state**: For IMC, the session's externalized KV state
   is restored from RAM into the slot's sequence via `StateSeqSetData`.
   Extension tokens are then decoded, or the sequence is cleared and rebuilt.
2. **Build the sampler**: A sampler chain is constructed from the request's
   sampling parameters (temperature, top_k, top_p, min_p, repetition
   penalties, etc.). If grammar-constrained output is requested, a separate
   grammar sampler is also created.
3. **Snapshot cached prefix**: For IMC, after cache build/extend but before
   suffix tokens are decoded, the cached prefix KV state is snapshotted to
   RAM via `StateSeqGetData`. This captures the reusable prefix for the next
   request.
4. **Tokenize the prompt**: The prompt string is converted into a sequence of
   token IDs. Only the non-cached portion of the prompt needs tokenization.
5. **Context window check**: The total token count (cached + new) is verified
   against the model's context window limit.

#### Step 10: Prefill (KV Cache Fill)

The prompt tokens are fed through the model in chunks to build up the KV
cache — this is the "prefill" phase. Tokens are added to a batch buffer up
to the configured batch size limit, then a GPU forward pass (decode) is
executed. When multiple slots are active, tokens are allocated round-robin
across slots so no single request can starve others. This repeats until all
prompt tokens have been processed.

For media requests, image or audio embeddings are interleaved with text
tokens and decoded through the model's multimodal pipeline.

#### Step 11: Token Generation (Decode Loop)

Once prefill is complete, the model enters the decode loop — generating one
output token per iteration:

1. **Forward pass**: The most recently sampled token is added to the batch and
   decoded through the model. With multiple active slots, all their tokens
   are batched together in a single forward pass for efficiency.
2. **Sampling**: The model's output logits are processed through the sampler
   chain to select the next token. If grammar constraints are active, the
   sampler respects the grammar rules.
3. **Speculative decoding** (optional): A smaller draft model generates
   candidate tokens ahead of the main model. These drafts are verified in
   a single batch forward pass, accepting correct predictions and rejecting
   mismatches. This can significantly increase tokens per second.

#### Step 12: Process Each Token

Each sampled token goes through a processing pipeline:

1. **Logprobs extraction**: If requested, token log-probabilities are
   extracted from the model's logits before the sampler state is updated.
2. **End-of-generation check**: If the token is an EOG (end-of-generation)
   token, generation stops and the request moves to the finish phase.
3. **UTF-8 assembly**: Tokens are converted to text bytes. Since a single
   Unicode character can span multiple tokens, partial bytes are buffered
   until a complete codepoint is available.
4. **Content classification**: A state machine categorizes the output into
   reasoning (think tags), completion (regular response), or tool call
   content. This determines how the text is accumulated and streamed.
5. **Token counting**: Each generated token is counted as either a reasoning
   token or a completion token for usage reporting.
6. **Max tokens check**: If the output token count reaches the requested
   limit, generation stops.
7. **Stream to client**: For non-tool content, each complete text fragment
   is sent as an SSE delta event through the response channel.

#### Step 13: Finish the Request

When generation ends (EOG token, max tokens, or error), the request is
finalized:

1. **Flush remaining text**: Any buffered UTF-8 bytes are flushed into the
   final response accumulators.
2. **Parse tool calls**: If the model generated tool call content, it is
   parsed into structured function calls with validated JSON arguments.
3. **Calculate metrics**: Tokens per second (TPS), time to first token
   (TTFT), and draft acceptance rates are computed.
4. **Send final response**: The complete response — including content,
   reasoning, tool calls, logprobs, and usage statistics — is sent through
   the response channel.
5. **Clean up the KV cache**:
   - IMC (all model types): the entire VRAM sequence is cleared. The cached
     conversation prefix was already snapshotted to RAM during slot
     initialization and will be restored on the next request.
   - Without caching, the entire sequence is cleared.
6. **Free resources**: The sampler, grammar sampler, and any multimodal
   resources (bitmaps, projection context) are freed.

#### Step 14: Release the Model

The response channel is closed, signaling to the caller that streaming is
complete. The semaphore slot is released, allowing the next queued request
to begin processing.

### 17.12 Inference Code Path (Detailed)

This section traces the function-level code path for a `ChatStreaming` request.
Each step corresponds to the high-level description in
[section 17.11](#1711-inference-code-path).

**1. `Kronk.ChatStreaming`** (`sdk/kronk/chat.go`)

- Validates context has a deadline.
- Wraps `Model.ChatStreaming` in a closure.

**2. `streaming()`** (`sdk/kronk/concurrency.go`)

- Calls `acquireModel()` — checks shutdown flag, increments `activeStreams`,
  acquires semaphore slot for backpressure.
- Spawns goroutine that calls `Model.ChatStreaming`, relays chunks to caller's
  channel.
- Defers `releaseModel()` (releases semaphore) and `close(ch)`.

**3. `Model.ChatStreaming`** (`sdk/kronk/model/chat.go`)

- Creates response channel, wraps with logging if `InsecureLogging` enabled.
- Increments `activeStreams` atomically.
- Spawns goroutine with `prepare-request` span.

**4. `validateAndCloneDocument()`** (`model/chat.go`)

- Validates `messages` field exists and is `[]D`.
- Calls `parseParams()` — extracts temperature, top_p, top_k, min_p,
  max_tokens, grammar, etc.
- Shallow-clones the document.

**5. `prepareContext()`** (`model/chat.go`)

- **Text path**: `prepareTextContext()` — flattens multi-part content arrays to
  plain strings.
- **Media path**: `prepareMediaContext()` — detects vision/audio, loads
  projection file via `mtmd.InitFromFile()`, converts OpenAI format to media
  bytes.
- Returns object type: `ObjectChatText` or `ObjectChatMedia`.

**6. `prepareCacheAndPrompt()`** (`model/chat.go`)

- **6a. `injectToolResponseNames()`** — adds `name`/`tool_call_name` to
  `role:"tool"` messages by matching `tool_call_id`.
- **6b. `processCache()`** (`model/caching.go`):
  - **IMC**: `processIMC()` — two-tier hash scan across sessions, finds best
    match (pure hit, extend, partial prefix trim, or rebuild from scratch),
    tokenizes extension tokens, sets `pending` flag on the selected session.
- **6c. `createPrompt()`** → `applyRequestJinjaTemplate()` — applies Jinja2
  chat template to remaining messages, returns prompt string + media bytes.

**7. `submitToBatchEngine()`** (`model/chat.go`)

- Builds `chatJob` struct with all request data, cache state, and IMC fields.
- Calls `batch.submit()` — sends job to `requestQ` channel, sends wake signal
  on `wakeCh`.
- Starts `queue-wait` span.

**8. `processLoop()` wakes** (`model/batch_engine.go`)

- Signal-based: wakes immediately on `wakeCh`, polls at 100µs when active,
  5ms when idle.
- Calls `processBatch()`.

**9. `processBatch()`** (`model/batch_engine.go`)

- Clears batch buffer.
- Executes any pending slot preemption.
- Prefills draft model for speculative decoding slots.
- Adds generation tokens for active slots (1 token per slot).
- Continues text prefill via round-robin `addPrefillChunk()` across slots.
- Continues media prefill via `addPrefillMediaChunk()`.
- **`fillSlots()`** (`model/batch_schedule.go`) — dequeues job, assigns to
  first-available slot (all IMC sessions use first-available routing).

**10. `startSlot()`** (`model/batch_slot_start.go`)

- Resets slot, ends `queue-wait` span, starts `process-request` and `prefill`
  spans.
- **Creates sampler**: `toSampler()` — builds llama.cpp sampler chain
  (temperature, top_k, top_p, min_p, repetition penalties, DRY, XTC, mirostat).
- **Creates grammar sampler** if grammar specified.
- **IMC KV restore from RAM**: restores externalized KV state from
  `session.kvState` into the slot's sequence via `StateSeqSetData`. Then
  decodes extension tokens via `decodeTokensIntoCache()`, or clears sequence
  for rebuild, or trims for partial prefix.
- **IMC KV snapshot to RAM**: after cache build/extend but before suffix
  decode, snapshots the cached prefix via `StateSeqGetData` into
  `session.kvState`.
- **Tokenize prompt**: `llama.Tokenize(vocab, prompt, addBOS, special=true)` —
  converts remaining prompt text to tokens.
- Context window check.
- Assembles draft prompt tokens for speculative decoding.
- **`addPrefillChunk()`** — adds first chunk of tokens to batch.

**11. Prefill phase** (`model/batch_prefill_text.go`)

- `addPrefillChunk()` adds tokens to batch in chunks up to `NBatch` limit.
- Each token: `batch.Add(token, position, seqIDs, isLast)`.
- Round-robin across slots via `NUBatch` chunk limit.
- **`llama.Decode(lctx, batch)`** — GPU forward pass, fills KV cache.
- **`llama.Synchronize(lctx)`** — waits for GPU completion.
- Repeats until all prefill tokens consumed.

**12. Token generation loop** (back in `processBatch`)

- For each active slot with `prefillDone=true`:
  - `batch.Add(sampled, nPast, seqIDs, true)` — add last sampled token.
  - `llama.Decode()` — forward pass.
  - **Speculative path**: `generateDraftTokens()` → add draft+sampled to batch
    → `verifySpeculativeTokens()`.

**13. `processSlotToken()`** (`model/batch_tokens.go`)

- **Sample**: `llama.SamplerSample(sampler, lctx, iBatch)` or grammar-aware
  `SampleWithGrammar()`.

**14. `handleSampledToken()`** (`model/batch_tokens.go`)

- **Extract logprobs**: `extractLogprobs()` via `llama.GetLogitsIth()` +
  log-softmax + top-k heap.
- **Accept token**: `llama.SamplerAccept()` (and grammar accept).
- **EOG check**: `llama.VocabIsEOG()` → if true, `finishSlot()`.
- **UTF-8 buffering**: `llama.TokenToPiece()` → buffer partial multi-byte
  codepoints → `extractCompleteUTF8()`.
- **First token**: records `prefillDone=true`, calculates TTFT, ends prefill
  span, starts `token-generation` span.
- **Processor state machine**: `stepGPT()` or `stepStandard()` — classifies
  content as reasoning/completion/tooling, detects think tags, tool call
  markers.
- **Token counting**: increments `reasonTokens` or `completionTokens`.
- **Max tokens check**: if `outputTokens >= maxTokens`, `finishSlot()`.
- **Accumulate**: appends to `finalContent`, `finalReasoning`, or
  `finalTooling` builders.
- **Stream**: `sendDeltaResponse()` — sends SSE chunk via response channel
  (skipped for tool content).

**15. `finishSlot()`** (`model/batch_finish.go`)

- **Flush UTF-8 buffer** — emit any remaining complete codepoints.
- **Parse tool calls**: `parseGPTToolCall()` or `parseToolCall()` — extracts
  function name, arguments, validates JSON.
- **Calculate metrics**: TPS = `(outputTokens-1) / elapsed`, TTFT, draft
  acceptance rate.
- **Send final response**: `sendFinalResponse()` with usage, content,
  reasoning, tool calls, logprobs.
- **KV cache cleanup**:
  - IMC (all model types): `MemorySeqRm(mem, seqID, -1, -1)` — full clear.
    Cached prefix already snapshotted to RAM in `startSlot`.
  - Non-IMC: `MemorySeqRm(mem, seqID, -1, -1)` — full clear.
- **Free resources**: free sampler, grammar sampler, MTMD bitmaps/chunks,
  mtmdCtx.
- **Close job channel** → `streaming()` goroutine drains → closes caller
  channel.
- **Decrement `activeStreams`**.

**16. `releaseModel()`** (`sdk/kronk/acquire.go`)

- Releases semaphore slot (`<-krn.sem`).
- Decrements `activeStreams`.
