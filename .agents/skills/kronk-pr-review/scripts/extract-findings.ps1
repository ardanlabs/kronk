# Usage: extract-findings.ps1 TRANSCRIPT RUN_DIR
# PowerShell port of extract-findings.sh (same output). Saves the aggregator's findings.md to
# RUN_DIR/findings.md exactly as written, taken from its JSONL transcript (Claude Code's agent
# output_file) so the orchestrator never re-types it. Takes the text between the last
# "===== findings.md =====" and "===== end =====" markers; it must start with
# "# Kronk DiffGuard Report". Uses the built-in ConvertFrom-Json.
# Exit: 0 saved | 2 usage | 4 no valid findings.md in the transcript.
param(
    [Parameter(Mandatory = $true, Position = 0)][string]$Transcript,
    [Parameter(Mandatory = $true, Position = 1)][string]$RunDir
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2
$utf8 = [Text.UTF8Encoding]::new($false)
function Die([int]$code, [string]$msg) { [Console]::Error.WriteLine("extract-findings: $msg"); exit $code }

if (-not (Test-Path -LiteralPath $Transcript -PathType Leaf)) { Die 2 "no transcript at $Transcript" }
if (-not (Test-Path -LiteralPath $RunDir -PathType Container)) { Die 2 "no run directory at $RunDir" }

$open = "`n===== findings.md =====`n"
$close = "`n===== end ====="

# Every JSON string holding both markers, in transcript order; the last is the final reply
# (earlier ones are the role file the aggregator read). Lines that aren't JSON are skipped.
$script:last = $null
function Walk($node) {
    if ($null -eq $node) { return }
    if ($node -is [string]) {
        if ($node.Contains($open) -and $node.Contains($close)) { $script:last = $node }
    } elseif ($node -is [System.Collections.IEnumerable]) {
        foreach ($item in $node) { Walk $item }
    } elseif ($node -is [psobject]) {
        foreach ($p in $node.PSObject.Properties) { Walk $p.Value }
    }
}

foreach ($line in [IO.File]::ReadLines((Resolve-Path -LiteralPath $Transcript).Path)) {
    if (-not $line) { continue }
    try { $obj = ConvertFrom-Json -InputObject $line } catch { continue }
    Walk $obj
}

if ($null -eq $script:last) { Die 4 "no findings.md between the markers in $Transcript" }
$body = $script:last.Substring($script:last.LastIndexOf($open) + $open.Length)
$body = $body.Substring(0, $body.IndexOf($close))
if (-not $body.StartsWith("# Kronk DiffGuard Report`n")) { Die 4 "no findings.md between the markers in $Transcript" }

$dest = Join-Path (Resolve-Path -LiteralPath $RunDir).Path 'findings.md'
[IO.File]::WriteAllText($dest, $body + "`n", $utf8)
Write-Output "findings=$dest lines=$(([IO.File]::ReadAllLines($dest)).Count)"
