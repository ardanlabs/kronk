#!/usr/bin/env bash
#
# Start N self-hosted GitHub Actions runners on this host.
#
# A runner process executes ONE job at a time, so a single container makes
# every job in a workflow run back to back. linux.yml has five jobs; with
# COUNT=5 they run concurrently instead of serially.
#
#   COUNT=4 ./start-runners.sh
#   COUNT=4 ./start-runners.sh --recreate    # replace running containers
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
#
# Build the image first:
#
#   docker build -t kronk-runner:vulkan zarf/docker/runner

set -euo pipefail

COUNT="${COUNT:-1}"
IMAGE="${IMAGE:-kronk-runner:vulkan}"
PREFIX="${PREFIX:-kronk-linux-gpu}"
ORG="${ORG:-ardanlabs}"
GROUP="${GROUP:-kronk}"
APP_ID="${APP_ID:-}"
APP_KEY="${APP_KEY:-}"

# The backend is read off the image rather than hardcoded, so a vulkan
# image can never register runners advertising rocm. Workflows select on
# `gpu` alone, so this label is descriptive — but a wrong one is actively
# misleading when reading the runner list in GitHub settings.
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

# Numeric GIDs from the host. Group NAMES would resolve against the
# container's /etc/group, where render/video may not exist, and an unset
# variable yields --group-add "" which docker rejects outright.
VIDEO_GID="$(getent group video  | cut -d: -f3)"
RENDER_GID="$(getent group render | cut -d: -f3)"
if [[ -z "$VIDEO_GID" || -z "$RENDER_GID" ]]; then
    echo "could not resolve video/render GIDs on this host" >&2
    exit 1
fi

# The Go module and build caches are safe to share: the toolchain locks
# them and is designed for concurrent use, and sharing is what makes a
# second runner cheap. ~/.kronk is per-runner instead — concurrent
# `kronk model pull` runs against one directory can race while writing
# the same partial file, and 6.6 GB per runner is nothing against the
# disk this host has.
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

    docker volume create "kronk-runner-kronk-${i}" >/dev/null

    docker run -d --restart=always \
        --name "$name" \
        --device /dev/kfd --device /dev/dri \
        --group-add "$VIDEO_GID" --group-add "$RENDER_GID" \
        --security-opt seccomp=unconfined \
        -v "kronk-runner-kronk-${i}:/root/.kronk" \
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

    echo "started $name (labels: ${LABELS})"
done

echo
echo "running runners:"
docker ps --filter "name=^${PREFIX}-" --format '  {{.Names}}\t{{.Status}}'
echo
echo "Confirm they registered into the '${GROUP}' group — registering into"
echo "Default leaves jobs queued forever with no error, because kronk is public:"
echo "  docker logs ${PREFIX}-1 | tail -20"
