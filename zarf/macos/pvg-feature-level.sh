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
# Run it as the user that owns the VMs — `pacha` on pachas-mac-studio. It
# needs no sudo and touches no system file.
#
# WHY
#
# macOS guests on Apple Silicon never see the real GPU. Virtualization.framework
# hands them a paravirtual device through ParavirtualizedGraphics.framework, and
# that device answers Metal capability queries at roughly Apple 5 — below the
# Apple 7 / Metal 3 line that llama.cpp uses to select its fast kernels:
#
#   ggml/src/ggml-metal/ggml-metal-device.m:731  has_simdgroup_reduction
#   ggml/src/ggml-metal/ggml-metal-device.m:734  has_simdgroup_mm
#   ggml/src/ggml-metal/ggml-metal-device.m:737  has_bfloat
#
# All three come out false in the VM, which gates SUM, SUM_ROWS, CUMSUM, MEAN,
# SOFT_MAX, GROUP_NORM, L2_NORM, ARGMAX, NORM, RMS_NORM and flash-attention onto
# CPU fallbacks (:1214-1234, :1344-1390). Every real Apple Silicon Mac is Apple 7
# or better, so the Metal CI leg currently exercises paths no kronk user hits and
# skips the ones all of them do.
#
# This preference is the host half of the workaround: it asks PVG to stop
# clamping the feature level it advertises to guests. It is read by the process
# that hosts the VM — `tart run`, spawned by the sand LaunchAgent as this user —
# so writing it into this user's defaults domain is the whole configuration.
# Tart and sand need no changes, and there is nothing here a different VM manager
# would do differently.
#
# MEASURED: THIS IS NOT THE LEVER
#
# Tested both ways on 2026-09-02, each time confirming the preference state and
# that `tart run` restarted after the change:
#
#   preference set,    shim off -> Apple7 = no
#   preference absent, shim on  -> Apple7 = yes, output identical to
#                                  preference set + shim on
#
# So the guest shim (build-metal-shim.sh) is sufficient alone, and this
# preference is not a prerequisite for it. It is currently OFF.
#
# Keep this script anyway. `metal-caps.m` measures what the device REPORTS, not
# what it can EXECUTE, and an unrestricted feature level could plausibly affect
# whether simdgroup or bfloat kernels actually run correctly once the shim
# advertises them. If the shimmed Metal leg starts failing inside kernels rather
# than at capability detection, enabling this is the first thing to try.
#
# A raised feature level is a REPORTING change. It does not add silicon. The
# paravirtual device may still implement an advertised capability incompletely,
# so a Metal CI failure after enabling this is ambiguous between a driver gap and
# a real kronk bug. Keep bare metal as the control for anything headed upstream.
# =============================================================================

set -euo pipefail

readonly DOMAIN="com.apple.gpusw.ParavirtualizedGraphics"
readonly KEY="ForceUnrestrictedDeviceFeatureLevel"
readonly AGENT="io.khoi.sand"

usage() {
    sed -n '2,60p' "$0" | sed 's/^# \{0,1\}//'
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
