package client

import (
	"sync"
)

// AgentProvider creates and configures AI agent processes.
// It combines the client factory with provider-specific configuration,
// providing a single object that can be passed through the orchestration
// layers instead of separate client + extensions.
type AgentProvider interface {
	// Type returns the provider type (claude, amp, codex, gemini).
	Type() ClientType

	// Client returns the HeadlessClient, creating it lazily on first call.
	// The client is cached and reused for subsequent calls.
	Client() (HeadlessClient, error)

	// Extensions returns the provider-specific configuration.
	// This includes model settings, modes, and other provider-specific options.
	Extensions() map[string]any
}

// agentProvider is the concrete implementation of AgentProvider.
type agentProvider struct {
	clientType ClientType
	extensions map[string]any

	// Lazy client creation
	client     HeadlessClient
	clientOnce sync.Once
	clientErr  error
}

// NewAgentProvider creates a provider for the given client type with extensions.
func NewAgentProvider(clientType ClientType, extensions map[string]any) AgentProvider {
	if extensions == nil {
		extensions = make(map[string]any)
	}
	return &agentProvider{
		clientType: clientType,
		extensions: extensions,
	}
}

// Type returns the provider type (claude, amp, codex, gemini).
func (p *agentProvider) Type() ClientType {
	return p.clientType
}

// Client returns the HeadlessClient, creating it lazily on first call.
// The client is cached and reused for subsequent calls.
// Returns an error if the client type is not registered.
func (p *agentProvider) Client() (HeadlessClient, error) {
	p.clientOnce.Do(func() {
		p.client, p.clientErr = NewClient(p.clientType)
	})
	return p.client, p.clientErr
}

// Extensions returns the provider-specific configuration.
// This includes model settings, modes, and other provider-specific options.
func (p *agentProvider) Extensions() map[string]any {
	return p.extensions
}
