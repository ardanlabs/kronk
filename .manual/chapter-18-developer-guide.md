# Chapter 18: Developer Guide

## Table of Contents

- [18.1 Quick Reference](#181-quick-reference)
- [18.2 Build & Test Commands](#182-build-test-commands)
- [18.3 Developer Setup](#183-developer-setup)
- [18.4 Project Architecture](#184-project-architecture)
- [18.5 BUI Frontend Development](#185-bui-frontend-development)
- [18.6 Code Style Guidelines](#186-code-style-guidelines)
- [18.7 SDK Internals](#187-sdk-internals)
  - [17.7.1 Package Structure](#1871-package-structure)
  - [17.7.2 Streaming Architecture](#1872-streaming-architecture)
  - [17.7.3 Concurrency Strategy](#1873-concurrency-strategy)
  - [17.7.4 Model Acquire/Release & Cleanup](#1874-model-acquirerelease-cleanup)
  - [17.7.5 Batch Engine Internals](#1875-batch-engine-internals)
  - [17.7.6 Context Pooling](#1876-context-pooling)
  - [17.7.7 IMC Implementation Details](#1877-imc-implementation-details)
  - [17.7.8 Tool Call Internals](#1878-tool-call-internals)
  - [17.7.9 Logprobs Implementation](#1879-logprobs-implementation)
- [18.8 Responses API Normalization](#188-responses-api-normalization)
- [18.9 Goroutine Budget](#189-goroutine-budget)
- [18.10 Request Tracing Spans](#1810-request-tracing-spans)
- [18.11 Inference Code Path](#1811-inference-code-path)
- [18.12 MTP Internals](#1812-mtp-internals)
  - [18.12.1 Auto-detection](#18121-auto-detection)
  - [18.12.2 Pre-Norm Hidden-State Plumbing](#18122-pre-norm-hidden-state-plumbing)
  - [18.12.3 Mirror Step and AR Draft Loop](#18123-mirror-step-and-ar-draft-loop)
  - [18.12.4 Verification on the MTP Path](#18124-verification-on-the-mtp-path)
  - [18.12.5 Three-pass `processBatch` and Phase A / Phase B Split](#18125-three-pass-processbatch-and-phase-a--phase-b-split)
  - [18.12.6 Hybrid Target Rollback](#18126-hybrid-target-rollback)
  - [18.12.7 Per-Slot MTP State](#18127-per-slot-mtp-state)
  - [18.12.8 Code Map](#18128-code-map)
  - [18.12.9 Testing](#18129-testing)

---

This chapter covers development workflows, build commands, and code
conventions for contributors to the Kronk project.

### 18.1 Quick Reference

Here is a quick chart of some of the more important make commands.

| Task             | Command                                                      |
| ---------------- | ------------------------------------------------------------ |
| Install CLI      | `make install-kronk`                                         |
| Run all tests    | `make test` (requires env vars below)                        |
| Test prereqs     | `export RUN_IN_PARALLEL=yes; export GITHUB_WORKSPACE=$(pwd)` |
| Single test      | `go test -v -count=1 -run TestName ./sdk/kronk/...`          |
| Run server       | `make kronk-server`                                          |
| Run server (bg)  | `make kronk-server-detach`                                   |
| Tail server log  | `make kronk-server-logs`                                     |
| Stop server      | `make kronk-server-stop`                                     |
| Build BUI        | `make bui-build`                                             |
| Generate docs    | `make kronk-docs`                                            |
| Tidy modules     | `make tidy`                                                  |
| Update deps      | `make deps-upgrade`                                          |
| Post-edit checks | `gofmt -s -w <files> && go vet ./... && staticcheck ./...`   |
| Developer setup  | `make setup` (configures git hooks)                          |

### 18.2 Build & Test Commands

**Install CLI locally:**

```shell
make install-kronk
```

This is the canonical install path used everywhere in the docs. Internally it
runs `go install ./cmd/kronk` with the project's build tags.

**Run all tests:**

```shell
# Set required environment variables (project root must be absolute)
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE=$(pwd)

# Run from project root directory
make test
```

`make test` expands to `test-only + lint + vuln-check + diff`. The `test-only`
target depends on `install-libraries` and `install-test-models`, so the
llama.cpp libraries and test GGUF models are downloaded automatically the
first time you run it.

For fast iteration (skip `lint`, `vuln-check`, and `diff`):

```shell
make test-only
```

**Run a single test:**

```shell
go test -v -count=1 -run TestName ./sdk/kronk/...
```

The path can target any package (e.g. `./cmd/...` or a specific subpackage);
`./sdk/kronk/...` is just where most inference tests live.

### 18.3 Developer Setup

Configure git hooks for automatic pre-commit checks:

```shell
make setup
```

This enables a pre-commit hook that automatically runs:

- `make kronk-docs` - Regenerates documentation
- `make bui-build` - Rebuilds the BUI frontend
- `gomod2nix --dir . --outdir zarf/nix` - Regenerates the Nix lock file
  (only if `gomod2nix` is on `PATH`)
- `git add -A` - Stages all generated changes so they land in the same commit

**Toolchain dependencies:**

`make setup` only configures git hooks. The lint/vuln/codegen toolchain is
installed separately:

```shell
make install-gotooling   # staticcheck, govulncheck, protoc-gen-go(-grpc), gomod2nix
make install-tooling     # brew: protobuf, grpcurl, node (only needed for codegen / BUI work)
```

A fresh checkout that skips `install-gotooling` will fail the `lint` and
`vuln-check` steps of `make test` and the post-edit checks listed in §18.1.

### 18.4 Project Architecture

**Directory Structure:**

| Directory                      | Purpose                                                                                                            |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| `cmd/kronk/`                   | CLI tool (subcommands: `catalog`, `client`, `devices`, `libs`, `model`, `run`, `security`, `server`)               |
| `cmd/server/`                  | OpenAI-compatible HTTP model server with embedded BUI. Internal gRPC over `bufconn` for the embedded auth service. |
| `cmd/server/api/`              | Transport layer: HTTP services, BUI frontend embed, tooling (docs generator, logfmt)                               |
| `cmd/server/app/`              | Application layer: `domain/` (auth, mcp, model, etc. handlers) + `sdk/` (cache, mux, security wiring)              |
| `cmd/server/foundation/`       | Cross-cutting infra: `logger/`, `web/` (request lifecycle, tracing helpers)                                        |
| `cmd/server/api/tooling/docs/` | Documentation generator (SDK godoc + CLI command tree → BUI markdown)                                              |
| `sdk/kronk/`                   | Public SDK API surface (`init.go`, `chat.go`, `embedding.go`, `rerank.go`, `tokenize.go`, concurrency)             |
| `sdk/kronk/model/`             | Core inference and caching engine (batch, slots, IMC, sampler, prefill/decode)                                     |
| `sdk/kronk/observ/`            | Observability packages: `metrics/`, `otel/`                                                                        |
| `sdk/pool/`                    | Multi-model pool: keeps a capped set of `Kronk` APIs warm with TTL-based eviction; used by the model server        |
| `sdk/tools/`                   | CLI tooling support: `defaults/`, `devices/`, `downloader/`, `github/`, `libs/`, `models/`                         |
| `examples/`                    | Standalone module (own `go.mod`) with runnable SDK examples — source for auto-generated example docs               |
| `zarf/`                        | Deployment assets: `docker/`, `kms/` (model config samples), `nix/` (gomod2nix lock)                               |

**Core Technology:**

Kronk uses [yzma](https://github.com/hybridgroup/yzma) (llama.cpp Go bindings)
for local inference with GGUF models.

### 18.5 BUI Frontend Development

The Browser UI is a React application located at:

```
cmd/server/api/frontends/bui/src/
```

**Directory Structure (`src/`):**

| Directory/File | Purpose                                                 |
| -------------- | ------------------------------------------------------- |
| `components/`  | React components (pages and UI elements)                |
| `contexts/`    | React context providers for shared state                |
| `hooks/`       | Reusable hooks (e.g. `useDraftSession.ts`)              |
| `lib/`         | Shared TypeScript utilities (`context.ts`, `format.ts`) |
| `services/`    | API client (`api.ts`)                                   |
| `types/`       | TypeScript type definitions                             |
| `main.tsx`     | Vite entry point — mounts `<App>` into `index.html`     |
| `App.tsx`      | Main app with routing configuration                     |
| `index.css`    | Global styles (CSS variables, component styles)         |

The BUI project root (`cmd/server/api/frontends/bui/`) also contains
`index.html`, `package.json`, `vite.config.ts`, and `tsconfig.json`.

**Build & Embed:**

The BUI is a Vite + React + TypeScript app. The build output is **embedded
into the Go binary** at compile time via `//go:embed static` in
[`cmd/server/api/services/kronk/kronk.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/cmd/server/api/services/kronk/kronk.go#L37-L38).
Editing source under `bui/src/` has no runtime effect until the bundle is
rebuilt and the server is recompiled.

| Workflow            | Command                                              |
| ------------------- | ---------------------------------------------------- |
| HMR dev server      | `npm run dev` (from `cmd/server/api/frontends/bui/`) |
| Production build    | `make bui-build` (or `npm run build`)                |
| Build + embed + run | `make kronk-server-build`                            |

**Component Conventions:**

Tooltip and form-label conventions are governed by
[`bui/src/components/AGENTS.md`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/cmd/server/api/frontends/bui/src/components/AGENTS.md).
All parameter explanations live in `PARAM_TOOLTIPS` and are accessed through
the type-safe `TooltipKey`, `FieldLabel`, and `labelWithTip` helpers in
`ParamTooltips.tsx`. Any new form field must add a `PARAM_TOOLTIPS` entry.

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
- Streaming support for pull operations (models and libraries)
- Auth-required endpoints accept token parameter

The catalog is local and personal — there is no `catalog pull` stream.

**Styling Conventions:**

- CSS variables defined in `:root` (colors: `--color-orange`, `--color-blue`, etc.)
- Common classes: `.card`, `.btn`, `.btn-primary`, `.form-group`, `.alert`, `.table-container`
- No CSS modules or styled-components; use global CSS classes

**Documentation Generation:**

The single binary at `cmd/server/api/tooling/docs/main.go` runs three
pipelines in order: SDK godoc → examples → manual chapters.

| Pipeline        | Generator Location                          | Source                        |
| --------------- | ------------------------------------------- | ----------------------------- |
| SDK godoc       | `cmd/server/api/tooling/docs/sdk/gofmt/`    | `go doc` output for `sdk/...` |
| Examples        | `cmd/server/api/tooling/docs/sdk/examples/` | `examples/` module            |
| Manual chapters | `cmd/server/api/tooling/docs/manual/`       | `.manual/chapter-*.md` files  |

The manual chapters pipeline is what turns this very file into BUI React
components — edits here flow into the BUI on the next `make kronk-docs`.

Generate all documentation:

```shell
make kronk-docs
# equivalent to: go run cmd/server/api/tooling/docs/*.go
```

The generator takes no flags; it always rebuilds all three pipelines.

### 18.6 Code Style Guidelines

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

- Consumer-facing structs (`Config`, `Result`, request/response DTOs) expose
  exported fields the caller fills in.
- Internal state structs keep fields unexported and surface behavior through
  methods on the exported type.
- Use the `Config` pattern for constructors.

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

Tests link against yzma (llama.cpp Go bindings), so they the
installed libraries plus test models. Always go through `make test` (or
`make test-only` for fast iteration) per §18.2.

**Post-edit Checks (per [AGENTS.md](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/AGENTS.md)):**

After modifying any `.go` file, run on the changed files / package:

```shell
gofmt -s -w <changed files>
go vet ./...
staticcheck ./...
go fix <changed package>
```

These are non-negotiable; they are also what `make test`'s `lint` step runs.

**Comment Conventions:**

- Doc comments are full sentences ending with a period.
- Package doc lives on the file that declares the package's main type or
  entry point (e.g. `package kronk` doc on `kronk.go`).
- Use a block separator between logical sections in larger files:

```go
// =============================================================================
// Tiered Replace
// =============================================================================
```

See [fuzzyedit.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/cmd/server/app/domain/mcpapp/fuzzyedit.go#L58-L60) for the canonical pattern.

**Import Order:**

Imports are grouped into three blocks separated by blank lines and sorted by
`goimports -local github.com/ardanlabs/kronk`:

1. Standard library
2. External packages
3. Internal packages (`github.com/ardanlabs/kronk/...`)

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

### 18.7 SDK Internals

This section documents implementation details for developers working on
the Kronk SDK packages.

#### 18.7.1 Package Structure

**sdk/kronk/** - Public SDK API surface:

| File             | Purpose                                                   |
| ---------------- | --------------------------------------------------------- |
| `acquire.go`     | Model pool acquire/release (top-level wrapper)            |
| `chat.go`        | Chat completion API                                       |
| `concurrency.go` | Generic streaming utilities, semaphore-based backpressure |
| `embedding.go`   | Embedding API                                             |
| `init.go`        | Initialization and configuration                          |
| `kronk.go`       | Main `Kronk` type, model pool management                  |
| `logger.go`      | Streaming response logger (used when insecure-logging on) |
| `rerank.go`      | Reranking API                                             |
| `response.go`    | OpenAI Responses API streaming                            |
| `tokenize.go`    | Token-count API                                           |

**sdk/kronk/model/** - Low-level inference engine.

The package is large (30+ files); files are grouped here by concern.

_Core types and lifecycle:_

| File          | Purpose                                                       |
| ------------- | ------------------------------------------------------------- |
| `model.go`    | `Model` type, llama context management, lifecycle             |
| `config.go`   | Model configuration (GPU, cache, batching, YaRN, etc.)        |
| `params.go`   | Sampling parameters                                           |
| `models.go`   | OpenAI-compatible types (`ChatMessage`, `ToolCall`, etc.)     |
| `chat.go`     | Chat inference entry point, request validation, batch routing |
| `embed.go`    | Embedding inference                                           |
| `rerank.go`   | Reranking inference                                           |
| `prompts.go`  | Jinja2 chat template application                              |
| `tokenize.go` | Token-count helper                                            |
| `media.go`    | Vision/audio media detection and conversion                   |
| `check.go`    | Model file SHA validation                                     |
| `logging.go`  | Streaming response logger (mirrors final response for log)    |
| `yzma.go`     | Workarounds for yzma FFI issues not yet fixed upstream        |

_Batch engine_ (`batch_*.go`):

| File                     | Purpose                                                            |
| ------------------------ | ------------------------------------------------------------------ |
| `batch_engine.go`        | `batchEngine` — parallel inference slots, request queue, wake loop |
| `batch_schedule.go`      | Slot assignment (first-available for all sessions)                 |
| `batch_slot.go`          | `chatJob` and slot state structures                                |
| `batch_slot_start.go`    | Slot init: KV restore from RAM, sampler build, prefix snapshot     |
| `batch_decode.go`        | MTMD batch decode helpers                                          |
| `batch_prefill_text.go`  | Round-robin token prefill across slots                             |
| `batch_prefill_media.go` | Vision/audio chunk prefill (interleaves embeddings with text)      |
| `batch_tokens.go`        | Per-token sampling and pipeline (logprobs, EOG, classify, stream)  |
| `batch_utf8.go`          | Partial-codepoint buffering across token boundaries                |
| `batch_finish.go`        | Request completion, metrics, KV cleanup per model type             |
| `batch_errors.go`        | Slot cancellation / shutdown error helpers                         |
| `batch_shutdown.go`      | Drain active slots and pending jobs on shutdown                    |
| `batch_speculative.go`   | Speculative decoding (draft model prefill + verify)                |

_Caching:_

| File                   | Purpose                                                      |
| ---------------------- | ------------------------------------------------------------ |
| `caching.go`           | Cache orchestration and routing (`cacheResult`)              |
| `caching_imc.go`       | IMC session matching, two-tier hash scan, prefix trim/extend |
| `caching_imc_media.go` | IMC media cache build/extend (vision/audio)                  |

_Sampling and grammar:_

| File                    | Purpose                                                       |
| ----------------------- | ------------------------------------------------------------- |
| `grammar.go`            | JSON Schema → GBNF conversion for grammar-constrained output  |
| `logprobs.go`           | Top-k token log probability extraction                        |
| `speculative_sparse.go` | Sparse candidate sampling for speculative decode verification |

_Content processors_ (per-template tool/reasoning classification):

| File                   | Purpose                                               |
| ---------------------- | ----------------------------------------------------- |
| `processor.go`         | Processor interface, content classifier state machine |
| `processor_parse.go`   | Router: dispatches to model-specific tool parser      |
| `processor_gemma.go`   | Gemma4-style tool calls                               |
| `processor_glm.go`     | GLM `<arg_key>`/`<arg_value>` tag tool calls          |
| `processor_gpt.go`     | GPT-OSS Harmony tool calls                            |
| `processor_qwen.go`    | Qwen3-Coder XML-like tool tags                        |
| `processor_mistral.go` | Mistral / Devstral tool calls                         |
| `processor_json.go`    | Standard JSON tool calls                              |

_Misc:_

| File      | Purpose                                                                |
| --------- | ---------------------------------------------------------------------- |
| `pool.go` | `contextPool` — parallel llama contexts for embedding/rerank workloads |

**sdk/pool/** - Multi-model pool used by the model server.

Holds a capped LRU-style cache of live `*kronk.Kronk` instances keyed by
model ID, so multiple models can stay warm and be acquired on demand
without paying the load cost on every request. Cache size and idle TTL
are configurable.

| File       | Purpose                                                                                          |
| ---------- | ------------------------------------------------------------------------------------------------ |
| `pool.go`  | `Pool` type, `Config`, `New`, `AquireModel`, `AquireCustom`, `ModelStatus`, `Shutdown`, eviction |
| `model.go` | `ModelDetail` struct returned by `ModelStatus()`                                                 |

Key behaviors:

- **Singleflight load** — concurrent `AquireModel` calls for the same model
  ID coalesce into a single load.
- **Pre-emptive eviction** — when the pool is full, the coldest idle entry
  is unloaded _before_ the new model is loaded so two large models never
  sit in VRAM at the same time.
- **Active-stream protection** — automatic TTL eviction of an entry with
  in-flight streams is rejected; the entry is re-inserted to keep it
  resident until the stream finishes.
- **Shutdown** — `Shutdown(ctx)` invalidates the cache and blocks until
  every entry has finished unloading (or `ctx` expires).

#### 18.7.2 Streaming Architecture

**Two streaming primitives** (`concurrency.go`):

- `streaming[T]` — 1:1 relay. Used by `ChatStreaming` to forward
  `model.ChatResponse` chunks straight to the caller.
- `streamingWith[T, U]` — 1:N event transformation. Used by
  `ResponseStreaming` to fan out a single upstream chunk into multiple SSE
  event types.

Both acquire the model on entry and release it from a `defer` when the
user-facing channel closes (see §18.7.4) — the lifecycle is not hand-rolled
in the per-API files.

**Response Streaming Pattern** (`response.go`, `concurrency.go`):

- `streamProcessor` has three phases: `Start()`, `Process(chunk)`, `Complete(lastChunk)`
- Phase flow: `Start` runs once before the upstream channel opens,
  `Process` runs once per upstream chunk, `Complete` runs once after the
  upstream channel closes and receives the last chunk seen.
- `streamState` struct maintains response ID, sequence numbers, aggregated usage
- SSE format: `event: <type>\ndata: <json>\n\n`

**FinishReason Handling:**

- `FinishReasonPtr *string` field with `FinishReason()` accessor
- Only three constants exist (`models.go`):
  `FinishReasonStop="stop"`, `FinishReasonTool="tool_calls"`,
  `FinishReasonError="error"`. There is no `"length"` — a `max_tokens` cap
  is reported as `FinishReasonStop`, unlike the OpenAI API.
- When `FinishReasonPtr != nil`, skip text/reasoning deltas (they duplicate previous content)
- Always process tool calls even with FinishReason set (may only arrive in final chunk)

#### 18.7.3 Concurrency Strategy

All concurrent requests on a single `Kronk` block on one semaphore; its
capacity is fixed at `New()` time and depends on the model class.
`acquireModel()` is the gate (see §18.7.4).

`NSeqMax` is the `nseq-max` knob from `model_config.yaml`, and behaves
differently depending on model type:

**Embedding and Reranking Models**:

- `NSeqMax` controls the internal context pool size (see §18.7.6)
- Model weights are shared, only KV cache memory is multiplied
- Inputs within a request are partitioned across pool contexts for parallel processing
- Semaphore capacity = `NSeqMax`

**Text Inference Models** (chat, completion, vision, audio):

- `NSeqMax` controls batch parallelism within the batch engine — the number
  of concurrent slots (see §18.7.5)
- Only one `model.Model` instance is created with multiple slots
- Semaphore capacity = `NSeqMax * queueDepth` (default `queueDepth=2`)
- Why ×2: with `queueDepth=2`, one request can sit on the batch engine's
  request queue while another is in prefill/decode, smoothing throughput
  across acquire → prefill → decode → release. Increase to absorb bursty
  load; decrease to bound queued memory.

**Detection Logic** ([kronk.go:63-83](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/kronk.go#L63-L83)):

```go
queueDepth := cfg.QueueDepth()
if queueDepth == 0 {
    queueDepth = 2
}

var semCapacity int

switch {
case mi.IsEmbedModel || mi.IsRerankModel:
    semCapacity = max(cfg.NSeqMax(), 1)
default:
    semCapacity = max(cfg.NSeqMax(), 1) * queueDepth
}
```

#### 18.7.4 Model Acquire/Release & Cleanup

The wrappers in `concurrency.go` (`streaming` / `streamingWith`, see §18.7.2)
are what call `acquireModel` and `releaseModel` — per-API files do not call
them directly.

**Acquisition** ([acquire.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/acquire.go)):

1. **Shutdown check**: take `shutdown.Lock()`; if `shutdownFlag` is set,
   return `"acquire-model: kronk has been unloaded"`.
2. **Active stream accounting**: increment the `activeStreams` atomic counter
   while still holding the shutdown lock (the unload path waits on this
   counter to drain).
3. **Backpressure slot**: block on the semaphore (`krn.sem <- struct{}{}`),
   respecting `ctx.Done()` — on context cancellation, decrement
   `activeStreams` and return `ctx.Err()`.
4. **Return model**: return the single `*model.Model` (`krn.model`).

**Release** (`acquire.go`):

1. Drain a slot from the semaphore (`<-krn.sem`).
2. Decrement `activeStreams`.

**Cleanup Flow:**

The KV-cache cleanup path depends on whether the request goes through the
batch engine. The decision is captured by the local `batching` flag in
[chat.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/chat.go).

_Batched path_ (text inference via IMC — the normal route for chat):

- The `m.resetContext()` defer in `chat.go` is gated on `!batching` and is
  skipped.
- Per-slot KV cleanup happens inside the batch engine in
  [batch_finish.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go),
  which clears each slot's sequence and frees per-request resources.
- `releaseModel()` then runs from the wrapper `defer` in `concurrency.go`
  after the user-facing channel closes.

_Non-batched path_ (e.g. some embed/rerank entrypoints, non-IMC media flows):

- `chat.go` registers `defer m.resetContext()` after `validateAndCloneDocument`
  and `prepareContext` succeed (not "before any processing").
- `resetContext()` calls `llama.Synchronize(m.lctx)` then
  `llama.MemoryClear(mem, true)` for each model memory.
- `releaseModel()` runs after that, from the wrapper `defer`.

**Key invariant:** the semaphore guarantees the model is never released
while a request is in flight — `releaseModel()` is only called from the
streaming wrapper's `defer`, which fires after the user-facing channel
closes.

#### 18.7.5 Batch Engine Internals

**ChatStreaming Decision Logic** ([chat.go:258-315](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/chat.go#L258-L315)):

All chat requests — text and media (`ObjectChatText` and `ObjectChatMedia`) —
flow through `submitToBatchEngine`. It builds a `chatJob` (carrying request
data plus the resolved IMC fields from `cacheResult`) and unconditionally
calls `m.batch.submit(&job)`. Returns:

- `true` on successful submit; the caller sets `batching = true` in `chat.go`
  so the non-batched cleanup defer is skipped (see §18.7.4).
- `false` only on submit error. The error has already been streamed to the
  caller via `sendChatError` and any IMC pending reservation cleared.

There is no longer an `m.batch == nil || object != ObjectChatText` early
return — vision/audio also runs through the batch engine.

**Batch Engine Architecture** ([batch_engine.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_engine.go)):

- `batchEngine` manages `nSlots` parallel `*slot` structs (slot type lives
  in [batch_slot.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot.go))
- Constructed via `newBatchEngine(m, nSlots)` during model setup;
  `nSlots = NSeqMax` (see §18.7.3)
- Queueing: `requestQ chan *chatJob` (buffered `nSlots*2`) is the public
  inbox; `pendingJobs` holds jobs already dequeued but unable to start
  because no slot was free, and is checked before reading `requestQ` again
- Each slot tracks: `seqID`, prompt tokens, decode state, sampler, response
  channel, logprobs, prefill state
- Signal-based wake: `wakeCh chan struct{}` (buffered size 1) is poked on
  every successful submit, eliminating up-to-1ms scheduling latency on
  request pickup
- Polling intervals when no wake signal arrives: 100µs (active slots
  generating), 5ms (idle, no active slots)

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
embeddings. Full IMC lifecycle is detailed in §18.7.7.

#### 18.7.6 Context Pooling

Kronk uses two distinct context strategies depending on the workload.

**Text inference: single shared context.**

- One `llama.Context` is created in `NewModel` and reused across requests.
- KV cleanup splits by path (see §18.7.4):
  - Non-batched path → `resetContext()` runs `llama.Synchronize(m.lctx)` then
    `llama.MemoryClear(mem, true)`.
  - Batched path (text/IMC) → per-slot cleanup happens in
    [batch_finish.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go);
    `resetContext()` is skipped.
- Reusing a single context avoids GPU memory fragmentation on all backends
  (CUDA, Metal, Vulkan, ROCm) caused by repeated context alloc/free.

**Embedding & rerank: `contextPool`** ([pool.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/pool.go)):

- `newContextPool(model, ctxParams, log, n)` creates `n = NSeqMax`
  parallel `llama.Context` instances. All share the same `llama.Model`
  (weights), so only KV cache memory is multiplied per context.
- Available context indices are tracked via a buffered `avail chan int`;
  callers acquire by receiving from the channel and release by sending the
  index back.
- Inputs within a single embed/rerank request are partitioned across pool
  contexts for parallel processing — this is the concurrency semantic
  documented in §18.7.3 for embedding/reranking models.

#### 18.7.7 IMC Implementation Details

**Key Functions:**

The four entry points an agent will grep for live in
[caching_imc.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/caching_imc.go) and [caching_imc_media.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/caching_imc_media.go):

- `processIMC` — session selection / strategy dispatch
- `extendIMCCache` — append new messages to a matched session
- `buildIMCCacheFromScratch` — fresh build (no usable prefix)
- `rebuildIMCFromPartialPrefix` — salvage a token-prefix overlap

**Critical Implementation Details:**

1. **Extension tokenization must use `special=true`**:
   `llama.Tokenize(m.vocab, extension, m.addBOSToken, true)` — the `true`
   in the 4th arg ensures ChatML tokens like `<|im_start|>` are recognized.
   The `addBOS` arg uses the model's `addBOSToken` setting, not a hardcoded
   value.
2. **Prefix mismatch detection**: Use `strings.HasPrefix(fullPrompt, prefixPrompt)` to detect Jinja template nondeterminism.
3. **`add_generation_prompt=false` for cached prefixes**: Creates valid prefix for extension. Generation prompt added only for final suffix.

**IMC Algorithm — 5 strategies** (per [caching_imc.go:54-68](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/caching_imc.go#L54-L68)):

`processIMC` snapshots all sessions and picks one of:

1. **Pure cache hit** — `cachedMsgCount == len(messages)-1`. Nothing to
   decode beyond the suffix; KV state is already correct.
2. **Hash-prefix extend** — a session's `cachedMsgsHash` matches the prefix
   hash of the incoming messages → extend with
   `messages[cachedMsgCount : len-1]`.
3. **System-prompt preserve** — only the system prompt hash matches. The
   sys prompt KV is preserved; the conversation body is rebuilt fresh on
   top of it.
4. **Token-prefix fallback** — no hash match, but a session's
   `cachedTokens` shares a leading run with the incoming prompt's tokens.
   Trim to the common prefix, rebuild the rest (`rebuildIMCFromPartialPrefix`).
5. **Rebuild from scratch** — no usable overlap. Pick an empty session, or
   evict the LRU session by `lastUsed`, and call
   `buildIMCCacheFromScratch`.

**Multi-user IMC:**

Each of the `NSeqMax` sessions is an independent conversation branch.
Concurrent users and sub-agents land in different sessions via hash
matching, so they don't trample each other's caches. When all sessions are
full, the LRU session is evicted on `lastUsed`.

**Text vs Media IMC:**

- **Text sessions** externalize KV to RAM via `StateSeqGetData` after each
  request and restore into any free slot via `StateSeqSetData` on the next
  request. Sessions migrate freely between slots.
- **Media sessions** (vision/audio) stay **slot-dedicated**: image/audio
  embeddings cannot be externalized through `StateSeqGetData/SetData`, so
  the session is bound to a fixed slot for its lifetime. Media-specific
  build/extend logic lives in
  [caching_imc_media.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/caching_imc_media.go);
  the `hasMedia`, `useMRoPE`, and `mediaKVCounts` fields track media state.

**IMC Lifecycle (All Sessions):**

1. `processIMC()` scans **sessions** (not slots) for a hash match
2. `fillSlots()` assigns the job to the **first available slot**
3. `startSlot()` restores cached KV from RAM via `StateSeqSetData`
4. Cache is extended/rebuilt as needed, then snapshotted back to RAM via `StateSeqGetData`
5. Suffix tokens are decoded and generation runs
6. `finishSlot()` clears the full VRAM sequence (cached prefix already lives in RAM)

**IMC Session State** ([model.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/model.go#L36-L60)):

```go
type imcSession struct {
    slotID            int           // Stable session-pool index (== seqID)
    seqID             llama.SeqId   // KV sequence ID this session uses while resident in VRAM
    cachedMsgsHash    string        // Hash of all cached messages
    cachedTokens      []llama.Token // Full token sequence in KV cache
    totalTokensCached int           // Total KV positions cached
    cachedMsgCount    int           // Number of messages cached
    kvState           SessionStore  // Externalized KV state (kvstorage backend)
    lastUsed          time.Time     // Last access time (for LRU eviction)
    pending           bool          // True when build/extend in-flight; protects kvState
    hasMedia          bool          // True if cached content includes media
    useMRoPE          bool          // True if cached media used M-RoPE
    mediaKVCounts     []int         // KV positions per media chunk
    sysPromptHash     string        // Hash of system prompt message
    sysPromptTokens   int           // Token count of system prompt
}
```

`imcSessions` is sized 1:1 with execution slots at startup, but
sessions are **not** bound to slots — `kvState` (a
[SessionStore](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/session_store.go#L57-L97))
externalizes the cached KV bytes between requests so any matched
session can run on any free slot. The `slotID`/`seqID` fields name
the session pool entry and the KV sequence the session occupies
while bytes are still resident in VRAM (used for KV-pressure
eviction of un-externalized sessions). The `pending` flag is the
per-session in-flight latch that protects `kvState` from concurrent
writers.

#### 18.7.8 Tool Call Internals

**Processor state machine** ([processor.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/processor.go)):

A per-slot `processor` classifies streaming output token-by-token into one
of four statuses (`processor.go:10-15`):

| Constant           | Value | Meaning                                |
| ------------------ | ----- | -------------------------------------- |
| `statusNone`       | 0     | Initial / between segments             |
| `statusReasoning`  | 1     | Inside `<think>` (or model equivalent) |
| `statusCompletion` | 2     | Regular response text                  |
| `statusTooling`    | 3     | Inside a tool call                     |

While in `statusTooling`, tokens are appended to `toolCallBuf` and not
streamed to the caller as completion text. When the tool-call segment
closes, the buffered content is handed to `parseToolCall` for structured
extraction.

**Tool call format dispatch** ([processor_parse.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/processor_parse.go)):

`parseToolCall` routes accumulated tool-call content to a model-specific
parser based on its format:

| File                   | Format                                                                |
| ---------------------- | --------------------------------------------------------------------- |
| `processor_json.go`    | Standard JSON `{"name":..., "arguments":...}`                         |
| `processor_qwen.go`    | Qwen3-Coder XML-like `<tool_call>` / `<function=...>` tags            |
| `processor_gpt.go`     | GPT-OSS Harmony (`<\|channel\|>commentary to=NAME<\|constrain\|>...`) |
| `processor_glm.go`     | GLM `<arg_key>` / `<arg_value>` pairs                                 |
| `processor_gemma.go`   | Gemma4 tool calls                                                     |
| `processor_mistral.go` | Mistral / Devstral tool calls                                         |

**Split-token tag handling:**

Some models (Qwen3-Coder variants) emit a bare `<function=...>` without the
`<tool_call>` wrapper, and the tag itself can be tokenized across multiple
tokens (e.g. `<`, `function`, `=`). The processor uses `pendingTagBuf` /
`inPendingTag` to accumulate fragments until the tag is complete or
disproven.

**GPT-OSS Harmony channel handling:**

For GPT-OSS, the processor accumulates the channel name (`channelBuf`) and
watches for `<|constrain|>` (`awaitingConstrain`). The function name is
extracted from `to=NAME` in the channel and stored in `toolFuncName` for
later assembly into a structured `ResponseToolCall`.

**Tool call ID:**

IDs are generated by `newToolCallID()` as `"call_" + uuid.NewString()` —
stable contract for the OpenAI-compatible response wire format.

**chatMessage unmarshaling** (`models.go`):

- `Content` can be `nil` for assistant messages with `tool_calls`.
- Handle `len(app.Content) == 0 || string(app.Content) == "null"` as valid
  empty content.

**ToolCallArguments type:**

- Custom type that marshals to a JSON string (OpenAI spec).
- Unmarshals from either a string or an object for non-compliant clients.

#### 18.7.9 Logprobs Implementation

**Implementation** ([logprobs.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/logprobs.go)):

- `extractLogprobs(lctx, vocab, sampledToken, iBatch, topK, buf)`: retrieves
  logits via `llama.GetLogitsIth(lctx, iBatch, nVocab)` and converts them
  to log probabilities. `iBatch` identifies which slot's logits row to read
  when multiple slots are batched in one forward pass (see §18.7.5). `buf`
  is a pre-allocated byte buffer reused across tokens to avoid per-token
  allocations during top-K decoding.
- `logSoftmax()`: numerically stable log-softmax using the log-sum-exp trick.
- `getTopKLogprobs()`: `container/heap` min-heap for O(n log k) top-k
  extraction.

**Gating:**

The extraction path runs only when `params.TopLogprobs > 0`. With logprobs
disabled, this work is skipped entirely on the hot path.

**Critical ordering — extract before `llama.SamplerAccept`** ([batch_tokens.go:38, 57](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_tokens.go#L38-L57)):

`SamplerAccept` mutates sampler state (repetition history, penalty
buffers, dry/xtc state). Reading logits after acceptance would no longer
reflect the probability landscape for the token we're trying to score.

This corresponds to step 12.1 of the request flow in §18.11.

### 18.8 Responses API Normalization

The SDK's [response.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/response.go)
exposes the OpenAI Responses API on top of the same `model.Chat` engine that
serves Chat Completions. To do that, the input document is normalized into a
Chat-Completions-style `messages` array before `model.Chat` /
`model.ChatStreaming` is invoked.

**Owner**: the SDK methods `Response` ([line 140](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/response.go#L140))
and `ResponseStreaming` ([line 163](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/response.go#L163))
call `convertInputToMessages(d)` themselves as the first step. Callers (CLI,
MCP, server handlers) do not need to invoke it.

**Why it exists**: lets the SDK accept both the Chat-style `messages` payload
and the Responses-style `input` payload (string, message list, or
function-call/output items).

**Normalization helpers** (all in `response.go`):

| Function                    | Responsibility                                                                                                                                                                |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `convertInputToMessages`    | Top-level entrypoint; orchestrates the helpers below                                                                                                                          |
| `inputToMessages`           | Turns the `input` field (string / messages / items) into `[]model.D`                                                                                                          |
| `normalizeResponsesItems`   | Maps Responses items (`function_call`, `function_call_output`, …) to Chat messages; groups consecutive `function_call`s into one assistant message with multiple `tool_calls` |
| `normalizeResponsesContent` | Translates Responses content parts (`input_text`, `output_text`, …) inside existing messages into Chat-format content                                                         |
| `normalizeTools`            | Converts Responses' flat tool definitions into the Chat-Completions `{ "function": {...} }` shape                                                                             |
| `injectInstructions`        | Promotes the Responses `instructions` field into a leading `role:"system"` message                                                                                            |
| `extractInputParams`        | Pulls Responses-only parameters (e.g. `Instructions`) into `inputParams` for downstream handling                                                                              |
| `extractTools`              | Reads the (already-normalized) tool list out of the document                                                                                                                  |

### 18.9 Goroutine Budget

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
| Embedded MCP `http.Server` (listener + conns)  | ~2-3       | `cmd/server/app/domain/mcpapp/start.go`  |
| OTEL background collector probe                | 1          | `sdk/kronk/observ/otel/otel.go`          |
| `otelhttp.NewHandler` internals                | ~1-2       | `cmd/server/foundation/web/web.go`       |
| Batch engine `processLoop`                     | 1          | `sdk/kronk/model/batch_engine.go`        |

The baseline assumes embedded auth and embedded MCP — the defaults when
`KRONK_AUTH_HOST` and `KRONK_MCP_HOST` are unset. Pointing either at an
external service removes its row from the table.

**Per-request goroutines (~3-5 each):**

The floor is three: the `http.Server` connection handler, the SDK wrapper
goroutine (`streaming` or `streamingWith`), and the
`ChatStreaming`/`ResponseStreaming` request goroutine. `wrapChannelForLogging`
adds one more when `InsecureLogging` is enabled.

| Source                                                       | Location                   |
| ------------------------------------------------------------ | -------------------------- |
| `http.Server` connection handler                             | Go stdlib                  |
| `ChatStreaming` request goroutine                            | `sdk/kronk/model/chat.go`  |
| `streaming()` wrapper goroutine (Chat / Embedding paths)     | `sdk/kronk/concurrency.go` |
| `streamingWith()` wrapper goroutine (ResponseStreaming path) | `sdk/kronk/concurrency.go` |
| `wrapChannelForLogging` (only when `InsecureLogging` is on)  | `sdk/kronk/model/chat.go`  |

The goroutine metric is a point-in-time snapshot from `runtime.NumGoroutine()`
captured every 10th request by the metrics middleware. It includes everything
in the process, including Go runtime internals. After active requests complete,
the count drops back to the baseline.

### 18.10 Request Tracing Spans

Each chat completion request produces the following trace hierarchy.
`prepare-request`, `queue-wait`, and `process-request` are sibling spans
under the request's root context — none is a child of another.

```
POST /v1/chat/completions
├── prepare-request                          Validation, caching, prompt creation
│   ├── process-cache                        Cache lookup/update (IMC, when enabled)
│   │   ├── cache-tokenize-imc-prefix-match  Token-prefix fallback (§18.7.7 strategy 4)
│   │   ├── cache-tokenize-imc-extend        Hash-prefix extend (strategy 2)
│   │   ├── cache-tokenize-imc-sysprompt-preserve  System-prompt preserve (strategy 3)
│   │   ├── cache-tokenize-imc-scratch       Rebuild from scratch (strategy 5)
│   │   ├── cache-tokenize-imc-media-text-extend  Media-IMC text extend
│   │   └── cache-decode                     KV-fill decode for build/extend
│   └── create-prompt                        Jinja template application
│
├── queue-wait                               Job sits in requestQ until a slot picks it up
│
└── process-request                          Batch engine slot processing
    ├── prefill                              Tokenize + KV fill (ends at first token)
    └── token-generation                     Decode loop producing output tokens
```

**Phase 1: prepare-request** runs in the `ChatStreaming` request goroutine.
It validates the document, processes the IMC cache, and creates the prompt
via the Jinja template. When caching is enabled, `process-cache` and its
`cache-tokenize-imc-*` and `cache-decode` children appear here. Only the
tokenize variant matching the strategy chosen by `processIMC` is emitted on
any given request.

**queue-wait** is its own top-level span (not an attribute). It is started
at the very end of `prepare-request` in the request goroutine and ended by
`startSlot` ([batch_slot_start.go:36-39](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot_start.go#L36-L39))
when the batch engine picks the job up off `requestQ`. Its duration is the
gap between `prepare-request` ending and `process-request` starting.

**Phase 2: process-request** runs in the batch engine's `processLoop`
goroutine. The `prefill` span covers tokenization and KV cache filling.
Time to first token (TTFT) is measured from prefill start to the first
output token. The `token-generation` span covers the decode loop that
produces output tokens.

Additional spans that may appear at the top level:

| Span                    | When                      | Description                                         |
| ----------------------- | ------------------------- | --------------------------------------------------- |
| `model-file-load-time`  | First request for a model | Loading the GGUF model file                         |
| `proj-file-load-time`   | Vision/audio requests     | Loading the multimodal projection file              |
| `imc-media-cache-build` | Vision/audio IMC builds   | Media-IMC cache build (separate from the text path) |

### 18.11 Inference Code Path

This section traces a `ChatStreaming` request end-to-end. Each step has a
high-level description followed by a **Code:** sub-block listing the
function calls and file locations the agent will navigate.

#### Step 1: Receive the Request

The caller provides a document containing messages and sampling parameters.
The SDK validates that the request's context has a deadline to prevent
unbounded processing.

**Code:**

- `Kronk.ChatStreaming` ([sdk/kronk/chat.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/chat.go)) — validates the context deadline and wraps `Model.ChatStreaming` in a closure.

#### Step 2: Acquire the Model

The kronk-level semaphore controls how many requests can be in-flight at
once. The request blocks here until a slot opens up, providing backpressure
when the system is under load. See §18.7.4 for the full acquire/release
contract.

**Code:**

- `streaming()` / `streamingWith()` ([sdk/kronk/concurrency.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/concurrency.go)) calls `acquireModel()` ([sdk/kronk/acquire.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/acquire.go)) — checks the shutdown flag, increments the kronk-level `krn.activeStreams`, and blocks on the semaphore (with `ctx.Done` cancellation).
- The wrapper goroutine `defer`s `releaseModel()` and `close(ch)`.

#### Step 3: Validate the Document

The request document is validated to ensure it contains properly structured
messages. Sampling parameters (temperature, top_p, top_k, min_p, max_tokens,
grammar, etc.) are extracted and resolved against model defaults. The document
is shallow-cloned so downstream processing can modify it without affecting the
caller.

**Code:**

- `Model.ChatStreaming` ([model/chat.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/chat.go)) creates the response channel, wraps it with `wrapChannelForLogging` if `InsecureLogging` is on, increments the **model-level** `m.activeStreams` (a separate counter from the kronk-level one in Step 2 — both are waited on independently during unload), and spawns the request goroutine with the `prepare-request` span (§18.10).
- `validateAndCloneDocument()` (`model/chat.go`) — validates the `messages` field, calls `parseParams()` to extract sampling parameters, shallow-clones the document.

#### Step 4: Prepare the Context

The system determines whether this is a text-only or media (vision/audio)
request:

- **Text**: multi-part content arrays are flattened into plain strings.
- **Media**: the projection model is loaded; media content (images or
  audio) is detected and converted into raw bytes for the encoder pipeline.

**Code:**

- `prepareContext()` (`model/chat.go`) returns `ObjectChatText` or `ObjectChatMedia`.
  - Text path: `prepareTextContext()`.
  - Media path: `prepareMediaContext()` — loads the projection file via `mtmd.InitFromFile()` and converts the OpenAI media format to byte slices.

#### Step 5: Process the Cache

If caching is enabled, the system checks whether any portion of the
conversation is already in the KV cache to avoid redundant computation. The
IMC algorithm picks one of **5 strategies** — see §18.7.7 for the full
decision tree (pure cache hit, hash-prefix extend, system-prompt preserve,
token-prefix fallback, or rebuild from scratch with LRU eviction).

Tool response messages are also enriched with their originating function
names so templates can render tool results correctly.

**Code:**

- `prepareCacheAndPrompt()` (`model/chat.go`):
  - `injectToolResponseNames()` — adds `name`/`tool_call_name` to `role:"tool"` messages by matching `tool_call_id`.
  - `processCache()` (`model/caching.go`) → `processIMC()` ([model/caching_imc.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/caching_imc.go)) — runs the §18.7.7 5-strategy selection across all `NSeqMax` sessions, tokenizes any extension tokens, and sets the `pending` flag on the chosen session.

#### Step 6: Apply the Chat Template

The remaining (non-cached) messages are run through the model's Jinja2 chat
template. This converts the structured message array into the exact prompt
string the model expects, including any special tokens, role markers, and
tool definitions. For media requests, raw media bytes are returned alongside
the text prompt.

**Code:**

- `createPrompt()` → `applyRequestJinjaTemplate()` (`model/chat.go`) — returns the prompt string plus media byte slices.

#### Step 7: Submit to the Batch Engine

The fully prepared request — prompt string, media bytes, sampling parameters,
and cache state — is packaged into a job and placed on the batch engine's
request queue. A wake signal is sent so the batch engine picks it up
immediately rather than waiting for its next poll cycle.

**Code:**

- `submitToBatchEngine()` (`model/chat.go`) — builds the `chatJob` struct (request data, cache state, IMC fields) and calls `batch.submit()` ([model/batch_engine.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_engine.go)), which pushes onto `requestQ` and pokes `wakeCh`. The `queue-wait` span (§18.10) starts here.

#### Step 8: Assign to a Slot

The batch engine's processing loop wakes up and checks for pending work. It
dequeues the job and assigns it to the first available processing slot. All
IMC sessions (text and media) use first-available slot assignment. If all
slots are busy, the longest-running slot is preempted after a configurable
timeout (`cache-slot-timeout`, see §18.7.5).

**Code:**

- `processLoop()` ([model/batch_engine.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_engine.go)) — signal-based wake on `wakeCh` (§18.7.5); polls at 100µs when active, 5ms when idle.
- `processBatch()` clears the batch buffer, runs any pending slot preemption, prefills the draft model for speculative slots, adds 1 generation token per active slot, then continues text prefill via round-robin `addPrefillChunk()` and media prefill via `addPrefillMediaChunk()`.
- `fillSlots()` ([model/batch_schedule.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_schedule.go)) dequeues the job and assigns it to a slot.

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

**Code:**

- `startSlot()` ([model/batch_slot_start.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot_start.go)) — resets the slot, ends the `queue-wait` span, and starts the `process-request` and `prefill` spans (§18.10).
- `toSampler()` builds the llama.cpp sampler chain (temperature, top_k, top_p, min_p, repetition penalties, DRY, XTC, mirostat); a separate grammar sampler is created if requested.
- IMC KV restore: `StateSeqSetData` from `session.kvState`, then `decodeTokensIntoCache()` for extend, or `MemorySeqRm` for rebuild, or partial trim.
- IMC KV snapshot: `StateSeqGetData` into `session.kvState` after build/extend.
- `llama.Tokenize(m.vocab, prompt, m.addBOSToken, true)` — `special=true` ensures ChatML markers are recognized (§18.7.7).
- Draft prompt assembly for speculative decoding.
- First chunk added via `addPrefillChunk()`.

#### Step 10: Prefill (KV Cache Fill)

The prompt tokens are fed through the model in chunks to build up the KV
cache — this is the "prefill" phase. Tokens are added to a batch buffer up
to the configured batch size limit, then a GPU forward pass (decode) is
executed. When multiple slots are active, tokens are allocated round-robin
across slots so no single request can starve others. This repeats until all
prompt tokens have been processed.

For media requests, image or audio embeddings are interleaved with text
tokens and decoded through the model's multimodal pipeline.

**Code:**

- `addPrefillChunk()` ([model/batch_prefill_text.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_prefill_text.go)) — adds tokens up to the `NBatch` cap; round-robin chunk size is `NUBatch`.
- Each token: `batch.Add(token, position, seqIDs, isLast)`.
- `llama.Decode(lctx, batch)` — GPU forward pass, fills KV cache.
- `llama.Synchronize(lctx)` — waits for GPU completion.
- Repeats until all prefill tokens are consumed.
- Media path: `addPrefillMediaChunk()` ([model/batch_prefill_media.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_prefill_media.go)) interleaves embeddings with text tokens.

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

**Code:**

- Back in `processBatch` (`model/batch_engine.go`), for each active slot with `prefillDone=true`: `batch.Add(sampled, nPast, seqIDs, true)` then `llama.Decode()`.
- Speculative path: `generateDraftTokens()` → batch in draft + sampled → `verifySpeculativeTokens()` ([model/batch_speculative.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_speculative.go)).
- `processSlotToken()` ([model/batch_tokens.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_tokens.go)) samples via `llama.SamplerSample(sampler, lctx, iBatch)` or, when grammar is active, `grammarSampler.SampleWithGrammar(lctx, sampler, iBatch)` ([model/grammar.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/grammar.go)).

#### Step 12: Process Each Token

Each sampled token goes through a processing pipeline:

1. **Logprobs extraction**: If requested, token log-probabilities are
   extracted from the model's logits before the sampler state is updated
   (§18.7.9 explains why the order matters).
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

**Code:**

- `handleSampledToken()` (`model/batch_tokens.go`) drives the pipeline:
  - `extractLogprobs()` ([model/logprobs.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/logprobs.go)) — `llama.GetLogitsIth` + log-softmax + top-k heap. Runs **before** `llama.SamplerAccept()` (§18.7.9).
  - `llama.SamplerAccept()` (and grammar accept).
  - `llama.VocabIsEOG()` → if true, jump to `finishSlot()`.
  - `llama.TokenToPiece()` → buffer partial multi-byte codepoints, then `extractCompleteUTF8()` ([model/batch_utf8.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_utf8.go)).
  - First token: records `prefillDone=true`, computes TTFT, ends the `prefill` span, starts the `token-generation` span.
  - Processor classification: `stepGPT()` for GPT-OSS, `stepStandard()` for everything else (§18.7.8). Classifies content into reasoning / completion / tooling and detects tool-call markers.
  - Counter increment: `reasonTokens` or `completionTokens`.
  - Max-tokens check: if `outputTokens >= maxTokens`, `finishSlot()`.
  - Accumulate into `finalContent` / `finalReasoning` / `finalTooling`.
  - `sendDeltaResponse()` for non-tool content (tool content is buffered until parse).

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

**Code:**

- `finishSlot()` ([model/batch_finish.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go)):
  - Flushes the UTF-8 buffer.
  - Tool-call parse: GPT-OSS path calls `parseGPTToolCall()` directly ([batch_finish.go:177](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go#L177)); all other models route through `parseToolCall` ([model/processor_parse.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/processor_parse.go)) — see §18.7.8.
  - Metrics: TPS = `(outputTokens - 1) / elapsed`, TTFT, draft-acceptance rate.
  - `sendFinalResponse()` with usage, content, reasoning, tool calls, logprobs.
  - KV cleanup: `MemorySeqRm(mem, seqID, -1, -1)` clears the full VRAM sequence; the IMC prefix is already in RAM from Step 9.
  - Frees the sampler, grammar sampler, MTMD bitmaps/chunks, and `mtmdCtx`.
  - Closes the job channel; the `streaming()` wrapper drains and closes the caller's channel.
  - Decrements the **model-level** `m.activeStreams`.

#### Step 14: Release the Model

The response channel is closed, signaling to the caller that streaming is
complete. The kronk-level semaphore slot is released, allowing the next
queued request to begin processing.

**Code:**

- `releaseModel()` ([sdk/kronk/acquire.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/acquire.go)) — drains a slot from the kronk-level semaphore (`<-krn.sem`) and decrements `krn.activeStreams` (the kronk-level counter; the model-level one was already decremented in Step 13).

### 18.12 MTP Internals

This section documents the engine internals of the MTP (Multi-Token
Prediction) drafter shipped in
[PR #593](https://github.com/ardanlabs/kronk/pull/593). The user-facing
operation guide lives in
[Chapter 6](#chapter-6-speculative-decoding--mtp); the configuration
surface lives in [Chapter 3 §3.12](#312-speculative-decoding). This
section is the only place that goes inside the FFI bindings, mirror
step, three-pass dispatch, and hybrid snapshot/restore.

The drafter sits behind a single `*draftModel` type
([model.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/model.go)),
selected once at model load by `selectAndLoadDraft`
([draft_mtp.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/draft_mtp.go)).
In separate-GGUF mode it loads a second `llama_model`; in MTP mode it
shares the target's `llama_model` and binds an MTP head that lives
inside the target GGUF.

Reference: `common/speculative.cpp common_speculative_impl_draft_mtp`
in upstream llama.cpp. Kronk's implementation lives in
[`draft_mtp.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/draft_mtp.go)
(load), [`batch_mtp.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_mtp.go)
(mirror + AR loop), and integration changes in
[`batch_engine.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_engine.go),
[`batch_slot.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot.go),
[`batch_slot_start.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot_start.go),
[`batch_speculative.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_speculative.go),
[`batch_prefill_text.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_prefill_text.go),
[`batch_finish.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go),
and the FFI bindings in
[`yzma.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/yzma.go).

#### 18.12.1 Auto-detection

`selectAndLoadDraft` runs once during model initialization
(`initGenerationRuntime` in `model.go`) and decides which drafter, if
any, to load:

```diagram
                       ╭───────────────────────╮
                       │ cfg.DraftModel != nil │──── yes ──▶  loadDraftModel  (separate-GGUF)
                       ╰──────────┬────────────╯
                                  │ no
                                  ▼
                       ╭───────────────────────╮
                       │ mtpNextNLayers(target)│──── 0 ──▶  return (nil, nil) — no drafter
                       ╰──────────┬────────────╯
                                  │ > 0
                                  ▼
                       ╭───────────────────────╮
                       │     MTPAvailable()    │──── false ──▶ skip (log reason: old llama.cpp)
                       ╰──────────┬────────────╯
                                  │ true
                                  ▼
                       loadDraftModelMTP  (auto-enabled; inherits NSeqMax)
```

`mtpNextNLayers` looks up the GGUF metadata key
`<arch>.nextn_predict_layers` (a uint32). Kronk matches by the unique
substring `nextn_predict_layers` so the same lookup works for every
architecture variant without first reading `general.architecture`.

`MTPAvailable()` probes whether the loaded llama.cpp library exports
the three pre-norm symbols listed in §18.12.2. Older builds (pre
`src/llama-ext.h`) won't have them — Kronk logs and starts up without
MTP rather than crashing on a missing symbol mid-request.

The historical `NSeqMax == 1` gate (present through earlier revisions
of PR #593) has been removed: the draft context inherits `NSeqMax`
from the target and hosts as many sequences as the target does. The
three-pass post-decode in `processBatch` (§18.12.5) makes the spec
verify path multi-slot safe.

When any of the gates fail, `selectAndLoadDraft` logs the specific
reason and returns `(nil, nil)`. The target still loads and serves
traffic — just without speculation.

#### 18.12.2 Pre-Norm Hidden-State Plumbing

The MTP path needs three llama.cpp C symbols that yzma upstream does
not yet bind. Kronk adds them locally in
[`yzma.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/yzma.go)
via the `jupiterrider/ffi` package:

| Symbol                                | Go wrapper                                            | Purpose                                                                                              |
| ------------------------------------- | ----------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `llama_set_embeddings_pre_norm`       | `SetEmbeddingsPreNorm(ctx, value, masked)`            | Toggle pre-norm extraction on a context. `masked=false` = dense (all rows); `masked=true` = sparse (logit-flagged rows only). |
| `llama_get_embeddings_pre_norm`       | `GetEmbeddingsPreNorm(ctx, nRows, nEmbd) []float32`   | Return the dense buffer produced by the most recent `llama_decode`. Used on the target.              |
| `llama_get_embeddings_pre_norm_ith`   | `GetEmbeddingsPreNormIth(ctx, i, nEmbd) []float32`    | Return a single row by output-table index. Used on the draft (masked) context.                       |

Two binding details worth highlighting:

- **Symbol probing is dual.** Each prep tries the C-linkage name first
  and falls back to the Itanium C++ ABI mangled form (e.g.
  `_Z29llama_set_embeddings_pre_normP13llama_contextbb`) so kronk
  binds against llama.cpp builds compiled with or without `LLAMA_API`
  on these declarations.
- **Best-effort init.** `InitYzmaWorkarounds` never fails on a missing
  pre-norm symbol. The corresponding `ffi.Fun` stays zero-valued and
  `MTPAvailable()` returns false, gating §18.12.1.

At load time `loadDraftModelMTP` sets:

- `SetEmbeddingsPreNorm(targetCtx, true, false)` — dense, every row
  accessible by raw batch index. Required for the mirror step
  (§18.12.3), which reads arbitrary rows from each completed target
  batch.
- `SetEmbeddingsPreNorm(draftCtx, true, true)` — sparse, only
  logits-flagged rows stored. The draft only needs the single output
  row of each AR step.

The flag is consumed at graph-build time, so it must be set **before**
the first decode on either context.

#### 18.12.3 Mirror Step and AR Draft Loop

Two functions in [`batch_mtp.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_mtp.go)
do the heavy lifting.

##### Mirror: `mirrorTargetBatchToMTPDraft`

After every successful target `llama_decode` + `llama_synchronize`, the
post-decode pass in `processBatch` calls the mirror to replay the
slot's just-decoded range into the draft context with `batch.embd`
populated from the target's pre-norm buffer.

Per-position alignment is **shift-right-by-1**, matching
`common_speculative_impl_draft_mtp`:

```
mirror[0]   : token = tgt[start+0],  embd = pendingH (slot's pre-batch h)
mirror[k>0] : token = tgt[start+k],  embd = h_tgt[start+k-1]
```

`pendingH` is a per-slot copy of the hidden row at the last committed
target position. On the very first decode of a sequence, no `h` has
been observed yet — that slot of the mirror batch is zeroed (the MTP
head's first prediction at position 0 is on a BOS / instruction
sentinel where exact `h` does not matter).

After the mirror succeeds, `pendingH` is updated to the last
target-batch row so it's ready as the slot-0 input of the next
mirror.

A few non-obvious correctness points enforced in the function:

- **Chunking by mirror capacity.** The mirror batch is allocated at
  `NBatch` capacity. When `effectiveCount > NBatch` the mirror is run
  in chunks, with `llama.Synchronize(draft)` **inside the chunk loop**
  before the next chunk overwrites `mirror.Embd`. Without the per-chunk
  sync, the next chunk's `copy()` into the Go-owned embd slice races
  the still-in-flight C read on async backends (Metal/CUDA) and
  corrupts the input.
- **`effectiveCount` is caller-provided.** Prefill chunks and plain
  gen-token decodes mirror `targetBatchCount` positions; the spec
  path mirrors only `1 + accepted` rows so rejected draft tokens are
  never reflected into draft KV.
- **`logits=true` only on the last row.** The mirror only needs the
  pre-norm row of the very last position (as the next `pendingH`), so
  only the last row is logits-flagged.

##### AR Draft: `generateDraftTokensMTP`

The drafter runs an auto-regressive loop on the MTP context. Each
iteration:

1. Build a single-token batch with `(curToken, pos, seqIDs)` and copy
   `curEmbd` into the embd slot.
2. `llama.Decode` + `llama.Synchronize` (async-backend safety again).
3. `llama.SamplerSample(greedy, ctx, -1)` to pick the next draft token.
4. `GetEmbeddingsPreNormIth(ctx, 0, nEmbd)` to read back the next
   hidden state.
5. EOG check; copy `nextEmbd` into `pendingH`; advance.

The loop stops on `chooseNDraft(s, draft.nDraft)` rounds (see
§18.12.4 acceptance scaling), or earlier on EOG or decode failure.

**Why MTP-only batches?** `llama.BatchInit(N, embd, nSeqMax)` allocates
**either** the token buffer **or** the embd buffer — never both —
based on its `embd` arg. MTP needs both per position. Kronk works
around this by calling `BatchInit(N, 0, 1)` to get a token-only batch
(with `pos`, `seq_id`, and `logits` arrays sized to `N`) and then
attaching a Go-allocated `[]float32` of size `N*nEmbd` as the embd
buffer. The Go slice is pinned (`runtime.Pinner`) for the batch's
lifetime and the `Batch.Embd` pointer is cleared **before**
`BatchFree` so llama.cpp's unconditional `free(batch.embd)` doesn't
`free()` a Go heap allocation.

These two MTP-only batches live on `draftModel`:

- `draftBatchMTP` — capacity 1, used by `generateDraftTokensMTP` per
  step.
- `mirrorBatchMTP` — capacity `NBatch`, used by the mirror step.

#### 18.12.4 Verification on the MTP Path

`verifySpeculativeTokens` is shared between separate-GGUF and MTP, but
the MTP path forces **greedy verification** unconditionally because
the MTP head currently runs only greedy sampling (`SamplerInitGreedy`)
and the AR loop does not capture sparse draft distributions. Running
the probabilistic verify path without a draft distribution would fall
through to `sampleFromProbs(target)` at every position and reject
every draft token unconditionally.

To compensate, the greedy branch is taught — only on the MTP path
(`mtpGreedy == true`) — to invoke the slot's **full sampler** at each
position instead of taking the raw target argmax. That preserves the
user's `temperature` / `top_k` / `top_p` shape on the emitted
sequence. The mathematical guarantee of distribution-equivalent
output (Leviathan et al., 2023) is lost on the MTP path — it is the
standard approximation when the draft distribution is unavailable.

`originalSampled` is also snapshotted before the verify loop, because
`handleSampledToken` mutates `s.sampled` as each accepted draft token
flows through the streaming pipeline. The hybrid re-decode path
(§18.12.6) needs the **original** sampled token at the base position;
using the mutated value would re-decode the wrong token and corrupt
every subsequent round.

After verify, the MTP mirror runs again over `1 + accepted` rows to
overwrite the AR-loop draft KV entries with target-derived hidden
states. That update is what makes the next round's `pendingH` reflect
reality.

`rollbackDraft` for MTP is also different from the separate-GGUF
path: it `MemorySeqRm`s the **entire** drafted range from the draft
KV before the post-verify mirror runs. llama.cpp's transformer KV
does not overwrite by `(seq, pos)` on re-decode — it appends another
slot, leaving duplicate entries that corrupt subsequent attention.
The mirror then writes the correct target-derived entries into clean
slots.

**Adaptive `nDraft` (acceptance EMA).** `chooseNDraft(s, maxDraft)`
in
[`batch_speculative.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_speculative.go)
scales `nDraft` down based on the slot's exponential moving average
of acceptance rate (`specAccEMA`). The EMA formula is
`0.9*old + 0.1*(accepted/nDraft)` and persists across requests on the
slot. The user-visible buckets (`< 0.30` → 0, `< 0.50` → 1, etc.) are
documented in Chapter 6 §6.4.

When the EMA collapses to ~0, `chooseNDraft` returns 0 and the spec
path is bypassed for that round — but the draft-tokens / accepted /
acceptance-rate fields are still emitted on the final slot log line
so dashboards see a stable schema. See `finishSlot` and
`sendFinalResponse`.

#### 18.12.5 Three-pass `processBatch` and Phase A / Phase B Split

When `nseq-max > 1` and two or more spec slots verify in the same
shared batch, the original monolithic `verifySpeculativeTokens` could
corrupt the target context's logit buffer for one slot while a peer
was still trying to read it. The hybrid `restoreTargetSpecSnapshot`
(§18.12.6) re-decodes a small batch on the target context, and that
re-decode **replaces the per-context logit buffer with logits for
only the re-decoded rows** — every other slot's batch rows return
`nullptr` from `llama_get_logits_ith`, crashing
`llama_sampler_sample` (`GGML_ASSERT(logits != nullptr)` at
`llama-sampler.cpp:850`).

The fix has two parts:

1. **`verifySpeculativeTokens` is split** in
   [batch_speculative.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_speculative.go):
   - **Phase A (`verifySpeculativeTokens`) — read-only.** Runs the
     verify loop (logit reads, accept / reject, per-accepted
     `handleSampledToken`), samples the bonus token, updates the
     acceptance EMA. Stashes `accepted`, `bonusToken`, and
     `originalSampled` on the slot via new `specPending*` fields and
     sets `specPendingFinalize = true`. Does NOT touch target KV,
     draft KV, `s.nPast`, or `s.iBatch`. `s.specDraftTokens` is
     deliberately retained for Phase B.
   - **Phase B (`finalizeSpeculativeTokens`) — mutating.** Runs the
     rollback (hybrid restore or `MemorySeqRm`), draft KV rollback,
     MTP mirror, sets `s.nPast`, emits the throttled `verify-done`
     log, streams the bonus token, sets `s.iBatch = -1`, and clears
     the pending fields. Early-returns silently when
     `specPendingFinalize` is false (Phase A short-circuited on EOG).

2. **`processBatch` post-decode is three-pass** in
   [batch_engine.go](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_engine.go):

| Pass | Slots                                                        | Work                                                                                                          |
| ---- | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------- |
| 1    | Non-spec (`s.specDraftTokens == nil`)                         | MTP mirror (if applicable) + `processSlotToken`. Target logit buffer is fully intact.                          |
| 2A   | Spec (`s.specDraftTokens != nil`)                             | Phase A — `verifySpeculativeTokens`. Pure reads on the target logit buffer, so all spec slots run safely back to back. |
| 2B   | Spec with `specPendingFinalize == true`                       | Phase B — `finalizeSpeculativeTokens`. Hybrid restores can wipe the logit buffer here; by this point every other spec slot has already consumed its logits. |

EOG handling: when `handleSampledToken` inside Phase A finishes the
slot (`finishSlot` → `reset`), the `specPending*` fields stay
defaulted and Phase B's first-line guard skips the slot. The
deferred EMA update in Phase A still fires once via `defer` so the
EMA is updated exactly once per round.

Under `nseq-max == 1` the ordering Phase A → Phase B for a single
spec slot is functionally identical to the old monolithic
`verifySpeculativeTokens`, so this split has no behavioral effect on
the single-slot path.

A subtle logprobs note: Phase B's bonus-token `handleSampledToken`
runs **after** any hybrid restore. The restore's re-decode marks
`logits = true` only on the last re-decoded position
(`basePast + accepted`), which is exactly the bonus token's iBatch
position, so logprob extraction at that site still works on the
hybrid path.

#### 18.12.6 Hybrid Target Rollback

Hybrid target models (transformer + recurrent layers) introduce a
problem the regular `MemorySeqRm` rollback cannot solve: the
recurrent layer has been **advanced through all `1+nDraft` decoded
positions** and there is no per-position trim. A partial-rejection
round would leave the recurrent state advanced past the accepted
boundary, and the next `llama_decode` would fail with `-1`.

Two helpers in `batch_speculative.go` solve this:

| Helper                          | What it does                                                                                                                                                |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `captureTargetSpecSnapshot(s)`  | Sizes `s.specSnapshot` via `StateSeqGetSize` and reads the full per-sequence state with `StateSeqGetData`. Called **before** the spec batch is decoded.     |
| `restoreTargetSpecSnapshot(s)`  | `StateSeqSetData` to rewind, then re-decode `(sampledAtBase + first accepted drafts)` so the seq ends at exactly `basePast + 1 + accepted` correct positions. |

The snapshot buffer is lazy-grow / never-shrink on the slot
(`s.specSnapshot`). Size scales with current KV occupancy, so the cost
grows with context length. Dense / pure-attention targets skip this
path entirely — `MemorySeqRm` is correct and much cheaper for them.

The captureTarget/restoreTarget hooks are gated on
`e.model.modelInfo.Type == ModelTypeHybrid` so the dense fast path is
untouched. If `captureTargetSpecSnapshot` errors,
`verifySpeculativeTokens` clears `s.specSnapshot` and falls through
to `MemorySeqRm`. The fallback is broken on hybrid partial-reject
rounds, but full-accept rounds still work, and the next request
begins with a fresh sequence anyway.

**Multi-slot interaction.** `restoreTargetSpecSnapshot`'s re-decode
invalidates the target's per-context logit buffer for every other
batch row. With `nseq-max > 1` this is benign because the restore
only runs in Pass 2B (`finalizeSpeculativeTokens`), after every other
spec slot has read its logits in Pass 2A. See §18.12.5.

**MTP mirror interaction.** The same re-decode also overwrites the
target's per-context **pre-norm** buffer with rows indexed against the
small rebatch, not the original shared `e.batch`. Phase B's MTP mirror
(`mirrorTargetBatchToMTPDraft`) would then read the wrong pre-norm
rows. To decouple the mirror from the restore, Phase A
(`verifySpeculativeTokens`) copies the slot's pre-norm rows into a
slot-local `s.verifyH` buffer via `captureVerifyPreNorm` **before** any
Phase B side-effect can mutate the live buffer. Phase B's mirror reads
from `s.verifyH` when populated, so MTP keeps running across partial
rejections on hybrid targets instead of being disabled for the rest of
the request.

#### 18.12.7 Per-Slot MTP State

PR #593 added the following fields to `slot` in
[`batch_slot.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot.go).
All are reset in `slot.reset()` with lazy-grow / never-shrink
buffer policy.

| Field                                                 | Purpose                                                                                                                                   |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `pendingH []float32`                                  | Copy of the most-recently committed target pre-norm row. Slot-0 input of the next mirror batch.                                            |
| `targetBatchStart / Count / BasePos`                  | Slot's contiguous range inside the shared target batch — captured at batch-add time so the post-decode mirror knows where its rows live. |
| `mtpHasBatch`                                         | True between `batch.Add()` and the post-decode mirror; cleared by the mirror.                                                              |
| `mtpDisabledForRequest`                               | Disables MTP for the remainder of the current request. Set at `startSlot` on IMC cache hits where the matched session has no draft-seq snapshot (or the draft restore returned 0 bytes) — MTP-aware IMC builds snapshot both target and draft seqs so this failsafe rarely fires on freshly-built caches. Also set inside `finalizeSpeculativeTokens` after a post-rollback mirror failure (the draft KV is wiped and the slot continues target-only). Cleared by `slot.reset()` when the slot is recycled for the next request. |
| `verifyH []float32`                                   | Slot-local cache of the target's pre-norm hidden-state rows for the just-decoded spec batch range (1+nDraft rows of nEmbd floats). Captured at the top of `verifySpeculativeTokens` (Phase A) BEFORE any Phase B side-effect (notably `restoreTargetSpecSnapshot`'s re-decode on a hybrid target) can invalidate the per-context pre-norm buffer. `mirrorTargetBatchToMTPDraft` reads from this buffer when populated and clears it after consumption. Lazy-grow / never-shrink. |
| `specSnapshot []byte`                                 | Pre-spec target state buffer for hybrid rollback (§18.12.6). Lazy-grow.                                                                    |
| `specRounds`                                          | Counter used to throttle per-round verify logging (logs first round, then every 32nd).                                                     |
| `specPendingFinalize bool`                            | Gates Phase B (§18.12.5). True between a successful Phase A and the matching Phase B. EOG in Phase A leaves it false so Phase B silently skips. |
| `specPendingAccepted int`                             | Phase A → Phase B hand-off: accepted draft count.                                                                                          |
| `specPendingBonusToken llama.Token`                   | Phase A → Phase B hand-off: bonus token sampled at `baseBatch + accepted`.                                                                  |
| `specPendingOriginalSampled llama.Token`              | Phase A → Phase B hand-off: snapshot of `s.sampled` taken before any `handleSampledToken` mutated it. Hybrid restore needs this for the re-decode at `basePast`. |

#### 18.12.8 Code Map

| File                                                                                                                                         | Role for MTP                                                                                                                       |
| -------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| [`sdk/kronk/model/draft_mtp.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/draft_mtp.go)                     | `mtpNextNLayers`, `loadDraftModelMTP`, `selectAndLoadDraft`. Sole source for MTP load + detect.                                    |
| [`sdk/kronk/model/batch_mtp.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_mtp.go)                     | `mirrorTargetBatchToMTPDraft`, `generateDraftTokensMTP`, `decodeTokensIntoCacheMTP` (IMC cache build with mirror), `mirrorBuildChunkToMTPDraft`, helpers (`batchTokensAt`, `mirrorBatchCapacity`). |
| [`sdk/kronk/model/yzma.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/yzma.go)                               | FFI bindings for the three pre-norm symbols; `MTPAvailable`, `SetEmbeddingsPreNorm`, `GetEmbeddingsPreNorm{,Ith}`.                 |
| [`sdk/kronk/model/model.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/model.go)                             | `draftModel` struct extended with MTP fields (`mtp`, `nEmbd`, MTP batches, pinned embd slices). `Unload` skips shared `ModelFree`. |
| [`sdk/kronk/model/batch_slot.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot.go)                   | `slot` struct extended with per-slot MTP state (`pendingH`, target-batch range, `mtpHasBatch`, `mtpDisabledForRequest`, `specSnapshot`, `specRounds`). |
| [`sdk/kronk/model/batch_slot_start.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_slot_start.go)       | Skips separate-draft-prefill on MTP; dispatches the MTP-aware `decodeTokensIntoCacheMTP` during IMC cache build so draft KV is populated in lock-step; snapshots/restores the draft seq + pendingH alongside the target so cache hits keep MTP running. Only disables MTP for a cache-hit request when the matched session has no draft snapshot. |
| [`sdk/kronk/model/batch_engine.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_engine.go)               | `processBatch` integration: claims slot's target-batch range at every add site, mirrors after every successful decode, dispatches MTP vs separate-GGUF draft generation. |
| [`sdk/kronk/model/batch_prefill_text.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_prefill_text.go)   | `addPrefillChunk` claims (or extends) the slot's MTP target-batch range so prefill rows get mirrored.                              |
| [`sdk/kronk/model/batch_speculative.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_speculative.go)     | Greedy-only MTP verify path; `originalSampled` snapshot; hybrid snapshot/restore; post-verify mirror; throttled `verify-done` log; MTP-specific `rollbackDraft`. |
| [`sdk/kronk/model/batch_finish.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/batch_finish.go)               | Always-emit draft metrics when a drafter is configured.                                                                            |
| [`sdk/kronk/model/params.go`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/model/params.go)                           | `top_p == 0 || == 1` from the request is treated as unset so the model-config `top_p` survives.                                    |

#### 18.12.9 Testing

Test package: [`sdk/kronk/tests/mtp/`](file:///Users/bill/code/go/src/github.com/ardanlabs/kronk/sdk/kronk/tests/mtp).

The suite is a smoke test against the
`unsloth/Qwen3.6-35B-A3B-MTP-UD-Q2_K_XL` target via `testlib.CfgMTPChat()`.
A successful `Chat` and `ChatStreaming` response implicitly verifies
that:

- The MTP draft context loaded (auto-detection passed).
- Pre-norm extraction is wired correctly on both contexts.
- The mirror step is in sync after every target decode.
- Speculation produced valid drafts and the target accepted and
  emitted clean text.

`TestMain` skips the whole suite when the MTP model file is not
downloaded, so contributors without the GGUF locally still get a green
run.

Run from the project root:

```shell
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE=$(pwd)
go test -v -count=1 ./sdk/kronk/tests/mtp/...
```
