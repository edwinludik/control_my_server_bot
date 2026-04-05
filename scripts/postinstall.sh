#!/bin/sh
set -e

systemctl daemon-reload
systemctl enable control-my-server-bot.service
systemctl start control-my-server-bot.service || true

echo "Control My Server Bot installed and started."
echo "Please edit /etc/control-my-server-bot/.env and restart the service."
