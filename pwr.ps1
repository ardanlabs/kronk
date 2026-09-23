# Native Windows equivalents of the development Make targets.
# From the repository root, run `./pwr.ps1 <target>` or `./pwr.ps1` to list targets.
# These commands require neither GNU Make nor a Unix shell.

# Windows PowerShell may block this script by default.
# To allow local scripts for your user in current and future terminal sessions,
# run the following command once:
#
#   Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned
#
# This setting applies to the Windows user, not only to the Kronk repository.
# An organization-managed execution policy may override this setting.

#Requires -Version 5.1

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet(
        "help",
        "setup",
        "install-gotooling",
        "install-tooling",
        "install-kronk",
        "install-libraries",
        "install-libraries-gh",
        "install-test-gh-models",
        "install-test-models",
        "install-test-malina",
        "install-class-models",
        "install-docker",
        "bui-install",
        "bui-build",
        "kronk-docs",
        "kronk-build"
    )]
    [string]$Target = "help"
)

# ==============================================================================
# Setup

# Configure git to use project hooks so pre-commit runs for all developers.
# ./pwr.ps1 setup
function Set-UpRepository {
    Invoke-InDirectory -Path $RepoRoot -Action {
        git config core.hooksPath .githooks
        Assert-NativeCommandSucceeded -Command "git"
    }
}

# ==============================================================================
# Install

# Install the required Go development tools.
# ./pwr.ps1 install-gotooling
function Install-GoTooling {
    Invoke-InDirectory -Path $RepoRoot -Action {
        go install honnef.co/go/tools/cmd/staticcheck@latest
        Assert-NativeCommandSucceeded -Command "go"

        go install golang.org/x/vuln/cmd/govulncheck@latest
        Assert-NativeCommandSucceeded -Command "go"

        go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
        Assert-NativeCommandSucceeded -Command "go"

        go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
        Assert-NativeCommandSucceeded -Command "go"

        go install github.com/nix-community/gomod2nix@latest
        Assert-NativeCommandSucceeded -Command "go"
    }
}

# Install the required Windows development tools with WinGet.
# ./pwr.ps1 install-tooling
function Install-Tooling {
    Install-WingetPackage -Id "Google.Protobuf"
    Install-WingetPackage -Id "fullstorydev.grpcurl"
    Install-WingetPackage -Id "OpenJS.NodeJS.LTS"
    Install-WingetPackage -Id "BurntSushi.ripgrep.MSVC"
    Install-WingetPackage -Id "Gyan.FFmpeg"
}

# Install the Kronk CLI.
# ./pwr.ps1 install-kronk
function Install-Kronk {
    Invoke-InDirectory -Path $RepoRoot -Action {
        go install ./cmd/kronk
        Assert-NativeCommandSucceeded -Command "go"
    }
}

# Use this to install or update llama.cpp, whisper.cpp, and
# stable-diffusion.cpp to the latest version. Used by the local test target so
# developers exercise the newest bundles before bumping each backend's
# well-known defaultVersion for a release.
# ./pwr.ps1 install-libraries
function Install-Libraries {
    Install-Kronk

    Write-Section "INSTALL LLAMA LIBRARIES (latest)"
    kronk libs --local
    Assert-NativeCommandSucceeded -Command "kronk"
    Write-Host

    Write-Section "INSTALL WHISPER LIBRARIES (latest)"
    kronk bucky libs --local
    Assert-NativeCommandSucceeded -Command "kronk"
    Write-Host

    Write-Section "INSTALL STABLE DIFFUSION LIBRARIES (latest)"
    kronk malina libs --local
    Assert-NativeCommandSucceeded -Command "kronk"
    Write-Host
}

# Use this to install the well-known defaultVersion of llama.cpp, whisper.cpp,
# and stable-diffusion.cpp baked into the SDK. This mirrors what CI does so the
# GH test target can be reproduced locally.
# ./pwr.ps1 install-libraries-gh
function Install-LibrariesGh {
    Install-Kronk

    Write-Section "INSTALL LLAMA LIBRARIES (defaultVersion)"
    kronk libs --local
    Assert-NativeCommandSucceeded -Command "kronk"
    Write-Host

    Write-Section "INSTALL WHISPER LIBRARIES (defaultVersion)"
    kronk bucky libs --local
    Assert-NativeCommandSucceeded -Command "kronk"
    Write-Host

    Write-Section "INSTALL STABLE DIFFUSION LIBRARIES (defaultVersion)"
    kronk malina libs --local
    Assert-NativeCommandSucceeded -Command "kronk"
    Write-Host
}

