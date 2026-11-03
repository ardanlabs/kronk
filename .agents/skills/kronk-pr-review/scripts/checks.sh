#!/bin/sh
# Usage: sh checks.sh RUN_DIR [--env K=V]... [--exclude PATH]... [--timeout 10m] [--no-tests]
#                    [--extra NAME,...|all]
# Runs every mechanical check ONCE, scoped to the files/units intake.sh recorded in
# RUN_DIR/context, in parallel. The depth comes from scope.txt: at diff depth file checks
# cover the changed files; at full depth (files/packages/project) they cover review-set.tsv,
# and unit checks (Go packages, crates, Terraform modules, charts, ...) cover packages.tsv and
# stack.tsv, which intake fills to match the depth. Checks that run the reviewed code (tests,
# Rust builds, Terraform init/validate, tflint plugins, mypy and Ansible plugins, repo gates) run
# only when scope.txt says trust=maintainer (a maintainer PR, see intake.sh); otherwise they are
# skipped and the rest run against neutral configs. Nothing writes to the working tree beyond
# build caches (Go cache, cargo target/): Terraform init uses a temp TF_DATA_DIR and leaves the
# lock file as it was; Python checks run without caches or bytecode.
# Output: RUN_DIR/context/checks/<check>.txt (full log) and RUN_DIR/context/checks/summary.tsv
# (check status exit lines file lens). Prints the summary.
# Kronk rules (AGENTS.md, .github/workflows/linux.yml static + race jobs) always apply:
# GITHUB_WORKSPACE=<repo root>; tests (maintainer PRs only) are `go test -race -short` on the
# reviewed packages only (never at project depth: AGENTS.md forbids a full repo test run), with
# KRONK_BASE_PATH=<empty temp store> so model-backed tests skip; go vet covers the whole module;
# packages under DEFAULT_EXCLUDES are never tested; staticcheck runs with -checks=all.
#   --env K=V      exported for every check; repeatable
#   --exclude PATH never run tests in packages/crates/projects under PATH (repo-relative); adds to
#                  the defaults
#   --timeout D    test timeout (default 10m, as in CI)
#   --no-tests     skip test runs
#   --extra NAMES  also run these user-approved rows of context/extra-checks.tsv (all = every row)
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
excludes=$DEFAULT_EXCLUDES timeout=10m tests=1 envkeys='' extra=''
while [ $# -gt 0 ]; do
  case $1 in
    --env|--exclude|--timeout|--extra) [ $# -ge 2 ] || die "$1 needs a value" ;;
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
    --extra) extra=${2-}; shift 2 ;;
    -h|--help) sed -n '2,27p' "$0"; exit 0 ;;
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
trust=$(sed -n 's/^trust=//p' "$ctx/scope.txt")
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
mkdir -p "$scratch/kronk-home"
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
  echo "trust=${trust:-untrusted}"
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
# runs_code: true when checks may execute the reviewed code (maintainer PR only)
runs_code() { [ "$trust" = maintainer ]; }
UNTRUSTED="runs the reviewed code: only for a maintainer PR (trust=${trust:-untrusted})"

