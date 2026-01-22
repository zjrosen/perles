package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewArtifact(t *testing.T) {
	key := "research"
	filename := "research.md"
	artifact := NewArtifact(key, filename)

	require.Equal(t, key, artifact.Key())
	require.Equal(t, filename, artifact.Filename())
}

func TestArtifact_KeyAndFilename(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		filename string
	}{
		{
			name:     "simple artifact",
			key:      "research",
			filename: "research.md",
		},
		{
			name:     "dynamic filename",
			key:      "report",
			filename: "{{.Date}}-report.md",
		},
		{
			name:     "file with path",
			key:      "output",
			filename: "output/report.md",
		},
		{
			name:     "empty values",
			key:      "",
			filename: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := NewArtifact(tt.key, tt.filename)
			require.Equal(t, tt.key, artifact.Key())
			require.Equal(t, tt.filename, artifact.Filename())
		})
	}
}

func TestArtifact_CaseSensitive(t *testing.T) {
	lower := NewArtifact("research", "research.md")
	upper := NewArtifact("Research", "Research.md")

	// Keys and filenames are case-sensitive
	require.NotEqual(t, lower.Key(), upper.Key())
	require.NotEqual(t, lower.Filename(), upper.Filename())
}
