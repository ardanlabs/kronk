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
[Chapter 2](chapter-02-installation.md), while model-level tuning belongs in
[Chapter 3](chapter-03-model-configuration.md).

## 8.1 Server Lifecycle

Start the server in the foreground:

```shell
kronk server start
```

The API listens on `0.0.0.0:11435` by default. `localhost:11435` works for a
client on the same machine, but the server is bound to all network interfaces.
Authentication is disabled by default. Before using an untrusted network,
bind to loopback, restrict access with a firewall or private network, or enable
the authentication described in [Chapter 12](chapter-12-security-authentication.md).

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
| `--api-host` | `KRONK_WEB_API_HOST` | `0.0.0.0:11435` | Main API bind address |
| `--debug-host` | `KRONK_WEB_DEBUG_HOST` | `0.0.0.0:11445` | Metrics and profiling bind address |
| `--base-path` | `KRONK_BASE_PATH` | `~/.kronk` | Root for Kronk data |
| `--model-config-file` | `KRONK_POOL_MODEL_CONFIG_FILE` | `<base>/models/model_config.yaml` | Per-model overrides |
| `--budget-percent` | `KRONK_POOL_BUDGET_PERCENT` | `80` | Memory-budget input for loaded models |
| `--models-in-pool` | `KRONK_POOL_MODELS_IN_POOL` | `10` | Maximum loaded entries in each model pool |
| `--pool-ttl` | `KRONK_POOL_TTL` | `20m` | Idle model retention time |
| `--web-admin-enabled` | `KRONK_WEB_ADMIN_ENABLED` | `true` | Serve the BUI under `/admin/` |
| `--auth-enabled` | `KRONK_AUTH_LOCAL_ENABLED` | `false` | Protect inference and administration with local authentication |
| `--admin-auth-enabled` | `KRONK_AUTH_ADMIN_ENABLED` | `false` | Protect administration without requiring inference authentication |
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

## 8.4 Model Pool and Resource Budgets

Kronk keeps loaded models in memory to avoid paying model-load latency on every
request. Three settings govern retention:

- `budget-percent` controls memory admission.
- `models-in-pool` places a count limit on each backend pool.
- `pool-ttl` unloads entries that remain unused past the configured duration.

At the default `budget-percent: 80`, each discrete GPU receives an 80% budget
minus 256 MiB of headroom. Host RAM receives a 75% budget because Kronk reserves
an additional five percentage points for the operating system, allocators, and
memory not represented in model estimates. Apple Silicon unified memory is
accounted as one host-memory pool rather than independent RAM and Metal VRAM.

Admission uses predicted model, KV-cache, and runtime memory. These predictions
are planning estimates, not a guarantee that every backend allocation will
succeed. Context size, cache types, sequence count, CPU offload, and model
architecture all affect the estimate.

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
behavior are covered in [Chapter 18](chapter-18-bucky.md).

Resource usage and eviction events are available through the logging and
metrics described in [Chapter 15](chapter-15-observability.md).

## 8.5 Model Configuration Files

The server reads per-model overrides from:

```text
~/.kronk/models/model_config.yaml
```

Kronk seeds the file on first use and preserves edits across upgrades. Entries
are merged over hardware-analysis recommendations rather than replacing the
entire runtime configuration.

Use another file without replacing the default:

```shell
kronk server start --model-config-file=./my-model_config.yaml
```

or:

```shell
KRONK_POOL_MODEL_CONFIG_FILE=./my-model_config.yaml kronk server start
```

The file format, variants, configuration keys, and tuning workflow are
documented in [Chapter 3](chapter-03-model-configuration.md). The repository's
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

Use `--local` for the same operations when the server is stopped. The BUI also
provides catalog and model views when enabled; see
[Chapter 13](chapter-13-browser-ui.md).

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
[Chapter 12](chapter-12-security-authentication.md) before exposing an
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

- [Chapter 2](chapter-02-installation.md) — installation, libraries, image
  variants, and data paths
- [Chapter 3](chapter-03-model-configuration.md) — per-model runtime settings
- [Chapter 12](chapter-12-security-authentication.md) — authentication, keys,
  tokens, and remote exposure
- [Chapter 13](chapter-13-browser-ui.md) — BUI operation and browser login
- [Chapter 15](chapter-15-observability.md) — logs, health checks, metrics,
  tracing, and profiling
- [Chapter 18](chapter-18-bucky.md) — transcription libraries, models, and pool
  behavior
