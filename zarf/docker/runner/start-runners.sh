#!/usr/bin/env bash
#
# Start N self-hosted GitHub Actions runners on this host.
#
#   COUNT=2 ./start-runners.sh
#   COUNT=2 ./start-runners.sh --recreate    # replace running containers
#
# A runner process executes ONE job at a time, but COUNT is a memory budget
# rather than a job count: see HOST MEMORY BUDGET below. This host runs two
# per fleet, and fleets are told apart by PREFIX; see TWO FLEETS below.
#
# Configuration comes from the environment:
#
#   COUNT       how many runners to start                   (default 1)
#   IMAGE       image to run                                (default kronk-runner:vulkan)
#   PREFIX      container / runner name prefix              (default kronk-linux-gpu)
#   APP_ID      GitHub App id                               (required)
#   APP_KEY     path to the App private key .pem            (required)
#   ORG         GitHub org                                  (default ardanlabs)
#   GROUP       runner group                                (default kronk)
#   LABELS      runner labels  (default: gpu,<backend>, where <backend>
#               comes from the image's com.ardanlabs.kronk.backend label)
#   MEMORY      per-container memory cap, docker size    (default 20g)
#               0 or empty leaves the container uncapped
#
# Build the image first:
#
#   docker build -t kronk-runner:vulkan zarf/docker/runner
#   docker build -t kronk-runner:rocm --target rocm zarf/docker/runner
#
# HOST MEMORY BUDGET
#
# An uncapped container sees every byte the host has, so one runaway job takes
# the box down with it — the other fleet's runner included. MEMORY caps each
# container, and --memory-swap is pinned to the same value because docker
# otherwise allows swap to twice the limit, and swapping a 16 GiB test into
# this host's 8 GiB of swap is slower than failing it.
#
# Measured on this host:
#
#   ~6.6 GiB   the base model set (.github/test-models.txt), per runner
#   ~16 GiB    peak for the MTP targets in .github/test-models-gpu.txt
#
# 20g therefore clears the largest job with headroom. Four of them nominally
# oversubscribe 62 GiB on purpose: the cap is a blast radius, not admission
# control, and gpu.yml schedules one GPU leg per fleet, so the realistic peak
# is two GPU legs at ~16 GiB beside two CPU jobs.
#
# The cap bounds system RAM only, not VRAM: this Strix Halo part carves GPU
# memory out of the same 62 GiB and that is not charged to the cgroup.
#
# TWO FLEETS ON ONE HOST
#
# One backend per image, one fleet per backend. gpu.yml's matrix puts the
# backend in runs-on, so each leg lands on its own fleet instead of on
# whichever runner GitHub picks first. Both fleets must be running, or the
# unmatched leg queues silently until the 24h run limit.
#
# The second fleet needs a PREFIX of its own: that is what keeps container
# names, runner names and the per-runner ~/.kronk volume distinct.
#
#   COUNT=2 APP_ID=... APP_KEY=~/kronk-runners.pem ./start-runners.sh
#   PREFIX=kronk-linux-rocm COUNT=2 IMAGE=kronk-runner:rocm \
#       APP_ID=... APP_KEY=~/kronk-runners.pem ./start-runners.sh
#
# The CPU jobs ask only for self-hosted/Linux/X64, so either fleet serves them
# and the split between the two is a capacity choice. Only the GPU legs are
# pinned.

set -euo pipefail

COUNT="${COUNT:-1}"
IMAGE="${IMAGE:-kronk-runner:vulkan}"
PREFIX="${PREFIX:-kronk-linux-gpu}"
ORG="${ORG:-ardanlabs}"
GROUP="${GROUP:-kronk}"
APP_ID="${APP_ID:-}"
APP_KEY="${APP_KEY:-}"
MEMORY="${MEMORY-20g}"

# The backend is read off the image rather than hardcoded, so a vulkan image
# can never register runners advertising rocm. gpu.yml's matrix puts the
# backend in runs-on: a wrong label here sends ROCm suites to a runner with no
# HIP userspace, and a missing one leaves that leg queued with no error.
if [[ -z "${LABELS:-}" ]]; then
    backend="$(docker image inspect \
        -f '{{ index .Config.Labels "com.ardanlabs.kronk.backend" }}' \
        "$IMAGE" 2>/dev/null || true)"

    if [[ -z "$backend" || "$backend" == "<no value>" ]]; then
        echo "cannot read com.ardanlabs.kronk.backend from $IMAGE." >&2
        echo "Build it from zarf/docker/runner, or set LABELS explicitly." >&2
        exit 1
    fi

    LABELS="gpu,${backend}"
