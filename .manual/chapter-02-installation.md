# Chapter 2: Installation & Quick Start

## Table of Contents

- [2.1 Quick Start (10 Minutes)](#21-quick-start-10-minutes)
- [2.2 Prerequisites](#22-prerequisites)
- [2.3 Installing the CLI](#23-installing-the-cli)
- [2.4 Docker / OCI Container](#24-docker--oci-container)
- [2.5 Installing Libraries](#25-installing-libraries)
- [2.6 Downloading Your First Model](#26-downloading-your-first-model)
- [2.7 Starting the Server](#27-starting-the-server)
- [2.8 Securing the Server and BUI](#28-securing-the-server-and-bui)
- [2.9 Model Configuration File](#29-model-configuration-file)
- [2.10 Verifying the Installation](#210-verifying-the-installation)
- [2.11 NixOS Setup](#211-nixos-setup)

---

### 2.1 Quick Start (10 Minutes)

This is the fastest path from nothing to a working local coding
assistant. Follow it top to bottom and you'll have the Kronk Model
Server running with a real coding model in about ten minutes — plus
however long your connection needs to pull the model (roughly 5 GB).
The sections after this one cover every step in more detail and the
alternatives (Docker, manual library installs, NixOS, deeper tuning).

Everything here runs on a laptop-class GPU. On Apple Silicon a 16 GB
M-series Mac is plenty; on Linux/Windows, 16 GB+ of VRAM runs the
recommended model comfortably.

**Step 1 — Install Kronk**

On macOS or Linux with Homebrew:

```shell
brew tap ardanlabs/kronk
brew trust ardanlabs/kronk
brew install kronk
```

Or with Go on any supported platform:

```shell
go install github.com/ardanlabs/kronk/cmd/kronk@latest
```

Confirm it's on your `PATH`:

```shell
kronk --help
```

**Step 2 — Start the server**

```shell
kronk server start --web-admin-enabled
```

On first run Kronk auto-detects your hardware (Metal, CUDA, Vulkan, or
CPU), downloads the matching llama.cpp libraries, and seeds a default
`~/.kronk/model_config.yaml`. When it's ready you'll see:

```
Kronk Model Server started
API: http://localhost:11435
BUI: http://localhost:11435/admin/
```

That's the OpenAI-compatible API and the Browser UI, both on port
11435. To run the same setup in the background use
`kronk server start --web-admin-enabled -d`; to stop it, use
`kronk server stop`.

**Step 3 — Download a coding model**

The recommended starter is **Qwopus3.5-4B-Coder**, a 4-billion-parameter
coding model. It's about 5 GB on disk, runs in roughly 14 GB of VRAM
with a 72k-token context window, and is already pre-configured in the
seeded `model_config.yaml` — no editing required to use it.

```shell
kronk model pull mradermacher/Qwopus3.5-4B-Coder.Q8_0 --local
```

The `--local` flag does the download directly against your filesystem
with nicer progress output. The model lands under `~/.kronk/models/`.
(Prefer clicking? Open the BUI at http://localhost:11435/admin/, go to
**Catalog → List**, find `Qwopus3.5-4B-Coder.Q8_0`, and hit download.)

**Step 4 — Verify it works**

Quickest check is the BUI: open http://localhost:11435/admin/, click
**Apps → Chat**, pick `Qwopus3.5-4B-Coder.Q8_0`, and ask it something.
The first message takes a few seconds while the model loads; after that
it's near-instant.

Prefer the terminal? Hit the API directly:

```shell
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mradermacher/Qwopus3.5-4B-Coder.Q8_0/AGENT",
    "messages": [{"role": "user", "content": "Write a Go function that reverses a string."}],
    "max_tokens": 256
  }'
```

You should get back a working Go function. That's it — Kronk is running
locally with a coding model, at zero per-token cost, and your source
code never leaves your machine.

**Step 5 — Put it to work**

- **Connect your editor or coding agent.** Point VS Code Chat,
  OpenCode, or any OpenAI-compatible client at
  `http://localhost:11435/v1`. See [Chapter 14: Client
  Integration](chapter-14-client-integration.md).
- **Want more capability?** With a 24 GB+ GPU you can step up to the
  35B-class MoE `unsloth/Qwen3.6-35B-A3B-UD-Q4_K_M` (also pre-seeded).
  Pull it the same way and select it in your client.
- **Tune for your hardware.** Context window, `nseq-max`, and KV cache
  quantization all live in `~/.kronk/model_config.yaml` — see section
  2.8 below and [Chapter 3: Model
  Configuration](chapter-03-model-configuration.md).

### 2.2 Prerequisites

**Required**

- Go 1.26 or later
- Internet connection (for downloading libraries and models)

**Recommended**

- GPU with Metal (macOS), CUDA (NVIDIA), or Vulkan support
- 16GB+ system RAM (96GB+ Recommended)

### 2.3 Installing the CLI

**Option 1: Homebrew (recommended for macOS and Linux)**

```shell
brew tap ardanlabs/kronk
brew trust ardanlabs/kronk
brew install kronk
```

To upgrade later:

```shell
brew upgrade kronk
```

The Homebrew formula is published from the [ardanlabs/homebrew-kronk](https://github.com/ardanlabs/homebrew-kronk) tap and is updated automatically on every Kronk release.

**Option 2: Go install (any supported platform)**

```shell
go install github.com/ardanlabs/kronk/cmd/kronk@latest
```

**Option 3: Pre-built binary**

Download the appropriate archive for your OS and architecture from the [GitHub releases page](https://github.com/ardanlabs/kronk/releases), extract the `kronk` binary, and place it on your `PATH`.

**Verify the installation**

```shell
kronk --help
```

You should see output listing available commands:

```
KRONK
Local LLM inference with hardware acceleration

USAGE
  kronk [command]

COMMANDS
  server    Start/stop the model server
  model     Manage local models (list, pull, remove, show, ps)
  catalog   Browse and manage the model catalog (list, show, remove)
  libs      Install/upgrade llama.cpp libraries
  security  Manage API keys and JWT tokens
  run       Run a model directly for interactive chat (no server needed)

QUICK START
  # List entries in the catalog
  kronk catalog list --local

  # Download a model (e.g., Qwen3-8B)
  kronk model pull Qwen3-0.6B-Q8_0 --local

  # Start the server with the Browser UI
  kronk server start --web-admin-enabled

  # Open the Browser UI
  open http://localhost:11435/admin/

FEATURES
  • Text, Vision, Audio, Embeddings, Reranking
  • Metal, CUDA, ROCm, Vulkan, CPU acceleration
  • Batch processing, message caching, YaRN context extension
  • Model pooling, catalog system, browser UI
  • MCP service, security, observability

MODES
  Web mode (default)  - Communicates with running server at localhost:11435
  Local mode (--local) - Direct file operations without server

ENVIRONMENT
  KRONK_BASE_PATH, KRONK_PROCESSOR, KRONK_LIB_VERSION
  KRONK_HF_TOKEN, KRONK_WEB_API_HOST, KRONK_TOKEN

FOR MORE
  kronk <command> --help    Get help for a command
  See AGENTS.md for documentation

Usage:
  kronk [flags]
  kronk [command]

Available Commands:
  catalog     Browse and manage the model catalog (list, show, remove)
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  libs        Install or upgrade llama.cpp libraries
  model       Manage local models (index, list, pull, remove, show, ps)
  run         Run an interactive chat session with a model
  security    Manage API security (keys and tokens)
  server      Start, stop, and manage the Kronk model server

Flags:
      --base-path string   Base path for kronk data (models, libraries, catalog, model_config)
  -h, --help               help for kronk
  -v, --version            version for kronk

Use "kronk [command] --help" for more information about a command.
```

### 2.4 Docker / OCI Container

Pre-built multi-arch container images are published to GHCR and Docker Hub on
every release. They bundle the kronk binary, the BUI, one or more
llama.cpp processor backends for LLM inference, the matching whisper.cpp
(bucky) backend for audio transcription via `/v1/audio/transcriptions`,
and `ffmpeg` for decoding non-PCM audio uploads — so the image is
offline-ready after the first pull (models still need to be downloaded
separately into the persisted `/kronk` volume). Five variants are produced;
pick the one that matches your hardware:

| Tag           | Hardware target                                 | Platforms                    |
| ------------- | ----------------------------------------------- | ---------------------------- |
| `latest-cpu`  | Any host, no GPU acceleration (smallest image)  | `linux/amd64`, `linux/arm64` |
| `latest-cuda` | NVIDIA GPUs (Linux + Windows-WSL2)              | `linux/amd64`, `linux/arm64` |
| `latest-vulkan` | Vendor-neutral GPU (AMD / NVIDIA / Intel)     | `linux/amd64`, `linux/arm64` |
| `latest-rocm` | AMD GPUs via ROCm                               | `linux/amd64`                |
| `latest-all`  | Bundles cpu + cuda + vulkan + rocm in one image | `linux/amd64`, `linux/arm64` |

NVIDIA Jetson (Orin / Xavier) is not part of the published set; build it on
demand with `--target runtime-jetson` as documented in the
[`Dockerfile`](../zarf/docker/kronk/Dockerfile) header.

Tag scheme:

- `:latest-<variant>` (e.g. `:latest-cuda`) — floats to the newest release of
  that variant.
- `:latest` — the CPU image (alias of `:latest-cpu`), the only variant
  guaranteed to run anywhere. For GPUs use `:latest-cuda` / `:latest-vulkan` /
  `:latest-rocm`.
- `:vX.Y.Z-<variant>` — immutable, tied to a released version (recommended for production).

Pull and run (CPU on any host):

```shell
docker pull ghcr.io/ardanlabs/kronk:latest
# or: docker pull ardanlabs/kronk:latest

docker run --rm \
    -p 11435:11435 \
    -v kronk-data:/kronk \
    ghcr.io/ardanlabs/kronk:latest
```

NVIDIA GPU (requires `nvidia-container-toolkit` on the host):

```shell
docker run --rm --gpus all \
    -p 11435:11435 \
    -v kronk-data:/kronk \
    ghcr.io/ardanlabs/kronk:latest-cuda
```

The `/kronk` volume persists models, libraries, catalog data, keys, and
badger state across container restarts — keep it on a host bind-mount or a
named volume.

The header comment of [`zarf/docker/kronk/Dockerfile`](../zarf/docker/kronk/Dockerfile)
documents every `docker run` invocation (AMD ROCm; Vulkan on AMD / NVIDIA /
Intel; Jetson; specific-card device passthrough; etc.), the audio
transcription workflow (pulling a whisper model, hitting
`/v1/audio/transcriptions`), and lists the full host-OS × GPU compatibility
matrix. See also [Chapter 18: Bucky](chapter-18-bucky.md) for full transcription
documentation.

The `:rocm` image is a special case: the upstream whisper.cpp build
matrix has no rocm bundle, so the rocm image ships the **vulkan** bucky
bundle instead and the container entrypoint transparently points
`KRONK_BUCKY_LIB_PATH` at it on ROCm hosts. Transcription therefore
stays GPU-accelerated on AMD GPUs via the RADV Vulkan driver.

**Running headless on a remote machine.** The examples above use
`--rm`, which deletes the container on stop — fine for a quick trial,
wrong for a server. For an unattended host, run detached with a restart
policy and a named volume so state on `/kronk` survives crashes and
reboots. Inside the container Kronk runs as UID/GID `10001` and stores
everything (models, catalog, keys, libraries) under `/kronk`.

```shell
docker run -d \
    --name kronk \
    --restart unless-stopped \
    -e KRONK_AUTH_LOCAL_ENABLED=true \
    -p 11435:11435 \
    -v kronk-data:/kronk \
    ghcr.io/ardanlabs/kronk:latest
```

Only the API port `11435` is published; the debug/metrics port `11445`
is intentionally left unpublished so it is not reachable from the
network. A bind mount (`-v /srv/kronk:/kronk`) works too, but requires
`sudo chown -R 10001:10001 /srv/kronk` first since the container is
non-root. Verify it is up:

```shell
curl http://localhost:11435/v1/liveness
docker logs -f kronk
```

**User security.** The `KRONK_AUTH_LOCAL_ENABLED=true` above turns on
the embedded JWT auth. On first start Kronk generates a master key and a
10-year admin token under `/kronk/keys/`. Retrieve the admin token, then
use it to mint scoped user tokens — never hand out the admin token
itself:

```shell
# Admin token (treat like a root password)
docker exec kronk cat /kronk/keys/master.jwt

# Mint a 30-day, rate-limited user token for clients
docker exec \
  -e KRONK_TOKEN="$(docker exec kronk cat /kronk/keys/master.jwt)" \
  kronk kronk security token create \
    --duration 720h \
    --endpoints "chat-completions:1000/day,embeddings:500/day"
```

Even with auth on, restrict port `11435` to trusted networks and
terminate TLS at a reverse proxy — Kronk serves plain HTTP. See
[Chapter 12: Security & Authentication](chapter-12-security-authentication.md)
for key rotation, endpoint/limit syntax, and standalone-auth mode.

**Auto-restart on reboot.** The `--restart unless-stopped` flag already
brings the container back after a daemon restart or host reboot, as long
as Docker starts on boot (`sudo systemctl enable docker`). To manage
Kronk as a first-class service with journald logging and ordering, use a
systemd unit at `/etc/systemd/system/kronk.service` instead of the
restart policy:

```ini
[Unit]
Description=Kronk LLM inference server
Requires=docker.service
After=docker.service

[Service]
Restart=always
RestartSec=5
ExecStartPre=-/usr/bin/docker rm -f kronk
ExecStart=/usr/bin/docker run --rm --name kronk \
  -e KRONK_AUTH_LOCAL_ENABLED=true \
  -p 11435:11435 \
  -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest
ExecStop=/usr/bin/docker stop kronk

[Install]
WantedBy=multi-user.target
```

```shell
sudo systemctl daemon-reload
sudo systemctl enable --now kronk
journalctl -u kronk -f
```

Use **either** the `--restart` policy **or** the systemd unit, not
both — with systemd the container is `--rm` and systemd owns its
lifecycle.

**Preinstalling models.** Pull models into the running container so the
box is ready immediately and can run offline. Because state lives on the
volume, models pulled once persist across updates and reboots. The
`KRONK_DOWNLOAD_ENABLED=false` default only blocks downloads triggered
from the browser UI; CLI `model pull` works regardless:

```shell
docker exec kronk kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local
docker exec kronk kronk catalog list --local

# Whisper (Bucky) audio model
docker exec kronk kronk bucky model pull ggml-tiny.bin
```

**Updating the image.** Data lives on the `kronk-data` volume, so
updating is pull-and-recreate — models and keys are untouched. Pin a
specific version tag in production so restarts are reproducible.

```shell
docker pull ghcr.io/ardanlabs/kronk:latest

# plain docker: recreate with the same run command
docker stop kronk && docker rm kronk
# docker run -d ... (as above)

# systemd deployment
sudo systemctl restart kronk

# verify
docker exec kronk kronk version
curl http://localhost:11435/v1/liveness
```

**Uninstalling.** Stop and remove the container, image, and — only if
you no longer need your models and tokens — the volume. Removing the
volume is irreversible.

```shell
# systemd deployment
sudo systemctl disable --now kronk
sudo rm /etc/systemd/system/kronk.service && sudo systemctl daemon-reload

# plain docker deployment
docker stop kronk && docker rm kronk

docker rmi ghcr.io/ardanlabs/kronk:latest
docker volume rm kronk-data   # deletes all models, catalog, and keys
```

**Google Cloud Run.** Kronk is a standard OCI image; deploy it per the
[Cloud Run docs](https://docs.cloud.google.com/run/docs/deploying). Cloud
Run terminates TLS, routes one port, and gives every instance an
**ephemeral** filesystem — `/kronk` does not survive a new revision or
cold start.

CPU-only service (Kronk listens on `11435` by default):

```shell
gcloud run deploy kronk \
    --image=ghcr.io/ardanlabs/kronk:latest \
    --region=us-central1 --port=11435 \
    --cpu=8 --memory=32Gi \
    --timeout=3600 --concurrency=4 \
    --min-instances=1 --no-cpu-throttling
```

- `--port=11435` — the default listen port; the debug port `11445` is not routed.
- `--timeout=3600` — Cloud Run max (60m), matching Kronk's `WriteTimeout` default.
- `--min-instances=1 --no-cpu-throttling` — keep the model resident; scale-to-zero reloads it on every cold start.

**Models.** `KRONK_DOWNLOAD_ENABLED` is `false` and the filesystem is
ephemeral, so bake the model into a derived image:

```dockerfile
FROM ghcr.io/ardanlabs/kronk:latest
RUN kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local --base-path /kronk
```

Or mount a bucket at `/kronk/models` — see
[Cloud Storage volume mounts](https://docs.cloud.google.com/run/docs/configuring/services/cloud-storage-volume-mounts).
GCS FUSE is unsuitable for badger state (catalog, keys); bake that in.

**Auth.** Gate at the edge with Cloud Run IAM (`--no-allow-unauthenticated`
+ `roles/run.invoker`), or use Kronk JWT
(`--set-env-vars=KRONK_AUTH_LOCAL_ENABLED=true`) with keys baked in — a new
revision otherwise regenerates them. See
[Chapter 12](chapter-12-security-authentication.md).

**GPU (NVIDIA L4).** Deploy the `-cuda` image with an attached GPU in a
[supported region](https://docs.cloud.google.com/run/docs/configuring/services/gpu)
(min 4 CPU / 16 GiB); Kronk auto-selects the `cuda` backend.

```shell
gcloud run deploy kronk \
    --image=ghcr.io/ardanlabs/kronk:latest-cuda \
    --region=us-central1 --port=11435 \
    --cpu=8 --memory=32Gi \
    --gpu=1 --gpu-type=nvidia-l4 --no-cpu-throttling \
    --timeout=3600 --concurrency=4 \
    --min-instances=1 --max-instances=1
```

Cloud Run requires `--max-instances` to be set when a GPU is attached.

### 2.5 Installing Libraries

Before running inference, you need the llama.cpp libraries for your machine. Kronk auto-detects your hardware and downloads the appropriate binaries.

**Option A: Via the Server**

Start the server with the BUI enabled and use it to download libraries:

```shell
kronk server start --web-admin-enabled
```

Open http://localhost:11435/admin/ in your browser and navigate to the Libraries page.

**Option B: Via CLI**

```shell
kronk libs --local
```

This downloads the **well-known default version** of llama.cpp baked into
the SDK and installs it under
`~/.kronk/libraries/<os>/<arch>/<processor>/` using auto-detected settings
(for example `~/.kronk/libraries/darwin/arm64/metal/`). Each
`(arch, os, processor)` triple lives in its own folder so multiple
bundles can coexist on the same machine.

To track and install the **latest** llama.cpp release instead of the
default version, opt in with `--upgrade`:

```shell
kronk libs --local --upgrade
```

> The standalone CLI defaults to the pinned default version so reinstalls
> are reproducible. The model server takes the opposite default
> (`--allow-upgrade=true`) so a long-running server picks up upstream
> fixes; see Chapter 8 §8.3 for that flag.

**Pinning a Specific Library Version**

Sometimes there are breaking changes to llama.cpp that require a matching version of yzma and Kronk. To ensure stability, you can install a specific library version:

```shell
kronk libs --version=b8864 --local
```

Or via environment variable:

```shell
KRONK_LIB_VERSION=b8864 kronk libs --local
```

Here are the known compatible versions:

| llama.cpp | yzma    | kronk  |
| --------- | ------- | ------ |
| b8864     | v1.12.0 | 1.23.1 |
| b8865+    | v1.13.0 | 1.23.2 |

If you experience unexpected behavior after a library upgrade, pin the version that matches your installed Kronk release using the table above.

**Environment Variables for Library Installation**

```
KRONK_LIB_PATH  - Library directory. See "KRONK_LIB_PATH semantics" below.
KRONK_PROCESSOR - `cpu`, `cuda`, `metal`, `rocm`, or `vulkan` (default: `cpu`)
KRONK_ARCH      - Architecture override: `amd64`, `arm64`
KRONK_OS        - OS override: `linux`, `darwin`, `windows`
```

**KRONK_LIB_PATH semantics**

`KRONK_LIB_PATH` is interpreted in one of three ways:

1. _Unset_ — the runtime resolves
   `<base>/libraries/<os>/<arch>/<processor>/` based on the detected (or
   `KRONK_*`-overridden) triple.
2. _Points at a directory containing a `version.json`_ — used as-is. This
   is the form to set when you want to switch the active install to a
   previously-downloaded triple folder. Example:

   ```shell
   export KRONK_LIB_PATH=~/.kronk/libraries/linux/amd64/cuda
   ```

3. _Points at a non-empty directory without a `version.json`_ — treated as
   a user-managed read-only build. Kronk will load libraries from it but
   never write to it; mutating CLI/HTTP operations against it return an
   error.

Switching the active install requires a server restart; libraries are not
hot-reloaded.

**Example: Install CUDA Libraries**

```shell
KRONK_PROCESSOR=cuda kronk libs --local
```

**Installing for Another Triple**

You can also install a bundle for a triple other than the current
machine's detected one — useful for prepping a shared filesystem or a
target host. The install lands in its own folder under the libraries
root and does not touch the active install:

```shell
# List every supported (arch, os, processor) combination
kronk libs --list-combinations

# Install the Linux/CUDA bundle alongside whatever is already active
kronk libs --install --arch=amd64 --os=linux --processor=cuda --local

# List installed bundles
kronk libs --list-installs

# Remove an install
kronk libs --remove-install --arch=amd64 --os=linux --processor=cuda --local
```

In web mode (the default — no `--local`) the same commands are dispatched
through the running server. Activate any installed bundle by exporting
`KRONK_LIB_PATH` to its folder and restarting the server.

**Audio (Bucky):** if you also plan to use speech-to-text, install the
whisper.cpp libraries with the parallel `kronk bucky libs` command. The
flags mirror `kronk libs` and the bundle lands under
`~/.kronk/bucky-libraries/`. See [Chapter 18: Bucky](chapter-18-bucky.md).

### 2.6 Downloading Your First Model

Kronk maintains your **personal catalog** at `~/.kronk/catalog.yaml`. On
first run it is seeded from an embedded starter list so you have something
to choose from immediately; the catalog grows as you pull more models or
resolve new IDs against HuggingFace.

List entries in the catalog:

```shell
kronk catalog list --local
```

Output:

```
VAL   MODEL ID                                            PROVIDER    FAMILY                              ARCH      MTMD   SIZE
✓     ggml-org/embeddinggemma-300m-qat-Q8_0               ggml-org    embeddinggemma-300m-qat-q8_0-GGUF   bert      -      329.0 MB
✓     unsloth/Qwen3-0.6B-Q8_0                             unsloth     Qwen3-0.6B-GGUF                     qwen3     -      699.0 MB
✗     bartowski/cerebras_Qwen3-Coder-REAP-25B-A3B-Q8_0    bartowski   Qwen3-Coder-REAP-25B-A3B-GGUF       qwen3moe  -      26.5 GB
✗     unsloth/LFM2.5-VL-1.6B-Q8_0                         unsloth     LFM2.5-VL-1.6B-GGUF                 lfm2      ✓      1.7 GB
```

The `VAL` column shows whether the model files have been downloaded and
validated locally; `MTMD` indicates a multimodal projection (mmproj) is
present.

Download a model (recommended starter: Qwen3-0.6B-Q8_0):

```shell
kronk model pull Qwen3-0.6B-Q8_0 --local
```

Models are stored in `~/.kronk/models/<provider>/<family>/` by default.
After the pull completes the catalog entry is updated with the resolved
provider, family, revision, and file sizes so subsequent lookups don't
need to hit HuggingFace.

**Audio (Bucky):** whisper models live in a separate flat layout at
`~/.kronk/bucky-models/ggml-<name>.bin` and are pulled with
`kronk bucky model pull <name>` (e.g. `ggml-tiny.bin`). See
[Chapter 18 §18.3](chapter-18-bucky.md#183-model-catalog-pull).

### 2.7 Starting the Server

Start the Kronk Model Server:

```shell
kronk server start
```

The server starts on `http://localhost:11435` by default. You'll see output like:

```
Kronk Model Server started
API: http://localhost:11435
```

The BUI is disabled by default. Add `--web-admin-enabled` to mount it at
`http://localhost:11435/admin/`.

**Running in Background**

To run the server as a background process:

```shell
kronk server start -d
```

**Stopping the Server**

```shell
kronk server stop
```

### 2.8 Securing the Server and BUI

Kronk separates inference authentication from administration so a server can
offer model inference without exposing model downloads, configuration,
playground sessions, or security management.

Choose one mode:

| Mode | Inference APIs and `GET /v1/models` | Management and playground APIs | BUI |
| --- | --- | --- | --- |
| Open | No token | No token | Optional, no login |
| Admin-only | No token | Admin token | Optional; password login |
| Fully protected | User or admin token | Admin token | Optional; password login |

The BUI is always optional. Without `--web-admin-enabled`, nothing is served
under `/admin/`.

**Open server (trusted networks only)**

Run headless with every API open:

```shell
kronk server start
```

Or attach the BUI without authentication:

```shell
kronk server start --web-admin-enabled
```

**Public inference with protected administration**

First choose a long, randomly generated password and store its SHA-256 digest.
The digest is not a slow password hash, so do not use a short or reused human
password.

```shell
KRONK_ADMIN_PASSWORD="$(openssl rand -base64 24)"
export KRONK_WEB_ADMIN_PASSWORD_SHA256="$(printf '%s' "$KRONK_ADMIN_PASSWORD" | shasum -a 256 | awk '{print $1}')"

kronk server start \
  --admin-auth-enabled \
  --web-admin-enabled \
  --web-admin-password-sha256="$KRONK_WEB_ADMIN_PASSWORD_SHA256"
```

On Linux, use `sha256sum` instead of `shasum -a 256` if `shasum` is not
installed. Open `https://your-server/admin/` and sign in with the value of
`KRONK_ADMIN_PASSWORD`. The password itself is never placed in server
configuration.

Inference endpoints remain open in this mode. Every Kronk management,
playground, and security-management endpoint requires an admin JWT.
`GET /v1/models` remains an inference endpoint.

The CLI reads its JWT from `KRONK_TOKEN`. The local auth service creates the
master admin token on first start:

```shell
export KRONK_TOKEN="$(cat ~/.kronk/keys/master.jwt)"
kronk model list
```

Treat `master.jwt` like a root credential. Do not distribute it to inference
clients.

**Fully protected server**

Use `--auth-enabled` to require JWTs for inference as well. This flag
automatically enables admin authentication:

```shell
kronk server start \
  --auth-enabled \
  --web-admin-enabled \
  --web-admin-password-sha256="$KRONK_WEB_ADMIN_PASSWORD_SHA256"
```

Create scoped user tokens for inference clients rather than giving them the
admin token. See [Chapter 12](chapter-12-security-authentication.md) for token
creation, endpoint grants, rate limits, and key rotation.

**Internet deployment requirements**

- Terminate HTTPS at a reverse proxy; Kronk itself serves plain HTTP.
- Keep `/admin/` and `/v1` on the same public origin so the secure BUI session
  cookie can authenticate API calls.
- Block `/admin/` at the proxy or firewall where browser administration should
  not be reachable.
- Password-based BUI login currently requires Kronk's local auth service; it is
  not available with `KRONK_AUTH_HOST`.
- Leave `KRONK_WEB_ADMIN_ENABLED` unset for a headless deployment.

The equivalent environment variables are `KRONK_AUTH_LOCAL_ENABLED`,
`KRONK_AUTH_ADMIN_ENABLED`, `KRONK_WEB_ADMIN_ENABLED`, and the masked
`KRONK_WEB_ADMIN_PASSWORD_SHA256`.

### 2.9 Model Configuration File

When Kronk starts the server for the first time, it automatically installs a default `model_config.yaml` file in the `~/.kronk/` directory. This file controls how each model behaves when loaded by the server — context window size, batch processing, caching, sampling parameters, and more.

**How It Works**

The default configuration is embedded inside the Kronk CLI binary. On first server start, if `~/.kronk/model_config.yaml` does not already exist, Kronk writes the embedded default to that path. Once the file exists, Kronk never overwrites it — your edits are preserved across upgrades.

The server logs the path it's using on startup:

```
startup  status=model config  path=/Users/you/.kronk/model_config.yaml
```

**File Structure**

The file is a YAML document where each top-level key is a model ID (or a model ID with a config variant suffix). Under each key you set the configuration options for that model. Here's a simplified example:

```yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 32768
  sampling-parameters:
    temperature: 0.7
    top_p: 0.8
    top_k: 20

unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M/AGENT:
  context-window: 131072
  nseq-max: 2
  sampling-parameters:
    temperature: 1.0
    top_k: 64
    top_p: 0.95

Qwen/Qwen3-8B-Q8_0/YARN:
  context-window: 131072
  rope-scaling-type: yarn
  yarn-orig-ctx: 32768
```

The `/YARN` suffix is a **config variant** — it lets you define multiple configurations for the same model. When making an API request, use the full variant name (e.g., `Qwen/Qwen3-8B-Q8_0/YARN`) as the `model` field to select that configuration.

**Available Options**

The file includes a commented reference at the top listing every option. Here are the most commonly used:

| Option                | Description                                            | Default |
| --------------------- | ------------------------------------------------------ | ------- |
| `context-window`      | Max tokens the model can process per request           | 8192    |
| `ngpu-layers`         | GPU layers to offload (0 = all, -1 = none)             | 0       |
| `flash-attention`     | Flash Attention mode: `enabled`, `disabled`, `auto`    | auto    |
| `incremental-cache`   | Enable IMC for agentic workflows                       | true    |
| `nseq-max`            | Max parallel sequences for batched inference           | 0       |
| `nbatch`              | Logical batch size                                     | `nubatch × nseq-max` |
| `nubatch`             | Physical batch size for prompt ingestion               | 2048    |
| `cache-type-k`        | KV cache key quantization: `f16`, `q8_0`, `q4_0`, etc. | —       |
| `cache-type-v`        | KV cache value quantization                            | —       |
| `sampling-parameters` | Nested block for temperature, top_p, top_k, min_p      | —       |

For the complete list of options and detailed explanations, see [Chapter 3: Model Configuration](#chapter-3-model-configuration).

**Editing the File**

Open the file in any text editor:

```shell
# macOS
open ~/.kronk/model_config.yaml

# Linux
nano ~/.kronk/model_config.yaml
```

After editing, restart the server to apply changes:

```shell
kronk server stop
kronk server start
```

**Configuration Priority**

When the server loads a model, configuration is resolved through two
layers (plus sampling defaults):

1. **Analysis defaults** — Hardware-aware values inferred from the GGUF
   metadata and the local devices (context window, batch sizes, cache
   types, flash attention, GPU layers).
2. **`model_config.yaml` overrides** — Your per-model overrides merged on
   top of the analysis defaults. Anything you set here wins.
3. **Sampling defaults** — Any zero-valued sampling fields are filled in
   from the SDK's built-in sampling defaults so the model always has a
   complete sampler configuration.

The catalog itself is **not** part of this layering — it is a resolution
cache (provider, family, revision, files) and not a source of tuning
knobs. All tuning lives in `model_config.yaml` (or in `model.Config` when
you're embedding the SDK directly).

**Tips**

- The key is the canonical model id — `provider/modelID` (for example
  `unsloth/Qwen3-0.6B-Q8_0`) or a variant such as
  `unsloth/Qwen3-0.6B-Q8_0/IMC` — not a file name.
- Use YAML anchors (`&name` and `<<: *name`) to share common settings between variants. The default file includes examples of this pattern.
- The `--model-config` server flag lets you point to an alternative config file for testing without modifying your main one.

### 2.10 Verifying the Installation

**Test via curl**

```shell
curl http://localhost:11435/v1/models
```

You should see a list of available models.

**Test Chat Completion**

_Note: It might take a few seconds the first time you call this because the
model needs to be loaded into memory first._

```shell
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-0.6B-Q8_0",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'
```

**Test via BUI**

Start Kronk with `--web-admin-enabled`, open
`http://localhost:11435/admin/`, and navigate to **Apps → Chat**. Select the
model you want to try and chat away.

### 2.11 NixOS Setup

NixOS does not follow the Filesystem Hierarchy Standard (FHS), so shared
libraries and binaries cannot be found in standard paths like `/usr/lib`. Kronk
requires llama.cpp shared libraries at runtime, which means on NixOS you need to
provide them through Nix rather than using the built-in `kronk libs` downloader.

A `flake.nix` is provided in `zarf/nix/` with dev shells for development and
build packages for producing a standalone `kronk` binary, each per GPU backend.

**Prerequisites**

- NixOS or Nix package manager with flakes enabled
- A supported GPU (Vulkan or CUDA), or CPU-only mode

**Available Dev Shells**

The flake provides multiple shells, one per GPU backend:

| Command                         | Backend | GPU Required         |
| ------------------------------- | ------- | -------------------- |
| `nix develop ./zarf/nix`        | CPU     | None                 |
| `nix develop ./zarf/nix#cpu`    | CPU     | None                 |
| `nix develop ./zarf/nix#vulkan` | Vulkan  | Vulkan-capable GPU   |
| `nix develop ./zarf/nix#cuda`   | CUDA    | NVIDIA GPU with CUDA |

**Building the Kronk CLI**

The flake also provides build packages that produce a wrapped `kronk` binary
with the correct llama.cpp backend and runtime libraries baked in:

| Command                       | Backend | GPU Required         |
| ----------------------------- | ------- | -------------------- |
| `nix build ./zarf/nix`        | CPU     | None                 |
| `nix build ./zarf/nix#cpu`    | CPU     | None                 |
| `nix build ./zarf/nix#vulkan` | Vulkan  | Vulkan-capable GPU   |
| `nix build ./zarf/nix#cuda`   | CUDA    | NVIDIA GPU with CUDA |

The Go binary is built and then wrapped per backend so
that `KRONK_LIB_PATH`, `KRONK_ALLOW_UPGRADE`, and `LD_LIBRARY_PATH` are set
automatically. No dev shell is required to run the resulting binary.

**Note:** The `vendorHash` in the flake must be updated whenever `go.mod` or
`go.sum` changes. Build with a fake hash and Nix will report the correct one.

**Environment Variables**

All shells and built packages automatically set the following:

| Variable              | Value                                    | Purpose                                              |
| --------------------- | ---------------------------------------- | ---------------------------------------------------- |
| `KRONK_LIB_PATH`      | Nix store path to the selected llama.cpp | Points Kronk to the Nix-managed llama.cpp libraries  |
| `KRONK_ALLOW_UPGRADE` | `false`                                  | Prevents Kronk from attempting to download libraries |
| `LD_LIBRARY_PATH`     | Includes `libffi` and `libstdc++`        | Required for FFI runtime linking                     |

**Important:** Because `KRONK_ALLOW_UPGRADE` is set to `false`, the `kronk libs`
command will not attempt to download or overwrite libraries. Library updates are
managed through `nix flake update` instead.

**Troubleshooting**

- **Library not found errors:** Ensure you are inside the `nix develop` shell
  or using a `nix build` output. The required `LD_LIBRARY_PATH` and
  `KRONK_LIB_PATH` are only set within the shell or the wrapped binary.
- **Vulkan not detected:** Verify your GPU drivers are installed at the NixOS
  system level (`hardware.opengl.enable = true` and appropriate driver packages
  in your NixOS configuration).
- **Go version mismatch:** The flake pins a specific Go version. If Kronk
  requires a newer version, update the `go_1_26` package reference in
  `flake.nix`.
- **vendorHash mismatch:** After updating Go dependencies, rebuild with a fake
  hash (e.g. `lib.fakeHash`) and Nix will print the correct `vendorHash`.

---
