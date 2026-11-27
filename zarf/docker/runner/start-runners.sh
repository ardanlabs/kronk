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
# job's writable layer. Nothing restarts the loops after a reboot, so add one
# crontab line PER FLEET (crontab -e, no root needed); a fleet without its line
# stays down after a reboot and its jobs queue with no error:
#
#   @reboot GPU=on DOCKER=none COUNT=2 APP_ID=... APP_KEY=$HOME/key.pem $HOME/start-runners.sh
#   @reboot GPU=on DOCKER=none PREFIX=kronk-linux-rocm COUNT=2 IMAGE=kronk-runner:rocm APP_ID=... APP_KEY=$HOME/key.pem $HOME/start-runners.sh
#
# Each entry must stay on ONE line: cron reads a wrapped paste as two broken
# entries. `crontab -l` should print exactly one @reboot line per fleet.
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
#   GPU         auto, on or off                             (default auto)
#               on passes /dev/dri (and /dev/kfd if present) with the host's
#               video/render GIDs, and fails if they are missing; off passes
#               no devices; auto is on when /dev/dri exists. Pin GPU=on on a
#               GPU host, so a missing driver fails here instead of quietly
#               registering runners without `gpu`.
#   DOCKER      rootless, privileged or none                (default rootless)
#               rootless gives each job its own unprivileged Docker daemon in
#               a sidecar; privileged trades that for a --privileged one with
#               fewer limits. See DOCKER IN JOBS in supervise-runner.sh.
#   DIND_IMAGE  the sidecar's image  (default docker:29-dind-rootless, or
#               docker:29-dind with DOCKER=privileged)
#   LABELS      runner labels, used verbatim when set. The default is
#               self-hosted,Linux,X64, plus gpu,<backend> with GPU on
#               (<backend> from the image's com.ardanlabs.kronk.backend
#               label). A set LABELS may not include gpu with GPU off.
#               Either way, docker is added unless DOCKER=none, and a set
#               LABELS may not include docker with DOCKER=none: jobs that
#               need a daemon (testcontainers, image builds) ask for it.
#   MEMORY      per-container memory cap, docker size    (default 20g)
#               0 or empty leaves the container uncapped
#   DIND_MEMORY the DinD sidecar's own cap, same format    (default MEMORY)
#   MEMORY_SCOPE container or slice                    (default container)
#               slice makes MEMORY one budget per runner, sidecar included;
#               see ONE BUDGET PER RUNNER below
#   FLEET_MEMORY with MEMORY_SCOPE=slice, the whole fleet's budget, docker
#               size                                    (default uncapped)
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
# control. linux.yml's API and SDK jobs can run GPU workloads concurrently;
# account for both when choosing the runner count.
#
# With Docker in jobs the sidecar is NOT inside the runner's cap: the containers a
# job starts are charged to the sidecar, which DIND_MEMORY caps separately. A
# job that uses Docker can therefore reach MEMORY + DIND_MEMORY (twice MEMORY
# by default); an idle daemon holds about 25 MiB. Lower DIND_MEMORY where jobs
# use Docker lightly and RAM is tight. MEMORY_SCOPE=slice puts both under one
# budget instead; see ONE BUDGET PER RUNNER below.
#
# The cap bounds system RAM only, not VRAM: this Strix Halo part carves GPU
# memory out of the same 62 GiB and that is not charged to the cgroup.
#
# ONE BUDGET PER RUNNER (MEMORY_SCOPE=slice)
#
# MEMORY_SCOPE=slice keeps the runner and its sidecar in separate containers
# but starts both in a systemd slice of the runner's own, so MEMORY bounds the
# runner, its daemon and every container the job starts together, and
# DIND_MEMORY must be unset. The runner slices sit in a fleet slice capped by
# FLEET_MEMORY. Neither container gets a cap of its own. A capped slice cannot
# swap, as with MEMORY_SCOPE=container. A runner past MEMORY loses its largest
# process, which may be on either side of the job; a fleet past FLEET_MEMORY
# can lose one in any of its runners, so a FLEET_MEMORY below COUNT x MEMORY
# trades isolation for density.
#
# Docker must use the systemd cgroup driver on cgroup v2, and root writes the
# slices once per host with install-slices.sh beside this file, given the same
# PREFIX, COUNT, MEMORY and FLEET_MEMORY. This script refuses to start when the
# driver or any slice's limits disagree.
#
#   sudo PREFIX=kronk-linux-wsl2 COUNT=4 MEMORY=12g FLEET_MEMORY=48g ./install-slices.sh
#   MEMORY_SCOPE=slice PREFIX=kronk-linux-wsl2 COUNT=4 MEMORY=12g FLEET_MEMORY=48g \
#       GPU=off LABELS=self-hosted,Linux,X64,wsl2 APP_ID=... APP_KEY=... ./start-runners.sh
#
# TWO FLEETS ON ONE HOST
#
# One backend per image, one fleet per backend. linux.yml's tests require
# the Vulkan fleet. A separate ROCm fleet is optional for diagnostics and
# static checks; it is no longer needed to run the test suites.
#
# The second fleet needs a PREFIX of its own: that is what keeps container
# names, runner names and the per-runner cache volumes distinct.
#
#   DOCKER=none COUNT=2 APP_ID=... APP_KEY=~/kronk-runners.pem ./start-runners.sh
#   DOCKER=none PREFIX=kronk-linux-rocm COUNT=2 IMAGE=kronk-runner:rocm \
#       APP_ID=... APP_KEY=~/kronk-runners.pem ./start-runners.sh
#
# Static checks ask only for self-hosted/Linux/X64, so either fleet can serve
# them. Test jobs additionally require gpu,vulkan.
#
# A HOST WITHOUT A GPU
#
# On a host with no /dev/dri (WSL2, a plain VM) GPU=off starts runners that
# get no devices or groups and are never labelled gpu or with a backend, so
# they take static checks and nothing GPU-bound. Any image from this directory
# works; LABELS names the host:
#
#   GPU=off LABELS=self-hosted,Linux,X64,wsl2 PREFIX=kronk-linux-wsl2 \
#       COUNT=4 MEMORY=12g APP_ID=... APP_KEY=~/kronk-runners.pem ./start-runners.sh
#
# WSL2 has /dev/dxg but no /dev/dri, so GPU=auto resolves to off there too;
# spelling it out keeps a later WSL change from turning devices on.
#
# UNDER SYSTEMD
#
# A Type=oneshot unit with RemainAfterExit=yes can run this script in place of
# the crontab line. The loops stay in the unit's cgroup (setsid does not leave
# it), so stopping the unit sends each one TERM, and it removes its runner and
# sidecar before exiting, which takes the runner offline; a running job is
# killed. The containers belong to dockerd, not to the unit, so a loop killed
# before its cleanup (TimeoutStopSec, SIGKILL) leaves them running and taking
# jobs. An ExecStopPost= removes whatever is left. Order the unit After= and
# Requires= docker.service:
#
#   [Service]
#   Type=oneshot
#   RemainAfterExit=yes
#   User=kronk
#   EnvironmentFile=/home/kronk/.kronk-runner.env
#   ExecStart=/home/kronk/kronk-runner/start-runners.sh
#   ExecStopPost=/bin/sh -c 'docker ps -aq --filter "name=^$${PREFIX}-" | xargs -r docker rm -f -v'
#   KillMode=control-group
#   TimeoutStopSec=90
#
# The env file holds the settings this script reads, one VAR=value per line.
#
# PREFIX must be unique across the org, not just the host: minting deletes an
# idle registration under the same name, so a second host reusing this host's
# names takes its runners offline.

