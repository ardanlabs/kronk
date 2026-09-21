#Requires -Version 5.1

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $PSScriptRoot

function Invoke-NativeCommand {
    param(
        [Parameter(Mandatory)]
        [string]$Command,

        [Parameter()]
        [string[]]$Arguments = @()
    )

    & $Command @Arguments

    if ($LASTEXITCODE -ne 0) {
        throw "$Command exited with code $LASTEXITCODE"
    }
}

function Invoke-InRepoRoot {
    param(
        [Parameter(Mandatory)]
        [scriptblock]$Action
    )

    Push-Location $RepoRoot
    try {
        & $Action
    }
    finally {
        Pop-Location
    }
}