# live LANG: reviewed (not deleted) files of LANG ("*" = any), repo-relative. Generated files
# reviewed on request (--include-generated) are read by reviewers but not linted.
live() { awk -F '\t' -v l="$1" '$1 != "D" && $5 != "generated" && $5 != "vendor" && (l == "*" || $4 == l) { print $7 }' "$scoped"; }
# units ECO: unit directories of an ecosystem from stack.tsv ("-" rows are file-level)
units() { awk -F '\t' -v e="$1" '$1 == e && $2 != "-" { print $2 }' "$ctx/stack.tsv" 2>/dev/null; }
# usfx ECO UNIT: log-name suffix, "@unit" when the ecosystem has more than one unit
usfx() { [ "$(units "$1" | grep -c .)" -gt 1 ] && printf '@%s' "$(printf '%s' "$2" | tr '/' '_')"; return 0; }
# excluded PATH: true when PATH is under a --exclude entry
excluded() { for e in $excludes; do case $1 in "$e"|"$e"/*) return 0 ;; esac; done; return 1; }
# under UNIT LANG: live files of LANG inside UNIT, relative to UNIT
under() { live "$2" | awk -v u="$1" 'u == "." { print; next } index($0, u "/") == 1 { print substr($0, length(u) + 2) }'; }
tmo=$(command -v timeout 2>/dev/null || command -v gtimeout 2>/dev/null)
# with_timeout CMD...: CMD under the test timeout when timeout/gtimeout exists
with_timeout() { if [ -n "$tmo" ]; then "$tmo" "$timeout" "$@"; else "$@"; fi; }

# --- base tree (apidiff, buf breaking) ---------------------------------------------------
# The base commit's files, extracted outside the repo with git archive: nothing is written
# to the working tree or .git. Removed when the script exits.
basesrc='' setcopy=''
trap 'rm -rf "$scratch"; [ -z "$basesrc" ] || rm -rf "$basesrc"; [ -z "$setcopy" ] || rm -rf "$setcopy"' EXIT
protos=$(live proto)
bufcfg=0
{ [ -f buf.yaml ] || [ -f buf.work.yaml ]; } && bufcfg=1
if [ -n "$basesha" ] && { { [ -s "$ctx/packages.tsv" ] && has apidiff; } ||
  { [ -n "$protos" ] && [ $bufcfg = 1 ] && has buf; }; }; then
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
    # -o /dev/null: a main package would otherwise write its binary into the repo.
    buildpkgs=$(go list -e -f '{{if or .GoFiles .CgoFiles}}{{.ImportPath}}{{end}}' $pkgs 2>/dev/null)
    if [ -n "$buildpkgs" ]; then job "go-build$sfx" go build -o /dev/null $buildpkgs
    else skip "go-build$sfx" "only test-only packages in scope"
    fi
    # Whole module, as CI does: catches callers in unchanged packages broken by the diff.
    job "go-vet$sfx" go vet ./...
    if ! runs_code; then skip "go-test$sfx" "$UNTRUSTED"
    elif [ $tests = 0 ]; then skip "go-test$sfx" "--no-tests"
    elif [ "$depth" = project ]; then skip "go-test$sfx" "project depth: AGENTS.md forbids a full repo test run"
    elif [ -z "$testpkgs" ]; then skip "go-test$sfx" "all packages excluded"
    else job "go-test$sfx" go test -race -short -count=1 -timeout "$timeout" $testpkgs
    fi
    if [ -e .golangci.yml ] || [ -e .golangci.yaml ] || [ -e .golangci.toml ] || [ -e .golangci.json ]; then
      # The repo's config can load custom linter plugins: read it only for a maintainer PR;
      # otherwise --no-config runs the default linters and never parses that file.
      if ! has golangci-lint; then skip "golangci-lint$sfx" "golangci-lint not installed"
      elif runs_code; then job "golangci-lint$sfx" golangci-lint run $pkgs
      else job "golangci-lint$sfx" golangci-lint run --no-config $pkgs
      fi
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

if [ -n "$protos" ]; then
  if [ $bufcfg = 0 ]; then skip buf-lint "no buf.yaml/buf.work.yaml at the repo root"
  elif ! has buf; then skip buf-lint "buf not installed"
  else
    set --
    for p in $protos; do set -- "$@" --path "$p"; done
    # buf.yaml v2 can name lint/breaking plugins (executables): the repo's config only for a
    # maintainer PR, otherwise buf's default rules from an inline config.
    if runs_code; then
      job buf-lint buf lint "$@"
      if [ -n "$basesrc" ]; then job buf-breaking buf breaking --against "$basesrc"
      else skip buf-breaking "base commit not available locally"
      fi
    else
      job buf-lint buf lint --config '{"version":"v2"}' "$@"
      if [ -n "$basesrc" ]; then job buf-breaking buf breaking --config '{"version":"v2"}' --against "$basesrc" --against-config '{"version":"v2"}'
      else skip buf-breaking "base commit not available locally"
      fi
    fi
  fi
fi

# BUI: `npm run build` is `tsc && vite build`; the type check is the part that can fail on a
# reviewed change, and it writes nothing.
bui=cmd/server/api/frontends/bui
if live ts | grep -q "^$bui/"; then
  if [ ! -d "$bui/node_modules" ]; then skip bui-tsc "BUI dependencies not installed (cd $bui && npm ci)"
  elif ! has npx; then skip bui-tsc "npx not installed"
  elif ! runs_code; then skip bui-tsc "$UNTRUSTED (node_modules tools)"
  else job bui-tsc sh -c 'cd "$1" && npx --no-install tsc --noEmit' _ "$bui"
  fi
fi

# Helm: the chart's own checks (.manual/chapter-20 §20.10.3); renders only, no cluster.
if live '*' | grep -q '^zarf/helm/kronk/'; then
  if [ ! -x zarf/helm/kronk/test.sh ]; then skip helm-test "zarf/helm/kronk/test.sh missing"
  elif ! runs_code; then skip helm-test "$UNTRUSTED (repo script)"
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

js_dirs=$( { live js; live ts; live vue; } | sed 's|/[^/]*$||' | sort -u)
if [ -n "$js_dirs" ] && has jscpd; then
  # shellcheck disable=SC2086
  job jscpd jscpd --min-tokens 50 --reporters console --silent $js_dirs
fi

# --- Rust, per crate -------------------------------------------------------------------
# cargo_audit LOCKFILE: cargo-audit on one lock file, from a directory outside the repo
cargo_audit() { ( cd / && "$(toolpath cargo-audit)" audit --file "$1" ); }
# cargo_audit_dirs: directories holding the Cargo.lock of each crate unit, deduplicated
cargo_lock_dirs() {
  units rust | while IFS= read -r u; do
    d=$u
    while [ ! -f "$d/Cargo.lock" ] && [ "$d" != . ]; do case $d in */*) d=${d%/*} ;; *) d=. ;; esac; done
    [ -f "$d/Cargo.lock" ] && printf '%s\n' "$d"
  done | sort -u
}
if [ -n "$(units rust)" ]; then
  if ! has cargo; then skip cargo "cargo not installed"
  else
    while IFS= read -r u; do
      [ -n "$u" ] || continue
      s=$(usfx rust "$u")
      (
        cd -- "$u" || exit 1
        # Any cargo call inside the crate reads its rust-toolchain.toml (a toolchain path) and
        # .cargo/config.toml (aliases, build.rustc): repo-chosen binaries run.
        if ! runs_code; then skip "cargo-fmt$s" "$UNTRUSTED (rust-toolchain.toml, .cargo/config.toml)"
        elif cargo fmt --version >/dev/null 2>&1; then job "cargo-fmt$s" cargo fmt --check
        else skip "cargo-fmt$s" "rustfmt not installed"
        fi
        # --locked: fail instead of rewriting Cargo.lock
        # clippy/check/test compile the crate: build scripts and proc macros run
        if ! runs_code; then skip "cargo-clippy$s" "$UNTRUSTED (build scripts, proc macros)"
        elif cargo clippy --version >/dev/null 2>&1; then job "cargo-clippy$s" cargo clippy --all-targets --locked -- -D warnings
        else skip "cargo-clippy$s" "clippy not installed"; job "cargo-check$s" cargo check --all-targets --locked
        fi
        if ! runs_code; then skip "cargo-test$s" "$UNTRUSTED"
        elif [ $tests = 0 ]; then skip "cargo-test$s" "--no-tests"
        elif excluded "$u"; then skip "cargo-test$s" "crate excluded"
        else job "cargo-test$s" with_timeout cargo test --all-targets --locked
        fi
        wait
      ) &
    done <<EOF
$(units rust)
EOF
    while IFS= read -r d; do
      [ -n "$d" ] || continue
      s=''; [ "$(cargo_lock_dirs | grep -c .)" -gt 1 ] && s="@$(printf '%s' "$d" | tr '/' '_')"
      (
        cd -- "$d" || exit 1
        # cargo-audit reads only Cargo.lock: run the binary itself from / (no cargo, no repo config).
        if has cargo-audit; then job "cargo-audit$s" cargo_audit "$PWD/Cargo.lock"; else skip "cargo-audit$s" "cargo-audit not installed"; fi
        if [ -f deny.toml ]; then
          # cargo-deny runs cargo metadata, which reads the repo's cargo config and toolchain file
          if ! runs_code; then skip "cargo-deny$s" "$UNTRUSTED (cargo metadata, repo cargo config)"
          elif has cargo-deny; then job "cargo-deny$s" cargo deny check; else skip "cargo-deny$s" "cargo-deny not installed"; fi
        fi
        wait
      ) &
    done <<EOF
$(cargo_lock_dirs)
EOF
  fi
fi

# --- Terraform / OpenTofu, per module --------------------------------------------------
# The stack marker names the binary (terraform or tofu). init downloads providers and modules
# into a temp TF_DATA_DIR. init may add this platform's hashes to .terraform.lock.hcl (validate
# needs them; a read-only lock fails wherever the lock lacks them), so the file is restored
# byte for byte afterwards, and one that init creates is removed. Locked versions still apply.
tfbin=$(awk -F '\t' '$1 == "terraform" { print $4; exit }' "$ctx/stack.tsv" 2>/dev/null)
tf_fmt() { rc=0; for f in "$@"; do "$tfbin" fmt -check -diff -no-color "$f" || rc=$?; done; return $rc; }
tf_validate() {
  d=$(mktemp -d) || return 1
  bak=''
  if [ -f .terraform.lock.hcl ]; then bak=$d.lock; cp .terraform.lock.hcl "$bak" || return 1; fi
  TF_DATA_DIR=$d "$tfbin" init -backend=false -input=false -no-color >"$d.init" 2>&1
  rc=$?
  if [ $rc = 0 ]; then TF_DATA_DIR=$d "$tfbin" validate -no-color; rc=$?
  else echo "# init failed (providers, modules, or credentials):"; cat "$d.init"
  fi
  # validate needs the lock file init wrote: put the original back only afterwards
  if [ -z "$bak" ]; then rm -f .terraform.lock.hcl
  elif ! cmp -s "$bak" .terraform.lock.hcl; then cat "$bak" >.terraform.lock.hcl
  fi
  rm -rf "$d" "$d.init" "$bak"; return $rc
}
# tf_lint: the repo's .tflint.hcl (and its plugins) only for a maintainer PR; otherwise a
# neutral config with the bundled rules and no plugin installation.
tf_lint() {
  cfg=''
  if ! runs_code; then cfg=$(mktemp) || return 1; printf 'config {\n}\n' >"$cfg"; tflint --config="$cfg" --format=compact --no-color; rc=$?; rm -f "$cfg"; return $rc; fi
  if [ -f .tflint.hcl ]; then cfg=$PWD/.tflint.hcl; elif [ -f "$root/.tflint.hcl" ]; then cfg=$root/.tflint.hcl; fi
  if [ -n "$cfg" ]; then tflint --init --config="$cfg" >/dev/null 2>&1; tflint --config="$cfg" --format=compact --no-color
  else tflint --format=compact --no-color
  fi
}
tf_files=$(live tf | grep -E '\.(tf|tofu|tfvars|tftest\.hcl)$')
if [ -n "$tf_files" ]; then
  if ! has "$tfbin"; then skip tf "$tfbin not installed"
  else
    # shellcheck disable=SC2086
    job tf-fmt tf_fmt $tf_files
    while IFS= read -r u; do
      [ -n "$u" ] || continue
      s=$(usfx terraform "$u")
      # init installs the providers and modules the change names, and validate runs them
      if runs_code; then ( cd -- "$u" || exit 1; job "tf-validate$s" tf_validate; wait ) &
      else skip "tf-validate$s" "$UNTRUSTED (providers, modules)"
      fi
    done <<EOF
$(units terraform)
EOF
  fi
  while IFS= read -r u; do
    [ -n "$u" ] || continue
    s=$(usfx terraform "$u")
    if has tflint; then ( cd -- "$u" || exit 1; job "tflint$s" tf_lint; wait ) &
    else skip "tflint$s" "tflint not installed"
    fi
  done <<EOF
$(units terraform)
EOF
fi

# --- Helm, Kustomize, K8s manifests -----------------------------------------------------
# kube_check FILE...: kubeconform + kube-linter on manifests
kube_check() {
  rc=0
  if has kubeconform; then echo "## kubeconform"; kubeconform -strict -ignore-missing-schemas -summary "$@" || rc=$?; fi
  if has kube-linter; then echo "## kube-linter"; kube-linter lint "$@" || rc=$?; fi
  return $rc
}
# render KIND UNIT: helm template / kustomize build, then kube_check on the output (in a
# .yaml file: kubeconform skips files without a YAML extension)
render() {
  t=$(mktemp -d) || return 1
  f=$t/render.yaml
  case $1 in
    helm) helm template review "$2" >"$f" ;;
    kustomize) if has kustomize; then kustomize build "$2" >"$f"; else kubectl kustomize "$2" >"$f"; fi ;;
  esac
  rc=$?
  [ $rc = 0 ] && { kube_check "$f"; rc=$?; }
  rm -rf "$t"; return $rc
}
while IFS= read -r u; do
  [ -n "$u" ] || continue
  s=$(usfx helm "$u")
  if has helm; then job "helm-lint$s" helm lint "$u"; job "helm-render$s" render helm "$u"
  else skip "helm$s" "helm not installed"
  fi
done <<EOF
$(units helm)
EOF
while IFS= read -r u; do
  [ -n "$u" ] || continue
  s=$(usfx kustomize "$u")
  if has kustomize || has kubectl; then job "kustomize$s" render kustomize "$u"
  else skip "kustomize$s" "kustomize/kubectl not installed"
  fi
done <<EOF
$(units kustomize)
EOF
k8s_plain=$(live k8s | awk -v K="$ctx/stack.tsv" '
  BEGIN { while ((getline l < K) > 0) { split(l, f, "\t"); if (f[1] == "kustomize") ku[f[2]] = 1 } }
  { for (u in ku) if (u == "." || index($0, u "/") == 1) next; print }')
if [ -n "$k8s_plain" ]; then
  if has kubeconform || has kube-linter; then
    # shellcheck disable=SC2086
    job k8s-manifests kube_check $k8s_plain
  else skip k8s-manifests "kubeconform/kube-linter not installed"
  fi
fi

# IaC misconfiguration scan: trivy, else checkov, over each infra unit directory plus the
# K8s and Docker files outside those units (a file target scans only that file).
# (BSD awk rejects newlines in -v values: the unit list goes through a file.)
iac_units=$out/.iac-units
{ units terraform; units helm; units kustomize; } | sort -u >"$iac_units"
iac_targets=$( { cat "$iac_units"; { live k8s; live docker; } | awk -v U="$iac_units" '
    BEGIN { while ((getline l < U) > 0) if (l != "") u[++n] = l }
    { for (i = 1; i <= n; i++) if (u[i] == "." || index($0, u[i] "/") == 1) next; print }'; } | grep . | sort -u)
rm -f "$iac_units"
# trivy reads trivy.yaml and .trivyignore from its working directory (output paths, module
# dirs, ignores): for an untrusted change it runs from a temp dir with a neutral config.
iac_scan() {
  rc=0
  tcfg=''
  if has trivy && ! runs_code; then
    tdir=$(mktemp -d) || return 1
    tcfg=$tdir/trivy.yaml; printf 'quiet: true\n' >"$tcfg"
  fi
  for x in "$@"; do
    echo "## $x"
    if has trivy && [ -n "$tcfg" ]; then
      ( cd -- "$tdir" && trivy config --config "$tcfg" --quiet --exit-code 1 "$root/$x" ) || rc=$?
    elif has trivy; then trivy config --quiet --exit-code 1 "$x" || rc=$?
    elif [ -d "$x" ]; then checkov --quiet --compact -d "$x" || rc=$?
    else checkov --quiet --compact -f "$x" || rc=$?
    fi
  done
  [ -z "$tcfg" ] || rm -rf "$tdir"
  return $rc
}
if [ -n "$iac_targets" ]; then
  # checkov loads the repo's .checkov.yaml, whose external-checks-dir runs Python checks from
  # the repo: only for a maintainer PR. trivy runs with a neutral config (iac_scan).
  if has trivy || { has checkov && runs_code; }; then
    # shellcheck disable=SC2086
    job iac-scan iac_scan $iac_targets
  elif has checkov; then skip iac-scan "$UNTRUSTED (checkov repo config can load Python checks; install trivy)"
  else skip iac-scan "trivy/checkov not installed"
  fi
fi

# --- Python, per project ----------------------------------------------------------------
# No caches or bytecode in the repo: ruff --no-cache, mypy --cache-dir=/dev/null, pytest
# without its cache provider, PYTHONDONTWRITEBYTECODE.
py_files=$(live py)
if [ -n "$py_files" ]; then
  export PYTHONDONTWRITEBYTECODE=1
  if has ruff; then
    # shellcheck disable=SC2086
    job ruff ruff check --no-cache --no-fix --output-format=concise $py_files
    # shellcheck disable=SC2086
    job ruff-format ruff format --no-cache --check --diff $py_files
  else skip ruff "ruff not installed"
  fi
  # shellcheck disable=SC2086
  if has bandit; then job bandit bandit -q $py_files; else skip bandit "bandit not installed"; fi
  while IFS= read -r u; do
    [ -n "$u" ] || continue
    s=$(usfx python "$u")
    (
      cd -- "$u" || exit 1
      py=python3; [ -x .venv/bin/python ] && py=.venv/bin/python
      if [ -f mypy.ini ] || [ -f .mypy.ini ] || grep -qs '^\[tool\.mypy' pyproject.toml || grep -qs '^\[mypy' setup.cfg; then
        rel=$(under "$u" py)
        # The repo's config can load mypy plugins: only for a maintainer PR; otherwise defaults.
        mcfg=''
        if ! runs_code; then mcfg=$out/.mypy-neutral.ini; printf '[mypy]\n' >"$mcfg"; fi
        # shellcheck disable=SC2086
        if has mypy; then job "mypy$s" mypy --cache-dir=/dev/null ${mcfg:+--config-file="$mcfg"} $rel; else skip "mypy$s" "mypy not installed"; fi
      fi
      if git ls-files -- . | grep -Eq '(^|/)(test_[^/]*|[^/]*_test)\.py$'; then
        if ! runs_code; then skip "pytest$s" "$UNTRUSTED"
        elif [ $tests = 0 ]; then skip "pytest$s" "--no-tests"
        elif excluded "$u"; then skip "pytest$s" "project excluded"
        else
          # the project's venv, else a pytest importable by python3, else a standalone pytest (pipx)
          set -- "$py" -m pytest
          if ! "$py" -m pytest --version >/dev/null 2>&1; then
            if has pytest; then set -- pytest; else set --; fi
          fi
          if [ $# = 0 ]; then skip "pytest$s" "pytest not installed"
          else
            ign=''
            # newline-separated, like every list here (IFS)
            for e in $excludes; do case $e in "$u"/*) ign="$ign--ignore=${e#"$u"/}
" ;; esac; done
            # shellcheck disable=SC2086
            job "pytest$s" with_timeout "$@" -q -p no:cacheprovider $ign
          fi
        fi
      fi
      for r in requirements*.txt; do
        [ -f "$r" ] || continue
        # --no-deps --disable-pip: audit the pins as written; resolving could run setup.py code
        if has pip-audit; then job "pip-audit$s@${r%.txt}" pip-audit --no-deps --disable-pip -r "$r"; else skip "pip-audit$s" "pip-audit not installed"; fi
      done
      wait
    ) &
  done <<EOF
$(units python)
EOF
fi

# --- Ansible, YAML, Dockerfiles, CI -----------------------------------------------------
ans_files=$(live ansible)
if [ -n "$ans_files" ]; then
  # shellcheck disable=SC2086
  # ansible-lint loads the repo's plugins, modules, and custom rules
  if ! runs_code; then skip ansible-lint "$UNTRUSTED (repo plugins and rules)"
  elif has ansible-lint; then job ansible-lint ansible-lint --offline -q $ans_files
  else skip ansible-lint "ansible-lint not installed"
  fi
fi
ycfg=''
for c in .yamllint .yamllint.yaml .yamllint.yml; do [ -e "$c" ] && ycfg=$c && break; done
if [ -n "$ycfg" ]; then
  y_files=$( { live yaml; live ansible; live k8s; live ci; } | grep -E '\.ya?ml$')
  if [ -n "$y_files" ]; then
    # shellcheck disable=SC2086
    if has yamllint; then job yamllint yamllint -f parsable $y_files; else skip yamllint "yamllint not installed"; fi
  fi
fi
dockerfiles=$(live docker | awk '{ b = tolower($0); sub(/.*\//, "", b) } b !~ /compose/')
if [ -n "$dockerfiles" ]; then
  # shellcheck disable=SC2086
  if has hadolint; then job hadolint hadolint $dockerfiles; else skip hadolint "hadolint not installed"; fi
fi
gha=$(live ci | grep -E '(^|/)\.github/|(^|/)action\.ya?ml$')
if [ -n "$gha" ]; then
  wf=$(printf '%s\n' "$gha" | grep '\.github/workflows/')
  if [ -n "$wf" ]; then
    # shellcheck disable=SC2086
    if has actionlint; then job actionlint actionlint -no-color $wf; else skip actionlint "actionlint not installed"; fi
  fi
  # shellcheck disable=SC2086
  if has zizmor; then job zizmor zizmor --offline $gha; else skip zizmor "zizmor not installed"; fi
fi

# --- extra checks the user approved (context/extra-checks.tsv: name dir lens command source) ---
# Repo gates run commands the reviewed repo defines: only for a maintainer PR.
if [ -n "$extra" ] && [ -f "$ctx/extra-checks.tsv" ] && ! runs_code; then skip extra-checks "$UNTRUSTED (repo-defined commands)"
elif [ -n "$extra" ] && [ -f "$ctx/extra-checks.tsv" ]; then
  while IFS="$(printf '\t')" read -r xn xd _ xc _; do
    case $xn in ''|\#*) continue ;; esac
    case ",$extra," in *",$xn,"*|,all,) ;; *) continue ;; esac
    case $xn in *[!A-Za-z0-9._-]*) continue ;; esac
    ( cd -- "${xd:-.}" || exit 1; job "x-$xn" sh -c "$xc"; wait ) &
  done <"$ctx/extra-checks.tsv"
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

semcfg=''
for c in .semgrep.yml .semgrep.yaml .semgrep; do [ -e "$c" ] && semcfg=$c && break; done
if has semgrep && [ -n "$semcfg" ]; then
  # shellcheck disable=SC2046
  job semgrep semgrep scan --metrics=off --config "$semcfg" $(live '*')
fi

wait
rm -f "$out/.mypy-neutral.ini"

# --- summary ----------------------------------------------------------------------------
# lens_of CHECK: the lens that owns a check's failures (extra checks: their lens column)
lens_of() {
  case $1 in
    x-*) awk -F '\t' -v n="${1#x-}" '$1 == n { print ($3 == "" ? "service-steward" : $3); exit }' "$ctx/extra-checks.tsv" ;;
    go-test*|cargo-test*|pytest*) echo harness-map ;;
    errcheck*) echo error-tripwire ;;
    apidiff*|buf-*) echo boundary-keeper ;;
    deadcode*) echo straight-line-pass ;;
    dupl*|jscpd*) echo clone-hunter ;;
    gosec*|govulncheck*|gitleaks|trufflehog|secrets|semgrep|cargo-audit*|cargo-deny*|bandit|pip-audit*|iac-scan|zizmor) echo security-sentinel ;;
    *) echo service-steward ;;
  esac
}
printf 'check\tstatus\texit\tlines\tfile\tlens\n' >"$out/summary.tsv"
for f in "$out"/*.exit; do
  [ -e "$f" ] || continue
  n=${f##*/}; n=${n%.exit}; e=$(cat "$f")
  case $e in skip) s=skipped ;; 0) s=pass ;; *) s=fail ;; esac
  # These exit 0 and report through output: any line not starting with "#" is a hit.
  case $n in
    dupl*|gofmt|deadcode*|apidiff*)
      [ "$s" = pass ] && grep -q '^[^#]' "$out/$n.txt" && s=findings ;;
  esac
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$n" "$s" "$e" "$(wc -l <"$out/$n.txt" | tr -d ' ')" "checks/$n.txt" "$(lens_of "$n")" >>"$out/summary.tsv"
  rm -f "$f"
done
cat "$out/summary.tsv"
