. "$PSScriptRoot\..\_common.ps1"

Invoke-InDirectory -Path $RepoRoot -Action {
    go install ./cmd/kronk
    Assert-NativeCommandSucceeded -Command "go"
}
