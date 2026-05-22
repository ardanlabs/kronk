---
name: writing-go
description: Apply modern Go syntax guidelines based on project's Go version. Use when user ask for modern Go code guidelines.
---

# Modern Go Guidelines

## Detected Go Version

!`grep -rh "^go " --include="go.mod" . 2>/dev/null | cut -d' ' -f2 | sort | uniq -c | sort -nr | head -1 | xargs | cut -d' ' -f2 | grep . || echo unknown`

## How to Use This Skill

DO NOT search for go.mod files or try to detect the version yourself. Use ONLY the version shown above.

**If version detected (not "unknown"):**

- Say: "This project is using Go X.XX, so I’ll stick to modern Go best practices and freely use language features up to and including this version. If you’d prefer a different target version, just let me know."
- Do NOT list features, do NOT ask for confirmation

**If version is "unknown":**

- Say: "Could not detect Go version in this repository"
- Use AskUserQuestion: "Which Go version should I target?" → [1.23] / [1.24] / [1.25] / [1.26]

**When writing Go code**, use ALL features from this document up to the target version:

- Prefer modern built-ins and packages (`slices`, `maps`, `cmp`) over legacy patterns
- Never use features from newer Go versions than the target
- Never use outdated patterns when a modern alternative is available

---

## Validating Go Code

After writing or editing any `.go` file, run these against the changed
package(s). All must pass; fix the code, do not suppress diagnostics.

```bash
gofmt -s -w <changed-files>
go vet ./<changed-pkg>/...
staticcheck ./<changed-pkg>/...   # if installed
go build ./...
go test ./<changed-pkg>/...
go fix ./...
```

If the repo's `AGENTS.md` specifies required env vars, directories to
skip, or extra checks, follow those instead.

---

## Features by Go Version

### Go 1.24+

- `t.Context()` not `context.WithCancel(context.Background())` in tests.
  ALWAYS use t.Context() when a test function needs a context.

Before:

```go
func TestFoo(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    result := doSomething(ctx)
}
```

After:

```go
func TestFoo(t *testing.T) {
    ctx := t.Context()
    result := doSomething(ctx)
}
```

- `omitzero` not `omitempty` in JSON struct tags.
  ALWAYS use omitzero for time.Duration, time.Time, structs, slices, maps.

Before:

```go
type Config struct {
    Timeout time.Duration `json:"timeout,omitempty"` // doesn't work for Duration!
}
```

After:

```go
type Config struct {
    Timeout time.Duration `json:"timeout,omitzero"`
}
```

- `b.Loop()` not `for i := 0; i < b.N; i++` in benchmarks.
  ALWAYS use b.Loop() for the main loop in benchmark functions.

Before:

```go
func BenchmarkFoo(b *testing.B) {
    for i := 0; i < b.N; i++ {
        doWork()
    }
}
```

After:

```go
func BenchmarkFoo(b *testing.B) {
    for b.Loop() {
        doWork()
    }
}
```

- `strings.SplitSeq` not `strings.Split` when iterating.
  ALWAYS use SplitSeq/FieldsSeq when iterating over split results in a for-range loop.

Before:

```go
for _, part := range strings.Split(s, ",") {
    process(part)
}
```

After:

```go
for part := range strings.SplitSeq(s, ",") {
    process(part)
}
```

Also: `strings.FieldsSeq`, `bytes.SplitSeq`, `bytes.FieldsSeq`.

### Go 1.25+

- `wg.Go(fn)` not `wg.Add(1)` + `go func() { defer wg.Done(); ... }()`.
  ALWAYS use wg.Go() when spawning goroutines with sync.WaitGroup.

Before:

```go
var wg sync.WaitGroup
for _, item := range items {
    wg.Add(1)
    go func() {
        defer wg.Done()
        process(item)
    }()
}
wg.Wait()
```

After:

```go
var wg sync.WaitGroup
for _, item := range items {
    wg.Go(func() {
        process(item)
    })
}
wg.Wait()
```

### Go 1.26+

- `new(val)` not `x := val; &x` — returns pointer to any value.
  Go 1.26 extends new() to accept expressions, not just types.
  Type is inferred: new(0) → *int, new("s") → *string, new(T{}) → \*T.
  DO NOT use `x := val; &x` pattern — always use new(val) directly.
  DO NOT use redundant casts like new(int(0)) — just write new(0).
  Common use case: struct fields with pointer types.

Before:

```go
timeout := 30
debug := true
cfg := Config{
    Timeout: &timeout,
    Debug:   &debug,
}
```

After:

```go
cfg := Config{
    Timeout: new(30),   // *int
    Debug:   new(true), // *bool
}
```

- `errors.AsType[T](err)` not `errors.As(err, &target)`.
  ALWAYS use errors.AsType when checking if error matches a specific type.

Before:

```go
var pathErr *os.PathError
if errors.As(err, &pathErr) {
    handle(pathErr)
}
```

After:

```go
if pathErr, ok := errors.AsType[*os.PathError](err); ok {
    handle(pathErr)
}
```
