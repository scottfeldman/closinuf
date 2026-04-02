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

if ! command -v go >/dev/null 2>&1 || ! command -v curl >/dev/null 2>&1; then
	apt-get update -qq
	apt-get install -y golang-go curl
fi

echo "Building closinuf..."
sudo -u "${APP_USER}" env HOME="${USER_HOME}" bash -c "cd '${INSTALL_DIR}' && go build -o closinuf ."
chown "${APP_USER}:${APP_USER}" "${INSTALL_DIR}/closinuf"

cat << 'BROWSER_SCRIPT' > /usr/local/bin/closinuf-browser.sh
#!/usr/bin/env sh
# Wait for the local app, then open the desktop default browser (autostart / desktop session).
export DISPLAY="${DISPLAY:-:0}"
until curl -sf "http://127.0.0.1:3000" >/dev/null 2>&1; do sleep 1; done
exec xdg-open "http://127.0.0.1:3000"
BROWSER_SCRIPT
chmod 0755 /usr/local/bin/closinuf-browser.sh

cat << UNIT > /etc/systemd/system/closinuf.service
[Unit]
Description=closinuf (Fiber on :3000)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${APP_USER}
Group=${APP_USER}
WorkingDirectory=${INSTALL_DIR}
Environment=HOME=${USER_HOME}
ExecStart=${INSTALL_DIR}/closinuf
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable closinuf.service
systemctl restart closinuf.service

sudo -u "${APP_USER}" mkdir -p "${USER_HOME}/.config/autostart"
cat << 'DESKTOP' > "${USER_HOME}/.config/autostart/closinuf-browser.desktop"
[Desktop Entry]
Type=Application
Name=Closinuf (browser)
Exec=/usr/local/bin/closinuf-browser.sh
X-GNOME-Autostart-enabled=true
DESKTOP
chown "${APP_USER}:${APP_USER}" "${USER_HOME}/.config/autostart/closinuf-browser.desktop"

if command -v raspi-config >/dev/null 2>&1; then
	echo "Enabling desktop autologin for ${APP_USER} (raspi-config)..."
	raspi-config nonint do_boot_behaviour B2 || true
else
	echo "raspi-config not found; enable desktop autologin manually in your display manager." >&2
fi

echo
echo "Installed for user ${APP_USER}."
echo "  Service: systemctl status closinuf"
echo "  Logs:    journalctl -u closinuf -f"
echo "Reboot so autologin and autostart apply cleanly."
