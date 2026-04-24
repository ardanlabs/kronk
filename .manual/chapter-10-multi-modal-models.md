# Chapter 10: Multi-Modal Models

## Table of Contents

- [10.1 Overview](#101-overview)
- [10.2 Vision Models](#102-vision-models)
- [10.3 Audio Models](#103-audio-models)
- [10.4 Plain Base64 Format](#104-plain-base64-format)
- [10.5 Configuration for Multi-Modal Models](#105-configuration-for-multi-modal-models)
- [10.6 Memory Requirements](#106-memory-requirements)
- [10.7 IMC and Multi-Modal Caching](#107-imc-and-multi-modal-caching)
- [10.8 Limitations](#108-limitations)
- [10.9 Example: Image Analysis](#109-example-image-analysis)
- [10.10 Example: Audio Transcription](#1010-example-audio-transcription)

---



Kronk supports vision and audio models that can process images, video frames,
and audio alongside text. This chapter covers how to use these models.

### 10.1 Overview

Multi-modal models combine a language model with a media projector that
converts images or audio into tokens the model can understand.

**Supported Media Types:**

- **Vision**: JPEG, PNG, GIF images
- **Audio**: WAV audio files

**Available Models (from catalog):**

```shell
kronk catalog list --filter-category=Image
kronk catalog list --filter-category=Audio
```

Example models:

- `Qwen2.5-VL-3B-Instruct-Q8_0` - Vision model
- `gemma-3-4b-it-q4_0` - Vision model
- `Qwen2-Audio-7B.Q8_0` - Audio model

### 10.2 Vision Models

Vision models analyze images and answer questions about their content.

**Download a Vision Model:**

```shell
kronk catalog pull Qwen2.5-VL-3B-Instruct-Q8_0
```

**API Request with Image (OpenAI Format):**

```shell
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2.5-VL-3B-Instruct-Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "text",
            "text": "What do you see in this image?"
          },
          {
            "type": "image_url",
            "image_url": {
              "url": "data:image/jpeg;base64,/9j/4AAQSkZJRg..."
            }
          }
        ]
      }
    ]
  }'
```

**Content Array Structure:**

The `content` field is an array of content parts:

```json
{
  "content": [
    { "type": "text", "text": "Describe this image" },
    {
      "type": "image_url",
      "image_url": { "url": "data:image/jpeg;base64,..." }
    }
  ]
}
```

**Supported image_url Formats:**

- Base64 data URL: `data:image/jpeg;base64,/9j/4AAQSkZJRg...`
- Base64 data URL: `data:image/png;base64,iVBORw0KGgo...`

### 10.3 Audio Models

Audio models transcribe and understand spoken content.

**Download an Audio Model:**

```shell
kronk catalog pull Qwen2-Audio-7B.Q8_0
```

**API Request with Audio:**

```shell
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2-Audio-7B.Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "text",
            "text": "Transcribe this audio and summarize what is said."
          },
          {
            "type": "input_audio",
            "input_audio": {
              "data": "UklGRi...",
              "format": "wav"
            }
          }
        ]
      }
    ]
  }'
```

**Audio Format:**

- `data` - Base64-encoded audio data
- `format` - Audio format (currently `wav` supported)

### 10.4 Plain Base64 Format

For simpler integrations, Kronk also accepts plain base64 as the message
content (without the structured OpenAI format):

```json
{
  "model": "Qwen2.5-VL-3B-Instruct-Q8_0",
  "messages": [
    {
      "role": "user",
      "content": "/9j/4AAQSkZJRgABAQEASABIAAD..."
    }
  ]
}
```

Kronk auto-detects the media type from the binary header:

- JPEG: starts with `FF D8 FF`
- PNG: starts with `89 50 4E 47`
- WAV: starts with `RIFF`

### 10.5 Configuration for Multi-Modal Models

Vision and audio models have specific configuration requirements:

```yaml
models:
  Qwen2.5-VL-3B-Instruct-Q8_0:
    n_ubatch: 2048 # Higher for image token processing
    n_seq_max: 2 # Process up to 2 requests concurrently
    context_window: 8192
```

**Key Considerations:**

- `n_ubatch` should be high (≥2048) for efficient image/audio token processing
- `n_seq_max` controls batch parallelism (multiple slots in shared context)
- Vision/audio models use the same batch engine as text models

### 10.6 Memory Requirements

Vision and audio models require additional memory for the projector:

**Vision Model Example (Qwen2.5-VL-3B):**

```
Model weights:     ~3.5 GB
Projector:         ~0.5 GB
KV cache (8K):     ~0.4 GB
─────────────────────────
Total:             ~4.4 GB
```

**Audio Model Example (Qwen2-Audio-7B):**

```
Model weights:     ~8 GB
Projector:         ~0.8 GB
KV cache (8K):     ~0.6 GB
─────────────────────────
Total:             ~9.4 GB
```

### 10.7 IMC and Multi-Modal Caching

IMC fully supports vision and audio models. Media embeddings (images, audio)
are cached in the KV cache alongside text tokens. After each request, the
entire cached prefix — including media embeddings — is snapshotted to RAM
via `StateSeqGetData` and the VRAM sequence is cleared. On the next request,
the cached state is restored from RAM into any available slot, just like
text-only sessions. Media is never re-encoded through the projection model
unless the conversation cache is rebuilt from scratch.

For example, in a multi-turn vision conversation:

1. **First request** (image + question): The image is encoded through the
   projection model and decoded into the KV cache alongside text tokens.
   After generation, the entire cached prefix (text + media KV) is
   snapshotted to RAM.

2. **Follow-up requests** (text-only): The cached state is restored from
   RAM into any available slot. Only new text tokens are decoded — the image
   embeddings are preserved in the restored KV state without re-encoding.

3. **New image in conversation**: If a new message contains media, IMC
   triggers a full rebuild through the mtmd pipeline to re-encode all media.

See [Chapter 5: Message Caching](chapter-05-message-caching.md) for full
details on IMC's caching algorithm.

### 10.8 Limitations

- Processing time varies with image resolution and audio duration

### 10.9 Example: Image Analysis

Complete example analyzing an image:

```shell
# Encode image to base64
IMAGE_B64=$(base64 -i photo.jpg)

# Send request
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2.5-VL-3B-Instruct-Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Describe this image in detail."},
          {
            "type": "image_url",
            "image_url": {"url": "data:image/jpeg;base64,${IMAGE_B64}"}
          }
        ]
      }
    ],
    "max_tokens": 1024
  }'
```

### 10.10 Example: Audio Transcription

Complete example transcribing audio:

```shell
# Encode audio to base64
AUDIO_B64=$(base64 -i recording.wav)

# Send request
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen2-Audio-7B.Q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Transcribe this audio."},
          {
            "type": "input_audio",
            "input_audio": {"data": "${AUDIO_B64}", "format": "wav"}
          }
        ]
      }
    ],
    "max_tokens": 2048
  }'
```

---

_Next: [Chapter 11: Security & Authentication](#chapter-11-security--authentication)_
