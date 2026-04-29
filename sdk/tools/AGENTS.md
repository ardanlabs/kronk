# AGENTS.md - sdk/tools

CLI tooling support for model and library management.

## Package Overview

- **defaults/** - Default paths, versions, and platform detection (arch/OS/processor)
- **devices/** - Compute device enumeration (CPU/GPU) and system RAM detection
- **downloader/** - HTTP file downloads with progress tracking and HuggingFace auth
- **github/** - GitHub API helpers
- **libs/** - llama.cpp library installation and version management
- **models/** - Local model file management, indexing, validation, resolver, and per-model config

## Directory Structure

Default base directory: `~/.kronk/`

```
~/.kronk/
├── libraries/         # llama.cpp shared libraries
└── models/            # Downloaded model files (GGUF)
    └── <org>/<family>/<model>.gguf
```

## defaults Package

Platform detection and default value resolution.

**Functions:**

- `BaseDir(override)` - Returns base path (`~/.kronk/` or override)
- `Arch(override)` - Detects CPU architecture (checks `KRONK_ARCH` env var)
- `OS(override)` - Detects operating system (checks `KRONK_OS` env var)
- `Processor(override)` - Returns processor type: cpu, cuda, metal, rocm, vulkan (checks `KRONK_PROCESSOR`)
- `LibVersion(override)` - Returns library version (checks `KRONK_LIB_VERSION`)
- `ModelConfigFile(override, basePath)` - Resolves the path to `model_config.yaml`
- `CatalogFile(override, basePath)` - Resolves the path to `catalog.yaml`

## downloader Package

HTTP file downloads with progress tracking.

**Download Function:**

```go
Download(ctx, src, dest, progressFunc, sizeInterval) (downloaded bool, err error)
```

- Supports HuggingFace auth via `KRONK_HF_TOKEN` env var
- Progress callbacks at configurable intervals (`SizeIntervalMIB10` = 10 MB)
- Network availability check before download

**ProgressReader:**

Wraps downloads to provide progress tracking. Implements `io.ReadCloser`.

## libs Package

llama.cpp library installation and version management.

**Key Operations:**

- `Download(ctx, log)` - Full workflow: check version, download if needed, install
- `InstalledVersion()` - Read current installed version from `version.json`
- `VersionInformation()` - Get both installed and latest available versions
- `DownloadVersion(ctx, log, version)` - Download specific version

**Version Tag:**

```go
type VersionTag struct {
    Version   string  // Installed version
    Arch      string  // Architecture (arm64, amd64)
    OS        string  // Operating system (darwin, linux, windows)
    Processor string  // Hardware (cpu, cuda, metal, rocm, vulkan)
    Latest    string  // Latest available version
}
```

**Upgrade Logic:**

- Checks if installed version matches latest
- Compares arch/OS/processor to detect platform changes
- `WithAllowUpgrade(false)` skips automatic upgrades

## models Package

Local model file management, indexing, and resolved configuration.

**Key Operations:**

- `BuildIndex(log)` - Scan models directory, validate files, create index
- `RetrievePath(modelID)` / `FullPath(modelID)` - Get file paths for a model
- `Files()` - List all downloaded models
- `Remove(modelID)` - Delete model files
- `KronkResolvedConfig(modelID, mc)` - Build a `model.Config` from analysis-derived
  defaults + user `model_config.yaml` overrides + sampling defaults
- `AnalysisDefaults(modelID)` - Returns hardware-aware defaults derived from GGUF metadata
- `LoadModelConfig(path)` - Parse a `model_config.yaml` file into a per-model override map
- `ResolveGrammar(*SamplingConfig)` - Resolve `.grm` filename references to grammar text

**Index File:**

`.index.yaml` in the models directory maps model IDs to file paths and validation status.

**Configuration Layering (`KronkResolvedConfig`):**

1. **Layer 1 — Analysis defaults** - hardware-aware values derived from the GGUF
   metadata (context window, batch sizes, cache types, flash attention, GPU layers).
2. **Layer 3 — `model_config.yaml`** - user overrides loaded via `LoadModelConfig`.
3. **Sampling defaults** - `SamplingConfig.WithDefaults()` fills any zero-valued
   sampling fields with the SDK's defaults.

The legacy catalog YAML middle layer is no longer applied.

**`model_config.yaml` Format:**

```yaml
google/gemma-3-4b-it-Q4_K_M:
  context-window: 32768
  nbatch: 2048
  nubatch: 512
  incremental-cache: true
  sampling-parameters:
    temperature: 0.7
    top_k: 40
```

Keys are model IDs. Variant suffixes such as `/IMC` allow distinct configurations
for the same on-disk model.

**Model ID Format:**

- Standard: `org/model-name` (e.g., `google/gemma-3-4b-it-Q4_K_M`)
- With config variant: `org/model-name/IMC` (e.g., `google/gemma-3-4b-it-Q4_K_M/IMC`)

## Environment Variables

| Variable            | Description                                       |
| ------------------- | ------------------------------------------------- |
| `KRONK_ARCH`        | Override CPU architecture detection               |
| `KRONK_OS`          | Override OS detection                             |
| `KRONK_PROCESSOR`   | Set processor type (cpu/cuda/metal/rocm/vulkan)   |
| `KRONK_LIB_VERSION` | Pin llama.cpp library version                     |
| `KRONK_HF_TOKEN`    | HuggingFace authentication token for gated models |
| `GITHUB_TOKEN`      | GitHub personal access token for higher API rate limits |
