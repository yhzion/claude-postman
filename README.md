# claude-postman

[![Release](https://img.shields.io/github/v/release/yhzion/claude-postman?style=flat-square)](https://github.com/yhzion/claude-postman/releases/latest)
[![CI](https://img.shields.io/github/actions/workflow/status/yhzion/claude-postman/release.yml?style=flat-square&label=build)](https://github.com/yhzion/claude-postman/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/yhzion/claude-postman?style=flat-square)](https://goreportcard.com/report/github.com/yhzion/claude-postman)
[![Go](https://img.shields.io/badge/go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![Downloads](https://img.shields.io/github/downloads/yhzion/claude-postman/total?style=flat-square&color=brightgreen)](https://github.com/yhzion/claude-postman/releases)
[![License](https://img.shields.io/github/license/yhzion/claude-postman?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey?style=flat-square)](https://github.com/yhzion/claude-postman)

Email relay server between you and [Claude Code](https://docs.anthropic.com/en/docs/claude-code).

```
You (Email) ←→ claude-postman (Relay Server) ←→ Claude Code (tmux session)
```

Send an email, Claude Code works on it, and you get the result back — all via email.

## Table of Contents

- [About](#about)
- [Installing and Updating](#installing-and-updating)
  - [Install Script](#install-script)
  - [Manual Install](#manual-install)
  - [Updating](#updating)
  - [Verify Installation](#verify-installation)
- [Quick Start](#quick-start)
- [Commands](#commands)
- [Configuration](#configuration)
  - [Config File](#config-file)
  - [Environment Variables](#environment-variables)
- [Troubleshooting](#troubleshooting)
- [Requirements](#requirements)
- [Documentation](#documentation)
- [License](#license)

## About

claude-postman is a background server that bridges email and Claude Code via tmux sessions.

- **Email-based control**: Operate Claude Code remotely through email
- **tmux session management**: Each task runs in an isolated tmux session
- **Offline queue**: Messages are queued and sent when the network is back
- **Rich HTML reports**: Markdown → HTML email with syntax-highlighted code blocks
- **Self-update**: Built-in `update` command to stay current
- **System service**: Register as systemd (Linux) or launchd (macOS) service

## Installing and Updating

### Install Script

```bash
curl -fsSL https://raw.githubusercontent.com/yhzion/claude-postman/main/install.sh | bash
```

The script automatically detects your OS and architecture, downloads the latest release binary, and installs it to `/usr/local/bin`.

### Manual Install

Build from source (requires Go 1.24+ and CGO):

```bash
git clone https://github.com/yhzion/claude-postman.git
cd claude-postman
go build -o claude-postman ./cmd/claude-postman
sudo mv claude-postman /usr/local/bin/
```

### Updating

```bash
claude-postman update
```

This checks the latest GitHub release and replaces the current binary. When you run any command, claude-postman also prints a notification if a newer version is available.

### Verify Installation

```bash
claude-postman --version
```

## Quick Start

```bash
# 1. Setup configuration (interactive wizard)
claude-postman init

# 2. Check environment
claude-postman doctor

# 3. Start the relay server
claude-postman serve
```

## Commands

```
claude-postman                     # Show help
claude-postman --version           # Show version

claude-postman init                # Setup configuration wizard
claude-postman serve               # Start the relay server (foreground)
claude-postman doctor              # Check environment and diagnose issues
claude-postman doctor --fix        # Diagnose + auto-fix where possible

claude-postman install-service     # Register as system service
claude-postman uninstall-service   # Remove system service
claude-postman update              # Update to the latest version
claude-postman uninstall           # Remove claude-postman completely
claude-postman uninstall --yes     # Remove without confirmation
```

### `doctor` checks

| Check | Description | `--fix` |
|-------|-------------|---------|
| Config | `config.toml` exists and is valid | — (run `init`) |
| Data directory | Data dir exists | Creates it |
| Database | SQLite file + migration status | Initializes DB |
| tmux | `tmux -V` available | — (install manually) |
| Claude Code | `claude --version` available | — (install manually) |
| SMTP | TCP connection to SMTP server | — (check settings) |
| IMAP | TCP connection to IMAP server | — (check settings) |
| Service | systemd/launchd registration | — (run `install-service`) |

Example output:

```
$ claude-postman doctor

Checking environment...

  ✅ Config: ~/.claude-postman/config.toml
  ✅ Data directory: ~/.claude-postman/data
  ✅ Database: OK (version 1)
  ❌ tmux: not found
  ✅ Claude Code: v2.1.47
  ✅ SMTP: smtp.gmail.com:587 (connected)
  ✅ IMAP: imap.gmail.com:993 (connected)
  ⚠️  Service: not registered

  ❌ tmux: Install with 'sudo apt install tmux' or 'brew install tmux'
  ⚠️  Service: Run 'sudo claude-postman install-service' to register
```

## Configuration

### Config File

Created by `claude-postman init` at `~/.claude-postman/config.toml`:

```toml
[general]
data_dir = "~/.claude-postman/data"
model = "sonnet"
poll_interval_sec = 30
session_timeout_min = 30

[email]
user = "you@gmail.com"
app_password = "xxxx-xxxx-xxxx-xxxx"
smtp_host = "smtp.gmail.com"
smtp_port = 587
imap_host = "imap.gmail.com"
imap_port = 993
```

### Environment Variables

Every config value can be overridden with `CLAUDE_POSTMAN_` prefixed environment variables:

```bash
CLAUDE_POSTMAN_DATA_DIR=/path/to/data
CLAUDE_POSTMAN_MODEL=sonnet
CLAUDE_POSTMAN_POLL_INTERVAL=30
CLAUDE_POSTMAN_SESSION_TIMEOUT=30
CLAUDE_POSTMAN_EMAIL_USER=you@gmail.com
CLAUDE_POSTMAN_EMAIL_PASSWORD=app-password
CLAUDE_POSTMAN_SMTP_HOST=smtp.gmail.com
CLAUDE_POSTMAN_SMTP_PORT=587
CLAUDE_POSTMAN_IMAP_HOST=imap.gmail.com
CLAUDE_POSTMAN_IMAP_PORT=993
```

## Troubleshooting

Run the built-in diagnostic tool:

```bash
claude-postman doctor
```

For automatic fixes (creates missing directories, initializes database):

```bash
claude-postman doctor --fix
```

| Issue | Solution |
|-------|----------|
| `Config: not found` | Run `claude-postman init` |
| `tmux: not found` | `sudo apt install tmux` or `brew install tmux` |
| `Claude Code: not found` | Install from [anthropic.com](https://docs.anthropic.com/en/docs/claude-code) |
| `SMTP: connection failed` | Check email credentials and firewall |
| `IMAP: connection failed` | Check IMAP settings, enable "Less secure apps" or use App Password |

## Requirements

- **OS**: macOS or Linux
- **tmux**: Session management
- **Claude Code**: AI coding assistant
- **Email account**: Gmail recommended (SMTP/IMAP with App Password)

## Documentation

### Architecture (SSOT)

- [01. tmux Output Capture](docs/architecture/01-tmux-output-capture.md)
- [02. Config](docs/architecture/02-config.md)
- [03. Storage (SQLite)](docs/architecture/03-storage.md)
- [04. Session Management](docs/architecture/04-session.md)
- [05. Email (SMTP/IMAP)](docs/architecture/05-email.md)
- [06. CLI, Service, Doctor](docs/architecture/06-service.md)

### References

- [Ideas & Planning](docs/ideas.md)
- [Use Cases](docs/usecases/SUMMARY.md)
- [Tech Stack Decisions](docs/tech-stack/)

## License

[MIT](LICENSE)
