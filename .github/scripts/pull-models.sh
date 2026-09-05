#!/usr/bin/env bash
#
# pull-models.sh — pull the models of one tier from a model list. Each line is
# `<backend> <model-id> [tier]`; tier defaults to base, as does the argument.

set -euo pipefail

list="${1:-}"
want="${2:-base}"

if [[ -z "$list" ]]; then
  echo "::error::usage: pull-models.sh <model-list> [tier]"
  exit 1
fi

if [[ ! -f "$list" ]]; then
  echo "::error::model list '$list' not found"
  exit 1
fi

echo "::group::$want models from $list"
while IFS= read -r line || [[ -n "$line" ]]; do
  # Strip comments / blanks.
  line="${line%%#*}"
  line="${line#"${line%%[![:space:]]*}"}"
  line="${line%"${line##*[![:space:]]}"}"
  [[ -z "$line" ]] && continue

  # shellcheck disable=SC2086  # intentional word-splitting on the spaces.
  set -- $line
  backend="$1"; model="$2"; tier="${3:-base}"

  # A typo'd tier belongs to no caller, so the model would silently never be
  # pulled by anyone. Fail on it the way an unknown backend fails.
  case "$tier" in
    base|gpu) ;;
    *)
      echo "::error file=${list}::Unknown tier '$tier' for '$model' (expected 'base' or 'gpu')"
      exit 1
      ;;
  esac
  [[ "$tier" == "$want" ]] || continue

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
