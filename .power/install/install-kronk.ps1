. "$PSScriptRoot\..\_common.ps1"

Invoke-InRepoRoot {
    Invoke-NativeCommand -Command "go" -Arguments @("install", "./cmd/kronk")
}
