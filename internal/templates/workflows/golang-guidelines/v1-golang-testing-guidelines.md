# Golang Testing Guidelines

This template provides testing guidelines for Go projects.

## Test Organization

- Place tests in `*_test.go` files alongside the code
- Use table-driven tests for multiple cases
- Keep test functions focused on single behaviors

## Test Naming

- Use `TestFunction_Scenario` naming convention
- Be descriptive about what is being tested
- Include edge cases in test names

## Assertions

- Use `github.com/stretchr/testify/require` for assertions
- Prefer `require.Equal` over manual comparison
- Use `require.NoError` for error checks

## Mocking

- Use interfaces for dependencies
- Generate mocks with mockery
- Prefer real implementations when practical
