#!/bin/bash
set -e

APP_DIR="/opt/control_my_server_bot"
APP_NAME="control_my_server_bot"
NEW_BINARY="${UPDATE_DIR}/${APP_NAME}.new"
OLD_BINARY="${APP_DIR}/${APP_NAME}.old"
APP_BINARY="${APP_DIR}/${APP_NAME}"

# If there is no update, just start the service
if [ ! -f "$NEW_BINARY" ]; then
    exit 0
fi

echo "Applying update..."

# Create a backup of the current binary
if [ -f "$APP_BINARY" ]; then
    mv "$APP_BINARY" "$OLD_BINARY"
fi

# Move the new binary to replace the current one
if mv "$NEW_BINARY" "$APP_BINARY"; then
    chmod 600 "$APP_BINARY"
    echo "Update applied successfully."
else
    echo "Failed to apply update, restoring backup..."
    if [ -f "$OLD_BINARY" ]; then
        rm -f "$APP_BINARY"
        mv "$OLD_BINARY" "$APP_BINARY"
    fi
    exit 1
fi

exit 0
