# Usage: checks.ps1 RUN_DIR [-Env K=V,...] [-Exclude PATH,...] [-Timeout 10m] [-NoTests] [-Extra NAME,...|all]
# PowerShell port of checks.sh (same outputs). Runs every mechanical check ONCE, scoped to the
# files/units intake recorded in RUN_DIR/context, in parallel; the depth in scope.txt picks
# the files (see checks.sh, also for what is written outside the run directory, and for the
# checks that run the reviewed code only when scope.txt says trust=maintainer). Output:
# RUN_DIR/context/checks/<check>.txt (full log) and RUN_DIR/context/checks/summary.tsv.
# Kronk rules (AGENTS.md, .github/workflows/linux.yml static + race jobs) always apply:
# GITHUB_WORKSPACE=<repo root>; tests (maintainer PRs only) are `go test -race -short` on the
# reviewed packages only (never at project depth: AGENTS.md forbids a full repo test run), with
# KRONK_BASE_PATH=<empty temp store> so model-backed tests skip; go vet covers the whole module;
# packages under $DefaultExcludes are never tested; staticcheck runs with -checks=all.
#   -Env K=V      set for every check
#   -Exclude PATH never run tests in packages/crates/projects under PATH (repo-relative); adds to
#                 the defaults
#   -Timeout D    test timeout (default 10m, as in CI)
#   -NoTests      skip test runs
#   -Extra NAMES  also run these user-approved rows of context/extra-checks.tsv (all = every row)
# A failing check is data for the reviewers, not a script failure. Exit: 0 ran | 2 usage.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, Position = 0)][string]$RunDir,
    [string[]]$Env = @(),
    [string[]]$Exclude = @(),
    [string]$Timeout = '10m',
    [switch]$NoTests,
    [string[]]$Extra = @()
)

# -File passes 'a,b' as one string: split list parameters ourselves (-Env only before NAME=).
$Env = @($Env | ForEach-Object { $_ -split ',(?=[A-Za-z_][A-Za-z0-9_]*=)' } | Where-Object { $_ })
$Exclude = @($Exclude | ForEach-Object { $_ -split ',' } | Where-Object { $_ })
$Extra = @($Extra | ForEach-Object { $_ -split ',' } | Where-Object { $_ })
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
$depthLine = $scopeLines | Where-Object { $_.StartsWith('depth=') } | Select-Object -First 1
$depth = if ($depthLine) { $depthLine.Substring(6) } else { 'diff' }
$trustLine = $scopeLines | Where-Object { $_.StartsWith('trust=') } | Select-Object -First 1
$trust = if ($trustLine) { $trustLine.Substring(6) } else { 'untrusted' }
# RunsCode: checks may execute the reviewed code (maintainer PR only)
$runsCode = $trust -ceq 'maintainer'
$untrusted = "runs the reviewed code: only for a maintainer PR (trust=$trust)"
# scoped: the files under review — the changed files, or the review set at full depth
$scoped = Join-Path $ctx $(if ($depth -eq 'diff') { 'files.tsv' } else { 'review-set.tsv' })
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
New-Item -ItemType Directory -Path (Join-Path $scratch 'kronk-home') -Force | Out-Null
if (-not ($Env | Where-Object { $_ -cmatch '^KRONK_BASE_PATH=' })) { $Env = @($Env) + "KRONK_BASE_PATH=$(Join-Path $scratch 'kronk-home')" }
New-Item -ItemType Directory -Path $out -Force | Out-Null

$envKeys = foreach ($kv in $Env) {
    if ($kv -cnotmatch '^([A-Za-z_][A-Za-z0-9_]*)=(.*)$') { Die "bad -Env '$kv' (need K=V)" }
    [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2]); $Matches[1]
}
$Exclude = @($Exclude | Where-Object { $_ } | ForEach-Object { $_.TrimEnd('/') })
$tests = -not $NoTests
Out-Text (Join-Path $out 'setup.txt') ("env_keys=$(@($envKeys) -join ' ')`ntest_excludes=$(($Exclude | ForEach-Object { "$_ " }) -join '')`ntest_timeout=$Timeout`ntests=$([int]$tests)`ntrust=$trust`n")

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

