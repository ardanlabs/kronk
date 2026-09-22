. "$PSScriptRoot\..\_common.ps1"

Invoke-InDirectory -Path $RepoRoot -Action {
    go run ./cmd/server/api/tooling/docs
    Assert-NativeCommandSucceeded -Command "go"
}
