# ==============================================================================
# Agents - Default bundle

# Install the OpenCode v2 bundle, or preserve v1 compatibility with -V1.
# Like the Make targets, this overwrites config files and replaces the skills tree.
# ./pwr.ps1 agents-default-opencode
# ./pwr.ps1 agents-default-opencode-v1
function Install-OpenCodeBundle {
    param([switch]$V1)

    $source = Join-Path $RepoRoot ".agents/default"
    $destination = Join-Path $HOME ".config/opencode"
    $config = if ($V1) { "opencode.v1.jsonc" } else { "opencode.jsonc" }

    New-Item -ItemType Directory -Path $destination -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $source "opencode/$config") -Destination (Join-Path $destination "opencode.jsonc") -Force
    Copy-Item -LiteralPath (Join-Path $source "opencode/tui.jsonc") -Destination (Join-Path $destination "tui.jsonc") -Force
    Copy-Item -LiteralPath (Join-Path $source "opencode/auth.json") -Destination (Join-Path $destination "auth.json") -Force
    Copy-Item -LiteralPath (Join-Path $source "AGENTS.md") -Destination (Join-Path $destination "AGENTS.md") -Force

    $skills = Join-Path $destination "skills"
    if (Test-Path -LiteralPath $skills) {
        Remove-Item -LiteralPath $skills -Recurse -Force
    }
    Copy-Item -LiteralPath (Join-Path $source "skills") -Destination $skills -Recurse -Force
}
