---
name: writing-go
description: Authoring or modifying Go source in any repo. Encodes Ardan house style, the post-edit toolchain (gofmt / vet / staticcheck / build), modern stdlib choices that should be preferred over recall-era idioms, and the gopls / go doc lookups required to verify any API before writing it. Load whenever the task involves reading, writing, or reviewing `.go` files.
---

# Writing Go

Your goal is to write Go that looks like the Go already in **this** repo.
Match the local house style. Prefer modern stdlib. Verify every API against
the live toolchain. Run the post-edit chain. Never suppress diagnostics.

## 0. Orient yourself in this repo (do this first, every session)

Before writing anything, discover what "this repo" actually does. These are
cheap commands — run them:

```bash
# Toolchain in use (do not assume a version).
go version
grep -E '^(go|toolchain) ' go.mod 2>/dev/null

# Module layout and package list.
go list ./...                          # all packages
go list -f '{{.Dir}}' ./... | head     # where they live on disk

# Repo conventions, if documented.
ls AGENTS.md CLAUDE.md CONTRIBUTING.md STYLE.md docs/ 2>/dev/null
```

Then pick **canonical exemplars from this repo** to imitate:

- A representative package with a constructor → look for `func New(` returning `(*T, error)`.
- A file that uses `context.Context` and the repo's logger → grep for `ctx context.Context`.
- A table-driven test → grep for `[]struct {` near `t.Run(`.
- The logging facade the repo uses (could be `log/slog`, `zap`, `logr`, an
  internal `applog` package, etc.) — find it before you write a log line.

```bash
rg -n 'func New\(' --type go | head
rg -n 'ctx context\.Context' --type go | head
rg -n 't\.Run\(' --type go | head
rg -n 'log/slog|go.uber.org/zap|github.com/go-logr/logr' --type go go.mod
```

Match what you find. The rules below are defaults; **the repo's existing
code wins** when it consistently does something else.

## 1. House style defaults

### Package & files

- One file per package owns the package doc comment:
  `// Package X provides support for ...` immediately above `package X`.
- If the repo uses a consistent section divider inside files
  (e.g. `// =====...=====`), reuse it. Do not invent a new one.
- No `init()` for setup. No package-level mutable state beyond small
  `var` defaults.

### Constructors

```go
// New constructs an X using default settings.
func New() (*X, error) {
    return NewWithOptions(Options{})
}

// NewWithOptions constructs an X with explicit options.
func NewWithOptions(opts Options) (*X, error) {
    if err := opts.validate(); err != nil {
        return nil, fmt.Errorf("validating options: %w", err)
    }

    x := X{
        opts: opts,
    }

    return &x, nil
}
```

- Constructor is named `New` (or `NewX` for a variant). Returns `(*T, error)`.
- Build a value, then return its address. **Do not** write
  `return &X{...}, nil`.
- Receiver name is short (1–3 chars) and **consistent** across every method
  on the type.

### Doc comments

- Every exported identifier has a doc comment. Full sentence. Starts with
  the identifier name. Present tense.
- `// Download pulls down a single file from a url to a specified destination.`
- Not `// downloads a file.` Not `// This function will download...`.

### Function signatures

- `ctx context.Context` is the first parameter. Always.
- The repo's logger (whatever it is) comes next when the function logs.
- Order after that: required inputs, then options/config.

### Errors

- Wrap with a short verb-phrase prefix, lowercased, no trailing period,
  `%w` for the cause:

  ```go
  return fmt.Errorf("creating directory: %w", err)
  ```

- Static errors → `errors.New("download: no network available")`.
- Combine multiple errors → `errors.Join(err1, err2)`. Do not concatenate
  with `fmt.Errorf` and `\n`.
- Inspect with `errors.Is` / `errors.As`. Never string-compare error text.
- Never silently swallow: no `_ = f()`, no empty `if err != nil {}`.

### Interfaces & types

- Return concrete types. Accept interfaces only where the boundary needs
  decoupling (e.g. `io.Reader`, the repo's logger interface).
- Small interfaces. Define them where they are consumed, not where they
  are implemented.
- Use type aliases (`type Logger = otherpkg.Logger`) for cross-package
  convenience, not redefinitions.
- `any`, never `interface{}`.

### Tests

- Same package (`package foo`) when testing internals;
  `package foo_test` when testing the public API.
- Table-driven with an inline anonymous struct slice. Field names are
  `name`, `input`, `want`, `wantErr` (or domain-appropriate).
- Failure messages follow `got X, want Y` format:
  ```go
  t.Errorf("Field: got %q, want %q", got.Field, want.Field)
  ```
- Use `t.Fatalf` only when later assertions cannot proceed.

## 2. Reach for the modern stdlib

Confirm the toolchain with `go version` first. Most of these apply on
Go 1.22+; iterators (`iter.Seq`) require 1.23+. If you have any doubt,
verify with `go doc`.

| Don't write                                            | Write instead                                                            |
| ------------------------------------------------------ | ------------------------------------------------------------------------ |
| `for i := 0; i < n; i++`                               | `for i := range n` (Go 1.22+)                                            |
| `for _, k := range sortedKeys(m) { ... }` (handrolled) | `for _, k := range slices.Sorted(maps.Keys(m))`                          |
| `sort.Slice(s, func(i, j int) bool { ... })`           | `slices.SortFunc(s, func(a, b T) int { ... })`                           |
| manual `contains` loop                                 | `slices.Contains(s, v)` / `slices.ContainsFunc`                          |
| manual `index` loop                                    | `slices.Index(s, v)` / `slices.IndexFunc`                                |
| `if a != "" { return a } ; return b`                   | `return cmp.Or(a, b)`                                                    |
| `if a > b { return a } ; return b`                     | `return max(a, b)` (builtin)                                             |
| concatenated multi-error strings                       | `errors.Join(errs...)`                                                   |
| `interface{}`                                          | `any`                                                                    |
| handrolled chunking                                    | `slices.Chunk(s, n)` (returns `iter.Seq`, 1.23+)                         |
| copying keys/values into a slice manually              | `slices.Collect(maps.Keys(m))` / `maps.Values(m)`                        |
| custom iteration callback API                          | return `iter.Seq[V]` / `iter.Seq2[K, V]` (1.23+) and let callers `range` |

For logging, use whatever facade the repo already uses (found in Section 0).
Do not introduce a new logger.

## 3. Verify before you write

The model you are running on cannot remember APIs accurately. Look them
up. Cheap commands, ground truth:

```bash
go version                              # confirm toolchain
go doc <pkg>.<Symbol>                   # signature + documentation
go doc -src <pkg>.<Symbol>              # implementation, for behavior
go doc <pkg>                            # full package surface
go list -m -versions <module>           # third-party version range
```

If `gopls` is available, use it after a write to ground your follow-up:

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
staticcheck ./<changed-pkg>/...         # if available; skip cleanly if not
go build ./...
```

Run tests scoped to the changed package(s), not the whole repo:

```bash
go test ./<changed-pkg>/...
```

If the repo's `AGENTS.md` / `CONTRIBUTING.md` specifies required env vars,
test directories to avoid, or extra checks, honor those instead of these
defaults.

If any tool reports a diagnostic, fix the code. Do not suppress.
