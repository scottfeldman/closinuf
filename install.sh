#!/usr/bin/env bash
# Install closinuf as a systemd service and open the default browser on :3000 at desktop login.
# Run from the repo root: sudo ./install.sh
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
	echo "Run as root, e.g.: sudo $0" >&2
	exit 1
fi

APP_USER="${SUDO_USER:-}"
if [[ -z "${APP_USER}" || "${APP_USER}" == root ]]; then
	APP_USER=pi
fi

INSTALL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
USER_HOME="$(getent passwd "${APP_USER}" | cut -d: -f6)"
if [[ -z "${USER_HOME}" || ! -d "${USER_HOME}" ]]; then
	echo "Home directory for user '${APP_USER}' not found." >&2
	exit 1
fi

CONFIG_TXT="/boot/firmware/config.txt"
if [[ ! -f "${CONFIG_TXT}" ]]; then
	echo "Error: ${CONFIG_TXT} not found (not a Raspberry Pi OS boot partition?)." >&2
	exit 1
fi

CONFIG_NEEDS_REBOOT=0

ensure_config_line() {
	local line="$1"
	if grep -qF "${line}" "${CONFIG_TXT}"; then
		return 0
	fi
	echo "${line}" >> "${CONFIG_TXT}"
	echo "Added to ${CONFIG_TXT}: ${line}"
	CONFIG_NEEDS_REBOOT=1
}

configure_boot() {
	echo "Configuring boot (SPI)..."
	ensure_config_line "dtparam=spi=on"
	# Kernel SPI CE defaults to GPIO 8/7; the HAT uses those for manual SS/.
	ensure_config_line "dtoverlay=spi0-2cs,cs0_pin=12,cs1_pin=13"
}

configure_boot

echo "Installing Go..."
if ! command -v go >/dev/null 2>&1 ; then
	apt-get update -qq
	apt-get install -y golang-go
fi

echo "Building closinuf..."
sudo -u "${APP_USER}" env HOME="${USER_HOME}" bash -c "cd '${INSTALL_DIR}' && go build -o closinuf ."
chown "${APP_USER}:${APP_USER}" "${INSTALL_DIR}/closinuf"

# SPI / GPIO access for LS7366R and foot switch
for grp in spi gpio; do
	if getent group "${grp}" >/dev/null; then
		usermod -aG "${grp}" "${APP_USER}" 2>/dev/null || true
	fi
done

cat << 'BROWSER_SCRIPT' > /usr/local/bin/closinuf-browser.sh
#!/usr/bin/env sh
# Wait for the local app, then open Chromium fullscreen.
export DISPLAY="${DISPLAY:-:0}"
URL="http://127.0.0.1:3000"
until curl -sf "$URL" >/dev/null 2>&1; do sleep 1; done
exec chromium --start-fullscreen "$URL"
BROWSER_SCRIPT
chmod 0755 /usr/local/bin/closinuf-browser.sh

cat << BROWSER_UNIT > /etc/systemd/system/closinuf-browser.service
[Unit]
Description=Open Chromium fullscreen for closinuf (3D point scanner)
After=network-online.target graphical.target closinuf.service
Wants=network-online.target closinuf.service

[Service]
Type=simple
User=${APP_USER}
Group=${APP_USER}
Environment=DISPLAY=:0
Environment=XAUTHORITY=${USER_HOME}/.Xauthority
ExecStart=/usr/local/bin/closinuf-browser.sh
Restart=on-failure
RestartSec=5

[Install]
WantedBy=graphical.target
BROWSER_UNIT

cat << UNIT > /etc/systemd/system/closinuf.service
[Unit]
Description=closinuf 3D point scanner
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/closinuf
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable closinuf.service closinuf-browser.service
systemctl restart closinuf.service
if systemctl is-active --quiet graphical.target; then
	systemctl restart closinuf-browser.service
fi

echo
echo "Installed for user ${APP_USER}."
echo "  Service: systemctl status closinuf"
echo "  Browser: systemctl status closinuf-browser"
echo "  Logs:    journalctl -u closinuf -f"
if [[ "${CONFIG_NEEDS_REBOOT}" -eq 1 ]]; then
	echo
	echo "Reboot required — boot config changed (SPI)."
	if [[ -t 0 ]]; then
		read -r -p "Reboot now? [Y/n] " ans
		if [[ -z "${ans}" || "${ans}" =~ ^[Yy]$ ]]; then
			reboot
		fi
	fi
	echo "Run: sudo reboot"
fi
