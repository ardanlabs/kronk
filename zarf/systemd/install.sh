#!/usr/bin/env bash
# =============================================================================
# install.sh
#
# Install, upgrade, or remove the Kronk systemd service. Run via:
#
#   make install-service            system service, kronk user, /var/lib/kronk
#   make uninstall-service
#   make install-user-service       user service, your user, ~/.kronk
#   make uninstall-user-service
#
# or directly:
#
#   sudo zarf/systemd/install.sh install /path/to/kronk
#   sudo zarf/systemd/install.sh uninstall
#   zarf/systemd/install.sh install-user /path/to/kronk
#   zarf/systemd/install.sh uninstall-user
#
# install is idempotent and doubles as the upgrade path: it copies the given
# kronk binary to /usr/local/bin, installs the kronk user and the unit, and
# starts the service, or restarts it when it is already running.
#
# uninstall stops and removes the unit and the sysusers entry. It keeps the
# binary, the kronk user, /var/lib/kronk (models, keys, catalog), and
# /etc/kronk.
#
# install-user and uninstall-user do the same for the user service without
# root: the binary goes to ~/.local/share/kronk/bin/kronk and the unit to
# ~/.config/systemd/user. uninstall-user keeps the binary and ~/.kronk.
# When the service could not use the GPU at boot, install-user uses sudo once
# to grant permanent GPU access (see grant_gpu_access).
# =============================================================================

set -euo pipefail

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BIN=/usr/local/bin/kronk
UNIT=/etc/systemd/system/kronk.service
SYSUSERS=/etc/sysusers.d/kronk.conf

# Deliberately not on PATH: a copy in ~/.local/bin would shadow the kronk you
# upgrade with go install or brew, and be picked up again as the install
# source, so upgrades would silently reinstall the old binary. Must match
# ExecStart= in kronk-user.service.
USER_BIN="$HOME/.local/share/kronk/bin/kronk"
USER_UNIT="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/kronk.service"
NVIDIA_UNIT=/etc/systemd/system/kronk-nvidia-uvm.service

usage() {
    echo "usage: $0 install <path-to-kronk> | uninstall | install-user <path-to-kronk> | uninstall-user" >&2
    exit 2
}

require_root() {
    if [[ $EUID -ne 0 ]]; then
        echo "$0: must run as root (use sudo)" >&2
        exit 1
    fi
}

install_service() {
    local src="${1:-}"
    if [[ -z "$src" || ! -x "$src" ]]; then
        echo "$0: kronk binary not found or not executable: '${src}'" >&2
        exit 1
    fi

    # install creates a new file, which also gives it the correct SELinux label.
    if [[ "$(readlink -f "$src")" != "$(readlink -f "$BIN" 2>/dev/null || true)" ]]; then
        echo "Installing $src to $BIN"
        install -m 0755 "$src" "$BIN"
    fi

    echo "Installing $SYSUSERS"
    install -D -m 0644 "$SRC_DIR/kronk.sysusers.conf" "$SYSUSERS"
    systemd-sysusers "$SYSUSERS"

    echo "Installing $UNIT"
    install -m 0644 "$SRC_DIR/kronk.service" "$UNIT"
    systemctl daemon-reload

    if systemctl is-active --quiet kronk; then
        echo "Restarting kronk"
        systemctl restart kronk
    else
        echo "Enabling and starting kronk"
        systemctl reset-failed kronk 2>/dev/null || true
        systemctl enable --now kronk
    fi

    echo
    echo "Kronk is installed. Follow startup with: journalctl -u kronk -f"
}

uninstall_service() {
    if systemctl list-unit-files kronk.service --no-legend 2>/dev/null | grep -q kronk; then
        echo "Stopping and disabling kronk"
        systemctl disable --now kronk || true
    fi

    rm -f "$UNIT" "$SYSUSERS"
    systemctl daemon-reload
    systemctl reset-failed kronk 2>/dev/null || true

    echo
    echo "Kronk service removed. Kept: $BIN, the kronk user, /var/lib/kronk, /etc/kronk."
}

require_user() {
    if [[ $EUID -eq 0 ]]; then
        echo "$0: run the user service commands as your own user, without sudo" >&2
        exit 1
    fi
}

# Both services listen on the same ports, so only one can be installed. An
# enabled but stopped service still starts at the next boot, so check both.
check_system_installed() {
    if systemctl is-enabled --quiet kronk 2>/dev/null ||
        systemctl is-active --quiet kronk 2>/dev/null; then
        echo "$0: kronk is already installed as a system service; run make uninstall-service first" >&2
        exit 1
    fi
}

# Runs as root under sudo, so check the invoking user's service manager.
check_user_installed() {
    [[ -n "${SUDO_USER:-}" ]] || return 0
    local m=(--user --machine="${SUDO_USER}@")
    if systemctl "${m[@]}" is-enabled --quiet kronk 2>/dev/null ||
        systemctl "${m[@]}" is-active --quiet kronk 2>/dev/null; then
        echo "$0: kronk is already installed as a user service; run make uninstall-user-service first" >&2
        exit 1
    fi
}

