package codex

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func TestCodexClient_Type(t *testing.T) {
	c := NewClient()
	require.Equal(t, client.ClientCodex, c.Type())
}

func TestCodexClient_NewClient(t *testing.T) {
	c := NewClient()
	require.NotNil(t, c)
	require.IsType(t, &CodexClient{}, c)
}

func TestCodexClient_Registration(t *testing.T) {
	// Verify Codex client is registered and can be created via NewClient
	c, err := client.NewClient(client.ClientCodex)
	require.NoError(t, err)
	require.NotNil(t, c)
	require.Equal(t, client.ClientCodex, c.Type())
}

func TestCodexClient_IsRegistered(t *testing.T) {
	// Verify using the IsRegistered helper function
	require.True(t, client.IsRegistered(client.ClientCodex), "ClientCodex should be registered via init()")

	// Also verify the client can be created via the registry (proves init() ran)
	c, err := client.NewClient(client.ClientCodex)
	require.NoError(t, err, "ClientCodex should be registered via init()")
	require.NotNil(t, c)
}

func TestCodexClient_AppearsInRegisteredClients(t *testing.T) {
	// Verify Codex appears in RegisteredClients() output
	registeredTypes := client.RegisteredClients()
	require.Contains(t, registeredTypes, client.ClientCodex, "ClientCodex should appear in RegisteredClients()")
}

func TestCodexClient_ImplementsInterface(t *testing.T) {
	// Compile-time check is already in client.go, but we can also verify at runtime
	var _ client.HeadlessClient = (*CodexClient)(nil)

	c := NewClient()
	var headlessClient client.HeadlessClient = c
	require.NotNil(t, headlessClient)
}