# Use this to install the test GH models.
# ./pwr.ps1 install-test-gh-models
function Install-TestGhModels {
    Install-Kronk

    Write-Section "INSTALL MODELS"
    Install-KronkModel "unsloth/Qwen3.5-0.8B-Q8_0"
    Install-KronkModel "unsloth/Qwen3-1.7B-Q4_K_M"
    Install-KronkModel "unsloth/Qwen3-0.6B-Q8_0"
    Install-KronkModel "mradermacher/Qwopus3.5-4B-Coder.Q4_K_M"
    Install-KronkModel "nomic-ai/nomic-embed-text-v1.5.Q8_0"
    Install-KronkModel "gpustack/bge-reranker-v2-m3-Q8_0"
    Install-BuckyModel "ggml-tiny.bin"
}

# Use this to install the test models.
# ./pwr.ps1 install-test-models
function Install-TestModels {
    Install-Kronk

    Write-Section "INSTALL KRONK MODELS"
    Install-KronkModel "unsloth/Qwen3-0.6B-Q8_0"
    Install-KronkModel "unsloth/Qwen3.5-0.8B-Q8_0"
    Install-KronkModel "mradermacher/Qwopus3.5-4B-Coder.Q4_K_M"
    Install-KronkModel "unsloth/LFM2-700M-Q8_0"
    Install-KronkModel "unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M"
    Install-KronkModel "unsloth/Qwen3.6-35B-A3B-MTP-GGUF:UD-Q2_K_XL"
    Install-KronkModel "ggml-org/Qwen2.5-Omni-3B-Q4_K_M"
    Install-KronkModel "unsloth/gpt-oss-20b-Q8_0"
    Install-KronkModel "unsloth/Qwen3-1.7B-Q4_K_M"
    Install-KronkModel "nomic-ai/nomic-embed-text-v1.5.Q8_0"
    Install-KronkModel "gpustack/bge-reranker-v2-m3-Q8_0"

    Write-Section "INSTALL BUCKY MODELS"
    Install-BuckyModel "ggml-tiny.bin"
    Install-BuckyModel "silero-vad"
}

# Use this to install the library and model required by the opt-in Malina tests.
# ./pwr.ps1 install-test-malina
function Install-TestMalina {
    Install-Kronk

    Write-Section "INSTALL STABLE DIFFUSION LIBRARIES"
    kronk malina libs --local
    Assert-NativeCommandSucceeded -Command "kronk"
    Write-Host

    Write-Section "INSTALL MALINA TEST MODEL"
    Install-MalinaModel "sd-1.5"
}

# Use this to install models for the class.
# ./pwr.ps1 install-class-models
function Install-ClassModels {
    Install-Kronk

    Write-Section "INSTALL MODELS"
    Install-KronkModel "unsloth/Qwen3-0.6B-Q8_0"
    Install-KronkModel "unsloth/Qwen3.5-0.8B-Q8_0"
    Install-KronkModel "mradermacher/Qwopus3.5-4B-Coder.Q8_0"
    Install-KronkModel "ornith-ai/Ornith-1.5-9B-Q4_K_M"
    Install-KronkModel "ggml-org/Qwen2.5-Omni-3B-Q8_0"
    Install-KronkModel "Qwen/Qwen3-Embedding-0.6B-Q8_0.gguf"
    Install-KronkModel "gpustack/bge-reranker-v2-m3-Q8_0"
    Install-BuckyModel "ggml-tiny.bin"
    Install-MalinaModel "sd-1.5"
}

# Install the Docker images.
# ./pwr.ps1 install-docker
function Install-DockerImages {
    $images = @(
        $OpenWebUiImage,
        $GrafanaImage,
        $PrometheusImage,
        $TempoImage,
        $LokiImage,
        $AlloyImage
    )

    $processes = foreach ($image in $images) {
        Start-Process -FilePath "docker" -ArgumentList @("pull", $image) -NoNewWindow -PassThru
    }

    foreach ($process in $processes) {
        $process.WaitForExit()
        if ($process.ExitCode -ne 0) {
            throw "docker pull exited with code $($process.ExitCode)"
        }
    }
}

# ==============================================================================
# Kronk BUI

# Install the Kronk BUI dependencies.
# ./pwr.ps1 bui-install
function Install-Bui {
    Invoke-InDirectory -Path $BuiDirectory -Action {
        npm install
        Assert-NativeCommandSucceeded -Command "npm"
    }
}