$files = @(ReadTsv $scoped)
# Generated files reviewed on request (-IncludeGenerated) are read by reviewers but not linted.
function Live([string]$lang) { @($files | Where-Object { $_[0] -ne 'D' -and $_[4] -ne 'generated' -and $_[4] -ne 'vendor' -and ($lang -eq '*' -or $_[3] -ceq $lang) } | ForEach-Object { $_[6] }) }
function DirOf([string]$p) { if ($p -cmatch '/') { $p -creplace '/[^/]*$', '' } else { '.' } }
# Units ECO: unit directories of an ecosystem from stack.tsv ("-" rows are file-level)
$stack = @(ReadTsv (Join-Path $ctx 'stack.tsv'))
function Units([string]$eco) { @($stack | Where-Object { $_[0] -ceq $eco -and $_[1] -ne '-' } | ForEach-Object { $_[1] }) }
# USfx ECO UNIT: log-name suffix, "@unit" when the ecosystem has more than one unit
function USfx([string]$eco, [string]$u) { if (@(Units $eco).Count -gt 1) { '@' + ($u -creplace '/', '_') } else { '' } }
function IsExcl([string]$p) { foreach ($e in $Exclude) { if ($p -ceq $e -or $p.StartsWith("$e/", [StringComparison]::Ordinal)) { return $true } }; $false }
# Under UNIT LANG: live files of LANG inside UNIT, relative to UNIT
function Under([string]$u, [string]$lang) { @(Live $lang | ForEach-Object { if ($u -eq '.') { $_ } elseif ($_.StartsWith("$u/", [StringComparison]::Ordinal)) { $_.Substring($u.Length + 1) } }) }
$onWin = [Environment]::OSVersion.Platform -eq 'Win32NT'
# timeout/gtimeout (coreutils) bound test runs; Windows' timeout.exe is something else.
$tmo = if ($onWin) { $null } else { Get-Command timeout, gtimeout -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1 }
function JobT([string]$name, [string]$dir, [string]$exe, [string[]]$a) {
    if ($tmo) { Job $name $dir $tmo.Source (@($Timeout, $exe) + $a) } else { Job $name $dir $exe $a }
}
function UnitDir([string]$u) { (Resolve-Path -LiteralPath $u).Path }

