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
#             its own stage because whisper.cpp publishes no ROCm bundle, so
#             bucky falls back to Vulkan even on a ROCm host.
#   rocm    — base plus the ROCm userspace. Not used yet; `rocminfo` is what
#             Kronk probes to select the backend, so this is what a future
#             KRONK_PROCESSOR=rocm job needs.
#   vulkan  — the default target, and what CI uses today.
#
# BUILD
#
#   docker build -t kronk-runner:vulkan zarf/docker/runner
#   docker build -t kronk-runner:rocm --target rocm zarf/docker/runner
#
# RUN
#
# Use start-runners.sh beside this file — it fills in the GIDs, volumes and
# registration env, and starts COUNT runners named <prefix>-1 .. <prefix>-N:
#
#   docker build -t kronk-runner:vulkan zarf/docker/runner
#   COUNT=5 APP_ID=... APP_KEY=~/kronk-runners.pem \
#       ./zarf/docker/runner/start-runners.sh
#
# COUNT matters: a runner process executes ONE job at a time, so a single
# container makes every job in a workflow run back to back. linux.yml has
# five jobs, so COUNT=5 is the point where they stop queueing behind each
# other.
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
# Workflows select on `gpu` alone and pick the backend through
# KRONK_PROCESSOR, so switching between Vulkan and ROCm needs no
# relabelling and no workflow edit. The backend label is descriptive: it
# says what the runner can actually do, which matters when reading the
# runner list in GitHub settings. start-runners.sh derives it from the
# image's com.ardanlabs.kronk.backend label rather than hardcoding it, so
# a vulkan image can never advertise itself as rocm.
#
# Do NOT add the backend to a workflow's runs-on. A job requiring `rocm`
# stops matching the moment the runner is rebuilt on Vulkan, and an
# unmatched job queues forever with no error message.

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
# rocm — not built today; kept so the switch is a --target away
# -----------------------------------------------------------------------------
FROM base AS rocm

# The backend this image provides. start-runners.sh reads it to derive the
# runner's GitHub labels, so a rocm image cannot end up advertising vulkan
# or the reverse.
LABEL com.ardanlabs.kronk.backend="rocm"
LABEL org.opencontainers.image.title="kronk-runner-rocm"
LABEL org.opencontainers.image.description="GitHub Actions runner for kronk CI, ROCm backend"

# Keep in sync with ROCM_VERSION in zarf/docker/kronk/Dockerfile, which
# documents why 7.x is required: libggml-hip.so needs libamdhip64.so.7,
# librocblas.so.5 and libhipblas.so.3, none of which the 6.4.x line
# provides. This runner is also a Strix Halo part (gfx1151), which wants
# the 7.x line regardless.
#
# NOT YET EXERCISED: linux.yml runs gpu-tests on Vulkan, so nothing builds
# this stage today.
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
# vulkan — default target (last stage), what CI runs on today
# -----------------------------------------------------------------------------
FROM base AS vulkan

# See the note on the rocm stage: this is what start-runners.sh turns into
# the runner's GitHub labels.
LABEL com.ardanlabs.kronk.backend="vulkan"
LABEL org.opencontainers.image.title="kronk-runner-vulkan"
LABEL org.opencontainers.image.description="GitHub Actions runner for kronk CI, Vulkan backend"