set -euo pipefail

COUNT="${COUNT:-1}"
IMAGE="${IMAGE:-kronk-runner:vulkan}"
PREFIX="${PREFIX:-kronk-linux-gpu}"
ORG="${ORG:-ardanlabs}"
GROUP="${GROUP:-kronk}"
APP_ID="${APP_ID:-}"
APP_KEY="${APP_KEY:-}"
MEMORY="${MEMORY-20g}"
dind_memory_set="${DIND_MEMORY+set}"
DIND_MEMORY="${DIND_MEMORY-$MEMORY}"
MEMORY_SCOPE="${MEMORY_SCOPE:-container}"
FLEET_MEMORY="${FLEET_MEMORY-}"
LOG_DIR="${LOG_DIR:-$HOME/.kronk-runners}"
GPU="${GPU:-auto}"
DOCKER="${DOCKER:-rootless}"
DIND_IMAGE="${DIND_IMAGE:-}"

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

case "$GPU" in
    auto) GPU=off; [[ -e /dev/dri ]] && GPU=on ;;
    on|off) ;;
    *) echo "GPU must be auto, on or off, got: $GPU" >&2; exit 1 ;;
esac

case "$DOCKER" in
    rootless|privileged|none) ;;
    *) echo "DOCKER must be rootless, privileged or none, got: $DOCKER" >&2; exit 1 ;;
