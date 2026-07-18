<!--
  This file is the Docker Hub repository description for
  https://hub.docker.com/r/ardanlabs/kronk. It is NOT auto-synced —
  after editing, paste the rendered contents into the Docker Hub UI
  (Repository → Settings → Description).

  SYNC CHECKLIST when changing publishing/signing behaviour:
    - .github/workflows/docker.yml  (the `Sign published manifests`
      step — cosign keyless config, COSIGN_REPOSITORY, etc.)
    - .manual/chapter-19-developer-guide.md  (the "Merge + sign"
      section under the Docker workflow notes)
    - This file — the "Image Tags You'll See" and "Verifying Image
      Signatures" sections below. Keep the verify commands and the
      signatures-repo name in sync with the workflow.
-->

# Kronk — Local LLM Inference & Audio Transcription Server

Kronk is a high-performance, GPU-accelerated server for local LLM inference and audio transcription. Built on llama.cpp and whisper.cpp, it provides an OpenAI-compatible API for chat completions, responses, embeddings, reranking, and audio transcription.

## Quick Start

Run the server with CPU inference:

```bash
docker run --rm -p 11435:11435 -v kronk-data:/kronk ghcr.io/ardanlabs/kronk:latest
```

Check that it's running:

```bash
curl http://localhost:11435/v1/liveness
```

Models download automatically on first use.

---

## Available Images

| Tag             | Description                                       | Architectures                |
|-----------------|---------------------------------------------------|------------------------------|
| `latest`        | CPU-only, runs everywhere (alias of `latest-cpu`) | `linux/amd64`, `linux/arm64` |
| `latest-cpu`    | CPU-only (smallest image, runs everywhere)        | `linux/amd64`, `linux/arm64` |
| `latest-cuda`   | NVIDIA CUDA                                       | `linux/amd64`, `linux/arm64` |
| `latest-vulkan` | Vulkan (works on AMD, Intel, NVIDIA)              | `linux/amd64`, `linux/arm64` |
| `latest-rocm`   | AMD ROCm                                          | `linux/amd64`                |
| `latest-all`    | All backends bundled (CPU, CUDA, Vulkan, ROCm)    | `linux/amd64`, `linux/arm64` |

**Tag scheme:**

- `latest-<variant>` (e.g. `latest-cuda`) floats to the newest release of that
  variant. `latest` is an alias of `latest-cpu`.
- `latest` is the **CPU** image — for GPU acceleration use the matching
  `latest-cuda` / `latest-vulkan` / `latest-rocm` tag.
- Every variant also has an immutable per-release tag, `<version>-<variant>`
  (e.g. `1.28.8-cuda`). **Pin these in production** so restarts are
  reproducible.
