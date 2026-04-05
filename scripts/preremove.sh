#!/bin/sh
set -e

SERVICE_NAME="control-my-server-bot.service"

if command -v systemctl >/dev/null 2>&1; then
    systemctl stop "$SERVICE_NAME" || true
    systemctl disable "$SERVICE_NAME" || true
fi
