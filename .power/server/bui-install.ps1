. "$PSScriptRoot\..\_common.ps1"

$BuiDirectory = Join-Path $RepoRoot "cmd/server/api/frontends/bui"

Invoke-InDirectory -Path $BuiDirectory -Action {
    npm install
    Assert-NativeCommandSucceeded -Command "npm"
}
