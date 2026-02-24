# AGENTS.md - sdk/tools

CLI tooling support for model, catalog, template, and library management.

## Package Overview

- **catalog/** - Catalog system for model metadata, config retrieval, and downloads
- **defaults/** - Default paths, versions, and platform detection (arch/OS/processor)
- **downloader/** - HTTP file downloads with progress tracking and HuggingFace auth
- **libs/** - llama.cpp library installation and version management
- **models/** - Local model file management, indexing, and validation
- **templates/** - Jinja template downloads and management

## Directory Structure

Default base directory: `~/.kronk/`

```
~/.kronk/
├── catalogs/          # Model catalog YAML files
├── libraries/         # llama.cpp shared libraries
├── models/            # Downloaded model files (GGUF)
│   └── <org>/<family>/<model>.gguf
└── templates/         # Jinja chat templates
```

## catalog Package

Manages model catalog system with metadata and configuration.

**Key Types:**

- `Catalog` - Main catalog manager
- `Model` - Model metadata (ID, files, capabilities, config)
- `ModelConfig` - Per-model settings (context window, batch size, cache types, sampling)
- `SamplingConfig` - Inference sampling parameters with `WithDefaults()` for zero-value handling

**Configuration Layering:**

`RetrieveModelConfig()` resolves configuration through a three-tier priority system:

1. **Model package defaults** - `SamplingConfig.WithDefaults()` applies `model.DefTemp`, `model.DefTopK`, etc. for any zero-valued sampling fields
2. **Catalog YAML** - Model-specific settings from catalog files (context window, batch sizes, cache types, sampling params)
3. **model_config.yaml** - User overrides loaded via `--model-config` flag (highest priority)

The model does not need to exist in the catalog. If not found, catalog settings are skipped and only model_config.yaml overrides (if any) plus defaults are applied.

**Resolution Flow:**

```
RetrieveModelConfig(modelID) → ModelConfig
  │
  ├─ 1. Try RetrieveModelDetails(modelID) from catalog
  │     └─ If found: cfg = catalog.ModelConfig
  │
  ├─ 2. Check c.modelConfig[modelID] from model_config.yaml
  │     └─ If found: override non-zero fields into cfg
  │
  └─ 3. cfg.Sampling = cfg.Sampling.WithDefaults()
        └─ Fill zero-valued sampling fields with model package defaults
```

**model_config.yaml Format:**

```yaml
google/gemma-3-4b-it-Q4_K_M:
  context-window: 32768
  nbatch: 2048
  nubatch: 512
  system-prompt-cache: true
  sampling-parameters:
    temperature: 0.7
    top_k: 40

google/gemma-3-4b-it-Q4_K_M/IMC:
  incremental-cache: true
```

Keys are model IDs. The `/IMC` variant allows different configs for the same model.

**Model ID Format:**

- Standard: `org/model-name` (e.g., `google/gemma-3-4b-it-Q4_K_M`)
- With config variant: `org/model-name/IMC` (e.g., `google/gemma-3-4b-it-Q4_K_M/IMC`)

## defaults Package

Platform detection and default value resolution.

**Functions:**

- `BaseDir(override)` - Returns base path (`~/.kronk/` or override)
- `Arch(override)` - Detects CPU architecture (checks `KRONK_ARCH` env var)
- `OS(override)` - Detects operating system (checks `KRONK_OS` env var)
- `Processor(override)` - Returns processor type: cpu, cuda, metal, rocm, vulkan (checks `KRONK_PROCESSOR`)
- `LibVersion(override)` - Returns library version (checks `KRONK_LIB_VERSION`)

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

Local model file management and indexing.

**Key Operations:**

- `BuildIndex(log)` - Scan models directory, validate files, create index
- `RetrievePath(modelID)` - Get file paths for a model
- `RetrieveList()` - List all downloaded models
- `Remove(modelID)` - Delete model files

**Index File:**

`.index.yaml` in models directory maps model IDs to file paths and validation status.

**Model Validation:**

Uses `model.CheckModel()` to verify GGUF file integrity. Validation status cached in index.

## templates Package

Jinja template management for chat formatting.

**Key Operations:**

- `Download(ctx, log)` - Download templates from GitHub repo
- `RetrieveTemplate(modelID)` - Get template for a specific model
- `RetrieveTemplateByName(name)` - Get template by filename

**Template Source:**

Default: `https://api.github.com/repos/ardanlabs/kronk_catalogs/contents/templates`

**Integration:**

Templates package embeds a `Catalog` instance for model lookups. Access via `templates.Catalog()`.

## Environment Variables

| Variable            | Description                                       |
| ------------------- | ------------------------------------------------- |
| `KRONK_ARCH`        | Override CPU architecture detection               |
| `KRONK_OS`          | Override OS detection                             |
| `KRONK_PROCESSOR`   | Set processor type (cpu/cuda/metal/rocm/vulkan)        |
| `KRONK_LIB_VERSION` | Pin llama.cpp library version                     |
| `KRONK_HF_TOKEN`    | HuggingFace authentication token for gated models |
