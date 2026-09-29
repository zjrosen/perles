# AGENTS.md - Perles Development Guide

This document provides essential information for AI agents and developers working
on the Perles codebase. It documents the actual patterns, conventions, and
commands observed in this repository.

## Project Overview

Perles is a terminal-based search and kanban board for [beads](https://github.com/steveyegge/beads)
issue tracking, built in Go using the Bubble Tea TUI framework. It includes a
**multi-agent AI orchestration system** for coordinating AI-powered development workflows.

**Requirements:**

- Go 1.27+
- Node.js + npm (version in `.nvmrc`) for `make build` / `make install`, which rebuild the
  embedded frontend first. `make build-go` skips npm and embeds the committed `frontend/dist/`.
- A project with a `.beads/` directory, using either:
  - beads >= v0.62.0 (`MinBeadsVersion`), backed by SQLite (`.beads/beads.db`) or a Dolt
    server (`"backend": "dolt"` with `"dolt_mode": "server"` in `.beads/metadata.json`).
    Embedded Dolt (the default when `dolt_mode` is absent) is not supported.
  - beads_rust (`backend: beads_rust` in the perles config)
- The `bd` CLI (or `br` for beads_rust) on `PATH`. Perles uses it for writes.
- golangci-lint v2 (for linting; `.golangci.yml` uses the v2 format)
- mockery v2 (for mock generation; `.mockery.yaml` uses the v2 format)

## Essential Commands

### Build & Run

```bash
make build          # Build frontend (npm) + Go binary with version info
make build-go       # Go-only build (embeds the committed frontend/dist/)
make build-frontend # Build the frontend only (cd frontend && npm install && npm run build)
make run            # Go-only build and run
make run-all        # Full build (frontend + Go) and run
make debug          # Go-only build and run with debug flag (-d)
make daemon         # Go-only build and run `perles daemon -p 19999`
make playground     # Go-only build and run the playground
make install        # Build frontend, then go install to $GOPATH/bin
make clean          # Clean build artifacts
make docs           # Serve the docs site locally (zensical, 127.0.0.1:8001)
make index-docs     # Rebuild the local qmd docs search index
make up / make down # Start / stop Jaeger (docker-compose) for local tracing
make jaeger         # Open the Jaeger UI (http://localhost:16686)

./perles            # Run the built binary
./perles -d         # Run with debug mode
./perles -c path    # Use specific config file
./perles -b path    # Specify beads directory
./perles -p 19999   # Fixed API server port (0 = auto-assign)
./perles --markdown-style light  # Markdown style: dark (default) or light
./perles init       # Create .perles/config.yaml in the current directory
./perles themes     # List theme presets
./perles playground # Run the UI component showcase / theme token viewer
./perles workflows  # List dashboard and chat panel workflow templates
./perles daemon -p N    # Run the control plane daemon (HTTP API; 0 = auto-assign port)
./perles update         # Self-update (--version vX.Y.Z for a specific release)
./perles registry:list  # List workflow registrations as JSON (-n NS, -l LABEL)
```

### Testing

```bash
make test           # Run all tests
make test-v         # Run tests with verbose output
make test-update    # Update golden test files for teatest snapshots

# Update specific golden tests (-update goes AFTER the package pattern: go test passes
# everything after an unknown flag to the test binary, so `go test -update ./pkg/...`
# only tests the package in the current directory)
go test ./internal/mode/search/... -update
go test ./internal/ui/board/... -update
go test ./internal/ui/shared/vimtextarea/... -update

# Run specific package tests
go test ./internal/beads/bql/...
go test ./internal/orchestration/v2/...
go test -v ./cmd/...
```

### Code Quality

```bash
make lint           # Run golangci-lint (configured in .golangci.yml)
make mocks          # Regenerate mocks using mockery (runs mocks-clean first)
make mocks-clean    # Remove internal/mocks/ (not the controlplane mocks)
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
│   ├── root.go            # Main command setup (config loading, TUI startup)
│   ├── backend.go         # Backend factory (beads, beads_rust)
│   ├── init.go            # Init command
│   ├── themes.go          # Theme commands
│   ├── workflows.go       # Workflow listing command
│   ├── registry_list.go   # registry:list command (registrations as JSON)
│   ├── daemon.go          # Control plane daemon + HTTP API
│   ├── update.go          # Self-update command
│   └── playground.go      # Playground command
├── internal/              # Internal packages (not exported)
│   ├── app/              # Root application model & orchestration
│   ├── task/             # Backend-agnostic issue DTOs & interfaces (Backend, TaskExecutor, QueryExecutor)
│   ├── beads/            # Beads backend (adapter, application, domain, infrastructure)
│   │   └── bql/          # Query language (lexer, parser, ast, validator, executor, sql)
│   ├── beadsrust/        # beads_rust backend adapter (reads beads.db, writes via `br`)
│   ├── registry/         # DDD-layered workflow registry
│   │   ├── application/  # Registry service facade (YAML loading, template rendering)
│   │   └── domain/       # Pure domain types (Registration, Chain, Node)
│   ├── templates/        # Embedded built-in registry workflows (template.yaml + shared v1-*.md)
│   ├── mode/             # Application modes with Controller interface
│   │   ├── kanban/       # Kanban board mode
│   │   ├── search/       # Search mode
│   │   ├── dashboard/    # Dashboard mode (multi-workflow orchestration management)
│   │   ├── playground/   # UI component showcase & theme token viewer
│   │   └── shared/       # Cross-mode utilities (Clipboard, Clock, user actions)
│   ├── cachemanager/     # Generic caching infrastructure
│   ├── git/              # Git executor for worktree operations (application, domain, infrastructure)
│   ├── orchestration/    # AI orchestration system
│   │   ├── client/       # Provider-agnostic headless AI client
│   │   │   └── providers/ # amp, claude, codex, cursor, gemini, opencode
│   │   ├── v2/           # V2 command processor & handlers
│   │   ├── controlplane/ # Multi-workflow control plane (api/ = HTTP API)
│   │   ├── mcp/          # Model Context Protocol server
│   │   ├── events/       # Process event types
│   │   ├── fabric/       # Inter-agent messaging (fabric_* MCP tools)
│   │   ├── workflow/     # Markdown workflow template registry (chat panel)
│   │   ├── session/      # Session tracking
│   │   ├── metrics/      # Token usage tracking
│   │   ├── message/      # Inter-agent message log
│   │   ├── tracing/      # OpenTelemetry tracing
│   │   ├── validation/   # Shared validation helpers (e.g. task IDs)
│   │   └── mock/         # State-based mock headless client/process for tests
│   ├── sessions/domain/  # Persisted session entity & repository interface
│   ├── infrastructure/   # SQLite storage (~/.perles/perles.db)
│   │   ├── sqlite/       # Connection lifecycle & repositories
│   │   └── migrations/   # Schema migrations
│   ├── frontend/         # HTTP handlers serving the embedded session viewer SPA
│   ├── presentation/     # DTOs & formatting for CLI output (registry:list)
│   ├── pubsub/           # Generic pub/sub event broker
│   ├── ui/               # UI components
│   │   ├── board/        # Kanban board view
│   │   ├── tree/         # Tree view for dependencies
│   │   ├── details/      # Issue details panel
│   │   ├── coleditor/    # Column editor UI
│   │   ├── commandpalette/ # Searchable picker modal component
│   │   ├── modals/       # Modal dialogs (help, issueeditor, commenteditor)
│   │   ├── nobeads/      # Empty state when no .beads directory
│   │   ├── outdated/     # Database version too old state
│   │   ├── embeddedmode/ # Shown when beads uses embedded Dolt (unsupported)
│   │   ├── serverdown/   # Shown when the Dolt server is unreachable
│   │   ├── shared/       # Reusable UI components
│   │   │   ├── chatpanel/   # AI chat panel (kanban/search, ctrl+w)
│   │   │   └── vimtextarea/ # Vim-like textarea widget
│   │   └── styles/       # Theme system
│   ├── config/           # Configuration management
│   ├── flags/            # Feature flags
│   ├── sound/            # Audio feedback for orchestration events
│   ├── paths/            # Path resolution (.beads dir, worktree redirects)
│   ├── log/              # Debug logging
│   ├── watcher/          # File system watching
│   ├── keys/             # Keyboard shortcut definitions
│   ├── mocks/            # Generated mockery mocks
│   └── testutil/         # Test utilities and builders
├── communityworkflows/   # Embedded community workflows (opt-in via orchestration.community_workflows)
├── frontend/             # React/Vite session viewer (dist/ is embedded in the binary)
├── docs/                 # Zensical documentation site (docs/assets/ holds screenshots and videos)
├── examples/             # Example configurations
│   ├── themes/          # Theme examples
│   └── user-actions.yaml # User action examples
├── main.go              # Entry point
├── Makefile             # Build automation
├── go.mod               # Go module dependencies
├── zensical.toml        # Docs site configuration
├── docker-compose.yml   # Local Jaeger for tracing (make up / make down)
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
- **Messages:** Descriptive with `Msg` suffix (`ShowToastMsg`, `SaveSearchAsColumnMsg`)
- **Constants:** PascalCase for enums (`FocusSearch`, `ModeKanban`, `ModeDashboard`)
- **Interfaces:** Small, focused, verb-er naming (`Controller`, `Builder`)

### Import Organization

```go
import (
    // Standard library
    "fmt"
    "os"
    
    // External dependencies
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    
    // Internal packages (module path github.com/zjrosen/perles)
    "github.com/zjrosen/perles/internal/app"
    "github.com/zjrosen/perles/internal/beads/domain"
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

- **Tool**: [mockery](https://github.com/vektra/mockery) v2 with `.mockery.yaml` config
- **Output**: `internal/mocks/` directory, except the controlplane mocks, which go to
  `internal/orchestration/controlplane/mocks/` (`make mocks-clean` only removes `internal/mocks/`)
- **Command**: `make mocks` to regenerate
- **Interfaces mocked** (see `.mockery.yaml`): `Backend`, `TaskExecutor`, `QueryExecutor`, `QueryHelpers`
  (`internal/task`), `CacheManager`, `Clock`, `Clipboard`, `AgentProvider`, `HeadlessClient`,
  `HeadlessProcess`, `GitExecutor` (`internal/git/application`), `SoundService`; plus `ControlPlane` and
  `HealthMonitor` in the controlplane mocks package
- **Hand-written mocks**: `internal/orchestration/mock` provides state-based `HeadlessClient`/`HeadlessProcess`
  fakes for orchestration tests

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
# Update all golden files (every package with golden tests, including formmodal and panes)
make test-update

# Update specific package (-update must come after the package pattern)
go test ./internal/mode/search/... -update

# formmodal also accepts UPDATE_GOLDEN=1 as an alternative to -update
UPDATE_GOLDEN=1 go test ./internal/ui/shared/formmodal/...
```

New golden-test packages must be added to the `test-update` list in the `Makefile`.
`internal/orchestration/client/providers/opencode/testdata/*.golden` are static fixtures that no
test reads, so `make test-update` leaves them alone.

## Architecture Patterns

### MVC-like Structure

- **Models:** Hold state and business logic
- **Update:** Handle events and state mutations (Bubble Tea pattern)
- **View:** Render UI from model state
- Components implement `tea.Model` interface

### Message Passing

Components communicate via Bubble Tea messages (e.g. `internal/mode/search/search.go`):

```go
// searchResultsMsg carries the results of a BQL query.
type searchResultsMsg struct {
    issues []task.Issue
    err    error
}
```

### Service Injection

Shared dependencies passed via `Services` struct (`internal/mode/mode.go`):

```go
type Services struct {
    // Backend-agnostic interfaces (internal/task)
    TaskExecutor      task.TaskExecutor
    QueryExecutor     task.QueryExecutor
    QueryHelpers      task.QueryHelpers        // nil if the backend has no structured query language
    SyntaxHighlighter task.SyntaxHighlighter   // nil if the backend has no query syntax highlighting
    Capabilities      task.BackendCapabilities // what the current backend supports

    Config             *config.Config
    ConfigPath         string
    DBPath             string
    WorkDir            string
    Clipboard          shared.Clipboard
    Clock              shared.Clock
    Flags              *flags.Registry
    Sounds             sound.SoundService
    GitExecutorFactory func(path string) appgit.GitExecutor
    SessionRepository  domain.SessionRepository // nil unless session persistence is enabled
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
    ModeDashboard
)
```

### Hybrid Architecture: DDD for Registry, Beads, Git, Sessions, Fabric

The codebase uses a hybrid architecture. UI and mode code (`internal/app`, `internal/mode`, `internal/ui`)
follows a flat-package pattern, while several subsystems use Domain-Driven Design (DDD) layering
(`domain/` for pure types, `application/` for ports and services, `infrastructure/` for I/O). Mode code
imports these layers directly (e.g. `mode.Services` holds an `appgit.GitExecutor` factory and a sessions
`domain.SessionRepository`).

| Package | Layers |
|---------|--------|
| `internal/registry/` | `domain/` (Registration, Chain, Node, DAG algorithms), `application/` (RegistryService) |
| `internal/beads/` | `domain/` (issue types, version checks), `application/` (ports such as `DBClient`), `infrastructure/` (SQLite/Dolt clients, `bd` executor), `adapter/` (implements the `internal/task` interfaces); `bql/` sits alongside |
| `internal/git/` | `domain/` (types, errors), `application/` (`GitExecutor` port), `infrastructure/` (git CLI executor) |
| `internal/sessions/domain/` | Persisted session entity and `SessionRepository` interface; SQLite implementation in `internal/infrastructure/sqlite/` |
| `internal/orchestration/fabric/` | `domain/` (core messaging types), with `repository/` (storage) and `persistence/` (JSONL event log) |

**Why DDD for the Registry:**
- Registry has complex domain logic (DAG topological sort, cycle detection)
- Clear separation needed between domain types and I/O concerns (YAML, templates)
- Enables thorough unit testing of domain logic without mocks
- Portable from sesh codebase where DDD patterns originated

**Registry Layer Responsibilities:**
- `internal/registry/domain/`: Pure Go with stdlib-only imports. Contains Registration, Chain, Node types and domain algorithms. No file I/O.
- `internal/registry/application/`: Service facade bridging domain to infrastructure. Handles YAML loading (built-in, community and user workflows), template rendering (`RenderTemplate`/`RenderEpicTemplate`), embed.FS access, and `GetSystemPromptTemplate()`. Lookups: `List`, `GetByNamespace`, `GetByKey`, `GetByLabels`, `GetTemplate`.

**Two Registry Systems:**
Perles has two distinct registry systems:
1. `internal/registry/domain.Registry` - DDD registration registry (namespace+key lookups, label filtering, DAG chains; identifiers `namespace::key::version::chain-key`). Backs the dashboard's New Workflow dialog (namespace `workflow`), the Dashboard Workflows section of `perles workflows`, and `perles registry:list`.
2. `internal/orchestration/workflow.Registry` - Markdown workflow template registry (ID-based lookups, categories, sources, target modes). Backs the chat panel's Workflows tab and the Chat Panel Workflows section of `perles workflows`.

**Import Aliasing Convention:**
When importing the registry application layer:

```go
import (
    appreg "github.com/zjrosen/perles/internal/registry/application"
    "github.com/zjrosen/perles/internal/templates"
)

// templates.RegistryFS() contains:
// - workflows/<name>/template.yaml (workflow definitions)
// - workflows/<name>/*.md (workflow-specific templates)
// - workflows/v1-epic-instructions.md, v1-human-review.md (shared templates)
svc, err := appreg.NewRegistryService(
    templates.RegistryFS(),
    communitySource,              // *appreg.CommunitySource, or nil for no community workflows
    appreg.UserRegistryBaseDir(), // ~/.perles (user workflows in workflows/<name>/template.yaml)
)

// Use workflow.Registry for chat panel markdown workflows
```

The TUI, `perles workflows` and `perles registry:list` all build their service with
`newRegistryService(cfg.Orchestration)` in `cmd/root.go`, which wires in the community workflows
enabled in `orchestration.community_workflows`.

**Key Interfaces:**
- `RegistryProvider`: Read-only interface for `Registry` (enables mock injection in tests)

## AI Orchestration System

Perles includes a sophisticated multi-agent AI orchestration layer for coordinating AI-powered development workflows.

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│      Dashboard Mode (TUI)  /  perles daemon (HTTP API)      │
│  TUI: workflow list, epic tree/details, coordinator panel   │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                        Control Plane                        │
│  Registry (in-memory or durable), supervisor,               │
│  cross-workflow event bus, health monitor, recovery         │
└─────────────────────────────────────────────────────────────┘
                            │  one v2.Infrastructure per workflow
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    V2 Command Processor                     │
│  CQRS-style: Commands mutate state, Queries bypass          │
├─────────────────────────────────────────────────────────────┤
│  Handlers: SpawnProcess, AssignTask, SendToProcess, etc.    │
│  Repositories: ProcessRepo, TaskRepo, QueueRepo (in-memory) │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                     Headless AI Clients                     │
│          (orchestration/client/providers/<name>/)           │
├─────────────────────────────────────────────────────────────┤
│      claude · amp · codex · gemini · opencode · cursor      │
└─────────────────────────────────────────────────────────────┘
```

The Kanban/Search chat panel (`ctrl+w`) bypasses the control plane: it creates its own single-process
`v2.NewSimpleInfrastructure` (no MCP server, workers or tasks).

### Key Components

| Package | Purpose |
|---------|---------|
| `orchestration/client/` | Provider-agnostic `HeadlessClient` and `HeadlessProcess` interfaces |
| `orchestration/client/providers/claude/` | Claude Code CLI integration |
| `orchestration/client/providers/amp/` | Amp CLI integration |
| `orchestration/client/providers/codex/` | OpenAI Codex CLI integration |
| `orchestration/client/providers/gemini/` | Gemini CLI integration |
| `orchestration/client/providers/opencode/` | OpenCode CLI integration (default model `anthropic/claude-opus-5-5`; MCP config via the `OPENCODE_CONFIG_CONTENT` env var) |
| `orchestration/client/providers/cursor/` | Cursor Agent CLI integration (MCP config written to `{workDir}/.cursor/mcp.json`; see [docs/CURSOR_AGENT.md](docs/CURSOR_AGENT.md)) |
| `orchestration/v2/` | Command processor, handlers, adapters, repositories |
| `orchestration/controlplane/` | Multi-workflow management (registry/durable registry, supervisor, event bus, health monitor, recovery, HTTP API in `api/`) |
| `orchestration/mcp/` | MCP servers exposing coordinator, worker and observer tools |
| `orchestration/fabric/` | Fabric channel/thread messaging (backs the `fabric_*` MCP tools) |
| `orchestration/events/` | Unified `ProcessEvent` type |
| `orchestration/workflow/` | Markdown workflow template registry for the chat panel (built-in + user-defined) |
| `orchestration/session/` | On-disk session logging |
| `orchestration/metrics/` | Token usage and cost tracking |
| `orchestration/tracing/` | OpenTelemetry tracing (enabled via `orchestration.tracing.enabled`) |
| `orchestration/validation/` | Shared validation helpers (e.g. task IDs) |

### Process Events

All orchestration events use the unified `ProcessEvent` type:

```go
type ProcessEvent struct {
    Type      ProcessEventType  // ProcessSpawned, ProcessOutput, ProcessStatusChange, etc.
    ProcessID string
    Role      ProcessRole       // RoleCoordinator, RoleWorker, or RoleObserver
    Status    ProcessStatus     // Pending, Starting, Ready, Working, Paused, Stopped, Retiring, Retired, Failed
    Phase     *ProcessPhase     // nil for coordinator; Idle, Implementing, AwaitingReview, Reviewing, AddressingFeedback, Committing
    TaskID    string
    Output    string
    Metrics   *metrics.TokenMetrics
    // ...
}

// Filter by role
if event.IsCoordinator() { ... }
if event.IsWorker() { ... }
if event.IsObserver() { ... }
```

### MCP Tools

Tools exposed to the AI coordinator via MCP (`orchestration/mcp/coordinator.go`):

- Workers: `spawn_worker`, `assign_task`, `replace_worker`, `retire_worker`, `stop_worker`, `query_worker_state`
- Review and commit: `assign_task_review`, `assign_review_feedback`, `approve_commit`
- Tasks: `get_task_status`, `mark_task_complete`, `mark_task_failed`
- Workflow: `generate_accountability_summary`, `signal_workflow_complete`, `notify_user`
- Fabric: `fabric_inbox`, `fabric_send`, `fabric_reply`, `fabric_ack`, `fabric_subscribe`,
  `fabric_unsubscribe`, `fabric_attach`, `fabric_history`, `fabric_read_thread`, `fabric_react`

Workers (`orchestration/mcp/worker.go`) get `report_implementation_complete`, `report_review_verdict`,
`post_accountability_summary`, and the same fabric tools plus the worker-only `fabric_join`.

The observer (`orchestration/mcp/observer.go`) gets `fabric_inbox`, `fabric_history`, `fabric_read_thread`,
`fabric_subscribe`, `fabric_ack`, `fabric_attach` and `fabric_react`, plus `fabric_send` restricted to
`#observer` and `fabric_reply` restricted to `#observer` threads.

### Workflow Templates

There are two kinds of workflow templates:

- **Dashboard workflows** (multi-agent DAG templates in registry namespace `workflow`, shown in the
  dashboard's New Workflow dialog):
  - Built-in: `internal/templates/workflows/<name>/template.yaml` (cook, debate, mediated-investigation,
    quick-plan, research-proposal, research-to-tasks; `golang-guidelines`, key `golang`, is in the
    `lang-guidelines` namespace instead), plus the shared `v1-epic-instructions.md` and `v1-human-review.md`
  - Community: `communityworkflows/workflows/` (opt-in via `orchestration.community_workflows`)
  - User: `~/.perles/workflows/<name>/template.yaml`. Bare template filenames resolve in the workflow's
    directory, then `~/.perles/workflows/`, then the built-in shared templates, so shared templates
    don't need to be copied. An invalid user workflow is skipped with a `skipping invalid workflow`
    warning in the debug log.
- **Chat panel workflows** (markdown, shown in the chat panel's Workflows tab): built-in
  `internal/orchestration/workflow/templates/chat_*.md` and user `~/.perles/workflows/*.md` files whose
  `target_mode` is `chat` or omitted. The other built-in `.md` files there (`target_mode: orchestration`)
  are loaded by `workflow.Registry` but not shown anywhere.

List available: `perles workflows` (both kinds) or `perles registry:list` (all registrations as JSON;
`-n NAMESPACE`, repeatable `-l LABEL`). See
[docs/orchestration/custom-workflows.md](docs/orchestration/custom-workflows.md).

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
        │       ├── observer/
        │       └── messages.jsonl
        └── 2026-01-11/
            └── e5f6g7h8-9012-uuid/
```

**Application name derivation** (`session.Factory.ResolveApplicationName`, used by both `perles` and `perles daemon`):
1. Config override (`orchestration.session_storage.application_name`, surrounding whitespace trimmed)
2. Git remote `origin` URL (repository name without `.git`; TUI only, the daemon has no git executor)
3. Working directory name (fallback)

**Configuration:**
```yaml
orchestration:
  session_storage:
    base_dir: ~/.perles/sessions        # Default; a leading ~ is expanded, and the result must be absolute
    application_name: my-custom-name    # Optional: override derived name
```

**SQLite persistence (opt-in):** with `session-persistence: true` under `flags:` (default off), the
TUI also stores workflow session state (state, worktree, ownership PIDs, metrics) in SQLite at
`~/.perles/perles.db` through the control plane's `DurableRegistry`, so dashboard workflows survive
restarts and can be resumed (ownership is reclaimed from dead processes). Rows are keyed by the
resolved application name, so changing `application_name` hides workflows persisted under the old
name. The `session_dir` column points at the session's directory under `base_dir`. Without the flag,
and always in `perles daemon`, an in-memory registry is used.

**Benefits:**
- **Single location**: All sessions in one place for easy backup
- **Date-based partitioning**: Easy cleanup of old sessions
- **Cross-project visibility**: The web session viewer served by the HTTP API lists sessions across
  applications (it reads the configured `base_dir`, default `~/.perles/sessions/`)
- **Survives project deletion**: Session history preserved independently
- **No project pollution**: Session data lives outside the project (note: `.perles/config.yaml` is
  created in the project by `perles init`, or by the first bare `perles` run when no config file
  exists anywhere)

## BQL (Beads Query Language)

### Fields

`id`, `type`, `status`, `priority`, `title`, `description`, `design`, `notes`,
`created`, `updated`, `defer_until`, `blocked`, `ready`, `pinned`, `is_template`, `label`,
`assignee`, `sender`, `created_by`, `mol_type`, `metadata.$key`

The `beads_rust` backend also accepts `owner` but not `mol_type`; `metadata.*` queries fail there
because its issues table has no metadata column.

### Operators

- **Comparison:** `=`, `!=`, `<`, `>`, `<=`, `>=`
- **String:** `~` (contains), `!~` (not contains)
- **Logical:** `and`, `or`, `not`, parentheses
- **Set:** `in (values)`, `not in (values)`

### Value Types

- **Strings:** `"quoted"` or `unquoted`
- **Priorities:** `P0`–`P4` (or `0`–`4`)
- **Booleans:** `true`, `false`
- **Dates:** `now`, `today`, `yesterday`, `-Nd` (e.g. `-7d`), `-Nh` (e.g. `-24h`), `-Nm` (months, e.g. `-3m`)
- **Null:** `nil` (for metadata existence checks)

### Special Clauses

- **EXPAND:** `expand up|down|all [depth N|*]` - traverse relationships
- **ORDER BY:** `order by field [asc|desc]` - sort results
- **Metadata:** `metadata.$key = "value"` - filter by issue metadata (`=`, `!=`, `~`, `!~` only).
  Values compare as text and must be strings or `nil`, so quote numbers and booleans
  (`metadata.points = "5"`, `metadata.flaky = "true"`). `=`/`!=` are case-sensitive, `~`/`!~` are
  case-insensitive substring matches, and `!=`/`!~` only match issues that have the key.
- **Metadata exists:** `metadata.$key != nil` - key exists (even if its value is JSON null), `metadata.$key = nil` - key does not exist

### Example Queries

```sql
type = bug and priority = P0
status != closed and ready = true
status = deferred or status = open and defer_until > now
title ~ "auth" and label in (security, urgent)
created > -7d order by priority asc
type = epic expand down depth 2
metadata.team = "backend"
metadata.sprint = nil
type = task and metadata.component ~ auth
```

## Configuration

### Config File Location

Lookup order (first found wins):

1. `--config`/`-c` path
2. `./.perles/config.yaml` (current directory)
3. `~/.config/perles/config.yaml`

`perles init` writes the default template (`config.DefaultConfigTemplate()`) to `./.perles/config.yaml`.
It refuses with `config file already exists: .perles/config.yaml` only when that file exists; a home
config does not block it. A bare `perles` run writes the same file when no config exists anywhere, but
only after the beads backend has opened and passed its version check, so the no-beads, embedded-mode,
server-down and outdated screens never leave a file behind. Other subcommands never create a config.
A `-c` path that does not exist is not created at startup (perles runs on built-in defaults), but
saving a view or column later writes to that path.

Keys omitted from the file fall back to `config.Defaults()`, registered as viper defaults by
`setConfigDefaults()` in `cmd/root.go` (`TestInitConfig_DefaultsMatchConfigDefaults` keeps the two in
sync, so a new key with a non-zero default needs both). A leading `~` (`~` or `~/...`) is expanded in
`beads_dir`, `orchestration.session_storage.base_dir`, `orchestration.tracing.file_path` and
`sound.events.*.override_sounds` right after loading and before validation (`Config.ExpandPaths()`);
`~user` and `$VARS` are not expanded.

Full reference: [docs/configuration/index.md](docs/configuration/index.md).

### Config Structure

```yaml
backend: beads                       # beads (default) or beads_rust
beads_dir: .beads                    # Project or .beads directory (priority: -b > BEADS_DIR > beads_dir > cwd)

ui:
  show_counts: true                  # Show issue counts
  show_status_bar: true              # Show status bar
  markdown_style: dark               # Markdown rendering style: dark (default) or light
  vim_mode: false                    # Vim keybindings in text inputs
  keybindings:                       # Mode-switch key overrides
    search: ctrl+space
    dashboard: ctrl+o

theme:
  preset: "catppuccin-mocha"         # Built-in theme (list with `perles themes`)
  colors:                            # Custom token overrides
    "text.primary": "#E0E0E0"
  # mode: is still accepted but ignored; use a light preset such as catppuccin-latte

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
        tree_mode: "deps"            # deps (default) or child

orchestration:                       # AI orchestration settings
  coordinator_client: claude         # claude (default), amp, codex, gemini, opencode, or cursor
  worker_client: claude              # same options
  # client: amp                      # Legacy fallback for both roles when the two keys above are unset
  observer_enabled: false            # Add a passive observer agent to workflows (observer_client defaults to claude)
  session_storage:                   # Centralized session storage
    base_dir: ~/.perles/sessions     # Default; ~ is expanded and the result must be absolute
    application_name: ""             # Override: defaults to git repo name, then directory name
  tracing:
    enabled: false                   # OpenTelemetry tracing for dashboard/daemon workflows
    exporter: file                   # file (default, ~/.config/perles/traces/traces.jsonl), otlp, none, or stdout (garbles the TUI; daemon only)

sound:                               # Top-level block (not under orchestration)
  events:                            # All six events are enabled by default
    workflow_complete:
      enabled: true
      override_sounds: ["~/.perles/sounds/done.wav"]  # .wav files under ~/.perles/sounds/, <= 1MB each

flags:
  session-persistence: false         # Persist dashboard workflows in ~/.perles/perles.db
```

The client resolvers (`CoordinatorClientType()`/`WorkerClientType()`) use the role-specific key, then
`client`, then `claude`; neither key has a viper default. The template from `perles init` sets both
role keys explicitly, so `client` only applies once they are removed. The default sound events are
`workflow_complete`, `review_verdict_approve`, `review_verdict_deny`, `worker_out_of_context`,
`coordinator_out_of_context` and `user_notification`; `observer_out_of_context` also plays unless set to
`enabled: false`. Once a config lists `sound.events`, viper uses that map instead of the default one, so
a listed event without `enabled: true` is silenced (unlisted events still play). `user_notification`
has no built-in sound (there is no embedded `notification.wav`), so it is silent unless
`override_sounds` is set. An invalid override path (not `.wav`, missing, outside
`~/.perles/sounds/` after resolving symlinks, or over 1MB) stops the TUI at startup with
`invalid sound configuration: ...`.

### User-Defined Actions

User actions allow you to define custom keybindings that execute shell commands with dynamic issue context. Actions work in **any mode where an issue is selected**:

- **Kanban mode** - issue selected in a column
- **Search mode** - issue selected in the results list (results pane focused)
- **Search tree sub-mode** - issue selected in the dependency tree

**Note:** User actions are NOT available in dashboard mode.

Actions are **fire-and-forget** - the rendered command runs with `sh -c` in the perles working directory, perles continues immediately without waiting for completion, and the command's stdout/stderr are discarded. This is ideal for launching external tools like AI assistants in new terminal panes.

**Important:** Config changes require restarting perles to take effect.

```yaml
ui:
  actions:
    issue_action:
      open-claude:
        key: "1"                    # Required: keybinding to trigger this action (0-9 only)
        command: 'tmux split-window -h claude "Work on {{.ID}}: "{{.TitleText}}'  # Required
        description: "Open Claude"  # Optional: shown in help overlay (?)
```

See [examples/user-actions.yaml](examples/user-actions.yaml) for more examples.

#### Template Variables

| Variable | Description | Escaped |
|----------|-------------|---------|
| `{{.ID}}` | Issue ID (e.g., "PROJ-123"), inserted as-is | No |
| `{{.Title}}` | Issue title, raw | No |
| `{{.TitleText}}` | Issue title as one single-quoted shell word (`Don't panic` renders as `'Don'\''t panic'`, an empty title as `''`) | Yes (POSIX single-quote) |

Any other field (e.g. `{{.Unknown}}`) fails with a "template rendering failed" error and the command does not run.

#### Allowed Keys (0-9 Only)

User actions are restricted to numeric keys only: `0`, `1`, `2`, `3`, `4`, `5`, `6`, `7`, `8`, `9`

This limitation ensures no conflicts with built-in keybindings across all modes.

#### Security Model

- **User configs are trusted** - Commands execute with user's shell permissions
- **TitleText is shell-escaped** - POSIX single-quote escaping (the value is wrapped in `'...'` and each embedded `'` becomes `'\''`), so `$(...)`, backticks, `$VARS`, `;`, `|` and newlines in a title are not interpreted. Use it bare: wrapping it in `'...'` or `"..."` undoes the escaping. To add fixed text, quote the text separately, directly next to it: `"Work on {{.ID}}: "{{.TitleText}}` is one argument
- **Title is NOT escaped** - `{{.Title}}` is inserted raw, so quotes or shell metacharacters in a title can break or inject into the command; prefer `{{.TitleText}}`
- **ID is not escaped** - Issue IDs are constrained identifiers, safe for direct use
- **tmux** - Pass the pane command to `split-window`/`new-window` as separate arguments (as in the example) so tmux execs it directly. A pane command given as one quoted string goes through `sh -c` a second time, and `{{.TitleText}}` is only escaped for one shell level, so keep it out of such strings

## Environment Variables

- `PERLES_DEBUG`: Enable debug mode (any non-empty value; same as `-d`)
- `PERLES_LOG`: Debug log file path (default: `debug.log`)
- `BEADS_DIR`: Beads directory (priority: `-b` > `BEADS_DIR` > `beads_dir` config > cwd; `perles daemon` has no `-b`)
- `BEADS_DOLT_SERVER_MODE=1` / `BEADS_DOLT_SHARED_SERVER` (`1` or `true`): Force server mode for a Dolt backend (`"backend": "dolt"`) regardless of `dolt_mode` in `.beads/metadata.json`
- `BEADS_DOLT_SERVER_HOST`, `BEADS_DOLT_SERVER_PORT`, `BEADS_DOLT_SERVER_USER`, `BEADS_DOLT_PASSWORD`: Dolt server connection overrides
- `VISUAL` / `EDITOR`: External editor opened with `ctrl+g` in text inputs (falls back to `vi`)
- `BROWSER`: Command used to open the session viewer (dashboard `o`)
- `GEMINI_API_KEY` / `GOOGLE_API_KEY`: Gemini auth when there is no `~/.gemini` OAuth token
- `TMUX`, `SSH_TTY` / `SSH_CLIENT` / `SSH_CONNECTION`, `STY`: Clipboard detection (OSC 52 over SSH and in GNU screen, native tools otherwise)
- `UPDATE_GOLDEN=1`: Updates formmodal golden files only (`internal/ui/shared/formmodal`); other golden tests use `-update` / `make test-update`

## Keyboard Shortcuts

### Kanban Mode

- **Navigation:** `h/j/k/l` or arrow keys
- **Column management:** `a` (add), `e` (edit), `d` (delete), `Ctrl+h/l` (move), `m` (toggle deps/child on a tree column)
- **View management:** `Ctrl+j`/`Ctrl+n` (next), `Ctrl+k`/`Ctrl+p` (previous), `Ctrl+v` (menu)
- **Actions:** `Enter` (open tree view), `n` (new issue), `ctrl+e` (edit issue: status, priority, etc.), `ctrl+d` (delete issue), `y` (copy ID), `ctrl+g` (git diff)
- **Mode switch:** `Ctrl+Space` (search), `/` (search column), `Ctrl+O` (dashboard); `Ctrl+Space` and `Ctrl+O` are configurable via `ui.keybindings`
- **General:** `?` (help), `q` (quit), `r` (refresh), `w` (toggle status bar)

### Search Mode (Supports regular search and tree sub-mode)

- **Navigation:** `j/k` (move), `h/l` (focus panes), `Tab`/`Ctrl+n` and `Ctrl+p` (cycle focus)
- **Search:** `/` (focus input), `Enter` (execute), `esc` (back to kanban)
- **Actions:** `Enter` (open tree view), `ctrl+e` (edit issue: status, priority, etc.), `ctrl+d` (delete issue), `c` (add comment, details pane), `y` (copy ID), `ctrl+g` (git diff)
- **Tree sub-mode:** `Enter` (refocus on node), `u` (previous root), `U` (original root), `d` (toggle direction), `m` (toggle deps/children)
- **Save:** `Ctrl+s` (save to view)
- **General:** `Ctrl+Space` (kanban), `?` (help), `q` (quit)

### Chat Panel (Kanban and Search)

- **Toggle:** `ctrl+w` (open/close; in the dashboard `ctrl+w` is the coordinator panel), `Tab` (switch chat/board focus)
- **Tabs:** `ctrl+j`/`ctrl+k` or `ctrl+]` (cycle Chat, Sessions, Workflows), `ctrl+t` (jump to Workflows)
- **Sessions:** `ctrl+n`/`ctrl+p` (cycle sessions); in the Sessions tab `enter` (switch or create), `d` twice (retire)
- **Workflows tab:** `j/k` (move), `enter` (send the selected workflow and return to Chat)

### Dashboard Mode (Multi-Workflow Management)

Orchestration runs from the dashboard; there is no separate orchestration mode.

- **Navigation:** `j/k` or `↓/↑` (move between workflows), `g` (first), `G` (last)
- **Focus Cycling:** `Tab` or `Ctrl+n` (next zone), `Shift+Tab` or `Ctrl+p` (prev zone); click a pane's `[+]`/`[-]` to maximize/restore it
- **Filter:** `/` (activate filter), `esc` (clear filter; back to kanban when no filter is set)
- **Actions:** `s` (start/resume), `x` (pause), `enter` (focus coordinator), `r` (rename), `o` (open session in browser), `a` (archive; requires the `session-persistence` flag)
- **Create:** `n` (new workflow)
- **Coordinator:** `ctrl+w` (toggle panel), `ctrl+k` / `ctrl+j` (previous/next tab), `Tab` (cycle message channel when the panel is focused), `ctrl+t` (thread picker, channels only), `@` (mention a process)
- **Slash commands** (coordinator input): `/stop <process-id> [--force]`, `/spawn`, `/retire <worker-id> [reason]`, `/replace <process-id> [reason]`
- **General:** `?` (help), `q` (back to kanban)

#### Epic Tree View

- **Visibility:** Shown below the workflow table whenever there is room (empty state if the workflow has no epic); focus it with `Tab`/`Shift+Tab`
- **Navigation:** `j/k` or `↓/↑` (move in tree when tree focused; `j/k/g/G` scroll when details focused)
- **Pane switch:** `h/l` (tree ↔ details)
- **Tree controls:** `enter` (refocus on selected issue), `d` (toggle down/up direction), `m` (toggle deps/children mode)
- **Actions:** `ctrl+e` (edit issue), `c` (add comment, details pane), `y` (copy ID in tree, description in details)

## Important Gotchas

### Database Requirements

- Requires a `.beads/` directory: beads (>= v0.62.0) backed by SQLite (`beads.db`; the default when
  `.beads/metadata.json` is absent) or a Dolt SQL server (`"backend": "dolt"` with `"dolt_mode": "server"`
  in `.beads/metadata.json`, or `BEADS_DOLT_SERVER_MODE=1`), or a beads_rust project with
  `backend: beads_rust` in the perles config
- Embedded Dolt, an unreachable Dolt server and an older beads version each show a dedicated screen
  instead of the board
- Auto-refresh watches `beads.db`/`beads.db-wal` (SQLite and beads_rust) or `.beads/last-touched` (Dolt server)

### Terminal Compatibility

- Requires terminal with 256 color support
- Mouse support optional but enhances UX
- Minimum recommended size: 80x24

### Golden Test Management

- Golden tests capture exact terminal output
- Update with `make test-update` or `go test ./pkg/... -update` (flag after the package pattern) when UI changes
- Review diffs carefully before committing

### Theme System

- Themes use semantic color tokens
- Custom themes override specific tokens via `theme.colors` (e.g. `type.bug` colors bug type badges;
  `border.highlight` is the focused-element color, and a user-set `border.focus` is an alias for it when
  `border.highlight` is not set)
- Presets: default, catppuccin-mocha, catppuccin-latte, dracula, nord, high-contrast, gruvbox (list with `perles themes`)
- `theme.mode` is accepted but ignored; use a light preset such as catppuccin-latte for light terminals
- An invalid theme (unknown preset or token, bad hex value) ignores the whole `theme` section: perles
  uses the default theme and shows a "Theme config ignored: ..." warning toast at startup

### Column Types

- **BQL columns:** Filter issues with queries
- **Tree columns:** Show dependency trees or child hierarchies (`tree_mode: deps` or `child`; the older
  `children` is also accepted). They always show the down direction.
- Mixed column types supported in same view

### Orchestration Safety

- Dashboard workflows can create git worktrees; a worktree perles created is removed only if workflow
  startup fails (user-owned worktrees are never removed)
- There is no exit prompt: stopping a workflow (including the automatic stop on app exit) is refused
  with `ErrUncommittedChanges` while its worktree has uncommitted changes, unless forced, so the
  worktree is left in place

## CI/CD

### GitHub Actions Workflow

- **Triggers:** Push to main, pull requests to main (`.github/workflows/ci.yml`)
- **Platforms:** Ubuntu (build, test, lint), macOS and Windows (`windows-2022`) (build, test)
- **Steps:** Node.js + Go setup → Build (`make build`, includes the frontend) → Test (`make test`); Lint on Ubuntu only
- **Go version:** 1.27
- **Linter:** golangci-lint (latest)
- **Docs:** `.github/workflows/docs.yml` builds the zensical docs site and deploys it to GitHub Pages on push to main

### Release Process

- Triggered by pushing a `v*` tag (`.github/workflows/release.yml`): builds the frontend, runs `make test`, then goreleaser
- Uses goreleaser (`.goreleaser.yml`) for releases
- Binaries built for linux/darwin, amd64/arm64
- Publishes a Homebrew formula to `zjrosen/homebrew-perles`
- Version extracted from git tags
- Install script at `install.sh`

## Development Tips

### Debugging

```bash
./perles -d                  # Enable debug mode (ctrl+x toggles the in-app log overlay)
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
4. Update help text in modals (`internal/ui/modals/help/`)
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

1. Add the token in `internal/beads/bql/token.go` and lex it in `lexer.go`
2. Update `internal/beads/bql/parser.go`
3. Generate SQL in `internal/beads/bql/sql.go` for both dialects (SQLite and MySQL/Dolt); wire into `executor.go` if needed
4. Add validation in `internal/beads/bql/validator.go` (beads_rust validates against its own field set in `internal/beadsrust/bql_executor.go`)
5. Update BQL documentation

### Creating a New Mode

1. Create package under `internal/mode/`
2. Implement `Controller` interface
3. Add an `AppMode` constant in `internal/mode/mode.go` and wire mode switching in `internal/app/`
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
- `ProcessWorkflowComplete` - Workflow completed
- `ProcessAutoRefreshRequired` - Coordinator context exhausted; refresh needed (TUI notification only)
- `ProcessUserNotification` - Coordinator requests user attention (e.g. a human checkpoint)

Filter by `Role` field: `events.RoleCoordinator`, `events.RoleWorker`, or `events.RoleObserver`
(helpers: `IsCoordinator()`, `IsWorker()`, `IsObserver()`)

### Subscribing to Events

Events are delivered via pub/sub brokers. Subscribe with a context for automatic cleanup:

```go
import (
    "context"
    "github.com/zjrosen/perles/internal/orchestration/events"
    "github.com/zjrosen/perles/internal/pubsub"
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
    "github.com/zjrosen/perles/internal/orchestration/events"
    "github.com/zjrosen/perles/internal/pubsub"
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
| `github.com/go-sql-driver/mysql` | Dolt server (MySQL protocol) driver |
| `github.com/golang-migrate/migrate/v4` | Schema migrations for `~/.perles/perles.db` |
| `github.com/lrstanley/bubblezone` | Mouse click zones |
| `go.opentelemetry.io/otel` | Orchestration tracing |
| `gopkg.in/yaml.v3` | Config saves (comment-preserving `yaml.Node` edits) and workflow YAML |
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
