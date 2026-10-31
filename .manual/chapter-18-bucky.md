# Chapter 18: Bucky (Audio Transcription)

## Table of Contents

- [18.1 Overview](#181-overview)
- [18.2 Install Whisper Libraries](#182-install-whisper-libraries)
- [18.3 Manage Models](#183-manage-models)
- [18.4 Server Configuration](#184-server-configuration)
- [18.5 Browser UI](#185-browser-ui)
- [18.6 Transcriptions and Translations API](#186-transcriptions-and-translations-api)
  - [18.6.1 Request and Response](#1861-request-and-response)
  - [18.6.2 Bucky Management Endpoints](#1862-bucky-management-endpoints)
- [18.7 Capability Boundaries and Developer Resources](#187-capability-boundaries-and-developer-resources)
- [18.8 Languages](#188-languages)
- [18.9 Troubleshooting](#189-troubleshooting)

---

Bucky is Kronk's speech-to-text subsystem. It uses
[`whisper.cpp`](https://github.com/ggerganov/whisper.cpp) and is available
through:

- the `/v1/audio/transcriptions` and `/v1/audio/translations` HTTP endpoints;
- the Browser UI (BUI) Translator;
- the `kronk bucky` management commands; and
- the Go packages under `sdk/bucky`.

Using Bucky requires both a compatible whisper.cpp library bundle and a
Whisper model. The libraries run the inference engine; the model contains the
speech-recognition weights.

Developer-level package, lifecycle, and test information belongs in
[Chapter 20: Developer Guide](https://www.kronkai.com/manual#chapter-20-developer-guide).

### 18.1 Overview

Bucky supports these common workflows:

- transcribe an uploaded or recorded audio file;
- translate speech from a supported language into English;
- return plain text, JSON, SRT, or WebVTT;
- transcribe separate audio channels as separate speakers through the SDK; and
- consume partial and final transcript events from live audio through the SDK.

The HTTP endpoint follows the OpenAI audio transcription request shape. It is
protected by Kronk's `transcriptions` endpoint grant in `full-protected` mode.
`authenticated` requires a valid token without grants; `open` and `management`
leave inference public. See Chapter 12.

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
| Linux            | `amd64`, `arm64` | `cpu`, `cuda`, `cuda12`, `cuda13`, `vulkan` |
| Windows          | `amd64`          | `cpu`, `cuda`, `cuda12` |

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

# Install the Linux CUDA 13 bundle alongside CUDA 12.
kronk bucky libs --install --arch=amd64 --os=linux --processor=cuda13

# List or remove installed bundles.
kronk bucky libs --list-installs
kronk bucky libs --remove-install --arch=amd64 --os=linux --processor=cuda
```

The commands use the running model server by default. Add `--local` to manage
the files directly without a server.

The BUI's **Whisper Libraries** screen provides the same installation and
removal operations when the server is running. Default library installation or
verification failures stop startup; install a compatible managed bundle with
`kronk bucky libs --local`, then **restart the server**. If startup reached native
initialization but Bucky failed, the server can remain running in degraded mode
and the BUI can manage libraries. It does not automatically retry initialization.

Libraries are installed below `~/.kronk/bucky-libraries/` by default. Kronk
normally treats the detected processor as a preference and verifies that the
corresponding whisper.cpp bundle can run on the host. CPU and Metal are retained
when published for the platform. CUDA is retained only when `nvidia-smi`
reports a compatible driver and every GPU visible through
`CUDA_VISIBLE_DEVICES` meets the bundle's compute-capability requirement:

| Platform | Minimum reported CUDA version | Minimum compute capability |
| -------- | ----------------------------- | -------------------------- |
| Linux `amd64` | 12.9 | 8.6 |
| Linux `arm64` | 12.9 | 8.7 |
| Windows `amd64` | 12.4 | 5.0 |

`cuda` and `cuda12` download the same CUDA 12 bundle into separate processor
directories. On Linux, automatic detection with a `cuda` preference first
tries `cuda13`: it requires a reported CUDA version of at least 13.0, the same
GPU capability thresholds above, and installed `libcudart.so.13` and
`libcublas.so.13`. If those checks fail, it tries CUDA 12. Linux CUDA 12 also
requires installed `libcudart.so.12` and `libcublas.so.12`. Library discovery
uses `ldconfig -p` and `LD_LIBRARY_PATH`; stale cache entries do not count.
The Linux bundles do not include these NVIDIA user-space libraries. A newer
driver alone does not supply them. Both CUDA runtime majors can coexist.
Keep Jetson Orin on CUDA 12 unless its JetPack/runtime supports CUDA 13.

An explicit `--install --processor=cuda13` downloads the requested bundle
without probing the current host, allowing installation for another machine.
CUDA 13 archives use `cuda-13` in their filenames; the Bucky processor option
is `cuda13`, without a hyphen. Windows remains CUDA 12 only.

If the preferred CUDA or Vulkan runtime is unavailable or incompatible, Linux
uses Vulkan when `vulkaninfo --summary` reports a usable GPU and otherwise
uses CPU. Other supported platforms fall back to CPU. Bucky has no separate
ROCm artifact, so a shared `KRONK_PROCESSOR=rocm` preference resolves to Vulkan
on a usable Linux Vulkan host and otherwise to CPU. Startup logs report the
preferred runtime, selected runtime, and reason.

To select a specific installed bundle or user-managed build authoritatively,
set its directory before starting the server:

```sh
export KRONK_BUCKY_LIB_PATH=~/.kronk/bucky-libraries/linux/amd64/cuda
```

For CUDA 13, use the corresponding `linux/amd64/cuda13` or
`linux/arm64/cuda13` directory. Prefer this Bucky-specific override rather
than setting `KRONK_PROCESSOR=cuda13` for the whole server: llama.cpp uses
different CUDA-major option names.

A non-empty `KRONK_BUCKY_LIB_PATH` bypasses automatic compatibility selection.
If the directory contains `version.json`, Kronk adopts its recorded platform
metadata unless explicitly overridden. A non-empty existing directory without
that file is treated as a read-only user-managed build and is not installed,
upgraded, or replaced. Default verified startup requires managed installation
metadata and fails for such a build. Independently trusted custom builds require
an advanced startup path with managed verification explicitly disabled; this
is not routine recovery for a broken installation.

The default downloader installs the authenticated whisper.cpp version bound
to this Bucky dependency, replacing older or arbitrary newer managed bundles
rather than assuming numerical version order implies ABI compatibility. An
offline installation must match the requested pin and platform. `--version`
and `--upgrade` explicitly opt out of the default pin. Restart after replacing
libraries. If a future upgrade changes the ABI, rebuild or replace user-managed
libraries yourself; Kronk never mutates a read-only build. Do not mix ggml
files from independently built Whisper and llama.cpp bundles.

The library tools also recognize these platform overrides:

| Variable          | Values                                  |
| ----------------- | --------------------------------------- |
| `KRONK_ARCH`      | `amd64`, `arm64`                        |
| `KRONK_OS`        | `linux`, `darwin`, `windows`            |
| `KRONK_PROCESSOR` | `cpu`, `metal`, `cuda`, `cuda-12`, `cuda-13`, `vulkan`; `rocm` maps to Vulkan or CPU |

Only combinations listed by `--list-combinations` can be installed. Platform
variables remain preferences during automatic Bucky selection; use
`KRONK_BUCKY_LIB_PATH` when the selected directory must be exact. Library
selection is process-wide, so restart the CLI process or server after changing
it.

The shared environment spellings `cuda-12` and `cuda-13` map to Bucky's
`cuda12` and `cuda13` processors. For Bucky install commands, use the
unhyphenated spelling shown by `--list-combinations`.

### 18.3 Manage Models

List the bundled model catalog:

```sh
kronk bucky model catalog
```

The transcription models currently included in that catalog are:

| Model            | Approximate size | Language support                         |
| ---------------- | ---------------- | ---------------------------------------- |
| `tiny`           | 75 MB            | Multilingual; fastest, lowest accuracy   |
| `tiny.en`        | 75 MB            | English only                             |
| `base`           | 142 MB           | Multilingual; fast                       |
| `base.en`        | 142 MB           | English only                             |
| `small`          | 466 MB           | Multilingual; balanced                   |
| `small.en`       | 466 MB           | English only                             |
| `medium`         | 1.5 GB           | Multilingual; accurate                   |
| `medium.en`      | 1.5 GB           | English only                             |
| `large-v3`       | 2.9 GB           | Multilingual; highest accuracy           |
| `large-v3-turbo` | 1.5 GB           | Multilingual; faster large-model variant |

The catalog also contains `silero-vad`, an auxiliary voice-activity detection
model. It is not a transcription model. SDK streaming uses its built-in energy
detector unless the application explicitly selects the Silero model.

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

The server discovers installed `.bin` files and loads a model when it is first
requested. Configure each model under the top-level `bucky-models` mapping in
`~/.kronk/models/model_config.yaml`, using the short ID shown by
`kronk bucky model list`:

```yaml
version: 2
bucky-models:
  tiny:
    nseq-max: 2
    queue-depth: 4
    admission-timeout: 3m
    nthreads: 0
```

`nseq-max` controls the number of concurrent operations. Increasing it increases
runtime memory without duplicating the model weights. `queue-depth` adds waiting
requests without adding inference capacity. `nthreads` controls native threads
per operation; zero preserves the whisper.cpp default. Restart the server after
changing these settings.

Bucky uses the server's shared pool settings:

| Setting          | Server default | Purpose                              |
| ---------------- | -------------- | ------------------------------------ |
| Models in pool   | `10`           | Maximum cached models                |
| Pool TTL         | `0`            | Idle expiration disabled by default; count/memory eviction still applies |
| Memory budget    | Shared         | Bucky, language, and Malina models compete for the shared budget |

`--media-backends-enabled=false` / `KRONK_MEDIA_BACKENDS_ENABLED=false` skips
both Bucky and Malina library downloads and initialization. Installing libraries
alone does not re-enable inference: enable media backends and restart. Library
installation/verification failures normally stop startup; native initialization
failures can leave Bucky unavailable while the server continues.

Server startup constructs the Bucky library manager from `KRONK_BASE_PATH` and
`KRONK_BUCKY_LIB_PATH`, downloads or verifies the selected bundle, and passes
the manager's resolved directory to the whisper runtime. This keeps a moved
base path and an explicit Bucky library path consistent across installation and
inference. The llama.cpp `--lib-path`/`KRONK_LIB_PATH` setting does not select
whisper.cpp libraries; use `KRONK_BUCKY_LIB_PATH` for Bucky.

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

### 18.6 Transcriptions and Translations API

#### 18.6.1 Request and Response

Send a `multipart/form-data` request to:

```text
POST /v1/audio/transcriptions
```

To translate supported source speech into English, use the OpenAI-compatible
translation route instead:

```text
POST /v1/audio/translations
```

The uploaded file is limited to **25 MiB**. Transcription and translation use
the shared `--inference-timeout` / `KRONK_WEB_INFERENCE_TIMEOUT`, default `60m`.

WAV, MP3, and FLAC decode in-process. Browser recordings and formats such as
WebM/Opus, MP4/AAC, OGG, and M4A require `ffmpeg` on the **server's PATH**, including
inside its container or service environment. Installing it only on the browser's
machine is not sufficient. Restart after fixing PATH or installing ffmpeg if
the running process has already failed to find it.

| Field                       | Required | Purpose |
| --------------------------- | -------- | ------- |
| `file`                      | Yes      | Audio file to decode and transcribe |
| `model`                     | Yes      | Installed model ID, such as `tiny` or `base.en` |
| `language`                  | No       | Whisper short language code such as `en`, `de`, or `fr`; empty means auto-detect |
| `prompt`                    | No       | Text that biases the initial decoder output |
| `translate`                 | No       | Kronk extension on the transcription route; `true` translates supported source speech to English |
| `temperature`               | No       | Initial decoding temperature |
| `temperature_inc`           | No       | Temperature increase used for fallback decoding |
| `entropy_threshold`         | No       | Entropy threshold for deciding whether to retry a decode |
| `logprob_threshold`         | No       | Average log-probability threshold for deciding whether to retry a decode |
| `no_speech_threshold`       | No       | Probability threshold used to classify a segment as silence |
| `greedy_best_of`            | No       | Number of candidates considered by greedy sampling |
| `beam_size`                 | No       | Positive beam size that selects beam-search sampling |
| `beam_search_patience`      | No       | Patience used by beam-search sampling |
| `length_penalty`            | No       | Decoder length penalty |
| `response_format`           | No       | `json` (default), `verbose_json`, `text`, `srt`, or `vtt` |
| `timestamp_granularities[]` | No       | `word` enables word-level timestamps in `verbose_json` |

Omitted sampling fields retain the defaults supplied by the loaded
whisper.cpp library. Sampling fields must contain valid numeric values.

Example:

```sh
curl -X POST http://localhost:11435/v1/audio/transcriptions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -F file=@samples/jfk.wav \
  -F model=tiny \
  -F temperature=0 \
  -F response_format=json
```

The default JSON response is:

```json
{"text":"And so my fellow Americans..."}
```

The translation route accepts the same options above and always enables
source-to-English translation, so it does
not require the Kronk-specific `translate` field. Its default JSON response has
the same `text` shape. A `verbose_json` translation identifies its task as
`translate` and its output language as `english`.
Prefer this route when verbose output must identify the translation task;
the transcription route's `translate=true` extension retains its transcription
task label.

`verbose_json` adds the detected language, duration, and timestamped segments.
When `timestamp_granularities[]=word` is requested, it also includes a `words`
array whose entries contain `word`, `start`, and `end` fields.
Segment `tokens`, `temperature`, `avg_logprob`, and `compression_ratio` are
compatibility placeholders (empty or zero), not measured decoder results. Do
not use them as confidence filters.
The `text`, `srt`, and `vtt` formats return their corresponding non-JSON media
types.

English-only models (`tiny.en`, `base.en`, `small.en`, and `medium.en`) only
accept an empty language hint or `en`. Use a multilingual model for other
languages or translation.

#### 18.6.2 Bucky Management Endpoints

The CLI and BUI use these management routes:

| Method and path                            | Purpose |
| ------------------------------------------ | ------- |
| `GET /v1/bucky/libs`                       | Show the active library installation |
| `GET /v1/bucky/libs/integrity`             | Verify the selected library bundle |
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

### 18.7 Capability Boundaries and Developer Resources

HTTP and BUI support uploaded or recorded-file transcription and translation.
Channel-separated diarization and continuous partial/final streaming are
SDK-only; they are not audio HTTP streaming routes. The Translator records
audio before uploading it rather than exposing the SDK's continuous stream.

Application developers should use the generated Bucky SDK reference and the
`examples/bucky` and `examples/bucky-stream` programs. The source-checkout
targets `make example-bucky` and `make example-bucky-stream` run those examples;
`make example-bucky-stream-vad` additionally downloads and selects Silero VAD.
These commands can download libraries/models and run inference or capture audio.

### 18.8 Languages

Whisper supports approximately 99 languages. Use its short language codes,
such as `en`, `de`, `fr`, `es`, or `zh`. An empty language value asks Whisper
to auto-detect the language.

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
| The upload is rejected for its size | Keep the audio file at or below 25 MiB. Split long recordings or re-encode them at a lower bitrate. |
| Audio decodes to no samples | The file may be corrupt or unsupported. Re-encode it as a 16 kHz mono WAV and retry. |
| GPU inference is unexpectedly slow | Check `KRONK_BUCKY_LIB_PATH` and the active bundle. A CPU bundle runs without GPU acceleration. |
| Browser recordings fail to decode | Make ffmpeg available on the server/container PATH and restart after a previous failed lookup. |
| Server startup fails during library installation or verification | Install the compatible managed bundle locally, inspect the verification error, then restart; do not routinely disable verification. |
| Libraries exist but audio inference is unavailable | Check `KRONK_MEDIA_BACKENDS_ENABLED`, enable it if needed, and restart. |

---

Next: [Chapter 19: Malina (Image Generation)](https://www.kronkai.com/manual#chapter-19-malina-image-generation)
