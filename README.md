# Perles

A terminal-based search and kanban board for [beads](https://github.com/anthropics/beads) issue tracking.

## Features

### Kanban Board
- Four-column default layout: Blocked, Ready, In Progress, Closed
- Fully customizable columns with BQL queries
- Multi-view support - create unlimited board views
- Real-time auto-refresh when database changes
- Column management: add, edit, reorder, delete

### BQL (Beads Query Language)
- Query syntax for filtering issues
- Fields: type, priority, status, blocked, ready, label, title, id, created, updated
- Operators: = != < > <= >= ~ (contains) !~ (not contains) in not-in
- Boolean logic: and, or, not, parentheses
- Sorting: order by field asc/desc
- Date filters: today, yesterday, -7d, -24h, -3m

### Search Mode
- Full-screen BQL-powered search interface
- Save searches as kanban columns
- Create new views from search results

### Issue Management
- View detailed issue information with markdown rendering
- Edit priority, status, and labels inline
- Navigate to dependency issues
- Copy issue IDs to clipboard

## Video Demos

### Kanban Board Navigation
<!-- VIDEO_PLACEHOLDER: kanban-navigation.mp4 -->
<!-- Demo: Navigate between columns with h/l, select issues with j/k, open detail view with Enter -->
*Video coming soon*

### BQL Search Mode
<!-- VIDEO_PLACEHOLDER: bql-search-demo.mp4 -->
<!-- Demo: Switch to search mode with Ctrl+Space, write BQL query, navigate results, save as column -->
*Video coming soon*

### Creating Custom Views
<!-- VIDEO_PLACEHOLDER: custom-views-demo.mp4 -->
<!-- Demo: Create new view with Ctrl+V, add columns with BQL queries, switch between views with Ctrl+J/K -->
*Video coming soon*

### Configuration & Theming
<!-- VIDEO_PLACEHOLDER: configuration-demo.mp4 -->
<!-- Demo: Show config file, customize columns with different BQL queries, change theme colors -->
*Video coming soon*

## Requirements

- A beads-enabled project (`.beads/` directory with `beads.db`)

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

## Keybindings

### Global

| Key | Action |
|-----|--------|
| `Ctrl+Space` | Switch between Kanban and Search modes |
| `?` | Toggle help overlay |
| `q` / `Ctrl+C` | Quit |

### Kanban Mode - Navigation

| Key | Action |
|-----|--------|
| `h` / `←` | Move to left column |
| `l` / `→` | Move to right column |
| `j` / `↓` | Move down in column |
| `k` / `↑` | Move up in column |
| `Enter` | View issue details |

### Kanban Mode - Views

| Key | Action |
|-----|--------|
| `Ctrl+J` / `Ctrl+N` | Next view |
| `Ctrl+K` / `Ctrl+P` | Previous view |
| `Ctrl+V` | Create new view |
| `Ctrl+D` | Delete current view |

### Kanban Mode - Columns

| Key | Action |
|-----|--------|
| `a` | Add new column |
| `e` | Edit current column |
| `Ctrl+H` | Move column left |
| `Ctrl+L` | Move column right |
| `/` | Open search with column's BQL query |

### Kanban Mode - Issues

| Key | Action |
|-----|--------|
| `r` | Refresh issues |
| `y` | Copy issue ID to clipboard |
| `s` | Change status (in detail view) |
| `p` | Change priority (in detail view) |
| `w` | Toggle status bar |

### Search Mode

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
| `Esc` | Blur input / Go back |

## Default Columns

The default view includes these columns (all configurable via BQL):

| Column | BQL Query |
|--------|-----------|
| **Blocked** | `status = open and blocked = true` |
| **Ready** | `status = open and ready = true` |
| **In Progress** | `status = in_progress` |
| **Closed** | `status = closed` |

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
| `label` | Issue labels | any label string |
| `title` | Issue title | any text (use ~ for contains) |
| `id` | Issue ID | e.g., bd-123 |
| `created` | Creation date | today, yesterday, -7d, -3m |
| `updated` | Last update | today, -24h |

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
created = today
created = yesterday
```

### Sorting

```bql
# Single field
status = open order by priority

# Multiple fields with direction
type = bug order by priority asc, created desc
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
```

## Configuration

Perles looks for configuration in these locations (in order):
1. `--config` flag
2. `.perles/config.yaml` (current directory)
3. `~/.config/perles/config.yaml`

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `auto_refresh` | bool | `true` | Auto-refresh when database changes |
| `auto_refresh_debounce` | duration | `1s` | Debounce delay for auto-refresh |
| `ui.show_counts` | bool | `true` | Show issue counts in column headers |
| `ui.show_status_bar` | bool | `true` | Show status bar at bottom |
| `theme.highlight` | hex | `#7C3AED` | Primary accent color |
| `theme.subtle` | hex | `#6B7280` | Muted text color |
| `theme.error` | hex | `#EF4444` | Error message color |
| `theme.success` | hex | `#10B981` | Success message color |

### Example Configuration

```yaml
# Auto-refresh when database changes
auto_refresh: true
auto_refresh_debounce: 1s

# UI settings
ui:
  show_counts: true
  show_status_bar: true

# Theme colors (hex format)
theme:
  highlight: "#7C3AED"
  subtle: "#6B7280"
  error: "#EF4444"
  success: "#10B981"

# Board views
views:
  - name: Default
    columns:
      - name: Blocked
        query: "status = open and blocked = true"
        color: "#FF8787"
      - name: Ready
        query: "status = open and ready = true"
        color: "#73F59F"
      - name: In Progress
        query: "status = in_progress"
        color: "#54A0FF"
      - name: Closed
        query: "status = closed"
        color: "#BBBBBB"

  - name: Bugs Only
    columns:
      - name: Open Bugs
        query: "type = bug and status = open"
        color: "#EF4444"
      - name: In Progress
        query: "type = bug and status = in_progress"
        color: "#F59E0B"
      - name: Fixed
        query: "type = bug and status = closed"
        color: "#10B981"
```

## Architecture

Perles reads from the beads SQLite database (`.beads/beads.db`) and executes `bd` CLI commands for mutations. This ensures data integrity and consistency with the beads ecosystem.

```
perles
├── cmd/perles/        # Main entry point
└── internal/
    ├── app/           # Root application model
    ├── beads/         # SQLite client and CLI executor
    ├── bql/           # BQL parser and executor
    ├── config/        # Configuration handling
    ├── keys/          # Keybinding definitions
    ├── mode/          # Mode controllers (kanban, search)
    ├── watcher/       # File watcher for auto-refresh
    └── ui/
        ├── board/     # Kanban board and column components
        ├── bqlinput/  # BQL input with syntax highlighting
        ├── coleditor/ # Column editor modal
        ├── colorpicker/ # Color picker for columns
        ├── details/   # Issue detail view
        ├── help/      # Help overlay
        ├── labeleditor/ # Label editor modal
        ├── modal/     # Generic modal component
        ├── picker/    # Status/priority picker
        ├── styles/    # Shared lip gloss styles
        ├── toaster/   # Toast notifications
        └── viewselector/ # View switcher component
```

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

## License

MIT
