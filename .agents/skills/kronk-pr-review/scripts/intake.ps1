# Usage: intake.ps1 [-Id ID] [-Base REF | -Range A..B | -Pr N | -Staged | -Files F,...]
#                   [-Exclude PATHSPEC,...] [-Depth diff|files|packages|project]
#                   [-MaxFileDiff N] [-MaxFileLines N] [-ReviewsDir DIR] [-IncludeGenerated] [-DryRun]
# PowerShell port of intake.sh (same outputs). Resolves the review scope in the current git
# repo and writes <ReviewsDir>/<id>/<NNN>/context/ (default .reviews at the repo root). Default scope: all uncommitted changes (staged + unstaged) + untracked
# files. -Exclude drops paths from the scope. -Depth: see intake.sh (review-set.tsv).
# Size guard: see intake.sh. Generated and vendored files are skipped unless -IncludeGenerated;
# symlinks are never reviewed. -DryRun prints the summary without writing anything.
# Trust: see intake.sh — trust=maintainer only for a -Pr from a maintainer, on a base-repo branch.
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
    [switch]$IncludeGenerated,
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
$LockFiles = '(go\.sum|package-lock\.json|yarn\.lock|pnpm-lock\.yaml|Cargo\.lock|composer\.lock|Gemfile\.lock|poetry\.lock|gomod2nix\.toml|\.terraform\.lock\.hcl|uv\.lock|Pipfile\.lock|Chart\.lock)'
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
        $meta = & gh pr view $Pr --json headRefOid,baseRefOid,url 2>$null | ConvertFrom-Json
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
    # Every file in the repo (tracked + untracked, not ignored): marker lookups without a
    # filesystem probe per file. Infrastructure code (Terraform, Helm, K8s, Ansible, Dockerfiles,
    # CI pipelines) is prod: it is reviewed like any other code.
    function DirOf([string]$p) { if ($p -cmatch '/') { $p -creplace '/[^/]*$', '' } else { '.' } }
    $repoFiles = @(GitOut ls-files --cached --others --exclude-standard | Where-Object { $_ })
    $charts = @($repoFiles | Where-Object { $_ -cmatch '(^|/)Chart\.yaml$' } | ForEach-Object { DirOf $_ })
    function InChart([string]$p) { foreach ($c in $charts) { if ($c -eq '.' -or $p.StartsWith("$c/", [StringComparison]::Ordinal)) { return $true } }; $false }
    function LangOf([string]$p) {
        $b = ($p -creplace '.*/', '').ToLowerInvariant()
        if ($p -cmatch '(^|/)\.github/workflows/[^/]+\.ya?ml$|(^|/)\.gitlab-ci\.ya?ml$|(^|/)\.gitlab/.*\.ya?ml$|(^|/)\.(circleci|buildkite|tekton|woodpecker)/.*\.ya?ml$' -or
            $b -cmatch '^(jenkinsfile.*|.*\.jenkinsfile|action\.ya?ml|azure-pipelines.*\.ya?ml|bitbucket-pipelines\.ya?ml|cloudbuild.*\.ya?ml|\.drone\.ya?ml|\.travis\.ya?ml|\.woodpecker\.ya?ml)$') { return 'ci' }
        if ($b -cmatch '\.go$') { return 'go' }
        if ($b -cmatch '\.rs$') { return 'rust' }
        if ($b -cmatch '\.vue$') { return 'vue' }
        if ($b -cmatch '\.(ts|tsx|mts|cts)$') { return 'ts' }
        if ($b -cmatch '\.(js|jsx|mjs|cjs)$') { return 'js' }
        if ($b -cmatch '\.(scss|sass|css|less)$') { return 'css' }
        if ($b -cmatch '\.rego$') { return 'rego' }
        if ($b -cmatch '\.proto$') { return 'proto' }
        if ($b -cmatch '\.sql$') { return 'sql' }
        if ($b -cmatch '\.pyi?$') { return 'py' }
        if ($b -ceq '.tflint.hcl') { return 'config' }
        if ($b -cmatch '\.(tf|tofu|tfvars|hcl)$|\.tf\.json$') { return 'tf' }
        if ($b -cmatch '^(dockerfile|containerfile)([.-].*)?$|\.(dockerfile|containerfile)$|^(docker-)?compose([.-].*)?\.ya?ml$') { return 'docker' }
        if ((InChart $p) -and ($b -cmatch '^(chart\.ya?ml|values[^/]*\.(ya?ml|json))$' -or ($p -cmatch '(^|/)templates/' -and $b -cmatch '\.(ya?ml|tpl|txt|json)$'))) { return 'helm' }
        if ($b -cmatch '^kustomization(\.ya?ml)?$') { return 'k8s' }
        if ($b -ceq 'ansible.cfg' -or ($b -cmatch '\.(ya?ml|j2)$' -and $p -cmatch '(^|/)(roles|playbooks|group_vars|host_vars|inventories|molecule)/')) { return 'ansible' }
        if ($b -cmatch '\.(ya?ml|tpl)$') { return 'yaml' }
        if ($b -cmatch '\.(sh|bash|zsh)$') { return 'sh' }
        if ($b -cmatch '\.(ps1|psm1)$') { return 'ps1' }
        if ($b -cmatch '^(go\.(mod|sum|work)|package(-lock)?\.json|yarn\.lock|pnpm-lock\.yaml|cargo\.(toml|lock)|pyproject\.toml|requirements[^/]*\.txt|pipfile(\.lock)?|poetry\.lock|uv\.lock)$') { return 'deps' }
        if ($b -cmatch '\.(md|markdown|rst|txt|adoc)$') { return 'doc' }
        if ($b -cmatch '\.(json|toml|ini|conf|cfg|env)$') { return 'config' }
        if ($b -cmatch '^(makefile|justfile|.*\.mk)$') { return 'make' }
        if ($b -cmatch '^(build|workspace)(\.bazel)?$|\.bzl$') { return 'bazel' }
        if ($b -cmatch '\.(java|kt|kts|scala|cs|rb|php|c|h|cc|cpp|cxx|hpp|swift|lua|ex|exs|dart|nix|zig|groovy|cue|jsonnet|libsonnet)$') { return $Matches[1] }
        'other'
    }
    function KindOf([string]$p, [string]$l) {
        if ($p -cmatch '(^|/)(vendor|node_modules|third_party)/') { return 'vendor' }
        if ($p -cmatch '(\.pb\.go|_gen\.go|\.gen\.go|zz_generated[^/]*|\.pb\.[a-z]+|\.min\.js|\.lock)$' -or $p -cmatch '(^|/)(dist|gen|generated|\.terraform)/' -or $p -cmatch ('(^|/)' + $LockFiles + '$')) { return 'generated' }
        # Kronk outputs: make kronk-docs (manual, SDK, examples, CLI reference) and the embedded BUI bundle
        if ($p -cmatch '^cmd/server/api/services/kronk/static/' -or $p -cmatch '^cmd/server/api/frontends/bui/src/components/(DocsManual|DocsSDK(Kronk|Model|Pool|Bucky|BuckyModel|Malina|MalinaModel|Examples))\.tsx$' -or $p -ceq 'cmd/server/api/frontends/bui/src/components/cli-reference.json') { return 'generated' }
        if ($p -cmatch '(_test\.go|\.(test|spec)\.[jt]sx?|_test\.py|_spec\.rb|\.tftest\.hcl)$' -or $p -cmatch '(^|/)(tests?|testdata|__tests__|e2e|molecule)/' -or $p -cmatch '(^|/)(test_[^/]*\.py|conftest\.py)$') { return 'test' }
        if ($l -eq 'doc') { return 'doc' }
        if ('config', 'yaml', 'deps', 'make' -contains $l) { return 'config' }
        'prod'
    }
    # YamlKind TEXT: k8s for a manifest (apiVersion + kind), ansible for a playbook/task file.
    function YamlKind([string]$text) {
        if ($text -cmatch '(?m)^- (hosts|import_playbook):|^[ \t-]*ansible\.(builtin|posix)\.') { return 'ansible' }
        if ($text -cmatch '(?m)^apiVersion:' -and $text -cmatch '(?m)^kind:') { return 'k8s' }
        ''
    }
    # Kronk owners (.manual/chapter-20-developer-guide.md §20.3); first match wins.
    function LayerOf([string]$p, [string]$l) {
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
            if ($k -ne 'generated' -and 'go', 'proto', 'ts', 'js', 'py', 'rust' -contains $l) {
                if ((Get-Content -LiteralPath $r.Path -TotalCount 5 -ErrorAction SilentlyContinue) -cmatch 'Code generated .*DO NOT EDIT') { $k = 'generated' }
            }
            if ($k -ne 'generated' -and 'js', 'ts', 'css' -contains $l) {
                foreach ($x in [IO.File]::ReadLines((Resolve-Path -LiteralPath $r.Path).Path, $latin1)) { if ($x.Length -gt 1000) { $k = 'generated'; break } }
            }
            if ($l -eq 'yaml') {
                $y = YamlKind ([IO.File]::ReadAllText((Resolve-Path -LiteralPath $r.Path).Path, $latin1))
                if ($y) { $l = $y; if ($k -eq 'config') { $k = 'prod' } }
            }
        }
        $langOf[$r.Path] = $l; $kindOf[$r.Path] = $k
        [pscustomobject]@{ St = $r.St; Add = $r.Add; Del = $r.Del; Lang = $l; Kind = $k; Layer = (LayerOf $r.Path $l); Path = $r.Path; Bytes = $r.Bytes; Start = $r.Start; End = $r.End }
    }
    $allRecs = @($recs2)
    function RowOf($f) { "$($f.St)`t$($f.Add)`t$($f.Del)`t$($f.Lang)`t$($f.Kind)`t$($f.Layer)`t$($f.Path)" }
    # Not reviewed: symlinks always; generated and vendored files unless -IncludeGenerated
    # (included ones keep kind generated/vendor in files.tsv).
    function IsSkipped($f) { $f.Kind -eq 'symlink' -or (-not $IncludeGenerated -and ($f.Kind -eq 'generated' -or $f.Kind -eq 'vendor')) }
    $fileRecs = @($allRecs | Where-Object { -not (IsSkipped $_) })
    $skippedRecs = @($allRecs | Where-Object { IsSkipped $_ })
    Out-Lines (Join-Path $ctx 'files.tsv') ($fileRecs | ForEach-Object { RowOf $_ })
    Out-Lines (Join-Path $ctx 'skipped.tsv') ($skippedRecs | ForEach-Object { RowOf $_ })

    # Files not reviewed: move their diff sections out of diff.patch into generated.patch
    # (secret scanning only).
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
    function RsSym([string]$s) {
        $n = $s -creplace '^pub(\([^)]*\))? +', ''
        while ($n -cmatch '^(async|unsafe|const|default|extern( "[^"]*")?) +[a-z]') { $n = $n -creplace '^[a-z]+( "[^"]*")? +', '' }
        if ($n -cmatch '^fn [A-Za-z_]') { return "func`t" + (($n -creplace '^fn ', '') -creplace '[^A-Za-z0-9_].*', '') }
        if ($n -cmatch '^(struct|enum|trait|union|type|mod) [A-Za-z_]') { return "type`t" + (($n -creplace '^[a-z]+ ', '') -creplace '[^A-Za-z0-9_].*', '') }
        if ($n -cmatch '^macro_rules! *[A-Za-z_]') { return "macro`t" + (($n -creplace '^macro_rules! *', '') -creplace '[^A-Za-z0-9_].*', '') }
        ''
    }
    function PySym([string]$s) {
        $n = $s -creplace '^async ', ''
        if ($n -cmatch '^def [A-Za-z_]') { return "func`t" + (($n -creplace '^def ', '') -creplace '[^A-Za-z0-9_].*', '') }
        if ($n -cmatch '^class [A-Za-z_]') { return "type`t" + (($n -creplace '^class ', '') -creplace '[^A-Za-z0-9_].*', '') }
        ''
    }
    # Terraform: names are the reference address (aws_s3_bucket.this, data.x.y, module.m, var.v).
    function TfSym([string]$s) {
        if ($s -cnotmatch '^(resource|data|module|variable|output) +"[^"]+"') { return '' }
        $k = $s -creplace ' .*', ''; $n = $s -creplace '^[a-z]+ +', ''
        if ($k -eq 'resource' -or $k -eq 'data') {
            if (-not ($n -cmatch '^"[^"]+" +"[^"]+"')) { return '' }
            $n = ($Matches[0] -creplace '"', '') -creplace ' +', '.'
            return "$k`t" + $(if ($k -eq 'data') { 'data.' } else { '' }) + $n
        }
        $null = $n -cmatch '^"([^"]+)"'; $nm = $Matches[1]
        $pre = switch ($k) { 'module' { 'module.' } 'variable' { 'var.' } default { '' } }
        "$k`t$pre$nm"
    }
    function HelmSym([string]$s) { if ($s -cnotmatch '\{\{-? *define +"') { return '' }; "template`t" + (($s -creplace '.*define +"', '') -creplace '".*', '') }
    function Sym([string]$s, [string]$l) {
        if ($l -eq 'go') { return GoSym $s }
        if ('ts', 'js', 'vue' -contains $l) { return JsSym $s }
        if ($l -eq 'rust') { return RsSym $s }
        if ($l -eq 'py') { return PySym $s }
        if ($l -eq 'tf') { return TfSym $s }
        if ($l -eq 'helm') { return HelmSym $s }
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
            if ('prod', 'generated', 'vendor' -contains $k) { if ($c -eq '+') { $prodAdd++ }; $prodF[$path] = 1 }
            if ($k -eq 'test') { $testF[$path] = 1 }
            if ($k -eq 'doc') { $comments = 1 }
            $t = $s -creplace '^[ \t]+', ''
            switch ($l) {
                'go' {
                    if ($t -cmatch '(^|[^A-Za-z0-9_])err([^A-Za-z0-9_]|$)|errors\.(Is|As|New|Join)|panic\(|recover\(|^defer |ctx\.Done|context\.With') { $failure = 1 }
                    if ($t -cmatch '^type [A-Za-z_]|struct \{|interface \{|^func New|^func \([^)]*\) [A-Z]|^func [A-Z]') { $contract = 1 }
                    if ($t -cmatch '^//|^/\*') { $comments = 1 }
                }
                { 'ts', 'js', 'vue' -contains $_ } {
                    if ($t -cmatch 'catch|\.then\(|await |fetch\(|reject|throw ') { $failure = 1 }
                    if ($t -cmatch 'defineProps|defineEmits|props:|emits:|^(export )?(interface|type) [A-Za-z_]|^export (class|function|const)') { $contract = 1 }
                    if ($t -cmatch '^//|^/\*|^\*') { $comments = 1 }
                }
                'sql' {
                    if ($t -cmatch 'BEGIN|ROLLBACK|COMMIT|begin|rollback|commit') { $failure = 1 }
                    if ($t -cmatch 'CREATE |ALTER |DROP |create |alter |drop ') { $contract = 1 }
                    if ($t -cmatch '^--') { $comments = 1 }
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
                'rust' {
                    if ($t -cmatch 'Result<|Err\(|\?[;)]|\.unwrap\(|\.expect\(|panic!|unreachable!|todo!|unsafe ') { $failure = 1 }
                    if ($t -cmatch '^pub |^#\[derive|^impl |^trait ') { $contract = 1 }
                    if ($t -cmatch '^//|^/\*') { $comments = 1 }
                }
                'py' {
                    if ($t -cmatch '^(try:|except|finally:|raise |assert )|with ') { $failure = 1 }
                    if ($t -cmatch '^(async )?def [A-Za-z]|^class [A-Za-z]|^__all__') { $contract = 1 }
                    if ($t -cmatch '^#|^"""') { $comments = 1 }
                }
                'tf' {
                    if ($t -cmatch 'lifecycle|prevent_destroy|create_before_destroy|ignore_changes|precondition|postcondition|validation|^check |depends_on|count *=|for_each *=') { $failure = 1 }
                    if ($t -cmatch '^(variable|output|module|provider|terraform|required_providers|resource|data|moved|removed|import) ') { $contract = 1 }
                    if ($t -cmatch '^(#|//|/\*)') { $comments = 1 }
                }
                { 'helm', 'k8s' -contains $_ } {
                    if ($t -cmatch 'Probe:|restartPolicy|terminationGracePeriod|preStop|PodDisruptionBudget|maxUnavailable|required |fail ') { $failure = 1 }
                    if ($t -cmatch '^kind:|^apiVersion:|selector:|matchLabels:|ports:|serviceAccountName|define |\.Values\.' -or $path -cmatch '(^|/)values[^/]*\.(ya?ml|json)$') { $contract = 1 }
                    if ($t -cmatch '^#|\{\{-? */\*') { $comments = 1 }
                }
                'ansible' {
                    if ($t -cmatch 'failed_when|ignore_errors|changed_when|^- *block:|rescue:|always:|any_errors_fatal|until:|retries:') { $failure = 1 }
                    if ($path -cmatch '(^|/)(defaults|vars|meta)/' -and $t -cnotmatch '^#') { $contract = 1 }
                    if ($t -cmatch '^#') { $comments = 1 }
                }
                'docker' {
                    if ($t -cmatch 'HEALTHCHECK|pipefail|set -e|\|\| *exit') { $failure = 1 }
                    if ($t -cmatch '^(FROM|EXPOSE|ENTRYPOINT|CMD|USER|ENV|ARG|VOLUME|ports:|image:)') { $contract = 1 }
                    if ($t -cmatch '^#') { $comments = 1 }
                }
                'ci' {
                    if ($t -cmatch 'continue-on-error|allow_failure|always\(\)|\|\| *true|set -e|retry|timeout') { $failure = 1 }
                    if ($t -cmatch '^on:|workflow_call|workflow_dispatch|inputs:|outputs:|secrets:|permissions:|needs:|stages:|parameters') { $contract = 1 }
                    if ($t -cmatch '^#|^//') { $comments = 1 }
                }
                { 'yaml', 'make' -contains $_ } { if ($t -cmatch '^#') { $comments = 1 } }
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
            # Search term and file glob per language; Terraform and Helm names are full addresses.
            $name = $s.Name -creplace '.*\.', ''
            $glob = switch -Regex -CaseSensitive ($s.Path) {
                '\.go$' { '*.go'; break }
                '\.rs$' { '*.rs'; break }
                '\.pyi?$' { '*.{py,pyi}'; break }
                '\.(tf|tofu|tfvars|hcl)$|\.tf\.json$' { '*.{tf,tofu,tfvars,hcl}'; if ($s.Kind -ne 'output') { $name = $s.Name }; break }
                '\.(yaml|yml|tpl|txt|json)$' { '*.{tpl,yaml,yml,txt}'; $name = $s.Name; break }
                default { '*.{ts,tsx,js,jsx,mjs,cjs,vue}' }
            }
            $hits = @(& rg -n -F -w --no-heading --sort path -g $glob -g '!vendor/**' -g '!node_modules/**' -g '!.reviews/**' -- $name . 2>$null)
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
    # changed files drop generated files (or mark them, with -IncludeGenerated); binary, empty,
    # excluded, and .reviews files are left out.
    $setRecs = New-Object System.Collections.Generic.List[object]
    $setBytes = 0
    if ($Depth -ne 'diff') {
        $changed = @($fileRecs | Where-Object { $_.St -ne 'D' } | ForEach-Object { $_.Path })
        $cand = switch ($Depth) {
            'files' { $changed }
            'packages' {
                $dirs = New-Object hashtable ([StringComparer]::Ordinal)
                foreach ($p in $changed) { $dirs[(DirOf $p)] = $true }
                $repoFiles | Where-Object { $dirs.ContainsKey((DirOf $_)) }
            }
            'project' { $repoFiles }
        }
        $rd = $ReviewsDir.TrimEnd('/') + '/'
        $cls = @(foreach ($p in @($cand | Where-Object { $_ } | OrdinalUnique)) {
                if ($p.StartsWith($rd, [StringComparison]::Ordinal) -or $p -cmatch '(^|/)\.reviews/' -or (IsExcluded $p)) { continue }
                $l = LangOf $p; $k = KindOf $p $l
                if ($IncludeGenerated -or ($k -ne 'generated' -and $k -ne 'vendor')) { [pscustomobject]@{ Lang = $l; Kind = $k; Layer = (LayerOf $p $l); Path = $p } }
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
            if (-not (Test-Path -LiteralPath $c.Path -PathType Leaf)) { continue }
            # Regular files only: a symlink can point outside the repo (~/.aws/credentials).
            $item = Get-Item -LiteralPath $c.Path -Force
            if ($item.LinkType -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { continue }
            $raw = [IO.File]::ReadAllBytes((Join-Path $top $c.Path))
            if ($raw.Length -eq 0 -or [Array]::IndexOf($raw, [byte]0) -ge 0) { continue }  # empty or binary
            $text = $latin1.GetString($raw)
            $gen = $lg.ContainsKey($c.Path) -or
                ('go', 'proto', 'ts', 'js', 'py', 'rust' -ccontains $c.Lang -and ((($text -split "`n", 6) | Select-Object -First 5) -join "`n") -cmatch 'Code generated .*DO NOT EDIT') -or
                ('js', 'ts', 'css' -ccontains $c.Lang -and @($text -split "`n" | Where-Object { $_.Length -gt 1000 }).Count)
            if ($gen) { if (-not $IncludeGenerated) { continue }; $c.Kind = 'generated' }
            if ($c.Lang -eq 'yaml') { $y = YamlKind $text; if ($y) { $c.Lang = $y; if ($c.Kind -eq 'config') { $c.Kind = 'prod' } } }
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
    $rev = $fileRecs
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
            $sug = if ($f.Kind -eq 'symlink') { 'skipped: symlinks are never reviewed (the target can be outside the repo)' } else { 'skipped: generated/vendored files are not reviewed unless opted in (Kronk commits regenerated outputs with their sources)' }
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
    $tools = foreach ($t in 'git', 'gh', 'go', 'gopls', 'staticcheck', 'golangci-lint', 'gosec', 'govulncheck', 'dupl',
        'checklocks', 'errcheck', 'nilaway', 'deadcode', 'apidiff', 'gitleaks', 'trufflehog', 'detect-secrets', 'semgrep', 'osv-scanner', 'trivy', 'grype', 'jscpd', 'pmd', 'ast-grep', 'rg', 'shellcheck',
        'node', 'npm', 'pnpm', 'yarn', 'opa', 'conftest', 'buf', 'sqlfluff', 'helm', 'kubectl', 'terraform', 'tofu', 'tflint', 'hadolint',
        'cargo', 'cargo-audit', 'cargo-deny', 'rust-analyzer', 'python3', 'ruff', 'mypy', 'pytest', 'bandit', 'pip-audit', 'ansible-lint',
        'yamllint', 'kubeconform', 'kube-linter', 'kustomize', 'checkov', 'actionlint', 'zizmor', 'timeout', 'gtimeout', 'jq', 'npx') {
        $c = Get-Command $t -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($c) { "$t`t$($c.Source)" }
    }
    Out-Lines (Join-Path $ctx 'tools.tsv') @($tools)

    # --- ecosystems: stack.tsv = eco unit files marker ------------------------------------
    # One row per build/deploy unit of the reviewed files (see intake.sh); "-" as unit for
    # file-level ecosystems. The Terraform marker is the binary to use.
    $hasTool = @{}; foreach ($r in @($tools)) { $hasTool[($r -split "`t")[0]] = $true }
    $tfWant = if ($repoFiles | Where-Object { $_ -cmatch '\.tofu$|(^|/)\.opentofu-version$' } | Select-Object -First 1) { 'tofu' } else { 'terraform' }
    $tfBin = $tfWant
    if (-not $hasTool[$tfWant]) {
        if ($tfWant -eq 'tofu' -and $hasTool['terraform']) { $tfBin = 'terraform' }
        elseif ($tfWant -eq 'terraform' -and $hasTool['tofu']) { $tfBin = 'tofu' }
    }
    $mk = @{}; foreach ($n in 'cargo', 'chart', 'kust', 'pyp', 'ans', 'pkg', 'tfm') { $mk[$n] = New-Object hashtable ([StringComparer]::Ordinal) }
    foreach ($f in $repoFiles) {
        $b = $f -creplace '.*/', ''; $d = DirOf $f
        if ($b -ceq 'Cargo.toml') { $mk.cargo[$d] = 1 }
        elseif ($b -ceq 'Chart.yaml') { $mk.chart[$d] = 1 }
        elseif ($b -cmatch '^(kustomization\.ya?ml|Kustomization)$') { $mk.kust[$d] = 1 }
        elseif ($b -cmatch '^(pyproject\.toml|setup\.py|setup\.cfg)$') { $mk.pyp[$d] = 1 }
        elseif ($b -ceq 'ansible.cfg') { $mk.ans[$d] = 1 }
        elseif ($b -ceq 'package.json') { $mk.pkg[$d] = 1 }
        elseif ($b -cmatch '\.(tf|tofu)$') { $mk.tfm[$d] = 1 }
    }
    function Up([string]$d, $set) { while ($true) { if ($set.ContainsKey($d)) { return $d }; if ($d -eq '.') { return '' }; $d = DirOf $d } }
    function CiOf([string]$p) {
        $b = ($p -creplace '.*/', '').ToLowerInvariant()
        if ($p -cmatch '(^|/)\.github/' -or $b -cmatch '^action\.ya?ml$') { return 'github-actions' }
        if ($p -cmatch '(^|/)\.gitlab') { return 'gitlab-ci' }
        if ($b -cmatch 'jenkinsfile') { return 'jenkins' }
        if ($p -cmatch '(^|/)\.circleci/') { return 'circleci' }
        if ($b -cmatch '^azure-pipelines') { return 'azure-pipelines' }
        if ($b -cmatch '^bitbucket-pipelines') { return 'bitbucket-pipelines' }
        if ($p -cmatch '(^|/)\.buildkite/') { return 'buildkite' }
        if ($p -cmatch '(^|/)\.tekton/') { return 'tekton' }
        if ($b -cmatch '^cloudbuild') { return 'cloud-build' }
        if ($b -cmatch 'drone') { return 'drone' }
        if ($b -cmatch 'travis') { return 'travis' }
        'woodpecker'
    }
    $units = New-Object hashtable ([StringComparer]::Ordinal)
    function AddUnit([string]$e, [string]$u, [string]$m) { $key = "$e`t$u`t$m"; $units[$key] = 1 + $(if ($units.ContainsKey($key)) { $units[$key] } else { 0 }) }
    foreach ($r in $scopedRecs) {
        if ($r.St -eq 'D') { continue }
        $p = $r.Path; $l = $r.Lang; $d = DirOf $p; $b = $p -creplace '.*/', ''
        if ($l -eq 'rust' -or $b -ceq 'Cargo.toml') { $u = Up $d $mk.cargo; if ($u) { AddUnit 'rust' $u 'Cargo.toml' } }
        elseif ($l -eq 'py' -or $b -ceq 'pyproject.toml' -or $b -cmatch '^requirements[^/]*\.txt$') { $u = Up $d $mk.pyp; if ($u) { AddUnit 'python' $u 'pyproject' } else { AddUnit 'python' '.' '-' } }
        elseif ($l -eq 'tf') { AddUnit 'terraform' $(if ($mk.tfm.ContainsKey($d)) { $d } else { '-' }) $tfBin }
        elseif ($l -eq 'helm') { $u = Up $d $mk.chart; if ($u) { AddUnit 'helm' $u 'Chart.yaml' } }
        elseif ($l -eq 'k8s') { $u = Up $d $mk.kust; if ($u) { AddUnit 'kustomize' $u 'kustomization' } else { AddUnit 'k8s' '-' 'manifests' } }
        elseif ($l -eq 'ansible') { $u = Up $d $mk.ans; if ($u) { AddUnit 'ansible' $u 'ansible.cfg' } else { AddUnit 'ansible' '.' '-' } }
        elseif ($l -eq 'docker') { AddUnit 'docker' '-' $(if ($b -cmatch 'compose') { 'compose' } else { 'dockerfile' }) }
        elseif ($l -eq 'ci') { AddUnit 'ci' '-' (CiOf $p) }
        elseif ('js', 'ts', 'vue' -contains $l) { $u = Up $d $mk.pkg; if ($u) { AddUnit 'node' $u 'package.json' } else { AddUnit 'node' '.' '-' } }
    }
    $goMods = @{}; foreach ($r in $pk) { $m = ($r -split "`t")[0]; $goMods[$m] = 1 + $(if ($goMods.ContainsKey($m)) { $goMods[$m] } else { 0 }) }
    $stack = [string[]]@(@($goMods.Keys | ForEach-Object { "go`t$_`t$($goMods[$_])`tgo.mod" }) +
        @($units.Keys | ForEach-Object { $f = $_ -split "`t"; "$($f[0])`t$($f[1])`t$($units[$_])`t$($f[2])" }))
    [Array]::Sort($stack, [StringComparer]::Ordinal)
    Out-Lines (Join-Path $ctx 'stack.tsv') $stack
    function Eco([string]$e, [string]$m = '') { [bool]($stack | Where-Object { $f = $_ -split "`t"; $f[0] -ceq $e -and (-not $m -or $f[3] -ceq $m) } | Select-Object -First 1) }
    function RepoHas([string]$re) { [bool]($repoFiles | Where-Object { $_ -cmatch $re } | Select-Object -First 1) }

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
            $mods = @($pk | ForEach-Object { ($_ -split "`t")[0] } | Select-Object -Unique)
            if ($mods | Where-Object { Get-ChildItem -LiteralPath $_ -File -Force -ErrorAction SilentlyContinue | Where-Object { $_.Name -cmatch '^\.golangci\.(ya?ml|toml|json)$' } }) {
                Need 'golangci-lint' 'repo golangci-lint config' 'go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest' -Go
            }
            Need 'gosec' 'Go security scan (optional, not in CI)' 'go install github.com/securego/gosec/v2/cmd/gosec@latest' -Go
            Need 'govulncheck' 'Go vulnerability scan' 'go install golang.org/x/vuln/cmd/govulncheck@latest' -Go
            Need 'dupl' 'Go clone detection (optional, not in CI)' 'go install github.com/mibk/dupl@latest' -Go
            Need 'checklocks' 'lock annotations (+checklocks) (optional, not in CI)' 'go install gvisor.dev/gvisor/tools/checklocks/cmd/checklocks@go' -Go
            Need 'errcheck' 'unchecked errors (optional, not in CI)' 'go install github.com/kisielk/errcheck@latest' -Go
            Need 'nilaway' 'nil dereferences (optional, not in CI)' 'go install go.uber.org/nilaway/cmd/nilaway@latest' -Go
            Need 'deadcode' 'unreachable functions (optional, not in CI)' 'go install golang.org/x/tools/cmd/deadcode@latest' -Go
            Need 'apidiff' 'exported API breaks (optional, not in CI)' 'go install golang.org/x/exp/cmd/apidiff@latest' -Go
        }
        if ($liveLangs['proto'] -and ((Test-Path -LiteralPath 'buf.yaml') -or (Test-Path -LiteralPath 'buf.work.yaml'))) {
            Need 'buf' 'proto lint and breaking changes' 'go install github.com/bufbuild/buf/cmd/buf@latest'
        }
        if ($liveLangs['sh']) { Need 'shellcheck' 'shell lint (optional, not in CI)' 'https://github.com/koalaman/shellcheck#installing' }
        if ($liveLangs['js'] -or $liveLangs['ts'] -or $liveLangs['vue']) { Need 'jscpd' 'JS/TS clone detection (optional, not in CI)' 'npm install -g jscpd' }
        if (-not $toolPaths.ContainsKey('trufflehog')) { Need 'gitleaks' 'secret scan (optional, not in CI)' 'go install github.com/zricethezav/gitleaks/v8@latest' }
        # BUI type check (bui-tsc) needs the BUI's own dependencies.
        if (($rev | Where-Object { $_.St -ne 'D' -and $_.Path -cmatch '^cmd/server/api/frontends/bui/.*\.tsx?$' } | Select-Object -First 1) -and
            -not (Test-Path -LiteralPath 'cmd/server/api/frontends/bui/node_modules' -PathType Container)) {
            "bui-deps`tBUI type check (tsc)`tcd cmd/server/api/frontends/bui && npm ci"
        }
        # Helm chart checks (helm-test) run zarf/helm/kronk/test.sh: helm plus python3 with PyYAML.
        if ($rev | Where-Object { $_.St -ne 'D' -and $_.Path -cmatch '^zarf/helm/' } | Select-Object -First 1) {
            # helm itself is requested by the chart row below (Eco 'helm').
            $py = $false; if ($toolPaths.ContainsKey('python3')) { & python3 -I -c 'import yaml' *> $null; $py = $LASTEXITCODE -eq 0 }
            if (-not $py) { "pyyaml`tHelm chart checks (zarf/helm/kronk/test.sh)`tpython3 -m pip install pyyaml" }
        }
        if (Eco 'rust') {
            Need 'cargo' 'Rust build, fmt, clippy, tests' 'https://rustup.rs'
            if ($toolPaths.ContainsKey('cargo')) {
                # from the user's home: rustup and cargo read rust-toolchain.toml and .cargo/config.toml
                # from the working directory and its parents (not the repo, a shared temp dir, or a volume root)
                Push-Location -LiteralPath ([Environment]::GetFolderPath('UserProfile'))
                & cargo fmt --version *> $null; $fmtRc = $LASTEXITCODE
                & cargo clippy --version *> $null; $clippyRc = $LASTEXITCODE
                Pop-Location
                if ($fmtRc -ne 0) { "rustfmt`tcargo fmt --check`trustup component add rustfmt" }
                if ($clippyRc -ne 0) { "clippy`tRust lints`trustup component add clippy" }
            }
            Need 'cargo-audit' 'Rust vulnerability scan' 'cargo install cargo-audit --locked'
            if (RepoHas '(^|/)deny\.toml$') { Need 'cargo-deny' 'repo cargo-deny config' 'cargo install cargo-deny --locked' }
        }
        if (Eco 'terraform') {
            if ($tfWant -eq 'tofu') { Need $tfBin 'OpenTofu fmt, init, validate' 'https://opentofu.org/docs/intro/install/' }
            else { Need $tfBin 'Terraform fmt, init, validate' 'https://developer.hashicorp.com/terraform/install' }
            Need 'tflint' 'Terraform lint' 'https://github.com/terraform-linters/tflint#installation'
        }
        if (Eco 'helm') { Need 'helm' 'chart lint and render' 'https://helm.sh/docs/intro/install/' }
        if ((Eco 'helm') -or (Eco 'kustomize') -or (Eco 'k8s')) {
            Need 'kubeconform' 'K8s schema validation' 'go install github.com/yannh/kubeconform/cmd/kubeconform@latest'
            Need 'kube-linter' 'K8s best practices' 'go install golang.stackrox.io/kube-linter/cmd/kube-linter@latest'
        }
        if ((Eco 'kustomize') -and -not $toolPaths.ContainsKey('kubectl')) {
            Need 'kustomize' 'render Kustomize roots' 'go install sigs.k8s.io/kustomize/kustomize/v5@latest'
        }
        if (((Eco 'terraform') -or (Eco 'helm') -or (Eco 'kustomize') -or (Eco 'k8s') -or (Eco 'docker')) -and -not $toolPaths.ContainsKey('checkov')) {
            Need 'trivy' 'IaC misconfiguration scan' 'https://trivy.dev/latest/getting-started/installation/'
        }
        if (Eco 'python') {
            Need 'ruff' 'Python lint and format' 'python3 -m pip install ruff'
            if (RepoHas '(^|/)(mypy\.ini|\.mypy\.ini)$') { Need 'mypy' 'repo mypy config' 'python3 -m pip install mypy' }
            if (RepoHas '(^|/)(test_[^/]*|[^/]*_test)\.py$') { Need 'pytest' 'Python tests' 'python3 -m pip install pytest' }
            Need 'bandit' 'Python security scan' 'python3 -m pip install bandit'
            if (RepoHas '(^|/)requirements[^/]*\.txt$') { Need 'pip-audit' 'Python vulnerability scan' 'python3 -m pip install pip-audit' }
        }
        if (Eco 'ansible') { Need 'ansible-lint' 'Ansible lint' 'python3 -m pip install ansible-lint' }
        if ((RepoHas '(^|/)\.yamllint(\.ya?ml)?$') -and ($liveLangs['yaml'] -or (Eco 'ansible') -or (Eco 'k8s') -or (Eco 'kustomize') -or (Eco 'ci'))) {
            Need 'yamllint' 'repo yamllint config' 'python3 -m pip install yamllint'
        }
        if (Eco 'docker' 'dockerfile') { Need 'hadolint' 'Dockerfile lint' 'https://github.com/hadolint/hadolint#install' }
        if (Eco 'ci' 'github-actions') {
            Need 'actionlint' 'GitHub Actions lint' 'go install github.com/rhysd/actionlint/cmd/actionlint@latest'
            Need 'zizmor' 'GitHub Actions security' 'python3 -m pip install zizmor'
        }
        if (@('.semgrep.yml', '.semgrep.yaml', '.semgrep') | Where-Object { Test-Path -LiteralPath $_ }) {
            Need 'semgrep' 'repo semgrep rules' 'python3 -m pip install semgrep'
        }
    ) | Where-Object { $_ }
    $missing = @($missing)
    Out-Lines (Join-Path $ctx 'missing.tsv') $missing

    # --- trust: may checks run the reviewed code? (see intake.sh) ----------------------------
    $trust = 'untrusted'; $trustReason = 'not a pull request (--pr)'
    if ($Pr) {
        $trustReason = 'could not read the PR author from GitHub'
        $tj = ((& gh pr view $Pr --json author,isCrossRepository,url --jq '.author.login + " " + (.isCrossRepository | tostring) + " " + .url' 2>$null) -join ' ').Trim()
        $parts = @($tj -split ' ') + @('', '', '')
        $login = $parts[0]; $cross = $parts[1]; $prUrl = $parts[2]
        $prHost = if ($prUrl -cmatch '^https://([^/]*)/[^/]*/[^/]*/pull/') { $Matches[1] } else { '' }
        $repo = if ($prUrl -cmatch '^https://[^/]*/([^/]*/[^/]*)/pull/') { $Matches[1] } else { '' }
        if ($login -and $repo) {
            if ($cross -ne 'false') { $trustReason = 'PR head branch is in a fork' }
            else {
                $perm = ((& gh api --hostname $prHost "repos/$repo/collaborators/$login/permission" --jq .permission 2>$null) -join '').Trim()
                if ('admin', 'maintain', 'write' -ccontains $perm) { $trust = 'maintainer'; $trustReason = "$login has $perm access to $repo; head branch in $repo" }
                elseif (-not $perm) { $trustReason = "could not read $login's permission on $repo" }
                else { $trustReason = "$login has $perm access to $repo" }
            }
        }
    }

    # --- scope.txt ----------------------------------------------------------------------
    $langs = ($rev | Group-Object Lang | ForEach-Object { "$($_.Name):$($_.Count)" }) -join ' '
    $branch = GitOut symbolic-ref --short -q HEAD; if (-not $branch) { $branch = 'detached' }
    $ecos = (@($stack | ForEach-Object { ($_ -split "`t")[0] }) | Group-Object -NoElement | ForEach-Object { "$($_.Name):$($_.Count)" }) -join ' '
    # idioms: the references/langs/<name>.md files reviewers load for the reviewed languages
    $idiomOf = @{ go = 'go'; rust = 'rust'; ansible = 'ansible'; docker = 'docker'; ci = 'ci'; sql = 'sql'; proto = 'proto'; rego = 'rego'
        vue = 'web'; ts = 'web'; js = 'web'; css = 'web'; py = 'python'; tf = 'terraform'; helm = 'helm-k8s'; k8s = 'helm-k8s'; sh = 'shell'; ps1 = 'shell'; make = 'shell' }
    $idioms = [string[]]@($scopedRecs | Where-Object { $_.St -ne 'D' -and $idiomOf.ContainsKey($_.Lang) } | ForEach-Object { $idiomOf[$_.Lang] } | Select-Object -Unique)
    [Array]::Sort($idioms, [StringComparer]::Ordinal)
    # unmapped.txt: reviewed files in languages no idiom file covers (for the discovery agent)
    $mapped = 'go', 'rust', 'vue', 'ts', 'js', 'css', 'py', 'tf', 'helm', 'k8s', 'ansible', 'docker', 'ci', 'sh', 'ps1', 'make', 'sql', 'proto', 'rego', 'doc', 'config', 'deps', 'yaml'
    $unmapped = @($scopedRecs | Where-Object { $_.St -ne 'D' -and $mapped -cnotcontains $_.Lang -and $_.Path -cnotmatch '(^|/)(\.git[a-z]*|\.editorconfig|\.DS_Store|LICENSE[^/]*|NOTICE|COPYING|CODEOWNERS)$' } |
            ForEach-Object { "$($_.Lang)`t$($_.Path)" })
    Out-Lines (Join-Path $ctx 'unmapped.txt') $unmapped
    $setInfo = if ($Depth -ne 'diff') { @("review_files=$($setRecs.Count)", "review_code_files=$($setCode.Count)", "review_lines=$setLines", "review_tokens_est=$tokSet") } else { @() }
    $scope = @("id=$slug", "nn=$nn", "run=$run", "root=$top", "scope=$desc", "depth=$Depth", "head=$head", "base=$baseSha", "branch=$branch",
        "files=$($allRecs.Count)", "reviewable_files=$($rev.Count)", "skipped_files=$($skippedRecs.Count)", "lines_added=$added", "lines_deleted=$deleted",
        "code_files=$($code.Count)", "code_lines_changed=$codeLines", "languages=$langs", "ecosystems=$ecos", "idioms=$($idioms -join ',')",
        "generated=$(if ($IncludeGenerated) { 'included' } else { 'skipped' })", "trust=$trust", "trust_reason=$trustReason", "go_packages=$($pk.Count)",
        "symbols=$($syms.Count)", "refs=$($refs.Count)", "shards=$shards",
        "diff_tokens_est=$tokReview") + $setInfo + @("noise_tokens_est=$tokNoise", "size_warnings=$($sizeLines.Count)", "missing_tools=$($missing.Count)") + $signals
    Out-Lines (Join-Path $ctx 'scope.txt') $scope

    # summary: scope, stack, unmapped languages (first 20), size warnings, missing tools
    $summary = @($scope)
    if ($stack.Count) { $summary += @('# stack: eco unit files marker') + $stack }
    if ($unmapped.Count) { $summary += @('# unmapped: lang path (first 20)') + @($unmapped | Select-Object -First 20) }
    if ($sizeLines.Count) { $summary += @('# size warnings: flag path metric est_tokens suggestion') + $sizeLines }
    if ($missing.Count) { $summary += @('# missing tools: tool needed_for install') + $missing }
    if ($DryRun) { '# dry run: nothing written'; $summary; exit 0 }

    New-Item -ItemType Directory -Path $idDir -Force | Out-Null
    if (Test-Path -LiteralPath $run) { Die 1 "run directory $run already exists" }
    New-Item -ItemType Directory -Path $run | Out-Null
    New-Item -ItemType Directory -Path "$run/lenses", "$run/security-issues/poc" | Out-Null
    Copy-Item -LiteralPath $ctx -Destination "$run/context" -Recurse
    $summary
} finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
