#!/usr/bin/env bash
# Manual CI orchestration only. The quick-check CLI and its retention rules own
# matrix execution, evidence validation, baseline roles, and comparison output.
set -euo pipefail

output=.tools/quick-check/output
diagnostics="${RUNNER_TEMP:?}/quick-check"
mkdir -p "$diagnostics"

case "${1:-}" in
  restore)
    # Restrict discovery to this workflow. A failed/interrupted candidate is
    # retained alongside passing baselines. Ignore expired artifacts.
    runs=$(gh api --paginate "repos/$GITHUB_REPOSITORY/actions/workflows/quick-check.yml/runs?per_page=100" \
      --jq '.workflow_runs[].id')
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
      rm "$diagnostics/quick-check-baselines.tgz"
      rm "$diagnostics/baselines.zip"
      echo "Restored retained run queue from Actions artifact $artifact"
    else
      echo "No retained run queue; this run will establish one."
    fi
    ;;
  prune)
    # Upload must have succeeded before this command is invoked. Enumerate only
    # this workflow's queue artifacts, never artifacts belonging to other jobs.
    : "${KEEP_ARTIFACT_ID:?}"
    runs=$(gh api --paginate "repos/$GITHUB_REPOSITORY/actions/workflows/quick-check.yml/runs?per_page=100" \
      --jq '.workflow_runs[].id')
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
    COPYFILE_DISABLE=1 tar -czf "$RUNNER_TEMP/quick-check-baselines.tgz" -C "$output" "${runs[@]}"
    ;;
  start)
    # The ephemeral VM belongs to this job; never operate on the physical host's
    # manually started server. Mark ownership before start so cleanup covers a
    # partial startup too. The make target compiles an API-only tools server.
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
        make "quick-compare-against-$role" \
          QUICK_CHECK_COMPARE_ARGS="-out $output/compare-$role" \
          QUICK_COMPARE_BASELINE="$(dirname "$baseline")" \
          QUICK_COMPARE_CANDIDATE="$(dirname "$(find "$output" -path '*/new-*/summary.json' -print -quit)")" \
          > "$output/compare-$role/terminal.log" 2>&1 || status=1
        cat "$output/compare-$role/terminal.log"
      else
        echo "Skipping $role comparison: no available baseline."
      fi
    done
    exit "$status"
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
      for log in "$diagnostics/server-start.log" "$diagnostics/server.log"; do
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
  *) echo "Usage: $0 restore|prune|snapshot|start|compare|finish" >&2; exit 2 ;;
esac
