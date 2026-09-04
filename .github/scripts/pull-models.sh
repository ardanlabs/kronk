#!/usr/bin/env bash
#
# pull-models.sh — pull every model named in the given list files. Each line
# is `<backend> <model-id>`; blank lines and `#` comments are ignored.

set -euo pipefail

lists=("$@")
if [[ "${#lists[@]}" -eq 0 ]]; then
  echo "::error::pull-models.sh needs at least one model list"
  exit 1
fi

for list in "${lists[@]}"; do
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
    # shellcheck disable=SC2086  # intentional word-splitting on the space.
    set -- $line
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
done
