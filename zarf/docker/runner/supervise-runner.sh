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
# `--exec CMD...` after the name starts the containers exactly as for a job but
# runs CMD in place of the runner, then tears everything down. It needs no
# App key or labels. Give it a name no live runner uses, or it replaces that
# runner's containers:
#
#   IMAGE=kronk-runner:vulkan ./supervise-runner.sh kronk-check --exec \
#       bash -c 'docker run --rm hello-world'
#
# With MEMORY_SCOPE=slice it also needs SLICE, a slice install-slices.sh wrote
# (docker would create a missing one with no limits).
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
#     job. Only the named cache volumes persist, and _diag is one of them.
#
# The cost is that nothing restarts these after a reboot; --restart=always used
# to cover that. start-runners.sh documents the crontab lines that do.
#
# DOCKER IN JOBS
#
# Unless DOCKER=none, each job gets a Docker daemon of its own: a sidecar named
# <name>-dind, started before the runner and removed with it. The runner shares
# the sidecar's network namespace and reaches the daemon at
# tcp://127.0.0.1:2375 (DOCKER_HOST), which nothing outside that namespace can
# reach. Published ports bind 127.0.0.1 by default (dockerd --ip), so a port a
# job publishes (`docker run -p 8080:80`, testcontainers) is reachable from the
# job at 127.0.0.1, testcontainers needs no host override, and no other runner
# can reach it. Each runner also has a bridge network of its own, so runners
# cannot reach each other at all. /_work is a volume mounted in both
# containers, so a workspace path given to `docker run -v` resolves in the
# daemon as it does in the job.
#
# The daemon's storage and /_work are per-job volumes, removed at job end with
# both containers, so a job's containers, networks, images and build cache do
# not reach the next job or any other runner. Images are pulled fresh per job.
# Neither container mounts the host's Docker socket or any host path holding
# the App key, so `docker ps` in a job lists only that job's containers.
#
# DOCKER=rootless, the default, is docker:dind-rootless, NOT privileged. The
# daemon runs as an unprivileged user (uid 1000, no capabilities), and root in
# the job's containers is a subordinate uid on the host. The sidecar needs:
#
#   seccomp, apparmor and systempaths unconfined  — rootlesskit creates user,
#       mount and network namespaces and mounts /proc in them, which the
#       default profiles and masked /proc paths forbid
#   label=disable  — SELinux labelling would deny the same mounts
#   /dev/net/tun   — slirp4netns, the daemon's network
#   /sys bound rw  — a fresh sysfs mount (buildx's docker-container driver,
#       nested --privileged) is refused under a read-only /sys. Without
#       capabilities the sidecar still cannot write sysfs.
#
# Two limits follow: a container the job starts cannot WRITE to a bind-mounted
# workspace directory (it is root's, and container root is not), so copy
# results out with `docker cp` or `docker build --output`, or chmod the
# directory first; and nested --memory/--cpus are ignored (the sidecar's cap,
# or the runner's slice, still holds).
#
# DOCKER=privileged is docker:dind with --privileged and has neither limit. It
# is host-root-equivalent for a job that sets out to escape: it can start a
# privileged container of its own and mount the host's disks, the App key
# included. Use it only on a host where every job is trusted to that degree.
#
# Either way the sidecar runs code from the job, and that is acceptable only
# because the kronk group's runners take reviewed same-repo code: linux.yml
# sends fork PRs to ubuntu-latest and runs one here only when a maintainer
# dispatches its reviewed SHA. A workflow that lets unreviewed code reach a
# self-hosted runner breaks that, with or without Docker.
#
# DOCKER=none runs the runner alone: no sidecar, no extra privileges.
#
# HOW A JOB ENDS
#
# A JIT runner exits when its job finishes; the loop then removes the sidecar
# and the per-job volumes, and mints the next config. Between jobs there is a
# runner up and listening, so `docker ps` looks the same as it did before.

set -euo pipefail

NAME="${1:?the runner name is required}"
shift

EXEC=false
case "${1:-}" in
    --exec) EXEC=true; shift; (( $# > 0 )) || { echo "--exec needs a command" >&2; exit 1; } ;;
    "")     ;;
    *)      echo "unknown argument: $1 (expected --exec CMD...)" >&2; exit 1 ;;
esac

IMAGE="${IMAGE:?IMAGE is required}"
if [[ "$EXEC" == false ]]; then
    LABELS="${LABELS:?LABELS is required}"
    APP_ID="${APP_ID:?APP_ID is required}"
    APP_KEY="${APP_KEY:?APP_KEY is required}"
