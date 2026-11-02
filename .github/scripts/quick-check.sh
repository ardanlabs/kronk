#!/usr/bin/env bash
# Transfers and Git persistence stay outside the measured make target.
set -euo pipefail

output=.tools/quick-check/output
history=.tools/quick-check/release-history.log
diagnostics="${RUNNER_TEMP:?}/quick-check"
results="$diagnostics/results"
mkdir -p "$diagnostics" "$output"

die() { echo "$*" >&2; exit 1; }

# Restrict discovery to the nightly workflow, not similarly named artifacts from
# other workflows. Artifact IDs order reruns correctly; workflow head_sha does not
# identify the main revision actually checked out by a rerun.
artifacts() {
  local runs run
  runs=$(gh api --paginate "repos/$GITHUB_REPOSITORY/actions/workflows/quick-check.yml/runs?per_page=100" --jq '.workflow_runs[].id') || return
  for run in $runs; do
    gh api --paginate "repos/$GITHUB_REPOSITORY/actions/runs/$run/artifacts?per_page=100" \
      --jq '.artifacts[] | select(.expired == false and (.name | test("^quick-check-run-[0-9]+-[0-9]+$"))) | .id' || return
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
}

# A detached, disposable worktree never checks out or resets a developer's local
# quick-check-results branch. Only exit 2 from ls-remote means genuinely absent.
open_results() {
  local status=0
  git ls-remote --exit-code --heads origin refs/heads/quick-check-results > "$diagnostics/results-ref" || status=$?
  if (( status == 2 )); then
    return 2
  elif (( status != 0 )); then
    die "Cannot read quick-check-results (Git exit $status)"
  fi
  git fetch --no-tags origin refs/heads/quick-check-results || die 'Cannot fetch quick-check-results'
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
  fi
}

push_results() {
  git -C "$results" push origin HEAD:refs/heads/quick-check-results
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
    for report in compare-release comparison.json comparison.log; do
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
      exit 0
    fi
    mkdir -p "$output/compare-release"
    status=0
    go run ./.tools/quick-check compare -out "$output" > "$output/compare-release/terminal.log" 2>&1 || status=$?
    for report in comparison.json comparison.log; do
      [[ ! -f "$output/$report" ]] || mv "$output/$report" "$output/compare-release/$report"
    done
    cat "$output/compare-release/terminal.log"
    exit "$status"
    ;;
  promote)
    : "${RELEASE:?}" "${COMMIT:?}"
    [[ "$COMMIT" =~ ^([0-9a-f]{40}|[0-9a-f]{64})$ ]] || die 'COMMIT must be a full tested SHA'
    [[ "$RELEASE" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]] || die 'Invalid release label'
    trap close_results EXIT
    status=0
    open_results || status=$?
    if (( status == 2 )); then
      git worktree add --detach "$results" HEAD
      orphan="quick-check-init-${GITHUB_RUN_ID:?}-${GITHUB_RUN_ATTEMPT:?}"
      git -C "$results" switch --orphan "$orphan"
      rm -f "$history"
    fi
    if (( status != 2 )); then restore_release; fi
    save_history
    # Search newest artifact ID first, including reruns; inspect actual evidence,
    # not the workflow event's head SHA. Expired/missing evidence is not promotable.
    ids=$(artifacts)
    found=false
    for id in $ids; do
      download "$id"
      if [[ "$revision" == "$COMMIT" ]]; then
        cp -R "$candidate" "$output/"
        found=true
        break
      fi
    done
    [[ "$found" == true ]] || die "No available nightly artifact for tested SHA $COMMIT"
    go run ./.tools/quick-check promote -out "$output" -release "$RELEASE" -commit "$COMMIT"
    # Export just the promoted run; never put full nightly evidence in Git.
    mkdir -p "$diagnostics/export-input"
    cp -R "$output/release-$RELEASE" "$diagnostics/export-input/"
    go run ./.tools/quick-check snapshot -out "$diagnostics/export-input" -dest "$diagnostics/compact"
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
    printf '## Promote to Release\n\nTested SHA: `%s`\n\nRelease baseline: `%s`\n\nSaved compact baseline and separate release history on `quick-check-results`; no tag or release published.\n' "$COMMIT" "$RELEASE" >> "${GITHUB_STEP_SUMMARY:?}"
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
      printf '\nOther workflows or host workloads may affect performance.\n'
    } >> "$GITHUB_STEP_SUMMARY"
    ;;
  *) echo "Usage: $0 restore|prepare|package|start|compare|promote|finish" >&2; exit 2 ;;
esac
