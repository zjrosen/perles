# Getting Started

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap zjrosen/perles
brew install perles
```

### Install Script

```bash
curl -sSL https://raw.githubusercontent.com/zjrosen/perles/main/install.sh | bash
```

### Go Install

Requires Go 1.21+:

```bash
go install github.com/zjrosen/perles@latest
```

### Build from Source

```bash
git clone https://github.com/zjrosen/perles.git
cd perles
make install
```

### Binary Downloads

Pre-built binaries for Linux and macOS (Intel and Apple Silicon) are available on the [Releases](https://github.com/zjrosen/perles/releases) page.

1. Download the archive for your platform
2. Extract: `tar -xzf perles_*.tar.gz`
3. Move to PATH: `sudo mv perles /usr/local/bin/`
4. Verify: `perles --version`

---

## Quick Start

Run `perles` in any directory containing a `.beads/` folder:

```bash
cd your-project
perles
```

You'll see the default kanban board with five columns: **Blocked**, **Deferred**, **Ready**, **In Progress**, and **Closed**.

---

## CLI Reference

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--beads-dir` | `-b` | Path to beads database directory |
| `--config` | `-c` | Path to config file |
| `--version` | `-v` | Print version |
| `--help` | `-h` | Print help |
| `--debug` | `-d` | Enable developer/debug mode |

### Commands

| Command | Description |
|---------|-------------|
| `perles` | Launch the TUI application |
| `perles themes` | List available theme presets |
| `perles workflows` | List available workflow templates |
| `perles playground` | Run the vimtextarea playground |

---

## Global Keybindings

These keybindings work across all modes:

| Key | Action |
|-----|--------|
| `ctrl+space` | Switch between Kanban and Search modes |
| `ctrl+o` | Enter Orchestration / Dashboard mode |
| `?` | Toggle help overlay |
| `ctrl+c` | Quit |

---

## Debug Mode

Debug mode provides logging and debugging tools for troubleshooting.

```bash
# Via flag
perles --debug

# Via environment variable
PERLES_DEBUG=1 perles

# With custom log path
PERLES_LOG=/tmp/perles.log perles --debug
```

- **Log file**: Output written to `debug.log` (or custom path via `PERLES_LOG`)
- **Log overlay**: Press `ctrl+x` to view logs in-app
- **Lifecycle logging**: Startup and shutdown events are logged

When reporting bugs, include the `debug.log` file:

1. Run perles with `--debug`
2. Reproduce the issue
3. Attach `debug.log` to your bug report
