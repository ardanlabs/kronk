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
        "llama-bench",
        "authapp-proto-gen",
        "lint",
        "vuln-check",
        "diff",
        "test-only",
        "test",
        "test-gh-only",
        "test-gh",
        "test-malina",
        "test-openvino",
        "tidy",
        "deps-upgrade",
        "build-deps-upgrade",
        "yzma-latest",
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
        $process = Start-Process -FilePath "docker" -ArgumentList @("pull", $image) -NoNewWindow -PassThru

        # Cache the handle before a fast process exits so Windows PowerShell 5.1
        # can reliably populate ExitCode after WaitForExit.
        $null = $process.Handle
        $process
    }

    foreach ($process in $processes) {
        $process.WaitForExit()
        if ($process.ExitCode -ne 0) {
            throw "docker pull exited with code $($process.ExitCode)"
        }
    }
}

# ==============================================================================
# Llama.cpp Programs

# Use this to see what devices are available on your machine. You need to
# install llama first.
# ./pwr.ps1 llama-bench
function Invoke-LlamaBench {
    $bench = Resolve-LlamaBench
    & $bench --list-devices
    Assert-NativeCommandSucceeded -Command "llama-bench"
}

# ==============================================================================
# Protobuf Support

# Generate the auth application Go and gRPC sources from authapp.proto.
# ./pwr.ps1 authapp-proto-gen
function Build-AuthAppProto {
    Invoke-InDirectory -Path $RepoRoot -Action {
        protoc `
            --go_out=cmd/server/app/domain/authapp `
            --go_opt=paths=source_relative `
            --go-grpc_out=cmd/server/app/domain/authapp `
            --go-grpc_opt=paths=source_relative `
            --proto_path=cmd/server/app/domain/authapp `
            cmd/server/app/domain/authapp/authapp.proto
        Assert-NativeCommandSucceeded -Command "protoc"
    }
}

# ==============================================================================
# Tests

# Run go vet and staticcheck across the repository.
# ./pwr.ps1 lint
function Invoke-Lint {
    Invoke-InDirectory -Path $RepoRoot -Action {
        go vet ./...
        Assert-NativeCommandSucceeded -Command "go vet"

        staticcheck -checks=all ./...
        Assert-NativeCommandSucceeded -Command "staticcheck"
    }
}

# Run govulncheck across the repository.
# ./pwr.ps1 vuln-check
function Invoke-VulnerabilityCheck {
    Invoke-InDirectory -Path $RepoRoot -Action {
        govulncheck ./...
        Assert-NativeCommandSucceeded -Command "govulncheck"
    }
}

# Report the changes that go fix would make across the repository.
# ./pwr.ps1 diff
function Show-GoFixDiff {
    Invoke-InDirectory -Path $RepoRoot -Action {
        go fix -diff ./...
        Assert-NativeCommandSucceeded -Command "go fix"
    }
}

