#!/usr/bin/env bash
#
# Supervise ONE runner: mint a JIT config, run one job in a throwaway
# container, repeat. Started by start-runners.sh; not usually run by hand.
#
#   IMAGE=kronk-runner:vulkan LABELS=gpu,vulkan APP_ID=... \
#       APP_KEY=~/key.pem ./supervise-runner.sh kronk-linux-gpu-1
#
# The name is positional so `pgrep -f "supervise-runner.sh <name>"` finds the
# loop for one runner; everything else comes from the environment.
#
# WHY A LOOP AND NOT --restart=always
#
# A JIT config is single-use, so a restarted container cannot re-register with
# the config it was given. Something outside the container has to mint a fresh
# one per job, and that something is this loop. What it buys:
#
#   - the App private key stays on the host. The container gets a config for
#     one named runner, good for one job, already consumed by the time the job
#     starts, so a job that reads its own environment learns nothing reusable.
#   - the container is --rm, so a job's writable layer does not reach the next
#     job. Only the named volumes persist, and _diag is one of them.
#
# The cost is that nothing restarts these after a reboot; --restart=always used
# to cover that. Add a crontab line (crontab -e, no root needed):
#
#   @reboot COUNT=2 APP_ID=... APP_KEY=$HOME/key.pem $HOME/start-runners.sh
#
# HOW A JOB ENDS
#
# A JIT runner exits when its job finishes, docker removes the container, and
# the loop mints the next config. Between jobs there is a container up and
# listening, so `docker ps` looks the same as it did before.

set -euo pipefail

NAME="${1:?the runner name is required}"
IMAGE="${IMAGE:?IMAGE is required}"
LABELS="${LABELS:?LABELS is required}"
APP_ID="${APP_ID:?APP_ID is required}"
APP_KEY="${APP_KEY:?APP_KEY is required}"
ORG="${ORG:-ardanlabs}"
GROUP="${GROUP:-kronk}"
MEMORY="${MEMORY-20g}"
VIDEO_GID="${VIDEO_GID:?VIDEO_GID is required}"
RENDER_GID="${RENDER_GID:?RENDER_GID is required}"

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Both flags carry the same value so the cap is a ceiling: --memory on its own
# leaves --memory-swap at twice the limit, and swapping a 16 GiB test into this
# host's 8 GiB of swap is slower than failing it. start-runners.sh validates.
mem_args=()
if [[ -n "$MEMORY" && "$MEMORY" != "0" ]]; then
    mem_args=(--memory "$MEMORY" --memory-swap "$MEMORY")
fi

# docker run is in the foreground, so killing this loop leaves the container
# behind unless it is cleaned up here.
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap 'cleanup; exit 0' TERM INT

while true; do
    if ! jit="$(NAME="$NAME" LABELS="$LABELS" ORG="$ORG" GROUP="$GROUP" \
        APP_ID="$APP_ID" APP_KEY="$APP_KEY" "$here/mint-jitconfig.sh")"; then
        echo "$(date -Is) $NAME: could not mint a JIT config, retrying in 60s" >&2
        sleep 60
        continue
    fi

    cleanup

    # The image's own entrypoint is bypassed: it registers with config.sh and a
    # registration token, which is the flow the App key existed for. run.sh
    # --jitconfig needs no token, and the group, labels and work folder are
    # already inside the config mint-jitconfig.sh built.
    #
    # RUNNER_ALLOW_RUNASROOT: run.sh refuses to start as root without it, and
    # this image runs as root.
    rc=0
    docker run --rm \
        --name "$NAME" \
        "${mem_args[@]}" \
        --device /dev/kfd --device /dev/dri \
        --group-add "$VIDEO_GID" --group-add "$RENDER_GID" \
        --security-opt seccomp=unconfined \
        -v "${NAME}-kronk:/root/.kronk" \
        -v "${NAME}-go:/root/go" \
        -v "${NAME}-gocache:/root/.cache/go-build" \
        -v "${NAME}-diag:/actions-runner/_diag" \
        -e RUNNER_ALLOW_RUNASROOT=1 \
        --entrypoint /actions-runner/run.sh \
        "$IMAGE" --jitconfig "$jit" || rc=$?

    # A job that fails is a normal exit here. A container that cannot start at
    # all would otherwise spin, burning JIT configs.
    if (( rc != 0 )); then
        echo "$(date -Is) $NAME: container exited $rc, next job in 15s" >&2
        sleep 15
    fi
done
