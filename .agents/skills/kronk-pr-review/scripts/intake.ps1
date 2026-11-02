# Usage: intake.ps1 [-Id ID] [-Base REF | -Range A..B | -Pr N | -Staged | -Files F,...]
#                   [-Exclude PATHSPEC,...] [-Depth diff|files|packages|project]
#                   [-MaxFileDiff N] [-MaxFileLines N] [-ReviewsDir DIR] [-Untrusted] [-DryRun]
# PowerShell port of intake.sh (same outputs). Resolves the review scope in the current git
# repo and writes <ReviewsDir>/<id>/<NNN>/context/ (default .reviews at the repo root). Default scope: all uncommitted changes (staged + unstaged) + untracked
# files. -Exclude drops paths from the scope. -Depth: see intake.sh (review-set.tsv).
# Size guard: see intake.sh. Symlinks are never reviewed. -DryRun prints the summary without
# writing anything. Trust (trusted= in scope.txt, -Untrusted): see intake.sh.
# Exit: 0 ok | 2 usage | 3 code on disk is not the code under review | 4 empty diff | 5 not a git repo.
[CmdletBinding()]
param(
    [string]$Id = '',
    [string]$Base = '',
    [string]$Range = '',
    [string]$Pr = '',
    [switch]$Staged,
    [string[]]$Files = @(),
    [string[]]$Exclude = @(),
    [string]$Depth = 'diff',
    [int]$MaxFileDiff = 800,
    [int]$MaxFileLines = 2000,
    [string]$ReviewsDir = '.reviews',
    [switch]$Untrusted,
    [switch]$DryRun
)

# -File passes 'a,b' as one string: split list parameters on commas ourselves.
$Files = @($Files | ForEach-Object { $_ -split ',' } | Where-Object { $_ })
$Exclude = @($Exclude | ForEach-Object { $_ -split ',' } | Where-Object { $_ })
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$utf8 = [Text.UTF8Encoding]::new($false)
# Patches are handled as Latin-1 (byte-exact, 1 char = 1 byte, like awk under LC_ALL=C);
# paths and names taken from them are re-decoded as UTF-8 with FixPath.
$latin1 = [Text.Encoding]::GetEncoding(28591)
function FixPath([string]$s) { $utf8.GetString($latin1.GetBytes($s)) }
$LockFiles = '(go\.sum|package-lock\.json|yarn\.lock|pnpm-lock\.yaml|Cargo\.lock|composer\.lock|Gemfile\.lock|poetry\.lock|gomod2nix\.toml)'
$ShardFiles = 30; $ShardLines = 2500; $BinFiles = 15; $BinLines = 1200; $MaxRefSymbols = 200; $MaxRefs = 25
# Full-depth shards weigh whole files, not changed lines.
$SetShardFiles = 30; $SetShardLines = 6000; $SetBinFiles = 20; $SetBinLines = 4000

function OrdDict { , (New-Object System.Collections.Specialized.OrderedDictionary ([StringComparer]::Ordinal)) }
function OrdinalUnique { $a = [string[]]@($input | Select-Object -Unique); [Array]::Sort($a, [StringComparer]::Ordinal); $a }
function Die([int]$code, [string]$msg) { [Console]::Error.WriteLine("intake: $msg"); exit $code }
function Out-Lines([string]$path, $lines, $enc = $utf8) {
    $items = @(@($lines) | Where-Object { $null -ne $_ })
    $text = if ($items.Count) { ($items -join "`n") + "`n" } else { '' }
    [IO.File]::WriteAllText($path, $text, $enc)
}
function GitOut { $o = & git -c core.quotePath=false @args 2>$null; if ($LASTEXITCODE -ne 0) { return $null }; $o }

if ('diff', 'files', 'packages', 'project' -cnotcontains $Depth) { Die 2 '-Depth must be diff, files, packages, or project' }
$scopes = @($Base, $Range, $Pr) | Where-Object { $_ }
if (@($scopes).Count + [int]$Staged.IsPresent + [int](@($Files).Count -gt 0) -gt 1) { Die 2 'pass only one scope' }

$top = GitOut rev-parse --show-toplevel
if (-not $top) { Die 5 'not inside a git repository' }
Set-Location -LiteralPath $top
$head = GitOut rev-parse HEAD
if (-not $head) { Die 5 'repository has no commits' }

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("intake-" + [guid]::NewGuid().ToString('N'))
$ctx = Join-Path $tmp 'context'
New-Item -ItemType Directory -Path $ctx -Force | Out-Null
$patch = Join-Path $ctx 'diff.patch'

