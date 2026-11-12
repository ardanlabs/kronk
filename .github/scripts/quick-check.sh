#!/usr/bin/env bash
# Artifact transfers stay outside the measured make target.
set -euo pipefail

platform=${QUICK_CHECK_PLATFORM:-macos-metal}
case "$platform" in
  macos-metal|linux-vulkan) ;;
  *) echo "Unknown quick-check platform: $platform" >&2; exit 2 ;;
esac
output=${QUICK_CHECK_OUTPUT:-.tools/quick-check/output/$platform}
history=.tools/quick-check/release-history-$platform.log
workflow=quick-check-$platform.yml
artifact_prefix=quick-check-$platform-run
diagnostics="${RUNNER_TEMP:?}/quick-check/$platform"
mkdir -p "$diagnostics" "$output"

die() { echo "$*" >&2; exit 1; }

# Restrict discovery to the nightly workflow, not similarly named artifacts from
# other workflows. Artifact IDs order reruns correctly; workflow head_sha does not
# identify the main revision actually checked out by a rerun.
artifacts() {
  local workflows workflow_id runs run pattern="$artifact_prefix"
  local include_legacy=true
  [[ "${1:-}" != current ]] || include_legacy=false
  # A renamed workflow can retain its ID, or leave the former workflow behind.
  # Read both for discovery, but pruning is restricted to the current workflow.
  # Never treat API failures as an empty artifact list.
  workflows=$(gh api --paginate "repos/$GITHUB_REPOSITORY/actions/workflows?per_page=100" \
    --jq ".workflows[] | select(.path == \".github/workflows/$workflow\" or ($include_legacy and \"$platform\" == \"macos-metal\" and .path == \".github/workflows/quick-check.yml\")) | .id") || return
  if [[ "$platform" == macos-metal && "$include_legacy" == true ]]; then pattern="($artifact_prefix|quick-check-run)"; fi
  for workflow_id in $workflows; do
    runs=$(gh api --paginate "repos/$GITHUB_REPOSITORY/actions/workflows/$workflow_id/runs?per_page=100" --jq '.workflow_runs[].id') || return
    for run in $runs; do
      gh api --paginate "repos/$GITHUB_REPOSITORY/actions/runs/$run/artifacts?per_page=100" \
        --jq ".artifacts[] | select(.expired == false and (.name | test(\"^$pattern-[0-9]+-[0-9]+$\"))) | .id" || return
    done
  done | sort -rnu
}

