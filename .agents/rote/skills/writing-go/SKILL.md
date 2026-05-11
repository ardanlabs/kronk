---
name: writing-go
description: Authoring or modifying Go source under the rote-routed config. Grounds API decisions in the live toolchain (`go doc`, `gopls`, `go list`, `go version`) instead of training data, runs the project-mandated `gofmt`/`go vet`/`staticcheck`/`go fix` chain after edits, and routes the `fuzzy_edit` fallback through the `kronk-mcp` skill. Load whenever the task involves reading, writing, or reviewing `.go` files.
---

# Writing Go (rote config)

## Hard rules

- **Never** invent or recall a stdlib / third-party API. Verify with
  `go doc` against the active Go version (and the `go.mod`-pinned
  version for third-party).
- **Never** assume a language feature exists. Confirm with
  `go version`. Go 1.26 is in scope; older toolchains may be active.
- **Always** run the post-edit chain (below) after every `.go` change.
- **Never** run a full repo test sweep, and never `go test` from
  `sdk/kronk/tests`.
- **Never** suppress a real diagnostic (`//nolint`, `_ = ...`) to
  make the chain green. Fix the underlying issue.
- **`fuzzy_edit` is a fallback**, not a primary edit tool. Use the
  host's native edit tool first; only when an exact-string-match edit
  fails, route the same change through `fuzzy_edit` per `kronk-mcp`.

## Verify before you write

```bash
go version                              # what's actually compiling?
go doc <pkg>.<Symbol>                   # signature + doc
go doc -src <pkg>.<Symbol>              # behavior questions → source
go doc <pkg>                            # package surface
go list -m -versions <module>           # available module versions
gopls definition  <file>:<line>:<col>   # type/symbol shape
gopls references  <file>:<line>:<col>
```

For "is feature X in Go 1.26?" — `go doc <pkg>.<NewSymbol>`. If
absent, it isn't available. Don't speculate.

## Editing mechanics

| Operation                            | Tool                                                                                                        |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------------- |
| Modify an existing `.go` file        | Host's native edit/replace **first**; on exact-match failure, fall back to `fuzzy_edit` via rote (`kronk-mcp`) |
| Create a brand-new `.go` file        | Host's native create-file tool (`create_file` / `Write`) — `fuzzy_edit` cannot create files                 |
| Read a file                          | Host's native `Read`                                                                                        |
| Run `go doc`, `gopls`, `go build`, `go test`, `rg`, etc. | Host `Bash` directly                                                                                        |

Create-file tool requires both `path`/`filePath` **and** `content`
(full file contents) on the first call. Calling without `content`
loops on schema errors.

When falling back to `fuzzy_edit`, load `skill({ name: "kronk-mcp" })`
and follow its workflow — do not invoke rote yourself.

## Post-edit chain (mandatory after any `.go` change)

```bash
gofmt -s -w <changed-files>
go vet ./<changed-pkg>/...
staticcheck ./<changed-pkg>/...
go fix ./<changed-pkg>/...
go build ./...
```

Tests (scoped, never repo-wide):

```bash
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE=<repo root>
go test ./<changed-pkg>/...
```
