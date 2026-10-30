#!/usr/bin/env bash
# CI orchestration only. The quick-check CLI and its retention rules own
# matrix execution, evidence validation, baseline roles, and comparison output.
set -euo pipefail

output=.tools/quick-check/output
history=.tools/quick-check/release-history.log
diagnostics="${RUNNER_TEMP:?}/quick-check"
mkdir -p "$diagnostics"

results_branch=quick-check-results
results="$RUNNER_TEMP/quick-check-results"

validate_queue() {
  # Go owns format/evidence validation, including selected-cell and failed runs.
  # Validate away from the restored queue; never replace full local evidence.
  local scratch
  scratch=$(mktemp -d "$diagnostics/validate.XXXXXX")
  if ! go run ./.tools/quick-check snapshot -out "$1" -dest "$scratch/queue"; then
    rm -rf "$scratch"
    return 1
  fi
  rm -rf "$scratch"
}

commit_results() {
  local path=$1 message=$2
  git -C "$results" add -- "$path"
  if git -C "$results" diff --cached --quiet; then
    echo "No changes to $path"
    return
  fi
  git -C "$results" -c user.name='github-actions[bot]' \
    -c user.email='41898282+github-actions[bot]@users.noreply.github.com' \
    commit -m "$message"
  # Never force-push or merge stale state. A competing writer fails safely.
  git -C "$results" push origin "HEAD:refs/heads/$results_branch"
  if [[ "$path" == "$history" ]]; then
    printf '\nRelease-history-only commit: `%s`.\n' "$(git -C "$results" rev-parse HEAD)" >> "${GITHUB_STEP_SUMMARY:?}"
  fi
}

