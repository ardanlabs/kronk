---
name: writing-go
description: Authoring or modifying Go source. Grounds API decisions in the live toolchain (`go doc`, `gopls`, `go list`, `go version`) instead of training data, and runs the project-mandated `gofmt`/`go vet`/`staticcheck`/`go fix` chain after edits. Load whenever the task involves reading, writing, or reviewing `.go` files.
---

# Writing Go

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

| Operation                            | Tool                                                                                            |
| ------------------------------------ | ----------------------------------------------------------------------------------------------- |
| Modify an existing `.go` file        | Host's native edit/replace **first**; fall back to Kronk MCP `fuzzy_edit` on exact-match failure |
| Create a brand-new `.go` file        | Host's native create-file tool (`create_file` / `Write`) — `fuzzy_edit` cannot create files     |
| Read a file                          | Host's native `Read`                                                                            |
| Run `go doc`, `gopls`, `go build`, `go test`, `rg`, etc. | Host `Bash` directly                                                                            |

Create-file tool requires both `path`/`filePath` **and** `content`
(full file contents) on the first call. Calling without `content`
loops on schema errors.

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
