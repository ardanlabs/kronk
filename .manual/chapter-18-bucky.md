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
- [18.9 Streaming Transcription (SDK)](#189-streaming-transcription-sdk)
  - [18.9.1 Event Model](#1891-event-model)
  - [18.9.2 Feeding Audio](#1892-feeding-audio)
  - [18.9.3 Stream Options](#1893-stream-options)
  - [18.9.4 Indefinite Sessions & Reset](#1894-indefinite-sessions-reset)
  - [18.9.5 Live Microphone Example](#1895-live-microphone-example)
- [18.10 Supported Languages](#1810-supported-languages)
- [18.11 Troubleshooting](#1811-troubleshooting)

---

This chapter is the user-facing operation guide for **Bucky**, the
audio transcription subsystem in Kronk. Bucky wraps
[`whisper.cpp`](https://github.com/ggerganov/whisper.cpp) (via the
`github.com/ardanlabs/bucky` FFI bindings) and exposes it through the
same SDK / server / CLI / BUI surfaces as the core LLM stack.

For developer-level internals (package layout, the per-handle
semaphore, the `whisper.State` pool, lifecycle, and tests) see the
_Bucky Internals_ section in
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

| Processor | Platforms                          | Notes                                             |
| --------- | ---------------------------------- | ------------------------------------------------- |
| `cpu`     | linux, darwin, windows (all archs) | Works everywhere. No GPU offload.                 |
| `metal`   | darwin (universal slice)           | Apple Silicon GPU offload via Metal.              |
| `cuda`    | linux, windows (amd64)             | NVIDIA GPU offload. Requires a CUDA-capable host. |
| `vulkan`  | linux (amd64)                      | Cross-platform GPU offload via Vulkan.            |

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
`.bin` suffix, so `ggml-ggml-tiny.bin.bin` ↔ `ggml-tiny.bin`.

#### 18.3.1 Bundled Catalog

| Short name       | Size   | Notes                                                   |
| ---------------- | ------ | ------------------------------------------------------- |
| `tiny`           | 75 MB  | multilingual, fastest, lowest accuracy                  |
| `ggml-tiny.bin`  | 75 MB  | english-only, fastest                                   |
| `base`           | 142 MB | multilingual, fast                                      |
| `base.en`        | 142 MB | english-only, fast                                      |
| `small`          | 466 MB | multilingual, balanced                                  |
| `small.en`       | 466 MB | english-only, balanced                                  |
| `medium`         | 1.5 GB | multilingual, accurate                                  |
| `medium.en`      | 1.5 GB | english-only, accurate                                  |
| `large-v3`       | 2.9 GB | multilingual, highest accuracy                          |
| `large-v3-turbo` | 1.5 GB | multilingual, near-large accuracy at small/medium speed |

The English-only (`.en`) variants are noticeably more accurate per
byte for English audio but reject any non-English language hint at
request time (see [§18.7.1](#1871-post-v1audiotranscriptions)).

#### 18.3.2 Pulling and Removing

```sh
# List the bundled catalog.
kronk bucky model catalog

# Download the tiny English model.
kronk bucky model pull ggml-tiny.bin

# List installed models with size and ggml header summary.
kronk bucky model list

# Remove a model.
kronk bucky model remove ggml-tiny.bin
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
    ├── ggml-ggml-tiny.bin.bin
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

| Setting        | Default | Source                               |
| -------------- | ------- | ------------------------------------ |
| `ModelsInPool` | `10`    | `sdk/bucky/pool.defaultModelsInPool` |
| `TTL`          | `5m`    | `sdk/bucky/pool.defaultTTL`          |
| `NSeqMax`      | `1`     | `sdk/bucky/model.Config`             |

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

| Field                       | Type   | Purpose                                                                      |
| --------------------------- | ------ | ---------------------------------------------------------------------------- |
| `file`                      | file   | Audio file (any format `bucky/pkg/audio` can decode to 16 kHz mono float32). |
| `model`                     | string | **Required.** Bucky model ID (short name, e.g. `ggml-tiny.bin`).             |
| `language`                  | string | BCP-47 / ISO 639-1 language hint. Empty → auto-detect.                       |
| `prompt`                    | string | Initial decoder bias prompt.                                                 |
| `translate`                 | bool   | When `true`, translate source audio to English.                              |
| `response_format`           | string | `json` (default), `verbose_json`, `text`, `srt`, `vtt`.                      |
| `timestamp_granularities[]` | string | `word` is accepted but currently emits an empty `words: []` array.           |

Behavior notes:

- The handler rejects requests against an English-only model
  (e.g. `ggml-tiny.bin`) when `language` is set to anything other than `""`
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
  -F model=ggml-tiny.bin \
  -F response_format=json
```

#### 18.7.2 Admin Endpoints

| Path                                    | Purpose                                      |
| --------------------------------------- | -------------------------------------------- |
| `GET  /v1/bucky/libs`                   | Current install + supported combinations.    |
| `POST /v1/bucky/libs/pull`              | Install / upgrade a library bundle.          |
| `GET  /v1/bucky/models`                 | List downloaded whisper models.              |
| `GET  /v1/bucky/models/catalog`         | List the bundled catalog.                    |
| `POST /v1/bucky/models/pull`            | Download a whisper model.                    |
| `GET  /v1/bucky/models/{model}/details` | ggml header + on-disk details for one model. |
| `DELETE /v1/bucky/models/{model}`       | Remove a model from disk.                    |

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
    mp, _ := mdls.Download(ctx, bucky.FmtLogger, "ggml-tiny.bin")

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

| Symbol                              | Purpose                                                                       |
| ----------------------------------- | ----------------------------------------------------------------------------- |
| `bucky.Init(opts...)`               | Register backend + load whisper.cpp shared library.                           |
| `bucky.New(opts...)`                | Construct a concurrently-safe `*Bucky` handle for one model.                  |
| `Bucky.Transcribe(...)`             | Transcribe 16 kHz mono float32 PCM (batch, one-shot).                         |
| `Bucky.TranscribeFile(...)`         | Decode an `io.Reader` (any supported format) and transcribe.                  |
| `Bucky.TranscribeChannels(...)`     | Channel-separated (diarized) transcribe: one speaker per channel.             |
| `Bucky.TranscribeChannelsFile(...)` | Decode an `io.Reader` preserving channels and diarize.                        |
| `Bucky.NewStream(...)`              | Open a live streaming session (see [18.9](#189-streaming-transcription-sdk)). |
| `Bucky.DetectLanguage(...)`         | Run language detection only.                                                  |
| `Bucky.ActiveStreams()`             | In-flight transcribe count (observability).                                   |
| `Bucky.SystemInfo()`                | Parsed `whisper.cpp` system info string.                                      |
| `Bucky.Unload(ctx)`                 | Wait for active streams to drain and unload the model.                        |
| `bucky.LangID/LangStr/LangMaxID`    | Language code ↔ id helpers.                                                   |

`Transcribe` accepts `model.TranscribeOption` values for per-call tuning:
`WithLanguage`, `WithTranslate`, `WithBeamSize`, `WithTranscribeNThreads`,
and the hallucination quality gates `WithNoSpeechThreshold` (no-speech
probability) / `WithLogProbThreshold` (average log-probability acceptance).
Both gates leave the whisper.cpp defaults (`0.6` and `-1.0`) in place
unless set; the streaming equivalents are in
[18.9.3](#1893-stream-options).

#### Channel-Separated Diarization

For recordings where each speaker is on a dedicated channel (call-center
and meeting captures often record one participant per channel),
`TranscribeChannels` / `TranscribeChannelsFile` transcribe every channel
separately and merge the results into a single diarized transcript. This
builds on the upstream `audio.SplitChannels` helper: native multi-channel
formats (WAV, FLAC) are de-interleaved and each channel resampled to 16
kHz, then transcribed on its own. Formats that require ffmpeg (WebM/Opus,
MP4/AAC, ...) are downmixed to a single channel, so they yield one
speaker.

```go
f, _ := os.Open("call.wav") // stereo: caller on L, agent on R
defer f.Close()

d, _ := b.TranscribeChannelsFile(ctx, f, model.WithLanguage("en"))

// d.Channels holds one Transcription per source channel.
for _, ct := range d.Channels {
    fmt.Printf("speaker %d: %s\n", ct.Channel, ct.Text)
}

// d.Segments merges every channel's segments sorted by start time,
// each tagged with the channel (speaker) it came from.
for _, s := range d.Segments {
    fmt.Printf("[%6dms] speaker %d: %s\n", s.StartMs, s.Channel, s.Text)
}
```

When you already hold decoded 16 kHz mono float32 channels (e.g. from
`model.DecodeChannels`), call `TranscribeChannels(ctx, channels, ...)`
directly.

### 18.9 Streaming Transcription (SDK)

`Bucky.Transcribe` is one-shot: hand it a full clip, get one result back.
For audio that arrives _over time_ — a microphone, a chunked HTTP upload,
a WebSocket, a long voice memo — use a **stream** instead. You `Feed`
samples as they arrive and consume incremental transcript **events** from
a channel; the stream owns the buffering, windowing, and silence detection
for you.

```go
ctx := context.Background()

stream, err := b.NewStream(ctx, model.WithStreamLanguage("en"))
if err != nil { /* ... */ }
defer stream.Close()

// Consumer: render partials live, commit on finals.
go func() {
    for ev := range stream.Events() {
        switch ev.Kind {
        case model.EventPartial: // tentative; REPLACE the live line
            fmt.Printf("\033[2K\r%s", ev.Text)
        case model.EventFinal:   // committed; APPEND and start a new line
            fmt.Printf("\033[2K\r%s\n", ev.Text)
        case model.EventError:
            fmt.Println("error:", ev.Err)
        }
    }
}()

// Producer: push 16 kHz mono float32 PCM as it arrives.
for samples := range incomingAudio {
    if err := stream.Feed(ctx, samples); err != nil { /* ... */ }
}

stream.Close() // final flush, then Events closes
```

A streaming session holds **one `whisper.State` from the pool for its
entire lifetime** and counts against `Bucky.ActiveStreams()`, so an open
stream blocks `Unload` exactly like an in-flight `Transcribe`. Call
`Close` exactly once when done; it is idempotent.

> **Architectural floor.** Whisper is a non-streaming model: its encoder
> works on fixed audio windows, so true token-by-token streaming is not
> possible. Like every Whisper "streaming" implementation (including
> whisper.cpp's own `examples/stream`), Bucky does _windowed_ inference
> under the hood and hides it behind this API. The practical consequences:
> the partial-update latency floor is `PartialEveryMs`, and a Final trails
> the actual end of speech by about one VAD frame.

#### 18.9.1 Event Model

Events mirror the "rolling hypothesis" UX of whisper.cpp's `stream`: the
text re-renders and revises as more audio arrives, then locks in when you
pause.

| Kind           | Meaning                                                                                                                                                                                            | Consumer action                          |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------- |
| `EventPartial` | Tentative, revisable re-decode of the current un-committed window. `Text` is the **full hypothesis** for that window (not a delta) and supersedes the previous partial. May be dropped under load. | **Replace** your pending-text buffer.    |
| `EventFinal`   | The un-committed region is committed and will not be revised. Never dropped.                                                                                                                       | **Append** `Text`; clear pending buffer. |
| `EventReset`   | Emitted after `Reset` (only when opened `WithEmitResetEvent`). `Text`/`Segments` empty.                                                                                                            | Clear any client-side accumulators.      |
| `EventError`   | Terminal. `Err` is set; `Events` closes immediately after.                                                                                                                                         | Stop; `Close` and (if needed) re-open.   |

Each `Event` also carries `StartMs` / `EndMs` (session-local; rebased to 0
after a `Reset` by default) and a `Segments` slice for callers that want
per-segment timing.

By default a Final commits when the speaker **pauses** (voice-activity
detection — see [18.9.3](#1893-stream-options)), so cuts land in natural
gaps instead of mid-word, and adjacent Finals do not duplicate the
boundary word. Across each commit the stream also rolls the previous
window's tail tokens forward as decoder context, preserving linguistic
continuity.

#### 18.9.2 Feeding Audio

The engine consumes one fixed internal format: **16 kHz, mono, float32 in
[-1, 1]**. Two feed methods are provided:

| Method    | Input                                            | Use when…                                                             |
| --------- | ------------------------------------------------ | --------------------------------------------------------------------- |
| `Feed`    | `[]float32` already at 16 kHz mono               | You already hold normalized samples (e.g. Web Audio, decoded files).  |
| `FeedPCM` | raw `[]byte` + an `model.AudioFormat` descriptor | You have raw capture-device PCM at an arbitrary rate / channel count. |

`FeedPCM` does the **pure-Go** interpret → downmix → resample → normalize
pipeline (no ffmpeg, no subprocess), carrying resampler phase across calls
so block boundaries that fall mid-frame introduce no discontinuity:

```go
format := model.AudioFormat{
    SampleRate: 48000,           // hardware rate; resampled to 16 kHz
    Channels:   2,               // downmixed to mono
    Sample:     model.Int16LE,   // or model.Float32LE
}
err := stream.FeedPCM(ctx, rawMicBytes, format)
```

Both methods are **producer-side** (call them from a single producer
goroutine) and apply backpressure: they block (respecting `ctx`) when the
internal buffer is full, so a fast producer cannot exhaust memory.

#### 18.9.3 Stream Options

Pass `model.StreamOption` values to `NewStream`. Every knob has a default;
all defaults match whisper.cpp `stream` conventions where applicable.

| Option                       | Default | Purpose                                                                                   |
| ---------------------------- | ------- | ----------------------------------------------------------------------------------------- |
| `WithStreamLanguage(code)`   | auto    | BCP-47 language hint; empty = auto-detect once at start.                                  |
| `WithStreamInitialPrompt(s)` | —       | Bias the decoder on the first window only.                                                |
| `WithStreamTranslate(bool)`  | false   | Translate source audio to English (multilingual models only).                             |
| `WithStreamNThreads(n)`      | model   | Override decode thread count for this session.                                            |
| `WithPartialEveryMs(ms)`     | 1000    | Partial-emit cadence. `<0` disables partials (final-only mode).                           |
| `WithCommitEveryMs(ms)`      | 6000    | Force a Final on a fixed cadence even without a pause (mirrors `stream`'s `n_new_line`).  |
| `WithMaxUtteranceMs(ms)`     | 25000   | Hard ceiling: force a Final if no silence is detected (stays under the 30 s mel window).  |
| `WithKeepMs(ms)`             | 300     | Trailing audio kept across a commit so a boundary word is not clipped.                    |
| `WithVAD(bool)`              | **on**  | Energy-ratio silence detection that gates Finals. Pass `false` for fixed-cadence commits. |
| `WithVADThreshold(f)`        | 0.6     | Trailing window is "silence" when its mean energy < `f` × the whole window's mean energy. |
| `WithPromptCarryover(bool)`  | **on**  | Seed each window's decode with the previous window's tail tokens (manual `condition_on_previous_text`). Pass `false` to decode every window independently. |
| `WithStreamNoSpeechThreshold(f)` | library (0.6) | Override whisper.cpp's no-speech probability gate for every decode in the session. `0` leaves the library default. |
| `WithStreamLogProbThreshold(f)`  | library (-1.0) | Override whisper.cpp's average log-probability acceptance gate for every decode in the session. Passing any value (including `0`) sets the override. |
| `WithEmitResetEvent(bool)`   | false   | Emit an `EventReset` after `Reset` completes.                                             |

**Prompt carryover** is **on by default**: at each commit the decoded
tail tokens are harvested and seeded into the next window's decode, so
the decoder keeps linguistic context across commits (the manual
equivalent of whisper.cpp's `condition_on_previous_text`). The trade-off
is that a strong prior — e.g. a committed question — can condition a
near-silent trailing window into a plausible but hallucinated
continuation. Pass `WithPromptCarryover(false)` to decode every window
independently and stop that. Like VAD, it is expressed by the negative
`StreamConfig.DisablePromptCarryover` field so the default-on behavior
needs no sentinel. `WithStreamInitialPrompt` is unaffected — it still
biases the first window regardless of carryover. When carryover is
disabled, `WithKeepPromptTokens` on a `Reset` becomes a no-op: the global
setting always wins.

**Quality gates** (`WithStreamNoSpeechThreshold` /
`WithStreamLogProbThreshold`) are a second lever against hallucinated
output on quiet audio. A lower no-speech threshold treats more borderline
segments as silence (raise it to be more permissive); a stricter (higher)
log-probability threshold rejects low-confidence decodes and retries them at
a higher temperature. The same
gates are available per call on the batch path via
`WithNoSpeechThreshold` / `WithLogProbThreshold`
([18.8](#188-sdk-quick-start)). Both leave the whisper.cpp defaults in
place unless set.

VAD is **on by default**: the detector is a pure-Go energy-ratio check
(the same approach as whisper.cpp's `vad_simple` — no model file, no extra
inference). It is expressed by the negative `StreamConfig.DisableVAD` field
(the `http.Transport.DisableKeepAlives` idiom) so the default-on behavior
needs no sentinel.

#### 18.9.4 Indefinite Sessions & Reset

A single `*Stream` is designed to run **indefinitely** — for hours, across
topic changes, mic mute/unmute, or "scratch that, start over". `Reset`
clears the audio buffer and rolling context **without** releasing the pool
slot, tearing down the worker, or closing `Events`, so you never re-pay
acquisition latency or lose GPU cache warmth for what is logically one
session. Resource use stays flat no matter how long a stream runs or how
often it is reset.

```go
// e.g. on a "new topic" / push-to-talk-release signal:
if err := stream.Reset(ctx); err != nil { /* ... */ }
```

`Reset` blocks until any in-flight decode finishes, then applies. Tune it
with `model.ResetOption`:

| Option                       | Default | Effect                                                                                                             |
| ---------------------------- | ------- | ------------------------------------------------------------------------------------------------------------------ |
| `WithFlushPending(bool)`     | true    | Run one final pass over buffered audio (emitting its Final) before clearing.                                       |
| `WithRebaseTimestamps(bool)` | true    | Restart `StartMs`/`EndMs` at 0 for subsequent events.                                                              |
| `WithKeepPromptTokens(bool)` | false   | Keep linguistic context across the reset ("rewind audio, keep context") instead of treating it as a hard boundary. |

`Reset` is safe to call from any goroutine (including the `Events`
consumer). After an `EventError` the worker has already exited, so don't
rely on `Reset` to recover — the stream is dead: `Close` it and open a new
one.

#### 18.9.5 Live Microphone Example

[`examples/bucky-stream/main.go`](../examples/bucky-stream/main.go)
(runnable with `make example-bucky-stream`) is a complete live-mic demo
that reproduces the whisper.cpp `stream` experience: it captures the
default microphone, renders partials in place and commits finals on their
own line, and ends when you **say "STOP"** (or press Ctrl-C).

```text
🎤 Mic is live — say something. Say "STOP" to end.
```

The **SDK itself is pure Go and needs no CGO.** The example adds CGO only
for microphone capture, via
[`github.com/gen2brain/malgo`](https://github.com/gen2brain/malgo)
(miniaudio), which lives entirely in the `examples` module — nothing in
`sdk/bucky` depends on it. The mic callback hands raw PCM to
`Stream.FeedPCM`; the rest is the `Feed → range Events → Close` pattern
above. (macOS prompts for microphone permission on first run.)

### 18.10 Supported Languages

`whisper.cpp` supports ~99 languages. Bucky exposes the full set
through `bucky.LangID` / `bucky.LangStr` / `bucky.LangMaxID`, and the
BUI Translator includes a shortlist of common ones plus an
**Auto-detect** option (`language=""`).

Pass the BCP-47 / ISO 639-1 short code (`en`, `de`, `fr`, …) in the
`language` form field or in `model.WithLanguage(...)`. Empty string
means auto-detect.

The **English-only** model variants (`ggml-tiny.bin`, `base.en`, `small.en`,
`medium.en`) reject any non-`en` language hint. Use the multilingual
variants for non-English audio.

### 18.11 Troubleshooting

| Symptom                                                             | Likely cause / fix                                                                                                                                    |
| ------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| Server logs `bucky init failed, running in degraded mode …`         | Whisper libraries not installed for the active triple. Run `kronk bucky libs` (or use the BUI), then restart the server.                              |
| `/v1/audio/transcriptions` returns `unknown model "<id>"`           | Model not pulled. Run `kronk bucky model pull <id>` (or use the BUI), then retry.                                                                     |
| `model[<id>] is english-only but language[<code>] was requested`    | You hit an `.en` model with a non-English `language` hint. Switch to a multilingual model (`tiny`, `base`, `small`, `medium`, `large-v3`).            |
| `transcribe: empty samples`                                         | The uploaded file decoded to zero samples — usually a corrupt file or a format `bucky/pkg/audio` cannot decode. Re-encode to 16 kHz mono WAV.         |
| `parse multipart form: …` with 413 / size errors                    | The upload exceeded 25 MB. Split the audio or down-sample to 16 kHz mono before upload.                                                               |
| GPU model loads but inference is suspiciously slow                  | Confirm the active bundle matches your hardware (`echo $KRONK_BUCKY_LIB_PATH`). A `cpu` bundle will silently work on a GPU host.                      |
| `unload: cannot unload, too many active-streams[n]`                 | A shutdown raced a long transcribe or an open stream. Increase the unload context deadline, or `Close` the stream / wait for in-flight requests.      |
| `NewStream` blocks or its context times out                         | Every pool slot is held by an open stream. A stream reserves one `whisper.State` for its whole lifetime; raise `NSeqMax` or `Close` an idle stream.   |
| Streaming emits no `EventPartial`, only finals                      | Partials are disabled (`WithPartialEveryMs` < 0), or the producer is feeding slower than `PartialEveryMs`. Feed steadily and use a positive cadence.  |
| A word is duplicated where two finals meet                          | VAD was turned off (`WithVAD(false)`), so a Final cut mid-word and the kept overlap re-decoded it. Leave VAD on (the default) so cuts land in pauses. |
| Whisper noise (`whisper_init_*`, `ggml_metal_*`) bleeds into stdout | Bucky installs `LogSilent` by default. If you forced `LogNormal` via `bucky.WithLogLevel(LogNormal)`, switch it back.                                 |

---

_Next: [Chapter 19: Developer Guide](chapter-19-developer-guide.md)_
