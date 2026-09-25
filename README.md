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

- A beads or beads-rust enabled project containing a` .beads/` directory
- Minimum beads database version v0.62.0 (run `bd migrate` to upgrade)

## License

MIT
