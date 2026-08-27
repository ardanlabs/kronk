# Chapter 8: Model Server

## Table of Contents

- [8.1 Server Lifecycle](#81-server-lifecycle)
- [8.2 Local and Server-Backed CLI Commands](#82-local-and-server-backed-cli-commands)
- [8.3 Essential Server Configuration](#83-essential-server-configuration)
- [8.4 Model Pool and Resource Budgets](#84-model-pool-and-resource-budgets)
- [8.5 Model Configuration Files](#85-model-configuration-files)
- [8.6 Catalog Operations](#86-catalog-operations)
- [8.7 Container Operations](#87-container-operations)
- [8.8 Related Administration Guides](#88-related-administration-guides)

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
| `--processor` | `KRONK_PROCESSOR` | Detected host backend | Processor bundle such as `metal`, `cuda`, `rocm`, `vulkan`, or `cpu` |
| `--arch` | `KRONK_ARCH` | Host architecture | Library-bundle architecture override |
| `--os` | `KRONK_OS` | Host operating system | Library-bundle operating-system override |
| `--bucky-lib-path` | `KRONK_BUCKY_LIB_PATH` | Detected bundle under `<base>/bucky-libraries` | Exact whisper.cpp library directory |
| `--model-config-file` | `KRONK_POOL_MODEL_CONFIG_FILE` | `<base>/models/model_config.yaml` | Per-model overrides |
| `--budget-percent` | `KRONK_POOL_BUDGET_PERCENT` | `90` | Memory-budget input for loaded models |
| `--models-in-pool` | `KRONK_POOL_MODELS_IN_POOL` | `10` | Maximum loaded entries in each model pool |
| `--pool-ttl` | `KRONK_POOL_TTL` | `0m` | Idle model retention time; `0` disables idle expiration |
| `--web-admin-enabled` | `KRONK_WEB_ADMIN_ENABLED` | `true` | Serve the BUI under `/admin/` |
| `--authorization-mode` | `KRONK_AUTHORIZATION_MODE` | unset | Select the API access policy |
| `--auth-enabled` | `KRONK_AUTH_LOCAL_ENABLED` | `false` | Protect inference and administration with local authentication |
| `--admin-auth-enabled` | `KRONK_AUTH_ADMIN_ENABLED` | `false` | Protect administration without requiring inference authentication |
| `--download-enabled` | `KRONK_DOWNLOAD_ENABLED` | `false` | Allow server-side model downloads |
| `--allow-upgrade` | `KRONK_ALLOW_UPGRADE` | `false` | Opt in to automatic native-library upgrades |
| `--llama-log` | `KRONK_LLAMA_LOG` | `1` | Enable or disable llama.cpp logging |

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

Version 1 makes this the single configuration file for both the server and its
models. The top-level shape is:

```yaml
version: 1
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
      password-sha-256: 18511e63760230cd17291273b607e7e13da2a2bb9a1750e0becdac08185a3c11
  auth:
    host: ""
    admin-enabled: false
    tls:
      enabled: false
      ca-file: ""
      server-name: ""
    local:
      issuer: kronk project
      enabled: false
  authorization:
    # mode: open, management, authenticated, or full-protected
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
  arch: ""
  os: ""
  processor: ""
  allow-upgrade: false
  insecure-logging: false
  hf-token: ""
  llama-log: 1
models:
  owner/model:
    context-window: 8192
    nseq-max: 2
```

The built-in defaults apply when a `kms` key is omitted. Configuration
precedence is built-in defaults, then `kms` YAML, then `KRONK_*` environment
variables, then explicitly supplied `kronk server start` flags. The
`--model-config-file` flag and `KRONK_POOL_MODEL_CONFIG_FILE` environment
variable select the YAML file itself and therefore remain outside the file.
Files without `version` are version 0 and retain the legacy model-only shape,
where model IDs are top-level keys.

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
headless example enables local authentication and exposes the API only through
the host loopback interface:

```shell
docker run -d \
  --name kronk \
  --restart unless-stopped \
  -e KRONK_AUTH_LOCAL_ENABLED=true \
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

## 8.8 Related Administration Guides

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
