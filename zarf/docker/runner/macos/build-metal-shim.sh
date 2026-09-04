#!/usr/bin/env bash
# =============================================================================
# build-metal-shim.sh
#
# Build cua's Lume Metal capability shim on this host and stage it where the
# ephemeral macOS runner VMs can reach it over virtiofs. README.md §4 has the
# full rationale; pvg-feature-level.sh is the host-side half of the pair.
#
#   ./build-metal-shim.sh                 # clone, build, verify, stage
#   CUA_REF=<sha> ./build-metal-shim.sh   # pin a known-good upstream revision
#   STAGE_DIR=~/ci-cache/kronk-tools ./build-metal-shim.sh
#
# Run as the user that owns the VMs (`pacha`). Needs Xcode Command Line Tools.
#
# WHAT IT IS: a process-scoped dylib, injected with DYLD_INSERT_LIBRARIES, that
# raises the Metal GPU family a guest process sees. GPU work still goes through
# Apple's paravirtual path, and removing the environment variables removes the
# effect entirely.
#
# BUILT HERE, NOT SHIPPED: upstream commits no binaries, and their release was
# built with CLT 26.4, so this host's 26.6 will not match their SHA256SUMS —
# hence the local PROVENANCE.txt written below. Not built inside the VM either:
# the guest's Xcode 26.6 carries the same Apple clang, and the VMs are
# ephemeral, so building there would repeat the work every job.
#
# CAVEATS, because they decide how CI has to be written:
#
#   - Universal dylib, never a single slice. DYLD_INSERT_LIBRARIES is inherited
#     by every child process and /usr/bin is arm64e, so an arm64-only dylib
#     makes dyld abort every system binary in the tree.
#   - It fails OPEN: a moved hook or a malformed LUME_METAL_APPLE_FAMILY_MAX
#     leaves the process on the Apple 5 fallback with a green build. Assert the
#     family in the job.
#   - Never advertise MTLGPUFamilyMetal3. MLX-LM uses it to select residency
#     sets the paravirtual device cannot create, and leaving it honest keeps it
#     usable as CI's "is this real hardware" gate.
#   - A reported capability is not a working one, so a failure under the shim is
#     ambiguous between a paravirtual driver gap and a real kronk bug.
# =============================================================================

set -euo pipefail

readonly REPO="https://github.com/trycua/cua.git"
readonly SUBDIR="libs/lume/metal-capability-shim"
readonly DYLIB="LumeMetalCapabilities.dylib"

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

# A shallow clone is enough to build. The resolved SHA is recorded below because
# "main" moves, and the provenance file is the only record of which source
# produced the staged binary.
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

for slice in arm64 arm64e; do
    if [[ ! -f "$src/dist/LumeMetalCapabilities-$slice.dylib" ]]; then
        echo "build produced no $slice dylib under $src/dist" >&2
        exit 1
    fi
done

# One file with both slices, so injection is safe into arm64 Go binaries and
# arm64e platform binaries alike.
lipo -create \
    "$src/dist/LumeMetalCapabilities-arm64.dylib" \
    "$src/dist/LumeMetalCapabilities-arm64e.dylib" \
    -output "$STAGE_DIR/$DYLIB"

if [[ "$(lipo -archs "$STAGE_DIR/$DYLIB")" != *arm64e* ]]; then
    echo "lipo produced no arm64e slice; injection would abort system binaries" >&2
    exit 1
fi

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
slices  : $(lipo -archs "$STAGE_DIR/$DYLIB")
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
