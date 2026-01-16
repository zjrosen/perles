package client

import (
	"context"
	"fmt"
)

// ClientType identifies the headless client provider.
type ClientType string

const (
	// ClientClaude is the Claude Code CLI client.
	ClientClaude ClientType = "claude"
	// ClientAmp is the Amp CLI client.
	ClientAmp ClientType = "amp"
	// ClientCodex is the OpenAI Codex CLI client.
	ClientCodex ClientType = "codex"
	// ClientGemini is the Gemini CLI client.
	ClientGemini ClientType = "gemini"
	// ClientOpenCode is the OpenCode CLI client.
	ClientOpenCode ClientType = "opencode"
	// ClientMock is a mock client for testing.
	ClientMock ClientType = "mock"
)

// HeadlessClient is a factory for spawning headless AI processes.
// Implementations handle the provider-specific details of process creation
// and configuration.
type HeadlessClient interface {
	// Type returns the client type identifier.
	Type() ClientType

	// Spawn creates and starts a headless process.
	// If cfg.SessionID is set, resumes an existing session.
	// If cfg.SessionID is empty, creates a new session.
	// Context is used for cancellation and timeout control.
	Spawn(ctx context.Context, cfg Config) (HeadlessProcess, error)
}

// ErrUnknownClientType is returned when an unknown client type is requested.
var ErrUnknownClientType = fmt.Errorf("unknown client type")

// ClientRegistry holds registered client factories.
// Use RegisterClient to add new client types.
var clientRegistry = make(map[ClientType]func() HeadlessClient)

// RegisterClient registers a client factory for the given type.
// This should be called in init() functions of provider packages.
func RegisterClient(clientType ClientType, factory func() HeadlessClient) {
	clientRegistry[clientType] = factory
}

// NewClient creates a HeadlessClient for the given type.
// Returns ErrUnknownClientType if the type is not registered.
func NewClient(clientType ClientType) (HeadlessClient, error) {
	factory, ok := clientRegistry[clientType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownClientType, clientType)
	}
	return factory(), nil
}

// RegisteredClients returns a slice of all registered client types.
func RegisteredClients() []ClientType {
	types := make([]ClientType, 0, len(clientRegistry))
	for t := range clientRegistry {
		types = append(types, t)
	}
	return types
}

// IsRegistered returns true if the given client type has been registered.
func IsRegistered(clientType ClientType) bool {
	_, ok := clientRegistry[clientType]
	return ok
}