esac

# The backend is read off the image rather than hardcoded, so a vulkan image
# can never register runners advertising rocm. Test jobs select the backend
# in runs-on: a wrong label sends tests to an incompatible image, and a
# missing one leaves the job queued with no error. Without a GPU the backend
# is not advertised at all: the image can run it, the host cannot.
if [[ -z "${LABELS:-}" ]]; then
    # A JIT runner gets exactly these labels: GitHub adds no self-hosted/OS/arch
    # defaults, and linux.yml asks for self-hosted,Linux,X64.
    LABELS="self-hosted,Linux,X64"

    if [[ "$GPU" == on ]]; then
        backend="$(docker image inspect \
            -f '{{ index .Config.Labels "com.ardanlabs.kronk.backend" }}' \
            "$IMAGE" 2>/dev/null || true)"

        if [[ -z "$backend" || "$backend" == "<no value>" ]]; then
            echo "cannot read com.ardanlabs.kronk.backend from $IMAGE." >&2
            echo "Build it from zarf/docker/runner, or set LABELS explicitly." >&2
            exit 1
        fi

        LABELS+=",gpu,${backend}"
    fi
fi

# A runner that claims a GPU it lacks takes GPU jobs it can only fail.
if [[ "$GPU" == off && ",$LABELS," == *,gpu,* ]]; then
    echo "LABELS includes gpu but GPU is off: $LABELS" >&2
    exit 1
fi

# Likewise for a Docker daemon: the label follows DOCKER, so a runner without
# a sidecar never takes a job that needs one.
if [[ "$DOCKER" == none && ",$LABELS," == *,docker,* ]]; then
    echo "LABELS includes docker but DOCKER is none: $LABELS" >&2
    exit 1
fi
if [[ "$DOCKER" != none && ",$LABELS," != *,docker,* ]]; then
    LABELS+=",docker"
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
for v in MEMORY DIND_MEMORY FLEET_MEMORY; do
    if [[ -n "${!v}" && "${!v}" != "0" && ! "${!v}" =~ ^[0-9]+[bkmgBKMG]?$ ]]; then
        echo "$v must be a docker size such as 20g, 8g or 512m, got: ${!v}" >&2
        exit 1
    fi
done

cap() { if [[ -z "$1" || "$1" == "0" ]]; then echo uncapped; else echo "$1"; fi; }

# install-slices.sh's names: the PREFIX's dashes would nest, so they become
# underscores and only "-<n>" nests a runner's slice in the fleet's.
slice_base="${PREFIX//-/_}"

# What systemctl show reports for a slice install-slices.sh wrote for $1.
slice_limits() {
    if [[ -z "$1" || "$1" == "0" ]]; then
        echo "infinity infinity"
        return
    fi
    local n="${1%[bkmgBKMG]}" mult=1
    case "${1:${#n}}" in
        k|K) mult=1024 ;;
        m|M) mult=$(( 1024 ** 2 )) ;;
        g|G) mult=$(( 1024 ** 3 )) ;;
    esac
    echo "$(( n * mult )) 0"
}

# Checked here rather than in a loop's first job: docker would otherwise create
# a missing slice on the spot, with no limits at all.
check_slice() {
    local unit="$1" want key val state="" path="" max="" swap=""
    want="$(slice_limits "$2")"
    while IFS='=' read -r key val; do
        case "$key" in
            LoadState)     state="$val" ;;
            FragmentPath)  path="$val" ;;
            MemoryMax)     max="$val" ;;
            MemorySwapMax) swap="$val" ;;
        esac
    done < <(systemctl show -p LoadState -p FragmentPath \
        -p MemoryMax -p MemorySwapMax "$unit")
    if [[ "$state" != loaded || -z "$path" ]]; then
        echo "$unit is not installed; as root, run:" >&2
        echo "  PREFIX=$PREFIX COUNT=$COUNT MEMORY=$MEMORY FLEET_MEMORY=$FLEET_MEMORY ${here}/install-slices.sh" >&2
        return 1
    fi
    if [[ "$max $swap" != "$want" ]]; then
        echo "$unit has MemoryMax=$max MemorySwapMax=$swap, want $want; as root, run:" >&2
        echo "  PREFIX=$PREFIX COUNT=$COUNT MEMORY=$MEMORY FLEET_MEMORY=$FLEET_MEMORY ${here}/install-slices.sh" >&2
        return 1
    fi
}

