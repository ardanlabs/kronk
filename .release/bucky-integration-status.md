# Bucky / Whisper Integration — Status Snapshot

Tracks the rolling state of the bucky (whisper.cpp) integration into kronk.
This file is the durable reference both step-6 and step-7 handoff prompts
point back to. Update it as steps land.

GitHub issue: <https://github.com/ardanlabs/kronk/issues/591>

Working branch: `bill/bucky` of <https://github.com/ardanlabs/kronk>.

---

## Step status

| Step | Scope | State |
|------|-------|-------|
| 1 | sdk/bucky package skeleton | ✅ Done |
| 2 | model.Model + statePool + Transcribe + DetectLanguage | ✅ Done |
| 3 | Concurrency wrapper (sdk/bucky.Whisper, sem, ActiveStreams, Unload drain) | ✅ Done |
| 4 | Init / log policy + libs/models catalog wiring | ✅ Done |
| 5 | Tests under sdk/bucky/tests/ | ✅ Done |
| 6 | CLI (`cmd/kronk/...`) restructure for multi-backend | ✅ Done |
| 6a | Server wiring — `/v1/bucky/libs*` + `/v1/bucky/models*` (incl. `…/{model}/details`) | ✅ Done |
| 7 | BUI surfaces for libs + models | ✅ Done |
| 7m | Manual chapters (1, 2, 8, 9, 13) | ⬜ In progress |
| 8 | Transcription endpoint `POST /v1/bucky/transcribe` + BUI view | ❌ Not started |
| 9 | Cross-backend `/v1/bucky/models/ps` + unified ModelPs view | ❌ Not started |

---

## What steps 1–5 delivered (design canon)

### Package layout

```
sdk/bucky/
├── acquire.go           // Whisper.acquireModel / releaseModel (sem + ActiveStreams)
├── init.go              // bucky.Init: registers backend, loads libwhisper, sets log policy
├── init_test.go         // Backend-registration unit tests
├── lang.go              // Whisper.DetectLanguage + LangID / LangStr passthroughs
├── logger.go            // Logger / LogLevel / DiscardLogger / FmtLogger (aliases to sdk/applog)
├── transcribe.go        // Whisper.Transcribe
├── whisper.go           // Whisper struct, New, NewWithContext, ModelInfo, Unload
├── model/               // Low-level wrapper over github.com/ardanlabs/bucky/pkg/whisper
│   ├── config.go
│   ├── lang.go
│   ├── model.go         // Model + NewModel + Unload (owns one whisper.Context)
│   ├── pool.go          // statePool: NSeqMax whisper.State instances over one Context
│   └── transcribe.go
├── poolloader/          // sdk/pool loader.Loader[*bucky.Whisper] implementation
│   └── whisper.go
└── tests/               // Runtime-dependent test packages (mirrors sdk/kronk/tests/)
    ├── testlib/
    │   ├── helpers.go   // WithRetry, LoadSamples, AssertTranscriptContains
    │   └── testlib.go   // Setup(), WithWhisper, CfgTinyEn, MPTinyEn, AudioFile
    └── transcribe/
        ├── main_test.go // TestMain — Setup + skip if model missing
        ├── suite_test.go// TestSuite/{Transcribe,TranscribeOnSegment,DetectLanguage}
        └── pool_test.go // Test_PooledTranscribe (with real concurrency assertions),
                         // Test_ActiveStreams, Test_UnloadDrain, Test_UnloadTimeout
```

### Hard design rules

1. **`sdk/bucky/*` MUST NOT import `sdk/kronk/*`.** The shared logger
   lives at `sdk/applog`; `sdk/kronk/applog` is a thin alias-only shim.
2. **`Config.NSeqMax` = real parallelism.** Each pooled `whisper.State`
   owns its own mel + KV + compute buffer, so NSeqMax goroutines can run
   genuine concurrent transcribes against one shared `whisper.Context`.
   This mirrors `sdk/kronk/model.contextPool` for embed/rerank.
3. **Outer `Whisper.sem` is sized 1:1 with `NSeqMax`** (the embed/rerank
   rule, **not** the text-gen `NSeqMax * QueueDepth` rule). Whisper has
   no batch engine. The old `QueueDepth` knob has been dropped.
4. **`Whisper.Unload(ctx)` drains.** It blocks until `ActiveStreams`
   returns to 0 or the context fires; on timeout it returns an error
   with substring `"too many active-streams"` and leaves the handle
   live so a later Unload can drain cleanly.
