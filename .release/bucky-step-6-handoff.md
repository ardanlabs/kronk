# Bucky Step 6 Handoff — CLI Restructure

Paste this into a new Amp thread to start step 6.

---

## Background

Read `.release/bucky-integration-status.md` first — it captures the
design canon established in steps 1–5 (sdk layout, NSeqMax = real
parallelism, sem sizing, Init / log policy, test infrastructure under
`sdk/bucky/tests/`).

GitHub issue: <https://github.com/ardanlabs/kronk/issues/591>

Working branch: `bill/bucky` of <https://github.com/ardanlabs/kronk>.

---

## Goal

Restructure the `cmd/kronk/` tree so the CLI is multi-backend-first.
Today every verb under `cmd/kronk/{libs,model,catalog,run}` is
implicitly llama-only. After step 6, llama lives in a dedicated
`cmd/kronk/kronk/` subtree, whisper lives under `cmd/kronk/bucky/`,
and a future `cmd/kronk/malina/` slot is reserved.

The CLI surface end users see:

| Old (still works)        | New (added)                            |
|--------------------------|----------------------------------------|
| `kronk lib pull ...`     | `kronk bucky lib pull ...`             |
| `kronk model pull ...`   | `kronk bucky model pull ...`           |
| `kronk model ls`         | `kronk bucky model ls`                 |
| `kronk catalog ...`      | (whisper catalog under `kronk bucky catalog` if applicable) |
| `kronk run ...`          | (whisper has no `run`; see below)      |

`kronk lib pull` continues to work for llama because `main.go` mounts
the kronk-backend verb tree at the top level. Bucky verbs mount under
a `bucky` parent command. Malina later does the same under `malina`.

---

## Hard constraints

1. **CLI / BUI have no backwards-compatibility promise** — feel free
   to rename, move, or remove subcommands inside the `cmd/kronk/`
   tree. The only invariant is that the top-level llama verbs
   (`kronk lib`, `kronk model`, `kronk catalog`, `kronk run`) keep
   working, because that's what existing users type.

2. **`sdk/` is OUT OF SCOPE — DO NOT TOUCH.** The asymmetry under
   `sdk/tools/` (llama at `sdk/tools/{libs,models}`, whisper at
   `sdk/tools/bucky/{libs,models}`) is a real wart but it's an SDK
   concern and SDK packages DO have a backwards-compatibility promise.
   Step 6 only restructures `cmd/kronk/`. The new CLI code imports
   the existing SDK paths unchanged.

3. **Server is OUT of scope.** Unifying the server to host all
   backends concurrently (one HTTP port, dispatch by model kind) is a
   later step — the last step in the integration sequence. Do not
   touch `cmd/server/` or any of its services.

4. **Transcription endpoint is OUT of scope.** Assume it exists; the
   later server step adds it. Don't add a `kronk bucky transcribe`
   CLI verb that needs an HTTP client to a non-existent endpoint —
   either skip the verb entirely for now, or have it call the bucky
   SDK directly (load model + Transcribe in-process, mirroring what
   `examples/bucky/main.go` does). Pick whichever is cleaner.

