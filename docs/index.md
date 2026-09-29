---
hide:
  - navigation
  - toc
---

# Perles

A terminal UI for [beads](https://github.com/steveyegge/beads) issue tracking powered by a custom **BQL (Beads Query Language)**. Search issues with boolean logic, filter by dates, traverse dependency trees, and build custom kanban views without leaving your terminal.

![Search Mode](assets/search.png)

![Kanban Board](assets/board.png)

![Issue Children](assets/issues-children.png)

---

## Key Features

<div class="grid cards" markdown>

- **Kanban Board**

    ---

    Organize issues into customizable columns powered by BQL queries or dependency trees. Create unlimited views with mixed column types.

    [Kanban Mode](usage/kanban.md)

- **BQL Search**

    ---

    Full-screen search interface powered by BQL (Beads Query Language) with boolean logic, date filters, and relationship traversal.

    [Search Mode](usage/search.md)

- **Dependency Explorer**

    ---

    Visualize and navigate issue relationships -- blockers, dependencies, and parent/child hierarchies as interactive trees.

    [Dependency Explorer](usage/dependency-explorer.md)

- **BQL Query Language**

    ---

    Purpose-built query language for filtering issues with operators, boolean logic, date ranges, sorting, and relationship expansion.

    [BQL Reference](bql/index.md)

- **AI Orchestration**

    ---

    Multi-agent control plane that coordinates headless AI agents for structured development workflows with parallel execution.

    [Orchestration](orchestration/index.md)

- **Theming**

    ---

    Comprehensive theme system with built-in presets (Catppuccin, Dracula, Nord) and fully customizable color tokens.

    [Theming](configuration/theming.md)

</div>

---

## Quick Start

### Install

```bash
curl -sSL https://raw.githubusercontent.com/zjrosen/perles/main/install.sh | bash
```

Or via Homebrew (macOS/Linux):

```bash
brew tap zjrosen/perles
brew install perles
```

### Run

```bash
cd your-project
perles
```

---

## Requirements

- A beads (`bd`) project with a `.beads/` directory, using SQLite or Dolt in server mode (`bd init --server`, or `"dolt_mode": "server"` in `.beads/metadata.json`) with the Dolt server running (`bd dolt start`). Embedded Dolt mode, the beads v0.63+ default, is not supported.
- Minimum beads database version v0.62.0 (run `bd migrate` to upgrade)
- Or a beads_rust (`br`) project: set `backend: beads_rust` in your perles [config](configuration/index.md) and have `br` on your `PATH`
- Go 1.27+ and Node.js/npm (needed by `make build`/`make install` for the frontend) if building from source. `go install` and `make build-go` need only Go.

See [Getting Started](getting-started.md#requirements) for the full list.
