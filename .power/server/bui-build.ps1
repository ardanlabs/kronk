. "$PSScriptRoot\..\_common.ps1"

$BuiDirectory = Join-Path $RepoRoot "cmd/server/api/frontends/bui"

Invoke-InDirectory -Path $BuiDirectory -Action {
    npm run build
    Assert-NativeCommandSucceeded -Command "npm"
}
