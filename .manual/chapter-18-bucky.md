# Chapter 18: Bucky (Audio Transcription)

## Table of Contents

- [18.1 Overview](#181-overview)
- [18.2 Installation & Libraries](#182-installation-libraries)
  - [18.2.1 Library Bundles](#1821-library-bundles)
  - [18.2.2 Installing via the CLI](#1822-installing-via-the-cli)
  - [18.2.3 Installing via the BUI](#1823-installing-via-the-bui)
  - [18.2.4 Environment Variables](#1824-environment-variables)
- [18.3 Model Catalog & Pull](#183-model-catalog-pull)
  - [18.3.1 Bundled Catalog](#1831-bundled-catalog)
  - [18.3.2 Pulling and Removing](#1832-pulling-and-removing)
  - [18.3.3 On-Disk Layout](#1833-on-disk-layout)
- [18.4 Server & Pool Configuration](#184-server-pool-configuration)
- [18.5 CLI Commands](#185-cli-commands)
- [18.6 BUI Usage](#186-bui-usage)
- [18.7 API Endpoint](#187-api-endpoint)
  - [18.7.1 `POST /v1/audio/transcriptions`](#1871-post-v1audiotranscriptions)
  - [18.7.2 Admin Endpoints](#1872-admin-endpoints)
- [18.8 SDK Quick Start](#188-sdk-quick-start)
- [18.9 Supported Languages](#189-supported-languages)
- [18.10 Troubleshooting](#1810-troubleshooting)

---

This chapter is the user-facing operation guide for **Bucky**, the
audio transcription subsystem in Kronk. Bucky wraps
[`whisper.cpp`](https://github.com/ggerganov/whisper.cpp) (via the
`github.com/ardanlabs/bucky` FFI bindings) and exposes it through the
same SDK / server / CLI / BUI surfaces as the core LLM stack.

For developer-level internals (package layout, the per-handle
semaphore, the `whisper.State` pool, lifecycle, and tests) see the
*Bucky Internals* section in
[Chapter 19: Developer Guide](chapter-19-developer-guide.md).

### 18.1 Overview

Bucky is a peer of the llama (kronk) backend. It is a separate
backend kind in the cross-backend registry
(`backend.KindWhisper`) and ships its own:

- SDK package — `sdk/bucky` (high-level handle) and
  `sdk/bucky/model` (low-level model + transcribe primitives).
- Tools — `sdk/tools/bucky/libs` (shared-library installer) and
  `sdk/tools/bucky/models` (whisper GGML model catalog).
- Pool — `sdk/bucky/pool`, sharing the unified `resman.Manager`
  with the llama pool so VRAM / RAM accounting is one budget across
  the whole host.
- CLI — the `kronk bucky …` sub-command tree.
- HTTP — the OpenAI-compatible `/v1/audio/transcriptions` endpoint
  plus `/v1/bucky/libs/*` and `/v1/bucky/models/*` admin endpoints.
- BUI — the **Translator** component, plus library and model
  management screens.

```diagram
╭──────────────╮  multipart   ╭──────────────╮   acquire   ╭───────────────╮
│  Client /    │ ───────────▶ │  audioapp    │ ──────────▶ │  bucky.Pool   │
│  BUI / curl  │              │  handler     │             │  (resman'd)   │
╰──────────────╯              ╰──────┬───────╯             ╰───────┬───────╯
                                     │ audio.Decode                │
                                     ▼                             ▼
                              ╭──────────────╮              ╭───────────────╮
                              │  float32 PCM │              │ *bucky.Bucky  │
                              │  16 kHz mono │              │ + model.Model │
                              ╰──────┬───────╯              ╰───────┬───────╯
                                     │                              │ Transcribe
                                     ╰─────────────────────────────▶│ (per-handle
                                                                    │  semaphore)
                                                                    ▼
                                                             ╭───────────────╮
                                                             │ whisper.cpp   │
                                                             │ (FFI)         │
                                                             ╰───────────────╯
```

A request flows: client → multipart upload → `audioapp.transcriptions`
handler → `audio.Decode` to 16 kHz mono float32 PCM →
`pool.Bucky.AquireModel` → `Bucky.Transcribe` → whisper.cpp →
formatted response (`json`, `verbose_json`, `text`, `srt`, or `vtt`).

The whisper context is single-stream, so concurrency comes from
NSeqMax-sized `whisper.State` pools per model handle and a per-handle
semaphore in front of them. Multiple models share the host through
the unified `resman`.

### 18.2 Installation & Libraries

Bucky uses **prebuilt** whisper.cpp shared libraries, downloaded into
the bucky libraries root. The default root is
`~/.kronk/bucky-libraries/` and the active install is selected by
`KRONK_BUCKY_LIB_PATH` (falls back to the default platform triple if
unset).

The whisper backend is registered with the cross-backend registry
even when the shared library is missing, so the server can boot in
**degraded mode** — the BUI / CLI can still download libraries and
the server will become functional once `bucky.Init` succeeds.

#### 18.2.1 Library Bundles

| Processor | Platforms                          | Notes                                                     |
| --------- | ---------------------------------- | --------------------------------------------------------- |
| `cpu`     | linux, darwin, windows (all archs) | Works everywhere. No GPU offload.                         |
| `metal`   | darwin (universal slice)           | Apple Silicon GPU offload via Metal.                      |
| `cuda`    | linux, windows (amd64)             | NVIDIA GPU offload. Requires a CUDA-capable host.         |
| `vulkan`  | linux (amd64)                      | Cross-platform GPU offload via Vulkan.                    |

#### 18.2.2 Installing via the CLI

```sh
# Install the default whisper.cpp libraries for the current host.
kronk bucky libs

# Install a specific whisper.cpp version.
kronk bucky libs --version=v1.7.0

# List supported (arch, os, processor) combinations.
kronk bucky libs --list-combinations

# Install a Linux/CUDA bundle alongside the active install.
kronk bucky libs --install --arch=amd64 --os=linux --processor=cuda

# List installed library bundles.
kronk bucky libs --list-installs

# Remove an install.
kronk bucky libs --remove-install --arch=amd64 --os=linux --processor=cuda
```

Every `bucky libs` verb honors `--local` to bypass the model server
and download directly. The default web mode talks to the server's
`/v1/bucky/libs/*` endpoints.

To switch between installed bundles point `KRONK_BUCKY_LIB_PATH` at
the bundle directory and restart the server:

```sh
export KRONK_BUCKY_LIB_PATH=~/.kronk/bucky-libraries/linux/amd64/cuda
```

#### 18.2.3 Installing via the BUI

The BUI's **Whisper Libraries** screen exposes the same operations:
list combinations, install / remove a triple, and view the currently
active bundle. After installing a bundle, restart the server (or wait
for the auto-init retry) so the bucky backend can load the shared
library.

#### 18.2.4 Environment Variables

| Variable               | Purpose                                                        |
| ---------------------- | -------------------------------------------------------------- |
| `KRONK_BUCKY_LIB_PATH` | Whisper library directory the server loads at startup.         |
| `KRONK_ARCH`           | Architecture override for CLI install ops: `amd64`, `arm64`.   |
| `KRONK_OS`             | OS override for CLI install ops: `linux`, `darwin`, `windows`. |
| `KRONK_PROCESSOR`      | Processor override: `cpu`, `metal`, `cuda`, `vulkan`.          |

### 18.3 Model Catalog & Pull

Whisper models are single GGML `.bin` files stored flat under the
bucky models root (default `~/.kronk/bucky-models/`). On-disk
filenames follow the upstream HuggingFace mirror convention:
`ggml-<name>.bin`. The short name strips the `ggml-` prefix and
`.bin` suffix, so `ggml-tiny.en.bin` ↔ `tiny.en`.

#### 18.3.1 Bundled Catalog

| Short name        | Size     | Notes                                                       |
| ----------------- | -------- | ----------------------------------------------------------- |
| `tiny`            | 75 MB    | multilingual, fastest, lowest accuracy                      |
| `tiny.en`         | 75 MB    | english-only, fastest                                       |
| `base`            | 142 MB   | multilingual, fast                                          |
| `base.en`         | 142 MB   | english-only, fast                                          |
| `small`           | 466 MB   | multilingual, balanced                                      |
| `small.en`        | 466 MB   | english-only, balanced                                      |
| `medium`          | 1.5 GB   | multilingual, accurate                                      |
| `medium.en`       | 1.5 GB   | english-only, accurate                                      |
| `large-v3`        | 2.9 GB   | multilingual, highest accuracy                              |
| `large-v3-turbo` | 1.5 GB   | multilingual, near-large accuracy at small/medium speed     |

The English-only (`.en`) variants are noticeably more accurate per
byte for English audio but reject any non-English language hint at
request time (see [§18.7.1](#1871-post-v1audiotranscriptions)).

#### 18.3.2 Pulling and Removing

```sh
# List the bundled catalog.
kronk bucky model catalog

# Download the tiny English model.
kronk bucky model pull tiny.en

# List installed models with size and ggml header summary.
kronk bucky model list

# Remove a model.
kronk bucky model remove tiny.en
```

`pull` accepts a short name, a full ggml filename (`ggml-tiny.bin`),
or a bare basename without extension. `--local` bypasses the model
server.

#### 18.3.3 On-Disk Layout

```diagram
~/.kronk/
├── bucky-libraries/
│   ├── darwin/arm64/metal/        ← active on Apple Silicon
│   ├── linux/amd64/cuda/          ← installed alongside (selected via KRONK_BUCKY_LIB_PATH)
│   └── linux/amd64/cpu/
└── bucky-models/
    ├── ggml-tiny.en.bin
    ├── ggml-base.bin
    └── ggml-large-v3-turbo.bin
```

### 18.4 Server & Pool Configuration

There is no per-model config file for whisper — Bucky discovers every
`.bin` under the models root, parses its ggml header, and serves it
under its short-name ID. The server-side wiring lives in
`cmd/server/api/services/kronk/main.go`:

1. `buckylibs.New(...)` resolves the active library bundle.
2. `buckymodels.NewWithPaths(...)` builds the on-disk index.
3. `bucky.Init(bucky.WithInitLibPath(...))` loads the whisper.cpp
   shared library. On failure the server logs a warning and runs
   in **degraded mode** — `/v1/bucky/libs/*` and `/v1/bucky/models/*`
   stay live so libraries can be downloaded, but
   `/v1/audio/transcriptions` will fail until a successful re-init.
4. The bucky pool is constructed with the **shared** `resman.Manager`
   so its memory reservations contend with the llama pool's.

Per-pool defaults:

| Setting        | Default     | Source                          |
| -------------- | ----------- | ------------------------------- |
| `ModelsInPool` | `10`        | `sdk/bucky/pool.defaultModelsInPool` |
| `TTL`          | `5m`        | `sdk/bucky/pool.defaultTTL`     |
| `NSeqMax`      | `1`         | `sdk/bucky/model.Config`        |

The pool's per-handle semaphore is sized **1:1** with `NSeqMax`,
matching the embedding / rerank rule in `sdk/kronk` (not the
text-generation `NSeqMax * QueueDepth` rule), because whisper has no
batch engine and each transcribe owns one `whisper.State` from
acquire to release.

`Pool.ModelStatus()` returns both **loaded** entries (from the engine
cache) and **loading** entries (in-flight reservations the engine
holds a ticket for in `resman`) so the BUI can show "loading…" for a
cold model.

### 18.5 CLI Commands

The `kronk bucky` tree mirrors the top-level llama verbs but targets
whisper. There is no `bucky run` because whisper has no chat /
generation surface.

```
kronk bucky
├── libs                              # install / upgrade whisper.cpp libraries
└── model
    ├── catalog                       # list the bundled catalog
    ├── list                          # list installed models
    ├── pull   <name|filename|url>    # download a model
    └── remove <name>                 # remove a model from disk
```

Every verb takes `--local` to bypass the model server.

### 18.6 BUI Usage

The BUI surfaces three Bucky-related screens:

1. **Whisper Libraries** — list / install / remove library bundles
   (same operations as `kronk bucky libs`).
2. **Whisper Models** — browse the bundled catalog, pull, list, and
   remove local models, view ggml header details.
3. **Translator** — the user-facing transcription workbench. Upload
   or record audio, pick a model, pick a language (or auto-detect),
   choose response format, and view the transcript (text and per-
   segment timestamps).

The Translator panel uses the `/v1/audio/transcriptions` endpoint
behind the scenes and exposes the same fields that endpoint accepts.

### 18.7 API Endpoint

#### 18.7.1 `POST /v1/audio/transcriptions`

OpenAI-compatible. `multipart/form-data` upload, **25 MB** max body.

| Field                       | Type      | Purpose                                                                       |
| --------------------------- | --------- | ----------------------------------------------------------------------------- |
| `file`                      | file      | Audio file (any format `bucky/pkg/audio` can decode to 16 kHz mono float32).  |
| `model`                     | string    | **Required.** Bucky model ID (short name, e.g. `tiny.en`).                    |
| `language`                  | string    | BCP-47 / ISO 639-1 language hint. Empty → auto-detect.                        |
| `prompt`                    | string    | Initial decoder bias prompt.                                                  |
| `translate`                 | bool      | When `true`, translate source audio to English.                               |
| `response_format`           | string    | `json` (default), `verbose_json`, `text`, `srt`, `vtt`.                       |
| `timestamp_granularities[]` | string    | `word` is accepted but currently emits an empty `words: []` array.            |

Behavior notes:

- The handler rejects requests against an English-only model
  (e.g. `tiny.en`) when `language` is set to anything other than `""`
  or `"en"`.
- The handler caps each request at a 30-minute internal deadline.
- `verbose_json` includes `segments[]` with `start`, `end`, `text`,
  and `no_speech_prob`. Word-level timestamps are not yet plumbed
  through from whisper.cpp, so the `words: []` field is intentionally
  empty when `timestamp_granularities[]=word` is requested.

Example:

```sh
curl -X POST http://localhost:8080/v1/audio/transcriptions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -F file=@samples/jfk.wav \
  -F model=tiny.en \
  -F response_format=json
```

#### 18.7.2 Admin Endpoints

| Path                                   | Purpose                                               |
| -------------------------------------- | ----------------------------------------------------- |
| `GET  /v1/bucky/libs`                  | Current install + supported combinations.             |
| `POST /v1/bucky/libs/pull`             | Install / upgrade a library bundle.                   |
| `GET  /v1/bucky/models`                | List downloaded whisper models.                       |
| `GET  /v1/bucky/models/catalog`        | List the bundled catalog.                             |
| `POST /v1/bucky/models/pull`           | Download a whisper model.                             |
| `GET  /v1/bucky/models/{model}/details`| ggml header + on-disk details for one model.          |
| `DELETE /v1/bucky/models/{model}`      | Remove a model from disk.                             |

These are the endpoints the BUI screens and the `--web` mode of the
`kronk bucky` CLI talk to.

### 18.8 SDK Quick Start

A minimal Go program. The fully worked example is in
[`examples/bucky/main.go`](../examples/bucky/main.go), runnable with
`make example-bucky`.

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/ardanlabs/bucky/pkg/audio"
    "github.com/ardanlabs/kronk/sdk/bucky"
    "github.com/ardanlabs/kronk/sdk/bucky/model"
    buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
    buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

func main() {
    ctx := context.Background()

    // 1. Make sure the whisper.cpp shared libs and a model are present.
    lib, _ := buckylibs.New()
    lib.Download(ctx, bucky.FmtLogger)

    mdls, _ := buckymodels.New()
    mp, _ := mdls.Download(ctx, bucky.FmtLogger, "tiny.en")

    // 2. Initialize the whisper backend (loads the shared library).
    if err := bucky.Init(); err != nil {
        fmt.Fprintln(os.Stderr, err); os.Exit(1)
    }

    // 3. Construct a handle for one model.
    b, _ := bucky.New(
        model.WithModelPath(mp.ModelFiles[0]),
        model.WithUseGPU(true),
    )
    defer b.Unload(ctx)

    // 4. Decode audio to 16 kHz mono float32 PCM and transcribe.
    f, _ := os.Open("samples/jfk.wav")
    defer f.Close()
    samples, _ := audio.Decode(f)

    tr, _ := b.Transcribe(ctx, samples, model.WithLanguage("en"))
    fmt.Println(tr.Text)
}
```

Key SDK entry points:

| Symbol                     | Purpose                                                       |
| -------------------------- | ------------------------------------------------------------- |
| `bucky.Init(opts...)`      | Register backend + load whisper.cpp shared library.           |
| `bucky.New(opts...)`       | Construct a concurrently-safe `*Bucky` handle for one model.  |
| `Bucky.Transcribe(...)`    | Transcribe 16 kHz mono float32 PCM.                           |
| `Bucky.DetectLanguage(...)`| Run language detection only.                                  |
| `Bucky.ActiveStreams()`    | In-flight transcribe count (observability).                   |
| `Bucky.SystemInfo()`       | Parsed `whisper.cpp` system info string.                      |
| `Bucky.Unload(ctx)`        | Wait for active streams to drain and unload the model.        |
| `bucky.LangID/LangStr/LangMaxID` | Language code ↔ id helpers.                             |

### 18.9 Supported Languages

`whisper.cpp` supports ~99 languages. Bucky exposes the full set
through `bucky.LangID` / `bucky.LangStr` / `bucky.LangMaxID`, and the
BUI Translator includes a shortlist of common ones plus an
**Auto-detect** option (`language=""`).

Pass the BCP-47 / ISO 639-1 short code (`en`, `de`, `fr`, …) in the
`language` form field or in `model.WithLanguage(...)`. Empty string
means auto-detect.

The **English-only** model variants (`tiny.en`, `base.en`, `small.en`,
`medium.en`) reject any non-`en` language hint. Use the multilingual
variants for non-English audio.

### 18.10 Troubleshooting

| Symptom                                                                 | Likely cause / fix                                                                                                                                |
| ----------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| Server logs `bucky init failed, running in degraded mode …`             | Whisper libraries not installed for the active triple. Run `kronk bucky libs` (or use the BUI), then restart the server.                          |
| `/v1/audio/transcriptions` returns `unknown model "<id>"`               | Model not pulled. Run `kronk bucky model pull <id>` (or use the BUI), then retry.                                                                 |
| `model[<id>] is english-only but language[<code>] was requested`         | You hit an `.en` model with a non-English `language` hint. Switch to a multilingual model (`tiny`, `base`, `small`, `medium`, `large-v3`).        |
| `transcribe: empty samples`                                             | The uploaded file decoded to zero samples — usually a corrupt file or a format `bucky/pkg/audio` cannot decode. Re-encode to 16 kHz mono WAV.     |
| `parse multipart form: …` with 413 / size errors                        | The upload exceeded 25 MB. Split the audio or down-sample to 16 kHz mono before upload.                                                           |
| GPU model loads but inference is suspiciously slow                      | Confirm the active bundle matches your hardware (`echo $KRONK_BUCKY_LIB_PATH`). A `cpu` bundle will silently work on a GPU host.                  |
| `unload: cannot unload, too many active-streams[n]`                     | A shutdown raced a long transcribe. Increase the unload context deadline, or wait for in-flight requests to finish.                               |
| Whisper noise (`whisper_init_*`, `ggml_metal_*`) bleeds into stdout     | Bucky installs `LogSilent` by default. If you forced `LogNormal` via `bucky.WithLogLevel(LogNormal)`, switch it back.                             |

---

*Next: [Chapter 19: Developer Guide](chapter-19-developer-guide.md)*
