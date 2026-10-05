# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.2.2] - 2026-10-05

### Added
- Added CODE_OF_CONDUCT.md for community guidelines
- Added SECURITY.md for vulnerability reporting and best practices
- Added CHANGELOG.md for tracking project changes
- Added issue and pull request templates
- Added Dockerfile for containerized deployment

### Changed
- CI now tags and publishes a release automatically on every push to `main`, bumping the minor version (`scripts/next_version.sh`)
- Update checks (`/get_update`) now use releases from `edwinludik-ai/control_my_server_bot`
- Updated .gitignore to exclude IDE configurations and database files
- Enhanced README with improved documentation

### Fixed
- `/start` and `/help` output showed literal backslashes and asterisks; the help text is now sent as HTML

## [1.2.1] - 2026-08-09

### Fixed
- Build errors in the project

## [1.2.0] - 2026-04-05

### Added
- Docker support for containerized deployment
- Version 1.2.0 release

## [1.1.0] - 2026-03-XX

### Added
- Previous version features

## [1.0.0] - 2026-02-XX

### Added
- Initial release of Control My Server Telegram Bot
- Core features: server status, CPU/RAM/disk monitoring
- Service management (start, stop, restart)
- Docker container control
- Multi-user support
- Systemd integration
- Package building (.deb, .rpm, .apk, Arch)
- CI/CD pipeline with GitHub Actions

---

## Types of Changes

- **Added**: New features
- **Changed**: Changes in existing functionality
- **Deprecated**: Soon-to-be removed features
- **Removed**: Removed features
- **Fixed**: Bug fixes
- **Security**: Vulnerability fixes

## Contributing

When contributing to this project, please update the changelog as part of your pull request. Add your changes under the `[Unreleased]` section following the existing format.

Releases are cut automatically by CI on every push to `main`:
- If the `VERSION` file holds a version that has no tag yet, that version is released as-is. Use this to choose a version by hand (e.g., a major bump).
- Otherwise the minor version is incremented and the patch reset to 0 (e.g., `1.2.2` → `1.3.0`).
- CI commits the new `VERSION` back to `main`, creates the matching `v*` tag, builds the packages, and publishes the GitHub release.

Before merging to `main`, move the changes from `[Unreleased]` into a new section for the version about to be released, using the date of the release.
