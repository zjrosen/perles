package opencode

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func TestOpenCodeClient_Type(t *testing.T) {
	c := NewClient()
	require.Equal(t, client.ClientOpenCode, c.Type())
}

func TestOpenCodeClient_NewClient(t *testing.T) {
	c := NewClient()
	require.NotNil(t, c)
	require.IsType(t, &OpenCodeClient{}, c)
}

func TestOpenCodeClient_Registration(t *testing.T) {
	// Verify OpenCode client is registered and can be created via NewClient
	c, err := client.NewClient(client.ClientOpenCode)
	require.NoError(t, err)
	require.NotNil(t, c)
	require.Equal(t, client.ClientOpenCode, c.Type())
}

func TestOpenCodeClient_IsRegistered(t *testing.T) {
	// Verify using the IsRegistered helper function
	require.True(t, client.IsRegistered(client.ClientOpenCode), "ClientOpenCode should be registered via init()")

	// Also verify the client can be created via the registry (proves init() ran)
	c, err := client.NewClient(client.ClientOpenCode)
	require.NoError(t, err, "ClientOpenCode should be registered via init()")
	require.NotNil(t, c)
}

func TestOpenCodeClient_AppearsInRegisteredClients(t *testing.T) {
	// Verify OpenCode appears in RegisteredClients() output
	registeredTypes := client.RegisteredClients()
	require.Contains(t, registeredTypes, client.ClientOpenCode, "ClientOpenCode should appear in RegisteredClients()")
}

func TestOpenCodeClient_ImplementsInterface(t *testing.T) {
	// Compile-time check is already in client.go, but we can also verify at runtime
	var _ client.HeadlessClient = (*OpenCodeClient)(nil)

	c := NewClient()
	var headlessClient client.HeadlessClient = c
	require.NotNil(t, headlessClient)
}
