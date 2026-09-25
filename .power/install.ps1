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
    param([switch]$SkipKronkInstall)

    if (-not $SkipKronkInstall) {
        Install-Kronk
    }

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
    param([switch]$SkipKronkInstall)

    if (-not $SkipKronkInstall) {
        Install-Kronk
    }

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
