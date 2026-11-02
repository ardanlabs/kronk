#!/bin/sh
# Usage: sh checks.sh RUN_DIR [--env K=V]... [--exclude PATH]... [--timeout 10m] [--no-tests] [--trust]
# Runs every mechanical check ONCE, scoped to the files/packages intake.sh recorded in
# RUN_DIR/context, in parallel. The depth comes from scope.txt: at diff depth file checks
# cover the changed files; at full depth (files/packages/project) they cover review-set.tsv,
# and Go checks cover packages.tsv, which intake fills to match the depth.
# Output: RUN_DIR/context/checks/<check>.txt (full log) and RUN_DIR/context/checks/summary.tsv
# (check status exit lines file). Prints the summary.
# Kronk rules (AGENTS.md, .github/workflows/linux.yml static + race jobs) always apply:
# GITHUB_WORKSPACE=<repo root>; tests are `go test -race -short` on the reviewed packages only
# (never at project depth: AGENTS.md forbids a full repo test run), with
# KRONK_BASE_PATH=<empty temp store> so model-backed tests skip; go vet covers the whole module;
# packages under DEFAULT_EXCLUDES are never tested; staticcheck runs with -checks=all.
#   --env K=V      exported for every check; repeatable
#   --exclude PATH never run tests in packages under PATH (repo-relative); adds to the defaults
#   --timeout D    go test -timeout (default 10m, as in CI)
#   --no-tests     skip test runs
#   --trust        run the reviewed tree's code even when scope.txt says trusted=no (a fork PR):
#                  go test, zarf/helm/kronk/test.sh, and the BUI's npx tsc. Without it they are
#                  skipped; the other checks only read the code (builds run no repo code).
# A failing check is data for the reviewers, not a script failure. Exit: 0 ran | 2 usage.

set -u

die() { echo "checks: $*" >&2; exit 2; }

[ $# -gt 0 ] || die "RUN_DIR required"
run=$1; shift
# Model-backed or server-backed suites: CI or deliberate human runs only (AGENTS.md, CI race job).
DEFAULT_EXCLUDES='sdk/kronk/tests
sdk/bucky/tests
sdk/kronk/pool
cmd/server/api/services/kronk/tests
'
excludes=$DEFAULT_EXCLUDES timeout=10m tests=1 trust=0 envkeys=''
while [ $# -gt 0 ]; do
  case $1 in
    --env|--exclude|--timeout) [ $# -ge 2 ] || die "$1 needs a value" ;;
  esac
  case $1 in
    --env)
      kv=${2-}; k=${kv%%=*}
      case $k in ''|[0-9]*|*[!A-Za-z0-9_]*) die "bad --env '$kv'" ;; esac
      [ "$k" != "$kv" ] || die "--env needs K=V"
      # shellcheck disable=SC2163 # exports the K=V pair itself
      export "$kv"; envkeys="$envkeys $k"; shift 2 ;;
    --exclude) excludes="$excludes${2%/}
"; shift 2 ;;
    --timeout) timeout=${2-}; shift 2 ;;
    --no-tests) tests=0; shift ;;
    --trust) trust=1; shift ;;
    -h|--help) sed -n '2,21p' "$0"; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

# Every list below (files, packages, modules, excludes) is newline-separated: split on newlines
# only, so paths with spaces stay one argument.
IFS='
'

ctx=$(cd -- "$run/context" 2>/dev/null && pwd) || die "no context in $run (run intake.sh first)"
root=$(sed -n 's/^root=//p' "$ctx/scope.txt")
basesha=$(sed -n 's/^base=//p' "$ctx/scope.txt")
depth=$(sed -n 's/^depth=//p' "$ctx/scope.txt")
# runcode: may checks execute code from the reviewed tree? Not for someone else's fork PR
# (trusted=no) unless --trust; a scope.txt without the key predates the check: trust it.
runcode=1 untrusted_why=''
if [ "$(sed -n 's/^trusted=//p' "$ctx/scope.txt")" = no ] && [ "$trust" = 0 ]; then
  runcode=0 untrusted_why="untrusted code ($(sed -n 's/^trust_reason=//p' "$ctx/scope.txt")); pass --trust / -Trust after reading the diff"
