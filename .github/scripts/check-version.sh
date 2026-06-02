#!/usr/bin/env bash
#
# check-version.sh — fail when a git tag does not match sdk/kronk.Version.
#
# Used by the release.yaml and docker.yml workflows on `push` of a `v*`
# tag to guarantee that whoever cut the tag also bumped the constant in
# sdk/kronk/kronk.go in the same commit. Both the release archives
# (goreleaser) and the published container images carry the constant as
# their reported version, so a mismatched tag would silently ship a
# binary that prints the previous release's version string.
#
# The tag is taken from $GITHUB_REF (refs/tags/vX.Y.Z) when running under
# GitHub Actions; otherwise pass the tag as the first positional arg.
# Tags must use the form `v<X.Y.Z>` — the leading `v` is stripped before
# comparison against the constant value.

set -euo pipefail

TAG="${1:-${GITHUB_REF:-}}"
TAG="${TAG#refs/tags/}"

if [[ -z "$TAG" ]]; then
    echo "::error::check-version.sh: no tag supplied (set \$GITHUB_REF or pass tag as arg 1)" >&2
    exit 1
fi

if [[ "$TAG" != v* ]]; then
    echo "::error::check-version.sh: tag '$TAG' does not start with 'v'" >&2
    exit 1
fi

EXPECTED="${TAG#v}"

# Pull the value out of `const Version = "x.y.z"` in sdk/kronk/kronk.go.
# Anchored to `^const Version` so a stray comment mentioning the symbol
# can never satisfy the match.
KRONK_GO="sdk/kronk/kronk.go"
ACTUAL="$(grep -E '^const Version = "[^"]+"' "$KRONK_GO" \
            | head -n1 \
            | sed -E 's/^const Version = "([^"]+)".*/\1/')"

if [[ -z "$ACTUAL" ]]; then
    echo "::error::check-version.sh: could not parse 'const Version' from $KRONK_GO" >&2
    exit 1
fi

if [[ "$ACTUAL" != "$EXPECTED" ]]; then
    cat >&2 <<EOF
::error::check-version.sh: tag/constant mismatch.
  git tag       : $TAG       (compared as: $EXPECTED)
  $KRONK_GO     : $ACTUAL

Bump the 'const Version' in $KRONK_GO to match the tag (or retag) and push again.
EOF
    exit 1
fi

echo "check-version.sh: OK ($KRONK_GO Version=$ACTUAL matches tag $TAG)"
