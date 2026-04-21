# Control My Server Telegram Bot

A Telegram bot to control your Linux server remotely.

## Installation

Download the latest version from the [Releases](https://github.com/edwinludik/control_my_server_bot/releases) page.

We provide packages for:
- Debian/Ubuntu (`.deb`)
- RHEL/CentOS/Fedora (`.rpm`)
- Arch Linux (`.pkg.tar.zst`)

### Systemd Integration
The packages include a systemd service file. After installation, you can:
```bash
systemctl enable --now control_my_server_bot
```

## Features
- `/start` or `/help`: Show help and commands.
- `/ping`: Return "Pong!".
- `/status`: Check server uptime, CPU, RAM, and disk space.
- `/get_cpu_usage`: Show current CPU usage.
- `/get_ram_usage`: Show current RAM usage.
- `/get_disk_usage`: Show free disk space on all drives.
- `/get_services`: List available services (all running or from a whitelist).
- `/restart_server`: Reboot the server.
- **Multi-user Support**: Add and manage additional users.
  - `/add_user <id>`: Grant full permissions to a user (Owner only).
  - `/delete_user <id>`: Remove a user (Owner only).
  - `/get_users`: List all additional authorized users (Owner only).
- Logging to a dedicated Telegram channel.
- Whitelist for controllable services.
- **High Availability**: Configured to run with elevated privileges and scheduling priority to remain responsive even when the server is under extreme load.

## Prerequisites
- Linux server with `systemd` (for running).
- A Telegram bot token (from @BotFather).
- Your Telegram user ID (owner).
- A Telegram channel ID for logs (create one if needed).
- Go 1.26 or higher – optional: if you want to build from source.

## Configuration

The bot is configured via environment variables. You need to provide these in a `.env` file in the working directory.

| Variable | Description | Default |
|----------|-------------|---------|
| `TELEGRAM_BOT_TOKEN` | **Required**. Your Telegram bot token. | |
| `TELEGRAM_OWNER_ID` | **Required**. Your Telegram User ID. | |
| `TELEGRAM_LOG_CHANNEL_ID` | **Required**. Telegram Channel ID for logs. | |
| `CONTROLLED_SERVICES` | Comma-separated list of services the bot can control. | (All available) |

## Installation Options

### 1. Manual Installation
1. **Create a dedicated system user**:
   ```bash
   sudo useradd --system --shell /bin/false --home-dir /opt/control_my_server_bot control_my_server_bot_user
   ```
2. **Clone the repository and build the bot**:
   ```bash
   git clone https://github.com/edwinludik/control_my_server_bot.git
   cd control_my_server_bot
   go build -o control_my_server_bot ./src
   ```
3. **Set up the installation directory**:
   ```bash
   sudo mkdir -p /opt/control_my_server_bot
   sudo cp control_my_server_bot /opt/control_my_server_bot/
   sudo cp apply_update.sh /opt/control_my_server_bot/
   sudo cp .env.example /opt/control_my_server_bot/.env
   # Edit /opt/control_my_server_bot/.env with your credentials
   sudo nano /opt/control_my_server_bot/.env
   ```
4. **Assign permissions**:
   ```bash
   sudo chown -R control_my_server_bot_user:control_my_server_bot_user /opt/control_my_server_bot
   sudo chmod 700 /opt/control_my_server_bot
   sudo chmod 600 /opt/control_my_server_bot/.env
   ```
5. **Install as a systemd service**:
   - Copy the provided `control_my_server_bot.service` to `/etc/systemd/system/`:
     ```bash
     sudo cp control_my_server_bot.service /etc/systemd/system/
     ```
   - Update the `User`, `Group`, `WorkingDirectory` and `ExecStart` in `/etc/systemd/system/control_my_server_bot.service` if they differ. For example, ensure it matches:
     ```ini
     User=control_my_server_bot_user
     Group=control_my_server_bot_user
     WorkingDirectory=/opt/control_my_server_bot
     ExecStart=/opt/control_my_server_bot/control_my_server_bot
     ```
   - Reload systemd, enable and start the service:
     ```bash
     sudo systemctl daemon-reload
     sudo systemctl enable control_my_server_bot.service
     sudo systemctl start control_my_server_bot.service
     ```
6. **Configure Polkit/Sudoers**:
   Follow the [Security and Responsiveness Note](#security-and-responsiveness-note) section below to allow the bot to manage services.

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
   - **Debian/Ubuntu**: `sudo dpkg -i control_my_server_bot_*.deb`
   - **RedHat/CentOS/Fedora**: `sudo rpm -i control_my_server_bot-*.rpm`
   - **Arch Linux**: `sudo pacman -U control_my_server_bot-*.pkg.tar.zst`
3. Configure the bot:
   Edit `/opt/control_my_server_bot/.env` with your credentials.
4. Restart the service:
   ```bash
   sudo systemctl restart control_my_server_bot.service
   ```

## Contributing

1. Fork the repository.
2. Create your feature branch (`git checkout -b feature/amazing-feature`).
3. Commit your changes (`git commit -m 'Add some amazing feature'`).
4. Push to the branch (`git push origin feature/amazing-feature`).
5. Open a Pull Request.

Each Pull Request and push to `main` triggers a GitHub Action that:
- Builds the Go binary.
- Runs a security scan (`govulncheck`).
- Generates Linux packages (.deb, .rpm, .pkg.tar.zst) as artifacts.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Security and Responsiveness Note
The bot runs as a dedicated non-root user (`control_my_server_bot_user`) for enhanced security. It uses Polkit (or a sudoers rule fallback) to allow restarting services and rebooting the server without requiring a password or `root` privileges for the bot process itself.

### Hardening Measures
- **Dedicated User**: The bot runs as `control_my_server_bot_user` with restricted access.
- **Polkit/Sudoers Control**: Only specific actions (`systemctl restart`, `reboot`) are permitted for the `control_my_server_bot_user` user.
- **File Permissions**: The bot automatically attempts to set restricted permissions (`0600`) on the `.env` and SQLite database files, and the installation directory is restricted to the `control_my_server_bot_user` user.
- **Error Sanitization**: System-level error details are logged to the private log channel but not sent directly to the user who triggered the command.

### Manual Sudo/Polkit Configuration (if not using packages)
If you are installing manually and don't want to run the bot as `root`, you should:
1. Create a dedicated user (e.g., `control_my_server_bot_user`).
2. Give the user ownership of the bot's directory.
3. Configure Polkit by adding a rule in `/etc/polkit-1/rules.d/10-control_my_server_bot_user.rules`:
   ```javascript
   polkit.addRule(function(action, subject) {
       if (subject.user == "control_my_server_bot_user") {
           if (action.id == "org.freedesktop.systemd1.manage-units" ||
               action.id == "org.freedesktop.login1.reboot" ||
               action.id == "org.freedesktop.login1.reboot-multiple-sessions") {
               return polkit.Result.YES;
           }
       }
   });
   ```
   *Note: This allows the user to restart ANY service. You may want to further restrict this if necessary.*

Alternatively, use `sudoers` fallback (not recommended if Polkit is available):
Add the following to `/etc/sudoers.d/control-bot`:
```sudoers
control_my_server_bot_user ALL=(ALL) NOPASSWD: /usr/bin/systemctl restart *, /usr/sbin/reboot
```
*(Ensure the bot code calls `reboot` or `systemctl` directly, and the user has permissions).*

The bot is also configured with high scheduling and I/O priority (`Nice=-10`, `CPUSchedulingPolicy=rr`, `IOSchedulingClass=realtime`) and protected from OOM-killing (`OOMScoreAdjust=-1000`). This ensures it remains responsive even when the server is under extreme load.
