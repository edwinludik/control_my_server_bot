#!/bin/sh
set -e

# Standard systemd service name
SERVICE_NAME="control_my_server_bot.service"
USER_NAME="control_my_server_bot_user"
INSTALL_DIR="/opt/control_my_server_bot"

# Create dedicated user if it doesn't exist
if ! id "$USER_NAME" >/dev/null 2>&1; then
    echo "Creating dedicated user: $USER_NAME"
    useradd --system --shell /bin/false --home-dir "$INSTALL_DIR" "$USER_NAME"
fi

# Set directory permissions
echo "Setting permissions for $INSTALL_DIR"
chown -R "$USER_NAME:$USER_NAME" "$INSTALL_DIR"
chmod 750 "$INSTALL_DIR"

# Polkit rule to allow cmsbot to restart services and reboot
POLKIT_RULE_DIR="/etc/polkit-1/rules.d"
if [ -d "$POLKIT_RULE_DIR" ]; then
    echo "Installing Polkit rule for $USER_NAME"
    cat > "$POLKIT_RULE_DIR/10-cmsbot.rules" <<EOF
polkit.addRule(function(action, subject) {
    if (subject.user == "$USER_NAME") {
        if (action.id == "org.freedesktop.systemd1.manage-units" ||
            action.id == "org.freedesktop.login1.reboot" ||
            action.id == "org.freedesktop.login1.reboot-multiple-sessions") {
            return polkit.Result.YES;
        }
    }
});
EOF
fi

# Fallback/Additional sudoers rule for systems without Polkit or for 'reboot' command specifically
SUDOERS_DIR="/etc/sudoers.d"
if [ -d "$SUDOERS_DIR" ]; then
    echo "Installing sudoers rule for $USER_NAME"
    echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/bin/systemctl restart *, /usr/sbin/reboot" > "$SUDOERS_DIR/control-bot"
    chmod 440 "$SUDOERS_DIR/control-bot"
fi

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME"
    systemctl start "$SERVICE_NAME" || true
fi

echo "Control My Server Bot installed and configured as a service."
echo "Running as user: $USER_NAME"
echo "Please edit /opt/control_my_server_bot/.env with your Telegram credentials."
echo "Then restart the service: sudo systemctl restart $SERVICE_NAME"
