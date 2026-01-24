package client

import (
	"sync"
)

// AgentProviderRole identifies the role for an AgentProvider.
type AgentProviderRole string

const (
	// RoleCoordinator is the coordinator role.
	RoleCoordinator = AgentProviderRole("COORDINATOR")
	// RoleWorker is the worker role.
	RoleWorker = AgentProviderRole("WORKER")
)

// AgentProviders maps roles to their providers.
// This is the preferred way to pass agent configuration through the orchestration stack.
type AgentProviders map[AgentProviderRole]AgentProvider

// Coordinator returns the coordinator provider.
// Panics if not set.
func (p AgentProviders) Coordinator() AgentProvider {
	if provider, ok := p[RoleCoordinator]; ok {
		return provider
	}
	panic("AgentProviders: coordinator provider not set")
}

// Worker returns the worker provider, falling back to coordinator if not set.
func (p AgentProviders) Worker() AgentProvider {
	if provider, ok := p[RoleWorker]; ok {
		return provider
	}
	return p.Coordinator()
}

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
