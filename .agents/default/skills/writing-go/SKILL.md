---
name: writing-go
description: Authoring or modifying Go source in this repo. Encodes Ardan house style, the post-edit toolchain (gofmt / vet / staticcheck / build), modern stdlib choices that should be preferred over recall-era idioms, and the gopls / go doc lookups required to verify any API before writing it. Load whenever the task involves reading, writing, or reviewing `.go` files.
---

# Writing Go

Your goal is to write Go that looks like the Go already in this repo. Match
the house style. Prefer modern stdlib. Verify every API against the live
toolchain. Run the post-edit chain. Never suppress diagnostics.

## 1. House style (match what's already in this repo)

Read these files when in doubt — they are the canonical exemplars:

- Constructors, receivers, package layout → `sdk/tools/models/models.go`
- `ctx` / `log` ordering, error wrapping, `context.Context` plumbing → `sdk/tools/models/download.go`
- Small leaf package, type aliasing, function types → `sdk/kronk/applog/applog.go`
- Table-driven tests → `sdk/tools/models/info_test.go`
- Top-level package function with progress callback → `sdk/tools/downloader/downloader.go`

The rules below are derived from those files. Follow them.

### Package & files

- One file per package owns the package doc comment:
  `// Package X provides support for ...` immediately above `package X`.
- Use the section divider `// =============================================================================`
  to separate logical groupings inside a file. Do not invent other dividers.
- No `init()`. No package-level mutable state beyond small `var` defaults
  (e.g. `var localFolder = "models"`).

### Constructors

```go
// New constructs the models system using default paths.
func New() (*Models, error) {
    return NewWithPaths("")
}

// NewWithPaths constructs the models system. If basePath is empty, the
// default location is used.
func NewWithPaths(basePath string) (*Models, error) {
    basePath = defaults.BaseDir(basePath)

    if err := os.MkdirAll(modelPath, 0755); err != nil {
        return nil, fmt.Errorf("creating models directory: %w", err)
    }

    m := Models{
        basePath:   basePath,
        modelsPath: modelPath,
    }

    return &m, nil
}
```

- Constructor is named `New` (or `NewX` for a variant). Returns `(*T, error)`.
- Build a value, then return its address. **Do not** write
  `return &Models{...}, nil`.
- Receiver name is short (1–3 chars) and **consistent** across every method
  on the type. `Models` → `m`. `ProgressReader` → `pr`.

### Doc comments

- Every exported identifier has a doc comment. Full sentence. Starts with
  the identifier name. Present tense.
- `// Download pulls down a single file from a url to a specified destination.`
- Not `// downloads a file.` Not `// This function will download...`.

### Function signatures

- `ctx context.Context` is the first parameter. Always.
- `log applog.Logger` is the second parameter when the function logs.
- Order after that: required inputs, then options/config.

```go
func (m *Models) Download(ctx context.Context, log applog.Logger, modelSource string) (Path, error)
```

### Errors

- Wrap with a short verb-phrase prefix, lowercased, no trailing period,
  `%w` for the cause:

  ```go
  return fmt.Errorf("creating models directory: %w", err)
  return fmt.Errorf("download-urls: no model URLs provided")
  ```

- Static errors → `errors.New("download: no network available")`.
- Combine multiple errors → `errors.Join(err1, err2)`. Do not concatenate
  with `fmt.Errorf` and `\n`.
- Inspect with `errors.Is` / `errors.As`. Never string-compare error text.
- Never silently swallow: no `_ = f()`, no empty `if err != nil {}`.

### Interfaces & types

- Return concrete types. Accept interfaces only where the boundary needs
  decoupling (e.g. `applog.Logger`, `io.Reader`).
- Small interfaces. Define them where they are consumed, not where they
  are implemented.
- For cross-package convenience aliases: `type Logger = applog.Logger`
  (alias, not redefinition).
- `any`, never `interface{}`.

### Tests

- Same package (`package models`) when testing internals;
  `package models_test` when testing the public API.