try {
    # --- scope -> diff.patch ----------------------------------------------------------
    $Exclude = @($Exclude | Where-Object { $_ } | ForEach-Object { $_.TrimEnd('/') })
    function GitRawTo([string]$file, [string[]]$a) {
        & git -c core.quotePath=false diff --no-color --no-ext-diff -U8 "--output=$file" @a 2>$null | Out-Null
    }
    function GitDiffTo([string]$file, [string[]]$a) {
        if ($Exclude.Count) { $a = @($a) + @('--', '.') + @($Exclude | ForEach-Object { ":(exclude)$_" }) }
        GitRawTo $file $a
    }
    function IsExcluded([string]$p) { foreach ($x in $Exclude) { if ($p -ceq $x -or $p.StartsWith("$x/", [StringComparison]::Ordinal)) { return $true } }; $false }
    function Append([string]$from) {
        if (Test-Path -LiteralPath $from) {
            $s = [IO.File]::OpenRead($from); $d = [IO.File]::Open($patch, 'Append')
            try { $s.CopyTo($d) } finally { $s.Dispose(); $d.Dispose() }
            Remove-Item -LiteralPath $from
        }
    }
    function SaveLog([string]$rng) { Out-Lines (Join-Path $ctx 'commits.txt') (GitOut log --no-color '--format=%h %s%n%b' $rng) }
    $part = Join-Path $tmp 'part.patch'
    # baseSha: the commit the diff is taken against (apidiff); empty when the PR
    # base is not available locally.
    $baseSha = $head

    if ($Base) {
        if (-not (GitOut rev-parse --verify -q "$Base^{commit}")) { Die 2 "unknown base ref: $Base" }
        $baseSha = GitOut merge-base $Base HEAD
        if (-not $baseSha) { Die 2 "no merge base between $Base and HEAD" }
        GitDiffTo $patch @("$Base...HEAD"); SaveLog "$Base..HEAD"; $desc = "$Base...HEAD"
    } elseif ($Range) {
        if ($Range -cnotmatch '^(.+?)\.\.(.+)$') { Die 2 '-Range must be A..B' }
        $bsha = GitOut rev-parse --verify -q "$($Matches[2])^{commit}"
        if (-not $bsha) { Die 2 "unknown ref: $($Matches[2])" }
        if ($bsha -ne $head) { Die 3 "range end $($Matches[2]) is not HEAD; check out $($Matches[2]) so checks run on the reviewed code" }
        $baseSha = GitOut rev-parse --verify -q "$($Matches[1])^{commit}"
        if (-not $baseSha) { Die 2 "unknown ref: $($Matches[1])" }
        GitDiffTo $patch @($Range); SaveLog $Range; $desc = $Range
    } elseif ($Pr) {
        if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { Die 2 '-Pr needs the gh CLI' }
        $meta = & gh pr view $Pr --json headRefOid,baseRefOid,url,isCrossRepository,author 2>$null | ConvertFrom-Json
        if (-not $meta) { Die 2 "gh pr view $Pr failed" }
        if ($meta.headRefOid -ne $head) { Die 3 "HEAD is not PR #$Pr head ($($meta.headRefOid)); run 'gh pr checkout $Pr' first" }
        & git cat-file -e "$($meta.baseRefOid)^{commit}" 2>$null
        if ($LASTEXITCODE -eq 0) {
            $mb = GitOut merge-base $meta.baseRefOid HEAD
            GitDiffTo $patch @("$mb...HEAD"); SaveLog "$mb..HEAD"
            $baseSha = "$mb"
        } else {
            if ($Exclude.Count) { Die 2 '-Exclude needs the PR base commit locally (git fetch it first)' }
            $baseSha = ''
            [IO.File]::WriteAllText($patch, ((& gh pr diff $Pr --color=never) -join "`n") + "`n", $utf8)
        }
        $pv = & gh pr view $Pr --json title,body,url 2>$null | ConvertFrom-Json
        if ($pv) { Out-Lines (Join-Path $ctx 'pr.md') @("# $($pv.title)", '', $pv.url, '', $pv.body) }
        $desc = "PR #$Pr ($($meta.url))"
    } elseif ($Staged) {
        GitDiffTo $patch @('--cached'); $desc = 'staged changes'
    } elseif (@($Files).Count) {
        [IO.File]::WriteAllText($patch, '', $utf8)
        foreach ($f in $Files) {
            & git ls-files --error-unmatch -- $f 2>$null | Out-Null
            if ($LASTEXITCODE -eq 0) { GitRawTo $part @('HEAD', '--', $f); Append $part }
            elseif (Test-Path -LiteralPath $f) { GitRawTo $part @('--no-index', '--', '/dev/null', $f); Append $part }
        }
        $desc = "files: $($Files -join ' ')"
    } else {
        GitDiffTo $patch @('HEAD')
        if (-not (Test-Path -LiteralPath $patch)) { [IO.File]::WriteAllText($patch, '', $utf8) }
        foreach ($f in @(GitOut ls-files --others --exclude-standard)) {
            if ($f -and -not (IsExcluded $f)) { GitRawTo $part @('--no-index', '--', '/dev/null', $f); Append $part }
        }
        $desc = 'uncommitted changes (staged + unstaged) + untracked files'
    }
    if (-not (Test-Path -LiteralPath $patch) -or (Get-Item -LiteralPath $patch).Length -eq 0) { Die 4 "no changes in scope ($desc)" }

    # --- trust: is HEAD someone else's fork PR? (see intake.sh) -------------------------
    $trusted = 'yes'; $trustReason = 'local'
    $branch = GitOut symbolic-ref --short -q HEAD
    $prInfo = $null
    if ($Pr) { $prInfo = $meta }
    elseif ($branch -and (Get-Command gh -ErrorAction SilentlyContinue)) {
        $prInfo = & gh pr view --json isCrossRepository,author 2>$null | ConvertFrom-Json
    }
    if ($prInfo -and $prInfo.isCrossRepository -eq $true) {
        $viewer = "$(& gh api user --jq .login 2>$null)".Trim()
        if (-not $viewer -or $viewer -cne "$($prInfo.author.login)") {
            $trusted = 'no'; $trustReason = "fork PR by $(if ($prInfo.author.login) { $prInfo.author.login } else { 'unknown' })"
        } else { $trustReason = 'own fork PR' }
    } elseif ($prInfo -and $prInfo.isCrossRepository -eq $false) { $trustReason = 'same-repository PR' }
    elseif ($Pr) { $trusted = 'no'; $trustReason = "fork status of PR #$Pr unknown" }
    if ($branch -and "$(GitOut config "branch.$branch.merge")" -clike 'refs/pull/*') { $trusted = 'no'; $trustReason = 'branch tracks a pull ref' }
    if ($Untrusted) { $trusted = 'no'; $trustReason = '-Untrusted' }

    # --- issue id ---------------------------------------------------------------------
    if (-not $Id) { $Id = GitOut symbolic-ref --short -q HEAD; if (-not $Id) { Die 2 'detached HEAD: pass -Id' } }
    $slug = (($Id.ToLowerInvariant() -creplace '[^a-z0-9._-]', '-') -creplace '-+', '-') -creplace '^[-.]*', '' -creplace '-*$', ''
    if (-not $slug -or $slug -eq '.' -or $slug -eq '..') { Die 2 "issue id '$Id' slugifies to nothing" }
    $last = 0
    $idDir = Join-Path $ReviewsDir $slug
    if (Test-Path -LiteralPath $idDir) {
        Get-ChildItem -LiteralPath $idDir -Directory | Where-Object { $_.Name -cmatch '^[0-9]+$' } |
            ForEach-Object { if ([int]$_.Name -gt $last) { $last = [int]$_.Name } }
    }
    $nn = '{0:D3}' -f ($last + 1)
    $run = "$ReviewsDir/$slug/$nn"

    # --- classify ---------------------------------------------------------------------
    function LangOf([string]$p) {
        $b = ($p -creplace '.*/', '').ToLowerInvariant()
        switch -Regex -CaseSensitive ($b) {
            '\.go$' { return 'go' }
            '\.(ts|tsx|mts|cts)$' { return 'ts' }
            '\.(js|jsx|mjs|cjs)$' { return 'js' }
            '\.(scss|sass|css|less)$' { return 'css' }
            '\.rego$' { return 'rego' }
            '\.proto$' { return 'proto' }
            '\.(ya?ml|tpl)$' { return 'yaml' }
            '\.(sh|bash|zsh)$' { return 'sh' }
            '\.(ps1|psm1)$' { return 'ps1' }
            '\.py$' { return 'py' }
            '\.(md|markdown|rst|txt|adoc)$' { return 'doc' }
            '\.(json|toml|ini|conf|cfg|env)$' { if ($b -cnotmatch '^package(-lock)?\.json$') { return 'config' } }
            '^(makefile|.*\.mk)$' { return 'make' }
            '^dockerfile|\.dockerfile$|^containerfile' { return 'docker' }
            '^go\.(mod|sum|work)$|^package(-lock)?\.json$|^(yarn\.lock|pnpm-lock\.yaml|cargo\.lock)$' { return 'deps' }
        }
        'other'
    }
    function KindOf([string]$p, [string]$l) {
        if ($p -cmatch '(^|/)(vendor|node_modules|third_party)/') { return 'vendor' }
        if ($p -cmatch '(\.pb\.go|_gen\.go|\.gen\.go|zz_generated[^/]*|\.pb\.[a-z]+|\.min\.js|\.lock)$' -or $p -cmatch '(^|/)(dist|gen|generated)/' -or $p -cmatch ('(^|/)' + $LockFiles + '$')) { return 'generated' }
        # Kronk outputs: make kronk-docs (manual, SDK, examples, CLI reference) and the embedded BUI bundle
        if ($p -cmatch '^cmd/server/api/services/kronk/static/' -or $p -cmatch '^cmd/server/api/frontends/bui/src/components/(DocsManual|DocsSDK(Kronk|Model|Pool|Bucky|BuckyModel|Malina|MalinaModel|Examples))\.tsx$' -or $p -ceq 'cmd/server/api/frontends/bui/src/components/cli-reference.json') { return 'generated' }
        if ($p -cmatch '(_test\.go|\.(test|spec)\.[jt]sx?|_test\.py)$' -or $p -cmatch '(^|/)(tests?|testdata|__tests__|e2e)/') { return 'test' }
        if ($l -eq 'doc') { return 'doc' }
        if ('config', 'yaml', 'deps', 'make', 'docker' -contains $l) { return 'config' }
        'prod'
    }
    # Kronk owners (.manual/chapter-20-developer-guide.md §20.3); first match wins.
    function LayerOf([string]$p) {
        switch -Regex -CaseSensitive ($p) {
            '^cmd/kronk/' { return 'cli' }
            '^cmd/server/foundation/' { return 'foundation' }
            '^cmd/server/app/sdk/' { return 'app-sdk' }
            '^cmd/server/app/' { return 'app' }
            '^cmd/server/api/frontends/bui/' { return 'bui' }
            '^cmd/server/api/tooling/' { return 'tooling' }
            '^cmd/server/' { return 'service' }
            '^sdk/kronk/model/' { return 'sdk-model' }
            '^sdk/kronk/parsers/' { return 'sdk-parsers' }
            '^sdk/(kronk/|bucky/|malina/)?pool/' { return 'sdk-pool' }
            '^sdk/kronk/' { return 'sdk-kronk' }
            '^sdk/bucky/' { return 'sdk-bucky' }
            '^sdk/malina/' { return 'sdk-malina' }
            '^sdk/tools/' { return 'sdk-tools' }
            '^sdk/' { return 'sdk' }
            '^examples/' { return 'examples' }
            '^\.manual/' { return 'manual' }
            '^zarf/' { return 'deploy' }
            '^\.github/' { return 'ci' }
        }
        '-'
    }
    function StripPath([string]$s, [string]$pfx) { FixPath ((($s.Substring(4)) -creplace "^$pfx/", '') -creplace "`t.*", '') }

    # Split on LF only (keeps CR bytes, like awk); drop the empty tail after the final LF.
    $lines = [IO.File]::ReadAllText($patch, $latin1).Split([char]10)
    if ($lines.Count -and $lines[-1] -eq '') { $lines = $lines[0..($lines.Count - 2)] }
    $recs = New-Object System.Collections.Generic.List[object]
    $cur = $null; $inh = $false; $i = -1
    function AddRec { if ($cur) { if (-not $cur.Path) { $cur.Path = $cur.GitPath }; $cur.End = $i - 1; if ($cur.Path) { $recs.Add($cur) } } }
    foreach ($ln in $lines) {
        $i++
        if ($ln.StartsWith('diff --git ')) {
            AddRec
            # fallback for header-only sections (mode change): "diff --git a/P b/P"
            $g = $ln.Substring(13); $gp = ''
            if (($g.Length - 3) % 2 -eq 0) { $n = ($g.Length - 3) / 2; if ($g.Substring(0, $n) -ceq $g.Substring($n + 3)) { $gp = FixPath $g.Substring(0, $n) } }
            $cur = [pscustomobject]@{ St = 'M'; Add = 0; Del = 0; Path = ''; GitPath = $gp; Bytes = 0; Start = $i; End = 0; Link = $false }; $inh = $true
            $cur.Bytes += $ln.Length + 1; continue
        }
        if (-not $cur) { continue }
        $cur.Bytes += $ln.Length + 1
        if ($inh) {
            # mode 120000 = symlink (new, deleted, mode change, or "index a..b 120000")
            if ($ln -cmatch '^(new file mode|deleted file mode|new mode|old mode|index [0-9a-f]+\.\.[0-9a-f]+) 120000$') { $cur.Link = $true }
            if ($ln.StartsWith('new file mode')) { $cur.St = 'A' }
            elseif ($ln.StartsWith('deleted file mode')) { $cur.St = 'D' }
            elseif ($ln.StartsWith('rename to ')) { $cur.St = 'R'; $cur.Path = FixPath $ln.Substring(10) }
            elseif ($ln.StartsWith('Binary files ') -and -not $cur.Path) {
                $p = $ln -creplace '^Binary files .* and (b/)?', '' -creplace ' differ$', ''
                if ($p -eq '/dev/null') { $p = $ln -creplace '^Binary files (a/)?', '' -creplace ' and .*', '' }
                $cur.Path = FixPath $p
            }
            elseif ($ln.StartsWith('--- ')) { if ($ln -ne '--- /dev/null' -and -not $cur.Path) { $cur.Path = StripPath $ln 'a' } }
            elseif ($ln.StartsWith('+++ ')) { if ($ln -ne '+++ /dev/null') { $cur.Path = StripPath $ln 'b' }; $inh = $false }
            elseif ($ln.StartsWith('@@')) { $inh = $false }
            continue
        }
        if ($ln.StartsWith('+')) { $cur.Add++ } elseif ($ln.StartsWith('-')) { $cur.Del++ }
    }
    $i = $lines.Count; AddRec

    $langOf = New-Object hashtable ([StringComparer]::Ordinal); $kindOf = New-Object hashtable ([StringComparer]::Ordinal)
    $recs2 = foreach ($r in $recs) {
        # Symlinks are never reviewed: the target can be outside the repo (~/.aws/credentials).
        $l = LangOf $r.Path; $k = if ($r.Link) { 'symlink' } else { KindOf $r.Path $l }
        $item = Get-Item -LiteralPath $r.Path -Force -ErrorAction SilentlyContinue
        if ($item -and ($item.LinkType -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint))) { $k = 'symlink' }
        if ($k -ne 'generated' -and $k -ne 'vendor' -and $k -ne 'symlink' -and $r.St -ne 'D' -and (Test-Path -LiteralPath $r.Path -PathType Leaf)) {
            if ((GitOut check-attr linguist-generated -- $r.Path) -cmatch ': (set|true)$') { $k = 'generated' }
            if ($k -ne 'generated' -and 'go', 'proto', 'ts', 'js', 'py' -contains $l) {
                if ((Get-Content -LiteralPath $r.Path -TotalCount 5 -ErrorAction SilentlyContinue) -cmatch 'Code generated .*DO NOT EDIT') { $k = 'generated' }
            }
            if ($k -ne 'generated' -and 'js', 'ts', 'css' -contains $l) {
                foreach ($x in [IO.File]::ReadLines((Resolve-Path -LiteralPath $r.Path).Path, $latin1)) { if ($x.Length -gt 1000) { $k = 'generated'; break } }
            }
        }
        $langOf[$r.Path] = $l; $kindOf[$r.Path] = $k
        [pscustomobject]@{ St = $r.St; Add = $r.Add; Del = $r.Del; Lang = $l; Kind = $k; Layer = (LayerOf $r.Path); Path = $r.Path; Bytes = $r.Bytes; Start = $r.Start; End = $r.End }
    }
    $allRecs = @($recs2)
    function RowOf($f) { "$($f.St)`t$($f.Add)`t$($f.Del)`t$($f.Lang)`t$($f.Kind)`t$($f.Layer)`t$($f.Path)" }
    function IsSkipped($f) { $f.Kind -eq 'generated' -or $f.Kind -eq 'vendor' -or $f.Kind -eq 'symlink' }
    $fileRecs = @($allRecs | Where-Object { -not (IsSkipped $_) })
    $skippedRecs = @($allRecs | Where-Object { IsSkipped $_ })
    Out-Lines (Join-Path $ctx 'files.tsv') ($fileRecs | ForEach-Object { RowOf $_ })
    Out-Lines (Join-Path $ctx 'skipped.tsv') ($skippedRecs | ForEach-Object { RowOf $_ })

    # Generated and vendored files are never reviewed: move their diff sections out of
    # diff.patch into generated.patch (secret scanning only).
    if ($skippedRecs.Count) {
        $drop = New-Object 'bool[]' $lines.Count
        foreach ($f in $skippedRecs) { for ($j = $f.Start; $j -le $f.End; $j++) { $drop[$j] = $true } }
        $kept = New-Object System.Collections.Generic.List[string]; $gen = New-Object System.Collections.Generic.List[string]
        for ($j = 0; $j -lt $lines.Count; $j++) { if ($drop[$j]) { $gen.Add($lines[$j]) } else { $kept.Add($lines[$j]) } }
        Out-Lines (Join-Path $ctx 'generated.patch') $gen $latin1
        Out-Lines $patch $kept $latin1
        $lines = $kept.ToArray()
    }
    if (-not $fileRecs.Count) { Die 4 "only generated/vendored files or symlinks changed in scope ($desc); nothing to review" }

    # --- routing signals + changed symbols ------------------------------------------
    function GoSym([string]$s) {
        if ($s -cmatch '^func \(') {
            $r = ($s -creplace '^func \([^)]*[ *]', '') -creplace '\).*', '' -creplace '\[.*', ''
            $n = ($s -creplace '^func \([^)]*\) *', '') -creplace '[[(].*', ''
            if ($r -and $n) { return "method`t$r.$n" }; return ''
        }
        if ($s -cmatch '^func [A-Za-z_]') { return "func`t" + (($s -creplace '^func ', '') -creplace '[[(].*', '') }
        if ($s -cmatch '^type [A-Za-z_]') { return "type`t" + (($s -creplace '^type ', '') -creplace '[ [].*', '') }
        ''
    }
    function JsSym([string]$s) {
        $n = ($s -creplace '^export (default )?', '') -creplace '^async ', ''
        if ($n -cmatch '^function\*? *[A-Za-z_$]') { return "func`t" + (($n -creplace '^function\*? *', '') -creplace '[^A-Za-z0-9_$].*', '') }
        if ($n -cmatch '^(class|interface|type|enum) [A-Za-z_$]') { return "type`t" + (($n -creplace '^[a-z]+ ', '') -creplace '[^A-Za-z0-9_$].*', '') }
        if ($s -cmatch '^export (const|let) [A-Za-z_$]') { return "const`t" + (($n -creplace '^(const|let) ', '') -creplace '[^A-Za-z0-9_$].*', '') }
        ''
    }
    function Sym([string]$s, [string]$l) {
        if ($l -eq 'go') { return GoSym $s }
        if ('ts', 'js' -contains $l) { return JsSym $s }
        ''
    }

    $ADD = OrdDict; $DEL = OrdDict; $MOD = OrdDict
    $prodF = New-Object hashtable ([StringComparer]::Ordinal); $testF = New-Object hashtable ([StringComparer]::Ordinal); $prodAdd = 0
    $failure = 0; $contract = 0; $comments = 0
    $path = ''; $inh = $false; $oldln = 0; $newln = 0; $hk = ''; $hunkAt = 0
    foreach ($ln in $lines) {
        if ($ln.StartsWith('diff --git ')) { $path = ''; $inh = $true; continue }
        if ($inh) {
            if ($ln.StartsWith('+++ ')) { if ($ln -ne '+++ /dev/null') { $path = StripPath $ln 'b' }; $inh = $false }
            elseif ($ln.StartsWith('--- ')) { if ($ln -ne '--- /dev/null' -and -not $path) { $path = StripPath $ln 'a' } }
            continue
        }
        if ($ln -cmatch '^@@ -(\d+)[^ ]* \+(\d+)[^@]*@@ ?(.*)$') {
            $oldln = [int]$Matches[1] - 1; $newln = [int]$Matches[2] - 1
            $hk = Sym $Matches[3] $langOf[$path]; $hunkAt = $newln + 1; continue
        }
        if (-not $path) { continue }
        if ($ln.StartsWith('+') -or $ln.StartsWith('-')) {
            $c = $ln[0]; $s = $ln.Substring(1); $l = $langOf[$path]; $k = $kindOf[$path]
            if ($c -eq '+') { $newln++ } else { $oldln++ }
            if ($k -eq 'generated' -or $k -eq 'vendor') { continue }
            if ($k -eq 'prod') { if ($c -eq '+') { $prodAdd++ }; $prodF[$path] = 1 }
            if ($k -eq 'test') { $testF[$path] = 1 }
            if ($k -eq 'doc') { $comments = 1 }
            $t = $s -creplace '^[ \t]+', ''
            switch ($l) {
                'go' {
                    if ($t -cmatch '(^|[^A-Za-z0-9_])err([^A-Za-z0-9_]|$)|errors\.(Is|As|New|Join)|panic\(|recover\(|^defer |ctx\.Done|context\.With') { $failure = 1 }
                    if ($t -cmatch '^type [A-Za-z_]|struct \{|interface \{|^func New|^func \([^)]*\) [A-Z]|^func [A-Z]') { $contract = 1 }
                    if ($t -cmatch '^//|^/\*') { $comments = 1 }
                }
                { 'ts', 'js' -contains $_ } {
                    if ($t -cmatch 'catch|\.then\(|await |fetch\(|reject|throw ') { $failure = 1 }
                    if ($t -cmatch '^(export )?(interface|type) [A-Za-z_]|^export (class|function|const)') { $contract = 1 }
                    if ($t -cmatch '^//|^/\*|^\*') { $comments = 1 }
                }
                'proto' {
                    if ($t -cmatch '^(message|service|rpc|enum|oneof) |= [0-9]+;') { $contract = 1 }
                    if ($t -cmatch '^//') { $comments = 1 }
                }
                'rego' { $failure = 1; if ($t -cmatch '^#') { $comments = 1 } }
                { 'sh', 'ps1' -contains $_ } {
                    if ($t -cmatch 'set -e|exit |\|\||ErrorAction|try \{|trap ') { $failure = 1 }
                    if ($t -cmatch '^#') { $comments = 1 }
                }
                { 'yaml', 'py', 'make', 'docker' -contains $_ } { if ($t -cmatch '^#') { $comments = 1 } }
            }
            if ($t -cmatch 'TODO|FIXME|XXX') { $comments = 1 }
            $sk = Sym $t $l
            if ($sk) {
                $key = "$path`n$sk"
                if ($c -eq '+') { if (-not $ADD.Contains($key)) { $ADD[$key] = $newln } }
                elseif (-not $DEL.Contains($key)) { $DEL[$key] = $oldln }
            } elseif ($hk) {
                $key = "$path`n$hk"; if (-not $MOD.Contains($key)) { $MOD[$key] = $hunkAt }
            }
            continue
        }
        if ($ln.StartsWith(' ')) { $oldln++; $newln++ }
    }
    $logic = [int]($prodAdd -ge 20 -or $testF.Count -gt 0)
    $signals = @("prod_files=$($prodF.Count)", "test_files=$($testF.Count)", "prod_lines_added=$prodAdd",
        "failure=$failure", "contract=$contract", "comments=$comments", "logic=$logic")
    Out-Lines (Join-Path $ctx 'signals.txt') $signals

    $syms = New-Object System.Collections.Generic.List[object]
    function AddSym($st, $key, $at) { $p, $s = $key -split "`n", 2; $kd, $nm = $s -split "`t", 2; $nm = FixPath $nm; $syms.Add([pscustomobject]@{ St = $st; Kind = $kd; Name = $nm; Path = $p; Line = [int]$at }) }
    foreach ($x in $ADD.Keys) { AddSym $(if ($DEL.Contains($x)) { 'modified' } else { 'added' }) $x $ADD[$x] }
    foreach ($x in $DEL.Keys) { if (-not $ADD.Contains($x)) { AddSym 'removed' $x $DEL[$x] } }
    foreach ($x in $MOD.Keys) { if (-not $ADD.Contains($x) -and -not $DEL.Contains($x)) { AddSym 'modified' $x $MOD[$x] } }
    function SymLine($o) { "$($o.St)`t$($o.Kind)`t$($o.Name)`t$($o.Path)`t$($o.Line)" }
    $syms.Sort([Comparison[object]] { param($x, $y) $c = [string]::CompareOrdinal($x.Path, $y.Path); if ($c) { return $c }; $c = $x.Line.CompareTo($y.Line); if ($c) { return $c }; [string]::CompareOrdinal((SymLine $x), (SymLine $y)) })
    Out-Lines (Join-Path $ctx 'symbols.tsv') ($syms | ForEach-Object { SymLine $_ })

    # --- references to changed symbols: refs.tsv = symbol path:line ------------------
    # Best effort with rg (word match, capped); reviewers refine with gopls. Skipped files excluded.
    $refs = New-Object System.Collections.Generic.List[string]
    if ((Get-Command rg -CommandType Application -ErrorAction SilentlyContinue) -and $syms.Count) {
        $skipSet = New-Object hashtable ([StringComparer]::Ordinal)
        foreach ($f in $skippedRecs) { $skipSet[$f.Path] = 1 }
        $done = 0
        foreach ($s in $syms) {
            if ($done -ge $MaxRefSymbols) { break }; $done++
            $name = $s.Name -creplace '.*\.', ''
            $glob = if ($s.Path -cmatch '\.go$') { '*.go' } else { '*.{ts,tsx,js,jsx,mjs,cjs}' }
            $hits = @(& rg -n -w --no-heading --sort path -g $glob -g '!vendor/**' -g '!node_modules/**' -g '!.reviews/**' -- $name . 2>$null)
            $c = 0
            foreach ($o in $hits) {
                if (-not $o) { continue }
                $parts = "$o" -split ':', 3
                $p = ($parts[0] -creplace '\\', '/') -creplace '^\./', ''
                if ($skipSet.ContainsKey($p) -or ($p -ceq $s.Path -and $parts[1] -ceq "$($s.Line)")) { continue }
                # skip Go definitions of the symbol itself
                if ($p -cmatch '\.go$' -and $parts.Count -gt 2 -and $parts[2] -cmatch ('^[ \t]*(func (\([^)]*\) )?' + $name + '[[(]|type ' + $name + '[ [])')) { continue }
                $c++; if ($c -le $MaxRefs) { $refs.Add("$($s.Name)`t$($p):$($parts[1])") }
            }
            if ($c -gt $MaxRefs) { $refs.Add("$($s.Name)`t+$($c - $MaxRefs) more") }
        }
    }
    Out-Lines (Join-Path $ctx 'refs.tsv') $refs

    # --- review set (full depth): review-set.tsv ------------------------------------------
    # status added deleted lang kind layer path lines — the files.tsv columns plus the file's
    # line count; status is "-" for files the diff doesn't touch. The same content rules as for
    # changed files drop generated files; binary, empty, excluded, and .reviews files are left out.
    function DirOf([string]$p) { if ($p -cmatch '/') { $p -creplace '/[^/]*$', '' } else { '.' } }
    $setRecs = New-Object System.Collections.Generic.List[object]
    $setBytes = 0
    if ($Depth -ne 'diff') {
        $changed = @($fileRecs | Where-Object { $_.St -ne 'D' } | ForEach-Object { $_.Path })
        $cand = switch ($Depth) {
            'files' { $changed }
            'packages' {
                $dirs = New-Object hashtable ([StringComparer]::Ordinal)
                foreach ($p in $changed) { $dirs[(DirOf $p)] = $true }
                @(GitOut ls-files --cached --others --exclude-standard) | Where-Object { $_ -and $dirs.ContainsKey((DirOf $_)) }
            }
            'project' { @(GitOut ls-files --cached --others --exclude-standard) }
        }
        $rd = $ReviewsDir.TrimEnd('/') + '/'
        $cls = @(foreach ($p in @($cand | Where-Object { $_ } | OrdinalUnique)) {
                if ($p.StartsWith($rd, [StringComparison]::Ordinal) -or $p -cmatch '(^|/)\.reviews/' -or (IsExcluded $p)) { continue }
                $l = LangOf $p; $k = KindOf $p $l
                if ($k -ne 'generated' -and $k -ne 'vendor') { [pscustomobject]@{ Lang = $l; Kind = $k; Layer = (LayerOf $p); Path = $p } }
            })
        $lg = New-Object hashtable ([StringComparer]::Ordinal)
        if ($cls.Count) {
            foreach ($o in @($cls | ForEach-Object { $_.Path } | & git -c core.quotePath=false check-attr --stdin linguist-generated 2>$null)) {
                if ("$o" -cmatch '^(.*): linguist-generated: (set|true)$') { $lg[$Matches[1]] = $true }
            }
        }
        $chg = New-Object hashtable ([StringComparer]::Ordinal)
        foreach ($f in $fileRecs) { $chg[$f.Path] = $f }
        foreach ($c in $cls) {
            if ($lg.ContainsKey($c.Path) -or -not (Test-Path -LiteralPath $c.Path -PathType Leaf)) { continue }
            # Regular files only: a symlink can point outside the repo (~/.aws/credentials).
            $item = Get-Item -LiteralPath $c.Path -Force
            if ($item.LinkType -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { continue }
            $raw = [IO.File]::ReadAllBytes((Join-Path $top $c.Path))
            if ($raw.Length -eq 0 -or [Array]::IndexOf($raw, [byte]0) -ge 0) { continue }  # empty or binary
            $text = $latin1.GetString($raw)
            if ('go', 'proto', 'ts', 'js', 'py' -ccontains $c.Lang -and ((($text -split "`n", 6) | Select-Object -First 5) -join "`n") -cmatch 'Code generated .*DO NOT EDIT') { continue }
            if ('js', 'ts', 'css' -ccontains $c.Lang -and @($text -split "`n" | Where-Object { $_.Length -gt 1000 }).Count) { continue }
            $n = [regex]::Matches($text, "`n").Count; if (-not $text.EndsWith("`n")) { $n++ }  # = awk records
            $f = $chg[$c.Path]
            $st, $a, $d = if ($f) { $f.St, $f.Add, $f.Del } else { '-', 0, 0 }
            $setRecs.Add([pscustomobject]@{ St = $st; Add = $a; Del = $d; Lang = $c.Lang; Kind = $c.Kind; Layer = $c.Layer; Path = $c.Path; Lines = $n })
            $setBytes += $raw.Length
        }
    }
    Out-Lines (Join-Path $ctx 'review-set.tsv') ($setRecs | ForEach-Object { "$(RowOf $_)`t$($_.Lines)" })
    $tokSet = [int][Math]::Floor(($setBytes + 3) / 4)
    $setLines = 0; foreach ($r in $setRecs) { $setLines += $r.Lines }

    # --- totals + shards --------------------------------------------------------------
    $rev = @($fileRecs | Where-Object { $_.Kind -ne 'generated' -and $_.Kind -ne 'vendor' })
    $code = @($rev | Where-Object { $_.Kind -ne 'doc' })
    $added = ($rev | Measure-Object Add -Sum).Sum; if (-not $added) { $added = 0 }
    $deleted = ($rev | Measure-Object Del -Sum).Sum; if (-not $deleted) { $deleted = 0 }
    $codeLines = ($code | ForEach-Object { $_.Add + $_.Del } | Measure-Object -Sum).Sum; if (-not $codeLines) { $codeLines = 0 }
    # Code files only (docs aren't sharded), grouped by directory. Diff depth weighs changed
    # lines; full depth weighs the review set's whole files, plus deleted files' removed lines.
    $setCode = @($setRecs | Where-Object { $_.Kind -ne 'doc' })
    $setCodeLines = 0; foreach ($r in $setCode) { $setCodeLines += $r.Lines }
    $shards = 0; $bigSet = $false
    $items = New-Object System.Collections.Generic.List[object]
    if ($Depth -eq 'diff') {
        $bf = $BinFiles; $bl = $BinLines
        $doShard = $code.Count -gt $ShardFiles -or $codeLines -gt $ShardLines
        foreach ($f in $code) { $items.Add([pscustomobject]@{ Dir = (DirOf $f.Path); N = $f.Add + $f.Del; Path = $f.Path }) }
    } else {
        $bf = $SetBinFiles; $bl = $SetBinLines
        $bigSet = $setCode.Count -gt $SetShardFiles -or $setCodeLines -gt $SetShardLines
        $doShard = $bigSet
        foreach ($f in $setCode) { $items.Add([pscustomobject]@{ Dir = (DirOf $f.Path); N = $f.Lines; Path = $f.Path }) }
        foreach ($f in $code) { if ($f.St -eq 'D') { $items.Add([pscustomobject]@{ Dir = (DirOf $f.Path); N = $f.Del; Path = $f.Path }) } }
    }
    if ($doShard) {
        $s = 1; $nf = 0; $nl = 0; $dir = $null; $out = @()
        $items.Sort([Comparison[object]] { param($x, $y) $c = [string]::CompareOrdinal($x.Dir, $y.Dir); if ($c) { $c } else { [string]::CompareOrdinal($x.Path, $y.Path) } })
        foreach ($i in $items) {
            if ($null -ne $dir -and $i.Dir -ne $dir -and ($nf -ge $bf -or $nl -ge $bl)) { $s++; $nf = 0; $nl = 0 }
            $dir = $i.Dir; $nf++; $nl += $i.N; $out += "$s`t$($i.Path)"
        }
        if ($s -gt 1) { $shards = $s; Out-Lines (Join-Path $ctx 'shards.tsv') $out }
    }

    # --- size guard: size.tsv = flag path metric est_tokens suggestion -------------------
    $size = New-Object System.Collections.Generic.List[object]
    $tokReview = 0; $tokNoise = 0
    foreach ($f in $allRecs) {
        $t = [int][Math]::Floor(($f.Bytes + 3) / 4); $b = $f.Path -creplace '.*/', ''
        $lock = $b -cmatch ('^' + $LockFiles + '$')
        if (IsSkipped $f) {
            $tokNoise += $t
            $lbl = $f.Kind + $(if ($lock) { '/lockfile' } else { '' })
            $sug = if ($f.Kind -eq 'symlink') { 'skipped: symlinks are never reviewed (the target can be outside the repo)' } else { 'skipped: generated/vendored files are never reviewed (Kronk commits regenerated outputs with their sources)' }
            $size.Add([pscustomobject]@{ F = 'noise'; P = $f.Path; M = "$lbl $($f.Add + $f.Del) lines"; T = $t; S = $sug })
            continue
        }
        $tokReview += $t
        if ($f.Kind -ne 'doc' -and $f.Add + $f.Del -gt $MaxFileDiff) {
            $size.Add([pscustomobject]@{ F = 'big-diff'; P = $f.Path; M = "$($f.Add + $f.Del) changed lines"; T = $t; S = 'split the change, or review it alone (--files) / --exclude it' })
        }
        if ($f.St -ne 'D' -and (Test-Path -LiteralPath $f.Path -PathType Leaf)) {
            $full = (Resolve-Path -LiteralPath $f.Path).Path
            $raw = [IO.File]::ReadAllBytes($full); $n = [regex]::Matches($latin1.GetString($raw), "`n").Count  # = wc -l
            if ($n -gt $MaxFileLines) {
                $len = $raw.Length
                $size.Add([pscustomobject]@{ F = 'huge-file'; P = $f.Path; M = "$n lines in file"; T = [int][Math]::Floor(($len + 3) / 4); S = 'agents read it for context every time; split the file' })
            }
        }
    }
    if ($code.Count -gt $ShardFiles -or $codeLines -gt $ShardLines) {
        $sug = if ($Depth -eq 'diff') { "review shards into $shards parts per heavy lens; split the PR along shards.tsv" } else { 'split the PR into smaller changes' }
        $size.Add([pscustomobject]@{ F = 'big-pr'; P = '-'; M = "$($code.Count) code files, $codeLines changed lines"; T = $tokReview; S = $sug })
    }
    if ($bigSet) {
        $size.Add([pscustomobject]@{ F = 'big-review-set'; P = '-'; M = "$($setCode.Count) code files, $setCodeLines lines at depth $Depth"; T = $tokSet; S = "review shards into $shards parts per heavy lens; pick a narrower depth or --exclude paths" })
    }
    $sizeLines = foreach ($flag in 'big-pr', 'big-review-set', 'big-diff', 'huge-file', 'noise') {
        $grp = New-Object System.Collections.Generic.List[object]
        foreach ($w in $size) { if ($w.F -eq $flag) { $grp.Add($w) } }
        $grp.Sort([Comparison[object]] { param($x, $y) $c = $y.T.CompareTo($x.T); if ($c) { $c } else { [string]::CompareOrdinal($x.P, $y.P) } })
        foreach ($w in $grp) { "$($w.F)`t$($w.P)`t$($w.M)`t$($w.T)`t$($w.S)" }
    }
    $sizeLines = @($sizeLines)
    Out-Lines (Join-Path $ctx 'size.tsv') $sizeLines

    # --- Go packages grouped by module --------------------------------------------------
    # The packages of the changed Go files, or of the review set's at full depth. Directories
    # the go tool ignores (testdata, _x, .x) are left out.
    # scopedRecs: the files under review — files.tsv, or review-set.tsv at full depth.
    $scopedRecs = @(if ($Depth -eq 'diff') { $fileRecs } else { $setRecs })
    $pk = foreach ($d in ($scopedRecs | Where-Object { $_.Lang -eq 'go' -and $_.St -ne 'D' -and $_.Kind -ne 'vendor' } |
            ForEach-Object { DirOf $_.Path } | Where-Object { $_ -eq '.' -or $_ -cnotmatch '(^|/)(testdata|[_.][^/]*)(/|$)' } | OrdinalUnique)) {
        if (-not (Test-Path -LiteralPath $d -PathType Container)) { continue }
        $m = $d
        while (-not (Test-Path -LiteralPath "$m/go.mod") -and $m -ne '.') { $m = if ($m -cmatch '/') { $m -creplace '/[^/]*$', '' } else { '.' } }
        if (-not (Test-Path -LiteralPath "$m/go.mod")) { continue }
        $rel = if ($m -eq $d) { '.' } elseif ($m -eq '.') { "./$d" } else { './' + $d.Substring($m.Length + 1) }
        "$m`t$rel"
    }
    $pk = @($pk)
    Out-Lines (Join-Path $ctx 'packages.tsv') $pk

    # --- applicable repo rules ----------------------------------------------------------
    $rules = foreach ($r in @(GitOut ls-files 'AGENTS.md' '*/AGENTS.md' 'CLAUDE.md' '*/CLAUDE.md')) {
        if (-not $r) { continue }
        $d = $r -creplace '[^/]*$', ''
        if (-not $d) { $r; continue }
        if ($scopedRecs | Where-Object { $_.Path.StartsWith($d, [StringComparison]::Ordinal) } | Select-Object -First 1) { $r }
    }
    # Kronk's ownership map and verification rules apply to every change.
    if (Test-Path -LiteralPath '.manual/chapter-20-developer-guide.md' -PathType Leaf) { $rules = @($rules) + '.manual/chapter-20-developer-guide.md' }
    Out-Lines (Join-Path $ctx 'rules.txt') @($rules)

    # --- tools --------------------------------------------------------------------------
    $tools = foreach ($t in 'git', 'gh', 'go', 'gopls', 'staticcheck', 'gosec', 'govulncheck', 'dupl', 'checklocks',
        'errcheck', 'nilaway', 'deadcode', 'apidiff', 'gitleaks', 'trufflehog', 'osv-scanner', 'trivy', 'grype', 'jscpd',
        'ast-grep', 'rg', 'jq', 'shellcheck', 'node', 'npm', 'npx', 'opa', 'helm', 'python3', 'hadolint') {
        $c = Get-Command $t -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($c) { "$t`t$($c.Source)" }
    }
    Out-Lines (Join-Path $ctx 'tools.tsv') @($tools)

    # --- missing.tsv: tool needed_for fix -------------------------------------------------
    # Tools this change needs but that are absent, or Go tools built with an older Go release
    # than the active toolchain (analyzers built that way fail on the newer standard library).
    # Checks skip what is missing; the user is told how to install it.
    $toolPaths = @{}; foreach ($r in @($tools)) { $f = $r -split "`t"; $toolPaths[$f[0]] = $f[1] }
    function GoMinor([string]$s) { if ($s -cmatch 'go(\d+)\.(\d+)') { [version]"$($Matches[1]).$($Matches[2])" } else { $null } }
    $goCur = if ($toolPaths.ContainsKey('go')) { GoMinor ((& go env GOVERSION 2>$null) -join '') } else { $null }
    function Need([string]$t, [string]$for, [string]$fix, [switch]$Go) {
        if (-not $toolPaths.ContainsKey($t)) { return "$t`t$for`t$fix" }
        if (-not $Go -or -not $goCur) { return }
        $built = GoMinor ((& go version $toolPaths[$t] 2>$null) -join '')
        if ($built -and $built -lt $goCur) { "$t`t$for (built with go$built, toolchain is go${goCur}: rebuild)`t$fix" }
    }
    $liveLangs = @{}; foreach ($r in $scopedRecs) { if ($r.St -ne 'D') { $liveLangs[$r.Lang] = $true } }
    $missing = @(
        if ($pk.Count) {
            Need 'go' 'build, vet, tests' 'https://go.dev/dl/'
            if ($toolPaths.ContainsKey('go') -and ((& go env CGO_ENABLED 2>$null) -join '') -ne '1') {
                "cgo`tgo test -race (CGO_ENABLED=0)`tinstall a C compiler (clang/gcc) and set CGO_ENABLED=1"
            }
            Need 'gopls' 'symbol lookups by reviewers (optional, not in CI)' 'go install golang.org/x/tools/gopls@latest' -Go
            Need 'staticcheck' 'staticcheck -checks=all (CI static job)' 'go install honnef.co/go/tools/cmd/staticcheck@master' -Go
            Need 'gosec' 'Go security scan (optional, not in CI)' 'go install github.com/securego/gosec/v2/cmd/gosec@latest' -Go
            Need 'govulncheck' 'Go vulnerability scan' 'go install golang.org/x/vuln/cmd/govulncheck@latest' -Go
            Need 'dupl' 'Go clone detection (optional, not in CI)' 'go install github.com/mibk/dupl@latest' -Go
            Need 'checklocks' 'lock annotations (+checklocks) (optional, not in CI)' 'go install gvisor.dev/gvisor/tools/checklocks/cmd/checklocks@go' -Go
            Need 'errcheck' 'unchecked errors (optional, not in CI)' 'go install github.com/kisielk/errcheck@latest' -Go
            Need 'nilaway' 'nil dereferences (optional, not in CI)' 'go install go.uber.org/nilaway/cmd/nilaway@latest' -Go
            Need 'deadcode' 'unreachable functions (optional, not in CI)' 'go install golang.org/x/tools/cmd/deadcode@latest' -Go
            Need 'apidiff' 'exported API breaks (optional, not in CI)' 'go install golang.org/x/exp/cmd/apidiff@latest' -Go
        }
        if ($liveLangs['sh']) { Need 'shellcheck' 'shell lint (optional, not in CI)' 'https://github.com/koalaman/shellcheck#installing' }
        if ($liveLangs['js'] -or $liveLangs['ts']) { Need 'jscpd' 'JS/TS clone detection (optional, not in CI)' 'npm install -g jscpd' }
        if (-not $toolPaths.ContainsKey('trufflehog')) { Need 'gitleaks' 'secret scan (optional, not in CI)' 'go install github.com/zricethezav/gitleaks/v8@latest' }
        # BUI type check (bui-tsc) needs the BUI's own dependencies.
        if (($rev | Where-Object { $_.St -ne 'D' -and $_.Path -cmatch '^cmd/server/api/frontends/bui/.*\.tsx?$' } | Select-Object -First 1) -and
            -not (Test-Path -LiteralPath 'cmd/server/api/frontends/bui/node_modules' -PathType Container)) {
            "bui-deps`tBUI type check (tsc)`tcd cmd/server/api/frontends/bui && npm ci"
        }
        # Helm chart checks (helm-test) run zarf/helm/kronk/test.sh: helm plus python3 with PyYAML.
        if ($rev | Where-Object { $_.St -ne 'D' -and $_.Path -cmatch '^zarf/helm/' } | Select-Object -First 1) {
            Need 'helm' 'Helm chart checks (zarf/helm/kronk/test.sh)' 'https://helm.sh/docs/intro/install/'
            $py = $false; if ($toolPaths.ContainsKey('python3')) { & python3 -I -c 'import yaml' *> $null; $py = $LASTEXITCODE -eq 0 }
            if (-not $py) { "pyyaml`tHelm chart checks (zarf/helm/kronk/test.sh)`tpython3 -m pip install pyyaml" }
        }
    ) | Where-Object { $_ }
    $missing = @($missing)
    Out-Lines (Join-Path $ctx 'missing.tsv') $missing

    # --- scope.txt ----------------------------------------------------------------------
    $langs = ($rev | Group-Object Lang | ForEach-Object { "$($_.Name):$($_.Count)" }) -join ' '
    if (-not $branch) { $branch = 'detached' }
    $setInfo = if ($Depth -ne 'diff') { @("review_files=$($setRecs.Count)", "review_code_files=$($setCode.Count)", "review_lines=$setLines", "review_tokens_est=$tokSet") } else { @() }
    $scope = @("id=$slug", "nn=$nn", "run=$run", "root=$top", "scope=$desc", "depth=$Depth", "head=$head", "base=$baseSha", "branch=$branch", "trusted=$trusted", "trust_reason=$trustReason",
        "files=$($allRecs.Count)", "reviewable_files=$($rev.Count)", "skipped_files=$($skippedRecs.Count)", "lines_added=$added", "lines_deleted=$deleted",
        "code_files=$($code.Count)", "code_lines_changed=$codeLines", "languages=$langs", "go_packages=$($pk.Count)",
        "symbols=$($syms.Count)", "refs=$($refs.Count)", "shards=$shards",
        "diff_tokens_est=$tokReview") + $setInfo + @("noise_tokens_est=$tokNoise", "size_warnings=$($sizeLines.Count)", "missing_tools=$($missing.Count)") + $signals
    Out-Lines (Join-Path $ctx 'scope.txt') $scope

    $sizeOut = @(); if ($sizeLines.Count) { $sizeOut = @('# size warnings: flag path metric est_tokens suggestion') + $sizeLines }
    $missOut = @(); if ($missing.Count) { $missOut = @('# missing tools: tool needed_for install') + $missing }
    if ($DryRun) { '# dry run: nothing written'; $scope; $sizeOut; $missOut; exit 0 }

    New-Item -ItemType Directory -Path $idDir -Force | Out-Null
    if (Test-Path -LiteralPath $run) { Die 1 "run directory $run already exists" }
    New-Item -ItemType Directory -Path $run | Out-Null
    New-Item -ItemType Directory -Path "$run/lenses", "$run/security-issues/poc" | Out-Null
    Copy-Item -LiteralPath $ctx -Destination "$run/context" -Recurse
    $scope
    $sizeOut
    $missOut
} finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
