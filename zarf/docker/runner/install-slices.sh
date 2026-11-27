#!/usr/bin/env bash
#
# Write the systemd slices that MEMORY_SCOPE=slice runs a fleet in. Run it as
# root once per host, and again whenever COUNT, MEMORY or FLEET_MEMORY change:
#
#   sudo PREFIX=kronk-linux-wsl2 COUNT=4 MEMORY=12g FLEET_MEMORY=48g \
#       ./install-slices.sh
#   sudo PREFIX=kronk-linux-wsl2 ./install-slices.sh --remove
#
# Pass start-runners.sh the same PREFIX, COUNT, MEMORY and FLEET_MEMORY: it
# refuses to start a fleet whose slices are missing or carry other limits.
#
#   PREFIX        the fleet's runner name prefix         (default kronk-linux-gpu)
#   COUNT         how many runner slices                 (default 1)
#   MEMORY        each runner's budget, docker size      (default 20g)
#   FLEET_MEMORY  the whole fleet's budget, docker size  (default uncapped)
#
# 0 or empty leaves that slice uncapped. A capped slice also gets
# MemorySwapMax=0, the same no-swap ceiling MEMORY_SCOPE=container sets per
# container.
#
# NAMES
#
# A dash in a slice name means nesting (a-b.slice lives in a.slice), so the
# PREFIX's dashes become underscores and only the runner index is nested:
#
#   kronk_linux_wsl2.slice      FLEET_MEMORY, every runner of the fleet
#   kronk_linux_wsl2-1.slice    MEMORY, runner kronk-linux-wsl2-1 and its sidecar
#
# Units go in /etc/systemd/system and are rewritten only when they change.
# Runner slices above COUNT are removed; one still holding containers is an
# error, because removing its unit file would not release its limits.

set -euo pipefail

PREFIX="${PREFIX:-kronk-linux-gpu}"
COUNT="${COUNT:-1}"
MEMORY="${MEMORY-20g}"
FLEET_MEMORY="${FLEET_MEMORY-}"
UNIT_DIR=/etc/systemd/system

REMOVE=false
case "${1:-}" in
    --remove) REMOVE=true ;;
    "")       ;;
    *)        echo "unknown flag: $1 (expected --remove)" >&2; exit 1 ;;
esac

if (( EUID != 0 )); then
    echo "run this as root (sudo): it writes $UNIT_DIR" >&2
    exit 1
fi

if ! [[ "$PREFIX" =~ ^[A-Za-z0-9][A-Za-z0-9_-]*$ ]]; then
    echo "PREFIX may hold only letters, digits, - and _, got: $PREFIX" >&2
    exit 1
fi

if ! [[ "$COUNT" =~ ^[0-9]+$ ]] || (( COUNT < 1 )); then
    echo "COUNT must be a positive integer, got: $COUNT" >&2
    exit 1
fi

for v in MEMORY FLEET_MEMORY; do
    if [[ -n "${!v}" && "${!v}" != "0" && ! "${!v}" =~ ^[0-9]+[bkmgBKMG]?$ ]]; then
        echo "$v must be a docker size such as 48g, 12g or 512m, got: ${!v}" >&2
        exit 1
    fi
done

base="${PREFIX//-/_}"

# Docker sizes are binary like systemd's, so only the suffix's case changes.
limits() {
    if [[ -z "$1" || "$1" == "0" ]]; then
        printf 'MemoryMax=infinity\nMemorySwapMax=infinity\n'
        return
    fi
    local n="${1%[bkmgBKMG]}" unit
    unit="$(tr 'kmg' 'KMG' <<<"${1:${#n}}")"
    [[ "$unit" == [bB] ]] && unit=""
    printf 'MemoryMax=%s%s\nMemorySwapMax=0\n' "$n" "$unit"
}

# Writes the unit only when its content differs, so a rerun is a no-op.
write_slice() {
    local unit="$1" desc="$2" size="$3" path="$UNIT_DIR/$1" content
    content="$(printf '# Written by zarf/docker/runner/install-slices.sh.\n[Unit]\nDescription=%s\n\n[Slice]\nMemoryAccounting=yes\n%s\n' \
        "$desc" "$(limits "$size")")"

    if [[ -f "$path" && "$(cat "$path")" == "$content" ]]; then
        echo "unchanged $unit"
        return
    fi
    printf '%s\n' "$content" >"$path.tmp"
    mv "$path.tmp" "$path"
    echo "wrote     $unit"
}

# Runner slices beyond COUNT (all of them with --remove), then the fleet slice.
stale=()
for path in "$UNIT_DIR/${base}"-*.slice; do
    [[ -e "$path" ]] || continue
    unit="${path##*/}"
    [[ "$unit" =~ ^${base}-([0-9]+)\.slice$ ]] || continue
    if [[ "$REMOVE" == true ]] || (( BASH_REMATCH[1] > COUNT )); then
        stale+=("$unit")
    fi
done
if [[ "$REMOVE" == true && -e "$UNIT_DIR/${base}.slice" ]]; then
    stale+=("${base}.slice")
fi

# A slice stays active after its last container leaves, so look for processes.
occupied() {
    local cg
    cg="$(systemctl show -p ControlGroup --value "$1")"
    [[ -n "$cg" && -d "/sys/fs/cgroup$cg" ]] || return 1
    [[ -n "$(find "/sys/fs/cgroup$cg" -name cgroup.procs -exec cat {} + 2>/dev/null)" ]]
}

for unit in "${stale[@]}"; do
    if occupied "$unit"; then
        echo "$unit still holds containers; stop them first:" >&2
        echo "  systemd-cgls -u $unit" >&2
        exit 1
    fi
done

for unit in "${stale[@]}"; do
    systemctl stop "$unit"
    rm -f "$UNIT_DIR/$unit"
    echo "removed   $unit"
done

if [[ "$REMOVE" == false ]]; then
    write_slice "${base}.slice" "Kronk runner fleet ${PREFIX}" "$FLEET_MEMORY"
    for (( i = 1; i <= COUNT; i++ )); do
        write_slice "${base}-${i}.slice" "Kronk runner ${PREFIX}-${i}" "$MEMORY"
    done
fi

systemctl daemon-reload

if [[ "$REMOVE" == false ]]; then
    echo
    units=("${base}.slice")
    for (( i = 1; i <= COUNT; i++ )); do units+=("${base}-${i}.slice"); done
    for unit in "${units[@]}"; do
        printf '  %-32s %s\n' "$unit" \
            "$(systemctl show -p MemoryMax -p MemorySwapMax "$unit" | tr '\n' ' ')"
    done
fi
