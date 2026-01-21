package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewArtifact(t *testing.T) {
	filename := "research.md"
	artifact := NewArtifact(filename)

	require.Equal(t, filename, artifact.Filename())
}

func TestArtifact_Filename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "markdown file",
			filename: "research.md",
		},
		{
			name:     "json file",
			filename: "tasks.json",
		},
		{
			name:     "file with path",
			filename: "output/report.md",
		},
		{
			name:     "empty filename",
			filename: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := NewArtifact(tt.filename)
			require.Equal(t, tt.filename, artifact.Filename())
		})
	}
}

func TestArtifact_CaseSensitive(t *testing.T) {
	lower := NewArtifact("research.md")
	upper := NewArtifact("Research.md")

	// Artifacts are compared by filename (case-sensitive)
	require.NotEqual(t, lower.Filename(), upper.Filename())
}
