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

# Generate the Kronk documentation and run the BUI development server.
# ./pwr.ps1 bui-run
function Start-Bui {
    Build-KronkDocs

    Invoke-InDirectory -Path $BuiDirectory -Action {
        npm run dev
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

# Upgrade the BUI dependencies within their current version ranges.
# ./pwr.ps1 bui-upgrade
function Update-Bui {
    Invoke-InDirectory -Path $BuiDirectory -Action {
        npm update
        Assert-NativeCommandSucceeded -Command "npm"
    }
}

# Upgrade the BUI dependencies to their latest versions.
# ./pwr.ps1 bui-upgrade-latest
function Update-BuiLatest {
    Invoke-InDirectory -Path $BuiDirectory -Action {
        npx npm-check-updates -u
        Assert-NativeCommandSucceeded -Command "npx"

        npm install
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

# Start the repository version of the Kronk server in the foreground and format
# its structured logs. Variables from an optional .env file are scoped to this
# invocation, and the project model configuration overrides that file.
# ./pwr.ps1 kronk-server
function Start-KronkServer {
    $variables = Read-DotEnvFile -Path (Join-Path $RepoRoot ".env")
    $variables["KRONK_POOL_MODEL_CONFIG_FILE"] = "zarf/kms/model_config.yaml"

    Invoke-WithEnvironment -Variables $variables -Action {
        Invoke-InDirectory -Path $RepoRoot -Action {
            $go = Get-Command go -CommandType Application -ErrorAction Stop | Select-Object -First 1
            $startInfo = New-Object Diagnostics.ProcessStartInfo
            $startInfo.FileName = $go.Source
            $startInfo.Arguments = "run ./cmd/kronk server start"
            $startInfo.WorkingDirectory = $RepoRoot
            $startInfo.UseShellExecute = $false
            $startInfo.RedirectStandardOutput = $true

            $process = New-Object Diagnostics.Process
            $process.StartInfo = $startInfo
            $processStarted = $false
            try {
                if (-not $process.Start()) {
                    throw "unable to start go run ./cmd/kronk server start"
                }
                $processStarted = $true

                while (($line = $process.StandardOutput.ReadLine()) -ne $null) {
                    Write-KronkLogLine -Line $line
                }

                $process.WaitForExit()
                if ($process.ExitCode -ne 0) {
                    throw "go run ./cmd/kronk server start exited with code $($process.ExitCode)"
                }
            }
            finally {
                if ($processStarted -and -not $process.HasExited) {
                    $process.Kill()
                    $process.WaitForExit()
                }
                $process.Dispose()
            }
        }
    }
}

# Build the documentation and BUI, then start the repository server in the
# foreground.
# ./pwr.ps1 kronk-server-build
function Start-BuiltKronkServer {
    Build-Kronk
    Start-KronkServer
}

# Build the documentation and BUI, then start the repository server as a
# detached process.
# ./pwr.ps1 kronk-server-detach
function Start-DetachedKronkServer {
    Build-Kronk
    Invoke-KronkCli -CommandArguments @("server", "start", "--detach")
}

# Follow the detached server log with native PowerShell. The CLI currently
# shells out to Unix tail, which is not available on a native Windows install.
# ./pwr.ps1 kronk-server-logs
function Show-KronkServerLogs {
    $basePath = if ([string]::IsNullOrWhiteSpace($env:KRONK_BASE_PATH)) {
        Join-Path $HOME ".kronk"
    }
    else {
        $env:KRONK_BASE_PATH
    }
    $logFile = Join-Path $basePath "kronk.log"
    if (-not (Test-Path -LiteralPath $logFile -PathType Leaf)) {
        throw "Kronk server log not found: $logFile"
    }

    Get-Content -LiteralPath $logFile -Tail 10 -Wait
}

# Stop the detached repository server recorded in the Kronk PID file.
# ./pwr.ps1 kronk-server-stop
function Stop-KronkServer {
    Invoke-KronkCli -CommandArguments @("server", "stop")
}

# Run llama-server directly with the Ornith development configuration from the
# Make target, using the native Windows Kronk library layout.
# ./pwr.ps1 llama-ornith
function Start-LlamaOrnith {
    $server = Resolve-LlamaServer
    $model = Join-Path $HOME ".kronk/models/ornith-ai/Ornith-1.5-35B-A3B-GGUF/Ornith-1.5-35B-Q8_0.gguf"

    & $server `
        -m $model `
        --alias "ornith-ai/Ornith-1.5-35B-Q8_0/AGENT" `
        --host "127.0.0.1" `
        --port "11435" `
        --no-cache-prompt `
        --ctx-size "262144" `
        --parallel "2" `
        --batch-size "4096" `
        --ubatch-size "4096" `
        --flash-attn "auto" `
        --swa-full `
        --cache-type-k "f16" `
        --cache-type-v "f16" `
        --temperature "0.6" `
        --top-k "20" `
        --top-p "1" `
        --min-p "0" `
        --repeat-last-n "64" `
        --repeat-penalty "1" `
        --dry-multiplier "0" `
        --jinja `
        --reasoning "auto" `
        --reasoning-format "deepseek" `
        --metrics
    Assert-NativeCommandSucceeded -Command "llama-server"
}
