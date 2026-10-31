# Chapter 19: Malina (Image Generation)

## Table of Contents

- [19.1 Overview](#191-overview)
- [19.2 Install Stable Diffusion Libraries](#192-install-stable-diffusion-libraries)
- [19.3 Manage Model Bundles](#193-manage-model-bundles)
- [19.4 Server Configuration and Resources](#194-server-configuration-and-resources)
- [19.5 Browser UI](#195-browser-ui)
- [19.6 Image Generation and Editing API](#196-image-generation-and-editing-api)
- [19.7 Progress and Administration](#197-progress-and-administration)
- [19.8 Limitations and Developer Resources](#198-limitations-and-developer-resources)
- [19.9 Troubleshooting](#199-troubleshooting)

---

**Experimental:** Malina's public SDK API is subject to change.

### 19.1 Overview

Malina is Kronk's image-generation subsystem, powered by
[stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp). It needs
both compatible native libraries and a supported model bundle. A bundle can be
a single checkpoint or several pipeline components; downloading a language-model
GGUF does not supply an image-generation pipeline.

| Surface | Supported workflows |
| ------- | ------------------- |
| Model server and BUI | Text-to-image and image-to-image; PNG results and server-wide progress |
| Local CLI | Library and curated model-bundle installation, listing, and removal |
| SDK | Image generation/editing plus ControlNet, ADetailer, video generation, upscaling, and Motion-JPEG encoding |

The server shares memory admission and retention controls with the language
and Bucky backends. Advanced SDK features are not additional HTTP routes.

### 19.2 Install Stable Diffusion Libraries

Install the pinned bundle for the current host:

```shell
kronk malina libs --local
kronk malina libs --list-combinations --local
kronk malina libs --list-installs --local
```

The Malina CLI currently requires `--local`. Server-side management is available
through the BUI and `/v1/malina/...` APIs, not through default CLI web mode.
Run `kronk malina libs --help` for version selection, installation for another
platform triple, and removal.

Libraries normally live under
`~/.kronk/malina-libraries/<os>/<arch>/<processor>/`. Supported combinations
for the pinned release include:

| Operating system | Architecture | Processors |
| ---------------- | ------------ | ---------- |
| macOS | `arm64` | `cpu`, `metal` |
| Linux | `amd64` | `cpu`, `vulkan`, `rocm` |
| Windows | `amd64` | `cpu`, `cuda`, `vulkan`, `rocm` |

Use `--list-combinations` for the installed release's current list. The macOS
`cpu` and `metal` choices use the same Metal-capable archive; selecting its CPU
directory alone does not force CPU execution. This pin has no Linux CUDA or
Linux/Windows arm64 bundle.

| Variable | Purpose |
| -------- | ------- |
| `KRONK_BASE_PATH` | Select the data root without migrating existing files |
| `KRONK_MALINA_LIB_PATH` | Select an exact stable-diffusion.cpp library directory |
| `MALINA_LIB` | Legacy fallback for the Malina library path |
| `KRONK_ARCH`, `KRONK_OS`, `KRONK_PROCESSOR` | Runtime-selection preferences; compatibility checks can select a fallback |

An explicit library path is authoritative. A nonempty directory without
`version.json` is a read-only user-managed build, not a managed installation.
Default verified server startup requires managed metadata and can reject it.
Using independently trusted custom builds requires an advanced verification
exception; do not disable verification as routine recovery.

Use the libraries pinned to this Kronk release. Newer native libraries are not
necessarily ABI-compatible; upgrade bindings and libraries together. Offline
managed installations must match the requested pin and platform. Installation
is staged so a failed download does not replace the working bundle. Restart
after changing the selected libraries; the process does not hot-reload them.

With default verification enabled, installation or verification failures stop
server startup. Repair the managed bundle locally, then restart. A later native
initialization failure can instead leave Malina unavailable while the server
continues; BUI repair is useful only if the server actually remains running.

### 19.3 Manage Model Bundles

Use the curated catalog rather than an arbitrary repository layout:

```shell
kronk malina model catalog --local
kronk malina model pull sd-1.5 --local
kronk malina model list --local
kronk malina model remove sd-1.5 --local
```

Models normally live under `~/.kronk/malina-models/<bundle>/`. The catalog
reports component files, licenses, gating, and download sizes. Budget disk for
multi-gigabyte downloads and installation staging, not just inference memory.
Complete installations are reused on subsequent runs.

| Bundle | Intended use |
| ------ | ------------ |
| `sd-1.5`, `sdxl-base-1.0` | Single-checkpoint image generation |
| `z-image-turbo`, `llada-image-turbo`, `moss-image-sft` | Component-based image generation |
| `flux2-klein-4b`, `flux2-klein-9b` | Component-based, license-gated image generation |
| `controlnet-canny-sd1.5`, `z-image-turbo-controlnet-union` | SDK ControlNet workflows |
| `adetailer-face-yolov8n` | SDK face refinement |
| `animatediff-sd1.5`, `wan2.2-s2v-14b` | SDK video workflows |
| `realesrgan-x4-anime` | SDK upscaling |

For gated FLUX.2 bundles, accept the provider's license and supply a read token
through `KRONK_HF_TOKEN` or `HF_TOKEN`. Review each model's license for your use;
availability in the catalog is not a grant of commercial rights.

Catalog selection does not change generation defaults. Apply model-specific
recommendations explicitly in HTTP fields or the BUI:

- **Z-Image-Turbo:** 8 steps and CFG scale 1.
- **LLaDA-Image-Turbo:** 4 steps and CFG scale 1. Its distinct native
  reference-image instruction-editing path is not exposed by Kronk.
- **MOSS-Image-SFT:** dimensions divisible by 16 and CFG scale 3. The curated
  bundle has not been locally inference-validated.

The server maps the required component roles for supported generation bundles;
operators do not need to configure individual component paths.

### 19.4 Server Configuration and Resources

Enable media backends and restart after installation.
`--media-backends-enabled=false` / `KRONK_MEDIA_BACKENDS_ENABLED=false` skips
both Malina and Bucky downloads and native initialization. Installing files
alone does not override this setting.

Configure bundles under `malina-models` in the versioned model configuration:

```yaml
version: 2
malina-models:
  sd-1.5:
    concurrency: 1
    queue-depth: 2
    admission-timeout: 3m
    cpu-threads: 0
```

Merge this mapping into the existing file rather than replacing its other
sections. `concurrency` defaults to 1. Each additional slot loads another
native model context and increases RAM/VRAM use substantially. Queue depth
adds waiting requests without adding execution capacity; it defaults to 0.
Additional callers can wait for admission up to the configured timeout,
default three minutes. Restart after changing configuration.

The shared server pool defaults to ten entries per backend with idle expiry
disabled. Memory pressure and count limits can still evict idle Malina models.
The three backend pools share a budget but evict only their own idle entries;
an idle language model is not automatically unloaded to admit an image model.
See [Chapter 8](https://www.kronkai.com/manual#84-model-pool-and-resource-budgets).

### 19.5 Browser UI

Use the library and model-management pages to prepare a compatible installation.
In **Image Generator**, select an installed generation model, enter a prompt,
and set dimensions, steps, CFG scale, and seed. Upload a PNG/JPEG to select
image-to-image and choose strength. The result shows dimensions and can be
downloaded as PNG.

The activity panel reports server-wide loading and generation progress, not
only the current browser's request. See Chapter 13 for browser authentication.

### 19.6 Image Generation and Editing API

All image routes follow inference authorization. `authenticated` requires a
valid token; `full-protected` additionally requires `image-generations`.
Inference is public in `open` and `management` modes.

Create an image with JSON:

```shell
curl http://localhost:11435/v1/images/generations \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "sd-1.5",
    "prompt": "A lighthouse during a thunderstorm",
    "size": "512x512",
    "steps": 20,
    "cfg_scale": 7,
    "seed": 42,
    "response_format": "b64_json"
  }'
```

Omit Authorization for an unprotected local server. Unknown JSON fields are
rejected. Request controls are:

| Field | Default | Contract |
| ----- | ------- | -------- |
| `model` | Required | Installed generation bundle ID |
| `prompt` | Required | Nonempty text prompt |
| `negative_prompt` | Empty | Undesired features |
| `size` | `512x512` for generation | `WIDTHxHEIGHT`; multiples of 8, each 64–1024, at most 1,048,576 pixels |
| `steps` | `20` | 1–1,000 |
| `cfg_scale` | `7` | Positive finite value; follow model recommendations |
| `seed` | `-1` | `-1` requests random selection |
| `n` | `1` | Only one image is supported |
| `response_format` | `b64_json` | Only base64 PNG output is supported; no URL response |

The response has `created` and a one-element `data` array containing
`b64_json`, `seed`, `width`, and `height`. Decode `b64_json` to save the PNG.
The returned seed retains the requested value, including `-1`; it does not
expose a resolved random seed. Do not expect identical pixels across model,
backend, or native-library changes.

Transform an image with multipart form fields:

```shell
curl http://localhost:11435/v1/images/edits \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -F model=sd-1.5 \
  -F prompt='A watercolor illustration at sunset' \
  -F image=@source.png \
  -F strength=0.6 \
  -F steps=20
```

Editing accepts the same generation controls plus required PNG/JPEG `image`
and optional `strength` (default `0.75`, greater than zero through 1). Uploads
are limited to 25 MiB and decoded images to 32 Mi pixels. Without `size`, the
server preserves aspect ratio, scales down to at most 1024 pixels per side,
and rounds dimensions down to multiples of eight. Explicit dimensions resize
the source to that size. The response has the same base64 PNG shape.

Both operations use the server's inference deadline. Cancellation requests
native work to stop but can take time to finish; it is not a guarantee of
immediate resource release.

### 19.7 Progress and Administration

`GET /v1/images/events` is an SSE stream reporting `scope: "global"`, `step`,
`steps`, `percent`, and `seconds_per_step`. Progress is process-global and not
request-correlated; it must not be treated as the completion signal for an
individual request. Use the POST response for that result.

Management APIs require administrator access in every authorization mode except
`open`:

| Method and path | Purpose |
| --------------- | ------- |
| `GET /v1/malina/libs` | Active native library installation |
| `GET /v1/malina/libs/integrity` | Verify the selected library bundle |
| `GET /v1/malina/libs/combinations` | Supported platform combinations |
| `GET /v1/malina/libs/installs` | Installed bundles |
| `POST /v1/malina/libs/pull` | Install libraries |
| `DELETE /v1/malina/libs/installs` | Remove an installation |
| `GET /v1/malina/models` | Installed model bundles |
| `GET /v1/malina/models/catalog` | Curated model catalog |
| `POST /v1/malina/models/pull` | Download a bundle |
| `DELETE /v1/malina/models/{model}` | Remove a model bundle |

See Chapter 9 for API conventions and Chapter 15 for server logs and metrics.

### 19.8 Limitations and Developer Resources

The server exposes basic text-to-image and image-to-image only. ControlNet,
ADetailer, video, upscaling, and AVI encoding remain SDK workflows. A feature
in upstream stable-diffusion.cpp does not establish a supported Kronk workflow
or a curated, licensed model bundle. The catalog is intentionally bounded.

Use the generated Malina SDK reference and `examples/malina*` for application
development. Source-checkout targets include `make example-malina`,
`make example-malina-img2img`, `make example-malina-controlnet`,
`make example-malina-adetailer`, `make example-malina-animatediff`,
`make example-malina-s2v`, and `make example-malina-upscale`. These can download
multi-gigabyte model bundles and execute native inference. They are not
required steps for operating the HTTP server.

### 19.9 Troubleshooting

| Symptom | Action |
| ------- | ------ |
| Startup fails during library installation or verification | Install the pinned managed bundle locally, inspect the error, then restart. Do not routinely disable verification. |
| No compatible host bundle | Check `kronk malina libs --list-combinations --local`; use a published platform/backend. |
| Files exist but image inference is unavailable | Enable media backends and restart; inspect native initialization errors. |
| Custom libraries cannot be upgraded or removed | Directories without managed metadata are read-only; manage them independently. |
| Gated download returns 401/403 | Accept the provider's license and supply a read token privately. |
| Invalid generation request | Check installed bundle ID, prompt, dimensions, steps, CFG scale, and edit strength. |
| Black/white images or NaN diagnostics | Verify the pinned library/model combination and model-specific guidance; report backend, format, and settings rather than changing unrelated defaults. |
| Admission timeout or server busy | Inspect running work and shared memory; reduce concurrency/sizing or unload unused models. Increase waiting only if callers can tolerate it. |
| Library path changed but behavior did not | Restart; native libraries are not hot-reloaded. |

---

Next: [Chapter 20: Developer Guide](https://www.kronkai.com/manual#chapter-20-developer-guide)
