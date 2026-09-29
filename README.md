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

- A beads (`bd`) project containing a `.beads/` directory, with the `bd` CLI on your `PATH` (perles uses it to create and update issues)
- Minimum beads database version v0.62.0 (run `bd migrate` to upgrade)
- If beads uses the Dolt backend, it must run in server mode. Embedded Dolt, the beads v0.63+ default, takes an exclusive lock and is not supported. Initialize with `bd init --server` (or set `"dolt_mode": "server"` in `.beads/metadata.json`, or `BEADS_DOLT_SERVER_MODE=1`) and start the server with `bd dolt start`.
  - Connection defaults are `127.0.0.1:3307`, user `root`, no password, database `beads`. Change them with the `dolt_server_host`, `dolt_server_port`, `dolt_server_user` and `dolt_database` fields in `.beads/metadata.json`. The `BEADS_DOLT_SERVER_HOST`, `BEADS_DOLT_SERVER_PORT` and `BEADS_DOLT_SERVER_USER` environment variables override those fields, and `BEADS_DOLT_PASSWORD` sets the password. If no port is set, perles uses the one `bd` writes to `.beads/dolt-server.port`.
- Or a beads_rust (`br`) project: set `backend: beads_rust` in your perles config (`.perles/config.yaml` or `~/.config/perles/config.yaml`) and have `br` on your `PATH`. Perles does not detect beads_rust on its own, and the v0.62.0 minimum applies only to beads.

## License

MIT
