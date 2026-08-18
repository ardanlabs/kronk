# Chapter 19: Malina (Image Generation)

## Table of Contents

- [19.1 Overview](#191-overview)
- [19.2 Install Stable Diffusion Libraries](#192-install-stable-diffusion-libraries)
- [19.3 Manage Model Bundles](#193-manage-model-bundles)
- [19.4 Go SDK](#194-go-sdk)
  - [19.4.1 Initialize Malina](#1941-initialize-malina)
  - [19.4.2 Load and Unload a Model](#1942-load-and-unload-a-model)
  - [19.4.3 Generate an Image](#1943-generate-an-image)
- [19.5 Multi-File Model Bundles](#195-multi-file-model-bundles)
- [19.6 Image-to-Image Generation](#196-image-to-image-generation)
- [19.7 Logging, Progress, and Diagnostics](#197-logging-progress-and-diagnostics)
- [19.8 Motion-JPEG Encoding](#198-motion-jpeg-encoding)
- [19.9 Examples](#199-examples)
- [19.10 Current Scope and Limitations](#1910-current-scope-and-limitations)
- [19.11 Troubleshooting](#1911-troubleshooting)

---

**Experimental:** The Malina SDK public API is subject to change.

Malina is Kronk's image-generation SDK. It uses
[`stable-diffusion.cpp`](https://github.com/leejet/stable-diffusion.cpp)
through the [`malina`](https://github.com/ardanlabs/malina) bindings and is
available through the Go packages under `sdk/malina`.

Using Malina requires both a compatible stable-diffusion.cpp library bundle
and a supported model bundle. The libraries provide the native inference
engine. The model bundle contains either one complete checkpoint or the set of
component files required by a diffusion pipeline.

### 19.1 Overview

The current SDK supports:

- text-to-image generation;
- image-to-image generation from a Go `image.Image`;
- single-checkpoint and multi-file diffusion pipelines;
- curated model-bundle download and validation;
- native library detection, installation, and version management;
- application-controlled native logging and progress reporting;
- stable-diffusion.cpp system diagnostics; and
- Motion-JPEG AVI encoding from generated or existing image frames.

The Malina API follows the same high-level shape as the Kronk and Bucky SDKs:

1. Detect and install compatible native libraries.
2. Initialize the process-wide native backend.
3. Download a curated model bundle.
4. Construct a handle for one loaded model.
5. Perform work through the handle.
6. Unload the handle.

Malina is currently an SDK and tooling integration. It is **not yet an
inference backend in the Kronk model server**. There are no Malina HTTP
generation endpoints, CLI management commands, BUI management screens, or
Malina model pool in this release. Model-server integration depends on
reliable memory and VRAM planning for stable-diffusion model bundles.

### 19.2 Install Stable Diffusion Libraries

The normal SDK flow detects the current host, resolves a compatible runtime,
and installs Kronk's pinned stable-diffusion.cpp version:

```go
ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
defer cancel()

libs, err := libs.New(
    libs.WithDetect(ctx, malina.FmtLogger),
)
if err != nil {
    return err
}

if _, err := libs.Download(ctx, malina.FmtLogger); err != nil {
    return err
}
```

Libraries are installed below `~/.kronk/malina-libraries/` by default, using
one directory per operating-system, architecture, and processor combination:

```text
~/.kronk/malina-libraries/<os>/<arch>/<processor>/
```

The published combinations are:

| Operating system | Architecture | Processors |
| ---------------- | ------------ | ---------- |
| macOS | `arm64` | `cpu`, `metal` |
| Linux | `amd64` | `cpu`, `vulkan`, `rocm` |
| Windows | `amd64` | `cpu`, `cuda`, `vulkan`, `rocm` |

Use `libs.SupportedCombinations()` as the source of truth for combinations
available to the installed Kronk version.

Automatic selection treats detected hardware and environment-derived runtime
values as preferences and falls back to a compatible runtime when possible.
The `libs.WithArch`, `libs.WithOS`, and `libs.WithProcessor` options are
authoritative when they form a published combination. An explicit library path
is also authoritative.

The library manager recognizes these path and platform overrides:

| Variable | Purpose |
| -------- | ------- |
| `KRONK_BASE_PATH` | Changes the root below which Kronk-managed files are stored. |
| `KRONK_MALINA_LIB_PATH` | Selects an exact stable-diffusion.cpp library directory. |
| `MALINA_LIB` | Legacy fallback for `KRONK_MALINA_LIB_PATH`. |
| `KRONK_ARCH` | Supplies the preferred architecture during automatic selection. |
| `KRONK_OS` | Supplies the preferred operating system during automatic selection. |
| `KRONK_PROCESSOR` | Supplies the preferred processor during automatic selection; compatibility checks may fall back. |

A selected directory containing `version.json` is treated as a
Kronk-managed installation. A non-empty directory without that file is
treated as a read-only user-managed build: Malina can load from it, but the
library manager will not upgrade, replace, or remove it.

By default, `Download` uses the stable-diffusion.cpp version pinned by the
Malina release that Kronk depends on. This is the supported choice. Advanced
users can request a specific version with `libs.WithVersion`, or opt into the
latest upstream release with:

```go
libs, err := libs.New(
    libs.WithDetect(ctx, malina.FmtLogger),
    libs.WithAllowUpgrade(true),
)
```

An upgraded upstream library may not be ABI-compatible with the Malina and
Kronk versions in use. Use this option for testing, not as a compatibility
guarantee. Library installation is staged and activated atomically so a
failed download does not replace a working installation.

### 19.3 Manage Model Bundles

Kronk provides a small, curated catalog rather than accepting arbitrary model
repository layouts. This keeps component roles and known-compatible files in
the high-level SDK instead of requiring applications to use Malina's raw
download API.

The current bundles are:

| Bundle | Files | License | Approximate download |
| ------ | ----- | ------- | -------------------- |
| `sd-1.5` | Stable Diffusion 1.5 checkpoint | CreativeML Open RAIL-M | 4.3 GB |
| `sdxl-base-1.0` | SDXL Base 1.0 checkpoint | CreativeML Open RAIL++-M | 6.9 GB |
| `flux2-klein-4b` | Diffusion model, VAE, and LLM text encoder | FLUX Non-Commercial | 5.3 GB |
| `flux2-klein-9b` | Diffusion model, VAE, and LLM text encoder | FLUX Non-Commercial | 11 GB |

Use `models.SupportedBundles()` to enumerate names and `models.Catalog()` to
inspect descriptions, licenses, gating, files, and component roles.

Models are installed below `~/.kronk/malina-models/` by default:

```text
~/.kronk/malina-models/<bundle>/
```

Download a single-file bundle with the backend-compatible `Download` method:

```go
mdls, err := models.New()
if err != nil {
    return err
}

mp, err := mdls.Download(
    ctx,
    malina.FmtLogger,
    models.BundleSD15.String(),
)
if err != nil {
    return err
}
```

For a component bundle, use `DownloadBundle` and configure the model from the
returned role-to-path manifest:

```go
manifest, err := mdls.DownloadBundle(ctx, models.BundleFlux2Klein9B)
if err != nil {
    return err
}
```

Downloads are staged, checked for all required non-empty files, recorded in
`manifest.json`, and then activated atomically. A complete installed bundle is
reused on later calls.

The FLUX.2 bundles are license-gated. Accept the model license on Hugging Face,
then provide a read token through `KRONK_HF_TOKEN` or `HF_TOKEN` before
downloading them.

### 19.4 Go SDK

The complete Stable Diffusion 1.5 flow is demonstrated by
[`examples/malina/main.go`](../examples/malina/main.go). The important phases
are initialization, model construction, generation, and unloading.

#### 19.4.1 Initialize Malina

After installing the libraries, initialize the process-wide backend from the
resolved directory:

```go
if err := malina.Init(
    malina.WithLibPath(libs.LibsPath()),
); err != nil {
    return err
}
```

Call `Init` before constructing a Malina handle. Repeated calls using the same
library path are safe. Once initialized, the process cannot switch to a
different stable-diffusion.cpp library directory.

#### 19.4.2 Load and Unload a Model

Construct one handle for one model checkpoint:

```go
mln, err := malina.New(
    model.WithModelPath(mp.ModelFiles[0]),
)
if err != nil {
    return err
}

defer func() {
    if err := mln.Unload(context.Background()); err != nil {
        fmt.Println("unload:", err)
    }
}()
```

Use `NewWithContext` when model loading needs a deadline or cancellation.
`Unload` stops new admission and waits until native work can be released
safely. If its context expires, cleanup continues in the background and a
later `Unload` call can wait for completion.

Each handle serializes generation through one reusable native model context.
Concurrent callers share a default total admitted capacity of 2, including the
running generation, and a three-minute admission timeout. Configure these with
`model.WithQueueDepth` and `model.WithAdmissionTimeout` when constructing the
handle.

#### 19.4.3 Generate an Image

Start with the stable-diffusion.cpp generation defaults and set a prompt:

```go
params := model.NewGenerateParams()
params.Prompt = "a small red sailboat crossing a calm mountain lake at sunrise"
params.Seed = 42

generated, err := mln.Generate(ctx, params)
if err != nil {
    return err
}

if err := os.WriteFile("malina.png", generated.PNG, 0o644); err != nil {
    return err
}
```

`GeneratedImage` contains an owned PNG byte slice, output dimensions, and the
requested seed value. When `-1` asks the native backend to select a random
seed, the result retains `-1`; it does not expose the effective native seed.

The default parameters are:

| Parameter | Default |
| --------- | ------- |
| Width | 512 |
| Height | 512 |
| Steps | 20 |
| CFG scale | 7 |
| Seed | `-1` (random) |
| Image-to-image strength | 0.75 |

Dimensions must be multiples of 8 from 64 through 1024, with no more than
1,048,576 total pixels. A request needs a non-empty prompt, positive finite
CFG scale, and between 1 and 1,000 steps.

Waiting for admission is cancellable. Once native generation starts, Malina
waits for it to finish before returning a cancellation error; the native
context cannot safely be reused or freed while stable-diffusion.cpp is still
working.

### 19.5 Multi-File Model Bundles

Some pipelines require several files with distinct roles. The FLUX.2 example
downloads a manifest and maps its diffusion, VAE, and LLM components into the
model configuration:

```go
manifest, err := mdls.DownloadBundle(ctx, models.BundleFlux2Klein9B)
if err != nil {
    return err
}

mln, err := malina.New(
    model.WithDiffusionModelPath(manifest.Files[string(models.RoleDiffusion)]),
    model.WithVAEPath(manifest.Files[string(models.RoleVAE)]),
    model.WithLLMPath(manifest.Files[string(models.RoleLLM)]),
)
if err != nil {
    return err
}
```

Use the exported `models.Role...` values instead of relying on filenames or
file ordering. This keeps application configuration tied to the curated
bundle contract.

### 19.6 Image-to-Image Generation

Set `GenerateParams.InitImage` to transform an existing Go image. The request
still needs a prompt, and `Strength` must be greater than zero and no more than
one:

```go
params := model.NewGenerateParams()
params.Prompt = "a watercolor painting at sunset"
params.InitImage = source
params.Strength = 0.6
params.Width = 768
params.Height = 512

generated, err := mln.Generate(ctx, params)
```

The requested dimensions are the generation dimensions; prepare the source
image and choose valid dimensions for the desired aspect ratio. The
[`malina-img2img`](../examples/malina-img2img/main.go) example accepts PNG and
JPEG input, constrains the image to 1024 pixels per side, and rounds dimensions
down to multiples of 8.

### 19.7 Logging, Progress, and Diagnostics

Native stable-diffusion.cpp and GGML diagnostics are silent by default. Enable
them when diagnosing native behavior:

```go
malina.Init(
    malina.WithLibPath(libs.LibsPath()),
    malina.WithLogLevel(malina.LogNormal),
)
```

Model-loading and generation progress uses the native terminal display by
default. Applications can replace it with a callback:

```go
func progress(step int, steps int, secondsPerStep float32) {
    fmt.Printf("\r%d/%d %.2fs/step", step, steps, secondsPerStep)
}

malina.Init(
    malina.WithLibPath(libs.LibsPath()),
    malina.WithProgress(progress),
)
```

Progress callbacks are process-wide and may be invoked concurrently. Protect
shared output or UI state as needed. Use
`malina.WithProgress(malina.DiscardProgress)` to suppress progress completely.

After initialization, inspect the native backend and host with:

```go
info, err := malina.SystemInfo()
if err != nil {
    return err
}

fmt.Println(info.NativeVersion)
fmt.Println(info.PhysicalCores)
fmt.Println(info.BackendDeviceCount)
fmt.Println(info.Description)
```

A loaded handle also exposes `SystemInfo`, `ModelInfo`, `ModelConfig`,
`ActiveGenerations`, and `Ready` for application observability.

### 19.8 Motion-JPEG Encoding

`model.SaveAVI` writes same-sized Go images as a Motion-JPEG AVI without
loading stable-diffusion.cpp or a model:

```go
if err := model.SaveAVI("output.avi", frames, 24, 90); err != nil {
    return err
}
```

The frame rate must be positive, JPEG quality must be from 1 through 100, and
all frames must have the same dimensions. This helper encodes an in-memory
frame sequence; it is not a streaming video-generation API.

### 19.9 Examples

The examples install their own compatible libraries and model bundles using
default Kronk paths. They do not require path environment variables.

| Command | Purpose |
| ------- | ------- |
| `make example-malina` | Generate a deterministic Stable Diffusion 1.5 PNG. |
| `make example-malina-flux2` | Generate a PNG with the multi-file FLUX.2 Klein 9B bundle. |
| `make example-malina-img2img` | Transform a PNG or JPEG with a prompt and strength. |
| `make example-malina-sd-encode` | Encode a directory of PNG/JPEG frames as Motion-JPEG AVI. |
| `make example-malina-system` | Install libraries and print native system diagnostics. |

The first run of an inference example can take significant time and disk
space because it downloads native libraries and multi-gigabyte model files.
Later runs reuse complete installations.

### 19.10 Current Scope and Limitations

- The public API is experimental and may change between Kronk releases.
- Malina image inference is available through the Go SDK, not the Kronk model
  server or its HTTP API.
- Malina models are not managed by the shared Kronk/Bucky model pool and do
  not yet participate in model-server RAM or VRAM admission and eviction.
- The curated catalog is intentionally small. The high-level SDK guarantees
  its listed component roles; arbitrary user-created bundle layouts are not a
  supported catalog contract.
- Native callbacks, backend initialization, model-context construction,
  generation, and destruction have process-wide synchronization constraints.
  Multiple handles are safe to construct, but native operations are currently
  serialized conservatively.
- Native generation cannot be interrupted safely after it begins. Context
  cancellation prevents queued work and controls the returned result, but it
  does not free an active native context early.

### 19.11 Troubleshooting

| Symptom | Action |
| ------- | ------ |
| `Init` cannot find or load stable-diffusion.cpp | Construct `libs.New` with detection, call `Download`, and pass `libs.LibsPath()` to `malina.Init`. |
| No compatible bundle is published for the host | Check `libs.SupportedCombinations()`; use one of the published operating-system, architecture, and processor triples. |
| A custom library directory cannot be upgraded or removed | A non-empty directory without `version.json` is intentionally read-only. Manage that build yourself or use a Kronk-managed install directory. |
| A gated FLUX.2 download returns 401 or 403 | Accept the Hugging Face license and set `KRONK_HF_TOKEN` or `HF_TOKEN` to a token with read access. |
| Generation reports an invalid request | Start with `model.NewGenerateParams`; provide a prompt and valid dimensions, steps, CFG scale, and image-to-image strength. |
| Generation waits and then returns an admission timeout | Other calls are using the handle's admitted capacity. Increase admitted capacity or the admission timeout only when the application can tolerate the additional work or wait. |
| Native diagnostic output is too verbose | Remove `malina.WithLogLevel(malina.LogNormal)`; native logging is silent by default. |
| Progress output is unwanted | Pass `malina.WithProgress(malina.DiscardProgress)` during initialization. |
| A different library path is rejected after initialization | Native initialization is process-wide. Restart the process to load a different stable-diffusion.cpp installation. |

---

Next: [Chapter 20: Developer Guide](https://www.kronkai.com/manual#chapter-20-developer-guide)