5. **`Whisper.ActiveStreams() atomic.Int32`** is observable to callers
   for health endpoints + tests.
6. **`bucky.Init()`** (a) registers the whisper backend in
   `sdk/tools/backend` under `KindWhisper`, (b) sets `LD_LIBRARY_PATH`
   (or PATH on Windows) so transitive ggml libs resolve, (c) calls
   `whisper.Load(libPath)`, (d) installs `whisper.LogSet(LogSilent())`
   by default. `WithLogLevel(LogNormal)` opts back into stderr noise;
   `WithInitLibPath(p)` overrides the lib directory.

### Test conventions (canon)

- All runtime-dependent tests live under `sdk/bucky/tests/<pkg>/` with
  the same shape as `sdk/kronk/tests/<pkg>/`: a `main_test.go` that
  calls `testlib.Setup()` + skips when the model is missing, a
  `suite_test.go` that runs `TestSuite` via `testlib.WithWhisper`, and
  any extra `*_test.go` files for cases that need their own model
  lifecycle (e.g., `pool_test.go` for Unload).
- `testlib` mirrors `sdk/kronk/tests/testlib`: `Setup()`, `WithWhisper`,
  `WithRetry`, `Goroutines`, `RunInParallel`, `MaxRetries`, `TestDuration`.
- Audio samples come from `$GITHUB_WORKSPACE/examples/samples/jfk.wav`.
  **Never** add `sdk/bucky/testdata/`.
- `Test_PooledTranscribe` actively proves concurrency: (a) peak
  `ActiveStreams` must reach `numInstances` during the parallel batch,
  (b) parallel wall-clock must stay under `1.5×` a warmed single-shot
  baseline. A serialized pool fails both assertions.

### Required environment

| Var | Value |
|-----|-------|
| `RUN_IN_PARALLEL` | `yes` |
| `GITHUB_WORKSPACE` | `/Users/bill/code/go/src/github.com/ardanlabs/kronk` (or your repo root) |
| `GITHUB_ACTIONS` | `true` (CI only — collapses Goroutines to 1) |

Library + model prerequisites for running tests:

- whisper.cpp shared library installed under `sdk/tools/bucky/libs.Path("")`
  (default: `~/.kronk/bucky-libraries/<os>/<arch>/<processor>/`).
- `tiny.en` whisper model under the bucky catalog
  (default: `~/.kronk/bucky-models/ggml-tiny.en.bin`).

`examples/bucky/main.go` is the canonical install + use example.

---

## External dependency state — IMPORTANT

`sdk/bucky` depends on `github.com/ardanlabs/bucky` for the FFI
bindings. The two FFI additions made during steps 2/3 —
`pkg/whisper/logs.go` (`LogSet` / `LogSilent` / `LogNormal`) and
`pkg/whisper/lang.go` (`LangAutoDetectWithState`) — are **now on
`origin/main`** of bucky (verified against `bdd40bc`, May 2026).

The temporary local replace directive in kronk's `go.mod` can be
dropped at any time:

```
replace github.com/ardanlabs/bucky => /Users/bill/code/go/src/github.com/ardanlabs/bucky
```

To drop it:

```bash
go get github.com/ardanlabs/bucky@main
# then remove the replace line from go.mod (examples/go.mod has no
# bucky replace today).
```

**Check at the start of any new bucky thread:**

```bash
git -C /Users/bill/code/go/src/github.com/ardanlabs/bucky log --oneline -5 origin/main
git -C /Users/bill/code/go/src/github.com/ardanlabs/bucky status
```

---

## Post-edit chain (mandatory after any .go change)

```bash
gofmt -s -w <changed-files>
go vet ./sdk/bucky/...
staticcheck ./sdk/bucky/...
go build ./...            # from repo root
go build ./...            # from examples/
RUN_IN_PARALLEL=yes \
GITHUB_WORKSPACE=$(pwd) \
go test ./sdk/bucky/...
```

Never run repo-wide tests. Never run tests from `sdk/kronk/tests`.

---

## Step 6 — what changed (CLI restructure)

The `cmd/kronk/` tree is now multi-backend-first:

