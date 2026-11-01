# Contributing to Perles

Thank you for your interest in contributing to Perles! This document provides guidelines for contributing to the project.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/perles.git`
3. Create a branch: `git checkout -b feature/your-feature`
4. Make your changes
5. Run tests and lint: `make test` and `make lint`
6. Commit with a descriptive message
7. Push and create a Pull Request

## Development Setup

### Prerequisites

- Go 1.27+
- Node.js and npm, used by `make build` and `make install` to build the embedded web frontend (release builds use the version in `.nvmrc`; CI uses the current LTS). `make build-go` skips this step and uses the committed `frontend/dist/`.
- golangci-lint v2 for `make lint` (CI runs the latest version)
- A beads or beads_rust project for testing (a directory containing `.beads/`), with the `bd` (or `br`) CLI on your `PATH`. beads projects need beads v1.0.0+. Embedded Dolt (the `bd init` default) works with bd v1.0.5+, and perles reads it through `bd`; Dolt server mode (`bd init --server`, then start the server with `bd dolt start`) is the other option. Select beads_rust with `backend: beads_rust` in the config. See the [README requirements](README.md#requirements) for details.

### Building

```bash
make build        # Build the frontend (npm) and the binary
make build-go     # Go-only build (uses the committed frontend/dist/)
make test         # Run tests
make test-v       # Verbose tests
make test-update  # Update golden files
make lint         # Run golangci-lint (CI fails on lint errors)
make mocks        # Regenerate mocks in internal/mocks/ (requires mockery)
make docs         # Preview the documentation site (requires zensical: pip install zensical)
```

## Project Structure

```
perles/
├── main.go                 # Entry point
├── cmd/                    # Cobra CLI commands (root, init, daemon, workflows, themes, ...)
├── internal/
│   ├── app/                # Root application model
│   ├── beads/              # beads backend (Dolt connection + client, embedded-Dolt snapshot backend, bd CLI executor)
│   │   └── bql/            # BQL lexer, parser, validator and executor
│   ├── beadsrust/          # beads_rust backend
│   ├── task/               # Backend-agnostic issue types and interfaces
│   ├── config/             # Configuration handling
│   ├── keys/               # Keybinding definitions
│   ├── mode/               # Mode controllers (kanban, search, dashboard, playground)
│   ├── orchestration/      # Multi-agent AI orchestration (providers, v2 processor, control plane, MCP)
│   ├── registry/           # Workflow registration registry (domain and application layers)
│   ├── templates/          # Embedded built-in workflow templates
│   ├── frontend/           # HTTP handlers for the web session viewer
│   ├── pubsub/             # Generic pub/sub event broker
│   ├── watcher/            # File watcher for auto-refresh
│   └── ui/                 # UI components
│       ├── board/          # Kanban board and column components
│       ├── coleditor/      # Column editor modal
│       ├── details/        # Issue detail view
│       ├── tree/           # Dependency tree view
│       ├── modals/         # Help overlay, issue editor, comment editor
│       ├── shared/         # Reusable components (modal, picker, colorpicker, toaster, formmodal, vimtextarea, chatpanel, ...)
│       ├── styles/         # Theme system and shared lip gloss styles
│       └── nobeads/, outdated/, unsupported/, serverdown/  # Screens shown when the board can't start
├── communityworkflows/     # Embedded community workflow templates
├── frontend/               # React session viewer (built into frontend/dist/ and embedded in the binary)
├── docs/                   # Documentation site sources
└── Makefile
```

See [AGENTS.md](AGENTS.md) for a more detailed tour of the codebase.

## Code Style

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable and function names
- Keep functions focused and under 50 lines when possible
- Add comments for exported functions
- Write tests for new functionality

## Testing

### Running Tests

```bash
make test              # All tests
make test-update       # Update golden files
```

### Real-bd Tests (Opt-in)

Two test suites run the `bd` on your `PATH` against throwaway embedded Dolt workspaces created under your home directory and removed afterwards. They never touch the repository's `.beads/`. `make test` and CI skip them; opt in with an environment variable:

```bash
# Snapshot backend parity with bd's own answers (blocked, ready, wisps, exclude_owners, comments)
PERLES_BD_INTEGRATION=1 go test ./internal/beads/adapter/ -run Integration -v

# Lock contention benchmark: 5,000 issues, external bd updates while perles rebuilds (a minute or two)
PERLES_BD_CONTENTION=1 go test ./internal/beads/adapter/ -run Contention -count=1 -v
```

Both skip, with a logged reason, under `-short`, without `bd` on `PATH`, with bd older than v1.0.5, or when bd refuses the throwaway workspace's location as unsafe. Some checks skip on older bd too: the `export.exclude_owners` and `bd schema` checks need bd v1.2.0+, and the exact `bd blocked --flags --raw` path needs a main-branch bd build newer than v1.3.2 (released versions use the `bd blocked --json` listing). The contention benchmark also needs `lsof` for its lock check, and skips only that check without it.

### Golden Tests

We use [teatest](https://github.com/charmbracelet/x/tree/main/exp/teatest) for TUI snapshot testing. These tests compare rendered output against golden files stored in `testdata/` directories.

If you intentionally change UI output:

1. Run `make test` to see failures
2. Review the diff to ensure changes are intentional
3. Run `make test-update` to update golden files
4. Commit the updated golden files with your changes

`make test-update` runs `go test -update` over the packages listed in the Makefile's `test-update` target. If you add golden tests to a package that isn't listed, add it there (or run `go test ./path/to/pkg/... -update`).

## Pull Request Process

1. Document new features in the relevant page under `docs/` (published to [zjrosen.github.io/perles](https://zjrosen.github.io/perles/); preview locally with `make docs`). Update README.md only for install or quick-start changes.
2. Add tests for new functionality
3. Ensure tests and lint pass (`make test` and `make lint`)
4. Update documentation as needed
5. Request review from maintainers

## Reporting Issues

- Use GitHub Issues for bug reports and feature requests
- Include perles version (`perles --version`)
- Include your task backend and its version (`bd --version` or `br --version`)
- Include Go version (`go version`) if you built from source
- For orchestration bugs, include the AI provider (claude, amp, codex, cursor, gemini, or opencode)
- Provide steps to reproduce bugs
- Attach relevant lines from a debug log if you can (`perles -d` writes `debug.log` in the current directory)
- Include relevant configuration if applicable

## Questions?

If you have questions about contributing, feel free to open a discussion or issue on GitHub.
