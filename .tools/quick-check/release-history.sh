#!/usr/bin/env bash
# Download recorded evidence and update local release history; never publish it.
set -euo pipefail

die() { echo "$*" >&2; exit 1; }
[[ $# -ge 1 && $# -le 3 ]] || die "Usage: bash $0 RELEASE [TESTED_COMMIT [BASELINE]]"
release=$1
[[ "$release" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]] || die 'Release must be a vX.Y.Z label'
commit=$(git rev-parse --verify "${2:-HEAD}^{commit}")
baseline=${3:-}
if [[ -n "$baseline" ]]; then
  [[ "$baseline" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]] || die 'Baseline must be a vX.Y.Z label'
fi
for tool in gh jq unzip go; do command -v "$tool" >/dev/null || die "Required tool not found: $tool"; done
GITHUB_REPOSITORY=${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}
export GITHUB_REPOSITORY

mkdir -p .tools/quick-check/output
work=$(mktemp -d .tools/quick-check/output/.release-history-XXXXXX)
trap 'rm -rf "$work"' EXIT
printf 'Preparing release %s from measured server commit %s\n' "$release" "$commit"

for platform in macos-metal linux-vulkan; do
  (
    export QUICK_CHECK_PLATFORM=$platform RUNNER_TEMP=$work
    export QUICK_CHECK_OUTPUT=$work/$platform/queue
    source .github/scripts/quick-check.sh library
    staged_history=$work/$platform/history.log
    # Tracked platform histories are the sole source of baseline timings.
    cp "$history" "$staged_history"
    # Select by measured server SHA, not workflow head SHA; newest evidence wins,
    # including failed attempts (the comparison command must reject those).
    ids=$(artifacts)
    for id in $ids; do
      download "$id"
      if [[ "$revision" == "$commit" ]]; then
        cp -R "$candidate" "$output/new-$commit"
        break
      fi
    done
    [[ -d "$output/new-$commit" ]] || die "No available $platform nightly artifact for tested SHA $commit"
    args=(-out "$output" -history "$staged_history" -release "$release")
    [[ -z "$baseline" ]] || args+=(-baseline "$baseline")
    go run ./.tools/quick-check compare-history "${args[@]}"
    [[ -s "$staged_history" ]] || die "No $platform release history generated"
  )
done

# Neither project history changes unless both platforms successfully validate.
for platform in macos-metal linux-vulkan; do
  cp "$work/$platform/history.log" ".tools/quick-check/release-history-$platform.log"
  echo "Updated .tools/quick-check/release-history-$platform.log"
done
echo 'Review the local history files and include them in your release commit. No tests rerun, commits created, or remote branches changed.'
