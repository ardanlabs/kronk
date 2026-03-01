# Chapter 2: Installation & Quick Start

## Table of Contents

- [2.1 Prerequisites](#21-prerequisites)
- [2.2 Installing the CLI](#22-installing-the-cli)
- [2.3 Installing Libraries](#23-installing-libraries)
- [2.4 Downloading Your First Model](#24-downloading-your-first-model)
- [2.5 Starting the Server](#25-starting-the-server)
- [2.6 Verifying the Installation](#26-verifying-the-installation)
- [2.7 Quick Start Summary](#27-quick-start-summary)
- [2.8 NixOS Setup](#28-nixos-setup)

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
  kronk catalog pull Qwen3-8B-Q8_0 --local

  # Start the server (runs on http://localhost:8080)
  kronk server start

  # Open the Browser UI
  open http://localhost:8080

FEATURES
  • Text, Vision, Audio, Embeddings, Reranking
  • Metal, CUDA, ROCm, Vulkan, CPU acceleration
  • Batch processing, message caching, YaRN context extension
  • Model pooling, catalog system, browser UI
  • MCP service, security, observability

MODES
  Web mode (default)  - Communicates with running server at localhost:8080
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

Open http://localhost:8080 in your browser and navigate to the Libraries page.

**Option B: Via CLI**

```shell
kronk libs --local
```

This downloads libraries to `~/.kronk/libraries/` using auto-detected settings.

**Environment Variables for Library Installation**

```
KRONK_LIB_PATH  - Library directory (default: `~/.kronk/libraries`)
KRONK_PROCESSOR - `cpu`, `cuda`, `metal`, `rocm`, or `vulkan` (default: `cpu`)
KRONK_ARCH      - Architecture override: `amd64`, `arm64`
KRONK_OS        - OS override: `linux`, `darwin`, `windows`
```

**Example: Install CUDA Libraries**

```shell
KRONK_PROCESSOR=cuda kronk libs --local
```

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
kronk catalog pull Qwen3-8B-Q8_0 --local
```

Models are stored in `~/.kronk/models/` by default.

### 2.5 Starting the Server

Start the Kronk Model Server:

```shell
kronk server start
```

The server starts on `http://localhost:8080` by default. You'll see output like:

```
Kronk Model Server started
API: http://localhost:8080
BUI: http://localhost:8080
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

### 2.6 Verifying the Installation

**Test via curl**

```shell
curl http://localhost:8080/v1/models
```

You should see a list of available models.

**Test Chat Completion**

_Note: It might take a few seconds the first time you call this because the
model needs to be loaded into memory first._

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'
```

**Test via BUI**

Open `http://localhost:8080` in your browser and navigate to the `Apps/Chat` app. Select the model you want to try and chat away.

### 2.7 Quick Start Summary

```shell
# 1. Install Kronk
go install github.com/ardanlabs/kronk/cmd/kronk@latest

# 2. Start the server (auto-installs libraries on first run)
kronk server start

# 3. Open BUI and download a model
open http://localhost:8080

# 4. Download via the BUI Catalog/List screen or use this CLI call
kronk catalog pull Qwen3-8B-Q8_0 --local

# 5. Test the API using this curl call or the BUI App/Chat screen
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "Qwen3-8B-Q8_0", "messages": [{"role": "user", "content": "Hello!"}]}'
```

### 2.8 NixOS Setup

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