fi

RECREATE=false
case "${1:-}" in
    --recreate) RECREATE=true ;;
    "")         ;;
    *)          echo "unknown flag: $1 (expected --recreate)" >&2; exit 1 ;;
esac

if [[ -z "$APP_ID" || -z "$APP_KEY" ]]; then
    echo "APP_ID and APP_KEY (path to the .pem) are required" >&2
    exit 1
fi

if [[ ! -r "$APP_KEY" ]]; then
    echo "cannot read APP_KEY: $APP_KEY" >&2
    exit 1
fi

if ! [[ "$COUNT" =~ ^[0-9]+$ ]] || (( COUNT < 1 )); then
    echo "COUNT must be a positive integer, got: $COUNT" >&2
    exit 1
fi

# Both flags carry the same value so the cap is a ceiling: --memory on its own
# leaves --memory-swap at twice the limit. Docker reads 0 as unlimited, so an
# explicit 0 (or an empty MEMORY) omits the flags instead.
mem_args=()
mem_desc="uncapped"
if [[ -n "$MEMORY" && "$MEMORY" != "0" ]]; then
    if ! [[ "$MEMORY" =~ ^[0-9]+[bkmgBKMG]?$ ]]; then
        echo "MEMORY must be a docker size such as 20g, 8g or 512m, got: $MEMORY" >&2
        exit 1
    fi

    mem_args=(--memory "$MEMORY" --memory-swap "$MEMORY")
    mem_desc="$MEMORY"
fi

# Numeric GIDs from the host. Group NAMES would resolve against the
# container's /etc/group, where render/video may not exist, and an unset
# variable yields --group-add "" which docker rejects outright.
VIDEO_GID="$(getent group video  | cut -d: -f3)"
RENDER_GID="$(getent group render | cut -d: -f3)"
if [[ -z "$VIDEO_GID" || -z "$RENDER_GID" ]]; then
    echo "could not resolve video/render GIDs on this host" >&2
    exit 1
fi

# The Go module and build caches are safe to share — the toolchain locks them
# and is designed for concurrent use — and they are shared across FLEETS too,
# since both compile the same code.
#
# ~/.kronk is per-runner instead: concurrent `kronk model pull` runs against
# one directory can race on the same partial file. The volume is named after
# the runner ("<prefix>-<n>-kronk") rather than the index, because two fleets
# both start at index 1 and an index-keyed name would reintroduce that race.
docker volume create kronk-runner-go      >/dev/null
docker volume create kronk-runner-gocache >/dev/null

for (( i = 1; i <= COUNT; i++ )); do
    name="${PREFIX}-${i}"

    if docker inspect "$name" >/dev/null 2>&1; then
        if [[ "$RECREATE" == true ]]; then
            echo "removing existing $name"
            docker rm -f "$name" >/dev/null
        else
            echo "$name already exists, skipping (use --recreate to replace)"
            continue
        fi
    fi

    docker volume create "${name}-kronk" >/dev/null

    docker run -d --restart=always \
        --name "$name" \
        "${mem_args[@]}" \
        --device /dev/kfd --device /dev/dri \
        --group-add "$VIDEO_GID" --group-add "$RENDER_GID" \
        --security-opt seccomp=unconfined \
        -v "${name}-kronk:/root/.kronk" \
        -v kronk-runner-go:/root/go \
        -v kronk-runner-gocache:/root/.cache/go-build \
        -e APP_ID="$APP_ID" \
        -e APP_PRIVATE_KEY="$(cat "$APP_KEY")" \
        -e APP_LOGIN="$ORG" \
        -e RUNNER_SCOPE=org \
        -e ORG_NAME="$ORG" \
        -e RUNNER_GROUP="$GROUP" \
        -e RUNNER_NAME="$name" \
        -e EPHEMERAL=1 \
        -e LABELS="$LABELS" \
        "$IMAGE" >/dev/null

    echo "started $name (labels: ${LABELS}, memory: ${mem_desc})"
done

echo
echo "running runners:"
docker ps --filter "name=^${PREFIX}-" --format '  {{.Names}}\t{{.Status}}'
echo
echo "Confirm they registered into the '${GROUP}' group — registering into"
echo "Default leaves jobs queued forever with no error, because kronk is public:"
echo "  docker logs ${PREFIX}-1 | tail -20"
