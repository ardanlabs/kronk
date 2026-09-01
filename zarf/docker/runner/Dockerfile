# GitHub Actions self-hosted runner image for the Linux CI box
# (kronk-linux-gpu-1, AMD Ryzen AI MAX+ 395 w/ Radeon 8060S).
#
# Base is ubuntu-noble (24.04) to match kronk's own runtime image
# (zarf/docker/kronk/Dockerfile) and GitHub's ubuntu-latest.
#
# STAGES
#
#   base    — runner plus the shared library runtime deps, including the
#             Vulkan loader and Mesa ICDs. Vulkan lives here rather than in
#             its own stage because whisper.cpp publishes no ROCm bundle,
#             so bucky falls back to Vulkan even on a ROCm host. That is a
#             runtime fallback inside the container, not a scheduling
#             claim: the rocm image is still labelled `rocm` alone.
#   rocm    — base plus the ROCm userspace; `rocminfo` is what Kronk probes
#             to select ROCm. Labelled `rocm`, and takes linux.yml's ROCm
#             matrix leg. Build it with --target rocm.
#   vulkan  — the default target. Labelled `vulkan`, and takes linux.yml's
#             Vulkan leg.
#
# linux.yml gates on both backends and each leg asks for its own label, so
# BOTH fleets have to be running. See TWO FLEETS in start-runners.sh.
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
# IMAGE selects the backend and defaults to the vulkan image, so the rocm
# fleet names its image and takes a PREFIX of its own:
#
#   COUNT=6 APP_ID=... APP_KEY=~/kronk-runners.pem \
#       ./zarf/docker/runner/start-runners.sh
#   PREFIX=kronk-linux-rocm COUNT=2 IMAGE=kronk-runner:rocm APP_ID=... \
#       APP_KEY=~/kronk-runners.pem ./zarf/docker/runner/start-runners.sh
#
# Two fleets on one host: the second needs a PREFIX of its own, and the
# labels route each matrix leg to its own fleet. See TWO FLEETS in
# start-runners.sh.
#
# COUNT matters: a runner process executes ONE job at a time, so a single
# container makes every job in a workflow run back to back. linux.yml has
# six jobs once the GPU matrix expands — four CPU jobs that either fleet
# can take, plus one GPU leg pinned to each — so six runners across the
# two fleets is the point where nothing queues.
#
# Three things that are easy to get wrong, all handled by the script:
#
#   RUNNER_GROUP  — omitting it registers into the Default group. That group
#                   does not allow public repositories, so every job queues
#                   forever with no error message. kronk is public.
#   --group-add   — pass numeric GIDs from the host. Group NAMES resolve
#                   against the container's /etc/group, where render/video
#                   may not exist. An unset variable yields --group-add ""
#                   and docker fails with "unable to find group".
#   volumes       — EPHEMERAL=1 destroys the container after every job, so
#                   nothing under / survives. The volumes are this runner's
#                   entire cache, and replace actions/cache, which the
#                   workflows skip on self-hosted:
#                     /root/.kronk          models (~6.6 GB) and native libs
#                     /root/go              GOMODCACHE and GOPATH/bin
#                     /root/.cache/go-build GOCACHE, the compile cache
#                   The Go caches are shared across runners (the toolchain
#                   locks them); ~/.kronk is per-runner, because concurrent
#                   `kronk model pull` runs against one directory can race.
#                   Safe because fork PRs are never scheduled onto these
#                   runners (see linux.yml).
#
# LABELS
#
# A runner is labelled `gpu` plus exactly ONE backend, and linux.yml's GPU
# matrix selects on it: the ROCm leg asks for `rocm`, the Vulkan leg for
# `vulkan`. One backend per image, one fleet per backend — that is what
# keeps ROCm suites off the Vulkan runners and vice versa, instead of
# leaving it to whichever runner GitHub picks first.
#
# start-runners.sh derives the GitHub labels from
# com.ardanlabs.kronk.backend rather than hardcoding them, so a rebuilt
# image cannot register runners advertising the wrong backend.
#
# THE RULE, because an unmatched runs-on does not fail — it queues,
# silently, until the run hits the 24h limit: every backend gated in
# linux.yml needs a fleet claiming it. Retiring a fleet means dropping its
# matrix entry in .github/workflows/linux.yml in the same change.