# --- base tree (apidiff, buf breaking) ---------------------------------------------------
# The base commit's files, extracted outside the repo with git archive: nothing is written
# to the working tree or .git. Removed once the checks finish.
$pk = @(ReadTsv (Join-Path $ctx 'packages.tsv'))
$baseSrc = ''
$protos = @(Live 'proto')
$bufCfg = (Test-Path -LiteralPath 'buf.yaml') -or (Test-Path -LiteralPath 'buf.work.yaml')
if ($baseSha -and (($pk.Count -and (Has 'apidiff')) -or ($protos.Count -and $bufCfg -and (Has 'buf')))) {
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

# Unreachable functions (tests count as roots) in the reviewed files of this module.
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
    # -o NUL / /dev/null: a main package would otherwise write its binary into the repo.
    Push-Location -LiteralPath $dir
    $buildPkgs = @(& go list -e -f '{{if or .GoFiles .CgoFiles}}{{.ImportPath}}{{end}}' @pkgs 2>$null | Where-Object { $_ })
    Pop-Location
    if ($buildPkgs.Count) { Job "go-build$sfx" $dir 'go' (@('build', '-o', $(if ([Environment]::OSVersion.Platform -eq 'Win32NT') { 'NUL' } else { '/dev/null' })) + $buildPkgs) }
    else { Skip "go-build$sfx" 'only test-only packages in scope' }
    # Whole module, as CI does: catches callers in unchanged packages broken by the diff.
    Job "go-vet$sfx" $dir 'go' @('vet', './...')
    if (-not $runsCode) { Skip "go-test$sfx" $untrusted }
    elseif (-not $tests) { Skip "go-test$sfx" '--no-tests' }
    elseif ($depth -eq 'project') { Skip "go-test$sfx" 'project depth: AGENTS.md forbids a full repo test run' }
    elseif (-not $testPkgs.Count) { Skip "go-test$sfx" 'all packages excluded' }
    else { Job "go-test$sfx" $dir 'go' (@('test', '-race', '-short', '-count=1', '-timeout', $Timeout) + $testPkgs) }
    $golangciCfg = @(Get-ChildItem -LiteralPath $dir -File -Force -ErrorAction SilentlyContinue | Where-Object { $_.Name -cmatch '^\.golangci\.(ya?ml|toml|json)$' })
    if ($golangciCfg.Count) {
        # The repo's config can load custom linter plugins: read it only for a maintainer PR;
        # otherwise --no-config runs the default linters and never parses that file.
        if (-not (Has 'golangci-lint')) { Skip "golangci-lint$sfx" 'golangci-lint not installed' }
        elseif ($runsCode) { Job "golangci-lint$sfx" $dir 'golangci-lint' (@('run') + $pkgs) }
        else { Job "golangci-lint$sfx" $dir 'golangci-lint' (@('run', '--no-config') + $pkgs) }
    }
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
    if (Has 'deadcode') { JobFn "deadcode$sfx" $dir "deadcode_scoped $m" $deadcodeFn @($pre, $scoped) }
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

# --- reviewed files by language ------------------------------------------------------
$goFiles = @(Live 'go')
if ($goFiles.Count -and (Has 'go')) { Job 'gofmt' $root 'gofmt' (@('-s', '-l') + $goFiles) }
if ($protos.Count) {
    if (-not $bufCfg) { Skip 'buf-lint' 'no buf.yaml/buf.work.yaml at the repo root' }
    elseif (-not (Has 'buf')) { Skip 'buf-lint' 'buf not installed' }
    else {
        # buf.yaml v2 can name lint/breaking plugins (executables): the repo's config only for a
        # maintainer PR, otherwise buf's default rules from an inline config.
        $neutral = if ($runsCode) { @() } else { @('--config', '{"version":"v2"}') }
        $againstNeutral = if ($runsCode) { @() } else { @('--against-config', '{"version":"v2"}') }
        Job 'buf-lint' $root 'buf' (@('lint') + $neutral + @($protos | ForEach-Object { '--path'; $_ }))
        if ($baseSrc) { Job 'buf-breaking' $root 'buf' (@('breaking') + $neutral + @('--against', $baseSrc) + $againstNeutral) }
        else { Skip 'buf-breaking' 'base commit not available locally' }
    }
}

# BUI: `npm run build` is `tsc && vite build`; the type check is the part that can fail on a
# reviewed change, and it writes nothing.
$bui = 'cmd/server/api/frontends/bui'
if (@(Live 'ts') | Where-Object { $_.StartsWith("$bui/", [StringComparison]::Ordinal) }) {
    if (-not (Test-Path -LiteralPath "$bui/node_modules" -PathType Container)) { Skip 'bui-tsc' "BUI dependencies not installed (cd $bui && npm ci)" }
    elseif (-not (Has 'npx')) { Skip 'bui-tsc' 'npx not installed' }
    elseif (-not $runsCode) { Skip 'bui-tsc' "$untrusted (node_modules tools)" }
    else { Job 'bui-tsc' (Join-Path $root $bui) 'npx' @('--no-install', 'tsc', '--noEmit') }
}

# Helm: the chart's own checks (.manual/chapter-20 §20.10.3); renders only, no cluster.
if (@(Live '*') | Where-Object { $_.StartsWith('zarf/helm/kronk/', [StringComparison]::Ordinal) }) {
    if (-not (Test-Path -LiteralPath 'zarf/helm/kronk/test.sh' -PathType Leaf)) { Skip 'helm-test' 'zarf/helm/kronk/test.sh missing' }
    elseif (-not $runsCode) { Skip 'helm-test' "$untrusted (repo script)" }
    elseif (-not (Has 'helm')) { Skip 'helm-test' 'helm not installed' }
    elseif (-not (Has 'python3') -or $(& python3 -I -c 'import yaml' *> $null; $LASTEXITCODE -ne 0)) { Skip 'helm-test' 'python3 with PyYAML not installed' }
    elseif (-not (Get-Command bash -CommandType Application -ErrorAction SilentlyContinue)) { Skip 'helm-test' 'bash not installed' }
    else { Job 'helm-test' $root 'bash' @('zarf/helm/kronk/test.sh') }
}

$sh = @(Live 'sh')
if ($sh.Count) {
    if (Has 'shellcheck') { Job 'shellcheck' $root 'shellcheck' $sh } else { Skip 'shellcheck' 'shellcheck not installed' }
}
$jsDirs = @(@(Live 'js') + @(Live 'ts') + @(Live 'vue') | ForEach-Object { $_ -creplace '/[^/]*$', '' } | Select-Object -Unique)
if ($jsDirs.Count -and (Has 'jscpd')) { Job 'jscpd' $root 'jscpd' (@('--min-tokens', '50', '--reporters', 'console', '--silent') + $jsDirs) }

# --- Rust, per crate -------------------------------------------------------------------
$rustUnits = @(Units 'rust')
if ($rustUnits.Count) {
    if (-not (Has 'cargo')) { Skip 'cargo' 'cargo not installed' }
    else {
        # Probe from outside the repo: inside it, rustup reads its rust-toolchain.toml (a toolchain path).
        Push-Location -LiteralPath ([IO.Path]::GetTempPath())
        & cargo fmt --version *> $null; $hasFmt = $LASTEXITCODE -eq 0
        & cargo clippy --version *> $null; $hasClippy = $LASTEXITCODE -eq 0
        Pop-Location
        foreach ($u in $rustUnits) {
            $s = USfx 'rust' $u; $dir = UnitDir $u
            # Any cargo call inside the crate reads its rust-toolchain.toml and .cargo/config.toml.
            if (-not $runsCode) { Skip "cargo-fmt$s" "$untrusted (rust-toolchain.toml, .cargo/config.toml)" }
            elseif ($hasFmt) { Job "cargo-fmt$s" $dir 'cargo' @('fmt', '--check') } else { Skip "cargo-fmt$s" 'rustfmt not installed' }
            # --locked: fail instead of rewriting Cargo.lock
            # clippy/check/test compile the crate: build scripts and proc macros run
            if (-not $runsCode) { Skip "cargo-clippy$s" "$untrusted (build scripts, proc macros)" }
            elseif ($hasClippy) { Job "cargo-clippy$s" $dir 'cargo' @('clippy', '--all-targets', '--locked', '--', '-D', 'warnings') }
            else { Skip "cargo-clippy$s" 'clippy not installed'; Job "cargo-check$s" $dir 'cargo' @('check', '--all-targets', '--locked') }
            if (-not $runsCode) { Skip "cargo-test$s" $untrusted }
            elseif (-not $tests) { Skip "cargo-test$s" '--no-tests' }
            elseif (IsExcl $u) { Skip "cargo-test$s" 'crate excluded' }
            else { JobT "cargo-test$s" $dir 'cargo' @('test', '--all-targets', '--locked') }
        }
        $lockDirs = [string[]]@(@(foreach ($u in $rustUnits) {
                    $d = $u
                    while (-not (Test-Path -LiteralPath "$d/Cargo.lock") -and $d -ne '.') { $d = DirOf $d }
                    if (Test-Path -LiteralPath "$d/Cargo.lock") { $d }
                }) | Select-Object -Unique)
        foreach ($d in $lockDirs) {
            $s = if ($lockDirs.Count -gt 1) { '@' + ($d -creplace '/', '_') } else { '' }; $dir = UnitDir $d
            # cargo-audit reads only Cargo.lock: run the binary itself from outside the repo.
            if (Has 'cargo-audit') { Job "cargo-audit$s" ([IO.Path]::GetTempPath()) 'cargo-audit' @('audit', '--file', (Join-Path $dir 'Cargo.lock')) } else { Skip "cargo-audit$s" 'cargo-audit not installed' }
            if (Test-Path -LiteralPath "$d/deny.toml") {
                # cargo-deny runs cargo metadata, which reads the repo's cargo config and toolchain file
                if (-not $runsCode) { Skip "cargo-deny$s" "$untrusted (cargo metadata, repo cargo config)" }
                elseif (Has 'cargo-deny') { Job "cargo-deny$s" $dir 'cargo' @('deny', 'check') } else { Skip "cargo-deny$s" 'cargo-deny not installed' }
            }
        }
    }
}

# --- Terraform / OpenTofu, per module --------------------------------------------------
# The stack marker names the binary (terraform or tofu). init downloads providers and modules
# into a temp TF_DATA_DIR. init may add this platform's hashes to .terraform.lock.hcl (validate
# needs them; a read-only lock fails wherever the lock lacks them), so the file is restored
# byte for byte afterwards, and one that init creates is removed. Locked versions still apply.
$tfRow = $stack | Where-Object { $_[0] -ceq 'terraform' } | Select-Object -First 1
$tfBin = if ($tfRow) { $tfRow[3] } else { '' }
$tfFmtFn = {
    param($st, [string]$bin, [string[]]$fs)
    foreach ($f in $fs) { & $bin fmt -check -diff -no-color $f 2>&1 | ForEach-Object { "$_" }; if ($LASTEXITCODE -ne 0) { $st.rc = $LASTEXITCODE } }
}
$tfValidateFn = {
    param($st, [string]$bin)
    $d = Join-Path ([IO.Path]::GetTempPath()) ('tf-data-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $d | Out-Null
    $env:TF_DATA_DIR = $d
    $lockPath = Join-Path (Get-Location).Path '.terraform.lock.hcl'
    $orig = if (Test-Path -LiteralPath $lockPath) { [IO.File]::ReadAllBytes($lockPath) } else { $null }
    $init = @(& $bin init -backend=false -input=false -no-color 2>&1 | ForEach-Object { "$_" })
    $initRc = $LASTEXITCODE
    if ($initRc -ne 0) { $st.rc = $initRc; '# init failed (providers, modules, or credentials):'; $init }
    else { & $bin validate -no-color 2>&1 | ForEach-Object { "$_" }; $st.rc = $LASTEXITCODE }
    # validate needs the lock file init wrote: put the original back only afterwards
    if ($null -eq $orig) { Remove-Item -LiteralPath $lockPath -Force -ErrorAction SilentlyContinue }
    elseif (-not (Test-Path -LiteralPath $lockPath) -or [Convert]::ToBase64String([IO.File]::ReadAllBytes($lockPath)) -cne [Convert]::ToBase64String($orig)) { [IO.File]::WriteAllBytes($lockPath, $orig) }
    Remove-Item -LiteralPath $d -Recurse -Force -ErrorAction SilentlyContinue
}
# The repo's .tflint.hcl (and its plugins) only for a maintainer PR; otherwise a neutral config
# with the bundled rules and no plugin installation.
$tfLintFn = {
    param($st, [string]$root, [bool]$trusted)
    $cfg = ''
    if (-not $trusted) {
        $cfg = [IO.Path]::GetTempFileName(); [IO.File]::WriteAllText($cfg, "config {`n}`n")
        & tflint "--config=$cfg" --format=compact --no-color 2>&1 | ForEach-Object { "$_" }
        $st.rc = $LASTEXITCODE; Remove-Item -LiteralPath $cfg -Force -ErrorAction SilentlyContinue; return
    }
    if (Test-Path -LiteralPath '.tflint.hcl') { $cfg = (Resolve-Path -LiteralPath '.tflint.hcl').Path }
    elseif (Test-Path -LiteralPath (Join-Path $root '.tflint.hcl')) { $cfg = Join-Path $root '.tflint.hcl' }
    if ($cfg) { & tflint --init "--config=$cfg" *> $null; & tflint "--config=$cfg" --format=compact --no-color 2>&1 | ForEach-Object { "$_" } }
    else { & tflint --format=compact --no-color 2>&1 | ForEach-Object { "$_" } }
    $st.rc = $LASTEXITCODE
}
$tfFiles = @(Live 'tf' | Where-Object { $_ -cmatch '\.(tf|tofu|tfvars|tftest\.hcl)$' })
if ($tfFiles.Count) {
    if (-not $tfBin -or -not (Has $tfBin)) { Skip 'tf' "$tfBin not installed" }
    else {
        JobFn 'tf-fmt' $root "tf_fmt $($tfFiles -join ' ')" $tfFmtFn @($tfBin, [string[]]$tfFiles)
        # init installs the providers and modules the change names, and validate runs them
        foreach ($u in @(Units 'terraform')) {
            if ($runsCode) { JobFn "tf-validate$(USfx 'terraform' $u)" (UnitDir $u) 'tf_validate' $tfValidateFn @($tfBin) }
            else { Skip "tf-validate$(USfx 'terraform' $u)" "$untrusted (providers, modules)" }
        }
    }
    foreach ($u in @(Units 'terraform')) {
        $s = USfx 'terraform' $u
        if (Has 'tflint') { JobFn "tflint$s" (UnitDir $u) 'tf_lint' $tfLintFn @($root, $runsCode) } else { Skip "tflint$s" 'tflint not installed' }
    }
}

# --- Helm, Kustomize, K8s manifests -----------------------------------------------------
# Render (helm template / kustomize build), then kubeconform + kube-linter on the output (in a
# .yaml file: kubeconform skips files without a YAML extension).
$kubeFn = {
    param($st, [string]$kind, [string]$target, [bool]$kc, [bool]$kl, [bool]$kust, [string[]]$fs)
    if ($kind) {
        $o = @(switch ($kind) {
                'helm' { & helm template review $target 2>&1 }
                'kustomize' { if ($kust) { & kustomize build $target 2>&1 } else { & kubectl kustomize $target 2>&1 } }
            })
        $rc = $LASTEXITCODE
        $o | Where-Object { $_ -is [Management.Automation.ErrorRecord] } | ForEach-Object { "$_" }
        if ($rc -ne 0) { $st.rc = $rc; return }
        $tdir = Join-Path ([IO.Path]::GetTempPath()) ('render-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tdir | Out-Null
        $f = Join-Path $tdir 'render.yaml'
        [IO.File]::WriteAllText($f, ((@($o | Where-Object { $_ -isnot [Management.Automation.ErrorRecord] }) | ForEach-Object { "$_" }) -join "`n") + "`n")
        $fs = @($f)
    }
    if ($kc) { '## kubeconform'; & kubeconform -strict -ignore-missing-schemas -summary @fs 2>&1 | ForEach-Object { "$_" }; if ($LASTEXITCODE -ne 0) { $st.rc = $LASTEXITCODE } }
    if ($kl) { '## kube-linter'; & kube-linter lint @fs 2>&1 | ForEach-Object { "$_" }; if ($LASTEXITCODE -ne 0) { $st.rc = $LASTEXITCODE } }
    if ($kind) { Remove-Item -LiteralPath $tdir -Recurse -Force -ErrorAction SilentlyContinue }
}
$kc = Has 'kubeconform'; $kl = Has 'kube-linter'; $kust = Has 'kustomize'
foreach ($u in @(Units 'helm')) {
    $s = USfx 'helm' $u
    if (Has 'helm') {
        Job "helm-lint$s" $root 'helm' @('lint', $u)
        JobFn "helm-render$s" $root "render helm $u" $kubeFn @('helm', $u, $kc, $kl, $kust, [string[]]@())
    } else { Skip "helm$s" 'helm not installed' }
}
foreach ($u in @(Units 'kustomize')) {
    $s = USfx 'kustomize' $u
    if ($kust -or (Has 'kubectl')) { JobFn "kustomize$s" $root "render kustomize $u" $kubeFn @('kustomize', $u, $kc, $kl, $kust, [string[]]@()) }
    else { Skip "kustomize$s" 'kustomize/kubectl not installed' }
}
$kuRoots = @(Units 'kustomize')
$k8sPlain = @(Live 'k8s' | Where-Object { $p = $_; -not ($kuRoots | Where-Object { $_ -eq '.' -or $p.StartsWith("$_/", [StringComparison]::Ordinal) }) })
if ($k8sPlain.Count) {
    if ($kc -or $kl) { JobFn 'k8s-manifests' $root 'kube_check' $kubeFn @('', '', $kc, $kl, $kust, [string[]]$k8sPlain) }
    else { Skip 'k8s-manifests' 'kubeconform/kube-linter not installed' }
}

# IaC misconfiguration scan: trivy, else checkov, over each infra unit directory plus the
# K8s and Docker files outside those units (a file target scans only that file).
$iacUnits = @(@(Units 'terraform') + @(Units 'helm') + @(Units 'kustomize') | Select-Object -Unique)
$iacFiles = @(@(Live 'k8s') + @(Live 'docker') | Where-Object { $p = $_; -not ($iacUnits | Where-Object { $_ -eq '.' -or $p.StartsWith("$_/", [StringComparison]::Ordinal) }) })
$iacDirs = [string[]]@(@($iacUnits) + @($iacFiles) | Where-Object { $_ } | Select-Object -Unique)
[Array]::Sort($iacDirs, [StringComparer]::Ordinal)
# trivy reads trivy.yaml and .trivyignore from its working directory (output paths, module
# dirs, ignores): for an untrusted change it runs from a temp dir with a neutral config.
$iacFn = {
    param($st, [bool]$trivy, [string[]]$targets, [bool]$neutral, [string]$repoRoot)
    $tdir = ''
    if ($trivy -and $neutral) {
        $tdir = Join-Path ([IO.Path]::GetTempPath()) ('trivy-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tdir -Force | Out-Null
        [IO.File]::WriteAllText((Join-Path $tdir 'trivy.yaml'), "quiet: true`n")
    }
    foreach ($x in $targets) {
        "## $x"
        if ($trivy -and $tdir) {
            Push-Location -LiteralPath $tdir
            & trivy config --config (Join-Path $tdir 'trivy.yaml') --quiet --exit-code 1 (Join-Path $repoRoot $x) 2>&1 | ForEach-Object { "$_" }
            Pop-Location
        }
        elseif ($trivy) { & trivy config --quiet --exit-code 1 $x 2>&1 | ForEach-Object { "$_" } }
        elseif (Test-Path -LiteralPath $x -PathType Container) { & checkov --quiet --compact -d $x 2>&1 | ForEach-Object { "$_" } }
        else { & checkov --quiet --compact -f $x 2>&1 | ForEach-Object { "$_" } }
        if ($LASTEXITCODE -ne 0) { $st.rc = $LASTEXITCODE }
    }
    if ($tdir) { Remove-Item -LiteralPath $tdir -Recurse -Force -ErrorAction SilentlyContinue }
}
if ($iacDirs.Count) {
    # checkov loads the repo's .checkov.yaml, whose external-checks-dir runs Python checks from
    # the repo: only for a maintainer PR. trivy runs with a neutral config ($iacFn).
    if ((Has 'trivy') -or ((Has 'checkov') -and $runsCode)) { JobFn 'iac-scan' $root "iac_scan $($iacDirs -join ' ')" $iacFn @((Has 'trivy'), $iacDirs, (-not $runsCode), $root) }
    elseif (Has 'checkov') { Skip 'iac-scan' "$untrusted (checkov repo config can load Python checks; install trivy)" }
    else { Skip 'iac-scan' 'trivy/checkov not installed' }
}

# --- Python, per project ----------------------------------------------------------------
# No caches or bytecode in the repo: ruff --no-cache, mypy without a cache dir, pytest without
# its cache provider, PYTHONDONTWRITEBYTECODE.
$pyFiles = @(Live 'py')
if ($pyFiles.Count) {
    $env:PYTHONDONTWRITEBYTECODE = '1'
    if (Has 'ruff') {
        Job 'ruff' $root 'ruff' (@('check', '--no-cache', '--no-fix', '--output-format=concise') + $pyFiles)
        Job 'ruff-format' $root 'ruff' (@('format', '--no-cache', '--check', '--diff') + $pyFiles)
    } else { Skip 'ruff' 'ruff not installed' }
    if (Has 'bandit') { Job 'bandit' $root 'bandit' (@('-q') + $pyFiles) } else { Skip 'bandit' 'bandit not installed' }
    $nullDir = if ($onWin) { 'nul' } else { '/dev/null' }
    foreach ($u in @(Units 'python')) {
        $s = USfx 'python' $u; $dir = UnitDir $u
        $py = if ($onWin) { Join-Path $dir '.venv\Scripts\python.exe' } else { Join-Path $dir '.venv/bin/python' }
        if (-not (Test-Path -LiteralPath $py)) { $py = if ($onWin) { 'python' } else { 'python3' } }
        $pyproject = Join-Path $dir 'pyproject.toml'; $setupCfg = Join-Path $dir 'setup.cfg'
        $mypyCfg = (Test-Path -LiteralPath (Join-Path $dir 'mypy.ini')) -or (Test-Path -LiteralPath (Join-Path $dir '.mypy.ini')) -or
            ((Test-Path -LiteralPath $pyproject) -and ([IO.File]::ReadAllLines($pyproject) -cmatch '^\[tool\.mypy')) -or
            ((Test-Path -LiteralPath $setupCfg) -and ([IO.File]::ReadAllLines($setupCfg) -cmatch '^\[mypy'))
        if ($mypyCfg) {
            # The repo's config can load mypy plugins: only for a maintainer PR; otherwise defaults.
            $mcfg = @()
            if (-not $runsCode) { $mf = Join-Path $out '.mypy-neutral.ini'; Out-Text $mf "[mypy]`n"; $mcfg = @("--config-file=$mf") }
            if (Has 'mypy') { Job "mypy$s" $dir 'mypy' (@("--cache-dir=$nullDir") + $mcfg + (Under $u 'py')) } else { Skip "mypy$s" 'mypy not installed' }
        }
        Push-Location -LiteralPath $dir
        $hasTests = [bool](@(& git -c core.quotePath=false ls-files -- . 2>$null) -cmatch '(^|/)(test_[^/]*|[^/]*_test)\.py$')
        Pop-Location
        if ($hasTests) {
            if (-not $runsCode) { Skip "pytest$s" $untrusted }
            elseif (-not $tests) { Skip "pytest$s" '--no-tests' }
            elseif (IsExcl $u) { Skip "pytest$s" 'project excluded' }
            else {
                # the project's venv, else a pytest importable by python, else a standalone pytest (pipx)
                & $py -m pytest --version *> $null
                $pyRun = @(if ($LASTEXITCODE -eq 0) { $py, '-m', 'pytest' } elseif (Has 'pytest') { 'pytest' })
                if ($pyRun.Count) {
                    $ign = @(foreach ($e in $Exclude) { if ($e.StartsWith("$u/", [StringComparison]::Ordinal)) { "--ignore=$($e.Substring($u.Length + 1))" } })
                    JobT "pytest$s" $dir $pyRun[0] (@($pyRun | Select-Object -Skip 1) + @('-q', '-p', 'no:cacheprovider') + $ign)
                } else { Skip "pytest$s" 'pytest not installed' }
            }
        }
        foreach ($r in @(Get-ChildItem -LiteralPath $dir -File -Filter 'requirements*.txt' | ForEach-Object { $_.Name } | Sort-Object)) {
            # --no-deps --disable-pip: audit the pins as written; resolving could run setup.py code
            if (Has 'pip-audit') { Job "pip-audit$s@$($r -creplace '\.txt$', '')" $dir 'pip-audit' @('--no-deps', '--disable-pip', '-r', $r) } else { Skip "pip-audit$s" 'pip-audit not installed' }
        }
    }
}

# --- Ansible, YAML, Dockerfiles, CI -----------------------------------------------------
$ans = @(Live 'ansible')
if ($ans.Count) {
    # ansible-lint loads the repo's plugins, modules, and custom rules
    if (-not $runsCode) { Skip 'ansible-lint' "$untrusted (repo plugins and rules)" }
    elseif (Has 'ansible-lint') { Job 'ansible-lint' $root 'ansible-lint' (@('--offline', '-q') + $ans) }
    else { Skip 'ansible-lint' 'ansible-lint not installed' }
}
if (@('.yamllint', '.yamllint.yaml', '.yamllint.yml') | Where-Object { Test-Path -LiteralPath $_ }) {
    $yf = @(@(Live 'yaml') + @(Live 'ansible') + @(Live 'k8s') + @(Live 'ci') | Where-Object { $_ -cmatch '\.ya?ml$' })
    if ($yf.Count) { if (Has 'yamllint') { Job 'yamllint' $root 'yamllint' (@('-f', 'parsable') + $yf) } else { Skip 'yamllint' 'yamllint not installed' } }
}
$dockerfiles = @(Live 'docker' | Where-Object { ($_ -creplace '.*/', '').ToLowerInvariant() -cnotmatch 'compose' })
if ($dockerfiles.Count) { if (Has 'hadolint') { Job 'hadolint' $root 'hadolint' $dockerfiles } else { Skip 'hadolint' 'hadolint not installed' } }
$gha = @(Live 'ci' | Where-Object { $_ -cmatch '(^|/)\.github/|(^|/)action\.ya?ml$' })
if ($gha.Count) {
    $wf = @($gha | Where-Object { $_ -cmatch '\.github/workflows/' })
    if ($wf.Count) { if (Has 'actionlint') { Job 'actionlint' $root 'actionlint' (@('-no-color') + $wf) } else { Skip 'actionlint' 'actionlint not installed' } }
    if (Has 'zizmor') { Job 'zizmor' $root 'zizmor' (@('--offline') + $gha) } else { Skip 'zizmor' 'zizmor not installed' }
}

# --- extra checks the user approved (context/extra-checks.tsv: name dir lens command source) ---
# Each command runs in this PowerShell (sh on the shell port).
$extraRows = @(ReadTsv (Join-Path $ctx 'extra-checks.tsv'))
# Repo gates run commands the reviewed repo defines: only for a maintainer PR.
if ($Extra.Count -and $extraRows.Count -and -not $runsCode) { Skip 'extra-checks' "$untrusted (repo-defined commands)" }
elseif ($Extra.Count -and $extraRows.Count) {
    $psExe = (Get-Process -Id $PID).Path
    foreach ($x in $extraRows) {
        if (-not $x[0] -or $x[0].StartsWith('#') -or $x.Count -lt 4) { continue }
        if ($Extra -cnotcontains $x[0] -and $Extra -cnotcontains 'all') { continue }
        if ($x[0] -cmatch '[^A-Za-z0-9._-]') { continue }
        $xd = if ($x[1]) { $x[1] } else { '.' }
        Job "x-$($x[0])" (UnitDir $xd) $psExe @('-NoProfile', '-Command', $x[3])
    }
}

# Secrets: the reviewed diff, or at full depth a copy of the review set (so generated and
# vendored files stay out; findings show repo-relative paths). generated.patch is scanned
# only on request, by the generated-scan agent (see references/roles/generated-scan.md).
$patches = @(Join-Path $ctx 'diff.patch')
$setCopy = ''
if ((Has 'gitleaks') -and $depth -ne 'diff') {
    $setCopy = Join-Path ([IO.Path]::GetTempPath()) ('checks-set-' + [guid]::NewGuid().ToString('N'))
    # Never follow a symlink out of the repo (intake already leaves them out of the set).
    foreach ($f in @(Live '*')) {
        $item = Get-Item -LiteralPath $f -Force -ErrorAction SilentlyContinue
        if (-not $item -or $item.LinkType -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { continue }
        $dst = Join-Path $setCopy $f
        New-Item -ItemType Directory -Path (Split-Path -Parent $dst) -Force | Out-Null
        Copy-Item -LiteralPath $f -Destination $dst
    }
    New-Item -ItemType Directory -Path $setCopy -Force | Out-Null
    & gitleaks help dir *> $null
    if ($LASTEXITCODE -eq 0) { Job 'gitleaks' $setCopy 'gitleaks' @('dir', '-v', '--no-banner', '--redact', '.') }
    else { Job 'gitleaks' $setCopy 'gitleaks' @('detect', '-v', '--no-git', '--no-banner', '--redact', '--source', '.') }
} elseif (Has 'gitleaks') {
    & gitleaks help stdin *> $null
    if ($LASTEXITCODE -eq 0) { Job 'gitleaks' $root 'gitleaks' @('stdin', '-v', '--no-banner', '--redact') $patches }
    else { Job 'gitleaks' $root 'gitleaks' @('detect', '-v', '--no-git', '--no-banner', '--redact', '--source', $patches[0]) }
} elseif (Has 'trufflehog') {
    Job 'trufflehog' $root 'trufflehog' (@('filesystem', '--no-update') + @(Live '*'))
} else {
    Skip 'secrets' 'no secret scanner (gitleaks/trufflehog) installed'
}
$semcfg = @('.semgrep.yml', '.semgrep.yaml', '.semgrep') | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
if ((Has 'semgrep') -and $semcfg) { Job 'semgrep' $root 'semgrep' (@('scan', '--metrics=off', '--config', $semcfg) + @(Live '*')) }

if ($jobs.Count) { $jobs | Wait-Job | Out-Null; $jobs | Receive-Job -ErrorAction SilentlyContinue | Out-Null; $jobs | Remove-Job }
Remove-Item -LiteralPath (Join-Path $out '.mypy-neutral.ini') -Force -ErrorAction SilentlyContinue
if ($baseSrc) { Remove-Item -LiteralPath $baseSrc -Recurse -Force -ErrorAction SilentlyContinue }
if ($setCopy) { Remove-Item -LiteralPath $setCopy -Recurse -Force -ErrorAction SilentlyContinue }
Remove-Item -LiteralPath $scratch -Recurse -Force -ErrorAction SilentlyContinue

# --- summary ----------------------------------------------------------------------------
$rows = New-Object System.Collections.Generic.List[string]
# LensOf CHECK: the lens that owns a check's failures (extra checks: their lens column)
function LensOf([string]$n) {
    if ($n.StartsWith('x-')) {
        $r = $extraRows | Where-Object { $_[0] -ceq $n.Substring(2) } | Select-Object -First 1
        if ($r -and $r.Count -gt 2 -and $r[2]) { return $r[2] }; return 'service-steward'
    }
    switch -Regex -CaseSensitive ($n) {
        '^(go-test|cargo-test|pytest)' { return 'harness-map' }
        '^errcheck' { return 'error-tripwire' }
        '^(apidiff|buf-)' { return 'boundary-keeper' }
        '^deadcode' { return 'straight-line-pass' }
        '^(dupl|jscpd)' { return 'clone-hunter' }
        '^(gosec|govulncheck|cargo-audit|cargo-deny|pip-audit)|^(gitleaks|trufflehog|secrets|semgrep|bandit|iac-scan|zizmor)$' { return 'security-sentinel' }
    }
    'service-steward'
}
$rows.Add("check`tstatus`texit`tlines`tfile`tlens")
$names = [string[]]@(Get-ChildItem -LiteralPath $out -Filter '*.exit' | ForEach-Object { $_.Name })
[Array]::Sort($names, [StringComparer]::Ordinal)
foreach ($nm in $names) {
    $f = Join-Path $out $nm; $n = $nm -creplace '\.exit$', ''; $e = [IO.File]::ReadAllText($f).Trim()
    $s = switch ($e) { 'skip' { 'skipped' } '0' { 'pass' } default { 'fail' } }
    $log = Join-Path $out "$n.txt"
    $lines = 0; foreach ($x in [IO.File]::ReadLines($log)) { $lines++ }
    # These exit 0 and report through output: any line not starting with "#" is a hit.
    if ($n -cmatch '^(dupl|gofmt$|deadcode|apidiff)' -and $s -eq 'pass' -and @([IO.File]::ReadLines($log) | Where-Object { $_ -cmatch '^[^#]' }).Count) { $s = 'findings' }
    $rows.Add("$n`t$s`t$e`t$lines`tchecks/$n.txt`t$(LensOf $n)")
    Remove-Item -LiteralPath $f
}
Out-Text (Join-Path $out 'summary.tsv') (($rows -join "`n") + "`n")
$rows
