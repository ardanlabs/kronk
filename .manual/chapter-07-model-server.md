# Chapter 7: Model Server

## Table of Contents

- [7.1 Starting the Server](#71-starting-the-server)
- [7.2 Stopping the Server](#72-stopping-the-server)
- [7.3 Server Configuration](#73-server-configuration)
- [7.4 Model Caching](#74-model-caching)
- [7.5 Model Config Files](#75-model-config-files)
- [7.6 Catalog System](#76-catalog-system)
- [7.7 Custom Catalog Repository](#77-custom-catalog-repository)
- [7.8 Templates](#78-templates)
- [7.9 Runtime Settings](#79-runtime-settings)
- [7.10 Logging](#710-logging)
- [7.11 Data Paths](#711-data-paths)
- [7.12 Complete Example](#712-complete-example)

---



The Kronk Model Server provides an OpenAI-compatible REST API for inference.
This chapter covers server configuration, management, and the catalog system.

**CLI Modes: Web vs Local**

Most CLI commands communicate with a running server by default:

```shell
kronk catalog list                # Talks to server at localhost:8080
kronk catalog pull Qwen3-8B-Q8_0  # Downloads via server
```

Add `--local` to run commands directly without a server:

```shell
kronk catalog list --local        # Direct file access
kronk catalog pull Qwen3-8B-Q8_0 --local
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
--models-in-cache →  KRONK_MODELS_IN_CACHE
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

The server starts on `http://localhost:8080` by default.

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

| Flag                     | Environment Variable             | Default          | Description                  |
| ------------------------ | -------------------------------- | ---------------- | ---------------------------- |
| `--api-host`             | `KRONK_WEB_API_HOST`             | `localhost:8080` | API host address             |
| `--debug-host`           | `KRONK_WEB_DEBUG_HOST`           | `localhost:8090` | Debug/pprof host address     |
| `--read-timeout`         | `KRONK_WEB_READ_TIMEOUT`         | `30s`            | HTTP read timeout            |
| `--write-timeout`        | `KRONK_WEB_WRITE_TIMEOUT`        | `15m`            | HTTP write timeout           |
| `--idle-timeout`         | `KRONK_WEB_IDLE_TIMEOUT`         | `1m`             | HTTP idle timeout            |
| `--shutdown-timeout`     | `KRONK_WEB_SHUTDOWN_TIMEOUT`     | `1m`             | Graceful shutdown timeout    |
| `--cors-allowed-origins` | `KRONK_WEB_CORS_ALLOWED_ORIGINS` | `*`              | Comma-separated CORS origins |

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

**Catalog Settings**

| Flag                    | Environment Variable              | Default        | Description                                            |
| ----------------------- | --------------------------------- | -------------- | ------------------------------------------------------ |
| `--catalog-github-repo` | `KRONK_CATALOG_GITHUB_REPO`       | GitHub API URL | GitHub repo URL for catalog files                      |
| `--model-config-file`   | `KRONK_CATALOG_MODEL_CONFIG_FILE` | _(empty)_      | Path to model-specific config YAML file                |
| `--catalog-repo-path`   | `KRONK_CATALOG_REPO_PATH`         | _(empty)_      | Path to cloned catalog repository for publishing edits |

**Template Settings**

| Flag                      | Environment Variable          | Default        | Description                        |
| ------------------------- | ----------------------------- | -------------- | ---------------------------------- |
| `--templates-github-repo` | `KRONK_TEMPLATES_GITHUB_REPO` | GitHub API URL | GitHub repo URL for template files |

**Cache Settings**

| Flag                       | Environment Variable                 | Default | Description                               |
| -------------------------- | ------------------------------------ | ------- | ----------------------------------------- |
| `--models-in-cache`        | `KRONK_CACHE_MODELS_IN_CACHE`        | `3`     | Maximum models kept loaded in memory      |
| `--cache-ttl`              | `KRONK_CACHE_TTL`                    | `20m`   | How long unused models stay loaded        |
| `--ignore-integrity-check` | `KRONK_CACHE_IGNORE_INTEGRITY_CHECK` | `true`  | Skip SHA256 integrity check on model load |

**Runtime Settings**

| Flag                 | Environment Variable     | Default    | Description                                               |
| -------------------- | ------------------------ | ---------- | --------------------------------------------------------- |
| `--base-path`        | `KRONK_BASE_PATH`        | `~/.kronk` | Base directory for all Kronk data                         |
| `--lib-path`         | `KRONK_LIB_PATH`         | _(empty)_  | Path to llama library directory                           |
| `--lib-version`      | `KRONK_LIB_VERSION`      | _(empty)_  | Specific llama library version                            |
| `--arch`             | `KRONK_ARCH`             | _(auto)_   | Architecture override (`amd64`, `arm64`)                  |
| `--os`               | `KRONK_OS`               | _(auto)_   | OS override (`linux`, `darwin`, `windows`)                |
| `--processor`        | `KRONK_PROCESSOR`        | _(auto)_   | Processor type (`cpu`, `metal`, `cuda`, `rocm`, `vulkan`) |
| `--hf-token`         | `KRONK_HF_TOKEN`         | _(empty)_  | Hugging Face API token for gated models                   |
| `--allow-upgrade`    | `KRONK_ALLOW_UPGRADE`    | `true`     | Allow automatic library upgrades                          |
| `--llama-log`        | `KRONK_LLAMA_LOG`        | `1`        | Llama log level (0=off, 1=on)                             |
| `--insecure-logging` | `KRONK_INSECURE_LOGGING` | `false`    | Log sensitive data (messages, model config)               |

**Example**

```shell
kronk server start \
  --api-host=0.0.0.0:8080 \
  --models-in-cache=5 \
  --cache-ttl=30m \
  --model-config-file=model-config.yaml \
  --catalog-repo-path=~/code/kronk_catalogs \
  --hf-token=hf_xxxxx
```

### 7.4 Model Caching

The server maintains a pool of loaded models to avoid reload latency.

**Configuration**

```shell
kronk server start \
  --models-in-cache=3 \
  --cache-ttl=5m
```

- `models-in-cache` - Maximum models kept loaded (default: 3)
- `cache-ttl` - How long unused models stay loaded (default: 5m)

When a new model is requested and the cache is full, the least recently
used model is unloaded.

### 7.5 Model Config Files

Create a YAML file to configure model-specific settings:

```yaml
# model-config.yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 32768
    n_seq_max: 4
    cache_type_k: q8_0
    cache_type_v: q8_0
    system_prompt_cache: true

  Llama-3.3-70B-Instruct-Q8_0:
    context_window: 8192
    n_gpu_layers: 0
    split_mode: row

  embeddinggemma-300m-qat-Q8_0:
    n_seq_max: 2
```

Start with the config file:

```shell
kronk server start --model-config-file=model-config.yaml
```

Or via environment variable:

```shell
export KRONK_CATALOG_MODEL_CONFIG_FILE=/path/to/model-config.yaml
kronk server start
```

**Project Reference Configuration**

The Kronk repository includes a comprehensive reference configuration with
recommended settings for various models and use cases:

```shell
export KRONK_CATALOG_MODEL_CONFIG_FILE=<clone_path>/zarf/kms/model_config.yaml
kronk server start
```

This file includes:

- Optimized configurations for coding agents (Cline, OpenCode)
- YaRN extended context examples
- SPC and IMC variants for different caching strategies
- Vision and audio model settings
- Detailed comments explaining each configuration option

Review `zarf/kms/model_config.yaml` for examples of YAML anchors, cache
configurations, and model-specific tuning.

### 7.6 Catalog System

The catalog provides a curated list of verified models with preconfigured
settings.

**List Available Models**

```shell
kronk catalog list
```

Output:

```
CATALOG              MODEL ID                         PULLED  ENDPOINT
Audio-Text-to-Text   Qwen2-Audio-7B.Q8_0              no      chat_completion
Embedding            embeddinggemma-300m-qat-Q8_0     no      embeddings
Image-Text-to-Text   gemma-3-4b-it-q4_0               no      chat_completion
Text-Generation      Qwen3-8B-Q8_0                    yes     chat_completion
Text-Generation      Llama-3.3-70B-Instruct-Q8_0      no      chat_completion
```

**Filter by Category**

```shell
kronk catalog list --filter-category=Embedding
```

**Pull a Model**

```shell
kronk catalog pull Qwen3-8B-Q8_0
```

**Show Model Details**

```shell
kronk catalog show Qwen3-8B-Q8_0
```

**Update Catalog**

_Note: We don't have a server version of this yet._

```shell
kronk catalog update --local
```

### 7.7 Custom Catalog Repository

Use a custom catalog repository:

```shell
kronk server start \
  --catalog-github-repo=https://github.com/myorg/my-catalog
```

### 7.8 Templates

Templates define chat formatting (Jinja templates) for different models.
Kronk downloads templates automatically from the offical templates repository.

https://github.com/ardanlabs/kronk_catalogs

You don't need this unless you want to maintain your own repository.

**Custom Templates Repository**

```shell
kronk server start \
  --templates-github-repo=https://github.com/myorg/my-templates
```

Templates are cached in `~/.kronk/templates/` by default.

### 7.9 Runtime Settings

**Processor Selection**

```shell
kronk server start --processor=cuda    # NVIDIA GPU
kronk server start --processor=metal   # Apple Silicon
kronk server start --processor=vulkan  # Cross-platform GPU
kronk server start --processor=rocm    # AMD GPU (ROCm/HIP)
kronk server start --processor=cpu     # CPU only
```

**Library Path**

```shell
kronk server start \
  --lib-path=/custom/path/to/libraries \
  --lib-version=b7406
```

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

### 7.10 Logging

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

### 7.11 Data Paths

Default data locations:

```
~/.kronk/
├── libraries/     # llama.cpp libraries
├── models/        # Downloaded models
├── templates/     # Chat templates
└── catalog/       # Catalog cache
```

**Custom Base Path**

```shell
kronk server start --base-path=/data/kronk
```

### 7.12 Complete Example

Production-ready server configuration:

```shell
kronk server start \
  --api-host=0.0.0.0:8080 \
  --models-in-cache=2 \
  --cache-ttl=10m \
  --model-config-file=/etc/kronk/models.yaml \
  --processor=cuda \
  --auth-enabled=true \
  -d
```

With model config:

```yaml
# /etc/kronk/models.yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 32768
    n_seq_max: 4
    cache_type_k: q8_0
    cache_type_v: q8_0
    incremental_cache: true
```

---