# Install the latest libraries and local test models, then run the local tests.
# ./pwr.ps1 test-only
function Invoke-TestsOnly {
    Install-Libraries
    Install-TestModels

    Write-Section "RUN TESTS"
    Invoke-WithEnvironment `
        -Remove $LocalTestEnvironmentOverrides `
        -Variables @{
            RUN_IN_PARALLEL = "yes"
            GITHUB_WORKSPACE = $RepoRoot
        } `
        -Action {
            Invoke-InDirectory -Path $RepoRoot -Action {
                go test -v -p=1 -count=1 ./cmd/kronk/...
                Assert-NativeCommandSucceeded -Command "go test ./cmd/kronk/..."

                go test -v -p=1 -count=1 ./cmd/server/...
                Assert-NativeCommandSucceeded -Command "go test ./cmd/server/..."

                go test -v -p=1 -count=1 ./sdk/...
                Assert-NativeCommandSucceeded -Command "go test ./sdk/..."

                go -C examples test -v -p=1 -count=1 ./...
                Assert-NativeCommandSucceeded -Command "go test ./examples/..."
            }
        }
}

# Run the local tests followed by lint, vulnerability, and go-fix checks.
# ./pwr.ps1 test
function Invoke-Tests {
    Invoke-TestsOnly
    Invoke-Lint
    Invoke-VulnerabilityCheck
    Show-GoFixDiff
}

# Install the pinned libraries and GH models, then reproduce the GH test set.
# ./pwr.ps1 test-gh-only
function Invoke-GhTestsOnly {
    Install-LibrariesGh
    Install-TestGhModels

    Write-Section "RUN GH ONLY TESTS"
    Invoke-WithEnvironment `
        -Remove $GhTestEnvironmentOverrides `
        -Variables @{
            RUN_IN_PARALLEL = "no"
            GITHUB_WORKSPACE = $RepoRoot
        } `
        -Action {
            Invoke-InDirectory -Path $RepoRoot -Action {
                go test -v -p=1 -count=1 ./cmd/kronk/...
                Assert-NativeCommandSucceeded -Command "go test ./cmd/kronk/..."

                go test -v -p=1 -count=1 ./cmd/server/...
                Assert-NativeCommandSucceeded -Command "go test ./cmd/server/..."

                $sdkPackages = @(go list ./sdk/...)
                Assert-NativeCommandSucceeded -Command "go list ./sdk/..."
                $sdkPackages = @($sdkPackages | Where-Object { $_ -notlike "*/sdk/kronk/tests" })
                go test -v -p=1 -count=1 $sdkPackages
                Assert-NativeCommandSucceeded -Command "go test SDK packages"

                go test -v -count=1 -timeout 20m ./sdk/kronk/tests/qwen3
                Assert-NativeCommandSucceeded -Command "go test qwen3"

                go test -v -count=1 -timeout 6m -run '^TestLengthTerminatedToolCallBecomesContent$' ./sdk/kronk/tests/qwen06
                Assert-NativeCommandSucceeded -Command "go test qwen06"

                go test -v -count=1 -timeout 6m -run '^TestSuite$' ./sdk/kronk/tests/draft
                Assert-NativeCommandSucceeded -Command "go test draft"

                go test -v -count=1 -timeout 6m -run '^TestSuite/SimpleMedia$' ./sdk/kronk/tests/vision
                Assert-NativeCommandSucceeded -Command "go test vision"

                go test -v -count=1 -timeout 6m -run '^TestSuite$' ./sdk/kronk/tests/vision_imc
                Assert-NativeCommandSucceeded -Command "go test vision_imc"

                go test -v -count=1 -timeout 6m -run '^(TestSuite|TestConcurrentEmbeddings)$' ./sdk/kronk/tests/embed
                Assert-NativeCommandSucceeded -Command "go test embed"

                go test -v -count=1 -timeout 6m -run '^TestSuite$' ./sdk/kronk/tests/rerank
                Assert-NativeCommandSucceeded -Command "go test rerank"

                go test -v -count=1 -timeout 20m ./sdk/kronk/tests/hybrid
                Assert-NativeCommandSucceeded -Command "go test hybrid"

                go test -v -count=1 -timeout 6m -run '^TestSuite$' ./sdk/kronk/tests/hybrid_vision_imc
                Assert-NativeCommandSucceeded -Command "go test hybrid_vision_imc"
            }
        }
}

# Run the GH test set followed by lint, vulnerability, and go-fix checks.
# ./pwr.ps1 test-gh
function Invoke-GhTests {
    Invoke-GhTestsOnly
    Invoke-Lint
    Invoke-VulnerabilityCheck
    Show-GoFixDiff
}

