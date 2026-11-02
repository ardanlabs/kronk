#!/bin/sh
# Usage: sh intake.sh [--id ID] [--base REF | --range A..B | --pr N | --staged | --files F...]
#                     [--exclude PATHSPEC]... [--depth diff|files|packages|project]
#                     [--max-file-diff N] [--max-file-lines N] [--reviews-dir DIR] [--dry-run]
# Resolves the review scope in the current git repo and writes the run directory
# <reviews-dir>/<id>/<NNN>/context/ (default .reviews at the repo root) (diff, file inventory, symbols, routing signals,
# shards, packages, tools, missing tools, rules, size warnings). Default scope: all uncommitted changes (staged + unstaged) +
# untracked files. --exclude drops paths from the scope (git pathspec; repeatable).
# --depth sets what reviewers may report on: diff (default) = the change only; files / packages /
# project = a full review of the changed files, every file in their directories, or the whole
# repo, listed in review-set.tsv (generated, vendored, binary, and excluded files left out).
# Size guard: flags files with > --max-file-diff changed lines (800), files agents must
# read with > --max-file-lines lines (2000), noise (generated/vendor/lockfiles) and PRs
# over the shard threshold. Symlinks are never reviewed. --dry-run prints the summary without
# writing anything.
# Exit: 0 ok | 2 usage | 3 code on disk is not the code under review | 4 empty diff | 5 not a git repo.

set -u
LC_ALL=C; export LC_ALL  # byte-wise sort/awk, identical on every machine

scope=unstaged base='' range='' pr='' id='' reviews=.reviews dry=0 files='' excl='' depth=diff
SHARD_FILES=30 SHARD_LINES=2500 BIN_FILES=15 BIN_LINES=1200 MAX_FILE_DIFF=800 MAX_FILE_LINES=2000
# Full-depth shards weigh whole files, not changed lines.
SET_SHARD_FILES=30 SET_SHARD_LINES=6000 SET_BIN_FILES=20 SET_BIN_LINES=4000
MAX_REF_SYMBOLS=200 MAX_REFS=25

die() { code=$1; shift; echo "intake: $*" >&2; exit "$code"; }

while [ $# -gt 0 ]; do
  case $1 in
    --id|--base|--range|--pr|--exclude|--depth|--max-file-diff|--max-file-lines|--reviews-dir)
      [ $# -ge 2 ] || die 2 "$1 needs a value" ;;
  esac
  case $1 in
    --id) id=${2-}; shift 2 ;;
    --base) scope=base base=${2-}; shift 2 ;;
    --range) scope=range range=${2-}; shift 2 ;;
    --pr) scope=pr pr=${2-}; shift 2 ;;
    --staged) scope=staged; shift ;;
    --files) scope=files; shift; while [ $# -gt 0 ] && [ "${1#--}" = "$1" ]; do files="$files$1
"; shift; done ;;
    --exclude) excl="$excl${2%/}
"; shift 2 ;;
    --depth) depth=${2-}; shift 2 ;;
    --max-file-diff) MAX_FILE_DIFF=${2-}; shift 2 ;;
    --max-file-lines) MAX_FILE_LINES=${2-}; shift 2 ;;
    --reviews-dir) reviews=${2-}; shift 2 ;;
    --dry-run) dry=1; shift ;;
    -h|--help) sed -n '2,16p' "$0"; exit 0 ;;
    *) die 2 "unknown argument: $1" ;;
  esac
done
case $depth in diff|files|packages|project) ;; *) die 2 "--depth must be diff, files, packages, or project" ;; esac

top=$(git rev-parse --show-toplevel 2>/dev/null) || die 5 "not inside a git repository"
cd "$top" || die 5 "cannot enter $top"
head=$(git rev-parse HEAD 2>/dev/null) || die 5 "repository has no commits"

tmp=$(mktemp -d) || die 1 "mktemp failed"
trap 'rm -rf "$tmp"' EXIT
ctx=$tmp/context
mkdir -p "$ctx"
patch=$ctx/diff.patch

