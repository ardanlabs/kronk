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
        "bui-run",
        "bui-build",
        "bui-upgrade",
        "bui-upgrade-latest",
        "kronk-build",
        "kronk-docs",
        "kronk-server",
        "kronk-server-build",
        "kronk-server-detach",
        "kronk-server-logs",
        "kronk-server-stop",
        "llama-ornith",
        "kronk-diagnose",
        "kronk-libs",
        "kronk-libs-local",
        "kronk-model-index",
        "kronk-model-index-local",
        "kronk-model-list",
        "kronk-model-list-local",
        "kronk-model-pull",
        "kronk-model-pull-local",
        "kronk-model-ps",
        "kronk-model-remove",
        "kronk-model-remove-local",
        "kronk-model-show",
        "kronk-model-show-local",
        "kronk-catalog-list",
        "kronk-catalog-list-local",
        "kronk-catalog-show",
        "kronk-catalog-show-local",
        "kronk-security-help",
        "kronk-security-key-list",
        "kronk-security-key-list-local",
        "kronk-security-token-create-local",
        "kronk-launch",
        "bucky-libs",
        "bucky-libs-local",
        "bucky-libs-combinations",
        "bucky-libs-combinations-local",
        "bucky-libs-installs",
        "bucky-libs-installs-local",
        "bucky-libs-install",
        "bucky-libs-install-local",
        "bucky-libs-remove-install",
        "bucky-libs-remove-install-local",
        "bucky-model-list",
        "bucky-model-list-local",
        "bucky-model-pull",
        "bucky-model-pull-local",
        "bucky-model-remove",
        "bucky-model-remove-local"
    )]
    [string]$Target = "help",

    [string]$Url,

    [string]$Id,

    [string]$Duration,

    [string]$Endpoints,

    [string]$Arch,

    [string]$Os,

    [string]$Processor,

    [string]$Name
)

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

function Invoke-KronkCli {
    param(
        [Parameter(Mandatory)]
        [string[]]$CommandArguments
    )

    Invoke-InDirectory -Path $RepoRoot -Action {
        go run ./cmd/kronk @CommandArguments
        Assert-NativeCommandSucceeded -Command "go run ./cmd/kronk"
    }
}

function Assert-TargetParameter {
    param(
        [AllowEmptyString()]
        [string]$Value,

        [Parameter(Mandatory)]
        [string]$Name,

        [Parameter(Mandatory)]
        [string]$TargetName
    )

    if ([string]::IsNullOrWhiteSpace($Value)) {
        throw "$TargetName requires -$Name"
    }
}

function Assert-BuckyPlatformParameters {
    param(
        [Parameter(Mandatory)]
        [string]$TargetName
    )

    Assert-TargetParameter -Value $Arch -Name "Arch" -TargetName $TargetName
    Assert-TargetParameter -Value $Os -Name "Os" -TargetName $TargetName
    Assert-TargetParameter -Value $Processor -Name "Processor" -TargetName $TargetName
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

function Read-DotEnvFile {
    param(
        [Parameter(Mandatory)]
        [string]$Path
    )

    $variables = @{}
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $variables
    }

    $lineNumber = 0
    foreach ($line in Get-Content -LiteralPath $Path) {
        $lineNumber++
        $entry = $line.Trim()
        if ($entry.Length -eq 0 -or $entry.StartsWith("#")) {
            continue
        }
        if ($entry.StartsWith("export ")) {
            $entry = $entry.Substring(7).TrimStart()
        }

        $separator = $entry.IndexOf('=')
        if ($separator -lt 1) {
            throw "invalid .env entry at ${Path}:$lineNumber"
        }

        $name = $entry.Substring(0, $separator).Trim()
        if ($name -notmatch '^[A-Za-z_][A-Za-z0-9_]*$') {
            throw "invalid .env variable name at ${Path}:$lineNumber"
        }

        $value = $entry.Substring($separator + 1).Trim()
        if ($value.Length -ge 2) {
            $doubleQuoted = $value.StartsWith('"') -and $value.EndsWith('"')
            $singleQuoted = $value.StartsWith("'") -and $value.EndsWith("'")
            if ($doubleQuoted -or $singleQuoted) {
                $value = $value.Substring(1, $value.Length - 2)
            }
        }

        $variables[$name] = $value
    }

    return $variables
}