```
cmd/kronk/
├── main.go             ← mounts llama verbs at top level, bucky/* under "bucky"
├── client/             ← cross-backend (unchanged)
├── devices/            ← cross-backend (unchanged)
├── security/           ← cross-backend (unchanged)
├── server/             ← cross-backend (unchanged)
├── kronk/              ← llama backend (moved from old top-level dirs)
│   ├── catalog/
│   ├── libs/
│   ├── model/
│   └── run/
└── bucky/              ← NEW — whisper backend (local-only)
    ├── bucky.go        ← parent "bucky" cobra command
    ├── libs/           ← "kronk bucky libs" (install / list / remove)
    └── model/          ← "kronk bucky model"
        ├── catalog/    ← "kronk bucky model catalog"
        ├── list/       ← "kronk bucky model list"
        ├── pull/       ← "kronk bucky model pull <name|url>"
        └── remove/     ← "kronk bucky model remove <name>"
```

The CLI surface:

| Llama (top level, unchanged)         | Whisper (NEW)                              |
|--------------------------------------|--------------------------------------------|
| `kronk libs ...`                     | `kronk bucky libs ...`                     |
| `kronk model ...`                    | `kronk bucky model ...`                    |
| `kronk catalog ...`                  | (no bucky catalog — bundled list only via `bucky model catalog`) |
| `kronk run <model>`                  | (no bucky run — whisper has no chat)       |

Decisions worth noting:

- **No malina/ slot was created** — reserved for a later step.
- **No shared `internal/backendcli/` factory.** The verb surfaces differ
  enough (llama has web+local with server endpoints; bucky is local-only)
  that a shared factory would have hidden real differences. Each backend
  owns its own cobra files; the only shared dependency is the
  cross-backend `cmd/kronk/client` package (used by both `bucky model`
  verbs and the llama verbs to read `--base-path`).
- **`bucky` verbs are local-only.** The server has no whisper endpoints
  yet, so adding a `--local` flag would have implied a non-existent web
  mode. Help text says so explicitly.
- **`bucky libs` has no `--upgrade` flag.** `sdk/tools/bucky/libs` does
  not expose a "track latest" knob (its `Download` always uses
  `download.DefaultWhisperVersion` unless `WithVersion` overrides), so
  the flag would have been a no-op. `--version=<tag>` is supported.
- **`KRONK_BUCKY_LIB_PATH`** is the runtime override surfaced by the
  bucky `libs` help and `printUseHint`. It mirrors `KRONK_LIB_PATH` for
  llama and is consumed by `sdk/tools/bucky/libs.Path`.
- **Cobra `Use:` strings** mirror the existing llama names exactly
  (`libs`, `model`, etc., plural where llama uses plural).

Verification (last green run):

```
go build ./...                                          # OK
( cd examples && go build ./... )                       # OK
go vet ./... && staticcheck ./...                        # clean
RUN_IN_PARALLEL=yes GITHUB_WORKSPACE=$(pwd) \
  go test ./sdk/bucky/...                               # ok (cached)
go run ./cmd/kronk --help                                # lists "bucky"
go run ./cmd/kronk bucky libs --list-combinations        # prints 12 triples
go run ./cmd/kronk bucky libs --list-installs            # shows active install
go run ./cmd/kronk bucky model catalog                   # prints bundled catalog
go run ./cmd/kronk bucky model list                      # lists installed models
```

---

## What landed after step 6 (server + BUI)

### Server endpoints (already wired in `cmd/server/app/domain/toolapp/route.go`)

```
GET    /v1/bucky/libs                          listBuckyLibs
POST   /v1/bucky/libs/pull                     pullBuckyLibs            (streaming SSE)
GET    /v1/bucky/libs/combinations             listBuckyLibsCombinations
GET    /v1/bucky/libs/installs                 listBuckyLibsInstalls
DELETE /v1/bucky/libs/installs                 removeBuckyLibsInstall

GET    /v1/bucky/models                        listBuckyModels
GET    /v1/bucky/models/catalog                listBuckyCatalog
POST   /v1/bucky/models/pull                   pullBuckyModel           (streaming SSE)
GET    /v1/bucky/models/{model}/details        detailsBuckyModel        (parsed ggml header)
DELETE /v1/bucky/models/{model}                removeBuckyModel
```

`detailsBuckyModel` returns the parsed `Header` produced by
`sdk/tools/bucky/models/header.go` (ModelType, IsMultilingual,
QuantizationName, all `n_*` fields). The 3-tier lookup is:
per-id `.header_cache/<id>.hdr` → on-disk model file → HTTP Range
`bytes=0-47` against the catalog URL, with write-through caching.
`Download` and `Remove` keep the cache consistent automatically.

