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

# Install the pinned libraries and local test models, then run the local tests.
# ./pwr.ps1 test-only
function Invoke-TestsOnly {
    Install-Libraries
    Install-TestModels -SkipKronkInstall

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
    Install-Libraries
    Install-TestGhModels -SkipKronkInstall

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
