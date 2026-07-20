# Chapter 2: Installation & Quick Start

## Table of Contents

- [2.1 Native Quick Start](#21-native-quick-start)
- [2.2 Choose an Installation Method](#22-choose-an-installation-method)
- [2.3 Container Quick Start](#23-container-quick-start)
- [2.4 Libraries](#24-libraries)
- [2.5 Models and Data Paths](#25-models-and-data-paths)
- [2.6 Running the Server](#26-running-the-server)
- [2.7 Verify the Installation](#27-verify-the-installation)
- [2.8 Security and Next Steps](#28-security-and-next-steps)

---

### 2.1 Native Quick Start

This path installs Kronk on the host, downloads a starter model, and starts the
model server. The first run requires an internet connection for native
libraries and model files.

**1. Install the CLI with Homebrew (macOS or Linux)**

```shell
brew tap ardanlabs/kronk
brew trust ardanlabs/kronk
brew install kronk
```

Other installation methods are listed in [Section 2.2](#22-choose-an-installation-method).

**2. Install the native inference libraries**

```shell
kronk libs --local
```

Kronk detects the operating system, architecture, and available GPU backend.
It selects Metal, CUDA, ROCm, or Vulkan when supported and falls back to CPU.

**3. Download a starter model**

```shell
kronk model pull mradermacher/Qwopus3.5-4B-Coder.Q8_0 --local
```

This coding model requires about 5.3 GB of disk space, including its multimodal
projection. On memory-constrained or CPU-only systems, start with the smaller
`unsloth/Qwen3-0.6B-Q8_0` instead.

**4. Start the server in the background**

```shell
kronk server start --api-host=127.0.0.1:11435 -d
```

The explicit host keeps this unauthenticated quick-start server reachable only
from the local machine. The API listens on `http://localhost:11435`. The
Browser UI (BUI) is enabled by default at `http://localhost:11435/admin/`.

**5. Try the model**

Open `http://localhost:11435/admin/`, select **Apps → Chat**, and choose the
downloaded model. The first request takes longer because Kronk must load the
model into memory.

You can also call the API directly:

```shell
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mradermacher/Qwopus3.5-4B-Coder.Q8_0/AGENT",
    "messages": [{"role": "user", "content": "Write a Go function that reverses a string."}],
    "max_tokens": 256
  }'
```

Stop the background server with:

```shell
kronk server stop
```

### 2.2 Choose an Installation Method

The prerequisites depend on how you install Kronk:

| Method | Platforms | Prerequisite |
| ------ | --------- | ------------ |
| Homebrew | macOS and Linux | Homebrew |
| Release archive | Supported release platforms | None beyond the host OS |
| `go install` | Platforms supported by the Go toolchain and Kronk libraries | Go 1.26 or later |
| Container | Any host supported by the selected Linux image | Docker or another OCI runtime |

Regardless of installation method, model downloads require enough disk space,
and inference requires enough RAM or VRAM for the selected model and
configuration. Quantization, context size, KV cache type, and concurrency all
affect memory use; parameter count alone is not a reliable requirement. See
[Chapter 3: Model Configuration](chapter-03-model-configuration.md).

#### Homebrew

```shell
brew tap ardanlabs/kronk
brew trust ardanlabs/kronk
brew install kronk
```

Upgrade an existing installation with:

```shell
brew upgrade kronk
```

#### Release archive

Download the archive for your operating system and architecture from the
[Kronk releases page](https://github.com/ardanlabs/kronk/releases), extract the
`kronk` executable, and place it on your `PATH`.

#### Go toolchain

```shell
go install github.com/ardanlabs/kronk/cmd/kronk@latest
```

Ensure the Go binary directory (normally `$(go env GOPATH)/bin`) is on your
`PATH`.

#### Confirm the CLI is installed

```shell
kronk --version
kronk --help
```

Use `kronk <command> --help` for the current command and flag reference rather
than relying on copied CLI output.

### 2.3 Container Quick Start

Release images are published to GHCR and Docker Hub. Each image contains the
Kronk executable, BUI, ffmpeg, and the native llama.cpp and whisper.cpp
libraries for its processor variant.

| Tag | Intended hardware | Published platforms |
| --- | ----------------- | ------------------- |
| `latest` or `latest-cpu` | CPU inference | `linux/amd64`, `linux/arm64` |
| `latest-cuda` | NVIDIA GPU | `linux/amd64`, `linux/arm64` |
| `latest-vulkan` | AMD, NVIDIA, or Intel through Vulkan | `linux/amd64`, `linux/arm64` |
| `latest-rocm` | AMD GPU through ROCm | `linux/amd64` |
| `latest-all` | All backends built for the target architecture | `linux/amd64`, `linux/arm64` |

Versioned tags use the form `vX.Y.Z-<variant>`. Pin a versioned tag for a
reproducible deployment; `latest-*` tags move when a release is published.

**Local CPU container**

This command binds the service only to the local machine, enables BUI model
downloads, and persists all state in a named volume:

```shell
docker run -d \
  --name kronk \
  --restart unless-stopped \
  -e KRONK_DOWNLOAD_ENABLED=true \
  -p 127.0.0.1:11435:11435 \
  -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest
```

Open `http://localhost:11435/admin/`, download a model from the catalog, and
use **Apps → Chat** to test it.

**NVIDIA container**

Install the
[NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html),
then add GPU access and use the CUDA image:

```shell
docker run -d \
  --name kronk \
  --restart unless-stopped \
  --runtime=nvidia --gpus all \
  -e KRONK_DOWNLOAD_ENABLED=true \
  -p 127.0.0.1:11435:11435 \
  -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest-cuda
```

AMD ROCm and Vulkan require host-specific device access. The tested command
lines and compatibility notes are maintained in the header of the
[`Dockerfile`](../zarf/docker/kronk/Dockerfile).

The container runs as UID/GID `10001`. A named volume needs no preparation. If
you use a host directory such as `/srv/kronk`, make it writable by that user
before starting the container:

```shell
sudo mkdir -p /srv/kronk
sudo chown -R 10001:10001 /srv/kronk
```

For pinned releases, updates, and remote operation,
see [Chapter 8: Model Server](chapter-08-model-server.md#87-container-operations).

### 2.4 Libraries

Kronk needs a native llama.cpp library bundle before it can run GGUF models.
The server checks the selected bundle during startup and installs it when
needed. To install it explicitly before starting the server, run:

```shell
kronk libs --local
```

The default location is:

```text
~/.kronk/libraries/<os>/<arch>/<processor>/
```

Examples include `darwin/arm64/metal` and `linux/amd64/cuda`. Multiple bundles
can coexist. Kronk selects the folder matching the detected host unless
`KRONK_ARCH`, `KRONK_OS`, `KRONK_PROCESSOR`, or `KRONK_LIB_PATH` overrides it.

Useful commands:

```shell
# Show supported bundles.
kronk libs --list-combinations

# Show installed bundles.
kronk libs --list-installs

# Install a particular published version.
kronk libs --local --version=<llama.cpp-build>

# Explicitly select CPU instead of an available GPU.
KRONK_PROCESSOR=cpu kronk libs --local
```

The normal command installs the llama.cpp version selected for the installed
Kronk release. `--upgrade` opts into the latest published llama.cpp build,
which may introduce upstream compatibility changes:

```shell
kronk libs --local --upgrade
```

Use `kronk libs --help` for cross-platform bundle installation and removal.
Changing the active library path requires a server restart; libraries are not
hot-reloaded.

Linux CUDA bundles depend on CUDA runtime libraries supplied by the host. A
working `nvidia-smi` confirms the driver but not necessarily the CUDA runtime.
If the CUDA backend does not load, inspect `libggml-cuda.so` with `ldd` and
install the runtime packages appropriate for the CUDA version used by the
current bundle. The CUDA container image already includes its matching runtime.

Speech-to-text uses a separate whisper.cpp bundle:

```shell
kronk bucky libs --local
```

See [Chapter 18: Bucky](chapter-18-bucky.md#182-installation-libraries) for
Bucky platforms, models, and configuration.

### 2.5 Models and Data Paths

List the starter catalog and download a model directly on the host:

```shell
kronk catalog list --local
kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local
```

Model sources may be bare IDs, canonical `provider/model` IDs, Hugging Face
URLs, or repository-and-quantization shorthands. Run
`kronk model pull --help` for all accepted forms.

The default data layout is:

```text
~/.kronk/
├── catalog/
│   └── catalog.yaml
├── libraries/
│   └── <os>/<arch>/<processor>/
├── models/
│   ├── model_config.yaml
│   ├── .index.yaml
│   └── <provider>/<family>/<model files>
├── bucky-libraries/
├── bucky-models/
└── keys/
```

Older installations that stored `catalog.yaml` or `model_config.yaml` directly
under `~/.kronk/` are migrated automatically when the new location is first
used.

Set `KRONK_BASE_PATH` or the global `--base-path` flag to move the entire data
root. Official containers set it to `/kronk`.

The model configuration file contains per-model and per-variant overrides. Do
not copy configuration values based only on model size; use
[Chapter 3](chapter-03-model-configuration.md) for context, cache, GPU, and
sampling settings.

### 2.6 Running the Server

Start in the foreground:

```shell
kronk server start --api-host=127.0.0.1:11435
```

Start in the background:

```shell
kronk server start --api-host=127.0.0.1:11435 -d
```

The background process writes logs to `~/.kronk/kronk.log`. Manage it with:

```shell
kronk server logs
kronk server stop
```

The server's configured default is `0.0.0.0:11435`, which listens on every
network interface. The commands above deliberately use `127.0.0.1` for local
operation. The API and BUI share port `11435`; the BUI is enabled by default
and is served under `/admin/`. Disable it for a headless server with:

```shell
kronk server start \
  --api-host=127.0.0.1:11435 \
  --web-admin-enabled=false
```

Server-side downloading is separately controlled by
`KRONK_DOWNLOAD_ENABLED` and defaults to `false`. Local CLI commands using
`--local` are not affected by this setting.

See [Chapter 8](chapter-08-model-server.md) for server flags, model pooling,
runtime paths, and deployment operations.

### 2.7 Verify the Installation

Check server liveness:

```shell
curl http://localhost:11435/v1/liveness
```

List models visible to the server:

```shell
curl http://localhost:11435/v1/models
```

Test a downloaded model:

```shell
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "unsloth/Qwen3-0.6B-Q8_0",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'
```

If authentication is enabled, add
`-H "Authorization: Bearer $KRONK_TOKEN"` to API requests.

### 2.8 Security and Next Steps

Authentication is disabled by default. Do not publish port `11435` to an
untrusted network until authentication and TLS termination are configured.
The BUI, management endpoints, and inference endpoints have separate security
controls; see [Chapter 12: Security & Authentication](chapter-12-security-authentication.md)
before operating a shared or internet-reachable server.

Continue with:

- [Chapter 3: Model Configuration](chapter-03-model-configuration.md) for
  memory, context, GPU, cache, and sampling settings.
- [Chapter 8: Model Server](chapter-08-model-server.md) for server operation
  and deployment.
- [Chapter 12: Security & Authentication](chapter-12-security-authentication.md)
  before exposing the service.
- [Chapter 14: Client Integration](chapter-14-client-integration.md) to connect
  an editor or coding agent.
- [Chapter 18: Bucky](chapter-18-bucky.md) for speech-to-text.

---
