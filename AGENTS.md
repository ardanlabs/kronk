# AGENTS.md

Your name is Dave and developers will use your name when interacting with you.

## Build & Test Commands

- Install CLI: `go install ./cmd/kronk`
- Run all tests: `make test` (requires `make install-libraries install-models` first)
- Single test: `go test -v -count=1 -run TestName ./sdk/kronk/...`
- Build server: `make kronk-server`
- Build BUI frontend: `make bui-build`
- Generate docs: `make kronk-docs`
- Tidy modules: `go mod tidy`
- Lint: `staticcheck ./...`

## Developer Setup

- Run `make setup` once to configure git hooks (enables pre-commit hook for all developers)
- Pre-commit hook runs `make kronk-docs` and `make bui-build` automatically

## Architecture

- **cmd/kronk/** - CLI tool for managing models, server, security (subcommands: catalog, libs, model, security, server)
- **cmd/server/** - OpenAI-compatible model server (gRPC + HTTP) with BUI frontend
- **cmd/server/api/tooling/docs/** - Documentation generator for BUI (SDK and CLI docs)
- **sdk/kronk/** - Core API: model loading, chat, embeddings, cache, metrics
- **sdk/observ/** - Observability utilities
- **sdk/security/** - JWT auth, OPA authorization, key management
- **sdk/tools/** - Library/model download utilities
- Uses **yzma** (llama.cpp Go bindings) for local inference with GGUF models

## BUI Frontend (React)

Location: `cmd/server/api/frontends/bui/src/`

**Directory Structure:**

- `components/` - React components (pages and UI elements)
- `contexts/` - React context providers for shared state
- `services/` - API client (`api.ts`)
- `types/` - TypeScript type definitions
- `App.tsx` - Main app with routing configuration
- `index.css` - Global styles (CSS variables, component styles)

**Routing**: Uses `react-router-dom` with `BrowserRouter`. Routes defined in `routeMap` in `App.tsx`.

**Adding new pages:**

1. Create component in `components/` (e.g., `DocsSDKKronk.tsx`)
2. Add page type to `Page` union in `App.tsx`
3. Add route path to `routeMap` in `App.tsx`
4. Add `<Route>` element in `App.tsx`
5. Add `<Link>` entry to menu in `components/Layout.tsx`

**Menu structure** (`Layout.tsx`): Uses `MenuCategory[]` with `id`, `label`, `items` (for leaf pages), or `subcategories` (for nested menus).

**State Management:**

- `TokenContext` - Stores API token in localStorage (key: `kronk_token`), persists across sessions
- `ModelListContext` - Caches model list data with invalidation support
- Access via hooks: `useToken()`, `useModelList()`

**API Service** (`services/api.ts`):

- `ApiService` class with methods for all endpoints
- Streaming support for pull operations (models, catalog, libs)
- Auth-required endpoints accept token parameter

**Styling:**

- CSS variables defined in `:root` (colors: `--color-orange`, `--color-blue`, etc.)
- Common classes: `.card`, `.btn`, `.btn-primary`, `.form-group`, `.alert`, `.table-container`
- No CSS modules or styled-components; use global CSS classes

**Documentation Generation:**

- SDK docs: Auto-generated via `cmd/server/api/tooling/docs/sdk/` using `go doc` output
- CLI docs: Auto-generated via `cmd/server/api/tooling/docs/cli/` from command definitions
- Examples: Auto-generated from `examples/` directory
- Run: `go run ./cmd/server/api/tooling/docs -pkg=all`

## CLI Commands

All commands support web mode (default) and `--local` mode.

**Environment Variables (web mode):**

- `KRONK_TOKEN` - Authentication token (required when auth enabled)
- `KRONK_WEB_API_HOST` - Server address (default: localhost:8080)

## Code Style

- Package comments: `// Package <name> provides...`
- Errors: use `fmt.Errorf("context: %w", err)` with lowercase prefix
- Declare package-level sentinel errors as `var ErrXxx = errors.New(...)`
- Structs: unexported fields, exported types; use `Config` pattern for constructors
- No CGO in tests: `CGO_ENABLED=0 go test ...`
- Imports: stdlib first, then external, then internal (goimports order)
- Avoid `else` and `else if` clauses; prefer `switch` statements or early returns

## Streaming Architecture (sdk/kronk/)

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

**FinishReason handling** (`response.go`):

- When `FinishReason != ""`, skip text/reasoning deltas (they duplicate previous content)
- Always process tool calls even with FinishReason set (may only arrive in final chunk)

**Message vs Delta in final chunks** (`chat.go`, `model/models.go`):

- Final chunk uses `Message` for full content (non-streaming compatibility)
- Tool calls are populated in **both** `Message` and `Delta` for streaming compatibility
- HTTP streaming clears `Choice[0].Message` on final chunk (`FinishReasonStop`) per OpenAI spec
- Test helpers: use `Delta` only if streaming AND `FinishReason` is empty; otherwise use `Message`

## Model & Inference (sdk/kronk/model/)

**Context Pooling** (`model.go`):

- `llama.Context` is created once in `NewModel` and reused across requests
- Call `resetContext()` (uses `llama.MemoryClear`) between requests to clear KV cache
- Avoids Vulkan memory fragmentation from repeated context alloc/dealloc

**KV Cache Type Configuration** (`config.go`):

- `CacheTypeK` and `CacheTypeV` fields on `Config` control cache precision
- Uses `GGMLType` constants: `GGMLTypeF16=1`, `GGMLTypeQ8_0=8`, `GGMLTypeBF16=30`, etc.
- `GGMLTypeAuto=-1` uses llama.cpp defaults

**Resource Lifecycle**:

- Sampler chain freed via `defer llama.SamplerFree(sampler)` in `processChatRequest`
- Media path: `mtmd.InputChunksInit()` must be freed with `mtmd.InputChunksFree(output)`

## GPT-OSS Processor (sdk/kronk/processor.go)

**Token handling for gpt-oss template**:

- `<|return|>` and `<|call|>` return `io.EOF` (end of generation)
- `<|end|>` is a section terminator (continues to next section)
- `<|channel|>commentary` triggers tool call mode (`statusTooling`)
- State machine: `awaitingChannel` → `collectingName` → content collection

**Repetition penalty**: Applied via `llama.SamplerInitPenalties` with defaults `RepeatPenalty=1.1`, `RepeatLastN=64`

## API Handler Notes (cmd/server/app/domain/)

**Input format conversion**: Both streaming and non-streaming Response APIs must call `convertInputToMessages(d)` to handle OpenAI Responses `input` field format

## Tool Call Handling (sdk/kronk/model/)

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

## Model Acquire/Release & Cleanup (sdk/kronk/)

**Model Pool Pattern:**

- Models are pooled via a channel (`krn.models`) for single-writer access
- `acquireModel()` blocks until a model is available, increments `activeStreams`
- `releaseModel()` returns the model to the pool, decrements `activeStreams`

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

## Model Configuration (sdk/kronk/model/config.go)

**Config Fields Reference:**

- `NSeqMax`: For text models, max parallel sequences for batched inference. For sequential models (embed/rerank/vision/audio), creates that many model instances in a pool. (0 = default of 1)
- `OffloadKQV`: KV cache on GPU (nil/true) or CPU (false)
- `OpOffload`: Tensor ops on GPU (nil/true) or CPU (false)
- `NGpuLayers`: Layers to offload (0 = all, -1 = none, N = specific count)
- `SplitMode`: Multi-GPU split (`SplitModeNone=0`, `SplitModeLayer=1`, `SplitModeRow=2` for MoE)

**Model-Specific Tuning Guidelines:**

- Vision/Audio models: keep `NUBatch` high (≥2048) for image/audio token processing
- MoE models: use `SplitModeRow` for multi-GPU, be cautious with aggressive cache quantization
- Embedding models: `NBatch` can equal `ContextWindow`, align `NUBatch` with sliding window
