package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSlugObserver_Constant(t *testing.T) {
	require.Equal(t, "observer", SlugObserver)
}

func TestSlugObserver_InFixedChannels(t *testing.T) {
	channels := FixedChannels()

	// Find the observer channel
	var found *Thread
	for i := range channels {
		if channels[i].Slug == SlugObserver {
			found = &channels[i]
			break
		}
	}

	require.NotNil(t, found, "SlugObserver should be in FixedChannels()")
	require.Equal(t, ThreadChannel, found.Type, "Observer should be a channel type")
	require.Equal(t, "Observer", found.Title, "Observer channel title should be 'Observer'")
	require.Equal(t, "User-to-observer communication", found.Purpose, "Observer channel purpose should match")
}
