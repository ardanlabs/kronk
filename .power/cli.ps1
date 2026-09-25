# ==============================================================================
# Kronk CLI - llama (kronk) verbs

# Diagnose the host and installed Kronk library bundles.
# ./pwr.ps1 kronk-diagnose
function Invoke-KronkDiagnose {
    Invoke-KronkCli -CommandArguments @("diagnose")
}

# Install the pinned llama.cpp libraries through the running model server.
# ./pwr.ps1 kronk-libs
function Install-KronkLibraries {
    Invoke-KronkCli -CommandArguments @("libs")
}

# Install the pinned llama.cpp, whisper.cpp, and stable-diffusion.cpp libraries
# locally without a running server.
# ./pwr.ps1 kronk-libs-local
function Install-KronkLibrariesLocal {
    Install-Libraries
}

# Rebuild the Kronk model index through the running model server.
# ./pwr.ps1 kronk-model-index
function Update-KronkModelIndex {
    Invoke-KronkCli -CommandArguments @("model", "index")
}

# Rebuild the local Kronk model index without a running server.
# ./pwr.ps1 kronk-model-index-local
function Update-KronkModelIndexLocal {
    Invoke-KronkCli -CommandArguments @("model", "index", "--local")
}

# List Kronk models through the running model server.
# ./pwr.ps1 kronk-model-list
function Get-KronkModels {
    Invoke-KronkCli -CommandArguments @("model", "list")
}

# List local Kronk models without a running server.
# ./pwr.ps1 kronk-model-list-local
function Get-KronkModelsLocal {
    Invoke-KronkCli -CommandArguments @("model", "list", "--local")
}

# Pull a Kronk model through the running model server.
# ./pwr.ps1 kronk-model-pull -Url "unsloth/Qwen3-1.7B-UD-Q8_K_XL"
function Install-KronkModelFromServer {
    Assert-TargetParameter -Value $Url -Name "Url" -TargetName "kronk-model-pull"
    Invoke-KronkCli -CommandArguments @("model", "pull", $Url)
}

# Pull a Kronk model locally without a running server.
# ./pwr.ps1 kronk-model-pull-local -Url "unsloth/Qwen3-1.7B-UD-Q8_K_XL"
function Install-KronkModelLocal {
    Assert-TargetParameter -Value $Url -Name "Url" -TargetName "kronk-model-pull-local"
    Invoke-KronkCli -CommandArguments @("model", "pull", "--local", $Url)
}

# List models currently loaded by the running model server.
# ./pwr.ps1 kronk-model-ps
function Get-KronkModelProcesses {
    Invoke-KronkCli -CommandArguments @("model", "ps")
}

# Remove a Kronk model through the running model server.
# ./pwr.ps1 kronk-model-remove -Id "bartowski/cerebras_qwen3-coder-reap-25b-a3b-q8_0"
function Remove-KronkModelFromServer {
    Assert-TargetParameter -Value $Id -Name "Id" -TargetName "kronk-model-remove"
    Invoke-KronkCli -CommandArguments @("model", "remove", $Id)
}

# Remove a local Kronk model without a running server.
# ./pwr.ps1 kronk-model-remove-local -Id "bartowski/cerebras_qwen3-coder-reap-25b-a3b-q8_0"
function Remove-KronkModelLocal {
    Assert-TargetParameter -Value $Id -Name "Id" -TargetName "kronk-model-remove-local"
    Invoke-KronkCli -CommandArguments @("model", "remove", "--local", $Id)
}

# Show details for a Kronk model through the running model server.
# ./pwr.ps1 kronk-model-show -Id "unsloth/Qwen3-1.7B-UD-Q8_K_XL"
function Show-KronkModel {
    Assert-TargetParameter -Value $Id -Name "Id" -TargetName "kronk-model-show"
    Invoke-KronkCli -CommandArguments @("model", "show", $Id)
}

# Show details for a local Kronk model without a running server.
# ./pwr.ps1 kronk-model-show-local -Id "unsloth/Qwen3-1.7B-UD-Q8_K_XL"
function Show-KronkModelLocal {
    Assert-TargetParameter -Value $Id -Name "Id" -TargetName "kronk-model-show-local"
    Invoke-KronkCli -CommandArguments @("model", "show", "--local", $Id)
}

