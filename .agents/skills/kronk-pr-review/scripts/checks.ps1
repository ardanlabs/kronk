# Usage: checks.ps1 RUN_DIR [-Env K=V,...] [-Exclude PATH,...] [-Timeout 10m] [-NoTests]
# PowerShell port of checks.sh (same outputs). Runs every mechanical check ONCE, scoped to the
# files/packages intake recorded in RUN_DIR/context, in parallel. Output:
# RUN_DIR/context/checks/<check>.txt (full log) and RUN_DIR/context/checks/summary.tsv.
# Kronk rules (AGENTS.md, .github/workflows/linux.yml static + race jobs) always apply:
# GITHUB_WORKSPACE=<repo root>; tests are `go test -race -short` on changed packages only, with
# KRONK_BASE_PATH=<empty temp store> so model-backed tests skip; go vet covers the whole module;
# packages under $DefaultExcludes are never tested; staticcheck runs with -checks=all.
#   -Env K=V      set for every check
#   -Exclude PATH never run tests in packages under PATH (repo-relative); adds to the defaults
#   -Timeout D    go test -timeout (default 10m, as in CI)
#   -NoTests      skip test runs
# A failing check is data for the reviewers, not a script failure. Exit: 0 ran | 2 usage.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, Position = 0)][string]$RunDir,
    [string[]]$Env = @(),
    [string[]]$Exclude = @(),
    [string]$Timeout = '10m',
    [switch]$NoTests
)

# -File passes 'a,b' as one string: split list parameters ourselves (-Env only before NAME=).
$Env = @($Env | ForEach-Object { $_ -split ',(?=[A-Za-z_][A-Za-z0-9_]*=)' } | Where-Object { $_ })
$Exclude = @($Exclude | ForEach-Object { $_ -split ',' } | Where-Object { $_ })
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2
$utf8 = [Text.UTF8Encoding]::new($false)
function Die([string]$msg) { [Console]::Error.WriteLine("checks: $msg"); exit 2 }
function Out-Text([string]$path, [string]$text) { [IO.File]::WriteAllText($path, $text, $utf8) }
function ReadTsv([string]$path) { if (Test-Path -LiteralPath $path) { @([IO.File]::ReadAllLines($path) | Where-Object { $_ } | ForEach-Object { , ($_ -split "`t") }) } else { @() } }

$ctxItem = Get-Item -LiteralPath (Join-Path $RunDir 'context') -ErrorAction SilentlyContinue
if (-not $ctxItem) { Die "no context in $RunDir (run intake first)" }
$ctx = $ctxItem.FullName
$scopeLines = [IO.File]::ReadAllLines((Join-Path $ctx 'scope.txt'))
$root = ($scopeLines | Where-Object { $_.StartsWith('root=') } | Select-Object -First 1).Substring(5)
$baseLine = $scopeLines | Where-Object { $_.StartsWith('base=') } | Select-Object -First 1
$baseSha = if ($baseLine) { $baseLine.Substring(5) } else { '' }
if (-not (Test-Path -LiteralPath $root -PathType Container)) { Die "cannot enter repo root $root" }
Set-Location -LiteralPath $root
$out = Join-Path $ctx 'checks'
# Model-backed or server-backed suites: CI or deliberate human runs only (AGENTS.md, CI race job).
$DefaultExcludes = @('sdk/kronk/tests', 'sdk/bucky/tests', 'sdk/kronk/pool', 'cmd/server/api/services/kronk/tests')
$Exclude = @($DefaultExcludes) + @($Exclude)
if (-not ($Env | Where-Object { $_ -cmatch '^GITHUB_WORKSPACE=' })) { $Env = @($Env) + "GITHUB_WORKSPACE=$((Get-Location).ProviderPath)" }
# Scratch space outside the repo: build output (checks stay read-only) and an empty model
# store, so in-package model-backed tests skip instead of running inference (they are CI work).
$scratch = Join-Path ([IO.Path]::GetTempPath()) ('checks-scratch-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path (Join-Path $scratch 'bin'), (Join-Path $scratch 'kronk-home') -Force | Out-Null
if (-not ($Env | Where-Object { $_ -cmatch '^KRONK_BASE_PATH=' })) { $Env = @($Env) + "KRONK_BASE_PATH=$(Join-Path $scratch 'kronk-home')" }
New-Item -ItemType Directory -Path $out -Force | Out-Null

$envKeys = foreach ($kv in $Env) {
    if ($kv -cnotmatch '^([A-Za-z_][A-Za-z0-9_]*)=(.*)$') { Die "bad -Env '$kv' (need K=V)" }
    [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2]); $Matches[1]
}
$Exclude = @($Exclude | Where-Object { $_ } | ForEach-Object { $_.TrimEnd('/') })
$tests = -not $NoTests
Out-Text (Join-Path $out 'setup.txt') ("env_keys=$(@($envKeys) -join ' ')`ntest_excludes=$(($Exclude | ForEach-Object { "$_ " }) -join '')`ntest_timeout=$Timeout`ntests=$([int]$tests)`n")