# Prints the comma-separated groups that own /dev/kfd and /dev/dri/renderD*
# (AMD, Intel, Vulkan) and that you are not in. Devices that are world
# accessible, as on Fedora, need no group.
missing_gpu_groups() {
    # Read the group database, not this process, so a membership added by an
    # earlier run counts before you log in again.
    local groups=" $(id -nG "$USER") " missing=() dev mode grp
    for dev in /dev/kfd /dev/dri/renderD*; do
        [[ -e "$dev" ]] || continue
        read -r mode grp < <(stat -c '%a %G' "$dev")
        (( (8#$mode & 8#006) == 8#006 )) && continue
        [[ "$groups" == *" $grp "* || " ${missing[*]} " == *" $grp "* ]] && continue
        missing+=("$grp")
    done
    (IFS=,; echo "${missing[*]}")
}

# A desktop session grants GPU access through a temporary device ACL, but a
# login without one, such as SSH, does not, so on Debian and Ubuntu the
# service would silently fall back to the CPU. Make the access permanent,
# which needs root once:
#   - join the groups that own the AMD/Intel GPU devices;
#   - on NVIDIA hosts, install kronk-nvidia-uvm.service, which creates the
#     device nodes at boot that the sandbox stops CUDA from creating.
grant_gpu_access() {
    local groups nvidia=""
    groups=$(missing_gpu_groups)
    if [[ -x /usr/bin/nvidia-modprobe && ! -f "$NVIDIA_UNIT" ]]; then
        nvidia=1
    fi
    [[ -n "$groups" || -n "$nvidia" ]] || return 0

    echo
    echo "To give the service permanent GPU access, including over SSH logins,"
    echo "Kronk needs root once to:"
    [[ -z "$groups" ]] || echo "  - add $USER to the ${groups} group(s): sudo usermod -aG $groups $USER"
    [[ -z "$nvidia" ]] || echo "  - install and enable $NVIDIA_UNIT"

    # Never use sudo unasked: cached credentials would skip its password prompt.
    local answer=""
    if [[ -t 0 ]]; then
        read -r -p "Run these with sudo now? [y/N] " answer || true
    fi
    if [[ "$answer" != [yY]* ]]; then
        echo "Skipped. Run make install-user-service again to set it up later."
        return 0
    fi

    if { [[ -z "$groups" ]] || sudo usermod -aG "$groups" "$USER"; } &&
        { [[ -z "$nvidia" ]] || {
            sudo install -m 0644 "$SRC_DIR/kronk-nvidia-uvm.service" "$NVIDIA_UNIT" &&
                sudo systemctl daemon-reload &&
                sudo systemctl enable --now kronk-nvidia-uvm.service
        }; }; then
        echo "GPU access is set up and takes effect from your next login."
        return 0
    fi

    echo
    echo "Warning: GPU setup failed; without a desktop session the service uses the CPU."
    echo "Run make install-user-service again to retry."
}

install_user_service() {
    local src="${1:-}"
    if [[ -z "$src" || ! -x "$src" ]]; then
        echo "$0: kronk binary not found or not executable: '${src}'" >&2
        exit 1
    fi
    check_system_installed

    if [[ "$(readlink -f "$src")" != "$(readlink -f "$USER_BIN" 2>/dev/null || true)" ]]; then
        echo "Installing $src to $USER_BIN"
        install -D -m 0755 "$src" "$USER_BIN"
    fi

    # BindPaths needs the directory to exist before the first start.
    mkdir -p "$HOME/.kronk"

    echo "Installing $USER_UNIT"
    install -D -m 0644 "$SRC_DIR/kronk-user.service" "$USER_UNIT"
    systemctl --user daemon-reload

    if systemctl --user is-active --quiet kronk; then
        echo "Restarting kronk"
        systemctl --user restart kronk
    else
        echo "Enabling and starting kronk"
        systemctl --user reset-failed kronk 2>/dev/null || true
        systemctl --user enable --now kronk
    fi

    echo
    echo "Kronk is installed. Follow startup with: journalctl --user -u kronk -f"
    echo "It runs while you are logged in. To run Kronk at boot, use make install-service."
    grant_gpu_access
}

uninstall_user_service() {
    if [[ -f "$USER_UNIT" ]]; then
        echo "Stopping and disabling kronk"
        systemctl --user disable --now kronk || true
    fi

    rm -f "$USER_UNIT"
    systemctl --user daemon-reload
    systemctl --user reset-failed kronk 2>/dev/null || true

    echo
    echo "Kronk user service removed. Kept: $USER_BIN and ~/.kronk."
    if [[ -f "$NVIDIA_UNIT" ]]; then
        echo "To also remove the NVIDIA helper: sudo systemctl disable --now kronk-nvidia-uvm && sudo rm $NVIDIA_UNIT"
    fi
}

case "${1:-}" in
install)
    require_root
    check_user_installed
    install_service "${2:-}"
    ;;
uninstall)
    require_root
    uninstall_service
    ;;
install-user)
    require_user
    install_user_service "${2:-}"
    ;;
uninstall-user)
    require_user
    uninstall_user_service
    ;;
*)
    usage
    ;;
esac