fi
# scoped: the files under review — the changed files, or the review set at full depth
scoped=$ctx/files.tsv
[ "${depth:-diff}" = diff ] || scoped=$ctx/review-set.tsv
cd -- "$root" 2>/dev/null || die "cannot enter repo root $root"
case " $envkeys " in
  *" GITHUB_WORKSPACE "*) ;;  # --env GITHUB_WORKSPACE=... wins
  *) GITHUB_WORKSPACE=$(pwd -P); export GITHUB_WORKSPACE; envkeys="$envkeys GITHUB_WORKSPACE" ;;
esac
# Scratch space outside the repo: build output (checks stay read-only) and an empty model
# store, so in-package model-backed tests skip instead of running inference (they are CI work).
scratch=$(mktemp -d) || die "mktemp failed"
mkdir -p "$scratch/bin" "$scratch/kronk-home"
case " $envkeys " in
  *" KRONK_BASE_PATH "*) ;;  # --env KRONK_BASE_PATH=... wins
  *) KRONK_BASE_PATH=$scratch/kronk-home; export KRONK_BASE_PATH; envkeys="$envkeys KRONK_BASE_PATH" ;;
esac
out=$ctx/checks
mkdir -p "$out" || die "cannot create $out"
has() { grep -q "^$1	" "$ctx/tools.tsv"; }
toolpath() { awk -F '\t' -v t="$1" '$1 == t { print $2; exit }' "$ctx/tools.tsv"; }

{
  echo "env_keys=${envkeys# }"
  printf 'test_excludes=%s\n' "$(printf '%s' "$excludes" | tr '\n' ' ')"
  echo "test_timeout=$timeout"
  echo "tests=$tests"
  echo "runs_reviewed_code=$runcode"
} >"$out/setup.txt"

# job NAME CMD...: run CMD in the background, log to $out/NAME.txt, exit code to NAME.exit.
job() {
  name=$1; shift
  (
    printf '# cmd:'; printf ' %s' "$@"; printf '\n'
    "$@" 2>&1
    echo $? >"$out/$name.exit"
  ) >"$out/$name.txt" 2>&1 &
}
skip() { printf '# skipped: %s\n' "$2" >"$out/$1.txt"; echo skip >"$out/$1.exit"; }

# live LANG: reviewed (not deleted) files of LANG ("*" = any), repo-relative
live() { awk -F '\t' -v l="$1" '$1 != "D" && $5 != "generated" && $5 != "vendor" && (l == "*" || $4 == l) { print $7 }' "$scoped"; }

# --- base tree (apidiff) ---------------------------------------------------------------
# The base commit's files, extracted outside the repo with git archive: nothing is written
# to the working tree or .git. Removed when the script exits.
basesrc='' setcopy=''
trap 'rm -rf "$scratch"; [ -z "$basesrc" ] || rm -rf "$basesrc"; [ -z "$setcopy" ] || rm -rf "$setcopy"' EXIT
if [ -n "$basesha" ] && [ -s "$ctx/packages.tsv" ] && has apidiff; then
  if basesrc=$(mktemp -d); then
    git archive "$basesha" | tar -x -C "$basesrc" || { rm -rf "$basesrc"; basesrc=''; }
  else basesrc=''
  fi
fi

