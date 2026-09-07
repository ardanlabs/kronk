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
# Each runner is a supervise-runner.sh loop, not a long-lived container: it
# mints a single-use JIT config per job and runs it in a --rm container, so the
# App private key never enters a container and no job inherits the previous
# job's writable layer. Nothing restarts the loops after a reboot, so add a
# crontab line for that (crontab -e, no root needed):
#
#   @reboot COUNT=2 APP_ID=... APP_KEY=$HOME/key.pem $HOME/start-runners.sh
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
#   LOG_DIR     where the supervisor loops log         (default ~/.kronk-runners)
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
#   ~16 GiB    peak for the `gpu`-tier MTP targets in .github/test-models.txt
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
# names, runner names and the per-runner cache volumes distinct.
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
LOG_DIR="${LOG_DIR:-$HOME/.kronk-runners}"

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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

# Validated here rather than in the supervisor, so a typo fails at the prompt
# instead of in a background loop's log. Docker reads 0 as unlimited, so an
# explicit 0 (or an empty MEMORY) means uncapped.
mem_desc="uncapped"
if [[ -n "$MEMORY" && "$MEMORY" != "0" ]]; then
    if ! [[ "$MEMORY" =~ ^[0-9]+[bkmgBKMG]?$ ]]; then
        echo "MEMORY must be a docker size such as 20g, 8g or 512m, got: $MEMORY" >&2
        exit 1
    fi

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

# Every cache is per-runner: ~/.kronk, the Go module cache, the Go build cache
# and _diag. Named after the runner ("<prefix>-<n>-...") rather than the index,
# because two fleets both start at index 1 and would collide.
#
# _diag is a volume because the containers are --rm: those logs are the only
# place llama.cpp's stderr survives a job.
mkdir -p "$LOG_DIR"

for (( i = 1; i <= COUNT; i++ )); do
    name="${PREFIX}-${i}"

    if pgrep -f "supervise-runner.sh $name\$" >/dev/null; then
        if [[ "$RECREATE" == true ]]; then
            echo "stopping existing supervisor for $name"
            pkill -f "supervise-runner.sh $name\$" || true
            docker rm -f "$name" >/dev/null 2>&1 || true
        else
            echo "$name is already supervised, skipping (use --recreate to replace)"
            continue
        fi
    fi

    docker volume create "${name}-kronk"   >/dev/null
    docker volume create "${name}-go"      >/dev/null
    docker volume create "${name}-gocache" >/dev/null
    docker volume create "${name}-diag"    >/dev/null

    # setsid so the loop outlives this shell and its ssh session.
    IMAGE="$IMAGE" LABELS="$LABELS" MEMORY="$MEMORY" ORG="$ORG" GROUP="$GROUP" \
    APP_ID="$APP_ID" APP_KEY="$APP_KEY" \
    VIDEO_GID="$VIDEO_GID" RENDER_GID="$RENDER_GID" \
        setsid nohup "${here}/supervise-runner.sh" "$name" \
        >>"$LOG_DIR/${name}.log" 2>&1 &
    disown || true

    echo "started $name (labels: ${LABELS}, memory: ${mem_desc}, log: $LOG_DIR/${name}.log)"
done

echo
echo "supervisors:"
sleep 1
pgrep -af "supervise-runner.sh ${PREFIX}-" | sed 's/^/  /'
echo
echo "A runner takes ~10s to appear. The group is baked into the JIT config, and"
echo "the wrong one leaves jobs queued forever with no error, because kronk is"
echo "public and Default excludes public repos:"
echo "  tail -20 $LOG_DIR/${PREFIX}-1.log"
echo "  docker ps --filter name=^${PREFIX}-"
