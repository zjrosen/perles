# Getting Started

## Requirements

- A [beads](https://github.com/steveyegge/beads) project (a `.beads/` directory). beads_rust projects also work if you set `backend: beads_rust` in your [config](configuration/index.md); perles doesn't detect beads_rust on its own.
- Beads database version v0.62.0 or newer (upgrade beads and run `bd migrate`). This minimum doesn't apply to beads_rust.
- The `bd` CLI on your `PATH`, which perles uses to create and update issues (`br` when using `backend: beads_rust`)
- If beads stores its data in Dolt, it must use server mode and the Dolt server must be running. Perles can't open embedded Dolt mode (the beads v0.63+ default) because it locks the database. Set up server mode with `bd init --server` (or `"dolt_mode": "server"` in `.beads/metadata.json`) and start the server with `bd dolt start`.
- Go 1.27+ (only for `go install` or building from source; `make install` also needs Node.js/npm)

If there's no `.beads/` directory, the database is too old, or Dolt is in embedded mode or its server isn't running, perles shows a screen explaining the problem instead of the board.

---

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

The script installs to `~/.local/bin` by default and warns if that directory isn't on your `PATH`. Set `INSTALL_DIR` to install somewhere else (the script falls back to `sudo` if it can't write there), or `VERSION` to install a specific release:

```bash
curl -sSL https://raw.githubusercontent.com/zjrosen/perles/main/install.sh | INSTALL_DIR=/usr/local/bin VERSION=vX.Y.Z bash
```

To upgrade later, run `perles update`. It re-runs the install script, so set `INSTALL_DIR` again if you used a custom location (for example `INSTALL_DIR=/usr/local/bin perles update`). Homebrew users should run `brew upgrade perles`.

### Go Install

Requires Go 1.27+:

```bash
go install github.com/zjrosen/perles@latest
```

### Build from Source

```bash
git clone https://github.com/zjrosen/perles.git
cd perles
make install
```

Requires Go 1.27+ and Node.js/npm (version in `.nvmrc`), because `make install` rebuilds the embedded web session viewer first. The built viewer is committed in `frontend/dist`, so without Node you can run `go install -trimpath .` instead (or `make build-go` to build `./perles` in the repo).

### Binary Downloads

Pre-built binaries for Linux and macOS (Intel and Apple Silicon) are available on the [Releases](https://github.com/zjrosen/perles/releases) page.

1. Download the archive for your platform
2. Extract: `tar -xzf perles_*.tar.gz`
3. Move to PATH: `sudo mv perles /usr/local/bin/`
4. Verify: `perles --version`

---

## Quick Start

Run `perles` in a directory containing a `.beads/` folder (see [Requirements](#requirements)):

```bash
cd your-project
perles
```

You'll see the default kanban board with five columns: **Blocked**, **Deferred**, **Ready**, **In Progress**, and **Closed**.

If no config file exists yet (no `--config`, no `./.perles/config.yaml` and no `~/.config/perles/config.yaml`), perles writes the default config to `.perles/config.yaml` in the current directory the first time the board opens, and saves column and view changes there. To use one config for all projects instead, create `~/.config/perles/config.yaml` before your first run. See [Configuration](configuration/index.md).

---

## CLI Reference

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--beads-dir` | `-b` | Path to beads database directory |
| `--config` | `-c` | Path to config file |
| `--port` | `-p` | Port for the control plane API and web session viewer, which start when you first open the Dashboard (0 = auto-assign; overrides `orchestration.api_port`) |
| `--markdown-style` | | Markdown rendering style: `dark` (default) or `light` (overrides `ui.markdown_style`) |
| `--version` | `-v` | Print version |
| `--help` | `-h` | Print help |
| `--debug` | `-d` | Enable developer/debug mode |

Every subcommand accepts `--config` and `--debug`, though only `perles` and `perles daemon` act on `--debug` (for example `perles daemon -d`). `--beads-dir`, `--port`, `--markdown-style` and `--version` apply only to `perles` itself; some subcommands have their own flags, such as `perles daemon --port` and `perles update --version`.

The beads directory is resolved from `-b`, then the `BEADS_DIR` environment variable, then `beads_dir` in your config, then the current directory. `perles daemon` has no `-b` flag, so use `BEADS_DIR` or `beads_dir` with it.

### Commands

| Command | Description |
|---------|-------------|
| `perles` | Launch the TUI application |
| `perles init` | Create `.perles/config.yaml` in the current directory with default settings (fails if the file already exists) |
| `perles themes` | List available theme presets |
| `perles workflows` | List the workflows offered by the Dashboard's New Workflow dialog and the chat panel's Workflows tab |
| `perles registry:list` | Print workflow registrations as JSON; filter with `-n`/`--namespace` and `-l`/`--label` (repeatable, all labels must match) |
| `perles daemon` | Run the control plane HTTP API and web session viewer without the TUI (`-p`/`--port` overrides `orchestration.api_port`; 0 = auto-assign) |
| `perles update` | Update to the latest release via the install script (`--version vX.Y.Z` installs a specific release) |
| `perles playground` | Interactive playground for UI components and theme tokens (vimtextarea, modals, pickers, tables, etc.) |
| `perles completion <shell>` | Generate a shell completion script (`bash`, `zsh`, `fish`, `powershell`) |

---

## Common Keybindings

These keybindings work in Kanban and Search modes:

| Key | Action |
|-----|--------|
| `ctrl+space` | Switch between Kanban and Search modes (configurable via `ui.keybindings.search`) |
| `ctrl+o` | Open the [Dashboard](orchestration/index.md) (multi-workflow orchestration) from Kanban (configurable via `ui.keybindings.dashboard`) |
| `ctrl+w` | Toggle the AI chat panel; `tab` switches focus between the chat panel and the board while it is open |
| `ctrl+g` | Open the git diff viewer |
| `?` | Toggle help overlay (also in the Dashboard) |
| `q` | Quit (in Search, only when not typing in the search input); returns to Kanban from the Dashboard |
| `ctrl+c` | Quit with confirmation; returns to Kanban from the Dashboard |

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
