#!/usr/bin/env bash
#
# pull-models.sh — pull all models from a model list. Each line is
# `<backend> <model-id>`.

set -euo pipefail

list="${1:-}"

if [[ -z "$list" || "$#" -ne 1 ]]; then
  echo "::error::usage: pull-models.sh <model-list>"
  exit 1
fi

if [[ ! -f "$list" ]]; then
  echo "::error::model list '$list' not found"
  exit 1
fi

echo "::group::models from $list"
while IFS= read -r line || [[ -n "$line" ]]; do
  # Strip comments / blanks.
  line="${line%%#*}"
  line="${line#"${line%%[![:space:]]*}"}"
  line="${line%"${line##*[![:space:]]}"}"
  [[ -z "$line" ]] && continue

  # shellcheck disable=SC2086  # intentional word-splitting on the spaces.
  set -- $line
  if [[ "$#" -ne 2 ]]; then
    echo "::error file=${list}::Expected '<backend> <model-id>': $line"
    exit 1
  fi
  backend="$1"; model="$2"

  case "$backend" in
    kronk) kronk model pull --local "$model" ;;
    bucky) kronk bucky model pull --local "$model" ;;
    *)
      echo "::error file=${list}::Unknown backend '$backend' (expected 'kronk' or 'bucky')"
      exit 1
      ;;
  esac
done <"$list"
echo "::endgroup::"
