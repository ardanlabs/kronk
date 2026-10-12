#!/usr/bin/env bash
# =============================================================================
# install.sh
#
# Install, upgrade, or remove the Kronk systemd service. Run via:
#
#   make install-service
#   make uninstall-service
#
# or directly:
#
#   sudo zarf/systemd/install.sh install /path/to/kronk
#   sudo zarf/systemd/install.sh uninstall
#
# install is idempotent and doubles as the upgrade path: it copies the given
# kronk binary to /usr/local/bin, installs the kronk user and the unit, and
# starts the service, or restarts it when it is already running. When the
# service has no models yet and the invoking user has a ~/.kronk, it offers
# to copy that data in (see offer_existing_data).
#
# uninstall stops and removes the unit and the sysusers entry. It keeps the
# binary, the kronk user, /var/lib/kronk (models, keys, catalog), and
# /etc/kronk.
# =============================================================================

set -euo pipefail

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BIN=/usr/local/bin/kronk
UNIT=/etc/systemd/system/kronk.service
SYSUSERS=/etc/sysusers.d/kronk.conf
STATE=/var/lib/kronk

usage() {
    echo "usage: $0 install <path-to-kronk> | uninstall" >&2
    exit 2
}

require_root() {
    if [[ $EUID -ne 0 ]]; then
        echo "$0: must run as root (use sudo)" >&2
        exit 1
    fi
}

# The service never reads ~/.kronk, because it runs as kronk and the sandbox
# hides /home. Offer a one-time copy of the invoking user's data, but only
# while the service has no models, so an upgrade never overwrites them.
offer_existing_data() {
    [[ -n "${SUDO_USER:-}" && "$SUDO_USER" != root ]] || return 0
    local home src
    home=$(getent passwd "$SUDO_USER" | cut -d: -f6)
    src="$home/.kronk"
    [[ -d "$src/models" ]] || return 0
    if [[ -d "$STATE/models" ]] && find "$STATE/models" -name '*.gguf' -print -quit | grep -q .; then
        return 0
    fi

    # Never prompt without a terminal; the default is to skip.
    [[ -t 0 ]] || return 0

    echo
    echo "Found existing Kronk data in $src ($(du -sh "$src" 2>/dev/null | cut -f1))."
    echo "The service uses $STATE and cannot read your home directory."

    # A running server writes to its data while we copy it.
    local pid
    pid=$(cat "$src/kronk.pid" 2>/dev/null || true)
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
        echo "To copy it, stop the Kronk server running from it (pid $pid) with"
        echo "'kronk server stop', then run the install again."
        return 0
    fi

    echo "Copying keeps your models, catalog, and keys; tokens you created keep working."
    local answer=""
    read -r -p "Copy $src into $STATE now? [y/N] " answer || true
    if [[ "$answer" != [yY]* ]]; then
        echo "Skipped. See Chapter 8 of the manual to copy it later."
        return 0
    fi

    if systemctl is-active --quiet kronk; then
        echo "Stopping kronk"
        systemctl stop kronk
    fi

    # Keep symlinks and modes, but not the home directory's owner or SELinux
    # labels: files take the labels of $STATE and are then owned by kronk.
    echo "Copying $src to $STATE"
    install -d -m 0750 -o kronk -g kronk "$STATE"
    cp -dR --preserve=mode,timestamps,links "$src/." "$STATE/"
    rm -f "$STATE/kronk.log" "$STATE/kronk.pid"
    chown -R kronk:kronk "$STATE"
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

    offer_existing_data

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

case "${1:-}" in
install)
    require_root
    install_service "${2:-}"
    ;;
uninstall)
    require_root
    uninstall_service
    ;;
*)
    usage
    ;;
esac
