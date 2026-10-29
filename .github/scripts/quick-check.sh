#!/usr/bin/env bash
# Manual CI orchestration only. The quick-check CLI and its retention rules own
# matrix execution, evidence validation, baseline roles, and comparison output.
set -euo pipefail

output=.tools/quick-check/output
history=.tools/quick-check/release-history.log
diagnostics="${RUNNER_TEMP:?}/quick-check"
mkdir -p "$diagnostics"

# Resolve numeric IDs, including the current workflow before it is registered on
# main. Never trust a name or artifact prefix alone to establish queue ownership.
queue_runs() {
  local workflows current workflow path
  workflows=$(gh api --paginate "repos/$GITHUB_REPOSITORY/actions/workflows?per_page=100" \
    --jq '.workflows[] | select(.path == ".github/workflows/quick-check.yml" or .path == ".github/workflows/promote-to-release.yml") | .id') || return
  current=$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/${GITHUB_RUN_ID:?}" --jq '.workflow_id') || return
  workflows=$(printf '%s\n%s\n' "$workflows" "$current" | sort -nu) || return
  while IFS= read -r workflow; do
    [[ -n "$workflow" ]] || continue
    path=$(gh api "repos/$GITHUB_REPOSITORY/actions/workflows/$workflow" --jq '.path') || return
    case "$path" in
      .github/workflows/quick-check.yml|.github/workflows/promote-to-release.yml)
        gh api --paginate "repos/$GITHUB_REPOSITORY/actions/workflows/$workflow/runs?per_page=100" \
          --jq '.workflow_runs[].id' || return
        ;;
      *) echo "Refusing queue discovery for workflow $workflow ($path)" >&2; return 1 ;;
    esac
  done <<< "$workflows"
}

case "${1:-}" in
  restore)
    # Both workflows share one queue. A failed/interrupted candidate is retained
    # alongside passing baselines. Ignore expired artifacts.
    runs=$(queue_runs)
    artifact=0
    while IFS= read -r run; do
      [[ -n "$run" ]] || continue
      artifacts=$(gh api --paginate "repos/$GITHUB_REPOSITORY/actions/runs/$run/artifacts?per_page=100" \
        --jq '.artifacts[] | select((.name | startswith("quick-check-queue-")) and .expired == false) | .id')
      while IFS= read -r id; do
        [[ -n "$id" ]] || continue
        # A rerun of an older workflow run can upload the newest queue.
        if (( id > artifact )); then artifact=$id; fi
      done <<< "$artifacts"
    done <<< "$runs"
    if (( artifact > 0 )); then
      gh api "repos/$GITHUB_REPOSITORY/actions/artifacts/$artifact/zip" > "$diagnostics/baselines.zip"
      mkdir -p "$output"
      unzip -q "$diagnostics/baselines.zip" -d "$diagnostics"
      tar -xzf "$diagnostics/quick-check-baselines.tgz" -C "$output"
      # Comparisons belong only to the candidate that produced them. History
      # starts from the committed canonical file, never an old download copy.
      rm -rf "$output/compare-commit" "$output/compare-release"
      rm -f "$output/release-history.log"
      rm "$diagnostics/quick-check-baselines.tgz"
      rm "$diagnostics/baselines.zip"
      echo "Restored retained run queue from Actions artifact $artifact"
    else
      echo "No retained run queue; this run will establish one."
    fi
    ;;
  prepare)
    revision=$(git rev-parse HEAD)
    for summary in "$output"/new-*/summary.json; do
      [[ -f "$summary" ]] || continue
      previous=$(jq -er '.server_revision' "$summary")
      if [[ "$previous" == "$revision" ]]; then
        echo 'skip=true' >> "${GITHUB_OUTPUT:?}"
        printf 'Skipping already tested commit %s\n' "$revision"
        printf '## Quick-check skipped\n\nCommit `%s` matches the latest retained candidate; no new test or upload.\n' "$revision" >> "${GITHUB_STEP_SUMMARY:?}"
        exit 0
      fi
    done
    echo 'skip=false' >> "${GITHUB_OUTPUT:?}"
    ;;
  prune)
    # Upload must have succeeded before this command is invoked. Enumerate only
    # the two approved workflows' queue artifacts, never unrelated artifacts.
    : "${KEEP_ARTIFACT_ID:?}"
    runs=$(queue_runs)
    while IFS= read -r run; do
      [[ -n "$run" ]] || continue
      artifacts=$(gh api --paginate "repos/$GITHUB_REPOSITORY/actions/runs/$run/artifacts?per_page=100" \
        --jq '.artifacts[] | select(.name | startswith("quick-check-queue-")) | .id')
      while IFS= read -r artifact; do
        [[ -n "$artifact" && "$artifact" != "$KEEP_ARTIFACT_ID" ]] || continue
        gh api --method DELETE "repos/$GITHUB_REPOSITORY/actions/artifacts/$artifact"
      done <<< "$artifacts"
    done <<< "$runs"
    ;;
  snapshot)
    runs=()
    for run in "$output"/new-* "$output"/old-* "$output"/release-*; do
      [[ ! -f "$run/summary.json" ]] || runs+=("${run##*/}")
    done
    [[ ${#runs[@]} -gt 0 ]] || { echo "No runs to preserve" >&2; exit 1; }
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
      for baseline in "$output/$prefix-"*/summary.json; do
        [[ ! -f "$baseline" ]] || available=true
      done
      if [[ "$available" == true ]]; then
        mkdir -p "$output/compare-$role"
        rm -f "$output/comparison.json" "$output/comparison.log"
        make "quick-compare-against-$role" \
          QUICK_CHECK_COMPARE_ARGS="-out $output" \
          QUICK_COMPARE_BASELINE="$(dirname "$baseline")" \
          QUICK_COMPARE_CANDIDATE="$(dirname "$(find "$output" -path '*/new-*/summary.json' -print -quit)")" \
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
      echo 'If changed, download release-history.log from the queue artifact, review it, and manually commit it to .tools/quick-check/release-history.log.'
      echo 'The committed canonical history is the starting point for every invocation; this workflow does not commit or push updates.'
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
  *) echo "Usage: $0 restore|prepare|prune|snapshot|start|compare|promote|finish" >&2; exit 2 ;;
esac
