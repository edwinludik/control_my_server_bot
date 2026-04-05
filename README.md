# Control My Server Telegram Bot

A Telegram bot to control your Linux server remotely via `systemctl` and `reboot`.

## Features
- `/start`: Show help and commands.
- `/status`: Check server uptime.
- `/list_services`: List available services (all running or from a whitelist).
- `/restart_service <name>`: Restart a specific service.
- `/restart_server`: Reboot the server.
- Logging to a dedicated Telegram channel.
- Whitelist for controllable services.

## Prerequisites
- Go 1.26 or higher (for building).
- Linux server with `systemd` (for running).
- A Telegram bot token (from @BotFather).
- Your Telegram user ID (owner).
- A Telegram channel ID for logs.

## Installation Options

### 1. Manual Installation
1. **Clone or copy the bot files** to your server (e.g., `/opt/control_my_server_bot`).
2. **Configure environment variables**:
   Create a `.env` file in the same directory:
   ```env
   TELEGRAM_BOT_TOKEN=your_token_here
   TELEGRAM_OWNER_ID=your_id_here
   TELEGRAM_LOG_CHANNEL_ID=your_channel_id_here
   CONTROLLED_SERVICES=nginx,docker,mysql  # Optional: comma-separated list
   ```
3. **Build the bot**:
   ```bash
   go build -o control_my_server_bot main.go
   ```
4. **Install as a systemd service**:
   - Copy the provided `control-bot.service` to `/etc/systemd/system/`:
     ```bash
     sudo cp control-bot.service /etc/systemd/system/
     ```
   - Update the `WorkingDirectory` and `ExecStart` in `/etc/systemd/system/control-bot.service` if you installed the bot in a different location.
   - Reload systemd, enable and start the service:
     ```bash
     sudo systemctl daemon-reload
     sudo systemctl enable control-bot.service
     sudo systemctl start control-bot.service
     ```

### 2. Debian Package (.deb)
For a cleaner installation on Debian-based systems (Ubuntu, Debian, etc.), you can build and install a `.deb` package.

**Building the package:**
1. Install `nfpm` (e.g., via `go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest`).
2. Run the build:
   ```bash
   make package-deb
   ```

**Installing the package:**
1. Copy the generated `.deb` file to your server.
2. Install it:
   ```bash
   sudo dpkg -i control-my-server-bot_1.0.0_amd64.deb
   ```
3. Configure the bot:
   Edit `/etc/control-my-server-bot/.env` with your credentials.
4. Restart the service:
   ```bash
   sudo systemctl restart control-my-server-bot.service
   ```

## Verify
- Check service status: `sudo systemctl status control-my-server-bot.service`
- Send `/start` to your bot on Telegram.

## Security Note
The bot uses `sudo` for `reboot` and `systemctl restart`. Ensure the user running the bot (as configured in the service file) has the necessary `sudo` permissions without password prompt if you want it to run seamlessly. In the provided template, it runs as `root`.
