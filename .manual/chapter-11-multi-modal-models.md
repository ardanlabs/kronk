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

---

Multimodal models combine a language model with a media projector that turns
images or audio into input the language model can process. They use the Chat
Completions endpoint described in [Chapter 9](chapter-09-api-endpoints.md).

## 11.1 Models and Projectors

Use the catalog to find models with image or audio capabilities:

```shell
kronk catalog list
```

The `MTMD` column identifies entries with a multimodal projector. The BUI
catalog also provides image, audio, and video capability filters. The live
catalog is the source of truth; examples in the seed catalog include:

- `unsloth/LFM2.5-VL-1.6B-Q8_0` for images;
- `ggml-org/Qwen2.5-Omni-3B-Q8_0` for images and audio; and
- `ggml-org/Qwen3-Omni-30B-A3B-Instruct-Q8_0` for image, audio, and
  video-capable model metadata.

Pulling a catalog model also pulls its companion projector when one is
available:

```shell
kronk model pull unsloth/LFM2.5-VL-1.6B-Q8_0
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

This shell example expands the base64 value before sending the request:

```shell
IMAGE_B64=$(base64 < photo.jpg | tr -d '\n')

curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  --data-binary @- <<EOF
{
  "model": "unsloth/LFM2.5-VL-1.6B-Q8_0",
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
[Chapter 18](chapter-18-bucky.md#1871-post-v1audiotranscriptions).

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
[Chapter 3 §3.6](chapter-03-model-configuration.md#36-memory-planning-and-quantization)
for memory planning and [Chapter 4](chapter-04-batch-processing.md) for
concurrency.

Most deployments should leave `nubatch` unset. Its normal default is 2048, but
MoE expert CPU offload can raise it to 4096. A multimodal encoder may require
an entire media-token chunk to fit in one physical batch, so lowering
`nubatch` can break media input. `proj-on-cpu: true` can keep the projector on
the CPU when accelerator memory is constrained, at a performance cost.

## 11.7 Message Caching

Incremental Message Caching can reuse unchanged media state for text-only
follow-up turns without encoding the media again. Changing, reordering,
removing, or appending media rebuilds the stable media plan through the
multimodal pipeline. See
[Chapter 5 §5.4](chapter-05-message-caching.md#54-media-requests) for the cache
behavior and limitations.

## 11.8 Limitations

- Media must be embedded as base64; Kronk does not fetch remote URLs.
- The current path accepts image and audio containers, not video containers.
- The selected model and projector must support the detected modality.
- Image resolution, media count, and audio duration affect latency and memory.
- Model quality and practical media limits vary by model and projector.

---

_Next: [Chapter 12: Security & Authentication](chapter-12-security-authentication.md)_
