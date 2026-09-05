# GitHub Actions self-hosted runner image for the Linux CI box
# (kronk-linux-gpu-1, AMD Ryzen AI MAX+ 395 w/ Radeon 8060S).
#
# Base is ubuntu-noble (24.04) to match kronk's own runtime image
# (zarf/docker/kronk/Dockerfile) and GitHub's ubuntu-latest.
#
# STAGES
#
#   base    — runner plus the shared runtime deps, including the Vulkan
#             loader and Mesa ICDs. Vulkan lives here because whisper.cpp
#             publishes no ROCm bundle, so bucky falls back to Vulkan even
#             on a ROCm host: a runtime fallback, not a scheduling claim.
#   rocm    — base plus the ROCm userspace; `rocminfo` is what Kronk probes
#             to select ROCm. Labelled `rocm`, takes gpu.yml's ROCm leg.
#             Build it with --target rocm.
#   vulkan  — the default target. Labelled `vulkan`, takes gpu.yml's
#             Vulkan leg.
#
# BUILD
#
#   docker build -t kronk-runner:vulkan zarf/docker/runner
#   docker build -t kronk-runner:rocm --target rocm zarf/docker/runner
#
# RUN
#
# Use start-runners.sh beside this file — it fills in the GIDs, volumes and
# registration env, and starts COUNT runners named <prefix>-1 .. <prefix>-N.
# IMAGE selects the backend, so the rocm fleet names its image and takes a
# PREFIX of its own:
#
#   COUNT=2 APP_ID=... APP_KEY=~/kronk-runners.pem \
#       ./zarf/docker/runner/start-runners.sh
#   PREFIX=kronk-linux-rocm COUNT=2 IMAGE=kronk-runner:rocm APP_ID=... \
#       APP_KEY=~/kronk-runners.pem ./zarf/docker/runner/start-runners.sh
#
# COUNT is bounded by RAM, not by the number of jobs in flight: a runner
# process takes ONE job at a time, but this host runs two per fleet, each
# capped by MEMORY. See HOST MEMORY BUDGET in start-runners.sh.
#
# Three things that are easy to get wrong, all handled by the script:
#
#   RUNNER_GROUP  — omitting it registers into the Default group, which does
#                   not allow public repositories, so every job queues
#                   forever with no error message. kronk is public.
#   --group-add   — pass numeric GIDs from the host. Group NAMES resolve
#                   against the container's /etc/group, where render/video
#                   may not exist, and an unset variable yields
#                   --group-add "", which docker rejects.
#   volumes       — EPHEMERAL=1 re-registers the runner fresh each job, so
#                   nothing a job leaves outside these volumes reaches the
#                   next one. They are this runner's whole cache, replacing
#                   actions/cache, which the workflows skip on self-hosted:
#                     /root/.kronk          models (~6.6 GB) and native libs
#                     /root/go              GOMODCACHE and GOPATH/bin
#                     /root/.cache/go-build GOCACHE, the compile cache
#                   The Go caches are shared across runners (the toolchain
#                   locks them); ~/.kronk is per-runner, because concurrent
#                   `kronk model pull` runs against one directory can race.
#                   Sharing is safe only because fork PRs are never
#                   scheduled onto these runners (see linux.yml).
#
# LABELS AND TWO FLEETS
#
# A runner is labelled `gpu` plus exactly ONE backend, and gpu.yml's matrix
# selects on it — that is what keeps ROCm suites off the Vulkan runners.
# start-runners.sh derives the labels from com.ardanlabs.kronk.backend, so a
# rebuilt image cannot register runners advertising the wrong backend.
#
# Both legs are gated in gpu.yml, so BOTH fleets have to be running: an
# unmatched runs-on does not fail, it queues silently until the 24h limit.
# Retiring a fleet means dropping its matrix entry in
# .github/workflows/gpu.yml in the same change.

ARG RUNNER_TAG="ubuntu-noble"

# -----------------------------------------------------------------------------
# base
# -----------------------------------------------------------------------------
FROM myoung34/github-runner:${RUNNER_TAG} AS base

USER root

# Mirrors the runtime deps in zarf/docker/kronk/Dockerfile. libffi8 is explicit
# rather than left transitive because it is the one that broke here; ffmpeg is
# required by bucky's audio decoder (sdk/bucky/model/decode.go) to transcode
# non-WAV/MP3/FLAC uploads.
#
# clinfo is a diagnostic for runner-check.yml's Tooling step — nothing in Kronk
# selects a backend on OpenCL. It enumerates devices only through an installed
# ICD, so on the vulkan image it resolves but reports 0 platforms. Expected.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        clinfo \
        curl \
        ffmpeg \
        libcurl4 \
        libffi8 \
        libgomp1 \
        libnuma1 \
        libopenblas0 \
        libstdc++6 \
        libvulkan1 \
        mesa-vulkan-drivers \
        vulkan-tools \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# -----------------------------------------------------------------------------
# rocm — the CI target: ROCm on top of base, so it carries both backends
# -----------------------------------------------------------------------------
FROM base AS rocm

# The backend this image is FOR. base's Vulkan loader is still underneath —
# bucky needs it — but this fleet takes ROCm work only. start-runners.sh reads
# this label to derive the runner's GitHub labels.
LABEL com.ardanlabs.kronk.backend="rocm"
LABEL org.opencontainers.image.title="kronk-runner-rocm"
LABEL org.opencontainers.image.description="GitHub Actions runner for kronk CI, ROCm backend"