- Table-driven with an inline anonymous struct slice. Field names are
  `name`, `input`, `want`, `wantErr` (or domain-appropriate).
- Failure messages follow `got X, want Y` format:
  ```go
  t.Errorf("Desc: got %q, want %q", info.Desc, expDesc)
  ```
- Use `t.Fatalf` only when later assertions cannot proceed.

## 2. Reach for the modern stdlib (Go 1.22+ / live toolchain is 1.26)

If you find yourself writing one of the patterns on the left, stop and
use the form on the right. Verify the symbol with `go doc` if you have
any doubt.

| Don't write                                            | Write instead                                 |
| ------------------------------------------------------ | --------------------------------------------- |
| `for i := 0; i < n; i++`                               | `for i := range n`                            |
| `for _, k := range sortedKeys(m) { ... }` (handrolled) | `for _, k := range slices.Sorted(maps.Keys(m))` |
| `sort.Slice(s, func(i, j int) bool { ... })`          | `slices.SortFunc(s, func(a, b T) int { ... })` |
| manual `contains` loop                                 | `slices.Contains(s, v)` / `slices.ContainsFunc` |
| manual `index` loop                                    | `slices.Index(s, v)` / `slices.IndexFunc`     |
| `if a != "" { return a } ; return b`                   | `return cmp.Or(a, b)`                         |
| `if a > b { return a } ; return b`                     | `return max(a, b)` (builtin)                  |
| concatenated multi-error strings                       | `errors.Join(errs...)`                        |
| `interface{}`                                          | `any`                                         |
| handrolled chunking                                    | `slices.Chunk(s, n)` (returns `iter.Seq`)     |
| copying keys/values into a slice manually              | `slices.Collect(maps.Keys(m))` / `maps.Values(m)` |
| custom iteration callback API                          | return `iter.Seq[V]` / `iter.Seq2[K, V]` and let callers `range` over it |

For logging in this repo, use `applog.Logger` (the function-type alias).
Do **not** import `log/slog` directly inside SDK packages — go through
`applog`.

## 3. Verify before you write

The model you are running on cannot remember APIs accurately. Look them
up. Cheap commands, ground truth:

```bash
go version                              # confirm toolchain (expect 1.26.x)
go doc <pkg>.<Symbol>                   # signature + documentation
go doc -src <pkg>.<Symbol>              # implementation, for behavior
go doc <pkg>                            # full package surface
go list -m -versions <module>           # third-party version range
```

LSP (gopls) is wired in. Use it after a write to ground your follow-up:

```bash
gopls definition <file>:<line>:<col>
gopls references <file>:<line>:<col>
gopls check      <file>                 # diagnostics gopls would surface
```

Rule: **if `go doc <pkg>.<Symbol>` returns nothing, the symbol does not
exist. Do not write it.**

## 4. Anti-patterns — do not write these

- `init()` for setup. Use explicit construction.
- `panic(...)` for normal error paths. Return an error.
- Naked returns in non-trivial functions.
- `_ = f()` to silence an error. Handle it or document why.
- `fmt.Errorf("...: %v", err)` for wrapping. Use `%w`.
- `time.Sleep` inside tests to wait for state. Synchronize properly.
- Generic helper packages named `utils`, `common`, `helpers`, `misc`.
- `//nolint`, `//gocyclo:ignore`, etc. Fix the underlying issue.
- Package-level mutable globals that aren't compile-time constants.

## 5. Post-edit chain (mandatory after any `.go` change)

Run these, in order, scoped to the changed package(s). All must pass.

```bash
gofmt -s -w <changed-files>
go vet ./<changed-pkg>/...
staticcheck ./<changed-pkg>/...
go build ./...
```

Scoped tests (never repo-wide, never from `sdk/kronk/tests`):

```bash
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE=<repo root>
go test ./<changed-pkg>/...
```

If any tool reports a diagnostic, fix the code. Do not suppress.
