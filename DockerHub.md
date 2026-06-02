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

| Tag              | Description                                | Architectures                |
|------------------|--------------------------------------------|------------------------------|
| `latest` / `all` | All backends (CPU, CUDA, Vulkan, ROCm)     | `linux/amd64`, `linux/arm64` |
| `cpu`            | CPU-only (smallest image, runs everywhere) | `linux/amd64`, `linux/arm64` |
| `cuda`           | NVIDIA CUDA only                           | `linux/amd64`, `linux/arm64` |
| `vulkan`         | Vulkan (works on AMD, Intel, NVIDIA)       | `linux/amd64`                |
| `rocm`           | AMD ROCm only                              | `linux/amd64`                |
| `jetson`         | NVIDIA Jetson Orin/Xavier (L4T runtime)    | `linux/arm64`                |

---

## Platform Support

| Host OS         | GPU       | Image              | Backend | Status                                                 |
|-----------------|-----------|--------------------|---------|--------------------------------------------------------|
| **Linux**       | NVIDIA    | `:cuda`            | CUDA    | ✅ Fully supported                                      |
| **Linux**       | AMD       | `:rocm`            | ROCm    | ✅ Fully supported                                      |
| **Linux**       | AMD       | `:vulkan`          | Vulkan  | ✅ Fully supported                                      |
| **Linux**       | Intel     | `:vulkan`          | Vulkan  | ✅ Fully supported                                      |
| **Linux**       | NVIDIA    | `:vulkan`          | Vulkan  | ✅ Fully supported                                      |
| **Linux**       | None      | `:cpu` / `:latest` | CPU     | ✅ Fully supported                                      |
| **Linux arm64** | Jetson    | `:jetson`          | CUDA    | ✅ Fully supported                                      |
| **Linux arm64** | SoC iGPU  | `:vulkan`          | Vulkan  | ✅ Fully supported                                      |
| **macOS**       | Any       | `:latest`          | CPU     | ⚠️ Works (Apple Silicon GPU not exposed to containers) |
| **Windows**     | NVIDIA    | `:cuda`            | CUDA    | ✅ Supported (Docker Desktop + WSL2)                    |
| **Windows**     | AMD/Intel | `:cpu`             | CPU     | ⚠️ Works (Vulkan via WSL2 unreliable for inference)    |
| **Windows**     | None      | `:cpu`             | CPU     | ✅ Fully supported                                      |

**Legend:**
- ✅ Fully supported with GPU acceleration
- ⚠️ Works with significant caveats

---

## GPU Run Examples

### NVIDIA GPU

Requires [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html).

```bash
docker run --rm --gpus all -p 11435:11435 -v kronk-data:/kronk ghcr.io/ardanlabs/kronk:latest
```

Specific GPUs by index:

```bash
docker run --rm --gpus '"device=0,1"' -p 11435:11435 -v kronk-data:/kronk ghcr.io/ardanlabs/kronk:latest
```

Specific GPUs by UUID:

```bash
docker run --rm --gpus '"device=GPU-3a1f...,GPU-9b2e..."' -p 11435:11435 -v kronk-data:/kronk ghcr.io/ardanlabs/kronk:latest
```

### AMD GPU (ROCm)

Requires `/dev/kfd` and `/dev/dri` access. The user running Docker must be in the `render` and `video` groups.

```bash
docker run --rm \
  --device=/dev/kfd --device=/dev/dri \
  --group-add video --group-add render \
  --security-opt seccomp=unconfined \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:rocm
```

### Vulkan (AMD, Intel, NVIDIA)

A single image that works across GPU vendors. Requires Mesa Vulkan drivers on the host.

```bash
docker run --rm --device=/dev/dri --group-add video --group-add render \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:vulkan
```

**Vulkan on NVIDIA:** Requires NVIDIA Container Toolkit with graphics capabilities:

```bash
docker run --rm --gpus all \
  -e NVIDIA_DRIVER_CAPABILITIES=compute,utility,graphics \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:vulkan
```

**Vulkan on Intel:** Expose `/dev/dri` (ANV driver):

```bash
docker run --rm --device=/dev/dri --group-add video --group-add render \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:vulkan
```

**Multi-GPU selection:** Pin a specific GPU with environment variables:

```bash
# Select by PCI ID (format: <vendor>:<device>)
# 1002 = AMD, 8086 = Intel, 10de = NVIDIA
docker run --rm --device=/dev/dri --group-add video --group-add render \
  -e MESA_VK_DEVICE_SELECT='1002:744c' \
  -p 11435:11435 -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:vulkan
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
docker exec -it <container> kronk bucky model pull tiny.en
```

Transcribe an audio file:

```bash
curl -X POST http://localhost:11435/v1/audio/transcriptions \
  -F file=@samples/jfk.wav \
  -F model=tiny.en \
  -F response_format=json
```

---

## Ports

| Port    | Service                                                               |
|---------|-----------------------------------------------------------------------|
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
|--------------------------------|--------------------------------|--------------------------------------------------------------------------------------|
| `KRONK_DOWNLOAD_ENABLED`       | `false`                        | Allow model/library downloads from the browser UI (disabled by default for security) |
| `KRONK_WEB_API_HOST`           | `0.0.0.0:11435`                | API bind address                                                                     |
| `KRONK_WEB_DEBUG_HOST`         | `:11445`                       | Debug server bind address                                                            |
| `KRONK_POOL_MODEL_CONFIG_FILE` | `/etc/kronk/model_config.yaml` | Path to model configuration file                                                     |
| `KRONK_BASE_PATH`              | `/kronk`                       | Base path for all Kronk data                                                         |

---

## Links

- **Source:** https://github.com/ardanlabs/kronk
- **Documentation:** https://github.com/ardanlabs/kronk#readme
- **Website:** https://kronkai.com
