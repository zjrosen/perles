# Perles

A terminal UI for [beads](https://github.com/steveyegge/beads) issue tracking powered by a custom **BQL (Beads Query Language)**. Search issues with boolean logic, filter by dates, traverse dependency trees, and build custom kanban views without leaving your terminal.

<p align="center">
  <img src="./docs/assets/board.png" width="1440" alt="kanban board">
</p>
<p align="center">
  <img src="./docs/assets/search.png" width="1440" alt="bql search">
</p>

## Documentation

Full documentation is available at **[zjrosen.github.io/perles](https://zjrosen.github.io/perles/)**.

## Quick Start

### Install Script

```bash
curl -sSL https://raw.githubusercontent.com/zjrosen/perles/main/install.sh | bash
```

### Homebrew (macOS/Linux)

```bash
brew tap zjrosen/perles
brew install perles
```

## Usage

Run `perles` in any directory containing a `.beads/` folder:

```bash
cd your-project
perles
```

## Requirements

- A beads (`bd`) project containing a `.beads/` directory, with the `bd` CLI on your `PATH` (perles uses it to create and update issues, and to read embedded Dolt workspaces)
- beads v1.0.0 or newer. To upgrade an older database, upgrade `bd`, then run `bd migrate schema` and `bd migrate` (see the [beads upgrade guide](https://github.com/steveyegge/beads/blob/main/docs/getting-started/upgrading.md)). For multi-agent orchestration, bd v1.3.2 or newer is recommended.
- Beads in one of these Dolt modes:
  - Embedded Dolt, the `bd init` default, needs bd v1.0.5 or newer. Perles reads it through `bd`, so the board catches up about 1–2 seconds after a write. See [Embedded Dolt workspaces](https://zjrosen.github.io/perles/configuration/#embedded-dolt-workspaces) for how it works and its limits.
  - Dolt server mode (`bd init --server`), with the server running (`bd dolt start` starts a bd-managed one), on Dolt 2.6.0 or newer. Server mode is recommended for very large (over about 5,000 issues) or write-heavy multi-agent workspaces. Perles finds the server the same way `bd` does; see [Configuration](https://zjrosen.github.io/perles/configuration/#environment-variables) for the settings it reads. To move an existing embedded project to server mode (optional), back it up with `bd backup init <dir>` and `bd backup sync`, then in a new directory run `bd init --server` and `bd backup restore --force <dir>`.
- Proxied-server and HTTP (`bd connect`) workspaces aren't supported yet. `bd migrate from-proxied-server-to-server` switches a proxied-server workspace to server mode.
- Or a beads_rust (`br`) project: set `backend: beads_rust` in your perles config (`.perles/config.yaml` or `~/.config/perles/config.yaml`) and have `br` on your `PATH`. Perles does not detect beads_rust on its own, and the v1.0.0 minimum applies only to beads.

## License

MIT
