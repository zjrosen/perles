# Golang Coding Guidelines

This template provides coding guidelines for Go projects.

## File Organization

- Group related functionality into packages
- Use short, descriptive package names
- Follow Go's standard library conventions

## Naming Conventions

- Use camelCase for unexported identifiers
- Use PascalCase for exported identifiers
- Use short names for local variables
- Use descriptive names for exported functions

## Error Handling

- Always check errors
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Return errors, don't panic

## Code Style

- Run `gofmt` and `goimports` on all code
- Follow golangci-lint recommendations
- Keep functions focused and small
