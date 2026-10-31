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
- [2.9 CLI Reference and Shell Completion](#29-cli-reference-and-shell-completion)

---

### 2.1 Native Quick Start

This path installs Kronk on the host, downloads a starter model, and starts the
model server. The first run requires an internet connection for native
libraries and model files. These examples assume the default server settings;
an existing installation keeps its saved configuration.

**1. Install the CLI with Homebrew (macOS or Linux)**

```shell
brew install ardanlabs/kronk/kronk
```

The fully qualified formula name adds the tap and trusts only the Kronk formula.
Do not run `brew trust ardanlabs/kronk` unless you intend to trust every current
and future formula, cask, and external command published by the tap.

Other installation methods are listed in [Section 2.2](#22-choose-an-installation-method).

**2. Install the native inference libraries**

```shell
kronk libs --local
```

`--local` runs this operation on the current machine without a server. Library,
model, and catalog commands that support this flag otherwise use a running
server by default.

Kronk detects the operating system, architecture, and available GPU backend.
It selects Metal, CUDA, ROCm, or Vulkan when supported and falls back to CPU.

**3. Download a starter model**

```shell
kronk model pull mradermacher/Qwopus3.5-4B-Coder.Q8_0 --local
```

This coding model requires about 5.3 GB of disk space, including its multimodal
projection. On memory-constrained or CPU-only systems, start with the smaller
`unsloth/Qwen3-0.6B-Q8_0` instead, using that ID in both the pull command and
the API examples below. Disk size is not the model's runtime memory requirement.

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
    "model": "mradermacher/Qwopus3.5-4B-Coder.Q8_0",
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
| `go install` | Platforms supported by the Go toolchain and Kronk libraries | Go 1.27.3 or later for this release |
| Container | Any host supported by the selected Linux image | Docker or another OCI runtime |
| Kubernetes / Helm | Linux nodes supported by the selected image | Helm, kubectl, a cluster, and a suitable StorageClass |

Regardless of installation method, model downloads require enough disk space,
and inference requires enough RAM or VRAM for the selected model and
configuration. Quantization, context size, KV cache type, and concurrency all
affect memory use; parameter count alone is not a reliable requirement. See
[Chapter 3: Model Configuration](https://www.kronkai.com/manual#chapter-3-model-configuration).

#### Homebrew

```shell
brew install ardanlabs/kronk/kronk
```

Homebrew automatically adds the tap and records trust for this formula. This is
narrower than `brew trust ardanlabs/kronk`, which trusts every current and future
item in the tap.

Upgrade an existing installation with:

```shell
brew upgrade ardanlabs/kronk/kronk
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
Kronk executable, BUI, ffmpeg, and native llama.cpp and whisper.cpp libraries.
Linux amd64 images also include stable-diffusion.cpp libraries for Malina;
arm64 images do not. The tag identifies the llama.cpp backend, not a guarantee
that every inference engine uses that backend: ROCm images use Vulkan for
transcription, and CUDA images provide Vulkan or CPU for image generation.

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

This command binds the service only to the local machine and persists all
state in a named volume:

```shell
docker run -d \
  --name kronk \
  --restart unless-stopped \
  -e KRONK_PROCESSOR=cpu \
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
  -p 127.0.0.1:11435:11435 \
  -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:latest-cuda
```

AMD ROCm and Vulkan require host-specific device access. Use the Compose
files below, or see the tested `docker run` command lines and the
compatibility matrix in the header of the
[`Dockerfile`](https://github.com/ardanlabs/kronk/blob/main/zarf/docker/kronk/Dockerfile).

**Docker Compose**

The repository includes Compose files in
[`zarf/docker/kronk`](https://github.com/ardanlabs/kronk/tree/main/zarf/docker/kronk)
that reuse the same `kronk-data` volume. The default file selects CPU inference
and publishes the API (`11435`) and debug server (`11445`, including metrics
and profiling) on localhost. Remove a `kronk` container started with
`docker run` first; its models are kept in the volume:

```shell
docker rm -f kronk
git clone --depth 1 https://github.com/ardanlabs/kronk.git
cd kronk/zarf/docker/kronk
docker compose up -d
```

Each backend has its own self-contained file. For a GPU, use the matching
one instead of `compose.yaml`:

```shell
docker compose -f compose.cuda.yaml up -d     # NVIDIA, needs the NVIDIA Container Toolkit
docker compose -f compose.rocm.yaml up -d     # AMD with ROCm, needs /dev/kfd and /dev/dri
docker compose -f compose.vulkan.yaml up -d   # AMD or Intel with Vulkan, needs /dev/dri
```

To use the `latest-all` image, set the GPU file's image to
`ghcr.io/ardanlabs/kronk:latest-all`, retaining its device access and processor
settings.

Settings, server options such as authentication, GPU details, and upgrades are
covered in [8.7 Container Operations](https://www.kronkai.com/manual#87-container-operations).

The container runs as UID/GID `10001`. A named volume needs no preparation. If
you use a host directory such as `/srv/kronk`, make it writable by that user
before starting the container:

```shell
sudo mkdir -p /srv/kronk
sudo chown -R 10001:10001 /srv/kronk
```

For pinned releases, updates, and remote operation,
see [Chapter 8: Model Server](https://www.kronkai.com/manual#87-container-operations).

**Kubernetes with Helm**

From a repository checkout, with your intended Kubernetes context selected:

```shell
helm upgrade --install kronk zarf/helm/kronk -n kronk --create-namespace \
  -f zarf/helm/kronk/examples/cpu.yaml --wait --timeout 20m
```

This installs one StatefulSet replica, a retained `50Gi` RWO data claim, and
the small `unsloth/Qwen3-0.6B-Q8_0` starter model. A default StorageClass is
required unless you select another class or an existing claim. Unlike the
local container quick start, the chart defaults to authenticated access and
disables the BUI. See [8.10 Kubernetes with Helm](https://www.kronkai.com/manual#810-kubernetes-with-helm)
for installation and essential operating warnings, and the
[chart README](https://github.com/ardanlabs/kronk/blob/main/zarf/helm/kronk/README.md)
for configuration, the first chat request, and troubleshooting. Installation is
from the local chart only. GPU examples require validation on real nodes with
compatible drivers and runtimes.

### 2.4 Libraries

Kronk needs a native llama.cpp library bundle before it can run GGUF models.
By default, the server checks the selected bundle during startup and installs
it when needed. To install it explicitly before starting the server, run:

```shell
kronk libs --local
```

The server also installs and initializes Bucky and Malina libraries by default
where supported. For a server that does not need transcription or image
generation, `--media-backends-enabled=false` skips those backend downloads and
initialization. This does not disable vision or audio input supported by a
llama.cpp model.

#### Selecting a library bundle

The default location is:

```text
~/.kronk/libraries/<os>/<arch>/<processor>/
```

Examples include `darwin/arm64/metal` and `linux/amd64/cuda`. Multiple bundles
can coexist. During automatic selection, Kronk first resolves the preferred
processor for the host. A CUDA 13 bundle requires every visible NVIDIA GPU to
have compute capability 7.5 or newer. If the visible devices are older, or
`CUDA_VISIBLE_DEVICES` leaves no CUDA device visible, Kronk selects a supported
ROCm or Vulkan host runtime when one is available and otherwise selects CPU.
An inconclusive NVIDIA probe retains CUDA rather than silently changing the
backend.

After installation, Kronk probes the preferred accelerator bundle before
loading it. If that bundle positively reports no accelerator and another
installed bundle for the same llama.cpp version positively reports a device,
Kronk selects the working installed bundle. This second check does not download
another bundle. Startup logs report the preferred processor, selected
processor, and reason for either retaining or changing it.

`KRONK_ARCH` and `KRONK_OS` select the bundle's target architecture and
operating system. Set `KRONK_PROCESSOR` when automatic backend selection is
not appropriate, such as deliberately running CPU inference on a GPU host.
An explicit processor or library path disables the server's automatic runtime
substitution; setting only the architecture or operating system does not.

List the supported and installed bundles without a running server:

```shell
kronk libs --local --list-combinations
kronk libs --local --list-installs
```

To deliberately select CPU inference, apply the setting to both installation
and server startup; setting it for one command does not persist it:

```shell
KRONK_PROCESSOR=cpu kronk libs --local
KRONK_PROCESSOR=cpu kronk server start
```

For server startup, `KRONK_LIB_PATH` or `--lib-path` can name an existing
versioned Kronk bundle or a user-managed library directory. A non-empty
user-managed directory without `version.json` is treated as read-only: Kronk
loads from it but does not install, upgrade, or replace its contents. Changing
the selected processor or library path requires a process restart; native
libraries are not hot-reloaded.

Linux CUDA bundles depend on CUDA runtime libraries supplied by the host. A
working `nvidia-smi` confirms the driver but not necessarily the CUDA runtime.
If the CUDA backend does not load, inspect `libggml-cuda.so` with `ldd` and
install the runtime packages appropriate for the CUDA version used by the
current bundle. The CUDA container image already includes its matching runtime.

#### Versions and upgrades

On a fresh installation, the normal command installs the llama.cpp build
pinned for the installed Kronk release and verifies the download against its
pinned release manifest. A newer installed build is retained rather than
automatically downgraded; use `--version` to select a particular release.
Once verification metadata is cached, subsequent verification can run offline.
Older installations without that metadata need an online verification first.

Prefer the pinned build. `--upgrade` opts into the latest published llama.cpp
build, which may be incompatible with the installed Kronk release:

```shell
kronk libs --local --upgrade
```

Use `kronk libs --help` for explicit version selection, cross-platform bundle
installation, and removal. The compatibility matrix is kept in the repository
[README](https://github.com/ardanlabs/kronk#project-status). Native backend
environment settings and their restart requirements are covered in
[Chapter 3](https://www.kronkai.com/manual#native-backend-environment-settings).

#### OpenVINO preview bundles

Kronk can download llama.cpp OpenVINO bundles for Linux amd64 and Windows
amd64. Kronk supports these bundles only on Intel systems. They can be tried to
improve inference performance by targeting an Intel CPU, integrated or discrete
Intel GPU, or Intel NPU. They do not provide acceleration for AMD or NVIDIA
GPUs; use ROCm or Vulkan for AMD GPUs and CUDA for NVIDIA GPUs.

OpenVINO remains an explicit, preview backend. It is not selected automatically,
and performance and compatibility depend on the model, quantization, workload,
and target device. Benchmark against Kronk's standard `cpu` backend when using
an Intel CPU, and validate complete serving workloads on the target system before
production use. Successfully loading one model is not confirmation that every
model or serving feature is supported.

Two environment variables select the runtime:

- `KRONK_PROCESSOR=openvino` selects the OpenVINO llama.cpp bundle.
- `GGML_OPENVINO_DEVICE=CPU|GPU|GPU.<index>|NPU` selects the device used by
  OpenVINO within the Intel family. OpenVINO defaults to `CPU` when this variable
  is unset, but setting it explicitly makes the intended target clear.

For example:

```shell
# Install the Linux amd64 bundle without making it the automatic default.
kronk libs --local --install \
  --arch=amd64 \
  --os=linux \
  --processor=openvino

# Select the bundle and an Intel GPU for one server process.
KRONK_PROCESSOR=openvino \
GGML_OPENVINO_DEVICE=GPU \
kronk server start
```

The OpenVINO runtime reads `GGML_OPENVINO_DEVICE` once during initialization;
restart Kronk after changing it. GPU and NPU targets require the corresponding
Intel host drivers and device permissions. If a requested device is not
available, upstream OpenVINO can fall back to CPU, so validate the installation
with the opt-in probe from a Kronk source checkout:

```shell
make test-openvino                         # CPU
make test-openvino OPENVINO_DEVICE=GPU
make test-openvino OPENVINO_DEVICE=GPU.0
make test-openvino OPENVINO_DEVICE=NPU
```

The probe requires a source checkout and Linux/Windows amd64. It downloads the
pinned bundle and a small diagnostic model, tests inference, and fails if
OpenVINO reports a fallback to CPU. Passing it is a smoke test, not validation
of every model or serving feature.

#### Bucky and Malina libraries

Speech-to-text and image generation use separate bundles. To install them
explicitly without a running server:

```shell
kronk bucky libs --local
kronk malina libs --local
```

Platform and backend support differ from llama.cpp. See
[Chapter 18: Bucky](https://www.kronkai.com/manual#182-install-whisper-libraries)
and [Chapter 19: Malina](https://www.kronkai.com/manual#192-install-stable-diffusion-libraries)
for supported bundles, models, and configuration.

### 2.5 Models and Data Paths

List the starter catalog and download a model directly on the host:

```shell
kronk catalog list --local
kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local
```

Model sources may be canonical `provider/model` IDs, Hugging Face URLs, or
repository-and-quantization shorthands. Run
`kronk model pull --help` for all accepted forms. To pin both the repository
and file, use `owner/repo/file.gguf` or a Hugging Face `blob` or `resolve` URL.
Kronk then resolves that exact file only within that repository.

The default data layout is:

```text
~/.kronk/
├── catalog/
│   └── catalog.yaml
├── libraries/
│   └── <os>/<arch>/<processor>/
├── lora/
│   └── [<organization>/]<adapter>.gguf
├── models/
│   ├── model_config.yaml
│   └── <provider>/<family>/<model files>
├── bucky-libraries/
├── bucky-models/
├── malina-libraries/
├── malina-models/
└── keys/
```

Set `KRONK_BASE_PATH` or the global `--base-path` flag to select a different
data root. This does not move existing files; migrate them separately when
reusing an installation. Use the same root for local management commands and
the server. Official containers set it to `/kronk`.

The `lora` directory holds optional, user-provided LoRA adapter GGUF files.
Kronk does not download these files. See
[Chapter 3 §3.7](https://www.kronkai.com/manual#lora-adapters) for placement
and configuration.

`models/model_config.yaml` contains server settings and overrides for language,
Bucky, and Malina models, including language-model variants. Kronk creates it
when absent and preserves existing edits. Restart the server after changing it.
Do not copy configuration values based only on model size; use
[Chapter 3](https://www.kronkai.com/manual#chapter-3-model-configuration) for context, cache, GPU, and
sampling settings.

### 2.6 Running the Server

Start in the foreground:

```shell
kronk server start --api-host=127.0.0.1:11435
```

Stop a foreground server with Ctrl-C. To run in the background instead:

```shell
kronk server start --api-host=127.0.0.1:11435 -d
```

The background process writes logs to `~/.kronk/kronk.log`. Manage it with:

```shell
kronk server logs
kronk server stop
```

If you selected another data root, use the same `--base-path` or
`KRONK_BASE_PATH` for logs and stop commands. A background launch can return
while startup is still in progress; follow the logs and verify the API before
sending inference requests.

The server defaults to `127.0.0.1:11435` for local-only access. The API and BUI
share port `11435`; the BUI is enabled by default and is served under
`/admin/`. Disable it for a headless server with:

```shell
kronk server start \
  --api-host=127.0.0.1:11435 \
  --web-admin-enabled=false
```

Peer downloads are separately controlled by `KRONK_DOWNLOAD_ENABLED`, which
defaults to `false`. It shares this server's models and library bundles with
other Kronk servers on the network.

See [Chapter 8](https://www.kronkai.com/manual#chapter-8-model-server) for server flags, model pooling,
runtime paths, and deployment operations.

#### Run as a Linux Service (systemd)

With Kronk installed in a permanent location, run these as your normal user:

```shell
kronk systemd install
kronk systemd start
kronk systemd status
```

The user service uses your existing `~/.kronk` by default and
enables startup at login. Use `kronk systemd install --boot` instead to
enable startup before login and operation after logout (may require
authorization). Stop any manually started server before starting the service.
Use `kronk systemd stop` and `kronk systemd logs` for this process, not the
detached-server commands. See [Chapter 8 §8.8](https://www.kronkai.com/manual#88-running-as-a-systemd-service)
for configuration, upgrades, and removal.

#### Run as a macOS Agent (launchd)

With Kronk installed in a permanent location, run these as your normal user:

```shell
kronk launchd install
kronk launchd start
kronk launchd status
```

The LaunchAgent uses your existing `~/.kronk` by default and enables startup
at desktop login. Stop any manually started server before starting the
agent. Use `kronk launchd stop` and `kronk launchd logs` to manage it.
This is a per-user agent, not a system-wide service or a before-login boot
service. See [Chapter 8 §8.9](https://www.kronkai.com/manual#89-running-as-a-launchd-agent)
for configuration, upgrades, logs, and removal.

### 2.7 Verify the Installation

Check server liveness:

```shell
curl http://localhost:11435/v1/liveness
```

List models visible to the server:

```shell
curl http://localhost:11435/v1/models
```

Test the starter model from Section 2.1, or substitute the ID of another
downloaded chat model:

```shell
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mradermacher/Qwopus3.5-4B-Coder.Q8_0",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'
```

If authentication is enabled, add
`-H "Authorization: Bearer $KRONK_TOKEN"` to protected API requests, using a
token with the required permissions. Health endpoints remain public. A
successful health check confirms the API is responding, not that a particular
model can load and generate; the chat request checks that separately.

### 2.8 Security and Next Steps

The native server defaults to open API access. Do not publish port `11435`
to an untrusted network until authentication and TLS termination are configured.
The BUI, management endpoints, and inference endpoints have separate security
controls; see [Chapter 12: Security & Authentication](https://www.kronkai.com/manual#chapter-12-security-and-authentication)
before operating a shared or internet-reachable server.

Continue with:

- [Chapter 3: Model Configuration](https://www.kronkai.com/manual#chapter-3-model-configuration) for
  memory, context, GPU, cache, and sampling settings.
- [Chapter 8: Model Server](https://www.kronkai.com/manual#chapter-8-model-server) for server operation
  and deployment.
- [Chapter 12: Security & Authentication](https://www.kronkai.com/manual#chapter-12-security-and-authentication)
  before exposing the service.
- [Chapter 14: Client Integration](https://www.kronkai.com/manual#chapter-14-client-integration) to connect
  an editor or coding agent.
- [Chapter 18: Bucky](https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription) for speech-to-text.
- [Chapter 19: Malina](https://www.kronkai.com/manual#chapter-19-malina-image-generation) for image generation.

### 2.9 CLI Reference and Shell Completion

The BUI's **Docs → CLI** reference and `kronk --help` list commands, flags,
declared defaults, and examples. Use this manual for workflows, environment
settings, and service configuration. Defaults shown in help are not necessarily
the effective server settings: saved configuration and environment variables
can supply values for flags you do not set. Chapter 8 describes server defaults
and configuration precedence.

Shell completion is available for Bash, Fish, PowerShell, and Zsh. For the
current session:

```shell
# Bash
source <(kronk completion bash)

# Fish
kronk completion fish | source

# PowerShell
kronk completion powershell | Out-String | Invoke-Expression

# Zsh (initialize compinit first if your shell has not already done so)
autoload -Uz compinit && compinit
source <(kronk completion zsh)
```

For future sessions, save Fish output to `~/.config/fish/completions/kronk.fish`,
or add the PowerShell command to your profile. Save Bash output to your installed
bash-completion directory (`/etc/bash_completion.d/kronk` on many Linux systems,
or `$(brew --prefix)/etc/bash_completion.d/kronk` with Homebrew). For Zsh, save the
output as `_kronk` in a directory on `$fpath`, then initialize completion in your
shell startup file. Create the destination directory if needed and use a location
your account can write to. `kronk completion <shell> --help` describes shell-specific
options such as omitting descriptions.

---
