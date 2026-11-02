#!/bin/sh
# Usage: sh extract-findings.sh TRANSCRIPT RUN_DIR
# Saves the aggregator's findings.md to RUN_DIR/findings.md exactly as written, taken from its
# JSONL transcript (Claude Code's agent output_file) so the orchestrator never re-types it.
# Takes the text between the last "===== findings.md =====" and "===== end =====" markers;
# it must start with "# Kronk DiffGuard Report". Needs jq.
# Exit: 0 saved | 2 usage | 3 jq missing | 4 no valid findings.md in the transcript.

set -u

die() { code=$1; shift; echo "extract-findings: $*" >&2; exit "$code"; }

[ $# -eq 2 ] || die 2 "usage: extract-findings.sh TRANSCRIPT RUN_DIR"
transcript=$1 run=$2
[ -f "$transcript" ] || die 2 "no transcript at $transcript"
[ -d "$run" ] || die 2 "no run directory at $run"
command -v jq >/dev/null 2>&1 || die 3 "jq not found"

open='===== findings.md ====='
close='===== end ====='
tmp="$run/.findings.md.tmp"

# Every JSON string holding both markers, in transcript order; the last is the final reply
# (earlier ones are the role file the aggregator read). Lines that aren't JSON are skipped.
jq -rRn --arg o "$open" --arg c "$close" '
  [inputs | fromjson? | .. | strings | select(contains("\n" + $o + "\n") and contains("\n" + $c))]
  | last // empty
  | split("\n" + $o + "\n") | last
  | split("\n" + $c) | first
' "$transcript" > "$tmp"
EXIT_CODE=$?
[ $EXIT_CODE -eq 0 ] || { rm -f "$tmp"; die 4 "jq failed on $transcript (exit $EXIT_CODE)"; }

if [ "$(head -n 1 "$tmp")" != '# Kronk DiffGuard Report' ]; then
  rm -f "$tmp"
  die 4 "no findings.md between the markers in $transcript"
fi

mv "$tmp" "$run/findings.md" || die 4 "cannot write $run/findings.md"
echo "findings=$run/findings.md lines=$(wc -l < "$run/findings.md" | tr -d ' ')"
