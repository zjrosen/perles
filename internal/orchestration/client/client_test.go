package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientCodex_Constant(t *testing.T) {
	require.Equal(t, ClientType("codex"), ClientCodex)
}

func TestClientType_AllConstants(t *testing.T) {
	// Verify all client type constants are defined and have expected values
	require.Equal(t, ClientType("claude"), ClientClaude)
	require.Equal(t, ClientType("amp"), ClientAmp)
	require.Equal(t, ClientType("codex"), ClientCodex)
	require.Equal(t, ClientType("gemini"), ClientGemini)
	require.Equal(t, ClientType("opencode"), ClientOpenCode)
	require.Equal(t, ClientType("mock"), ClientMock)
}

func TestClientOpenCode_Constant(t *testing.T) {
	require.Equal(t, ClientType("opencode"), ClientOpenCode)
}