# apidiff_pkgs MODULE PKG...: incompatible exported-API changes of each package against
# the base tree. Runs in the head module directory. New, main, and internal packages skipped.
apidiff_pkgs() {
  m=$1; shift
  bmod=$basesrc/$m
  for p in "$@"; do
    [ -d "$bmod/$p" ] || continue
    ip=$(go list -f '{{if ne .Name "main"}}{{.ImportPath}}{{end}}' "$p" 2>/dev/null)
    case $ip in ''|*/internal|*/internal/*|internal/*) continue ;; esac
    ex=$basesrc/apidiff-$(printf '%s' "$ip" | tr '/' '_').export
    if ! (cd -- "$bmod" && apidiff -w "$ex" "$ip") >/dev/null 2>"$ex.err"; then
      printf '## %s: base export failed\n' "$ip"; sed -n 1,5p "$ex.err"; continue
    fi
    res=$(apidiff -incompatible "$ex" "$ip" 2>&1)
    [ -z "$res" ] || printf '## %s\n%s\n' "$ip" "$res"
  done
}

# build_pkgs OUTDIR PKG...: build library packages normally (results discarded) and main
# packages into OUTDIR, so nothing is written to the repo. `go build -o DIR` rejects a list with
# no main package, hence the split.
build_pkgs() {
  bout=$1; shift
  mains=$(go list -e -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' "$@" 2>/dev/null)
  libs=$(go list -e -f '{{if ne .Name "main"}}{{.ImportPath}}{{end}}' "$@" 2>/dev/null)
  brc=0
  [ -z "$libs" ] || go build $libs || brc=$?
  [ -z "$mains" ] || go build -o "$bout/" $mains || brc=$?
  return $brc
}

# deadcode_scoped: unreachable functions (tests count as roots) in the reviewed files of
# this module. $1 = module dir relative to the repo root.
deadcode_scoped() {
  m=$1
  pre=''; [ "$m" = . ] || pre=$m/
  log=$(mktemp) || return 1
  deadcode -test ./... >"$log" 2>&1
  rc=$?
  if [ $rc != 0 ]; then
    if grep -q 'no main packages' "$log"; then echo '# no main or test packages: nothing to analyze'; rc=0
    else cat "$log"
    fi
    rm -f "$log"; return $rc
  fi
  awk -v pre="$pre" -v files="$scoped" '
    BEGIN { while ((getline l < files) > 0) { split(l, f, "\t"); if (f[1] != "D") ch[f[7]] = 1 } }
    { p = $0; sub(/:[0-9]+:[0-9]+: .*/, "", p); sub(/^\.\//, "", p)
      if ((pre p) in ch || p in ch) print }' "$log"
  rm -f "$log"
}

