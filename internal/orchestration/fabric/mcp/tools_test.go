package mcp

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFabricToolSchema_IncludesObserverChannel(t *testing.T) {
	// Test that ToolFabricSend includes "observer" in channel enum
	t.Run("ToolFabricSend", func(t *testing.T) {
		channelProp := ToolFabricSend.InputSchema.Properties["channel"]
		require.NotNil(t, channelProp, "channel property should exist")
		require.True(t, slices.Contains(channelProp.Enum, "observer"),
			"ToolFabricSend channel enum should include 'observer'")
	})

	// Test that ToolFabricSubscribe includes "observer" in channel enum
	t.Run("ToolFabricSubscribe", func(t *testing.T) {
		channelProp := ToolFabricSubscribe.InputSchema.Properties["channel"]
		require.NotNil(t, channelProp, "channel property should exist")
		require.True(t, slices.Contains(channelProp.Enum, "observer"),
			"ToolFabricSubscribe channel enum should include 'observer'")
	})

	// Test that ToolFabricUnsubscribe includes "observer" in channel enum
	t.Run("ToolFabricUnsubscribe", func(t *testing.T) {
		channelProp := ToolFabricUnsubscribe.InputSchema.Properties["channel"]
		require.NotNil(t, channelProp, "channel property should exist")
		require.True(t, slices.Contains(channelProp.Enum, "observer"),
			"ToolFabricUnsubscribe channel enum should include 'observer'")
	})

	// Test that ToolFabricHistory includes "observer" in channel enum
	t.Run("ToolFabricHistory", func(t *testing.T) {
		channelProp := ToolFabricHistory.InputSchema.Properties["channel"]
		require.NotNil(t, channelProp, "channel property should exist")
		require.True(t, slices.Contains(channelProp.Enum, "observer"),
			"ToolFabricHistory channel enum should include 'observer'")
	})
}