case "$MEMORY_SCOPE" in
    container)
        if [[ -n "$FLEET_MEMORY" ]]; then
            echo "FLEET_MEMORY needs MEMORY_SCOPE=slice" >&2
            exit 1
        fi
        mem_desc="$(cap "$MEMORY")"
        if [[ "$DOCKER" != none ]]; then
            mem_desc+=", dind $(cap "$DIND_MEMORY")"
        fi ;;
    slice)
        if [[ -n "$dind_memory_set" ]]; then
            echo "DIND_MEMORY has no effect with MEMORY_SCOPE=slice: MEMORY covers the sidecar too" >&2
            exit 1
        fi
        if ! [[ "$PREFIX" =~ ^[A-Za-z0-9][A-Za-z0-9_-]*$ ]]; then
            echo "with MEMORY_SCOPE=slice, PREFIX may hold only letters, digits, - and _, got: $PREFIX" >&2
            exit 1
        fi
        driver="$(docker info -f '{{.CgroupDriver}} {{.CgroupVersion}}')"
        if [[ "$driver" != "systemd 2" ]]; then
            echo "MEMORY_SCOPE=slice needs docker's systemd cgroup driver on cgroup v2, got: $driver" >&2
            exit 1
        fi
        check_slice "${slice_base}.slice" "$FLEET_MEMORY" || exit 1
        for (( i = 1; i <= COUNT; i++ )); do
            check_slice "${slice_base}-${i}.slice" "$MEMORY" || exit 1
        done ;;
    *)
        echo "MEMORY_SCOPE must be container or slice, got: $MEMORY_SCOPE" >&2
        exit 1 ;;
esac

# Numeric GIDs from the host. Group NAMES would resolve against the
# container's /etc/group, where render/video may not exist, and an unset
# variable yields --group-add "" which docker rejects outright.
VIDEO_GID=""
RENDER_GID=""
if [[ "$GPU" == on ]]; then
    if [[ ! -e /dev/dri ]]; then
        echo "GPU=on but this host has no /dev/dri (use GPU=off on a host without a GPU)" >&2
        exit 1
    fi

    VIDEO_GID="$(getent group video  | cut -d: -f3)"
    RENDER_GID="$(getent group render | cut -d: -f3)"
    if [[ -z "$VIDEO_GID" || -z "$RENDER_GID" ]]; then
        echo "could not resolve video/render GIDs on this host" >&2
        exit 1
    fi
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
            docker rm -f -v "$name" "${name}-dind" >/dev/null 2>&1 || true

            # The old loop runs its cleanup once its container is gone; a new
            # loop started before then would have its sidecar removed by it.
            for (( t = 0; t < 30; t++ )); do
                pgrep -f "supervise-runner.sh $name\$" >/dev/null || break
                sleep 1
            done
        else
            echo "$name is already supervised, skipping (use --recreate to replace)"
            continue
        fi
    fi

    docker volume create "${name}-kronk"   >/dev/null
    docker volume create "${name}-go"      >/dev/null
    docker volume create "${name}-gocache" >/dev/null
    docker volume create "${name}-diag"    >/dev/null

    slice=""
    if [[ "$MEMORY_SCOPE" == slice ]]; then
        slice="${slice_base}-${i}.slice"
        mem_desc="$(cap "$MEMORY") in $slice, fleet $(cap "$FLEET_MEMORY")"
    fi

    # setsid so the loop outlives this shell and its ssh session.
    IMAGE="$IMAGE" LABELS="$LABELS" MEMORY="$MEMORY" ORG="$ORG" GROUP="$GROUP" \
    APP_ID="$APP_ID" APP_KEY="$APP_KEY" \
    GPU="$GPU" VIDEO_GID="$VIDEO_GID" RENDER_GID="$RENDER_GID" \
    DOCKER="$DOCKER" DIND_IMAGE="$DIND_IMAGE" DIND_MEMORY="$DIND_MEMORY" \
    MEMORY_SCOPE="$MEMORY_SCOPE" SLICE="$slice" \
        setsid nohup "${here}/supervise-runner.sh" "$name" \
        >>"$LOG_DIR/${name}.log" 2>&1 &
    disown || true

    echo "started $name (labels: ${LABELS}, gpu: ${GPU}, docker: ${DOCKER}, memory: ${mem_desc}, log: $LOG_DIR/${name}.log)"
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