# --- Go, per module ------------------------------------------------------------------
gover=$(go env GOVERSION 2>/dev/null | sed -n 's/^go\([0-9][0-9]*\.[0-9][0-9]*\).*/\1/p')
gomaj=${gover%%.*} gomin=${gover#*.}
modules=$(cut -f1 "$ctx/packages.tsv" | sort -u)
nmod=$(printf '%s\n' "$modules" | grep -c .)
for m in $modules; do
  sfx=''
  [ "$nmod" -gt 1 ] && sfx="@$(printf '%s' "$m" | tr '/' '_')"
  pkgs=$(awk -F '\t' -v m="$m" '$1 == m { print $2 }' "$ctx/packages.tsv")
  testpkgs=''
  for p in $pkgs; do
    full=${p#./}; [ "$m" = . ] || full=$m/${p#./}; [ "$full" = . ] && full=''
    ex=0
    for e in $excludes; do
      case $full in "$e"|"$e"/*) ex=1 ;; esac
    done
    [ $ex = 1 ] || testpkgs="$testpkgs$p
"
  done
  # shellcheck disable=SC2086 # package lists split on newlines (IFS) by design
  (
    cd -- "$m" || exit 1
    # Test-only packages (e.g. sdk/kronk/tests/*) have nothing to build; go vet still type-checks them.
    buildpkgs=$(go list -e -f '{{if or .GoFiles .CgoFiles}}{{.ImportPath}}{{end}}' $pkgs 2>/dev/null)
    if [ -n "$buildpkgs" ]; then job "go-build$sfx" build_pkgs "$scratch/bin" $buildpkgs
    else skip "go-build$sfx" "only test-only packages in scope"
    fi
    # Whole module, as CI does: catches callers in unchanged packages broken by the diff.
    job "go-vet$sfx" go vet ./...
    if [ $tests = 0 ]; then skip "go-test$sfx" "--no-tests"
    elif [ $runcode = 0 ]; then skip "go-test$sfx" "$untrusted_why"
    elif [ "$depth" = project ]; then skip "go-test$sfx" "project depth: AGENTS.md forbids a full repo test run"
    elif [ -z "$testpkgs" ]; then skip "go-test$sfx" "all packages excluded"
    else job "go-test$sfx" go test -race -short -count=1 -timeout "$timeout" $testpkgs
    fi
    if has staticcheck; then job "staticcheck$sfx" staticcheck -checks=all $pkgs; else skip "staticcheck$sfx" "staticcheck not installed"; fi
    if has checklocks; then job "checklocks$sfx" go vet -vettool="$(toolpath checklocks)" $pkgs
    else skip "checklocks$sfx" "checklocks not installed"
    fi
    if has errcheck; then job "errcheck$sfx" errcheck $pkgs; else skip "errcheck$sfx" "errcheck not installed"; fi
    if has nilaway; then
      modpath=$(awk '$1 == "module" { gsub(/"/, "", $2); print $2; exit }' go.mod)
      job "nilaway$sfx" nilaway -pretty-print=false -include-pkgs="$modpath" $pkgs
    else skip "nilaway$sfx" "nilaway not installed"
    fi
    if has deadcode; then job "deadcode$sfx" deadcode_scoped "$m"; else skip "deadcode$sfx" "deadcode not installed"; fi
    if ! has apidiff; then skip "apidiff$sfx" "apidiff not installed"
    elif [ -z "$basesrc" ]; then skip "apidiff$sfx" "base commit not available locally"
    else job "apidiff$sfx" apidiff_pkgs "$m" $pkgs
    fi
    if [ -n "$gover" ] && { [ "$gomaj" -gt 1 ] || [ "$gomin" -ge 26 ]; }; then job "go-fix$sfx" go fix -diff $pkgs
    else skip "go-fix$sfx" "go fix modernizers need Go 1.26+"
    fi
    job "go-mod-tidy$sfx" go mod tidy -diff
    if has gosec; then job "gosec$sfx" gosec -quiet $pkgs; else skip "gosec$sfx" "gosec not installed"; fi
    if has govulncheck; then job "govulncheck$sfx" govulncheck $pkgs; else skip "govulncheck$sfx" "govulncheck not installed"; fi
    if has dupl; then
      dirs=$(printf '%s\n' $pkgs)
      job "dupl$sfx" dupl -t 50 -plumbing $dirs
    else skip "dupl$sfx" "dupl not installed"
    fi
    wait
  ) &
done

# --- reviewed files by language ------------------------------------------------------
go_files=$(live go)
if [ -n "$go_files" ] && has go; then
  # shellcheck disable=SC2086
  job gofmt gofmt -s -l $go_files
fi

# BUI: `npm run build` is `tsc && vite build`; the type check is the part that can fail on a
# reviewed change, and it writes nothing.
bui=cmd/server/api/frontends/bui
if live ts | grep -q "^$bui/"; then
  if [ ! -d "$bui/node_modules" ]; then skip bui-tsc "BUI dependencies not installed (cd $bui && npm ci)"
  elif ! has npx; then skip bui-tsc "npx not installed"
  elif [ $runcode = 0 ]; then skip bui-tsc "$untrusted_why"
  else job bui-tsc sh -c 'cd "$1" && npx --no-install tsc --noEmit' _ "$bui"
  fi
fi

# Helm: the chart's own checks (.manual/chapter-20 §20.10.3); renders only, no cluster.
if live '*' | grep -q '^zarf/helm/kronk/'; then
  if [ ! -x zarf/helm/kronk/test.sh ]; then skip helm-test "zarf/helm/kronk/test.sh missing"
  elif [ $runcode = 0 ]; then skip helm-test "$untrusted_why"
  elif ! has helm; then skip helm-test "helm not installed"
  elif ! python3 -I -c 'import yaml' >/dev/null 2>&1; then skip helm-test "python3 with PyYAML not installed"
  else job helm-test zarf/helm/kronk/test.sh
  fi
fi

sh_files=$(live sh)
if [ -n "$sh_files" ]; then
  if has shellcheck; then
    # shellcheck disable=SC2086
    job shellcheck shellcheck $sh_files
  else skip shellcheck "shellcheck not installed"
  fi
fi

js_dirs=$( { live js; live ts; } | sed 's|/[^/]*$||' | sort -u)
if [ -n "$js_dirs" ] && has jscpd; then
  # shellcheck disable=SC2086
  job jscpd jscpd --min-tokens 50 --reporters console --silent $js_dirs
fi

# Secrets: the reviewed diff, or at full depth a copy of the review set (so generated and
# vendored files stay out; findings show repo-relative paths). generated.patch is scanned
# only on request, by the generated-scan agent (see references/roles/generated-scan.md).
if has gitleaks && [ "$scoped" != "$ctx/files.tsv" ]; then
  if setcopy=$(mktemp -d); then
    # Never follow a symlink out of the repo (intake already leaves them out of the set).
    live '*' | while IFS= read -r f; do
      [ -L "$f" ] && continue
      mkdir -p "$setcopy/$(dirname -- "$f")" && cp -P -p -- "$f" "$setcopy/$f"
    done
    if gitleaks help dir >/dev/null 2>&1; then
      # shellcheck disable=SC2016 # $1 expands in the child shell
      job gitleaks sh -c 'cd "$1" && gitleaks dir -v --no-banner --redact .' _ "$setcopy"
    else
      # shellcheck disable=SC2016
      job gitleaks sh -c 'cd "$1" && gitleaks detect -v --no-git --no-banner --redact --source .' _ "$setcopy"
    fi
  else setcopy=''; skip gitleaks "cannot create a temp copy of the review set"
  fi
elif has gitleaks; then
  if gitleaks help stdin >/dev/null 2>&1; then
    # shellcheck disable=SC2016 # $1 expands in the child shell
    job gitleaks sh -c 'gitleaks stdin -v --no-banner --redact <"$1"' _ "$ctx/diff.patch"
  else
    job gitleaks gitleaks detect -v --no-git --no-banner --redact --source "$ctx/diff.patch"
  fi
elif has trufflehog; then
  # shellcheck disable=SC2046
  job trufflehog trufflehog filesystem --no-update $(live '*')
else
  skip secrets "no secret scanner (gitleaks/trufflehog) installed"
fi

wait

# --- summary ----------------------------------------------------------------------------
printf 'check\tstatus\texit\tlines\tfile\n' >"$out/summary.tsv"
for f in "$out"/*.exit; do
  [ -e "$f" ] || continue
  n=${f##*/}; n=${n%.exit}; e=$(cat "$f")
  case $e in skip) s=skipped ;; 0) s=pass ;; *) s=fail ;; esac
  # These exit 0 and report through output: any line not starting with "#" is a hit.
  case $n in
    dupl*|gofmt|deadcode*|apidiff*)
      [ "$s" = pass ] && grep -q '^[^#]' "$out/$n.txt" && s=findings ;;
  esac
  printf '%s\t%s\t%s\t%s\t%s\n' "$n" "$s" "$e" "$(wc -l <"$out/$n.txt" | tr -d ' ')" "checks/$n.txt" >>"$out/summary.tsv"
  rm -f "$f"
done
cat "$out/summary.tsv"
