# Modern Go - Details

Longer notes the guidelines point to. Read only the section you are sent to.

- [go fix](#go-fix) - mechanical modernization of existing code
- [uuid](#uuid) - standard library `uuid` package (Go 1.27+)
- [json/v2](#jsonv2) - `encoding/json/v2` (Go 1.27+)

---

## go fix

Applies most guidelines mechanically, for example:

- range-over-int
- `min` / `max`
- `any`
- `strings.Cut`
- `maps.*`
- `errors.AsType`
- `wg.Go`
- `new(expr)`

### Requirements

- An installed Go 1.26+ toolchain (check with `go version`).
- Run it on the target packages **before** reading their files.

### Steps

Run one command at a time and stop on the first failure. `./p` is each changed package by exact path; never `./...` or `./p/...`, which can reach suites the repo forbids or that need external resources:

```
go fix -diff ./p       # 1. preview
go fix ./p             # 2. apply
go fix ./p             # 3. apply again: one fix can enable another
gofmt -s -w <files>    # 4. every Go file go fix touched
go vet ./p             # 5. verify it builds and vets
staticcheck ./p        # 6. if installed
go test -count=1 ./p   # 7. verify tests pass
```

The repo's `AGENTS.md` wins over this list: apply its required environment variables, skip the test directories it forbids, and never run a full repository test unless it says to.

Then read the fixed files and apply the remaining guidelines by hand.

### Limits

- **Version-aware:** only applies what go.mod's `go` line / `//go:build go1.X` permits.
- **Generated files:** skipped; fix the generator instead.
- **Build configs:** analyzes one per run; re-run per GOOS/GOARCH for tagged code.
- **Syntactic only:** compile errors afterwards (e.g. unused variables) are expected; fix them by hand.
- **Libraries:** never migrates json/v2, uuid or `httptest.NewTestServer`. Do those by hand; they can change behavior.

### Fixers

- `go tool fix help` lists them.
- `-name=false` disables one (e.g. `-minmax=false`).
- Go 1.27 changes:
  - adds `atomictypes`, `embedlit`, `slicesbackward`, `unsafefuncs`
  - drops `fmtappendf`
  - renames `waitgroup` -> `waitgroupgo`

### Committing

Commit `go fix` output separately from manual changes.

---

## uuid

Standard library package, Go 1.27+.

### API

| Function | Notes |
|---|---|
| `uuid.New()` | v4 (random) |
| `uuid.NewV7()` | time-ordered; prefer for DB primary keys |
| `uuid.Parse`, `uuid.MustParse` | parse from string |
| `uuid.Nil()` | the zero UUID |
| `u.Compare(v)` | ordering (method) |

- Comparable with `==`.
- Implements `TextMarshaler` / `TextUnmarshaler`.

### Databases

- `database/sql` `Scan` accepts `*uuid.UUID` (from a string or a 16-byte source).
- There is **no** `driver.Valuer`:
  - pgx v5 accepts it as a query argument (it is a `[16]byte`).
  - Other drivers: verify, or pass `id.String()`.

---

## json/v2

Packages: `encoding/json/v2` and `encoding/json/jsontext`. Go 1.27+.

### API

- `json.Marshal(v, opts...)`
- `json.MarshalWrite(w, v)`
- `json.UnmarshalRead(r, &v)`

### Migrating from v1

v1 is now backed by v2 and stays supported. Migrating is optional and **changes wire output**:

| Behavior | v1 | v2 |
|---|---|---|
| nil slice / map | `null` | `[]` / `{}` - drop nil guards that exist only for JSON |
| HTML characters | escaped (`<` -> `\u003c`) | not escaped - pass `jsontext.EscapeForHTML(true)` to keep v1 bytes |
| Field name matching | case-insensitive | case-sensitive |
| Duplicate names, invalid UTF-8 | accepted | rejected |
| `time.Duration` | integer nanoseconds | no default (error) - convert explicitly |
| `omitempty` | omits `false`, `0`, `""`, nil, empty slice / map | omits values that encode as `null`, `""`, `{}`, `[]` - prefer `omitzero` |
