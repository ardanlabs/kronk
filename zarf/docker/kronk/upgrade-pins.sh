#!/usr/bin/env bash
# =============================================================================
# upgrade-pins.sh
#
# Refresh the SHA / digest / fingerprint pins in zarf/docker/kronk/Dockerfile
# to their latest upstream values, in place. Run via:
#
#   make deps-upgrade
#
# What gets updated:
#   - ubuntu:24.04 image index digest (3 occurrences)
#   - nvcr.io/nvidia/l4t-cuda:<tag> image digest
#   - NODE_SHA256_X64 / NODE_SHA256_ARM64 for the currently pinned NODE_VERSION
#   - ROCM_KEY_FINGERPRINT from repo.radeon.com
#   - LIBSTDCXX_VERSION from ppa:ubuntu-toolchain-r/test on jammy
#     (requires a working `docker` daemon; slow ~30s — skip with --no-libstdcxx)
#
# What deliberately does NOT auto-bump:
#   - NODE_VERSION, ROCM_VERSION  → semantic version bumps are a decision
#   - Go toolchain version       → driven by `go.mod`, fetched live at build
#
# Review the result with `git diff` before committing.
#
# Dependencies: bash, curl, gpg, sed, awk, sha256sum-or-shasum,
#               docker (only for --libstdcxx / default mode).
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOCKERFILE="${SCRIPT_DIR}/Dockerfile"

DO_LIBSTDCXX=1
for arg in "$@"; do
    case "$arg" in
        --no-libstdcxx) DO_LIBSTDCXX=0 ;;
        -h|--help)
            sed -n '2,28p' "$0" | sed 's/^# \{0,1\}//'
            exit 0 ;;
        *) echo "unknown flag: $arg" >&2; exit 2 ;;
    esac
done

[[ -f "$DOCKERFILE" ]] || { echo "Dockerfile not found at $DOCKERFILE" >&2; exit 1; }

# -----------------------------------------------------------------------------
# Helpers
# -----------------------------------------------------------------------------

log() { printf '\033[1;36m>>>\033[0m %s\n' "$*"; }

# Portable in-place edit (BSD/macOS sed needs `-i ''`, GNU sed doesn't).
sed_inplace() {
    local expr="$1" file="$2" tmp
    tmp="$(mktemp)"
    sed -E "$expr" "$file" > "$tmp"
    mv "$tmp" "$file"
}

# Extract current value of an ENV/ARG line: get_dockerfile_value NODE_VERSION
get_dockerfile_value() {
    local name="$1"
    awk -v n="$name" '
        $0 ~ "^(ENV|ARG) " n "=" {
            sub("^(ENV|ARG) " n "=\"?", "")
            sub("\"$", "")
            sub("[[:space:]].*$", "")
            print; exit
        }' "$DOCKERFILE"
}

# Resolve the index digest for a multi-platform image reference.
# Returns empty string (not non-zero) on any failure so callers can
# produce a useful error message instead of `set -e` aborting silently.
docker_image_digest() {
    local ref="$1"
    docker buildx imagetools inspect "$ref" 2>/dev/null \
        | awk '/^Digest:/ {print $2; exit}' \
        || true
}

# sha256 of a URL's content — uses sha256sum on Linux, shasum on macOS.
url_sha256() {
    local url="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        curl -fsSL "$url" | sha256sum | awk '{print $1}'
    else
        curl -fsSL "$url" | shasum -a 256 | awk '{print $1}'
    fi
}

# -----------------------------------------------------------------------------
# 1. ubuntu:24.04 index digest
# -----------------------------------------------------------------------------
log "Resolving ubuntu:24.04 digest"
UBUNTU_DIGEST="$(docker_image_digest ubuntu:24.04)"
[[ "$UBUNTU_DIGEST" =~ ^sha256:[a-f0-9]{64}$ ]] \
    || { echo "ubuntu:24.04 digest lookup failed: '$UBUNTU_DIGEST'" >&2; exit 1; }
echo "    → $UBUNTU_DIGEST"
sed_inplace \
    "s|(ubuntu:24\.04@sha256:)[a-f0-9]{64}|\1${UBUNTU_DIGEST#sha256:}|g" \
    "$DOCKERFILE"

# -----------------------------------------------------------------------------
# 2. nvcr.io/nvidia/l4t-cuda:<tag> digest
# -----------------------------------------------------------------------------
# Extract the current tag from the Dockerfile so we don't accidentally
# jump L4T versions — that's a deliberate decision (the L4T tag must
# match the host's JetPack release).
# Splitting the FROM line on `:` and `@` yields:
#   $1 = "FROM nvcr.io/nvidia/l4t-cuda"
#   $2 = "<tag>"          ← what we want
#   $3 = "sha256"
#   $4 = "<digest> AS runtime-jetson"
L4T_TAG="$(awk -F'[:@]' '/nvcr\.io\/nvidia\/l4t-cuda:.*@sha256:/ {print $2; exit}' "$DOCKERFILE")"
[[ -n "$L4T_TAG" ]] || { echo "could not parse L4T tag from Dockerfile" >&2; exit 1; }

