# AGENTS.md - Perles Development Guide

This document provides essential information for AI agents and developers working
on the Perles codebase. It documents the actual patterns, conventions, and
commands observed in this repository.

## Project Overview

Perles is a terminal-based search and kanban board for [beads](https://github.com/steveyegge/beads)
issue tracking, built in Go using the Bubble Tea TUI framework. It includes a
**multi-agent AI orchestration system** for coordinating AI-powered development workflows.

**Requirements:**

- Go 1.24+
- A beads-enabled project (`.beads/` directory with `beads.db`)
- golangci-lint (for linting)
- mockery (for mock generation)

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
./perles playground # Run the vimtextarea playground
./perles workflows  # List available workflow templates
```

### Testing

```bash
make test           # Run all tests
make test-v         # Run tests with verbose output
make test-update    # Update golden test files for teatest snapshots

# Update specific golden tests
go test -update ./internal/mode/search/...
go test -update ./internal/mode/orchestration/...
go test -update ./internal/ui/board/...
go test -update ./internal/ui/shared/vimtextarea/...

# Run specific package tests
go test ./internal/bql/...
go test ./internal/orchestration/v2/...
go test -v ./cmd/...
```

### Code Quality

```bash
make lint           # Run golangci-lint (configured in .golangci.yml)
make mocks          # Generate mocks using mockery
make mocks-clean    # Remove generated mocks
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
│   ├── themes.go          # Theme commands
│   ├── workflows.go       # Workflow listing command
│   └── playground.go      # Playground command
├── internal/              # Internal packages (not exported)
│   ├── app/              # Root application model & orchestration
│   ├── mode/             # Application modes with Controller interface
│   │   ├── kanban/       # Kanban board mode
│   │   ├── search/       # Search mode
│   │   ├── orchestration/# AI orchestration mode (multi-agent TUI)
│   │   ├── playground/   # VimTextarea testing playground
│   │   └── shared/       # Cross-mode utilities (Clipboard, Clock)
│   ├── beads/            # Database client and domain models
│   ├── bql/              # Query language implementation
│   │   ├── lexer.go      # Tokenization
│   │   ├── parser.go     # AST building
│   │   ├── executor.go   # Query execution
│   │   └── validator.go  # Query validation
│   ├── cachemanager/     # Generic caching infrastructure
│   ├── git/              # Git executor for worktree operations
│   ├── orchestration/    # AI orchestration system
│   │   ├── client/       # Provider-agnostic headless AI client
│   │   ├── amp/          # Amp CLI integration
│   │   ├── claude/       # Claude Code CLI integration
│   │   ├── codex/        # OpenAI Codex CLI integration
│   │   ├── v2/           # V2 command processor & handlers
│   │   ├── mcp/          # Model Context Protocol server
│   │   ├── events/       # Process event types
│   │   ├── workflow/     # Workflow template registry
│   │   ├── session/      # Session tracking
│   │   ├── metrics/      # Token usage tracking
│   │   └── message/      # Inter-agent message log
│   ├── pubsub/           # Generic pub/sub event broker
│   ├── ui/               # UI components
│   │   ├── board/        # Kanban board view
│   │   ├── tree/         # Tree view for dependencies
│   │   ├── details/      # Issue details panel
│   │   ├── coleditor/    # Column editor UI
│   │   ├── commandpalette/ # Command palette for orchestration
│   │   ├── modals/       # Modal dialogs
│   │   ├── nobeads/      # Empty state when no .beads DB
│   │   ├── outdated/     # Database version too old state
│   │   ├── shared/       # Reusable UI components
│   │   │   └── vimtextarea/ # Vim-like textarea widget
│   │   └── styles/       # Theme system
│   ├── config/           # Configuration management
│   ├── log/              # Debug logging
│   ├── watcher/          # File system watching
│   ├── keys/             # Keyboard shortcut definitions
│   ├── mocks/            # Generated mockery mocks
│   └── testutil/         # Test utilities and builders
├── examples/             # Example configurations
│   └── themes/          # Theme examples
├── assets/              # Screenshots and videos
├── main.go              # Entry point
├── Makefile             # Build automation
├── go.mod               # Go module dependencies
├── .mockery.yaml        # Mockery configuration
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
- **Constants:** PascalCase for enums (`FocusSearch`, `ModeKanban`, `ModeOrchestration`)
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

| Library | Purpose |
|---------|---------|
| `github.com/stretchr/testify` | Assertions (`require.NoError`, `require.Equal`) |
| `github.com/charmbracelet/x/exp/teatest` | Golden/snapshot testing for TUI components |
| `pgregory.net/rapid` | Property-based testing for stress testing |
| Standard `testing` package | Base test infrastructure |

### Mock Generation

- **Tool**: [mockery](https://github.com/vektra/mockery) with `.mockery.yaml` config
- **Output**: `internal/mocks/` directory
- **Command**: `make mocks` to regenerate
- **Interfaces mocked**: `BeadsClient`, `BQLExecutor`, `CacheManager`, `Clock`, `Clipboard`, `HeadlessClient`, `HeadlessProcess`, `GitExecutor`

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

**Property-Based Test (with Rapid):**

```go
func TestProcessor_Concurrent(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        // Generate random inputs
        // Verify invariants hold
    })
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
go test -update ./internal/mode/orchestration/...
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
    Client             beads.BeadsClient
    Executor           bql.BQLExecutor
    Config             *config.Config
    ConfigPath         string
    DBPath             string
    WorkDir            string
    Clipboard          shared.Clipboard
    Clock              shared.Clock
    GitExecutorFactory func(path string) git.GitExecutor
}
```

### Controller Interface

Different modes implement the same interface:

```go
type Controller interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (Controller, tea.Cmd)
    View() string
    SetSize(width, height int) Controller
}
```

### Application Modes

```go
const (
    ModeKanban AppMode = iota
    ModeSearch
    ModeOrchestration
)
```

## AI Orchestration System

Perles includes a sophisticated multi-agent AI orchestration layer for coordinating AI-powered development workflows.

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Orchestration Mode (TUI)                  │
├─────────────┬───────────────────────┬───────────────────────┤
│ Coordinator │   Shared Message Log  │    Worker Outputs     │
│    Chat     │                       │                       │
│   (~25%)    │       (~40%)          │       (~35%)          │
└─────────────┴───────────────────────┴───────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                     V2 Command Processor                     │
│  CQRS-style: Commands mutate state, Queries bypass          │
├─────────────────────────────────────────────────────────────┤
│  Handlers: SpawnProcess, AssignTask, SendToProcess, etc.    │
│  Repositories: ProcessRepo, TaskRepo, QueueRepo (in-memory) │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                         Headless AI Clients                               │
├─────────────────┬─────────────────┬─────────────────┬─────────────────────┤
│   Claude Code   │       Amp       │      Codex      │      OpenCode       │
│   (claude/)     │     (amp/)      │    (codex/)     │    (opencode/)      │
└─────────────────┴─────────────────┴─────────────────┴─────────────────────┘
```

### Key Components

| Package | Purpose |
|---------|---------|
| `orchestration/client/` | Provider-agnostic `HeadlessClient` and `HeadlessProcess` interfaces |
| `orchestration/amp/` | Amp CLI integration |
| `orchestration/claude/` | Claude Code CLI integration |
| `orchestration/codex/` | OpenAI Codex CLI integration |
| `orchestration/opencode/` | OpenCode CLI integration (GLM-4.7, file-based MCP config) |
| `orchestration/v2/` | Command processor, handlers, adapters, repositories |
| `orchestration/mcp/` | MCP server exposing coordinator/worker tools |
| `orchestration/events/` | Unified `ProcessEvent` type |
| `orchestration/workflow/` | Workflow template registry (built-in + user-defined) |
| `orchestration/session/` | On-disk session logging |
| `orchestration/metrics/` | Token usage and cost tracking |

### Process Events

All orchestration events use the unified `ProcessEvent` type:

```go
type ProcessEvent struct {
    Type      ProcessEventType  // ProcessSpawned, ProcessOutput, ProcessStatusChange, etc.
    ProcessID string
    Role      ProcessRole       // RoleCoordinator or RoleWorker
    Status    ProcessStatus     // Pending, Ready, Working, Paused, Stopped, Retired, Failed
    Phase     ProcessPhase      // Idle, Implementing, Reviewing, Committing, etc.
    Output    string
    Metrics   *metrics.TokenMetrics
    // ...
}

// Filter by role
if event.IsCoordinator() { ... }
if event.IsWorker() { ... }
```

### MCP Coordinator Tools

Tools exposed to the AI coordinator via MCP:

- `spawn_worker`, `assign_task`, `replace_worker`, `retire_worker`
- `send_to_worker`, `post_message`, `read_message_log`
- `get_task_status`, `mark_task_complete`, `mark_task_failed`
- `prepare_handoff`, `stop_worker`, `generate_accountability_summary`

### Workflow Templates

Built-in workflows in `orchestration/workflow/`:
- `quick_plan.md` - Quick planning workflow
- `cook.md` - Cooking/implementation workflow
- `research_to_tasks.md` - Research to task breakdown

User workflows: `~/.config/perles/workflows/`

List available: `perles workflows`

### Centralized Session Storage

Orchestration sessions are stored in a centralized user home directory to simplify backup, management, and cross-project querying.

**Default location:** `~/.perles/sessions/`

**Directory structure:**
```
~/.perles/
└── sessions/
    ├── sessions.json                    # Global session index
    └── {application_name}/              # Application name (repo name or custom)
        ├── sessions.json                # Per-application session index
        ├── 2026-01-10/
        │   └── a1b2c3d4-5678-uuid/
        │       ├── metadata.json
        │       ├── coordinator/
        │       ├── workers/
        │       └── messages.jsonl
        └── 2026-01-11/
            └── e5f6g7h8-9012-uuid/
```

**Application name derivation:**
1. Config override (`orchestration.session_storage.application_name`)
2. Git remote URL (repository name without `.git`)
3. Working directory name (fallback)

**Configuration:**
```yaml
orchestration:
  session_storage:
    base_dir: ~/.perles/sessions        # Default location
    application_name: my-custom-name    # Optional: override derived name
```

**Benefits:**
- **Single location**: All sessions in one place for easy backup
- **Date-based partitioning**: Easy cleanup of old sessions
- **Cross-project visibility**: Future CLI can list/search sessions across projects
- **Survives project deletion**: Session history preserved independently
- **No project pollution**: Projects stay clean (no `.perles/` directory needed)

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
  markdown_style: dark               # Markdown rendering style

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

orchestration:                       # AI orchestration settings
  client: "claude"                   # claude, amp, codex, gemini, or opencode
  disable_worktrees: false           # Disable git worktree isolation
  session_storage:                   # Centralized session storage
    base_dir: ~/.perles/sessions     # Default storage location
    application_name: ""             # Override: defaults to git repo name
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

### Orchestration Mode

- **Navigation:** `Ctrl+f` (cycle focus between panes)
- **Coordinator:** Type in chat input, `Enter` (send)
- **Control:** `Ctrl+z` (pause), `Ctrl+r` (replace coordinator)
- **Workflows:** `Ctrl+p` (workflow template palette)
- **General:** `?` (help), `q` (quit with confirmation)

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

### Orchestration Safety

- Orchestration mode can create/delete git worktrees
- Prompts if worktree has uncommitted changes before exit
- Use `disable_worktrees: true` to disable worktree isolation

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
- BQL queries and dependency graphs are cached via `CacheManager`
- Caches are flushed on database file changes
- Debounce file system events

## Common Tasks

### Adding a New Column Type

1. Update `ColumnConfig` in `internal/config/config.go`
2. Implement renderer in `internal/ui/board/`
3. Add validation in `ValidateColumns()`
4. Update form in `internal/ui/coleditor/`
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

All orchestration events use the unified `ProcessEvent` type (`internal/orchestration/events/process.go`):

- `ProcessSpawned` - New process created
- `ProcessOutput` - Process produced output
- `ProcessStatusChange` - Process status changed
- `ProcessTokenUsage` - Process token usage update
- `ProcessIncoming` - Message sent to process
- `ProcessError` - Process error occurred
- `ProcessQueueChanged` - Process message queue changed
- `ProcessReady` - Process ready for input
- `ProcessWorking` - Process processing

Filter by `Role` field: `events.RoleCoordinator` or `events.RoleWorker`

### Subscribing to Events

Events are delivered via pub/sub brokers. Subscribe with a context for automatic cleanup:

```go
import (
    "context"
    "perles/internal/orchestration/events"
    "perles/internal/pubsub"
)

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Subscribe to unified event bus
ch := v2EventBus.Subscribe(ctx)

for event := range ch {
    if processEvent, ok := event.Payload.(events.ProcessEvent); ok {
        if processEvent.IsCoordinator() {
            // Handle coordinator event
        } else if processEvent.IsWorker() {
            // Handle worker event
        }
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
    v2Listener *pubsub.ContinuousListener[any]
    ctx        context.Context
    cancel     context.CancelFunc
}

// Initialize listener
func (m Model) initListeners(v2EventBus *pubsub.Broker[any]) (Model, tea.Cmd) {
    m.v2Listener = pubsub.NewContinuousListener(m.ctx, v2EventBus)
    return m, m.v2Listener.Listen()
}

// Handle events in Update
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    switch msg := msg.(type) {
    case pubsub.Event[any]:
        if processEvent, ok := msg.Payload.(events.ProcessEvent); ok {
            if processEvent.IsWorker() {
                // Handle worker event
            } else if processEvent.IsCoordinator() {
                // Handle coordinator event
            }
        }
        // Always continue listening!
        return m, m.v2Listener.Listen()
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

## Key Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/bubbles` | TUI components |
| `github.com/charmbracelet/lipgloss` | TUI styling |
| `github.com/charmbracelet/glamour` | Markdown rendering |
| `github.com/charmbracelet/x/exp/teatest` | Golden/snapshot testing |
| `github.com/spf13/cobra` | CLI commands |
| `github.com/spf13/viper` | Configuration loading |
| `github.com/ncruces/go-sqlite3` | SQLite database driver |
| `github.com/fsnotify/fsnotify` | File system watching |
| `github.com/stretchr/testify` | Test assertions |
| `pgregory.net/rapid` | Property-based testing |

## Resources

- [Bubble Tea Documentation](https://github.com/charmbracelet/bubbletea)
- [Beads Issue Tracker](https://github.com/steveyegge/beads)
- [Lipgloss Styling](https://github.com/charmbracelet/lipgloss)
- [Orchestration V2 Docs](./internal/orchestration/v2/docs/README.md)
- [Project README](./README.md)
- [Contributing Guide](./CONTRIBUTING.md)
