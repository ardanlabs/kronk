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
#   VIDEO_GID=$(getent group video  | cut -d: -f3)
#   RENDER_GID=$(getent group render | cut -d: -f3)
#
#   docker run -d --restart=always --name kronk-linux-gpu-1 \
#     --device /dev/kfd --device /dev/dri \
#     --group-add "$VIDEO_GID" --group-add "$RENDER_GID" \
#     --security-opt seccomp=unconfined \
#     -v kronk-runner-kronk:/root/.kronk \
#     -v kronk-runner-go:/root/go \
#     -v kronk-runner-gocache:/root/.cache/go-build \
#     -e APP_ID=... -e APP_PRIVATE_KEY="$(cat key.pem)" -e APP_LOGIN=ardanlabs \
#     -e RUNNER_SCOPE=org -e ORG_NAME=ardanlabs \
#     -e RUNNER_GROUP=kronk \
#     -e RUNNER_NAME=kronk-linux-gpu-1 \
#     -e EPHEMERAL=1 \
#     -e LABELS=gpu,rocm \
#     kronk-runner:vulkan
#
# Three things that are easy to get wrong:
#
#   RUNNER_GROUP  — omitting it registers into the Default group. That group
#                   does not allow public repositories, so every job queues
#                   forever with no error message. kronk is public.
#   --group-add   — pass numeric GIDs from the host. Group NAMES resolve
#                   against the container's /etc/group, where render/video
#                   may not exist. An unset variable yields --group-add ""
#                   and docker fails with "unable to find group".
#   volumes       — EPHEMERAL=1 destroys the container after every job, so
#                   nothing under / survives. The three volumes are this
#                   runner's entire cache, and replace actions/cache, which
#                   the workflows skip on self-hosted:
#                     /root/.kronk         models (~6.6 GB) and native libs
#                     /root/go             GOMODCACHE and GOPATH/bin
#                     /root/.cache/go-build GOCACHE, the compile cache
#                   Safe because fork PRs are never scheduled onto this
#                   runner (see linux.yml).
#
# The labels only need to carry `gpu`; linux.yml picks the backend through
# KRONK_PROCESSOR, so moving between Vulkan and ROCm needs no relabelling.

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

# Deliberately NEWER than the 6.4.3 pin in zarf/docker/kronk/Dockerfile.
# This runner is a Ryzen AI MAX+ 395 (Strix Halo, gfx1151), and the 6.4.x
# line predates usable support for that target — it needs the 7.x line.
# The kronk runtime image is a separate decision: bumping it changes what
# ships to users, so it is left alone here.
#
# repo.radeon.com/rocm/apt publishes noble packages for every 7.x release;
# 7.2.4 is the newest at time of writing. The signing key is shared across
# versions, so the fingerprint pin below survives a version bump.
#
# NOT YET EXERCISED: linux.yml runs gpu-tests on Vulkan, so nothing builds
# this stage today. Before switching KRONK_PROCESSOR back to rocm, check
# that the llama.cpp ROCm bundle Kronk downloads is built against a
# compatible HIP runtime — a bundle compiled for 6.x may not load against
# a 7.x userspace.
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

LABEL org.opencontainers.image.title="kronk-runner-vulkan"
LABEL org.opencontainers.image.description="GitHub Actions runner for kronk CI, Vulkan backend"
