# Contributing to Perles

Thank you for your interest in contributing to Perles! This document provides guidelines for contributing to the project.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/perles.git`
3. Create a branch: `git checkout -b feature/your-feature`
4. Make your changes
5. Run tests: `make test`
6. Commit with a descriptive message
7. Push and create a Pull Request

## Development Setup

### Prerequisites

- Go 1.24+
- A beads-enabled project for testing (`.beads/` directory with `beads.db`)

### Building

```bash
make build        # Build binary
make test         # Run tests
make test-v       # Verbose tests
make test-update  # Update golden files
```

## Project Structure

```
perles/
├── cmd/                 # CLI entry point
├── internal/
│   ├── app/            # Root application model
│   ├── beads/          # Database client and CLI executor
│   ├── bql/            # BQL parser and executor
│   ├── config/         # Configuration handling
│   ├── keys/           # Keybinding definitions
│   ├── mode/           # Mode controllers (kanban, search)
│   ├── watcher/        # File watcher for auto-refresh
│   └── ui/             # UI components
│       ├── board/      # Kanban board and column components
│       ├── bqlinput/   # BQL input with syntax highlighting
│       ├── coleditor/  # Column editor modal
│       ├── colorpicker/# Color picker for columns
│       ├── details/    # Issue detail view
│       ├── help/       # Help overlay
│       ├── labeleditor/# Label editor modal
│       ├── modal/      # Generic modal component
│       ├── picker/     # Status/priority picker
│       ├── styles/     # Shared lip gloss styles
│       ├── toaster/    # Toast notifications
│       └── viewselector/# View switcher component
└── Makefile
```

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

### Golden Tests

We use [teatest](https://github.com/charmbracelet/x/tree/main/exp/teatest) for TUI snapshot testing. These tests compare rendered output against golden files stored in `testdata/` directories.

If you intentionally change UI output:

1. Run `make test` to see failures
2. Review the diff to ensure changes are intentional
3. Run `make test-update` to update golden files
4. Commit the updated golden files with your changes

## Pull Request Process

1. Update README.md if adding new features
2. Add tests for new functionality
3. Ensure all tests pass (`make test`)
4. Update documentation as needed
5. Request review from maintainers

## Reporting Issues

- Use GitHub Issues for bug reports and feature requests
- Include perles version (`perles --version`)
- Include Go version (`go version`)
- Provide steps to reproduce bugs
- Include relevant configuration if applicable

## Questions?

If you have questions about contributing, feel free to open a discussion or issue on GitHub.
