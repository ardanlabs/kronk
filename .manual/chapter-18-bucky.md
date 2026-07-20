# Chapter 18: Bucky (Audio Transcription)

## Table of Contents

- [18.1 Overview](#181-overview)
- [18.2 Install Whisper Libraries](#182-install-whisper-libraries)
- [18.3 Manage Models](#183-manage-models)
- [18.4 Server Configuration](#184-server-configuration)
- [18.5 Browser UI](#185-browser-ui)
- [18.6 Transcriptions API](#186-transcriptions-api)
  - [18.6.1 Request and Response](#1861-request-and-response)
  - [18.6.2 Bucky Management Endpoints](#1862-bucky-management-endpoints)
- [18.7 Go SDK](#187-go-sdk)
  - [18.7.1 Batch Transcription](#1871-batch-transcription)
  - [18.7.2 Channel-Separated Diarization](#1872-channel-separated-diarization)
  - [18.7.3 Streaming Transcription](#1873-streaming-transcription)
- [18.8 Languages](#188-languages)
- [18.9 Troubleshooting](#189-troubleshooting)

---

Bucky is Kronk's speech-to-text subsystem. It uses
[`whisper.cpp`](https://github.com/ggerganov/whisper.cpp) and is available
through:

- the `/v1/audio/transcriptions` HTTP endpoint;
- the Browser UI (BUI) Translator;
- the `kronk bucky` management commands; and
- the Go packages under `sdk/bucky`.

Using Bucky requires both a compatible whisper.cpp library bundle and a
Whisper model. The libraries run the inference engine; the model contains the
speech-recognition weights.

Developer-level package, lifecycle, and test information belongs in
[Chapter 19: Developer Guide](chapter-19-developer-guide.md).

### 18.1 Overview

Bucky supports these common workflows:

- transcribe an uploaded or recorded audio file;
- translate speech from a supported language into English;
- return plain text, JSON, SRT, or WebVTT;
- transcribe separate audio channels as separate speakers through the SDK; and
- consume partial and final transcript events from live audio through the SDK.

The HTTP endpoint follows the OpenAI audio transcription request shape. It is
protected by Kronk's `transcriptions` authentication permission when server
authentication is enabled.

Whisper models use GGML `.bin` files and are separate from the GGUF models used
by Kronk's language-model backend. Bucky models and language models share the
server's memory budget and pool controls.

### 18.2 Install Whisper Libraries

Install the default library bundle for the current host:

```sh
kronk bucky libs
```

The supported bundles are:

| Operating system | Architecture   | Processors              |
| ---------------- | -------------- | ----------------------- |
| macOS            | `amd64`, `arm64` | `cpu`, `metal`        |
| Linux            | `amd64`, `arm64` | `cpu`, `cuda`, `vulkan` |
| Windows          | `amd64`          | `cpu`, `cuda`         |

Use the CLI as the current source of truth for available combinations:

```sh
kronk bucky libs --list-combinations
```

Other useful operations are:

```sh
# Install a particular version for the current host.
kronk bucky libs --version=v1.7.0

# Install another bundle alongside the active one.
kronk bucky libs --install --arch=amd64 --os=linux --processor=cuda

# List or remove installed bundles.
kronk bucky libs --list-installs
kronk bucky libs --remove-install --arch=amd64 --os=linux --processor=cuda
```

The commands use the running model server by default. Add `--local` to manage
the files directly without a server.

The BUI's **Whisper Libraries** screen provides the same installation and
removal operations. If Bucky failed to initialize because its libraries were
missing, install a compatible bundle and **restart the server**. The running
server does not automatically retry Bucky initialization.

Libraries are installed below `~/.kronk/bucky-libraries/` by default. Kronk
normally selects the bundle for the current platform. To select a specific
installed bundle, set its directory before starting the server:

```sh
export KRONK_BUCKY_LIB_PATH=~/.kronk/bucky-libraries/linux/amd64/cuda
```

The library tools also recognize these platform overrides:

| Variable          | Values                                  |
| ----------------- | --------------------------------------- |
| `KRONK_ARCH`      | `amd64`, `arm64`                        |
| `KRONK_OS`        | `linux`, `darwin`, `windows`            |
| `KRONK_PROCESSOR` | `cpu`, `metal`, `cuda`, `vulkan`        |

Only combinations listed by `--list-combinations` can be installed.

### 18.3 Manage Models

List the bundled model catalog:

```sh
kronk bucky model catalog
```

The transcription models currently included in that catalog are:

| Model            | Approximate size | Language support                         |
| ---------------- | ---------------- | ---------------------------------------- |
| `tiny`           | 75 MB            | Multilingual; fastest, lowest accuracy   |
| `base`           | 142 MB           | Multilingual; fast                       |
| `base.en`        | 142 MB           | English only                             |
| `small`          | 466 MB           | Multilingual; balanced                   |
| `small.en`       | 466 MB           | English only                             |
| `medium`         | 1.5 GB           | Multilingual; accurate                   |
| `medium.en`      | 1.5 GB           | English only                             |
| `large-v3`       | 2.9 GB           | Multilingual; highest accuracy           |
| `large-v3-turbo` | 1.5 GB           | Multilingual; faster large-model variant |

The catalog also contains `silero-vad`, an auxiliary voice-activity detection
model. It is not a transcription model and is not required by the SDK's
built-in streaming silence detection.

Pull, list, and remove models with:

```sh
kronk bucky model pull tiny
kronk bucky model list
kronk bucky model remove tiny
```

The pull command accepts a catalog name, a GGML filename such as
`ggml-tiny.bin`, or a complete download URL. The catalog name `tiny` and the
filename `ggml-tiny.bin` identify the same multilingual model. English-only
models use the `.en` suffix, such as `base.en`.

Add `--local` to any model command to operate directly on disk. Installed
models are stored below `~/.kronk/bucky-models/` by default, using filenames
such as `ggml-tiny.bin` and `ggml-base.en.bin`.

### 18.4 Server Configuration

Start the model server normally after installing the libraries and at least
one model. Bucky uses the standard server address, whose default is:

```text
http://localhost:11435
```

Whisper models do not use Kronk's per-model YAML configuration. The server
discovers installed `.bin` files and loads a model when it is first requested.

Bucky uses the server's shared pool settings:

| Setting          | Server default | Purpose                              |
| ---------------- | -------------- | ------------------------------------ |
| Models in pool   | `10`           | Maximum cached models                |
| Pool TTL         | `20m`          | How long an idle model remains loaded |
| Memory budget    | Shared         | Bucky and language models compete for the same host memory budget |

If the server starts without usable Whisper libraries, it logs that Bucky is
running in degraded mode. Library and model management remain available, but
transcription cannot work until the libraries are installed and the server is
restarted.

### 18.5 Browser UI

The BUI provides three Bucky-related areas:

1. **Whisper Libraries** installs and removes compatible library bundles.
2. **Whisper Models** browses the catalog and manages downloaded models.
3. **Translator** records or uploads audio and displays its transcript.

In Translator:

1. Select an installed model.
2. Upload an audio file or record from the microphone.
3. Leave the source language on **Auto-detect**, or select a language hint.
4. Optionally enable translation to English or provide a decoder prompt.
5. Select **Transcribe** or **Translate**.

Translator requests `verbose_json` and displays the text and segment timing.
It does not expose every field or response format supported by the HTTP API.
Use the API directly when you need plain text, SRT, WebVTT, or explicit
timestamp options.

### 18.6 Transcriptions API

#### 18.6.1 Request and Response

Send a `multipart/form-data` request to:

```text
POST /v1/audio/transcriptions
```

The uploaded file is limited to **25 MB**. Each transcription has a 30-minute
server deadline.

| Field                       | Required | Purpose |
| --------------------------- | -------- | ------- |
| `file`                      | Yes      | Audio file to decode and transcribe |
| `model`                     | Yes      | Installed model ID, such as `tiny` or `base.en` |
| `language`                  | No       | Whisper short language code such as `en`, `de`, or `fr`; empty means auto-detect |
| `prompt`                    | No       | Text that biases the initial decoder output |
| `translate`                 | No       | `true` translates supported source speech to English |
| `response_format`           | No       | `json` (default), `verbose_json`, `text`, `srt`, or `vtt` |
| `timestamp_granularities[]` | No       | `word` is accepted; word data is not yet available and returns an empty `words` array in `verbose_json` |

Example:

```sh
curl -X POST http://localhost:11435/v1/audio/transcriptions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -F file=@samples/jfk.wav \
  -F model=tiny \
  -F response_format=json
```

The default JSON response is:

```json
{"text":"And so my fellow Americans..."}
```

`verbose_json` adds the detected language, duration, and timestamped segments.
The `text`, `srt`, and `vtt` formats return their corresponding non-JSON media
types.

English-only models (`base.en`, `small.en`, and `medium.en`) only accept an
empty language hint or `en`. Use a multilingual model for other languages or
translation.

#### 18.6.2 Bucky Management Endpoints

The CLI and BUI use these management routes:

| Method and path                            | Purpose |
| ------------------------------------------ | ------- |
| `GET /v1/bucky/libs`                       | Show the active library installation |
| `GET /v1/bucky/libs/combinations`          | List supported platform combinations |
| `GET /v1/bucky/libs/installs`              | List installed library bundles |
| `POST /v1/bucky/libs/pull`                 | Install a library bundle |
| `DELETE /v1/bucky/libs/installs`           | Remove a library bundle |
| `GET /v1/bucky/models`                     | List installed models |
| `GET /v1/bucky/models/catalog`             | List the bundled catalog |
| `POST /v1/bucky/models/pull`               | Download a model |
| `GET /v1/bucky/models/{model}/details`     | Show model header and file details |
| `DELETE /v1/bucky/models/{model}`          | Remove a model |

Mutating management routes require administrator authorization when
authentication is enabled.

### 18.7 Go SDK

The Go SDK supports batch transcription, channel-separated diarization, and
live streaming. See [`examples/bucky/main.go`](../examples/bucky/main.go) for a
complete program that installs the current-host libraries, downloads a model,
initializes Bucky, and handles errors.

#### 18.7.1 Batch Transcription

After calling `bucky.Init` and constructing a `*bucky.Bucky` with the path to
an installed model, transcribe an audio reader directly:

```go
f, err := os.Open("samples/jfk.wav")
if err != nil {
    return err
}
defer f.Close()

tr, err := b.TranscribeFile(ctx, f, model.WithLanguage("en"))
if err != nil {
    return err
}

fmt.Println(tr.Text)
```

Use `Transcribe` instead when the audio is already decoded to 16 kHz mono
`[]float32`. Options include language, translation, initial prompt, beam size,
thread count, and no-speech or log-probability thresholds. Consult the Go API
documentation for the complete option list.

#### 18.7.2 Channel-Separated Diarization

`TranscribeChannelsFile` treats each source channel as a separate speaker and
merges their timestamped segments:

```go
d, err := b.TranscribeChannelsFile(ctx, f, model.WithLanguage("en"))
if err != nil {
    return err
}

for _, seg := range d.Segments {
    fmt.Printf("[%dms] speaker %d: %s\n", seg.StartMs, seg.Channel, seg.Text)
}
```

This works best with native multichannel formats such as WAV or FLAC. Formats
decoded through ffmpeg may be downmixed and therefore produce only one
speaker channel.

#### 18.7.3 Streaming Transcription

Use `NewStream` when audio arrives over time. A stream emits tentative
partials and committed finals:

```go
stream, err := b.NewStream(ctx, model.WithStreamLanguage("en"))
if err != nil {
    return err
}
defer stream.Close()

go func() {
    for ev := range stream.Events() {
        switch ev.Kind {
        case model.EventPartial:
            // Replace the currently displayed partial with ev.Text.
        case model.EventFinal:
            // Append ev.Text to the permanent transcript.
        case model.EventError:
            log.Println(ev.Err)
        }
    }
}()

for samples := range incomingAudio {
    if err := stream.Feed(ctx, samples); err != nil {
        return err
    }
}

return stream.Close()
```

`EventPartial` contains the complete pending hypothesis, not a text delta, and
may be dropped when the event consumer falls behind. `EventFinal` is the text
to append and is not dropped.

`Feed` accepts normalized 16 kHz mono `[]float32`. For raw microphone data,
`FeedPCM` accepts little-endian `int16` or `float32` PCM and performs downmixing
and resampling:

```go
format := model.AudioFormat{
    SampleRate: 48000,
    Channels:   2,
    Sample:     model.Int16LE,
}

if err := stream.FeedPCM(ctx, rawPCM, format); err != nil {
    return err
}
```

Call `Feed` or `FeedPCM` from one producer goroutine. Both methods apply
backpressure when their input queue is full. Real-time capture callbacks must
not block; the [`examples/bucky-stream`](../examples/bucky-stream/main.go)
program therefore uses an intermediate channel and deliberately drops capture
buffers if its pump falls behind.

Important stream defaults are:

| Behavior             | Default |
| -------------------- | ------- |
| Partial update       | Every 1,000 ms |
| Forced final         | Every 6,000 ms without a pause |
| Maximum utterance    | 25,000 ms |
| Silence detection    | Enabled |
| Prompt carryover     | Enabled |

Options such as `WithPartialEveryMs`, `WithCommitEveryMs`, `WithVAD`, and
`WithPromptCarryover` change these behaviors. A negative partial interval
disables partial events.

`Reset` starts a new logical session while keeping the stream open. By default
it flushes pending audio and restarts timestamps at zero. After an
`EventError`, close the failed stream and open a new one instead of resetting
it.

Always close a stream. An open stream reserves SDK inference capacity and can
prevent model unloading. SDK users that need concurrent streams can configure
`model.WithNSeqMax` when creating the Bucky handle; this is an SDK setting, not
a server configuration field.

### 18.8 Languages

Whisper supports approximately 99 languages. Use its short language codes,
such as `en`, `de`, `fr`, `es`, or `zh`. An empty language value asks Whisper
to auto-detect the language.

The SDK exposes `bucky.LangID`, `bucky.LangStr`, and `bucky.LangMaxID` for
enumerating and converting the codes known to the loaded whisper.cpp library.

Use a multilingual model (`tiny`, `base`, `small`, `medium`, `large-v3`, or
`large-v3-turbo`) for non-English speech. Models ending in `.en` are English
only. Whisper translation converts supported source speech to English; it does
not translate into arbitrary target languages.

### 18.9 Troubleshooting

| Symptom | Action |
| ------- | ------ |
| Server logs `bucky init failed, running in degraded mode` | Install a library bundle compatible with the host, then restart the server. |
| Transcription reports an unknown model | Run `kronk bucky model list`; pull the required model if it is absent. |
| An English-only model rejects the language | Use `en`, omit the hint, or switch to a multilingual model. |
| The upload is rejected for its size | Keep the audio file at or below 25 MB. Split long recordings or re-encode them at a lower bitrate. |
| Audio decodes to no samples | The file may be corrupt or unsupported. Re-encode it as a 16 kHz mono WAV and retry. |
| GPU inference is unexpectedly slow | Check `KRONK_BUCKY_LIB_PATH` and the active bundle. A CPU bundle runs without GPU acceleration. |
| A new SDK stream blocks or times out | Another stream is holding all configured SDK stream capacity. Close idle streams or create the handle with a larger `NSeqMax`. |
| Streaming emits finals but no partials | Check that `WithPartialEveryMs` was not given a negative value and that audio is arriving continuously. |
| Streaming repeats words at boundaries | Keep silence detection enabled and avoid overly short forced-final intervals. |
| Whisper diagnostic output appears in the terminal | Do not select `bucky.LogNormal`; the default `LogSilent` suppresses whisper.cpp diagnostics. |

---

_Next: [Chapter 19: Developer Guide](chapter-19-developer-guide.md)_
