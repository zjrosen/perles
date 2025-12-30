# Perles

Perles is a terminal UI for [beads](https://github.com/steveyegge/beads) issue tracking, powered by a custom **BQL (Beads Query Language)**. Search with boolean logic, filter by dates, traverse dependency trees, and build custom kanban views — without leaving your terminal. BQL also drives custom configured kanban boards, each column is defined by a query, so you can slice your issues however you want.

<p align="center">
  <img src="./assets/search.png" width="1440" alt="search">
</p>

<p align="center">
  <img src="./assets/board.png" width="1440" alt="board">
</p>

<p align="center">
  <img src="./assets/issues-dependencies.png" width="1440" alt="board">
</p>

## Requirements

- A beads-enabled project (`.beads/` directory with `beads.db`)
- Minimum beads database version v0.41.0. run `bd migrate` to upgrade after updating beads

## Installation

### Install Script

```bash
curl -sSL https://raw.githubusercontent.com/zjrosen/perles/main/install.sh | bash
```

### Homebrew (macOS/Linux)

```bash
brew tap zjrosen/perles
brew install perles
```

### Go Install

Requires Go 1.21+

```bash
go install github.com/zjrosen/perles@latest
```

### Build from Source

```bash
git clone https://github.com/zjrosen/perles.git
cd perles
make install
perles
```

### Binary Downloads

Pre-built binaries for Linux and macOS (both Intel and Apple Silicon) are available on the [Releases](https://github.com/zjrosen/perles/releases) page.

1. Download the archive for your platform
2. Extract: `tar -xzf perles_*.tar.gz`
3. Move to PATH: `sudo mv perles /usr/local/bin/`
4. Verify: `perles --version`

## Usage

Run `perles` in any directory containing a `.beads/` folder:

```bash
cd your-project
perles
```

### CLI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--beads-dir` | `-b` | Path to beads database directory |
| `--config` | `-c` | Path to config file |
| `--no-auto-refresh` | | Disable automatic board refresh |
| `--version` | `-v` | Print version |
| `--help` | `-h` | Print help |
| `--debug` | `-d` | Enable developer/debug mode |

### CLI Commands

| Command | Description |
|---------|-------------|
| `perles` | Launch the TUI application |
| `perles themes` | List available theme presets |
| `perles workflows` | List available workflow templates |

### Global Keybindings

| Key | Action |
|-----|--------|
| `Ctrl+Space` | Switch between Kanban and Search modes |
| `?` | Toggle help overlay |
| `q` / `Ctrl+C` | Quit |

---

## Kanban Mode

Organize issues into customizable columns powered by BQL queries or dependency trees.

### BQL Columns

<p align="center">
  <img src="./assets/board.png" width="1440" alt="board">
</p>

### Mixed Column Types (BQL + Dependency Trees)

<p align="center">
  <img src="./assets/multiple-column-types.png" width="1440" alt="board">
</p>

### Features

- Four-column default layout: Blocked, Ready, In Progress, Closed
- Fully customizable columns with BQL queries or dependency trees
- Multi-view support — create unlimited board views
- Real-time auto-refresh when database changes
- Column management: add, edit, reorder, delete

### Videos

#### Navigating Views and Columns

Use `h` and `l` to move left and right between columns, `ctrl+h` / `ctrl+l` to move column positions and `ctrl+n` / `ctrl+p` to switch between views. Use `ctrl+v` to open the view menu to create, rename or delete a view.

https://github.com/user-attachments/assets/174dc673-66fa-46be-9ca5-fbd5ac0034dd

#### Adding a New Column

Use `a` from kanban mode to add a new column.

https://github.com/user-attachments/assets/8ce16144-15dd-4509-8cd9-aa8e07477b5d

### Keybindings

#### Navigation

| Key | Action |
|-----|--------|
| `h` / `←` | Move to left column |
| `l` / `→` | Move to right column |
| `j` / `↓` | Move down in column |
| `k` / `↑` | Move up in column |
| `Enter` | View issue details |

#### Views

| Key | Action |
|-----|--------|
| `Ctrl+J` / `Ctrl+N` | Next view |
| `Ctrl+K` / `Ctrl+P` | Previous view |
| `Ctrl+V` | View menu (Create/Delete/Rename) |
| `Ctrl+D` | Delete current column |

#### Columns

| Key | Action |
|-----|--------|
| `a` | Add new column |
| `e` | Edit current column |
| `Ctrl+H` | Move column left |
| `Ctrl+L` | Move column right |
| `/` | Open search with column's BQL query |

#### Issues

| Key | Action |
|-----|--------|
| `r` | Refresh issues |
| `y` | Copy issue ID to clipboard |
| `w` | Toggle status bar |

### Details View

| Key | Action |
|-----|--------|
| `e` | Open edit menu (labels, priority, status) |
| `d` | Delete issue |
| `j` / `k` | Scroll content |
| `Esc` | Back to kanban board |

### Default Columns

The default view includes these columns (all configurable via BQL):

| Column | BQL Query |
|--------|-----------|
| **Blocked** | `status = open and blocked = true` |
| **Ready** | `status = open and ready = true` |
| **In Progress** | `status = in_progress` |
| **Closed** | `status = closed` |

---

## Search Mode

Full-screen BQL-powered search interface with live results and issue details.

<p align="center">
  <img src="./assets/search.png" width="1440" alt="search">
</p>

### Features

- Full-screen BQL-powered search interface
- Save searches as kanban columns
- Create new views from search results
- Sub-mode for viewing issue dependencies and hierarchies

### Videos

#### BQL Search

Use `ctrl+space` to switch modes between Kanban and Search or while on a column use `/` to be dropped into search mode with the current columns BQL query.

https://github.com/user-attachments/assets/d0d61c71-a037-4f7b-9718-15156d6bf278

#### Creating a View from Search Results

Use `ctrl+s` from search mode to save the BQL query to a new or existing view.

https://github.com/user-attachments/assets/21085552-a62f-441e-bba7-0960c00f5029

### Keybindings

| Key | Action |
|-----|--------|
| `/` | Focus search input |
| `Enter` | Execute query / Edit field |
| `h` | Move to results list |
| `l` | Move to details panel |
| `j` / `k` | Navigate results |
| `y` | Copy issue ID |
| `s` | Change status |
| `p` | Change priority |
| `Ctrl+S` | Save search as column |
| `Esc` | Exit to kanban mode |

---

## Dependency Explorer

Visualize and navigate issue relationships — blockers, dependencies, and parent/child hierarchies.

### Dependency Chain

<p align="center">
  <img src="./assets/issues-dependencies.png" width="1440" alt="board">
</p>

### Parent/Child Hierarchy

<p align="center">
  <img src="./assets/issues-children.png" width="1440" alt="parent child hierarchy">
</p>

### Keybindings (Tree View)

| Key | Action |
|-----|--------|
| `j` / `k` | Move cursor up/down |
| `l` / `Tab` | Focus details panel |
| `h` | Focus tree panel |
| `Enter` | Refocus tree on selected node |
| `u` | Go back to previous root |
| `U` | Go to original root |
| `d` | Toggle direction (up/down) |
| `m` | Toggle mode (deps/children) |
| `y` | Copy issue ID |
| `/` | Switch to list mode |
| `Esc` | Exit to kanban mode |

---

## BQL Query Language

Perles uses BQL (Beads Query Language) to filter and organize issues. BQL is used in column definitions and search mode.

### Basic Syntax

```
field operator value [and|or field operator value ...]
```

### Available Fields

| Field | Description | Example Values |
|-------|-------------|----------------|
| `status` | Issue status | open, in_progress, closed |
| `type` | Issue type | bug, feature, task, epic, chore |
| `priority` | Priority level | P0, P1, P2, P3, P4 |
| `blocked` | Has blockers | true, false |
| `ready` | Ready to work | true, false |
| `pinned` | Is pinned | true, false |
| `is_template` | Is a template | true, false |
| `label` | Issue labels | any label string |
| `title` | Issue title | any text (use ~ for contains) |
| `description` | Issue description | any text (use ~ for contains) |
| `design` | Design notes | any text (use ~ for contains) |
| `notes` | Issue notes | any text (use ~ for contains) |
| `id` | Issue ID | e.g., bd-123 |
| `assignee` | Assigned user | username |
| `sender` | Issue sender | username |
| `created_by` | Issue creator | username |
| `hook_bead` | Agent's current work | bead ID |
| `role_bead` | Agent's role definition | bead ID |
| `agent_state` | Agent state | idle, running, stuck, stopped |
| `role_type` | Agent role type | polecat, crew, witness, etc. |
| `rig` | Agent's rig name | rig name (empty for town-level) |
| `mol_type` | Molecule type | string |
| `created` | Creation date | today, yesterday, -7d, -3m |
| `updated` | Last update | today, -24h |
| `last_activity` | Agent last activity | today, -24h |

### Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equals | `status = open` |
| `!=` | Not equals | `type != chore` |
| `<` | Less than | `priority < P2` |
| `>` | Greater than | `priority > P3` |
| `<=` | Less or equal | `priority <= P1` |
| `>=` | Greater or equal | `created >= -7d` |
| `~` | Contains | `title ~ auth` |
| `!~` | Not contains | `title !~ test` |
| `in` | In list | `status in (open, in_progress)` |
| `not in` | Not in list | `label not in (backlog)` |

### Boolean Logic

```bql
# AND - both conditions must match
status = open and priority = P0

# OR - either condition matches
type = bug or type = feature

# NOT - negate a condition
not blocked = true

# Parentheses for grouping
(type = bug or type = feature) and priority <= P1
```

### Date Filters

```bql
# Relative dates
created >= -7d          # Last 7 days
updated >= -24h         # Last 24 hours
created >= -3m          # Last 3 months

# Named dates
created >= today
created >= yesterday
```

### Sorting

```bql
# Single field
status = open order by priority

# Multiple fields with direction
type = bug order by priority asc, created desc
```

### Expand (Include Related Issues)

The `expand` keyword includes related issues in your search results, allowing you to see complete issue hierarchies and dependency chains.

```bql
# Basic syntax
<filter> expand <direction> [depth <n>]
```

#### Expansion Directions

| Direction | Description |
|-----------|-------------|
| `up` | Issues you depend on (parents + blockers) |
| `down` | Issues that depend on you (children + blocked issues) |
| `all` | Both directions combined |

#### Depth Control

| Depth | Description |
|-------|-------------|
| `depth 1` | Direct relationships only (default) |
| `depth 2-10` | Include relationships up to N levels deep |
| `depth *` | Unlimited depth (follows all relationships) |

#### Examples

```bql
# Get an epic and all its children
type = epic expand down

# Get an epic and all descendants (unlimited depth)
type = epic expand down depth *

# Get an issue and everything blocking it
id = bd-123 expand up

# Get an issue and all related issues (both directions)
id = bd-123 expand all depth *

# Get all epics with their full hierarchies
type = epic expand all depth *
```

### Example Queries

```bql
# Critical bugs
type = bug and priority = P0

# Ready work, excluding backlog
status = open and ready = true and label not in (backlog)

# Recently updated high-priority items
priority <= P1 and updated >= -24h order by updated desc

# Search by title
title ~ authentication or title ~ login

# Epic with all its children
type = epic expand down depth *
```

---

## Configuration

Perles looks for configuration in these locations (in order):
1. `--config` flag
2. `.perles/config.yaml` (current directory)
3. `~/.config/perles/config.yaml`

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `beads_dir` | string | `""` | Path to beads database directory (default: current directory) |
| `auto_refresh` | bool | `true` | Auto-refresh when database changes |
| `ui.show_counts` | bool | `true` | Show issue counts in column headers |
| `ui.show_status_bar` | bool | `true` | Show status bar at bottom |
| `theme.preset` | string | `""` | Theme preset name (see Theming section) |
| `theme.colors.*` | hex | varies | Individual color token overrides |

### Example Configuration

```yaml
# Path to beads database directory (default: current directory)
# beads_dir: /path/to/project

# Auto-refresh when database changes
auto_refresh: true

# UI settings
ui:
  show_counts: true
  show_status_bar: true

# Theme (use a preset or customize colors)
theme:
  # preset: catppuccin-mocha  # Uncomment to use a theme preset
  # colors:                    # Override specific colors
  #   text.primary: "#FFFFFF"
  #   status.error: "#FF0000"

# Board views
views:
  - name: Default
    columns:
      - name: Blocked
        type: bql
        query: "status = open and blocked = true"
        color: "#FF8787"
      - name: Ready
        query: "status = open and ready = true"
        color: "#73F59F"
      - name: In Progress
        type: bql
        query: "status = in_progress"
        color: "#54A0FF"
      - name: Closed
        type: bql
        query: "status = closed"
        color: "#BBBBBB"

  - name: Bugs Only
    columns:
      - name: Open Bugs
        type: bql
        query: "type = bug and status = open"
        color: "#EF4444"
      - name: In Progress
        type: bql
        query: "type = bug and status = in_progress"
        color: "#F59E0B"
      - name: Fixed
        type: bql
        query: "type = bug and status = closed"
        color: "#10B981"

  - name: Work
    columns:
      - name: Current
        type: tree
        issue_id: bd-123
        tree_mode: child
        color: "#EF4444"
```

---

## Theming

Perles supports comprehensive theming with built-in presets and customizable color tokens.

### Quick Start with Presets

Use a built-in theme preset by adding to your config:

```yaml
theme:
  preset: catppuccin-mocha
```

### Available Presets

Run `perles themes` to see all available presets:

| Preset | Description |
|--------|-------------|
| `default` | Default perles theme |
| `catppuccin-mocha` | Warm, cozy dark theme |
| `catppuccin-latte` | Warm, cozy light theme |
| `dracula` | Dark theme with vibrant colors |
| `nord` | Arctic, north-bluish palette |
| `high-contrast` | High contrast for accessibility |

### Customizing Colors

Override specific colors while using a preset:

```yaml
theme:
  preset: dracula
  colors:
    status.error: "#FF0000"
    priority.critical: "#FF5555"
```

Or create a fully custom theme:

```yaml
theme:
  colors:
    text.primary: "#FFFFFF"
    text.muted: "#888888"
    status.success: "#00FF00"
    status.error: "#FF0000"
    border.default: "#444444"
    border.focus: "#FFFFFF"
```

### Color Tokens

Colors are organized by category:

| Category | Tokens |
|----------|--------|
| **Text** | `text.primary`, `text.secondary`, `text.muted`, `text.description`, `text.placeholder` |
| **Border** | `border.default`, `border.focus`, `border.highlight` |
| **Status** | `status.success`, `status.warning`, `status.error` |
| **Priority** | `priority.critical`, `priority.high`, `priority.medium`, `priority.low`, `priority.backlog` |
| **Issue Status** | `issue.status.open`, `issue.status.in_progress`, `issue.status.closed` |
| **Issue Type** | `type.task`, `type.bug`, `type.feature`, `type.epic`, `type.chore` |
| **BQL Syntax** | `bql.keyword`, `bql.operator`, `bql.field`, `bql.string`, `bql.literal` |
| **Buttons** | `button.text`, `button.primary.bg`, `button.primary.focus`, `button.danger.bg` |
| **Toast** | `toast.success`, `toast.error`, `toast.info`, `toast.warn` |

See `internal/ui/styles/tokens.go` for the complete list of 51 color tokens.

---

## Developer Mode

Developer mode provides logging and debugging tools for troubleshooting and development.

### Enabling Debug Mode

```bash
# Via flag
perles --debug

# Via environment variable
PERLES_DEBUG=1 perles

# With custom log path
PERLES_LOG=/tmp/perles.log perles --debug
```

### Features

- **Log file**: All log output is written to `debug.log` (or custom path via `PERLES_LOG`)
- **Log overlay**: Press `Ctrl+X` to view logs in-app without leaving the TUI
- **Lifecycle logging**: Application startup and shutdown events are logged

<p align="center">
  <img src="./assets/debug-logs-overlay.png" width="1440" alt="board">
</p>

### Reporting Issues

When reporting bugs, please include the `debug.log` file to help with diagnosis:

1. Run perles with `--debug` flag
2. Reproduce the issue
3. Attach `debug.log` to your bug report

---

## Development

### Testing

Run tests:

```bash
make test        # Run all tests
make test-v      # Run with verbose output
```

### Golden Tests

Some tests use [teatest](https://github.com/charmbracelet/x/tree/main/exp/teatest) for snapshot testing of TUI output. These tests compare rendered output against golden files stored in `testdata/` directories.

When you intentionally change UI output, update the golden files:

```bash
make test-update
```

This regenerates golden files in packages with teatest tests (currently `internal/ui/help`). Review the changes before committing to ensure they're expected.

---

## License

MIT
