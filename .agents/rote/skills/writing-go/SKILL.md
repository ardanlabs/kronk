---
name: writing-go
description: Post-edit toolchain to run after writing or modifying any `.go` file. Load whenever a task changes Go source.
---

# Writing Go

After writing or editing any `.go` file, run these against the changed
package(s). All must pass; fix the code, do not suppress diagnostics.

```bash
gofmt -s -w <changed-files>
go vet ./<changed-pkg>/...
staticcheck ./<changed-pkg>/...   # if installed
go build ./...
go test ./<changed-pkg>/...
```

If the repo's `AGENTS.md` specifies required env vars, directories to
skip, or extra checks, follow those instead.