# Build the Kronk BUI.
# ./pwr.ps1 bui-build
function Build-Bui {
    Invoke-InDirectory -Path $BuiDirectory -Action {
        npm run build
        Assert-NativeCommandSucceeded -Command "npm"
    }
}

# ==============================================================================
# Kronk Server

# Generate the Kronk documentation.
# ./pwr.ps1 kronk-docs
function Build-KronkDocs {
    Invoke-InDirectory -Path $RepoRoot -Action {
        go run ./cmd/server/api/tooling/docs
        Assert-NativeCommandSucceeded -Command "go"
    }
}

# Generate the Kronk documentation and build the BUI.
# ./pwr.ps1 kronk-build
function Build-Kronk {
    Build-KronkDocs
    Build-Bui
}

# ==============================================================================
# Runtime Setup

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RepoRoot = $PSScriptRoot
$BuiDirectory = Join-Path $RepoRoot "cmd/server/api/frontends/bui"

$OpenWebUiImage = "ghcr.io/open-webui/open-webui:v0.11.1"
$GrafanaImage = "grafana/grafana:13.2.1"
$PrometheusImage = "prom/prometheus:v3.14.0"
$TempoImage = "grafana/tempo:3.0.3"
$LokiImage = "grafana/loki:3.7.7"
$AlloyImage = "grafana/alloy:v1.19.2"

# ==============================================================================
# Helpers

function Assert-NativeCommandSucceeded {
    param(
        [Parameter(Mandatory)]
        [string]$Command
    )

    if ($LASTEXITCODE -ne 0) {
        throw "$Command exited with code $LASTEXITCODE"
    }
}

function Invoke-InDirectory {
    param(
        [Parameter(Mandatory)]
        [string]$Path,

        [Parameter(Mandatory)]
        [scriptblock]$Action
    )

    Push-Location $Path
    try {
        & $Action
    }
    finally {
        Pop-Location
    }
}

function Install-WingetPackage {
    param(
        [Parameter(Mandatory)]
        [string]$Id
    )

    winget list --id $Id --exact --source winget --accept-source-agreements | Out-Null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "$Id is already installed."
        return
    }

    winget install --id $Id --exact --source winget --accept-package-agreements --accept-source-agreements
    Assert-NativeCommandSucceeded -Command "winget"
}

function Install-KronkModel {
    param(
        [Parameter(Mandatory)]
        [string]$Model
    )

    kronk model pull --local $Model
    Assert-NativeCommandSucceeded -Command "kronk"
    Write-Host
}

function Install-BuckyModel {
    param(
        [Parameter(Mandatory)]
        [string]$Model
    )

    kronk bucky model pull --local $Model
    Assert-NativeCommandSucceeded -Command "kronk"
    Write-Host
}

function Install-MalinaModel {
    param(
        [Parameter(Mandatory)]
        [string]$Model
    )

    kronk malina model pull --local $Model
    Assert-NativeCommandSucceeded -Command "kronk"
    Write-Host
}

function Write-Section {
    param(
        [Parameter(Mandatory)]
        [string]$Title
    )

    Write-Host "========== $Title =========="
}

function Show-Targets {
    Write-Host @"
Usage: ./pwr.ps1 <target>

Targets:
  setup
  install-gotooling
  install-tooling
  install-kronk
  install-libraries
  install-libraries-gh
  install-test-gh-models
  install-test-models
  install-test-malina
  install-class-models
  install-docker
  bui-install
  bui-build
  kronk-docs
  kronk-build
"@
}

# ==============================================================================
# Target Dispatch

switch ($Target) {
    "help" {
        Show-Targets
    }
    "setup" {
        Set-UpRepository
    }
    "install-gotooling" {
        Install-GoTooling
    }
    "install-tooling" {
        Install-Tooling
    }
    "install-kronk" {
        Install-Kronk
    }
    "install-libraries" {
        Install-Libraries
    }
    "install-libraries-gh" {
        Install-LibrariesGh
    }
    "install-test-gh-models" {
        Install-TestGhModels
    }
    "install-test-models" {
        Install-TestModels
    }
    "install-test-malina" {
        Install-TestMalina
    }
    "install-class-models" {
        Install-ClassModels
    }
    "install-docker" {
        Install-DockerImages
    }
    "bui-install" {
        Install-Bui
    }
    "bui-build" {
        Build-Bui
    }
    "kronk-docs" {
        Build-KronkDocs
    }
    "kronk-build" {
        Build-Kronk
    }
}
