# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.4.5] - 2026-02-22

### Improved
- `update`: show restart hint when no service is installed
  - "If claude-postman serve is running, restart it to apply the update."

## [v0.4.4] - 2026-02-22

### Added
- `update`: detect installed service and guide restart after update
  - Linux: prints `sudo systemctl restart claude-postman` (requires root)
  - macOS: automatically restarts via launchctl

## [v0.4.3] - 2026-02-22

### Added
- `send-template` command: manually send a session creation template email
- `serve`: send template email on every startup as SMTP verification gate
  - Serve refuses to start if template email fails (ensures SMTP is working)

## [v0.4.2] - 2026-02-22

### Added
- `init`: send session template email after setup completes
  - Template email is sent to the user's own address via SMTP
  - Template Message-ID stored in DB for security verification
  - Users forward this email to create new Claude Code sessions (template reference + From verification)

## [v0.4.1] - 2026-02-22

### Fixed
- `install-service` / `uninstall-service`: show actionable `sudo` hint with absolute binary path instead of raw "permission denied" on Linux
- `install-service`: resolve original user via `SUDO_USER` when running under sudo, so the systemd unit runs as the correct user with the correct HOME

## [v0.4.0] - 2026-02-22

### Changed
- Init wizard migrated from plain text prompts to [charmbracelet/huh](https://github.com/charmbracelet/huh) TUI forms
  - Interactive mode: styled form widgets with keyboard navigation
  - Accessible mode: plain text fallback for scripted/piped input
  - Password fields masked in interactive mode (EchoModePassword)
- Removed internal prompt helpers (`readLine`, `prompt`, `promptSecret`, `promptChoice`, `promptInt`) — replaced by huh form fields

## [v0.3.1] - 2026-02-22

### Changed
- Install script now uses `~/.local/bin/` instead of `/usr/local/bin/`
  - No more sudo required for install or subsequent updates
  - PATH guidance shown if `~/.local/bin` is not in PATH

### Added
- CHANGELOG.md

## [v0.3.0] - 2026-02-22

### Added
- Interactive setup wizard (`claude-postman init`)
  - 3-step flow: Data Directory, Email Account, Default Model
  - Gmail/Outlook provider presets with auto-fill host/port
  - Provider-specific app password help text
  - Re-run support: existing values shown as defaults with compact "Change? (y/N)" flow
  - SMTP/IMAP connection test after setup
  - Config file saved with 0600 permissions, directories with 0700

## [v0.2.0] - 2026-02-22

### Added
- `claude-postman uninstall` command
  - Stops and removes system service (systemd/launchd)
  - Removes config and data directory (`~/.claude-postman/`)
  - Removes binary itself
  - `--yes` flag to skip confirmation prompt

### Fixed
- Darwin (macOS) release build: CGO cross-compilation for arm64/amd64

## [v0.1.0] - 2026-02-22

### Added

#### Core Modules
- **Storage** (SQLite via `mattn/go-sqlite3`)
  - Sessions CRUD, outbox queue, inbox processing, template management
  - Transaction support via `Store.Tx()`
  - Embedded SQL migration system
- **Config** (`BurntSushi/toml`)
  - TOML config file loading (`~/.claude-postman/config.toml`)
  - Environment variable overrides for all settings
  - Validation of required fields
  - Email provider presets (Gmail, Outlook)
- **Session** (tmux)
  - tmux session creation/deletion for Claude Code
  - FIFO-based completion signal handling
  - Claude Code resume capability
  - Session recovery on server restart
- **Email** (SMTP/IMAP via `emersion/go-imap` v2)
  - IMAP polling with sender verification
  - Session matching (Session-ID, In-Reply-To, References)
  - Template email sending for new session creation
  - Outbox with exponential backoff retry (30s ~ 8m, max 5 retries)
  - Markdown to HTML rendering with code highlighting (`goldmark` + `chroma`)
- **Serve** (main loop)
  - IMAP polling goroutine + outbox flushing goroutine
  - Per-session FIFO reader goroutines
  - Graceful shutdown with signal handling

#### CLI Commands
- `claude-postman serve` - Start the relay server
- `claude-postman doctor` - Environment diagnostics with `--fix` auto-repair
- `claude-postman install-service` - Register as systemd (Linux) / launchd (macOS) service
- `claude-postman uninstall-service` - Remove system service
- `claude-postman update` - Self-update to latest release

#### Infrastructure
- GitHub Actions release pipeline
  - Linux builds: amd64, arm64
  - macOS builds: amd64, arm64
  - Auto-generated release notes
- Pre-commit hooks: go-fmt, go-imports, go-vet
- Pre-push hooks: golangci-lint, go-build, go-test
- 79+ unit tests across all modules

[Unreleased]: https://github.com/yhzion/claude-postman/compare/v0.4.5...HEAD
[v0.4.5]: https://github.com/yhzion/claude-postman/compare/v0.4.4...v0.4.5
[v0.4.4]: https://github.com/yhzion/claude-postman/compare/v0.4.3...v0.4.4
[v0.4.3]: https://github.com/yhzion/claude-postman/compare/v0.4.2...v0.4.3
[v0.4.2]: https://github.com/yhzion/claude-postman/compare/v0.4.1...v0.4.2
[v0.4.1]: https://github.com/yhzion/claude-postman/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/yhzion/claude-postman/compare/v0.3.1...v0.4.0
[v0.3.1]: https://github.com/yhzion/claude-postman/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/yhzion/claude-postman/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/yhzion/claude-postman/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/yhzion/claude-postman/releases/tag/v0.1.0