# Keep in sync with ROCM_VERSION in zarf/docker/kronk/Dockerfile: libggml-hip.so
# needs libamdhip64.so.7, librocblas.so.5 and libhipblas.so.3, none of which the
# 6.4.x line provides, and this Strix Halo part (gfx1151) wants 7.x or newer
# regardless. Bump only after checking the bundle's NEEDED sonames, or the ROCm
# matrix leg starts failing to dlopen libggml-hip.so.
#
# 10.0 is a repo change, not just a version bump: AMD moved the core packages
# off repo.radeon.com (whose apt channel stops at 7.2.4) onto stable.repo.amd.com
# under an `amdrocm<ver>` naming scheme, and the tree moved from /opt/rocm to
# /opt/rocm/core-<ver>. zarf/docker/kronk/Dockerfile made that move in #924 and
# this file was left behind on 7.2.4, so the ROCm CI leg was testing a runtime
# the product no longer ships.
ARG ROCM_VERSION="10.0"
ARG ROCM_PKG_VERSION="10.0.0-4"
ARG ROCM_KEY_FINGERPRINT="D0F004A0025A1145C7807FCD0701EAC4D5E02107"

# 10.0 splits the device code per GPU target, and this fleet is Strix Halo only.
# The full `amdrocm10.0` metapackage pulls all 25 gfx targets (~6.6 GB of
# downloads) to ship 24 sets of code objects no runner here can execute; the
# gfx1151 leaf pulls ~184 MB and drags in the same amdrocm-base10.0 /
# amdrocm-runtime10.0 / amdrocm-llvm10.0 the metapackage would. Set this to a
# different target if the fleet ever gains a non-Strix-Halo part; use the bare
# `amdrocm${ROCM_VERSION}` metapackage if it gains several.
ARG ROCM_GFX_TARGET="gfx1151"

# 10.0 installs under /opt/rocm/core-<ver> rather than /opt/rocm, so both the
# loader config and PATH below have to follow it.
ENV ROCM_PATH="/opt/rocm/core-${ROCM_VERSION}"

# amdrocm-opencl is the OpenCL ICD, here only so base's clinfo has a platform to
# report on this fleet; it brings the ocl-icd-libopencl1 loader with it. Nothing
# in Kronk reads OpenCL. rocminfo, rocm-smi and rocm_agent_enumerator arrive via
# amdrocm-base10.0, which amdrocm-core10.0-gfx1151 depends on — runner-check.yml
# requires all three on PATH, so a packaging change that drops them fails there.
#
# The key fingerprint is verified before the repo is trusted, so a swapped key
# fails the build instead of silently installing from an unknown source.
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends gnupg; \
    curl -fsSL https://stable.repo.amd.com/rocm/gpg/packages.gpg -o /tmp/rocm.gpg.key; \
    ACTUAL_FPR="$(gpg --show-keys --with-colons /tmp/rocm.gpg.key \
        | awk -F: '/^fpr:/ {print $10; exit}')"; \
    if [ "${ACTUAL_FPR}" != "${ROCM_KEY_FINGERPRINT}" ]; then \
        echo "ROCm key fingerprint mismatch: expected ${ROCM_KEY_FINGERPRINT}, got ${ACTUAL_FPR}" >&2; \
        exit 1; \
    fi; \
    gpg --dearmor < /tmp/rocm.gpg.key > /usr/share/keyrings/rocm-archive-keyring.gpg; \
    rm /tmp/rocm.gpg.key; \
    printf '%s\n' \
        'Types: deb' \
        'URIs: https://stable.repo.amd.com/rocm/core/packages/ubuntu2404/' \
        'Suites: stable' \
        'Components: main' \
        'Architectures: amd64' \
        'Signed-By: /usr/share/keyrings/rocm-archive-keyring.gpg' \
        > /etc/apt/sources.list.d/amdrocm-stable.sources; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        "amdrocm${ROCM_VERSION}-${ROCM_GFX_TARGET}=${ROCM_PKG_VERSION}" \
        "amdrocm-opencl${ROCM_VERSION}=${ROCM_PKG_VERSION}"; \
    echo "${ROCM_PATH}/lib" > /etc/ld.so.conf.d/rocm.conf; \
    ldconfig; \
    for t in rocminfo rocm-smi; do \
        [ -x "${ROCM_PATH}/bin/${t}" ] || { \
            echo "${t} missing from ${ROCM_PATH}/bin after install; runner-check.yml requires it" >&2; \
            exit 1; \
        }; \
    done; \
    apt-get purge -y --auto-remove gnupg; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*

# The ld.so.conf line above covers only the libraries, so without this rocminfo
# and rocm-smi report MISSING in runner-check.yml's Tooling step. Mirrors PATH in
# zarf/docker/kronk/Dockerfile, the image this one exists to reproduce.
ENV PATH="${ROCM_PATH}/bin:${PATH}"

# -----------------------------------------------------------------------------
# vulkan — default target (last stage). Vulkan only: skips the ROCm
#          layer's ~2 GB, and cannot run gpu.yml's ROCm matrix leg.
# -----------------------------------------------------------------------------
FROM base AS vulkan

# As on the rocm stage, start-runners.sh turns this into the runner's GitHub
# labels. This fleet takes gpu.yml's Vulkan leg only.
LABEL com.ardanlabs.kronk.backend="vulkan"
LABEL org.opencontainers.image.title="kronk-runner-vulkan"
LABEL org.opencontainers.image.description="GitHub Actions runner for kronk CI, Vulkan backend"