fi
ORG="${ORG:-ardanlabs}"
GROUP="${GROUP:-kronk}"
MEMORY="${MEMORY-20g}"
DIND_MEMORY="${DIND_MEMORY-$MEMORY}"
GPU="${GPU:-on}"
DOCKER="${DOCKER:-rootless}"

case "$DOCKER" in
    rootless)   DIND_IMAGE="${DIND_IMAGE:-docker:29-dind-rootless}" ;;
    privileged) DIND_IMAGE="${DIND_IMAGE:-docker:29-dind}" ;;
    none)       ;;
    *)          echo "DOCKER must be rootless, privileged or none, got: $DOCKER" >&2; exit 1 ;;
esac

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

MEMORY_SCOPE="${MEMORY_SCOPE:-container}"

# MEMORY_SCOPE=container: both flags carry the same value so the cap is a
# ceiling: --memory on its own leaves --memory-swap at twice the limit, and
# swapping a 16 GiB test into this host's 8 GiB of swap is slower than failing
# it. start-runners.sh validates. The sidecar is capped on its own by
# DIND_MEMORY (default MEMORY): the job's containers are charged to it, not to
# the runner.
#
# MEMORY_SCOPE=slice: neither container is capped; both start in SLICE, whose
# limits install-slices.sh set and start-runners.sh checked. MEMORY and
# DIND_MEMORY are not read.
mem_args=()
dind_mem_args=()
case "$MEMORY_SCOPE" in
    container)
        if [[ -n "$MEMORY" && "$MEMORY" != "0" ]]; then
            mem_args=(--memory "$MEMORY" --memory-swap "$MEMORY")
        fi
        if [[ -n "$DIND_MEMORY" && "$DIND_MEMORY" != "0" ]]; then
            dind_mem_args=(--memory "$DIND_MEMORY" --memory-swap "$DIND_MEMORY")
        fi ;;
    slice)
        mem_args=(--cgroup-parent "${SLICE:?SLICE is required with MEMORY_SCOPE=slice}")
        dind_mem_args=("${mem_args[@]}") ;;
    *)
        echo "MEMORY_SCOPE must be container or slice, got: $MEMORY_SCOPE" >&2
        exit 1 ;;
esac

# GPU=off (a host without /dev/dri, such as WSL2) passes no devices or groups.
# start-runners.sh resolves GPU=auto and the GIDs before starting this loop.
gpu_args=()
if [[ "$GPU" == on ]]; then
    gpu_args=(--device /dev/dri
        --group-add "${VIDEO_GID:?VIDEO_GID is required with GPU=on}"
        --group-add "${RENDER_GID:?RENDER_GID is required with GPU=on}")
    if [[ -e /dev/kfd ]]; then
        gpu_args+=(--device /dev/kfd)
    fi
fi

