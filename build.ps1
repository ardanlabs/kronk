#Requires -Version 7.0

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [string]$Task = "help"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$BuiDirectory = Join-Path $PSScriptRoot "cmd/server/api/frontends/bui"

function Invoke-NativeCommand {
    param(
        [Parameter(Mandatory)]
        [string]$Command,

        [Parameter()]
        [string[]]$Arguments = @()
    )

    Write-Host "> $Command $($Arguments -join ' ')"
    & $Command @Arguments

    if ($LASTEXITCODE -ne 0) {
        throw "$Command exited with code $LASTEXITCODE"
    }
}

function Show-Help {
    @"
Kronk Windows development tasks

Usage:
  .\build.ps1 <task>

Tasks:
  help             Show this help.
  install-kronk    Install the Kronk CLI.
  kronk-docs       Generate the Kronk documentation.
  bui-install      Install the browser UI dependencies.
  bui-build        Build the browser UI.
  kronk-build      Generate documentation and build the browser UI.
"@ | Write-Host
}

function Invoke-Task {
    param(
        [Parameter(Mandatory)]
        [string]$Name
    )

    switch ($Name) {
        "help" {
            Show-Help
        }
        "install-kronk" {
            Invoke-NativeCommand "go" @("install", "./cmd/kronk")
        }
        "kronk-docs" {
            Invoke-NativeCommand "go" @("run", "./cmd/server/api/tooling/docs")
        }
        "bui-install" {
            Invoke-NativeCommand "npm" @("--prefix", $BuiDirectory, "install")
        }
        "bui-build" {
            Invoke-NativeCommand "npm" @("--prefix", $BuiDirectory, "run", "build")
        }
        "kronk-build" {
            Invoke-Task "kronk-docs"
            Invoke-Task "bui-build"
        }
        default {
            throw "unknown task '$Name'; run '.\build.ps1 help' to list available tasks"
        }
    }
}

Push-Location $PSScriptRoot
try {
    Invoke-Task $Task
}
catch {
    Write-Error $_
    exit 1
}
finally {
    Pop-Location
}
