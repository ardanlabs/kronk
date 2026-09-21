. "$PSScriptRoot\..\_common.ps1"

$BuiDirectory = Join-Path $RepoRoot "cmd/server/api/frontends/bui"

Invoke-InRepoRoot {
    Invoke-NativeCommand -Command "npm" -Arguments @("--prefix", $BuiDirectory, "run", "build")
}