# List catalog entries through the running model server.
# ./pwr.ps1 kronk-catalog-list
function Get-KronkCatalog {
    Invoke-KronkCli -CommandArguments @("catalog", "list")
}

# List local catalog entries without a running server.
# ./pwr.ps1 kronk-catalog-list-local
function Get-KronkCatalogLocal {
    Invoke-KronkCli -CommandArguments @("catalog", "list", "--local")
}

# Show a catalog entry through the running model server.
# ./pwr.ps1 kronk-catalog-show -Id "unsloth/Qwen3-1.7B-UD-Q8_K_XL"
function Show-KronkCatalogEntry {
    Assert-TargetParameter -Value $Id -Name "Id" -TargetName "kronk-catalog-show"
    Invoke-KronkCli -CommandArguments @("catalog", "show", $Id)
}

# Show a local catalog entry without a running server.
# ./pwr.ps1 kronk-catalog-show-local -Id "unsloth/Qwen3-1.7B-UD-Q8_K_XL"
function Show-KronkCatalogEntryLocal {
    Assert-TargetParameter -Value $Id -Name "Id" -TargetName "kronk-catalog-show-local"
    Invoke-KronkCli -CommandArguments @("catalog", "show", "--local", $Id)
}

# Show help for Kronk security commands.
# ./pwr.ps1 kronk-security-help
function Show-KronkSecurityHelp {
    Invoke-KronkCli -CommandArguments @("security", "--help")
}

# List security keys through the running model server.
# ./pwr.ps1 kronk-security-key-list
function Get-KronkSecurityKeys {
    Invoke-KronkCli -CommandArguments @("security", "key", "list")
}

# List local security keys without a running server.
# ./pwr.ps1 kronk-security-key-list-local
function Get-KronkSecurityKeysLocal {
    Invoke-KronkCli -CommandArguments @("security", "key", "list", "--local")
}

# Create a local security token without a running server.
# ./pwr.ps1 kronk-security-token-create-local -Duration "5m" -Endpoints "chat-completions"
function New-KronkSecurityTokenLocal {
    Assert-TargetParameter -Value $Duration -Name "Duration" -TargetName "kronk-security-token-create-local"
    Assert-TargetParameter -Value $Endpoints -Name "Endpoints" -TargetName "kronk-security-token-create-local"
    Invoke-KronkCli -CommandArguments @(
        "security",
        "token",
        "create",
        "--local",
        "--duration",
        $Duration,
        "--endpoints",
        $Endpoints
    )
}

# Launch OpenCode with the Qwen3.6 35B-A3B coding model.
# ./pwr.ps1 kronk-launch
function Start-KronkOpenCode {
    Invoke-KronkCli -CommandArguments @(
        "launch",
        "opencode",
        "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL"
    )
}

# ==============================================================================
# Bucky (whisper) verbs

# Install or update Bucky libraries through the running model server.
# ./pwr.ps1 bucky-libs
function Install-BuckyLibraries {
    Invoke-KronkCli -CommandArguments @("bucky", "libs")
}

# Install or update local Bucky libraries without a running server.
# ./pwr.ps1 bucky-libs-local
function Install-BuckyLibrariesLocal {
    Invoke-KronkCli -CommandArguments @("bucky", "libs", "--local")
}

# List available Bucky library combinations through the running model server.
# ./pwr.ps1 bucky-libs-combinations
function Get-BuckyLibraryCombinations {
    Invoke-KronkCli -CommandArguments @("bucky", "libs", "--list-combinations")
}

# List available local Bucky library combinations without a running server.
# ./pwr.ps1 bucky-libs-combinations-local
function Get-BuckyLibraryCombinationsLocal {
    Invoke-KronkCli -CommandArguments @("bucky", "libs", "--local", "--list-combinations")
}

# List installed Bucky library bundles through the running model server.
# ./pwr.ps1 bucky-libs-installs
function Get-BuckyLibraryInstalls {
    Invoke-KronkCli -CommandArguments @("bucky", "libs", "--list-installs")
}

