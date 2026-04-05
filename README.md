# Control My Server Telegram Bot

A Telegram bot to control your Linux server remotely via `systemctl` and `reboot`.

## Features
- `/start` or `/help`: Show help and commands.
- `/status`: Check server uptime.
- `/list_services`: List available services (all running or from a whitelist).
- `/restart_service <name>`: Restart a specific service.
- `/restart_server`: Reboot the server.
- **Multi-user Support**: Add and manage additional users via SQLite.
  - `/add_user <id>`: Grant full permissions to a user.
  - `/delete_user <id>`: Remove a user.
  - `/list_users`: List all additional authorized users.
- Logging to a dedicated Telegram channel.
- Whitelist for controllable services.
- **High Availability**: Configured to run with elevated privileges and scheduling priority to remain responsive even when the server is under extreme load.

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
   - Copy the provided `control_my_server_bot.service` to `/etc/systemd/system/`:
     ```bash
     sudo cp control_my_server_bot.service /etc/systemd/system/
     ```
   - Update the `WorkingDirectory` and `ExecStart` in `/etc/systemd/system/control_my_server_bot.service` if you installed the bot in a different location.
   - Reload systemd, enable and start the service:
     ```bash
     sudo systemctl daemon-reload
     sudo systemctl enable control_my_server_bot.service
     sudo systemctl start control_my_server_bot.service
     ```

### 2. Linux Packages (.deb, .rpm, .pkg.tar.zst)
For a cleaner installation, you can build and install a package for your specific distribution.

**Building the packages:**
1. Install `nfpm` (e.g., via `go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest`).
2. Run the build for all platforms:
   ```bash
   make packages
   ```
   Or for a specific platform:
   ```bash
   make package-deb   # Debian/Ubuntu
   make package-rpm   # RedHat/CentOS/Fedora
   make package-arch  # Arch Linux
   ```

**Installing the package:**
1. Copy the generated package file to your server.
2. Install it:
   - **Debian/Ubuntu**: `sudo dpkg -i control_my_server_bot_1.0.0_amd64.deb`
   - **RedHat/CentOS/Fedora**: `sudo rpm -i control_my_server_bot-1.0.0.x86_64.rpm`
   - **Arch Linux**: `sudo pacman -U control_my_server_bot-1.0.0-1-x86_64.pkg.tar.zst`
3. Configure the bot:
   Edit `/etc/control_my_server_bot/.env` with your credentials.
4. Restart the service:
   ```bash
   sudo systemctl restart control_my_server_bot.service
   ```

## Verify
- Check service status: `sudo systemctl status control_my_server_bot.service`
- Send `/start` to your bot on Telegram.

## Security and Responsiveness Note
The bot uses `sudo` (implicitly via `User=root`) for `reboot` and `systemctl restart`. In the provided template, it runs as `root`.

The bot is also configured with high scheduling and I/O priority (`Nice=-10`, `CPUSchedulingPolicy=rr`, `IOSchedulingClass=realtime`) and protected from OOM-killing (`OOMScoreAdjust=-1000`). This ensures it remains responsive even when the server is under extreme load.