`bucky_libs.go` mirrors the llama libs surface but with no
"allow-upgrade" knob (bucky's `libs` package has no such toggle) and
no peer-download endpoint. A version override goes through
`DownloadFor(arch, os, processor, version)` against the active triple
when the caller does not supply a full triple.

### BUI surfaces

- `cmd/server/api/frontends/bui/src/components/BuckyLibs.tsx` — mirrors
  `LibsPull.tsx` minus the "Allow Upgrade" toggle and the Peer Bundle
  section. Uses `KRONK_BUCKY_LIB_PATH` and `~/.kronk/bucky-libraries`
  in activation hints.
- `cmd/server/api/frontends/bui/src/components/BuckyModels.tsx` —
  sortable table joining `/v1/bucky/models/catalog` with
  `/v1/bucky/models`. Columns: Name, Size, Notes, Installed, Status,
  Actions. Row click expands a `DetailsPanel` that lazily calls
  `getBuckyModelDetails(id)` and caches per-id in component state.
  Pull/Remove buttons stopPropagation so they don't toggle the panel.
- `cmd/server/api/frontends/bui/src/components/Layout.tsx` — restructured
  into top-level "Kronk" (Models / Catalog / Libs subcats) and "Bucky"
  (Models / Libs subcats). "Running" was moved out of kronk/Models to
  its own top-level category and currently points only at the existing
  `ModelPs` page (kronk-only until step 9 lands).
- Page header renames: ModelList → "GGUF Models", LibsPull → "Llama.cpp
  Libs", ModelPull → "HF Pull GGUF Model", KMSPull → "KMS Pull GGUF
  Model", BuckyLibs → "Whisper.cpp Libs".

### SDK additions

- `sdk/tools/bucky/models/header.go` — `Header` struct (magic `0x67676d6c`
  + 11 int32 fields), `ReadHeader(path)`, `FetchHeader(ctx, url)`,
  `(*Models).Header(id)`, `(*Models).CatalogHeader(ctx, id)`. Cache lives
  under `<modelsPath>/.header_cache/<id>.hdr`. `BuildIndex` already
  ignores it because it only walks top-level `.bin` files.
- `Header.ModelType()` / `IsMultilingual()` / `QuantizationName()`
  derive readable labels from the raw fields.
- `CatalogEntry` gained a `Notes` string field; all 11 bundled entries
  populated with lowercase notes (multilingual/english-only, fastest/
  fast/balanced/accurate, etc.). Plumbed through `BuckyCatalogEntry`
  JSON so the BUI Notes column renders.

Empirical timings (throwaway probe):

- `tiny.en` local read: ~530µs.
- `base` / `large-v3-turbo` first remote fetch: ~330–400ms.
- Second fetch: ~100µs (cache hit).
- HF returns `206 + 48 bytes` to a Range request; values match
  whisper.cpp `WHISPER_LOG_INFO` exactly.

---

## What's still TODO

- **Manual chapters 1 / 2 / 8 / 9 / 13** — document the whisper backend,
  the new CLI verbs, the SDK split (`sdk/kronk` vs `sdk/bucky`), the
  on-disk layout (`~/.kronk/bucky-libraries`, `~/.kronk/bucky-models`),
  the new BUI tabs, and a "coming soon" line in the API chapter for the
  transcription endpoint. See `.release/bucky-step-7-handoff.md`
  Section B.

- **Transcription endpoint.** `POST /v1/bucky/transcribe` — multipart
  audio (wav/mp3/flac), optional language hint, streams JSON segments.
  Backed by `sdk/bucky.Whisper` (loader, semaphore sized 1:1 with
  NSeqMax, `ActiveStreams()` observable). Once it lands, add a BUI
  transcription view (file/drop input, language dropdown driven by
  `bucky.LangStr(0..LangMaxID())`, transcript output with optional
  per-segment timestamps).

- **Cross-backend "running models" surface.** `Running` top-level menu
  exists but currently shows only `/v1/kronk/models/ps`. Add
  `/v1/bucky/models/ps` (or equivalent) that reports
  `Whisper.ActiveStreams()` per the design canon, then rewire
  `ModelPs.tsx` to merge both and tag each row with backend kind.

- **Peer download for bucky (deferred).** There is no
  `/v1/bucky/libs/peer-*` or `/v1/bucky/models/peer-*` endpoint, which
  is why `BuckyLibs.tsx` omits the peer section. If peer transfer is
  later wanted for bucky, mirror the kronk download endpoints.