$tools = @{}; foreach ($r in @(ReadTsv (Join-Path $ctx 'tools.tsv'))) { $tools[$r[0]] = $r[1] }
function Has([string]$t) { $tools.ContainsKey($t) }

$jobs = New-Object System.Collections.Generic.List[object]
$runner = {
    param([string]$dir, [string]$log, [string]$exe, [string[]]$a, [string[]]$stdin)
    Set-Location -LiteralPath $dir
    $enc = [Text.UTF8Encoding]::new($false)
    $head = "# cmd: $exe $($a -join ' ')`n"
    try {
        if ($stdin) {
            $text = ($stdin | ForEach-Object { [IO.File]::ReadAllText($_) }) -join ''
            $res = $text | & $exe @a 2>&1
        } else {
            $res = & $exe @a 2>&1
        }
        $code = $LASTEXITCODE
    } catch { $res = @("$_"); $code = 127 }
    $body = (@($res) | ForEach-Object { "$_" }) -join "`n"
    if ($body) { $body += "`n" }
    [IO.File]::WriteAllText($log, $head + $body, $enc)
    [IO.File]::WriteAllText(($log -replace '\.txt$', '.exit'), "$code", $enc)
}
function Job([string]$name, [string]$dir, [string]$exe, [string[]]$a, [string[]]$stdin = @()) {
    $jobs.Add((Start-Job -ScriptBlock $runner -ArgumentList $dir, (Join-Path $out "$name.txt"), $exe, $a, $stdin))
}
# JobFn NAME DIR TITLE FN ARGS: run scriptblock FN in DIR as a job; FN gets a state hashtable
# first (set .rc for the exit code), then ARGS. Its output is the log.
$fnRunner = {
    param([string]$dir, [string]$log, [string]$title, [string]$code, [object[]]$a)
    Set-Location -LiteralPath $dir
    $enc = [Text.UTF8Encoding]::new($false)
    $st = @{ rc = 0 }
    try { $res = & ([scriptblock]::Create($code)) $st @a 2>&1 } catch { $res = @("$_"); $st.rc = 127 }
    $body = (@($res) | ForEach-Object { "$_" }) -join "`n"
    if ($body) { $body += "`n" }
    [IO.File]::WriteAllText($log, "# cmd: $title`n" + $body, $enc)
    [IO.File]::WriteAllText(($log -replace '\.txt$', '.exit'), "$($st.rc)", $enc)
}
function JobFn([string]$name, [string]$dir, [string]$title, [scriptblock]$fn, [object[]]$a) {
    $jobs.Add((Start-Job -ScriptBlock $fnRunner -ArgumentList $dir, (Join-Path $out "$name.txt"), $title, $fn.ToString(), $a))
}
function Skip([string]$name, [string]$why) {
    Out-Text (Join-Path $out "$name.txt") "# skipped: $why`n"
    Out-Text (Join-Path $out "$name.exit") 'skip'
}

$files = @(ReadTsv (Join-Path $ctx 'files.tsv'))
function Live([string]$lang) { @($files | Where-Object { $_[0] -ne 'D' -and ($lang -eq '*' -or $_[3] -ceq $lang) } | ForEach-Object { $_[6] }) }

