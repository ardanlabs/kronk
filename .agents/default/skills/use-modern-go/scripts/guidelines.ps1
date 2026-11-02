# Usage: guidelines.ps1 [-Go X.Y] [[-Path] PATH]. Same as guidelines.sh; PowerShell 5.1+.
param([string]$Path = '.', [string]$Go = '')

# Major.minor of File's go line, or Default when it has none.
function Get-GoVersion([string]$File, [string]$Default) {
    foreach ($line in Get-Content -LiteralPath $File -ErrorAction SilentlyContinue) {
        if ($line -match '^\s*go\s+(\d+)\.(\d+)') { return [version]"$($Matches[1]).$($Matches[2])" }
    }
    if ($Default) { [version]$Default }
}

# Nearest Name in Start or its parents.
function Find-Up([string]$Start, [string]$Name) {
    for ($d = $Start; $d; $d = Split-Path -Parent $d) {
        $candidate = Join-Path $d $Name
        if (Test-Path -LiteralPath $candidate -PathType Leaf) { return $candidate }
    }
}

$item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
if (-not $item) { 'go unknown'; exit 0 }
$start = if ($item.PSIsContainer) { $item.FullName } else { $item.DirectoryName }

$ver = $null
if ($Go -match '^(?:go)?(\d+)\.(\d+)') { $ver = [version]"$($Matches[1]).$($Matches[2])" }
if (-not $ver) { $f = Find-Up $start 'go.mod'; if ($f) { $ver = Get-GoVersion $f '1.16' } }
if (-not $ver) { $f = Find-Up $start 'go.work'; if ($f) { $ver = Get-GoVersion $f '1.18' } }
if (-not $ver) {
    $inGit = $false
    if (Get-Command git -ErrorAction SilentlyContinue) {
        # git prints UTF-8; the console code page would garble non-ASCII paths.
        $enc = [Console]::OutputEncoding
        try { [Console]::OutputEncoding = [Text.UTF8Encoding]::new($false) } catch {}
        $rel = @(git -C $start -c core.quotePath=false ls-files go.mod '*/go.mod' 2>$null)
        $inGit = $LASTEXITCODE -eq 0
        try { [Console]::OutputEncoding = $enc } catch {}
    }
    if (-not $inGit) {
        $prefix = $start.TrimEnd('\', '/').Length + 1
        $rel = @(Get-ChildItem -LiteralPath $start -Recurse -File -Force -Filter go.mod -ErrorAction SilentlyContinue |
            ForEach-Object { $_.FullName.Substring($prefix) })
    }
    $ver = $rel | Where-Object { $_ -and $_ -notmatch '(^|[\\/])(testdata|vendor|node_modules|\.git)[\\/]' } |
        ForEach-Object { Get-GoVersion (Join-Path $start $_) '' } | Sort-Object | Select-Object -First 1
}
if (-not $ver) { 'go unknown'; exit 0 }

"go $($ver.Major).$($ver.Minor)"
$data = [IO.Path]::Combine($PSScriptRoot, '..', 'references', 'features.txt')
foreach ($line in Get-Content -LiteralPath $data) {
    if ($line -match '^(\d+\.\d+) (.*)$' -and [version]$Matches[1] -le $ver) { "- $($Matches[2])" }
}