5. **Don't touch any test under `sdk/bucky/tests/`** beyond fixing
   import paths if the CLI move forces it (it shouldn't).

---

## Folder layout (target)

```
cmd/kronk/
├── main.go             ← mounts kronk/* at top level, bucky/* under "bucky"
├── internal/
│   └── backendcli/     ← shared cobra factory for verb trees (see below)
├── client/             ← cross-backend (existing — leave alone)
├── devices/            ← cross-backend (existing — leave alone)
├── security/           ← cross-backend (existing — leave alone)
├── server/             ← cross-backend (existing — leave alone)
├── kronk/              ← llama backend (moved from current top-level dirs)
│   ├── catalog/
│   ├── libs/
│   ├── model/
│   └── run/
├── bucky/              ← NEW — whisper backend
│   ├── lib/
│   ├── model/
│   └── (transcribe, if you decide to add it locally — see #4 above)
└── malina/             ← reserved; do not create yet
```

Verify the actual `cmd/kronk/` tree before starting — `ls cmd/kronk/`
and confirm the existing subdirs match what's listed above. If
anything extra is there (e.g., new helpers added since this prompt
was written), preserve them.

---

## CLI shape (target)

- `kronk lib pull ...` → llama (default backend; same as today)
- `kronk model pull ...` → llama
- `kronk catalog ...` → llama
- `kronk run ...` → llama
- `kronk bucky lib pull ...` → whisper
- `kronk bucky model pull ...` → whisper
- (future) `kronk malina lib pull ...` → malina

Backend-specific help text is honest: `kronk lib pull --help` talks
about GGUF / llama.cpp libraries; `kronk bucky lib pull --help` talks
about whisper.cpp libraries.

Backend-specific flag value spaces don't collide because each subtree
owns its own flag set.

---

## Implementation sketch

### Shared factory

Create `cmd/kronk/internal/backendcli/backendcli.go` exporting a
`BackendCommands(kind backend.Kind, ...) *cobra.Command` factory (or
a small set of verb-specific factories — pick what's least clever).
The factory produces a uniform verb tree (`lib`, `model`, etc.) for
any backend kind by consuming the existing `sdk/tools/backend.Backend`
record (which already carries `NewLibs` and `NewCatalog` factories).

If the verb implementations differ enough between backends that a
single factory is too clever, drop it and write per-backend verb
files that each call a thin shared helper for the parts they
genuinely share (download progress reporting, output formatting,
etc.). Don't force symmetry that hides real differences.

### main.go mounting

```
rootCmd.AddCommand(kronk.LibsCmd)      // → "kronk lib"
rootCmd.AddCommand(kronk.ModelCmd)     // → "kronk model"
rootCmd.AddCommand(kronk.CatalogCmd)   // → "kronk catalog"
rootCmd.AddCommand(kronk.RunCmd)       // → "kronk run"

buckyCmd := &cobra.Command{Use: "bucky", Short: "whisper backend"}
buckyCmd.AddCommand(bucky.LibCmd)      // → "kronk bucky lib"
buckyCmd.AddCommand(bucky.ModelCmd)    // → "kronk bucky model"
rootCmd.AddCommand(buckyCmd)

// cross-backend verbs continue to mount at top level
rootCmd.AddCommand(server.Cmd)
rootCmd.AddCommand(client.Cmd)
rootCmd.AddCommand(security.Cmd)
rootCmd.AddCommand(devices.Cmd)
```

(Adjust to whatever the existing main.go pattern actually is — read
it before editing.)

### Backend-specific subcommand naming

You'll notice the table uses `bucky lib` (singular) while the
existing top-level uses `lib` (singular too — the dir is `libs/` but
the cobra `Use:` is presumably `lib`; verify). Whatever the existing
llama subcommand names are, mirror them under `bucky` exactly so
users can predict the verbs.

### Cobra `Use:` strings

Update `Use:` and `Short:` for each new subcommand so help text
reads naturally:

- `kronk bucky lib pull metal-arm64` — `Short` mentions whisper.cpp
- `kronk bucky model pull tiny.en` — `Short` mentions whisper models
- `kronk bucky --help` — `Short` describes the whisper backend

---

## Verification

After the restructure:

```bash
# Build cleanly from both modules.
go build ./...                  # repo root
( cd examples && go build ./... )

# Existing tests still pass.
RUN_IN_PARALLEL=yes GITHUB_WORKSPACE=$(pwd) go test ./sdk/bucky/...

# Smoke-test the CLI.
go run ./cmd/kronk --help              # lists "bucky" alongside top-level verbs
go run ./cmd/kronk lib --help          # llama lib help (unchanged content)
go run ./cmd/kronk bucky --help        # whisper backend help
go run ./cmd/kronk bucky lib --help    # whisper lib help

# Existing llama commands still work (don't actually pull, just dry-run).
go run ./cmd/kronk lib --help
go run ./cmd/kronk model --help

# New whisper commands invoke the bucky catalog / libs APIs.
go run ./cmd/kronk bucky lib --help
go run ./cmd/kronk bucky model --help
```

If any verb under the new `cmd/kronk/kronk/` subtree exports a
function or variable that was previously top-level (e.g., another
binary imports `cmd/kronk/libs.Cmd`), grep for those imports and
update them too. The `cmd/` tree generally doesn't get imported by
SDK code, but verify.

---

## Process constraints

- Load the `writing-go` skill before touching any `.go`.
- Run the post-edit chain after every `.go` change:
  ```
  gofmt -s -w <changed-files>
  go vet ./...
  staticcheck ./...
  go build ./...                # repo root
  ( cd examples && go build ./... )
  ```
- Never run repo-wide tests. Never run tests from `sdk/kronk/tests/`.
- Required env for any test command:
  ```
  export RUN_IN_PARALLEL=yes
  export GITHUB_WORKSPACE=<repo root>
  ```

---

## External dependency state

`sdk/bucky` depends on FFI additions in `github.com/ardanlabs/bucky`
(`LangAutoDetectWithState`, `LogSet`, `LogSilent`, `LogNormal`) that
may still live on Bill's local working tree of bucky rather than
`origin/main`. The `replace` directives in `go.mod` and
`examples/go.mod` reflect that. Check at the start:

```bash
git -C /Users/bill/code/go/src/github.com/ardanlabs/bucky log --oneline -5 origin/main
git -C /Users/bill/code/go/src/github.com/ardanlabs/bucky status
```

If those symbols are now on `origin/main`, run
`go get github.com/ardanlabs/bucky@main` (and the same from the
`examples/` module) and remove the `replace` lines from both go.mod
files. If not, leave the replaces alone.

Step 6 should not need to touch this — but verify the build is green
before you start so any failure is yours and not a stale-dep issue.

---

## When you're done

1. Run the full verification block above.
2. Update `.release/bucky-integration-status.md`: flip step 6 to
   ✅ Done and add a short "what changed" note describing the new
   `cmd/kronk/kronk/` vs `cmd/kronk/bucky/` layout.
3. Update `.release/bucky-step-7-handoff.md` if step 6 made any
   decisions that change the BUI surface (e.g., new CLI verbs the
   docs panels should describe).