case "${1:-}" in
  restore)
    # No artifact fallback: an absent/corrupt branch must not erase baselines.
    git fetch --depth=1 --no-tags origin "refs/heads/$results_branch:refs/remotes/origin/$results_branch"
    git worktree add --detach "$results" "refs/remotes/origin/$results_branch"
    [[ -d "$results/queue" ]] || { echo 'Missing saved queue' >&2; exit 1; }
    [[ ! -e "$output" ]] || { echo "Refusing to overwrite existing $output" >&2; exit 1; }
    mkdir -p "$(dirname "$output")"
    cp -R "$results/queue" "$output"
    rm -f "$history"
    if [[ -f "$results/$history" ]]; then
      cp "$results/$history" "$history"
    fi
    validate_queue "$output"
    # The queue push may have succeeded before its separate history push failed.
    if [[ -f "$output/release-history.log" ]]; then
      cp "$output/release-history.log" "$history"
      mkdir -p "$results/$(dirname "$history")"
      cp "$history" "$results/$history"
      commit_results "$history" 'Recover release history from saved queue'
    fi
    echo "Restored retained queue and history from $results_branch at $(git -C "$results" rev-parse HEAD)"
    ;;
  prepare)
    revision=$(git rev-parse HEAD)
    for summary in "$output"/new-*/comparison-snapshot.json; do
      [[ -f "$summary" ]] || continue
      previous=$(jq -er '.summary.server_revision' "$summary")
      if [[ "$previous" == "$revision" ]]; then
        echo 'skip=true' >> "${GITHUB_OUTPUT:?}"
        printf 'Skipping already tested commit %s\n' "$revision"
        printf '## Quick-check skipped\n\nCommit `%s` matches the latest retained candidate; no new test or upload.\n' "$revision" >> "${GITHUB_STEP_SUMMARY:?}"
        exit 0
      fi
    done
    echo 'skip=false' >> "${GITHUB_OUTPUT:?}"
    ;;
  save-queue)
    # Export completely before touching Git state. Never commit the full docs.
    stage=$(mktemp -d "$diagnostics/export.XXXXXX")
    go run ./.tools/quick-check snapshot -out "$output" -dest "$stage/queue"
    # Retain history with the queue so a retry can finish its later history commit.
    [[ ! -f "$history" ]] || cp "$history" "$stage/queue/release-history.log"
    rm -rf "$results/queue"
    mv "$stage/queue" "$results/queue"
    rmdir "$stage"
    commit_results queue "Rotate quick-check queue (Actions run ${GITHUB_RUN_ID:?})"
    ;;
  save-history)
    if [[ -f "$history" ]]; then
      mkdir -p "$results/$(dirname "$history")"
      cp "$history" "$results/$history"
      commit_results "$history" "Update release history for ${RELEASE:-release comparison}"
    fi
    ;;
  snapshot)
    runs=()
    for run in "$output"/new-* "$output"/old-* "$output"/release-* "$output"/[0-9]*T*Z-*; do
      [[ ! -d "$run" ]] || runs+=("${run##*/}")
    done
    [[ ${#runs[@]} -gt 0 ]] || { echo "No runs to preserve" >&2; exit 1; }
    [[ ! -f "$history" ]] || cp "$history" "$output/release-history.log"
    for report in compare-commit compare-release release-history.log; do
      [[ ! -e "$output/$report" ]] || runs+=("$report")
    done
    COPYFILE_DISABLE=1 tar -czf "$RUNNER_TEMP/quick-check-baselines.tgz" -C "$output" "${runs[@]}"
    ;;
  start)
    # The ephemeral VM belongs to this job; never operate on the physical host's
    # manually started server. Mark ownership before start so cleanup covers a
    # partial startup too. The make target compiles an API-only tools server.
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
    echo "Server did not become ready" >&2
    exit 1
    ;;
  compare)
    # Both comparisons must be attempted independently; a failed commit
    # comparison must not suppress the release comparison.
    rm -f "$diagnostics/release-history-before.log" "$output/release-history.log"
    if [[ -f "$history" ]]; then
      cp "$history" "$diagnostics/release-history-before.log"
    fi
    status=0
    for role in commit release; do
      prefix=old
      [[ "$role" != release ]] || prefix=release
      available=false
      for baseline in "$output/$prefix-"*; do
        [[ ! -d "$baseline" ]] || { available=true; break; }
      done
      if [[ "$available" == true ]]; then
        mkdir -p "$output/compare-$role"
        rm -f "$output/comparison.json" "$output/comparison.log"
        make "quick-compare-against-$role" \
          QUICK_CHECK_COMPARE_ARGS="-out $output" \
          QUICK_COMPARE_BASELINE="$baseline" \
          QUICK_COMPARE_CANDIDATE="$(find "$output" -maxdepth 1 -type d -name 'new-*' -print -quit)" \
          > "$output/compare-$role/terminal.log" 2>&1 || status=1
        for report in comparison.json comparison.log; do
          [[ ! -f "$output/$report" ]] || mv "$output/$report" "$output/compare-$role/$report"
        done
        cat "$output/compare-$role/terminal.log"
      else
        echo "Skipping $role comparison: no available baseline."
      fi
    done
    if [[ -f "$history" ]] && ! cmp -s "$history" "$diagnostics/release-history-before.log"; then
      cp "$history" "$output/release-history.log"
      echo 'Release history updated; its download copy is included in the queue artifact.'
    fi
    exit "$status"
    ;;
  promote)
    : "${RELEASE:?}" "${COMMIT:?}"
    rm -f "$diagnostics/release-history-before.log" "$output/release-history.log"
    if [[ -f "$history" ]]; then
      cp "$history" "$diagnostics/release-history-before.log"
    fi
    mkdir -p "$output/compare-release"
    rm -f "$output/comparison.json" "$output/comparison.log"
    # Go promotes retained evidence locally; no server, models,
    # GitHub requests, or transfers belong to this command.
    status=0
    go run ./.tools/quick-check promote -release "$RELEASE" -commit "$COMMIT" \
      > "$output/compare-release/terminal.log" 2>&1 || status=$?
    for report in comparison.json comparison.log; do
      [[ ! -f "$output/$report" ]] || mv "$output/$report" "$output/compare-release/$report"
    done
    cat "$output/compare-release/terminal.log"
    if (( status != 0 )); then exit "$status"; fi
    if [[ -f "$history" ]] && ! cmp -s "$history" "$diagnostics/release-history-before.log"; then
      cp "$history" "$output/release-history.log"
      echo 'Release history updated; its download copy is included in the queue artifact.'
    fi
    {
      echo '## Promote to Release'
      printf '\nTested SHA: `%s`\n\nIntended release: `%s`\n' "$COMMIT" "$RELEASE"
      echo
      echo 'Retained evidence promoted locally; no release was published or tagged.'
      echo
      echo 'The queue is saved to quick-check-results first; changed release history is saved in a separate later commit.'
    } >> "${GITHUB_STEP_SUMMARY:?}"
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
    # Full finalized run evidence is preserved by snapshot/upload separately.
    # Also print startup/cleanup diagnostics when the check could not complete.
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
  *) echo "Usage: $0 restore|prepare|save-queue|save-history|snapshot|start|compare|promote|finish" >&2; exit 2 ;;
esac
