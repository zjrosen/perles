package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAgentProvider(t *testing.T) {
	t.Run("creates provider with type and extensions", func(t *testing.T) {
		extensions := map[string]any{
			ExtClaudeModel: "opus",
		}
		p := NewAgentProvider(ClientClaude, extensions)

		assert.Equal(t, ClientClaude, p.Type())
		assert.Equal(t, "opus", p.Extensions()[ExtClaudeModel])
	})

	t.Run("handles nil extensions", func(t *testing.T) {
		p := NewAgentProvider(ClientAmp, nil)

		assert.Equal(t, ClientAmp, p.Type())
		assert.NotNil(t, p.Extensions())
		assert.Empty(t, p.Extensions())
	})
}

func TestAgentProvider_Client(t *testing.T) {
	t.Run("returns error for unregistered client type", func(t *testing.T) {
		p := NewAgentProvider("unknown", nil)

		client, err := p.Client()

		assert.Error(t, err)
		assert.Nil(t, client)
		assert.ErrorIs(t, err, ErrUnknownClientType)
	})

	t.Run("returns registered client", func(t *testing.T) {
		// Register a mock client for testing
		RegisterClient(ClientMock, func() HeadlessClient {
			return &mockHeadlessClient{}
		})
		defer func() {
			delete(clientRegistry, ClientMock)
		}()

		p := NewAgentProvider(ClientMock, nil)

		client, err := p.Client()

		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, ClientMock, client.Type())
	})

	t.Run("caches client on subsequent calls", func(t *testing.T) {
		callCount := 0
		RegisterClient(ClientMock, func() HeadlessClient {
			callCount++
			return &mockHeadlessClient{}
		})
		defer func() {
			delete(clientRegistry, ClientMock)
		}()

		p := NewAgentProvider(ClientMock, nil)

		client1, _ := p.Client()
		client2, _ := p.Client()

		assert.Same(t, client1, client2)
		assert.Equal(t, 1, callCount, "factory should only be called once")
	})
}

// mockHeadlessClient is a test double for HeadlessClient.
type mockHeadlessClient struct {
	spawnFunc func(context.Context, Config) (HeadlessProcess, error)
}

func (m *mockHeadlessClient) Type() ClientType {
	return ClientMock
}

func (m *mockHeadlessClient) Spawn(ctx context.Context, cfg Config) (HeadlessProcess, error) {
	if m.spawnFunc != nil {
		return m.spawnFunc(ctx, cfg)
	}
	return &mockHeadlessProcess{}, nil
}

// mockHeadlessProcess is a test double for HeadlessProcess.
type mockHeadlessProcess struct{}

func (m *mockHeadlessProcess) Events() <-chan OutputEvent { return make(chan OutputEvent) }
func (m *mockHeadlessProcess) Errors() <-chan error       { return make(chan error) }
func (m *mockHeadlessProcess) SessionRef() string         { return "" }
func (m *mockHeadlessProcess) Status() ProcessStatus      { return StatusRunning }
func (m *mockHeadlessProcess) IsRunning() bool            { return true }
func (m *mockHeadlessProcess) WorkDir() string            { return "/mock" }
func (m *mockHeadlessProcess) PID() int                   { return 12345 }
func (m *mockHeadlessProcess) Cancel() error              { return nil }
func (m *mockHeadlessProcess) Wait() error                { return nil }