ARG RUNNER_TAG="ubuntu-noble"

# -----------------------------------------------------------------------------
# base
# -----------------------------------------------------------------------------
FROM myoung34/github-runner:${RUNNER_TAG} AS base

USER root

# Mirrors the runtime deps in zarf/docker/kronk/Dockerfile. libffi8 is listed
# explicitly rather than relied on as a transitive dependency, since it is the
# one that broke here. ffmpeg is required by bucky's audio decoder
# (sdk/bucky/model/decode.go) to transcode non-WAV/MP3/FLAC uploads.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
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

# The backend this image is FOR. base's Vulkan loader is still underneath
# — bucky needs it, since whisper.cpp has no ROCm bundle — but this runner
# takes ROCm work only, and linux.yml's Vulkan leg goes to the vulkan
# fleet. start-runners.sh reads this to derive the runner's GitHub labels,
# so a rocm image cannot end up advertising vulkan or the reverse.
LABEL com.ardanlabs.kronk.backend="rocm"
LABEL org.opencontainers.image.title="kronk-runner-rocm"
LABEL org.opencontainers.image.description="GitHub Actions runner for kronk CI, ROCm backend"

# Keep in sync with ROCM_VERSION in zarf/docker/kronk/Dockerfile, which
# documents why 7.x is required: libggml-hip.so needs libamdhip64.so.7,
# librocblas.so.5 and libhipblas.so.3, none of which the 6.4.x line
# provides. This runner is also a Strix Halo part (gfx1151), which wants
# the 7.x line regardless.
#
# Bumping this without checking the bundle's NEEDED sonames is how the
# ROCm matrix leg starts failing to dlopen libggml-hip.so.
ARG ROCM_VERSION="7.2.4"
ARG ROCM_KEY_FINGERPRINT="CA8BB4727A47B4D09B4EE8969386B48A1A693C5C"

# The key fingerprint is verified before the repo is trusted, so a swapped
# key fails the build instead of silently installing from an unknown source.
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends gnupg; \
    curl -fsSL https://repo.radeon.com/rocm/rocm.gpg.key -o /tmp/rocm.gpg.key; \
    ACTUAL_FPR="$(gpg --show-keys --with-colons /tmp/rocm.gpg.key \
        | awk -F: '/^fpr:/ {print $10; exit}')"; \
    if [ "${ACTUAL_FPR}" != "${ROCM_KEY_FINGERPRINT}" ]; then \
        echo "ROCm key fingerprint mismatch: expected ${ROCM_KEY_FINGERPRINT}, got ${ACTUAL_FPR}" >&2; \
        exit 1; \
    fi; \
    gpg --dearmor < /tmp/rocm.gpg.key > /usr/share/keyrings/rocm-archive-keyring.gpg; \
    rm /tmp/rocm.gpg.key; \
    echo "deb [arch=amd64 signed-by=/usr/share/keyrings/rocm-archive-keyring.gpg] \
        https://repo.radeon.com/rocm/apt/${ROCM_VERSION} noble main" \
        > /etc/apt/sources.list.d/rocm.list; \
    printf 'Package: *\nPin: release o=repo.radeon.com\nPin-Priority: 600\n' \
        > /etc/apt/preferences.d/rocm-pin-600; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        rocm-hip-libraries \
        rocm-smi-lib \
        rocminfo; \
    echo "/opt/rocm/lib" > /etc/ld.so.conf.d/rocm.conf; \
    ldconfig; \
    apt-get purge -y --auto-remove gnupg; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*

# -----------------------------------------------------------------------------
# vulkan — default target (last stage). Vulkan only: skips the ROCm
#          layer's ~2 GB, and cannot run linux.yml's ROCm matrix leg.
# -----------------------------------------------------------------------------
FROM base AS vulkan

# See the note on the rocm stage: this is what start-runners.sh turns into
# the runner's GitHub labels. This fleet takes linux.yml's Vulkan leg; the
# ROCm leg is never scheduled onto it.
LABEL com.ardanlabs.kronk.backend="vulkan"
LABEL org.opencontainers.image.title="kronk-runner-vulkan"
LABEL org.opencontainers.image.description="GitHub Actions runner for kronk CI, Vulkan backend"
