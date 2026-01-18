package gemini

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func TestGeminiClient_Type(t *testing.T) {
	c := NewClient()
	require.Equal(t, client.ClientGemini, c.Type())
}

func TestGeminiClient_NewClient(t *testing.T) {
	c := NewClient()
	require.NotNil(t, c)
	require.IsType(t, &GeminiClient{}, c)
}

func TestGeminiClient_Registration(t *testing.T) {
	// Verify Gemini client is registered and can be created via NewClient
	c, err := client.NewClient(client.ClientGemini)
	require.NoError(t, err)
	require.NotNil(t, c)
	require.Equal(t, client.ClientGemini, c.Type())
}

func TestGeminiClient_IsRegistered(t *testing.T) {
	// Verify using the IsRegistered helper function
	require.True(t, client.IsRegistered(client.ClientGemini), "ClientGemini should be registered via init()")

	// Also verify the client can be created via the registry (proves init() ran)
	c, err := client.NewClient(client.ClientGemini)
	require.NoError(t, err, "ClientGemini should be registered via init()")
	require.NotNil(t, c)
}

func TestGeminiClient_AppearsInRegisteredClients(t *testing.T) {
	// Verify Gemini appears in RegisteredClients() output
	registeredTypes := client.RegisteredClients()
	require.Contains(t, registeredTypes, client.ClientGemini, "ClientGemini should appear in RegisteredClients()")
}

func TestGeminiClient_ImplementsInterface(t *testing.T) {
	// Compile-time check is already in client.go, but we can also verify at runtime
	var _ client.HeadlessClient = (*GeminiClient)(nil)

	c := NewClient()
	var headlessClient client.HeadlessClient = c
	require.NotNil(t, headlessClient)
}