# Run the native Malina SDK and model-server integration tests explicitly.
# These tests require stable-diffusion.cpp and the sd-1.5 model, and are not
# part of the ordinary local or GitHub test suites.
# ./pwr.ps1 test-malina
function Invoke-MalinaTests {
    Install-TestMalina

    Write-Section "RUN MALINA INTEGRATION TESTS"
    Invoke-WithEnvironment `
        -Remove $LocalTestEnvironmentOverrides `
        -Variables @{
            RUN_IN_PARALLEL = "no"
            GITHUB_WORKSPACE = $RepoRoot
        } `
        -Action {
            Invoke-InDirectory -Path $RepoRoot -Action {
                go test -v -p=1 -count=1 -timeout 20m -tags=malina_integration -run '^TestMalinaModelInference$' ./sdk/malina
                Assert-NativeCommandSucceeded -Command "go test Malina SDK integration"

                go test -v -p=1 -count=1 -timeout 20m -tags=malina_integration -run '^TestImageGenerationModel$' ./cmd/server/app/domain/imageapp
                Assert-NativeCommandSucceeded -Command "go test Malina server integration"
            }
        }
}

# Run the native OpenVINO diagnostic and inference probe explicitly. This test
# downloads the pinned llama.cpp bundle and small diagnostic model, and is not
# part of the ordinary local or GitHub test suites. OPENVINO_DEVICE selects the
# OpenVINO target: CPU, GPU, GPU.<index>, or NPU.
# ./pwr.ps1 test-openvino
# $env:OPENVINO_DEVICE = "GPU"; ./pwr.ps1 test-openvino
function Invoke-OpenVinoTest {
    $goOs = (go env GOOS).Trim()
    Assert-NativeCommandSucceeded -Command "go env GOOS"
    $goArch = (go env GOARCH).Trim()
    Assert-NativeCommandSucceeded -Command "go env GOARCH"
    $platform = "$goOs/$goArch"
    if ($platform -notin @("linux/amd64", "windows/amd64")) {
        throw "test-openvino requires linux/amd64 or windows/amd64; got $platform"
    }

    $device = if ([string]::IsNullOrWhiteSpace($env:OPENVINO_DEVICE)) {
        "CPU"
    }
    else {
        $env:OPENVINO_DEVICE
    }
    if ($device -notmatch '^(CPU|GPU|NPU|GPU\.[0-9]+)$') {
        throw "OPENVINO_DEVICE must be CPU, GPU, GPU.<index>, or NPU"
    }

    Write-Section "RUN OPENVINO INTEGRATION TEST device[$device]"
    Invoke-WithEnvironment `
        -Remove @("KRONK_LIB_PATH", "KRONK_ARCH", "KRONK_OS") `
        -Variables @{
            KRONK_PROCESSOR = "openvino"
            GGML_OPENVINO_DEVICE = $device
            RUN_IN_PARALLEL = "no"
            GITHUB_WORKSPACE = $RepoRoot
        } `
        -Action {
            Invoke-InDirectory -Path $RepoRoot -Action {
                go test -v -p=1 -count=1 -timeout 20m -tags=openvino_integration -run '^TestOpenVINOInference$' ./sdk/tools/diagnose
                Assert-NativeCommandSucceeded -Command "go test OpenVINO integration"
            }
        }
}

# ==============================================================================
# Go Modules Support

# Tidy the root and examples Go modules.
# ./pwr.ps1 tidy
function Invoke-GoModTidy {
    Invoke-InDirectory -Path $RepoRoot -Action {
        go mod tidy
        Assert-NativeCommandSucceeded -Command "go mod tidy"
    }

    Invoke-InDirectory -Path $ExamplesDirectory -Action {
        go mod tidy
        Assert-NativeCommandSucceeded -Command "go mod tidy (examples)"
    }
}

# Upgrade the BUI, root module, examples module, and Yzma dependencies.
# ./pwr.ps1 deps-upgrade
function Update-Dependencies {
    Invoke-InDirectory -Path $BuiDirectory -Action {
        npm update
        Assert-NativeCommandSucceeded -Command "npm update"
    }

    Invoke-InDirectory -Path $RepoRoot -Action {
        go get -u -v ./...
        Assert-NativeCommandSucceeded -Command "go get"

        go get github.com/hybridgroup/yzma@main
        Assert-NativeCommandSucceeded -Command "go get yzma"

        go mod tidy
        Assert-NativeCommandSucceeded -Command "go mod tidy"
    }

    Invoke-InDirectory -Path $ExamplesDirectory -Action {
        go get -u -v ./...
        Assert-NativeCommandSucceeded -Command "go get (examples)"

        go get github.com/hybridgroup/yzma@main
        Assert-NativeCommandSucceeded -Command "go get yzma (examples)"

        go mod tidy
        Assert-NativeCommandSucceeded -Command "go mod tidy (examples)"
    }
}

# Upgrade dependencies and refresh the Docker build pins. The pin refresh uses
# native PowerShell and requires Docker Desktop and gpg on PATH, but not Bash.
# ./pwr.ps1 build-deps-upgrade
function Update-BuildDependencies {
    Update-Dependencies
    Update-DockerBuildPins
}

