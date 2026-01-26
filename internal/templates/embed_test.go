package templates

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTemplate_NoHardcodedPaths(t *testing.T) {
	fsys := RegistryFS()

	var matches []string
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "docs/proposals") {
			matches = append(matches, path)
		}
		return nil
	})

	require.NoError(t, err)
	require.Empty(t, matches, "found hardcoded docs/proposals in templates: %v", matches)
}
