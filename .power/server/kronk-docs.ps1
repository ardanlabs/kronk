. "$PSScriptRoot\..\_common.ps1"

Invoke-InRepoRoot {
    Invoke-NativeCommand -Command "go" -Arguments @("run", "./cmd/server/api/tooling/docs")
}