dind="${NAME}-dind"
net="${NAME}-net"
net_args=(--network "$net")
if [[ "$DOCKER" != none ]]; then
    net_args=(--network "container:$dind"
        -v "${NAME}-work:/_work"
        -e DOCKER_HOST=tcp://127.0.0.1:2375)
fi

# The daemon also listens on the default unix socket inside its own namespace,
# for containers that mount it (container actions, testcontainers' reaper).
daemon_args=(--host=unix:///var/run/docker.sock
    --host=tcp://127.0.0.1:2375 --tls=false --ip=127.0.0.1)
dind_args=()
case "$DOCKER" in
    rootless)
        dind_args=(--security-opt seccomp=unconfined
            --security-opt apparmor=unconfined
            --security-opt systempaths=unconfined
            --security-opt label=disable
            --device /dev/net/tun
            -v /sys:/sys
            -e DOCKERD_ROOTLESS_ROOTLESSKIT_FLAGS="-p 127.0.0.1:2375:2375/tcp"
            # dockerd picks published ports from 32768-60999 in rootlesskit's
            # namespace; rootlesskit binds them here, where the job's client
            # sockets live. Disjoint ranges, or a bind fails "address already
            # in use".
            --sysctl net.ipv4.ip_local_port_range="61000 65535"
            -v "${NAME}-dind-data:/home/rootless/.local/share/docker")
        daemon_args+=(--host=unix:///run/user/1000/docker.sock) ;;
    privileged)
        dind_args=(--privileged
            -v "${NAME}-dind-data:/var/lib/docker") ;;
esac

# The containers belong to dockerd, not to this loop, so killing the loop
# leaves them running, and the runner still taking jobs, unless they are
# removed here. A SIGKILL skips this; see UNDER SYSTEMD in start-runners.sh.
# Removing the sidecar kills every container the job started, since they all
# live inside it. -v also removes the anonymous volume the dind images declare
# for /var/lib/docker, which would otherwise leak one per job.
cleanup() {
    docker rm -f -v "$NAME" "$dind" >/dev/null 2>&1 || true
    docker volume rm -f "${NAME}-work" "${NAME}-dind-data" >/dev/null 2>&1 || true
}

# systemctl stop signals every process in the unit's cgroup, again and again
# until no new one appears, so the docker commands cleanup forks would get TERM
# too and die with the containers still running. An ignored signal stays
# ignored across exec, and SIGKILL still ends them at the unit's timeout.
on_term() {
    trap '' TERM INT
    cleanup
    exit 0
}
trap on_term TERM INT

# A sleep that a TERM interrupts, for the same reason as run_runner's wait.
pause() {
    sleep "$1" &
    wait "$!"
}

# A bridge of its own per runner: docker isolates user-defined bridges from
# each other, so one runner's job cannot reach another's ports.
ensure_network() {
    docker network inspect "$net" >/dev/null 2>&1 ||
        docker network create "$net" >/dev/null
}

# A fresh daemon takes a few seconds, and a job must not see it half-started.
start_dind() {
    docker run -d --rm \
        --name "$dind" \
        --network "$net" \
        "${dind_mem_args[@]}" \
        "${dind_args[@]}" \
        -v "${NAME}-work:/_work" \
        "$DIND_IMAGE" \
        dockerd "${daemon_args[@]}" >/dev/null || return 1

    for (( t = 0; t < 60; t++ )); do
        if docker exec "$dind" docker -H tcp://127.0.0.1:2375 version >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    echo "$(date -Is) $NAME: $dind did not answer within 60s" >&2
    docker logs --tail 20 "$dind" >&2 || true
    return 1
}

# Starts whatever the job needs beside the runner; false if it could not.
prepare() {
    cleanup
    ensure_network || return 1
    if [[ "$DOCKER" != none ]]; then
        start_dind || return 1
    fi
}

# The image's own entrypoint is bypassed: it registers with config.sh and a
# registration token, which is the flow the App key existed for. run.sh
# --jitconfig needs no token, and the group, labels and work folder (/_work,
# mint-jitconfig.sh's default) are already inside the config.
#
# RUNNER_ALLOW_RUNASROOT: run.sh refuses to start as root without it, and
# this image runs as root.
#
# In the background with a wait, because bash runs a trap only once the
# foreground command returns, and docker run passes TERM to the container
# instead of exiting. A wait returns as the signal arrives, so stopping the
# loop (pkill, systemctl stop) removes the containers at once and the runner
# goes offline, rather than the loop being killed with them still running.
run_runner() {
    local entrypoint="$1"
    shift
    docker run --rm \
        --name "$NAME" \
        "${mem_args[@]}" \
        "${gpu_args[@]}" \
        "${net_args[@]}" \
        --security-opt seccomp=unconfined \
        -v "${NAME}-kronk:/root/.kronk" \
        -v "${NAME}-go:/root/go" \
        -v "${NAME}-gocache:/root/.cache/go-build" \
        -v "${NAME}-diag:/actions-runner/_diag" \
        -e RUNNER_ALLOW_RUNASROOT=1 \
        --entrypoint "$entrypoint" \
        "$IMAGE" "$@" &
    wait "$!"
}

if [[ "$EXEC" == true ]]; then
    rc=0
    if prepare; then
        run_runner "$@" || rc=$?
    else
        rc=1
    fi
    cleanup
    exit "$rc"
fi

while true; do
    # Before minting, so a sidecar that cannot start does not burn a config.
    if ! prepare; then
        echo "$(date -Is) $NAME: could not start $dind, retrying in 60s" >&2
        pause 60
        continue
    fi

    if ! jit="$(NAME="$NAME" LABELS="$LABELS" ORG="$ORG" GROUP="$GROUP" \
        APP_ID="$APP_ID" APP_KEY="$APP_KEY" "$here/mint-jitconfig.sh")"; then
        echo "$(date -Is) $NAME: could not mint a JIT config, retrying in 60s" >&2
        pause 60
        continue
    fi

    rc=0
    run_runner /actions-runner/run.sh --jitconfig "$jit" || rc=$?

    # At job end, not at the next start: whatever the job left running in its
    # daemon goes now.
    cleanup

    # A job that fails is a normal exit here. A container that cannot start at
    # all would otherwise spin, burning JIT configs.
    if (( rc != 0 )); then
        echo "$(date -Is) $NAME: container exited $rc, next job in 15s" >&2
        pause 15
    fi
done
