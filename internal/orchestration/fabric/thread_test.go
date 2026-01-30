package fabric

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
)

func TestThread_TypeChecks(t *testing.T) {
	tests := []struct {
		name       string
		thread     domain.Thread
		isChannel  bool
		isMessage  bool
		isArtifact bool
	}{
		{
			name:       "channel",
			thread:     domain.Thread{Type: domain.ThreadChannel},
			isChannel:  true,
			isMessage:  false,
			isArtifact: false,
		},
		{
			name:       "message",
			thread:     domain.Thread{Type: domain.ThreadMessage},
			isChannel:  false,
			isMessage:  true,
			isArtifact: false,
		},
		{
			name:       "artifact",
			thread:     domain.Thread{Type: domain.ThreadArtifact},
			isChannel:  false,
			isMessage:  false,
			isArtifact: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.isChannel, tt.thread.Type == domain.ThreadChannel)
			require.Equal(t, tt.isMessage, tt.thread.Type == domain.ThreadMessage)
			require.Equal(t, tt.isArtifact, tt.thread.Type == domain.ThreadArtifact)
		})
	}
}

func TestThread_IsArchived(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		archivedAt *time.Time
		want       bool
	}{
		{
			name:       "not archived",
			archivedAt: nil,
			want:       false,
		},
		{
			name:       "archived",
			archivedAt: &now,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread := domain.Thread{ArchivedAt: tt.archivedAt}
			require.Equal(t, tt.want, thread.IsArchived())
		})
	}
}

func TestThread_HasMention(t *testing.T) {
	thread := domain.Thread{
		Mentions: []string{"COORDINATOR", "WORKER.1"},
	}

	require.True(t, thread.HasMention("COORDINATOR"))
	require.True(t, thread.HasMention("WORKER.1"))
	require.False(t, thread.HasMention("WORKER.2"))
	require.False(t, thread.HasMention(""))
}

func TestFixedChannels(t *testing.T) {
	channels := domain.FixedChannels()

	require.Len(t, channels, 6)

	slugs := make([]string, len(channels))
	for i, ch := range channels {
		slugs[i] = ch.Slug
		require.Equal(t, domain.ThreadChannel, ch.Type)
		require.NotEmpty(t, ch.Title)
		require.NotEmpty(t, ch.Purpose)
	}

	require.Contains(t, slugs, domain.SlugRoot)
	require.Contains(t, slugs, domain.SlugSystem)
	require.Contains(t, slugs, domain.SlugTasks)
	require.Contains(t, slugs, domain.SlugPlanning)
	require.Contains(t, slugs, domain.SlugGeneral)
	require.Contains(t, slugs, domain.SlugObserver)
}
