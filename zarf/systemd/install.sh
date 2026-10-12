#!/usr/bin/env bash
# =============================================================================
# install.sh
#
# Install, upgrade, or remove the Kronk systemd service at one of two levels.
# Run via:
#
#   make install-system-service     system level: kronk user, at boot, /var/lib/kronk
#   make uninstall-system-service
#   make install-user-service       user level: your user, at login, ~/.kronk
#   make uninstall-user-service
#
# or directly:
#
#   sudo zarf/systemd/install.sh install --system-level /path/to/kronk
#   sudo zarf/systemd/install.sh uninstall --system-level
#   zarf/systemd/install.sh install --user-level /path/to/kronk
#   zarf/systemd/install.sh uninstall --user-level
#
# Both levels listen on the same ports, so only one can be installed; install
# refuses while the other level is enabled or running, or while a server
# started with `kronk server start --detach` is running from ~/.kronk.
#
# install is idempotent and doubles as the upgrade path: it copies the given
# kronk binary into place when it differs from the installed one, installs the
# unit, and starts the service, or restarts it when it is already running.
#
# System level: the binary goes to /usr/local/bin and the kronk user is
# created with sysusers. When the service has no models yet and the invoking
# user has a ~/.kronk, it points out how to copy it in or use the user level
# instead (see warn_existing_data). Uninstall keeps the binary, the kronk
# user, /var/lib/kronk, and /etc/kronk.
#
# User level: no root needed. The binary goes to
# ~/.local/share/kronk/bin/kronk and the unit to ~/.config/systemd/user. When
# the service could not use the GPU without a desktop session, it offers to
# use sudo once to grant permanent GPU access (see grant_gpu_access).
# Uninstall keeps the binary and ~/.kronk.
# =============================================================================

set -euo pipefail

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BIN=/usr/local/bin/kronk
UNIT=/etc/systemd/system/kronk.service
SYSUSERS=/etc/sysusers.d/kronk.conf
STATE=/var/lib/kronk

# Deliberately not on PATH: a copy in ~/.local/bin would shadow the kronk you
# upgrade with go install or brew, and be picked up again as the install
# source, so upgrades would silently reinstall the old binary. Must match
# ExecStart= in kronk-user.service.
USER_BIN="$HOME/.local/share/kronk/bin/kronk"
USER_UNIT="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/kronk.service"
NVIDIA_UNIT=/etc/systemd/system/kronk-nvidia-uvm.service

usage() {
    echo "usage: $0 install --system-level|--user-level <path-to-kronk>" >&2
    echo "       $0 uninstall --system-level|--user-level" >&2
    exit 2
}

require_root() {
    if [[ $EUID -ne 0 ]]; then
        echo "$0: --system-level must run as root (use sudo)" >&2
        exit 1
    fi
}

require_user() {
    if [[ $EUID -eq 0 ]]; then
        echo "$0: run --user-level as your own user, without sudo" >&2
        exit 1
    fi
}

require_binary() {
    if [[ -z "$1" || ! -x "$1" ]]; then
        echo "$0: kronk binary not found or not executable: '$1'" >&2
        exit 1
    fi
}

# Home directory of the user who ran sudo, empty when there is none.
sudo_home() {
    [[ -n "${SUDO_USER:-}" && "$SUDO_USER" != root ]] || return 0
    getent passwd "$SUDO_USER" | cut -d: -f6
}

# A server started with `kronk server start --detach` listens on the same
# ports as the service and, at user level, writes to the same ~/.kronk.
# Refuse while one is running from the given data directory.
check_detached() {
    local pid
    pid=$(cat "$1/kronk.pid" 2>/dev/null || true)
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
        echo "$0: a Kronk server is running from $1 (pid $pid); stop it with 'kronk server stop' first" >&2
        exit 1
    fi
}

# An enabled but stopped service still starts at the next boot, so check
# both enabled and active.
check_system_installed() {
    if systemctl is-enabled --quiet kronk 2>/dev/null ||
        systemctl is-active --quiet kronk 2>/dev/null; then
        echo "$0: kronk is already installed at system level; run make uninstall-system-service first" >&2
        exit 1
    fi
}

# Runs as root under sudo, so check the invoking user's service manager.
check_user_installed() {
    [[ -n "${SUDO_USER:-}" && "$SUDO_USER" != root ]] || return 0
    local m=(--user --machine="${SUDO_USER}@")
    if systemctl "${m[@]}" is-enabled --quiet kronk 2>/dev/null ||
        systemctl "${m[@]}" is-active --quiet kronk 2>/dev/null; then
        echo "$0: kronk is already installed at user level; run make uninstall-user-service first" >&2
        exit 1
    fi
}