- **NVIDIA Jetson (Orin / Xavier):** published as `:jetson`. It is built on
  demand via the `--target runtime-jetson` path documented in the
  [Dockerfile](https://github.com/ardanlabs/kronk/blob/main/zarf/docker/kronk/Dockerfile),
  not as part of the automatic build matrix. See the run example below.

**Development snapshots** (bleeding edge, not for production) are published
from every push to `main` in a separate repo,
[`ardanlabs/kronk-main`](https://hub.docker.com/r/ardanlabs/kronk-main), as
`main-<sha>-<variant>` (immutable per-commit) and `main-latest-<variant>`
(floating). They are cosign-signed the same way as releases, with signatures
in [`ardanlabs/kronk-main-signatures`](https://hub.docker.com/r/ardanlabs/kronk-main-signatures).

---

## Platform Support

| Host OS         | GPU       | Image             | Backend | Status                                                 |
| --------------- | --------- | ----------------- | ------- | ------------------------------------------------------ |
| **Linux**       | NVIDIA    | `:latest-cuda`    | CUDA    | ✅ Fully supported                                     |
| **Linux**       | AMD       | `:latest-rocm`    | ROCm    | ✅ Fully supported                                     |
| **Linux**       | AMD       | `:latest-vulkan`  | Vulkan  | ✅ Fully supported                                     |
| **Linux**       | Intel     | `:latest-vulkan`  | Vulkan  | ✅ Fully supported                                     |
| **Linux**       | NVIDIA    | `:latest-vulkan`  | Vulkan  | ✅ Fully supported                                     |
| **Linux**       | None      | `:latest`         | CPU     | ✅ Fully supported                                     |
| **Linux arm64** | Jetson    | `:jetson`         | CUDA    | ✅ Fully supported (built on demand — see tag scheme)  |
| **Linux arm64** | SoC iGPU  | `:latest-vulkan`  | Vulkan  | ✅ Fully supported                                     |
| **macOS**       | Any       | `:latest`         | CPU     | ⚠️ Works (Apple Silicon GPU not exposed to containers) |
| **Windows**     | NVIDIA    | `:latest-cuda`    | CUDA    | ✅ Supported (Docker Desktop + WSL2)                   |
| **Windows**     | AMD/Intel | `:latest`         | CPU     | ⚠️ Works (Vulkan via WSL2 unreliable for inference)    |
| **Windows**     | None      | `:latest`         | CPU     | ✅ Fully supported                                     |

**Legend:**

- ✅ Fully supported with GPU acceleration
- ⚠️ Works with significant caveats

---

## GPU Run Examples

### NVIDIA GPU

Requires [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html).
Pass `--runtime=nvidia` unless the NVIDIA runtime is already the default in
`/etc/docker/daemon.json` (passing it when it already is the default is
harmless). The CUDA runtime libraries are baked into the `:latest-cuda`
image — the container runtime injects only the driver, so `nvidia-smi`
working inside the container does not by itself mean inference is
GPU-accelerated.

```bash
docker run --rm --runtime=nvidia --gpus all -p 11435:11435 -v kronk-data:/kronk ghcr.io/ardanlabs/kronk:latest-cuda
```

Specific GPUs by index:

```bash
docker run --rm --runtime=nvidia --gpus '"device=0,1"' -p 11435:11435 -v kronk-data:/kronk ghcr.io/ardanlabs/kronk:latest-cuda
```

Specific GPUs by UUID:

```bash
docker run --rm --runtime=nvidia --gpus '"device=GPU-3a1f...,GPU-9b2e..."' -p 11435:11435 -v kronk-data:/kronk ghcr.io/ardanlabs/kronk:latest-cuda
```

### AMD GPU (ROCm)

Requires `/dev/kfd` and `/dev/dri` access. The user running Docker must be in the `render` and `video` groups.

```bash
docker run --rm \
  --device=/dev/kfd --device=/dev/dri \
  --group-add video --group-add render \
  --security-opt seccomp=unconfined \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest-rocm
```

### Vulkan (AMD, Intel, NVIDIA)

A single image that works across GPU vendors. Requires Mesa Vulkan drivers on the host.

```bash
docker run --rm --device=/dev/dri --group-add video --group-add render \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest-vulkan
```

**Vulkan on NVIDIA:** Requires NVIDIA Container Toolkit with graphics capabilities:

```bash
docker run --rm --runtime=nvidia --gpus all \
  -e NVIDIA_DRIVER_CAPABILITIES=compute,utility,graphics \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest-vulkan
```

**Vulkan on Intel:** Expose `/dev/dri` (ANV driver):

```bash
docker run --rm --device=/dev/dri --group-add video --group-add render \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest-vulkan
```

**Multi-GPU selection:** Pin a specific GPU with environment variables:

```bash
# Select by PCI ID (format: <vendor>:<device>)
# 1002 = AMD, 8086 = Intel, 10de = NVIDIA
docker run --rm --device=/dev/dri --group-add video --group-add render \
  -e MESA_VK_DEVICE_SELECT='1002:744c' \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest-vulkan
```

Verify Vulkan devices inside the container:

```bash
docker exec -it <container> vulkaninfo --summary
```

### NVIDIA Jetson (Orin / Xavier)

Requires JetPack 6+ and `nvidia-container-runtime` configured as the default runtime.

```bash
docker run --rm --runtime nvidia \
  -e NVIDIA_DRIVER_CAPABILITIES=compute,utility,graphics \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:jetson
```

Verify CUDA detection:

```bash
docker run --rm --runtime nvidia \
  --entrypoint nvidia-smi \
  ghcr.io/ardanlabs/kronk:jetson
```

---

## Audio Transcription (Bucky)

Every image includes whisper.cpp shared libraries and `ffmpeg` for decoding non-PCM audio uploads (WebM/Opus, MP4/AAC, OGG, M4A).

Pull a Whisper model (models are ~50-500MB, not baked into images):

```bash
docker exec -it <container> kronk bucky model pull ggml-tiny.bin
```

Transcribe an audio file:

```bash
curl -X POST http://localhost:11435/v1/audio/transcriptions \
  -F file=@samples/jfk.wav \
  -F model=ggml-tiny.bin \
  -F response_format=json
```

---

## Ports

| Port    | Service                                                               |
| ------- | --------------------------------------------------------------------- |
| `11435` | Main API (chat completions, embeddings, models, `/v1/liveness`, etc.) |
| `11445` | Debug server (Prometheus `/metrics`, pprof, statsviz)                 |

To keep the debug server local-only, override `KRONK_WEB_DEBUG_HOST=127.0.0.1:11445`.

---

## Volume & Persistence

Mount a volume on `/kronk` to persist models, catalog, keys, and libraries across container restarts:

```bash
docker run --rm -p 11435:11435 -v kronk-data:/kronk ghcr.io/ardanlabs/kronk:latest
```

Models, libraries, catalog data, and API keys are all stored under `/kronk`. Named volumes are recommended — bind mounts require `chown 10001:10001` on the host directory (the container runs as UID/GID 10001).

---

## Environment Variables

| Variable                       | Default                        | Description                                                                          |
| ------------------------------ | ------------------------------ | ------------------------------------------------------------------------------------ |
| `KRONK_DOWNLOAD_ENABLED`       | `false`                        | Allow model/library downloads from the browser UI (disabled by default for security) |
| `KRONK_WEB_API_HOST`           | `0.0.0.0:11435`                | API bind address                                                                     |
| `KRONK_WEB_DEBUG_HOST`         | `:11445`                       | Debug server bind address                                                            |
| `KRONK_POOL_MODEL_CONFIG_FILE` | `/etc/kronk/model_config.yaml` | Path to model configuration file                                                     |
| `KRONK_BASE_PATH`              | `/kronk`                       | Base path for all Kronk data                                                         |

---

## Verifying Image Signatures

Every published manifest is signed with [cosign](https://github.com/sigstore/cosign) **keyless** (Sigstore / Fulcio / Rekor) by the [`docker.yml`](https://github.com/ardanlabs/kronk/blob/main/.github/workflows/docker.yml) GitHub Actions workflow. There are no long-lived signing keys — the signature is bound to the workflow identity and recorded in the public Rekor transparency log.

To avoid cluttering the `ardanlabs/kronk` tag list with `sha256-*.sig` entries, **Docker Hub signatures are stored in a sibling repository**, [`ardanlabs/kronk-signatures`](https://hub.docker.com/r/ardanlabs/kronk-signatures). Verifiers must point cosign at that repo via `COSIGN_REPOSITORY`:

```bash
COSIGN_REPOSITORY=ardanlabs/kronk-signatures \
  cosign verify ardanlabs/kronk:latest \
    --certificate-identity-regexp \
      'https://github.com/ardanlabs/kronk/.github/workflows/docker.yml@.*' \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The mirror images on GHCR keep signatures co-located (the default cosign layout), so verifying those does not need `COSIGN_REPOSITORY`:

```bash
cosign verify ghcr.io/ardanlabs/kronk:latest \
  --certificate-identity-regexp \
    'https://github.com/ardanlabs/kronk/.github/workflows/docker.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Substitute `latest` with any published tag (`<version>-<variant>`, `latest-<variant>`, etc.). A successful verification prints the Fulcio cert, the GitHub Actions workflow identity, and the Rekor log entry index.

---

## Links

- **Source:** https://github.com/ardanlabs/kronk
- **Headless remote deployment guide:** https://github.com/ardanlabs/kronk/blob/main/.manual/chapter-02-installation.md#24-docker--oci-container
- **Documentation:** https://github.com/ardanlabs/kronk#readme
- **Website:** https://kronkai.com
- **Signatures (Docker Hub):** https://hub.docker.com/r/ardanlabs/kronk-signatures
