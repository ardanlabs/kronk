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
  - [16.7.4 Model Acquire/Release & Cleanup](#1674-model-acquirerelease-cleanup)
  - [16.7.5 Batch Engine Internals](#1675-batch-engine-internals)
  - [16.7.6 Context Pooling](#1676-context-pooling)
  - [16.7.7 IMC Implementation Details](#1677-imc-implementation-details)
  - [16.7.8 Tool Call Internals](#1678-tool-call-internals)
  - [16.7.9 Logprobs Implementation](#1679-logprobs-implementation)
- [17.8 API Handler Notes](#178-api-handler-notes)
- [17.9 Goroutine Budget](#179-goroutine-budget)
- [17.10 Request Tracing Spans](#1710-request-tracing-spans)
- [17.11 Reference Threads](#1711-reference-threads)

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
CGO_ENABLED=0 go test ./...
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
| `caching.go`   | System prompt and IMC cache management                |
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

#### 16.7.4 Model Acquire/Release & Cleanup

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

#### 16.7.5 Batch Engine Internals

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

**Slots vs Sequences:**

- `slot.id` = slot index (for logging)
- `slot.seqID` = llama.cpp sequence ID (determines KV cache partition)
- `slot.seqIDs` = pre-allocated slice for efficient `batchAdd` calls

Sequences are isolated partitions in the shared KV cache memory. Slot seqIDs
always start at 0 — no sequences are reserved for caching. SPC decodes
saved tokens directly into slot sequences. IMC binds each slot's sequence
to a conversation.

#### 16.7.6 Context Pooling

- `llama.Context` is created once in `NewModel` and reused across requests
- Call `resetContext()` between requests to clear KV cache
- Avoids Vulkan memory fragmentation from repeated context alloc/dealloc

#### 16.7.7 IMC Implementation Details

**Critical Implementation Details:**

1. **Extension tokenization must use `special=true`**: Use `llama.Tokenize(vocab, extension, false, true)` to ensure ChatML tokens like `<|im_start|>` are recognized.

2. **Prefix mismatch detection**: Use `strings.HasPrefix(fullPrompt, prefixPrompt)` to detect Jinja template nondeterminism.

3. **`add_generation_prompt=false` for cached prefixes**: Creates valid prefix for extension. Generation prompt added only for final suffix.

**IMC Algorithm:**

1. First request (cache empty): Cache `messages[0:len-1]`, generate from last message
2. Subsequent requests (prefix match): Extend cache with `messages[cachedCount:len-1]`
3. New thread (prefix mismatch): Rebuild cache from scratch

**IMC Session State:**

```go
type imcSession struct {
    hash      string      // Hash of all cached messages
    tokens    int         // Total tokens in cache
    msgCount  int         // Number of messages cached
    promptLen int         // Length of templated prefix
    seqID     llama.SeqId // Assigned cache sequence ID
    lastUsed  time.Time   // For future eviction
}
```

#### 16.7.8 Tool Call Internals

**chatMessage Unmarshaling** (`models.go`):

- `Content` can be `nil` for assistant messages with tool_calls
- Handle `len(app.Content) == 0 || string(app.Content) == "null"` as valid empty content

**ToolCallArguments Type:**

- Custom type that marshals to JSON string (OpenAI spec)
- Unmarshals from either string or object for non-compliant clients

#### 16.7.9 Logprobs Implementation

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
│   ├── process-cache            Cache lookup/update (SPC or IMC, when enabled)
│   │   └── cache-tokenize-*     Tokenization for cache (spc, imc-extend, imc-scratch)
│   └── create-prompt            Jinja template application
│
│        ← queue wait →          Job sits in requestQ channel until batch engine picks it up
│
└── process-request              Batch engine slot processing
    ├── prefill                  Tokenization + KV cache fill (ends at first output token)
    └── token-generation         Decode loop producing output tokens
```

**Phase 1: prepare-request** runs in the `ChatStreaming` goroutine. It
validates the document, processes caches (SPC/IMC), and creates the prompt
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

### 17.11 Reference Threads

See `THREADS.md` for important past conversations and decisions worth
preserving.