log "Resolving nvcr.io/nvidia/l4t-cuda:${L4T_TAG} digest"
L4T_DIGEST="$(docker_image_digest "nvcr.io/nvidia/l4t-cuda:${L4T_TAG}")"
[[ "$L4T_DIGEST" =~ ^sha256:[a-f0-9]{64}$ ]] \
    || { echo "L4T digest lookup failed: '$L4T_DIGEST'" >&2; exit 1; }
echo "    → $L4T_DIGEST"
sed_inplace \
    "s|(nvcr\.io/nvidia/l4t-cuda:${L4T_TAG}@sha256:)[a-f0-9]{64}|\1${L4T_DIGEST#sha256:}|" \
    "$DOCKERFILE"

# -----------------------------------------------------------------------------
# 3. Node.js tarball SHAs for the currently pinned NODE_VERSION
# -----------------------------------------------------------------------------
NODE_VERSION="$(get_dockerfile_value NODE_VERSION)"
[[ -n "$NODE_VERSION" ]] || { echo "NODE_VERSION not found in Dockerfile" >&2; exit 1; }

log "Fetching Node.js ${NODE_VERSION} SHAs"
NODE_SHA256_X64="$(url_sha256 \
    "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz")"
NODE_SHA256_ARM64="$(url_sha256 \
    "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-arm64.tar.xz")"
[[ "$NODE_SHA256_X64"   =~ ^[a-f0-9]{64}$ ]] || { echo "bad x64 sha"   >&2; exit 1; }
[[ "$NODE_SHA256_ARM64" =~ ^[a-f0-9]{64}$ ]] || { echo "bad arm64 sha" >&2; exit 1; }
echo "    → x64   $NODE_SHA256_X64"
echo "    → arm64 $NODE_SHA256_ARM64"
sed_inplace \
    "s|(ARG NODE_SHA256_X64=\")[a-f0-9]{64}|\1${NODE_SHA256_X64}|" \
    "$DOCKERFILE"
sed_inplace \
    "s|(ARG NODE_SHA256_ARM64=\")[a-f0-9]{64}|\1${NODE_SHA256_ARM64}|" \
    "$DOCKERFILE"

# -----------------------------------------------------------------------------
# 4. ROCm apt key fingerprint
# -----------------------------------------------------------------------------
log "Fetching ROCm apt key fingerprint"
ROCM_FPR="$(curl -fsSL https://repo.radeon.com/rocm/rocm.gpg.key \
    | gpg --show-keys --with-colons 2>/dev/null \
    | awk -F: '/^fpr:/ {print $10; exit}')"
[[ "$ROCM_FPR" =~ ^[A-F0-9]{40}$ ]] \
    || { echo "bad ROCm fingerprint: '$ROCM_FPR'" >&2; exit 1; }
echo "    → $ROCM_FPR"
sed_inplace \
    "s|(ARG ROCM_KEY_FINGERPRINT=\")[A-F0-9]+|\1${ROCM_FPR}|" \
    "$DOCKERFILE"

# -----------------------------------------------------------------------------
# 5. LIBSTDCXX_VERSION from ppa:ubuntu-toolchain-r/test (jammy)
# -----------------------------------------------------------------------------
if (( DO_LIBSTDCXX )); then
    if ! command -v docker >/dev/null 2>&1; then
        echo "    docker not found — skipping LIBSTDCXX_VERSION refresh" >&2
    else
        log "Probing libstdc++6 candidate version from ubuntu-toolchain-r/test"
        LIBSTDCXX_VERSION="$(docker run --rm --pull=missing ubuntu:22.04 sh -c '
            export DEBIAN_FRONTEND=noninteractive
            apt-get -qq update >/dev/null
            apt-get -qq install -y --no-install-recommends \
                ca-certificates gnupg software-properties-common >/dev/null
            add-apt-repository -y ppa:ubuntu-toolchain-r/test >/dev/null 2>&1
            apt-get -qq update >/dev/null
            apt-cache policy libstdc++6 | awk "/Candidate:/ {print \$2; exit}"
        ')"
        if [[ -z "$LIBSTDCXX_VERSION" ]]; then
            echo "    libstdc++6 candidate probe returned empty — leaving pin alone" >&2
        else
            echo "    → $LIBSTDCXX_VERSION"
            # Version strings contain `+` and `~`; sed-escape them via [^"]+
            # on the right side instead of \1<value>: we just rewrite the
            # whole quoted value.
            sed_inplace \
                "s|(ARG LIBSTDCXX_VERSION=\")[^\"]+|\1${LIBSTDCXX_VERSION}|" \
                "$DOCKERFILE"
        fi
    fi
else
    log "Skipping LIBSTDCXX_VERSION probe (--no-libstdcxx)"
fi

log "Done. Review changes with: git diff -- $DOCKERFILE"
