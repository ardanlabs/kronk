#!/bin/sh
# Usage: sh guidelines.sh [--go X.Y] [PATH]  -> "go X.Y" + matching ../references/features.txt lines.
# Version: --go; nearest go.mod up (no go line: 1.16); nearest go.work (1.18); lowest go.mod
# below PATH (git-tracked, else find; skips testdata/vendor/node_modules); else "go unknown". No Go needed.

data="$(cd -- "$(dirname -- "$0")/../references" 2>/dev/null && pwd)/features.txt"

path=. ver=
while [ $# -gt 0 ]; do
  case $1 in
    --go) ver=${2-}; shift; [ $# -gt 0 ] && shift ;;
    --go=*) ver=${1#--go=}; shift ;;
    *) path=$1; shift ;;
  esac
done

[ -e "$path" ] || { echo "go unknown"; exit 0; }
[ -d "$path" ] || path=$(dirname -- "$path")
cd -- "$path" 2>/dev/null || { echo "go unknown"; exit 0; }

# gover FILE DEFAULT: major.minor of FILE's go line, or DEFAULT when it has none.
gover() {
  awk -v d="$2" '$1 == "go" { split($2, v, "."); print v[1] "." v[2] + 0; f = 1; exit }
    END { if (!f && d != "") print d }' "$1"
}

# findup NAME: nearest NAME in the current directory or its parents.
findup() {
  d=$(pwd -P)
  while :; do
    [ -f "$d/$1" ] && { echo "$d/$1"; return 0; }
    [ "$d" = / ] && return 1
    d=$(dirname -- "$d")
  done
}

[ -z "$ver" ] || ver=$(echo "$ver" | awk '{ sub(/^go/, ""); split($0, v, "."); if (v[1] ~ /^[0-9]+$/ && v[2] ~ /^[0-9]/) print v[1] "." v[2] + 0 }')
[ -n "$ver" ] || { f=$(findup go.mod) && ver=$(gover "$f" 1.16); }
[ -n "$ver" ] || { f=$(findup go.work) && ver=$(gover "$f" 1.18); }
[ -n "$ver" ] || ver=$(
  { git -c core.quotePath=false ls-files go.mod '*/go.mod' 2>/dev/null ||
      find . \( -name .git -o -name node_modules \) -prune -o -name go.mod -print; } |
    grep -Ev '(^|/)(testdata|vendor|node_modules)/' | tr '\n' '\0' |
    xargs -0 awk '$1 == "go" { split($2, v, "."); print v[1] "." v[2] + 0; nextfile }' 2>/dev/null |
    sort -t. -k1,1n -k2,2n | head -n 1
)
[ -n "$ver" ] || { echo "go unknown"; exit 0; }

echo "go $ver"
awk -v t="$ver" 'function n(s, a) { split(s, a, "."); return a[1] * 1000 + a[2] }
  /^[0-9]/ && n($1) <= n(t) { sub(/^[^ ]+ /, "- "); print }' "$data"
