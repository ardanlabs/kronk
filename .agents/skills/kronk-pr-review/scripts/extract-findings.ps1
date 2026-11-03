# Usage: extract-findings.ps1 [-Discovery] TRANSCRIPT RUN_DIR
# PowerShell port of extract-findings.sh (same output). Saves the aggregator's findings.md to
# RUN_DIR/findings.md exactly as written, taken from its JSONL transcript (Claude Code's agent
# output_file) so the orchestrator never re-types it. Takes the text between the last
# "===== findings.md =====" and "===== end =====" markers; it must start with
# "# Kronk DiffGuard Report". -Discovery: saves the discovery agent's stack.md and
# extra-checks.tsv to RUN_DIR/context instead (see extract-findings.sh). Uses the built-in
# ConvertFrom-Json.
# Exit: 0 saved | 2 usage | 4 no valid block in the transcript.
param(
    [Parameter(Mandatory = $true, Position = 0)][string]$Transcript,
    [Parameter(Mandatory = $true, Position = 1)][string]$RunDir,
    [switch]$Discovery
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2
$utf8 = [Text.UTF8Encoding]::new($false)
function Die([int]$code, [string]$msg) { [Console]::Error.WriteLine("extract-findings: $msg"); exit $code }

if (-not (Test-Path -LiteralPath $Transcript -PathType Leaf)) { Die 2 "no transcript at $Transcript" }
if (-not (Test-Path -LiteralPath $RunDir -PathType Container)) { Die 2 "no run directory at $RunDir" }

$open = if ($Discovery) { "`n===== stack.md =====`n" } else { "`n===== findings.md =====`n" }
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

if ($null -eq $script:last) { Die 4 "no valid block between the markers in $Transcript" }
$body = $script:last.Substring($script:last.LastIndexOf($open) + $open.Length)
$body = $body.Substring(0, $body.IndexOf($close))
if ($Discovery) {
    $ctxDir = Join-Path $RunDir 'context'
    if (-not (Test-Path -LiteralPath $ctxDir -PathType Container)) { Die 2 "no context directory in $RunDir" }
    $mid = "`n===== extra-checks.tsv =====`n"
    $body += "`n"  # an empty extra-checks block ends right after its marker line
    $at = $body.IndexOf($mid)
    if (-not $body.StartsWith("# Stack`n") -or $at -lt 0) { Die 4 "no stack.md and extra-checks.tsv between the markers in $Transcript" }
    $ctxDir = (Resolve-Path -LiteralPath $ctxDir).Path
    [IO.File]::WriteAllText((Join-Path $ctxDir 'stack.md'), $body.Substring(0, $at) + "`n", $utf8)
    $x = $body.Substring($at + $mid.Length).TrimEnd("`n")
    [IO.File]::WriteAllText((Join-Path $ctxDir 'extra-checks.tsv'), $(if ($x) { $x + "`n" } else { '' }), $utf8)
    $n = @([IO.File]::ReadAllLines((Join-Path $ctxDir 'extra-checks.tsv')) | Where-Object { $_ }).Count
    Write-Output "stack=$(Join-Path $ctxDir 'stack.md') extra_checks=$n"
    exit 0
}
if (-not $body.StartsWith("# Kronk DiffGuard Report`n")) { Die 4 "no findings.md between the markers in $Transcript" }

$dest = Join-Path (Resolve-Path -LiteralPath $RunDir).Path 'findings.md'
[IO.File]::WriteAllText($dest, $body + "`n", $utf8)
Write-Output "findings=$dest lines=$(([IO.File]::ReadAllLines($dest)).Count)"
