# Native Windows equivalents of the development Make targets.
# From the repository root, run `./pwr.ps1 <target>` or `./pwr.ps1` to list targets.
# These commands require neither GNU Make nor a Unix shell.

#Requires -Version 5.1

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet(
        "help",
        "install-kronk",
        "bui-install",
        "bui-build",
        "kronk-docs",
        "kronk-build"
    )]
    [string]$Target = "help"
)

# ==============================================================================
# Install

# Install the Kronk CLI.
# ./pwr.ps1 install-kronk
function Install-Kronk {
    Invoke-InDirectory -Path $RepoRoot -Action {
        go install ./cmd/kronk
        Assert-NativeCommandSucceeded -Command "go"
    }
}

# ==============================================================================
# Kronk BUI

# Install the Kronk BUI dependencies.
# ./pwr.ps1 bui-install
function Install-Bui {
    Invoke-InDirectory -Path $BuiDirectory -Action {
        npm install
        Assert-NativeCommandSucceeded -Command "npm"
    }
}

# Build the Kronk BUI.
# ./pwr.ps1 bui-build
function Build-Bui {
    Invoke-InDirectory -Path $BuiDirectory -Action {
        npm run build
        Assert-NativeCommandSucceeded -Command "npm"
    }
}

# ==============================================================================
# Kronk Server

# Generate the Kronk documentation.
# ./pwr.ps1 kronk-docs
function Build-KronkDocs {
    Invoke-InDirectory -Path $RepoRoot -Action {
        go run ./cmd/server/api/tooling/docs
        Assert-NativeCommandSucceeded -Command "go"
    }
}

# Generate the Kronk documentation and build the BUI.
# ./pwr.ps1 kronk-build
function Build-Kronk {
    Build-KronkDocs
    Build-Bui
}

# ==============================================================================
# Runtime Setup

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RepoRoot = $PSScriptRoot
$BuiDirectory = Join-Path $RepoRoot "cmd/server/api/frontends/bui"

# ==============================================================================
# Helpers

function Assert-NativeCommandSucceeded {
    param(
        [Parameter(Mandatory)]
        [string]$Command
    )

    if ($LASTEXITCODE -ne 0) {
        throw "$Command exited with code $LASTEXITCODE"
    }
}

function Invoke-InDirectory {
    param(
        [Parameter(Mandatory)]
        [string]$Path,

        [Parameter(Mandatory)]
        [scriptblock]$Action
    )

    Push-Location $Path
    try {
        & $Action
    }
    finally {
        Pop-Location
    }
}

function Show-Targets {
    Write-Host @"
Usage: ./pwr.ps1 <target>

Targets:
  install-kronk
  bui-install
  bui-build
  kronk-docs
  kronk-build
"@
}

# ==============================================================================
# Target Dispatch

switch ($Target) {
    "help" {
        Show-Targets
    }
    "install-kronk" {
        Install-Kronk
    }
    "bui-install" {
        Install-Bui
    }
    "bui-build" {
        Build-Bui
    }
    "kronk-docs" {
        Build-KronkDocs
    }
    "kronk-build" {
        Build-Kronk
    }
}
