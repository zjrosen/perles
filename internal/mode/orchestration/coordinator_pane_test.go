package orchestration

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildCoordinatorTitle_WithPort(t *testing.T) {
	m := New(Config{})
	m.mcpPort = 8467

	title := m.buildCoordinatorTitle()

	// Should contain port in muted style
	require.Contains(t, title, "COORDINATOR")
	require.Contains(t, title, "(8467)")
}

func TestBuildCoordinatorTitle_NoPort(t *testing.T) {
	m := New(Config{})
	m.mcpPort = 0

	title := m.buildCoordinatorTitle()

	// Should NOT contain port when port is 0
	require.Contains(t, title, "COORDINATOR")
	require.NotContains(t, title, "(")
	require.NotContains(t, title, ")")
}

func TestBuildCoordinatorTitle_MutedStyleApplied(t *testing.T) {
	m := New(Config{})
	m.mcpPort = 12345

	title := m.buildCoordinatorTitle()

	// The port should be styled with TitleContextStyle (muted).
	// Since Lipgloss applies ANSI escape codes, we just verify the structure
	// contains both COORDINATOR and the port number.
	require.Contains(t, title, "COORDINATOR")
	require.Contains(t, title, "12345")
}
