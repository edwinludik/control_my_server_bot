# Security Policy

## Supported Versions

The following versions of Control My Server Bot are currently being supported with security updates:

| Version | Supported |
|---------|----------|
| 1.2.x   | Yes      |
| 1.1.x   | Yes      |
| 1.0.x   | No       |

**Note:** Only the latest major version and the previous major version receive security updates. We recommend all users upgrade to the latest stable version.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.**

If you discover a security vulnerability in Control My Server Bot, please report it responsibly:

1. **Email**: Send a detailed report to [edwin.ludik@gmail.com](mailto:edwin.ludik@gmail.com)
2. **Subject**: Use "[SECURITY] Control My Server Bot Vulnerability" as the subject line
3. **Include**:
   - A clear description of the vulnerability
   - Steps to reproduce the issue
   - Impact assessment (if known)
   - Your contact information
   - Any proof-of-concept code (if available)

We will acknowledge your report within 48 hours and provide a timeline for resolution.

## Security Best Practices

### For Users

1. **Run as dedicated user**: Always run the bot as the `control_my_server_bot_user` user, never as root
2. **Restrict permissions**: Use Polkit or sudoers to grant only necessary permissions
3. **Secure .env file**: Ensure your `.env` file has 0600 permissions and contains only the bot token
4. **Network isolation**: Consider running the bot on an internal network or with firewall restrictions
5. **Regular updates**: Keep the bot updated to receive security patches
6. **Token security**: Never share your Telegram bot token; rotate it immediately if compromised

### For Contributors

1. **Security review**: All pull requests undergo security scanning with `govulncheck` and `gosec`
2. **Dependency scanning**: Go modules are regularly audited for vulnerabilities
3. **Input validation**: Always validate and sanitize user input in commands
4. **Error handling**: Never expose sensitive system information in error messages to users

## Security Features

This bot implements several security measures:

- **Dedicated non-root user**: Runs as a separate system user with limited privileges
- **Authorization checks**: All commands validate user permissions before execution
- **Polkit/Sudoers integration**: Fine-grained control over system operations
- **Error sanitization**: System errors are logged privately, not shown to users
- **Secure file permissions**: Automatic permission restrictions on sensitive files
- **High availability configuration**: Elevated priorities to remain responsive under load

## Known Limitations

Please be aware of the following security considerations:

1. **Service control**: Users with bot access can restart any service on your system (unless restricted via CONTROLLED_SERVICES)
2. **Server reboot**: The bot can reboot your server if the `/restart_server` command is used
3. **Docker control**: Users can control Docker containers (start, stop, restart, etc.)
4. **Telegram API**: Security depends on Telegram's API security; ensure your account is protected with 2FA

## Security Updates

Security updates are released as patch versions (e.g., v1.2.1, v1.2.2) and include:

- Fixes for security vulnerabilities
- Critical bug fixes that could impact security
- Dependency updates addressing security issues

We recommend enabling automatic updates or regularly checking for new releases.

## Credits

We would like to thank the following security researchers for responsibly disclosing vulnerabilities (list will be populated as reports are received).