download() {
  local id=$1
  rm -rf "$diagnostics/evidence"
  mkdir -p "$diagnostics/evidence"
  gh api "repos/$GITHUB_REPOSITORY/actions/artifacts/$id/zip" > "$diagnostics/evidence.zip"
  unzip -q "$diagnostics/evidence.zip" -d "$diagnostics/evidence"
  tar -xzf "$diagnostics/evidence/quick-check-run.tgz" -C "$diagnostics/evidence"
  local candidates=("$diagnostics/evidence"/new-*)
  [[ ${#candidates[@]} == 1 && -d "${candidates[0]}" ]] || die "Artifact $id must contain exactly one finalized candidate"
  candidate=${candidates[0]}
  revision=$(jq -er '.server_revision | select(test("^([0-9a-f]{40}|[0-9a-f]{64})$"))' "$candidate/summary.json")
  jq -e '.status == "PASS" or .status == "FAIL" or .status == "INTERRUPTED"' "$candidate/summary.json" >/dev/null
  [[ -f "$candidate/events.ndjson" && -f "$candidate/tool.log" ]] || die "Incomplete evidence in artifact $id"
  validate_platform "$candidate/summary.json"
}

validate_platform() {
  local os=darwin identity=
  if [[ "$platform" == linux-vulkan ]]; then os=linux; identity=linux-vulkan-runner; fi
  # Failed/interrupted attempts can precede the first server metadata response.
  jq -e --arg os "$os" --arg identity "$identity" \
    '(.machine.os == null or .machine.os == $os) and (.benchmark_identity // "") == $identity' "$1" >/dev/null || die "Evidence does not belong to $platform"
}

case "${1:-}" in
  prune)
    # Called only after a successful upload. Discovery is scoped to this
    # platform's workflows and full run artifacts; never delete other evidence.
    ids=$(artifacts current)
    [[ "${UPLOADED_ARTIFACT_ID:-}" =~ ^[0-9]+$ ]] || die 'Pruning requires the successfully uploaded artifact ID'
    # Include the confirmed upload even if Actions artifact listing is delayed.
    ids=$(printf '%s\n%s\n' "$ids" "$UPLOADED_ARTIFACT_ID" | sort -rnu)
    kept=0
    for id in $ids; do
      kept=$((kept + 1))
      if (( kept > 2 )); then
        gh api --method DELETE "repos/$GITHUB_REPOSITORY/actions/artifacts/$id"
      fi
    done
    ;;
  prepare)
    ids=$(artifacts)
    latest=${ids%%$'\n'*}
    if [[ -n "$latest" ]]; then
      download "$latest"
      if [[ "$revision" == "$(git rev-parse HEAD)" ]]; then
        echo 'skip=true' >> "${GITHUB_OUTPUT:?}"
        printf 'Skipping already tested commit %s (artifact %s)\n' "$revision" "$latest"
        printf '## Quick-check skipped\n\nCommit `%s` matches finalized artifact %s.\n' "$revision" "$latest" >> "${GITHUB_STEP_SUMMARY:?}"
        exit 0
      fi
    fi
    echo 'skip=false' >> "${GITHUB_OUTPUT:?}"
    ;;
  package)
    candidates=("$output"/new-*)
    [[ ${#candidates[@]} == 1 && -f "${candidates[0]}/summary.json" ]] || die 'No single finalized candidate to preserve'
    jq -e '.status == "PASS" or .status == "FAIL" or .status == "INTERRUPTED"' "${candidates[0]}/summary.json" >/dev/null
    [[ -f "${candidates[0]}/events.ndjson" && -f "${candidates[0]}/tool.log" ]] || die 'Candidate evidence is incomplete'
    files=("${candidates[0]##*/}")
    for report in compare-release comparison.json comparison.log comparison.md; do
      [[ ! -e "$output/$report" ]] || files+=("$report")
    done
    COPYFILE_DISABLE=1 tar -czf "$RUNNER_TEMP/quick-check-run.tgz" -C "$output" "${files[@]}"
    ;;
  start)
    # Preserve the Pacha VM's API-only tools server and scoped Metal setup.
    if ! kronk server stop > "$diagnostics/server-stop-before.log" 2>&1; then
      if [[ -e "${KRONK_BASE_PATH:-$HOME/.kronk}/kronk.pid" ]]; then
        cat "$diagnostics/server-stop-before.log" >&2
        exit 1
      fi
      echo 'No existing detached server PID; starting a fresh server.'
    fi
    touch "$diagnostics/server-owned"
    make server-for-tools > "$diagnostics/server-start.log" 2>&1
    for ((attempt=0; attempt<60; attempt++)); do
      if curl --fail --silent --max-time 5 http://127.0.0.1:11435/v1/readiness >/dev/null; then
        exit 0
      fi
      sleep 2
    done
    die 'Server did not become ready'
    ;;
  compare)
    mkdir -p "$output/compare-release"
    # A failed comparison must never publish a previous invocation's report.
    for report in comparison.json comparison.log comparison.md; do
      rm -f "$output/$report" "$output/compare-release/$report"
    done
    status=0
    # No -release: the tracked main history is strictly a read-only baseline.
    go run ./.tools/quick-check compare-history -out "$output" -history "$history" > "$output/compare-release/terminal.log" 2>&1 || status=$?
    for report in comparison.json comparison.log comparison.md; do
      [[ ! -f "$output/$report" ]] || mv "$output/$report" "$output/compare-release/$report"
    done
    cat "$output/compare-release/terminal.log"
    if [[ -f "$output/compare-release/comparison.md" ]]; then
      printf '\n' >> "${GITHUB_STEP_SUMMARY:?}"
      cat "$output/compare-release/comparison.md" >> "$GITHUB_STEP_SUMMARY"
    else
      printf '\n### Release comparison\n\nNo comparison report could be generated (exit status %s). See the comparison step logs and run artifact for details.\n' "$status" >> "${GITHUB_STEP_SUMMARY:?}"
    fi
    exit "$status"
    ;;
  library)
    # Share read-only artifact discovery with the local release-history command.
    return 0
    ;;
  finish)
    if [[ -f "$diagnostics/server-owned" ]]; then
      if ! make kronk-server-stop > "$diagnostics/server-stop.log" 2>&1; then
        echo '::warning::Server cleanup failed'
        cat "$diagnostics/server-stop.log"
      fi
      if [[ -f "$HOME/.kronk/kronk.log" ]]; then
        cp "$HOME/.kronk/kronk.log" "$diagnostics/server.log"
      fi
    fi
    if [[ "${CHECK_OUTCOME:-}" != success ]]; then
      for log in "$diagnostics/server-stop-before.log" "$diagnostics/server-start.log" "$diagnostics/server.log"; do
        [[ ! -f "$log" ]] || tail -n 100 "$log"
      done
    fi
    {
      echo '## Quick-check'
      printf '\nTested commit: `%s`\n' "$(git rev-parse HEAD)"
      printf '\nRunner: `%s`; check step: **%s**\n' "$RUNNER_IDENTITY" "${CHECK_OUTCOME:-not run}"
    } >> "$GITHUB_STEP_SUMMARY"
    ;;
  *) echo "Usage: $0 prepare|package|start|compare|finish|prune" >&2; exit 2 ;;
esac
