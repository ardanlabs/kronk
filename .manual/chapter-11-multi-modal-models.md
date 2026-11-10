# Chapter 11: Multimodal Models

## Table of Contents

- [11.1 Models and Projectors](#111-models-and-projectors)
- [11.2 Supported Inputs](#112-supported-inputs)
- [11.3 Sending an Image](#113-sending-an-image)
- [11.4 Sending Audio](#114-sending-audio)
- [11.5 Go SDK Helpers](#115-go-sdk-helpers)
- [11.6 Configuration and Resources](#116-configuration-and-resources)
- [11.7 Message Caching](#117-message-caching)
- [11.8 Limitations](#118-limitations)
- [11.9 Text-to-Speech SDK](#119-text-to-speech-sdk)

---

Multimodal models combine a language model with a media projector that turns
images or audio into input the language model can process. They use the Chat
Completions endpoint described in [Chapter 9](https://www.kronkai.com/manual#chapter-9-api-endpoints).
Text-to-speech is a separate SDK operation described in [§11.9](#119-text-to-speech-sdk);
it does not use Chat Completions or Bucky's transcription API.

## 11.1 Models and Projectors

Use the catalog to find models with image or audio capabilities:

```shell
kronk catalog list
```

The `MTMD` column identifies entries with a multimodal projector. The BUI
catalog also provides image, audio, and video capability filters. The live
catalog is the source of truth; examples in the seed catalog include:

- `ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0` for images;
- `ggml-org/Qwen2.5-Omni-3B-Q8_0` for images and audio; and
- `ggml-org/Qwen3-Omni-30B-A3B-Instruct-Q8_0` for image, audio, and
  video-capable model metadata.

Pulling a catalog model also pulls its companion projector when one is
available:

```shell
kronk model pull ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0
```

The model and projector capabilities must match the submitted media. A model
without a projector rejects media, as does a projector without support for the
detected image or audio type.

## 11.2 Supported Inputs

Kronk recognizes media from its decoded file signature rather than trusting a
declared MIME type or extension:

| Media | Recognized containers |
| ----- | --------------------- |
| Images | JPEG, PNG, GIF, WebP |
| Audio | WAV, MP3, Ogg, FLAC |

For REST requests, prefer an ordered content array containing text and one or
more media parts. Media values can be base64 data URLs or raw base64. Despite
the `image_url` and `video_url` field names, Kronk does not fetch `http://` or
`https://` URLs.

Kronk also recognizes a plain base64 string used as the entire message
`content`. That legacy form is less useful because it cannot place text and
media together in one ordered content array.

Actual video containers such as MP4 and WebM are not decoded by the current
media path. For video analysis, extract frames and send them as supported
images in the intended order.

## 11.3 Sending an Image

Place media before the question unless the selected model documents another
order. Several multimodal templates were trained with the media token first,
and Kronk preserves the order of all content parts.

> **Note:** Despite the `image_url` field name, Kronk does not download images
> from `http://` or `https://` URLs. The client must download the image and send
> it as a base64 data URL or raw base64 value.

This shell example expands the base64 value before sending the request:

```shell
IMAGE_B64=$(base64 < photo.jpg | tr -d '\n')

curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  --data-binary @- <<EOF
{
  "model": "ggml-org/Qwen2.5-VL-3B-Instruct-Q8_0",
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "image_url",
          "image_url": {"url": "data:image/jpeg;base64,${IMAGE_B64}"}
        },
        {"type": "text", "text": "Describe this image."}
      ]
    }
  ],
  "max_tokens": 1024
}
EOF
```

An ordered content array can contain multiple images and text parts. Whether a
particular number or resolution of images works well depends on the selected
model and projector.

## 11.4 Sending Audio

Audio uses an `input_audio` content part in the same ordered array:

```json
{
  "type": "input_audio",
  "input_audio": {
    "data": "data:audio/wav;base64,UklGRi...",
    "format": "wav"
  }
}
```

Put this part before the text question. The `format` field is accepted for
client compatibility, but Kronk currently determines the actual format from
the decoded bytes rather than this value.

Use a multimodal chat model when you need conversational questions, summaries,
or reasoning about audio. For a dedicated speech-to-text API, use Bucky's
`POST /v1/audio/transcriptions` endpoint described in
[Chapter 18](https://www.kronkai.com/manual#1861-request-and-response).

## 11.5 Go SDK Helpers

Go applications can read media into a byte slice and use:

- `model.ImageMessage(question, image, format)`; or
- `model.AudioMessage(question, audio, format)`.

These helpers create one user turn with media before text. `model.VideoMessage`
constructs a `video_url` part, but it does not add video-container decoding;
send extracted frames with `ImageMessage` for the current media path.

## 11.6 Configuration and Resources

Multimodal requests use the same batch engine and concurrency controls as text
requests. The projector adds weights and runtime buffers, while image
resolution, audio duration, context length, and `nseq-max` affect resource use.
Use the BUI VRAM Calculator rather than adding model, projector, and KV file
sizes as a complete memory estimate. See
[Chapter 3 §3.6](https://www.kronkai.com/manual#36-memory-planning-and-quantization)
for memory planning and [Chapter 4](https://www.kronkai.com/manual#chapter-4-batch-processing) for
concurrency.

Most deployments should leave `prefill-batch-size` at its 2048-token default.
A multimodal encoder may require an entire media-token chunk to fit in one
batch, so lowering
`prefill-batch-size` below that capacity can break media input.
`proj-on-cpu: true` can keep the projector on the CPU when accelerator memory
is constrained, at a performance cost. This relieves discrete VRAM but does not
remove the projector's memory cost on unified-memory systems.

Media preparation is scheduled alongside other requests, but an individual
image or audio chunk can still monopolize computation while it is encoded or
decoded. Do not assume that a text-prefill chunk limit bounds every media
operation's latency.

## 11.7 Message Caching

Incremental Message Caching can reuse unchanged media state for text-only
follow-up turns without repeating model media encoding/prefill for the cached
portion. Continue sending the complete conversation, including original media;
IMC does not reconstruct omitted inputs. Appending media can also reuse a
compatible prefix when native-stream and attention-mode checks permit.
Changing, reordering, or removing cached inputs requires another compatible
session or rebuilding the prefix. See
[Chapter 5 §5.4](https://www.kronkai.com/manual#54-media-requests) for the cache
behavior and limitations.

## 11.8 Limitations

- Media must be embedded as base64; Kronk does not fetch remote URLs.
- The current path accepts image and audio containers, not video containers.
- The selected model and projector must support the detected modality.
- Image resolution, media count, and audio duration affect latency and memory.
- Model quality and practical media limits vary by model and projector.
- Kronk does not impose fixed media payload, image-dimension, or media-count
  limits. Image decoding allocates memory in proportion to decoded dimensions,
  and a request can retain multiple decoded media items at once. Applications
  that accept media from untrusted sources must validate those resources before
  passing them to Kronk.

## 11.9 Text-to-Speech SDK

`krn.Speech(ctx, model.SpeechRequest{Input: text})` generates a complete PCM16
little-endian mono WAV. Speech models are detected automatically; `WithSpeech`
is optional configuration, not a switch required to enable speech. This is a
Go SDK feature: no `/v1/audio/speech` endpoint or BUI speech playground is
provided yet. The normal model-download tools can still install these bundles.

### 11.9.1 Models and Installation

The verified bundles are:

| Model source | Matching companion | Verified output and voice selection |
| --- | --- | --- |
| `ggml-org/chatterbox-turbo-Q8_0` | F16 audio decoder | English, 24 kHz WAV; empty voice uses the bundled `default` voice |
| `ggml-org/Qwen3-TTS-12Hz-1.7B-Base-Q8_0` | Matching Q8_0 audio decoder | 24 kHz WAV; use an empty voice, including when the bundle has no named presets |

Install with the regular Hugging Face/catalog downloader. It discovers and
downloads the matching decoder; you do not need to supply a separate projector
URL:

```shell
kronk model pull --local ggml-org/chatterbox-turbo-Q8_0
kronk model pull --local ggml-org/Qwen3-TTS-12Hz-1.7B-Base-Q8_0
```

From a source checkout, `make install-speak-models` installs both bundles.
Files remain in the normal model store under `~/.kronk/models/` unless you
configure a different data root. The companion is named `mmproj-*.gguf`, but
it is an audio decoder, not an image projector. Use the matching companion,
not an arbitrary GGUF with the same model family name.

Run the complete SDK example from the repository root:

```shell
make example-speech
afplay examples/speech.wav # macOS playback; use a WAV player on other systems.
```

The example installs compatible native libraries and the selected model on
first use, writes `examples/speech.wav` only after successful synthesis, and
prints its sample rate, sample count, duration, and byte count. Its
`modelSource` constant selects Qwen by default; the commented Chatterbox
source can be substituted without specifying a decoder URL. A failed run
does not replace a WAV left by an earlier successful run.

Use Kronk's library manager to install the compatible native bundle before
calling `kronk.Init`. Older or incompatible libraries without native audio
generation return `model.ErrSpeechUnsupported`. Soprano, Pocket TTS, other
Qwen sizes/variants, and Chatterbox Multilingual are not qualified by these
two working bundles; upstream native support alone is not Kronk support.

### 11.9.2 Go SDK Usage

`models.Download` returns both `ModelFiles` and the automatically discovered
`ProjFile`. After native initialization, a buffered speech operation can use
that returned `models.Path` directly:

```go
func speak(ctx context.Context, mp models.Path) (err error) {
    krn, err := kronk.New(
        model.WithModelFiles(mp.ModelFiles),
        model.WithProjFile(mp.ProjFile),
    )
    if err != nil {
        return err
    }
    defer func() {
        err = errors.Join(err, krn.Unload(context.Background()))
    }()

    response, err := krn.Speech(ctx, model.SpeechRequest{
        Input: "I played column four. Your turn!",
    })
    if err != nil {
        return err
    }
    return os.WriteFile("speech.wav", response.Audio, 0644)
}
```

This uses the standard `context`, `errors`, and `os` packages plus
`github.com/ardanlabs/kronk/sdk/kronk`, `sdk/kronk/model`, and
`sdk/tools/models`. See `examples/speech/main.go` for complete installation,
initialization, and error handling.

`SpeechResponse.Audio` contains the WAV header and audio data, not base64 or
raw PCM. `SampleRate` is samples per second and `Samples` excludes the header;
duration is `float64(Samples) / float64(SampleRate)` seconds. The caller owns
the bytes, which remain valid after another request or model unload. No
partial audio is returned on failure. Speech calls do not require a chat
template and must not be sent through chat, embedding, or reranking methods.

### 11.9.3 Inputs, Voices, and Configuration

- `Input` must be nonblank valid UTF-8 without NUL bytes, at most 64 KiB. That
  byte limit is not a promise that the text fits: text, model/voice
  conditioning, and the entire generation-frame budget must fit the context.
- Empty `Voice`, or the compatibility alias `"default"`, uses the model's
  default conditioning. Other voice names must be presets in that exact
  decoder bundle. OpenAI voice names such as `alloy` are not a supported
  universal voice list. Qwen Base does not need a preset named `default`.
- The request has no language, reference-audio, voice-cloning, speed, or
  speech-sampling options. The tested path is English; Qwen currently uses
  the native English default rather than exposing its family's language
  selector. Chat sampling settings and `max_tokens` do not configure speech.
- `model.WithSpeech(&model.SpeechConfig{MaxFrames: 256})` can lower the
  generation ceiling. Nil configuration or `MaxFrames: 0` uses **512**;
  accepted explicit limits are **1–512**. Frames are model-specific units,
  not characters, PCM samples, or a common duration across models. Reaching
  the ceiling without end-of-speech returns `model.ErrSpeechLimit`, not
  successful truncated audio.
- Runtime settings are **2,048 context tokens per slot**, **512-token
  prefill and physical batches**, and **partitioned F16 K/V caches**.
  Incompatible explicit overrides, IMC, speculative decoding, and adapters
  are rejected. Speech resolves these fixed limits instead of applying chat
  AutoTune recommendations.
- `model.WithNSeqMax(n)` sets the number of concurrent speech slots; the
  default is **1**. Requests can overlap, but native inference is serialized;
  more slots do not promise a throughput speedup. Memory requirements grow
  with slot count even though the loaded model weights are shared.

Use a context deadline for processing. Cancellation is checked between native
steps and around final audio generation; it may wait for an in-flight native
call. A canceled call returns only after its sequence cleanup, so its slot
can be reused without discarding a neighboring request. The handle's normal
admission timeout and queue controls also apply.

Match errors with `errors.Is`:

| Error | Meaning |
| --- | --- |
| `model.ErrInvalidRequest` | Invalid input, unknown named voice, or insufficient context for the request |
| `model.ErrSpeechUnsupported` | Non-speech model, missing/incompatible decoder, or unavailable native speech capability |
| `model.ErrSpeechLimit` | No end-of-speech within `MaxFrames`; no audio is returned |
| `context.Canceled`, `context.DeadlineExceeded` | Caller cancellation or deadline |
| `kronk.ErrAdmissionTimeout` | Could not obtain a request permit within the configured admission timeout |

### 11.9.4 Memory Planning and Typed Pools

The `sdk/kronk/pool` package acquires speech models through the normal
`AquireModel` operation. The returned handle supports the same `Speech`
method. Decoder discovery, fixed runtime defaults, and speech-aware memory
reservations are automatic. `WithSpeech` is a Go SDK option; there is no
`speech`/`max-frames` entry in `model_config.yaml` at present. Custom pooled
configurations can carry `model.Config.PtrSpeech` through `AquireCustom`.

Memory planning includes the backbone, decoder, KV caches, slot-dependent
host memory, and retained working buffers. On unified-memory systems these
are charged once to RAM, not again to a separate VRAM budget. An active
speech request protects its model from eviction. Failed loads release their
reservations; idle eviction or pool shutdown unloads the model and releases
its reservation. A bundle that cannot fit returns `pool.ErrNoCapacity`;
temporary pressure with no idle eviction candidate returns `pool.ErrServerBusy`.

**Qualified memory policies are limited to the verified b12223 Metal bundles
above**, through 512 frames. Their estimates include calibrated allowances,
not portable peak-memory guarantees. CPU/CUDA decoder estimates and unknown
decoder layouts fail planning explicitly rather than silently reserving only
file size or using a chat estimate. Direct SDK synthesis is not disabled by
this calculator restriction, but a working direct call does not establish
qualified pooled memory support on another backend. Lower frame limits can
reduce output allowances without reducing all retained decoder workspace.

Incremental/streaming audio delivery, selectable output formats, reference
voices, and an HTTP/BUI synthesis interface remain outside this SDK contract.

---

_Next: [Chapter 12: Security & Authentication](https://www.kronkai.com/manual#chapter-12-security-and-authentication)_