# --- scope -> diff.patch + notes ------------------------------------------------
git_raw() { git -c core.quotePath=false diff --no-color --no-ext-diff -U8 "$@"; }
# git_diff ARGS: git_raw ARGS -- . :(exclude)<each --exclude>
git_diff() {
  if [ -n "$excl" ]; then
    set -- "$@" -- .
    while IFS= read -r x; do [ -n "$x" ] && set -- "$@" ":(exclude)$x"; done <<EOF
$excl
EOF
  fi
  git_raw "$@"
}
# excluded PATH: true when PATH falls under an --exclude entry
excluded() {
  printf '%s' "$excl" | {
    while IFS= read -r x; do
      case $1 in "$x"|"$x"/*) exit 0 ;; esac
    done
    exit 1
  }
}

# basesha: the commit the diff is taken against (apidiff); empty when the PR
# base is not available locally.
basesha=$head
case $scope in
  unstaged)
    git_diff HEAD >"$patch"
    git -c core.quotePath=false ls-files --others --exclude-standard | while IFS= read -r f; do
      [ -n "$excl" ] && excluded "$f" && continue
      git_raw --no-index -- /dev/null "$f"
    done >>"$patch"
    desc="uncommitted changes (staged + unstaged) + untracked files"
    ;;
  staged)
    git_diff --cached >"$patch"
    desc="staged changes"
    ;;
  base)
    git rev-parse --verify -q "$base^{commit}" >/dev/null || die 2 "unknown base ref: $base"
    basesha=$(git merge-base "$base" HEAD) || die 2 "no merge base between $base and HEAD"
    git_diff "$base...HEAD" >"$patch"
    git log --no-color --format='%h %s%n%b' "$base..HEAD" >"$ctx/commits.txt"
    desc="$base...HEAD"
    ;;
  range)
    a=${range%%..*} b=${range##*..}
    [ -n "$a" ] && [ -n "$b" ] && [ "$a" != "$range" ] || die 2 "--range must be A..B"
    bsha=$(git rev-parse --verify -q "$b^{commit}") || die 2 "unknown ref: $b"
    [ "$bsha" = "$head" ] || die 3 "range end $b is not HEAD; check out $b so checks run on the reviewed code"
    basesha=$(git rev-parse --verify -q "$a^{commit}") || die 2 "unknown ref: $a"
    git_diff "$range" >"$patch"
    git log --no-color --format='%h %s%n%b' "$range" >"$ctx/commits.txt"
    desc="$range"
    ;;
  pr)
    command -v gh >/dev/null 2>&1 || die 2 "--pr needs the gh CLI"
    meta=$(gh pr view "$pr" --json headRefOid,baseRefOid,url --jq '.headRefOid+" "+.baseRefOid+" "+.url') ||
      die 2 "gh pr view $pr failed"
    # shellcheck disable=SC2086 # split "head base url"
    set -- $meta
    [ "$1" = "$head" ] || die 3 "HEAD is not PR #$pr head ($1); run 'gh pr checkout $pr' first"
    if git cat-file -e "$2^{commit}" 2>/dev/null; then
      mb=$(git merge-base "$2" HEAD) && git_diff "$mb...HEAD" >"$patch" &&
        git log --no-color --format='%h %s%n%b' "$mb..HEAD" >"$ctx/commits.txt"
      basesha=${mb-}
    else
      [ -z "$excl" ] || die 2 "--exclude needs the PR base commit locally (git fetch it first)"
      basesha=''
      gh pr diff "$pr" --color=never >"$patch"
    fi
    gh pr view "$pr" --json title,body,url --jq '"# "+.title+"\n\n"+.url+"\n\n"+.body' >"$ctx/pr.md" 2>/dev/null
    desc="PR #$pr ($3)"
    ;;
  files)
    [ -n "$files" ] || die 2 "--files needs at least one path"
    printf '%s' "$files" | while IFS= read -r f; do
      [ -n "$f" ] || continue
      if git ls-files --error-unmatch -- "$f" >/dev/null 2>&1; then git_raw HEAD -- "$f"
      elif [ -e "$f" ]; then git_raw --no-index -- /dev/null "$f"
      fi
    done >"$patch"
    desc="files: $(printf '%s' "$files" | tr '\n' ' ' | sed 's/ $//')"
    ;;
esac

[ -s "$patch" ] || die 4 "no changes in scope ($desc)"

# --- issue id ---------------------------------------------------------------------
[ -n "$id" ] || id=$(git symbolic-ref --short -q HEAD) || die 2 "detached HEAD: pass --id"
slug=$(printf '%s' "$id" | tr '[:upper:]' '[:lower:]' | sed -e 's/[^a-z0-9._-]/-/g' -e 's/--*/-/g' -e 's/^[-.]*//' -e 's/-*$//')
[ -n "$slug" ] && [ "$slug" != . ] && [ "$slug" != .. ] || die 2 "issue id '$id' slugifies to nothing"

last=0
for d in "$reviews/$slug"/*/; do
  d=${d%/}; d=${d##*/}
  case $d in ''|*[!0-9]*) continue ;; esac
  d=${d#"${d%%[!0]*}"}  # strip leading zeros: 08 is not octal
  [ "${d:-0}" -gt "$last" ] && last=$d
done
nn=$(printf '%03d' $((last + 1)))
run=$reviews/$slug/$nn

# lang/kind/layer of a repo-relative path; shared by files.tsv and review-set.tsv.
classify_awk='
  function lang(p,  b) {
    b = p; sub(/.*\//, "", b); b = tolower(b)
    if (b ~ /\.go$/) return "go"
    if (b ~ /\.(ts|tsx|mts|cts)$/) return "ts"
    if (b ~ /\.(js|jsx|mjs|cjs)$/) return "js"
    if (b ~ /\.(scss|sass|css|less)$/) return "css"
    if (b ~ /\.rego$/) return "rego"
    if (b ~ /\.proto$/) return "proto"
    if (b ~ /\.(ya?ml|tpl)$/) return "yaml"
    if (b ~ /\.(sh|bash|zsh)$/) return "sh"
    if (b ~ /\.(ps1|psm1)$/) return "ps1"
    if (b ~ /\.py$/) return "py"
    if (b ~ /\.(md|markdown|rst|txt|adoc)$/) return "doc"
    if (b ~ /^go\.(mod|sum|work)$|^package(-lock)?\.json$|^(yarn\.lock|pnpm-lock\.yaml|cargo\.lock)$/) return "deps"
    if (b ~ /\.(json|toml|ini|conf|cfg|env)$/) return "config"
    if (b ~ /^(makefile|.*\.mk)$/) return "make"
    if (b ~ /^dockerfile|\.dockerfile$|^containerfile/) return "docker"
    return "other"
  }
  function kind(p, l) {
    if (p ~ /(^|\/)(vendor|node_modules|third_party)\//) return "vendor"
    if (p ~ /(\.pb\.go|_gen\.go|\.gen\.go|zz_generated[^\/]*|\.pb\.[a-z]+|\.min\.js|\.lock)$/ || p ~ /(^|\/)(dist|gen|generated)\// || p ~ /(^|\/)(go\.sum|package-lock\.json|yarn\.lock|pnpm-lock\.yaml|Cargo\.lock|composer\.lock|Gemfile\.lock|poetry\.lock|gomod2nix\.toml)$/) return "generated"
    # Kronk outputs: make kronk-docs (manual, SDK, examples, CLI reference) and the embedded BUI bundle
    if (p ~ /^cmd\/server\/api\/services\/kronk\/static\// || p ~ /^cmd\/server\/api\/frontends\/bui\/src\/components\/(DocsManual|DocsSDK(Kronk|Model|Pool|Bucky|BuckyModel|Malina|MalinaModel|Examples))\.tsx$/ || p ~ /^cmd\/server\/api\/frontends\/bui\/src\/components\/cli-reference\.json$/) return "generated"
    if (p ~ /(_test\.go|\.(test|spec)\.[jt]sx?|_test\.py)$/ || p ~ /(^|\/)(tests?|testdata|__tests__|e2e)\//) return "test"
    if (l == "doc") return "doc"
    if (l == "config" || l == "yaml" || l == "deps" || l == "make" || l == "docker") return "config"
    return "prod"
  }
  # Kronk owners (.manual/chapter-20-developer-guide.md §20.3); first match wins.
  function layer(p) {
    if (p ~ /^cmd\/kronk\//) return "cli"
    if (p ~ /^cmd\/server\/foundation\//) return "foundation"
    if (p ~ /^cmd\/server\/app\/sdk\//) return "app-sdk"
    if (p ~ /^cmd\/server\/app\//) return "app"
    if (p ~ /^cmd\/server\/api\/frontends\/bui\//) return "bui"
    if (p ~ /^cmd\/server\/api\/tooling\//) return "tooling"
    if (p ~ /^cmd\/server\//) return "service"
    if (p ~ /^sdk\/kronk\/model\//) return "sdk-model"
    if (p ~ /^sdk\/kronk\/parsers\//) return "sdk-parsers"
    if (p ~ /^sdk\/(kronk\/|bucky\/|malina\/)?pool\//) return "sdk-pool"
    if (p ~ /^sdk\/kronk\//) return "sdk-kronk"
    if (p ~ /^sdk\/bucky\//) return "sdk-bucky"
    if (p ~ /^sdk\/malina\//) return "sdk-malina"
    if (p ~ /^sdk\/tools\//) return "sdk-tools"
    if (p ~ /^sdk\//) return "sdk"
    if (p ~ /^examples\//) return "examples"
    if (p ~ /^\.manual\//) return "manual"
    if (p ~ /^zarf\//) return "deploy"
    if (p ~ /^\.github\//) return "ci"
    return "-"
  }
'

# --- per-file inventory: files.tsv ----------------------------------------------
# status added deleted lang kind layer path
awk "$classify_awk"'
  # one row per diff section: sec status added deleted lang kind layer path bytes
  function flush() {
    if (sec == 0) return
    if (path == "") path = gitpath
    if (path == "") return
    l = lang(path)
    printf "%d\t%s\t%d\t%d\t%s\t%s\t%s\t%s\t%d\n", sec, st, add, del, l, lnk ? "symlink" : kind(path, l), layer(path), path, bytes
  }
  { bytes += length($0) + 1 }
  /^diff --git / {
    bytes -= length($0) + 1; flush()
    sec++; st = "M"; add = del = lnk = 0; path = ""; inh = 1; bytes = length($0) + 1
    # fallback for header-only sections (mode change): "diff --git a/P b/P"
    g = substr($0, 14); n = (length(g) - 3) / 2
    gitpath = (n == int(n) && substr(g, 1, n) == substr(g, n + 4)) ? substr(g, 1, n) : ""
    next
  }
  # mode 120000 = symlink (new, deleted, mode change, or "index a..b 120000")
  inh && / 120000$/ && /^(new file mode|deleted file mode|new mode|old mode|index [0-9a-f]+\.\.[0-9a-f]+) / { lnk = 1 }
  inh && /^new file mode/ { st = "A"; next }
  inh && /^deleted file mode/ { st = "D"; next }
  inh && /^rename to / { st = "R"; path = substr($0, 11); next }
  inh && /^Binary files / { if (path == "") { p = $0; sub(/^Binary files .* and (b\/)?/, "", p); sub(/ differ$/, "", p); if (p == "/dev/null") { p = $0; sub(/^Binary files (a\/)?/, "", p); sub(/ and .*/, "", p) } path = p } next }
  inh && /^--- / { if ($0 != "--- /dev/null") { p = substr($0, 5); sub(/^a\//, "", p); sub(/\t.*/, "", p); if (path == "") path = p } next }
  inh && /^\+\+\+ / { if ($0 != "+++ /dev/null") { p = substr($0, 5); sub(/^b\//, "", p); sub(/\t.*/, "", p); path = p } inh = 0; next }
  /^@@/ { inh = 0; next }
  !inh && /^\+/ { add++; next }
  !inh && /^-/ { del++; next }
  END { flush() }
' "$patch" >"$tmp/all.raw"

# Re-classify symlinks on disk (an untracked one has no mode in its diff) and generated
# files the path rules miss: "Code generated ... DO NOT EDIT", linguist-generated in
# .gitattributes, or minified/bundled output (a line > 1000 chars). Symlinks are never
# reviewed: the target can be outside the repo (~/.aws/credentials).
tab=$(printf '\t')
while IFS="$tab" read -r sec st a d l k ly p by; do
  [ -L "$p" ] && k=symlink
  if [ "$k" != generated ] && [ "$k" != vendor ] && [ "$k" != symlink ] && [ "$st" != D ] && [ -f "$p" ]; then
    case $(git check-attr linguist-generated -- "$p" 2>/dev/null) in *": set"|*": true") k=generated ;; esac
    if [ "$k" != generated ]; then
      case $l in
        go|proto|ts|js|py) head -n 5 -- "$p" | grep -q 'Code generated .*DO NOT EDIT' && k=generated ;;
      esac
      case $l in
        js|ts|css) awk 'length($0) > 1000 { f = 1; exit } END { exit !f }' "$p" && k=generated ;;
      esac
    fi
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$sec" "$st" "$a" "$d" "$l" "$k" "$ly" "$p" "$by"
done <"$tmp/all.raw" >"$tmp/all.tsv"

# Generated and vendored files and symlinks are never reviewed: move their diff sections out
# of diff.patch into generated.patch (secret scanning only) and list them in skipped.tsv.
: >"$ctx/skipped.tsv"
awk -F '\t' -v OFS='\t' -v SK="$ctx/skipped.tsv" '
  { row = $2 OFS $3 OFS $4 OFS $5 OFS $6 OFS $7 OFS $8 }
  $6 == "generated" || $6 == "vendor" || $6 == "symlink" { print row > SK; next }
  { print row }' "$tmp/all.tsv" >"$ctx/files.tsv"
if [ -s "$ctx/skipped.tsv" ]; then
  awk -F '\t' -v GEN="$ctx/generated.patch" '
    FNR == NR { if ($6 == "generated" || $6 == "vendor" || $6 == "symlink") drop[$1] = 1; next }
    /^diff --git / { sec++ }
    { if (sec in drop) print > GEN; else print }' "$tmp/all.tsv" "$patch" >"$tmp/kept.patch" &&
    mv "$tmp/kept.patch" "$patch"
fi
[ -s "$patch" ] || die 4 "only generated/vendored files or symlinks changed in scope ($desc); nothing to review"

# --- routing signals + changed symbols ------------------------------------------
# signals.txt: key=value. symbols.tsv: status kind name path line
awk -v SIG="$ctx/signals.txt" -v SYM="$ctx/symbols.tsv" '
  BEGIN { FS = "\t" }
  FNR == NR { L[$7] = $4; K[$7] = $5; next }
  function gosym(s,  r, n) {
    if (s ~ /^func \(/) {
      r = s; sub(/^func \([^)]*[ *]/, "", r); sub(/\).*/, "", r); sub(/\[.*/, "", r)
      n = s; sub(/^func \([^)]*\) */, "", n); sub(/[[(].*/, "", n)
      return (r != "" && n != "") ? "method\t" r "." n : ""
    }
    if (s ~ /^func [A-Za-z_]/) { n = s; sub(/^func /, "", n); sub(/[[(].*/, "", n); return "func\t" n }
    if (s ~ /^type [A-Za-z_]/) { n = s; sub(/^type /, "", n); sub(/[ [].*/, "", n); return "type\t" n }
    return ""
  }
  function jssym(s,  n) {
    n = s; sub(/^export (default )?/, "", n); sub(/^async /, "", n)
    if (n ~ /^function\*? *[A-Za-z_$]/) { sub(/^function\*? */, "", n); sub(/[^A-Za-z0-9_$].*/, "", n); return "func\t" n }
    if (n ~ /^(class|interface|type|enum) [A-Za-z_$]/) { sub(/^[a-z]+ /, "", n); sub(/[^A-Za-z0-9_$].*/, "", n); return "type\t" n }
    if (s ~ /^export (const|let) [A-Za-z_$]/) { sub(/^(const|let) /, "", n); sub(/[^A-Za-z0-9_$].*/, "", n); return "const\t" n }
    return ""
  }
  function sym(s, l) { return l == "go" ? gosym(s) : (l == "ts" || l == "js") ? jssym(s) : "" }
  function note(tab, key, at) { if (key == "") return; if (!((path SUBSEP key) in tab)) tab[path SUBSEP key] = at }
  /^diff --git / { path = ""; inh = 1; next }
  inh && /^\+\+\+ / { if ($0 != "+++ /dev/null") { path = substr($0, 5); sub(/^b\//, "", path); sub(/\t.*/, "", path) } inh = 0; next }
  inh && /^--- / { if ($0 != "--- /dev/null" && path == "") { path = substr($0, 5); sub(/^a\//, "", path); sub(/\t.*/, "", path) } next }
  inh { next }
  /^@@/ {
    h = $0; o = h; sub(/^@@ -/, "", o); sub(/[ ,].*/, "", o); oldln = o - 1
    n = h; sub(/^@@ -[^ ]* \+/, "", n); sub(/[ ,].*/, "", n); newln = n - 1
    ctxs = h; sub(/^@@ [^@]*@@ ?/, "", ctxs); hk = sym(ctxs, L[path]); hunkat = newln + 1
    next
  }
  path == "" { next }
  /^[+-]/ {
    c = substr($0, 1, 1); s = substr($0, 2); l = L[path]; k = K[path]
    if (c == "+") newln++; else oldln++
    if (k == "generated" || k == "vendor") next
    if (k == "prod") { if (c == "+") prodadd++; prodf[path] = 1 }
    if (k == "test") testf[path] = 1
    if (k == "doc") comments = 1
    t = s; sub(/^[ \t]+/, "", t)
    if (l == "go") {
      if (t ~ /(^|[^A-Za-z0-9_])err([^A-Za-z0-9_]|$)|errors\.(Is|As|New|Join)|panic\(|recover\(|^defer |ctx\.Done|context\.With/) failure = 1
      if (t ~ /^type [A-Za-z_]|struct \{|interface \{|^func New|^func \([^)]*\) [A-Z]|^func [A-Z]/) contract = 1
      if (t ~ /^\/\/|^\/\*/) comments = 1
    } else if (l == "ts" || l == "js") {
      if (t ~ /catch|\.then\(|await |fetch\(|reject|throw /) failure = 1
      if (t ~ /^(export )?(interface|type) [A-Za-z_]|^export (class|function|const)/) contract = 1
      if (t ~ /^\/\/|^\/\*|^\*/) comments = 1
    } else if (l == "proto") {
      if (t ~ /^(message|service|rpc|enum|oneof) |= [0-9]+;/) contract = 1
      if (t ~ /^\/\//) comments = 1
    } else if (l == "rego") {
      failure = 1; if (t ~ /^#/) comments = 1
    } else if (l == "sh" || l == "ps1") {
      if (t ~ /set -e|exit |\|\||ErrorAction|try \{|trap /) failure = 1
      if (t ~ /^#/) comments = 1
    } else if (l == "yaml" || l == "py" || l == "make" || l == "docker") {
      if (t ~ /^#/) comments = 1
    }
    if (t ~ /TODO|FIXME|XXX/) comments = 1
    sk = sym(t, l)
    if (sk != "") {
      if (c == "+") note(ADD, sk, newln); else note(DEL, sk, oldln)
    } else if (hk != "") note(MOD, hk, hunkat)
    next
  }
  /^ / { oldln++; newln++; next }
  END {
    for (x in prodf) np++
    for (x in testf) nt++
    printf "prod_files=%d\ntest_files=%d\nprod_lines_added=%d\nfailure=%d\ncontract=%d\ncomments=%d\nlogic=%d\n",
      np, nt, prodadd, failure, contract, comments, (prodadd >= 20 || nt > 0) > SIG
    for (x in ADD) { split(x, p, SUBSEP); st = (x in DEL) ? "modified" : "added"; print st "\t" p[2] "\t" p[1] "\t" ADD[x] > SYM }
    for (x in DEL) if (!(x in ADD)) { split(x, p, SUBSEP); print "removed\t" p[2] "\t" p[1] "\t" DEL[x] > SYM }
    for (x in MOD) if (!(x in ADD) && !(x in DEL)) { split(x, p, SUBSEP); print "modified\t" p[2] "\t" p[1] "\t" MOD[x] > SYM }
  }
' "$ctx/files.tsv" "$patch"
# symbols.tsv columns: status kind name path line  (sorted by path, line)
if [ -s "$ctx/symbols.tsv" ]; then
  awk 'BEGIN { FS = OFS = "\t" } { print $1, $2, $3, $4, $5 }' "$ctx/symbols.tsv" | sort -t "$(printf '\t')" -k4,4 -k5,5n >"$ctx/symbols.sorted" &&
    mv "$ctx/symbols.sorted" "$ctx/symbols.tsv"
else
  : >"$ctx/symbols.tsv"
fi

# --- references to changed symbols: refs.tsv = symbol path:line ------------------
# Best effort with rg (word match, capped); reviewers refine with gopls. Skipped files excluded.
: >"$ctx/refs.tsv"
if command -v rg >/dev/null 2>&1 && [ -s "$ctx/symbols.tsv" ]; then
  head -n "$MAX_REF_SYMBOLS" "$ctx/symbols.tsv" | while IFS="$tab" read -r _ _ sym path line; do
    name=${sym##*.}
    case $path in *.go) glob='*.go' ;; *) glob='*.{ts,tsx,js,jsx,mjs,cjs}' ;; esac
    rg -n -w --no-heading --sort path -g "$glob" -g '!vendor/**' -g '!node_modules/**' -g '!.reviews/**' -- "$name" . 2>/dev/null |
      awk -F: -v s="$sym" -v dp="$path" -v dl="$line" -v max="$MAX_REFS" -v sk="$ctx/skipped.tsv" -v nm="$name" '
        BEGIN { while ((getline l < sk) > 0) { split(l, f, "\t"); skip[f[7]] = 1 } }
        { p = $1; sub(/^\.\//, "", p); if ((p in skip) || (p == dp && $2 == dl)) next
          t = $0; sub(/^[^:]*:[^:]*:/, "", t)   # skip Go definitions of the symbol itself
          if (p ~ /\.go$/ && t ~ ("^[ \t]*(func (\\([^)]*\\) )?" nm "[[(]|type " nm "[ [])")) next
          n++; if (n <= max) print s "\t" p ":" $2 }
        END { if (n > max) print s "\t+" (n - max) " more" }'
  done >"$ctx/refs.tsv"
fi

# --- review set (full depth): review-set.tsv ----------------------------------------
# status added deleted lang kind layer path lines — the files.tsv columns plus the file's
# line count; status is "-" for files the diff doesn't touch. The same content rules as for
# changed files drop generated files; binary, empty, excluded, and .reviews files are left out.
review_files=0 review_lines=0 tok_set=0
: >"$ctx/review-set.tsv"
if [ "$depth" != diff ]; then
  printf '%s' "$excl" >"$tmp/excl"
  # shellcheck disable=SC2016 # awk code
  dirof='{ d = $0; if (d !~ /\//) d = "."; else sub(/\/[^\/]*$/, "", d) }'
  awk -F '\t' '$1 != "D" { print $7 }' "$ctx/files.tsv" >"$tmp/set.changed"
  case $depth in
    files) cat "$tmp/set.changed" ;;
    packages)
      awk "$dirof"'{ print d }' "$tmp/set.changed" | sort -u >"$tmp/set.dirs"
      git -c core.quotePath=false ls-files --cached --others --exclude-standard |
        awk "$dirof"'FILENAME != "-" { D[$0] = 1; next } d in D' "$tmp/set.dirs" - ;;
    project) git -c core.quotePath=false ls-files --cached --others --exclude-standard ;;
  esac | sort -u | awk -F '\t' -v OFS='\t' -v RD="${reviews%/}/" "$classify_awk"'
    FILENAME != "-" { if ($0 != "") X[$0] = 1; next }
    $0 == "" || index($0, RD) == 1 || $0 ~ /(^|\/)\.reviews\// { next }
    { for (x in X) if ($0 == x || index($0, x "/") == 1) next
      l = lang($0); k = kind($0, l)
      if (k != "generated" && k != "vendor") print l, k, layer($0), $0 }' "$tmp/excl" - >"$tmp/set.raw"
  # Regular files only: a symlink can point outside the repo (~/.aws/credentials) and would
  # put that file in front of every reviewer. Shell builtins only, so no fork per file.
  while IFS="$tab" read -r l k ly p; do
    [ -L "$p" ] || [ ! -f "$p" ] || printf '%s\t%s\t%s\t%s\n' "$l" "$k" "$ly" "$p"
  done <"$tmp/set.raw" >"$tmp/set.cls"
  # Batched over all candidates (xargs): per-file forks are too slow for a whole repo. Paths
  # get a ./ prefix so awk never reads one as a var=value assignment.
  # files0 LANG_RE: "./path" of the set.cls rows whose lang matches, NUL-separated
  files0() { awk -F '\t' -v re="$1" '$1 ~ re { print "./" $4 }' "$tmp/set.cls" | tr '\n' '\0'; }
  cut -f4 "$tmp/set.cls" | git check-attr --stdin linguist-generated 2>/dev/null |
    sed -n -e 's/: linguist-generated: set$//p' -e 's/: linguist-generated: true$//p' |
    sed 's|^|./|' >"$tmp/set.skip"
  # shellcheck disable=SC2016 # awk programs run through xargs
  files0 '^(go|proto|ts|js|py)$' |
    xargs -0 awk 'FNR > 5 { nextfile } /Code generated .*DO NOT EDIT/ { print FILENAME; nextfile }' >>"$tmp/set.skip" 2>/dev/null
  # shellcheck disable=SC2016
  files0 '^(js|ts|css)$' |
    xargs -0 awk 'length($0) > 1000 { print FILENAME; nextfile }' >>"$tmp/set.skip" 2>/dev/null
  # Text files only: grep -I lists a file when it is non-empty and not binary.
  # shellcheck disable=SC2016
  files0 . | xargs -0 grep -Il . -- 2>/dev/null | tr '\n' '\0' |
    xargs -0 awk '{ l[FILENAME]++; b[FILENAME] += length($0) + 1 }
      END { for (f in l) print f "\t" l[f] "\t" b[f] }' >"$tmp/set.size" 2>/dev/null
  awk -F '\t' -v OFS='\t' -v SK="$tmp/set.skip" -v SZ="$tmp/set.size" -v FT="$ctx/files.tsv" -v TOT="$tmp/set.tot" '
    BEGIN { while ((getline l < SK) > 0) skip[substr(l, 3)] = 1
            while ((getline l < SZ) > 0) { split(l, f, "\t"); p = substr(f[1], 3); ln[p] = f[2]; by[p] = f[3] }
            while ((getline l < FT) > 0) { split(l, f, "\t"); ch[f[7]] = f[1] OFS f[2] OFS f[3] } }
    ($4 in skip) || !($4 in ln) { next }
    { print (($4 in ch) ? ch[$4] : "-" OFS 0 OFS 0), $1, $2, $3, $4, ln[$4]; n++; nl += ln[$4]; b += by[$4] }
    END { print n + 0, nl + 0, int((b + 3) / 4) > TOT }' "$tmp/set.cls" >"$ctx/review-set.tsv"
  read -r review_files review_lines tok_set <"$tmp/set.tot"
fi

# --- totals + shards ------------------------------------------------------------
# shellcheck disable=SC2046 # split the five totals
set -- $(awk -F '\t' '$5 != "generated" && $5 != "vendor" { n++; a += $2; d += $3 }
  $5 != "generated" && $5 != "vendor" && $5 != "doc" { cn++; cl += $2 + $3 }
  END { print n + 0, a + 0, d + 0, cn + 0, cl + 0 }' "$ctx/files.tsv")
reviewable=$1 added=$2 deleted=$3 codefiles=$4 codelines=$5
total=$(wc -l <"$tmp/all.tsv" | tr -d ' ')
skipped=$(wc -l <"$ctx/skipped.tsv" | tr -d ' ')
# Code files only (docs aren't sharded), grouped by directory. Diff depth weighs changed
# lines; full depth weighs the review set's whole files, plus deleted files' removed lines.
# shellcheck disable=SC2046 # split the two totals
set -- $(awk -F '\t' '$5 != "doc" { n++; l += $8 } END { print n + 0, l + 0 }' "$ctx/review-set.tsv")
setcodefiles=$1 setcodelines=$2
shards=0 bigset=0
if [ "$depth" = diff ]; then
  bf=$BIN_FILES bl=$BIN_LINES
  { [ "$codefiles" -gt "$SHARD_FILES" ] || [ "$codelines" -gt "$SHARD_LINES" ]; } && shards=1
  awk -F '\t' '$5 != "doc" { print $7 "\t" $2 + $3 }' "$ctx/files.tsv" >"$tmp/shard.in"
else
  bf=$SET_BIN_FILES bl=$SET_BIN_LINES
  { [ "$setcodefiles" -gt "$SET_SHARD_FILES" ] || [ "$setcodelines" -gt "$SET_SHARD_LINES" ]; } && shards=1 bigset=1
  { awk -F '\t' '$5 != "doc" { print $7 "\t" $8 }' "$ctx/review-set.tsv"
    awk -F '\t' '$1 == "D" && $5 != "doc" { print $7 "\t" $3 }' "$ctx/files.tsv"; } >"$tmp/shard.in"
fi
if [ "$shards" = 1 ]; then
  awk -F '\t' '{ d = $1; if (d !~ /\//) d = "."; else sub(/\/[^\/]*$/, "", d); print d "\t" $2 "\t" $1 }' "$tmp/shard.in" |
    sort -t "$tab" -k1,1 -k3,3 | awk -F '\t' -v BF="$bf" -v BL="$bl" '
    BEGIN { s = 1 }
    $1 != dir && (nf >= BF || nl >= BL) { s++; nf = nl = 0 }
    { dir = $1; nf++; nl += $2; print s "\t" $3 }
  ' >"$ctx/shards.tsv"
  shards=$(awk -F '\t' 'END { print $1 + 0 }' "$ctx/shards.tsv")
  [ "$shards" -gt 1 ] || { shards=0; rm -f "$ctx/shards.tsv"; }
fi

# --- size guard: size.tsv = flag path metric est_tokens suggestion -------------------
# Token estimate: bytes / 4. Flags: big-pr, big-review-set, big-diff, huge-file, noise (skipped files).
awk -F '\t' '$1 != "D" { print $7 }' "$ctx/files.tsv" |
  while IFS= read -r f; do
    [ -f "$f" ] && printf '%s\t%s\t%s\n' "$f" "$(wc -l <"$f" | tr -d ' ')" "$(wc -c <"$f" | tr -d ' ')"
  done >"$tmp/filesize.tsv"
# all.tsv: sec status added deleted lang kind layer path bytes
awk -F '\t' -v OFS='\t' -v MD="$MAX_FILE_DIFF" -v ML="$MAX_FILE_LINES" -v SF="$SHARD_FILES" -v SL="$SHARD_LINES" \
    -v CF="$codefiles" -v CL="$codelines" -v NS="$shards" -v DEPTH="$depth" -v TOT="$tmp/totals" -v OUT="$tmp/size.raw" '
  FNR == NR { fl[$1] = $2; fb[$1] = $3; next }
  {
    p = $8; n = $3 + $4; t = int(($9 + 3) / 4); b = p; sub(/.*\//, "", b)
    if ($6 == "generated" || $6 == "vendor" || $6 == "symlink") {
      noise += t
      lock = (b ~ /^(go\.sum|package-lock\.json|yarn\.lock|pnpm-lock\.yaml|Cargo\.lock|composer\.lock|Gemfile\.lock|poetry\.lock|gomod2nix\.toml)$/)
      print "noise", p, $6 (lock ? "/lockfile" : "") " " n " lines", t, ($6 == "symlink" ? "skipped: symlinks are never reviewed (the target can be outside the repo)" : "skipped: generated/vendored files are never reviewed (Kronk commits regenerated outputs with their sources)") > OUT
      next
    }
    review += t
    if ($6 != "doc" && n > MD) print "big-diff", p, n " changed lines", t, "split the change, or review it alone (--files) / --exclude it" > OUT
    if ((p in fl) && fl[p] > ML) print "huge-file", p, fl[p] " lines in file", int((fb[p] + 3) / 4), "agents read it for context every time; split the file" > OUT
  }
  END {
    if (CF > SF || CL > SL)
      print "big-pr", "-", CF " code files, " CL " changed lines", review + 0, (DEPTH == "diff" ? "review shards into " NS " parts per heavy lens; split the PR along shards.tsv" : "split the PR into smaller changes") > OUT
    print review + 0, noise + 0 > TOT
  }
' "$tmp/filesize.tsv" "$tmp/all.tsv"
if [ "$bigset" = 1 ]; then
  printf 'big-review-set\t-\t%s code files, %s lines at depth %s\t%s\t%s\n' "$setcodefiles" "$setcodelines" "$depth" "$tok_set" \
    "review shards into $shards parts per heavy lens; pick a narrower depth or --exclude paths" >>"$tmp/size.raw"
fi
: >"$ctx/size.tsv"
for flag in big-pr big-review-set big-diff huge-file noise; do
  [ -f "$tmp/size.raw" ] && awk -F '\t' -v f="$flag" '$1 == f' "$tmp/size.raw" | sort -t "$tab" -k4,4nr -k2,2 >>"$ctx/size.tsv"
done
read -r tok_review tok_noise <"$tmp/totals"

# --- Go packages grouped by module ----------------------------------------------
# The packages of the changed Go files, or of the review set's at full depth. Directories
# the go tool ignores (testdata, _x, .x) are left out.
# scoped: the file listing what is reviewed — files.tsv, or review-set.tsv at full depth.
scoped=$ctx/files.tsv
[ "$depth" = diff ] || scoped=$ctx/review-set.tsv
awk -F '\t' '$4 == "go" && $1 != "D" && $5 != "vendor" { d = $7; if (d !~ /\//) d = "."; else sub(/\/[^\/]*$/, "", d)
    if (d !~ /(^|\/)(testdata|[_.][^\/]*)(\/|$)/ || d == ".") print d }' "$scoped" |
  sort -u | while IFS= read -r d; do
    [ -d "$d" ] || continue
    m=$d
    while [ ! -f "$m/go.mod" ] && [ "$m" != . ]; do
      case $m in */*) m=${m%/*} ;; *) m=. ;; esac
    done
    [ -f "$m/go.mod" ] || continue
    if [ "$m" = "$d" ]; then rel=.; else rel=./${d#"$m"/}; fi
    printf '%s\t%s\n' "$m" "$rel"
  done >"$ctx/packages.tsv"

# --- applicable repo rules (AGENTS.md / CLAUDE.md on the path of reviewed files) -
git -c core.quotePath=false ls-files 'AGENTS.md' '*/AGENTS.md' 'CLAUDE.md' '*/CLAUDE.md' 2>/dev/null |
  awk -F '\t' 'NR == FNR { p[$7] = 1; next }
    { d = $0; sub(/[^\/]*$/, "", d); if (d == "") { print; next }
      for (f in p) if (index(f, d) == 1) { print; next } }' "$scoped" - >"$ctx/rules.txt"
# Kronk's ownership map and verification rules apply to every change.
[ -f .manual/chapter-20-developer-guide.md ] && echo .manual/chapter-20-developer-guide.md >>"$ctx/rules.txt"

# --- tools ----------------------------------------------------------------------
for t in git gh go gopls staticcheck gosec govulncheck dupl checklocks errcheck nilaway deadcode apidiff \
  gitleaks trufflehog osv-scanner trivy grype jscpd ast-grep rg jq shellcheck node npm npx opa helm \
  python3 hadolint; do
  p=$(command -v "$t" 2>/dev/null) && printf '%s\t%s\n' "$t" "$p"
done >"$ctx/tools.tsv"

# --- missing.tsv: tool needed_for fix ---------------------------------------------
# Tools this change needs but that are absent, or Go tools built with an older Go
# release than the active toolchain (analyzers built that way fail on the newer
# standard library). Checks skip what is missing; the user is told how to install it.
has_tool() { grep -q "^$1	" "$ctx/tools.tsv"; }
live_lang() { awk -F '\t' -v l="$1" '$1 != "D" && $4 == l { f = 1; exit } END { exit !f }' "$scoped"; }
gominor() { sed -n 's/.*go\([0-9][0-9]*\.[0-9][0-9]*\).*/\1/p' | head -n 1; }
gocur=''
has_tool go && gocur=$(go env GOVERSION 2>/dev/null | gominor)
need() {  # need TOOL NEEDED_FOR INSTALL [go]: record TOOL if missing (or stale, for Go tools)
  if ! has_tool "$1"; then
    printf '%s\t%s\t%s\n' "$1" "$2" "$3"
  elif [ "${4-}" = go ] && [ -n "$gocur" ]; then
    built=$(go version "$(command -v "$1")" 2>/dev/null | gominor)
    if [ -n "$built" ] && [ "$built" != "$gocur" ] &&
      [ "$(printf '%s\n%s\n' "$built" "$gocur" | sort -t . -k1,1n -k2,2n | head -n 1)" = "$built" ]; then
      printf '%s\t%s (built with go%s, toolchain is go%s: rebuild)\t%s\n' "$1" "$2" "$built" "$gocur" "$3"
    fi
  fi
}
{
  if [ -s "$ctx/packages.tsv" ]; then
    need go "build, vet, tests" "https://go.dev/dl/"
    if has_tool go && [ "$(go env CGO_ENABLED 2>/dev/null)" != 1 ]; then
      printf 'cgo\tgo test -race (CGO_ENABLED=0)\tinstall a C compiler (clang/gcc) and set CGO_ENABLED=1\n'
    fi
    need gopls "symbol lookups by reviewers (optional, not in CI)" "go install golang.org/x/tools/gopls@latest" go
    need staticcheck "staticcheck -checks=all (CI static job)" "go install honnef.co/go/tools/cmd/staticcheck@master" go
    need gosec "Go security scan (optional, not in CI)" "go install github.com/securego/gosec/v2/cmd/gosec@latest" go
    need govulncheck "Go vulnerability scan" "go install golang.org/x/vuln/cmd/govulncheck@latest" go
    need dupl "Go clone detection (optional, not in CI)" "go install github.com/mibk/dupl@latest" go
    need checklocks "lock annotations (+checklocks) (optional, not in CI)" \
      "go install gvisor.dev/gvisor/tools/checklocks/cmd/checklocks@go" go
    need errcheck "unchecked errors (optional, not in CI)" "go install github.com/kisielk/errcheck@latest" go
    need nilaway "nil dereferences (optional, not in CI)" "go install go.uber.org/nilaway/cmd/nilaway@latest" go
    need deadcode "unreachable functions (optional, not in CI)" "go install golang.org/x/tools/cmd/deadcode@latest" go
    need apidiff "exported API breaks (optional, not in CI)" "go install golang.org/x/exp/cmd/apidiff@latest" go
  fi
  if live_lang sh; then
    need shellcheck "shell lint (optional, not in CI)" "https://github.com/koalaman/shellcheck#installing"
  fi
  if live_lang js || live_lang ts; then
    need jscpd "JS/TS clone detection (optional, not in CI)" "npm install -g jscpd"
  fi
  # BUI type check (bui-tsc) needs the BUI's own dependencies.
  if awk -F '\t' '$1 != "D" && $7 ~ /^cmd\/server\/api\/frontends\/bui\/.*\.tsx?$/ { f = 1; exit } END { exit !f }' "$ctx/files.tsv" &&
    [ ! -d cmd/server/api/frontends/bui/node_modules ]; then
    printf 'bui-deps\tBUI type check (tsc)\tcd cmd/server/api/frontends/bui && npm ci\n'
  fi
  # Helm chart checks (helm-test) run zarf/helm/kronk/test.sh: helm plus python3 with PyYAML.
  if awk -F '\t' '$1 != "D" && $7 ~ /^zarf\/helm\// { f = 1; exit } END { exit !f }' "$ctx/files.tsv"; then
    need helm "Helm chart checks (zarf/helm/kronk/test.sh)" "https://helm.sh/docs/intro/install/"
    if ! python3 -c 'import yaml' >/dev/null 2>&1; then
      printf 'pyyaml\tHelm chart checks (zarf/helm/kronk/test.sh)\tpython3 -m pip install pyyaml\n'
    fi
  fi
  has_tool trufflehog || need gitleaks "secret scan (optional, not in CI)" "go install github.com/zricethezav/gitleaks/v8@latest"
} >"$ctx/missing.tsv"

# --- scope.txt ------------------------------------------------------------------
langs=$(awk -F '\t' '$5 != "generated" && $5 != "vendor" { c[$4]++ } END { for (l in c) printf "%s:%d ", l, c[l] }' "$ctx/files.tsv")
{
  echo "id=$slug"
  echo "nn=$nn"
  echo "run=$run"
  echo "root=$top"
  echo "scope=$desc"
  echo "depth=$depth"
  echo "head=$head"
  echo "base=$basesha"
  echo "branch=$(git symbolic-ref --short -q HEAD || echo detached)"
  echo "files=$total"
  echo "reviewable_files=$reviewable"
  echo "skipped_files=$skipped"
  echo "lines_added=$added"
  echo "lines_deleted=$deleted"
  echo "code_files=$codefiles"
  echo "code_lines_changed=$codelines"
  echo "languages=${langs% }"
  echo "go_packages=$(wc -l <"$ctx/packages.tsv" | tr -d ' ')"
  echo "symbols=$(wc -l <"$ctx/symbols.tsv" | tr -d ' ')"
  echo "refs=$(wc -l <"$ctx/refs.tsv" | tr -d ' ')"
  echo "shards=$shards"
  echo "diff_tokens_est=$tok_review"
  if [ "$depth" != diff ]; then
    echo "review_files=$review_files"
    echo "review_code_files=$setcodefiles"
    echo "review_lines=$review_lines"
    echo "review_tokens_est=$tok_set"
  fi
  echo "noise_tokens_est=$tok_noise"
  echo "size_warnings=$(wc -l <"$ctx/size.tsv" | tr -d ' ')"
  echo "missing_tools=$(wc -l <"$ctx/missing.tsv" | tr -d ' ')"
  cat "$ctx/signals.txt"
} >"$ctx/scope.txt"

if [ "$dry" = 1 ]; then
  echo "# dry run: nothing written"
  cat "$ctx/scope.txt"
  [ -s "$ctx/size.tsv" ] && { echo "# size warnings: flag path metric est_tokens suggestion"; cat "$ctx/size.tsv"; }
[ -s "$ctx/missing.tsv" ] && { echo "# missing tools: tool needed_for install"; cat "$ctx/missing.tsv"; }
  exit 0
fi

mkdir -p "$reviews/$slug" || die 1 "cannot create $reviews/$slug"
mkdir "$run" 2>/dev/null || die 1 "run directory $run already exists"
mkdir -p "$run/lenses" "$run/security-issues/poc" || die 1 "cannot populate $run"
cp -R "$ctx" "$run/context" || die 1 "cannot write $run/context"
cat "$run/context/scope.txt"
[ -s "$ctx/size.tsv" ] && { echo "# size warnings: flag path metric est_tokens suggestion"; cat "$ctx/size.tsv"; }
[ -s "$ctx/missing.tsv" ] && { echo "# missing tools: tool needed_for install"; cat "$ctx/missing.tsv"; }
exit 0
