# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Added CODE_OF_CONDUCT.md for community guidelines
- Added SECURITY.md for vulnerability reporting and best practices
- Added CHANGELOG.md for tracking project changes
- Added issue and pull request templates
- Added Dockerfile for containerized deployment

### Changed
- Updated .gitignore to exclude IDE configurations and database files
- Enhanced README with improved documentation

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

When releasing a new version:
1. Create a new section for the version at the top
2. Update the date
3. Move all changes from `[Unreleased]` to the new version section
4. Create a new `[Unreleased]` section
5. Update the VERSION file
6. Create a Git tag matching the version (e.g., `v1.2.1`)
