#!/bin/sh
set -e

# Standard systemd service name
SERVICE_NAME="control_my_server_bot.service"

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME"
    systemctl start "$SERVICE_NAME" || true
fi

echo "Control My Server Bot installed and configured as a service."
echo "Please edit /etc/control_my_server_bot/.env with your Telegram credentials."
echo "Then restart the service: sudo systemctl restart $SERVICE_NAME"
