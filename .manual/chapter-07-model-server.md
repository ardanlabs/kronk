# Chapter 7: Model Server

## Table of Contents

- [7.1 Starting the Server](#71-starting-the-server)
- [7.2 Stopping the Server](#72-stopping-the-server)
- [7.3 Server Configuration](#73-server-configuration)
- [7.4 Model Caching](#74-model-caching)
- [7.5 Model Config Files](#75-model-config-files)
- [7.6 Catalog System](#76-catalog-system)
- [7.7 Runtime Settings](#77-runtime-settings)
- [7.8 Logging](#78-logging)
- [7.9 Data Paths](#79-data-paths)
- [7.10 Complete Example](#710-complete-example)

---

The Kronk Model Server provides an OpenAI-compatible REST API for inference.
This chapter covers server configuration, management, and the catalog system.

**CLI Modes: Web vs Local**

Most CLI commands communicate with a running server by default:

```shell
kronk catalog list                  # Talks to server at localhost:11435
kronk model pull Qwen3-0.6B-Q8_0    # Downloads via server
```

Add `--local` to run commands directly without a server:

```shell
kronk catalog list --local          # Direct file access
kronk model pull Qwen3-0.6B-Q8_0 --local
kronk libs --local
```

Use `--local` when:

- The server isn't running yet
- You're setting up on the same machine where the server will run
- You prefer direct file operations

Use web mode (no flag) when:

- The server is running
- You want progress streaming in the BUI
- You're managing a remote server via `KRONK_WEB_API_HOST`

**Environment Variables**

Every command-line flag has a corresponding environment variable. The naming
convention is `KRONK_` followed by the flag name in uppercase with hyphens
replaced by underscores:

```
--api-host        →  KRONK_WEB_API_HOST
--models-in-cache →  KRONK_CACHE_MODELS_IN_CACHE
--cache-ttl       →  KRONK_CACHE_TTL
--processor       →  KRONK_PROCESSOR
--hf-token        →  KRONK_HF_TOKEN
```

Environment variables are useful for:

- Configuration in Docker/Kubernetes deployments
- Setting defaults without repeating flags
- Keeping secrets out of command history

### 7.1 Starting the Server

**Install the CLI** (if not already installed)

```shell
go install github.com/ardanlabs/kronk/cmd/kronk@latest
```

**Basic Start**

```shell
kronk server start
```

The server starts on `http://localhost:11435` by default.

**Background Mode**

Run the server as a background process:

```shell
kronk server start -d
```

**Custom Host/Port**

```shell
kronk server start --api-host=0.0.0.0:9000
```

### 7.2 Stopping the Server

```shell
kronk server stop
```

### 7.3 Server Configuration

Configuration can be set via command-line flags or environment variables. Every
flag has a corresponding environment variable using the `KRONK_` prefix with
underscores replacing hyphens.

**Web Settings**

| Flag                     | Environment Variable             | Default           | Description                  |
| ------------------------ | -------------------------------- | ----------------- | ---------------------------- |
| `--api-host`             | `KRONK_WEB_API_HOST`             | `localhost:11435` | API host address             |
| `--debug-host`           | `KRONK_WEB_DEBUG_HOST`           | `localhost:8090`  | Debug/pprof host address     |
| `--read-timeout`         | `KRONK_WEB_READ_TIMEOUT`         | `30s`             | HTTP read timeout            |
| `--write-timeout`        | `KRONK_WEB_WRITE_TIMEOUT`        | `15m`             | HTTP write timeout           |
| `--idle-timeout`         | `KRONK_WEB_IDLE_TIMEOUT`         | `1m`              | HTTP idle timeout            |
| `--shutdown-timeout`     | `KRONK_WEB_SHUTDOWN_TIMEOUT`     | `1m`              | Graceful shutdown timeout    |
| `--cors-allowed-origins` | `KRONK_WEB_CORS_ALLOWED_ORIGINS` | `*`               | Comma-separated CORS origins |

**Authentication Settings**

| Flag             | Environment Variable       | Default         | Description                                               |
| ---------------- | -------------------------- | --------------- | --------------------------------------------------------- |
| `--auth-host`    | `KRONK_AUTH_HOST`          | _(empty)_       | External auth service host. Leave empty to use local auth |
| `--auth-enabled` | `KRONK_AUTH_LOCAL_ENABLED` | `false`         | Enable local JWT authentication                           |
| `--auth-issuer`  | `KRONK_AUTH_LOCAL_ISSUER`  | `kronk project` | Issuer name for local JWT tokens                          |

**Tracing Settings (Tempo)**

| Flag                   | Environment Variable       | Default          | Description                          |
| ---------------------- | -------------------------- | ---------------- | ------------------------------------ |
| `--tempo-host`         | `KRONK_TEMPO_HOST`         | `localhost:4317` | OpenTelemetry collector host         |
| `--tempo-service-name` | `KRONK_TEMPO_SERVICE_NAME` | `kronk`          | Service name for traces              |
| `--tempo-probability`  | `KRONK_TEMPO_PROBABILITY`  | `0.25`           | Trace sampling probability (0.0-1.0) |

**Cache & Model Configuration Settings**

| Flag                  | Environment Variable            | Default                       | Description                                                                                         |
| --------------------- | ------------------------------- | ----------------------------- | --------------------------------------------------------------------------------------------------- |
| `--model-config-file` | `KRONK_CACHE_MODEL_CONFIG_FILE` | `<base>/model_config.yaml`    | Path to per-model configuration overrides. Defaults to the file under your `--base-path`.           |
| `--models-in-cache`   | `KRONK_CACHE_MODELS_IN_CACHE`   | `2`                           | Maximum distinct models kept loaded in memory                                                       |
| `--cache-ttl`         | `KRONK_CACHE_TTL`               | `20m`                         | How long an unused model stays loaded                                                               |

**Runtime Settings**

| Flag                 | Environment Variable     | Default    | Description                                               |
| -------------------- | ------------------------ | ---------- | --------------------------------------------------------- |
| `--base-path`        | `KRONK_BASE_PATH`        | `~/.kronk` | Base directory for all Kronk data                         |
| `--lib-path`         | `KRONK_LIB_PATH`         | _(empty)_  | Override path Kronk loads llama.cpp libraries from. Empty resolves the default per-triple folder under the libraries root (`<base>/libraries/<os>/<arch>/<processor>/`). A directory containing a `version.json` is used as-is. A non-empty directory without a `version.json` is treated as a read-only user-managed build. See chapter 2.3 for full semantics. |
| `--lib-version`      | `KRONK_LIB_VERSION`      | _(empty)_  | Specific llama library version                            |
| `--arch`             | `KRONK_ARCH`             | _(auto)_   | Architecture override (`amd64`, `arm64`)                  |
| `--os`               | `KRONK_OS`               | _(auto)_   | OS override (`linux`, `darwin`, `windows`)                |
| `--processor`        | `KRONK_PROCESSOR`        | _(auto)_   | Processor type (`cpu`, `metal`, `cuda`, `rocm`, `vulkan`) |
| `--hf-token`         | `KRONK_HF_TOKEN`         | _(empty)_  | Hugging Face API token for gated models                   |
| `--allow-upgrade`    | `KRONK_ALLOW_UPGRADE`    | `true`     | Allow automatic library upgrades to the latest llama.cpp release. The server defaults to `true` so a long-running server tracks upstream fixes. The standalone `kronk libs` CLI defaults to `false` (installs the well-known default version) and opts in via `--upgrade`. |
| `--llama-log`        | `KRONK_LLAMA_LOG`        | `1`        | Llama log level (0=off, 1=on)                             |
| `--insecure-logging` | `KRONK_INSECURE_LOGGING` | `false`    | Log sensitive data (messages, model config)               |

**Example**

```shell
kronk server start \
  --api-host=0.0.0.0:11435 \
  --models-in-cache=5 \
  --cache-ttl=30m \
  --model-config-file=./model_config.yaml \
  --hf-token=hf_xxxxx
```

### 7.4 Model Caching

The server maintains a pool of loaded models to avoid reload latency.

**Configuration**

```shell
kronk server start \
  --models-in-cache=3 \
  --cache-ttl=20m
```

- `models-in-cache` - Maximum distinct models kept loaded (default: 2)
- `cache-ttl` - How long an unused model stays loaded (default: 20m)

When a new model is requested and the cache is full, the least recently
used model is unloaded.

### 7.5 Model Config Files

The server reads per-model overrides from `~/.kronk/model_config.yaml` by
default. Kronk seeds this file from an embedded default on first server
start; your edits are preserved across upgrades.

The file is a flat map keyed by canonical model id (`provider/modelID`,
optionally with a `/variant` suffix). Each entry's keys map 1:1 to
`model.Config` and use kebab-case:

```yaml
# ~/.kronk/model_config.yaml

unsloth/Qwen3-0.6B-Q8_0:
  context-window: 32768
  nseq-max: 4
  cache-type-k: q8_0
  cache-type-v: q8_0
  incremental-cache: true

unsloth/Ministral-3-14B-Instruct-2512-Q4_0:
  context-window: 8192
  ngpu-layers: 0
  split-mode: row

ggml-org/embeddinggemma-300m-qat-Q8_0:
  nseq-max: 2
```

To point at an alternative file (for testing without modifying your main
one):

```shell
kronk server start --model-config-file=./my-test-config.yaml
```

Or via environment variable:

```shell
export KRONK_CACHE_MODEL_CONFIG_FILE=/path/to/model_config.yaml
kronk server start
```

**Project Reference Configuration**

The Kronk repository includes a comprehensive reference configuration with
recommended settings for various models and use cases at
`zarf/kms/model_config.yaml`. It includes:

- Optimized configurations for coding agents (Cline, OpenCode)
- YaRN extended context examples
- IMC configuration for message caching
- Vision and audio model settings
- Detailed comments explaining each configuration option
- Examples of YAML anchors for sharing common settings between variants

### 7.6 Catalog System

The catalog (`~/.kronk/catalog.yaml`) is your **personal** catalog of
models. On first run Kronk seeds it from an embedded starter list so you
have something to choose from immediately; the catalog grows as you pull
models or resolve new IDs against HuggingFace.

Each entry is a resolution cache — provider, family (HF repo), revision,
file list, sizes, optional MMProj projection, and detected capabilities.
Templates come from the GGUF metadata of the downloaded model itself and
are not stored here.

**List entries in the catalog**

```shell
kronk catalog list
```

Output (real columns):

```
VAL   MODEL ID                                       PROVIDER   FAMILY                              ARCH      MTMD   SIZE
✓     ggml-org/embeddinggemma-300m-qat-Q8_0          ggml-org   embeddinggemma-300m-qat-q8_0-GGUF   bert      -      329.0 MB
✓     unsloth/Qwen3-0.6B-Q8_0                        unsloth    Qwen3-0.6B-GGUF                     qwen3     -      699.0 MB
✗     unsloth/Ministral-3-14B-Instruct-2512-Q4_0     unsloth    Ministral-3-14B-Instruct-2512-GGUF  llama     -      8.0 GB
```

`VAL` indicates whether the model files have been downloaded and
validated locally; `MTMD` indicates a multimodal projection (mmproj) is
present.

**Show catalog entry details**

```shell
kronk catalog show unsloth/Qwen3-0.6B-Q8_0
```

**Pull (download) a model**

```shell
kronk model pull unsloth/Qwen3-0.6B-Q8_0
```

After the pull completes, the catalog entry is enriched with the resolved
provider, family, revision, and file sizes so subsequent lookups don't
need to hit HuggingFace.

**Remove a catalog entry**

```shell
kronk catalog remove unsloth/Qwen3-0.6B-Q8_0   # also removes downloaded files
```

The same operations are available in the BUI's Catalog and Model views.

### 7.7 Runtime Settings

**Processor Selection**

```shell
kronk server start --processor=cuda    # NVIDIA GPU
kronk server start --processor=metal   # Apple Silicon
kronk server start --processor=vulkan  # Cross-platform GPU
kronk server start --processor=rocm    # AMD GPU (ROCm/HIP)
kronk server start --processor=cpu     # CPU only
```

**Library Path and Version Pinning**

You can point to a custom library directory and pin a specific llama.cpp version for stability:

```shell
kronk server start \
  --lib-path=/custom/path/to/libraries \
  --lib-version=b8864
```

Or via environment variable:

```shell
KRONK_LIB_VERSION=b8864 kronk server start
```

Breaking changes in llama.cpp can cause incompatibilities with yzma and Kronk. Use `--lib-version` (or `KRONK_LIB_VERSION`) to lock the server to a known-good version:

| llama.cpp | yzma    | kronk  |
| --------- | ------- | ------ |
| b8864     | v1.12.0 | 1.23.1 |
| b8865+    | v1.13.0 | 1.23.2 |

If you set `--allow-upgrade=false`, automatic library upgrades are disabled and the server will only use the version you have installed.

**Hugging Face Token**

For gated models requiring authentication:

```shell
kronk server start --hf-token=hf_xxxxx
```

Or via environment variable:

```shell
export KRONK_HF_TOKEN=hf_xxxxx
kronk server start
```

### 7.8 Logging

**llama.cpp Logging**

```shell
kronk server start --llama-log=1    # Enable llama.cpp logs
kronk server start --llama-log=0    # Disable (default)
```

**Insecure Logging**

Enable logging of message content (for debugging only):

```shell
kronk server start --insecure-logging=true
```

**Warning:** This logs sensitive data. Never use in production.

**View Server Logs**

```shell
kronk server logs
```

### 7.9 Data Paths

Default data locations:

```
~/.kronk/
├── catalog.yaml                        # Personal catalog (resolution cache + provider list)
├── model_config.yaml                   # Per-model configuration overrides
├── libraries/                          # llama.cpp libraries (one folder per triple)
│   └── <os>/<arch>/<processor>/        # e.g. darwin/arm64/metal/, linux/amd64/cuda/
│       ├── libllama.so / .dylib / .dll
│       └── version.json
└── models/                             # Downloaded model files
    ├── .index.yaml                     # Local file index (validated state per model)
    └── <provider>/<family>/<file>.gguf
```

Each `(arch, os, processor)` library install lives in its own folder.
The runtime loads the folder for the detected triple by default; set
`KRONK_LIB_PATH` to a different triple folder (and restart) to switch
active install. See chapter 2.3 for `KRONK_LIB_PATH` semantics and the
install-management commands.

**Custom Base Path**

```shell
kronk server start --base-path=/data/kronk
```

`--base-path` shifts every file above to live under the new root.

### 7.10 Complete Example

Production-ready server configuration:

```shell
kronk server start \
  --api-host=0.0.0.0:11435 \
  --models-in-cache=2 \
  --cache-ttl=20m \
  --model-config-file=/etc/kronk/model_config.yaml \
  --processor=cuda \
  --auth-enabled=true \
  -d
```

With model config:

```yaml
# /etc/kronk/model_config.yaml

unsloth/Qwen3-0.6B-Q8_0:
  context-window: 32768
  nseq-max: 4
  cache-type-k: q8_0
  cache-type-v: q8_0
  incremental-cache: true
```

---
