#!/usr/bin/env bash
#
# check-go-version.sh — fail when go.mod and .go-version disagree on the
# Go minor version.
#
# Contract:
#   go.mod's `go` directive      — minimum language version the code
#                                  uses (e.g. "go 1.26.0"). Sets the
#                                  floor for downstream consumers.
#   .go-version                  — exact toolchain CI installs and
#                                  what asdf/mise/goenv/gvm pick up
#                                  for contributors (e.g. "1.26.3").
#
# The two are allowed to differ on the *patch* component (and should:
# CI typically pins a patched toolchain ahead of the minimum). They
# must agree on `<major>.<minor>` — a drift there means somebody bumped
# one file and forgot the other, and CI would silently build with a
# toolchain older or newer than the code declares it needs.
#
# Run from the repo root (CI) or anywhere (script self-locates).
#
# Exits non-zero on mismatch, with a GitHub-Actions ::error annotation.
#
# Invocation:
#   ./.github/scripts/check-go-version.sh

set -euo pipefail

cd "$(dirname "$0")/../.."

GOMOD_FILE="go.mod"
GOVERSION_FILE=".go-version"

# Pull the version out of `go X.Y[.Z]` in go.mod. Anchored to the start
# of a line so a `// go 1.99` comment elsewhere can't satisfy the match.
GOMOD_RAW="$(awk '/^go [0-9]/ {print $2; exit}' "$GOMOD_FILE")"
GOVERSION_RAW="$(tr -d '[:space:]' <"$GOVERSION_FILE")"

if [[ -z "$GOMOD_RAW" ]]; then
    echo "::error::check-go-version.sh: could not parse 'go' directive from $GOMOD_FILE" >&2
    exit 1
fi
if [[ -z "$GOVERSION_RAW" ]]; then
    echo "::error::check-go-version.sh: $GOVERSION_FILE is empty" >&2
    exit 1
fi

# Minor = first two dot-separated components ("1.26" from "1.26.3").
GOMOD_MINOR="$(echo "$GOMOD_RAW" | cut -d. -f1,2)"
GOVERSION_MINOR="$(echo "$GOVERSION_RAW" | cut -d. -f1,2)"

if [[ "$GOMOD_MINOR" != "$GOVERSION_MINOR" ]]; then
    cat >&2 <<EOF
::error::check-go-version.sh: Go minor-version drift between $GOMOD_FILE and $GOVERSION_FILE.
  $GOMOD_FILE       : go $GOMOD_RAW       (minor: $GOMOD_MINOR)
  $GOVERSION_FILE   : $GOVERSION_RAW       (minor: $GOVERSION_MINOR)

Bump both files together so CI's toolchain matches the minimum language
version declared in $GOMOD_FILE. Patch differences are fine; minor
differences are not.
EOF
    exit 1
fi

echo "check-go-version.sh: OK ($GOMOD_FILE=go $GOMOD_RAW, $GOVERSION_FILE=$GOVERSION_RAW, minor $GOMOD_MINOR matches)"