function Resolve-LlamaProgram {
    param(
        [Parameter(Mandatory)]
        [string]$Program
    )

    if (-not [string]::IsNullOrWhiteSpace($env:KRONK_LIB_PATH)) {
        $executable = Join-Path $env:KRONK_LIB_PATH $Program
        if (Test-Path -LiteralPath $executable -PathType Leaf) {
            return $executable
        }
        throw "$Program was not found under KRONK_LIB_PATH: $env:KRONK_LIB_PATH"
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
    $candidates = @(Get-ChildItem -LiteralPath $libraries -Filter $Program -File -Recurse -ErrorAction SilentlyContinue)
    if (-not [string]::IsNullOrWhiteSpace($env:KRONK_PROCESSOR)) {
        $processorPath = Join-Path $libraries $env:KRONK_PROCESSOR
        $candidates = @($candidates | Where-Object { $_.DirectoryName -eq $processorPath })
    }

    if ($candidates.Count -eq 1) {
        return $candidates[0].FullName
    }
    if ($candidates.Count -eq 0) {
        throw "$Program was not found under $libraries; install llama libraries first"
    }

    throw "multiple $Program installations were found; set KRONK_PROCESSOR or KRONK_LIB_PATH to select one"
}

function Resolve-LlamaBench {
    return Resolve-LlamaProgram -Program "llama-bench.exe"
}

function Resolve-LlamaServer {
    return Resolve-LlamaProgram -Program "llama-server.exe"
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
Usage: ./pwr.ps1 <target> [options]

Targets:

Install (.power/install.ps1):
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

Development (.power/dev.ps1):
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

Server (.power/server.ps1):
  bui-install
  bui-run
  bui-build
  bui-upgrade
  bui-upgrade-latest
  kronk-build
  kronk-docs
  kronk-server
  kronk-server-build
  kronk-server-detach
  kronk-server-logs
  kronk-server-stop
  llama-ornith

CLI (.power/cli.ps1):
  kronk-diagnose
  kronk-libs
  kronk-libs-local
  kronk-model-index
  kronk-model-index-local
  kronk-model-list
  kronk-model-list-local
  kronk-model-pull -Url <model>
  kronk-model-pull-local -Url <model>
  kronk-model-ps
  kronk-model-remove -Id <model>
  kronk-model-remove-local -Id <model>
  kronk-model-show -Id <model>
  kronk-model-show-local -Id <model>
  kronk-catalog-list
  kronk-catalog-list-local
  kronk-catalog-show -Id <model>
  kronk-catalog-show-local -Id <model>
  kronk-security-help
  kronk-security-key-list
  kronk-security-key-list-local
  kronk-security-token-create-local -Duration <duration> -Endpoints <grants>
  kronk-launch
  bucky-libs
  bucky-libs-local
  bucky-libs-combinations
  bucky-libs-combinations-local
  bucky-libs-installs
  bucky-libs-installs-local
  bucky-libs-install -Arch <arch> -Os <os> -Processor <processor>
  bucky-libs-install-local -Arch <arch> -Os <os> -Processor <processor>
  bucky-libs-remove-install -Arch <arch> -Os <os> -Processor <processor>
  bucky-libs-remove-install-local -Arch <arch> -Os <os> -Processor <processor>
  bucky-model-list
  bucky-model-list-local
  bucky-model-pull -Name <model>
  bucky-model-pull-local -Name <model>
  bucky-model-remove -Name <model>
  bucky-model-remove-local -Name <model>
"@
}

# ==============================================================================
# Target Implementations

. (Join-Path $PSScriptRoot ".power/install.ps1")
. (Join-Path $PSScriptRoot ".power/dev.ps1")
. (Join-Path $PSScriptRoot ".power/server.ps1")
. (Join-Path $PSScriptRoot ".power/cli.ps1")

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
    "bui-run" {
        Start-Bui
    }
    "bui-build" {
        Build-Bui
    }
    "bui-upgrade" {
        Update-Bui
    }
    "bui-upgrade-latest" {
        Update-BuiLatest
    }
    "kronk-build" {
        Build-Kronk
    }
    "kronk-docs" {
        Build-KronkDocs
    }
    "kronk-server" {
        Start-KronkServer
    }
    "kronk-server-build" {
        Start-BuiltKronkServer
    }
    "kronk-server-detach" {
        Start-DetachedKronkServer
    }
    "kronk-server-logs" {
        Show-KronkServerLogs
    }
    "kronk-server-stop" {
        Stop-KronkServer
    }
    "llama-ornith" {
        Start-LlamaOrnith
    }
    "kronk-diagnose" {
        Invoke-KronkDiagnose
    }
    "kronk-libs" {
        Install-KronkLibraries
    }
    "kronk-libs-local" {
        Install-KronkLibrariesLocal
    }
    "kronk-model-index" {
        Update-KronkModelIndex
    }
    "kronk-model-index-local" {
        Update-KronkModelIndexLocal
    }
    "kronk-model-list" {
        Get-KronkModels
    }
    "kronk-model-list-local" {
        Get-KronkModelsLocal
    }
    "kronk-model-pull" {
        Install-KronkModelFromServer
    }
    "kronk-model-pull-local" {
        Install-KronkModelLocal
    }
    "kronk-model-ps" {
        Get-KronkModelProcesses
    }
    "kronk-model-remove" {
        Remove-KronkModelFromServer
    }
    "kronk-model-remove-local" {
        Remove-KronkModelLocal
    }
    "kronk-model-show" {
        Show-KronkModel
    }
    "kronk-model-show-local" {
        Show-KronkModelLocal
    }
    "kronk-catalog-list" {
        Get-KronkCatalog
    }
    "kronk-catalog-list-local" {
        Get-KronkCatalogLocal
    }
    "kronk-catalog-show" {
        Show-KronkCatalogEntry
    }
    "kronk-catalog-show-local" {
        Show-KronkCatalogEntryLocal
    }
    "kronk-security-help" {
        Show-KronkSecurityHelp
    }
    "kronk-security-key-list" {
        Get-KronkSecurityKeys
    }
    "kronk-security-key-list-local" {
        Get-KronkSecurityKeysLocal
    }
    "kronk-security-token-create-local" {
        New-KronkSecurityTokenLocal
    }
    "kronk-launch" {
        Start-KronkOpenCode
    }
    "bucky-libs" {
        Install-BuckyLibraries
    }
    "bucky-libs-local" {
        Install-BuckyLibrariesLocal
    }
    "bucky-libs-combinations" {
        Get-BuckyLibraryCombinations
    }
    "bucky-libs-combinations-local" {
        Get-BuckyLibraryCombinationsLocal
    }
    "bucky-libs-installs" {
        Get-BuckyLibraryInstalls
    }
    "bucky-libs-installs-local" {
        Get-BuckyLibraryInstallsLocal
    }
    "bucky-libs-install" {
        Install-BuckyLibraryBundle
    }
    "bucky-libs-install-local" {
        Install-BuckyLibraryBundleLocal
    }
    "bucky-libs-remove-install" {
        Remove-BuckyLibraryBundle
    }
    "bucky-libs-remove-install-local" {
        Remove-BuckyLibraryBundleLocal
    }
    "bucky-model-list" {
        Get-BuckyModels
    }
    "bucky-model-list-local" {
        Get-BuckyModelsLocal
    }
    "bucky-model-pull" {
        Install-BuckyModelFromServer
    }
    "bucky-model-pull-local" {
        Install-BuckyModelLocal
    }
    "bucky-model-remove" {
        Remove-BuckyModelFromServer
    }
    "bucky-model-remove-local" {
        Remove-BuckyModelLocal
    }
}