# List installed local Bucky library bundles without a running server.
# ./pwr.ps1 bucky-libs-installs-local
function Get-BuckyLibraryInstallsLocal {
    Invoke-KronkCli -CommandArguments @("bucky", "libs", "--local", "--list-installs")
}

# Install a Bucky library bundle through the running model server.
# ./pwr.ps1 bucky-libs-install -Arch "amd64" -Os "windows" -Processor "cpu"
function Install-BuckyLibraryBundle {
    Assert-BuckyPlatformParameters -TargetName "bucky-libs-install"
    Invoke-KronkCli -CommandArguments @(
        "bucky",
        "libs",
        "--install",
        "--arch=$Arch",
        "--os=$Os",
        "--processor=$Processor"
    )
}

# Install a local Bucky library bundle without a running server.
# ./pwr.ps1 bucky-libs-install-local -Arch "amd64" -Os "windows" -Processor "cpu"
function Install-BuckyLibraryBundleLocal {
    Assert-BuckyPlatformParameters -TargetName "bucky-libs-install-local"
    Invoke-KronkCli -CommandArguments @(
        "bucky",
        "libs",
        "--local",
        "--install",
        "--arch=$Arch",
        "--os=$Os",
        "--processor=$Processor"
    )
}

# Remove an installed Bucky library bundle through the running model server.
# ./pwr.ps1 bucky-libs-remove-install -Arch "amd64" -Os "windows" -Processor "cpu"
function Remove-BuckyLibraryBundle {
    Assert-BuckyPlatformParameters -TargetName "bucky-libs-remove-install"
    Invoke-KronkCli -CommandArguments @(
        "bucky",
        "libs",
        "--remove-install",
        "--arch=$Arch",
        "--os=$Os",
        "--processor=$Processor"
    )
}

# Remove an installed local Bucky library bundle without a running server.
# ./pwr.ps1 bucky-libs-remove-install-local -Arch "amd64" -Os "windows" -Processor "cpu"
function Remove-BuckyLibraryBundleLocal {
    Assert-BuckyPlatformParameters -TargetName "bucky-libs-remove-install-local"
    Invoke-KronkCli -CommandArguments @(
        "bucky",
        "libs",
        "--local",
        "--remove-install",
        "--arch=$Arch",
        "--os=$Os",
        "--processor=$Processor"
    )
}

# List Bucky models through the running model server.
# ./pwr.ps1 bucky-model-list
function Get-BuckyModels {
    Invoke-KronkCli -CommandArguments @("bucky", "model", "list")
}

# List local Bucky models without a running server.
# ./pwr.ps1 bucky-model-list-local
function Get-BuckyModelsLocal {
    Invoke-KronkCli -CommandArguments @("bucky", "model", "list", "--local")
}

# Pull a Bucky model through the running model server.
# ./pwr.ps1 bucky-model-pull -Name "ggml-tiny.bin"
function Install-BuckyModelFromServer {
    Assert-TargetParameter -Value $Name -Name "Name" -TargetName "bucky-model-pull"
    Invoke-KronkCli -CommandArguments @("bucky", "model", "pull", $Name)
}

# Pull a local Bucky model without a running server.
# ./pwr.ps1 bucky-model-pull-local -Name "ggml-tiny.bin"
function Install-BuckyModelLocal {
    Assert-TargetParameter -Value $Name -Name "Name" -TargetName "bucky-model-pull-local"
    Invoke-KronkCli -CommandArguments @("bucky", "model", "pull", "--local", $Name)
}

# Remove a Bucky model through the running model server.
# ./pwr.ps1 bucky-model-remove -Name "ggml-tiny.bin"
function Remove-BuckyModelFromServer {
    Assert-TargetParameter -Value $Name -Name "Name" -TargetName "bucky-model-remove"
    Invoke-KronkCli -CommandArguments @("bucky", "model", "remove", $Name)
}

# Remove a local Bucky model without a running server.
# ./pwr.ps1 bucky-model-remove-local -Name "ggml-tiny.bin"
function Remove-BuckyModelLocal {
    Assert-TargetParameter -Value $Name -Name "Name" -TargetName "bucky-model-remove-local"
    Invoke-KronkCli -CommandArguments @("bucky", "model", "remove", "--local", $Name)
}
