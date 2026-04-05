#!/bin/sh
set -e

systemctl stop control-my-server-bot.service || true
systemctl disable control-my-server-bot.service || true
