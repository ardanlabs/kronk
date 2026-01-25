# AGENTS.md

Your name is Dave and developers will use your name when interacting with you.

## Build & Test Commands

- Install CLI: `go install ./cmd/kronk`
- Run all tests: `make test` (requires `make install-libraries install-models` first)
  - Tests require environment variables:
    ```
    export RUN_IN_PARALLEL=yes
    export GITHUB_WORKSPACE=/path/to/kronk  # project root where makefile is located
    ```
  - Tests must be run from the project root directory
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

## Core SDK Documentation

Detailed documentation for the core inference packages is maintained in their respective directories:

- **`sdk/kronk/AGENTS.md`** - Streaming architecture, model pool acquire/release, cleanup flow, GPT-OSS processor
- **`sdk/kronk/model/AGENTS.md`** - Context pooling, KV cache configuration, resource lifecycle, tool call handling, model config fields

## API Handler Notes (cmd/server/app/domain/)

**Input format conversion**: Both streaming and non-streaming Response APIs must call `convertInputToMessages(d)` to handle OpenAI Responses `input` field format

**Default context parameters**: Chat and Response handlers call `cache.ApplyDefaults()` to merge per-model sampling defaults from `model_config.yaml` into requests

## Sampling Parameter Defaults

Default sampling parameters are defined in two places that **must stay in sync**:

- **Source of truth**: `sdk/kronk/model/params.go` (Go constants `defTemp`, `defTopK`, etc.)
- **Documentation**: `zarf/kms/model_config.yaml` (`default-context` section in example)

When updating default values, update both files. The cache package (`cmd/server/app/sdk/cache/cache.go`) uses `default-context` from the YAML to apply per-model overrides via `ApplyDefaults()`

## Reference Threads

See `THREADS.md` for important past conversations worth preserving.
