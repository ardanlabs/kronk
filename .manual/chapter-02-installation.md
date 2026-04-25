# Chapter 2: Installation & Quick Start

## Table of Contents

- [2.1 Prerequisites](#21-prerequisites)
- [2.2 Installing the CLI](#22-installing-the-cli)
- [2.3 Installing Libraries](#23-installing-libraries)
- [2.4 Downloading Your First Model](#24-downloading-your-first-model)
- [2.5 Starting the Server](#25-starting-the-server)
- [2.6 Model Configuration File](#26-model-configuration-file)
- [2.7 Verifying the Installation](#27-verifying-the-installation)
- [2.8 Quick Start Summary](#28-quick-start-summary)
- [2.9 NixOS Setup](#29-nixos-setup)

---

### 2.1 Prerequisites

**Required**

- Go 1.26 or later
- Internet connection (for downloading libraries and models)

**Recommended**

- GPU with Metal (macOS), CUDA (NVIDIA), or Vulkan support
- 16GB+ system RAM (96GB+ Recommended)

### 2.2 Installing the CLI

Install Kronk using Go:

```shell
go install github.com/ardanlabs/kronk/cmd/kronk@latest
```

Verify the installation:

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
  catalog   Manage model catalogs (list, pull, show, update)
  model     Manage local models (list, pull, remove, show, ps)
  libs      Install/upgrade llama.cpp libraries
  security  Manage API keys and JWT tokens
  run       Run a model directly for interactive chat (no server needed)

QUICK START
  # List available models
  kronk catalog list --local

  # Download a model (e.g., Qwen3-8B)
  kronk catalog pull Qwen3-0.6B-Q8_0 --local

  # Start the server (runs on http://localhost:11435)
  kronk server start

  # Open the Browser UI
  open http://localhost:11435

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
  catalog     Manage model catalogs (list, pull, show, update)
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  libs        Install or upgrade llama.cpp libraries
  model       Manage local models (index, list, pull, remove, show, ps)
  run         Run an interactive chat session with a model
  security    Manage API security (keys and tokens)
  server      Start, stop, and manage the Kronk model server

Flags:
      --base-path string   Base path for kronk data (models, templates, catalog)
  -h, --help               help for kronk
  -v, --version            version for kronk

Use "kronk [command] --help" for more information about a command.
```

### 2.3 Installing Libraries

Before running inference, you need the llama.cpp libraries for your machine. Kronk auto-detects your hardware and downloads the appropriate binaries.

**Option A: Via the Server**

Start the server and use the BUI to download libraries:

```shell
kronk server start
```

Open http://localhost:11435 in your browser and navigate to the Libraries page.

**Option B: Via CLI**

```shell
kronk libs --local
```

This downloads libraries to `~/.kronk/libraries/<os>/<arch>/<processor>/`
using auto-detected settings (for example
`~/.kronk/libraries/darwin/arm64/metal/`). Each `(arch, os, processor)`
triple lives in its own folder so multiple bundles can coexist on the same
machine.

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

### 2.4 Downloading Your First Model

Kronk provides a curated catalog of verified models. List available models:

```shell
kronk catalog list --local
```

Output:

```
CATALOG              MODEL ID                                 ARCH     SIZE       PULLED   ENDPOINT
Rerank               bge-reranker-v2-m3-Q8_0                  Dense    636.0 MB   yes      rerank
Text-Generation      cerebras_Qwen3-Coder-REAP-25B-A3B-Q8_0   MoE      26.5 GB    yes      chat_completion
Embedding            embeddinggemma-300m-qat-Q8_0             Dense    329.0 MB   yes      embeddings
Image-Text-to-Text   GLM-4.6V-UD-Q5_K_XL                      MoE      80.3 GB    yes      chat_completion
```

Download a model (recommended starter: Qwen3-8B):

```shell
kronk catalog pull Qwen3-0.6B-Q8_0 --local
```

Models are stored in `~/.kronk/models/` by default.

### 2.5 Starting the Server

Start the Kronk Model Server:

```shell
kronk server start
```

The server starts on `http://localhost:11435` by default. You'll see output like:

```
Kronk Model Server started
API: http://localhost:11435
BUI: http://localhost:11435
```

**Running in Background**

To run the server as a background process:

```shell
kronk server start -d
```

**Stopping the Server**

```shell
kronk server stop
```

### 2.6 Model Configuration File

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
Qwen3-8B-Q8_0:
  context-window: 32768
  sampling-parameters:
    temperature: 0.7
    top_p: 0.8
    top_k: 20

gemma-4-26B-A4B-it-UD-Q4_K_M/AGENT:
  context-window: 131072
  nseq-max: 2
  sampling-parameters:
    temperature: 1.0
    top_k: 64
    top_p: 0.95

Qwen3-8B-Q8_0/YARN:
  context-window: 131072
  rope-scaling-type: yarn
  yarn-orig-ctx: 32768
```

The `/YARN` suffix is a **config variant** — it lets you define multiple configurations for the same model. When making an API request, use the full variant name (e.g., `Qwen3-8B-Q8_0/YARN`) as the `model` field to select that configuration.

**Available Options**

The file includes a commented reference at the top listing every option. Here are the most commonly used:

| Option                | Description                                            | Default |
| --------------------- | ------------------------------------------------------ | ------- |
| `context-window`      | Max tokens the model can process per request           | 8192    |
| `ngpu-layers`         | GPU layers to offload (0 = all, -1 = none)             | 0       |
| `flash-attention`     | Flash Attention mode: `enabled`, `disabled`, `auto`    | auto    |
| `incremental-cache`   | Enable IMC for agentic workflows                       | true    |
| `nseq-max`            | Max parallel sequences for batched inference           | 0       |
| `nbatch`              | Logical batch size                                     | 2048    |
| `nubatch`             | Physical batch size for prompt ingestion               | 512     |
| `cache-type-k`        | KV cache key quantization: `f16`, `q8_0`, `q4_0`, etc. | —       |
| `cache-type-v`        | KV cache value quantization                            | —       |
| `sampling-parameters` | Nested block for temperature, top_p, top_k, min_p      | —       |

For the complete list of options and detailed explanations, see [Chapter 3: Model Configuration](chapter-03-model-configuration.md).

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

When the server loads a model, configuration is resolved through three tiers (highest priority wins):

1. **model_config.yaml** — Your local overrides (highest priority)
2. **Catalog defaults** — Settings from the model's catalog entry
3. **Built-in defaults** — Fallback values from the model package

This means any option you set in `model_config.yaml` overrides what the catalog provides for that model.

**Tips**

- You can configure models that aren't in the catalog — just use the model's file name (without `.gguf`) as the key.
- Use YAML anchors (`&name` and `<<: *name`) to share common settings between variants. The default file includes examples of this pattern.
- The `--model-config` server flag lets you point to an alternative config file for testing without modifying your main one.

### 2.7 Verifying the Installation

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

Open `http://localhost:11435` in your browser and navigate to the `Apps/Chat` app. Select the model you want to try and chat away.

### 2.8 Quick Start Summary

```shell
# 1. Install Kronk
go install github.com/ardanlabs/kronk/cmd/kronk@latest

# 2. Start the server (auto-installs libraries on first run)
kronk server start

# 3. Open BUI and download a model
open http://localhost:11435

# 4. Download via the BUI Catalog/List screen or use this CLI call
kronk catalog pull Qwen3-0.6B-Q8_0 --local

# 5. Test the API using this curl call or the BUI App/Chat screen
curl http://localhost:11435/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "Qwen3-0.6B-Q8_0", "messages": [{"role": "user", "content": "Hello!"}]}'
```

### 2.9 NixOS Setup

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

The Go binary is built once with `CGO_ENABLED=0`, then wrapped per backend so
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
