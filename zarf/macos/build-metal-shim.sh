#!/usr/bin/env bash
# =============================================================================
# build-metal-shim.sh
#
# Build cua's Lume Metal capability shim on this host and stage it where the
# ephemeral macOS runner VMs can reach it over virtiofs.
#
#   ./build-metal-shim.sh                 # clone, build, verify, stage
#   CUA_REF=<sha> ./build-metal-shim.sh   # pin a known-good upstream revision
#   STAGE_DIR=~/ci-cache/kronk-tools ./build-metal-shim.sh
#
# Run as the user that owns the VMs (`pacha`). Needs Xcode Command Line Tools.
#
# WHAT IT IS
#
# A process-scoped dylib, injected with DYLD_INSERT_LIBRARIES, that raises the
# Metal GPU family a guest process sees. GPU work still goes through Apple's
# paravirtual path — nothing is passed through, no kernel or host binary is
# patched, and removing the environment variables removes the effect entirely.
#
# It is the guest half of the pair; pvg-feature-level.sh is the host half, and
# that one is worth trying alone first. See README.md for the full rationale and
# the ordering.
#
# WHY BUILD IT HERE RATHER THAN SHIP A BINARY
#
# Upstream deliberately commits no binaries: libs/lume/metal-capability-shim/
# Release/ holds only PROVENANCE.md and SHA256SUMS. Their evidence-matched
# release used Command Line Tools 26.4. This host runs 26.6, so the bytes will
# not match their manifest and `Scripts/verify.sh --no-build` provenance-matching
# does not apply — build from source and record your own hashes, which is what
# the provenance file written below is for.
#
# Host CLT 26.6.0 and the guest's Xcode 26.6 carry the same compiler (Apple clang
# 21.0.0, clang-2100.1.1.101), so a host build is what the guest would have
# produced. That is why this does not build inside the VM: the VMs are ephemeral
# and would repeat the work every job for an identical artifact.
#
# WHY arm64 AND NOT arm64e
#
# The consumer is a Go test binary. Go emits plain arm64, so the arm64 dylib is
# the one that loads. Both are built; only arm64 is staged by default.
#
# CAVEATS THAT BELONG IN YOUR HEAD, NOT JUST IN THE README
#
#   - Upstream calls this experimental and version-sensitive: it leans on private
#     Metal implementation details that Apple may change in any macOS release.
#   - It fails OPEN. A missing hook or an absent/malformed LUME_METAL_APPLE_
#     FAMILY_MAX leaves the process untouched, so CI silently drops back to the
#     Apple 5 fallback path instead of failing. Assert the family in the job.
#   - Never advertise MTLGPUFamilyMetal3. Upstream is explicit: MLX-LM uses that
#     answer to select residency sets the paravirtual device cannot create. It
#     also makes Metal3 the one capability probe the shim cannot fool, which is
#     what makes it a usable "is this real hardware" gate in CI.
#   - A reported capability is not a working capability. Failures under the shim
#     are ambiguous between a paravirtual driver gap and a real kronk bug.
# =============================================================================

set -euo pipefail

readonly REPO="https://github.com/trycua/cua.git"
readonly SUBDIR="libs/lume/metal-capability-shim"
readonly DYLIB="LumeMetalCapabilities-arm64.dylib"

CUA_REF="${CUA_REF:-main}"
# STAGE_DIR is mounted into the guests, so it holds the artifact only; the
# upstream checkout lives in WORK_DIR, outside it.
STAGE_DIR="${STAGE_DIR:-$HOME/ci-cache/kronk-tools}"
WORK_DIR="${WORK_DIR:-$HOME/.cache/kronk-metal-shim}"

if [[ "$(uname -m)" != "arm64" ]]; then
    echo "this must run on Apple Silicon, got $(uname -m)" >&2
    exit 1
fi

if ! xcode-select -p >/dev/null 2>&1; then
    echo "Xcode Command Line Tools are required: xcode-select --install" >&2
    exit 1
fi

mkdir -p "$WORK_DIR" "$STAGE_DIR"

# A shallow clone is enough to build, but the resolved SHA is recorded below:
# "main" is a moving target and the provenance file is the only thing that will
# tell a future reader which source produced the staged binary.
if [[ -d "$WORK_DIR/cua/.git" ]]; then
    echo "updating existing checkout in $WORK_DIR/cua"
    git -C "$WORK_DIR/cua" fetch --depth 1 origin "$CUA_REF"
    git -C "$WORK_DIR/cua" checkout --detach FETCH_HEAD
else
    echo "cloning $REPO at $CUA_REF"
    git clone --depth 1 --branch "$CUA_REF" "$REPO" "$WORK_DIR/cua" 2>/dev/null \
        || git clone --depth 1 "$REPO" "$WORK_DIR/cua"
fi

src="$WORK_DIR/cua/$SUBDIR"
if [[ ! -x "$src/Scripts/build.sh" ]]; then
    echo "upstream layout changed: $src/Scripts/build.sh is missing" >&2
    exit 1
fi

resolved_ref="$(git -C "$WORK_DIR/cua" rev-parse HEAD)"

echo "building..."
( cd "$src" && ./Scripts/build.sh && ./Scripts/verify.sh )

if [[ ! -f "$src/dist/$DYLIB" ]]; then
    echo "build produced no $DYLIB under $src/dist" >&2
    exit 1
fi

cp "$src/dist/$DYLIB" "$STAGE_DIR/$DYLIB"
shasum -a 256 "$STAGE_DIR/$DYLIB" > "$STAGE_DIR/SHA256SUMS"

cat > "$STAGE_DIR/PROVENANCE.txt" <<PROV
Lume Metal capability shim, built for the kronk macOS CI runners.

built    : $(date -u +%Y-%m-%dT%H:%M:%SZ)
host     : $(sw_vers -productName) $(sw_vers -productVersion) ($(sw_vers -buildVersion)), $(uname -m)
compiler : $(clang --version | head -1)
sdk      : $(xcode-select -p)
source   : $REPO
revision : $resolved_ref
subdir   : $SUBDIR
artifact : $(cat "$STAGE_DIR/SHA256SUMS")

Upstream's own release binaries were built with Command Line Tools 26.4 and
their Release/SHA256SUMS will NOT match the hash above. That is expected; this
file is the local substitute for that manifest.
PROV

echo
echo "staged:"
ls -la "$STAGE_DIR"
echo
cat "$STAGE_DIR/PROVENANCE.txt"
echo
echo "Next: mount $STAGE_DIR into the VMs (see README.md), then inject it on the"
echo "Metal leg only. Guest path is /Volumes/My Shared Files/<mount name>/."
