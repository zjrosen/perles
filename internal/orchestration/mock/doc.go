// Package mock provides mock implementations of the client interfaces for testing.
//
// This package provides MockClient and MockProcess that implement the
// client.HeadlessClient and client.HeadlessProcess interfaces respectively.
// These mocks allow tests to simulate AI process behavior without spawning
// real Claude or Amp processes.
//
// Note: This package complements the mockery-generated mocks in internal/mocks.
// The mockery mocks are useful for strict expectation-based testing, while
// this package provides more flexible, state-based mocks with helper methods
// for common testing patterns.
//
// # MockClient
//
// Client implements client.HeadlessClient with configurable behavior:
//
//	// Create a mock client
//	mockClient := mock.NewClient()
//
//	// Configure spawn behavior
//	mockClient.SpawnFunc = func(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
//	    proc := mock.NewProcess()
//	    proc.SetSessionID("test-session")
//	    return proc, nil
//	}
//
// # MockProcess
//
// Process implements client.HeadlessProcess with injectable events and errors:
//
//	proc := mock.NewProcess()
//
//	// Send an init event
//	proc.SendInitEvent("sess-123", "/work")
//
//	// Send a text message
//	proc.SendTextEvent("Hello!")
//
//	// Complete the process
//	proc.Complete()
//
// # Registration
//
// The mock client is automatically registered with the client package
// when the mock package is imported:
//
//	import _ "perles/internal/orchestration/mock"
//
//	client, err := client.NewClient(client.ClientMock)
package mock
