# AGENTS.md

## Build & Test Commands
- Install CLI: `go install ./cmd/kronk`
- Run all tests: `make test` (requires `make install-libraries install-models` first)
- Single test: `go test -v -count=1 -run TestName ./sdk/kronk/...`
- Build server: `make kronk-server`
- Tidy modules: `go mod tidy`
- Lint: `staticcheck ./...`

## Architecture
- **cmd/kronk/** - CLI tool for managing models, server, security
- **cmd/server/** - OpenAI-compatible model server (gRPC + HTTP) with BUI frontend
- **sdk/kronk/** - Core API: model loading, chat, embeddings, cache, metrics
- **sdk/security/** - JWT auth, OPA authorization, key management
- **sdk/tools/** - Library/model download utilities
- Uses **yzma** (llama.cpp Go bindings) for local inference with GGUF models

## Code Style
- Package comments: `// Package <name> provides...`
- Errors: use `fmt.Errorf("context: %w", err)` with lowercase prefix
- Declare package-level sentinel errors as `var ErrXxx = errors.New(...)`
- Structs: unexported fields, exported types; use `Config` pattern for constructors
- No CGO in tests: `CGO_ENABLED=0 go test ...`
- Imports: stdlib first, then external, then internal (goimports order)