# =============================================================================
# System level

# The service never reads ~/.kronk, because it runs as kronk and the sandbox
# hides /home. Point out an existing ~/.kronk with models while the service
# has none yet, so the warning does not repeat on every upgrade.
warn_existing_data() {
    local home
    home=$(sudo_home)
    [[ -n "$home" && -d "$home/.kronk/models" ]] || return 0
    if [[ -d "$STATE/models" ]] && find "$STATE/models" -name '*.gguf' -print -quit | grep -q .; then
        return 0
    fi

    echo
    echo "Note: you have existing Kronk data in $home/.kronk ($(du -sh "$home/.kronk" 2>/dev/null | cut -f1))."
    echo "The system service uses $STATE and cannot read your home directory. Either:"
    echo "  - copy it in by hand: see \"Using Data from an Existing Installation\" in Chapter 8"
    echo "    of the manual, or"
    echo "  - use ~/.kronk in place with the user-level service instead:"
    echo "    make uninstall-system-service && make install-user-service"
}

install_system() {
    local src="${1:-}"
    require_binary "$src"
    check_user_installed
    check_detached "$(sudo_home)/.kronk"

    # install creates a new file, which also gives it the correct SELinux label.
    if ! cmp -s "$src" "$BIN"; then
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
    echo "Kronk is installed at system level. Follow startup with: journalctl -u kronk -f"
    warn_existing_data
}

uninstall_system() {
    if systemctl list-unit-files kronk.service --no-legend 2>/dev/null | grep -q kronk; then
        echo "Stopping and disabling kronk"
        systemctl disable --now kronk || true
    fi

    rm -f "$UNIT" "$SYSUSERS"
    systemctl daemon-reload
    systemctl reset-failed kronk 2>/dev/null || true

    echo
    echo "Kronk system service removed. Kept: $BIN, the kronk user, /var/lib/kronk, /etc/kronk."
}

# =============================================================================
# User level

# Prints the comma-separated groups that own /dev/kfd and /dev/dri/renderD*
# (AMD, Intel, Vulkan) and that you are not in. Devices that are world
# accessible, as on Fedora, need no group.
missing_gpu_groups() {
    # Read the group database, not this process, so a membership added by an
    # earlier run counts before you log in again.
    local groups missing=() dev mode grp
    groups=" $(id -nG "$USER") "
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
        if [[ -n "$groups" ]]; then
            # The user service manager keeps the groups it started with. Once
            # every session ends it stops, and the next login starts it, and
            # Kronk with it, with the new groups.
            echo "GPU access is set up. The new group applies only to new sessions: log out"
            echo "of all your sessions (desktop and SSH) and log back in, and Kronk starts"
            echo "with GPU access."
        else
            echo "GPU access is set up."
        fi
        return 0
    fi

    echo
    echo "Warning: GPU setup failed; without a desktop session the service uses the CPU."
    echo "Run make install-user-service again to retry."
}

install_user() {
    local src="${1:-}"
    require_binary "$src"
    check_system_installed
    check_detached "$HOME/.kronk"

    if ! cmp -s "$src" "$USER_BIN"; then
        echo "Installing $src to $USER_BIN"
        install -D -m 0755 "$src" "$USER_BIN"
    fi

    # BindPaths needs the directory to exist before the first start.
    mkdir -p "$HOME/.kronk"

    echo "Installing $USER_UNIT"
    install -D -m 0644 "$SRC_DIR/kronk-user.service" "$USER_UNIT"
    systemctl --user daemon-reload

    # Before starting, so the NVIDIA device nodes exist when Kronk probes the
    # GPU and no restart is needed afterwards.
    grant_gpu_access

    echo
    if systemctl --user is-active --quiet kronk; then
        echo "Restarting kronk"
        systemctl --user restart kronk
    else
        echo "Enabling and starting kronk"
        systemctl --user reset-failed kronk 2>/dev/null || true
        systemctl --user enable --now kronk
    fi

    echo
    echo "Kronk is installed at user level. Follow startup with: journalctl --user -u kronk -f"
    echo "It runs while you are logged in. To run Kronk at boot, use make install-system-service."
}

uninstall_user() {
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

# =============================================================================

case "${1:-} ${2:-}" in
"install --system-level")
    require_root
    install_system "${3:-}"
    ;;
"uninstall --system-level")
    require_root
    uninstall_system
    ;;
"install --user-level")
    require_user
    install_user "${3:-}"
    ;;
"uninstall --user-level")
    require_user
    uninstall_user
    ;;
*)
    usage
    ;;
esac
