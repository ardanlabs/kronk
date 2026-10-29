# Chapter 8: Model Server

## Table of Contents

- [8.1 Server Lifecycle](#81-server-lifecycle)
- [8.2 Local and Server-Backed CLI Commands](#82-local-and-server-backed-cli-commands)
- [8.3 Essential Server Configuration](#83-essential-server-configuration)
- [8.4 Model Pool and Resource Budgets](#84-model-pool-and-resource-budgets)
- [8.5 Model Configuration Files](#85-model-configuration-files)
- [8.6 Catalog Operations](#86-catalog-operations)
- [8.7 Container Operations](#87-container-operations)
- [8.8 Running as a systemd Service](#88-running-as-a-systemd-service)
- [8.9 Running as a launchd Agent](#89-running-as-a-launchd-agent)
- [8.10 Kubernetes with Helm](#810-kubernetes-with-helm)
- [8.11 Related Administration Guides](#811-related-administration-guides)

---

The Kronk model server provides OpenAI-compatible inference APIs and manages
downloaded models, native libraries, and loaded model instances. This chapter
focuses on operating that server. Installation is covered in
[Chapter 2](https://www.kronkai.com/manual#chapter-2-installation-quick-start), while model-level tuning belongs in
[Chapter 3](https://www.kronkai.com/manual#chapter-3-model-configuration).

## 8.1 Server Lifecycle

Start the server in the foreground:

```shell
kronk server start
```

The API listens on `127.0.0.1:11435` by default and is reachable only from the
local machine. To expose it on another interface, set `--api-host` explicitly,
restrict access with a firewall or private network, and configure the
authorization described in [Chapter 12](https://www.kronkai.com/manual#chapter-12-security-and-authentication).

To bind only to the local machine:

```shell
kronk server start --api-host=127.0.0.1:11435
```

Run the server in the background with:

```shell
kronk server start --detach
```

Detached mode records the process ID and redirects server output to these
paths by default:

```text
~/.kronk/kronk.pid
~/.kronk/kronk.log
```

Setting `KRONK_BASE_PATH` before starting the detached server moves both files
under that root.

Use these commands for a detached server:

```shell
kronk server logs
kronk server stop
```

`server logs` follows the detached log file, and `server stop` signals the
process recorded in the PID file. A foreground server logs to its terminal and
should be stopped with the normal terminal or service-manager signal instead.

The server handles `SIGINT` and `SIGTERM` and allows in-flight work to stop
within the configured shutdown timeout.

## 8.2 Local and Server-Backed CLI Commands

Model and catalog commands use the running server by default:

```shell
kronk catalog list
kronk model pull unsloth/Qwen3-0.6B-Q8_0
```

The client connects to `localhost:11435` unless `KRONK_WEB_API_HOST` or the
corresponding host flag selects another server.

Add `--local` to operate directly on files and libraries without contacting a
server:

```shell
kronk catalog list --local
kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local
kronk libs --local
```

Local mode is useful for initial setup, offline administration, and installing
models into a stopped server's data directory. Use server-backed mode when
administering a remote host or when browser progress needs to observe the
operation.

## 8.3 Essential Server Configuration

Common settings can be supplied as flags or environment variables:

| Flag | Environment variable | Effective default | Purpose |
| ---- | -------------------- | ----------------- | ------- |
| `--api-host` | `KRONK_WEB_API_HOST` | `127.0.0.1:11435` | Main API bind address |
| `--debug-host` | `KRONK_WEB_DEBUG_HOST` | `127.0.0.1:11445` | Metrics and profiling bind address |
| `--inference-timeout` | `KRONK_WEB_INFERENCE_TIMEOUT` | `60m` | Total timeout for inference admission, preparation, slot waiting, and generation |
| `--write-timeout` | `KRONK_WEB_WRITE_TIMEOUT` | `61m` | HTTP response write timeout; `0` disables it, otherwise it must exceed the inference timeout |
| `--base-path` | `KRONK_BASE_PATH` | `~/.kronk` | Root for Kronk data |
| `--lib-path` | `KRONK_LIB_PATH` | Detected bundle under `<base>/libraries` | Exact llama.cpp library directory or libraries root |
| `--lib-version` | `KRONK_LIB_VERSION` | unset | llama.cpp release, optionally pinned as `VERSION@sha256:<64-hex-digest>` |
| `--lib-verify-enabled` | `KRONK_LIB_VERIFY_ENABLED` | `true` | Verify the selected llama.cpp and whisper.cpp bundles before loading them |
| `--processor` | `KRONK_PROCESSOR` | Detected host backend | Processor bundle such as `metal`, `cuda`, `rocm`, `vulkan`, or `cpu` |
| `--arch` | `KRONK_ARCH` | Host architecture | Library-bundle architecture override |
| `--os` | `KRONK_OS` | Host operating system | Library-bundle operating-system override |
| `--bucky-lib-path` | `KRONK_BUCKY_LIB_PATH` | Detected bundle under `<base>/bucky-libraries` | Exact whisper.cpp library directory |
| `--model-config-file` | `KRONK_POOL_MODEL_CONFIG_FILE` | `<base>/models/model_config.yaml` | Per-model overrides |
| `--budget-percent` | `KRONK_POOL_BUDGET_PERCENT` | `90` | Memory-budget input for loaded models |
| `--models-in-pool` | `KRONK_POOL_MODELS_IN_POOL` | `10` | Maximum loaded entries in each model pool |
| `--pool-ttl` | `KRONK_POOL_TTL` | `0m` | Idle model retention time; `0` disables idle expiration |
| `--web-admin-enabled` | `KRONK_WEB_ADMIN_ENABLED` | `true` | Serve the BUI under `/admin/` |
| `--authorization-mode` | `KRONK_AUTHORIZATION_MODE` | `open` | Select the API access policy |
| `--download-enabled` | `KRONK_DOWNLOAD_ENABLED` | `false` | Share this server's models and library bundles with other Kronk servers on the network (peer downloads) |
| `--lib-download-enabled` | `KRONK_LIB_DOWNLOAD_ENABLED` | `true` | Download or update the llama.cpp library during server startup |
| `--media-backends-enabled` | `KRONK_MEDIA_BACKENDS_ENABLED` | `true` | Download and load the Bucky and Malina native backends during server startup |
| `--allow-upgrade` | `KRONK_ALLOW_UPGRADE` | `false` | Opt in to automatic native-library upgrades |
| `--llama-log` | `KRONK_LLAMA_LOG` | `0` | Enable or disable llama.cpp logging |

Most server configuration flags map to environment variables, but names follow
the server's configuration hierarchy rather than a universal text conversion.
For example, `--budget-percent` maps to `KRONK_POOL_BUDGET_PERCENT`.
`--detach` is a CLI process-control flag and has no environment equivalent.

Run the following for the complete current list, including HTTP timeouts,
CORS, tracing, external authentication, processor selection, and library
overrides:

```shell
kronk server start --help
```

Keep tokens and passwords in protected environment or secret-manager settings,
not shared shell scripts. `--insecure-logging` can expose prompts and model
configuration and should be limited to controlled debugging.

### Runtime selection and overrides

With no processor or library-path override, server startup detects a preferred
llama.cpp backend and verifies it before native libraries are loaded. CUDA 13
requires compute capability 7.5 or newer on every visible NVIDIA GPU. Older or
hidden CUDA devices cause host detection to prefer an available ROCm or Vulkan
runtime and then CPU. If compatibility cannot be determined conclusively,
Kronk retains the preferred backend.

Kronk also probes the installed preferred accelerator bundle. It changes to a
different installed bundle only when the preferred bundle positively reports
no accelerator and a same-version alternative positively reports a device.
This probe never installs another bundle. The startup log records the preferred
and selected processors and the reason for the decision.

`--processor` and `--lib-path`, or their environment equivalents, are strict
operator choices and disable automatic fallback. A custom non-empty library
directory without `version.json` is treated as a read-only user-managed build;
the server loads it but does not upgrade or replace it. `--base-path` moves the
managed library root along with other Kronk data, while `--lib-path` selects the
llama.cpp location specifically. Restart the server after changing any native
library selection setting.

Set `--lib-download-enabled=false` or `KRONK_LIB_DOWNLOAD_ENABLED=false` to
skip the automatic llama.cpp download at startup. Kronk still loads the library
selected by `--lib-path` or `KRONK_LIB_PATH`.

Set `--media-backends-enabled=false` or
`KRONK_MEDIA_BACKENDS_ENABLED=false` to skip automatic Bucky and Malina bundle
downloads and native backend initialization. This does not affect llama.cpp
download or initialization.

Library verification is enabled by default. Set `--lib-verify-enabled=false` or
`KRONK_LIB_VERIFY_ENABLED=false` to disable it. Kronk verifies the selected
llama.cpp bundle before its device probe and verifies the selected whisper.cpp
bundle before either backend loads native code. A failure leaves only the
affected backend in degraded mode.

For an externally pinned llama.cpp check, set `--lib-version` or
`KRONK_LIB_VERSION` to `VERSION@sha256:<64-hex-digest>`. The digest identifies
Yzma's release manifest, which identifies the platform archives and their
installed files. The default llama.cpp and whisper.cpp versions already include
their published manifest digests. The builders publish each manifest as a
release asset, GitHub records its SHA-256, and Kronk carries that digest as the
trust anchor. Yzma and Bucky validate the manifest and selected archive during
download; startup verification then hashes the extracted files against the
authenticated manifest. Post-install verification requires an install record
and per-file hashes in the release manifest.

Bucky performs separate whisper.cpp runtime selection. Its managed bundles
live below `<base>/bucky-libraries`, and `KRONK_BUCKY_LIB_PATH` authoritatively
selects a different bundle or user-managed build. It does not use
`--lib-path`/`KRONK_LIB_PATH`. See
[Chapter 18 §18.2](https://www.kronkai.com/manual#182-install-whisper-libraries)
for Bucky's CUDA, Vulkan, and CPU fallback rules.

## 8.4 Model Pool and Resource Budgets

Kronk keeps loaded models in memory to avoid paying model-load latency on every
request. Three settings govern retention:

- `budget-percent` controls memory admission.
- `models-in-pool` places a count limit on each backend pool.
- `pool-ttl` unloads entries that remain unused past the configured duration.
  Set it to `0` to keep loaded models resident regardless of inactivity. This
  does not prevent eviction caused by the pool-count limit, memory pressure,
  explicit unload, or server shutdown.

At the default `budget-percent: 90`, each discrete GPU receives a 90% budget
minus 256 MiB of headroom. Host RAM receives an 85% budget because Kronk reserves
an additional five percentage points for the operating system, allocators, and
memory not represented in model estimates. Apple Silicon unified memory is
accounted as one host-memory pool rather than independent RAM and Metal VRAM.

Admission uses predicted model, KV-cache, and runtime memory. These predictions
are planning estimates, not a guarantee that every backend allocation will
succeed. Context size, cache types, sequence count, CPU offload, and model
architecture all affect the estimate.

After a model and its target, draft/MTP, and projection contexts are initialized,
Kronk checks the backend's live free-memory report before publishing the model to
the pool. Metal, CUDA, and ROCm loads that no longer retain the configured
headroom are unloaded and rejected with an insufficient-memory error. Vulkan is
logged as advisory because drivers without the memory-budget extension can
report the entire heap as free. CPU backends do not expose a portable live-memory
value and skip this check.

On multi-GPU systems, Kronk accounts for llama.cpp's model distribution across
the selected devices. Automatic splits use available GPUs, while explicit
`devices` and `tensor-split` configuration control the proportions. Each
assigned share must fit within that GPU's individual budget; unused capacity on
another card cannot satisfy an over-budget share.

When a new load exceeds the count or memory budget, Kronk evicts an idle model.
For memory pressure it prefers an idle entry that frees enough memory without
unloading a needlessly large model, then falls back to the coldest idle entry.
Models with active streams are not evicted. If no idle entry can make room, the
request returns a server-busy error and the client should retry later.

The Bucky and LLM pools share the same byte budget, so transcription and
language-model loads can compete for memory. Bucky installation and pool
behavior are covered in [Chapter 18](https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription).

Resource usage and eviction events are available through the logging and
metrics described in [Chapter 15](https://www.kronkai.com/manual#chapter-15-observability).

## 8.5 Model Configuration Files

The server reads per-model overrides from:

```text
~/.kronk/models/model_config.yaml
```

Kronk seeds the file on first use and preserves edits across upgrades. Entries
are merged over hardware-analysis recommendations rather than replacing the
entire runtime configuration.

Version 2 makes this the single configuration file for the server and all model
backends. Version 1 files containing only `kms` and `models` remain readable.
The top-level shape is:

```yaml
version: 2
kms:
  web:
    api-host: 127.0.0.1:11435
    debug-host: 127.0.0.1:11445
    read-timeout: 30s
    write-timeout: 61m
    inference-timeout: 60m
    idle-timeout: 1m
    shutdown-timeout: 1m
    cors-allowed-origins: ["*"]
    admin:
      enabled: true
      session-duration: 12h
      password-sha-256: 18511e63760230cd17291273b607e7e13da2a2bb9a1750e0becdac08185a3c11
  auth:
    host: ""
    tls:
      enabled: false
      ca-file: ""
      server-name: ""
    local:
      issuer: kronk project
  authorization:
    mode: open # open, management, authenticated, or full-protected
  mcp:
    enabled: true
    host: ""
    auth-enabled: false
    brave-api-key: ""
  download:
    enabled: false
  tempo:
    host: localhost:4317
    service-name: kronk
    probability: 0.25
  pool:
    budget-percent: 90
    models-in-pool: 10
    ttl: 0m
  base-path: ""
  lib-path: ""
  bucky-lib-path: ""
  lib-version: ""
  lib-download-enabled: true
  media-backends-enabled: true
  arch: ""
  os: ""
  processor: ""
  allow-upgrade: false
  insecure-logging: false
  hf-token: ""
  llama-log: 0
models:
  owner/model:
    context-window: 8192
    nseq-max: 2
bucky-models:
  tiny:
    nseq-max: 2
malina-models:
  sd-1.5:
    concurrency: 2
```

The built-in defaults apply when a `kms` key is omitted. Configuration
precedence is built-in defaults, then `kms` YAML, then `KRONK_*` environment
variables, then explicitly supplied `kronk server start` flags. The
`--model-config-file` flag and `KRONK_POOL_MODEL_CONFIG_FILE` environment
variable select the YAML file itself and therefore remain outside the file.
The backend sections are top-level mappings alongside `models`. Bucky keys are
the short IDs shown by `kronk bucky model list`; Malina keys are bundle IDs
shown by `kronk malina model list`. Restart the server after changing them.

Automatic tuning is enabled by default in the model server. An explicit
`context-window` or `nseq-max` in this file is treated as a fixed sizing
constraint, not a value the tuner may reduce. When KV cache types are omitted,
the server tries `f16` and then `q8_0` for that exact context and concurrency;
it never automatically quantizes below `q8_0`. See
[Chapter 3 §3.2](https://www.kronkai.com/manual#32-automatic-tuning) for the
complete selection order and recommended configuration style.

Use another file without replacing the default:

```shell
kronk server start --model-config-file=./my-model_config.yaml
```

or:

```shell
KRONK_POOL_MODEL_CONFIG_FILE=./my-model_config.yaml kronk server start
```

The file format, variants, configuration keys, and tuning workflow are
documented in [Chapter 3](https://www.kronkai.com/manual#chapter-3-model-configuration). The repository's
commented reference file is `zarf/kms/model_config.yaml`.

## 8.6 Catalog Operations

The personal model catalog is stored at
`~/.kronk/catalog/catalog.yaml`. Kronk seeds it with a starter catalog and adds
resolved model information as models are discovered or downloaded.

Common operations are:

```shell
# List catalog entries and local validation state.
kronk catalog list

# Inspect one entry.
kronk catalog show unsloth/Qwen3-0.6B-Q8_0

# Download a model and reconcile its catalog metadata.
kronk model pull unsloth/Qwen3-0.6B-Q8_0

# Remove the catalog entry and its downloaded files.
kronk catalog remove unsloth/Qwen3-0.6B-Q8_0
```

Catalog entries identify the provider, source family, revision, files, sizes,
and detected capabilities. Chat templates come from downloaded GGUF metadata
and are not stored as catalog configuration.

Catalog schema version 2 adds the optional `mtp_url` field. Set it to a fully
qualified Hugging Face download URL when a compatible MTP companion lives in a
different repository from its target model. When it is absent, Kronk retains
the version 1 behavior and builds the companion URL from the entry's provider,
family, revision, and `mtp_orig` fields. Version 1 catalogs remain readable and
are stamped as version 2 the next time the current Kronk version rewrites them;
their entries do not require a data migration.

Source specificity controls catalog resolution:

- A canonical `provider/modelID` selects one provider explicitly.
- `provider/repo:quantization` pins the repository and selects the matching
  quantization there.
- `owner/repo/file.gguf`, or an equivalent Hugging Face `blob` or `resolve`
  URL, pins both the repository and upstream filename.
- A repository root or tree URL returns its GGUF files for explicit selection
  rather than choosing one automatically.

Exact file pins never fall back to a different repository or to another
catalog entry with the same quantization suffix. This matters when target and
MTP drafter files have similar names: companion discovery remains inside the
pinned repository, preventing a cached target, drafter, or unrelated model
from being substituted. The resulting canonical ID can still reflect Kronk's
normal on-disk rename rules, such as the `mtp-` prefix for a dedicated MTP
repository.

Use `--local` for the same operations when the server is stopped. The BUI also
provides catalog and model views when enabled; see
[Chapter 13](https://www.kronkai.com/manual#chapter-13-browser-ui-bui).

## 8.7 Container Operations

Chapter 2 covers image variants and initial container startup. For a persistent
deployment, use a versioned image tag and retain `/kronk` in a volume. This
headless example fully protects the API and exposes it only through the host
loopback interface:

```shell
docker run -d \
  --name kronk \
  --restart unless-stopped \
  -e KRONK_AUTHORIZATION_MODE=full-protected \
  -e KRONK_WEB_ADMIN_ENABLED=false \
  -p 127.0.0.1:11435:11435 \
  -v kronk-data:/kronk \
  ghcr.io/ardanlabs/kronk:vX.Y.Z-cpu
```

Choose the processor-specific tag documented in Chapter 2. Terminate TLS at a
reverse proxy or keep the service on a trusted private network. Read
[Chapter 12](https://www.kronkai.com/manual#chapter-12-security-and-authentication) before exposing an
authenticated server remotely.

Install and inspect models directly in the persistent volume without enabling
browser downloads:

```shell
docker exec kronk kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local
docker exec kronk kronk catalog list --local
```

Inspect the running container with:

```shell
docker logs -f kronk
docker exec kronk kronk --version
curl http://localhost:11435/v1/liveness
```

To update a pinned image, pull the new tag and recreate the container with the
same volume and settings:

```shell
docker pull ghcr.io/ardanlabs/kronk:vX.Y.Z-cpu
docker stop kronk
docker rm kronk
# Repeat the docker run command with the new versioned tag.
```

Models, configuration, catalog state, and authentication keys remain in the
named volume. Removing `kronk-data` permanently deletes that state and is not
part of a normal image update.

**Docker Compose**

The Compose files in
[`zarf/docker/kronk`](https://github.com/ardanlabs/kronk/tree/main/zarf/docker/kronk)
use the same container name (`kronk`) and volume (`kronk-data`) as the
examples above, so the `docker exec` and `docker logs` commands work
unchanged. Start with
[2.3 Container Quick Start](https://www.kronkai.com/manual#23-container-quick-start).
All `docker compose` commands run from the directory holding the files.

Without a repository checkout, download only the Compose files. Replace `main`
with the release tag of the image you pin:

```shell
mkdir kronk && cd kronk
for f in compose.yaml compose.cuda.yaml compose.rocm.yaml compose.vulkan.yaml; do
  curl -fsSLO "https://raw.githubusercontent.com/ardanlabs/kronk/main/zarf/docker/kronk/$f"
done
```

These variables configure the Compose files. Set them in the shell or in a
`.env` file in the Compose directory:

| Variable | Default | Purpose |
| -------- | ------- | ------- |
| `KRONK_IMAGE_VERSION` | `latest` | `latest` or a release such as `vX.Y.Z`; pin a release outside local testing |
| `KRONK_BIND_ADDR` | `127.0.0.1` | Host address for ports `11435` and `11445` |
| `KRONK_DOWNLOAD_ENABLED` | `false` | Share models and library bundles with other Kronk servers (peer downloads) |
| `KRONK_RENDER_GID` / `KRONK_VIDEO_GID` | `render` / `video` | Host group IDs for GPU devices (ROCm, Vulkan) |
| `KRONK_TEMPO_HOST` | unset | OTLP endpoint, e.g. `tempo:4317` |

Only set `KRONK_BIND_ADDR=0.0.0.0` on a trusted network: port `11445` serves
pprof and metrics without authentication, and Docker's published ports are
not blocked by host firewalls such as ufw.

Do not put server settings in `.env`. Compose uses `.env` only for the
variables that the Compose files refer to, such as those in the table above. Any
other variable in `.env` never reaches the container, and Compose gives no
warning. For example, `KRONK_AUTHORIZATION_MODE=full-protected` in `.env`
leaves the server unprotected. Pass server settings, such as the
authentication settings above, through a `compose.override.yaml` next to
the Compose files:

```yaml
services:
  kronk:
    environment:
      - KRONK_AUTHORIZATION_MODE=full-protected
      - KRONK_WEB_ADMIN_ENABLED=false
```

A plain `docker compose up -d` loads `compose.override.yaml` automatically.
With `-f` or `COMPOSE_FILE`, list it last. To avoid repeating `-f`, create a
`.env` such as this one, which selects Vulkan, loads the override, and pins
the image (the separator line makes it work on Windows too):

```text
COMPOSE_PATH_SEPARATOR=:
COMPOSE_FILE=compose.vulkan.yaml:compose.override.yaml
KRONK_IMAGE_VERSION=vX.Y.Z
```

In a repository checkout, `make kronk-up KRONK_GPU=vulkan` (or `cuda`, `rocm`)
starts the same from the repository root and adds the override when it exists.

The GPU files need these host components:

| File | Host requirements |
| -------- | ----------------- |
| `compose.cuda.yaml` | NVIDIA driver and [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html) |
| `compose.rocm.yaml` | `linux/amd64`, ROCm-capable kernel driver, `/dev/kfd` and `/dev/dri` |
| `compose.vulkan.yaml` | Mesa Vulkan drivers, `/dev/dri` |

- Check the backend in use with
  `docker compose logs kronk | grep "installing/updating libraries"`; the
  `processor` field shows it. `compose.yaml` sets `KRONK_PROCESSOR=cpu`,
  because the image's software Vulkan driver would otherwise make detection
  pick `vulkan` without a GPU.
- NVIDIA: to use specific cards, replace `count: all` in `compose.cuda.yaml`
  with `device_ids: ["0"]` (indices or UUIDs from `nvidia-smi -L`).
- ROCm: `HIP_VISIBLE_DEVICES` and `HSA_OVERRIDE_GFX_VERSION` are passed
  through. For an unlisted GPU that is not detected, try
  `HSA_OVERRIDE_GFX_VERSION=10.3.0` (RDNA2) or `11.0.0` (RDNA3). The ROCm
  image is about 30 GB unpacked; on integrated GPUs, Vulkan is usually smaller
  and faster.
- Vulkan: `VK_LOADER_DRIVERS_SELECT` and `MESA_VK_DEVICE_SELECT` are passed
  through to pick one GPU.
- Each file is self-contained and pulls the image for its own backend
  (`-cpu`, `-cuda`, `-rocm`, `-vulkan`). To run the `all` image, change the
  image tag in the matching GPU file to `-all`.
- macOS is CPU only; Windows uses `compose.cuda.yaml` with Docker Desktop and
  WSL2. Startup errors are covered in
  [17.2 Libraries and Devices](https://www.kronkai.com/manual#172-libraries-and-devices).

To upgrade, run from the Compose directory with the same `-f` files or `.env`:

- Pinned version: change `KRONK_IMAGE_VERSION` to the new release, then run
  `docker compose up -d`. If you downloaded the Compose files for a release
  tag, download them again for the new tag first.
- `latest`: run `docker compose pull`, then `docker compose up -d`.

The `kronk-net` network is shared with the
[observability stack](https://www.kronkai.com/manual#153-bundled-observability-stack)
and can remain after both stacks stop; remove it with
`docker network rm kronk-net`.

## 8.8 Running as a systemd Service

On Linux, `kronk systemd` installs and manages a systemd **user service**.
It runs as your normal user and uses your existing `~/.kronk` models,
libraries, and keys. It is separate from `kronk server start --detach`:
systemd owns the process, restarts it after failures, and stores its logs
in the journal. Do not use `sudo kronk systemd`.

Install Kronk in a permanent location using `go install`, Homebrew, or a
release binary, then run:

```shell
kronk systemd install
kronk systemd start
kronk systemd status
curl http://localhost:11435/v1/liveness
```

Stop any existing foreground or detached server first to avoid port and
database-lock conflicts. Use Ctrl-C for a foreground server or
`kronk server stop` for a detached server. Once systemd manages the server,
use `kronk systemd stop` and `kronk systemd logs` instead.

### Installation and Startup

The service definition is embedded in the executable; there is no separate
download or shell script. Installation records the absolute path of the
running executable and writes `~/.config/systemd/user/kronk.service`.
With `XDG_CONFIG_HOME` set, that directory replaces `~/.config` for both
the service file and the optional configuration file below. Do not install
using `go run`: its temporary executable will disappear.

Installation enables startup at login but does not start the server.
To start at boot without logging in and keep running after logout, use:

```shell
kronk systemd install --boot
```

`--boot` runs `loginctl enable-linger` for your account. It may require
authorization and affects all your user services. If already installed,
use `loginctl enable-linger` directly, or reinstall with `--force --boot`.
A machine-wide service with a dedicated system account is not installed.

### Configuration

The optional `~/.config/kronk/kronk.env` contains server environment settings
(see §8.3). Create its directory if needed and protect sensitive settings:

```shell
mkdir -p ~/.config/kronk
touch ~/.config/kronk/kronk.env
chmod 600 ~/.config/kronk/kronk.env
```

For example:

```text
KRONK_AUTHORIZATION_MODE=management
# Optional GPU settings; use values appropriate to your host:
# KRONK_PROCESSOR=rocm
# PATH=/opt/rocm/bin:/usr/local/bin:/usr/bin:/bin
# LD_LIBRARY_PATH=/opt/rocm/lib
```

This is not a shell script: use `NAME=value`, without `export`, and absolute
paths, not `~`, `$HOME`, or `$PATH` substitutions. The service does not read
your shell startup files. GPU detection tools and native libraries must be
available in its environment. Configure GPU-specific values here if a
working interactive shell depends on them.

`--base-path` or `KRONK_BASE_PATH` when installing records a custom data
directory in the unit; `KRONK_BASE_PATH` in `kronk.env` overrides that value.
Other shell settings are not copied into the unit automatically. By default
the API listens on `127.0.0.1:11435`. Configure authorization before exposing
it on a public interface with `KRONK_WEB_API_HOST`.

### Managing and Removing the Service

```shell
kronk systemd stop
kronk systemd start
kronk systemd restart
kronk systemd status
kronk systemd logs
kronk systemd uninstall
```

Restart after editing `kronk.env` or upgrading the binary at the same path.
If the binary moves, run `kronk systemd install --force` to update its path,
then restart. An existing unit is not overwritten without `--force`.
`status` reports systemd process state, not HTTP readiness; startup can
include library downloads. Check `/v1/liveness` before sending requests.

Use `systemctl --user edit kronk` for systemd overrides. The unit allows
150 seconds to stop. If increasing `KRONK_WEB_SHUTDOWN_TIMEOUT` above its
default of one minute, set `TimeoutStopSec` to at least twice that timeout
plus 30 seconds. The unit requires systemd 240 or newer (`Type=exec`).

Uninstall stops and disables the service, removes the unit, and reloads
systemd. It preserves models, libraries, keys, `kronk.env`, and systemd
drop-ins. It also preserves user lingering because other services may
depend on it; use `loginctl disable-linger` only if none do.

## 8.9 Running as a launchd Agent

On macOS, `kronk launchd` installs and manages a per-user **LaunchAgent**.
It runs as you, reuses your existing `~/.kronk` models, libraries, keys, and
server configuration, and starts at desktop login. It is not a system-wide
LaunchDaemon and does not run before login or continue after logout.
Run the commands as your normal user, not with `sudo`; a macOS desktop
login session must be available.

Install Kronk in a permanent location using `go install`, Homebrew, or a
release binary, then run:

```shell
kronk launchd install
kronk launchd start
kronk launchd status
curl http://localhost:11435/v1/liveness
```

Installation writes the embedded definition to
`~/Library/LaunchAgents/com.ardanlabs.kronk.plist` and enables startup at
login, without loading or starting the agent immediately. It records the
absolute path used to invoke Kronk, preserving stable symlinks such as
Homebrew's `/opt/homebrew/bin/kronk` (Apple Silicon) or
`/usr/local/bin/kronk` (Intel). Do not install using `go run`: its temporary
executable will disappear.

Stop any existing foreground server with Ctrl-C, or a detached server with
`kronk server stop`, before starting the agent. The agent runs
`kronk server start` in the foreground; launchd owns its process and logs.
Use `kronk launchd stop` and `kronk launchd logs` for that process, not the
detached-server commands.

### LaunchAgent Configuration

`--base-path` or `KRONK_BASE_PATH` at installation records a custom data
directory. Otherwise Kronk uses `~/.kronk`. Server settings in the existing
`model_config.yaml` continue to apply (see §8.5). To override settings for
the agent, edit its plist's `EnvironmentVariables` dictionary. For example,
add these entries inside that dictionary:

```xml
<key>KRONK_WEB_API_HOST</key>
<string>127.0.0.1:11435</string>
<key>KRONK_AUTHORIZATION_MODE</key>
<string>management</string>
```

Launchd does not source shell startup files or read the systemd
`kronk.env` file. Other shell settings are not automatically copied at
installation. Use literal values and absolute paths, not `~`, `$HOME`, or
`$PATH` substitutions. The plist includes a `PATH` covering both standard
Homebrew locations and macOS system tools. Its initial permissions are
`0600`; keep it private if adding sensitive environment values.

Validate an edited plist, then reload it:

```shell
plutil -lint ~/Library/LaunchAgents/com.ardanlabs.kronk.plist
kronk launchd restart
```

The default API listener remains local-only at `127.0.0.1:11435`. Configure
authorization before exposing it on a public interface. `status` shows
launchd's agent information, not HTTP readiness; startup may include library
downloads. Check `/v1/liveness` before sending requests.

### LaunchAgent Lifecycle and Logs

```shell
kronk launchd stop
kronk launchd start
kronk launchd restart
kronk launchd status
kronk launchd logs
kronk launchd uninstall
```

`RunAtLoad` starts the server when the agent is loaded. `KeepAlive` restarts
it if the process exits, with a 10-second throttle for repeated short-lived
runs. `stop` unloads the agent so it stays stopped for the current login
session, until explicitly started again. It starts again at the next
desktop login. `restart` gracefully unloads and reloads the plist, applying
configuration changes rather than just killing the process. The plist
requests a 150-second `ExitTimeOut` for graceful HTTP shutdown and model
unload, but macOS can cap this: a 60-second effective limit was observed on
the development host. Inspect `kronk launchd status` for the effective exit
timeout. If shutdown can exceed it, reduce `KRONK_WEB_SHUTDOWN_TIMEOUT` so
twice that timeout plus 30 seconds fits the available grace period. Merely
increasing `ExitTimeOut` does not guarantee a longer grace period.

Both standard output and standard error append to
`~/Library/Logs/kronk/kronk.log`. `logs` follows that file, including after
rotation; Ctrl-C stops following, not the server. Arrange log rotation if
running long-term, since launchd does not impose a size limit on this file.

Restart after upgrading the binary at the same path. If its location
changes, reinstall with `kronk launchd install --force`, then restart.
`--force` replaces the plist and resets manual plist edits; save those edits
first. Models, keys, server YAML configuration, and logs are preserved.
Uninstall unloads and disables the agent and removes its plist, retaining
server data and logs. Any environment settings stored only in the removed
plist are removed with it.

## 8.10 Kubernetes with Helm

The chart in `zarf/helm/kronk` runs the published image as one server with a
retained data PVC; it does not provide synchronized HA. Requirements are an
existing Kubernetes 1.27+ cluster, Helm 3, kubectl, a default StorageClass (or
configured storage), and enough node memory/disk for your models. The default
claim is `50Gi` RWO. Image and model downloads require network access.

From a repository checkout, copy the CPU example to your own values file and
install into the intended cluster:

```shell
kubectl config current-context
cp zarf/helm/kronk/examples/cpu.yaml my-kronk.yaml
helm upgrade --install kronk zarf/helm/kronk -n kronk --create-namespace \
  -f my-kronk.yaml --wait --timeout 20m
helm test kronk -n kronk
kubectl port-forward -n kronk service/kronk 11435:11435
```

Authentication is enabled and the BUI is disabled by default. Port-forwarding
provides temporary local access; keep authentication enabled and configure TLS
and access restrictions before exposing the API remotely. Optional metrics also
expose unauthenticated pprof and must remain restricted to trusted clients.

GPU deployments require compatible node drivers, device access, and runtime
configuration; selecting an image variant does not provision a GPU. CPU smoke
tests do not validate GPU inference. Uninstall retains data, but deleting its
PVC **or namespace** can destroy models and keys. Back up before maintenance.

See the [Helm chart README](https://github.com/ardanlabs/kronk/blob/main/zarf/helm/kronk/README.md)
for the complete first-request walkthrough, backend selection, configuration,
troubleshooting, upgrades, and storage/key management. Keep deployment overrides
in your own values file rather than editing the chart.

## 8.11 Related Administration Guides

Detailed administration is divided by responsibility:

- [Chapter 2](https://www.kronkai.com/manual#chapter-2-installation-quick-start) — installation, libraries, image
  variants, and data paths
- [Chapter 3](https://www.kronkai.com/manual#chapter-3-model-configuration) — per-model runtime settings
- [Chapter 12](https://www.kronkai.com/manual#chapter-12-security-and-authentication) — authentication, keys,
  tokens, and remote exposure
- [Chapter 13](https://www.kronkai.com/manual#chapter-13-browser-ui-bui) — BUI operation and browser login
- [Chapter 15](https://www.kronkai.com/manual#chapter-15-observability) — logs, health checks, metrics,
  tracing, and profiling
- [Chapter 18](https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription) — transcription libraries, models, and pool
  behavior