# --- base tree (apidiff) ---------------------------------------------------------------
# The base commit's files, extracted outside the repo with git archive: nothing is written
# to the working tree or .git. Removed once the checks finish.
$pk = @(ReadTsv (Join-Path $ctx 'packages.tsv'))
$baseSrc = ''
if ($baseSha -and $pk.Count -and (Has 'apidiff')) {
    $baseSrc = Join-Path ([IO.Path]::GetTempPath()) ('checks-base-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $baseSrc | Out-Null
    $tar = "$baseSrc.tar"
    & git archive -o $tar $baseSha 2>$null
    if ($LASTEXITCODE -eq 0) { & tar -x -f $tar -C $baseSrc 2>$null }
    if ($LASTEXITCODE -ne 0) { Remove-Item -LiteralPath $baseSrc -Recurse -Force -ErrorAction SilentlyContinue; $baseSrc = '' }
    Remove-Item -LiteralPath $tar -Force -ErrorAction SilentlyContinue
}

# Incompatible exported-API changes of each package against the base tree. Runs in the head
# module directory. New, main, and internal packages are skipped.
$apidiffFn = {
    param($st, [string]$m, [string]$baseSrc, [string[]]$pkgs)
    $bmod = Join-Path $baseSrc $m
    foreach ($p in $pkgs) {
        if (-not (Test-Path -LiteralPath (Join-Path $bmod $p) -PathType Container)) { continue }
        $ip = ((& go list -f '{{if ne .Name "main"}}{{.ImportPath}}{{end}}' $p 2>$null) -join '').Trim()
        if (-not $ip -or $ip -cmatch '(^|/)internal(/|$)') { continue }
        $ex = Join-Path $baseSrc ('apidiff-' + ($ip -creplace '/', '_') + '.export')
        Push-Location -LiteralPath $bmod
        $err = @(& apidiff -w $ex $ip 2>&1); $rc = $LASTEXITCODE
        Pop-Location
        if ($rc -ne 0) { "## ${ip}: base export failed"; $err | Select-Object -First 5 | ForEach-Object { "$_" }; continue }
        $res = @(& apidiff -incompatible $ex $ip 2>&1 | ForEach-Object { "$_" })
        if ($res.Count) { "## $ip"; $res }
    }
}

# Unreachable functions (tests count as roots) in the changed files of this module.
# Library packages build normally (results discarded); main packages go to the scratch bin dir,
# so nothing is written to the repo. `go build -o DIR` rejects a list with no main package.
$buildFn = {
    param($st, [string]$bin, [string[]]$pkgs)
    $mains = @(& go list -e -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' @pkgs 2>$null | Where-Object { $_ })
    $libs = @(& go list -e -f '{{if ne .Name "main"}}{{.ImportPath}}{{end}}' @pkgs 2>$null | Where-Object { $_ })
    if ($libs.Count) { & go build @libs 2>&1 | ForEach-Object { "$_" }; if ($LASTEXITCODE -ne 0) { $st.rc = $LASTEXITCODE } }
    if ($mains.Count) { & go build -o ($bin + [IO.Path]::DirectorySeparatorChar) @mains 2>&1 | ForEach-Object { "$_" }; if ($LASTEXITCODE -ne 0) { $st.rc = $LASTEXITCODE } }
}
$deadcodeFn = {
    param($st, [string]$pre, [string]$filesTsv)
    $res = @(& deadcode -test ./... 2>&1 | ForEach-Object { "$_" })
    if ($LASTEXITCODE -ne 0) {
        if ($res -cmatch 'no main packages') { return '# no main or test packages: nothing to analyze' }
        $st.rc = $LASTEXITCODE; return $res
    }
    $ch = @{}
    foreach ($l in [IO.File]::ReadAllLines($filesTsv)) { $f = $l -split "`t"; if ($f.Count -ge 7 -and $f[0] -ne 'D') { $ch[$f[6]] = $true } }
    foreach ($ln in $res) {
        $p = (($ln -creplace ':\d+:\d+: .*$', '') -replace '\\', '/') -creplace '^\./', ''
        if ($ch.ContainsKey("$pre$p") -or $ch.ContainsKey($p)) { $ln }
    }
}

# --- Go, per module ------------------------------------------------------------------
$goVer = $null
if ((((& go env GOVERSION 2>$null) -join '')) -cmatch '^go(\d+)\.(\d+)') { $goVer = [version]"$($Matches[1]).$($Matches[2])" }
$modules = @($pk | ForEach-Object { $_[0] } | Select-Object -Unique)
foreach ($m in $modules) {
    $sfx = if ($modules.Count -gt 1) { '@' + ($m -creplace '/', '_') } else { '' }
    $dir = (Resolve-Path -LiteralPath $m).Path
    $pkgs = @($pk | Where-Object { $_[0] -ceq $m } | ForEach-Object { $_[1] })
    $testPkgs = @(foreach ($p in $pkgs) {
            $full = $p -creplace '^\./', ''
            if ($m -ne '.') { $full = "$m/$full" -creplace '/\.$', '' }
            if ($full -eq '.') { $full = '' }
            $ex = $false
            foreach ($e in $Exclude) { if ($full -ceq $e -or $full.StartsWith("$e/", [StringComparison]::Ordinal)) { $ex = $true } }
            if (-not $ex) { $p }
        })
    # Test-only packages (e.g. sdk/kronk/tests/*) have nothing to build; go vet still type-checks them.
    Push-Location -LiteralPath $dir
    $buildPkgs = @(& go list -e -f '{{if or .GoFiles .CgoFiles}}{{.ImportPath}}{{end}}' @pkgs 2>$null | Where-Object { $_ })
    Pop-Location
    if ($buildPkgs.Count) { JobFn "go-build$sfx" $dir "build_pkgs $($buildPkgs -join ' ')" $buildFn @((Join-Path $scratch 'bin'), [string[]]$buildPkgs) }
    else { Skip "go-build$sfx" 'only test-only packages changed' }
    # Whole module, as CI does: catches callers in unchanged packages broken by the diff.
    Job "go-vet$sfx" $dir 'go' @('vet', './...')
    if (-not $tests) { Skip "go-test$sfx" '--no-tests' }
    elseif (-not $testPkgs.Count) { Skip "go-test$sfx" 'all changed packages excluded' }
    else { Job "go-test$sfx" $dir 'go' (@('test', '-race', '-short', '-count=1', '-timeout', $Timeout) + $testPkgs) }
    if (Has 'staticcheck') { Job "staticcheck$sfx" $dir 'staticcheck' (@('-checks=all') + $pkgs) } else { Skip "staticcheck$sfx" 'staticcheck not installed' }
    if (Has 'checklocks') { Job "checklocks$sfx" $dir 'go' (@('vet', "-vettool=$($tools['checklocks'])") + $pkgs) }
    else { Skip "checklocks$sfx" 'checklocks not installed' }
    if (Has 'errcheck') { Job "errcheck$sfx" $dir 'errcheck' $pkgs } else { Skip "errcheck$sfx" 'errcheck not installed' }
    if (Has 'nilaway') {
        $modLine = [IO.File]::ReadAllLines((Join-Path $dir 'go.mod')) | Where-Object { $_ -cmatch '^\s*module\s' } | Select-Object -First 1
        $modPath = (($modLine -creplace '^\s*module\s+', '') -creplace '"', '').Trim()
        Job "nilaway$sfx" $dir 'nilaway' (@('-pretty-print=false', "-include-pkgs=$modPath") + $pkgs)
    } else { Skip "nilaway$sfx" 'nilaway not installed' }
    $pre = if ($m -eq '.') { '' } else { "$m/" }
    if (Has 'deadcode') { JobFn "deadcode$sfx" $dir "deadcode_changed $m" $deadcodeFn @($pre, (Join-Path $ctx 'files.tsv')) }
    else { Skip "deadcode$sfx" 'deadcode not installed' }
    if (-not (Has 'apidiff')) { Skip "apidiff$sfx" 'apidiff not installed' }
    elseif (-not $baseSrc) { Skip "apidiff$sfx" 'base commit not available locally' }
    else { JobFn "apidiff$sfx" $dir "apidiff_pkgs $m $($pkgs -join ' ')" $apidiffFn @($m, $baseSrc, [string[]]$pkgs) }
    if ($goVer -and $goVer -ge [version]'1.26') { Job "go-fix$sfx" $dir 'go' (@('fix', '-diff') + $pkgs) }
    else { Skip "go-fix$sfx" 'go fix modernizers need Go 1.26+' }
    Job "go-mod-tidy$sfx" $dir 'go' @('mod', 'tidy', '-diff')
    if (Has 'gosec') { Job "gosec$sfx" $dir 'gosec' (@('-quiet') + $pkgs) } else { Skip "gosec$sfx" 'gosec not installed' }
    if (Has 'govulncheck') { Job "govulncheck$sfx" $dir 'govulncheck' $pkgs } else { Skip "govulncheck$sfx" 'govulncheck not installed' }
    if (Has 'dupl') { Job "dupl$sfx" $dir 'dupl' (@('-t', '50', '-plumbing') + $pkgs) } else { Skip "dupl$sfx" 'dupl not installed' }
}

# --- changed files by language -------------------------------------------------------
$goFiles = @(Live 'go')
if ($goFiles.Count -and (Has 'go')) { Job 'gofmt' $root 'gofmt' (@('-s', '-l') + $goFiles) }

# BUI: `npm run build` is `tsc && vite build`; the type check is the part that can fail on a
# reviewed change, and it writes nothing.
$bui = 'cmd/server/api/frontends/bui'
if (@(Live 'ts') | Where-Object { $_.StartsWith("$bui/", [StringComparison]::Ordinal) }) {
    if (-not (Test-Path -LiteralPath "$bui/node_modules" -PathType Container)) { Skip 'bui-tsc' "BUI dependencies not installed (cd $bui && npm ci)" }
    elseif (-not (Has 'npx')) { Skip 'bui-tsc' 'npx not installed' }
    else { Job 'bui-tsc' (Join-Path $root $bui) 'npx' @('--no-install', 'tsc', '--noEmit') }
}

# Helm: the chart's own checks (.manual/chapter-20 §20.10.3); renders only, no cluster.
if (@(Live '*') | Where-Object { $_.StartsWith('zarf/helm/kronk/', [StringComparison]::Ordinal) }) {
    if (-not (Test-Path -LiteralPath 'zarf/helm/kronk/test.sh' -PathType Leaf)) { Skip 'helm-test' 'zarf/helm/kronk/test.sh missing' }
    elseif (-not (Has 'helm')) { Skip 'helm-test' 'helm not installed' }
    elseif (-not (Has 'python3') -or $(& python3 -c 'import yaml' *> $null; $LASTEXITCODE -ne 0)) { Skip 'helm-test' 'python3 with PyYAML not installed' }
    elseif (-not (Get-Command bash -CommandType Application -ErrorAction SilentlyContinue)) { Skip 'helm-test' 'bash not installed' }
    else { Job 'helm-test' $root 'bash' @('zarf/helm/kronk/test.sh') }
}

$sh = @(Live 'sh')
if ($sh.Count) {
    if (Has 'shellcheck') { Job 'shellcheck' $root 'shellcheck' $sh } else { Skip 'shellcheck' 'shellcheck not installed' }
}
$jsDirs = @(@(Live 'js') + @(Live 'ts') | ForEach-Object { $_ -creplace '/[^/]*$', '' } | Select-Object -Unique)
if ($jsDirs.Count -and (Has 'jscpd')) { Job 'jscpd' $root 'jscpd' (@('--min-tokens', '50', '--reporters', 'console', '--silent') + $jsDirs) }

# Secrets: the reviewed diff only. generated.patch is scanned only on request, by the
# generated-scan agent (see references/roles/generated-scan.md).
$patches = @(Join-Path $ctx 'diff.patch')
if (Has 'gitleaks') {
    & gitleaks help stdin *> $null
    if ($LASTEXITCODE -eq 0) { Job 'gitleaks' $root 'gitleaks' @('stdin', '--no-banner', '--redact') $patches }
    else { Job 'gitleaks' $root 'gitleaks' @('detect', '--no-git', '--no-banner', '--redact', '--source', $patches[0]) }
} elseif (Has 'trufflehog') {
    Job 'trufflehog' $root 'trufflehog' (@('filesystem', '--no-update') + @(Live '*'))
} else {
    Skip 'secrets' 'no secret scanner (gitleaks/trufflehog) installed'
}

if ($jobs.Count) { $jobs | Wait-Job | Out-Null; $jobs | Receive-Job -ErrorAction SilentlyContinue | Out-Null; $jobs | Remove-Job }
if ($baseSrc) { Remove-Item -LiteralPath $baseSrc -Recurse -Force -ErrorAction SilentlyContinue }
Remove-Item -LiteralPath $scratch -Recurse -Force -ErrorAction SilentlyContinue

# --- summary ----------------------------------------------------------------------------
$rows = New-Object System.Collections.Generic.List[string]
$rows.Add("check`tstatus`texit`tlines`tfile")
$names = [string[]]@(Get-ChildItem -LiteralPath $out -Filter '*.exit' | ForEach-Object { $_.Name })
[Array]::Sort($names, [StringComparer]::Ordinal)
foreach ($nm in $names) {
    $f = Join-Path $out $nm; $n = $nm -creplace '\.exit$', ''; $e = [IO.File]::ReadAllText($f).Trim()
    $s = switch ($e) { 'skip' { 'skipped' } '0' { 'pass' } default { 'fail' } }
    $log = Join-Path $out "$n.txt"
    $lines = 0; foreach ($x in [IO.File]::ReadLines($log)) { $lines++ }
    # These exit 0 and report through output: any line not starting with "#" is a hit.
    if ($n -cmatch '^(dupl|gofmt$|deadcode|apidiff)' -and $s -eq 'pass' -and @([IO.File]::ReadLines($log) | Where-Object { $_ -cmatch '^[^#]' }).Count) { $s = 'findings' }
    $rows.Add("$n`t$s`t$e`t$lines`tchecks/$n.txt")
    Remove-Item -LiteralPath $f
}
Out-Text (Join-Path $out 'summary.tsv') (($rows -join "`n") + "`n")
$rows
