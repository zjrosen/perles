# AGENTS.md - Perles Development Guide

This document provides essential information for AI agents and developers working
on the Perles codebase. It documents the actual patterns, conventions, and
commands observed in this repository.

## Project Overview

Perles is a terminal-based search and kanban board for [beads](https://github.com/steveyegge/beads)
issue tracking, built in Go using the Bubble Tea TUI framework.

**Requirements:**

- Go 1.24+
- A beads-enabled project (`.beads/` directory with `beads.db`)
- golangci-lint (for linting)

## Essential Commands

### Build & Run

```bash
make build          # Build the binary with version info
make run            # Build and run
make debug          # Build and run with debug flag (-d)
make install        # Install to $GOPATH/bin
make clean          # Clean build artifacts

./perles            # Run the built binary
./perles -d         # Run with debug mode
./perles -c path    # Use specific config file
./perles -b path    # Specify beads directory
```

### Testing

```bash
make test           # Run all tests
make test-v         # Run tests with verbose output
make test-update    # Update golden test files for teatest snapshots

# Update specific golden tests
go test -update ./internal/mode/search/...
go test -update ./internal/ui/board/...

# Run specific package tests
go test ./internal/bql/...
go test -v ./cmd/...
```

### Code Quality

```bash
make lint           # Run golangci-lint (configured in .golangci.yml)
go fmt ./...        # Format code (Go standard)
```

### Version Control

```bash
# The project uses standard Git workflow
# Version is automatically extracted from git tags
git describe --tags --always --dirty  # Version format used
```

## Code Organization

### Directory Structure

```
perles/
├── cmd/                    # CLI commands (cobra)
│   ├── root.go            # Main command setup
│   ├── init.go            # Init command
│   └── themes.go          # Theme commands
├── internal/              # Internal packages (not exported)
│   ├── app/              # Root application model & orchestration
│   ├── mode/             # Application modes with Controller interface
│   │   ├── kanban/       # Kanban board mode
│   │   └── search/       # Search mode
│   ├── beads/            # Database client and domain models
│   ├── bql/              # Query language implementation
│   │   ├── lexer.go      # Tokenization
│   │   ├── parser.go     # AST building
│   │   ├── executor.go   # Query execution
│   │   └── validator.go  # Query validation
│   ├── ui/               # UI components
│   │   ├── board/        # Kanban board view
│   │   ├── tree/         # Tree view for dependencies
│   │   ├── details/      # Issue details panel
│   │   ├── forms/        # Form components
│   │   ├── modals/       # Modal dialogs
│   │   ├── shared/       # Reusable UI components
│   │   └── styles/       # Theme system
│   ├── config/           # Configuration management
│   ├── log/              # Debug logging
│   ├── watcher/          # File system watching
│   ├── keys/             # Keyboard shortcut definitions
│   └── testutil/         # Test utilities and builders
├── examples/             # Example configurations
│   └── themes/          # Theme examples
├── assets/              # Screenshots and videos
├── main.go              # Entry point
├── Makefile             # Build automation
├── go.mod               # Go module dependencies
└── .golangci.yml        # Linter configuration
```

## Code Conventions

### Package Naming

- Short, lowercase, descriptive names (`app`, `mode`, `beads`, `bql`, `ui`)
- Internal packages under `internal/` directory
- Package comment at top of main file: `// Package x provides...`

### File Naming

- Snake_case for file names: `search.go`, `search_test.go`, `golden_test.go`
- Test files alongside implementation: `foo.go` → `foo_test.go`
- Golden test files in `testdata/`: `testdata/*.golden`

### Type & Variable Naming

- **Exported types:** PascalCase (`Model`, `Client`, `Controller`)
- **Unexported types:** camelCase (`searchModel`, `columnConfig`)
- **Messages:** Descriptive with `Msg` suffix (`DBChangedMsg`, `SaveSearchAsColumnMsg`)
- **Constants:** PascalCase for enums (`FocusSearch`, `ModeKanban`)
- **Interfaces:** Small, focused, verb-er naming (`Controller`, `Builder`)

### Import Organization

```go
import (
    // Standard library
    "fmt"
    "os"
    
    // External dependencies
    "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    
    // Internal packages
    "perles/internal/app"
    "perles/internal/beads"
)
```

### Error Handling

- Explicit error returns: `func Foo() (string, error)`
- Wrap errors with context: `fmt.Errorf("failed to load: %w", err)`
- Check errors immediately, don't ignore
- Use early returns for error cases

## Testing Patterns

### Test Frameworks

- **testify:** For assertions (`require`)
- **teatest:** For golden/snapshot testing of TUI components
- Standard Go testing package as base

### Test File Organization

- Unit tests: `*_test.go` alongside implementation
- Golden files: `testdata/*.golden`
- Test utilities: `internal/testutil/`

### Common Test Patterns

**Basic Unit Test:**

```go
func TestComponent_Method(t *testing.T) {
    m := New()
    result := m.Method(input)
    require.Equal(t, expected, result)
}
```

**Golden/Snapshot Test:**

```go
func TestComponent_View_Golden(t *testing.T) {
    m := New().SetSize(100, 30)
    view := m.View()
    teatest.RequireEqualOutput(t, []byte(view))
}
```

**Database Test Setup:**

```go
func TestWithDB(t *testing.T) {
    db := testutil.NewTestDB(t)
    testutil.NewBuilder(t, db).
        WithStandardTestData().
        Build()
    // test code...
}
```

### Test Data Builders

The `testutil` package provides fluent builders for creating test issue setup:

```go
WithIssue("issue-id",
    Title("Bug in auth"),
    Status("open"),
    Priority(1),
    Labels("bug", "urgent"))
```

### Updating Golden Tests

```bash
# Update all golden files
make test-update

# Update specific package
go test -update ./internal/mode/search/...
```

## Architecture Patterns

### MVC-like Structure

- **Models:** Hold state and business logic
- **Update:** Handle events and state mutations (Bubble Tea pattern)
- **View:** Render UI from model state
- Components implement `tea.Model` interface

### Message Passing

Components communicate via Bubble Tea messages:

```go
type SearchExecutedMsg struct {
    Query   string
    Results []Issue
}
```

### Service Injection

Shared dependencies passed via `Services` struct:

```go
type Services struct {
    Client  *beads.Client
    Config  *config.Config
    Logger  *log.Logger
    Watcher *watcher.Watcher
}
```

### Controller Interface

Different modes implement the same interface:

```go
type Controller interface {
    Update(tea.Msg) (tea.Model, tea.Cmd)
    View() string
    Init() tea.Cmd
    SetSize(width, height int)
}
```

## BQL (Beads Query Language)

### Fields

`id`, `type`, `status`, `priority`, `title`, `body`, `created`, `updated`,
`parent_id`, `blocked`, `blocks`, `ready`, `label`, `assignee`

### Operators

- **Comparison:** `=`, `!=`, `<`, `>`, `<=`, `>=`
- **String:** `~` (contains), `!~` (not contains)
- **Logical:** `and`, `or`, `not`, parentheses
- **Set:** `in (values)`, `not in (values)`

### Value Types

- **Strings:** `"quoted"` or `unquoted`
- **Priorities:** `P0`, `P1`, `P2`, `P3`, `P4`
- **Booleans:** `true`, `false`
- **Dates:** `today`, `yesterday`, `-7d`, `-30d`

### Special Clauses

- **EXPAND:** `expand up|down|all [depth N|*]` - traverse relationships
- **ORDER BY:** `order by field [asc|desc]` - sort results

### Example Queries

```sql
type = bug and priority = P0
status != closed and ready = true
title ~ "auth" and label in (security, urgent)
created > -7d order by priority asc
type = epic expand down depth 2
```

## Configuration

### Config File Location

Default: `~/.config/perles/config.yaml`

### Config Structure

```yaml
beads_dir: .beads                    # Database directory
auto_refresh: true                    # Watch for changes

ui:
  show_counts: true                   # Show issue counts
  show_status_bar: true              # Show status bar

theme:
  preset: "catppuccin-mocha"         # Built-in theme
  mode: "dark"                       # Force dark/light
  colors:                            # Custom overrides
    "text.primary": "#E0E0E0"

views:                               # Board views
  - name: "Default"
    columns:
      - name: "Blocked"
        type: "bql"                  # or "tree"
        query: "blocked = true"
        color: "#FF8787"
      - name: "Dependencies"
        type: "tree"
        issue_id: "ISSUE-123"
        tree_mode: "deps"            # or "child"
```

## Environment Variables

- `PERLES_DEBUG`: Enable debug mode
- `PERLES_LOG`: Debug log file path (default: `debug.log`)
- `UPDATE_GOLDEN`: Test variable for updating golden files

## Keyboard Shortcuts

### Kanban Mode

- **Navigation:** `h/j/k/l` or arrow keys
- **Column management:** `a` (add), `e` (edit), `Ctrl+h/l` (move)
- **View management:** `Ctrl+n/p` (switch), `Ctrl+v` (menu)
- **Actions:** `Enter` (open), `s` (status), `p` (priority), `y` (copy ID)
- **Mode switch:** `Ctrl+Space` (search), `/` (search column)
- **General:** `?` (help), `q` (quit), `r` (refresh)

### Search Mode (Supports regular search and tree sub-mode)

- **Navigation:** `j/k` (move), `h/l` (focus panes)
- **Search:** `/` (focus input), `Enter` (execute)
- **Actions:** `s` (status), `p` (priority), `y` (copy ID)
- **Save:** `Ctrl+s` (save to view)
- **General:** `Ctrl+Space` (kanban), `?` (help), `q` (quit)

## Important Gotchas

### Database Requirements

- Requires `.beads/beads.db` SQLite database
- Database must be initialized with beads schema
- Auto-refresh watches the database file for changes

### Terminal Compatibility

- Requires terminal with 256 color support
- Mouse support optional but enhances UX
- Minimum recommended size: 80x24

### Golden Test Management

- Golden tests capture exact terminal output
- Update with `go test -update` when UI changes
- Review diffs carefully before committing

### Theme System

- Themes use semantic color tokens
- Custom themes override specific tokens
- Presets: default, catppuccin-*, dracula, nord, high-contrast

### Column Types

- **BQL columns:** Filter issues with queries
- **Tree columns:** Show dependency trees or child hierarchies
- Mixed column types supported in same view

## CI/CD

### GitHub Actions Workflow

- **Triggers:** Push to main, pull requests
- **Platforms:** Ubuntu, macOS
- **Steps:** Build → Test → Lint
- **Go version:** 1.24
- **Linter:** golangci-lint (latest)

### Release Process

- Uses goreleaser for releases
- Binaries built for linux/darwin, amd64/arm64
- Version extracted from git tags
- Install script at `install.sh`

## Development Tips

### Debugging

```bash
./perles -d                  # Enable debug mode
tail -f debug.log           # Watch debug output
PERLES_DEBUG=1 ./perles     # Alternative debug enable
```

### Working with Bubble Tea

- Components are immutable - always return new instances
- Use commands for async operations
- Messages drive all state changes
- Keep Update methods pure

### Adding New Features

1. Define messages in appropriate package
2. Implement handler in Update method
3. Add keybinding in `internal/keys/`
4. Update help text in modals
5. Write tests including golden tests
6. Update this document if needed

### Performance Considerations

- Database queries are the main bottleneck
- Use pagination for large result sets
- Cache frequently accessed data
- Debounce file system events

## Common Tasks

### Adding a New Column Type

1. Update `ColumnConfig` in `internal/config/config.go`
2. Implement renderer in `internal/ui/board/`
3. Add validation in `ValidateColumns()`
4. Update form in `internal/ui/forms/columneditor/`
5. Write tests for new functionality

### Adding a BQL Operator

1. Update lexer in `internal/bql/lexer.go`
2. Update parser in `internal/bql/parser.go`
3. Implement in executor `internal/bql/executor.go`
4. Add validation in `internal/bql/validator.go`
5. Update BQL documentation

### Creating a New Mode

1. Create package under `internal/mode/`
2. Implement `Controller` interface
3. Add to mode switching in `internal/app/`
4. Define keybindings in `internal/keys/`
5. Add help documentation

## Pub/Sub Event System

Perles uses a generic pub/sub broker for decoupled event communication between the orchestration layer and TUI. This architecture enables multiple subscribers (TUI, logging, metrics) to receive events without tight coupling.

### Event Types

Two event types are used in orchestration:

**CoordinatorEvent** (`internal/orchestration/events/coordinator.go`):
- `CoordinatorChat` - Text output from the coordinator
- `CoordinatorStatusChange` - Status transitions (ready, working, paused, stopped)
- `CoordinatorTokenUsage` - Token usage updates
- `CoordinatorError` - Error notifications
- `CoordinatorReady` - Coordinator ready for input
- `CoordinatorWorking` - Coordinator processing

**WorkerEvent** (`internal/orchestration/events/worker.go`):
- `WorkerSpawned` - New worker created
- `WorkerOutput` - Worker produced output
- `WorkerStatusChange` - Worker status changed
- `WorkerTokenUsage` - Worker token usage update
- `WorkerIncoming` - Message sent to worker
- `WorkerError` - Worker error occurred

### Subscribing to Events

Events are delivered via the pub/sub broker. Subscribe with a context for automatic cleanup:

```go
import (
    "context"
    "perles/internal/orchestration/events"
    "perles/internal/pubsub"
)

// Subscribe to coordinator events
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

coordCh := coordinator.Subscribe(ctx)
workerCh := coordinator.Workers().Subscribe(ctx)

// Receive events
for event := range coordCh {
    switch event.Payload.Type {
    case events.CoordinatorChat:
        fmt.Printf("[%s] %s\n", event.Payload.Role, event.Payload.Content)
    case events.CoordinatorTokenUsage:
        fmt.Printf("Tokens: %d/%d\n", event.Payload.ContextTokens, event.Payload.ContextWindow)
    }
}
```

### Using ContinuousListener in Bubble Tea

For Bubble Tea integration, use `ContinuousListener` to maintain subscription state across the update loop:

```go
import (
    "context"
    tea "github.com/charmbracelet/bubbletea"
    "perles/internal/orchestration/events"
    "perles/internal/pubsub"
)

type Model struct {
    coordListener  *pubsub.ContinuousListener[events.CoordinatorEvent]
    workerListener *pubsub.ContinuousListener[events.WorkerEvent]
    ctx            context.Context
    cancel         context.CancelFunc
}

// Initialize listeners after coordinator starts
func (m Model) initListeners(coord *coordinator.Coordinator) (Model, tea.Cmd) {
    m.coordListener = pubsub.NewContinuousListener(m.ctx, coord.Broker)
    m.workerListener = pubsub.NewContinuousListener(m.ctx, coord.Workers())

    return m, tea.Batch(
        m.coordListener.Listen(),
        m.workerListener.Listen(),
    )
}

// Handle events in Update
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    switch msg := msg.(type) {
    case pubsub.Event[events.CoordinatorEvent]:
        // Handle coordinator event
        switch msg.Payload.Type {
        case events.CoordinatorChat:
            m = m.addMessage(msg.Payload.Role, msg.Payload.Content)
        case events.CoordinatorReady:
            m.status = "ready"
        }
        // Continue listening (always!)
        return m, m.coordListener.Listen()

    case pubsub.Event[events.WorkerEvent]:
        // Handle worker event
        switch msg.Payload.Type {
        case events.WorkerOutput:
            m = m.addWorkerOutput(msg.Payload.WorkerID, msg.Payload.Output)
        }
        // Continue listening (always!)
        return m, m.workerListener.Listen()
    }
    return m, nil
}
```

### Key Points

1. **Context-based cleanup**: Subscriptions are automatically removed when the context is cancelled
2. **Non-blocking publish**: Events are dropped if subscriber channels are full (prevents blocking)
3. **Multiple subscribers**: Any number of subscribers can receive the same events
4. **Thread-safe**: Brokers are safe for concurrent publish/subscribe operations
5. **Always continue listening**: In Bubble Tea, always return a new `Listen()` command after handling an event

### Broker Methods

```go
// Create a broker
broker := pubsub.NewBroker[T]()
broker := pubsub.NewBrokerWithBuffer[T](bufferSize) // Custom buffer

// Subscribe (auto-cleanup on context cancel)
ch := broker.Subscribe(ctx)

// Publish (non-blocking)
broker.Publish(pubsub.UpdatedEvent, payload)

// Check subscriber count
count := broker.SubscriberCount()

// Clean shutdown
broker.Close()
```

## Resources

- [Bubble Tea Documentation](https://github.com/charmbracelet/bubbletea)
- [Beads Issue Tracker](https://github.com/steveyegge/beads)
- [Lipgloss Styling](https://github.com/charmbracelet/lipgloss)
- [Project README](./README.md)
- [Contributing Guide](./CONTRIBUTING.md)

