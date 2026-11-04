#!/usr/bin/env bash
# Transfers and Git persistence stay outside the measured make target.
set -euo pipefail

platform=${QUICK_CHECK_PLATFORM:-macos-metal}
case "$platform" in
  macos-metal|linux-vulkan) ;;
  *) echo "Unknown quick-check platform: $platform" >&2; exit 2 ;;
esac
output=.tools/quick-check/output/$platform
history=.tools/quick-check/release-history-$platform.log
workflow=quick-check-$platform.yml
results_branch=quick-check-results-$platform
artifact_prefix=quick-check-$platform-run
diagnostics="${RUNNER_TEMP:?}/quick-check/$platform"
results="$diagnostics/results"
mkdir -p "$diagnostics" "$output"

die() { echo "$*" >&2; exit 1; }

# Restrict discovery to the nightly workflow, not similarly named artifacts from
# other workflows. Artifact IDs order reruns correctly; workflow head_sha does not
# identify the main revision actually checked out by a rerun.
artifacts() {
  local workflows workflow_id runs run pattern="$artifact_prefix"
  # A renamed workflow can retain its ID, or leave the former workflow behind.
  # Discover both without treating API failures as an empty artifact list.
  workflows=$(gh api --paginate "repos/$GITHUB_REPOSITORY/actions/workflows?per_page=100" \
    --jq ".workflows[] | select(.path == \".github/workflows/$workflow\" or (\"$platform\" == \"macos-metal\" and .path == \".github/workflows/quick-check.yml\")) | .id") || return
  if [[ "$platform" == macos-metal ]]; then pattern="($artifact_prefix|quick-check-run)"; fi
  for workflow_id in $workflows; do
    runs=$(gh api --paginate "repos/$GITHUB_REPOSITORY/actions/workflows/$workflow_id/runs?per_page=100" --jq '.workflow_runs[].id') || return
    for run in $runs; do
      gh api --paginate "repos/$GITHUB_REPOSITORY/actions/runs/$run/artifacts?per_page=100" \
        --jq ".artifacts[] | select(.expired == false and (.name | test(\"^$pattern-[0-9]+-[0-9]+$\"))) | .id" || return
    done
  done | sort -rn
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

# A detached, disposable worktree never checks out or resets a developer's local
# quick-check-results branch. Only exit 2 from ls-remote means genuinely absent.
open_results() {
  local status=0
  local read_branch=$results_branch
  legacy_results=false
  git ls-remote --exit-code --heads origin "refs/heads/$results_branch" > "$diagnostics/results-ref" || status=$?
  # Until the operator migrates the macOS branch, nightlies may read the old one.
  # Promotion must not silently initialize a replacement without its history.
  if (( status == 2 )) && [[ "$platform" == macos-metal ]]; then
    status=0
    read_branch=quick-check-results
    git ls-remote --exit-code --heads origin "refs/heads/$read_branch" > "$diagnostics/results-ref" || status=$?
    if (( status == 0 )); then legacy_results=true; fi
  fi
  if (( status == 2 )); then
    return 2
  elif (( status != 0 )); then
    die "Cannot read $results_branch (Git exit $status)"
  fi
  git fetch --no-tags origin "refs/heads/$read_branch" || die "Cannot fetch $read_branch"
  git worktree add --detach "$results" FETCH_HEAD || die 'Cannot open results checkout'
}

close_results() {
  if [[ -d "$results" ]]; then git worktree remove --force "$results"; fi
  if [[ -n "${orphan:-}" ]]; then git branch -D "$orphan"; fi
}

restore_release() {
  local label
  [[ -f "$results/release/comparison-snapshot.json" ]] || die 'Results branch has no release snapshot'
  label=$(jq -er '.release | select(test("^v[0-9]+\\.[0-9]+\\.[0-9]+([-+][0-9A-Za-z.-]+)?$"))' "$results/release/promotion.json")
  jq -e --slurpfile promotion "$results/release/promotion.json" \
    '.version == 1 and (.summary.server_revision == $promotion[0].server_revision) and (.cells | length > 0)' \
    "$results/release/comparison-snapshot.json" >/dev/null
  jq '.summary' "$results/release/comparison-snapshot.json" > "$diagnostics/release-summary.json"
  validate_platform "$diagnostics/release-summary.json"
  mkdir -p "$output/release-$label"
  cp -R "$results/release/." "$output/release-$label/"
  rm -f "$output/release-$label/pending-history.log" "$history"
  # Let Go verify snapshot checksums and cross-field evidence before any server
  # measurement (and before retrying a pending history write during promotion).
  go run ./.tools/quick-check snapshot -out "$output" -dest "$diagnostics/validated-release"
  # A baseline push may have succeeded before its separate history push failed.
  if [[ -f "$results/release/pending-history.log" ]]; then
    cp "$results/release/pending-history.log" "$history"
  elif [[ -f "$results/$history" ]]; then
    cp "$results/$history" "$history"
  elif [[ "$platform" == macos-metal && -f "$results/.tools/quick-check/release-history.log" ]]; then
    cp "$results/.tools/quick-check/release-history.log" "$history"
  fi
}

push_results() {
  git -C "$results" push origin "HEAD:refs/heads/$results_branch"
}

commit_results() {
  git -C "$results" -c user.name='github-actions[bot]' \
    -c user.email='41898282+github-actions[bot]@users.noreply.github.com' commit -m "$1"
}

# Baseline commits carry recovery history until the next baseline replaces it.
# Copy, rather than move, so the later commit changes only the canonical document
# at the same path as main and can be cherry-picked without baseline files.
save_history() {
  if [[ -f "$results/release/pending-history.log" ]]; then
    mkdir -p "$results/$(dirname "$history")"
    cp "$results/release/pending-history.log" "$results/$history"
    git -C "$results" add "$history"
    if ! git -C "$results" diff --cached --quiet; then
      commit_results 'Save quick-check release history'
      push_results
    fi
  fi
}

promotion_args() {
  : "${RELEASE:?}" "${COMMIT:?}"
  [[ "$COMMIT" =~ ^([0-9a-f]{40}|[0-9a-f]{64})$ ]] || die 'COMMIT must be a full tested SHA'
  [[ "$RELEASE" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]] || die 'Invalid release label'
}

case "${1:-}" in
  restore)
    trap close_results EXIT
    status=0
    open_results || status=$?
    if (( status == 2 )); then
      rm -f "$history"
      echo 'No results branch; the first nightly has no release baseline.'
    else
      restore_release
    fi
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
    baselines=("$output"/release-*)
    if [[ ! -d "${baselines[0]}" ]]; then
      echo 'Skipping release comparison: no release baseline.'
      printf '\n### Release comparison\n\nSkipped: no release baseline is available.\n' >> "${GITHUB_STEP_SUMMARY:?}"
      exit 0
    fi
    mkdir -p "$output/compare-release"
    status=0
    go run ./.tools/quick-check compare -out "$output" -history "$history" > "$output/compare-release/terminal.log" 2>&1 || status=$?
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
  stage-promotion)
    promotion_args
    trap close_results EXIT
    status=0
    open_results || status=$?
    [[ "$legacy_results" == false ]] || die 'Migrate quick-check-results to quick-check-results-macos-metal and rename its history document before promotion'
    if (( status == 2 )); then
      rm -f "$history"
      echo absent > "$diagnostics/staged-ref"
    else
      git -C "$results" rev-parse HEAD > "$diagnostics/staged-ref"
      restore_release
    fi
    found=false
    # Already-published baselines remain retryable after Actions evidence expires.
    if [[ -f "$output/release-$RELEASE/promotion.json" ]] && \
      jq -e --arg commit "$COMMIT" '.server_revision == $commit' "$output/release-$RELEASE/promotion.json" >/dev/null; then
      found=true
    else
      ids=$(artifacts)
      for id in $ids; do
        download "$id"
        if [[ "$revision" == "$COMMIT" ]]; then
          cp -R "$candidate" "$output/"
          found=true
          break
        fi
      done
    fi
    [[ "$found" == true ]] || die "No available $platform nightly artifact for tested SHA $COMMIT"
    go run ./.tools/quick-check promote -out "$output" -release "$RELEASE" -commit "$COMMIT" -history "$history"
    # Export just the promoted run; never put full nightly evidence in Git.
    mkdir -p "$diagnostics/export-input"
    cp -R "$output/release-$RELEASE" "$diagnostics/export-input/"
    go run ./.tools/quick-check snapshot -out "$diagnostics/export-input" -dest "$diagnostics/compact"
    printf '%s %s\n' "$RELEASE" "$COMMIT" > "$diagnostics/staged-promotion"
    ;;
  publish-promotion)
    promotion_args
    [[ -f "$diagnostics/staged-promotion" && "$(cat "$diagnostics/staged-promotion")" == "$RELEASE $COMMIT" ]] || die 'Promotion must be staged before publishing'
    trap close_results EXIT
    status=0
    open_results || status=$?
    current=absent
    if (( status != 2 )); then current=$(git -C "$results" rev-parse HEAD); fi
    [[ "$current" == "$(cat "$diagnostics/staged-ref")" ]] || die "$results_branch changed after preparation; rerun promotion"
    if (( status == 2 )); then
      git worktree add --detach "$results" HEAD
      orphan="quick-check-init-$platform-${GITHUB_RUN_ID:?}-${GITHUB_RUN_ATTEMPT:?}"
      git -C "$results" switch --orphan "$orphan"
    fi
    rm -rf "$results/release"
    cp -R "$diagnostics/compact/release-$RELEASE" "$results/release"
    if [[ -f "$history" ]]; then
      cp "$history" "$results/release/pending-history.log"
    fi
    git -C "$results" add release
    if ! git -C "$results" diff --cached --quiet; then
      commit_results "Promote quick-check $COMMIT to $RELEASE"
      push_results
    fi
    save_history
    history_commit=$(git -C "$results" log -1 --format=%H -- "$history")
    printf '## Promote to Release — %s\n\nTested SHA: `%s`\n\nRelease baseline: `%s`\n\nSaved compact baseline on `%s`. History commit to cherry-pick manually: `%s` (`%s`). No tag or release published.\n' "$platform" "$COMMIT" "$RELEASE" "$results_branch" "${history_commit:-none (no history entry)}" "$history" >> "${GITHUB_STEP_SUMMARY:?}"
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
  *) echo "Usage: $0 restore|prepare|package|start|compare|stage-promotion|publish-promotion|finish" >&2; exit 2 ;;
esac
