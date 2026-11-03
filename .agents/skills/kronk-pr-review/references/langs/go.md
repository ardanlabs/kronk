# Go idioms

**Tools:** `gopls` MCP (`go_symbol_references`, `go_search`, `go_file_context`), else `gopls`/`go doc`, then `rg`. Skills: `use-modern-go` on every `.go` file; `kronk-service-architecture` on Go under `cmd/`, `sdk/`, or `examples/` — `references/types.md` for HTTP and SDK shapes, `references/lifecycle.md` for the area the change touches, `references/branching.md` for conditionals.

**Checks** (`checks/summary.tsv`; they mirror the Linux CI `static` job, so a hit fails CI; package-wide tools report on whole packages, only in-scope hits count):
- `go-build`, `go-vet` (whole module, as CI runs it), `staticcheck` (`-checks=all`, as CI) — each hit is a finding at the Gate its effect warrants; style-only `ST*` checks are G1 unless CI would fail, then G2. `golangci-lint` runs only with a repo config (`--no-config` for an untrusted change: its config can load plugins).
- `checklocks` — a violation of an existing `+checklocks`/`+checkatomic` annotation is a data race (G4). "may require checklocks annotation for X" names the lock guarding those fields: verify in-scope code holds it on every access.
- `nilaway` — candidate nil dereferences; confirm the nil flow before reporting, else Open check.
- `go-fix`, `gofmt` (`-s`, per `AGENTS.md`), `go-mod-tidy` — CI fails on any diff: G2.
- `apidiff` — incompatible exported-API change: confirm every caller (including `examples/`) was updated; an unannounced break of a `sdk/*` package is G3 (Boundary Keeper).
- `errcheck` — a finding only when the lost error matters (failed write, commit, close of a written file); a dropped `Close` on a read-only body is G1 at most.
- `deadcode` — unreachable functions; an exported function of a library package may be public API.
- `gosec`, `govulncheck` — Security Sentinel (`govulncheck` also runs in CI).

## Correctness
`context` stored or not propagated, cancellation ignored; nil derefs; slice/map bounds; data races; leaked goroutines, streams, files, or native objects (yzma/llama.cpp, whisper, stable-diffusion handles); admission permits, active-use counts, and pool reservations released on every return path; slot/sequence state isolated between jobs; surprising zero values. Ownership follows Chapter 20 §20.2/§20.3: inference, caching, eviction, and validation policy stays in the SDK owner, not a handler or CLI command; cross-cutting concerns in `mid`/`observ`; shallow branching.

## Failure paths
Errors checked, wrapped with `%w`, matched with `errors.Is`/`errors.AsType`, not `_`-discarded when they matter; overwritten before use; zero value returned after failure; `defer` cleanup failures that matter; goroutines with no error path; native return codes and nil handles checked; SDK sentinels mapped in `errs.FromSDK` (a missing case surfaces as 500).

## Contracts
HTTP wire shapes (pass-through `*HTTP` methods in `sdk/kronk`, typed structs, capped multipart) stay on `encoding/json` v1; exported vs unexported fields, constructors and options (`New`/`NewWithContext` + `With*`), zero-value meaning, value vs pointer receivers; type sets (`Parse`/`MustParse`, full method set, initials receivers, in `*_types.go`); small consumer-owned interfaces; needless interfaces/generics; new internal type aliases; helper naming in `model.go`; sentinels and their `errs.FromSDK` mapping. Import direction: `sdk` never imports `cmd`; `foundation` never imports `app`/`sdk`; one `<x>app` never imports another (except `authapp`); `examples/` never imports `cmd` or `internal`.
- G4: wire-compatibility break, import-direction violation, constructible invalid state that reaches native code. Extra fields: `Contract: <name> (HTTP / SDK / config)`, `Crossing: <helper or none>`.

## Tests
Table-driven tests with named subtests; `t.Context()`; `errors.Is`/`errors.AsType` on error paths; negative and validation cases; boundaries; cancellation and timeouts (permit and slot released, stream closed); failed load or unload (reservation released or retained as specified); concurrent slots or acquisitions (isolation, coalescing); HTTP request/response through the domain handler including malformed input; `to*`/`from*` helpers; type-set `Parse` round-trips; parsers fed fragmented as well as whole input; Responses normalization in-place mutation. For a maintainer PR tests ran with `-race -short`: a `WARNING: DATA RACE` whose stack touches in-scope code is G4 — quote the two conflicting frames; prove a point with one `GITHUB_WORKSPACE=<root> go test -count=1 -run TestX ./pkg`. Otherwise `go test` didn't run.

## Security
`gosec`, `govulncheck` rows; trace taint from request/env/file to sink with `gopls`; path traversal out of the model store, `exec.Command` with a shell, `html/template` vs `text/template`, `InsecureSkipVerify`, `math/rand` for tokens; user content logged without insecure-logging mode.

## Clarity
Exported identifiers documented, starting with the identifier's name (they become the BUI SDK docs); type-set docs state the zero value; comments on lifecycle code state who owns release. Branch chains the diff adds or rewrites → default-first assignment or a naked switch (`branching.md`; an existing `else if` chain is never a finding). Don't flatten `to*`/`from*` helpers, type sets, `mid` middleware, or ownership boundaries (handler → pool → handle → engine); don't merge the generation and sequence-batch engines or always-rebuild the Responses `model.D` (Chapter 20 forbids both).