# Upgrade Yzma directly from its main branch without the Go module proxy.
# ./pwr.ps1 yzma-latest
function Update-YzmaLatest {
    Invoke-WithEnvironment -Variables @{ GOPROXY = "direct" } -Action {
        Invoke-InDirectory -Path $RepoRoot -Action {
            go get github.com/hybridgroup/yzma@main
            Assert-NativeCommandSucceeded -Command "go get yzma"
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
$ExamplesDirectory = Join-Path $RepoRoot "examples"
$DockerfilePath = Join-Path $RepoRoot "zarf/docker/kronk/Dockerfile"

$LocalTestEnvironmentOverrides = @(
    "KRONK_TEST_HOSTED",
    "KRONK_BASE_PATH",
    "KRONK_LIB_PATH",
    "KRONK_BUCKY_LIB_PATH",
    "KRONK_MALINA_LIB_PATH",
    "MALINA_LIB",
    "KRONK_PROCESSOR",
    "KRONK_ARCH",
    "KRONK_OS"
)
$GhTestEnvironmentOverrides = @(
    "KRONK_BASE_PATH",
    "KRONK_LIB_PATH",
    "KRONK_BUCKY_LIB_PATH",
    "KRONK_PROCESSOR",
    "KRONK_ARCH",
    "KRONK_OS"
)

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

function Invoke-WithEnvironment {
    param(
        [string[]]$Remove = @(),

        [hashtable]$Variables = @{},

        [Parameter(Mandatory)]
        [scriptblock]$Action
    )

    $original = @{}
    $names = @($Remove) + @($Variables.Keys)
    foreach ($name in ($names | Select-Object -Unique)) {
        $original[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
    }

    try {
        foreach ($name in $Remove) {
            [Environment]::SetEnvironmentVariable($name, $null, "Process")
        }
        foreach ($name in $Variables.Keys) {
            [Environment]::SetEnvironmentVariable($name, [string]$Variables[$name], "Process")
        }

        & $Action
    }
    finally {
        foreach ($name in $original.Keys) {
            [Environment]::SetEnvironmentVariable($name, $original[$name], "Process")
        }
    }
}

function Resolve-LlamaBench {
    if (-not [string]::IsNullOrWhiteSpace($env:KRONK_LIB_PATH)) {
        $bench = Join-Path $env:KRONK_LIB_PATH "llama-bench.exe"
        if (Test-Path -LiteralPath $bench -PathType Leaf) {
            return $bench
        }
        throw "llama-bench.exe was not found under KRONK_LIB_PATH: $env:KRONK_LIB_PATH"
    }

    $basePath = if ([string]::IsNullOrWhiteSpace($env:KRONK_BASE_PATH)) {
        Join-Path $HOME ".kronk"
    }
    else {
        $env:KRONK_BASE_PATH
    }

    $arch = if ([string]::IsNullOrWhiteSpace($env:KRONK_ARCH)) {
        $goArch = (go env GOARCH).Trim()
        Assert-NativeCommandSucceeded -Command "go env GOARCH"
        $goArch
    }
    else {
        $env:KRONK_ARCH
    }

    $libraries = Join-Path $basePath "libraries/windows/$arch"
    $candidates = @(Get-ChildItem -LiteralPath $libraries -Filter "llama-bench.exe" -File -Recurse -ErrorAction SilentlyContinue)
    if (-not [string]::IsNullOrWhiteSpace($env:KRONK_PROCESSOR)) {
        $processorPath = Join-Path $libraries $env:KRONK_PROCESSOR
        $candidates = @($candidates | Where-Object { $_.DirectoryName -eq $processorPath })
    }

    if ($candidates.Count -eq 1) {
        return $candidates[0].FullName
    }
    if ($candidates.Count -eq 0) {
        throw "llama-bench.exe was not found under $libraries; install llama libraries first"
    }

    throw "multiple llama-bench installations were found; set KRONK_PROCESSOR or KRONK_LIB_PATH to select one"
}

function Get-DockerImageDigest {
    param(
        [Parameter(Mandatory)]
        [string]$Image
    )

    $output = @(docker buildx imagetools inspect $Image 2>$null)
    Assert-NativeCommandSucceeded -Command "docker buildx imagetools inspect $Image"

    $digestLine = $output | Where-Object { $_ -match '^Digest:\s+(sha256:[a-f0-9]{64})\s*$' } | Select-Object -First 1
    if (-not $digestLine) {
        throw "digest lookup failed for ${Image}"
    }

    return ([regex]::Match($digestLine, 'sha256:[a-f0-9]{64}')).Value
}

function Get-UrlSha256 {
    param(
        [Parameter(Mandatory)]
        [string]$Url
    )

    $tempFile = [IO.Path]::GetTempFileName()
    try {
        Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $tempFile
        return (Get-FileHash -LiteralPath $tempFile -Algorithm SHA256).Hash.ToLowerInvariant()
    }
    finally {
        Remove-Item -LiteralPath $tempFile -Force -ErrorAction SilentlyContinue
    }
}

function Get-RocmKeyFingerprint {
    $tempFile = [IO.Path]::GetTempFileName()
    try {
        Invoke-WebRequest -UseBasicParsing -Uri "https://stable.repo.amd.com/rocm/gpg/packages.gpg" -OutFile $tempFile
        $output = @(gpg --show-keys --with-colons $tempFile 2>$null)
        Assert-NativeCommandSucceeded -Command "gpg"

        $fingerprintLine = $output | Where-Object { $_ -like "fpr:*" } | Select-Object -First 1
        if (-not $fingerprintLine) {
            throw "ROCm key fingerprint was not found"
        }
        return $fingerprintLine.Split(':')[9]
    }
    finally {
        Remove-Item -LiteralPath $tempFile -Force -ErrorAction SilentlyContinue
    }
}

function Update-DockerBuildPins {
    if (-not (Test-Path -LiteralPath $DockerfilePath -PathType Leaf)) {
        throw "Dockerfile not found at $DockerfilePath"
    }

    $dockerfile = [IO.File]::ReadAllText($DockerfilePath)

    Write-Host ">>> Resolving ubuntu:24.04 digest" -ForegroundColor Cyan
    $ubuntuDigest = Get-DockerImageDigest -Image "ubuntu:24.04"
    Write-Host "    -> $ubuntuDigest"
    $dockerfile = [regex]::Replace(
        $dockerfile,
        '(ubuntu:24\.04@sha256:)[a-f0-9]{64}',
        '${1}' + $ubuntuDigest.Substring(7)
    )

    $l4tMatch = [regex]::Match($dockerfile, 'nvcr\.io/nvidia/l4t-cuda:([^@\s]+)@sha256:[a-f0-9]{64}')
    if (-not $l4tMatch.Success) {
        throw "could not parse L4T tag from Dockerfile"
    }
    $l4tTag = $l4tMatch.Groups[1].Value

    Write-Host ">>> Resolving nvcr.io/nvidia/l4t-cuda:${l4tTag} digest" -ForegroundColor Cyan
    $l4tDigest = Get-DockerImageDigest -Image "nvcr.io/nvidia/l4t-cuda:${l4tTag}"
    Write-Host "    -> $l4tDigest"
    $l4tPattern = '(nvcr\.io/nvidia/l4t-cuda:' + [regex]::Escape($l4tTag) + '@sha256:)[a-f0-9]{64}'
    $dockerfile = [regex]::Replace($dockerfile, $l4tPattern, '${1}' + $l4tDigest.Substring(7))

    $nodeMatch = [regex]::Match($dockerfile, '(?m)^(?:ENV|ARG) NODE_VERSION="?([^"\s]+)')
    if (-not $nodeMatch.Success) {
        throw "NODE_VERSION not found in Dockerfile"
    }
    $nodeVersion = $nodeMatch.Groups[1].Value

    Write-Host ">>> Fetching Node.js $nodeVersion SHAs" -ForegroundColor Cyan
    $nodeShaX64 = Get-UrlSha256 -Url "https://nodejs.org/dist/v${nodeVersion}/node-v${nodeVersion}-linux-x64.tar.xz"
    $nodeShaArm64 = Get-UrlSha256 -Url "https://nodejs.org/dist/v${nodeVersion}/node-v${nodeVersion}-linux-arm64.tar.xz"
    if ($nodeShaX64 -notmatch '^[a-f0-9]{64}$') {
        throw "bad x64 sha"
    }
    if ($nodeShaArm64 -notmatch '^[a-f0-9]{64}$') {
        throw "bad arm64 sha"
    }
    Write-Host "    -> x64   $nodeShaX64"
    Write-Host "    -> arm64 $nodeShaArm64"
    $dockerfile = [regex]::Replace($dockerfile, '(ARG NODE_SHA256_X64=")[a-f0-9]{64}', '${1}' + $nodeShaX64)
    $dockerfile = [regex]::Replace($dockerfile, '(ARG NODE_SHA256_ARM64=")[a-f0-9]{64}', '${1}' + $nodeShaArm64)

    Write-Host ">>> Fetching ROCm apt key fingerprint" -ForegroundColor Cyan
    $rocmFingerprint = Get-RocmKeyFingerprint
    if ($rocmFingerprint -notmatch '^[A-F0-9]{40}$') {
        throw "bad ROCm fingerprint: '$rocmFingerprint'"
    }
    Write-Host "    -> $rocmFingerprint"
    $dockerfile = [regex]::Replace($dockerfile, '(ARG ROCM_KEY_FINGERPRINT=")[A-F0-9]+', '${1}' + $rocmFingerprint)

    Write-Host ">>> Probing libstdc++6 candidate version from ubuntu-toolchain-r/test" -ForegroundColor Cyan
    $probeScript = @'
export DEBIAN_FRONTEND=noninteractive
apt-get -qq update >/dev/null
apt-get -qq install -y --no-install-recommends ca-certificates gnupg software-properties-common >/dev/null
add-apt-repository -y ppa:ubuntu-toolchain-r/test >/dev/null 2>&1
apt-get -qq update >/dev/null
apt-cache policy libstdc++6 | awk '/Candidate:/ {print $2; exit}'
'@
    $probeOutput = @(docker run --rm --pull=missing ubuntu:22.04 sh -c $probeScript)
    Assert-NativeCommandSucceeded -Command "docker run libstdc++ probe"
    $libstdcxxVersion = $probeOutput | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -Last 1
    if ($libstdcxxVersion) {
        $libstdcxxVersion = $libstdcxxVersion.Trim()
        Write-Host "    -> $libstdcxxVersion"
        $dockerfile = [regex]::Replace($dockerfile, '(ARG LIBSTDCXX_VERSION=")[^"]+', '${1}' + $libstdcxxVersion)
    }
    else {
        Write-Warning "libstdc++6 candidate probe returned empty; leaving pin alone"
    }

    $utf8NoBom = New-Object Text.UTF8Encoding($false)
    [IO.File]::WriteAllText($DockerfilePath, $dockerfile, $utf8NoBom)
    Write-Host ">>> Done. Review changes with: git diff -- $DockerfilePath" -ForegroundColor Cyan
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
  llama-bench
  authapp-proto-gen
  lint
  vuln-check
  diff
  test-only
  test
  test-gh-only
  test-gh
  test-malina
  test-openvino
  tidy
  deps-upgrade
  build-deps-upgrade
  yzma-latest
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
    "llama-bench" {
        Invoke-LlamaBench
    }
    "authapp-proto-gen" {
        Build-AuthAppProto
    }
    "lint" {
        Invoke-Lint
    }
    "vuln-check" {
        Invoke-VulnerabilityCheck
    }
    "diff" {
        Show-GoFixDiff
    }
    "test-only" {
        Invoke-TestsOnly
    }
    "test" {
        Invoke-Tests
    }
    "test-gh-only" {
        Invoke-GhTestsOnly
    }
    "test-gh" {
        Invoke-GhTests
    }
    "test-malina" {
        Invoke-MalinaTests
    }
    "test-openvino" {
        Invoke-OpenVinoTest
    }
    "tidy" {
        Invoke-GoModTidy
    }
    "deps-upgrade" {
        Update-Dependencies
    }
    "build-deps-upgrade" {
        Update-BuildDependencies
    }
    "yzma-latest" {
        Update-YzmaLatest
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
