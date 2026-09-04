#!/usr/bin/env bash
# =============================================================================
# pvg-feature-level.sh
#
# Toggle Apple's unrestricted ParavirtualizedGraphics feature level for the
# macOS VMs launched by THIS user, and restart them so the change takes.
#
#   ./pvg-feature-level.sh status
#   ./pvg-feature-level.sh enable
#   ./pvg-feature-level.sh disable
#
# Run it as the user that owns the VMs — `pacha` on pachas-mac-studio. It needs
# no sudo and touches no system file: the preference is read by `tart run`,
# spawned by the sand LaunchAgent as this user, so this user's defaults domain
# is the whole configuration.
#
# WHY
#
# A macOS guest on Apple Silicon only ever sees a paravirtual GPU, and that
# device answers Metal capability queries at roughly Apple 5 — below the
# Apple 7 / Metal 3 line llama.cpp uses to pick its fast kernels
# (ggml-metal-device.m:731-737, which gate SOFT_MAX, RMS_NORM, CUMSUM,
# flash attention and more onto CPU fallbacks). This preference asks PVG to
# stop clamping the level it advertises.
#
# MEASURED 2026-09-02: THIS IS NOT THE LEVER
#
# Preference set with the shim off gave Apple7 = no; the guest shim
# (build-metal-shim.sh) alone gave Apple7 = yes, with output identical to shim
# plus preference. The preference is currently OFF, and is not a prerequisite.
#
# Kept as a lead: metal-caps.m measures what the device REPORTS, not what it
# EXECUTES, and a raised feature level adds no silicon. If a shimmed Metal leg
# starts failing inside kernels rather than at capability detection, enabling
# this is the first thing to try — and keep bare metal as the control, since a
# failure under it is ambiguous between a driver gap and a real kronk bug.
# =============================================================================

set -euo pipefail

readonly DOMAIN="com.apple.gpusw.ParavirtualizedGraphics"
readonly KEY="ForceUnrestrictedDeviceFeatureLevel"
readonly AGENT="io.khoi.sand"

usage() {
    sed -n '2,36p' "$0" | sed 's/^# \{0,1\}//'
    exit "${1:-0}"
}

# The VMs are ephemeral GitHub Actions runners and sand keeps them alive, so a
# restart is how the preference reaches a new `tart run` — but it also kills
# whatever job is mid-flight. Refuse to do that silently.
restart_sand() {
    local uid; uid="$(id -u)"

    if ! launchctl print "gui/${uid}/${AGENT}" >/dev/null 2>&1; then
        echo "warning: ${AGENT} is not loaded for uid ${uid}; restart the VMs yourself" >&2
        return 0
    fi

    echo "restarting ${AGENT} (this kills any job currently running on these runners)"
    launchctl kickstart -k "gui/${uid}/${AGENT}"
    echo "restarted. Give the VMs a minute to re-register before dispatching work."
}

confirm() {
    [[ "${ASSUME_YES:-}" == "1" ]] && return 0

    read -r -p "$1 [y/N] " reply
    [[ "$reply" == "y" || "$reply" == "Y" ]]
}

cmd_status() {
    echo "domain: ${DOMAIN}"

    if ! defaults read "$DOMAIN" >/dev/null 2>&1; then
        echo "state : UNSET (guests get the clamped, Apple 5-class feature level)"
        return 0
    fi

    local value
    value="$(defaults read "$DOMAIN" "$KEY" 2>/dev/null || echo "<key absent>")"
    echo "state : ${KEY} = ${value}"

    echo
    echo "running VMs (a preference change only reaches a VM started after it):"
    pgrep -fl 'tart run' | sed 's/^/  /' || echo "  none"
}

cmd_enable() {
    confirm "Enable ${KEY} and restart the runners?" || { echo "aborted"; exit 1; }

    defaults write "$DOMAIN" "$KEY" -bool true
    echo "wrote ${KEY} = 1"
    restart_sand
}

cmd_disable() {
    confirm "Remove ${KEY} and restart the runners?" || { echo "aborted"; exit 1; }

    # `defaults delete` on an absent key exits non-zero; that is a no-op here,
    # not a failure, so it must not trip `set -e`.
    defaults delete "$DOMAIN" "$KEY" 2>/dev/null || echo "${KEY} was not set"
    echo "removed ${KEY}"
    restart_sand
}

case "${1:-}" in
    status)          cmd_status  ;;
    enable)          cmd_enable  ;;
    disable)         cmd_disable ;;
    -h|--help|help)  usage 0     ;;
    "")              usage 1     ;;
    *)               echo "unknown command: $1" >&2; usage 1 ;;
esac
